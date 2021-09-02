// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package instancestorage

import vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

// GetConfiguredInstanceVolumes returns instance volumes configured in VM resource.
func GetConfiguredInstanceVolumes(vm *vmopv1alpha1.VirtualMachine) []*vmopv1alpha1.PersistentVolumeClaimVolumeSource {
	pvcs := []*vmopv1alpha1.PersistentVolumeClaimVolumeSource{}
	for _, volume := range vm.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil &&
			volume.PersistentVolumeClaim.InstanceVolumeClaim != nil {
			pvcs = append(pvcs, volume.PersistentVolumeClaim)
		}
	}
	return pvcs
}

// InstanceVolumeEqComparator compares Instance Volumes configured in VM resource & VMClass.
// returns true if instance volumes configured in VMClass exists in VM resource, false otherwise.
func InstanceVolumeEqComparator(vm *vmopv1alpha1.VirtualMachine, storage vmopv1alpha1.InstanceStorage) bool {
	for _, volume := range storage.Volumes {
		if !isPVCExistsInVMSpec(vm, storage.StorageClass, volume) {
			return false
		}
	}
	return true
}

func isPVCExistsInVMSpec(vm *vmopv1alpha1.VirtualMachine, storageClass string, isv vmopv1alpha1.InstanceStorageVolume) bool {
	for _, pvc := range GetConfiguredInstanceVolumes(vm) {
		if pvc.InstanceVolumeClaim.StorageClass == storageClass &&
			pvc.InstanceVolumeClaim.Size == isv.Size {
			return true
		}
	}
	return false
}
