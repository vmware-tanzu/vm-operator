// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"fmt"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// ControllerSpec is information about a VirtualDeviceController to which an
// unmanaged volume is attached.
type ControllerSpec struct {
	Key         int32
	Bus         int32
	Type        vmopv1.VirtualControllerType
	ScsiType    vmopv1.SCSIControllerType
	SharingMode vmopv1.VirtualControllerSharingMode
}

// VirtualDiskTarget is the controller:bus:slot target ID at which a disk is
// located.
type VirtualDiskTarget struct {
	ControllerType vmopv1.VirtualControllerType
	ControllerBus  int32
	UnitNumber     int32
}

func (t VirtualDiskTarget) String() string {
	return fmt.Sprintf("%s:%d:%d",
		t.ControllerType, t.ControllerBus, t.UnitNumber)
}

// VirtualDiskInfo is information about an unmanaged volume.
type VirtualDiskInfo struct {
	pkgutil.VirtualDiskInfo
	ProfileIDs         []string
	StorageClass       string
	StoragePolicyID    string
	Type               vmopv1.UnmanagedVolumeClaimVolumeType
	Target             VirtualDiskTarget
	NewCapacityInBytes int64
}

// UnmanagedVolumeInfo is information about a VM's unmanaged volumes.
type UnmanagedVolumeInfo struct {
	Disks       []VirtualDiskInfo
	Controllers map[int32]ControllerSpec

	// Volumes maps a disk's target ID to the volume from the VM spec.
	Volumes map[string]vmopv1.VirtualMachineVolume
}

// GetUnmanagedVolumeInfoFromVM returns information about a VM and its
// unmanaged volumes.
func GetUnmanagedVolumeInfoFromVM(
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	skippedLinkedClones bool) UnmanagedVolumeInfo {

	var (
		devices   []vimtypes.BaseVirtualDevice
		snapshots []vimtypes.VirtualMachineFileLayoutExSnapshotLayout
	)

	if moVM.Config != nil {
		devices = moVM.Config.Hardware.Device
	}
	if moVM.LayoutEx != nil {
		snapshots = moVM.LayoutEx.Snapshot
	}

	return GetUnmanagedVolumeInfo(
		vm,
		devices,
		snapshots,
		skippedLinkedClones)
}

// GetUnmanagedVolumeInfoFromConfigSpec returns information about a VM and its
// unmanaged volumes from a ConfigSpec.
func GetUnmanagedVolumeInfoFromConfigSpec(
	vm *vmopv1.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec) UnmanagedVolumeInfo {

	if configSpec == nil {
		return UnmanagedVolumeInfo{}
	}

	devices := make([]vimtypes.BaseVirtualDevice, len(configSpec.DeviceChange))
	for i, dc := range configSpec.DeviceChange {
		devices[i] = dc.GetVirtualDeviceConfigSpec().Device
	}

	return GetUnmanagedVolumeInfo(
		vm,
		devices,
		nil,
		false)
}

// GetUnmanagedVolumeInfo returns information about a VM and its
// unmanaged volumes.
//
//nolint:gocyclo
func GetUnmanagedVolumeInfo(
	vm *vmopv1.VirtualMachine,
	devices []vimtypes.BaseVirtualDevice,
	snapshots []vimtypes.VirtualMachineFileLayoutExSnapshotLayout,
	skippedLinkedClones bool) UnmanagedVolumeInfo {

	if vm == nil || len(devices) == 0 {
		return UnmanagedVolumeInfo{}
	}

	var (
		snapshotDiskKeys = map[int32]struct{}{}
		info             = UnmanagedVolumeInfo{
			Controllers: map[int32]ControllerSpec{},
			Volumes:     map[string]vmopv1.VirtualMachineVolume{},
		}
	)

	// Build a map of existing volumes by target for quick lookup.
	for _, vol := range vm.Spec.Volumes {
		if pvc := vol.PersistentVolumeClaim; pvc != nil {
			var (
				ctrlType = pvc.ControllerType
				ctrlBus  = pvc.ControllerBusNumber
				diskUnit = pvc.UnitNumber
			)
			if ctrlType != "" && ctrlBus != nil && diskUnit != nil {
				t := VirtualDiskTarget{
					ControllerType: ctrlType,
					ControllerBus:  *ctrlBus,
					UnitNumber:     *diskUnit,
				}
				info.Volumes[t.String()] = vol
			}
		}
	}

	// Find all of the disks that are participating in snapshots.
	for _, snap := range snapshots {
		for _, disk := range snap.Disk {
			snapshotDiskKeys[disk.Key] = struct{}{}
		}
	}

	// Get the virtual disk and controller information.
	for _, device := range devices {

		var spec ControllerSpec

		switch d := device.(type) {
		case *vimtypes.VirtualDisk:
			if d.VDiskId == nil || d.VDiskId.Id == "" { // Skip FCDs.
				di := pkgutil.GetVirtualDiskInfo(d)
				if di.UnitNumber == nil {
					di.UnitNumber = ptr.To[int32](0)
				}
				if di.UUID != "" && di.FileName != "" { // Skip if no UUID/file.
					_, inSnap := snapshotDiskKeys[d.Key]
					if !skippedLinkedClones ||
						!di.HasParent ||
						(di.HasParent && inSnap) {
						// Include disks that are not child disks, or if they
						// are, are the running point for a snapshot.
						info.Disks = append(
							info.Disks,
							VirtualDiskInfo{VirtualDiskInfo: di})
					}
				}
			}

		case vimtypes.BaseVirtualController:
			bd := d.GetVirtualController()

			spec.Key = bd.Key
			spec.Bus = bd.BusNumber

			switch dd := d.(type) {

			case *vimtypes.VirtualIDEController:
				spec.Type = vmopv1.VirtualControllerTypeIDE

			case vimtypes.BaseVirtualSCSIController:
				spec.Type = vmopv1.VirtualControllerTypeSCSI

				switch d.(type) {
				case *vimtypes.ParaVirtualSCSIController:
					spec.ScsiType = vmopv1.SCSIControllerTypeParaVirtualSCSI
				case *vimtypes.VirtualLsiLogicController:
					spec.ScsiType = vmopv1.SCSIControllerTypeLsiLogic
				case *vimtypes.VirtualLsiLogicSASController:
					spec.ScsiType = vmopv1.SCSIControllerTypeLsiLogicSAS
				case *vimtypes.VirtualBusLogicController:
					spec.ScsiType = vmopv1.SCSIControllerTypeBusLogic
				}

				switch dd.GetVirtualSCSIController().SharedBus {
				case vimtypes.VirtualSCSISharingNoSharing:
					spec.SharingMode = vmopv1.VirtualControllerSharingModeNone
				case vimtypes.VirtualSCSISharingPhysicalSharing:
					spec.SharingMode = vmopv1.VirtualControllerSharingModePhysical
				case vimtypes.VirtualSCSISharingVirtualSharing:
					spec.SharingMode = vmopv1.VirtualControllerSharingModeVirtual
				}

			case *vimtypes.VirtualNVMEController:
				spec.Type = vmopv1.VirtualControllerTypeNVME

				switch dd.SharedBus {
				case string(vimtypes.VirtualNVMEControllerSharingNoSharing):
					spec.SharingMode = vmopv1.VirtualControllerSharingModeNone
				case string(vimtypes.VirtualNVMEControllerSharingPhysicalSharing):
					spec.SharingMode = vmopv1.VirtualControllerSharingModePhysical
				}

			case vimtypes.BaseVirtualSATAController:
				spec.Type = vmopv1.VirtualControllerTypeSATA
			}

			if spec.Type != "" {
				info.Controllers[bd.Key] = spec
			}
		}
	}

	// Update the list of disks with their target IDs.
	for i, di := range info.Disks {
		t := VirtualDiskTarget{
			ControllerType: info.Controllers[di.ControllerKey].Type,
			ControllerBus:  info.Controllers[di.ControllerKey].Bus,
			UnitNumber:     *di.UnitNumber,
		}
		info.Disks[i].Target = t

		// Update the disk info with the volume type if it is known.
		if v, ok := info.Volumes[t.String()]; ok {
			if pvc := v.PersistentVolumeClaim; pvc != nil {
				if uvc := pvc.UnmanagedVolumeClaim; uvc != nil {
					info.Disks[i].Type = uvc.Type
				}
			}
		}
	}

	return info
}
