/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vmoperator

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/klogr"
)

const VirtualMachineFinalizer string = "virtualmachine.vmoperator.vmware.com"

func (v VirtualMachineStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	// Invoke the parent implementation to strip the Status
	v.DefaultStorageStrategy.PrepareForCreate(ctx, obj)

	o := obj.(*VirtualMachine)

	// Add a finalizer so that our controllers can process deletion
	finalizers := append(o.GetFinalizers(), VirtualMachineFinalizer)
	o.SetFinalizers(finalizers)
}

// Validate checks that an instance of VirtualMachine is well formed
func (v VirtualMachineStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	vm := obj.(*VirtualMachine)

	log := klogr.New()
	log.V(4).Info("Validating fields for VirtualMachine", "namespace", vm.Namespace, "name", vm.Name)
	errors := field.ErrorList{}

	if vm.Spec.ImageName == "" {
		errors = append(errors, field.Required(field.NewPath("spec", "imageName"), ""))
	}

	if vm.Spec.ClassName == "" {
		errors = append(errors, field.Required(field.NewPath("spec", "className"), ""))
	}

	return errors
}
