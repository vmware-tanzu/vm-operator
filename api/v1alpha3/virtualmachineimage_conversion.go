// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha3

import (
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
)

// ConvertTo converts this VirtualMachineImage to the Hub version.
func (src *VirtualMachineImage) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineImage)
	return Convert_v1alpha3_VirtualMachineImage_To_v1alpha4_VirtualMachineImage(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineImage.
func (dst *VirtualMachineImage) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineImage)
	return Convert_v1alpha4_VirtualMachineImage_To_v1alpha3_VirtualMachineImage(src, dst, nil)
}

// ConvertTo converts this VirtualMachineImageList to the Hub version.
func (src *VirtualMachineImageList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineImageList)
	return Convert_v1alpha3_VirtualMachineImageList_To_v1alpha4_VirtualMachineImageList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineImageList.
func (dst *VirtualMachineImageList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineImageList)
	return Convert_v1alpha4_VirtualMachineImageList_To_v1alpha3_VirtualMachineImageList(src, dst, nil)
}

// ConvertTo converts this ClusterVirtualMachineImage to the Hub version.
func (src *ClusterVirtualMachineImage) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.ClusterVirtualMachineImage)
	return Convert_v1alpha3_ClusterVirtualMachineImage_To_v1alpha4_ClusterVirtualMachineImage(src, dst, nil)
}

// ConvertFrom converts the hub version to this ClusterVirtualMachineImage.
func (dst *ClusterVirtualMachineImage) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.ClusterVirtualMachineImage)
	return Convert_v1alpha4_ClusterVirtualMachineImage_To_v1alpha3_ClusterVirtualMachineImage(src, dst, nil)
}

// ConvertTo converts this ClusterVirtualMachineImageList to the Hub version.
func (src *ClusterVirtualMachineImageList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.ClusterVirtualMachineImageList)
	return Convert_v1alpha3_ClusterVirtualMachineImageList_To_v1alpha4_ClusterVirtualMachineImageList(src, dst, nil)
}

// ConvertFrom converts the hub version to this ClusterVirtualMachineImageList.
func (dst *ClusterVirtualMachineImageList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.ClusterVirtualMachineImageList)
	return Convert_v1alpha4_ClusterVirtualMachineImageList_To_v1alpha3_ClusterVirtualMachineImageList(src, dst, nil)
}
