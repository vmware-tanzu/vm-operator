/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinereplicaset

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	apierrorsutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/prober"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

var (
	// stateConfirmationTimeout is the amount of time allowed to wait for desired
	// state. This is used to ensure a VirtualMachine reaches its desired state
	// within a time period.
	stateConfirmationTimeout = 10 * time.Second

	// stateConfirmationInterval is the amount of time between polling
	// attempts for the desired state.
	stateConfirmationInterval = 100 * time.Millisecond

	// replicaSetKind contains the schema.GroupVersionKind for the VirtualMachineReplicaSet type.
	replicaSetKind = vmopv1.SchemeGroupVersion.WithKind("VirtualMachineReplicaSet")
)

const (
	finalizerName = "virtualmachinereplicaset.vmoperator.vmware.com"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1.VirtualMachineReplicaSet{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)))

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		Owns(&vmopv1.VirtualMachine{}).
		Watches(&vmopv1.VirtualMachine{},
			handler.EnqueueRequestsFromMapFunc(r.VMToReplicaSets(ctx)),
		).
		WithOptions(controller.Options{MaxConcurrentReconciles: ctx.MaxConcurrentReconciles}).
		Complete(r)
}

// VMToReplicaSets is a mapper function to be used to enqueue requests for
// reconciliation for VirtualMachineSetReplicaSets that might adopt a VM.
func (r *Reconciler) VMToReplicaSets(
	ctx *pkgctx.ControllerManagerContext) func(_ context.Context, o client.Object) []reconcile.Request {

	// If the replica VM already has a controller reference, return early. If not,
	// enqueue a reconcile if the VM matches the selector specified in the
	// ReplicaSet.
	// Note that we don't support adopting a replica since that might
	// involve moving it across compute and storage entities -- which may not
	// be possible.
	return func(_ context.Context, o client.Object) []reconcile.Request {
		result := []ctrl.Request{}
		vm, ok := o.(*vmopv1.VirtualMachine)
		if !ok {
			panic(fmt.Sprintf("Expected a VirtualMachine, but got a %T", o))
		}

		// If this VM already has a controller reference, no reconcile requests
		// are queued.
		if metav1.GetControllerOfNoCopy(vm) != nil {
			return nil
		}

		vmrss, err := r.getReplicaSetsForVM(ctx, vm)
		if err != nil {
			ctx.Logger.Error(err, "Failed getting VM ReplicaSets for VM")
			return nil
		}

		for _, rs := range vmrss {
			name := client.ObjectKey{Name: rs.Name, Namespace: rs.Namespace}
			result = append(result, ctrl.Request{NamespacedName: name})
		}

		return result
	}
}

// getReplicaSetsForVM returns the ReplicaSets that have a selector selecting
// the labels of a given VM.
func (r *Reconciler) getReplicaSetsForVM(
	ctx *pkgctx.ControllerManagerContext,
	vm *vmopv1.VirtualMachine) ([]*vmopv1.VirtualMachineReplicaSet, error) {

	if len(vm.Labels) == 0 {
		// VMs managed by VirtualMachineReplicaSet are always expected to have labels set.
		return nil, nil
	}

	rsList := &vmopv1.VirtualMachineReplicaSetList{}
	if err := r.Client.List(ctx, rsList, client.InNamespace(vm.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list VirtualMachineReplicaSets: %w", err)
	}

	var rss []*vmopv1.VirtualMachineReplicaSet
	for _, rs := range rsList.Items {
		rs := rs
		selector, err := metav1.LabelSelectorAsSelector(rs.Spec.Selector)
		if err != nil {
			continue
		}

		if selector.Empty() {
			continue
		}

		if selector.Matches(labels.Set(vm.Labels)) {
			rss = append(rss, &rs)
		}
	}

	return rss, nil
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder) *Reconciler {

	return &Reconciler{
		Context:  ctx,
		Client:   client,
		Logger:   logger,
		Recorder: recorder,
	}
}

// Reconciler reconciles a VirtualMachine object.
type Reconciler struct {
	client.Client
	Context  context.Context
	Logger   logr.Logger
	Recorder record.Recorder

	Prober prober.Manager
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinereplicasets,verbs=create;get;list;watch;update;patch;
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinereplicasets/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	rs := &vmopv1.VirtualMachineReplicaSet{}
	if err := r.Get(ctx, req.NamespacedName, rs); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	rsCtx := &pkgctx.VirtualMachineReplicaSetContext{
		Context:    ctx,
		Logger:     ctrl.Log.WithName("VirtualMachineReplicaSet").WithValues("namespace", rs.Namespace, "name", rs.Name),
		ReplicaSet: rs,
	}

	patchHelper, err := patch.NewHelper(rs, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper for %s: %w", rsCtx.String(), err)
	}

	defer func() {
		if err := patchHelper.Patch(ctx, rs); err != nil {
			if reterr == nil {
				reterr = err
			}
			rsCtx.Logger.Error(err, "patch failed")
		}
	}()

	if !rs.DeletionTimestamp.IsZero() {
		err := r.ReconcileDelete(rsCtx)
		if err != nil {
			rsCtx.Logger.Error(err, "Failed to reconcile VirtualMachineReplicaSet")
		}

		return ctrl.Result{}, err
	}

	result, err := r.ReconcileNormal(rsCtx)
	if err != nil {
		rsCtx.Logger.Error(err, "Failed to reconcile VirtualMachineReplicaSet")
		return ctrl.Result{}, err
	}

	return result, nil
}

func (r *Reconciler) ReconcileDelete(ctx *pkgctx.VirtualMachineReplicaSetContext) error {
	ctx.Logger.Info("Reconciling VirtualMachineReplicaSet Deletion")

	if controllerutil.ContainsFinalizer(ctx.ReplicaSet, finalizerName) {
		defer func() {
			r.Recorder.EmitEvent(ctx.ReplicaSet, "Delete", nil, false)
		}()

		controllerutil.RemoveFinalizer(ctx.ReplicaSet, finalizerName)
	}

	return nil
}

// adoptOrphan sets the VirtualMachineReplicaSet as a controller OwnerReference
// to the VirtualMachine.
func (r *Reconciler) adoptOrphan(
	ctx *pkgctx.VirtualMachineReplicaSetContext,
	rs *vmopv1.VirtualMachineReplicaSet,
	vm *vmopv1.VirtualMachine) error {

	patch := client.MergeFrom(vm.DeepCopy())
	newRef := *metav1.NewControllerRef(rs, vmopv1.SchemeGroupVersion.WithKind("VirtualMachineReplicaSet"))
	vm.OwnerReferences = append(vm.OwnerReferences, newRef)
	return r.Client.Patch(ctx, vm, patch)
}

func (r *Reconciler) ReconcileNormal(ctx *pkgctx.VirtualMachineReplicaSetContext) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(ctx.ReplicaSet, finalizerName) {
		// Set the finalizer and return so the object is patched immediately.
		controllerutil.AddFinalizer(ctx.ReplicaSet, finalizerName)
		return ctrl.Result{}, nil
	}

	ctx.Logger.Info("Reconciling VirtualMachineReplicaSet")

	if ctx.ReplicaSet.Labels == nil {
		ctx.ReplicaSet.Labels = make(map[string]string)
	}

	if ctx.ReplicaSet.Spec.Selector.MatchLabels == nil {
		ctx.ReplicaSet.Spec.Selector.MatchLabels = make(map[string]string)
	}

	if ctx.ReplicaSet.Spec.Template.Labels == nil {
		ctx.ReplicaSet.Spec.Template.Labels = make(map[string]string)
	}

	selectorMap, err := metav1.LabelSelectorAsMap(ctx.ReplicaSet.Spec.Selector)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf(
			"failed to convert VirtualMachineReplicaSet %q label selector to a map: %w",
			ctx.ReplicaSet.Name, err)
	}

	// Get all VirtualMachines linked to this VirtualMachineReplicaSet.
	allVMs := &vmopv1.VirtualMachineList{}
	if err := r.Client.List(ctx,
		allVMs,
		client.InNamespace(ctx.ReplicaSet.Namespace),
		client.MatchingLabels(selectorMap),
	); err != nil {
		return ctrl.Result{},
			fmt.Errorf("failed to list virtual machines matched by the VirtualMachineReplicaSet. selector: %q, err: %v",
				selectorMap, err)
	}

	// Filter out irrelevant VirtualMachines (i.e. IsControlledBy something else) and
	// claim orphaned machines.
	// Note that claim does not mean that we will try to conform the VM to meet the
	// desired spec since that might involve moving the VM across
	// compute and storage boundaries.
	// VirtualMachines in deleted state are not excluded.
	filteredVMs := make([]*vmopv1.VirtualMachine, 0, len(allVMs.Items))
	for idx := range allVMs.Items {
		vm := &allVMs.Items[idx]

		// Attempt to adopt VirtualMachine if it has no controller references.
		if metav1.GetControllerOfNoCopy(vm) == nil {
			if err := r.adoptOrphan(ctx, ctx.ReplicaSet, vm); err != nil {
				ctx.Logger.Error(err, "VirtualMachineReplicaSet failed to adopt VirtualMachine", "vm", vm.Name)
				r.Recorder.Warnf(ctx.ReplicaSet, "FailedAdopt", "Failed to adopt VirtualMachine %q: %v", vm.Name, err)

				continue
			}
			ctx.Logger.Info("Adopted VirtualMachine", "vm", vm.Name)
			r.Recorder.Eventf(ctx.ReplicaSet, "SuccessfulAdopt", "Adopted VirtualMachine %q", vm.Name)
		} else if !metav1.IsControlledBy(vm, ctx.ReplicaSet) {
			// Skip this VirtualMachine if it is controlled by someone else.
			continue
		}

		filteredVMs = append(filteredVMs, vm)
	}

	// If not already present, add a label specifying the VirtualMachineReplicaSet name to VirtualMachines.
	for _, vm := range filteredVMs {
		// Note: MustEqualValue is used here as the value of this label will be
		// a hash if the VirtualMachineReplicaSet name is longer than 63 characters.
		if rsName, ok := vm.Labels[vmopv1.VirtualMachineReplicaSetNameLabel]; ok &&
			MustEqualValue(ctx.ReplicaSet.Name, rsName) {

			continue
		}

		// Label not present, patch the VirtualMachine.
		helper, err := patch.NewHelper(vm, r.Client)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create patch helper to apply %s label to VirtualMachine %q: %w",
				vmopv1.VirtualMachineReplicaSetNameLabel, vm.Name, err)
		}

		// MustFormatValue is used here as the value of this label will be a
		// hash if the VirtualMachineReplicaSet name is longer than max allowed
		// label value length of 63 characters.
		vm.Labels[vmopv1.VirtualMachineReplicaSetNameLabel] = util.MustFormatValue(ctx.ReplicaSet.Name)

		if err := helper.Patch(ctx, vm); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to apply %s label to VirtualMachine %q: %w",
				vmopv1.VirtualMachineReplicaSetNameLabel, vm.Name, err)
		}

		ctx.Logger.V(5).Info(
			"Updated VirtualMachine with the VirtualMachineReplicaSet name label",
			"vm", vm,
			"key", vmopv1.VirtualMachineReplicaSetNameLabel,
			"value", util.MustFormatValue(ctx.ReplicaSet.Name))

		// TODO: Propagate the Deployment label from Deployment to replica set to VirtualMachine if
		// it is set on the replica set.
	}

	// TODO: Handle in place propagation of fields to machines before syncing replicas.

	syncErr := r.syncReplicas(ctx, ctx.ReplicaSet, filteredVMs)

	// Update the status of the VirtualMachineReplicaSet even in case of error
	// since syncing might have resulted in replicas being added or removed.
	r.updateStatus(ctx, ctx.ReplicaSet, filteredVMs)

	if syncErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to sync VirtualMachineReplicaSet replicas: %w", syncErr)
	}

	var replicas int32
	if ctx.ReplicaSet.Spec.Replicas != nil {
		replicas = *ctx.ReplicaSet.Spec.Replicas
	}

	// Queue faster reconciles until all replicas are available.
	if ctx.ReplicaSet.Status.ReadyReplicas != replicas {
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// MustEqualValue returns true if the replica set name equals either the label
// value or its hashed value.
func MustEqualValue(rsName, labelValue string) bool {
	return labelValue == util.MustFormatValue(rsName)
}

// getNewVirtualMachine creates a new VirtualMachine object. The name of the newly created resource is going
// to be created by the API server, we set the generateName field.
func (r *Reconciler) getNewVirtualMachine(rs *vmopv1.VirtualMachineReplicaSet) *vmopv1.VirtualMachine {
	vm := &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", rs.Name),
			// Note: by setting the ownerRef on creation we signal to the
			// VirtualMachine controller that this is not a stand-alone VM.
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(rs, replicaSetKind)},
			Namespace:       rs.Namespace,
			Labels: map[string]string{
				vmopv1.VirtualMachineReplicaSetNameLabel: util.MustFormatValue(rs.Name),
			},
			Annotations: rs.Spec.Template.Annotations,
		},
		Spec: rs.Spec.Template.Spec,
	}

	vm.Labels = getLabelsFromVMReplicaSet(rs)

	vm.Annotations = getAnnotationsFromReplicaSet(rs)

	// TODO: Propagate the VirtualMachineDeploymentNameLabel from VirtualMachineReplicaSet to VirtualMachines if it exists.

	return vm
}

func getLabelsFromVMReplicaSet(rs *vmopv1.VirtualMachineReplicaSet) map[string]string {
	labels := make(map[string]string, len(rs.Spec.Template.Labels))

	// Copy over the labels from the template
	maps.Copy(labels, rs.Spec.Template.Labels)

	// Ensure that the ReplicaSetLabel is always present.
	labels[vmopv1.VirtualMachineReplicaSetNameLabel] = util.MustFormatValue(rs.Name)

	return labels
}

func getAnnotationsFromReplicaSet(rs *vmopv1.VirtualMachineReplicaSet) map[string]string {
	annotations := make(map[string]string, len(rs.Spec.Template.Annotations))

	// Copy over the labels from the template
	maps.Copy(annotations, rs.Spec.Template.Annotations)

	return annotations
}

// syncReplicas scales VirtualMachine resources up or down.
func (r *Reconciler) syncReplicas(
	ctx *pkgctx.VirtualMachineReplicaSetContext,
	rs *vmopv1.VirtualMachineReplicaSet,
	vms []*vmopv1.VirtualMachine) error {

	if rs.Spec.Replicas == nil {
		return fmt.Errorf("the Replicas field in Spec for VirtualMachineReplicaSet %v is nil, this should not be allowed", rs.Name)
	}
	diff := len(vms) - int(*(rs.Spec.Replicas))
	switch {
	case diff < 0:
		diff *= -1
		ctx.Logger.Info("ReplicaSet is scaling up",
			"currentReplicas", len(vms),
			"desiredReplicas", *rs.Spec.Replicas,
			"vmsToBeCreated", diff,
		)

		var (
			vmList []*vmopv1.VirtualMachine
			errs   []error
		)

		for i := 0; i < diff; i++ {
			vm := r.getNewVirtualMachine(rs)
			log := ctx.Logger.WithValues("vm", vm.Name)
			log.Info("Creating VM", "index", i+1, "totalVMsToBeCreated", diff)

			if err := r.Client.Create(ctx, vm); err != nil {
				log.Error(err, "Error while creating VirtualMachine")
				r.Recorder.Warnf(rs, "FailedCreate", "Failed to create VirtualMachine: %v", err)
				errs = append(errs, err)
				conditions.MarkFalse(
					rs,
					vmopv1.VirtualMachinesCreatedCondition,
					vmopv1.VirtualMachineCreationFailedReason,
					err.Error(),
				)
				continue
			}

			log.V(5).Info("Created VM", "index", i+1, "totalVMsToBeCreated", diff)
			r.Recorder.Eventf(rs, "SuccessfulCreate", "Created vm %q", vm.Name)
			vmList = append(vmList, vm)
		}

		if len(errs) > 0 {
			return apierrorsutil.NewAggregate(errs)
		}

		return r.waitForVMCreation(ctx, vmList)
	case diff > 0:
		ctx.Logger.Info("ReplicaSet is scaling down",
			"currentReplicas", len(vms),
			"desiredReplicas", *(rs.Spec.Replicas),
			"vmsToBeCreated", diff,
			"deletePolicy", "oldestFirst",
		)

		deletePriorityFunc, err := getDeletePriorityFunc(rs)
		if err != nil {
			return err
		}

		var errs []error
		vmsToDelete := getMachinesToDeletePrioritized(vms, diff, deletePriorityFunc)
		for i, vm := range vmsToDelete {
			log := ctx.Logger.WithValues("vm", vm.Name)
			if vm.GetDeletionTimestamp().IsZero() {
				log.Info("Deleting VM to scale down replicaset", "index", i+1, "totalVMsToBeDeleted", diff)

				if err := r.Client.Delete(ctx, vm); err != nil {
					log.Error(err, "Unable to delete VM")
					r.Recorder.Warnf(rs, "FailedDelete", "Failed to delete VM %q: %v", vm.Name, err)
					errs = append(errs, err)
					continue
				}
				log.V(5).Info("Deleted VM", "index", i+1, "totalVMsToBeDeleted", diff)
				r.Recorder.Eventf(rs, "SuccessfulDelete", "Deleted VM %q", vm.Name)
			} else {
				log.Info("Waiting for VM to be deleted", "index", i+1, "totalVMsToBeDeleted", diff)
			}
		}

		if len(errs) > 0 {
			return apierrorsutil.NewAggregate(errs)
		}
		return r.waitForVMDeletion(ctx, vmsToDelete)
	}

	return nil
}

func (r *Reconciler) waitForVMDeletion(ctx *pkgctx.VirtualMachineReplicaSetContext, vmList []*vmopv1.VirtualMachine) error {
	for _, vm := range vmList {
		pollErr := wait.PollUntilContextTimeout(ctx, stateConfirmationInterval, stateConfirmationTimeout, false, func(ctx context.Context) (bool, error) {
			m := &vmopv1.VirtualMachine{}
			key := client.ObjectKey{Namespace: vm.Namespace, Name: vm.Name}
			err := r.Client.Get(ctx, key, m)
			if apierrors.IsNotFound(err) || !m.DeletionTimestamp.IsZero() {
				return true, nil
			}
			return false, err
		})

		if pollErr != nil {
			ctx.Logger.Error(pollErr, "Failed waiting for VirtualMachine to be deleted", "vm", vm.Name)
			return fmt.Errorf("failed waiting for VirtualMachine %q to be deleted: %w", vm.Name, pollErr)
		}
	}
	return nil
}

func (r *Reconciler) waitForVMCreation(ctx *pkgctx.VirtualMachineReplicaSetContext, vmList []*vmopv1.VirtualMachine) error {
	for _, vm := range vmList {
		pollErr := wait.PollUntilContextTimeout(ctx, stateConfirmationInterval, stateConfirmationTimeout, false, func(ctx context.Context) (bool, error) {
			key := client.ObjectKey{Namespace: vm.Namespace, Name: vm.Name}
			if err := r.Client.Get(ctx, key, &vmopv1.VirtualMachine{}); err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
				return false, err
			}

			return true, nil
		})

		if pollErr != nil {
			ctx.Logger.Error(pollErr, "Failed waiting for VirtualMachine to be created", "vm", vm.Name)
			return fmt.Errorf("failed waiting for VirtualMachine: %q to be created: %w", vm.Name, pollErr)
		}
	}

	return nil
}

// updateStatus updates the Status field of the VirtualMachineReplicaSet.
func (r *Reconciler) updateStatus(
	ctx *pkgctx.VirtualMachineReplicaSetContext,
	rs *vmopv1.VirtualMachineReplicaSet,
	filteredVMs []*vmopv1.VirtualMachine) {

	newStatus := rs.Status.DeepCopy()

	// Count the number of VMs that are matched by the replica set selector.
	// Note that the matching VMs may have more labels than what is specified
	// in the template.
	fullyLabeledReplicasCount := 0
	readyReplicasCount := 0
	desiredReplicas := *rs.Spec.Replicas
	// Create a selector from labels since that is significantly faster at scale
	// and initializing a selector directly.
	templateLabel := labels.Set(rs.Spec.Template.Labels).AsSelectorPreValidated()

	for _, vm := range filteredVMs {
		if templateLabel.Matches(labels.Set(vm.Labels)) {
			fullyLabeledReplicasCount++
		}

		// TODO: Figure out an equivalent of Ready condition on the VirtualMachine
		// resource so we can populate the ready and available replicas in the Status.
		// For now, we count all replicas as ready and available.
		readyReplicasCount++

	}

	newStatus.Replicas = int32(len(filteredVMs))
	newStatus.FullyLabeledReplicas = int32(fullyLabeledReplicasCount)
	newStatus.ReadyReplicas = int32(readyReplicasCount)

	// Copy the newly calculated status into the VirtualMachineReplicaSet.
	if rs.Status.Replicas != newStatus.Replicas ||
		rs.Status.FullyLabeledReplicas != newStatus.FullyLabeledReplicas ||
		rs.Status.ReadyReplicas != newStatus.ReadyReplicas ||
		rs.Generation != rs.Status.ObservedGeneration {

		ctx.Logger.Info("Updating status",
			"replicaCountOld", rs.Status.Replicas,
			"replicaCountNew", newStatus.Replicas,
			"replicaCountDesired", desiredReplicas,
			"fullyLabeledReplicaCountOld", rs.Status.FullyLabeledReplicas,
			"fullyLabeledReplicaCountNew", newStatus.FullyLabeledReplicas,
			"readyReplicasOld", rs.Status.ReadyReplicas,
			"readyReplicasNew", newStatus.ReadyReplicas,
			"observedGenerationOld", rs.Status.ObservedGeneration,
			"observedGenerationNew", newStatus.ObservedGeneration)

		// Save the generation number we acted on, otherwise we might wrongfully indicate
		// that we've seen a spec update when we retry.
		newStatus.ObservedGeneration = rs.Generation
		newStatus.DeepCopyInto(&rs.Status)
	}

	switch {
	// We are scaling up
	case newStatus.Replicas < desiredReplicas:
		conditions.MarkFalse(
			rs,
			vmopv1.ResizedCondition,
			vmopv1.ScalingUpReason,
			"Scaling up VirtualMachineReplicaSet to %d replicas (currentReplicas %d)",
			desiredReplicas,
			newStatus.Replicas)
	// We are scaling down
	case newStatus.Replicas > desiredReplicas:
		conditions.MarkFalse(
			rs,
			vmopv1.ResizedCondition,
			vmopv1.ScalingDownReason,
			"Scaling down VirtualMachineReplicaSet to %d replicas (currentReplicas %d)",
			desiredReplicas,
			newStatus.Replicas)
		// This means that we have sufficient number of VirtualMachine objects.
		conditions.MarkTrue(rs, vmopv1.VirtualMachinesCreatedCondition)
	// We have reached the desired number of replicas.
	default:
		// Make sure last resize operation is marked as completed.
		// Note that resize is only marked complete when all the VirtualMachine
		// objects have their Ready condition set.

		if newStatus.ReadyReplicas == newStatus.Replicas {
			if conditions.IsFalse(rs, vmopv1.ResizedCondition) {
				ctx.Logger.Info("All the replicas are ready", "replicas", newStatus.ReadyReplicas)
			}
			conditions.MarkTrue(rs, vmopv1.ResizedCondition)
		}
		// This means that we have sufficient number of VirtualMachine objects.
		conditions.MarkTrue(rs, vmopv1.VirtualMachinesCreatedCondition)
	}
	// TODO: Set aggregate condition based on the condition of the individual Virtual Machines
}
