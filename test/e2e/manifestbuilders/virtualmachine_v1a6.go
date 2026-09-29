// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	"slices"

	corev1 "k8s.io/api/core/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1cloudinit "github.com/vmware-tanzu/vm-operator/api/v1alpha6/cloudinit"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	vmopv1sysprep "github.com/vmware-tanzu/vm-operator/api/v1alpha6/sysprep"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// VirtualMachineA6 returns the v1alpha6 VirtualMachine described by vmYaml.
// The PVCs referenced by vmYaml.PVCs are not included; see
// PersistentVolumeClaims. An error is returned if an inline cloud config or
// sysprep cannot be decoded.
func VirtualMachineA6(vmYaml VirtualMachineYaml) (*vmopv1.VirtualMachine, error) {
	vm := &vmopv1.VirtualMachine{
		TypeMeta:   typeMeta(vmopv1.GroupVersion.String(), "VirtualMachine"),
		ObjectMeta: objectMeta(vmYaml),
		Spec: vmopv1.VirtualMachineSpec{
			GroupName:    vmYaml.GroupName,
			ClassName:    vmYaml.VMClassName,
			StorageClass: vmYaml.StorageClassName,
			ImageName:    vmYaml.ImageName,
			GuestID:      vmYaml.GuestID,
			PowerState:   vmopv1.VirtualMachinePowerState(vmYaml.PowerState),
			Affinity:     vmYaml.Affinity.DeepCopy(),
			Hardware:     vmYaml.Hardware.DeepCopy(),
			Policies:     slices.Clone(vmYaml.Policies),
		},
	}

	if vmYaml.Bootstrap.hasBootstrap() {
		bootstrap, err := bootstrapA6(vmYaml.Bootstrap)
		if err != nil {
			return nil, err
		}
		vm.Spec.Bootstrap = bootstrap
	}

	for _, pvc := range vmYaml.PVCs {
		v := vmopv1.VirtualMachineVolume{
			Name: pvc.VolumeName,
			VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
				PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvc.ClaimName,
					},
				},
			},
			ControllerBusNumber: pvc.ControllerBusNumber,
			UnitNumber:          pvc.UnitNumber,
			ApplicationType:     pvc.ApplicationType,
		}
		if pvc.ControllerType != nil {
			v.ControllerType = *pvc.ControllerType
		}
		if pvc.SharingMode != nil {
			v.SharingMode = vmopv1.VolumeSharingMode(*pvc.SharingMode)
		}
		if pvc.DiskMode != nil {
			v.DiskMode = vmopv1.VolumeDiskMode(*pvc.DiskMode)
		}
		vm.Spec.Volumes = append(vm.Spec.Volumes, v)
	}

	return vm, nil
}

func bootstrapA6(in Bootstrap) (*vmopv1.VirtualMachineBootstrapSpec, error) {
	out := &vmopv1.VirtualMachineBootstrapSpec{}

	if ci := in.CloudInit; ci != nil {
		cloudConfig, err := unmarshalInline[vmopv1cloudinit.CloudConfig](ci.CloudConfig)
		if err != nil {
			return nil, err
		}
		if ci.RawCloudConfig != nil || cloudConfig != nil {
			out.CloudInit = &vmopv1.VirtualMachineBootstrapCloudInitSpec{
				CloudConfig: cloudConfig,
			}
			if ci.RawCloudConfig != nil {
				out.CloudInit.RawCloudConfig = &vmopv1common.SecretKeySelector{
					Name: ci.RawCloudConfig.Name,
					Key:  ci.RawCloudConfig.Key,
				}
			}
		}
	}

	if sp := in.Sysprep; sp != nil {
		sysprep, err := unmarshalInline[vmopv1sysprep.Sysprep](sp.Sysprep)
		if err != nil {
			return nil, err
		}
		if sp.RawSysprep != nil || sysprep != nil {
			out.Sysprep = &vmopv1.VirtualMachineBootstrapSysprepSpec{
				Sysprep: sysprep,
			}
			if sp.RawSysprep != nil {
				out.Sysprep.RawSysprep = &vmopv1common.SecretKeySelector{
					Name: sp.RawSysprep.Name,
					Key:  sp.RawSysprep.Key,
				}
			}
		}
	}

	if va := in.VAppConfig; va != nil && (va.RawProperties != nil || va.Properties != nil) {
		out.VAppConfig = &vmopv1.VirtualMachineBootstrapVAppConfigSpec{}
		if va.RawProperties != nil {
			out.VAppConfig.RawProperties = *va.RawProperties
		}
		if va.Properties != nil {
			for _, p := range *va.Properties {
				out.VAppConfig.Properties = append(out.VAppConfig.Properties,
					vmopv1common.KeyValueOrSecretKeySelectorPair{
						Key: p.Key,
						Value: vmopv1common.ValueOrSecretKeySelector{
							Value: ptr.To(p.Value.Value),
						},
					})
			}
		}
	}

	if lp := in.LinuxPrep; lp != nil {
		out.LinuxPrep = &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{
			HardwareClockIsUTC:     ptr.To(lp.HardwareClockIsUTC),
			TimeZone:               lp.TimeZone,
			CustomizeAtNextPowerOn: lp.CustomizeAtNextPowerOn,
		}
	}

	return out, nil
}
