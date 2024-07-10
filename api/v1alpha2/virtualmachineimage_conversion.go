// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
)

func Convert_v1alpha3_VirtualMachineImageStatus_To_v1alpha2_VirtualMachineImageStatus(
	in *vmopv1.VirtualMachineImageStatus, out *VirtualMachineImageStatus, s apiconversion.Scope) error {
	return autoConvert_v1alpha3_VirtualMachineImageStatus_To_v1alpha2_VirtualMachineImageStatus(in, out, s)
}

// ConvertTo converts this VirtualMachineImage to the Hub version.
func (src *VirtualMachineImage) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineImage)
	if err := Convert_v1alpha2_VirtualMachineImage_To_v1alpha3_VirtualMachineImage(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &vmopv1.VirtualMachineImage{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	dst.Status = restored.Status

	return nil
}

// ConvertFrom converts the hub version to this VirtualMachineImage.
func (dst *VirtualMachineImage) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineImage)
	if err := Convert_v1alpha3_VirtualMachineImage_To_v1alpha2_VirtualMachineImage(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this VirtualMachineImageList to the Hub version.
func (src *VirtualMachineImageList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineImageList)
	return Convert_v1alpha2_VirtualMachineImageList_To_v1alpha3_VirtualMachineImageList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineImageList.
func (dst *VirtualMachineImageList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineImageList)
	return Convert_v1alpha3_VirtualMachineImageList_To_v1alpha2_VirtualMachineImageList(src, dst, nil)
}

// ConvertTo converts this ClusterVirtualMachineImage to the Hub version.
func (src *ClusterVirtualMachineImage) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.ClusterVirtualMachineImage)
	if err := Convert_v1alpha2_ClusterVirtualMachineImage_To_v1alpha3_ClusterVirtualMachineImage(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &vmopv1.ClusterVirtualMachineImage{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	dst.Status = restored.Status

	return nil
}

// ConvertFrom converts the hub version to this ClusterVirtualMachineImage.
func (dst *ClusterVirtualMachineImage) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.ClusterVirtualMachineImage)
	if err := Convert_v1alpha3_ClusterVirtualMachineImage_To_v1alpha2_ClusterVirtualMachineImage(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this ClusterVirtualMachineImageList to the Hub version.
func (src *ClusterVirtualMachineImageList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.ClusterVirtualMachineImageList)
	return Convert_v1alpha2_ClusterVirtualMachineImageList_To_v1alpha3_ClusterVirtualMachineImageList(src, dst, nil)
}

// ConvertFrom converts the hub version to this ClusterVirtualMachineImageList.
func (dst *ClusterVirtualMachineImageList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.ClusterVirtualMachineImageList)
	return Convert_v1alpha3_ClusterVirtualMachineImageList_To_v1alpha2_ClusterVirtualMachineImageList(src, dst, nil)
}
