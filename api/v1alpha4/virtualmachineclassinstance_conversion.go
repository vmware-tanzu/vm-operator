// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha4

import (
	"fmt"

	apiconversion "k8s.io/apimachinery/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

// ConvertTo converts this VirtualMachineClassInstance to the Hub version.
func (src *VirtualMachineClassInstance) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineClassInstance)
	return Convert_v1alpha4_VirtualMachineClassInstance_To_v1alpha6_VirtualMachineClassInstance(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineClassInstance.
func (dst *VirtualMachineClassInstance) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineClassInstance)
	return Convert_v1alpha6_VirtualMachineClassInstance_To_v1alpha4_VirtualMachineClassInstance(src, dst, nil)
}

// Convert_v1alpha4_VirtualMachineClassInstance_To_v1alpha6_VirtualMachineClassInstance restores the immutable class snapshot.
func Convert_v1alpha4_VirtualMachineClassInstance_To_v1alpha6_VirtualMachineClassInstance(in *VirtualMachineClassInstance, out *vmopv1.VirtualMachineClassInstance, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha4_VirtualMachineClassInstance_To_v1alpha6_VirtualMachineClassInstance(in, out, s); err != nil {
		return err
	}
	out.ObjectMeta = *in.ObjectMeta.DeepCopy()
	var preserved vmopv1.VirtualMachineClassInstance
	ok, err := utilconversion.UnmarshalData(out, &preserved)
	if len(out.Annotations) == 0 {
		out.Annotations = nil
	}
	if err != nil {
		return fmt.Errorf("restore VMClassInstance conversion data: %w", err)
	}
	if !ok {
		return nil
	}
	var spoke VirtualMachineClassSpec
	if err := Convert_v1alpha6_VirtualMachineClassSpec_To_v1alpha4_VirtualMachineClassSpec(&preserved.Spec.VirtualMachineClassSpec, &spoke, s); err != nil {
		return err
	}
	var projected vmopv1.VirtualMachineClassSpec
	if err := Convert_v1alpha4_VirtualMachineClassSpec_To_v1alpha6_VirtualMachineClassSpec(&spoke, &projected, s); err != nil {
		return err
	}
	vmopv1.RestoreClassPolicies(&out.Spec.VirtualMachineClassSpec, &preserved.Spec.VirtualMachineClassSpec, &projected)
	return nil
}

// Convert_v1alpha6_VirtualMachineClassInstance_To_v1alpha4_VirtualMachineClassInstance preserves the immutable class snapshot.
func Convert_v1alpha6_VirtualMachineClassInstance_To_v1alpha4_VirtualMachineClassInstance(in *vmopv1.VirtualMachineClassInstance, out *VirtualMachineClassInstance, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha6_VirtualMachineClassInstance_To_v1alpha4_VirtualMachineClassInstance(in, out, s); err != nil {
		return err
	}
	out.ObjectMeta = *in.ObjectMeta.DeepCopy()
	if err := utilconversion.MarshalData(in, out); err != nil {
		return fmt.Errorf("preserve VMClassInstance conversion data: %w", err)
	}
	return nil
}
