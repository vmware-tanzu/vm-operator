// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha6

import (
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
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
