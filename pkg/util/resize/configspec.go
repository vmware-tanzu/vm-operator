// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package resize

import (
	"context"
	"reflect"
	"slices"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

// CreateResizeConfigSpec takes the current VM state in the ConfigInfo and compares it to the
// desired state in the ConfigSpec, returning a ConfigSpec with any required changes to drive
// the desired state.
func CreateResizeConfigSpec(
	_ context.Context,
	ci vimtypes.VirtualMachineConfigInfo,
	cs vimtypes.VirtualMachineConfigSpec) (vimtypes.VirtualMachineConfigSpec, error) {

	outCS := vimtypes.VirtualMachineConfigSpec{}

	compareAnnotation(ci, cs, &outCS)
	compareManagedBy(ci, cs, &outCS)
	compareHardware(ci, cs, &outCS)
	compareCPUAllocation(ci, cs, &outCS)
	compareCPUHotAddOrRemove(ci, cs, &outCS)
	compareCPUAffinity(ci, cs, &outCS)
	compareCPUPerfCounter(ci, cs, &outCS)
	compareLatencySensitivity(ci, cs, &outCS)

	return outCS, nil
}

// compareAnnotation compares the ConfigInfo.Annotation.
func compareAnnotation(
	ci vimtypes.VirtualMachineConfigInfo,
	cs vimtypes.VirtualMachineConfigSpec,
	outCS *vimtypes.VirtualMachineConfigSpec) {

	if ci.Annotation == "" {
		// Only change the Annotation if it is currently unset.
		outCS.Annotation = cs.Annotation
	}
}

// compareManagedBy compares the ConfigInfo.ManagedBy.
func compareManagedBy(
	ci vimtypes.VirtualMachineConfigInfo,
	cs vimtypes.VirtualMachineConfigSpec,
	outCS *vimtypes.VirtualMachineConfigSpec) {

	if ci.ManagedBy == nil {
		// Only change the ManagedBy if it is currently unset.
		outCS.ManagedBy = cs.ManagedBy
	}
}

// compareHardware compares the ConfigInfo.Hardware.
func compareHardware(
	ci vimtypes.VirtualMachineConfigInfo,
	cs vimtypes.VirtualMachineConfigSpec,
	outCS *vimtypes.VirtualMachineConfigSpec) {

	cmp(ci.Hardware.NumCPU, cs.NumCPUs, &outCS.NumCPUs)
	cmp(ci.Hardware.NumCoresPerSocket, cs.NumCoresPerSocket, &outCS.NumCoresPerSocket)
	// outCS.AutoCoresPerSocket = ...
	cmp(int64(ci.Hardware.MemoryMB), cs.MemoryMB, &outCS.MemoryMB)
	cmpPtr(ci.Hardware.VirtualICH7MPresent, cs.VirtualICH7MPresent, &outCS.VirtualICH7MPresent)
	cmpPtr(ci.Hardware.VirtualSMCPresent, cs.VirtualSMCPresent, &outCS.VirtualSMCPresent)
	cmp(ci.Hardware.MotherboardLayout, cs.MotherboardLayout, &outCS.MotherboardLayout)
	cmp(ci.Hardware.SimultaneousThreads, cs.SimultaneousThreads, &outCS.SimultaneousThreads)

	compareHardwareDevices(ci, cs, outCS)
}

func compareHardwareDevices(
	ci vimtypes.VirtualMachineConfigInfo,
	cs vimtypes.VirtualMachineConfigSpec,
	outCS *vimtypes.VirtualMachineConfigSpec) {

	// The VM's current virtual devices.
	deviceList := object.VirtualDeviceList(ci.Hardware.Device)
	// The VM's desired virtual devices.
	csDeviceList := util.DevicesFromConfigSpec(&cs)

	var deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec

	pciDeviceChanges := comparePCIDevices(deviceList, csDeviceList)
	deviceChanges = append(deviceChanges, pciDeviceChanges...)

	outCS.DeviceChange = deviceChanges
}

func comparePCIDevices(
	currentPCIDevices, desiredPCIDevices []vimtypes.BaseVirtualDevice) []vimtypes.BaseVirtualDeviceConfigSpec {

	pciPassthruFromConfigSpec := util.SelectVirtualPCIPassthrough(desiredPCIDevices)
	expectedPCIDevices := virtualmachine.CreatePCIDevicesFromConfigSpec(pciPassthruFromConfigSpec)

	var deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec
	for _, expectedDev := range expectedPCIDevices {
		expectedPci := expectedDev.(*vimtypes.VirtualPCIPassthrough)
		expectedBacking := expectedPci.Backing
		expectedBackingType := reflect.TypeOf(expectedBacking)

		var matchingIdx = -1
		for idx, curDev := range currentPCIDevices {
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
			currentPCIDevices = append(currentPCIDevices[:matchingIdx], currentPCIDevices[matchingIdx+1:]...)
		}
	}
	// Remove any unmatched existing devices.
	removeDeviceChanges := make([]vimtypes.BaseVirtualDeviceConfigSpec, 0, len(currentPCIDevices))
	for _, dev := range currentPCIDevices {
		removeDeviceChanges = append(removeDeviceChanges, &vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
			Device:    dev,
		})
	}

	// Process any removes first.
	return append(removeDeviceChanges, deviceChanges...)
}

// compareCPUAllocation compares CPU resource allocation.
func compareCPUAllocation(
	ci vimtypes.VirtualMachineConfigInfo,
	cs vimtypes.VirtualMachineConfigSpec,
	outCS *vimtypes.VirtualMachineConfigSpec) {
	// nothing to change
	if cs.CpuAllocation == nil {
		return
	}

	ciCPUAllocation := ci.CpuAllocation
	csCPUAllocation := cs.CpuAllocation

	var cpuReservation *int64
	if csCPUAllocation.Reservation != nil {
		if ciCPUAllocation == nil || ciCPUAllocation.Reservation == nil || *ciCPUAllocation.Reservation != *csCPUAllocation.Reservation {
			cpuReservation = csCPUAllocation.Reservation
		}
	}

	var cpuLimit *int64
	if csCPUAllocation.Limit != nil {
		if ciCPUAllocation == nil || ciCPUAllocation.Limit == nil || *ciCPUAllocation.Limit != *csCPUAllocation.Limit {
			cpuLimit = csCPUAllocation.Limit
		}
	}

	var cpuShares *vimtypes.SharesInfo
	if csCPUAllocation.Shares != nil {
		if ciCPUAllocation == nil || ciCPUAllocation.Shares == nil ||
			ciCPUAllocation.Shares.Level != csCPUAllocation.Shares.Level ||
			(csCPUAllocation.Shares.Level == vimtypes.SharesLevelCustom && ciCPUAllocation.Shares.Shares != csCPUAllocation.Shares.Shares) {
			cpuShares = csCPUAllocation.Shares
		}
	}

	if cpuReservation != nil || cpuLimit != nil || cpuShares != nil {
		outCS.CpuAllocation = &vimtypes.ResourceAllocationInfo{}

		if cpuReservation != nil {
			outCS.CpuAllocation.Reservation = cpuReservation
		}

		if cpuLimit != nil {
			outCS.CpuAllocation.Limit = cpuLimit
		}

		if cpuShares != nil {
			outCS.CpuAllocation.Shares = cpuShares
		}
	}
}

// compareCPUHotAddOrRemove compares CPU hot add and remove enabled.
func compareCPUHotAddOrRemove(
	ci vimtypes.VirtualMachineConfigInfo,
	cs vimtypes.VirtualMachineConfigSpec,
	outCS *vimtypes.VirtualMachineConfigSpec) {
	cmpPtr(ci.CpuHotAddEnabled, cs.CpuHotAddEnabled, &outCS.CpuHotAddEnabled)
	cmpPtr(ci.CpuHotRemoveEnabled, cs.CpuHotRemoveEnabled, &outCS.CpuHotRemoveEnabled)
}

// compareCPUPerfCounter compares virtual CPU performance counter enablement.
func compareCPUPerfCounter(
	ci vimtypes.VirtualMachineConfigInfo,
	cs vimtypes.VirtualMachineConfigSpec,
	outCS *vimtypes.VirtualMachineConfigSpec) {
	cmpPtr(ci.VPMCEnabled, cs.VPMCEnabled, &outCS.VPMCEnabled)
}

// compareCPUAffinity compares CPU affinity settings in ConfigSpec.
func compareCPUAffinity(
	ci vimtypes.VirtualMachineConfigInfo,
	cs vimtypes.VirtualMachineConfigSpec,
	outCS *vimtypes.VirtualMachineConfigSpec) {

	if cs.CpuAffinity == nil {
		return
	}

	if ci.CpuAffinity == nil {
		outCS.CpuAffinity = cs.CpuAffinity
	}

	if ci.CpuAffinity != nil {
		slices.Sort(ci.CpuAffinity.AffinitySet)
		slices.Sort(cs.CpuAffinity.AffinitySet)
		if !reflect.DeepEqual(ci.CpuAffinity.AffinitySet, cs.CpuAffinity.AffinitySet) {
			outCS.CpuAffinity = cs.CpuAffinity
		}
	}
}

// compareLatencySensitivity compares the latency-sensitivity of the virtual machine.
func compareLatencySensitivity(
	ci vimtypes.VirtualMachineConfigInfo,
	cs vimtypes.VirtualMachineConfigSpec,
	outCS *vimtypes.VirtualMachineConfigSpec) {

	if cs.LatencySensitivity == nil {
		return
	}

	if ci.LatencySensitivity == nil ||
		ci.LatencySensitivity.Sensitivity != cs.LatencySensitivity.Sensitivity ||
		ci.LatencySensitivity.Level != cs.LatencySensitivity.Level {
		outCS.LatencySensitivity = &vimtypes.LatencySensitivity{
			Level: cs.LatencySensitivity.Level,
			// deprecated since vsphere 5.5
			//Sensitivity: cs.LatencySensitivity.Sensitivity,
		}
	}
}

func cmp[T comparable](a, b T, c *T) {
	if a != b {
		*c = b
	}
}

func cmpPtr[T comparable](a *T, b *T, c **T) {
	if (a == nil || b == nil) || *a != *b {
		*c = b
	}
}
