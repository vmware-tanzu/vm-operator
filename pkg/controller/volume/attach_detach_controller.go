/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package volume

import (
	"context"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/controller/common"
	"github.com/vmware-tanzu/vm-operator/pkg/controller/virtualmachine"
	volumeproviders "github.com/vmware-tanzu/vm-operator/pkg/controller/volume/providers"
	cnsv1alpha1 "gitlab.eng.vmware.com/hatchway/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"
)

const (
	ControllerName = "volume-attach-detach-controller"
)

var log = logf.Log.WithName(ControllerName)

// Add creates a new Volume Attach Detach Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {

	return &ReconcileVolume{
		Client:         mgr.GetClient(),
		scheme:         mgr.GetScheme(),
		volumeProvider: volumeproviders.CnsVolumeProvider(mgr.GetClient()),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(virtualmachine.ControllerName, mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: common.GetMaxReconcileNum()})
	if err != nil {
		return err
	}

	// Watch for changes to VirtualMachine
	err = c.Watch(&source.Kind{Type: &vmoperatorv1alpha1.VirtualMachine{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes for CnsNodeVmAttachment, and enqueue VirtualMachine which is the owner of CnsNodeVmAttachment
	// Note: While we could filter out VM's which doesn't have volumes in the spec or status, we don't do it because
	// we need to handle scenarios where we could leak CNSNodeVMAttachment resources ().
	err = c.Watch(&source.Kind{Type: &cnsv1alpha1.CnsNodeVmAttachment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &vmoperatorv1alpha1.VirtualMachine{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileVolume{}

type ReconcileVolume struct {
	client.Client
	scheme         *runtime.Scheme
	volumeProvider volumeproviders.VolumeProviderInterface
}

// Reconcile reconciles a VirtualMachine object and processes the volumes for attach/detach.
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list;watch;
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cns.vmware.com,resources=cnsnodevmattachments,verbs=create;delete;get;list;watch;patch;update
// +kubebuilder:rbac:groups=cns.vmware.com,resources=cnsnodevmattachments/status,verbs=get;list
func (r *ReconcileVolume) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx := context.Background()

	vm := &vmoperatorv1alpha1.VirtualMachine{}
	err := r.Get(ctx, request.NamespacedName, vm)
	if err != nil {
		if apiErrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if !vm.ObjectMeta.DeletionTimestamp.IsZero() {
		// Nothing to do since the Garbage Collector is expected to handle the deletion of dependent resources like the
		// CNSNodeVMAttachment if using the CNS volume provider.
		// TODO: We could still ensure the VM is not deleted before all volumes are detached. We don't do this today and
		// we expect the volume provider to handle scenarios where the VM is deleted before the volumes are
		// detached & removed.
		return reconcile.Result{}, err
	}

	if err := r.reconcileNormal(ctx, vm); err != nil {
		log.Error(err, "Failed to reconcile VirtualMachine", "name", vm.NamespacedName())
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileVolume) reconcileNormal(ctx context.Context, origVM *vmoperatorv1alpha1.VirtualMachine) error {
	log.Info("Reconciling VirtualMachine for processing volumes ", "name", origVM.NamespacedName())
	defer func() {
		log.Info("Finished Reconciling VirtualMachine for processing volumes", "name", origVM.NamespacedName())
	}()

	vm := origVM.DeepCopy()

	// Before the VM being created or updated, figure out the Volumes[] VirtualMachineVolumes change.
	// This step determines what CnsNodeVmAttachments need to be created or deleted
	// BMV: Push into provider
	volumesToAdd, volumesToDelete := volumeproviders.GetVmVolumesToProcess(vm)

	volumeOpsErrs := r.reconcileVolumes(ctx, vm, volumesToAdd, volumesToDelete)

	// TODO: When there are multiple volumes, the ordering seems to change. We need to do an in-place update of the
	//  volume status. Otherwise, this will result in an unwanted status update call.
	if !apiequality.Semantic.DeepEqual(vm.Status.Volumes, origVM.Status.Volumes) {
		if err := r.Status().Update(ctx, vm); err != nil {
			log.Error(err, "Failed to update VirtualMachine status", "name", vm.NamespacedName())
			return err
		}
	}

	if volumeOpsErrs.HasOccurred() {
		return volumeOpsErrs
	}
	return nil
}

func (r *ReconcileVolume) reconcileVolumes(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine,
	volumesToAdd map[client.ObjectKey]bool, volumesToDelete map[client.ObjectKey]bool) volumeproviders.CombinedVolumeOpsErrors {

	volumeOpsErrs := volumeproviders.CombinedVolumeOpsErrors{}

	// Create CnsNodeVMAttachments on demand if VM has been reconciled properly
	err := r.volumeProvider.AttachVolumes(ctx, vm, volumesToAdd)
	volumeOpsErrs.Append(err)

	// Delete CnsNodeVMAttachments on demand if VM has been reconciled properly
	err = r.volumeProvider.DetachVolumes(ctx, vm, volumesToDelete)
	volumeOpsErrs.Append(err)

	// Update the VirtualMachineVolumeStatus based on the status of respective CnsNodeVmAttachment instance
	err = r.volumeProvider.UpdateVmVolumesStatus(ctx, vm)
	volumeOpsErrs.Append(err)

	// 1. AttachVolumes() returns error when it fails to create CnsNodeVmAttachment instances (partially or completely)
	//  1.1 If the attach operation [succeeds partially | fails completely], there is no reason to return without
	// calling(r.Status().Update(ctx, vm)) to update the status for the succeeded set of volumes.
	//
	// 2. DetachVolumes() returns error when it fails to delete CnsNodeVmAttachment instances (partially or completely)
	//  2.1 If the detach operation [succeeds partially | fails completely], there is no reason to return without
	// calling (r.Status().Update(ctx, vm)) to update the status for the succeeded set of volumes.
	//
	// 3. UpdateVmVolumesStatus() does not call client.Status().Update(), it only modifies the vm object by updating the vm.Status.Volumes.
	// It returns error when it fails to get the CnsNodeVmAttachment instance which is supposed to exist.
	//  3.1 If update volumes status succeeds partially, there is no reason to return without calling (r.Status().Update(ctx, vm))

	return volumeOpsErrs
}
