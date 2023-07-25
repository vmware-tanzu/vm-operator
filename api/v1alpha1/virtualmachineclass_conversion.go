// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha2"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
)

// ConvertTo converts this VirtualMachineClass to the Hub version.
func (src *VirtualMachineClass) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.VirtualMachineClass)

	if err := Convert_v1alpha1_VirtualMachineClass_To_v1alpha2_VirtualMachineClass(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &v1alpha2.VirtualMachineClass{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	dst.Status = restored.Status
	return nil
}

// ConvertFrom converts the hub version to this VirtualMachineClass.
func (dst *VirtualMachineClass) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.VirtualMachineClass)
	if err := Convert_v1alpha2_VirtualMachineClass_To_v1alpha1_VirtualMachineClass(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this VirtualMachineClassList to the Hub version.
func (src *VirtualMachineClassList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.VirtualMachineClassList)
	return Convert_v1alpha1_VirtualMachineClassList_To_v1alpha2_VirtualMachineClassList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineClassList.
func (dst *VirtualMachineClassList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.VirtualMachineClassList)
	return Convert_v1alpha2_VirtualMachineClassList_To_v1alpha1_VirtualMachineClassList(src, dst, nil)
}

func Convert_v1alpha2_VirtualMachineClassStatus_To_v1alpha1_VirtualMachineClassStatus(
	in *v1alpha2.VirtualMachineClassStatus, out *VirtualMachineClassStatus, s apiconversion.Scope) error {

	return autoConvert_v1alpha2_VirtualMachineClassStatus_To_v1alpha1_VirtualMachineClassStatus(in, out, s)
}
