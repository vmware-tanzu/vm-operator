// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	apiconversion "k8s.io/apimachinery/pkg/conversion"

	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

// ConvertTo converts this VirtualMachineClass to the Hub version.
func (src *VirtualMachineClass) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineClass)
	return Convert_v1alpha1_VirtualMachineClass_To_v1alpha6_VirtualMachineClass(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineClass.
func (dst *VirtualMachineClass) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineClass)
	return Convert_v1alpha6_VirtualMachineClass_To_v1alpha1_VirtualMachineClass(src, dst, nil)
}

// ConvertTo converts this VirtualMachineClassList to the Hub version.
func (src *VirtualMachineClassList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineClassList)
	return Convert_v1alpha1_VirtualMachineClassList_To_v1alpha6_VirtualMachineClassList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineClassList.
func (dst *VirtualMachineClassList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineClassList)
	return Convert_v1alpha6_VirtualMachineClassList_To_v1alpha1_VirtualMachineClassList(src, dst, nil)
}

// Convert_v1alpha1_VirtualMachineClass_To_v1alpha6_VirtualMachineClass converts a class and restores preserved policies.
func Convert_v1alpha1_VirtualMachineClass_To_v1alpha6_VirtualMachineClass(in *VirtualMachineClass, out *vmopv1.VirtualMachineClass, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha1_VirtualMachineClass_To_v1alpha6_VirtualMachineClass(in, out, s); err != nil {
		return err
	}
	return vmopv1.RestoreClassConversionData(&in.ObjectMeta, out,
		Convert_v1alpha6_VirtualMachineClassSpec_To_v1alpha1_VirtualMachineClassSpec,
		Convert_v1alpha1_VirtualMachineClassSpec_To_v1alpha6_VirtualMachineClassSpec, s)
}

// Convert_v1alpha6_VirtualMachineClass_To_v1alpha1_VirtualMachineClass preserves policies that this API cannot express.
func Convert_v1alpha6_VirtualMachineClass_To_v1alpha1_VirtualMachineClass(in *vmopv1.VirtualMachineClass, out *VirtualMachineClass, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha6_VirtualMachineClass_To_v1alpha1_VirtualMachineClass(in, out, s); err != nil {
		return err
	}
	return vmopv1.PreserveClassConversionData(in, &out.ObjectMeta)
}

// Convert_v1alpha1_VirtualMachineClassSpec_To_v1alpha6_VirtualMachineClassSpec converts legacy scalar presets to fixed policies.
func Convert_v1alpha1_VirtualMachineClassSpec_To_v1alpha6_VirtualMachineClassSpec(in *VirtualMachineClassSpec, out *vmopv1.VirtualMachineClassSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha1_VirtualMachineClassSpec_To_v1alpha6_VirtualMachineClassSpec(in, out, s); err != nil {
		return err
	}
	out.Resources.Size = vmopv1.FixedClassResourcePair(*resource.NewQuantity(in.Hardware.Cpus, resource.DecimalSI), in.Hardware.Memory)
	return Convert_v1alpha1_VirtualMachineClassResources_To_v1alpha6_VirtualMachineClassResources(&in.Policies.Resources, &out.Resources, s)
}

// Convert_v1alpha6_VirtualMachineClassSpec_To_v1alpha1_VirtualMachineClassSpec projects class defaults into legacy fields.
func Convert_v1alpha6_VirtualMachineClassSpec_To_v1alpha1_VirtualMachineClassSpec(in *vmopv1.VirtualMachineClassSpec, out *VirtualMachineClassSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha6_VirtualMachineClassSpec_To_v1alpha1_VirtualMachineClassSpec(in, out, s); err != nil {
		return err
	}
	cpu, memory := vmopv1.ClassResourcePairDefaults(in.Resources.Size)
	out.Hardware.Cpus = cpu.Value()
	out.Hardware.Memory = memory
	return Convert_v1alpha6_VirtualMachineClassResources_To_v1alpha1_VirtualMachineClassResources(&in.Resources, &out.Policies.Resources, s)
}

// Convert_v1alpha1_VirtualMachineClassHardware_To_v1alpha6_VirtualMachineClassHardware converts hardware presets; resources are moved by the spec conversion.
func Convert_v1alpha1_VirtualMachineClassHardware_To_v1alpha6_VirtualMachineClassHardware(in *VirtualMachineClassHardware, out *vmopv1.VirtualMachineClassHardware, s apiconversion.Scope) error {
	return autoConvert_v1alpha1_VirtualMachineClassHardware_To_v1alpha6_VirtualMachineClassHardware(in, out, s)
}

// Convert_v1alpha6_VirtualMachineClassHardware_To_v1alpha1_VirtualMachineClassHardware converts hardware presets; resources are restored by the spec conversion.
func Convert_v1alpha6_VirtualMachineClassHardware_To_v1alpha1_VirtualMachineClassHardware(in *vmopv1.VirtualMachineClassHardware, out *VirtualMachineClassHardware, s apiconversion.Scope) error {
	return autoConvert_v1alpha6_VirtualMachineClassHardware_To_v1alpha1_VirtualMachineClassHardware(in, out, s)
}

// Convert_v1alpha1_VirtualMachineClassResources_To_v1alpha6_VirtualMachineClassResources converts legacy allocation quantities.
func Convert_v1alpha1_VirtualMachineClassResources_To_v1alpha6_VirtualMachineClassResources(in *VirtualMachineClassResources, out *vmopv1.VirtualMachineClassResources, s apiconversion.Scope) error {
	if err := Convert_v1alpha1_VirtualMachineResourceSpec_To_v1alpha6_VirtualMachineClassResourceSpec(&in.Requests, &out.Requests, s); err != nil {
		return err
	}
	return Convert_v1alpha1_VirtualMachineResourceSpec_To_v1alpha6_VirtualMachineClassResourceSpec(&in.Limits, &out.Limits, s)
}

// Convert_v1alpha6_VirtualMachineClassResources_To_v1alpha1_VirtualMachineClassResources projects allocation defaults.
func Convert_v1alpha6_VirtualMachineClassResources_To_v1alpha1_VirtualMachineClassResources(in *vmopv1.VirtualMachineClassResources, out *VirtualMachineClassResources, s apiconversion.Scope) error {
	if err := Convert_v1alpha6_VirtualMachineClassResourceSpec_To_v1alpha1_VirtualMachineResourceSpec(&in.Requests, &out.Requests, s); err != nil {
		return err
	}
	return Convert_v1alpha6_VirtualMachineClassResourceSpec_To_v1alpha1_VirtualMachineResourceSpec(&in.Limits, &out.Limits, s)
}

// Convert_v1alpha1_VirtualMachineResourceSpec_To_v1alpha6_VirtualMachineClassResourceSpec converts legacy allocations to fixed ranges.
func Convert_v1alpha1_VirtualMachineResourceSpec_To_v1alpha6_VirtualMachineClassResourceSpec(in *VirtualMachineResourceSpec, out *vmopv1.VirtualMachineClassResourceSpec, _ apiconversion.Scope) error {
	*out = vmopv1.FixedClassResourcePair(in.Cpu, in.Memory)
	return nil
}

// Convert_v1alpha6_VirtualMachineClassResourceSpec_To_v1alpha1_VirtualMachineResourceSpec projects allocation defaults.
func Convert_v1alpha6_VirtualMachineClassResourceSpec_To_v1alpha1_VirtualMachineResourceSpec(in *vmopv1.VirtualMachineClassResourceSpec, out *VirtualMachineResourceSpec, _ apiconversion.Scope) error {
	out.Cpu, out.Memory = vmopv1.ClassResourcePairDefaults(*in)
	return nil
}
