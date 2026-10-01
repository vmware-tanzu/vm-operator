// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a2cloudinit "github.com/vmware-tanzu/vm-operator/api/v1alpha2/cloudinit"
	vmopv1a2common "github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
	vmopv1a2sysprep "github.com/vmware-tanzu/vm-operator/api/v1alpha2/sysprep"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// VirtualMachineA2 returns the v1alpha2 VirtualMachine described by vmYaml.
// Each vmYaml.NetworkA2.Interfaces entry becomes a network interface named
// eth0, eth1, and so on. An error is returned if an inline cloud config or
// sysprep cannot be decoded.
func VirtualMachineA2(vmYaml VirtualMachineYaml) (*vmopv1a2.VirtualMachine, error) {
	vm := &vmopv1a2.VirtualMachine{
		TypeMeta:   typeMeta(vmopv1a2.GroupVersion.String(), "VirtualMachine"),
		ObjectMeta: objectMeta(vmYaml),
		Spec: vmopv1a2.VirtualMachineSpec{
			ClassName:    vmYaml.VMClassName,
			StorageClass: vmYaml.StorageClassName,
			ImageName:    vmYaml.ImageName,
			PowerState:   vmopv1a2.VirtualMachinePowerState(vmYaml.PowerState),
			PowerOffMode: vmopv1a2.VirtualMachinePowerOpMode(vmYaml.PowerOffMode),
		},
	}

	if len(vmYaml.NetworkA2.Interfaces) > 0 {
		vm.Spec.Network = &vmopv1a2.VirtualMachineNetworkSpec{}
		for i, iface := range vmYaml.NetworkA2.Interfaces {
			vm.Spec.Network.Interfaces = append(vm.Spec.Network.Interfaces,
				vmopv1a2.VirtualMachineNetworkInterfaceSpec{
					Name: fmt.Sprintf("eth%d", i),
					Network: &vmopv1a2common.PartialObjectRef{
						TypeMeta: metav1.TypeMeta{
							APIVersion: iface.APIVersion,
							Kind:       iface.Kind,
						},
						Name: iface.Name,
					},
				})
		}
	}

	if vmYaml.ResourcePolicy != "" {
		vm.Spec.Reserved = &vmopv1a2.VirtualMachineReservedSpec{
			ResourcePolicyName: vmYaml.ResourcePolicy,
		}
	}

	if vmYaml.Bootstrap.hasBootstrap() {
		bootstrap, err := bootstrapA2(vmYaml.Bootstrap)
		if err != nil {
			return nil, err
		}
		vm.Spec.Bootstrap = bootstrap
	}

	for _, name := range vmYaml.PVCNames {
		vm.Spec.Volumes = append(vm.Spec.Volumes, vmopv1a2.VirtualMachineVolume{
			Name: name,
			VirtualMachineVolumeSource: vmopv1a2.VirtualMachineVolumeSource{
				PersistentVolumeClaim: &vmopv1a2.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: name,
					},
				},
			},
		})
	}

	return vm, nil
}

func bootstrapA2(in Bootstrap) (*vmopv1a2.VirtualMachineBootstrapSpec, error) {
	out := &vmopv1a2.VirtualMachineBootstrapSpec{}

	if ci := in.CloudInit; ci != nil {
		cloudConfig, err := unmarshalInline[vmopv1a2cloudinit.CloudConfig](ci.CloudConfig)
		if err != nil {
			return nil, err
		}
		if ci.RawCloudConfig != nil || cloudConfig != nil {
			out.CloudInit = &vmopv1a2.VirtualMachineBootstrapCloudInitSpec{
				CloudConfig: cloudConfig,
			}
			if ci.RawCloudConfig != nil {
				out.CloudInit.RawCloudConfig = &vmopv1a2common.SecretKeySelector{
					Name: ci.RawCloudConfig.Name,
					Key:  ci.RawCloudConfig.Key,
				}
			}
		}
	}

	if sp := in.Sysprep; sp != nil {
		sysprep, err := unmarshalInline[vmopv1a2sysprep.Sysprep](sp.Sysprep)
		if err != nil {
			return nil, err
		}
		if sp.RawSysprep != nil || sysprep != nil {
			out.Sysprep = &vmopv1a2.VirtualMachineBootstrapSysprepSpec{
				Sysprep: sysprep,
			}
			if sp.RawSysprep != nil {
				out.Sysprep.RawSysprep = &vmopv1a2common.SecretKeySelector{
					Name: sp.RawSysprep.Name,
					Key:  sp.RawSysprep.Key,
				}
			}
		}
	}

	if va := in.VAppConfig; va != nil && (va.RawProperties != nil || va.Properties != nil) {
		out.VAppConfig = &vmopv1a2.VirtualMachineBootstrapVAppConfigSpec{}
		if va.RawProperties != nil {
			out.VAppConfig.RawProperties = *va.RawProperties
		}
		if va.Properties != nil {
			for _, p := range *va.Properties {
				out.VAppConfig.Properties = append(out.VAppConfig.Properties,
					vmopv1a2common.KeyValueOrSecretKeySelectorPair{
						Key: p.Key,
						Value: vmopv1a2common.ValueOrSecretKeySelector{
							Value: ptr.To(p.Value.Value),
						},
					})
			}
		}
	}

	if lp := in.LinuxPrep; lp != nil {
		out.LinuxPrep = &vmopv1a2.VirtualMachineBootstrapLinuxPrepSpec{
			HardwareClockIsUTC: ptr.To(lp.HardwareClockIsUTC),
			TimeZone:           lp.TimeZone,
		}
	}

	return out, nil
}
