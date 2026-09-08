// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package kubevm holds the linkage and down-mapping helpers shared between
// VM Operator's mutating webhook and its controllers/virtualmachine/kubevmlink
// reconciler, both of which resolve a VM Operator VirtualMachine's
// configuration from an owning kube-vm.io VirtualMachine.
package kubevm

import (
	"fmt"

	kubevmv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

// AnnotationKey is the annotation a VM Operator VirtualMachine carries to
// name the kube-vm.io VirtualMachine that adopted it as its infrastructure
// object. Its value is the generic object's name, in the same namespace.
const AnnotationKey = "kube-vm.io/virtual-machine"

// GenericObjectName returns the name of the generic VirtualMachine the given
// provider object's annotation names, and whether the annotation is set.
func GenericObjectName(vm *vmopv1.VirtualMachine) (string, bool) {
	name, ok := vm.Annotations[AnnotationKey]
	return name, ok && name != ""
}

// IsMutuallyLinked reports whether the provider object's annotation and the
// generic object's spec.infrastructureRef mutually name each other in the
// same namespace. Adoption must never proceed on anything less than a
// mutual, two-sided declaration.
func IsMutuallyLinked(vm *vmopv1.VirtualMachine, generic *kubevmv1a1.VirtualMachine) bool {
	if vm == nil || generic == nil {
		return false
	}

	name, ok := GenericObjectName(vm)
	if !ok || name != generic.Name || vm.Namespace != generic.Namespace {
		return false
	}

	ref := generic.Spec.InfrastructureRef
	return ref.APIGroup == vmopv1.GroupName &&
		ref.Kind == "VirtualMachine" &&
		ref.Name == vm.Name
}

// ConflictingOwner returns a non-nil error when the provider object already
// carries a controller owner reference naming a generic VirtualMachine other
// than the one given. A provider object may be adopted by at most one
// generic object.
func ConflictingOwner(vm *vmopv1.VirtualMachine, generic *kubevmv1a1.VirtualMachine) error {
	for _, ref := range vm.GetOwnerReferences() {
		if ref.Controller == nil || !*ref.Controller {
			continue
		}
		if ref.APIVersion != kubevmv1a1.GroupVersion.String() || ref.Kind != "VirtualMachine" {
			continue
		}
		if ref.Name != generic.Name {
			return fmt.Errorf(
				"provider object %s/%s is already controlled by generic VirtualMachine %q",
				vm.Namespace, vm.Name, ref.Name)
		}
	}
	return nil
}
