// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
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

// VirtualDiskInfo is information about an unmanaged volume.
type VirtualDiskInfo struct {
	pkgutil.VirtualDiskInfo
	ProfileIDs      []string
	StorageClass    string
	StoragePolicyID string
	Type            vmopv1.UnmanagedVolumeClaimVolumeType
}

// UnmanagedVolumeInfo is information about a VM's unmanaged volumes.
type UnmanagedVolumeInfo struct {
	Disks       []VirtualDiskInfo
	Controllers map[int32]ControllerSpec
	Volumes     map[string]vmopv1.VirtualMachineVolume
}

// GetUnmanagedVolumeInfo returns information about a VM and its
// unmanaged volumes.
func GetUnmanagedVolumeInfo(
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine) UnmanagedVolumeInfo {

	if vm == nil ||
		moVM.Config == nil ||
		len(moVM.Config.Hardware.Device) == 0 ||
		moVM.LayoutEx == nil {

		return UnmanagedVolumeInfo{}
	}

	var (
		snapshotDiskKeys = map[int32]struct{}{}
		info             = UnmanagedVolumeInfo{
			Controllers: map[int32]ControllerSpec{},
			Volumes:     map[string]vmopv1.VirtualMachineVolume{},
		}
	)

	// Find all of the disks that are participating in snapshots.
	for _, snap := range moVM.LayoutEx.Snapshot {
		for _, disk := range snap.Disk {
			snapshotDiskKeys[disk.Key] = struct{}{}
		}
	}

	// Build a map of existing volumes by name for quick lookup.
	for _, vol := range vm.Spec.Volumes {
		if pvc := vol.PersistentVolumeClaim; pvc != nil {
			if uvc := pvc.UnmanagedVolumeClaim; uvc != nil {
				info.Volumes[uvc.UUID] = vol
			}
		}
	}

	// Get the virtual disk and controller information.
	for _, device := range moVM.Config.Hardware.Device {

		var spec ControllerSpec

		switch d := device.(type) {
		case *vimtypes.VirtualDisk:
			if d.VDiskId == nil || d.VDiskId.Id == "" { // Skip FCDs.
				di := pkgutil.GetVirtualDiskInfo(d)
				if di.UUID != "" && di.FileName != "" { // Skip if no UUID/file.
					_, inSnap := snapshotDiskKeys[d.Key]
					if !di.HasParent || (di.HasParent && inSnap) {
						// Include disks that are not child disks, or if they
						// are, are the running point for a snapshot.
						var diskType vmopv1.UnmanagedVolumeClaimVolumeType
						if vol, ok := info.Volumes[di.UUID]; ok {
							uvc := vol.PersistentVolumeClaim.UnmanagedVolumeClaim
							if uvc != nil {
								diskType = uvc.Type
							}
						}

						info.Disks = append(
							info.Disks,
							VirtualDiskInfo{
								VirtualDiskInfo: di,
								Type:            diskType,
							})
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

	return info
}
