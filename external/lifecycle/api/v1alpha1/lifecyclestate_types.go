// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// LifecycleState is the Schema for the lifecyclestates API.
//
// Target identity is expressed via metadata: the owning workload is
// referenced by metadata.ownerReferences (binding) and by the
// metadata.labels[lifecycle.vcfa.vmware.com/target-group|target-kind]
// selectors (indexing). LifecycleState has no spec.target field.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=lstate
// +kubebuilder:printcolumn:name="Target",type="string",JSONPath=".metadata.labels['lifecycle\\.vcfa\\.vmware\\.com/target-kind']",description="Target kind from labels"
// +kubebuilder:printcolumn:name="Stages",type="integer",JSONPath=".status.stages[*].name",description="Number of stages tracked in status",priority=1
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time since creation"
type LifecycleState struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LifecycleStateSpec   `json:"spec,omitempty"`
	Status LifecycleStateStatus `json:"status,omitempty"`
}

// LifecycleStateSpec is the desired stage-level configuration of a
// LifecycleState. It carries no target identity; see the type comment
// on LifecycleState for how the target is referenced.
type LifecycleStateSpec struct {
	// +optional
	// +listType=map
	// +listMapKey=name
	Stages []StageSpec `json:"stages,omitempty"`
}

// StageSpec captures user-controlled intent for a single stage in the
// workflow: the hook set (indirectly, via LifecycleHook resources that
// target this stage) and the pause/resume switches honored by the
// LifecycleState controller.
//
// WorkflowPaused and WorkflowResumed encode a two-step handshake that
// allows a user to resume a paused workflow without racing the
// controller. Legal steady-state combinations are:
//
//   - {Paused=false, Resumed=false}: default; the stage is advancing or
//     has not yet been reached. This is the ONLY legal combination at
//     the moment a stage is introduced into spec.stages (CREATE of the
//     LifecycleState, or a stage added to spec.stages at UPDATE).
//     WorkflowPaused is controller-managed; WorkflowResumed is only
//     meaningful as an acknowledgement of an existing pause, so neither
//     may be pre-set at introduction.
//   - {Paused=true,  Resumed=false}: controller has paused at this
//     stage; the user has not yet acknowledged.
//   - {Paused=false, Resumed=true}:  user requests resume. This is the
//     handshake step: the user clears Paused and sets Resumed. It is
//     only valid as a transition from {Paused=true, Resumed=false} (or
//     idempotent re-apply from the same {false, true}); introducing a
//     stage in this combination is rejected.
//   - {Paused=true,  Resumed=true}:  invalid and rejected by the
//     validating webhook.
//
// Clearing Paused=true -> false directly (without also setting
// Resumed=true) is only allowed when HooksReady=True; otherwise the
// pause-cancel guard rejects the transition to prevent silent unblock.
// The full validation lives in v1alpha1.ValidateStageHandshake.
type StageSpec struct {
	// +kubebuilder:validation:Required
	Name            string `json:"name"`
	WorkflowPaused  bool   `json:"workflowPaused"`
	WorkflowResumed bool   `json:"workflowResumed"`
}

// LifecycleStateStatus is the controller-maintained view of the
// workflow. It is written exclusively by the LifecycleState controller
// via the status subresource.
type LifecycleStateStatus struct {
	// +optional
	// +listType=map
	// +listMapKey=name
	Stages []StageStatus `json:"stages,omitempty"`
}

// StageStatus summarizes progress for a single stage: the aggregate
// Conditions (WorkflowPaused, HooksReady) and the per-hook state map.
type StageStatus struct {
	Name string `json:"name"`

	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=lifecycleHookRef
	Hooks []HookStatus `json:"hooks,omitempty"`
}

// HookState is the lifecycle state of a single hook entry within a
// stage. The state machine is Pending -> InProgress -> (Succeeded|Failed).
type HookState string

const (
	// HookStatePending marks a hook that has been registered but has
	// not yet been observed by its owner.
	HookStatePending HookState = "Pending"
	// HookStateInProgress marks a hook whose owner has acknowledged it
	// and is actively working toward a terminal state.
	HookStateInProgress HookState = "InProgress"
	// HookStateSucceeded is a terminal state indicating the hook owner
	// completed its work successfully.
	HookStateSucceeded HookState = "Succeeded"
	// HookStateFailed is a terminal state indicating the hook owner
	// reported an unrecoverable failure or the hook timed out under
	// TimeoutPolicyFail.
	HookStateFailed HookState = "Failed"
)

// HookStatus is the controller-maintained status of a single hook
// entry within a stage.
type HookStatus struct {
	// +kubebuilder:validation:Required
	LifecycleHookRef string `json:"lifecycleHookRef"`

	// +kubebuilder:validation:Required
	LifecycleHookUID types.UID `json:"lifecycleHookUID"`

	// OwnedBy is the identity of the hook creator (copied from the
	// LifecycleHook's created-by annotation at initialization time).
	// Immutable after set.
	// +kubebuilder:validation:Required
	OwnedBy string `json:"ownedBy"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Pending;InProgress;Succeeded;Failed
	State HookState `json:"state"`

	// +optional
	Message string `json:"message,omitempty"`

	// +kubebuilder:validation:Required
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`
}

// LifecycleStateList contains a list of LifecycleState.
// +kubebuilder:object:root=true
type LifecycleStateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LifecycleState `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &LifecycleState{}, &LifecycleStateList{})
}
