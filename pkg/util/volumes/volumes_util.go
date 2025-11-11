// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package volumes

import (
	"context"
	"fmt"
	"slices"

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

// VirtualDiskInfo is information about a volume.
type VirtualDiskInfo struct {
	pkgutil.VirtualDiskInfo
	ProfileIDs         []string
	StorageClass       string
	StoragePolicyID    string
	Type               vmopv1.UnmanagedVolumeClaimVolumeType
	Target             VirtualDiskTarget
	NewCapacityInBytes int64
	Snapshot           bool
	LinkedClone        bool
	FCD                bool
}

// VolumeInfo is information about a VM's volumes.
type VolumeInfo struct {
	// Disks is a list of disks configured on the VM.
	Disks []VirtualDiskInfo

	// Controllers maps a VM's controller keys to information about each
	// controller.
	Controllers map[int32]ControllerSpec

	// Volumes maps a disk's target ID to the volume from the VM spec.
	Volumes map[string]vmopv1.VirtualMachineVolume
}

// FilterOutEmptyUUIDOrFilename returns only the disks that have non-empty UUIDs
// and filenames.
func FilterOutEmptyUUIDOrFilename(disks ...VirtualDiskInfo) []VirtualDiskInfo {
	return slices.DeleteFunc(
		disks,
		func(e VirtualDiskInfo) bool {
			return e.UUID == "" || e.FileName == ""
		})
}

// FilterOutSnapshots returns only the disks that are not snapshot disks.
func FilterOutSnapshots(disks ...VirtualDiskInfo) []VirtualDiskInfo {
	return slices.DeleteFunc(
		disks,
		func(e VirtualDiskInfo) bool {
			return e.Snapshot
		})
}

// FilterOutLinkedClones returns only the disks that are not linked clones.
func FilterOutLinkedClones(disks ...VirtualDiskInfo) []VirtualDiskInfo {
	return slices.DeleteFunc(
		disks,
		func(e VirtualDiskInfo) bool {
			return e.LinkedClone
		})
}

// FilterOutFCDs returns only the disks that are not FCDs.
func FilterOutFCDs(disks ...VirtualDiskInfo) []VirtualDiskInfo {
	return slices.DeleteFunc(
		disks,
		func(e VirtualDiskInfo) bool {
			return e.FCD
		})
}

// GetVolumeInfoFromVM returns information about a VM and its volumes.
func GetVolumeInfoFromVM(
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine) VolumeInfo {

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

	return GetVolumeInfo(vm, devices, snapshots)
}

// GetVolumeInfoFromConfigSpec returns information about a VM and its
// volumes from a ConfigSpec.
//
// Please note, it is recommended that pkgutil.EnsureDisksHaveControllers be
// used on the provided ConfigSpec to ensure all disks:
//   - Are connected to controllers
//   - Have non-nil and unique unit numbers
func GetVolumeInfoFromConfigSpec(
	vm *vmopv1.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec) VolumeInfo {

	if configSpec == nil {
		return VolumeInfo{}
	}

	devices := make([]vimtypes.BaseVirtualDevice, len(configSpec.DeviceChange))
	for i, dc := range configSpec.DeviceChange {
		devices[i] = dc.GetVirtualDeviceConfigSpec().Device
	}

	return GetVolumeInfo(vm, devices, nil)
}

// GetVolumeInfo returns information about a VM and its volumes.
//
//nolint:gocyclo
func GetVolumeInfo(
	vm *vmopv1.VirtualMachine,
	devices []vimtypes.BaseVirtualDevice,
	snapshots []vimtypes.VirtualMachineFileLayoutExSnapshotLayout) VolumeInfo {

	if vm == nil || len(devices) == 0 {
		return VolumeInfo{}
	}

	var (
		snapshotDiskKeys = map[int32]struct{}{}
		info             = VolumeInfo{
			Controllers: map[int32]ControllerSpec{},
			Volumes:     map[string]vmopv1.VirtualMachineVolume{},
		}
	)

	// Build a map of existing volumes by target for quick lookup.
	for _, vol := range vm.Spec.Volumes {
		if pvc := vol.PersistentVolumeClaim; pvc != nil {
			var (
				ctrlType = vol.ControllerType
				ctrlBus  = vol.ControllerBusNumber
				diskUnit = vol.UnitNumber
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
	for i, device := range devices {

		var spec ControllerSpec

		switch d := device.(type) {
		case *vimtypes.VirtualDisk:
			di := pkgutil.GetVirtualDiskInfo(d)
			if di.UnitNumber == nil {
				// This is only possible when the devices originate from a
				// ConfigSpec that has *not* been subjected to
				// pkgutil.EnsureDisksHaveControllers. Otherwise, the disks
				// from config.hardware.devices on an existing VM will always
				// have a unit number set.
				panic(fmt.Sprintf(
					"disk at device index %d missing unit number", i))
			}
			_, inSnap := snapshotDiskKeys[d.Key]
			info.Disks = append(
				info.Disks,
				VirtualDiskInfo{
					VirtualDiskInfo: di,
					Snapshot:        inSnap,
					LinkedClone:     di.HasParent && !inSnap,
					FCD:             d.VDiskId != nil && d.VDiskId.Id != "",
				})

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

type contextKey uint8

const infoContextKey contextKey = 0

// WithContext returns a new context with the provided volume info.
func WithContext(
	parent context.Context,
	info VolumeInfo) context.Context {

	return context.WithValue(parent, infoContextKey, info)
}

// FromContext returns the volume info from the context.
func FromContext(ctx context.Context) (VolumeInfo, bool) {
	obj := ctx.Value(infoContextKey)
	if obj == nil {
		return VolumeInfo{}, false
	}
	val, ok := obj.(VolumeInfo)
	return val, ok
}
