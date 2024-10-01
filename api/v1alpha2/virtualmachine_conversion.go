// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	vmopv1a2common "github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
)

func Convert_v1alpha3_VirtualMachineBootstrapCloudInitSpec_To_v1alpha2_VirtualMachineBootstrapCloudInitSpec(
	in *vmopv1.VirtualMachineBootstrapCloudInitSpec, out *VirtualMachineBootstrapCloudInitSpec, s apiconversion.Scope) error {

	return autoConvert_v1alpha3_VirtualMachineBootstrapCloudInitSpec_To_v1alpha2_VirtualMachineBootstrapCloudInitSpec(in, out, s)
}

func Convert_v1alpha3_VirtualMachineNetworkConfigDNSStatus_To_v1alpha2_VirtualMachineNetworkConfigDNSStatus(
	in *vmopv1.VirtualMachineNetworkConfigDNSStatus, out *VirtualMachineNetworkConfigDNSStatus, s apiconversion.Scope) error {

	return autoConvert_v1alpha3_VirtualMachineNetworkConfigDNSStatus_To_v1alpha2_VirtualMachineNetworkConfigDNSStatus(in, out, s)
}

func Convert_v1alpha3_VirtualMachineNetworkSpec_To_v1alpha2_VirtualMachineNetworkSpec(
	in *vmopv1.VirtualMachineNetworkSpec, out *VirtualMachineNetworkSpec, s apiconversion.Scope) error {

	return autoConvert_v1alpha3_VirtualMachineNetworkSpec_To_v1alpha2_VirtualMachineNetworkSpec(in, out, s)
}

func Convert_v1alpha3_VirtualMachineSpec_To_v1alpha2_VirtualMachineSpec(
	in *vmopv1.VirtualMachineSpec, out *VirtualMachineSpec, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha3_VirtualMachineSpec_To_v1alpha2_VirtualMachineSpec(in, out, s); err != nil {
		return err
	}

	// If out.imageName is empty but in.image.name is non-empty, then on down-
	// convert, copy in.image.name to out.imageName.
	if out.ImageName == "" && in.Image != nil {
		out.ImageName = in.Image.Name
	}

	return nil
}

func Convert_v1alpha2_VirtualMachineVolumeStatus_To_v1alpha3_VirtualMachineVolumeStatus(
	in *VirtualMachineVolumeStatus, out *vmopv1.VirtualMachineVolumeStatus, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha2_VirtualMachineVolumeStatus_To_v1alpha3_VirtualMachineVolumeStatus(in, out, s); err != nil {
		return err
	}

	if out.Type == "" {
		out.Type = vmopv1.VirtualMachineStorageDiskTypeManaged
	}

	return nil
}

func Convert_v1alpha3_VirtualMachineStatus_To_v1alpha2_VirtualMachineStatus(
	in *vmopv1.VirtualMachineStatus, out *VirtualMachineStatus, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha3_VirtualMachineStatus_To_v1alpha2_VirtualMachineStatus(in, out, s); err != nil {
		return err
	}

	out.Volumes = nil
	for i := range in.Volumes {
		if in.Volumes[i].Type != vmopv1.VirtualMachineStorageDiskTypeClassic {

			// Only down-convert volume statuses if the volume is managed.
			var vol VirtualMachineVolumeStatus
			if err := Convert_v1alpha3_VirtualMachineVolumeStatus_To_v1alpha2_VirtualMachineVolumeStatus(
				&in.Volumes[i], &vol, s); err != nil {
				return err
			}
			out.Volumes = append(out.Volumes, vol)
		}
	}

	return nil
}

func Convert_v1alpha2_VirtualMachineStatus_To_v1alpha3_VirtualMachineStatus(
	in *VirtualMachineStatus, out *vmopv1.VirtualMachineStatus, s apiconversion.Scope) error {

	return autoConvert_v1alpha2_VirtualMachineStatus_To_v1alpha3_VirtualMachineStatus(in, out, s)
}

func Convert_v1alpha3_VirtualMachineVolumeStatus_To_v1alpha2_VirtualMachineVolumeStatus(
	in *vmopv1.VirtualMachineVolumeStatus, out *VirtualMachineVolumeStatus, s apiconversion.Scope) error {

	return autoConvert_v1alpha3_VirtualMachineVolumeStatus_To_v1alpha2_VirtualMachineVolumeStatus(in, out, s)
}

func Convert_v1alpha3_VirtualMachine_To_v1alpha2_VirtualMachine(
	in *vmopv1.VirtualMachine, out *VirtualMachine, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha3_VirtualMachine_To_v1alpha2_VirtualMachine(in, out, s); err != nil {
		return err
	}

	// Copy in.spec.image into out.status.image on down-convert.
	if i := in.Spec.Image; i != nil {
		out.Status.Image = &vmopv1a2common.LocalObjectRef{
			APIVersion: vmopv1.GroupVersion.String(),
			Kind:       i.Kind,
			Name:       i.Name,
		}
	}

	// Copy in.spec.network.domainName to
	// out.spec.bootstrap.sysprep.sysprep.identification.joinDomain on
	// down-convert.
	if net := in.Spec.Network; net != nil && net.DomainName != "" {
		if bs := out.Spec.Bootstrap; bs != nil {
			if sp := bs.Sysprep; sp != nil {
				if spsp := sp.Sysprep; spsp != nil {
					if spid := spsp.Identification; spid != nil {
						spid.JoinDomain = net.DomainName
					}
				}
			}
		}
	}

	// Handle the renaming of pause annotation on down convert.
	if val, ok := in.Annotations[vmopv1.PauseAnnotation]; ok {
		if out.Annotations == nil {
			out.Annotations = make(map[string]string)
		}
		out.Annotations[PauseAnnotation] = val
		// Remove the pause annotation corresponding to the Hub.
		// This would also remove the annotation if someone created a
		// v1a2 VM with "paused" annotation.
		delete(out.Annotations, vmopv1.PauseAnnotation)
	}

	return nil
}

func restore_v1alpha3_VirtualMachineCryptoSpec(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.Crypto = src.Spec.Crypto
}

func restore_v1alpha3_VirtualMachineImage(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.Image = src.Spec.Image
	dst.Spec.ImageName = src.Spec.ImageName
}

func restore_v1alpha3_VirtualMachineSpecNetworkDomainName(dst, src *vmopv1.VirtualMachine) {
	var (
		dstDN string
		srcDN string
	)

	if net := dst.Spec.Network; net != nil {
		dstDN = net.DomainName
	}
	if net := src.Spec.Network; net != nil {
		srcDN = net.DomainName
	}

	if dstDN == "" && srcDN != dstDN {
		if dst.Spec.Network == nil {
			dst.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
		}
		dst.Spec.Network.DomainName = srcDN
	}
}

func Convert_v1alpha2_VirtualMachine_To_v1alpha3_VirtualMachine(in *VirtualMachine, out *vmopv1.VirtualMachine, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha2_VirtualMachine_To_v1alpha3_VirtualMachine(in, out, s); err != nil {
		return err
	}

	// For existing VMs, we want to ensure out.spec.image is only updated if
	// this conversion is not part of a create operation. We can determine that
	// by looking at the object's generation. Any generation value > 0 means the
	// resource has been written to etcd. The only time generation is 0 is the
	// initial application of the resource before it has been written to etcd.
	//
	// For VMs being created, this behavior prevents spec.image from being set,
	// causing the VM's mutation webhook to resolve spec.image from the value of
	// spec.imageName.
	//
	// For existing VMs, out.spec.image can be set to ensure the printer column
	// for spec.image.name is non-empty whenever possible.
	if in.Generation > 0 {
		if i := in.Status.Image; i != nil && i.Kind != "" && i.Name != "" {
			out.Spec.Image = &vmopv1.VirtualMachineImageRef{
				Kind: i.Kind,
				Name: i.Name,
			}
		} else if in.Spec.ImageName != "" {
			out.Spec.Image = &vmopv1.VirtualMachineImageRef{
				Name: in.Spec.ImageName,
			}
		}
	}

	// Copy in.bootstrap.sysprep.sysprep.identification.joinDomain to
	// out.spec.domainName on up-convert.
	if bs := in.Spec.Bootstrap; bs != nil {
		if sp := bs.Sysprep; sp != nil {
			if spsp := sp.Sysprep; spsp != nil {
				if spid := spsp.Identification; spid != nil {
					if dn := spid.JoinDomain; dn != "" {
						if out.Spec.Network == nil {
							out.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
						}
						out.Spec.Network.DomainName = dn
					}
				}
			}
		}
	}

	// Handle the renaming of pause annotation on up convert.
	if val, ok := in.Annotations[PauseAnnotation]; ok {
		annotations := out.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations[vmopv1.PauseAnnotation] = val

		// Remove the pause annotation from v1alpha1.
		// This would also remove the annotation if someone created a
		// v1a1 VM with "pause-reconcile" annotation.
		delete(annotations, PauseAnnotation)
		out.GetObjectMeta().SetAnnotations(annotations)
	}

	return nil
}

func restore_v1alpha3_VirtualMachineInstanceUUID(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.InstanceUUID = src.Spec.InstanceUUID
}

func restore_v1alpha3_VirtualMachineBiosUUID(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.BiosUUID = src.Spec.BiosUUID
}

func restore_v1alpha3_VirtualMachineBootstrapCloudInitInstanceID(
	dst, src *vmopv1.VirtualMachine) {

	var iid string
	if bs := src.Spec.Bootstrap; bs != nil {
		if ci := bs.CloudInit; ci != nil {
			iid = ci.InstanceID
		}
	}

	if iid == "" {
		return
	}

	if dst.Spec.Bootstrap == nil {
		dst.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{}
	}
	if dst.Spec.Bootstrap.CloudInit == nil {
		dst.Spec.Bootstrap.CloudInit = &vmopv1.VirtualMachineBootstrapCloudInitSpec{}
	}
	dst.Spec.Bootstrap.CloudInit.InstanceID = iid
}

func restore_v1alpha3_VirtualMachineGuestID(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.GuestID = src.Spec.GuestID
}

func restore_v1alpha3_VirtualMachineCdrom(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.Cdrom = src.Spec.Cdrom
}

// ConvertTo converts this VirtualMachine to the Hub version.
func (src *VirtualMachine) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachine)
	if err := Convert_v1alpha2_VirtualMachine_To_v1alpha3_VirtualMachine(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &vmopv1.VirtualMachine{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	// BEGIN RESTORE

	restore_v1alpha3_VirtualMachineImage(dst, restored)
	restore_v1alpha3_VirtualMachineInstanceUUID(dst, restored)
	restore_v1alpha3_VirtualMachineBiosUUID(dst, restored)
	restore_v1alpha3_VirtualMachineBootstrapCloudInitInstanceID(dst, restored)
	restore_v1alpha3_VirtualMachineSpecNetworkDomainName(dst, restored)
	restore_v1alpha3_VirtualMachineGuestID(dst, restored)
	restore_v1alpha3_VirtualMachineCdrom(dst, restored)
	restore_v1alpha3_VirtualMachineCryptoSpec(dst, restored)

	// END RESTORE

	dst.Status = restored.Status

	return nil
}

// ConvertFrom converts the hub version to this VirtualMachine.
func (dst *VirtualMachine) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachine)
	if err := Convert_v1alpha3_VirtualMachine_To_v1alpha2_VirtualMachine(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this VirtualMachineList to the Hub version.
func (src *VirtualMachineList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineList)
	return Convert_v1alpha2_VirtualMachineList_To_v1alpha3_VirtualMachineList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineList.
func (dst *VirtualMachineList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineList)
	return Convert_v1alpha3_VirtualMachineList_To_v1alpha2_VirtualMachineList(src, dst, nil)
}
