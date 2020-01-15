// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmoperator

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const VirtualMachineServiceFinalizer string = "virtualmachineservice.vmoperator.vmware.com"

func (v VirtualMachineServiceStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	// Invoke the parent implementation to strip the Status
	v.DefaultStorageStrategy.PrepareForCreate(ctx, obj)

	o := obj.(*VirtualMachineService)

	// Add a finalizer so that our controllers can process deletion
	finalizers := append(o.GetFinalizers(), VirtualMachineServiceFinalizer)
	o.SetFinalizers(finalizers)
}

// Validate checks that an instance of VirtualMachineService is well formed
func (VirtualMachineServiceStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	service := obj.(*VirtualMachineService)

	log := logf.Log.WithName("virtual-machine-service-strategy")
	log.V(4).Info("Validating fields for VirtualMachineService", "namespace", service.Namespace, "name", service.Name)
	errors := field.ErrorList{}

	if service.Spec.Type == "" {
		errors = append(errors, field.Required(field.NewPath("spec", "type"), ""))
	}

	if len(service.Spec.Ports) == 0 {
		errors = append(errors, field.Required(field.NewPath("spec", "ports"), ""))
	}

	if len(service.Spec.Selector) == 0 {
		errors = append(errors, field.Required(field.NewPath("spec", "selector"), ""))
	}

	return errors
}
