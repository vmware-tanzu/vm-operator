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
// ────────────────────────────────────────────────────────────────────────────

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
// This function deliberately omits topology, CPU/memory hot-add flags, and
// latency sensitivity — matching the narrow scope of the VMResizeCPUMemory
// resize path.
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

func ensureMemoryAdvanced(vm *vmopv1.VirtualMachine) {
	if vm.Spec.MemoryAdvanced == nil {
		vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{}
	}
}
