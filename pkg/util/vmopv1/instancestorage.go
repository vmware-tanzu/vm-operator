// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
)

// IsInstanceStoragePresent checks if VM Spec has instance volumes added to its Volumes list.
func IsInstanceStoragePresent(vm *vmopv1.VirtualMachine) bool {
	for _, vol := range vm.Spec.Volumes {
		if pvc := vol.PersistentVolumeClaim; pvc != nil && pvc.InstanceVolumeClaim != nil {
			return true
		}
	}
	return false
}

// FilterInstanceStorageVolumes returns instance storage volumes present in VM spec.
func FilterInstanceStorageVolumes(vm *vmopv1.VirtualMachine) []vmopv1.VirtualMachineVolume {
	var volumes []vmopv1.VirtualMachineVolume
	for _, vol := range vm.Spec.Volumes {
		if pvc := vol.PersistentVolumeClaim; pvc != nil && pvc.InstanceVolumeClaim != nil {
			volumes = append(volumes, vol)
		}
	}

	return volumes
}

func IsInsufficientQuota(err error) bool {
	if apierrors.IsForbidden(err) {
		e := err.Error()
		return strings.Contains(e, "insufficient quota") || strings.Contains(e, "exceeded quota")
	}

	return false
}
