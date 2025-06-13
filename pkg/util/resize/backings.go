// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package resize

import (
	vimtypes "github.com/vmware/govmomi/vim25/types"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
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

func MatchVirtualEthernetCardNetworkBackingInfo(
	expectedBacking *vimtypes.VirtualEthernetCardNetworkBackingInfo,
	baseBacking vimtypes.BaseVirtualDeviceBackingInfo) bool {

	curBacking, ok := baseBacking.(*vimtypes.VirtualEthernetCardNetworkBackingInfo)
	if !ok {
		return false
	}

	// Ignore Network field since this is just used for testing.
	// InPassthroughMode is deprecated.

	return MatchVirtualDeviceDeviceBackingInfo(
		expectedBacking.GetVirtualDeviceDeviceBackingInfo(),
		curBacking.GetVirtualDeviceDeviceBackingInfo())
}

func MatchVirtualEthernetCardDistributedVirtualPortBackingInfo(
	expectedBacking *vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo,
	baseBacking vimtypes.BaseVirtualDeviceBackingInfo) bool {

	curBacking, ok := baseBacking.(*vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
	if !ok {
		return false
	}

	return expectedBacking.Port.SwitchUuid == curBacking.Port.SwitchUuid &&
		expectedBacking.Port.PortgroupKey == curBacking.Port.PortgroupKey
}

func MatchVirtualEthernetCardOpaqueNetworkBackingInfo(
	expectedBacking *vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo,
	baseBacking vimtypes.BaseVirtualDeviceBackingInfo) bool {

	curBacking, ok := baseBacking.(*vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo)
	if !ok {
		return false
	}

	return expectedBacking.OpaqueNetworkId == curBacking.OpaqueNetworkId &&
		expectedBacking.OpaqueNetworkType == curBacking.OpaqueNetworkType
}

func MatchVirtualPCIPassthroughVmiopBackingInfo(
	expectedBacking *vimtypes.VirtualPCIPassthroughVmiopBackingInfo,
	baseBacking vimtypes.BaseVirtualDeviceBackingInfo) bool {

	curBacking, ok := baseBacking.(*vimtypes.VirtualPCIPassthroughVmiopBackingInfo)
	if !ok {
		return false
	}

	if expectedBacking.Vgpu != curBacking.Vgpu {
		return false
	}

	return true
}

func MatchVirtualPCIPassthroughDynamicBackingInfo(
	expectedBacking *vimtypes.VirtualPCIPassthroughDynamicBackingInfo,
	baseBacking vimtypes.BaseVirtualDeviceBackingInfo) bool {

	curBacking, ok := baseBacking.(*vimtypes.VirtualPCIPassthroughDynamicBackingInfo)
	if !ok {
		return false
	}

	// This is based on govmoni's SelectByBackingInfo().

	if label := expectedBacking.CustomLabel; label != "" {
		if label != curBacking.CustomLabel {
			return false
		}
	}

	if len(expectedBacking.AllowedDevice) == 0 {
		return true
	}

	for _, x := range curBacking.AllowedDevice {
		for _, y := range expectedBacking.AllowedDevice {
			if x.DeviceId == y.DeviceId && x.VendorId == y.VendorId {
				return true
			}
		}
	}

	return false
}

func MatchVirtualPCIPassthroughDVXBackingInfo(
	b *vimtypes.VirtualPCIPassthroughDvxBackingInfo,
	baseBacking vimtypes.BaseVirtualDeviceBackingInfo) bool {

	a, ok := baseBacking.(*vimtypes.VirtualPCIPassthroughDvxBackingInfo)
	if !ok {
		return false
	}

	if a.DeviceClass == b.DeviceClass {
		if len(a.ConfigParams) == len(b.ConfigParams) {
			am := make(map[string]any, len(a.ConfigParams))
			bm := make(map[string]any, len(b.ConfigParams))
			for i := 0; i < len(a.ConfigParams); i++ {
				aov := a.ConfigParams[i].GetOptionValue()
				bov := b.ConfigParams[i].GetOptionValue()
				am[aov.Key] = aov.Value
				bm[bov.Key] = bov.Value
			}
			return apiequality.Semantic.DeepEqual(am, bm)
		}
	}

	return false
}
