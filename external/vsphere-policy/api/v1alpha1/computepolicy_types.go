// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ComputePolicySpec defines the desired state of ComputePolicy.
type ComputePolicySpec struct {
	// +optional

	// Description specifies the desired description of the policy.
	Description string `json:"description,omitempty"`

	// +optional

	// PolicyID specifies the ID of the underlying vSphere compute policy, if
	// one is associated with this IaaS object.
	PolicyID string `json:"policyID,omitempty"`

	// +optional
	// +kubebuilder:default=Mandatory

	// EnforcementMode specifies how the policy is enforced.
	//
	// The valid modes include: Mandatory and Optional.
	EnforcementMode PolicyEnforcementMode `json:"enforcementMode,omitempty"`

	// +optional

	// Match is used to match workloads to which this policy should be applied.
	//
	// A mandatory policy with this field unset is applied to all workloads in
	// the namespace.
	Match *MatchSpec `json:"match,omitempty"`

	// +optional
	// +listType=set

	// Tags specifies the names of the TagPolicy objects in the same namespace
	// that contain the information about the vSphere tags used to activate this
	// policy.
	Tags []string `json:"tags,omitempty"`
}

// ComputePolicyStatus defines the observed state of ComputePolicy.
type ComputePolicyStatus struct {
	// +optional

	// ObservedGeneration describes the value of the metadata.generation field
	// the last time this object was reconciled by its primary controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional

	// Conditions describes any conditions associated with this object.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:storageversion:true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Enforcement-Mode",type="string",JSONPath=".spec.enforcementMode"
// +kubebuilder:printcolumn:name="Description",type="string",JSONPath=".spec.description"

// ComputePolicy is the schema for the ComputePolicy API and
// represents the desired state and observed status of a ComputePolicy
// resource.
type ComputePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComputePolicySpec   `json:"spec,omitempty"`
	Status ComputePolicyStatus `json:"status,omitempty"`
}

func (p ComputePolicy) GetConditions() []metav1.Condition {
	return p.Status.Conditions
}

func (p *ComputePolicy) SetConditions(conditions []metav1.Condition) {
	p.Status.Conditions = conditions
}

func (p ComputePolicyStatus) GetConditions() []metav1.Condition {
	return p.Conditions
}

func (p *ComputePolicyStatus) SetConditions(conditions []metav1.Condition) {
	p.Conditions = conditions
}

// +kubebuilder:object:root=true

// ComputePolicyList contains a list of ComputePolicy objects.
type ComputePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComputePolicy `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &ComputePolicy{}, &ComputePolicyList{})
}
