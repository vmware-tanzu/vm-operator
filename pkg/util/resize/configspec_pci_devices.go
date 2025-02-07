// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package resize

import (
	"reflect"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

// comparePCIDevices is the old "Session" comparison code in which we try to match devices
// by the backing instead of zipping.
func comparePCIDevices(
	desiredPCIDevices, currentDevices []vimtypes.BaseVirtualDevice) []vimtypes.BaseVirtualDeviceConfigSpec {

	currentPassthruPCIDevices := pkgutil.SelectVirtualPCIPassthrough(currentDevices)

	pciPassthruFromConfigSpec := pkgutil.SelectVirtualPCIPassthrough(desiredPCIDevices)
	expectedPCIDevices := virtualmachine.CreatePCIDevicesFromConfigSpec(pciPassthruFromConfigSpec)

	var deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec
	for _, expectedDev := range expectedPCIDevices {
		expectedPci := expectedDev.(*vimtypes.VirtualPCIPassthrough)
		expectedBacking := expectedPci.Backing
		expectedBackingType := reflect.TypeOf(expectedBacking)

		var matchingIdx = -1
		for idx, curDev := range currentPassthruPCIDevices {
			curBacking := curDev.GetVirtualDevice().Backing
			if curBacking == nil || reflect.TypeOf(curBacking) != expectedBackingType {
				continue
			}

			var backingMatch bool
			switch a := curBacking.(type) {
			case *vimtypes.VirtualPCIPassthroughVmiopBackingInfo:
				b := expectedBacking.(*vimtypes.VirtualPCIPassthroughVmiopBackingInfo)
				backingMatch = a.Vgpu == b.Vgpu

			case *vimtypes.VirtualPCIPassthroughDynamicBackingInfo:
				currAllowedDevs := a.AllowedDevice
				b := expectedBacking.(*vimtypes.VirtualPCIPassthroughDynamicBackingInfo)
				if a.CustomLabel == b.CustomLabel && len(b.AllowedDevice) > 0 {
					// b.AllowedDevice has only one element because CreatePCIDevices() adds only one device based
					// on the devices listed in vmclass.spec.hardware.devices.dynamicDirectPathIODevices.
					expectedAllowedDev := b.AllowedDevice[0]
					for i := 0; i < len(currAllowedDevs) && !backingMatch; i++ {
						backingMatch = expectedAllowedDev.DeviceId == currAllowedDevs[i].DeviceId &&
							expectedAllowedDev.VendorId == currAllowedDevs[i].VendorId
					}
				}
			}

			if backingMatch {
				matchingIdx = idx
				break
			}
		}

		if matchingIdx == -1 {
			deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
				Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
				Device:    expectedPci,
			})
		} else {
			// There could be multiple vGPUs with same BackingInfo. Remove current device if matching found.
			currentPassthruPCIDevices = append(currentPassthruPCIDevices[:matchingIdx], currentPassthruPCIDevices[matchingIdx+1:]...)
		}
	}
	// Remove any unmatched existing devices.
	removeDeviceChanges := make([]vimtypes.BaseVirtualDeviceConfigSpec, 0, len(currentPassthruPCIDevices))
	for _, dev := range currentPassthruPCIDevices {
		removeDeviceChanges = append(removeDeviceChanges, &vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
			Device:    dev,
		})
	}

	// Process any removes first.
	return append(removeDeviceChanges, deviceChanges...)
}
