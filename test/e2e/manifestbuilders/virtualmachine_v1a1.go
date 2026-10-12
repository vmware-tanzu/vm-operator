// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	corev1 "k8s.io/api/core/v1"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
)

// VirtualMachineA1 returns the v1alpha1 VirtualMachine described by vmYaml.
func VirtualMachineA1(vmYaml VirtualMachineYaml) *vmopv1a1.VirtualMachine {
	vm := &vmopv1a1.VirtualMachine{
		TypeMeta:   typeMeta(vmopv1a1.GroupVersion.String(), "VirtualMachine"),
		ObjectMeta: objectMeta(vmYaml),
		Spec: vmopv1a1.VirtualMachineSpec{
			ClassName:          vmYaml.VMClassName,
			StorageClass:       vmYaml.StorageClassName,
			ImageName:          vmYaml.ImageName,
			ResourcePolicyName: vmYaml.ResourcePolicy,
			PowerState:         vmopv1a1.VirtualMachinePowerState(vmYaml.PowerState),
			PowerOffMode:       vmopv1a1.VirtualMachinePowerOpMode(vmYaml.PowerOffMode),
		},
	}

	if vmYaml.Network.Type != "" {
		vm.Spec.NetworkInterfaces = []vmopv1a1.VirtualMachineNetworkInterface{
			{
				NetworkName: vmYaml.Network.Name,
				NetworkType: vmYaml.Network.Type,
			},
		}
	}

	if vmYaml.ConfigMapName != "" || vmYaml.SecretName != "" || vmYaml.Transport != "" {
		vm.Spec.VmMetadata = &vmopv1a1.VirtualMachineMetadata{
			ConfigMapName: vmYaml.ConfigMapName,
			SecretName:    vmYaml.SecretName,
			Transport:     vmopv1a1.VirtualMachineMetadataTransport(vmYaml.Transport),
		}
	}

	for _, name := range vmYaml.PVCNames {
		vm.Spec.Volumes = append(vm.Spec.Volumes, vmopv1a1.VirtualMachineVolume{
			Name: name,
			PersistentVolumeClaim: &vmopv1a1.PersistentVolumeClaimVolumeSource{
				PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: name,
				},
			},
		})
	}

	return vm
}
