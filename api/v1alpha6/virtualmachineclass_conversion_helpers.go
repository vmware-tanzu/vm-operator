// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha6

import (
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiconversion "k8s.io/apimachinery/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
)

// FixedClassResource converts a legacy scalar into a fixed class range.
func FixedClassResource(value resource.Quantity) *ResourceQuantityRange {
	minimum, maximum, def := value.DeepCopy(), value.DeepCopy(), value.DeepCopy()
	return &ResourceQuantityRange{Min: &minimum, Max: &maximum, Default: &def}
}

// ClassResourceDefault returns a class range's default for an older API.
func ClassResourceDefault(value *ResourceQuantityRange) resource.Quantity {
	if value == nil || value.Default == nil {
		return resource.Quantity{}
	}
	return value.Default.DeepCopy()
}

// RestoreClassPolicies restores fields an older API cannot represent unless
// that API's writer changed their projected value. Projected is the preserved
// class converted down and back up without its conversion annotation.
func RestoreClassPolicies(dst, preserved, projected *VirtualMachineClassSpec) {
	for _, group := range []struct {
		dst, preserved, projected *VirtualMachineClassResourceSpec
	}{
		{&dst.Resources.Size, &preserved.Resources.Size, &projected.Resources.Size},
		{&dst.Resources.Requests, &preserved.Resources.Requests, &projected.Resources.Requests},
		{&dst.Resources.Limits, &preserved.Resources.Limits, &projected.Resources.Limits},
	} {
		if apiequality.Semantic.DeepEqual(group.dst.CPU, group.projected.CPU) {
			group.dst.CPU = group.preserved.CPU
		}
		if apiequality.Semantic.DeepEqual(group.dst.Memory, group.projected.Memory) {
			group.dst.Memory = group.preserved.Memory
		}
	}
}

// RestoreClassConversionData restores class policies preserved by an older API.
// The version-specific converters project each policy into that API's fields so
// an older writer's edits take precedence over the preserved policy.
func RestoreClassConversionData[S any](
	inMeta *metav1.ObjectMeta,
	out *VirtualMachineClass,
	down func(*VirtualMachineClassSpec, *S, apiconversion.Scope) error,
	up func(*S, *VirtualMachineClassSpec, apiconversion.Scope) error,
	s apiconversion.Scope,
) error {
	var preserved VirtualMachineClass
	ok, err := restoreClassData(inMeta, &out.ObjectMeta, &preserved, "VMClass")
	if err != nil || !ok {
		return err
	}
	return restoreClassSpec(&out.Spec, &preserved.Spec, down, up, s)
}

// RestoreClassInstanceConversionData restores the class policies in an older
// API's immutable class snapshot, honoring edits to its projected fields.
func RestoreClassInstanceConversionData[S any](
	inMeta *metav1.ObjectMeta,
	out *VirtualMachineClassInstance,
	down func(*VirtualMachineClassSpec, *S, apiconversion.Scope) error,
	up func(*S, *VirtualMachineClassSpec, apiconversion.Scope) error,
	s apiconversion.Scope,
) error {
	var preserved VirtualMachineClassInstance
	ok, err := restoreClassData(inMeta, &out.ObjectMeta, &preserved, "VMClassInstance")
	if err != nil || !ok {
		return err
	}
	return restoreClassSpec(&out.Spec.VirtualMachineClassSpec, &preserved.Spec.VirtualMachineClassSpec, down, up, s)
}

// PreserveClassConversionData preserves policies missing from an older API and
// copies metadata before adding the conversion annotation.
func PreserveClassConversionData(in *VirtualMachineClass, outMeta *metav1.ObjectMeta) error {
	return preserveClassData(in, &in.ObjectMeta, outMeta, "VMClass")
}

// PreserveClassInstanceConversionData preserves the immutable class snapshot
// for an older API without modifying the source metadata.
func PreserveClassInstanceConversionData(in *VirtualMachineClassInstance, outMeta *metav1.ObjectMeta) error {
	return preserveClassData(in, &in.ObjectMeta, outMeta, "VMClassInstance")
}

func preserveClassData(in metav1.Object, inMeta, outMeta *metav1.ObjectMeta, kind string) error {
	*outMeta = *inMeta.DeepCopy()
	if err := utilconversion.MarshalData(in, outMeta); err != nil {
		return fmt.Errorf("preserve %s conversion data: %w", kind, err)
	}
	return nil
}

func restoreClassData(inMeta, outMeta *metav1.ObjectMeta, preserved any, kind string) (bool, error) {
	*outMeta = *inMeta.DeepCopy()
	ok, err := utilconversion.UnmarshalData(outMeta, preserved)
	if len(outMeta.Annotations) == 0 {
		outMeta.Annotations = nil
	}
	if err != nil {
		return false, fmt.Errorf("restore %s conversion data: %w", kind, err)
	}
	return ok, nil
}

func restoreClassSpec[S any](
	out, preserved *VirtualMachineClassSpec,
	down func(*VirtualMachineClassSpec, *S, apiconversion.Scope) error,
	up func(*S, *VirtualMachineClassSpec, apiconversion.Scope) error,
	s apiconversion.Scope,
) error {
	var spoke S
	if err := down(preserved, &spoke, s); err != nil {
		return err
	}
	var projected VirtualMachineClassSpec
	if err := up(&spoke, &projected, s); err != nil {
		return err
	}
	RestoreClassPolicies(out, preserved, &projected)
	return nil
}
