// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmoperator

import (
	"context"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const VMClassImmutable = "VM Classes are immutable"

// VirtualMachineClasses are cluster-scoped
func (VirtualMachineClassStrategy) NamespaceScoped() bool {
	return false
}

// VirtualMachineClassesStatus are cluster-scoped
func (VirtualMachineClassStatusStrategy) NamespaceScoped() bool {
	return false
}

// isMemoryRequestValid validates memory reservation against specified limit
func isMemoryRequestValid(request, limit VirtualMachineResourceSpec) bool {
	reqExists := !request.Memory.IsZero()
	limitExists := !limit.Memory.IsZero()

	if reqExists && limitExists && request.Memory.Value() > limit.Memory.Value() {
		return false
	}

	return true
}

// isCPURequestValid validates CPU reservation against specified limit
func isCPURequestValid(request, limit VirtualMachineResourceSpec) bool {
	reqExists := !request.Cpu.IsZero()
	limitExists := !limit.Cpu.IsZero()

	if reqExists && limitExists && request.Cpu.Value() > limit.Cpu.Value() {
		return false
	}

	return true
}

// validateMemory validates the memory reservation specified
func validateMemory(vmclass VirtualMachineClass) field.ErrorList {
	errors := field.ErrorList{}

	// check the reservation against limits
	if !isMemoryRequestValid(vmclass.Spec.Policies.Resources.Requests, vmclass.Spec.Policies.Resources.Limits) {
		errors = append(errors,
			field.Invalid(
				field.NewPath("spec", "policies", "resources", "requests", "memory"),
				vmclass.Spec.Policies.Resources.Requests.Memory.Value(), "Memory request should not be larger than Memory limit"))
	}

	//TODO: Validate req and limit against hardware configuration of the class

	return errors
}

// validateCPU validates the CPU reservation specified
func validateCPU(vmclass VirtualMachineClass) field.ErrorList {
	errors := field.ErrorList{}

	// check the reservation against limits
	if !isCPURequestValid(vmclass.Spec.Policies.Resources.Requests, vmclass.Spec.Policies.Resources.Limits) {
		errors = append(errors,
			field.Invalid(
				field.NewPath("spec", "policies", "resources", "requests", "cpu"),
				vmclass.Spec.Policies.Resources.Requests.Cpu.Value(), "CPU request should not be larger than CPU limit"))
	}

	//TODO: Validate req and limit against hardware configuration of the class

	return errors
}

func validateClassUpdate(new, old VirtualMachineClass) field.ErrorList {
	errors := field.ErrorList{}
	if !reflect.DeepEqual(new.Spec, old.Spec) {
		errors = append(errors, field.Forbidden(field.NewPath("spec"), VMClassImmutable))
	}

	return errors
}

//ValidateUpdate makes sure the VM Classes are immutable.
//NOTE: This is for 1.0 and will change in the future.
func (VirtualMachineClassStrategy) ValidateUpdate(ctx context.Context, obj, old runtime.Object) field.ErrorList {
	newVmClass := obj.(*VirtualMachineClass)
	oldVmClass := old.(*VirtualMachineClass)
	log := logf.Log.WithName("virtual-machine-class-strategy")
	log.V(4).Info("Validating Update for VirtualMachineClass", "namespace", newVmClass.Namespace, "name", newVmClass.Name)

	return validateClassUpdate(*newVmClass, *oldVmClass)
}

// Validate checks that an instance of VirtualMachineClass is well formed
func (VirtualMachineClassStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	vmClass := obj.(*VirtualMachineClass)

	log := logf.Log.WithName("virtual-machine-class-strategy")
	log.V(4).Info("Validating fields for VirtualMachineClass", "namespace", vmClass.Namespace, "name", vmClass.Name)

	memErrors := validateMemory(*vmClass)
	cpuErrors := validateCPU(*vmClass)

	return append(memErrors, cpuErrors...)
}
