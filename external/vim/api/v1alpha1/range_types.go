// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import "k8s.io/apimachinery/pkg/api/resource"

// HardwareVersionRange describes a range of hardware versions with min
// and max.
type HardwareVersionRange struct {
	// +optional

	// Min is the minimum value.
	// A zero value means there is no minimum.
	Min HardwareVersion `json:"min,omitempty"`

	// +optional

	// Max is the maximum value.
	// A zero value means there is no maximum.
	Max HardwareVersion `json:"max,omitempty"`
}

// IntRange describes a range of 32-bit integer values with min and max.
type IntRange struct {
	// +required

	// Min is the minimum value.
	Min int32 `json:"min"`

	// +required

	// Max is the maximum value.
	Max int32 `json:"max"`
}

// LongRange describes a range of 64-bit integer values with min and
// max.
type LongRange struct {
	// +required

	// Min is the minimum value.
	Min int64 `json:"min"`

	// +required

	// Max is the maximum value.
	Max int64 `json:"max"`
}

// ResourceQuantityRange describes a range of resource.Quantity values with
// min and max.
type ResourceQuantityRange struct {
	// +required

	// Min is the minimum value.
	Min resource.Quantity `json:"min"`

	// +required

	// Max is the maximum value.
	Max resource.Quantity `json:"max"`
}
