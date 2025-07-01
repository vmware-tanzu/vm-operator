// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
)

func Convert_v1alpha1_VirtualMachineSetResourcePolicySpec_To_v1alpha4_VirtualMachineSetResourcePolicySpec(
	in *VirtualMachineSetResourcePolicySpec, out *vmopv1.VirtualMachineSetResourcePolicySpec, s apiconversion.Scope) error {

	out.Folder = in.Folder.Name
	for _, mod := range in.ClusterModules {
		out.ClusterModuleGroups = append(out.ClusterModuleGroups, mod.GroupName)
	}

	return autoConvert_v1alpha1_VirtualMachineSetResourcePolicySpec_To_v1alpha4_VirtualMachineSetResourcePolicySpec(in, out, s)
}

func Convert_v1alpha4_VirtualMachineSetResourcePolicySpec_To_v1alpha1_VirtualMachineSetResourcePolicySpec(
	in *vmopv1.VirtualMachineSetResourcePolicySpec, out *VirtualMachineSetResourcePolicySpec, s apiconversion.Scope) error {

	out.Folder.Name = in.Folder
	for _, name := range in.ClusterModuleGroups {
		out.ClusterModules = append(out.ClusterModules, ClusterModuleSpec{GroupName: name})
	}

	return autoConvert_v1alpha4_VirtualMachineSetResourcePolicySpec_To_v1alpha1_VirtualMachineSetResourcePolicySpec(in, out, s)
}

// ConvertTo converts this VirtualMachineSetResourcePolicy to the Hub version.
func (src *VirtualMachineSetResourcePolicy) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineSetResourcePolicy)
	return Convert_v1alpha1_VirtualMachineSetResourcePolicy_To_v1alpha4_VirtualMachineSetResourcePolicy(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineSetResourcePolicy.
func (dst *VirtualMachineSetResourcePolicy) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineSetResourcePolicy)
	return Convert_v1alpha4_VirtualMachineSetResourcePolicy_To_v1alpha1_VirtualMachineSetResourcePolicy(src, dst, nil)
}

// ConvertTo converts this VirtualMachineSetResourcePolicyList to the Hub version.
func (src *VirtualMachineSetResourcePolicyList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineSetResourcePolicyList)
	return Convert_v1alpha1_VirtualMachineSetResourcePolicyList_To_v1alpha4_VirtualMachineSetResourcePolicyList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineSetResourcePolicyList.
func (dst *VirtualMachineSetResourcePolicyList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineSetResourcePolicyList)
	return Convert_v1alpha4_VirtualMachineSetResourcePolicyList_To_v1alpha1_VirtualMachineSetResourcePolicyList(src, dst, nil)
}
