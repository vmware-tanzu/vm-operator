// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"fmt"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
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
		if newCapacityInBytes < vmDisk.CapacityInBytes {
			err := fmt.Errorf("cannot shrink boot disk from %d bytes to %d bytes",
				vmDisk.CapacityInBytes, newCapacityInBytes)
			return nil, err
		}

		if vmDisk.CapacityInBytes < newCapacityInBytes {
			// vSphere cannot extend a disk that has a parent, ex. the boot disk
			// of a Fast Deploy VM. Wait for the disk to be promoted.
			if pkgutil.GetVirtualDiskInfo(vmDisk).HasParent {
				vmCtx.Logger.Info(
					"Skipping boot disk resize until the disk is promoted",
					"requestedBytes", newCapacityInBytes,
					"currentBytes", vmDisk.CapacityInBytes)
				return nil, nil
			}

			// If the boot disk has a PVC, the PVC is the source of truth for
			// the size, ex. like any other disk. The register unmanaged volumes
			// reconciler raises the PVC request to the requested capacity.
			if pkgcfg.FromContext(vmCtx).Features.AllDisksArePVCs {
				info := pkgvol.GetVolumeInfoFromVM(vmCtx.VM, vmCtx.MoVM)
				if name := info.BootDiskPVCName(); name != "" {
					vmCtx.Logger.Info(
						"Skipping boot disk resize since the disk has a PVC",
						"pvcName", name)
					return nil, nil
				}
			}

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
