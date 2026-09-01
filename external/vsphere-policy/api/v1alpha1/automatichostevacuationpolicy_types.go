// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AutomaticHostEvacuationPolicySpec defines the desired state of
// AutomaticHostEvacuationPolicy.
type AutomaticHostEvacuationPolicySpec struct {
	// +optional

	// Description specifies the desired description of the policy.
	Description string `json:"description,omitempty"`

	// +mandatory

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

// AutomaticHostEvacuationPolicyStatus defines the observed state of
// AutomaticHostEvacuationPolicy.
type AutomaticHostEvacuationPolicyStatus struct {
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

// AutomaticHostEvacuationPolicy is the schema for the
// AutomaticHostEvacuationPolicy API and represents the desired state and
// observed status of an AutomaticHostEvacuationPolicy resource.
// Workloads associated with this policy are automatically powered off by DRS
// when they cannot be evacuated from a host entering maintenance mode.
type AutomaticHostEvacuationPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AutomaticHostEvacuationPolicySpec   `json:"spec,omitempty"`
	Status AutomaticHostEvacuationPolicyStatus `json:"status,omitempty"`
}

// GetConditions returns the conditions associated with the policy.
func (p *AutomaticHostEvacuationPolicy) GetConditions() []metav1.Condition {
	return p.Status.Conditions
}

// SetConditions sets the conditions associated with the policy.
func (p *AutomaticHostEvacuationPolicy) SetConditions(conditions []metav1.Condition) {
	p.Status.Conditions = conditions
}

// GetMatchSpec returns the policy's match criteria.
func (p *AutomaticHostEvacuationPolicy) GetMatchSpec() *MatchSpec {
	return p.Spec.Match
}

// GetEnforcementMode returns the policy's enforcement mode.
func (p *AutomaticHostEvacuationPolicy) GetEnforcementMode() PolicyEnforcementMode {
	return p.Spec.EnforcementMode
}

// GetPolicyTags returns the names of the TagPolicy objects associated with
// this policy.
func (p *AutomaticHostEvacuationPolicy) GetPolicyTags() []string {
	return p.Spec.Tags
}

// GetConditions returns the conditions associated with the policy status.
func (p *AutomaticHostEvacuationPolicyStatus) GetConditions() []metav1.Condition {
	return p.Conditions
}

// SetConditions sets the conditions associated with the policy status.
func (p *AutomaticHostEvacuationPolicyStatus) SetConditions(conditions []metav1.Condition) {
	p.Conditions = conditions
}

// +kubebuilder:object:root=true

// AutomaticHostEvacuationPolicyList contains a list of
// AutomaticHostEvacuationPolicy objects.
type AutomaticHostEvacuationPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AutomaticHostEvacuationPolicy `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &AutomaticHostEvacuationPolicy{}, &AutomaticHostEvacuationPolicyList{})
}
