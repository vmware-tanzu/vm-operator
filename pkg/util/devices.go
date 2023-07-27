// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"reflect"

	vimTypes "github.com/vmware/govmomi/vim25/types"
)

// SelectDeviceFn returns true if the provided virtual device is a match.
type SelectDeviceFn[T vimTypes.BaseVirtualDevice] func(dev vimTypes.BaseVirtualDevice) bool

// SelectDevices returns a slice of the devices that match at least one of the
// provided selector functions.
func SelectDevices[T vimTypes.BaseVirtualDevice](
	devices []vimTypes.BaseVirtualDevice,
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
func SelectDevicesByType[T vimTypes.BaseVirtualDevice](
	devices []vimTypes.BaseVirtualDevice,
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
func SelectDevicesByBackingType[B vimTypes.BaseVirtualDeviceBackingInfo](
	devices []vimTypes.BaseVirtualDevice,
) []vimTypes.BaseVirtualDevice {

	var selectedDevices []vimTypes.BaseVirtualDevice
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
	T vimTypes.BaseVirtualDevice,
	B vimTypes.BaseVirtualDeviceBackingInfo,
](
	devices []vimTypes.BaseVirtualDevice,
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
	devices []vimTypes.BaseVirtualDevice,
	deviceTypes ...vimTypes.BaseVirtualDevice,
) []vimTypes.BaseVirtualDevice {

	validDeviceTypes := map[reflect.Type]struct{}{}
	for i := range deviceTypes {
		validDeviceTypes[reflect.TypeOf(deviceTypes[i])] = struct{}{}
	}
	selectorFn := func(dev vimTypes.BaseVirtualDevice) bool {
		_, ok := validDeviceTypes[reflect.TypeOf(dev)]
		return ok
	}
	return SelectDevices[vimTypes.BaseVirtualDevice](devices, selectorFn)
}

// SelectVirtualPCIPassthrough returns a slice of *VirtualPCIPassthrough
// devices.
func SelectVirtualPCIPassthrough(
	devices []vimTypes.BaseVirtualDevice,
) []*vimTypes.VirtualPCIPassthrough {

	return SelectDevicesByType[*vimTypes.VirtualPCIPassthrough](devices)
}

// IsDeviceNvidiaVgpu returns true if the provided device is an Nvidia vGPU.
func IsDeviceNvidiaVgpu(dev vimTypes.BaseVirtualDevice) bool {
	if dev, ok := dev.(*vimTypes.VirtualPCIPassthrough); ok {
		_, ok := dev.Backing.(*vimTypes.VirtualPCIPassthroughVmiopBackingInfo)
		return ok
	}
	return false
}

// IsDeviceDynamicDirectPathIO returns true if the provided device is a
// dynamic direct path I/O device..
func IsDeviceDynamicDirectPathIO(dev vimTypes.BaseVirtualDevice) bool {
	if dev, ok := dev.(*vimTypes.VirtualPCIPassthrough); ok {
		_, ok := dev.Backing.(*vimTypes.VirtualPCIPassthroughDynamicBackingInfo)
		return ok
	}
	return false
}

// SelectNvidiaVgpu return a slice of Nvidia vGPU devices.
func SelectNvidiaVgpu(
	devices []vimTypes.BaseVirtualDevice,
) []*vimTypes.VirtualPCIPassthrough {
	return selectVirtualPCIPassthroughWithVmiopBacking(devices)
}

// SelectVirtualPCIPassthroughWithVmiopBacking returns a slice of PCI devices with VmiopBacking.
func selectVirtualPCIPassthroughWithVmiopBacking(
	devices []vimTypes.BaseVirtualDevice,
) []*vimTypes.VirtualPCIPassthrough {

	return SelectDevicesByDeviceAndBackingType[
		*vimTypes.VirtualPCIPassthrough,
		*vimTypes.VirtualPCIPassthroughVmiopBackingInfo,
	](devices)
}

// SelectDynamicDirectPathIO returns a slice of dynamic direct path I/O devices.
func SelectDynamicDirectPathIO(
	devices []vimTypes.BaseVirtualDevice,
) []*vimTypes.VirtualPCIPassthrough {

	return SelectDevicesByDeviceAndBackingType[
		*vimTypes.VirtualPCIPassthrough,
		*vimTypes.VirtualPCIPassthroughDynamicBackingInfo,
	](devices)
}

func IsEthernetCard(dev vimTypes.BaseVirtualDevice) bool {
	switch dev.(type) {
	case *vimTypes.VirtualE1000, *vimTypes.VirtualE1000e, *vimTypes.VirtualPCNet32, *vimTypes.VirtualVmxnet2, *vimTypes.VirtualVmxnet3, *vimTypes.VirtualVmxnet3Vrdma, *vimTypes.VirtualSriovEthernetCard:
		return true
	default:
		return false
	}
}

func isDiskOrDiskController(dev vimTypes.BaseVirtualDevice) bool {
	switch dev.(type) {
	case *vimTypes.VirtualDisk, *vimTypes.VirtualIDEController, *vimTypes.VirtualNVMEController, *vimTypes.VirtualSATAController, *vimTypes.VirtualSCSIController:
		return true
	default:
		return false
	}
}

// isNonRDMDisk returns true for all virtual disk devices excluding disks with a raw device mapping backing.
func isNonRDMDisk(dev vimTypes.BaseVirtualDevice) bool {
	if dev, ok := dev.(*vimTypes.VirtualDisk); ok {
		_, hasRDMBacking := dev.Backing.(*vimTypes.VirtualDiskRawDiskMappingVer1BackingInfo)
		return !hasRDMBacking
	}

	return false
}
