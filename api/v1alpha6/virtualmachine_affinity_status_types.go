// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha6

// +kubebuilder:validation:Enum=Affinity;AntiAffinity

// VirtualMachineAffinityPolicyType is the kind of a VM Operator affinity
// placement policy.
type VirtualMachineAffinityPolicyType string

const (
	// VirtualMachineAffinityPolicyTypeAffinity indicates the policy keeps the
	// VM together with the VMs that carry the target labels.
	VirtualMachineAffinityPolicyTypeAffinity VirtualMachineAffinityPolicyType = "Affinity"

	// VirtualMachineAffinityPolicyTypeAntiAffinity indicates the policy keeps
	// the VM apart from the VMs that carry the target labels.
	VirtualMachineAffinityPolicyTypeAntiAffinity VirtualMachineAffinityPolicyType = "AntiAffinity"
)

// +kubebuilder:validation:Enum=RequiredDuringSchedulingRequiredDuringExecution;RequiredDuringSchedulingPreferredDuringExecution;PreferredDuringSchedulingPreferredDuringExecution

// VirtualMachineAffinityPolicyStrictness describes how strictly a policy is
// enforced. The values match the names of the spec.affinity term lists.
type VirtualMachineAffinityPolicyStrictness string

const (
	// VirtualMachineAffinityPolicyStrictnessRequiredDuringSchedulingRequiredDuringExecution
	// indicates the policy must be met during placement and when the VM is powered on.
	VirtualMachineAffinityPolicyStrictnessRequiredDuringSchedulingRequiredDuringExecution VirtualMachineAffinityPolicyStrictness = "RequiredDuringSchedulingRequiredDuringExecution"

	// VirtualMachineAffinityPolicyStrictnessRequiredDuringSchedulingPreferredDuringExecution
	// indicates the policy must be met during placement and is preferred
	// when the VM is powered on.
	VirtualMachineAffinityPolicyStrictnessRequiredDuringSchedulingPreferredDuringExecution VirtualMachineAffinityPolicyStrictness = "RequiredDuringSchedulingPreferredDuringExecution"

	// VirtualMachineAffinityPolicyStrictnessPreferredDuringSchedulingPreferredDuringExecution
	// indicates the policy is preferred during placement and when the VM is powered on.
	VirtualMachineAffinityPolicyStrictnessPreferredDuringSchedulingPreferredDuringExecution VirtualMachineAffinityPolicyStrictness = "PreferredDuringSchedulingPreferredDuringExecution"
)

// +kubebuilder:validation:Enum=Pending;Compliant;NotCompliant;NotApplicable;Unknown

// VirtualMachineAffinityCompliance is the VM's compliance with a policy.
type VirtualMachineAffinityCompliance string

const (
	// VirtualMachineAffinityCompliancePending indicates the vCenter compute
	// policy for this entry is not yet known, so no compliance is available.
	VirtualMachineAffinityCompliancePending VirtualMachineAffinityCompliance = "Pending"

	// VirtualMachineAffinityComplianceCompliant indicates vCenter recorded
	// the VM as compliant with the policy.
	VirtualMachineAffinityComplianceCompliant VirtualMachineAffinityCompliance = "Compliant"

	// VirtualMachineAffinityComplianceNotCompliant indicates vCenter recorded
	// the VM as not compliant with the policy.
	VirtualMachineAffinityComplianceNotCompliant VirtualMachineAffinityCompliance = "NotCompliant"

	// VirtualMachineAffinityComplianceNotApplicable indicates vCenter recorded
	// the policy as not applicable to the VM, e.g. the VM is powered off.
	VirtualMachineAffinityComplianceNotApplicable VirtualMachineAffinityCompliance = "NotApplicable"

	// VirtualMachineAffinityComplianceUnknown indicates vCenter recorded an
	// unknown or unrecognized compliance state for the VM.
	VirtualMachineAffinityComplianceUnknown VirtualMachineAffinityCompliance = "Unknown"
)

// +kubebuilder:validation:Enum=Self;Peer

// VirtualMachineAffinityPolicyOrigin describes why a policy applies to the VM.
type VirtualMachineAffinityPolicyOrigin string

const (
	// VirtualMachineAffinityPolicyOriginSelf indicates the VM's own
	// spec.affinity requests the policy.
	VirtualMachineAffinityPolicyOriginSelf VirtualMachineAffinityPolicyOrigin = "Self"

	// VirtualMachineAffinityPolicyOriginPeer indicates another VM's
	// spec.affinity requests the policy, and it applies to this VM because
	// this VM carries the policy's target labels.
	VirtualMachineAffinityPolicyOriginPeer VirtualMachineAffinityPolicyOrigin = "Peer"
)

// VirtualMachineAffinityLabel is a resolved target label pair of a policy.
type VirtualMachineAffinityLabel struct {
	// +required

	// Key is the label key.
	Key string `json:"key"`

	// +required

	// Value is the label value.
	Value string `json:"value"`
}

// VirtualMachineAffinityPolicyStatus describes one VM Operator affinity
// policy that applies to the VM.
type VirtualMachineAffinityPolicyStatus struct {
	// +required

	// Type is the kind of the policy.
	Type VirtualMachineAffinityPolicyType `json:"type"`

	// +required

	// Strictness describes how strictly the policy is enforced.
	Strictness VirtualMachineAffinityPolicyStrictness `json:"strictness"`

	// +required

	// TopologyKey is the topology at which the policy applies, either
	// kubernetes.io/hostname or topology.kubernetes.io/zone.
	TopologyKey string `json:"topologyKey"`

	// +required
	// +listType=atomic

	// Targets are the label pairs the policy targets, sorted by key, then
	// value, with no duplicates.
	Targets []VirtualMachineAffinityLabel `json:"targets"`

	// +optional

	// PolicyID is the vCenter compute policy ID. It is empty while the
	// compliance is Pending.
	PolicyID string `json:"policyID,omitempty"`

	// +required

	// Compliance is the most recent compliance state that vCenter recorded
	// for the VM, or Pending if the policy ID is not yet known.
	Compliance VirtualMachineAffinityCompliance `json:"compliance"`

	// +required

	// Origin describes why the policy applies to the VM: Self when the VM's
	// own spec.affinity requests it, or Peer when another VM's spec.affinity
	// requests it and this VM carries the target labels.
	Origin VirtualMachineAffinityPolicyOrigin `json:"origin"`
}

// VirtualMachineAffinityStatus reports the VM Operator affinity policies
// that apply to the VM.
type VirtualMachineAffinityStatus struct {
	// +optional
	// +listType=atomic

	// Policies describes each VM Operator affinity policy that applies to the
	// VM and the VM's compliance with it.
	Policies []VirtualMachineAffinityPolicyStatus `json:"policies,omitempty"`
}
