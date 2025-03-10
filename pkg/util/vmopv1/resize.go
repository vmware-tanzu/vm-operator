// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
)

const (
	// LastResizedAnnotationKey denotes the VM Class that the VM was last resized from.
	LastResizedAnnotationKey = vmopv1.GroupName + "/last-resized-vm-class"
)

type lastResizedAnnotation struct {
	Name       string    `json:"name"`
	UID        types.UID `json:"uid,omitempty"`
	Generation int64     `json:"generation,omitempty"`
}

// SetLastResizedAnnotation sets the resize annotation to match the given class.
func SetLastResizedAnnotation(
	vm *vmopv1.VirtualMachine,
	vmClass vmopv1.VirtualMachineClass) error {

	b, err := json.Marshal(lastResizedAnnotation{
		Name:       vmClass.Name,
		UID:        vmClass.UID,
		Generation: vmClass.Generation,
	})
	if err != nil {
		return err
	}

	if vm.Annotations == nil {
		vm.Annotations = make(map[string]string)
	}
	vm.Annotations[LastResizedAnnotationKey] = string(b)

	return nil
}

// MustSetLastResizedAnnotation sets the resize annotation to match the given class.
// Panic if fails.
func MustSetLastResizedAnnotation(
	vm *vmopv1.VirtualMachine,
	vmClass vmopv1.VirtualMachineClass) {

	err := SetLastResizedAnnotation(vm, vmClass)
	if err != nil {
		panic(err)
	}
}

// SetLastResizedAnnotationClassName sets the resize annotation to just of the class name.
// This is called from the VM mutation wehbook to record the prior class name of a VM
// that does not already have the annotation (so that ResizeNeeded() will return true).
// Note className may be empty if the VM was classless.
func SetLastResizedAnnotationClassName(
	vm *vmopv1.VirtualMachine,
	className string) error {

	b, err := json.Marshal(lastResizedAnnotation{
		Name: className,
	})
	if err != nil {
		return err
	}

	if vm.Annotations == nil {
		vm.Annotations = make(map[string]string)
	}
	vm.Annotations[LastResizedAnnotationKey] = string(b)

	return nil
}

// GetLastResizedAnnotation returns the VM Class Name, UID, Generation, and true from the
// last resize annotation if present. Otherwise returns false.
func GetLastResizedAnnotation(vm vmopv1.VirtualMachine) (string, string, int64, bool) {
	lra, ok := getLastResizeAnnotation(vm)
	if !ok {
		return "", "", 0, false
	}

	return lra.Name, string(lra.UID), lra.Generation, true
}

func getLastResizeAnnotation(vm vmopv1.VirtualMachine) (lastResizedAnnotation, bool) {
	val, ok := vm.Annotations[LastResizedAnnotationKey]
	if !ok {
		return lastResizedAnnotation{}, false
	}

	var lra lastResizedAnnotation
	if err := json.Unmarshal([]byte(val), &lra); err != nil {
		return lastResizedAnnotation{}, false
	}

	return lra, true
}

func ResizeNeeded(
	vm vmopv1.VirtualMachine,
	vmClass vmopv1.VirtualMachineClass) bool {

	lra, exists := getLastResizeAnnotation(vm)
	if !exists {
		// The VM does not have the last resized annotation. Most likely this is an existing VM
		// and the VM Spec.ClassName hasn't been changed (because the mutation webhook sets the
		// annotation when it changes). However, if the same class opt-in annotation is set, do
		// a resize to sync this VM to its class.
		_, ok := vm.Annotations[vmopv1.VirtualMachineSameVMClassResizeAnnotation]
		return ok
	}

	if vm.Spec.ClassName != lra.Name {
		// Usual resize case: the VM Spec.ClassName has been changed so need to resize.
		return true
	}

	// Resizing as the class itself changes is only performed with an opt-in annotation
	// because a change in the class could break existing VMs.
	if _, ok := vm.Annotations[vmopv1.VirtualMachineSameVMClassResizeAnnotation]; ok {
		same := vmClass.UID == lra.UID && vmClass.Generation == lra.Generation
		return !same
	}

	return false
}
