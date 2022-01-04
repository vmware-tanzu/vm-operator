// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package instancestorage

import (
	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
)

// IsConfigured checks if VM spec has instance volumes to identify if VM is configured with instance storage
// and returns true/false accordingly.
func IsConfigured(vm *vmopv1alpha1.VirtualMachine) bool {
	for _, vol := range vm.Spec.Volumes {
		if pvc := vol.PersistentVolumeClaim; pvc != nil && pvc.InstanceVolumeClaim != nil {
			return true
		}
	}
	return false
}

// FilterVolumes returns instance storage volumes present in VM spec.
func FilterVolumes(vm *vmopv1alpha1.VirtualMachine) []vmopv1alpha1.VirtualMachineVolume {
	var volumes []vmopv1alpha1.VirtualMachineVolume
	for _, vol := range vm.Spec.Volumes {
		if pvc := vol.PersistentVolumeClaim; pvc != nil && pvc.InstanceVolumeClaim != nil {
			volumes = append(volumes, vol)
		}
	}

	return volumes
}
