// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"k8s.io/apimachinery/pkg/api/resource"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

// ─── Design note ────────────────────────────────────────────────────────────
//
// To maintain backward compatibility for class-based resize, when a resize is
// triggered (ResizeNeeded returns true), the class is authoritative and
// overrides all spec compute fields it defines — whether those values were
// user-specified or backfilled from a previous reconcile. The reconciler copies
// the class's compute fields into vm.Spec before building the vSphere
// ConfigSpec diff so that OverwriteSpecComputeConfig applies the class intent
// rather than the current spec values.
//
// This sync is performed in the reconciler rather than the mutating webhook
// because the VirtualMachineClass need not exist when the webhook fires —
// the class can be created at any point after the VM is admitted as well.
//
// For the full class resize path (VMResize feature), device changes such as
// PCI passthrough are also applied within the same reconcile. Keeping the spec
// sync here ensures that spec changes and device changes are applied atomically
// in a single vSphere reconfigure call. The subsequent reconcile observes no
// diff and is a no-op.
//
// ────────────────────────────────────────────────────────────────────────────

// SyncClassComputeToSpec copies all compute configuration from a class-derived
// ConfigSpec into vm.Spec. It is called when a full class-based resize is
// triggered (VMResize feature, ResizeNeeded returns true) so that
// OverwriteSpecComputeConfig applies the class intent rather than stale or
// backfilled spec values.
//
// Fields not set in classCS are left unchanged in spec. Any field that classCS
// explicitly configures is unconditionally written to spec so that the class is
// the authority for this resize action.
func SyncClassComputeToSpec(
	vm *vmopv1.VirtualMachine,
	classCS vimtypes.VirtualMachineConfigSpec) {

	syncClassSizeToSpec(vm, classCS)
	syncClassAllocationToSpec(vm, classCS)
	syncClassTopologyToSpec(vm, classCS)
	syncClassLatencySensitivityToSpec(vm, classCS)
	syncClassCPUFlagsToSpec(vm, classCS)
	syncClassMemoryFlagsToSpec(vm, classCS)
}

// SyncClassSizeAndAllocationToSpec copies CPU/memory size, allocation, and the
// memory reservation lock from a class-derived ConfigSpec into vm.Spec. It is
// called when a CPU/memory-only resize is triggered (VMResizeCPUMemory feature)
// so that OverwriteSpecComputeConfig does not override the class's new CPU/
// memory values with backfilled spec values.
//
// MemoryReservationLockedToMax is included because it is semantically part of
// memory allocation — a guaranteed class locks the reservation to the full
// memory size, and without syncing this field OverwriteSpecComputeConfig would
// apply the backfilled value instead.
//
// Unlike SyncClassComputeToSpec, this function deliberately omits topology,
// CPU/memory hot-add flags, and latency sensitivity — matching the narrow scope
// of the VMResizeCPUMemory resize path.
func SyncClassSizeAndAllocationToSpec(
	vm *vmopv1.VirtualMachine,
	classCS vimtypes.VirtualMachineConfigSpec) {

	syncClassSizeToSpec(vm, classCS)
	syncClassAllocationToSpec(vm, classCS)
	syncClassMemoryReservationLockToSpec(vm, classCS)
}

func syncClassSizeToSpec(vm *vmopv1.VirtualMachine, cs vimtypes.VirtualMachineConfigSpec) {
	if cs.NumCPUs > 0 {
		ensureSize(vm)
		vm.Spec.Resources.Size.CPU = resource.NewQuantity(int64(cs.NumCPUs), resource.DecimalSI)
	}
	if cs.MemoryMB > 0 {
		ensureSize(vm)
		vm.Spec.Resources.Size.Memory = resource.NewQuantity(
			cs.MemoryMB*1024*1024, resource.BinarySI)
	}
}

func syncClassAllocationToSpec(vm *vmopv1.VirtualMachine, cs vimtypes.VirtualMachineConfigSpec) {
	if a := cs.CpuAllocation; a != nil {
		if res := a.Reservation; res != nil {
			if *res > 0 {
				ensureRequests(vm)
				vm.Spec.Resources.Requests.CPU = resource.NewQuantity(*res, resource.DecimalSI)
			} else if vm.Spec.Resources != nil && vm.Spec.Resources.Requests != nil {
				// Class sets no CPU reservation (best-effort, Reservation=0): clear any
				// previously backfilled or user-specified reservation so that
				// OverwriteSpecComputeConfig does not re-apply the old value.
				vm.Spec.Resources.Requests.CPU = nil
			}
		}
		if lim := a.Limit; lim != nil {
			if *lim > 0 {
				ensureLimits(vm)
				vm.Spec.Resources.Limits.CPU = resource.NewQuantity(*lim, resource.DecimalSI)
			} else if *lim < 0 && vm.Spec.Resources != nil && vm.Spec.Resources.Limits != nil {
				// Class sets unlimited CPU (Limit=-1): clear any previously backfilled
				// or user-specified limit so the VM is uncapped in vSphere.
				vm.Spec.Resources.Limits.CPU = nil
			}
		}
	}

	if a := cs.MemoryAllocation; a != nil {
		if res := a.Reservation; res != nil {
			if *res > 0 {
				ensureRequests(vm)
				vm.Spec.Resources.Requests.Memory = resource.NewQuantity(
					*res*1024*1024, resource.BinarySI)
			} else if vm.Spec.Resources != nil && vm.Spec.Resources.Requests != nil {
				// Class sets no memory reservation (best-effort, Reservation=0): clear any
				// previously backfilled or user-specified reservation.
				vm.Spec.Resources.Requests.Memory = nil
			}
		}
		if lim := a.Limit; lim != nil {
			if *lim > 0 {
				ensureLimits(vm)
				vm.Spec.Resources.Limits.Memory = resource.NewQuantity(
					*lim*1024*1024, resource.BinarySI)
			} else if *lim < 0 && vm.Spec.Resources != nil && vm.Spec.Resources.Limits != nil {
				// Class sets unlimited memory (Limit=-1): clear any previously backfilled
				// or user-specified limit.
				vm.Spec.Resources.Limits.Memory = nil
			}
		}
	}
}

func syncClassTopologyToSpec(vm *vmopv1.VirtualMachine, cs vimtypes.VirtualMachineConfigSpec) {
	if cs.NumCoresPerSocket != nil {
		ensureTopology(vm)
		cps := *cs.NumCoresPerSocket
		vm.Spec.CPUAdvanced.Topology.CoresPerSocket = &cps
	}

	if n := cs.VirtualNuma; n != nil &&
		n.CoresPerNumaNode != nil &&
		*n.CoresPerNumaNode > 0 &&
		cs.NumCPUs > 0 &&
		cs.NumCPUs%*n.CoresPerNumaNode == 0 {

		// Only sync when NumCPUs divides evenly by CoresPerNumaNode: a
		// non-exact division would round-trip through
		// vnumaNodeCountField's inverse division in compute_overwrite.go
		// (CoresPerNumaNode = NumCPUs / VNUMANodeCount) and silently drift
		// from what the class's CoresPerNumaNode actually specified.
		nodeCount := cs.NumCPUs / *n.CoresPerNumaNode
		if nodeCount > 0 {
			ensureTopology(vm)
			vm.Spec.CPUAdvanced.Topology.VNUMANodeCount = &nodeCount
		}
	}
}

func syncClassLatencySensitivityToSpec(vm *vmopv1.VirtualMachine, cs vimtypes.VirtualMachineConfigSpec) {
	if cs.LatencySensitivity == nil {
		return
	}

	var level vmopv1.VirtualMachineLatencySensitivityLevel
	switch cs.LatencySensitivity.Level {
	case vimtypes.LatencySensitivitySensitivityLevelHigh:
		if cs.SimultaneousThreads == 2 {
			level = vmopv1.VirtualMachineLatencySensitivityHighWithHyperthreading
		} else {
			level = vmopv1.VirtualMachineLatencySensitivityHigh
		}
	case vimtypes.LatencySensitivitySensitivityLevelNormal:
		level = vmopv1.VirtualMachineLatencySensitivityNormal
	default:
		return
	}

	ensureCPUAdvanced(vm)
	vm.Spec.CPUAdvanced.LatencySensitivity = &level
}

func syncClassCPUFlagsToSpec(vm *vmopv1.VirtualMachine, cs vimtypes.VirtualMachineConfigSpec) {
	if cs.CpuHotAddEnabled != nil {
		ensureCPUAdvanced(vm)
		v := *cs.CpuHotAddEnabled
		vm.Spec.CPUAdvanced.HotAddEnabled = &v
	}

	if cs.Flags != nil && cs.Flags.VvtdEnabled != nil {
		ensureCPUAdvanced(vm)
		v := *cs.Flags.VvtdEnabled
		vm.Spec.CPUAdvanced.IOMMUEnabled = &v
	}

	if cs.NestedHVEnabled != nil {
		ensureCPUAdvanced(vm)
		v := *cs.NestedHVEnabled
		vm.Spec.CPUAdvanced.NestedHardwareVirtualizationEnabled = &v
	}

	if cs.VPMCEnabled != nil {
		ensureCPUAdvanced(vm)
		v := *cs.VPMCEnabled
		vm.Spec.CPUAdvanced.PerformanceCountersEnabled = &v
	}
}

// syncClassMemoryReservationLockToSpec syncs the MemoryReservationLockedToMax
// field from the class ConfigSpec to vm.Spec. It is used by the narrow
// VMResizeCPUMemory path because the lock is directly tied to memory allocation
// semantics: a guaranteed class sets it true, a best-effort class sets it false,
// and OverwriteSpecComputeConfig will apply whatever the spec says — so the spec
// must reflect the class's intent before OverwriteSpecComputeConfig runs.
func syncClassMemoryReservationLockToSpec(vm *vmopv1.VirtualMachine, cs vimtypes.VirtualMachineConfigSpec) {
	if cs.MemoryReservationLockedToMax != nil {
		ensureMemoryAdvanced(vm)
		v := *cs.MemoryReservationLockedToMax
		vm.Spec.MemoryAdvanced.ReservationLockedToMax = &v
	}
}

func syncClassMemoryFlagsToSpec(vm *vmopv1.VirtualMachine, cs vimtypes.VirtualMachineConfigSpec) {
	if cs.MemoryHotAddEnabled != nil {
		ensureMemoryAdvanced(vm)
		v := *cs.MemoryHotAddEnabled
		vm.Spec.MemoryAdvanced.HotAddEnabled = &v
	}

	syncClassMemoryReservationLockToSpec(vm, cs)
}

// ─── lazy-init helpers ──────────────────────────────────────────────────────

func ensureResources(vm *vmopv1.VirtualMachine) {
	if vm.Spec.Resources == nil {
		vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{}
	}
}

func ensureSize(vm *vmopv1.VirtualMachine) {
	ensureResources(vm)
	if vm.Spec.Resources.Size == nil {
		vm.Spec.Resources.Size = &vmopv1.VirtualMachineResourceQuantity{}
	}
}

func ensureRequests(vm *vmopv1.VirtualMachine) {
	ensureResources(vm)
	if vm.Spec.Resources.Requests == nil {
		vm.Spec.Resources.Requests = &vmopv1.VirtualMachineResourceQuantity{}
	}
}

func ensureLimits(vm *vmopv1.VirtualMachine) {
	ensureResources(vm)
	if vm.Spec.Resources.Limits == nil {
		vm.Spec.Resources.Limits = &vmopv1.VirtualMachineResourceQuantity{}
	}
}

func ensureCPUAdvanced(vm *vmopv1.VirtualMachine) {
	if vm.Spec.CPUAdvanced == nil {
		vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{}
	}
}

func ensureTopology(vm *vmopv1.VirtualMachine) {
	ensureCPUAdvanced(vm)
	if vm.Spec.CPUAdvanced.Topology == nil {
		vm.Spec.CPUAdvanced.Topology = &vmopv1.VirtualMachineCPUTopologySpec{}
	}
}

func ensureMemoryAdvanced(vm *vmopv1.VirtualMachine) {
	if vm.Spec.MemoryAdvanced == nil {
		vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{}
	}
}
