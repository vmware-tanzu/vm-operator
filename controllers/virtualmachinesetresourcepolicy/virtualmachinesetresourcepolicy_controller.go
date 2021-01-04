// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinesetresourcepolicy

import (
	goctx "context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

const (
	finalizerName = "virtualmachinesetresourcepolicy.vmoperator.vmware.com"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1alpha1.VirtualMachineSetResourcePolicy{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()
	)

	r := NewReconciler(
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		ctx.VmProvider,
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		Complete(r)
}

func NewReconciler(
	client client.Client,
	logger logr.Logger,
	vmProvider vmprovider.VirtualMachineProviderInterface) *VirtualMachineSetResourcePolicyReconciler {

	return &VirtualMachineSetResourcePolicyReconciler{
		Client:     client,
		Logger:     logger,
		VMProvider: vmProvider,
	}
}

// VirtualMachineReconciler reconciles a VirtualMachine object
type VirtualMachineSetResourcePolicyReconciler struct {
	client.Client
	Logger     logr.Logger
	VMProvider vmprovider.VirtualMachineProviderInterface
}

// ReconcileNormal reconciles a VirtualMachineSetResourcePolicy.
func (r *VirtualMachineSetResourcePolicyReconciler) ReconcileNormal(ctx *context.VirtualMachineSetResourcePolicyContext) error {
	if !controllerutil.ContainsFinalizer(ctx.ResourcePolicy, finalizerName) {
		// Return here so the VirtualMachineSetResourcePolicy can be patched immediately. This ensures that the resource policies are cleaned up
		// properly when they are deleted.
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
func (r *VirtualMachineSetResourcePolicyReconciler) deleteResourcePolicy(ctx *context.VirtualMachineSetResourcePolicyContext) error {
	resourcePolicy := ctx.ResourcePolicy

	// Skip deleting a VirtualMachineSetResourcePolicy if it is referenced by a VirtualMachine.
	vmsInNamespace := &vmopv1alpha1.VirtualMachineList{}
	if err := r.List(ctx, vmsInNamespace, client.InNamespace(resourcePolicy.Namespace)); err != nil {
		ctx.Logger.Error(err, "Failed to list VMs in namespace", "namespace", resourcePolicy.Namespace)
		return err
	}

	// TODO: to handle the race here.
	for _, vm := range vmsInNamespace.Items {
		if vm.Spec.ResourcePolicyName == resourcePolicy.Name {
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

func (r *VirtualMachineSetResourcePolicyReconciler) ReconcileDelete(ctx *context.VirtualMachineSetResourcePolicyContext) error {
	resourcePolicy := ctx.ResourcePolicy
	ctx.Logger.Info("Reconciling VirtualMachineSetResourcePolicy Deletion")
	defer func() {
		ctx.Logger.Info("Finished Reconciling VirtualMachineSetResourcePolicy Deletion")
	}()

	if controllerutil.ContainsFinalizer(resourcePolicy, finalizerName) {
		if err := r.deleteResourcePolicy(ctx); err != nil {
			return err
		}

		controllerutil.RemoveFinalizer(resourcePolicy, finalizerName)
	}

	return nil
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesetresourcepolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesetresourcepolicies/status,verbs=get;update;patch

func (r *VirtualMachineSetResourcePolicyReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx := goctx.Background()

	rp := &vmopv1alpha1.VirtualMachineSetResourcePolicy{}
	if err := r.Get(ctx, req.NamespacedName, rp); err != nil {
		if apiErrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	rpCtx := &context.VirtualMachineSetResourcePolicyContext{
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
