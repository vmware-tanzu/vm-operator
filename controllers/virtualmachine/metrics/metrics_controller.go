// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"context"
	"fmt"
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	pkgmetrics "github.com/vmware-tanzu/vm-operator/pkg/metrics"
)

const (
	finalizerName = "vmoperator.vmware.com/vm-metrics"
)

// SkipNameValidation is used for testing to allow multiple controllers with the
// same name.
var SkipNameValidation *bool

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controllerName      = "vm-metrics"
		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controllerName))
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
	)

	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: ctx.MaxConcurrentReconciles,
		SkipNameValidation:      SkipNameValidation,
		LogConstructor:          pkglog.ControllerLogConstructor(controllerNameShort, &vmopv1.VirtualMachine{}, mgr.GetScheme()),
	})
	if err != nil {
		return err
	}

	return c.Watch(source.Kind(
		mgr.GetCache(),
		&vmopv1.VirtualMachine{},
		&handler.TypedEnqueueRequestForObject[*vmopv1.VirtualMachine]{},
		predicate.TypedFuncs[*vmopv1.VirtualMachine]{
			CreateFunc: func(_ event.TypedCreateEvent[*vmopv1.VirtualMachine]) bool {
				return true
			},
			UpdateFunc: func(e event.TypedUpdateEvent[*vmopv1.VirtualMachine]) bool {
				return VMMetricsFieldsChanged(e.ObjectOld, e.ObjectNew)
			},
			DeleteFunc: func(_ event.TypedDeleteEvent[*vmopv1.VirtualMachine]) bool {
				return true
			},
		},
	))
}

func NewReconciler(
	ctx context.Context,
	client client.Client) *Reconciler {

	return &Reconciler{
		Context:   ctx,
		Client:    client,
		vmMetrics: pkgmetrics.NewVMMetrics(),
	}
}

var _ reconcile.Reconciler = &Reconciler{}

// Reconciler reconciles VirtualMachine objects to update Prometheus metrics.
type Reconciler struct {
	client.Client
	Context   context.Context
	vmMetrics *pkgmetrics.VMMetrics
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get

func (r *Reconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request) (ctrl.Result, error) {

	ctx = pkgcfg.JoinContext(ctx, r.Context)

	vm := &vmopv1.VirtualMachine{}
	if err := r.Get(ctx, req.NamespacedName, vm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !vm.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.reconcileDelete(ctx, vm)
	}

	return ctrl.Result{}, r.reconcileNormal(ctx, vm)
}

func (r *Reconciler) reconcileDelete(
	ctx context.Context,
	vm *vmopv1.VirtualMachine) error {

	if !controllerutil.ContainsFinalizer(vm, finalizerName) {
		return nil
	}

	r.vmMetrics.DeleteMetrics(ctx, vm.Name, vm.Namespace)

	return r.patchFinalizer(ctx, vm, func() {
		controllerutil.RemoveFinalizer(vm, finalizerName)
	})
}

func (r *Reconciler) reconcileNormal(
	ctx context.Context,
	vm *vmopv1.VirtualMachine) error {

	if !controllerutil.ContainsFinalizer(vm, finalizerName) {
		return r.patchFinalizer(ctx, vm, func() {
			controllerutil.AddFinalizer(vm, finalizerName)
		})
	}

	r.vmMetrics.RegisterVMCreateOrUpdateMetrics(ctx, vm)

	return nil
}

// patchFinalizer applies a merge patch to persist a finalizer change. The
// mutateFn should call controllerutil.AddFinalizer or RemoveFinalizer on vm
// before the patch is computed and sent.
func (r *Reconciler) patchFinalizer(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	mutateFn func()) error {

	patch := client.MergeFrom(vm.DeepCopy())
	mutateFn()
	return r.Patch(ctx, vm, patch)
}

// VMMetricsFieldsChanged returns true if any field that affects VM metrics
// has changed between the old and new objects, or if finalizer management
// is required.
func VMMetricsFieldsChanged(oldVM, newVM *vmopv1.VirtualMachine) bool {
	if oldVM == nil || newVM == nil {
		return true
	}

	// Spec.PowerState -> vm_powerstate metric.
	if oldVM.Spec.PowerState != newVM.Spec.PowerState {
		return true
	}

	// Status.PowerState -> vm_powerstate metric.
	if oldVM.Status.PowerState != newVM.Status.PowerState {
		return true
	}

	// Status.Conditions -> vm_status_condition_status and vm_status_phase
	// metrics. Compare only the fields that affect metrics (Type, Status,
	// Reason), ignoring LastTransitionTime, ObservedGeneration, and Message.
	if !conditionsEqualForMetrics(oldVM.Status.Conditions, newVM.Status.Conditions) {
		return true
	}

	// Network IP presence -> vm_status_ip metric. Only care about whether
	// an IP is assigned, not the actual address value.
	if vmHasIP(oldVM) != vmHasIP(newVM) {
		return true
	}

	// DeletionTimestamp transition triggers finalizer removal.
	if oldVM.DeletionTimestamp.IsZero() != newVM.DeletionTimestamp.IsZero() {
		return true
	}

	// Finalizer changes (e.g. our own finalizer being persisted).
	if !slices.Equal(oldVM.Finalizers, newVM.Finalizers) {
		return true
	}

	return false
}

func vmHasIP(vm *vmopv1.VirtualMachine) bool {
	if vm.Status.Network == nil {
		return false
	}
	return vm.Status.Network.PrimaryIP4 != "" || vm.Status.Network.PrimaryIP6 != ""
}

// conditionsEqualForMetrics compares two condition slices considering only the
// fields used by the metrics: Type, Status, and Reason.
func conditionsEqualForMetrics(a, b []metav1.Condition) bool {
	if len(a) != len(b) {
		return false
	}

	type key struct {
		condType string
		status   string
		reason   string
	}

	setA := make(map[key]struct{}, len(a))
	for i := range a {
		setA[key{a[i].Type, string(a[i].Status), a[i].Reason}] = struct{}{}
	}

	for i := range b {
		k := key{b[i].Type, string(b[i].Status), b[i].Reason}
		if _, ok := setA[k]; !ok {
			return false
		}
	}

	return true
}
