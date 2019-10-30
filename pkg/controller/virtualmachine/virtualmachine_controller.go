/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachine

import (
	"context"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	volumeproviders "github.com/vmware-tanzu/vm-operator/pkg/controller/virtualmachine/providers"
	v1 "k8s.io/api/core/v1"
	storagetypev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/controller/common"
	"github.com/vmware-tanzu/vm-operator/pkg/controller/common/record"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	cnsv1alpha1 "gitlab.eng.vmware.com/hatchway/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"
)

const (
	OpCreate = "CreateVM"
	OpDelete = "DeleteVM"
	OpUpdate = "UpdateVM"
	OpCheck  = "CheckVM"

	ControllerName = "virtualmachine-controller"

	// We should add more uniqueness to the OpId to prevent the collision incurred by different vm-operator to aid
	// debugging at VPXD.
	RandomLen = 8
)

var log = logf.Log.WithName(ControllerName)

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
		Client:         mgr.GetClient(),
		scheme:         mgr.GetScheme(),
		vmProvider:     provider,
		volumeProvider: volumeproviders.CnsVolumeProvider(mgr.GetClient()),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(ControllerName, mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: common.GetMaxReconcileNum()})
	if err != nil {
		return err
	}

	// Watch for changes to VirtualMachine
	err = c.Watch(&source.Kind{Type: &vmoperatorv1alpha1.VirtualMachine{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes for CnsNodeVmAttachment, and enqueue VirtualMachine which is the owner of CnsNodeVmAttachment
	err = c.Watch(&source.Kind{Type: &cnsv1alpha1.CnsNodeVmAttachment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &vmoperatorv1alpha1.VirtualMachine{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileVirtualMachine{}

// ReconcileVirtualMachine reconciles a VirtualMachine object
type ReconcileVirtualMachine struct {
	client.Client
	scheme         *runtime.Scheme
	vmProvider     vmprovider.VirtualMachineProviderInterface
	volumeProvider volumeproviders.VolumeProviderInterface
}

// Reconcile reads that state of the cluster for a VirtualMachine object and makes changes based on the state read
// and what is in the VirtualMachine.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclasses,verbs=get;list
// +kubebuilder:rbac:groups=cns.vmware.com,resources=cnsnodevmattachments,verbs=create;delete;get;list;watch;patch;update
// +kubebuilder:rbac:groups=cns.vmware.com,resources=cnsnodevmattachments/status,verbs=get;list
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=vmware.com,resources=virtualnetworkinterfaces;virtualnetworkinterfaces/status,verbs=create;get;list;patch;delete;watch;update
// +kubebuilder:rbac:groups="",resources=events;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps/status,verbs=get;update;patch
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

	clusterID, err := r.vmProvider.GetClusterID(ctx, request.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}
	ctx = context.WithValue(ctx, vimtypes.ID{}, "vmoperator-"+ControllerName+"-"+clusterID+"-"+instance.Name)

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
		log.Error(err, "Failed to reconcile VirtualMachine", "name", instance.NamespacedName())
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileVirtualMachine) deleteVm(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine) (err error) {
	defer record.EmitEvent(vm, OpDelete, &err, false)

	// TODO 	//  The length limit of OpId is 256, we will implement a function to detects if the total length exceeds
	//  the limit and then “smartly” trims each component of the op id to an abbreviated version
	opId := ctx.Value(vimtypes.ID{}).(string)
	ctx = context.WithValue(ctx, vimtypes.ID{}, opId+"-delete-"+common.RandomString(RandomLen))

	err = r.vmProvider.DeleteVirtualMachine(ctx, vm)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("To be deleted VirtualMachine was not found", "name", vm.NamespacedName())
			return nil
		}
		log.Error(err, "Failed to delete VirtualMachine", "name", vm.NamespacedName())
		return err
	}

	vm.Status.Phase = vmoperatorv1alpha1.Deleted
	log.V(4).Info("Deleted VirtualMachine", "name", vm.NamespacedName())

	return nil
}

// Process a level trigger for this VM: create if it doesn't exist otherwise update the existing VM.
func (r *ReconcileVirtualMachine) reconcileVm(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine) (err error) {
	log.Info("Reconciling VirtualMachine", "name", vm.NamespacedName())

	exists, err := r.vmProvider.DoesVirtualMachineExist(ctx, vm.Namespace, vm.Name)
	if err != nil {
		log.Error(err, "Failed to check if VirtualMachine exists from provider", "name", vm.NamespacedName())
		record.EmitEvent(vm, OpCheck, &err, false)
		return err
	}

	// Before the VM being created or updated, figure out the Volumes[] VirtualMachineVolumes change.
	// This step determines what CnsNodeVmAttachments need to be created or deleted
	vmVolumesToProcess, _ := volumeproviders.GetVmVolumesToProcess(vm)

	if exists {
		err = r.updateVm(ctx, vm)
	} else {
		err = r.createVm(ctx, vm)
	}

	if err != nil {
		return err
	}

	// Create CnsNodeVMAttachments on demand if VM has been reconciled properly
	err = r.volumeProvider.AttachVolumes(ctx, vm, vmVolumesToProcess)
	if err != nil {
		return err
	}

	// TODO: Delete CnsNodeVMAttachments on demand if VM has been reconciled properly

	err = r.volumeProvider.UpdateVmVolumesStatus(ctx, vm)
	if err != nil {
		return err
	}

	if uErr := r.Status().Update(ctx, vm); uErr != nil {
		log.Error(uErr, "Failed to update VirtualMachine status", "name", vm.NamespacedName())
		if err != nil {
			return err
		}
		return uErr
	}

	return nil
}

func (r *ReconcileVirtualMachine) processStorageClass(ctx context.Context, vmSpec *v1alpha1.VirtualMachineSpec) (string, error) {
	if len(vmSpec.StorageClass) == 0 {
		return "", nil
	}

	scl := &storagetypev1.StorageClass{}
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: "", Name: vmSpec.StorageClass}, scl)
	if err != nil {
		klog.Errorf("Failed to get storage class: %v", err)
		return "", err
	}

	return scl.Parameters["storagePolicyID"], nil
}

func (r *ReconcileVirtualMachine) createVm(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine) (err error) {
	defer record.EmitEvent(vm, OpCreate, &err, false)

	// TODO 	//  The length limit of OpId is 256, we will implement a function to detects if the total length exceeds
	//  the limit and then “smartly” trims each component of the op id to an abbreviated version
	opId := ctx.Value(vimtypes.ID{}).(string)
	ctx = context.WithValue(ctx, vimtypes.ID{}, opId+"-create-"+common.RandomString(RandomLen))

	vmClass := &vmoperatorv1alpha1.VirtualMachineClass{}
	err = r.Get(ctx, client.ObjectKey{Name: vm.Spec.ClassName}, vmClass)
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
			log.Error(err, "Failed to get VirtualMachineMetadata ConfigMap",
				"vmName", vm.NamespacedName(), "configMapName", metadata.ConfigMapName)
			return err
		}

		vmMetadata = configMap.Data
	}

	policyID, err := r.processStorageClass(ctx, &vm.Spec)
	if err != nil {
		return err
	}
	vm.Status.Phase = vmoperatorv1alpha1.Creating

	err = r.vmProvider.CreateVirtualMachine(ctx, vm, *vmClass, vmMetadata, policyID)
	if err != nil {
		log.Error(err, "Provider failed to create VirtualMachine", "name", vm.NamespacedName())
		return err
	}

	vm.Status.Phase = vmoperatorv1alpha1.Created
	pkg.AddAnnotations(&vm.ObjectMeta)

	return nil
}

func (r *ReconcileVirtualMachine) updateVm(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine) (err error) {
	defer record.EmitEvent(vm, OpUpdate, &err, false)

	// TODO 	//  The length limit of OpId is 256, we will implement a function to detects if the total length exceeds
	//  the limit and then “smartly” trims each component of the op id to an abbreviated version
	opId := ctx.Value(vimtypes.ID{}).(string)
	ctx = context.WithValue(ctx, vimtypes.ID{}, opId+"-update-"+common.RandomString(RandomLen))

	vmClass := &vmoperatorv1alpha1.VirtualMachineClass{}
	err = r.Get(ctx, client.ObjectKey{Name: vm.Spec.ClassName}, vmClass)
	if err != nil {
		log.Error(err, "Failed to get VirtualMachineClass for VirtualMachine",
			"vmName", vm.NamespacedName(), "class", vm.Spec.ClassName)
		return err
	}

	// if a resourcePolicy is specified, verify that it exists
	if vm.Spec.ResourcePolicyName != "" {
		resourcePolicy := &vmoperatorv1alpha1.VirtualMachineSetResourcePolicy{}
		err = r.Get(ctx, client.ObjectKey{Name: vm.Spec.ResourcePolicyName, Namespace: vm.Namespace}, resourcePolicy)

		if err != nil {
			log.Error(err, "Failed to get ResourcePolicy for VirtualMachine",
				"vmName", vm.NamespacedName(), "class", vm.Spec.ResourcePolicyName)
			return err
		}
		// Set the resource policy name manually for now. This should be passed like VM classes ideally.
		vm.Status.ResourcePolicyName = resourcePolicy.Name
	}

	var vmMetadata vmprovider.VirtualMachineMetadata
	if metadata := vm.Spec.VmMetadata; metadata != nil {
		configMap := &v1.ConfigMap{}
		err := r.Get(ctx, types.NamespacedName{Name: metadata.ConfigMapName, Namespace: vm.Namespace}, configMap)
		if err != nil {
			log.Error(err, "Failed to get VirtualMachineMetadata ConfigMap",
				"vmName", vm.NamespacedName(), "configMapName", metadata.ConfigMapName)
			return err
		}

		vmMetadata = configMap.Data
	}

	err = r.vmProvider.UpdateVirtualMachine(ctx, vm, *vmClass, vmMetadata)
	if err != nil {
		log.Error(err, "Provider failed to update VirtualMachine", "name", vm.NamespacedName())
		return err
	}

	return nil
}
