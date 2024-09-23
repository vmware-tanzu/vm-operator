// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"strconv"

	apiconversion "k8s.io/apimachinery/pkg/conversion"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
)

func Convert_v1alpha1_VirtualMachineClassSpec_To_v1alpha3_VirtualMachineClassSpec(
	in *VirtualMachineClassSpec, out *vmopv1.VirtualMachineClassSpec, s apiconversion.Scope) error {

	return autoConvert_v1alpha1_VirtualMachineClassSpec_To_v1alpha3_VirtualMachineClassSpec(in, out, s)
}

func Convert_v1alpha3_VirtualMachineClassSpec_To_v1alpha1_VirtualMachineClassSpec(
	in *vmopv1.VirtualMachineClassSpec, out *VirtualMachineClassSpec, s apiconversion.Scope) error {

	return autoConvert_v1alpha3_VirtualMachineClassSpec_To_v1alpha1_VirtualMachineClassSpec(in, out, s)
}

const (
	reservedProfileID = "vmoperator.vmware.com/reserved-profile-id"
	reservedSlots     = "vmoperator.vmware.com/reserved-slots"
)

func Convert_v1alpha1_VirtualMachineClass_To_v1alpha3_VirtualMachineClass(
	in *VirtualMachineClass, out *vmopv1.VirtualMachineClass, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha1_VirtualMachineClass_To_v1alpha3_VirtualMachineClass(in, out, s); err != nil {
		return err
	}

	if v := in.Annotations[reservedProfileID]; v != "" {
		out.Spec.ReservedProfileID = v
		delete(in.Annotations, reservedProfileID)
	}

	if v := in.Annotations[reservedSlots]; v != "" {
		i, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			return err
		}
		out.Spec.ReservedSlots = int32(i)
		delete(in.Annotations, reservedSlots)
	}

	return nil
}

func Convert_v1alpha3_VirtualMachineClass_To_v1alpha1_VirtualMachineClass(
	in *vmopv1.VirtualMachineClass, out *VirtualMachineClass, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha3_VirtualMachineClass_To_v1alpha1_VirtualMachineClass(in, out, s); err != nil {
		return err
	}

	if v := in.Spec.ReservedProfileID; v != "" {
		if out.Annotations == nil {
			out.Annotations = map[string]string{}
		}
		out.Annotations[reservedProfileID] = v
	}

	if v := in.Spec.ReservedSlots; v > 0 {
		if out.Annotations == nil {
			out.Annotations = map[string]string{}
		}
		out.Annotations[reservedSlots] = strconv.Itoa(int(v))
	}

	return nil
}

// ConvertTo converts this VirtualMachineClass to the Hub version.
func (src *VirtualMachineClass) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineClass)
	return Convert_v1alpha1_VirtualMachineClass_To_v1alpha3_VirtualMachineClass(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineClass.
func (dst *VirtualMachineClass) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineClass)
	return Convert_v1alpha3_VirtualMachineClass_To_v1alpha1_VirtualMachineClass(src, dst, nil)
}

// ConvertTo converts this VirtualMachineClassList to the Hub version.
func (src *VirtualMachineClassList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineClassList)
	return Convert_v1alpha1_VirtualMachineClassList_To_v1alpha3_VirtualMachineClassList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineClassList.
func (dst *VirtualMachineClassList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineClassList)
	return Convert_v1alpha3_VirtualMachineClassList_To_v1alpha1_VirtualMachineClassList(src, dst, nil)
}

func Convert_v1alpha3_VirtualMachineClassStatus_To_v1alpha1_VirtualMachineClassStatus(
	in *vmopv1.VirtualMachineClassStatus, out *VirtualMachineClassStatus, s apiconversion.Scope) error {

	return autoConvert_v1alpha3_VirtualMachineClassStatus_To_v1alpha1_VirtualMachineClassStatus(in, out, s)
}
