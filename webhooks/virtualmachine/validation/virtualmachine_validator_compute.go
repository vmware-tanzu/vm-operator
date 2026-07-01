// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/validation/field"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

// validateComputeConfig validates spec.resources, spec.cpuAdvanced, and
// spec.memoryAdvanced. Called from both ValidateCreate and ValidateUpdate.
func (v validator) validateComputeConfig(
	ctx *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	if !pkgcfg.FromContext(ctx).Features.TelcoVMServiceAPI {
		notEnabled := fmt.Sprintf(featureNotEnabled, "VM Compute Config (via TelcoVMServiceAPI)")
		if vm.Spec.Resources != nil {
			allErrs = append(allErrs, field.Forbidden(
				specPath.Child("resources"), notEnabled))
		}
		if vm.Spec.CPUAdvanced != nil {
			allErrs = append(allErrs, field.Forbidden(
				specPath.Child("cpuAdvanced"), notEnabled))
		}
		if vm.Spec.MemoryAdvanced != nil {
			allErrs = append(allErrs, field.Forbidden(
				specPath.Child("memoryAdvanced"), notEnabled))
		}
		return allErrs
	}

	allErrs = append(allErrs, validateComputeResourceFields(specPath, vm)...)
	allErrs = append(allErrs, validateComputeLatencySensitivity(specPath, vm)...)
	allErrs = append(allErrs, validateComputeReservationLockedToMax(specPath, vm)...)
	allErrs = append(allErrs, validateComputeTopology(specPath, vm)...)

	return allErrs
}

// validateComputeConfigBackfilledFieldsNotChanged checks that spec.resources,
// spec.cpuAdvanced, and spec.memoryAdvanced are unchanged during the schema
// upgrade window.
//
// Returns one field.Forbidden error per changed top-level field.
func validateComputeConfigBackfilledFieldsNotChanged(
	specPath *field.Path,
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList

	if !apiequality.Semantic.DeepEqual(vm.Spec.Resources, oldVM.Spec.Resources) {
		allErrs = append(allErrs, field.Forbidden(
			specPath.Child("resources"), notUpgraded))
	}
	if !apiequality.Semantic.DeepEqual(vm.Spec.CPUAdvanced, oldVM.Spec.CPUAdvanced) {
		allErrs = append(allErrs, field.Forbidden(
			specPath.Child("cpuAdvanced"), notUpgraded))
	}
	if !apiequality.Semantic.DeepEqual(vm.Spec.MemoryAdvanced, oldVM.Spec.MemoryAdvanced) {
		allErrs = append(allErrs, field.Forbidden(
			specPath.Child("memoryAdvanced"), notUpgraded))
	}

	return allErrs
}

// validateComputeResourceFields validates the spec.resources sub-fields:
// zero checks on size/limits, and ordering constraints between
// requests/size/limits.
func validateComputeResourceFields(specPath *field.Path, vm *vmopv1.VirtualMachine) field.ErrorList {
	resources := vm.Spec.Resources
	if resources == nil {
		return nil
	}

	var (
		allErrs  field.ErrorList
		resPath  = specPath.Child("resources")
		sizePath = resPath.Child("size")
		reqPath  = resPath.Child("requests")
		limPath  = resPath.Child("limits")
		size     = resources.Size
		req      = resources.Requests
		lim      = resources.Limits
	)

	// size.{cpu,memory} > 0 when set
	if size != nil {
		if size.CPU != nil && (size.CPU.IsZero() || size.CPU.Value() < 0) {
			allErrs = append(allErrs, field.Invalid(
				sizePath.Child("cpu"), size.CPU.String(),
				"must be greater than 0 when set"))
		}
		if size.Memory != nil && (size.Memory.IsZero() || size.Memory.Value() < 0) {
			allErrs = append(allErrs, field.Invalid(
				sizePath.Child("memory"), size.Memory.String(),
				"must be greater than 0 when set"))
		}
	}

	// limits.{cpu,memory} must be > 0 or -1 when set (nil = unlimited, -1 = unlimited sentinel)
	if lim != nil {
		if lim.CPU != nil && lim.CPU.IsZero() {
			allErrs = append(allErrs, field.Invalid(
				limPath.Child("cpu"), lim.CPU.String(),
				"must be greater than 0 or -1 (unlimited) when set (nil = unlimited)"))
		}
		if lim.Memory != nil && lim.Memory.IsZero() {
			allErrs = append(allErrs, field.Invalid(
				limPath.Child("memory"), lim.Memory.String(),
				"must be greater than 0 or -1 (unlimited) when set (nil = unlimited)"))
		}
	}

	// requests.cpu ≤ limits.cpu
	if req != nil && lim != nil &&
		req.CPU != nil && lim.CPU != nil {
		if req.CPU.Cmp(*lim.CPU) > 0 {
			allErrs = append(allErrs, field.Invalid(
				reqPath.Child("cpu"), req.CPU.String(),
				"must be less than or equal to limits.cpu"))
		}
	}

	// requests.memory ≤ size.memory
	if req != nil && size != nil &&
		req.Memory != nil && size.Memory != nil {
		if req.Memory.Cmp(*size.Memory) > 0 {
			allErrs = append(allErrs, field.Invalid(
				reqPath.Child("memory"), req.Memory.String(),
				"must be less than or equal to size.memory"))
		}
	}

	// requests.memory ≤ limits.memory
	if req != nil && lim != nil &&
		req.Memory != nil && lim.Memory != nil {
		if req.Memory.Cmp(*lim.Memory) > 0 {
			allErrs = append(allErrs, field.Invalid(
				reqPath.Child("memory"), req.Memory.String(),
				"must be less than or equal to limits.memory"))
		}
	}

	return allErrs
}

// validateComputeLatencySensitivity enforces full CPU and memory reservation
// when latencySensitivity is High or HighWithHyperthreading.
//
// Memory reservation invariant is satisfied when either:
//   - requests.memory != nil AND size.memory != nil AND requests.memory == size.memory
//   - OR memoryAdvanced.reservationLockedToMax == true
//
// CPU reservation invariant is satisfied when requests.cpu is set and > 0.
// The webhook cannot verify that the value equals 100% of vCPU capacity
// (which depends on the host CPU speed), so it only checks that a non-zero
// reservation is explicitly provided.
//
// Any other combination (including partial nil) is rejected because the
// full-reservation invariant cannot be verified with incomplete information.
func validateComputeLatencySensitivity(specPath *field.Path, vm *vmopv1.VirtualMachine) field.ErrorList {
	cpuAdv := vm.Spec.CPUAdvanced
	if cpuAdv == nil || cpuAdv.LatencySensitivity == nil {
		return nil
	}

	ls := *cpuAdv.LatencySensitivity
	if ls != vmopv1.VirtualMachineLatencySensitivityHigh &&
		ls != vmopv1.VirtualMachineLatencySensitivityHighWithHyperthreading {
		return nil
	}

	var (
		allErrs field.ErrorList
		lsPath  = specPath.Child("cpuAdvanced").Child("latencySensitivity")
	)

	// Full memory reservation: reservationLockedToMax or requests.memory == size.memory.
	if !vmopv1util.FullMemReservationSpecMet(vm) {
		allErrs = append(allErrs, field.Invalid(lsPath, string(ls),
			"requires full memory reservation: "+
				"spec.resources.requests.memory must equal spec.resources.size.memory, "+
				"or spec.memoryAdvanced.reservationLockedToMax must be true"))
	}

	// Full CPU reservation: requests.cpu must be explicitly set to > 0.
	// The webhook cannot verify the exact MHz value equals 100% of vCPU
	// capacity (which depends on host CPU speed at placement time), so it
	// only checks that a non-zero reservation is explicitly provided.
	resources := vm.Spec.Resources
	cpuReservationMet := resources != nil &&
		resources.Requests != nil &&
		resources.Requests.CPU != nil &&
		!resources.Requests.CPU.IsZero()
	if !cpuReservationMet {
		allErrs = append(allErrs, field.Invalid(lsPath, string(ls),
			"High Latency Sensitivity requires you to set 100% CPU reservation for this VM"))
	}

	return allErrs
}

// validateComputeReservationLockedToMax validates mutual exclusion and ordering
// rules when memoryAdvanced.reservationLockedToMax is true:
//   - requests.memory must not be set: vSphere locks the reservation to the full
//     guest size and overrides any explicit value in the first reconcile; in
//     subsequent reconciles the reservation-only ConfigSpec is validated by
//     vSphere, causing an InvalidArgument error if the value differs from the
//     locked size.
//   - limits.memory, when set, must be >= size.memory: vSphere requires
//     Limit >= Reservation, and the lock pins the effective Reservation to
//     size.memory.
func validateComputeReservationLockedToMax(specPath *field.Path, vm *vmopv1.VirtualMachine) field.ErrorList {
	var (
		allErrs   field.ErrorList
		resources = vm.Spec.Resources
		memAdv    = vm.Spec.MemoryAdvanced
	)

	if memAdv == nil || !ptr.Deref(memAdv.ReservationLockedToMax) {
		return nil
	}

	if resources != nil && resources.Requests != nil && resources.Requests.Memory != nil {
		allErrs = append(allErrs, field.Invalid(
			specPath.Child("resources").Child("requests").Child("memory"),
			resources.Requests.Memory.String(),
			"must not be set when spec.memoryAdvanced.reservationLockedToMax is true: "+
				"vSphere owns the memory reservation when the lock is active"))
	}

	if resources != nil &&
		resources.Limits != nil && resources.Limits.Memory != nil &&
		resources.Size != nil && resources.Size.Memory != nil {
		if resources.Limits.Memory.Cmp(*resources.Size.Memory) < 0 {
			allErrs = append(allErrs, field.Invalid(
				specPath.Child("resources").Child("limits").Child("memory"),
				resources.Limits.Memory.String(),
				"must be greater than or equal to size.memory when "+
					"spec.memoryAdvanced.reservationLockedToMax is true: "+
					"vSphere pins the reservation to size.memory and requires Limit >= Reservation"))
		}
	}

	return allErrs
}

// validateComputeTopology validates spec.cpuAdvanced.topology constraints:
//   - vnumaNodeCount > 0 requires coresPerSocket > 0 (auto/zero coresPerSocket
//     is not allowed when vnumaNodeCount is explicit).
//   - When vnumaNodeCount > 0 and coresPerSocket > 0 and size.cpu is set:
//     size.cpu must be evenly divisible by vnumaNodeCount; the derived
//     coresPerNumaNode (size.cpu / vnumaNodeCount) must be a multiple or
//     divisor of coresPerSocket.
func validateComputeTopology(specPath *field.Path, vm *vmopv1.VirtualMachine) field.ErrorList {
	cpuAdv := vm.Spec.CPUAdvanced
	if cpuAdv == nil || cpuAdv.Topology == nil {
		return nil
	}

	var allErrs field.ErrorList
	t := cpuAdv.Topology
	topoPath := specPath.Child("cpuAdvanced").Child("topology")

	if t.VNUMANodeCount != nil && *t.VNUMANodeCount > 0 &&
		(t.CoresPerSocket == nil || *t.CoresPerSocket == 0) {
		allErrs = append(allErrs, field.Invalid(
			topoPath.Child("vnumaNodeCount"), *t.VNUMANodeCount,
			"requires coresPerSocket to be set to an explicit (non-zero) value"))
	}

	if t.VNUMANodeCount != nil && *t.VNUMANodeCount > 0 &&
		t.CoresPerSocket != nil && *t.CoresPerSocket > 0 {
		resources := vm.Spec.Resources
		if resources != nil && resources.Size != nil && resources.Size.CPU != nil {
			numCPU := resources.Size.CPU.Value()
			vnumaNodes := int64(*t.VNUMANodeCount)
			coresPerSocket := int64(*t.CoresPerSocket)

			if numCPU%vnumaNodes != 0 {
				allErrs = append(allErrs, field.Invalid(
					topoPath.Child("vnumaNodeCount"), *t.VNUMANodeCount,
					"size.cpu must be evenly divisible by vnumaNodeCount"))
			} else {
				coresPerNode := numCPU / vnumaNodes
				if coresPerNode%coresPerSocket != 0 && coresPerSocket%coresPerNode != 0 {
					allErrs = append(allErrs, field.Invalid(
						topoPath.Child("vnumaNodeCount"), *t.VNUMANodeCount,
						"derived coresPerNumaNode must be a multiple or divisor of coresPerSocket"))
				}
			}
		}
	}

	return allErrs
}
