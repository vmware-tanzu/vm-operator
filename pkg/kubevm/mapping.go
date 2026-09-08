// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kubevm

import (
	kubevmv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

// ApplyDelegatedFields copies the generic object's four delegated spec
// values onto the provider object, filling only fields that are currently
// empty so a value the user set directly on the provider object is never
// overwritten.
func ApplyDelegatedFields(vm *vmopv1.VirtualMachine, generic *kubevmv1a1.VirtualMachine) {
	spec := generic.Spec

	if vm.Spec.ClassName == "" {
		if it := spec.InstanceType; it != nil {
			vm.Spec.ClassName = it.Name
		}
	}

	if vm.Spec.Image == nil {
		if bd := spec.BootDisk; bd != nil && bd.Source.Image != nil {
			vm.Spec.Image = &vmopv1.VirtualMachineImageRef{
				Kind: bd.Source.Image.Kind,
				Name: bd.Source.Image.Name,
			}
		}
	}

	if vm.Spec.StorageClass == "" {
		if bd := spec.BootDisk; bd != nil {
			vm.Spec.StorageClass = bd.StorageClassName
		}
	}

	if vm.Spec.PowerState == "" {
		vm.Spec.PowerState = PowerStateFor(spec.PowerState)
	}
}

// PowerStateFor down-maps the generic object's desired power state onto
// VM Operator's own power state type. The enum values are spelled
// identically in both APIs, so this is a type conversion, not a translation.
func PowerStateFor(p kubevmv1a1.PowerState) vmopv1.VirtualMachinePowerState {
	return vmopv1.VirtualMachinePowerState(p)
}

// DelegatedFieldsUpToDate reports whether the generic object's instance
// type, boot disk image reference and storage class still match the values
// already persisted on the provider object. Once they diverge, the change
// cannot be applied — those provider fields are immutable after create —
// and the caller should report UpToDate=False with reason
// UnsupportedByProvider rather than treat it as an error to retry.
func DelegatedFieldsUpToDate(vm *vmopv1.VirtualMachine, generic *kubevmv1a1.VirtualMachine) bool {
	spec := generic.Spec

	if it := spec.InstanceType; it != nil && it.Name != vm.Spec.ClassName {
		return false
	}

	if bd := spec.BootDisk; bd != nil {
		if img := bd.Source.Image; img != nil {
			if vm.Spec.Image == nil ||
				vm.Spec.Image.Kind != img.Kind ||
				vm.Spec.Image.Name != img.Name {
				return false
			}
		}
		if bd.StorageClassName != "" && bd.StorageClassName != vm.Spec.StorageClass {
			return false
		}
	}

	return true
}
