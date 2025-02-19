// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha3

import (
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
)

// ConvertTo converts this VirtualMachineImageCache to the Hub version.
func (i *VirtualMachineImageCache) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineImageCache)
	return Convert_v1alpha3_VirtualMachineImageCache_To_v1alpha4_VirtualMachineImageCache(i, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineImageCache.
func (i *VirtualMachineImageCache) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineImageCache)
	return Convert_v1alpha4_VirtualMachineImageCache_To_v1alpha3_VirtualMachineImageCache(src, i, nil)
}

// ConvertTo converts this VirtualMachineImageCacheList to the Hub version.
func (src *VirtualMachineImageCacheList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineImageCacheList)
	return Convert_v1alpha3_VirtualMachineImageCacheList_To_v1alpha4_VirtualMachineImageCacheList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineImageCacheList.
func (dst *VirtualMachineImageCacheList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineImageCacheList)
	return Convert_v1alpha4_VirtualMachineImageCacheList_To_v1alpha3_VirtualMachineImageCacheList(src, dst, nil)
}
