// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package resize

import (
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

	pciDeviceChanges := comparePCIDevices(csDeviceList, deviceList)
	deviceChanges = append(deviceChanges, pciDeviceChanges...)

	moreDeviceChanges := compareDevicesByZipping(csDeviceList, deviceList)
	deviceChanges = append(deviceChanges, moreDeviceChanges...)

	outCS.DeviceChange = deviceChanges
}

// compareDevicesByZipping determine what if any DeviceChange entries are needed by zipping
// the expected and current devices of each supported type together. That is, the devices are
// compared in their relative order, and Edit, Add, and/or Remove DeviceChanges are created
// if needed to update the VM configuration.
func compareDevicesByZipping(
	expectedDevices, currentDevices []vimtypes.BaseVirtualDevice) []vimtypes.BaseVirtualDeviceConfigSpec {

	/*
		// &vimtypes.VirtualIDEController{} (default device)
		// &vimtypes.VirtualNVDIMMController{}
		// &vimtypes.VirtualNVMEController{}
		// &vimtypes.VirtualPCIController{} (default device)
		// &vimtypes.VirtualPS2Controller{} (default device)
		// &vimtypes.VirtualSATAController{}
		// &vimtypes.VirtualAHCIController{}
		// &vimtypes.ParaVirtualSCSIController{}
		// &vimtypes.VirtualBusLogicController{}
		// &vimtypes.VirtualLsiLogicController{}
		// &vimtypes.VirtualLsiLogicSASController{}
		// &vimtypes.VirtualSIOController{} (default device)
		&vimtypes.VirtualUSBController{}
		&vimtypes.VirtualUSBXHCIController{}
		// &vimtypes.VirtualFloppy{}
		// &vimtypes.VirtualKeyboard{} (default device)
		&vimtypes.VirtualMachineVMCIDevice{} (default device)
		&vimtypes.VirtualMachineVideoCard{} (default device)
		// &vimtypes.VirtualPCIPassthrough{}
		&vimtypes.VirtualParallelPort{}
		&vimtypes.VirtualPointingDevice{} (default device)
		&vimtypes.VirtualPrecisionClock{}
		&vimtypes.VirtualSCSIPassthrough{}
		&vimtypes.VirtualSerialPort{}
		&vimtypes.VirtualEnsoniq1371{}
		&vimtypes.VirtualHdAudioCard{}
		&vimtypes.VirtualSoundBlaster16{}
		&vimtypes.VirtualTPM{}
		// &vimtypes.VirtualUSB{}
		&vimtypes.VirtualWDT{}
	*/

	var deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec

	// TODO:
	//   Is there a right generics machination so we put this in a map w/o reflection madness
	//   Add a GroupByType() so we don't keep iterating over the whole device lists
	//   Ideally have the fetched VirtualDeviceOptions for a few fields so we know the default

	{
		exp, cur := selectDevsByType[*vimtypes.VirtualUSBController](expectedDevices, currentDevices)
		dc := zipVirtualDevicesOfType(MatchVirtualUSBController, exp, cur, false)
		deviceChanges = append(deviceChanges, dc...)
	}

	{
		exp, cur := selectDevsByType[*vimtypes.VirtualUSBXHCIController](expectedDevices, currentDevices)
		dc := zipVirtualDevicesOfType(MatchVirtualUSBXHCIController, exp, cur, false)
		deviceChanges = append(deviceChanges, dc...)
	}

	{
		exp, cur := selectDevsByType[*vimtypes.VirtualMachineVMCIDevice](expectedDevices, currentDevices)
		dc := zipVirtualDevicesOfType(MatchVirtualMachineVMCIDevice, exp, cur, true)
		deviceChanges = append(deviceChanges, dc...)
	}

	{
		exp, cur := selectDevsByType[*vimtypes.VirtualMachineVideoCard](expectedDevices, currentDevices)
		dc := zipVirtualDevicesOfType(MatchVirtualMachineVideoCard, exp, cur, true)
		deviceChanges = append(deviceChanges, dc...)
	}

	{
		exp, cur := selectDevsByType[*vimtypes.VirtualParallelPort](expectedDevices, currentDevices)
		dc := zipVirtualDevicesOfType(MatchVirtualParallelPort, exp, cur, false)
		deviceChanges = append(deviceChanges, dc...)
	}

	/*
		{
			BMV: Disable until we can determine if fields in the backing can actually be edited.
			exp, cur := selectDevsByType[*vimtypes.VirtualPointingDevice](expectedDevices, currentDevices)
			dc := zipVirtualDevicesOfType(MatchVirtualPointingDevice, exp, cur, true)
			deviceChanges = append(deviceChanges, dc...)
		}
	*/

	{
		exp, cur := selectDevsByType[*vimtypes.VirtualPrecisionClock](expectedDevices, currentDevices)
		dc := zipVirtualDevicesOfType(MatchVirtualPrecisionClock, exp, cur, false)
		deviceChanges = append(deviceChanges, dc...)
	}

	{
		exp, cur := selectDevsByType[*vimtypes.VirtualSCSIPassthrough](expectedDevices, currentDevices)
		dc := zipVirtualDevicesOfType(MatchVirtualSCSIPassthrough, exp, cur, false)
		deviceChanges = append(deviceChanges, dc...)
	}

	{
		exp, cur := selectDevsByType[*vimtypes.VirtualSerialPort](expectedDevices, currentDevices)
		dc := zipVirtualDevicesOfType(MatchVirtualSerialPort, exp, cur, false)
		deviceChanges = append(deviceChanges, dc...)
	}

	{
		exp, cur := selectDevsByType[*vimtypes.VirtualEnsoniq1371](expectedDevices, currentDevices)
		dc := zipVirtualDevicesOfType(MatchVirtualEnsoniq1371, exp, cur, false)
		deviceChanges = append(deviceChanges, dc...)
	}

	{
		exp, cur := selectDevsByType[*vimtypes.VirtualHdAudioCard](expectedDevices, currentDevices)
		dc := zipVirtualDevicesOfType(MatchVirtualHdAudioCard, exp, cur, false)
		deviceChanges = append(deviceChanges, dc...)
	}

	{
		exp, cur := selectDevsByType[*vimtypes.VirtualSoundBlaster16](expectedDevices, currentDevices)
		dc := zipVirtualDevicesOfType(MatchVirtualSoundBlaster16, exp, cur, false)
		deviceChanges = append(deviceChanges, dc...)
	}

	{
		exp, cur := selectDevsByType[*vimtypes.VirtualTPM](expectedDevices, currentDevices)
		dc := zipVirtualDevicesOfType(MatchVirtualTPM, exp, cur, false)
		deviceChanges = append(deviceChanges, dc...)
	}

	{
		exp, cur := selectDevsByType[*vimtypes.VirtualWDT](expectedDevices, currentDevices)
		dc := zipVirtualDevicesOfType(MatchVirtualWDT, exp, cur, false)
		deviceChanges = append(deviceChanges, dc...)
	}

	return deviceChanges
}

func selectDevsByType[T vimtypes.BaseVirtualDevice](
	expectedDevices, currentDevices []vimtypes.BaseVirtualDevice) ([]T, []T) {

	return pkgutil.SelectDevicesByType[T](expectedDevices), pkgutil.SelectDevicesByType[T](currentDevices)
}

func zipVirtualDevicesOfType[T vimtypes.BaseVirtualDevice](
	matchDevFn func(expectedDev, curDev T) vimtypes.BaseVirtualDevice,
	expectedDevs, curDevs []T, defaultDevice bool) []vimtypes.BaseVirtualDeviceConfigSpec {

	var deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec //nolint:prealloc

	// For the expected and current devices of a type, zip the devices together to determine
	// what, if any, edits need to be done. Then add and remove what is leftover.
	minLen := min(len(expectedDevs), len(curDevs))
	for idx := range minLen {
		expectedDev, curDev := expectedDevs[idx], curDevs[idx]

		editDev := matchDevFn(expectedDev, curDev)
		if editDev != nil {
			deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
				Operation: vimtypes.VirtualDeviceConfigSpecOperationEdit,
				Device:    editDev,
			})
		}
	}

	// Add new expected devices.
	for idx := range expectedDevs[minLen:] {
		deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
			Device:    expectedDevs[idx],
		})
	}

	// Remove unmatched existing devices but don't remove default devices since
	// they are added automatically during create.
	if !defaultDevice {
		for _, curDev := range curDevs[minLen:] {
			deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
				Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
				Device:    curDev,
			})
		}
	}

	return deviceChanges
}

func MatchVirtualUSBController(
	expectedDev, curDev *vimtypes.VirtualUSBController) vimtypes.BaseVirtualDevice {

	// NOTE: Max of one device. Uses DeviceKey 7000.

	editDev := &vimtypes.VirtualUSBController{}

	// TODO: Handle VirtualController
	cmpPtr(curDev.AutoConnectDevices, expectedDev.AutoConnectDevices, &editDev.AutoConnectDevices) // TODO: Always false?
	cmpPtr(curDev.EhciEnabled, expectedDev.EhciEnabled, &editDev.EhciEnabled)

	if !reflect.DeepEqual(editDev, &vimtypes.VirtualUSBController{}) {
		editDev.Key = curDev.Key
		return editDev
	}

	return nil
}

func MatchVirtualUSBXHCIController(
	expectedDev, curDev *vimtypes.VirtualUSBXHCIController) vimtypes.BaseVirtualDevice {

	// NOTE: Max of one device. Uses DeviceKey 14000.

	editDev := &vimtypes.VirtualUSBXHCIController{}

	// TODO: Handle VirtualController
	cmpPtr(curDev.AutoConnectDevices, expectedDev.AutoConnectDevices, &editDev.AutoConnectDevices) // TODO: Always false?

	if !reflect.DeepEqual(editDev, &vimtypes.VirtualUSBXHCIController{}) {
		editDev.Key = curDev.Key
		return editDev
	}

	return nil
}

func MatchVirtualMachineVMCIDevice(
	expectedDev, curDev *vimtypes.VirtualMachineVMCIDevice) vimtypes.BaseVirtualDevice {

	// NOTE: Max of one device. Uses DeviceKey 12000. Default device.

	edit := false
	editDev := &vimtypes.VirtualMachineVMCIDevice{}

	// Ignore dev.Id
	cmpPtrEdit(curDev.AllowUnrestrictedCommunication, expectedDev.AllowUnrestrictedCommunication, &editDev.AllowUnrestrictedCommunication, &edit)
	cmpPtrEdit(curDev.FilterEnable, expectedDev.FilterEnable, &editDev.FilterEnable, &edit)

	if expectedDev.FilterInfo != nil {
		if curDev.FilterInfo == nil || !slices.Equal(curDev.FilterInfo.Filters, expectedDev.FilterInfo.Filters) {
			editDev.FilterInfo = expectedDev.FilterInfo
			edit = true
		}
	}

	if edit {
		editDev.Key = curDev.Key
		return editDev
	}

	return nil
}

func MatchVirtualMachineVideoCard(
	expectedDev, curDev *vimtypes.VirtualMachineVideoCard) vimtypes.BaseVirtualDevice {

	// NOTE: Max of one device. Uses DeviceKey 500. Default device.

	editDev := &vimtypes.VirtualMachineVideoCard{}

	// TODO: Handle VirtualController
	cmp(curDev.VideoRamSizeInKB, expectedDev.VideoRamSizeInKB, &editDev.VideoRamSizeInKB)
	cmp(curDev.NumDisplays, expectedDev.NumDisplays, &editDev.NumDisplays)
	cmpPtr(curDev.UseAutoDetect, expectedDev.UseAutoDetect, &editDev.UseAutoDetect)
	cmpPtr(curDev.Enable3DSupport, expectedDev.Enable3DSupport, &editDev.Enable3DSupport)
	cmp(curDev.Use3dRenderer, expectedDev.Use3dRenderer, &editDev.Use3dRenderer)
	cmp(curDev.GraphicsMemorySizeInKB, expectedDev.GraphicsMemorySizeInKB, &editDev.GraphicsMemorySizeInKB)

	if !reflect.DeepEqual(editDev, &vimtypes.VirtualMachineVideoCard{}) {
		editDev.Key = curDev.Key
		return editDev
	}

	return nil
}

func MatchVirtualParallelPort(
	expectedDev, curDev *vimtypes.VirtualParallelPort) vimtypes.BaseVirtualDevice {

	// NOTE: Max of four devices. Uses DeviceKeys 10000-10003.

	// No fields: just a VirtualDevice.
	var match bool

	expectedBacking, curBacking := expectedDev.Backing, curDev.Backing
	switch {
	case expectedBacking != nil && curBacking != nil:
		switch eb := expectedBacking.(type) {
		case *vimtypes.VirtualParallelPortDeviceBackingInfo:
			match = MatchVirtualParallelPortDeviceBackingInfo(eb, curBacking)
		case *vimtypes.VirtualParallelPortFileBackingInfo:
			match = MatchVirtualParallelPortFileBackingInfo(eb, curBacking)
		default:
			match = false
		}
	case expectedBacking == nil && curBacking == nil:
		// NOTE: this device is expected to have a backing.
		match = true
	default:
		match = false
	}

	if !match {
		editDev := &vimtypes.VirtualParallelPort{}
		editDev.Key = curDev.Key
		editDev.Backing = expectedBacking
		return editDev
	}

	return nil
}

func MatchVirtualPointingDevice(
	expectedDev, curDev *vimtypes.VirtualPointingDevice) vimtypes.BaseVirtualDevice {

	// NOTE: Max of one device. Uses DeviceKey 700. Default device.

	// No fields: just a VirtualDevice.
	var match bool

	expectedBacking, curBacking := expectedDev.Backing, curDev.Backing
	switch {
	case expectedBacking != nil && curBacking != nil:
		switch eb := expectedBacking.(type) {
		case *vimtypes.VirtualPointingDeviceDeviceBackingInfo:
			match = MatchVirtualPointingDeviceDeviceBackingInfo(eb, curBacking)
		default:
			match = false
		}
	case expectedBacking == nil && curBacking == nil:
		// NOTE: this device is expected to have a backing.
		match = true
	default:
		match = false
	}

	if !match {
		editDev := &vimtypes.VirtualPointingDevice{}
		editDev.Key = curDev.Key
		editDev.Backing = expectedBacking
		return editDev
	}

	return nil
}

func MatchVirtualPrecisionClock(
	expectedDev, curDev *vimtypes.VirtualPrecisionClock) vimtypes.BaseVirtualDevice {

	// NOTE: Max of one device. Uses DeviceKey 19000.

	// No fields: just a VirtualDevice.
	var match bool

	expectedBacking, curBacking := expectedDev.Backing, curDev.Backing
	switch {
	case expectedBacking != nil && curBacking != nil:
		switch eb := expectedBacking.(type) {
		case *vimtypes.VirtualPrecisionClockSystemClockBackingInfo:
			match = MatchVirtualPrecisionClockSystemClockBackingInfo(eb, curBacking)
		default:
			match = false
		}
	case expectedBacking == nil && curBacking == nil:
		// NOTE: this device is expected to have a backing.
		match = true
	default:
		match = false
	}

	if !match {
		editDev := &vimtypes.VirtualPrecisionClock{}
		editDev.Key = curDev.Key
		editDev.Backing = expectedBacking
		return editDev
	}

	return nil
}

func MatchVirtualSCSIPassthrough(
	expectedDev, curDev *vimtypes.VirtualSCSIPassthrough) vimtypes.BaseVirtualDevice {

	// NOTE: Uses DeviceKeys 2000-2063, 131072-132095.

	// No fields: just a VirtualDevice.
	var match bool

	expectedBacking, curBacking := expectedDev.Backing, curDev.Backing
	switch {
	case expectedBacking != nil && curBacking != nil:
		switch eb := expectedBacking.(type) {
		case *vimtypes.VirtualSCSIPassthroughDeviceBackingInfo:
			match = MatchVirtualSCSIPassthroughDeviceBackingInfo(eb, curBacking)
		default:
			match = false
		}
	case expectedBacking == nil && curBacking == nil:
		// NOTE: this device is expected to have a backing.
		match = true
	default:
		match = false
	}

	if !match {
		editDev := &vimtypes.VirtualSCSIPassthrough{}
		editDev.Key = curDev.Key
		editDev.Backing = expectedBacking
		return editDev
	}

	return nil
}

func doBaseVirtualSoundCardMatch(
	expectedDev, curDev vimtypes.BaseVirtualSoundCard) bool {

	// NOTE: Max of one device. Uses DeviceKey 5000.

	// No fields: just a VirtualDevice.
	var match bool

	expectedBacking, curBacking := expectedDev.GetVirtualSoundCard().Backing, curDev.GetVirtualSoundCard().Backing
	switch {
	case expectedBacking != nil && curBacking != nil:
		switch eb := expectedBacking.(type) {
		case *vimtypes.VirtualSoundCardDeviceBackingInfo:
			match = MatchVirtualSoundCardDeviceBackingInfo(eb, curBacking)
		default:
			match = false
		}
	case expectedBacking == nil && curBacking == nil:
		// NOTE: this device is expected to have a backing.
		match = true
	default:
		match = false
	}

	return match
}

func MatchVirtualEnsoniq1371(
	expectedDev, curDev *vimtypes.VirtualEnsoniq1371) vimtypes.BaseVirtualDevice {

	match := doBaseVirtualSoundCardMatch(expectedDev, curDev)
	if !match {
		editDev := &vimtypes.VirtualEnsoniq1371{}
		editDev.Key = curDev.Key
		editDev.Backing = expectedDev.Backing
		return editDev
	}

	return nil
}

func MatchVirtualHdAudioCard(
	expectedDev, curDev *vimtypes.VirtualHdAudioCard) vimtypes.BaseVirtualDevice {

	match := doBaseVirtualSoundCardMatch(expectedDev, curDev)
	if !match {
		editDev := &vimtypes.VirtualHdAudioCard{}
		editDev.Key = curDev.Key
		editDev.Backing = expectedDev.Backing
		return editDev
	}

	return nil
}

func MatchVirtualSoundBlaster16(
	expectedDev, curDev *vimtypes.VirtualSoundBlaster16) vimtypes.BaseVirtualDevice {

	match := doBaseVirtualSoundCardMatch(expectedDev, curDev)
	if !match {
		editDev := &vimtypes.VirtualSoundBlaster16{}
		editDev.Key = curDev.Key
		editDev.Backing = expectedDev.Backing
		return editDev
	}

	return nil
}

func MatchVirtualSerialPort(
	expectedDev, curDev *vimtypes.VirtualSerialPort) vimtypes.BaseVirtualDevice {

	// NOTE: Max of 32 devices. Uses DeviceKeys 9000-9031.

	// No fields: just a VirtualDevice.
	var match bool

	expectedBacking, curBacking := expectedDev.Backing, curDev.Backing
	switch {
	case expectedBacking != nil && curBacking != nil:
		switch eb := expectedBacking.(type) {
		case *vimtypes.VirtualSerialPortDeviceBackingInfo:
			match = MatchVirtualSerialPortDeviceBackingInfo(eb, curBacking)
		case *vimtypes.VirtualSerialPortFileBackingInfo:
			match = MatchVirtualSerialPortFileBackingInfo(eb, curBacking)
		case *vimtypes.VirtualSerialPortPipeBackingInfo:
			match = MatchVirtualSerialPortPipeBackingInfo(eb, curBacking)
		case *vimtypes.VirtualSerialPortThinPrintBackingInfo:
			match = MatchVirtualSerialPortThinPrintBackingInfo(eb, curBacking)
		case *vimtypes.VirtualSerialPortURIBackingInfo:
			match = MatchVirtualSerialPortURIBackingInfo(eb, curBacking)
		default:
			match = false
		}
	case expectedBacking == nil && curBacking == nil:
		// NOTE: this device is expected to have a backing.
		match = true
	default:
		match = false
	}

	if !match {
		editDev := &vimtypes.VirtualSerialPort{}
		editDev.Key = curDev.Key
		editDev.Backing = expectedBacking
		return editDev
	}

	return nil
}

func MatchVirtualTPM(
	expectedDev, curDev *vimtypes.VirtualTPM) vimtypes.BaseVirtualDevice {

	// NOTE: Max of one device. Uses DeviceKey 11000.

	edit := false
	editDev := &vimtypes.VirtualTPM{}

	// TODO: Handle VirtualDevice.

	byteSliceEq := func(x, y []byte) bool { return slices.Equal(x, y) } //nolint:gocritic

	if !slices.EqualFunc(expectedDev.EndorsementKeyCertificateSigningRequest,
		curDev.EndorsementKeyCertificateSigningRequest, byteSliceEq) {
		editDev.EndorsementKeyCertificateSigningRequest = expectedDev.EndorsementKeyCertificateSigningRequest
		edit = true
	}

	if !slices.EqualFunc(expectedDev.EndorsementKeyCertificate, curDev.EndorsementKeyCertificate, byteSliceEq) {
		editDev.EndorsementKeyCertificate = expectedDev.EndorsementKeyCertificate
		edit = true
	}

	if edit {
		editDev.Key = curDev.Key
		return editDev
	}

	return nil
}

func MatchVirtualWDT(
	expectedDev, curDev *vimtypes.VirtualWDT) vimtypes.BaseVirtualDevice {

	// NOTE: Max of one device. Uses DeviceKey 18000.

	edit := false
	editDev := &vimtypes.VirtualWDT{}

	cmpEdit(curDev.RunOnBoot, expectedDev.RunOnBoot, &editDev.RunOnBoot, &edit)
	// Ignore dev.Running field. Or set editDev.Running = curDev.Running? Funky API.

	if edit {
		editDev.Key = curDev.Key
		return editDev
	}

	return nil
}

func cmpEdit[T comparable](a, b T, c *T, edit *bool) {
	if a != b {
		*c = b
		*edit = true
	}
}

func cmpPtrEdit[T comparable](a *T, b *T, c **T, edit *bool) {
	if a == nil && b == nil {
		return
	}

	if a == nil {
		*c = b
		*edit = true
		return
	}

	if b == nil {
		// When the desired value is nil we leave the existing value as-is.
		return
	}

	if *a != *b {
		*c = b
		*edit = true
	}
}

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
