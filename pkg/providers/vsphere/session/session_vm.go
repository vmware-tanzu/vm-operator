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

	capacity := pkgutil.GetRequestedBootDiskCapacity(vmCtx.VM)
	if capacity == 0 {
		return nil, nil
	}

	// Assume the first disk is the boot disk. We can make this smarter by
	// looking at the disk path or whatever else later.
	// TODO: De-dupe this with resizeBootDiskDeviceChange() in the clone path.
	bootDisk := pkgutil.GetBootDisk(virtualDisks)
	if bootDisk == nil {
		return nil, fmt.Errorf("could not find the boot disk to change capacity")
	}

	var (
		newCapacityInBytes = capacity
		oldCapacityInBytes = pkgutil.GetDiskCapacityInBytes(bootDisk)
	)

	if newCapacityInBytes < oldCapacityInBytes {
		return nil, fmt.Errorf("cannot shrink boot disk from %d bytes to %d bytes",
			oldCapacityInBytes, newCapacityInBytes)
	}

	if oldCapacityInBytes == newCapacityInBytes {
		return nil, nil
	}

	// vSphere cannot extend a disk that has a parent, ex. the boot disk of a
	// Fast Deploy VM. Wait for the disk to be promoted.
	if pkgutil.IsLinkedCloneDisk(bootDisk, vmCtx.MoVM.LayoutEx) {
		vmCtx.Logger.V(4).Info(
			"Skipping boot disk resize until the disk is promoted",
			"requestedBytes", newCapacityInBytes,
			"currentBytes", oldCapacityInBytes)
		return nil, nil
	}

	// If the boot disk has a PVC, the PVC is the source of truth for the size,
	// ex. like any other disk. The register unmanaged volumes reconciler raises
	// the PVC request to the requested capacity.
	if pkgcfg.FromContext(vmCtx).Features.AllDisksArePVCs {
		info := pkgvol.GetVolumeInfoFromVM(vmCtx.VM, vmCtx.MoVM)
		if name := info.BootDiskPVCName(); name != "" {
			vmCtx.Logger.Info(
				"Skipping boot disk resize since the disk has a PVC",
				"pvcName", name)
			return nil, nil
		}
	}

	bootDisk.CapacityInBytes = newCapacityInBytes
	return []vimtypes.BaseVirtualDeviceConfigSpec{
		&vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationEdit,
			Device:    bootDisk,
		},
	}, nil
}
