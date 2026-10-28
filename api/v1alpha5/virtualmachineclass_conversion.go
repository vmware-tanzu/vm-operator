// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha5

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	apiconversion "k8s.io/apimachinery/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

// ConvertTo converts this VirtualMachineClass to the Hub version.
func (src *VirtualMachineClass) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineClass)
	return Convert_v1alpha5_VirtualMachineClass_To_v1alpha6_VirtualMachineClass(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineClass.
func (dst *VirtualMachineClass) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineClass)
	return Convert_v1alpha6_VirtualMachineClass_To_v1alpha5_VirtualMachineClass(src, dst, nil)
}

// ConvertTo converts this VirtualMachineClassList to the Hub version.
func (src *VirtualMachineClassList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineClassList)
	return Convert_v1alpha5_VirtualMachineClassList_To_v1alpha6_VirtualMachineClassList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineClassList.
func (dst *VirtualMachineClassList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineClassList)
	return Convert_v1alpha6_VirtualMachineClassList_To_v1alpha5_VirtualMachineClassList(src, dst, nil)
}

// Convert_v1alpha5_VirtualMachineClass_To_v1alpha6_VirtualMachineClass converts a class and restores preserved policies.
func Convert_v1alpha5_VirtualMachineClass_To_v1alpha6_VirtualMachineClass(in *VirtualMachineClass, out *vmopv1.VirtualMachineClass, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha5_VirtualMachineClass_To_v1alpha6_VirtualMachineClass(in, out, s); err != nil {
		return err
	}
	out.ObjectMeta = *in.ObjectMeta.DeepCopy()
	var preserved vmopv1.VirtualMachineClass
	ok, err := utilconversion.UnmarshalData(out, &preserved)
	if len(out.Annotations) == 0 {
		out.Annotations = nil
	}
	if err != nil {
		return fmt.Errorf("restore VMClass conversion data: %w", err)
	}
	if !ok {
		return nil
	}
	var spoke VirtualMachineClassSpec
	if err := Convert_v1alpha6_VirtualMachineClassSpec_To_v1alpha5_VirtualMachineClassSpec(&preserved.Spec, &spoke, s); err != nil {
		return err
	}
	var projected vmopv1.VirtualMachineClassSpec
	if err := Convert_v1alpha5_VirtualMachineClassSpec_To_v1alpha6_VirtualMachineClassSpec(&spoke, &projected, s); err != nil {
		return err
	}
	vmopv1.RestoreClassPolicies(&out.Spec, &preserved.Spec, &projected)
	return nil
}

// Convert_v1alpha6_VirtualMachineClass_To_v1alpha5_VirtualMachineClass preserves policies that this API cannot express.
func Convert_v1alpha6_VirtualMachineClass_To_v1alpha5_VirtualMachineClass(in *vmopv1.VirtualMachineClass, out *VirtualMachineClass, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha6_VirtualMachineClass_To_v1alpha5_VirtualMachineClass(in, out, s); err != nil {
		return err
	}
	out.ObjectMeta = *in.ObjectMeta.DeepCopy()
	if err := utilconversion.MarshalData(in, out); err != nil {
		return fmt.Errorf("preserve VMClass conversion data: %w", err)
	}
	return nil
}

// Convert_v1alpha5_VirtualMachineClassSpec_To_v1alpha6_VirtualMachineClassSpec converts legacy scalar presets to fixed policies.
func Convert_v1alpha5_VirtualMachineClassSpec_To_v1alpha6_VirtualMachineClassSpec(in *VirtualMachineClassSpec, out *vmopv1.VirtualMachineClassSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha5_VirtualMachineClassSpec_To_v1alpha6_VirtualMachineClassSpec(in, out, s); err != nil {
		return err
	}
	out.Resources.Size.CPU = vmopv1.FixedClassResource(*resource.NewQuantity(in.Hardware.Cpus, resource.DecimalSI))
	out.Resources.Size.Memory = vmopv1.FixedClassResource(in.Hardware.Memory)
	out.Resources.Requests.CPU = vmopv1.FixedClassResource(in.Policies.Resources.Requests.Cpu)
	out.Resources.Requests.Memory = vmopv1.FixedClassResource(in.Policies.Resources.Requests.Memory)
	out.Resources.Limits.CPU = vmopv1.FixedClassResource(in.Policies.Resources.Limits.Cpu)
	out.Resources.Limits.Memory = vmopv1.FixedClassResource(in.Policies.Resources.Limits.Memory)
	return nil
}

// Convert_v1alpha6_VirtualMachineClassSpec_To_v1alpha5_VirtualMachineClassSpec projects class defaults into legacy fields.
func Convert_v1alpha6_VirtualMachineClassSpec_To_v1alpha5_VirtualMachineClassSpec(in *vmopv1.VirtualMachineClassSpec, out *VirtualMachineClassSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha6_VirtualMachineClassSpec_To_v1alpha5_VirtualMachineClassSpec(in, out, s); err != nil {
		return err
	}
	cpu := vmopv1.ClassResourceDefault(in.Resources.Size.CPU)
	out.Hardware.Cpus = cpu.Value()
	out.Hardware.Memory = vmopv1.ClassResourceDefault(in.Resources.Size.Memory)
	out.Policies.Resources.Requests.Cpu = vmopv1.ClassResourceDefault(in.Resources.Requests.CPU)
	out.Policies.Resources.Requests.Memory = vmopv1.ClassResourceDefault(in.Resources.Requests.Memory)
	out.Policies.Resources.Limits.Cpu = vmopv1.ClassResourceDefault(in.Resources.Limits.CPU)
	out.Policies.Resources.Limits.Memory = vmopv1.ClassResourceDefault(in.Resources.Limits.Memory)
	return nil
}

// Convert_v1alpha5_VirtualMachineClassHardware_To_v1alpha6_VirtualMachineClassHardware converts hardware presets; resources are moved by the spec conversion.
func Convert_v1alpha5_VirtualMachineClassHardware_To_v1alpha6_VirtualMachineClassHardware(in *VirtualMachineClassHardware, out *vmopv1.VirtualMachineClassHardware, s apiconversion.Scope) error {
	return autoConvert_v1alpha5_VirtualMachineClassHardware_To_v1alpha6_VirtualMachineClassHardware(in, out, s)
}

// Convert_v1alpha6_VirtualMachineClassHardware_To_v1alpha5_VirtualMachineClassHardware converts hardware presets; resources are restored by the spec conversion.
func Convert_v1alpha6_VirtualMachineClassHardware_To_v1alpha5_VirtualMachineClassHardware(in *vmopv1.VirtualMachineClassHardware, out *VirtualMachineClassHardware, s apiconversion.Scope) error {
	return autoConvert_v1alpha6_VirtualMachineClassHardware_To_v1alpha5_VirtualMachineClassHardware(in, out, s)
}

// Convert_v1alpha5_VirtualMachineClassResources_To_v1alpha6_VirtualMachineClassResources converts legacy allocation quantities.
func Convert_v1alpha5_VirtualMachineClassResources_To_v1alpha6_VirtualMachineClassResources(in *VirtualMachineClassResources, out *vmopv1.VirtualMachineClassResources, _ apiconversion.Scope) error {
	out.Requests.CPU = vmopv1.FixedClassResource(in.Requests.Cpu)
	out.Requests.Memory = vmopv1.FixedClassResource(in.Requests.Memory)
	out.Limits.CPU = vmopv1.FixedClassResource(in.Limits.Cpu)
	out.Limits.Memory = vmopv1.FixedClassResource(in.Limits.Memory)
	return nil
}

// Convert_v1alpha6_VirtualMachineClassResources_To_v1alpha5_VirtualMachineClassResources projects allocation defaults.
func Convert_v1alpha6_VirtualMachineClassResources_To_v1alpha5_VirtualMachineClassResources(in *vmopv1.VirtualMachineClassResources, out *VirtualMachineClassResources, _ apiconversion.Scope) error {
	out.Requests.Cpu = vmopv1.ClassResourceDefault(in.Requests.CPU)
	out.Requests.Memory = vmopv1.ClassResourceDefault(in.Requests.Memory)
	out.Limits.Cpu = vmopv1.ClassResourceDefault(in.Limits.CPU)
	out.Limits.Memory = vmopv1.ClassResourceDefault(in.Limits.Memory)
	return nil
}

// Convert_v1alpha5_VirtualMachineResourceSpec_To_v1alpha6_VirtualMachineClassResourceSpec converts legacy allocations to fixed ranges.
func Convert_v1alpha5_VirtualMachineResourceSpec_To_v1alpha6_VirtualMachineClassResourceSpec(in *VirtualMachineResourceSpec, out *vmopv1.VirtualMachineClassResourceSpec, _ apiconversion.Scope) error {
	out.CPU = vmopv1.FixedClassResource(in.Cpu)
	out.Memory = vmopv1.FixedClassResource(in.Memory)
	return nil
}

// Convert_v1alpha6_VirtualMachineClassResourceSpec_To_v1alpha5_VirtualMachineResourceSpec projects allocation defaults.
func Convert_v1alpha6_VirtualMachineClassResourceSpec_To_v1alpha5_VirtualMachineResourceSpec(in *vmopv1.VirtualMachineClassResourceSpec, out *VirtualMachineResourceSpec, _ apiconversion.Scope) error {
	out.Cpu = vmopv1.ClassResourceDefault(in.CPU)
	out.Memory = vmopv1.ClassResourceDefault(in.Memory)
	return nil
}
