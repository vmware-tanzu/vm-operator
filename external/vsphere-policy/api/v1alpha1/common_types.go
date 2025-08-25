// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ReadyConditionType is the Ready condition type that summarizes the
	// operational state of an API.
	ReadyConditionType = "Ready"
)

// LocalObjectRef describes a reference to another object in the same
// namespace as the referrer.
type LocalObjectRef struct {
	// APIVersion defines the versioned schema of this representation of an
	// object. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources
	APIVersion string `json:"apiVersion"`

	// Kind is a string value representing the REST resource this object
	// represents.
	// Servers may infer this from the endpoint the client submits requests to.
	// Cannot be updated.
	// In CamelCase.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	Kind string `json:"kind"`

	// Name refers to a unique resource in the current namespace.
	// More info: http://kubernetes.io/docs/user-guide/identifiers#names
	Name string `json:"name"`
}

// +kubebuilder:validation:Enum=Pod;VirtualMachine

// WorkloadType represents a possible workload type.
type WorkloadType string

const (
	WorkloadTypePod            WorkloadType = "Pod"
	WorkloadTypeVirtualMachine WorkloadType = "VirtualMachine"
)

// +kubebuilder:validation:Enum=Mandatory;Optional

// PolicyType represents a possible policy type.
type PolicyType string

const (
	PolicyTypeMandatory PolicyType = "Mandatory"
	PolicyTypeOptional  PolicyType = "Optional"
)

// +kubebuilder:validation:Enum=Darwin;Linux;Netware;Other;Solaris;Windows

// GuestFamilyType represents a possible guest family type.
type GuestFamilyType string

const (
	GuestFamilyTypeDarwin  GuestFamilyType = "Darwin"
	GuestFamilyTypeLinux   GuestFamilyType = "Linux"
	GuestFamilyTypeNetware GuestFamilyType = "Netware"
	GuestFamilyTypeOther   GuestFamilyType = "Other"
	GuestFamilyTypeSolaris GuestFamilyType = "Solaris"
	GuestFamilyTypeWindows GuestFamilyType = "Windows"
)

// ToVimType returns the Vim identifier for the GuestFamilyType.
func (t GuestFamilyType) ToVimType() string {
	switch t {
	case GuestFamilyTypeDarwin:
		return "darwinGuestFamily"
	case GuestFamilyTypeLinux:
		return "linuxGuest"
	case GuestFamilyTypeNetware:
		return "netwareGuest"
	case GuestFamilyTypeOther:
		return "otherGuestFamily"
	case GuestFamilyTypeSolaris:
		return "solarisGuest"
	case GuestFamilyTypeWindows:
		return "windowsGuest"
	}
	return string(t)
}

// FromVimGuestFamily returns the GuestFamilyType from the Vim identifier.
func FromVimGuestFamily(t string) GuestFamilyType {
	switch t {
	case "darwinGuestFamily":
		return GuestFamilyTypeDarwin
	case "linuxGuest":
		return GuestFamilyTypeLinux
	case "netwareGuest":
		return GuestFamilyTypeNetware
	case "otherGuestFamily":
		return GuestFamilyTypeOther
	case "solarisGuest":
		return GuestFamilyTypeSolaris
	case "windowsGuest":
		return GuestFamilyTypeWindows
	}
	return ""
}

// +kubebuilder:validation:Enum=Equal;NotEqual
type ValueSelectorOperator string

const (
	ValueSelectorOpEqual    ValueSelectorOperator = "Equal"
	ValueSelectorOpNotEqual ValueSelectorOperator = "NotEqual"
)

type StringMatcherSpec struct {
	// +optional
	// +kubebuilder:default=Equal

	// Op describes the operation performed against the specified value.
	Op ValueSelectorOperator `json:"op,omitempty"`

	// +optional

	// Value describes the subject of the evaluation.
	Value string `json:"value,omitempty"`
}

type GuestFamilyMatcherSpec struct {
	// +optional
	// +kubebuilder:default=Equal

	// Op describes the operation performed against the specified value.
	Op ValueSelectorOperator `json:"op,omitempty"`

	// +optional

	// Value describes the subject of the evaluation.
	Value GuestFamilyType `json:"value,omitempty"`
}

type MatchGuestSpec struct {
	// +optional

	// GuestID matches the workload's guest ID.
	//
	// Please the following location for the value values:
	// https://developer.broadcom.com/xapis/vsphere-web-services-api/latest/vim.vm.GuestOsDescriptor.GuestOsIdentifier.html.
	GuestID *StringMatcherSpec `json:"guestID,omitempty"`

	// +optional

	// GuestFamily matches the workload's guest family.
	// Valid values are: Darwin, Linux, Netware, Other, Solaris, and Windows.
	GuestFamily *GuestFamilyMatcherSpec `json:"guestFamily,omitempty"`
}

type MatchWorkloadSpec struct {
	// +optional

	// Guest matches information about the workload's guest operating system.
	Guest *MatchGuestSpec `json:"guest,omitempty"`

	// +optional

	// Labels matches labels on the workload in question.
	Labels []metav1.LabelSelectorRequirement `json:"labels,omitempty"`
}

type MatchImageSpec struct {
	// +optional

	// Name matches the image name.
	Name *StringMatcherSpec `json:"name,omitempty"`

	// +optional

	// Labels matches labels on the image.
	Labels []metav1.LabelSelectorRequirement `json:"labels,omitempty"`
}

// +kubebuilder:validation:Enum=And;Or
type MatchesBoolean string

const (
	MatchesBooleanAnd MatchesBoolean = "And"
	MatchesBooleanOr  MatchesBoolean = "Or"
)

type MatchSpec struct {
	// +optional
	// +kubebuilder:default=And

	// Op describes the boolean operation used to evaluate the elements from
	// the match field.
	//
	// Please note, this field does *not* apply to the image and workload
	// fields. They are *always* boolean and'd together with the results of the
	// match field.
	//
	// The default operation is a boolean and.
	Op MatchesBoolean `json:"operation,omitempty"`

	// +optional

	// Match describes additional matchers that are evaluated using the boolean
	// operation described by the operation field.
	Match []MatchSpec `json:"match,omitempty"`

	// +optional

	// Image matches information about the image used by the workload.
	Image *MatchImageSpec `json:"image,omitempty"`

	// +optional

	// Workload matches information about the workload.
	Workload *MatchWorkloadSpec `json:"workload,omitempty"`
}
