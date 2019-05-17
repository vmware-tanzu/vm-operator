/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachine

import (
	"context"

	v1 "k8s.io/api/core/v1"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"

	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator"
	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("virtualmachine-controller")

// Add creates a new VirtualMachine Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	// Get provider registered in the manager's main()
	provider := vmprovider.GetVmProviderOrDie()

	return &ReconcileVirtualMachine{
		Client:     mgr.GetClient(),
		scheme:     mgr.GetScheme(),
		vmProvider: provider,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("virtualmachine-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to VirtualMachine
	err = c.Watch(&source.Kind{Type: &vmoperatorv1alpha1.VirtualMachine{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileVirtualMachine{}

// ReconcileVirtualMachine reconciles a VirtualMachine object
type ReconcileVirtualMachine struct {
	client.Client
	scheme     *runtime.Scheme
	vmProvider vmprovider.VirtualMachineProviderInterface
}

// Reconcile reads that state of the cluster for a VirtualMachine object and makes changes based on the state read
// and what is in the VirtualMachine.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclasses,verbs=get;list
func (r *ReconcileVirtualMachine) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx := context.Background()

	// Fetch the VirtualMachine instance
	instance := &vmoperatorv1alpha1.VirtualMachine{}
	err := r.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		const finalizerName = vmoperator.VirtualMachineFinalizer

		if lib.ContainsString(instance.ObjectMeta.Finalizers, finalizerName) {
			if err := r.deleteVm(ctx, instance); err != nil {
				// return with error so that it can be retried
				return reconcile.Result{}, err
			}

			// remove our finalizer from the list and update it.
			instance.ObjectMeta.Finalizers = lib.RemoveString(instance.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(ctx, instance); err != nil {
				return reconcile.Result{}, err
			}
		}

		// Our finalizer has finished, so the reconciler can do nothing.
		return reconcile.Result{}, nil
	}

	if err := r.reconcileVm(ctx, instance); err != nil {
		log.Error(err, "failed to reconcile VirtualMachine", "VirtualMachine", instance)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileVirtualMachine) deleteVm(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine) error {
	// TODO(bryanv) Move inside provider
	vm.Status.Phase = vmoperatorv1alpha1.Deleted

	err := r.vmProvider.DeleteVirtualMachine(ctx, vm)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Cannot find VirtualMachine that is to be deleted", "name", vm.NamespacedName())
			return nil
		}
		log.Error(err, "Failed to delete VirtualMachine", "name", vm.NamespacedName())
		return err
	}

	log.V(4).Info("Deleted VirtualMachine", "name", vm.NamespacedName())
	return nil
}

// Process a level trigger for this VM: create if it doesn't exist otherwise update the existing VM.
func (r *ReconcileVirtualMachine) reconcileVm(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine) error {
	log.Info("Process VirtualMachine CreateOrUpdate", "name", vm.NamespacedName())

	_, err := r.vmProvider.GetVirtualMachine(ctx, vm.Namespace, vm.Name)
	switch {
	case errors.IsNotFound(err):
		err = r.createVm(ctx, vm)
	case err != nil:
		log.Error(err, "Failed to get VirtualMachine from provider", "name", vm.NamespacedName())
	default:
		err = r.updateVm(ctx, vm)
	}

	if err != nil {
		return err
	}

	if err := r.Update(ctx, vm); err != nil {
		log.Error(err, "Failed to update VirtualMachine", "name", vm.NamespacedName())
		return err
	}

	return nil
}

func (r *ReconcileVirtualMachine) createVm(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine) error {
	vm.Status.Phase = vmoperatorv1alpha1.Creating

	vmClass := &vmoperatorv1alpha1.VirtualMachineClass{}
	err := r.Get(ctx, types.NamespacedName{Name: vm.Spec.ClassName, Namespace: vm.Namespace}, vmClass)
	if err != nil {
		log.Error(err, "Failed to get VirtualMachineClass for VirtualMachine",
			"vmName", vm.NamespacedName(), "class", vm.Spec.ClassName)
		return err
	}

	var vmMetadata vmprovider.VirtualMachineMetadata
	if metadata := vm.Spec.VmMetadata; metadata != nil {
		configMap := &v1.ConfigMap{}
		err := r.Get(ctx, types.NamespacedName{Name: metadata.ConfigMapName, Namespace: vm.Namespace}, configMap)
		if err != nil {
			log.Error(err, "Failed to get ConfigMap for VirtualMachineMetadata",
				"vmName", vm.NamespacedName(), "configMapName", metadata.ConfigMapName)
			return err
		}

		vmMetadata = configMap.Data
	}

	tmpVm, err := r.vmProvider.CreateVirtualMachine(ctx, vm, *vmClass, vmMetadata)
	if err != nil {
		log.Error(err, "Provider failed to create VirtualMachine", "name", vm.NamespacedName())
		return err
	}

	// TODO(bryanv) Move inside provider
	tmpVm.Status.Phase = vmoperatorv1alpha1.Created

	// TODO(bryanv) Hack until we fix providers
	tmpVm.Status.DeepCopyInto(&vm.Status)
	pkg.AddAnnotations(&vm.ObjectMeta)

	return nil
}

func (r *ReconcileVirtualMachine) updateVm(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine) error {
	tmpVm, err := r.vmProvider.UpdateVirtualMachine(ctx, vm)
	if err != nil {
		log.Error(err, "Provider failed to update VirtualMachine", "name", vm.NamespacedName())
		return err
	}

	// TODO(bryanv) Hack until we fix providers
	tmpVm.Status.DeepCopyInto(&vm.Status)
	return nil
}
