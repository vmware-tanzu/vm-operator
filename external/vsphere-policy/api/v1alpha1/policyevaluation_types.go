// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PolicyEvaluationGuestSpec struct {
	// +optional

	// GuestID describes the guest ID of the workload.
	//
	// Please the following location for the valid values:
	// https://developer.broadcom.com/xapis/vsphere-web-services-api/latest/vim.vm.GuestOsDescriptor.GuestOsIdentifier.html.
	GuestID string `json:"guestID,omitempty"`

	// +optional

	// GuestFamily describes the guest family of the workload.
	//
	// Valid values are: Darwin, Linux, Netware, Other, Solaris, and Windows.
	GuestFamily GuestFamilyType `json:"guestFamily,omitempty"`
}
type PolicyEvaluationImageSpec struct {
	// +optional

	// Name describes the name of the workload's image.
	Name string `json:"name,omitempty"`

	// +optional

	// Labels describe the labels from the workload's image.
	Labels map[string]string `json:"labels,omitempty"`
}

type PolicyEvaluationWorkloadSpec struct {
	// +optional

	// Guest describes information about the workload's guest.
	Guest *PolicyEvaluationGuestSpec `json:"guest,omitempty"`

	// +optional

	// Labels describe the labels from the workload.
	Labels map[string]string `json:"labels,omitempty"`
}

// PolicyEvaluationSpec defines the desired state of PolicyEvaluation.
type PolicyEvaluationSpec struct {
	// +optional

	// Image describes information about the image used by the workload.
	Image *PolicyEvaluationImageSpec `json:"image,omitempty"`

	// +optional

	// Workload descriptions information about the workload.
	Workload *PolicyEvaluationWorkloadSpec `json:"workload,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=name
	// +listMapKey=kind

	// Policies describes the policies that should be explicitly considered as
	// part of this evaluation.
	Policies []LocalObjectRef `json:"policies,omitempty"`
}

type PolicyEvaluationResult struct {
	// +required

	// APIVersion describes the API version of the policy object to apply.
	APIVersion string `json:"apiVersion"`

	// +required

	// Kind describes the kind of the policy object to apply.
	Kind string `json:"kind"`

	// +required

	// Name describes the name of the policy object to apply.
	Name string `json:"name"`

	// +optional

	// Generation describes the value of the metadata.generation field
	// of the policy object when it was applied.
	Generation int64 `json:"observedGeneration,omitempty"`

	// +optional
	// +listType=set

	// Tags specifies the UUIDs of any vSphere tags that may be required to
	// activate the policy.
	Tags []string `json:"tags,omitempty"`
}

// PolicyEvaluationStatus defines the observed state of PolicyEvaluation.
type PolicyEvaluationStatus struct {
	// +optional

	// ObservedGeneration describes the value of the metadata.generation field
	// the last time this object was reconciled by its primary controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=name
	// +listMapKey=kind

	// Policies specifies the list of policies to apply.
	Policies []PolicyEvaluationResult `json:"policies,omitempty"`

	// +optional

	// Conditions describes any conditions associated with this object.
	//
	// The Ready condition will be present once this request has been evaluated.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=policyeval
// +kubebuilder:storageversion:true
// +kubebuilder:subresource:status

// PolicyEvaluation is the schema for the PolicyEvaluation API and
// represents the desired state and observed status of a PolicyEvaluation
// resource.
type PolicyEvaluation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PolicyEvaluationSpec   `json:"spec,omitempty"`
	Status PolicyEvaluationStatus `json:"status,omitempty"`
}

func (p PolicyEvaluation) GetConditions() []metav1.Condition {
	return p.Status.Conditions
}

func (p *PolicyEvaluation) SetConditions(conditions []metav1.Condition) {
	p.Status.Conditions = conditions
}

func (p PolicyEvaluationStatus) GetConditions() []metav1.Condition {
	return p.Conditions
}

func (p *PolicyEvaluationStatus) SetConditions(conditions []metav1.Condition) {
	p.Conditions = conditions
}

// +kubebuilder:object:root=true

// PolicyEvaluationList contains a list of PolicyEvaluation objects.
type PolicyEvaluationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PolicyEvaluation `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &PolicyEvaluation{}, &PolicyEvaluationList{})
}
