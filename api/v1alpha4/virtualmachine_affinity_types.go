// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha4

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:validation:Enum=In;NotIn;Exists;DoesNotExist;Gt;Lt

// ZoneSelectorOperator specifies the type of operator used by
// the zone selector to represent key-value relationships.
type ZoneSelectorOperator string

const (
	ZoneSelectorOpIn           ZoneSelectorOperator = "In"
	ZoneSelectorOpNotIn        ZoneSelectorOperator = "NotIn"
	ZoneSelectorOpExists       ZoneSelectorOperator = "Exists"
	ZoneSelectorOpDoesNotExist ZoneSelectorOperator = "DoesNotExist"
	ZoneSelectorOpGt           ZoneSelectorOperator = "Gt"
	ZoneSelectorOpLt           ZoneSelectorOperator = "Lt"
)

// ZoneSelectorRequirement defines the key value relationships for a matching zone selector.
type ZoneSelectorRequirement struct {
	// Key is the label key to which the selector applies.
	Key string `json:"key"`

	// Operator represents a key's relationship to a set of values.
	// Valid operators are In, NotIn, Exists, DoesNotExist. Gt, and Lt.
	Operator ZoneSelectorOperator `json:"operator"`

	// +optional
	// +listType=atomic

	// Values is a list of values to which the operator applies.
	// If the operator is In or NotIn, the values list must be non-empty.
	// If the operator is Exists or DoesNotExist, the values list must be empty.
	// If the operator is Gt or Lt, the values list must have a single element,
	// which will be interpreted as an integer.
	Values []string `json:"values,omitempty"`
}

// ZoneSelectorTerm defines the matching zone selector requirements for zone based affinity/anti-affinity scheduling.
type ZoneSelectorTerm struct {
	// +optional
	// +listType=atomic

	// MatchExpressions is a list of zone selector requirements by zone's
	// labels.
	MatchExpressions []ZoneSelectorRequirement `json:"matchExpressions,omitempty"`

	// +optional
	// +listType=atomic

	// MatchFields is a list of zone selector requirements by zone's fields.
	MatchFields []ZoneSelectorRequirement `json:"matchFields,omitempty"`
}

// VirtualMachineAffinityZoneAffinitySpec defines the affinity scheduling rules
// related to zones.
type VirtualMachineAffinityZoneAffinitySpec struct {
	// +optional
	// +listType=atomic

	// RequiredDuringSchedulingIgnoredDuringExecution describes affinity
	// requirements that must be met or the VM will not be scheduled.
	//
	// When there are multiple elements, the lists of zones corresponding to
	// each term are intersected, i.e. all terms must be satisfied.
	RequiredDuringSchedulingIgnoredDuringExecution []ZoneSelectorTerm `json:"requiredDuringSchedulingIgnoredDuringExecution,omitempty"`

	// +optional
	// +listType=atomic

	// PreferredDuringSchedulingIgnoredDuringExecution describes affinity
	// requirements that should be met, but the VM can still be scheduled if
	// the requirement cannot be satisfied. The scheduler will prefer to schedule VMs
	// that satisfy the anti-affinity expressions specified by this field, but it may choose to
	// violate one or more of the expressions.
	//
	// When there are multiple elements, the lists of zones corresponding to
	// each term are intersected, i.e. all terms must be satisfied.
	PreferredDuringSchedulingIgnoredDuringExecution []ZoneSelectorTerm `json:"preferredDuringSchedulingIgnoredDuringExecution,omitempty"`
}

// VirtualMachineAntiAffinityZoneAffinitySpec defines the anti-affinity scheduling rules
// related to zones.
type VirtualMachineAntiAffinityZoneAffinitySpec struct {
	// +optional
	// +listType=atomic

	// RequiredDuringSchedulingIgnoredDuringExecution describes affinity
	// requirements that must be met or the VM will not be scheduled.
	//
	// When there are multiple elements, the lists of zones corresponding to
	// each term are intersected, i.e. all terms must be satisfied.
	RequiredDuringSchedulingIgnoredDuringExecution []ZoneSelectorTerm `json:"requiredDuringSchedulingIgnoredDuringExecution,omitempty"`

	// +optional
	// +listType=atomic

	// PreferredDuringSchedulingIgnoredDuringExecution describes affinity
	// requirements that should be met, but the VM can still be scheduled if
	// the requirement cannot be satisfied. The scheduler will prefer to schedule VMs to
	// that satisfy the anti-affinity expressions specified by this field, but it may choose to
	// violate one or more of the expressions.
	//
	// When there are multiple elements, the lists of zones corresponding to
	// each term are intersected, i.e. all terms must be satisfied.
	PreferredDuringSchedulingIgnoredDuringExecution []ZoneSelectorTerm `json:"preferredDuringSchedulingIgnoredDuringExecution,omitempty"`
}

// VMAffinityTerm defines the VM affinity/anti-affinity term.
type VMAffinityTerm struct {
	// +optional

	// LabelSelector is a label query over a set of VMs.
	// When omitted, this term matches with no VMs.
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`

	// TopologyKey describes where this VM should be co-located (affinity) or not
	// co-located (anti-affinity).
	// Commonly used values include:
	// `kubernetes.io/hostname` -- The rule is executed in the context of a node/host.
	// `topology.kubernetes.io/zone` -- This rule is executed in the context of a zone.
	//
	// Please note, The following rules apply when specifying the topology key in the context of a zone/host.
	//
	// - When topology key is in the context of a zone, the only supported verbs are
	//   PreferredDuringSchedulingIgnoredDuringExecution and RequiredDuringSchedulingIgnoredDuringExecution.
	// - When topology key is in the context of a host, the only supported verbs are
	//   PreferredDuringSchedulingPreferredDuringExecution and RequiredDuringSchedulingPreferredDuringExecution
	//   for VM-VM node-level anti-affinity scheduling.
	// - When topology key is in the context of a host, the only supported verbs are
	//   PreferredDuringSchedulingIgnoredDuringExecution and RequiredDuringSchedulingIgnoredDuringExecution
	//   for VM-VM node-level anti-affinity scheduling.
	TopologyKey string `json:"topologyKey"`
}

// VirtualMachineAffinityVMAffinitySpec defines the affinity requirements for scheduling
// rules related to other VMs.
type VirtualMachineAffinityVMAffinitySpec struct {
	// +optional
	// +listType=atomic

	// RequiredDuringSchedulingIgnoredDuringExecution describes affinity
	// requirements that must be met or the VM will not be scheduled.
	//
	// When there are multiple elements, the lists of nodes corresponding to
	// each term are intersected, i.e. all terms must be satisfied.
	RequiredDuringSchedulingIgnoredDuringExecution []VMAffinityTerm `json:"requiredDuringSchedulingIgnoredDuringExecution,omitempty"`

	// +optional
	// +listType=atomic

	// PreferredDuringSchedulingIgnoredDuringExecution describes affinity
	// requirements that should be met, but the VM can still be scheduled if
	// the requirement cannot be satisfied. The scheduler will prefer to schedule VMs
	// that satisfy the anti-affinity expressions specified by this field, but it may choose to
	// violate one or more of the expressions.
	//
	// When there are multiple elements, the lists of nodes corresponding to
	// each term are intersected, i.e. all terms must be satisfied.
	PreferredDuringSchedulingIgnoredDuringExecution []VMAffinityTerm `json:"preferredDuringSchedulingIgnoredDuringExecution,omitempty"`
}

// VirtualMachineAntiAffinityVMAffinitySpec defines the anti-affinity requirements for scheduling
// rules related to other VMs.
type VirtualMachineAntiAffinityVMAffinitySpec struct {
	// +optional
	// +listType=atomic

	// RequiredDuringSchedulingIgnoredDuringExecution describes anti-affinity
	// requirements that must be met or the VM will not be scheduled.
	//
	// When there are multiple elements, the lists of nodes corresponding to
	// each term are intersected, i.e. all terms must be satisfied.
	RequiredDuringSchedulingIgnoredDuringExecution []VMAffinityTerm `json:"requiredDuringSchedulingIgnoredDuringExecution,omitempty"`

	// +optional
	// +listType=atomic

	// PreferredDuringSchedulingIgnoredDuringExecution describes anti-affinity
	// requirements that should be met, but the VM can still be scheduled if
	// the requirement cannot be satisfied. The scheduler will prefer to schedule VMs
	// that satisfy the affinity expressions specified by this field, but it may choose to
	// violate one or more of the expressions.
	//
	// When there are multiple elements, the lists of nodes corresponding to
	// each term are intersected, i.e. all terms must be satisfied.
	PreferredDuringSchedulingIgnoredDuringExecution []VMAffinityTerm `json:"preferredDuringSchedulingIgnoredDuringExecution,omitempty"`

	// +optional
	// +listType=atomic

	// RequiredDuringSchedulingPreferredExecution describes anti-affinity
	// requirements that must be met or the VM will not be scheduled. Additionally,
	// it also describes the anti-affinity requirements that should be met during run-time,
	// but the VM can still be run if the requirements cannot be satisfied.
	//
	// When there are multiple elements, the lists of nodes corresponding to
	// each term are intersected, i.e. all terms must be satisfied.
	RequiredDuringSchedulingPreferredDuringExecution []VMAffinityTerm `json:"requiredDuringSchedulingPreferredDuringExecution,omitempty"`

	// +optional
	// +listType=atomic

	// PreferredDuringSchedulingPreferredDuringExecution describes anti-affinity
	// requirements that should be met, but the VM can still be scheduled if
	// the requirement cannot be satisfied. The scheduler will prefer to schedule VMs
	// that satisfy the affinity expressions specified by this field, but it may choose to
	// violate one or more of the expressions. Additionally,
	// it also describes the anti-affinity requirements that should be met during run-time,
	// but the VM can still be run if the requirements cannot be satisfied.
	//
	// When there are multiple elements, the lists of nodes corresponding to
	// each term are intersected, i.e. all terms must be satisfied.
	PreferredDuringSchedulingPreferredDuringExecution []VMAffinityTerm `json:"preferredDuringSchedulingPreferredDuringExecution,omitempty"`
}

// VirtualMachineAffinitySpec defines the group of affinity scheduling rules.
type VirtualMachineAffinitySpec struct {
	// +optional

	// ZoneAffinity describes affinity scheduling rules related to a zone.
	ZoneAffinity *VirtualMachineAffinityZoneAffinitySpec `json:"zoneAffinity,omitempty"`

	// +optional

	// ZoneAntiAffinity describes anti-affinity scheduling rules related to a zone.
	ZoneAntiAffinity *VirtualMachineAntiAffinityZoneAffinitySpec `json:"zoneAntiAffinity,omitempty"`

	// +optional

	// VMAffinity describes affinity scheduling rules related to other VMs.
	VMAffinity *VirtualMachineAffinityVMAffinitySpec `json:"vmAffinity,omitempty"`

	// +optional

	// VMAntiAffinity describes anti-affinity scheduling rules related to other VMs.
	VMAntiAffinity *VirtualMachineAntiAffinityVMAffinitySpec `json:"vmAntiAffinity,omitempty"`
}
