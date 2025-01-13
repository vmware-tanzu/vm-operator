// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"reflect"

	vimtypes "github.com/vmware/govmomi/vim25/types"
)

// SelectDeviceFn returns true if the provided virtual device is a match.
type SelectDeviceFn[T vimtypes.BaseVirtualDevice] func(dev vimtypes.BaseVirtualDevice) bool

// SelectDevices returns a slice of the devices that match at least one of the
// provided selector functions.
func SelectDevices[T vimtypes.BaseVirtualDevice](
	devices []vimtypes.BaseVirtualDevice,
	selectorFns ...SelectDeviceFn[T],
) []T {

	var selectedDevices []T
	for i := range devices {
		if t, ok := devices[i].(T); ok {
			for j := range selectorFns {
				if selectorFns[j](t) {
					selectedDevices = append(selectedDevices, t)
					break
				}
			}

		}
	}
	return selectedDevices
}

// SelectDevicesByType returns a slice of the devices that are of type T.
func SelectDevicesByType[T vimtypes.BaseVirtualDevice](
	devices []vimtypes.BaseVirtualDevice,
) []T {

	var selectedDevices []T
	for i := range devices {
		if t, ok := devices[i].(T); ok {
			selectedDevices = append(selectedDevices, t)
		}
	}
	return selectedDevices
}

// SelectDevicesByBackingType returns a slice of the devices that have a
// backing of type B.
func SelectDevicesByBackingType[B vimtypes.BaseVirtualDeviceBackingInfo](
	devices []vimtypes.BaseVirtualDevice,
) []vimtypes.BaseVirtualDevice {

	var selectedDevices []vimtypes.BaseVirtualDevice
	for i := range devices {
		if baseDev := devices[i]; baseDev != nil {
			if dev := baseDev.GetVirtualDevice(); dev != nil {
				if _, ok := dev.Backing.(B); ok {
					selectedDevices = append(selectedDevices, baseDev)
				}
			}
		}
	}
	return selectedDevices
}

// SelectDevicesByDeviceAndBackingType returns a slice of the devices that are
// of type T with a backing of type B.
func SelectDevicesByDeviceAndBackingType[
	T vimtypes.BaseVirtualDevice,
	B vimtypes.BaseVirtualDeviceBackingInfo,
](
	devices []vimtypes.BaseVirtualDevice,
) []T {

	var selectedDevices []T
	for i := range devices {
		if t, ok := devices[i].(T); ok {
			if dev := t.GetVirtualDevice(); dev != nil {
				if _, ok := dev.Backing.(B); ok {
					selectedDevices = append(selectedDevices, t)
				}
			}
		}
	}
	return selectedDevices
}

// SelectDevicesByTypes returns a slice of the devices that match at least one
// the provided device types.
func SelectDevicesByTypes(
	devices []vimtypes.BaseVirtualDevice,
	deviceTypes ...vimtypes.BaseVirtualDevice,
) []vimtypes.BaseVirtualDevice {

	validDeviceTypes := map[reflect.Type]struct{}{}
	for i := range deviceTypes {
		validDeviceTypes[reflect.TypeOf(deviceTypes[i])] = struct{}{}
	}
	selectorFn := func(dev vimtypes.BaseVirtualDevice) bool {
		_, ok := validDeviceTypes[reflect.TypeOf(dev)]
		return ok
	}
	return SelectDevices[vimtypes.BaseVirtualDevice](devices, selectorFn)
}

// SelectVirtualPCIPassthrough returns a slice of *VirtualPCIPassthrough
// devices.
func SelectVirtualPCIPassthrough(
	devices []vimtypes.BaseVirtualDevice,
) []*vimtypes.VirtualPCIPassthrough {

	return SelectDevicesByType[*vimtypes.VirtualPCIPassthrough](devices)
}

// IsDeviceNvidiaVgpu returns true if the provided device is an Nvidia vGPU.
func IsDeviceNvidiaVgpu(dev vimtypes.BaseVirtualDevice) bool {
	if dev, ok := dev.(*vimtypes.VirtualPCIPassthrough); ok {
		_, ok := dev.Backing.(*vimtypes.VirtualPCIPassthroughVmiopBackingInfo)
		return ok
	}
	return false
}

// IsDeviceDynamicDirectPathIO returns true if the provided device is a
// dynamic direct path I/O device.
func IsDeviceDynamicDirectPathIO(dev vimtypes.BaseVirtualDevice) bool {
	if dev, ok := dev.(*vimtypes.VirtualPCIPassthrough); ok {
		_, ok := dev.Backing.(*vimtypes.VirtualPCIPassthroughDynamicBackingInfo)
		return ok
	}
	return false
}

// HasDeviceChangeDeviceByType returns true of one of the device change's dev is that of type T.
func HasDeviceChangeDeviceByType[T vimtypes.BaseVirtualDevice](
	deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec,
) bool {
	for i := range deviceChanges {
		if spec := deviceChanges[i].GetVirtualDeviceConfigSpec(); spec != nil {
			if dev := spec.Device; dev != nil {
				if _, ok := dev.(T); ok {
					return true
				}
			}
		}
	}
	return false
}

// HasVirtualPCIPassthroughDeviceChange returns true if any of the device changes are for a passthrough device.
func HasVirtualPCIPassthroughDeviceChange(
	devices []vimtypes.BaseVirtualDeviceConfigSpec,
) bool {
	return HasDeviceChangeDeviceByType[*vimtypes.VirtualPCIPassthrough](devices)
}

// SelectNvidiaVgpu return a slice of Nvidia vGPU devices.
func SelectNvidiaVgpu(
	devices []vimtypes.BaseVirtualDevice,
) []*vimtypes.VirtualPCIPassthrough {
	return selectVirtualPCIPassthroughWithVmiopBacking(devices)
}

// selectVirtualPCIPassthroughWithVmiopBacking returns a slice of PCI devices with VmiopBacking.
func selectVirtualPCIPassthroughWithVmiopBacking(
	devices []vimtypes.BaseVirtualDevice,
) []*vimtypes.VirtualPCIPassthrough {

	return SelectDevicesByDeviceAndBackingType[
		*vimtypes.VirtualPCIPassthrough,
		*vimtypes.VirtualPCIPassthroughVmiopBackingInfo,
	](devices)
}

// SelectDynamicDirectPathIO returns a slice of dynamic direct path I/O devices.
func SelectDynamicDirectPathIO(
	devices []vimtypes.BaseVirtualDevice,
) []*vimtypes.VirtualPCIPassthrough {

	return SelectDevicesByDeviceAndBackingType[
		*vimtypes.VirtualPCIPassthrough,
		*vimtypes.VirtualPCIPassthroughDynamicBackingInfo,
	](devices)
}

func IsEthernetCard(dev vimtypes.BaseVirtualDevice) bool {
	_, ok := dev.(vimtypes.BaseVirtualEthernetCard)
	return ok
}

// isNonRDMDisk returns true for all virtual disk devices excluding disks with a raw device mapping backing.
func isNonRDMDisk(dev vimtypes.BaseVirtualDevice) bool {
	if dev, ok := dev.(*vimtypes.VirtualDisk); ok {
		_, hasRDMBacking := dev.Backing.(*vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo)
		return !hasRDMBacking
	}

	return false
}

// GetPreferredDiskFormat gets the preferred disk format. This function returns
// 4k if available, native 512 if available, an empty value if no formats are
// provided, else the first available format is returned.
func GetPreferredDiskFormat[T string | vimtypes.DatastoreSectorFormat](
	diskFormats ...T) vimtypes.DatastoreSectorFormat {

	if len(diskFormats) == 0 {
		return ""
	}

	var (
		supports4k        bool
		supportsNative512 bool
	)

	for i := range diskFormats {
		switch diskFormats[i] {
		case T(vimtypes.DatastoreSectorFormatNative_4k):
			supports4k = true
		case T(vimtypes.DatastoreSectorFormatNative_512):
			supportsNative512 = true
		}
	}

	if supports4k {
		return vimtypes.DatastoreSectorFormatNative_4k
	}
	if supportsNative512 {
		return vimtypes.DatastoreSectorFormatNative_512
	}

	return vimtypes.DatastoreSectorFormat(diskFormats[0])
}
