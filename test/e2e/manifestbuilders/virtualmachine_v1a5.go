// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	corev1 "k8s.io/api/core/v1"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

// VirtualMachineA5 returns the v1alpha5 VirtualMachine described by vmYaml.
// The PVCs referenced by vmYaml.PVCs are not included; see
// PersistentVolumeClaims.
func VirtualMachineA5(vmYaml VirtualMachineYaml) *vmopv1a5.VirtualMachine {
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
			Hardware:     hardwareToV1A5(vmYaml.Hardware),
		},
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

	return vm
}

// VirtualMachineYaml.Hardware uses the v1alpha6 type, but VirtualMachineA5 must
// emit its v1alpha5 equivalent. hardwareToV1A5 copies the v1alpha6 values into
// the structurally identical v1alpha5 type field by field.

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

func copyPtr[T any](in *T) *T {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}
