// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RequiredDuringExecutionVMPlacementPolicySpec defines the desired state of
// RequiredDuringExecutionVMPlacementPolicy.
type RequiredDuringExecutionVMPlacementPolicySpec struct {
	// +optional

	// Description specifies the desired description of the policy.
	Description string `json:"description,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced

// RequiredDuringExecutionVMPlacementPolicy is the schema for the
// RequiredDuringExecutionVMPlacementPolicy API.
//
// The presence of RequiredDuringExecutionVMPlacementPolicy in a
// namespace allows VM workloads to be eligible for
// requiredDuringExecution Affinity and Anti-Affinity Policies.
type RequiredDuringExecutionVMPlacementPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec RequiredDuringExecutionVMPlacementPolicySpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// RequiredDuringExecutionVMPlacementPolicyList contains a list of
// RequiredDuringExecutionVMPlacementPolicy objects.
type RequiredDuringExecutionVMPlacementPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RequiredDuringExecutionVMPlacementPolicy `json:"items"`
}

func init() {
	objectTypes = append(objectTypes,
		&RequiredDuringExecutionVMPlacementPolicy{},
		&RequiredDuringExecutionVMPlacementPolicyList{})
}
