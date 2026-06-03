// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import "k8s.io/apimachinery/pkg/api/resource"

// BaseOptionType is the base type for all option types, providing common
// read-only metadata.
// It corresponds to vim.option.OptionType.
type BaseOptionType struct {
	// +optional

	// ValueIsReadonly indicates whether or not a user can modify a value
	// belonging to this option type.
	ValueIsReadonly *bool `json:"valueIsReadonly,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="[has(self.bool), has(self.int), has(self.long), has(self.string),has(self.resourceQuantity), has(self.choice)].filter(x, x).size() <= 1",message="at most one of bool, int, long, string, resourceQuantity, or choice may be specified"

// OptionType maps the polymorphic option type hierarchy to a
// Kubernetes-compatible flat structure.
// It corresponds to vim.option.OptionType.
type OptionType struct {
	// +optional

	// Bool contains a boolean option.
	Bool *BoolOption `json:"bool,omitempty"`

	// +optional

	// Int contains an integer range option.
	Int *IntOption `json:"int,omitempty"`

	// +optional

	// Long contains a long integer range option.
	Long *LongOption `json:"long,omitempty"`

	// +optional

	// String contains a string option.
	String *StringOption `json:"string,omitempty"`

	// +optional

	// ResourceQuantity contains a resource quantity range option.
	ResourceQuantity *ResourceQuantityOption `json:"resourceQuantity,omitempty"`

	// +optional

	// Choice contains a string choice option.
	Choice *ChoiceOption `json:"choice,omitempty"`
}

// StringOption describes a string option with a default value and
// whether the option is supported.
// It corresponds to vim.option.StringOption.
type StringOption struct {
	BaseOptionType `json:",inline"`

	// +optional

	// Default is the default value for this option.
	Default string `json:"default,omitempty"`

	// +optional

	// ValidCharacters contains the set of valid characters. If a string option
	// is not specified, all strings are allowed.
	ValidCharacters string `json:"validCharacters,omitempty"`
}

// BoolOption describes a boolean option with a default value and
// whether the option is supported.
// It corresponds to vim.option.BoolOption.
type BoolOption struct {
	BaseOptionType `json:",inline"`

	// +optional

	// Default is the default value for this option.
	Default bool `json:"default,omitempty"`

	// +optional

	// Supported indicates whether this option is supported.
	Supported bool `json:"supported,omitempty"`
}

// IntOption describes a range of integer values with min, max, and default.
type IntOption struct {
	BaseOptionType `json:",inline"`
	IntRange       `json:",inline"`

	// +required

	// Default is the default value.
	Default int32 `json:"default"`
}

// LongOption describes a range of long values with min, max, and default.
type LongOption struct {
	BaseOptionType `json:",inline"`
	LongRange      `json:",inline"`

	// +required

	// Default is the default value.
	Default int64 `json:"default"`
}

// ResourceQuantityOption describes a range of resource.Quantity values with
// min, max, and default.
type ResourceQuantityOption struct {
	BaseOptionType        `json:",inline"`
	ResourceQuantityRange `json:",inline"`

	// +required

	// Default is the default value.
	Default resource.Quantity `json:"default"`
}

// ChoiceOption describes a set of valid string choices and the default
// choice for an option.
// It corresponds to vim.option.ChoiceOption.
type ChoiceOption struct {
	BaseOptionType `json:",inline"`

	// +kubebuilder:validation:MaxItems=32

	// Choices lists the valid values for this option.
	Choices []ElementDescription `json:"choices"`

	// +optional

	// DefaultIndex is the index of the default choice value.
	DefaultIndex int32 `json:"defaultIndex,omitempty"`
}

// OptionDef describes a single configuration option definition with its
// associated type.
// It corresponds to vim.option.OptionDef.
type OptionDef struct {
	// +required

	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:Type=object
	// +kubebuilder:pruning:PreserveUnknownFields

	// OptionType describes the type and valid values for this option.
	OptionType OptionType `json:"optionType"`
}

// HostFeatureCapability describes a feature that a host is capable of
// providing at a particular value.
// It corresponds to vim.host.FeatureCapability.
type HostFeatureCapability struct {
	// Key is the accessor name for the feature capability.
	Key string `json:"key"`

	// FeatureName is the name of the feature. Identical to Key.
	FeatureName string `json:"featureName"`

	// Value is the opaque value the feature is capable of.
	Value string `json:"value"`
}

// HostFeatureMask describes a mask applied to a host feature capability
// to enforce a specific value for EVC compatibility.
// It corresponds to vim.host.FeatureMask.
type HostFeatureMask struct {
	// Key is the accessor name for the feature mask.
	Key string `json:"key"`

	// FeatureName is the name of the feature. Identical to Key.
	FeatureName string `json:"featureName"`

	// Value is the opaque value to apply to the host feature capability.
	// The masking operation is encoded in the value.
	Value string `json:"value"`
}

// VirtualMachineFeatureRequirement describes a feature requirement for
// a virtual machine as a key/name/value triple.
// It corresponds to vim.vm.FeatureRequirement.
type VirtualMachineFeatureRequirement struct {
	// Key is the accessor name for the feature requirement test.
	Key string `json:"key"`

	// FeatureName is the name of the feature. Identical to Key.
	FeatureName string `json:"featureName"`

	// Value is the opaque value for the feature operation. The
	// operation is encoded in the value.
	Value string `json:"value"`
}

// HostCpuIdInfo represents CPU features of a host or the CPU feature
// requirements of a virtual machine, expressed as CPUID bit masks.
// It corresponds to vim.host.CpuIdInfo.
//
// Deprecated: As of vSphere API 5.1, use HostFeatureMask for host
// masking and for virtual machines with hardware version 9 or later.
type HostCpuIdInfo struct {
	// Level is the CPUID input level (EAX value passed to CPUID).
	Level int32 `json:"level"`

	// +optional

	// Vendor restricts this mask to a specific CPU vendor when set.
	Vendor string `json:"vendor,omitempty"`

	// +optional

	// Eax is the bit mask string for the EAX CPUID register.
	Eax string `json:"eax,omitempty"`

	// +optional

	// Ebx is the bit mask string for the EBX CPUID register.
	Ebx string `json:"ebx,omitempty"`

	// +optional

	// Ecx is the bit mask string for the ECX CPUID register.
	Ecx string `json:"ecx,omitempty"`

	// +optional

	// Edx is the bit mask string for the EDX CPUID register.
	Edx string `json:"edx,omitempty"`
}

// KeyAnyValue is a non-localized key/value pair where the value is
// represented as a string.
// It corresponds to vmodl.KeyAnyValue.
type KeyAnyValue struct {
	// Key is the key.
	Key string `json:"key"`

	// Value is the value, represented as a string.
	Value string `json:"value"`
}
