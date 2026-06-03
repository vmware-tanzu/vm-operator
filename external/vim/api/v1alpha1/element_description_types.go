// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

// ElementDescriptionType identifies the concrete runtime type of an
// element description. The values correspond to vSphere API class names
// (e.g. vim.ElementDescription, vim.ExtendedElementDescription).
type ElementDescriptionType string

const (
	// ElementDescriptionTypeBase identifies a base vim.ElementDescription.
	ElementDescriptionTypeBase ElementDescriptionType = "ElementDescription"

	// ElementDescriptionTypeExtended identifies a
	// vim.ExtendedElementDescription.
	ElementDescriptionTypeExtended ElementDescriptionType = "ExtendedElementDescription"

	// ElementDescriptionTypeEVCMode identifies a vim.EVCMode.
	ElementDescriptionTypeEVCMode ElementDescriptionType = "EVCMode"

	// ElementDescriptionTypeFeatureEVCMode identifies a vim.FeatureEVCMode.
	ElementDescriptionTypeFeatureEVCMode ElementDescriptionType = "FeatureEVCMode"

	// ElementDescriptionTypeOptionDef identifies a vim.OptionDef.
	ElementDescriptionTypeOptionDef ElementDescriptionType = "OptionDef"
)

////////////////////////////////////////////////////////////////////////////////
//
// Please note, the following validation rule is not supported due to the
// amount of complexity it introduces. To verify this:
//
// 1. Replace the leading "-" with "+"
// 2. Run `make generate`
// 3. Run `ginkgo -v ./controllers/virtualmachineclass`
//
// This will result in multiple errors related to loading the CRDs.
//
// -kubebuilder:validation:XValidation:rule="[has(self.extendedElementDescription), has(self.evcMode), has(self.featureEvcMode), has(self.optionDef)].filter(x, x).size() <= 1",message="at most one of extendedElementDescription, evcMode, featureEvcMode, or optionDef may be specified"
//
////////////////////////////////////////////////////////////////////////////////

// ElementDescription maps the polymorphic vim.ElementDescription type
// hierarchy to a Kubernetes-compatible flat structure.
// It corresponds to vim.ElementDescription and its subtypes.
type ElementDescription struct {
	// Key is the enumeration or literal ID being described.
	Key string `json:"key"`

	// Label is the display label.
	Label string `json:"label"`

	// Summary is the summary description.
	Summary string `json:"summary"`

	// Type is the type of the element description.
	Type ElementDescriptionType `json:"type"`

	// +optional

	// ExtendedElementDescription contains data specific to a
	// vim.ExtendedElementDescription.
	ExtendedElementDescription *ExtendedElementDescription `json:"extendedElementDescription,omitempty"`

	// +optional

	// EVCMode contains data specific to a vim.EVCMode.
	EVCMode *EVCMode `json:"evcMode,omitempty"`

	// +optional

	// FeatureEVCMode contains data specific to a vim.FeatureEVCMode.
	FeatureEVCMode *FeatureEVCMode `json:"featureEvcMode,omitempty"`

	// +optional

	// OptionDef contains data specific to a vim.OptionDef.
	OptionDef *OptionDef `json:"optionDef,omitempty"`
}

// ExtendedElementDescription contains the fields that extend
// vim.ElementDescription in vim.ExtendedElementDescription.
// It corresponds to vim.ExtendedElementDescription.
type ExtendedElementDescription struct {
	// MessageCatalogKeyPrefix is the key to the localized message string in
	// the catalog. The label and summary in the parent BaseElementDescription
	// correspond to catalog entries at "<key>.label" and "<key>.summary"
	// respectively.
	MessageCatalogKeyPrefix string `json:"messageCatalogKeyPrefix"`

	// +optional

	// MessageArg provides named arguments used to substitute parameters in
	// the localized message string.
	MessageArg []KeyAnyValue `json:"messageArg,omitempty"`
}

// EVCMode contains the fields that extend vim.ElementDescription in
// vim.EVCMode.
// It corresponds to vim.EVCMode.
type EVCMode struct {
	// Vendor is the CPU hardware vendor required for this mode.
	Vendor string `json:"vendor"`

	// VendorTier is the ordering index for the set of modes that apply to a
	// given CPU vendor. Use this only for feature-superset comparisons, not
	// to infer specific feature presence.
	VendorTier int32 `json:"vendorTier"`

	// Track contains identifiers for feature groups that are at least
	// partially present in the guaranteed features for this mode. Use this
	// only for feature-superset comparisons, not to infer specific feature
	// presence.
	Track []string `json:"track"`

	// +optional

	// FeatureCapability describes the feature capability baseline guaranteed
	// on a cluster where this EVC mode is configured.
	FeatureCapability []HostFeatureCapability `json:"featureCapability,omitempty"`

	// +optional

	// FeatureMask contains the masks that limit a host's capabilities to the
	// EVC mode baseline.
	FeatureMask []HostFeatureMask `json:"featureMask,omitempty"`

	// +optional

	// FeatureRequirement contains the host feature capability conditions that
	// must be met for the EVC mode baseline.
	FeatureRequirement []VirtualMachineFeatureRequirement `json:"featureRequirement,omitempty"`

	// +optional

	// GuaranteedCPUFeatures describes the CPU feature baseline for this EVC
	// mode.
	//
	// Deprecated: As of vSphere API 6.5, use FeatureCapability instead.
	GuaranteedCPUFeatures []HostCpuIdInfo `json:"guaranteedCPUFeatures,omitempty"`
}

// FeatureEVCMode contains the fields that extend vim.ElementDescription in
// vim.FeatureEVCMode.
// It corresponds to vim.FeatureEVCMode.
type FeatureEVCMode struct {
	// +optional

	// Capability describes the feature capability baseline guaranteed on a
	// cluster where this EVC mode is configured.
	Capability []HostFeatureCapability `json:"capability,omitempty"`

	// +optional

	// Mask contains the masks that limit a host's capabilities to the EVC
	// mode baseline.
	Mask []HostFeatureMask `json:"mask,omitempty"`

	// +optional

	// Requirement contains the host feature capability conditions that must
	// be met for the EVC mode baseline.
	Requirement []VirtualMachineFeatureRequirement `json:"requirement,omitempty"`
}
