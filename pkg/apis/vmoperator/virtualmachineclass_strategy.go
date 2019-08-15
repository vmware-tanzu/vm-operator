/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vmoperator

import (
	"context"

	"k8s.io/klog"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

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

// Validate checks that an instance of VirtualMachineClass is well formed
func (VirtualMachineClassStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	vmClass := obj.(*VirtualMachineClass)
	klog.V(4).Infof("Validating fields for VirtualMachineClass %s/%s", vmClass.Namespace, vmClass.Name)

	memErrors := validateMemory(*vmClass)
	cpuErrors := validateCPU(*vmClass)

	return append(memErrors, cpuErrors...)
}
