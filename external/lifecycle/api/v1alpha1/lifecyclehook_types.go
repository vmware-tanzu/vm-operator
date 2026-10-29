// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LifecycleHook is the Schema for the lifecyclehooks API.
//
// A LifecycleHook registers one actor's interest in pausing one stage of one
// target (group, kind) in a namespace. Lifecycle Operator fans out the union
// of all registered hooks for a stage into the status of the target's
// LifecycleState.
//
// VM Operator vendors this type for API completeness only. It does not read
// or list LifecycleHook directly; hook discovery is done via
// LifecycleSubscribedStages.
// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=lh
// +kubebuilder:printcolumn:name="Target Group",type="string",JSONPath=".spec.target.group",description="Target group"
// +kubebuilder:printcolumn:name="Target Kind",type="string",JSONPath=".spec.target.kind",description="Target kind"
// +kubebuilder:printcolumn:name="Stage",type="string",JSONPath=".spec.stage",description="Stage name"
// +kubebuilder:printcolumn:name="Executor",type="string",JSONPath=".spec.executor.name",description="Hook executor"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time since creation"
type LifecycleHook struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec LifecycleHookSpec `json:"spec,omitempty"`
}

// LifecycleHookSpec is the declared intent of a LifecycleHook.
type LifecycleHookSpec struct {
	// +kubebuilder:validation:Required
	Target LifecycleHookTarget `json:"target"`

	// +kubebuilder:validation:Required
	Stage string `json:"stage"`

	// +optional
	Executor LifecycleHookExecutor `json:"executor,omitempty"`

	// +optional
	TimeoutPolicy LifecycleHookTimeoutPolicy `json:"timeoutPolicy,omitempty"`
}

// LifecycleHookTarget identifies a (group, kind) pair that the hook targets.
type LifecycleHookTarget struct {
	// +kubebuilder:validation:Required
	Group string `json:"group"`

	// +kubebuilder:validation:Required
	Kind string `json:"kind"`
}

// LifecycleHookExecutor identifies who executes the hook.
type LifecycleHookExecutor struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// LifecycleHookTimeoutPolicy specifies the timeout for a hook.
type LifecycleHookTimeoutPolicy struct {
	// +optional
	Duration *metav1.Duration `json:"duration,omitempty"`

	// +optional
	Policy string `json:"policy,omitempty"`
}

// LifecycleHookList contains a list of LifecycleHook.
// +kubebuilder:object:root=true
type LifecycleHookList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LifecycleHook `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &LifecycleHook{}, &LifecycleHookList{})
}
