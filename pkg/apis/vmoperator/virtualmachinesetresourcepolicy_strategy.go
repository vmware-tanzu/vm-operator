// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmoperator

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	VirtualMachineSetResourcePolicyFinalizer = "virtualmachinesetresourcepolicy.vmoperator.vmware.com"
)

// copied from virtualmachine_strategy.go
func (v VirtualMachineSetResourcePolicyStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	// Invoke the parent implementation to strip the Status
	v.DefaultStorageStrategy.PrepareForCreate(ctx, obj)

	o := obj.(*VirtualMachineSetResourcePolicy)

	// Add a finalizer so that our controllers can process deletion
	finalizers := append(o.GetFinalizers(), VirtualMachineSetResourcePolicyFinalizer)
	o.SetFinalizers(finalizers)
}

// validateResourcePoolMemory validates the memory reservation specified for an RP
func validateResourcePoolMemory(resourcePolicy VirtualMachineSetResourcePolicy) field.ErrorList {
	errors := field.ErrorList{}

	// check the reservation against limits
	if !isMemoryRequestValid(resourcePolicy.Spec.ResourcePool.Reservations, resourcePolicy.Spec.ResourcePool.Limits) {
		errors = append(errors,
			field.Invalid(
				field.NewPath("spec", "resourcepool", "reservations", "memory"),
				resourcePolicy.Spec.ResourcePool.Reservations.Memory.Value(), "Memory reservation should not be larger than Memory limit"))
	}

	//TODO: Validate req and limit against hardware configuration of all the VMs in the resource pool

	return errors
}

// validateResourcePoolCPU validates the CPU reservation specified for an RP
func validateResourcePoolCPU(resourcePolicy VirtualMachineSetResourcePolicy) field.ErrorList {
	errors := field.ErrorList{}

	// check the reservation against limits
	if !isCPURequestValid(resourcePolicy.Spec.ResourcePool.Reservations, resourcePolicy.Spec.ResourcePool.Limits) {
		errors = append(errors,
			field.Invalid(
				field.NewPath("spec", "resourcepool", "reservations", "cpu"),
				resourcePolicy.Spec.ResourcePool.Reservations.Cpu.Value(), "CPU reservation should not be larger than CPU limit"))
	}

	//TODO: Validate req and limit against hardware configuration of all the VMs in the resource pool

	return errors
}

// Validate checks if an instance of VirtualMachineSetResourcePolicy is well formed
func (VirtualMachineSetResourcePolicyStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	o := obj.(*VirtualMachineSetResourcePolicy)

	log := logf.Log.WithName("virtual-machine-set-resource-policy-strategy")
	log.V(4).Info("Validating fields for VirtualMachineSetResourcePolicy", "namespace", o.Namespace, "name", o.Name)

	memErrors := validateResourcePoolMemory(*o)
	cpuErrors := validateResourcePoolCPU(*o)

	return append(memErrors, cpuErrors...)
}
