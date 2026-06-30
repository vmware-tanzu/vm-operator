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
	allErrs = append(allErrs, validateUPTv2MemoryReservation(specPath, vm)...)

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
		if size.CPU != nil && size.CPU.IsZero() {
			allErrs = append(allErrs, field.Invalid(
				sizePath.Child("cpu"), size.CPU.String(),
				"must be greater than 0 when set"))
		}
		if size.Memory != nil && size.Memory.IsZero() {
			allErrs = append(allErrs, field.Invalid(
				sizePath.Child("memory"), size.Memory.String(),
				"must be greater than 0 when set"))
		}
	}

	// limits.{cpu,memory} > 0 when set (nil = unlimited)
	if lim != nil {
		if lim.CPU != nil && lim.CPU.IsZero() {
			allErrs = append(allErrs, field.Invalid(
				limPath.Child("cpu"), lim.CPU.String(),
				"must be greater than 0 when set (nil = unlimited)"))
		}
		if lim.Memory != nil && lim.Memory.IsZero() {
			allErrs = append(allErrs, field.Invalid(
				limPath.Child("memory"), lim.Memory.String(),
				"must be greater than 0 when set (nil = unlimited)"))
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
// CPU reservation invariant is satisfied when either:
//   - cpuAdvanced.reservationLockedToMax == true
//   - OR requests.cpu is set and > 0
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
		allErrs   field.ErrorList
		resources = vm.Spec.Resources
		lsPath    = specPath.Child("cpuAdvanced").Child("latencySensitivity")
	)

	if !vmopv1util.FullMemReservationSpecMet(vm) {
		allErrs = append(allErrs, field.Invalid(lsPath, string(ls),
			"requires full memory reservation: "+
				"spec.resources.requests.memory must equal spec.resources.size.memory, "+
				"or spec.memoryAdvanced.reservationLockedToMax must be true"))
	}

	// Full CPU reservation: reservationLockedToMax or requests.cpu > 0.
	// The webhook cannot verify the exact MHz value equals 100% (that depends
	// on the host's CPU speed at placement time), so we only check that a
	// non-zero reservation is present.
	cpuReservationMet := ptr.Deref(cpuAdv.ReservationLockedToMax) ||
		(resources != nil &&
			resources.Requests != nil &&
			resources.Requests.CPU != nil &&
			!resources.Requests.CPU.IsZero())
	if !cpuReservationMet {
		allErrs = append(allErrs, field.Invalid(lsPath, string(ls),
			"High Latency Sensitivity requires you to set 100% CPU reservation for this VM"))
	}

	return allErrs
}

// validateComputeReservationLockedToMax validates mutual exclusion rules:
//   - cpuAdvanced.reservationLockedToMax=true is mutually exclusive with requests.cpu
//   - memoryAdvanced.reservationLockedToMax=true is compatible with requests.memory only
//     when size.memory is also set and requests.memory equals size.memory, because
//     vSphere locks the reservation to the full size in that mode. When size.memory
//     is absent the comparison is skipped (no reference value to check against).
func validateComputeReservationLockedToMax(specPath *field.Path, vm *vmopv1.VirtualMachine) field.ErrorList {
	var (
		allErrs   field.ErrorList
		resources = vm.Spec.Resources
		cpuAdv    = vm.Spec.CPUAdvanced
		memAdv    = vm.Spec.MemoryAdvanced
	)

	if cpuAdv != nil && ptr.Deref(cpuAdv.ReservationLockedToMax) {
		if resources != nil && resources.Requests != nil && resources.Requests.CPU != nil {
			allErrs = append(allErrs, field.Invalid(
				specPath.Child("cpuAdvanced").Child("reservationLockedToMax"),
				true,
				"mutually exclusive with spec.resources.requests.cpu"))
		}
	}

	if memAdv != nil && ptr.Deref(memAdv.ReservationLockedToMax) {
		if resources != nil && resources.Requests != nil && resources.Requests.Memory != nil {
			// Only validate when size.memory is present; without it there is no
			// reference value to compare the reservation against.
			if resources.Size != nil && resources.Size.Memory != nil &&
				resources.Requests.Memory.Cmp(*resources.Size.Memory) != 0 {
				allErrs = append(allErrs, field.Invalid(
					specPath.Child("memoryAdvanced").Child("reservationLockedToMax"),
					true,
					"mutually exclusive with spec.resources.requests.memory "+
						"unless requests.memory equals size.memory"))
			}
		}
	}

	return allErrs
}

// validateUPTv2MemoryReservation rejects any interface spec that sets
// vmxnet3.uptv2Enabled=true without ensuring full guest memory reservation.
// vSphere requires full memory reservation when UPTv2 is enabled.
//
// Full reservation is satisfied when either:
//   - spec.memoryAdvanced.reservationLockedToMax == true
//   - spec.resources.requests.memory equals spec.resources.size.memory
func validateUPTv2MemoryReservation(specPath *field.Path, vm *vmopv1.VirtualMachine) field.ErrorList {
	if vmopv1util.FullMemReservationSpecMet(vm) {
		return nil
	}
	if vm.Spec.Network == nil {
		return nil
	}

	const msg = "requires full guest memory reservation: " +
		"set spec.memoryAdvanced.reservationLockedToMax=true, " +
		"or set spec.resources.requests.memory equal to spec.resources.size.memory"

	var allErrs field.ErrorList
	ifacesPath := specPath.Child("network", "interfaces")
	for i, iface := range vm.Spec.Network.Interfaces {
		if iface.VMXNet3 != nil && ptr.Deref(iface.VMXNet3.UPTv2Enabled) {
			allErrs = append(allErrs, field.Invalid(
				ifacesPath.Index(i).Child("vmxnet3", "uptv2Enabled"), true, msg))
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

	if ptr.Deref(t.ExposeVNUMAOnCPUHotAdd) {
		if !ptr.Deref(cpuAdv.HotAddEnabled) {
			allErrs = append(allErrs, field.Invalid(
				topoPath.Child("exposeVnumaOnCpuHotadd"), true,
				"requires cpuAdvanced.hotAddEnabled to be true"))
		}
	}

	if t.VNUMANodeCount != nil &&
		ptr.Deref(cpuAdv.HotAddEnabled) &&
		!ptr.Deref(t.ExposeVNUMAOnCPUHotAdd) {
		allErrs = append(allErrs, field.Invalid(
			topoPath.Child("vnumaNodeCount"), *t.VNUMANodeCount,
			"requires cpuAdvanced.topology.exposeVnumaOnCpuHotadd=true when cpuAdvanced.hotAddEnabled=true"))
	}

	return allErrs
}
