// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmoperator

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
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

func validateVolumes(vm *VirtualMachine) field.ErrorList {
	errors := field.ErrorList{}
	volumesPath := field.NewPath("spec", "volumes")
	volumeNames := map[string]bool{}
	for i, volume := range vm.Spec.Volumes {
		if volume.Name == "" {
			errors = append(errors, field.Required(volumesPath.Index(i).Child("name"), ""))
		}
		if volume.PersistentVolumeClaim == nil || volume.PersistentVolumeClaim.ClaimName == "" {
			errors = append(errors, field.Required(volumesPath.Index(i).Child("persistentVolumeClaim", "claimName"), ""))
		}
		if volumeNames[volume.Name] {
			errors = append(errors, field.Duplicate(volumesPath.Index(i).Child("name"), volume.Name))
		}
		volumeNames[volume.Name] = true
	}
	return errors
}

// Validate checks that an instance of VirtualMachine is well formed
func (v VirtualMachineStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	vm := obj.(*VirtualMachine)

	log := logf.Log.WithName("virtual-machine-strategy")
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

	volumesErrors := validateVolumes(vm)
	errors = append(errors, volumesErrors...)

	return errors
}
