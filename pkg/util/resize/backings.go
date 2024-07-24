// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package resize

import (
	vimtypes "github.com/vmware/govmomi/vim25/types"
)

func MatchVirtualDeviceDeviceBackingInfo(
	expectedBacking, curBacking *vimtypes.VirtualDeviceDeviceBackingInfo) bool {

	if expectedBacking.DeviceName != curBacking.DeviceName {
		return false
	}

	if expectedBacking.UseAutoDetect == nil {
		// An unset UseAutoDetect will default to a set FALSE.
		return curBacking.UseAutoDetect == nil || !*curBacking.UseAutoDetect
	}

	return curBacking.UseAutoDetect != nil && *expectedBacking.UseAutoDetect == *curBacking.UseAutoDetect
}

func MatchVirtualDeviceFileBackingInfo(
	expectedBacking, curBacking *vimtypes.VirtualDeviceFileBackingInfo) bool {

	// Ignore Datastore and BackingObjectID
	return expectedBacking.FileName == curBacking.FileName
}

func MatchVirtualDevicePipeBackingInfo(
	expectedBacking, curBacking *vimtypes.VirtualDevicePipeBackingInfo) bool {

	return expectedBacking.PipeName == curBacking.PipeName
}

func MatchVirtualDeviceURIBackingInfo(
	expectedBacking, curBacking *vimtypes.VirtualDeviceURIBackingInfo) bool {

	return expectedBacking.ServiceURI == curBacking.ServiceURI &&
		expectedBacking.Direction == curBacking.Direction &&
		expectedBacking.ProxyURI == curBacking.ProxyURI
}

func MatchVirtualParallelPortDeviceBackingInfo(
	expectedBacking *vimtypes.VirtualParallelPortDeviceBackingInfo,
	baseBacking vimtypes.BaseVirtualDeviceBackingInfo) bool {

	curBacking, ok := baseBacking.(*vimtypes.VirtualParallelPortDeviceBackingInfo)
	if !ok {
		return false
	}

	// No fields.

	return MatchVirtualDeviceDeviceBackingInfo(
		expectedBacking.GetVirtualDeviceDeviceBackingInfo(),
		curBacking.GetVirtualDeviceDeviceBackingInfo())
}

func MatchVirtualParallelPortFileBackingInfo(
	expectedBacking *vimtypes.VirtualParallelPortFileBackingInfo,
	baseBacking vimtypes.BaseVirtualDeviceBackingInfo) bool {

	curBacking, ok := baseBacking.(*vimtypes.VirtualParallelPortFileBackingInfo)
	if !ok {
		return false
	}

	// No fields.

	return MatchVirtualDeviceFileBackingInfo(
		expectedBacking.GetVirtualDeviceFileBackingInfo(),
		curBacking.GetVirtualDeviceFileBackingInfo())
}

func MatchVirtualPointingDeviceDeviceBackingInfo(
	expectedBacking *vimtypes.VirtualPointingDeviceDeviceBackingInfo,
	baseBacking vimtypes.BaseVirtualDeviceBackingInfo) bool {

	curBacking, ok := baseBacking.(*vimtypes.VirtualPointingDeviceDeviceBackingInfo)
	if !ok {
		return false
	}

	if curBacking.HostPointingDevice != expectedBacking.HostPointingDevice {
		// TODO: Have the VirtualPointingDeviceOption to determine the default.
		if expectedBacking.HostPointingDevice != "" || curBacking.HostPointingDevice != "autodetect" {
			return false
		}
	}

	return MatchVirtualDeviceDeviceBackingInfo(
		expectedBacking.GetVirtualDeviceDeviceBackingInfo(),
		curBacking.GetVirtualDeviceDeviceBackingInfo())
}

func MatchVirtualPrecisionClockSystemClockBackingInfo(
	expectedBacking *vimtypes.VirtualPrecisionClockSystemClockBackingInfo,
	baseBacking vimtypes.BaseVirtualDeviceBackingInfo) bool {

	curBacking, ok := baseBacking.(*vimtypes.VirtualPrecisionClockSystemClockBackingInfo)
	if !ok {
		return false
	}

	return expectedBacking.Protocol == curBacking.Protocol
}

func MatchVirtualSoundCardDeviceBackingInfo(
	expectedBacking *vimtypes.VirtualSoundCardDeviceBackingInfo,
	baseBacking vimtypes.BaseVirtualDeviceBackingInfo) bool {

	curBacking, ok := baseBacking.(*vimtypes.VirtualSoundCardDeviceBackingInfo)
	if !ok {
		return false
	}

	// No fields.

	return MatchVirtualDeviceDeviceBackingInfo(
		expectedBacking.GetVirtualDeviceDeviceBackingInfo(),
		curBacking.GetVirtualDeviceDeviceBackingInfo())
}

func MatchVirtualSerialPortDeviceBackingInfo(
	expectedBacking *vimtypes.VirtualSerialPortDeviceBackingInfo,
	baseBacking vimtypes.BaseVirtualDeviceBackingInfo) bool {

	curBacking, ok := baseBacking.(*vimtypes.VirtualSerialPortDeviceBackingInfo)
	if !ok {
		return false
	}

	// No fields.

	return MatchVirtualDeviceDeviceBackingInfo(
		expectedBacking.GetVirtualDeviceDeviceBackingInfo(),
		curBacking.GetVirtualDeviceDeviceBackingInfo())
}

func MatchVirtualSerialPortFileBackingInfo(
	expectedBacking *vimtypes.VirtualSerialPortFileBackingInfo,
	baseBacking vimtypes.BaseVirtualDeviceBackingInfo) bool {

	curBacking, ok := baseBacking.(*vimtypes.VirtualSerialPortFileBackingInfo)
	if !ok {
		return false
	}

	// No fields.

	return MatchVirtualDeviceFileBackingInfo(
		expectedBacking.GetVirtualDeviceFileBackingInfo(),
		curBacking.GetVirtualDeviceFileBackingInfo())
}

func MatchVirtualSerialPortPipeBackingInfo(
	expectedBacking *vimtypes.VirtualSerialPortPipeBackingInfo,
	baseBacking vimtypes.BaseVirtualDeviceBackingInfo) bool {

	curBacking, ok := baseBacking.(*vimtypes.VirtualSerialPortPipeBackingInfo)
	if !ok {
		return false
	}

	if expectedBacking.Endpoint != curBacking.Endpoint {
		return false
	}
	if !ptrEqual(expectedBacking.NoRxLoss, curBacking.NoRxLoss) {
		return false
	}

	return MatchVirtualDevicePipeBackingInfo(
		expectedBacking.GetVirtualDevicePipeBackingInfo(),
		curBacking.GetVirtualDevicePipeBackingInfo())
}

func MatchVirtualSerialPortThinPrintBackingInfo(
	_ *vimtypes.VirtualSerialPortThinPrintBackingInfo,
	baseBacking vimtypes.BaseVirtualDeviceBackingInfo) bool {

	// No fields.
	_, ok := baseBacking.(*vimtypes.VirtualSerialPortThinPrintBackingInfo)
	return ok
}

func MatchVirtualSerialPortURIBackingInfo(
	expectedBacking *vimtypes.VirtualSerialPortURIBackingInfo,
	baseBacking vimtypes.BaseVirtualDeviceBackingInfo) bool {

	curBacking, ok := baseBacking.(*vimtypes.VirtualSerialPortURIBackingInfo)
	if !ok {
		return false
	}

	// No fields.

	return MatchVirtualDeviceURIBackingInfo(
		expectedBacking.GetVirtualDeviceURIBackingInfo(),
		curBacking.GetVirtualDeviceURIBackingInfo())
}

func MatchVirtualSCSIPassthroughDeviceBackingInfo(
	expectedBacking *vimtypes.VirtualSCSIPassthroughDeviceBackingInfo,
	baseBacking vimtypes.BaseVirtualDeviceBackingInfo) bool {

	curBacking, ok := baseBacking.(*vimtypes.VirtualSCSIPassthroughDeviceBackingInfo)
	if !ok {
		return false
	}

	// No fields.

	return MatchVirtualDeviceDeviceBackingInfo(
		expectedBacking.GetVirtualDeviceDeviceBackingInfo(),
		curBacking.GetVirtualDeviceDeviceBackingInfo())
}
