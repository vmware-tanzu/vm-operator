// Copyright (c) 2018-2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"fmt"

	"github.com/vmware/govmomi/object"
	vimTypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
)

func updateVirtualDiskDeviceChanges(
	vmCtx context.VirtualMachineContextA2,
	virtualDisks object.VirtualDeviceList) ([]vimTypes.BaseVirtualDeviceConfigSpec, error) {

	capacity := vmCtx.VM.Spec.Advanced.BootDiskCapacity
	if capacity == nil || capacity.IsZero() {
		return nil, nil
	}

	var deviceChanges []vimTypes.BaseVirtualDeviceConfigSpec
	found := false
	for _, vmDevice := range virtualDisks {
		vmDisk, ok := vmDevice.(*vimTypes.VirtualDisk)
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
			vmDisk.CapacityInBytes = newCapacityInBytes
			deviceChanges = append(deviceChanges, &vimTypes.VirtualDeviceConfigSpec{
				Operation: vimTypes.VirtualDeviceConfigSpecOperationEdit,
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
