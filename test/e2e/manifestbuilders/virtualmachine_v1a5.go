// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	corev1 "k8s.io/api/core/v1"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1a5cloudinit "github.com/vmware-tanzu/vm-operator/api/v1alpha5/cloudinit"
	vmopv1a5common "github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	vmopv1a5sysprep "github.com/vmware-tanzu/vm-operator/api/v1alpha5/sysprep"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// VirtualMachineA5 returns the v1alpha5 VirtualMachine described by vmYaml.
// The PVCs referenced by vmYaml.PVCs are not included; see
// PersistentVolumeClaims. An error is returned if an inline cloud config or
// sysprep cannot be decoded.
func VirtualMachineA5(vmYaml VirtualMachineYaml) (*vmopv1a5.VirtualMachine, error) {
	vm := &vmopv1a5.VirtualMachine{
		TypeMeta:   typeMeta(vmopv1a5.GroupVersion.String(), "VirtualMachine"),
		ObjectMeta: objectMeta(vmYaml),
		Spec: vmopv1a5.VirtualMachineSpec{
			GroupName:    vmYaml.GroupName,
			ClassName:    vmYaml.VMClassName,
			StorageClass: vmYaml.StorageClassName,
			ImageName:    vmYaml.ImageName,
			GuestID:      vmYaml.GuestID,
			PowerState:   vmopv1a5.VirtualMachinePowerState(vmYaml.PowerState),
			Affinity:     affinityToV1A5(vmYaml.Affinity),
			Hardware:     hardwareToV1A5(vmYaml.Hardware),
			Policies:     policiesToV1A5(vmYaml.Policies),
		},
	}

	if vmYaml.Bootstrap.hasBootstrap() {
		bootstrap, err := bootstrapA5(vmYaml.Bootstrap)
		if err != nil {
			return nil, err
		}
		vm.Spec.Bootstrap = bootstrap
	}

	for _, pvc := range vmYaml.PVCs {
		v := vmopv1a5.VirtualMachineVolume{
			Name: pvc.VolumeName,
			VirtualMachineVolumeSource: vmopv1a5.VirtualMachineVolumeSource{
				PersistentVolumeClaim: &vmopv1a5.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvc.ClaimName,
					},
				},
			},
			ControllerBusNumber: pvc.ControllerBusNumber,
			UnitNumber:          pvc.UnitNumber,
			ApplicationType:     vmopv1a5.VolumeApplicationType(pvc.ApplicationType),
		}
		if pvc.ControllerType != nil {
			v.ControllerType = vmopv1a5.VirtualControllerType(*pvc.ControllerType)
		}
		if pvc.SharingMode != nil {
			v.SharingMode = vmopv1a5.VolumeSharingMode(*pvc.SharingMode)
		}
		if pvc.DiskMode != nil {
			v.DiskMode = vmopv1a5.VolumeDiskMode(*pvc.DiskMode)
		}
		vm.Spec.Volumes = append(vm.Spec.Volumes, v)
	}

	return vm, nil
}

func bootstrapA5(in Bootstrap) (*vmopv1a5.VirtualMachineBootstrapSpec, error) {
	out := &vmopv1a5.VirtualMachineBootstrapSpec{}

	if ci := in.CloudInit; ci != nil {
		cloudConfig, err := unmarshalInline[vmopv1a5cloudinit.CloudConfig](ci.CloudConfig)
		if err != nil {
			return nil, err
		}
		if ci.RawCloudConfig != nil || cloudConfig != nil {
			out.CloudInit = &vmopv1a5.VirtualMachineBootstrapCloudInitSpec{
				CloudConfig: cloudConfig,
			}
			if ci.RawCloudConfig != nil {
				out.CloudInit.RawCloudConfig = &vmopv1a5common.SecretKeySelector{
					Name: ci.RawCloudConfig.Name,
					Key:  ci.RawCloudConfig.Key,
				}
			}
		}
	}

	if sp := in.Sysprep; sp != nil {
		sysprep, err := unmarshalInline[vmopv1a5sysprep.Sysprep](sp.Sysprep)
		if err != nil {
			return nil, err
		}
		if sp.RawSysprep != nil || sysprep != nil {
			out.Sysprep = &vmopv1a5.VirtualMachineBootstrapSysprepSpec{
				Sysprep: sysprep,
			}
			if sp.RawSysprep != nil {
				out.Sysprep.RawSysprep = &vmopv1a5common.SecretKeySelector{
					Name: sp.RawSysprep.Name,
					Key:  sp.RawSysprep.Key,
				}
			}
		}
	}

	if va := in.VAppConfig; va != nil && (va.RawProperties != nil || va.Properties != nil) {
		out.VAppConfig = &vmopv1a5.VirtualMachineBootstrapVAppConfigSpec{}
		if va.RawProperties != nil {
			out.VAppConfig.RawProperties = *va.RawProperties
		}
		if va.Properties != nil {
			for _, p := range *va.Properties {
				out.VAppConfig.Properties = append(out.VAppConfig.Properties,
					vmopv1a5common.KeyValueOrSecretKeySelectorPair{
						Key: p.Key,
						Value: vmopv1a5common.ValueOrSecretKeySelector{
							Value: ptr.To(p.Value.Value),
						},
					})
			}
		}
	}

	if lp := in.LinuxPrep; lp != nil {
		out.LinuxPrep = &vmopv1a5.VirtualMachineBootstrapLinuxPrepSpec{
			HardwareClockIsUTC:     ptr.To(lp.HardwareClockIsUTC),
			TimeZone:               lp.TimeZone,
			CustomizeAtNextPowerOn: lp.CustomizeAtNextPowerOn,
		}
	}

	return out, nil
}

// The VirtualMachineYaml fields below use the v1alpha6 types, but
// VirtualMachineA5 must emit their v1alpha5 equivalents. The helpers that
// follow copy the v1alpha6 values into the structurally identical v1alpha5
// types field by field.

func affinityToV1A5(in *vmopv1.AffinitySpec) *vmopv1a5.AffinitySpec {
	if in == nil {
		return nil
	}

	out := &vmopv1a5.AffinitySpec{}
	if a := in.VMAffinity; a != nil {
		out.VMAffinity = &vmopv1a5.VMAffinitySpec{
			RequiredDuringSchedulingPreferredDuringExecution:  affinityTermsToV1A5(a.RequiredDuringSchedulingPreferredDuringExecution),
			PreferredDuringSchedulingPreferredDuringExecution: affinityTermsToV1A5(a.PreferredDuringSchedulingPreferredDuringExecution),
		}
	}
	if a := in.VMAntiAffinity; a != nil {
		out.VMAntiAffinity = &vmopv1a5.VMAntiAffinitySpec{
			RequiredDuringSchedulingPreferredDuringExecution:  affinityTermsToV1A5(a.RequiredDuringSchedulingPreferredDuringExecution),
			PreferredDuringSchedulingPreferredDuringExecution: affinityTermsToV1A5(a.PreferredDuringSchedulingPreferredDuringExecution),
		}
	}

	return out
}

func affinityTermsToV1A5(in []vmopv1.VMAffinityTerm) []vmopv1a5.VMAffinityTerm {
	if in == nil {
		return nil
	}

	out := make([]vmopv1a5.VMAffinityTerm, len(in))
	for i, t := range in {
		out[i] = vmopv1a5.VMAffinityTerm{
			LabelSelector: t.LabelSelector.DeepCopy(),
			TopologyKey:   t.TopologyKey,
		}
	}

	return out
}

func hardwareToV1A5(in *vmopv1.VirtualMachineHardwareSpec) *vmopv1a5.VirtualMachineHardwareSpec {
	if in == nil {
		return nil
	}

	out := &vmopv1a5.VirtualMachineHardwareSpec{}

	for _, c := range in.Cdrom {
		out.Cdrom = append(out.Cdrom, vmopv1a5.VirtualMachineCdromSpec{
			Name: c.Name,
			Image: vmopv1a5.VirtualMachineImageRef{
				Kind: c.Image.Kind,
				Name: c.Image.Name,
			},
			ControllerBusNumber: copyPtr(c.ControllerBusNumber),
			ControllerType:      vmopv1a5.VirtualControllerType(c.ControllerType),
			UnitNumber:          copyPtr(c.UnitNumber),
			Connected:           copyPtr(c.Connected),
			AllowGuestControl:   copyPtr(c.AllowGuestControl),
		})
	}

	for _, c := range in.IDEControllers {
		out.IDEControllers = append(out.IDEControllers, vmopv1a5.IDEControllerSpec{
			BusNumber: c.BusNumber,
		})
	}

	for _, c := range in.NVMEControllers {
		out.NVMEControllers = append(out.NVMEControllers, vmopv1a5.NVMEControllerSpec{
			BusNumber:   c.BusNumber,
			SharingMode: vmopv1a5.VirtualControllerSharingMode(c.SharingMode),
		})
	}

	for _, c := range in.SATAControllers {
		out.SATAControllers = append(out.SATAControllers, vmopv1a5.SATAControllerSpec{
			BusNumber: c.BusNumber,
		})
	}

	for _, c := range in.SCSIControllers {
		out.SCSIControllers = append(out.SCSIControllers, vmopv1a5.SCSIControllerSpec{
			BusNumber:   c.BusNumber,
			SharingMode: vmopv1a5.VirtualControllerSharingMode(c.SharingMode),
			Type:        vmopv1a5.SCSIControllerType(c.Type),
		})
	}

	return out
}

func policiesToV1A5(in []vmopv1.PolicySpec) []vmopv1a5.PolicySpec {
	if in == nil {
		return nil
	}

	out := make([]vmopv1a5.PolicySpec, len(in))
	for i, p := range in {
		out[i] = vmopv1a5.PolicySpec{
			APIVersion: p.APIVersion,
			Kind:       p.Kind,
			Name:       p.Name,
		}
	}

	return out
}

func copyPtr[T any](in *T) *T {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}
