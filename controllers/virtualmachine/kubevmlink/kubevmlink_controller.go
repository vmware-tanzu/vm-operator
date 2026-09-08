// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package kubevmlink reconciles a VM Operator VirtualMachine that is owned
// by a kube-vm.io VirtualMachine, keeping the delegated spec.powerState
// field in step with the generic object and reporting when a post-create
// edit to an immutable delegated field cannot be applied.
package kubevmlink

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kubevmv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/kubevm"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1.VirtualMachine{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-kubevmlink-controller", strings.ToLower(controlledTypeName))
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controllerNameShort),
		record.New(mgr.GetEventRecorder(controllerNameShort)),
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		Named(controllerNameShort).
		Watches(&kubevmv1a1.VirtualMachine{},
			handler.EnqueueRequestsFromMapFunc(genericToProviderMapperFn())).
		WithOptions(controller.Options{
			LogConstructor: pkglog.ControllerLogConstructor(controllerNameShort, controlledType, mgr.GetScheme()),
		}).
		Complete(r)
}

// genericToProviderMapperFn maps a kube-vm.io VirtualMachine event to the
// provider object its spec.infrastructureRef names, when that reference
// points at a vmoperator.vmware.com VirtualMachine.
func genericToProviderMapperFn() handler.MapFunc {
	return func(_ context.Context, o client.Object) []reconcile.Request {
		generic, ok := o.(*kubevmv1a1.VirtualMachine)
		if !ok {
			return nil
		}

		ref := generic.Spec.InfrastructureRef
		if ref.APIGroup != vmopv1.GroupName || ref.Kind != "VirtualMachine" {
			return nil
		}

		return []reconcile.Request{
			{NamespacedName: types.NamespacedName{
				Namespace: generic.Namespace,
				Name:      ref.Name,
			}},
		}
	}
}

// NewReconciler returns a new reconciler for VirtualMachine objects owned by
// a kube-vm.io VirtualMachine.
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

// Reconciler reassert the delegated spec.powerState field from an owning
// kube-vm.io VirtualMachine, and reports when a post-create edit to an
// immutable delegated field cannot be applied.
type Reconciler struct {
	client.Client
	Context  context.Context
	Logger   logr.Logger
	Recorder record.Recorder
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kube-vm.io,resources=virtualmachines,verbs=list;watch

// Reconcile reasserts the delegated spec.powerState field from the owning
// kube-vm.io VirtualMachine onto the provider object, and sets the UpToDate
// condition false when a delegated field that is immutable after create no
// longer matches what the generic object requests.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	if !pkgcfg.FromContext(ctx).Features.KubeVMProvider {
		return ctrl.Result{}, nil
	}

	vm := &vmopv1.VirtualMachine{}
	if err := r.Get(ctx, req.NamespacedName, vm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, err := patch.NewHelper(vm, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf(
			"failed to init patch helper for %s: %w", req.NamespacedName, err)
	}
	defer func() {
		if err := patchHelper.Patch(ctx, vm); err != nil {
			if reterr == nil {
				reterr = err
			}
		}
	}()

	logger := pkglog.FromContextOrDefault(ctx).WithValues("vmName", req.NamespacedName)

	return pkgerr.ResultFromError(r.reconcileNormal(ctx, logger, vm))
}

func (r *Reconciler) reconcileNormal(
	ctx context.Context,
	logger logr.Logger,
	vm *vmopv1.VirtualMachine) error {

	genericName, ok := kubevm.GenericObjectName(vm)
	if !ok {
		return nil
	}

	if vm.Spec.GroupName != "" {
		conditions.MarkFalse(vm,
			vmopv1.VirtualMachineConditionUpToDate,
			"GroupMemberConflict",
			"%s",
			"cannot be both a VirtualMachineGroup member and owned by a generic VirtualMachine")

		return pkgerr.NoRequeueError{
			Message: fmt.Sprintf(
				"VirtualMachine %q has both spec.groupName and the %s annotation set",
				vm.Name, kubevm.AnnotationKey),
		}
	}

	generic := &kubevmv1a1.VirtualMachine{}
	if err := r.Get(
		ctx,
		types.NamespacedName{Namespace: vm.Namespace, Name: genericName},
		generic); err != nil {

		if apierrors.IsNotFound(err) {
			return pkgerr.NoRequeueError{
				Message: fmt.Sprintf("generic VirtualMachine %q not found", genericName),
			}
		}
		return fmt.Errorf("failed to get generic VirtualMachine %q: %w", genericName, err)
	}

	if !kubevm.IsMutuallyLinked(vm, generic) {
		logger.V(4).Info("Provider object and generic object do not mutually name each other, skipping")
		return nil
	}

	desiredPowerState := kubevm.PowerStateFor(generic.Spec.PowerState)
	if desiredPowerState != "" && vm.Spec.PowerState != desiredPowerState {
		vm.Spec.PowerState = desiredPowerState
	}

	if kubevm.DelegatedFieldsUpToDate(vm, generic) {
		conditions.MarkTrue(vm, vmopv1.VirtualMachineConditionUpToDate)
	} else {
		conditions.MarkFalse(vm,
			vmopv1.VirtualMachineConditionUpToDate,
			"UnsupportedByProvider",
			"%s",
			"the generic VirtualMachine requests a change to an instance type, "+
				"boot disk image, or storage class that cannot be applied after create")
	}

	return nil
}
