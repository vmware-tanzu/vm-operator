// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package instancestorage

import vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

// IsConfigured checks if VM spec has instance volumes to identify if VM is configured with instance storage
// and returns true/false accordingly.
func IsConfigured(vm *vmopv1alpha1.VirtualMachine) bool {
	isConfigured := false
	for _, volume := range vm.Spec.Volumes {
		if pvc := volume.PersistentVolumeClaim; pvc != nil {
			if pvc.InstanceVolumeClaim != nil {
				isConfigured = true
				break
			}
		}
	}

	return isConfigured
}
