// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinesetresourcepolicy

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator"
	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/controller/common"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

const controllerName = "virtualmachinesetresourcepolicy-controller"

var log = logf.Log.WithName(controllerName)

// Add creates a new VirtualMachineSetResourcePolicy Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	// Get provider registered in the manager's main()
	provider := vmprovider.GetVmProviderOrDie()

	return &ReconcileVirtualMachineSetResourcePolicy{
		Client:     mgr.GetClient(),
		scheme:     mgr.GetScheme(),
		vmProvider: provider,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: common.GetMaxReconcileNum()})
	if err != nil {
		return err
	}

	// Watch for changes to VirtualMachineSetResourcePolicy resources
	err = c.Watch(&source.Kind{Type: &vmoperatorv1alpha1.VirtualMachineSetResourcePolicy{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileVirtualMachineSetResourcePolicy{}

// VirtualMachineSetResourcePolicyReconciler reconciles a VirtualMachineSetResourcePolicy object
type ReconcileVirtualMachineSetResourcePolicy struct {
	client.Client
	scheme     *runtime.Scheme
	vmProvider vmprovider.VirtualMachineProviderInterface
}

// reconcileCreateOrUpdate reconciles a VirtualMachineSetResourcePolicy.
func (r *ReconcileVirtualMachineSetResourcePolicy) reconcileCreateOrUpdate(ctx context.Context, policy *vmoperatorv1alpha1.VirtualMachineSetResourcePolicy) error {
	logger := log.WithValues("namespace", policy.Namespace, "name", policy.Name)
	logger.V(4).Info("Reconciling CreateOrUpdate VirtualMachineSetResourcePolicy")

	err := r.vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(ctx, policy)
	if err != nil {
		return err
	}

	if err := r.Status().Update(ctx, policy); err != nil {
		logger.Error(err, "Failed to update VirtualMachineSetResourcePolicy status")
		return err
	}

	logger.V(4).Info("Reconciled CreateOrUpdate VirtualMachineSetResourcePolicy without errors.")

	return nil
}

// reconcileDelete reconciles a deleted VirtualMachineSetResourcePolicy resource.
func (r *ReconcileVirtualMachineSetResourcePolicy) reconcileDelete(ctx context.Context, resourcePolicy *vmoperatorv1alpha1.VirtualMachineSetResourcePolicy) (err error) {
	logger := log.WithValues("namespace", resourcePolicy.Namespace, "name", resourcePolicy.Name)

	// Skip deleting a VirtualMachineSetResourcePolicy if it is referenced by a VirtualMachine.
	vmsInNamespace := &vmoperatorv1alpha1.VirtualMachineList{}
	err = r.List(ctx, &client.ListOptions{Namespace: resourcePolicy.Namespace}, vmsInNamespace)
	if err != nil {
		log.Error(err, "Failed to list VMs in namespace", "namespace", resourcePolicy.Namespace)
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

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesetresourcepolicy,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesetresourcepolicy/status,verbs=get;update;patch
func (r *ReconcileVirtualMachineSetResourcePolicy) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := log.WithValues("namespace", request.Namespace, "name", request.Name)

	logger.V(4).Info("Reconciling VirtualMachineSetResourcePolicy resource")
	instance := &vmoperatorv1alpha1.VirtualMachineSetResourcePolicy{}
	err := r.Get(context.Background(), request.NamespacedName, instance)
	if err != nil {
		logger.Error(err, "There was some error retrieving the resource")
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	ctx := context.Background()

	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		if lib.ContainsString(instance.ObjectMeta.Finalizers, vmoperator.VirtualMachineSetResourcePolicyFinalizer) {
			if err := r.reconcileDelete(ctx, instance); err != nil {
				// return with error so that it can be retried
				return reconcile.Result{}, err
			}

			// remove our finalizer from the list and update it.
			instance.ObjectMeta.Finalizers = lib.RemoveString(instance.ObjectMeta.Finalizers, vmoperator.VirtualMachineSetResourcePolicyFinalizer)
			if err := r.Update(ctx, instance); err != nil {
				return reconcile.Result{}, err
			}
		}

		// finalizer has finished. Resource is deleted. Nothing to reconcile.
		return reconcile.Result{}, nil
	}

	if err = r.reconcileCreateOrUpdate(ctx, instance); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
