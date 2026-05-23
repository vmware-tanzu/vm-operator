// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/validation/field"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

// validateComputeConfig validates spec.resources, spec.cpuAdvanced, and
// spec.memoryAdvanced. Called from both ValidateCreate and ValidateUpdate.
func (v validator) validateComputeConfig(
	ctx *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	if !pkgcfg.FromContext(ctx).Features.TelcoVMServiceAPI {
		if vm.Spec.Resources != nil {
			allErrs = append(allErrs, field.Forbidden(
				specPath.Child("resources"),
				"requires TelcoVMServiceAPI supervisor capability"))
		}
		if vm.Spec.CPUAdvanced != nil {
			allErrs = append(allErrs, field.Forbidden(
				specPath.Child("cpuAdvanced"),
				"requires TelcoVMServiceAPI supervisor capability"))
		}
		if vm.Spec.MemoryAdvanced != nil {
			allErrs = append(allErrs, field.Forbidden(
				specPath.Child("memoryAdvanced"),
				"requires TelcoVMServiceAPI supervisor capability"))
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
	r := vm.Spec.Resources
	if r == nil {
		return nil
	}

	var allErrs field.ErrorList
	resPath := specPath.Child("resources")
	sizePath := resPath.Child("size")
	reqPath := resPath.Child("requests")
	limPath := resPath.Child("limits")

	if s := r.Size; s != nil {
		if s.CPU != nil && s.CPU.IsZero() {
			allErrs = append(allErrs, field.Invalid(
				sizePath.Child("cpu"), s.CPU.String(),
				"must be greater than 0 when set"))
		}
		if s.Memory != nil && s.Memory.IsZero() {
			allErrs = append(allErrs, field.Invalid(
				sizePath.Child("memory"), s.Memory.String(),
				"must be greater than 0 when set"))
		}
	}

	if l := r.Limits; l != nil {
		if l.CPU != nil && l.CPU.IsZero() {
			allErrs = append(allErrs, field.Invalid(
				limPath.Child("cpu"), l.CPU.String(),
				"must be greater than 0 when set (nil = unlimited)"))
		}
		if l.Memory != nil && l.Memory.IsZero() {
			allErrs = append(allErrs, field.Invalid(
				limPath.Child("memory"), l.Memory.String(),
				"must be greater than 0 when set (nil = unlimited)"))
		}
	}

	// requests.cpu ≤ limits.cpu
	if r.Requests != nil && r.Limits != nil &&
		r.Requests.CPU != nil && r.Limits.CPU != nil {
		if r.Requests.CPU.Cmp(*r.Limits.CPU) > 0 {
			allErrs = append(allErrs, field.Invalid(
				reqPath.Child("cpu"), r.Requests.CPU.String(),
				"must be less than or equal to limits.cpu"))
		}
	}

	// requests.memory ≤ size.memory
	if r.Requests != nil && r.Size != nil &&
		r.Requests.Memory != nil && r.Size.Memory != nil {
		if r.Requests.Memory.Cmp(*r.Size.Memory) > 0 {
			allErrs = append(allErrs, field.Invalid(
				reqPath.Child("memory"), r.Requests.Memory.String(),
				"must be less than or equal to size.memory"))
		}
	}

	// size.memory ≤ limits.memory
	if r.Size != nil && r.Limits != nil &&
		r.Size.Memory != nil && r.Limits.Memory != nil {
		if r.Size.Memory.Cmp(*r.Limits.Memory) > 0 {
			allErrs = append(allErrs, field.Invalid(
				sizePath.Child("memory"), r.Size.Memory.String(),
				"must be less than or equal to limits.memory"))
		}
	}

	return allErrs
}

// validateComputeLatencySensitivity enforces the full memory reservation
// requirement when latencySensitivity is High or HighWithHyperthreading.
//
// The reservation invariant is satisfied when either:
//   - requests.memory != nil AND size.memory != nil AND requests.memory == size.memory
//   - OR memoryAdvanced.reservationLockedToMax == true
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

	if vm.Spec.MemoryAdvanced != nil &&
		vm.Spec.MemoryAdvanced.ReservationLockedToMax != nil &&
		*vm.Spec.MemoryAdvanced.ReservationLockedToMax {
		return nil // reservationLockedToMax satisfies the invariant
	}

	memReservationMet := false
	if r := vm.Spec.Resources; r != nil {
		if r.Requests != nil && r.Size != nil &&
			r.Requests.Memory != nil && r.Size.Memory != nil &&
			r.Requests.Memory.Cmp(*r.Size.Memory) == 0 {
			memReservationMet = true
		}
	}

	if !memReservationMet {
		return field.ErrorList{field.Invalid(
			specPath.Child("cpuAdvanced").Child("latencySensitivity"),
			string(ls),
			"requires full memory reservation: "+
				"spec.resources.requests.memory must equal spec.resources.size.memory, "+
				"or spec.memoryAdvanced.reservationLockedToMax must be true")}
	}

	return nil
}

// validateComputeReservationLockedToMax validates mutual exclusion rules:
//   - cpuAdvanced.reservationLockedToMax=true is mutually exclusive with requests.cpu
//   - memoryAdvanced.reservationLockedToMax=true is mutually exclusive with requests.memory
func validateComputeReservationLockedToMax(specPath *field.Path, vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	if vm.Spec.CPUAdvanced != nil &&
		vm.Spec.CPUAdvanced.ReservationLockedToMax != nil &&
		*vm.Spec.CPUAdvanced.ReservationLockedToMax {

		if vm.Spec.Resources != nil &&
			vm.Spec.Resources.Requests != nil &&
			vm.Spec.Resources.Requests.CPU != nil {
			allErrs = append(allErrs, field.Invalid(
				specPath.Child("cpuAdvanced").Child("reservationLockedToMax"),
				true,
				"mutually exclusive with spec.resources.requests.cpu"))
		}
	}

	if vm.Spec.MemoryAdvanced != nil &&
		vm.Spec.MemoryAdvanced.ReservationLockedToMax != nil &&
		*vm.Spec.MemoryAdvanced.ReservationLockedToMax {

		if vm.Spec.Resources != nil &&
			vm.Spec.Resources.Requests != nil &&
			vm.Spec.Resources.Requests.Memory != nil {
			allErrs = append(allErrs, field.Invalid(
				specPath.Child("memoryAdvanced").Child("reservationLockedToMax"),
				true,
				"mutually exclusive with spec.resources.requests.memory"))
		}
	}

	return allErrs
}

// validateComputeTopology validates spec.cpuAdvanced.topology constraints:
//   - When vnumaNodeCount is set together with coresPerSocket and size.cpu:
//     size.cpu must be evenly divisible by vnumaNodeCount; the derived
//     coresPerNumaNode (size.cpu / vnumaNodeCount) must be a multiple or
//     divisor of coresPerSocket.
//   - exposeVnumaOnCpuHotadd=true requires hotAddEnabled=true.
//   - vnumaNodeCount set with hotAddEnabled=true requires exposeVnumaOnCpuHotadd=true.
func validateComputeTopology(specPath *field.Path, vm *vmopv1.VirtualMachine) field.ErrorList {
	cpuAdv := vm.Spec.CPUAdvanced
	if cpuAdv == nil || cpuAdv.Topology == nil {
		return nil
	}

	var allErrs field.ErrorList
	t := cpuAdv.Topology
	topoPath := specPath.Child("cpuAdvanced").Child("topology")

	if t.VNUMANodeCount != nil && t.CoresPerSocket == nil {
		allErrs = append(allErrs, field.Invalid(
			topoPath.Child("vnumaNodeCount"), *t.VNUMANodeCount,
			"requires coresPerSocket to be set"))
	}

	if t.VNUMANodeCount != nil && t.CoresPerSocket != nil {
		if vm.Spec.Resources != nil && vm.Spec.Resources.Size != nil &&
			vm.Spec.Resources.Size.CPU != nil {

			numCPU := vm.Spec.Resources.Size.CPU.Value()
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

	if t.ExposeVNUMAOnCPUHotAdd != nil && *t.ExposeVNUMAOnCPUHotAdd {
		if cpuAdv.HotAddEnabled == nil || !*cpuAdv.HotAddEnabled {
			allErrs = append(allErrs, field.Invalid(
				topoPath.Child("exposeVnumaOnCpuHotadd"), true,
				"requires cpuAdvanced.hotAddEnabled to be true"))
		}
	}

	if t.VNUMANodeCount != nil &&
		cpuAdv.HotAddEnabled != nil && *cpuAdv.HotAddEnabled &&
		(t.ExposeVNUMAOnCPUHotAdd == nil || !*t.ExposeVNUMAOnCPUHotAdd) {
		allErrs = append(allErrs, field.Invalid(
			topoPath.Child("vnumaNodeCount"), *t.VNUMANodeCount,
			"requires cpuAdvanced.topology.exposeVnumaOnCpuHotadd=true when cpuAdvanced.hotAddEnabled=true"))
	}

	return allErrs
}
