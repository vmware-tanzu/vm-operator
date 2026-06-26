// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha3

import (
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

func Convert_v1alpha6_VirtualMachineGroupBootOrderGroup_To_v1alpha3_VirtualMachineGroupBootOrderGroup(
	in *vmopv1.VirtualMachineGroupBootOrderGroup, out *VirtualMachineGroupBootOrderGroup, s apiconversion.Scope) error {

	return autoConvert_v1alpha6_VirtualMachineGroupBootOrderGroup_To_v1alpha3_VirtualMachineGroupBootOrderGroup(in, out, s)
}

func restore_v1alpha6_VirtualMachineGroupBootOrderPowerOffDelay(dst, src *vmopv1.VirtualMachineGroup) {
	for i := range src.Spec.BootOrder {
		if i >= len(dst.Spec.BootOrder) {
			break
		}
		dst.Spec.BootOrder[i].PowerOffDelay = src.Spec.BootOrder[i].PowerOffDelay
	}
}

// ConvertTo converts this VirtualMachineGroup to the Hub version.
func (src *VirtualMachineGroup) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineGroup)
	if err := Convert_v1alpha3_VirtualMachineGroup_To_v1alpha6_VirtualMachineGroup(src, dst, nil); err != nil {
		return err
	}

	restored := &vmopv1.VirtualMachineGroup{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	// BEGIN RESTORE

	restore_v1alpha6_VirtualMachineGroupBootOrderPowerOffDelay(dst, restored)

	// END RESTORE

	return nil
}

// ConvertFrom converts the hub version to this VirtualMachineGroup.
func (dst *VirtualMachineGroup) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineGroup)
	if err := Convert_v1alpha6_VirtualMachineGroup_To_v1alpha3_VirtualMachineGroup(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata.
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this VirtualMachineGroupList to the Hub version.
func (src *VirtualMachineGroupList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineGroupList)
	return Convert_v1alpha3_VirtualMachineGroupList_To_v1alpha6_VirtualMachineGroupList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineGroupList.
func (dst *VirtualMachineGroupList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineGroupList)
	return Convert_v1alpha6_VirtualMachineGroupList_To_v1alpha3_VirtualMachineGroupList(src, dst, nil)
}
