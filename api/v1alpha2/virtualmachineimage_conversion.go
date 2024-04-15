// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha3"
)

// ConvertTo converts this VirtualMachineImage to the Hub version.
func (src *VirtualMachineImage) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha3.VirtualMachineImage)
	return Convert_v1alpha2_VirtualMachineImage_To_v1alpha3_VirtualMachineImage(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineImage.
func (dst *VirtualMachineImage) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha3.VirtualMachineImage)
	return Convert_v1alpha3_VirtualMachineImage_To_v1alpha2_VirtualMachineImage(src, dst, nil)
}

// ConvertTo converts this VirtualMachineImageList to the Hub version.
func (src *VirtualMachineImageList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha3.VirtualMachineImageList)
	return Convert_v1alpha2_VirtualMachineImageList_To_v1alpha3_VirtualMachineImageList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineImageList.
func (dst *VirtualMachineImageList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha3.VirtualMachineImageList)
	return Convert_v1alpha3_VirtualMachineImageList_To_v1alpha2_VirtualMachineImageList(src, dst, nil)
}

// ConvertTo converts this ClusterVirtualMachineImage to the Hub version.
func (src *ClusterVirtualMachineImage) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha3.ClusterVirtualMachineImage)
	return Convert_v1alpha2_ClusterVirtualMachineImage_To_v1alpha3_ClusterVirtualMachineImage(src, dst, nil)
}

// ConvertFrom converts the hub version to this ClusterVirtualMachineImage.
func (dst *ClusterVirtualMachineImage) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha3.ClusterVirtualMachineImage)
	return Convert_v1alpha3_ClusterVirtualMachineImage_To_v1alpha2_ClusterVirtualMachineImage(src, dst, nil)
}

// ConvertTo converts this ClusterVirtualMachineImageList to the Hub version.
func (src *ClusterVirtualMachineImageList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha3.ClusterVirtualMachineImageList)
	return Convert_v1alpha2_ClusterVirtualMachineImageList_To_v1alpha3_ClusterVirtualMachineImageList(src, dst, nil)
}

// ConvertFrom converts the hub version to this ClusterVirtualMachineImageList.
func (dst *ClusterVirtualMachineImageList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha3.ClusterVirtualMachineImageList)
	return Convert_v1alpha3_ClusterVirtualMachineImageList_To_v1alpha2_ClusterVirtualMachineImageList(src, dst, nil)
}
