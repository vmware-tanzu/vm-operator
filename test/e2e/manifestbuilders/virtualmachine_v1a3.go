// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	corev1 "k8s.io/api/core/v1"

	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// VirtualMachineA3 returns the v1alpha3 VirtualMachine described by vmYaml.
func VirtualMachineA3(vmYaml VirtualMachineYaml) *vmopv1a3.VirtualMachine {
	vm := &vmopv1a3.VirtualMachine{
		TypeMeta:   typeMeta(vmopv1a3.GroupVersion.String(), "VirtualMachine"),
		ObjectMeta: objectMeta(vmYaml),
		Spec: vmopv1a3.VirtualMachineSpec{
			ClassName:    vmYaml.VMClassName,
			StorageClass: vmYaml.StorageClassName,
			ImageName:    vmYaml.ImageName,
			PowerState:   vmopv1a3.VirtualMachinePowerState(vmYaml.PowerState),
			GuestID:      vmYaml.GuestID,
		},
	}

	if c := vmYaml.Crypto; c != nil {
		vm.Spec.Crypto = &vmopv1a3.VirtualMachineCryptoSpec{
			EncryptionClassName:   c.EncryptionClassName,
			UseDefaultKeyProvider: ptr.To(c.UseDefaultKeyProvider),
		}
	}

	for _, name := range vmYaml.PVCNames {
		vm.Spec.Volumes = append(vm.Spec.Volumes, vmopv1a3.VirtualMachineVolume{
			Name: name,
			VirtualMachineVolumeSource: vmopv1a3.VirtualMachineVolumeSource{
				PersistentVolumeClaim: &vmopv1a3.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: name,
					},
				},
			},
		})
	}

	for _, c := range vmYaml.Cdrom {
		vm.Spec.Cdrom = append(vm.Spec.Cdrom, vmopv1a3.VirtualMachineCdromSpec{
			Name: c.Name,
			Image: vmopv1a3.VirtualMachineImageRef{
				Name: c.ImageName,
				Kind: c.ImageKind,
			},
			Connected:         ptr.To(c.Connected),
			AllowGuestControl: ptr.To(c.AllowGuestControl),
		})
	}

	return vm
}
