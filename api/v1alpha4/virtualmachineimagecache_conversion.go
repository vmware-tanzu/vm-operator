// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha4

import (
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
)

// ConvertTo converts this VirtualMachineImageCache to the Hub version.
func (i *VirtualMachineImageCache) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineImageCache)
	return Convert_v1alpha4_VirtualMachineImageCache_To_v1alpha5_VirtualMachineImageCache(i, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineImageCache.
func (i *VirtualMachineImageCache) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineImageCache)
	return Convert_v1alpha5_VirtualMachineImageCache_To_v1alpha4_VirtualMachineImageCache(src, i, nil)
}
