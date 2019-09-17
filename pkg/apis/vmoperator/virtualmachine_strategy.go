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

const (
	VirtualMachineFinalizer = "virtualmachine.vmoperator.vmware.com"
	NsxtNetworkType         = "nsx-t"
)

func (v VirtualMachineStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	// Invoke the parent implementation to strip the Status
	v.DefaultStorageStrategy.PrepareForCreate(ctx, obj)

	o := obj.(*VirtualMachine)

	// Add a finalizer so that our controllers can process deletion
	finalizers := append(o.GetFinalizers(), VirtualMachineFinalizer)
	o.SetFinalizers(finalizers)
}

func validateNetworkType(vm *VirtualMachine) field.ErrorList {
	errors := field.ErrorList{}
	nifPath := field.NewPath("spec", "networkInterfaces")
	for i, nif := range vm.Spec.NetworkInterfaces {
		if nif.NetworkName == "" {
			errors = append(errors, field.Required(nifPath.Index(i), nif.NetworkName))
		}
		switch nif.NetworkType {
		case NsxtNetworkType,
			"":
		default:
			errors = append(errors, field.NotSupported(nifPath.Index(i), nif.NetworkType, []string{NsxtNetworkType, ""}))
		}
	}
	return errors
}

// Validate checks that an instance of VirtualMachine is well formed
func (v VirtualMachineStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	vm := obj.(*VirtualMachine)

	log := klogr.New()
	log.V(4).Info("Validating fields for VirtualMachine", "namespace", vm.Namespace, "name", vm.Name)
	errors := field.ErrorList{}

	if vm.Spec.ImageName == "" {
		errors = append(errors, field.Required(field.NewPath("spec", "imageName"), "imageName must be provided"))
	}

	if vm.Spec.ClassName == "" {
		errors = append(errors, field.Required(field.NewPath("spec", "className"), "className must be provided"))
	}

	networkTypeErrors := validateNetworkType(vm)
	errors = append(errors, networkTypeErrors...)

	return errors
}
