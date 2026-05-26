// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package backfill

import (
	"context"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/api/resource"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// ComputeConfigFromMoVM populates spec.resources, spec.cpuAdvanced, and
// spec.memoryAdvanced from the live vSphere VM ConfigInfo.
//
// Spec wins: only nil spec fields are written; existing non-nil values are
// left unchanged so that any intentional pre-upgrade user setting is
// preserved.
//
// Called once per VM during schema upgrade when FeatureVersionTelcoVMServiceAPI
// is being set. Returns true if any field was mutated.
func ComputeConfigFromMoVM(
	_ context.Context,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine) (bool, error) {

	if moVM.Config == nil {
		return false, nil
	}

	ci := moVM.Config
	mutated := backfillSize(vm, ci)
	if backfillAllocation(vm, ci) {
		mutated = true
	}
	if backfillLatencySensitivity(vm, ci) {
		mutated = true
	}
	if backfillCPUTopology(vm, ci) {
		mutated = true
	}
	if backfillCPUFlags(vm, ci) {
		mutated = true
	}
	if backfillMemoryFlags(vm, ci) {
		mutated = true
	}

	return mutated, nil
}

// backfillSize populates spec.resources.size.{cpu,memory} from hardware config.
func backfillSize(vm *vmopv1.VirtualMachine, ci *vimtypes.VirtualMachineConfigInfo) bool {
	mutated := false

	if ci.Hardware.NumCPU > 0 &&
		(vm.Spec.Resources == nil ||
			vm.Spec.Resources.Size == nil ||
			vm.Spec.Resources.Size.CPU == nil) {

		initSize(vm)
		vm.Spec.Resources.Size.CPU = resource.NewQuantity(
			int64(ci.Hardware.NumCPU), resource.DecimalSI)
		mutated = true
	}

	if ci.Hardware.MemoryMB > 0 &&
		(vm.Spec.Resources == nil ||
			vm.Spec.Resources.Size == nil ||
			vm.Spec.Resources.Size.Memory == nil) {

		initSize(vm)
		vm.Spec.Resources.Size.Memory = resource.NewQuantity(
			int64(ci.Hardware.MemoryMB)*1024*1024, resource.BinarySI)
		mutated = true
	}

	return mutated
}

// backfillAllocation populates spec.resources.{requests,limits}.{cpu,memory}
// from CpuAllocation and MemoryAllocation.
//
// Zero reservations and non-positive limits (including the -1 "unlimited"
// sentinel) are not backfilled — they map to the nil = "not set" convention.
func backfillAllocation(vm *vmopv1.VirtualMachine, ci *vimtypes.VirtualMachineConfigInfo) bool {
	mutated := false

	if a := ci.CpuAllocation; a != nil {
		if res := a.Reservation; res != nil && *res > 0 {
			if vm.Spec.Resources == nil ||
				vm.Spec.Resources.Requests == nil ||
				vm.Spec.Resources.Requests.CPU == nil {

				initRequests(vm)
				vm.Spec.Resources.Requests.CPU = resource.NewQuantity(*res, resource.DecimalSI)
				mutated = true
			}
		}
		if lim := a.Limit; lim != nil && *lim > 0 {
			if vm.Spec.Resources == nil ||
				vm.Spec.Resources.Limits == nil ||
				vm.Spec.Resources.Limits.CPU == nil {

				initLimits(vm)
				vm.Spec.Resources.Limits.CPU = resource.NewQuantity(*lim, resource.DecimalSI)
				mutated = true
			}
		}
	}

	if a := ci.MemoryAllocation; a != nil {
		if res := a.Reservation; res != nil && *res > 0 {
			if vm.Spec.Resources == nil ||
				vm.Spec.Resources.Requests == nil ||
				vm.Spec.Resources.Requests.Memory == nil {

				initRequests(vm)
				vm.Spec.Resources.Requests.Memory = resource.NewQuantity(
					*res*1024*1024, resource.BinarySI)
				mutated = true
			}
		}
		if lim := a.Limit; lim != nil && *lim > 0 {
			if vm.Spec.Resources == nil ||
				vm.Spec.Resources.Limits == nil ||
				vm.Spec.Resources.Limits.Memory == nil {

				initLimits(vm)
				vm.Spec.Resources.Limits.Memory = resource.NewQuantity(
					*lim*1024*1024, resource.BinarySI)
				mutated = true
			}
		}
	}

	return mutated
}

// backfillLatencySensitivity populates spec.cpuAdvanced.latencySensitivity
// from the vSphere LatencySensitivity config.
//
// High + SimultaneousThreads=2 → HighWithHyperthreading.
// High otherwise → High.
// Normal → Normal.
// Low and all other levels are not backfilled (nil = default is equivalent).
func backfillLatencySensitivity(
	vm *vmopv1.VirtualMachine,
	ci *vimtypes.VirtualMachineConfigInfo) bool {

	if ci.LatencySensitivity == nil {
		return false
	}

	if vm.Spec.CPUAdvanced != nil && vm.Spec.CPUAdvanced.LatencySensitivity != nil {
		return false // spec wins
	}

	var level vmopv1.VirtualMachineLatencySensitivityLevel
	switch ci.LatencySensitivity.Level {
	case vimtypes.LatencySensitivitySensitivityLevelHigh:
		if ci.Hardware.SimultaneousThreads == 2 {
			level = vmopv1.VirtualMachineLatencySensitivityHighWithHyperthreading
		} else {
			level = vmopv1.VirtualMachineLatencySensitivityHigh
		}
	case vimtypes.LatencySensitivitySensitivityLevelNormal:
		level = vmopv1.VirtualMachineLatencySensitivityNormal
	default:
		return false
	}

	initCPUAdvanced(vm)
	vm.Spec.CPUAdvanced.LatencySensitivity = &level
	return true
}

// backfillCPUTopology populates spec.cpuAdvanced.topology fields from
// hardware config and vNUMA info.
func backfillCPUTopology(
	vm *vmopv1.VirtualMachine,
	ci *vimtypes.VirtualMachineConfigInfo) bool {

	mutated := false

	if ci.Hardware.NumCoresPerSocket != nil && *ci.Hardware.NumCoresPerSocket > 0 {
		if vm.Spec.CPUAdvanced == nil ||
			vm.Spec.CPUAdvanced.Topology == nil ||
			vm.Spec.CPUAdvanced.Topology.CoresPerSocket == nil {

			initTopology(vm)
			cps := *ci.Hardware.NumCoresPerSocket
			vm.Spec.CPUAdvanced.Topology.CoresPerSocket = &cps
			mutated = true
		}
	}

	if n := ci.NumaInfo; n != nil {
		// Derive vnumaNodeCount when AutoCoresPerNumaNode is not set to true
		// (nil means "not configured" which is treated as manual = false).
		if (n.AutoCoresPerNumaNode == nil || !*n.AutoCoresPerNumaNode) &&
			ci.Hardware.NumCPU > 0 &&
			n.CoresPerNumaNode != nil &&
			*n.CoresPerNumaNode > 0 &&
			ci.Hardware.NumCPU%*n.CoresPerNumaNode == 0 {

			if vm.Spec.CPUAdvanced == nil ||
				vm.Spec.CPUAdvanced.Topology == nil ||
				vm.Spec.CPUAdvanced.Topology.VNUMANodeCount == nil {

				initTopology(vm)
				nodeCount := ci.Hardware.NumCPU / *n.CoresPerNumaNode
				vm.Spec.CPUAdvanced.Topology.VNUMANodeCount = &nodeCount
				mutated = true
			}
		}

		if n.VnumaOnCpuHotaddExposed != nil && *n.VnumaOnCpuHotaddExposed {
			if vm.Spec.CPUAdvanced == nil ||
				vm.Spec.CPUAdvanced.Topology == nil ||
				vm.Spec.CPUAdvanced.Topology.ExposeVNUMAOnCPUHotAdd == nil {

				initTopology(vm)
				vm.Spec.CPUAdvanced.Topology.ExposeVNUMAOnCPUHotAdd = ptr.To(true)
				mutated = true
			}
		}
	}

	return mutated
}

// backfillCPUFlags populates spec.cpuAdvanced boolean flags from ConfigInfo.
// Only true values are backfilled — false and nil both mean "disabled" and
// are indistinguishable from the unset default.
func backfillCPUFlags(
	vm *vmopv1.VirtualMachine,
	ci *vimtypes.VirtualMachineConfigInfo) bool {

	mutated := false

	if ci.CpuHotAddEnabled != nil && *ci.CpuHotAddEnabled {
		if vm.Spec.CPUAdvanced == nil || vm.Spec.CPUAdvanced.HotAddEnabled == nil {
			initCPUAdvanced(vm)
			vm.Spec.CPUAdvanced.HotAddEnabled = ptr.To(true)
			mutated = true
		}
	}

	if ci.Flags.VvtdEnabled != nil && *ci.Flags.VvtdEnabled {
		if vm.Spec.CPUAdvanced == nil || vm.Spec.CPUAdvanced.IOMMUEnabled == nil {
			initCPUAdvanced(vm)
			vm.Spec.CPUAdvanced.IOMMUEnabled = ptr.To(true)
			mutated = true
		}
	}

	if ci.NestedHVEnabled != nil && *ci.NestedHVEnabled {
		if vm.Spec.CPUAdvanced == nil || vm.Spec.CPUAdvanced.NestedHardwareVirtualizationEnabled == nil {
			initCPUAdvanced(vm)
			vm.Spec.CPUAdvanced.NestedHardwareVirtualizationEnabled = ptr.To(true)
			mutated = true
		}
	}

	if ci.VPMCEnabled != nil && *ci.VPMCEnabled {
		if vm.Spec.CPUAdvanced == nil || vm.Spec.CPUAdvanced.PerformanceCountersEnabled == nil {
			initCPUAdvanced(vm)
			vm.Spec.CPUAdvanced.PerformanceCountersEnabled = ptr.To(true)
			mutated = true
		}
	}

	return mutated
}

// backfillMemoryFlags populates spec.memoryAdvanced boolean flags.
// Only true values are backfilled.
func backfillMemoryFlags(
	vm *vmopv1.VirtualMachine,
	ci *vimtypes.VirtualMachineConfigInfo) bool {

	mutated := false

	if ci.MemoryHotAddEnabled != nil && *ci.MemoryHotAddEnabled {
		if vm.Spec.MemoryAdvanced == nil || vm.Spec.MemoryAdvanced.HotAddEnabled == nil {
			initMemoryAdvanced(vm)
			vm.Spec.MemoryAdvanced.HotAddEnabled = ptr.To(true)
			mutated = true
		}
	}

	if ci.MemoryReservationLockedToMax != nil && *ci.MemoryReservationLockedToMax {
		if vm.Spec.MemoryAdvanced == nil || vm.Spec.MemoryAdvanced.ReservationLockedToMax == nil {
			initMemoryAdvanced(vm)
			vm.Spec.MemoryAdvanced.ReservationLockedToMax = ptr.To(true)
			mutated = true
		}
	}

	return mutated
}

// ------------------------------------------------------------------ //
// Lazy-init helpers — only call after confirming a write is needed.
// ------------------------------------------------------------------ //

func initResources(vm *vmopv1.VirtualMachine) {
	if vm.Spec.Resources == nil {
		vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{}
	}
}

func initSize(vm *vmopv1.VirtualMachine) {
	initResources(vm)
	if vm.Spec.Resources.Size == nil {
		vm.Spec.Resources.Size = &vmopv1.VirtualMachineResourceQuantity{}
	}
}

func initRequests(vm *vmopv1.VirtualMachine) {
	initResources(vm)
	if vm.Spec.Resources.Requests == nil {
		vm.Spec.Resources.Requests = &vmopv1.VirtualMachineResourceQuantity{}
	}
}

func initLimits(vm *vmopv1.VirtualMachine) {
	initResources(vm)
	if vm.Spec.Resources.Limits == nil {
		vm.Spec.Resources.Limits = &vmopv1.VirtualMachineResourceQuantity{}
	}
}

func initCPUAdvanced(vm *vmopv1.VirtualMachine) {
	if vm.Spec.CPUAdvanced == nil {
		vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{}
	}
}

func initTopology(vm *vmopv1.VirtualMachine) {
	initCPUAdvanced(vm)
	if vm.Spec.CPUAdvanced.Topology == nil {
		vm.Spec.CPUAdvanced.Topology = &vmopv1.VirtualMachineCPUTopologySpec{}
	}
}

func initMemoryAdvanced(vm *vmopv1.VirtualMachine) {
	if vm.Spec.MemoryAdvanced == nil {
		vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{}
	}
}
