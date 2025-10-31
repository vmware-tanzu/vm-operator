// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package register

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/pbm"
	pbmtypes "github.com/vmware/govmomi/pbm/types"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	pkgdatastore "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/datastore"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	unmanagedvolsutil "github.com/vmware-tanzu/vm-operator/pkg/vmconfig/volumes/unmanaged/util"
)

// Condition is the name of the condition that stores the result.
const Condition = "VirtualMachineUnmanagedVolumesRegister"

// ErrPendingRegister is returned from Reconcile to indicate to exit the VM
// reconcile workflow early.
var ErrPendingRegister = pkgerr.NoRequeueNoErr(
	"has unmanaged volumes pending registration")

type reconciler struct{}

var _ vmconfig.Reconciler = reconciler{}

// New returns a new Reconciler for registering a VM's unmanaged volumes as
// PVCs.
func New() vmconfig.Reconciler {
	return reconciler{}
}

// Name returns the unique name used to identify the reconciler.
func (r reconciler) Name() string {
	return "unmanagedvolumes-register"
}

func (r reconciler) OnResult(
	_ context.Context,
	_ *vmopv1.VirtualMachine,
	_ mo.VirtualMachine,
	_ error) error {

	return nil
}

// +kubebuilder:rbac:groups=cns.vmware.com,resources=cnsregistervolumes,verbs=get;list;watch;delete;deletecollection
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=create;delete;get;list;watch;patch;update

// Reconcile ensures all non-PVC disks become PVCs.
func Reconcile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec) error {

	return New().Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
}

// Reconcile ensures all non-PVC disks become PVCs.
func (r reconciler) Reconcile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec) error {

	if ctx == nil {
		panic("context is nil")
	}
	if k8sClient == nil {
		panic("k8sClient is nil")
	}
	if vimClient == nil {
		panic("vimClient is nil")
	}
	if vm == nil {
		panic("vm is nil")
	}

	if pkgcond.IsTrue(vm, Condition) {
		return nil
	}

	info := unmanagedvolsutil.GetUnmanagedVolumeInfo(vm, moVM)

	hasConfigSpecChanges, err := ensureUnmanagedDisksHaveStoragePolicies(
		ctx,
		k8sClient,
		vimClient,
		vm,
		configSpec,
		&info)
	if err != nil {
		pkgcond.MarkError(
			vm,
			Condition,
			"ErrStoragePolicies",
			err)
		return err
	}
	if hasConfigSpecChanges {
		pkgcond.MarkFalse(
			vm,
			Condition,
			"PendingStoragePolicies",
			"")
		return nil
	}

	hasPendingVolumes, err := registerUnmanagedDisks(
		ctx,
		k8sClient,
		vimClient,
		vm,
		info)
	if err != nil {
		pkgcond.MarkError(
			vm,
			Condition,
			"ErrRegistration",
			err)
		return err
	}
	if hasPendingVolumes {
		pkgcond.MarkFalse(
			vm,
			Condition,
			"PendingRegistration",
			"")
		return ErrPendingRegister
	}

	// Clean up any remaining CnsRegisterVolume objects for this VM.
	if err := cleanupAllCnsRegisterVolumesForVM(
		ctx,
		k8sClient,
		vm); err != nil {

		pkgcond.MarkError(
			vm,
			Condition,
			"ErrCleanup",
			err)
		return err
	}

	pkgcond.MarkTrue(vm, Condition)
	return nil
}

func ensureUnmanagedDisksHaveStoragePolicies(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec,
	info *unmanagedvolsutil.UnmanagedVolumeInfo) (bool, error) {

	logger := logr.FromContextOrDiscard(ctx).
		WithName("ensureUnmanagedDisksHaveStoragePolicies")
	logger.Info(
		"Ensuring unmanaged disks have storage policies",
		"disks", info.Disks,
		"volumes", info.Volumes)

	// For information on lookup up the profile ID for a disk, see the doc
	// https://developer.broadcom.com/xapis/vmware-storage-policy-api/latest/pbm.ServerObjectRef.html.
	pbmClient, err := pbm.NewClient(ctx, vimClient)
	if err != nil {
		return false, fmt.Errorf("failed to get pbm client: %w", err)
	}

	var (
		pbmRefToDeviceKey = map[string]int32{}
		pbmRefs           = make([]pbmtypes.PbmServerObjectRef, len(info.Disks))
	)

	// Get the storage profile IDs used by the VM and disks (if any).
	for i, di := range info.Disks {
		// Get the PVC for this unmanaged disk if it already exists.
		var (
			obj = &corev1.PersistentVolumeClaim{}
			key = ctrlclient.ObjectKey{
				Namespace: vm.Namespace,
			}
		)
		if vol, ok := info.Volumes[info.Disks[i].UUID]; ok {
			if pvc := vol.PersistentVolumeClaim; pvc != nil {
				if uvc := pvc.UnmanagedVolumeClaim; uvc != nil {
					key.Name = pvc.ClaimName
				}
			}
		}
		if err := k8sClient.Get(ctx, key, obj); err != nil {
			if !apierrors.IsNotFound(err) {
				return false, fmt.Errorf(
					"failed to get pvc for unmanaged volumes: %w", err)
			}
			if di.Type == vmopv1.UnmanagedVolumeClaimVolumeTypeFromImage {
				return false, fmt.Errorf(
					"failed to get pvc for unmanaged volume from image: %w",
					err)
			}
		}

		if scn := obj.Spec.StorageClassName; scn != nil && *scn != "" {
			// The unmanaged disk points to a PVC that has a defined storage
			// class, so use it.
			info.Disks[i].StorageClass = *scn
		} else {
			// Could not find a PVC with an existing storage class for the
			// unmanaged disk, so default the VM to using the VM's storage
			// class.
			info.Disks[i].StorageClass = vm.Spec.StorageClass
		}

		// Add the disk to the list of PBM object references to query.
		pbmRefs[i] = pbmtypes.PbmServerObjectRef{
			ObjectType: "virtualDiskId",
			Key: fmt.Sprintf(
				"%s:%d",
				vm.Status.UniqueID,
				info.Disks[i].DeviceKey,
			),
		}
		pbmRefToDeviceKey[pbmRefs[i].Key] = info.Disks[i].DeviceKey
	}

	if len(pbmRefs) > 0 {
		logger.Info("Querying associated profiles", "pbmObjectRefs", pbmRefs)

		pbmResults, err := pbmClient.QueryAssociatedProfiles(ctx, pbmRefs)
		if err != nil {
			return false, fmt.Errorf(
				"failed to query associated profiles: %w", err)
		}

		logger.Info("Queried associated profiles", "result", pbmResults)
		for _, r := range pbmResults {
			if lp := len(r.ProfileId); lp > 0 {
				for i := range info.Disks {
					k := pbmRefToDeviceKey[r.Object.Key]
					if k == info.Disks[i].DeviceKey {
						for _, p := range r.ProfileId {
							if p.UniqueId != "" {
								info.Disks[i].ProfileIDs = append(
									info.Disks[i].ProfileIDs, p.UniqueId)
							}
						}
					}
				}
			}
		}
	}

	var (
		objList                storagev1.StorageClassList
		hasConfigSpecChanges   bool
		storageClassToPolicyID = map[string]string{}
		policyIDToStorageClass = map[string]string{}
	)

	// Collect the list of StorageClass/StoragePolicy mappings.
	if err := k8sClient.List(ctx, &objList); err != nil {
		return false, fmt.Errorf("failed to list storage classes: %w", err)
	}
	for _, i := range objList.Items {
		if !strings.HasSuffix(i.Name, "latebinding") {
			pid := i.Parameters["storagePolicyID"]
			storageClassToPolicyID[i.Name] = pid
			policyIDToStorageClass[pid] = i.Name
		}
	}

	// Update the disks with their intended policy ID.
	for i := range info.Disks {
		for _, pid := range info.Disks[i].ProfileIDs {
			if className := policyIDToStorageClass[pid]; className != "" {
				info.Disks[i].StoragePolicyID = pid
				break
			}
		}
		if info.Disks[i].StoragePolicyID == "" {
			hasConfigSpecChanges = true
			pid := storageClassToPolicyID[info.Disks[i].StorageClass]
			info.Disks[i].StoragePolicyID = pid

			alreadyChanged := false
			for j := range configSpec.DeviceChange {
				dc := configSpec.DeviceChange[j].GetVirtualDeviceConfigSpec()
				if dc.Device.GetVirtualDevice().Key == dc.Device.GetVirtualDevice().Key {
					dc.Profile = append(
						dc.Profile,
						&vimtypes.VirtualMachineDefinedProfileSpec{
							ProfileId: pid,
						})
					alreadyChanged = true
				}
			}
			if !alreadyChanged {
				configSpec.DeviceChange = append(configSpec.DeviceChange,
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationEdit,
						Device:    info.Disks[i].Device,
						Profile: []vimtypes.BaseVirtualMachineProfileSpec{
							&vimtypes.VirtualMachineDefinedProfileSpec{
								ProfileId: pid,
							},
						},
					})
			}
		}
	}

	return hasConfigSpecChanges, nil
}

func registerUnmanagedDisks(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	info unmanagedvolsutil.UnmanagedVolumeInfo) (bool, error) {

	var hasPendingVolumes bool

	logger := logr.FromContextOrDiscard(ctx).WithName("registerUnmanagedDisks")
	logger.Info(
		"Registering unmanaged disks",
		"disks", info.Disks,
		"volumes", info.Volumes)

	for _, di := range info.Disks {
		// Step 1: Look for the volume entry from spec.volumes.
		if volume, ok := info.Volumes[di.UUID]; ok {

			logger.Info("Processing unmanaged disk",
				"disk", di, "volume", volume)

			// Step 2: Ensure PVC exists.
			pvcName := volume.PersistentVolumeClaim.ClaimName
			pvc, err := ensurePVCForUnmanagedDisk(
				ctx,
				k8sClient,
				vm,
				pvcName,
				di)
			if err != nil {
				return false, fmt.Errorf(
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
					vimClient,
					vm,
					pvcName,
					di); err != nil {

					return false, fmt.Errorf(
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

					return false, err
				}
			}
		}
	}

	return hasPendingVolumes, nil
}

// ensurePVCForUnmanagedDisk ensures a PVC exists for an unmanaged disk.
func ensurePVCForUnmanagedDisk(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
	pvcName string,
	diskInfo unmanagedvolsutil.VirtualDiskInfo) (*corev1.PersistentVolumeClaim, error) {

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

		// Assign the storage class to the PVC.
		obj.Spec.StorageClassName = &diskInfo.StorageClass

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
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	pvcName string,
	diskInfo unmanagedvolsutil.VirtualDiskInfo) (*cnsv1alpha1.CnsRegisterVolume, error) {

	var (
		obj = &cnsv1alpha1.CnsRegisterVolume{}
		key = ctrlclient.ObjectKey{
			Namespace: vm.Namespace,
			Name:      pvcName,
		}
	)

	if err := k8sClient.Get(ctx, key, obj); err != nil {

		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf(
				"failed to get CnsRegisterVolume %s: %w", key, err)
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

		// Get the datastore URL for the provided file name.
		datastoreURL, err := pkgdatastore.GetDatastoreURLFromDatastorePath(
			ctx,
			vimClient,
			diskInfo.FileName)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to get datastore url for %q: %w",
				diskInfo.FileName, err)
		}

		obj.Spec = cnsv1alpha1.CnsRegisterVolumeSpec{
			PvcName:     pvcName,
			DiskURLPath: datastoreURL,
		}

		// Set the AccessMode to ReadWriteMany if the existing,
		// underlying disk has a sharing mode set to MultiWriter. Otherwise init
		// the AccessMode to ReadWriteOnce.
		if diskInfo.Sharing == vimtypes.VirtualDiskSharingSharingMultiWriter {
			obj.Spec.AccessMode = corev1.ReadWriteMany
		} else {
			obj.Spec.AccessMode = corev1.ReadWriteOnce
		}

		// Create the CRV.
		if err := k8sClient.Create(ctx, obj); err != nil {
			return nil, fmt.Errorf("failed to create crv %s: %w", key, err)
		}
	}

	return obj, nil
}

// cleanupCnsRegisterVolumeForVM cleans up a CnsRegisterVolume object.
func cleanupCnsRegisterVolumeForVM(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
	pvcName string) error {

	if err := ctrlclient.IgnoreNotFound(k8sClient.Delete(
		ctx,
		&cnsv1alpha1.CnsRegisterVolume{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: vm.Namespace,
				Name:      pvcName,
			},
		})); err != nil {

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
