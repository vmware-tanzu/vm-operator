// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha3

import (
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
)

// ConvertTo converts this VirtualMachineSetResourcePolicy to the Hub version.
func (src *VirtualMachineSetResourcePolicy) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineSetResourcePolicy)
	return Convert_v1alpha3_VirtualMachineSetResourcePolicy_To_v1alpha4_VirtualMachineSetResourcePolicy(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineSetResourcePolicy.
func (dst *VirtualMachineSetResourcePolicy) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineSetResourcePolicy)
	return Convert_v1alpha4_VirtualMachineSetResourcePolicy_To_v1alpha3_VirtualMachineSetResourcePolicy(src, dst, nil)
}

// ConvertTo converts this VirtualMachineSetResourcePolicyList to the Hub version.
func (src *VirtualMachineSetResourcePolicyList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineSetResourcePolicyList)
	return Convert_v1alpha3_VirtualMachineSetResourcePolicyList_To_v1alpha4_VirtualMachineSetResourcePolicyList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineSetResourcePolicyList.
func (dst *VirtualMachineSetResourcePolicyList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineSetResourcePolicyList)
	return Convert_v1alpha4_VirtualMachineSetResourcePolicyList_To_v1alpha3_VirtualMachineSetResourcePolicyList(src, dst, nil)
}
