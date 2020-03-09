// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
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
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

const (
	controllerName = "virtualmachinesetresourcepolicy-controller"
	finalizerName  = "virtualmachinesetresourcepolicy.vmoperator.vmware.com"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	return add(mgr, newReconciler(ctx, mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(ctx *context.ControllerManagerContext, mgr manager.Manager) reconcile.Reconciler {
	return &VirtualMachineSetResourcePolicyReconciler{
		Client:     mgr.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("VirtualMachineSetResourcePolicy"),
		Scheme:     mgr.GetScheme(),
		vmProvider: ctx.VmProvider,
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
	Log        logr.Logger
	Scheme     *runtime.Scheme
	vmProvider vmprovider.VirtualMachineProviderInterface
}

// reconcileNormal reconciles a VirtualMachineSetResourcePolicy.
func (r *VirtualMachineSetResourcePolicyReconciler) reconcileNormal(ctx goctx.Context, policy *vmoperatorv1alpha1.VirtualMachineSetResourcePolicy) error {
	logger := r.Log.WithValues("namespace", policy.Namespace, "name", policy.Name)
	logger.V(4).Info("Reconciling CreateOrUpdate VirtualMachineSetResourcePolicy")

	// Not so nice way of dealing with reconcile twice when updating status issue
	var origClusterModsStatus vmoperatorv1alpha1.VirtualMachineSetResourcePolicyStatus
	policy.Status.DeepCopyInto(&origClusterModsStatus)

	if !lib.ContainsString(policy.ObjectMeta.Finalizers, finalizerName) {
		policy.ObjectMeta.Finalizers = append(policy.ObjectMeta.Finalizers, finalizerName)
		if err := r.Update(ctx, policy); err != nil {
			return err
		}
	}

	err := r.vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(ctx, policy)
	if err != nil {
		return err
	}

	if !reflect.DeepEqual(origClusterModsStatus, policy.Status) {
		if err := r.Status().Update(ctx, policy); err != nil {
			logger.Error(err, "Failed to update VirtualMachineSetResourcePolicy status")
			return err
		}
	}

	logger.V(4).Info("Reconciled CreateOrUpdate VirtualMachineSetResourcePolicy without errors")

	return nil
}

// reconcileDelete reconciles a deleted VirtualMachineSetResourcePolicy resource.
func (r *VirtualMachineSetResourcePolicyReconciler) reconcileDelete(ctx goctx.Context, resourcePolicy *vmoperatorv1alpha1.VirtualMachineSetResourcePolicy) (err error) {
	logger := r.Log.WithValues("namespace", resourcePolicy.Namespace, "name", resourcePolicy.Name)

	// Skip deleting a VirtualMachineSetResourcePolicy if it is referenced by a VirtualMachine.
	vmsInNamespace := &vmoperatorv1alpha1.VirtualMachineList{}
	err = r.List(ctx, vmsInNamespace, client.InNamespace(resourcePolicy.Namespace))
	if err != nil {
		r.Log.Error(err, "Failed to list VMs in namespace", "namespace", resourcePolicy.Namespace)
		return err
	}

	for _, vm := range vmsInNamespace.Items {
		if vm.Spec.ResourcePolicyName == resourcePolicy.Name {
			return fmt.Errorf("failing VirtualMachineSetResourcePolicy deletion since VM: '%s' is referencing it, resourcePolicyName: '%s'",
				vm.NamespacedName(), resourcePolicy.NamespacedName())
		}
	}

	logger.V(4).Info("Attempting to delete VirtualMachineSetResourcePolicy")
	if err := r.vmProvider.DeleteVirtualMachineSetResourcePolicy(ctx, resourcePolicy); err != nil {
		logger.Error(err, "error in deleting VirtualMachineSetResourcePolicy")
		return err
	}
	logger.Info("Deleted VirtualMachineSetResourcePolicy successfully")

	return nil
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesetresourcepolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesetresourcepolicies/status,verbs=get;update;patch

func (r *VirtualMachineSetResourcePolicyReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := goctx.Background()
	logger := r.Log.WithValues("virtualmachinesetresourcepolicy", req.NamespacedName)
	logger.V(4).Info("Reconciling VirtualMachineSetResourcePolicy resource")

	instance := &vmoperatorv1alpha1.VirtualMachineSetResourcePolicy{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		if lib.ContainsString(instance.ObjectMeta.Finalizers, finalizerName) {
			if err := r.reconcileDelete(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}

			instance.ObjectMeta.Finalizers = lib.RemoveString(instance.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}
		}

		// finalizer has finished. Resource is deleted. Nothing to reconcile.
		return ctrl.Result{}, nil
	}

	if err = r.reconcileNormal(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
