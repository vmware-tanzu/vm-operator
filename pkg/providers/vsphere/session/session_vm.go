// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"fmt"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgvol "github.com/vmware-tanzu/vm-operator/pkg/util/volumes"
)

func updateVirtualDiskDeviceChanges(
	vmCtx pkgctx.VirtualMachineContext,
	virtualDisks object.VirtualDeviceList) ([]vimtypes.BaseVirtualDeviceConfigSpec, error) {

	advanced := vmCtx.VM.Spec.Advanced
	if advanced == nil {
		return nil, nil
	}

	capacity := advanced.BootDiskCapacity
	if capacity == nil || capacity.IsZero() {
		return nil, nil
	}

	// Skip resizing ISO VMs with attached CD-ROMs as their boot disks are FCDs
	// and should be managed by PVCs.
	if hw := vmCtx.VM.Spec.Hardware; hw != nil && len(hw.Cdrom) > 0 {
		return nil, nil
	}

	var deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec
	found := false
	for _, vmDevice := range virtualDisks {
		vmDisk, ok := vmDevice.(*vimtypes.VirtualDisk)
		if !ok {
			continue
		}

		// Assume the first disk as the boot disk. We can make this smarter by
		// looking at the disk path or whatever else later.
		// TODO: De-dupe this with resizeBootDiskDeviceChange() in the clone path.

		newCapacityInBytes := capacity.Value()

		bootDisk, pvcName := getBootDiskInfo(vmCtx)

		// A linked clone, or a disk with a snapshot, cannot be extended. The
		// capacity is not applied after the VM is deployed.
		if bootDisk != nil && (bootDisk.LinkedClone || bootDisk.Snapshot) {
			vmCtx.Logger.Info(
				"Skipping spec.advanced.bootDiskCapacity since the boot "+
					"disk is a linked clone or has a snapshot",
				"linkedClone", bootDisk.LinkedClone,
				"snapshot", bootDisk.Snapshot,
				"requestedBytes", capacity.Value(),
				"currentBytes", vmDisk.CapacityInBytes)
			return nil, nil
		}

		// Once the boot disk has a PVC, the PVC is the source of truth for the
		// size of the disk, like any other disk.
		if pvcName != "" {
			if newCapacityInBytes > vmDisk.CapacityInBytes {
				vmCtx.Logger.Info(
					"Ignoring spec.advanced.bootDiskCapacity since the boot "+
						"disk has a PVC, resize the PVC instead",
					"pvcName", pvcName,
					"requestedBytes", newCapacityInBytes,
					"currentBytes", vmDisk.CapacityInBytes)
			}
			return nil, nil
		}

		if newCapacityInBytes < vmDisk.CapacityInBytes {
			err := fmt.Errorf("cannot shrink boot disk from %d bytes to %d bytes",
				vmDisk.CapacityInBytes, newCapacityInBytes)
			return nil, err
		}

		if vmDisk.CapacityInBytes < newCapacityInBytes {
			vmDisk.CapacityInBytes = newCapacityInBytes
			deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
				Operation: vimtypes.VirtualDeviceConfigSpecOperationEdit,
				Device:    vmDisk,
			})
		}

		found = true
		break
	}

	if !found {
		return nil, fmt.Errorf("could not find the boot disk to change capacity")
	}

	return deviceChanges, nil
}

// getBootDiskInfo returns the first disk, which is assumed to be the boot disk,
// and the name of the PVC that backs it, if any. The disk is nil if the VM has
// no disks.
func getBootDiskInfo(
	vmCtx pkgctx.VirtualMachineContext) (*pkgvol.VirtualDiskInfo, string) {

	info := pkgvol.GetVolumeInfoFromVM(vmCtx.VM, vmCtx.MoVM)
	if len(info.Disks) == 0 {
		return nil, ""
	}

	disk := info.Disks[0]
	vol := info.Volumes[disk.Target.String()]
	if vol == nil || vol.PersistentVolumeClaim == nil {
		return &disk, ""
	}

	return &disk, vol.PersistentVolumeClaim.ClaimName
}
