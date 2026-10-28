// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha6

const (
	// ReadyConditionType is the Ready condition type that summarizes the operational state of a VM Operator API object.
	ReadyConditionType = "Ready"

	// VirtualMachineConditionLifecycleHookReady reflects the last lifecycle
	// stage with hooks that VM Operator evaluated: Create, PowerStateChange,
	// Delete or ResourceDelete. It is True when the stage's hooks finished
	// without failure or the hold was abandoned, False while the stage is
	// blocked or a hook failed, and Unknown when VM Operator could not
	// evaluate the stage's hooks (the action stays held and is retried). The
	// condition is absent until a stage with hooks is reached, and never
	// written for a VM in a namespace with no hooks; consumers treat absent
	// as True.
	VirtualMachineConditionLifecycleHookReady = "LifecycleHookReady"

	// VirtualMachineLifecycleHookReadyCreateStageReason is the
	// VirtualMachineConditionLifecycleHookReady reason when the Create stage
	// was the last one evaluated.
	VirtualMachineLifecycleHookReadyCreateStageReason = "CreateStage"

	// VirtualMachineLifecycleHookReadyPowerStateChangeStageReason is the
	// VirtualMachineConditionLifecycleHookReady reason when the
	// PowerStateChange stage was the last one evaluated.
	VirtualMachineLifecycleHookReadyPowerStateChangeStageReason = "PowerStateChangeStage"

	// VirtualMachineLifecycleHookReadyDeleteStageReason is the
	// VirtualMachineConditionLifecycleHookReady reason when the Delete stage
	// was the last one evaluated.
	VirtualMachineLifecycleHookReadyDeleteStageReason = "DeleteStage"

	// VirtualMachineLifecycleHookReadyResourceDeleteStageReason is the
	// VirtualMachineConditionLifecycleHookReady reason when the
	// ResourceDelete stage was the last one evaluated.
	VirtualMachineLifecycleHookReadyResourceDeleteStageReason = "ResourceDeleteStage"
)
