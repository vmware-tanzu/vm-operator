// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LifecycleSubscribedStages is the Schema for the lifecyclesubscribedstages API.
//
// It records, for one (namespace, target group, target kind), the stages that
// at least one live LifecycleHook subscribes to. It is written only by the
// lifecycle framework; absence means no stage is subscribed. Target identity is
// carried by the target-group and target-kind labels.
// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=lss
// +kubebuilder:printcolumn:name="Group",type="string",JSONPath=".metadata.labels['lifecycle\\.vcfa\\.vmware\\.com/target-group']",description="Target group from labels"
// +kubebuilder:printcolumn:name="Kind",type="string",JSONPath=".metadata.labels['lifecycle\\.vcfa\\.vmware\\.com/target-kind']",description="Target kind from labels"
// +kubebuilder:printcolumn:name="Stages",type="string",JSONPath=".spec.stages",description="Subscribed stages"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time since creation"
type LifecycleSubscribedStages struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec LifecycleSubscribedStagesSpec `json:"spec,omitempty"`
}

// LifecycleSubscribedStagesSpec is the framework-computed set of subscribed stages.
type LifecycleSubscribedStagesSpec struct {
	// Stages is never empty: an empty set is represented by the object not existing.
	// +kubebuilder:validation:MinItems=1
	// +listType=set
	// +kubebuilder:validation:items:MinLength=1
	Stages []string `json:"stages"`
}

// LifecycleSubscribedStagesList contains a list of LifecycleSubscribedStages.
// +kubebuilder:object:root=true
type LifecycleSubscribedStagesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LifecycleSubscribedStages `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &LifecycleSubscribedStages{}, &LifecycleSubscribedStagesList{})
}
