// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package resize

import (
	"context"
	"reflect"
	"slices"

	vimtypes "github.com/vmware/govmomi/vim25/types"
)

// CreateResizeConfigSpec takes the current VM state in the ConfigInfo and compares it to the
// desired state in the ConfigSpec, returning a ConfigSpec with any required changes to drive
// the desired state.
func CreateResizeConfigSpec(
	_ context.Context,
	ci vimtypes.VirtualMachineConfigInfo,
	cs vimtypes.VirtualMachineConfigSpec) (vimtypes.VirtualMachineConfigSpec, error) {

	outCS := vimtypes.VirtualMachineConfigSpec{}

	compareHardware(ci, cs, &outCS)
	compareCPUAllocation(ci, cs, &outCS)
	compareCPUHotAddOrRemove(ci, cs, &outCS)
	compareCPUAffinity(ci, cs, &outCS)
	compareCPUPerfCounter(ci, cs, &outCS)
	compareLatencySensitivity(ci, cs, &outCS)

	return outCS, nil
}

// compareHardware compares the ConfigSpec.Hardware.
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
	// outCS.Device = ...
	cmp(ci.Hardware.MotherboardLayout, cs.MotherboardLayout, &outCS.MotherboardLayout)
	cmp(ci.Hardware.SimultaneousThreads, cs.SimultaneousThreads, &outCS.SimultaneousThreads)
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
