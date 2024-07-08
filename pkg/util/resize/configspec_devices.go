// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package resize

import (
	"fmt"
	"reflect"
	"slices"

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

	simpleChanges := compareDevicesSimple(deviceList, csDeviceList)
	deviceChanges = append(deviceChanges, simpleChanges...)

	outCS.DeviceChange = deviceChanges
}

// selectByTypes returns a new list with devices that are equal to or extend the given type.
func selectByTypes(deviceTypes []vimtypes.BaseVirtualDevice) func(device vimtypes.BaseVirtualDevice) bool {
	// Modeled off of VirtualDeviceList::SelectByType.

	dtypes := map[reflect.Type]struct{}{}
	dnames := map[string]struct{}{}
	for _, deviceType := range deviceTypes {
		dtype := reflect.TypeOf(deviceType)
		if dtype != nil {
			dtypes[dtype] = struct{}{}
			dname := dtype.Elem().Name()
			dnames[dname] = struct{}{}
		}
	}

	return func(device vimtypes.BaseVirtualDevice) bool {
		t := reflect.TypeOf(device)

		if _, ok := dtypes[t]; ok {
			return true
		}

		for dname := range dnames {
			_, ok := t.Elem().FieldByName(dname)
			if ok {
				return true
			}
		}

		return false
	}
}

func compareDevicesSimple(
	currentDevices, expectedDevices []vimtypes.BaseVirtualDevice) []vimtypes.BaseVirtualDeviceConfigSpec {

	devTypes := []vimtypes.BaseVirtualDevice{
		// vimtypes.VirtualSoundCard{}:
		&vimtypes.VirtualEnsoniq1371{},
		&vimtypes.VirtualHdAudioCard{},
		&vimtypes.VirtualSoundBlaster16{},

		&vimtypes.VirtualUSBController{},
		&vimtypes.VirtualUSBXHCIController{},
	}
	selectFunc := selectByTypes(devTypes)

	currentDevices = object.VirtualDeviceList(currentDevices).Select(selectFunc)
	expectedDevices = object.VirtualDeviceList(expectedDevices).Select(selectFunc)

	var deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec //nolint:prealloc
	for _, expectedDev := range expectedDevices {
		expectedDevType := reflect.TypeOf(expectedDev)
		matchingIdx := -1
		var editDeviceChanges []vimtypes.BaseVirtualDeviceConfigSpec

		for idx, curDev := range currentDevices {
			curDevType := reflect.TypeOf(curDev)
			if expectedDevType != curDevType {
				continue
			}

			match := false

			switch d := expectedDev.(type) {
			case vimtypes.BaseVirtualSoundCard:
				match, editDeviceChanges = matchSoundCards(d, curDev.(vimtypes.BaseVirtualSoundCard))
			case *vimtypes.VirtualUSBController:
				match, editDeviceChanges = matchUSBControllers(d, curDev.(*vimtypes.VirtualUSBController))
			case *vimtypes.VirtualUSBXHCIController:
				match, editDeviceChanges = matchUSBSHCIController(d, curDev.(*vimtypes.VirtualUSBXHCIController))
			default:
				panic(fmt.Sprintf("case for type %T missing in devTypes", d))
			}

			if match {
				matchingIdx = idx
				break
			}
		}

		if matchingIdx == -1 {
			deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
				Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
				Device:    expectedDev,
			})
		} else {
			deviceChanges = append(deviceChanges, editDeviceChanges...)
			currentDevices = slices.Delete(currentDevices, matchingIdx, matchingIdx+1)
		}
	}

	for _, curDev := range currentDevices {
		deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
			Device:    curDev,
		})
	}

	return deviceChanges
}

func matchSoundCards(
	_, _ vimtypes.BaseVirtualSoundCard) (bool, []vimtypes.BaseVirtualDeviceConfigSpec) {
	// All sound cards are just VirtualSoundCards without any options so just the types matter.
	return true, nil
}

func matchUSBControllers(
	expectedDev, curDev *vimtypes.VirtualUSBController) (bool, []vimtypes.BaseVirtualDeviceConfigSpec) {

	edit := false

	// TODO: Handle the VirtualController

	if expectedDev.AutoConnectDevices != nil {
		if curDev.AutoConnectDevices == nil || *expectedDev.AutoConnectDevices != *curDev.AutoConnectDevices {
			curDev.AutoConnectDevices = expectedDev.AutoConnectDevices
			edit = true
		}
	}

	if expectedDev.EhciEnabled != nil {
		if curDev.EhciEnabled == nil || *expectedDev.EhciEnabled != *curDev.EhciEnabled {
			curDev.EhciEnabled = expectedDev.EhciEnabled
			edit = true
		}
	}

	var deviceEdits []vimtypes.BaseVirtualDeviceConfigSpec
	if edit {
		deviceEdits = append(deviceEdits, &vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationEdit,
			Device:    curDev,
		})
	}

	return true, deviceEdits
}

func matchUSBSHCIController(
	expectedDev, curDev *vimtypes.VirtualUSBXHCIController) (bool, []vimtypes.BaseVirtualDeviceConfigSpec) {

	edit := false

	// TODO: Handle the VirtualController

	if expectedDev.AutoConnectDevices != nil {
		if curDev.AutoConnectDevices == nil || *expectedDev.AutoConnectDevices != *curDev.AutoConnectDevices {
			curDev.AutoConnectDevices = expectedDev.AutoConnectDevices
			edit = true
		}
	}

	var deviceEdits []vimtypes.BaseVirtualDeviceConfigSpec
	if edit {
		deviceEdits = append(deviceEdits, &vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationEdit,
			Device:    curDev,
		})
	}

	return true, deviceEdits
}

func comparePCIDevices(
	currentDevices, desiredPCIDevices []vimtypes.BaseVirtualDevice) []vimtypes.BaseVirtualDeviceConfigSpec {

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
