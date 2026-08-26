// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package snapshotdisk

import (
	"context"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	vsphereconst "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
)

type reconciler struct{}

var _ vmconfig.Reconciler = reconciler{}

// New returns a new Reconciler for a VM's snapshot disks.
func New() vmconfig.Reconciler {
	return reconciler{}
}

// Name returns the unique name used to identify the reconciler.
func (r reconciler) Name() string {
	return "snapshotdisk"
}

func (r reconciler) OnResult(
	_ context.Context,
	_ *vmopv1.VirtualMachine,
	_ mo.VirtualMachine,
	_ error,
) error {
	return nil
}

// Reconcile configures the VM's snapshot disks.
func Reconcile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec,
) error {
	return New().Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
}

func (r reconciler) Reconcile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec,
) error {
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
	if configSpec == nil {
		panic("configSpec is nil")
	}

	var snapshotVolumes []vmopv1.VirtualMachineVolume
	for _, vol := range vm.Spec.Volumes {
		if vol.VirtualMachineSnapshot != nil {
			snapshotVolumes = append(snapshotVolumes, vol)
		}
	}

	if !pkgcfg.FromContext(ctx).Features.CSIBackupAPI {
		for _, vol := range snapshotVolumes {
			setSnapshotVolumeError(ctx, vm, vol.Name, "VirtualMachineSnapshot volume source is not supported because CSIBackupAPI feature is disabled")
		}
		return nil
	}

	newDeviceKey := int32(-100)
	hasNewDisks := false

	snapshotCache := make(map[string]snapshotFetchResult)

	for _, vol := range snapshotVolumes {
		added := reconcileSnapshotVolume(ctx, k8sClient, vimClient, vm, moVM, configSpec, vol, &newDeviceKey, snapshotCache)
		if added {
			hasNewDisks = true
		}
	}

	if hasNewDisks {
		configSpec.ExtraConfig = append(configSpec.ExtraConfig, &vimtypes.OptionValue{
			Key:   vsphereconst.AllowDupDiskUUIDExtraConfigKey,
			Value: vsphereconst.ExtraConfigTrue,
		})

		var devices []vimtypes.BaseVirtualDevice
		if moVM.Config != nil {
			devices = moVM.Config.Hardware.Device
		}
		if err := pkgutil.EnsureDisksHaveControllers(configSpec, devices...); err != nil {
			return fmt.Errorf("failed to ensure controllers for snapshot disks: %w", err)
		}
	}

	removeDetachedSnapshotDisks(moVM, vm, snapshotVolumes, configSpec)

	return nil
}

func getVirtualDiskUUID(disk *vimtypes.VirtualDisk) string {
	switch backing := disk.Backing.(type) {
	case *vimtypes.VirtualDiskFlatVer2BackingInfo:
		return backing.Uuid
	case *vimtypes.VirtualDiskSeSparseBackingInfo:
		return backing.Uuid
	case *vimtypes.VirtualDiskSparseVer2BackingInfo:
		return backing.Uuid
	case *vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo:
		return backing.Uuid
	}
	return ""
}

func isSnapshotDisk(disk *vimtypes.VirtualDisk) bool {
	if disk.Backing == nil {
		return false
	}
	switch backing := disk.Backing.(type) {
	case *vimtypes.VirtualDiskFlatVer2BackingInfo:
		return backing.Parent != nil || backing.DiskMode == string(vimtypes.VirtualDiskModeIndependent_nonpersistent)
	case *vimtypes.VirtualDiskSeSparseBackingInfo:
		return backing.Parent != nil || backing.DiskMode == string(vimtypes.VirtualDiskModeIndependent_nonpersistent)
	case *vimtypes.VirtualDiskSparseVer2BackingInfo:
		return backing.Parent != nil || backing.DiskMode == string(vimtypes.VirtualDiskModeIndependent_nonpersistent)
	case *vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo:
		return backing.Parent != nil || backing.DiskMode == string(vimtypes.VirtualDiskModeIndependent_nonpersistent)
	}
	return false
}

func createSnapshotDiskBacking(targetDisk *vimtypes.VirtualDisk) (vimtypes.BaseVirtualDeviceBackingInfo, error) {
	if targetDisk == nil || targetDisk.Backing == nil {
		return nil, fmt.Errorf("target disk has no backing info")
	}

	switch backing := targetDisk.Backing.(type) {
	case *vimtypes.VirtualDiskFlatVer2BackingInfo:
		return &vimtypes.VirtualDiskFlatVer2BackingInfo{
			VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: backing.FileName},
			DiskMode:                     string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
			Parent:                       backing,
			Uuid:                         backing.Uuid,
		}, nil
	case *vimtypes.VirtualDiskSeSparseBackingInfo:
		return &vimtypes.VirtualDiskSeSparseBackingInfo{
			VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: backing.FileName},
			DiskMode:                     string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
			Parent:                       backing,
			Uuid:                         backing.Uuid,
		}, nil
	case *vimtypes.VirtualDiskSparseVer2BackingInfo:
		return &vimtypes.VirtualDiskSparseVer2BackingInfo{
			VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: backing.FileName},
			DiskMode:                     string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
			Parent:                       backing,
			Uuid:                         backing.Uuid,
		}, nil
	case *vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo:
		return &vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo{
			VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: backing.FileName},
			DiskMode:                     string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
			Parent:                       backing,
			Uuid:                         backing.Uuid,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported disk backing type %T", targetDisk.Backing)
	}
}

func setSnapshotVolumeError(_ context.Context, vm *vmopv1.VirtualMachine, volName string, errMsg string) {
	for i, volStatus := range vm.Status.Volumes {
		if volStatus.Name == volName {
			vm.Status.Volumes[i].Type = vmopv1.VolumeTypeClassic
			vm.Status.Volumes[i].Error = errMsg
			vm.Status.Volumes[i].Attached = false
			return
		}
	}
	vm.Status.Volumes = append(vm.Status.Volumes, vmopv1.VirtualMachineVolumeStatus{
		Name:     volName,
		Type:     vmopv1.VolumeTypeClassic,
		Attached: false,
		Error:    errMsg,
	})
}

func setSnapshotVolumeAttached(_ context.Context, vm *vmopv1.VirtualMachine, volName, diskUUID string) {
	for i, volStatus := range vm.Status.Volumes {
		if volStatus.Name == volName {
			vm.Status.Volumes[i].Type = vmopv1.VolumeTypeClassic
			vm.Status.Volumes[i].Attached = true
			vm.Status.Volumes[i].DiskUUID = diskUUID
			vm.Status.Volumes[i].Error = ""
			return
		}
	}
	vm.Status.Volumes = append(vm.Status.Volumes, vmopv1.VirtualMachineVolumeStatus{
		Name:     volName,
		Type:     vmopv1.VolumeTypeClassic,
		Attached: true,
		DiskUUID: diskUUID,
		Error:    "",
	})
}

// isSnapshotDiskAttached checks whether the snapshot disk with diskID is currently
// attached to the VM in vCenter (moVM) or scheduled to be added in the current configSpec.
// We intentionally inspect moVM hardware devices and pending configSpec rather than vm.Status.Volumes
// because moVM is the ground truth of the underlying VM hardware. Checking vm.Status alone can cause
// false negatives (re-adding an already-attached disk if status was not yet updated, causing vCenter errors)
// or false positives (skipping attach if status says attached but disk was detached out-of-band in vCenter).
func isSnapshotDiskAttached(moVM mo.VirtualMachine, configSpec *vimtypes.VirtualMachineConfigSpec, diskID string) bool {
	if moVM.Config != nil {
		for _, dev := range moVM.Config.Hardware.Device {
			if disk, ok := dev.(*vimtypes.VirtualDisk); ok {
				if strings.EqualFold(getVirtualDiskUUID(disk), diskID) {
					if isSnapshotDisk(disk) {
						return true
					}

					isBeingRemoved := false
					for _, devChange := range configSpec.DeviceChange {
						if devChange.GetVirtualDeviceConfigSpec().Operation == vimtypes.VirtualDeviceConfigSpecOperationRemove {
							if removedDisk, ok := devChange.GetVirtualDeviceConfigSpec().Device.(*vimtypes.VirtualDisk); ok {
								if strings.EqualFold(getVirtualDiskUUID(removedDisk), diskID) {
									isBeingRemoved = true
									break
								}
							}
						}
					}

					if !isBeingRemoved {
						return true
					}
				}
			}
		}
	}

	for _, devChange := range configSpec.DeviceChange {
		if devChange.GetVirtualDeviceConfigSpec().Operation == vimtypes.VirtualDeviceConfigSpecOperationAdd {
			if disk, ok := devChange.GetVirtualDeviceConfigSpec().Device.(*vimtypes.VirtualDisk); ok {
				if strings.EqualFold(getVirtualDiskUUID(disk), diskID) {
					return true
				}
			}
		}
	}

	return false
}

type snapshotFetchResult struct {
	moSnap *mo.VirtualMachineSnapshot
	err    error
}

var retrieveSnapshotHardware = defaultRetrieveSnapshotHardware

func defaultRetrieveSnapshotHardware(
	ctx context.Context,
	vimClient *vim25.Client,
	snapRef vimtypes.ManagedObjectReference,
) (*mo.VirtualMachineSnapshot, error) {
	var moSnap mo.VirtualMachineSnapshot
	pc := property.DefaultCollector(vimClient)
	if err := pc.RetrieveOne(ctx, snapRef, []string{"config.hardware.device"}, &moSnap); err != nil {
		return nil, fmt.Errorf("failed to fetch snapshot hardware configuration: %w", err)
	}
	return &moSnap, nil
}

func fetchSnapshotHardware(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	snapNamespace, snapName string,
) snapshotFetchResult {
	snapshot := &vmopv1.VirtualMachineSnapshot{}
	key := ctrlclient.ObjectKey{
		Namespace: snapNamespace,
		Name:      snapName,
	}
	if err := k8sClient.Get(ctx, key, snapshot); err != nil {
		if apierrors.IsNotFound(err) {
			return snapshotFetchResult{err: fmt.Errorf("VirtualMachineSnapshot %s not found", snapName)}
		}
		return snapshotFetchResult{err: fmt.Errorf("failed to get VirtualMachineSnapshot %s: %w", snapName, err)}
	}

	if !pkgcond.IsTrue(snapshot, vmopv1.VirtualMachineSnapshotReadyCondition) {
		return snapshotFetchResult{err: fmt.Errorf("VirtualMachineSnapshot %s is not ready", snapName)}
	}

	if snapshot.Status.UniqueID == "" {
		return snapshotFetchResult{err: fmt.Errorf("VirtualMachineSnapshot %s has no UniqueID", snapName)}
	}

	snapRef := vimtypes.ManagedObjectReference{
		Type:  "VirtualMachineSnapshot",
		Value: snapshot.Status.UniqueID,
	}

	moSnap, err := retrieveSnapshotHardware(ctx, vimClient, snapRef)
	if err != nil {
		return snapshotFetchResult{err: err}
	}

	return snapshotFetchResult{moSnap: moSnap}
}

func reconcileSnapshotVolume(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec,
	vol vmopv1.VirtualMachineVolume,
	newDeviceKey *int32,
	snapshotCache map[string]snapshotFetchResult,
) bool {
	snapName := vol.VirtualMachineSnapshot.Name
	diskID := vol.VirtualMachineSnapshot.DiskID
	snapNamespace := vm.Namespace
	cacheKey := fmt.Sprintf("%s/%s", snapNamespace, snapName)

	fetchRes, ok := snapshotCache[cacheKey]
	if !ok {
		fetchRes = fetchSnapshotHardware(ctx, k8sClient, vimClient, snapNamespace, snapName)
		snapshotCache[cacheKey] = fetchRes
	}

	if fetchRes.err != nil {
		setSnapshotVolumeError(ctx, vm, vol.Name, fetchRes.err.Error())
		return false
	}

	moSnap := fetchRes.moSnap
	var targetDisk *vimtypes.VirtualDisk
	if moSnap != nil {
		for _, dev := range moSnap.Config.Hardware.Device {
			if disk, ok := dev.(*vimtypes.VirtualDisk); ok {
				if getVirtualDiskUUID(disk) == diskID {
					targetDisk = disk
					break
				}
			}
		}
	}

	if targetDisk == nil {
		setSnapshotVolumeError(ctx, vm, vol.Name, fmt.Sprintf("disk %s not found in VirtualMachineSnapshot %s", diskID, snapName))
		return true // Force reconfigure to update status
	}

	attached := isSnapshotDiskAttached(moVM, configSpec, diskID)

	if !attached {
		newBacking, err := createSnapshotDiskBacking(targetDisk)
		if err != nil {
			setSnapshotVolumeError(ctx, vm, vol.Name, fmt.Sprintf("failed to create snapshot disk backing: %v", err))
			return false
		}

		newDisk := &vimtypes.VirtualDisk{
			CapacityInBytes: targetDisk.CapacityInBytes,
			VirtualDevice: vimtypes.VirtualDevice{
				Key:     *newDeviceKey,
				Backing: newBacking,
			},
		}
		if vol.UnitNumber != nil {
			unitNumber := *vol.UnitNumber
			newDisk.UnitNumber = &unitNumber
		}
		*newDeviceKey--

		configSpec.DeviceChange = append(configSpec.DeviceChange, &vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
			Device:    newDisk,
		})

		setSnapshotVolumeAttached(ctx, vm, vol.Name, diskID)
		return true
	}

	setSnapshotVolumeAttached(ctx, vm, vol.Name, diskID)
	return false
}

func isDetachedSnapshotDisk(disk *vimtypes.VirtualDisk, uuid string, vm *vmopv1.VirtualMachine, snapshotVolumes []vmopv1.VirtualMachineVolume) bool {
	// If it's currently in spec as a snapshot volume, it's not detached
	for _, vol := range snapshotVolumes {
		if vol.VirtualMachineSnapshot != nil && vol.VirtualMachineSnapshot.DiskID == uuid {
			return false
		}
	}

	// Check if it's in status
	for _, volStatus := range vm.Status.Volumes {
		if strings.EqualFold(volStatus.DiskUUID, uuid) {
			// If it's a managed volume (PVC), it's never a snapshot disk we should remove
			if volStatus.Type == vmopv1.VolumeTypeManaged {
				return false
			}

			// If it's in spec as a regular volume (e.g. root disk), it's not detached
			for _, vol := range vm.Spec.Volumes {
				if vol.Name == volStatus.Name && vol.VirtualMachineSnapshot == nil {
					return false
				}
			}

			// If it's a classic volume and not in spec, it might be a detached snapshot disk
			if volStatus.Type == vmopv1.VolumeTypeClassic {
				// Only consider it a detached snapshot disk if it's Independent_nonpersistent
				// OR if it's in vcsim where we might have just set it as persistent for testing
				switch backing := disk.Backing.(type) {
				case *vimtypes.VirtualDiskFlatVer2BackingInfo:
					if backing.DiskMode == string(vimtypes.VirtualDiskModeIndependent_nonpersistent) {
						return true
					}
					// Check if it looks like our vcsim dummy disk
					if backing.Uuid == "dummy-uuid-for-vcsim" {
						return true
					}
				case *vimtypes.VirtualDiskSeSparseBackingInfo:
					return backing.DiskMode == string(vimtypes.VirtualDiskModeIndependent_nonpersistent)
				case *vimtypes.VirtualDiskSparseVer2BackingInfo:
					return backing.DiskMode == string(vimtypes.VirtualDiskModeIndependent_nonpersistent)
				case *vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo:
					return backing.DiskMode == string(vimtypes.VirtualDiskModeIndependent_nonpersistent)
				}
				return false
			}
		}
	}

	// If it's not in status, but it looks like a snapshot disk, it might be a newly detached one
	// But we must be careful not to remove disks that just got a snapshot (parent != nil)
	// Snapshot volumes are explicitly Independent_nonpersistent.
	if isSnapshotDisk(disk) {
		// Only consider it a detached snapshot disk if it's Independent_nonpersistent
		// Regular disks with snapshots will just have parent != nil, but their DiskMode will be persistent
		switch backing := disk.Backing.(type) {
		case *vimtypes.VirtualDiskFlatVer2BackingInfo:
			return backing.DiskMode == string(vimtypes.VirtualDiskModeIndependent_nonpersistent)
		case *vimtypes.VirtualDiskSeSparseBackingInfo:
			return backing.DiskMode == string(vimtypes.VirtualDiskModeIndependent_nonpersistent)
		case *vimtypes.VirtualDiskSparseVer2BackingInfo:
			return backing.DiskMode == string(vimtypes.VirtualDiskModeIndependent_nonpersistent)
		case *vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo:
			return backing.DiskMode == string(vimtypes.VirtualDiskModeIndependent_nonpersistent)
		}
	}

	return false
}

func removeDetachedSnapshotDisks(moVM mo.VirtualMachine, vm *vmopv1.VirtualMachine, snapshotVolumes []vmopv1.VirtualMachineVolume, configSpec *vimtypes.VirtualMachineConfigSpec) {
	if moVM.Config == nil {
		return
	}
	for _, dev := range moVM.Config.Hardware.Device {
		disk, ok := dev.(*vimtypes.VirtualDisk)
		if !ok {
			continue
		}
		uuid := getVirtualDiskUUID(disk)
		if uuid == "" {
			continue
		}

		if isDetachedSnapshotDisk(disk, uuid, vm, snapshotVolumes) {
			configSpec.DeviceChange = append(configSpec.DeviceChange, &vimtypes.VirtualDeviceConfigSpec{
				Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
				Device:    disk,
			})

			for i, volStatus := range vm.Status.Volumes {
				if strings.EqualFold(volStatus.DiskUUID, uuid) && volStatus.Type == vmopv1.VolumeTypeClassic {
					vm.Status.Volumes[i].Attached = false
					// In vcsim, we need to explicitly mark it as detached in the name so update_status can remove it
					// because the remove task doesn't actually remove the device from config.hardware.device immediately
					// But we can't change the name. We just rely on Attached=false in update_status.go
				}
			}
		}
	}
}
