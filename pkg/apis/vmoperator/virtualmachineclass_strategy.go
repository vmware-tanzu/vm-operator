/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vmoperator

import (
	"context"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/klogr"
)

const VMClassImmutable = "VM Classes are immutable"

func (VirtualMachineClassStrategy) NamespaceScoped() bool {
	return false
}

func (VirtualMachineClassStatusStrategy) NamespaceScoped() bool {
	return false
}

func validateMemory(vmClass VirtualMachineClass) field.ErrorList {
	resources := vmClass.Spec.Policies.Resources
	errors := field.ErrorList{}
	reqExists := !resources.Requests.Memory.IsZero()
	limitExists := !resources.Limits.Memory.IsZero()

	if reqExists && limitExists {
		if resources.Requests.Memory.Value() > resources.Limits.Memory.Value() {
			errors = append(errors,
				field.Invalid(
					field.NewPath("spec", "requests"),
					resources.Requests.Memory.Value(), "Memory Limits must not be smaller than Memory Requests"))
		}

		//TODO: Validate req and limit against hardware configuration of the class
	}

	return errors
}

func validateCPU(vmClass VirtualMachineClass) field.ErrorList {
	resources := vmClass.Spec.Policies.Resources
	errors := field.ErrorList{}
	reqExists := !resources.Requests.Cpu.IsZero()
	limitExists := !resources.Limits.Cpu.IsZero()

	if reqExists && limitExists {
		if resources.Requests.Cpu.Value() > resources.Limits.Cpu.Value() {
			errors = append(errors,
				field.Invalid(
					field.NewPath("spec", "requests"),
					resources.Requests.Cpu.Value(), "CPU Limits must not be smaller than CPU Requests"))
		}

		//TODO: Validate req and limit against hardware configuration of the class
	}

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
	log := klogr.New()
	log.V(4).Info("Validating Update for VirtualMachineClass", "namespace", newVmClass.Namespace, "name", newVmClass.Name)

	return validateClassUpdate(*newVmClass, *oldVmClass)
}

// Validate checks that an instance of VirtualMachineClass is well formed
func (VirtualMachineClassStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	vmClass := obj.(*VirtualMachineClass)

	log := klogr.New()
	log.V(4).Info("Validating fields for VirtualMachineClass", "namespace", vmClass.Namespace, "name", vmClass.Name)

	memErrors := validateMemory(*vmClass)
	cpuErrors := validateCPU(*vmClass)

	return append(memErrors, cpuErrors...)
}
