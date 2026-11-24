// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha3

import (
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

// ConvertTo converts this VirtualMachineSetResourcePolicy to the Hub version.
func (src *VirtualMachineSetResourcePolicy) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineSetResourcePolicy)
	return Convert_v1alpha3_VirtualMachineSetResourcePolicy_To_v1alpha6_VirtualMachineSetResourcePolicy(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineSetResourcePolicy.
func (dst *VirtualMachineSetResourcePolicy) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineSetResourcePolicy)
	return Convert_v1alpha6_VirtualMachineSetResourcePolicy_To_v1alpha3_VirtualMachineSetResourcePolicy(src, dst, nil)
}

// ConvertTo converts this VirtualMachineSetResourcePolicyList to the Hub version.
func (src *VirtualMachineSetResourcePolicyList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineSetResourcePolicyList)
	return Convert_v1alpha3_VirtualMachineSetResourcePolicyList_To_v1alpha6_VirtualMachineSetResourcePolicyList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineSetResourcePolicyList.
func (dst *VirtualMachineSetResourcePolicyList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineSetResourcePolicyList)
	return Convert_v1alpha6_VirtualMachineSetResourcePolicyList_To_v1alpha3_VirtualMachineSetResourcePolicyList(src, dst, nil)
}

// Convert_v1alpha3_VirtualMachineResourceSpec_To_v1alpha6_VirtualMachineResourceSpec maps the Cpu field to the hub's CPU field.
func Convert_v1alpha3_VirtualMachineResourceSpec_To_v1alpha6_VirtualMachineResourceSpec(
	in *VirtualMachineResourceSpec, out *vmopv1.VirtualMachineResourceSpec, _ apiconversion.Scope) error {

	out.CPU = in.Cpu
	out.Memory = in.Memory
	return nil
}

// Convert_v1alpha6_VirtualMachineResourceSpec_To_v1alpha3_VirtualMachineResourceSpec maps the hub's CPU field to the Cpu field.
func Convert_v1alpha6_VirtualMachineResourceSpec_To_v1alpha3_VirtualMachineResourceSpec(
	in *vmopv1.VirtualMachineResourceSpec, out *VirtualMachineResourceSpec, _ apiconversion.Scope) error {

	out.Cpu = in.CPU
	out.Memory = in.Memory
	return nil
}
