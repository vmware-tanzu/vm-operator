// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinegroup

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apierrorsutil "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

const (
	finalizerName = "vmoperator.vmware.com/virtualmachinegroup"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1.VirtualMachineGroup{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		Watches(&vmopv1.VirtualMachineGroup{},
			handler.EnqueueRequestsFromMapFunc(vmGroupToParentGroupMapperFn())).
		// TODO(sai): Add a mapper function to enqueue requests for VM.Spec.GroupName changes.
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ctx.MaxConcurrentReconciles,
		}).
		Complete(r)
}

// vmGroupToParentGroupMapperFn returns a mapper function that enqueues
// reconcile requests for VirtualMachineGroup when another VirtualMachineGroup's
// Spec.GroupName pointing to it changes.
func vmGroupToParentGroupMapperFn() handler.MapFunc {
	return func(_ context.Context, o client.Object) []reconcile.Request {
		vmGroup, ok := o.(*vmopv1.VirtualMachineGroup)
		if !ok {
			panic(fmt.Sprintf("Expected a VirtualMachineGroup, but got a %T", o))
		}

		var requests []reconcile.Request

		if vmGroup.Spec.GroupName != "" {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKey{
					Namespace: vmGroup.Namespace,
					Name:      vmGroup.Spec.GroupName,
				},
			})
		}

		return requests
	}
}

// NewReconciler returns a new reconciler for VirtualMachineGroup objects.
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

// Reconciler reconciles a VirtualMachineGroup object.
type Reconciler struct {
	client.Client
	Context  context.Context
	Logger   logr.Logger
	Recorder record.Recorder
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinegroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinegroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get;update;patch

// Reconcile reconciles a VirtualMachineGroup object.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)
	ctx = record.WithContext(ctx, r.Recorder)

	vmGroup := &vmopv1.VirtualMachineGroup{}
	if err := r.Get(ctx, req.NamespacedName, vmGroup); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger := ctrl.Log.WithName("VirtualMachineGroup").WithValues("name", req.NamespacedName)

	vmGroupCtx := &pkgctx.VirtualMachineGroupContext{
		Context: ctx,
		Logger:  logger,
		VMGroup: vmGroup,
	}

	patchHelper, err := patch.NewHelper(vmGroup, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper for %s: %w", req.NamespacedName, err)
	}

	defer func() {
		if err := patchHelper.Patch(ctx, vmGroup); err != nil {
			if reterr == nil {
				reterr = err
			}
			vmGroupCtx.Logger.Error(err, "patch failed")
		}
	}()

	if !vmGroup.DeletionTimestamp.IsZero() {
		return r.ReconcileDelete(vmGroupCtx)
	}

	return r.ReconcileNormal(vmGroupCtx)
}

func (r *Reconciler) ReconcileDelete(ctx *pkgctx.VirtualMachineGroupContext) (ctrl.Result, error) {
	ctx.Logger.Info("Reconciling VirtualMachineGroup Deletion")

	if controllerutil.ContainsFinalizer(ctx.VMGroup, finalizerName) {
		controllerutil.RemoveFinalizer(ctx.VMGroup, finalizerName)
	}

	ctx.Logger.Info("Finished Reconciling VirtualMachineGroup Deletion")
	return ctrl.Result{}, nil
}

func (r *Reconciler) ReconcileNormal(ctx *pkgctx.VirtualMachineGroupContext) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(ctx.VMGroup, finalizerName) {
		controllerutil.AddFinalizer(ctx.VMGroup, finalizerName)
		return ctrl.Result{}, nil
	}

	ctx.Logger.Info("Reconciling VirtualMachineGroup")

	defer func(beforeVMGroupStatus *vmopv1.VirtualMachineGroupStatus) {
		if !apiequality.Semantic.DeepEqual(beforeVMGroupStatus, &ctx.VMGroup.Status) {
			ctx.Logger.Info("Finished Reconciling VirtualMachineGroup with updates to the CR",
				"createdTime", ctx.VMGroup.CreationTimestamp, "currentTime", time.Now().Format(time.RFC3339))
		} else {
			ctx.Logger.Info("Finished Reconciling VirtualMachineGroup")
		}
	}(ctx.VMGroup.Status.DeepCopy())

	if err := r.reconcileMembers(ctx); err != nil {
		ctx.Logger.Error(err, "Failed to reconcile group members")
		return ctrl.Result{}, err
	}

	if err := r.reconcilePlacement(ctx); err != nil {
		ctx.Logger.Error(err, "Failed to reconcile group placement")
		return ctrl.Result{}, err
	}

	if err := r.reconcilePowerState(ctx); err != nil {
		ctx.Logger.Error(err, "Failed to reconcile group power state")
		return ctrl.Result{}, err
	}

	setReadyCondition(ctx)

	return ctrl.Result{}, nil
}

// reconcileMembers reconciles all current members of the group and updates
// the group's Status.Members accordingly.
func (r *Reconciler) reconcileMembers(ctx *pkgctx.VirtualMachineGroupContext) error {
	ownerRef := []metav1.OwnerReference{
		{
			APIVersion: ctx.VMGroup.APIVersion,
			Kind:       ctx.VMGroup.Kind,
			Name:       ctx.VMGroup.Name,
			UID:        ctx.VMGroup.UID,
		},
	}

	existingStatuses := make(map[string]*vmopv1.VirtualMachineGroupMemberStatus, len(ctx.VMGroup.Status.Members))
	for i := range ctx.VMGroup.Status.Members {
		ms := &ctx.VMGroup.Status.Members[i]
		key := ms.Kind + "/" + ms.Name
		existingStatuses[key] = ms
	}

	var (
		newStatuses = make([]vmopv1.VirtualMachineGroupMemberStatus, len(ctx.VMGroup.Spec.Members))
		errs        = make([]error, len(ctx.VMGroup.Spec.Members))
	)

	for _, member := range ctx.VMGroup.Spec.Members {
		key := member.Kind + "/" + member.Name

		// Check if we have an existing status to update, or create a new one.
		var ms *vmopv1.VirtualMachineGroupMemberStatus
		if s, ok := existingStatuses[key]; ok {
			ms = s.DeepCopy()
		} else {
			ms = &vmopv1.VirtualMachineGroupMemberStatus{
				Name: member.Name,
				Kind: member.Kind,
			}
		}

		objKey := client.ObjectKey{Namespace: ctx.VMGroup.Namespace, Name: member.Name}
		switch member.Kind {
		case "VirtualMachine":
			vm := &vmopv1.VirtualMachine{}
			if err := r.Get(ctx, objKey, vm); err != nil {
				conditions.MarkError(ms, vmopv1.VirtualMachineGroupMemberConditionGroupLinked, "GetError", err)
				errs = append(errs, err)
				newStatuses = append(newStatuses, *ms)
				continue
			}

			// TODO(sai): Check if the VM.Spec.GroupName is set to VMGroup.Name
			// and append an error if not. Add this check after introducing the
			// VM.Spec.GroupName API.

			patch := client.MergeFrom(vm.DeepCopy())
			vm.OwnerReferences = ownerRef
			if err := r.Patch(ctx, vm, patch); err != nil {
				conditions.MarkError(ms, vmopv1.VirtualMachineGroupMemberConditionGroupLinked, "OwnerRefPatchError", err)
				errs = append(errs, err)
				newStatuses = append(newStatuses, *ms)
				continue
			}

			conditions.MarkTrue(ms, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)

		case "VirtualMachineGroup":
			group := &vmopv1.VirtualMachineGroup{}
			if err := r.Get(ctx, objKey, group); err != nil {
				conditions.MarkError(ms, vmopv1.VirtualMachineGroupMemberConditionGroupLinked, "GetError", err)
				errs = append(errs, err)
				newStatuses = append(newStatuses, *ms)
				continue
			}

			if group.Spec.GroupName != ctx.VMGroup.Name {
				err := fmt.Errorf("member has a different group name: %s", group.Spec.GroupName)
				conditions.MarkError(ms, vmopv1.VirtualMachineGroupMemberConditionGroupLinked, "InvalidGroupName", err)
				errs = append(errs, err)
				newStatuses = append(newStatuses, *ms)
				continue
			}

			patch := client.MergeFrom(group.DeepCopy())
			group.OwnerReferences = ownerRef
			if err := r.Patch(ctx, group, patch); err != nil {
				conditions.MarkError(ms, vmopv1.VirtualMachineGroupMemberConditionGroupLinked, "OwnerRefPatchError", err)
				errs = append(errs, err)
				newStatuses = append(newStatuses, *ms)
				continue
			}

			conditions.MarkTrue(ms, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
		}

		newStatuses = append(newStatuses, *ms)
	}

	ctx.VMGroup.Status.Members = newStatuses
	return apierrorsutil.NewAggregate(errs)
}

// TODO(sai): Implement placement logic for all unplaced VMs in the group.
func (r *Reconciler) reconcilePlacement(ctx *pkgctx.VirtualMachineGroupContext) error {
	return nil
}

// TODO(sai): Implement power state logic for all group members.
func (r *Reconciler) reconcilePowerState(ctx *pkgctx.VirtualMachineGroupContext) error {
	return nil
}

// setReadyCondition sets the Ready condition for the group based on the
// status of its members.
func setReadyCondition(ctx *pkgctx.VirtualMachineGroupContext) {
	if len(ctx.VMGroup.Status.Members) == 0 {
		conditions.MarkTrue(ctx.VMGroup, vmopv1.ReadyConditionType)
		return
	}

	var readyCount int
	var totalCount = len(ctx.VMGroup.Status.Members)
	var failureReasons []string

	for _, member := range ctx.VMGroup.Status.Members {
		ready, reason := isMemberReady(member)
		if ready {
			readyCount++
		} else {
			failureReasons = append(failureReasons, reason)
		}
	}

	if readyCount == totalCount {
		conditions.MarkTrue(ctx.VMGroup, vmopv1.ReadyConditionType)
	} else {
		msg := fmt.Sprintf("%d of %d members ready", readyCount, totalCount)
		if len(failureReasons) > 0 {
			msg = fmt.Sprintf("%s: %s", msg, strings.Join(failureReasons, "; "))
		}
		conditions.MarkFalse(ctx.VMGroup, vmopv1.ReadyConditionType, "MembersNotReady", "%s", msg)
	}
}

// isMemberReady checks if a member has all expected conditions set to true.
func isMemberReady(member vmopv1.VirtualMachineGroupMemberStatus) (bool, string) {
	// Define expected conditions based on member kind.
	var expectedConditions []string
	switch member.Kind {
	case "VirtualMachine":
		expectedConditions = []string{
			vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
			vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
			vmopv1.VirtualMachineConditionPlacementReady,
		}
	case "VirtualMachineGroup":
		expectedConditions = []string{
			vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
			vmopv1.ReadyConditionType,
		}
	}

	conditionMap := make(map[string]*metav1.Condition)
	for _, condition := range member.Conditions {
		conditionMap[condition.Type] = &condition
	}

	memberKey := fmt.Sprintf("%s/%s", member.Kind, member.Name)
	for _, expectedType := range expectedConditions {
		condition, exists := conditionMap[expectedType]
		if !exists {
			return false, fmt.Sprintf("%s missing condition %s", memberKey, expectedType)
		}

		if condition.Status != metav1.ConditionTrue {
			return false, fmt.Sprintf("%s condition %s is %s", memberKey, expectedType, condition.Status)
		}
	}

	return true, ""
}
