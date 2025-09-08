// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TagPolicySpec defines the desired state of TagPolicy.
type TagPolicySpec struct {
	// +optional
	// +listType=set

	// Tags specifies the UUIDs of the vSphere tags that are applied to a
	// workload as a result of this policy.
	Tags []string `json:"tags,omitempty"`
}

// TagPolicyStatus defines the observed state of TagPolicy.
type TagPolicyStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:storageversion:true
// +kubebuilder:subresource:status

// TagPolicy is the schema for the TagPolicy API and
// represents the desired state and observed status of a TagPolicy
// resource.
type TagPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TagPolicySpec   `json:"spec,omitempty"`
	Status TagPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TagPolicyList contains a list of TagPolicy objects.
type TagPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TagPolicy `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &TagPolicy{}, &TagPolicyList{})
}
