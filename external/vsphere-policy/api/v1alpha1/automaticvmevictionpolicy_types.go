// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AutomaticVMEvictionPolicySpec defines the desired state of
// AutomaticVMEvictionPolicy.
type AutomaticVMEvictionPolicySpec struct {
	// +optional
	// +kubebuilder:validation:MaxLength=1024

	// Description specifies the desired description of the policy.
	Description string `json:"description,omitempty"`

	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=64

	// PolicyID specifies the ID of the underlying vSphere compute policy, if
	// one is associated with this IaaS object.
	PolicyID string `json:"policyID,omitempty"`

	// +optional
	// +kubebuilder:default=Mandatory

	// EnforcementMode specifies how the policy is enforced.
	//
	// The valid modes include: Mandatory and Optional. Defaults to Mandatory.
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

// AutomaticVMEvictionPolicyStatus defines the observed state of
// AutomaticVMEvictionPolicy.
type AutomaticVMEvictionPolicyStatus struct {
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

// AutomaticVMEvictionPolicy is the schema for the
// AutomaticVMEvictionPolicy API and represents the desired state and
// observed status of an AutomaticVMEvictionPolicy resource.
// Workloads associated with this policy are automatically powered off by DRS
// when they cannot be evacuated from a host entering maintenance mode. The
// workload remains associated with that host and is not relocated; it is
// powered back on once the host exits maintenance mode.
type AutomaticVMEvictionPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AutomaticVMEvictionPolicySpec   `json:"spec,omitempty"`
	Status AutomaticVMEvictionPolicyStatus `json:"status,omitempty"`
}

// GetConditions returns the conditions associated with the policy.
func (p *AutomaticVMEvictionPolicy) GetConditions() []metav1.Condition {
	return p.Status.Conditions
}

// SetConditions sets the conditions associated with the policy.
func (p *AutomaticVMEvictionPolicy) SetConditions(conditions []metav1.Condition) {
	p.Status.Conditions = conditions
}

// GetConditions returns the conditions associated with the policy status.
func (p *AutomaticVMEvictionPolicyStatus) GetConditions() []metav1.Condition {
	return p.Conditions
}

// SetConditions sets the conditions associated with the policy status.
func (p *AutomaticVMEvictionPolicyStatus) SetConditions(conditions []metav1.Condition) {
	p.Conditions = conditions
}

// +kubebuilder:object:root=true

// AutomaticVMEvictionPolicyList contains a list of
// AutomaticVMEvictionPolicy objects.
type AutomaticVMEvictionPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AutomaticVMEvictionPolicy `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &AutomaticVMEvictionPolicy{}, &AutomaticVMEvictionPolicyList{})
}
