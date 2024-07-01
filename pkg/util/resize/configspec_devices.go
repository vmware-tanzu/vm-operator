// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package resize

import (
	"reflect"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

func compareHardwareDevices(
	ci vimtypes.VirtualMachineConfigInfo,
	cs vimtypes.VirtualMachineConfigSpec,
	outCS *vimtypes.VirtualMachineConfigSpec) {

	// The VM's current virtual devices.
	deviceList := object.VirtualDeviceList(ci.Hardware.Device)
	// The VM's desired virtual devices.
	csDeviceList := pkgutil.DevicesFromConfigSpec(&cs)

	var deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec

	pciDeviceChanges := comparePCIDevices(deviceList, csDeviceList)
	deviceChanges = append(deviceChanges, pciDeviceChanges...)

	outCS.DeviceChange = deviceChanges
}

func comparePCIDevices(
	currentPCIDevices, desiredPCIDevices []vimtypes.BaseVirtualDevice) []vimtypes.BaseVirtualDeviceConfigSpec {

	currentPassthruPCIDevices := pkgutil.SelectVirtualPCIPassthrough(currentPCIDevices)

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
