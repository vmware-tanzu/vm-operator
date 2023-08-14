// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	goctx "context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	patch "github.com/vmware-tanzu/vm-operator/pkg/patch2"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

const (
	finalizerName = "virtualmachinesetresourcepolicy.vmoperator.vmware.com"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1.VirtualMachineSetResourcePolicy{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()
	)

	r := NewReconciler(
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		ctx.VMProviderA2,
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		Complete(r)
}

func NewReconciler(
	client client.Client,
	logger logr.Logger,
	vmProvider vmprovider.VirtualMachineProviderInterfaceA2) *Reconciler {
	return &Reconciler{
		Client:     client,
		Logger:     logger,
		VMProvider: vmProvider,
	}
}

// Reconciler reconciles a VirtualMachineSetResourcePolicy object.
type Reconciler struct {
	client.Client
	Logger     logr.Logger
	VMProvider vmprovider.VirtualMachineProviderInterfaceA2
}

// ReconcileNormal reconciles a VirtualMachineSetResourcePolicy.
func (r *Reconciler) ReconcileNormal(ctx *context.VirtualMachineSetResourcePolicyContextA2) error {
	if !controllerutil.ContainsFinalizer(ctx.ResourcePolicy, finalizerName) {
		// Return here so the VirtualMachineSetResourcePolicy can be patched immediately. This ensures that
		// the resource policies are cleaned up properly when they are deleted.
		controllerutil.AddFinalizer(ctx.ResourcePolicy, finalizerName)
		return nil
	}

	ctx.Logger.Info("Reconciling VirtualMachineSetResourcePolicy")
	defer func() {
		ctx.Logger.Info("Finished Reconciling VirtualMachineSetResourcePolicy")
	}()

	if err := r.VMProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(ctx, ctx.ResourcePolicy); err != nil {
		ctx.Logger.Error(err, "Provider failed to reconcile VirtualMachineSetResourcePolicy")
		return err
	}

	return nil
}

// deleteResourcePolicy deletes a VirtualMachineSetResourcePolicy resource.
func (r *Reconciler) deleteResourcePolicy(ctx *context.VirtualMachineSetResourcePolicyContextA2) error {
	resourcePolicy := ctx.ResourcePolicy

	// Skip deleting a VirtualMachineSetResourcePolicy if it is referenced by a VirtualMachine.
	vmsInNamespace := &vmopv1.VirtualMachineList{}
	if err := r.List(ctx, vmsInNamespace, client.InNamespace(resourcePolicy.Namespace)); err != nil {
		ctx.Logger.Error(err, "Failed to list VMs in namespace", "namespace", resourcePolicy.Namespace)
		return err
	}

	for _, vm := range vmsInNamespace.Items {
		if vm.Spec.Reserved.ResourcePolicyName == resourcePolicy.Name {
			return fmt.Errorf("failing VirtualMachineSetResourcePolicy deletion since VM: '%s' is referencing it, resourcePolicyName: '%s'",
				vm.NamespacedName(), resourcePolicy.NamespacedName())
		}
	}

	ctx.Logger.V(4).Info("Attempting to delete VirtualMachineSetResourcePolicy")
	if err := r.VMProvider.DeleteVirtualMachineSetResourcePolicy(ctx, resourcePolicy); err != nil {
		ctx.Logger.Error(err, "error in deleting VirtualMachineSetResourcePolicy")
		return err
	}
	ctx.Logger.Info("Deleted VirtualMachineSetResourcePolicy successfully")

	return nil
}

func (r *Reconciler) ReconcileDelete(ctx *context.VirtualMachineSetResourcePolicyContextA2) error {
	ctx.Logger.Info("Reconciling VirtualMachineSetResourcePolicy Deletion")
	defer func() {
		ctx.Logger.Info("Finished Reconciling VirtualMachineSetResourcePolicy Deletion")
	}()

	if controllerutil.ContainsFinalizer(ctx.ResourcePolicy, finalizerName) {
		if err := r.deleteResourcePolicy(ctx); err != nil {
			return err
		}

		controllerutil.RemoveFinalizer(ctx.ResourcePolicy, finalizerName)
	}

	return nil
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesetresourcepolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesetresourcepolicies/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(ctx goctx.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	rp := &vmopv1.VirtualMachineSetResourcePolicy{}
	if err := r.Get(ctx, req.NamespacedName, rp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	rpCtx := &context.VirtualMachineSetResourcePolicyContextA2{
		Context:        ctx,
		Logger:         r.Logger.WithName("VirtualMachineSetResourcePolicy").WithValues("name", rp.NamespacedName()),
		ResourcePolicy: rp,
	}

	patchHelper, err := patch.NewHelper(rp, r.Client)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to init patch helper for %s", rpCtx.String())
	}
	defer func() {
		if err := patchHelper.Patch(ctx, rp); err != nil {
			if reterr == nil {
				reterr = err
			}
			rpCtx.Logger.Error(err, "patch failed")
		}
	}()

	if !rp.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.ReconcileDelete(rpCtx)
	}

	return ctrl.Result{}, r.ReconcileNormal(rpCtx)
}
