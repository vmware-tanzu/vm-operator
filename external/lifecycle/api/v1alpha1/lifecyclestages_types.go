// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LifecycleStages is the Schema for the lifecyclestages API.
//
// It is the cluster-scoped catalog of the stages one target (group, kind)
// exposes to the lifecycle framework. The operator that owns the target
// publishes it; the lifecycle framework reads it to validate LifecycleHooks and
// to build LifecycleStates.
//
// VM Operator writes the instance for VirtualMachine on startup.
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Group",type="string",JSONPath=".spec.target.group",description="Target group"
// +kubebuilder:printcolumn:name="Kind",type="string",JSONPath=".spec.target.kind",description="Target kind"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time since creation"
type LifecycleStages struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec LifecycleStagesSpec `json:"spec,omitempty"`
}

// LifecycleStagesSpec is the declared set of stages for one target.
type LifecycleStagesSpec struct {
	// +kubebuilder:validation:Required
	Target LifecycleStagesTarget `json:"target"`

	// Stages lists the stages the target exposes, in no particular order.
	// +kubebuilder:validation:MinItems=1
	// +listType=map
	// +listMapKey=name
	Stages []LifecycleStageDefinition `json:"stages"`
}

// LifecycleStagesTarget identifies the (group, kind) the stages belong to.
type LifecycleStagesTarget struct {
	// +kubebuilder:validation:Required
	Group string `json:"group"`

	// +kubebuilder:validation:Required
	Kind string `json:"kind"`
}

// LifecycleStageDefinition declares one stage a target exposes.
type LifecycleStageDefinition struct {
	// Name is the stage name referenced by LifecycleHook.spec.stage and
	// LifecycleState.spec.stages[].name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Description is a human-readable summary of what the stage pauses before.
	// +optional
	Description string `json:"description,omitempty"`

	// Type is how often the stage is reached for one target, ex. Reentrant.
	// +optional
	Type string `json:"type,omitempty"`

	// Blocking indicates whether the target waits for the stage's hooks.
	// +optional
	Blocking bool `json:"blocking,omitempty"`

	// EventName is the Event the target emits after the stage's action, if
	// any.
	// +optional
	EventName string `json:"eventName,omitempty"`

	// ConditionName is the condition on the target that reflects the stage's
	// hook status.
	// +optional
	ConditionName string `json:"conditionName,omitempty"`
}

// LifecycleStagesList contains a list of LifecycleStages.
// +kubebuilder:object:root=true
type LifecycleStagesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LifecycleStages `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &LifecycleStages{}, &LifecycleStagesList{})
}
