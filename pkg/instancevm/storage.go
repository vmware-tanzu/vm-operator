// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package instancevm

import vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

// IsInstanceStorageConfigured checks if VM spec has an annotation to identifying if VM is configured with instance storage
// and returns true/false accordingly.
func IsInstanceStorageConfigured(vm *vmopv1alpha1.VirtualMachine) bool {
	_, exists := vm.Annotations[PVCsAnnotationKey]
	return exists
}
