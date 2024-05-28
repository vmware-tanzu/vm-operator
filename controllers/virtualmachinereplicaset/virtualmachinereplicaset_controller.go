// Copyright (c) 2019-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinereplicaset

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/prober"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
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
		// TODO: Should we also watch the volumes so we get an event for changes
		// to a volume mounted to a VM that is part of a ReplicaSet?
		WithOptions(controller.Options{MaxConcurrentReconciles: ctx.MaxConcurrentReconciles}).
		Complete(r)

}

// VMToReplicaSets is a mapper function to be used to enqueue requests for reconciliation for VirtualMachineSetReplicaSets that might adopt a VM.
func (r *Reconciler) VMToReplicaSets(ctx *pkgctx.ControllerManagerContext) func(_ context.Context, o client.Object) []reconcile.Request {

	// If the replica VM already has a controller reference, return early. If
	// not, enqueue a reconcile if the VM matches the selector specified in the
	// ReplicaSet. Note that we don't support adopting a replica since that
	// might involve relocating it across compute and storage entities -- which
	// is not supported. Instead, we emit a Condition on the ReplicaSet resource
	// indicating it has orphaned replicas.
	return func(_ context.Context, o client.Object) []reconcile.Request {
		result := []ctrl.Request{}
		vm, ok := o.(*vmopv1.VirtualMachine)
		if !ok {
			panic(fmt.Sprintf("Expected a VirtualMachine, but got a %T", o))
		}

		// If this VM already has a controller reference, no reconciles are
		// queued.
		for _, ref := range vm.ObjectMeta.GetOwnerReferences() {
			if ref.Controller != nil && *ref.Controller {
				return nil
			}
		}

		vmrsss, err := r.getReplicaSetsForVM(ctx, vm)
		if err != nil {
			ctx.Logger.Error(err, "Failed getting VM ReplicaSets for VM")
			return nil
		}

		for _, rs := range vmrsss {
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
		return nil, fmt.Errorf("vm %v has no labels, This is unexpected", client.ObjectKeyFromObject(vm))
	}

	rsList := &vmopv1.VirtualMachineReplicaSetList{}
	if err := r.Client.List(ctx, rsList, client.InNamespace(vm.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list VirtualMachineReplicaSets: %w", err)
	}

	var rss []*vmopv1.VirtualMachineReplicaSet
	for idx := range rsList.Items {
		rs := &rsList.Items[idx]

		selector, err := metav1.LabelSelectorAsSelector(rs.Spec.Selector)
		if err != nil {
			continue
		}

		if selector.Empty() {
			continue
		}

		if selector.Matches(labels.Set(vm.Labels)) {
			rss = append(rss, rs)
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
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper for %s: %w", rsCtx, err)
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

	if err := r.ReconcileNormal(rsCtx); err != nil {
		rsCtx.Logger.Error(err, "Failed to reconcile VirtualMachineReplicaSet")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) ReconcileNormal(ctx *pkgctx.VirtualMachineReplicaSetContext) error {
	if !controllerutil.ContainsFinalizer(ctx.ReplicaSet, finalizerName) {
		// Set the finalizer and return immediately so the object is patched immediately.
		controllerutil.AddFinalizer(ctx.ReplicaSet, finalizerName)
		return nil
	}

	ctx.Logger.Info("Reconciling VirtualMachineReplicaSet")

	return nil
}

func (r *Reconciler) ReconcileDelete(ctx *pkgctx.VirtualMachineReplicaSetContext) (reterr error) {
	if controllerutil.ContainsFinalizer(ctx.ReplicaSet, finalizerName) {
		defer func() {
			r.Recorder.EmitEvent(ctx.ReplicaSet, "Delete", reterr, false)
		}()

		// Handle deletion of replicas

		controllerutil.RemoveFinalizer(ctx.ReplicaSet, finalizerName)
	}

	ctx.Logger.Info("Finished Reconciling VirtualMachineReplicaSet Deletion")

	return nil
}
