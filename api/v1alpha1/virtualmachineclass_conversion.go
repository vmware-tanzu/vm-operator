// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
)

func restore_v1alpha4_VirtualMachineClassDisplayName(dst, src *vmopv1.VirtualMachineClass) {
	dst.Spec.DisplayName = src.Spec.DisplayName
}

// ConvertTo converts this VirtualMachineClass to the Hub version.
func (src *VirtualMachineClass) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineClass)
	if err := Convert_v1alpha1_VirtualMachineClass_To_v1alpha4_VirtualMachineClass(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &vmopv1.VirtualMachineClass{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	// BEGIN RESTORE

	restore_v1alpha4_VirtualMachineClassDisplayName(dst, restored)

	// END RESTORE

	dst.Status = restored.Status

	return nil
}

// ConvertFrom converts the hub version to this VirtualMachineClass.
func (dst *VirtualMachineClass) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineClass)
	if err := Convert_v1alpha4_VirtualMachineClass_To_v1alpha1_VirtualMachineClass(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this VirtualMachineClassList to the Hub version.
func (src *VirtualMachineClassList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineClassList)
	return Convert_v1alpha1_VirtualMachineClassList_To_v1alpha4_VirtualMachineClassList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineClassList.
func (dst *VirtualMachineClassList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineClassList)
	return Convert_v1alpha4_VirtualMachineClassList_To_v1alpha1_VirtualMachineClassList(src, dst, nil)
}

func Convert_v1alpha4_VirtualMachineClassSpec_To_v1alpha1_VirtualMachineClassSpec(in *vmopv1.VirtualMachineClassSpec, out *VirtualMachineClassSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha4_VirtualMachineClassSpec_To_v1alpha1_VirtualMachineClassSpec(in, out, s); err != nil {
		return err
	}

	return nil
}
