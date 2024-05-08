// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"encoding/json"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
)

const (
	// LastResizedAnnotationKey denotes the VM Class that the VM was last resized from.
	LastResizedAnnotationKey = "vmoperator.vmware.com/last-resized-vm-class"
)

type lastResizedAnnotation struct {
	Name       string
	UID        string
	Generation int64
}

// SetResizeAnnotation sets the resize annotation to match the given class.
func SetResizeAnnotation(
	vm *vmopv1.VirtualMachine,
	vmClass vmopv1.VirtualMachineClass) error {

	b, err := json.Marshal(lastResizedAnnotation{
		Name:       vmClass.Name,
		UID:        string(vmClass.UID),
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

// GetResizeAnnotation returns the VM Class Name, UID, Generation, and true from the last resize annotation
// if present. Otherwise returns false.
func GetResizeAnnotation(vm vmopv1.VirtualMachine) (name, uid string, generation int64, ok bool) {
	val, exists := vm.Annotations[LastResizedAnnotationKey]
	if !exists {
		return
	}

	var v lastResizedAnnotation
	if err := json.Unmarshal([]byte(val), &v); err != nil {
		return
	}

	return v.Name, v.UID, v.Generation, true
}
