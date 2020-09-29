// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinesetresourcepolicy

import (
	goctx "context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

const (
	controllerName          = "virtualmachinesetresourcepolicy-controller"
	resourcePolicyFinalizer = "virtualmachinesetresourcepolicy.vmoperator.vmware.com"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	return add(mgr, NewReconciler(ctx, mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(ctx *context.ControllerManagerContext, mgr manager.Manager) reconcile.Reconciler {
	return &VirtualMachineSetResourcePolicyReconciler{
		Client:     mgr.GetClient(),
		Logger:     ctrllog.Log.WithName("controllers").WithName(controllerName),
		Scheme:     mgr.GetScheme(),
		VMProvider: ctx.VmProvider,
	}
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(controllerName).
		For(&vmoperatorv1alpha1.VirtualMachineSetResourcePolicy{}).
		Complete(r)
}

// VirtualMachineSetResourcePolicyReconciler reconciles a VirtualMachineSetResourcePolicy object
type VirtualMachineSetResourcePolicyReconciler struct {
	client.Client
	Logger     logr.Logger
	Scheme     *runtime.Scheme
	VMProvider vmprovider.VirtualMachineProviderInterface
}

// ReconcileNormal reconciles a VirtualMachineSetResourcePolicy.
func (r *VirtualMachineSetResourcePolicyReconciler) ReconcileNormal(ctx *context.VirtualMachineSetResourcePolicyContext) error {
	resourcePolicy := ctx.ResourcePolicy
	ctx.Logger.Info("Reconciling VirtualMachineSetResourcePolicy")

	// Not so nice way of dealing with reconcile twice when updating status issue
	var origClusterModsStatus vmoperatorv1alpha1.VirtualMachineSetResourcePolicyStatus
	resourcePolicy.Status.DeepCopyInto(&origClusterModsStatus)

	// Add the finalizer if it is not set.
	if !controllerutil.ContainsFinalizer(resourcePolicy, resourcePolicyFinalizer) {
		controllerutil.AddFinalizer(resourcePolicy, resourcePolicyFinalizer)
		if err := r.Update(ctx, resourcePolicy); err != nil {
			ctx.Logger.Error(err, "Failed to add finalizer to the resource policy", "finalizerName", resourcePolicyFinalizer)
			return err
		}
	}

	if err := r.VMProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(ctx, resourcePolicy); err != nil {
		ctx.Logger.Error(err, "Provider failed to reconcile VirtualMachineSetResourcePolicy")
		return err
	}

	if !reflect.DeepEqual(origClusterModsStatus, resourcePolicy.Status) {
		if err := r.Status().Update(ctx, resourcePolicy); err != nil {
			ctx.Logger.Error(err, "Failed to update VirtualMachineSetResourcePolicy status")
			return err
		}
	}

	ctx.Logger.V(4).Info("Reconcile VirtualMachineSetResourcePolicy completed")
	return nil
}

// deleteResourcePolicy deletes a VirtualMachineSetResourcePolicy resource.
func (r *VirtualMachineSetResourcePolicyReconciler) deleteResourcePolicy(ctx *context.VirtualMachineSetResourcePolicyContext) error {
	resourcePolicy := ctx.ResourcePolicy

	// Skip deleting a VirtualMachineSetResourcePolicy if it is referenced by a VirtualMachine.
	vmsInNamespace := &vmoperatorv1alpha1.VirtualMachineList{}
	if err := r.List(ctx, vmsInNamespace, client.InNamespace(resourcePolicy.Namespace)); err != nil {
		ctx.Logger.Error(err, "Failed to list VMs in namespace", "namespace", resourcePolicy.Namespace)
		return err
	}

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
	ctx.Logger.Info("Reconciling VirtualMachineSetResourcePolicy delete")

	if controllerutil.ContainsFinalizer(resourcePolicy, resourcePolicyFinalizer) {
		if err := r.deleteResourcePolicy(ctx); err != nil {
			return err
		}

		controllerutil.RemoveFinalizer(resourcePolicy, resourcePolicyFinalizer)
		if err := r.Update(ctx, resourcePolicy); err != nil {
			return err
		}
	}

	return nil
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesetresourcepolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesetresourcepolicies/status,verbs=get;update;patch

func (r *VirtualMachineSetResourcePolicyReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := goctx.Background()
	logger := r.Logger.WithValues("virtualmachinesetresourcepolicy", req.NamespacedName)
	logger.V(4).Info("Reconciling VirtualMachineSetResourcePolicy resource")

	instance := &vmoperatorv1alpha1.VirtualMachineSetResourcePolicy{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	resourcePolicyCtx := &context.VirtualMachineSetResourcePolicyContext{
		Context:        ctx,
		Logger:         r.Logger.WithName(instance.Namespace).WithName(instance.Name),
		ResourcePolicy: instance,
	}

	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		err = r.ReconcileDelete(resourcePolicyCtx)
		return ctrl.Result{}, err
	}

	if err = r.ReconcileNormal(resourcePolicyCtx); err != nil {
		logger.Error(err, "Failed to reconcile VirtualMachineSetResourcePolicy")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
