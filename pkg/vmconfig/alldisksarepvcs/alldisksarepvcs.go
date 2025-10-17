// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package alldisksarepvcs

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
)

var ErrUnmanagedVols = pkgerr.NoRequeueNoErr("has unmanaged volumes")

type reconciler struct{}

var _ vmconfig.Reconciler = reconciler{}

const (
	ReasonTaskError = "DiskPromotionTaskError"
	ReasonPending   = "DiskPromotionPending"
	ReasonRunning   = "DiskPromotionRunning"

	PromoteDisksTaskKey = "VirtualMachine.promoteDisks"
)

// New returns a new Reconciler for a VM's disk promotion state.
func New() vmconfig.Reconciler {
	return reconciler{}
}

// Name returns the unique name used to identify the reconciler.
func (r reconciler) Name() string {
	return "alldisksarepvcs"
}

func (r reconciler) OnResult(
	_ context.Context,
	_ *vmopv1.VirtualMachine,
	_ mo.VirtualMachine,
	_ error) error {

	return nil
}

// Reconcile ensures all non-PVC disks become PVCs.
func Reconcile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	_ *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	_ *vimtypes.VirtualMachineConfigSpec) error {

	return New().Reconcile(ctx, k8sClient, nil, vm, moVM, nil)
}

// Reconcile ensures all non-PVC disks become PVCs.
func (r reconciler) Reconcile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	_ *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	_ *vimtypes.VirtualMachineConfigSpec) error {

	if ctx == nil {
		panic("context is nil")
	}
	if k8sClient == nil {
		panic("k8sClient is nil")
	}
	if vm == nil {
		panic("vm is nil")
	}

	if moVM.Config == nil || len(moVM.Config.Hardware.Device) == 0 {
		return nil
	}

	var (
		unmanagedDisks      []pkgutil.VirtualDiskInfo
		volumeByUUID        = map[string]vmopv1.VirtualMachineVolume{}
		controllerKeyToBus  = map[int32]*int32{}
		controllerKeyToType = map[int32]vmopv1.VirtualControllerType{}
		snapshotDiskKeys    = map[int32]struct{}{}
	)

	// Find all of the disks that are participating in snapshots.
	if layoutEx := moVM.LayoutEx; layoutEx != nil {
		for _, snap := range layoutEx.Snapshot {
			for _, disk := range snap.Disk {
				snapshotDiskKeys[disk.Key] = struct{}{}
			}
		}
	}

	// Build a map of existing volumes by name for quick lookup.
	for _, vol := range vm.Spec.Volumes {
		if pvc := vol.PersistentVolumeClaim; pvc != nil {
			if uvc := pvc.UnmanagedVolumeClaim; uvc != nil {
				volumeByUUID[uvc.UUID] = vol
			}
		}
	}

	// Iterate through hardware devices to find non-FCD disks and disk
	// controllers.
	for _, device := range moVM.Config.Hardware.Device {

		switch d := device.(type) {
		case *vimtypes.VirtualDisk:
			if d.VDiskId == nil || d.VDiskId.Id == "" { // Skip FCDs.
				di := pkgutil.GetVirtualDiskInfo(d)
				if di.UUID != "" && di.FileName != "" { // Skip if no UUID/file.
					_, inSnap := snapshotDiskKeys[d.Key]
					if !di.HasParent || (di.HasParent && inSnap) {
						// Include disks that are not child disks, or if they
						// are, are the running point for a snapshot.
						unmanagedDisks = append(unmanagedDisks, di)
					}
				}
			}
		case *vimtypes.VirtualIDEController:
			controllerKeyToType[d.Key] = vmopv1.VirtualControllerTypeIDE
			controllerKeyToBus[d.Key] = &d.BusNumber
		case vimtypes.BaseVirtualSCSIController:
			bd := d.GetVirtualSCSIController()
			controllerKeyToType[bd.Key] = vmopv1.VirtualControllerTypeSCSI
			controllerKeyToBus[bd.Key] = &bd.BusNumber
		case *vimtypes.VirtualNVMEController:
			controllerKeyToType[d.Key] = vmopv1.VirtualControllerTypeNVME
			controllerKeyToBus[d.Key] = &d.BusNumber
		case vimtypes.BaseVirtualSATAController:
			bd := d.GetVirtualSATAController()
			controllerKeyToType[bd.Key] = vmopv1.VirtualControllerTypeSATA
			controllerKeyToBus[bd.Key] = &bd.BusNumber
		}
	}

	// addedToSpec tracks whether anything in the next for loop adds an entry to
	// spec.volumes.
	var addedToSpec bool

	// Process each unmanaged disk.
	for _, di := range unmanagedDisks {
		// Step 1: Look for existing volume entry with the provided UUID.
		if _, exists := volumeByUUID[di.UUID]; !exists {
			// Step 2: If no such entry exists, add one.
			vm.Spec.Volumes = append(
				vm.Spec.Volumes,
				vmopv1.VirtualMachineVolume{
					Name: di.UUID,
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: pkgutil.GeneratePVCName(
									vm.Name,
									di.UUID,
								),
							},
							UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{
								Type: vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM,
								Name: di.UUID,
								UUID: di.UUID,
							},
							ControllerBusNumber: controllerKeyToBus[di.ControllerKey],
							ControllerType:      controllerKeyToType[di.ControllerKey],
							UnitNumber:          di.UnitNumber,
						},
					},
				},
			)
			addedToSpec = true
		}
	}

	// If any unmanaged volumes were added to spec, then return a NoRequeueError
	// to ensure the spec is patched before we create any PVCs/CnsRegisterVolume
	// objects.
	if addedToSpec {
		return ErrUnmanagedVols
	}

	// hasPendingVolumes tracks whether or not any PVCs are not yet bound.
	var hasPendingVolumes bool

	for _, di := range unmanagedDisks {
		// Step 1: Look for the volume entry from spec.volumes.
		volume := volumeByUUID[di.UUID]

		// Step 2: Ensure PVC exists.
		pvcName := volume.PersistentVolumeClaim.ClaimName
		pvc, err := ensurePVCForUnmanagedDisk(
			ctx,
			k8sClient,
			vm,
			volume.PersistentVolumeClaim.ClaimName,
			di)
		if err != nil {
			return fmt.Errorf(
				"failed to ensure pvc %s for disk %s: %w",
				pvcName, di.UUID, err)
		}

		switch pvc.Status.Phase {
		case "", corev1.ClaimPending:
			// Step 3: Check if PVC is bound and create CnsRegisterVolume if
			//         needed.
			if _, err := ensureCnsRegisterVolumeForDisk(
				ctx,
				k8sClient,
				vm,
				pvcName,
				di); err != nil {

				return fmt.Errorf(
					"failed to ensure CnsRegisterVolume %s: %w",
					pvcName, err)
			}
			hasPendingVolumes = true

		case corev1.ClaimBound:
			// Step 4: Clean up completed CnsRegisterVolume objects.
			if err := cleanupCnsRegisterVolumeForVM(
				ctx,
				k8sClient,
				vm,
				pvcName); err != nil {

				return err
			}
		}
	}

	// If there were PVCs that were not yet bound, return a NoRequeue error to
	// wait for CnsRegisterVolume completion.
	if hasPendingVolumes {
		return ErrUnmanagedVols
	}

	// Clean up any remaining CnsRegisterVolume objects for this VM
	if err := cleanupAllCnsRegisterVolumesForVM(
		ctx,
		k8sClient,
		vm); err != nil {

		return err
	}

	return nil
}

// ensurePVCForUnmanagedDisk ensures a PVC exists for an unmanaged disk
func ensurePVCForUnmanagedDisk(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
	pvcName string,
	diskInfo pkgutil.VirtualDiskInfo) (*corev1.PersistentVolumeClaim, error) {

	const virtualMachine = "VirtualMachine"

	var (
		obj = &corev1.PersistentVolumeClaim{}
		key = ctrlclient.ObjectKey{
			Namespace: vm.Namespace,
			Name:      pvcName,
		}

		expDSRef = corev1.TypedObjectReference{
			APIGroup: &vmopv1.GroupVersion.Group,
			Kind:     virtualMachine,
			Name:     vm.Name,
		}

		expOwnerRef = metav1.OwnerReference{
			APIVersion: vmopv1.GroupVersion.String(),
			Kind:       virtualMachine,
			Name:       vm.Name,
			UID:        vm.UID,
		}
	)

	if err := k8sClient.Get(ctx, key, obj); err != nil {

		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get pvc %s: %w", key, err)
		}

		//
		// The PVC is not found, create a new one.
		//

		// Set the object metadata.
		obj.Name = pvcName
		obj.Namespace = vm.Namespace

		// Set an OwnerRef on the PVC pointing back to the VM.
		obj.OwnerReferences = []metav1.OwnerReference{expOwnerRef}

		// Set dataSourceRef to point to this VM.
		obj.Spec.DataSourceRef = &expDSRef

		// Set the PVC's AccessModes to ReadWriteMany if the existing,
		// underlying disk has a sharing mode set to MultiWriter. Otherwise init
		// the PVC's AccessModes to ReadWriteOnce.
		if diskInfo.Sharing == vimtypes.VirtualDiskSharingSharingMultiWriter {
			obj.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			}
		} else {
			obj.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			}
		}

		// Initialize the requested size of the PVC to the current capacity of
		// the existing disk.
		obj.Spec.Resources.Requests = corev1.ResourceList{
			corev1.ResourceStorage: ptr.Deref(
				kubeutil.BytesToResource(diskInfo.CapacityInBytes)),
		}

		// Create the PVC.
		if err := k8sClient.Create(ctx, obj); err != nil {
			return nil, fmt.Errorf("failed to create pvc %s: %w", key, err)
		}

		return obj, nil
	}

	var (
		hasDSRef    bool
		hasOwnerRef bool
	)

	// Check to see if the PVC already has the expected OwnerRef.
	for _, r := range obj.OwnerReferences {
		if r.APIVersion == expOwnerRef.APIVersion &&
			r.Kind == expOwnerRef.Kind &&
			r.Name == expOwnerRef.Name &&
			r.UID == expOwnerRef.UID {

			hasOwnerRef = true
			break
		}
	}

	// Check to see if the PVC already has the expected DataSourceRef.
	if dsRef := obj.Spec.DataSourceRef; dsRef != nil {
		if dsRef.APIGroup != nil &&
			*dsRef.APIGroup == *expDSRef.APIGroup &&
			dsRef.Kind == expDSRef.Kind &&
			dsRef.Name == expDSRef.Name {

			hasDSRef = true
		}
	}

	if hasOwnerRef && hasDSRef {
		return obj, nil
	}

	objPatch := ctrlclient.MergeFrom(obj.DeepCopy())

	// Ensure the PVC's dataSourceRef to point to this VM.
	obj.Spec.DataSourceRef = &expDSRef

	// And the OwnerReference to the PVC that points back to the VM.
	if !hasOwnerRef {
		obj.OwnerReferences = append(obj.OwnerReferences, expOwnerRef)
	}

	if err := k8sClient.Patch(ctx, obj, objPatch); err != nil {
		return nil, fmt.Errorf("failed to patch pvc %s: %w", key, err)
	}

	return obj, nil
}

// ensureCnsRegisterVolumeForDisk ensures a CnsRegisterVolume exists for an
// unmanaged disk.
func ensureCnsRegisterVolumeForDisk(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
	pvcName string,
	diskInfo pkgutil.VirtualDiskInfo) (*cnsv1alpha1.CnsRegisterVolume, error) {

	var (
		obj = &cnsv1alpha1.CnsRegisterVolume{}
		key = ctrlclient.ObjectKey{
			Namespace: vm.Namespace,
			Name:      pvcName,
		}
	)

	if err := k8sClient.Get(ctx, key, obj); err != nil {

		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get crv %s: %w", key, err)
		}

		//
		// The CRV is not found, create a new one.
		//
		// Set the object metadata.
		obj.Name = pvcName
		obj.Namespace = vm.Namespace

		obj.Labels = map[string]string{
			pkgconst.CreatedByLabel: vm.Name,
		}

		// Set a ControllerOwnerRef on the CRV pointing back to the VM.
		obj.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         vmopv1.GroupVersion.String(),
				BlockOwnerDeletion: ptr.To(true),
				Controller:         ptr.To(true),
				Kind:               "VirtualMachine",
				Name:               vm.Name,
				UID:                vm.UID,
			},
		}

		obj.Spec = cnsv1alpha1.CnsRegisterVolumeSpec{
			PvcName:     pvcName,
			DiskURLPath: generateDiskURLPath(diskInfo.FileName),
		}

		// Set the AccessMode to ReadWriteMany if the existing,
		// underlying disk has a sharing mode set to MultiWriter. Otherwise init
		// the AccessMode to ReadWriteOnce.
		if diskInfo.Sharing == vimtypes.VirtualDiskSharingSharingMultiWriter {
			obj.Spec.AccessMode = corev1.ReadWriteMany
		} else {
			obj.Spec.AccessMode = corev1.ReadWriteOnce
		}

		// Create the CVR.
		if err := k8sClient.Create(ctx, obj); err != nil {
			return nil, fmt.Errorf("failed to create cvr %s: %w", key, err)
		}
	}

	return obj, nil
}

// generateDiskURLPath generates a disk URL path for CnsRegisterVolume
func generateDiskURLPath(fileName string) string {
	// TODO: Implement proper disk URL path generation based on the file name
	// For now, return the fileName as-is, but this should be enhanced to generate
	// the proper URL format required by CnsRegisterVolume
	return fileName
}

// cleanupCnsRegisterVolumeForVM cleans up a CnsRegisterVolume object.
func cleanupCnsRegisterVolumeForVM(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
	pvcName string) error {

	if err := k8sClient.Delete(
		ctx,
		&cnsv1alpha1.CnsRegisterVolume{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: vm.Namespace,
				Name:      pvcName,
			},
		}); err != nil && !apierrors.IsNotFound(err) {

		return fmt.Errorf("failed to delete CnsRegisterVolume %s: %w",
			pvcName, err)
	}

	return nil
}

// cleanupAllCnsRegisterVolumesForVM cleans up all CnsRegisterVolume objects for
// a VM.
func cleanupAllCnsRegisterVolumesForVM(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine) error {

	if err := k8sClient.DeleteAllOf(
		ctx,
		&cnsv1alpha1.CnsRegisterVolume{},
		ctrlclient.InNamespace(vm.Namespace),
		ctrlclient.MatchingLabels{
			pkgconst.CreatedByLabel: vm.Name,
		}); err != nil {

		return fmt.Errorf(
			"failed to delete CnsRegisterVolume objects for VM %s: %w",
			vm.NamespacedName(), err)
	}

	return nil
}
