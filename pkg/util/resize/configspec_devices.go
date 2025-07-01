// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package resize

import (
	"reflect"
	"slices"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

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

	pciDeviceChanges := ComparePCIDevices(csDeviceList, deviceList)
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

func deviceEdited[T any](dev *T) bool {
	var e T
	return !reflect.DeepEqual(dev, &e)
}

func MatchVirtualDevice(expectedDev, curDev, editDev vimtypes.BaseVirtualDevice) {

	// TODO - Note that for now we'll just handle the backings in the per device matching func.
	_ = expectedDev
	_ = curDev
	_ = editDev
}

func MatchVirtualController(expectedDev, curDev, editDev vimtypes.BaseVirtualController) {

	// TODO
	MatchVirtualDevice(
		expectedDev.GetVirtualController().GetVirtualDevice(),
		curDev.GetVirtualController().GetVirtualDevice(),
		editDev.GetVirtualController().GetVirtualDevice())
}

func MatchVirtualUSBController(
	expectedDev, curDev *vimtypes.VirtualUSBController) vimtypes.BaseVirtualDevice {

	// NOTE: Max of one device. Uses DeviceKey 7000.
	editDev := &vimtypes.VirtualUSBController{}

	MatchVirtualController(expectedDev, curDev, editDev)

	cmpPtr(curDev.AutoConnectDevices, expectedDev.AutoConnectDevices, &editDev.AutoConnectDevices) // TODO: Always false?
	cmpPtr(curDev.EhciEnabled, expectedDev.EhciEnabled, &editDev.EhciEnabled)

	if deviceEdited(editDev) {
		editDev.Key = curDev.Key
		return editDev
	}

	return nil
}

func MatchVirtualUSBXHCIController(
	expectedDev, curDev *vimtypes.VirtualUSBXHCIController) vimtypes.BaseVirtualDevice {

	// NOTE: Max of one device. Uses DeviceKey 14000.
	editDev := &vimtypes.VirtualUSBXHCIController{}

	MatchVirtualController(expectedDev, curDev, editDev)

	cmpPtr(curDev.AutoConnectDevices, expectedDev.AutoConnectDevices, &editDev.AutoConnectDevices) // TODO: Always false?

	if deviceEdited(editDev) {
		editDev.Key = curDev.Key
		return editDev
	}

	return nil
}

func MatchVirtualMachineVMCIDevice(
	expectedDev, curDev *vimtypes.VirtualMachineVMCIDevice) vimtypes.BaseVirtualDevice {

	// NOTE: Max of one device. Uses DeviceKey 12000. Default device.
	editDev := &vimtypes.VirtualMachineVMCIDevice{}

	MatchVirtualDevice(expectedDev, curDev, editDev)

	// Ignore dev.Id
	cmpPtr(curDev.AllowUnrestrictedCommunication, expectedDev.AllowUnrestrictedCommunication, &editDev.AllowUnrestrictedCommunication)
	cmpPtr(curDev.FilterEnable, expectedDev.FilterEnable, &editDev.FilterEnable)
	if expectedDev.FilterInfo != nil {
		if curDev.FilterInfo == nil || !slices.Equal(curDev.FilterInfo.Filters, expectedDev.FilterInfo.Filters) {
			editDev.FilterInfo = expectedDev.FilterInfo
		}
	}

	if deviceEdited(editDev) {
		editDev.Key = curDev.Key
		return editDev
	}

	return nil
}

func MatchVirtualMachineVideoCard(
	expectedDev, curDev *vimtypes.VirtualMachineVideoCard) vimtypes.BaseVirtualDevice {

	// NOTE: Max of one device. Uses DeviceKey 500. Default device.
	editDev := &vimtypes.VirtualMachineVideoCard{}

	MatchVirtualDevice(expectedDev, curDev, editDev)

	cmp(curDev.VideoRamSizeInKB, expectedDev.VideoRamSizeInKB, &editDev.VideoRamSizeInKB)
	cmp(curDev.NumDisplays, expectedDev.NumDisplays, &editDev.NumDisplays)
	cmpPtr(curDev.UseAutoDetect, expectedDev.UseAutoDetect, &editDev.UseAutoDetect)
	cmpPtr(curDev.Enable3DSupport, expectedDev.Enable3DSupport, &editDev.Enable3DSupport)
	cmp(curDev.Use3dRenderer, expectedDev.Use3dRenderer, &editDev.Use3dRenderer)
	cmp(curDev.GraphicsMemorySizeInKB, expectedDev.GraphicsMemorySizeInKB, &editDev.GraphicsMemorySizeInKB)

	if deviceEdited(editDev) {
		editDev.Key = curDev.Key
		return editDev
	}

	return nil
}

func MatchVirtualParallelPort(
	expectedDev, curDev *vimtypes.VirtualParallelPort) vimtypes.BaseVirtualDevice {

	// NOTE: Max of four devices. Uses DeviceKeys 10000-10003.
	editDev := &vimtypes.VirtualParallelPort{}

	MatchVirtualDevice(expectedDev, curDev, editDev)
	// No fields: just a VirtualDevice.

	cmpBacking(expectedDev, curDev, editDev,
		func(exp, cur vimtypes.BaseVirtualDeviceBackingInfo) bool {
			switch eb := exp.(type) {
			case *vimtypes.VirtualParallelPortDeviceBackingInfo:
				return MatchVirtualParallelPortDeviceBackingInfo(eb, cur)
			case *vimtypes.VirtualParallelPortFileBackingInfo:
				return MatchVirtualParallelPortFileBackingInfo(eb, cur)
			default:
				return false
			}
		})

	if deviceEdited(editDev) {
		editDev.Key = curDev.Key
		return editDev
	}

	return nil
}

func MatchVirtualPointingDevice(
	expectedDev, curDev *vimtypes.VirtualPointingDevice) vimtypes.BaseVirtualDevice {

	// NOTE: Max of one device. Uses DeviceKey 700. Default device.
	editDev := &vimtypes.VirtualPointingDevice{}

	MatchVirtualDevice(expectedDev, curDev, editDev)
	// No fields: just a VirtualDevice.

	cmpBacking(expectedDev, curDev, editDev,
		func(exp, cur vimtypes.BaseVirtualDeviceBackingInfo) bool {
			switch eb := exp.(type) {
			case *vimtypes.VirtualPointingDeviceDeviceBackingInfo:
				return MatchVirtualPointingDeviceDeviceBackingInfo(eb, cur)
			default:
				return false
			}
		})

	if deviceEdited(editDev) {
		editDev.Key = curDev.Key
		return editDev
	}

	return nil
}

func MatchVirtualPrecisionClock(
	expectedDev, curDev *vimtypes.VirtualPrecisionClock) vimtypes.BaseVirtualDevice {

	// NOTE: Max of one device. Uses DeviceKey 19000.
	editDev := &vimtypes.VirtualPrecisionClock{}

	MatchVirtualDevice(expectedDev, curDev, editDev)
	// No fields: just a VirtualDevice.

	cmpBacking(expectedDev, curDev, editDev,
		func(exp, cur vimtypes.BaseVirtualDeviceBackingInfo) bool {
			switch eb := exp.(type) {
			case *vimtypes.VirtualPrecisionClockSystemClockBackingInfo:
				return MatchVirtualPrecisionClockSystemClockBackingInfo(eb, cur)
			default:
				return false
			}
		})

	if deviceEdited(editDev) {
		editDev.Key = curDev.Key
		return editDev
	}

	return nil
}

func MatchVirtualSCSIPassthrough(
	expectedDev, curDev *vimtypes.VirtualSCSIPassthrough) vimtypes.BaseVirtualDevice {

	// NOTE: Uses DeviceKeys 2000-2063, 131072-132095.
	editDev := &vimtypes.VirtualSCSIPassthrough{}

	MatchVirtualDevice(expectedDev, curDev, editDev)
	// No fields: just a VirtualDevice.

	cmpBacking(expectedDev, curDev, editDev,
		func(exp, cur vimtypes.BaseVirtualDeviceBackingInfo) bool {
			switch eb := exp.(type) {
			case *vimtypes.VirtualSCSIPassthroughDeviceBackingInfo:
				return MatchVirtualSCSIPassthroughDeviceBackingInfo(eb, cur)
			default:
				return false
			}
		})

	if deviceEdited(editDev) {
		editDev.Key = curDev.Key
		return editDev
	}

	return nil
}

func matchBaseVirtualSoundCard(
	expectedDev, curDev, editDev vimtypes.BaseVirtualSoundCard) {

	// NOTE: Max of one device. Uses DeviceKey 5000.

	MatchVirtualDevice(
		expectedDev.GetVirtualSoundCard().GetVirtualDevice(),
		curDev.GetVirtualSoundCard().GetVirtualDevice(),
		editDev.GetVirtualSoundCard().GetVirtualDevice())
	// No fields: just a VirtualDevice.

	cmpBacking(
		expectedDev.GetVirtualSoundCard().GetVirtualDevice(),
		curDev.GetVirtualSoundCard().GetVirtualDevice(),
		editDev.GetVirtualSoundCard().GetVirtualDevice(),
		func(exp, cur vimtypes.BaseVirtualDeviceBackingInfo) bool {
			switch eb := exp.(type) {
			case *vimtypes.VirtualSoundCardDeviceBackingInfo:
				return MatchVirtualSoundCardDeviceBackingInfo(eb, cur)
			default:
				return false
			}
		})
}

func MatchVirtualEnsoniq1371(
	expectedDev, curDev *vimtypes.VirtualEnsoniq1371) vimtypes.BaseVirtualDevice {

	editDev := &vimtypes.VirtualEnsoniq1371{}
	matchBaseVirtualSoundCard(expectedDev, curDev, editDev)

	if deviceEdited(editDev) {
		editDev.Key = curDev.Key
		return editDev
	}

	return nil
}

func MatchVirtualHdAudioCard(
	expectedDev, curDev *vimtypes.VirtualHdAudioCard) vimtypes.BaseVirtualDevice {

	editDev := &vimtypes.VirtualHdAudioCard{}
	matchBaseVirtualSoundCard(expectedDev, curDev, editDev)

	if deviceEdited(editDev) {
		editDev.Key = curDev.Key
		return editDev
	}

	return nil
}

func MatchVirtualSoundBlaster16(
	expectedDev, curDev *vimtypes.VirtualSoundBlaster16) vimtypes.BaseVirtualDevice {

	editDev := &vimtypes.VirtualSoundBlaster16{}
	matchBaseVirtualSoundCard(expectedDev, curDev, editDev)

	if deviceEdited(editDev) {
		editDev.Key = curDev.Key
		return editDev
	}

	return nil
}

func MatchVirtualSerialPort(
	expectedDev, curDev *vimtypes.VirtualSerialPort) vimtypes.BaseVirtualDevice {

	// NOTE: Max of 32 devices. Uses DeviceKeys 9000-9031.
	editDev := &vimtypes.VirtualSerialPort{}

	MatchVirtualDevice(expectedDev, curDev, editDev)
	// No fields: just a VirtualDevice.

	cmpBacking(expectedDev, curDev, editDev,
		func(exp, cur vimtypes.BaseVirtualDeviceBackingInfo) bool {
			switch eb := exp.(type) {
			case *vimtypes.VirtualSerialPortDeviceBackingInfo:
				return MatchVirtualSerialPortDeviceBackingInfo(eb, cur)
			case *vimtypes.VirtualSerialPortFileBackingInfo:
				return MatchVirtualSerialPortFileBackingInfo(eb, cur)
			case *vimtypes.VirtualSerialPortPipeBackingInfo:
				return MatchVirtualSerialPortPipeBackingInfo(eb, cur)
			case *vimtypes.VirtualSerialPortThinPrintBackingInfo:
				return MatchVirtualSerialPortThinPrintBackingInfo(eb, cur)
			case *vimtypes.VirtualSerialPortURIBackingInfo:
				return MatchVirtualSerialPortURIBackingInfo(eb, cur)
			default:
				return false
			}
		})

	if deviceEdited(editDev) {
		editDev.Key = curDev.Key
		return editDev
	}

	return nil
}

func MatchVirtualTPM(
	expectedDev, curDev *vimtypes.VirtualTPM) vimtypes.BaseVirtualDevice {

	// NOTE: Max of one device. Uses DeviceKey 11000.
	editDev := &vimtypes.VirtualTPM{}

	MatchVirtualDevice(expectedDev, curDev, editDev)

	byteSliceEq := func(x, y []byte) bool { return slices.Equal(x, y) } //nolint:gocritic

	if !slices.EqualFunc(expectedDev.EndorsementKeyCertificateSigningRequest,
		curDev.EndorsementKeyCertificateSigningRequest,
		byteSliceEq) {
		editDev.EndorsementKeyCertificateSigningRequest = expectedDev.EndorsementKeyCertificateSigningRequest
	}
	if !slices.EqualFunc(expectedDev.EndorsementKeyCertificate,
		curDev.EndorsementKeyCertificate,
		byteSliceEq) {
		editDev.EndorsementKeyCertificate = expectedDev.EndorsementKeyCertificate
	}

	if deviceEdited(editDev) {
		editDev.Key = curDev.Key
		return editDev
	}

	return nil
}

func MatchVirtualWDT(
	expectedDev, curDev *vimtypes.VirtualWDT) vimtypes.BaseVirtualDevice {

	// NOTE: Max of one device. Uses DeviceKey 18000.
	editDev := &vimtypes.VirtualWDT{}

	MatchVirtualDevice(expectedDev, curDev, editDev)

	cmp(curDev.RunOnBoot, expectedDev.RunOnBoot, &editDev.RunOnBoot)
	// Ignore dev.Running field. Or set editDev.Running = curDev.Running? Funky API.

	if deviceEdited(editDev) {
		editDev.Key = curDev.Key
		return editDev
	}

	return nil
}

func cmpBacking(
	expectedDev, curDev, editDev vimtypes.BaseVirtualDevice,
	cmpFn func(e, b vimtypes.BaseVirtualDeviceBackingInfo) bool) {

	expectedBacking, curBacking := expectedDev.GetVirtualDevice().Backing, curDev.GetVirtualDevice().Backing

	var backingMatch bool
	if expectedBacking != nil && curBacking != nil {
		backingMatch = cmpFn(expectedBacking, curBacking)
	} else {
		// If an expected backing isn't specified, we just leave the current
		// backing - if any - alone.
		backingMatch = expectedBacking == nil
	}

	if !backingMatch {
		editDev.GetVirtualDevice().Backing = expectedBacking
	}
}

func CmpEdit[T comparable](a, b T, c *T, edit *bool) {
	if a != b {
		*c = b
		*edit = true
	}
}

func CmpPtrEdit[T comparable](a *T, b *T, c **T, edit *bool) {
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
