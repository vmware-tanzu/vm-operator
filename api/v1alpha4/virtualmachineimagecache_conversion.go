// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha4

import (
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
)

// ConvertTo converts this VirtualMachineImageCache to the Hub version.
func (src *VirtualMachineImageCache) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineImageCache)
	if err := Convert_v1alpha4_VirtualMachineImageCache_To_v1alpha5_VirtualMachineImageCache(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &vmopv1.VirtualMachineImageCache{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	dst.Status = restored.Status

	return nil
}

// ConvertFrom converts the hub version to this VirtualMachineImageCache.
func (dst *VirtualMachineImageCache) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineImageCache)
	if err := Convert_v1alpha5_VirtualMachineImageCache_To_v1alpha4_VirtualMachineImageCache(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this VirtualMachineImageCacheList to the Hub version.
func (src *VirtualMachineImageCacheList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineImageCacheList)
	return Convert_v1alpha4_VirtualMachineImageCacheList_To_v1alpha5_VirtualMachineImageCacheList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineImageCacheList.
func (dst *VirtualMachineImageCacheList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineImageCacheList)
	return Convert_v1alpha5_VirtualMachineImageCacheList_To_v1alpha4_VirtualMachineImageCacheList(src, dst, nil)
}
