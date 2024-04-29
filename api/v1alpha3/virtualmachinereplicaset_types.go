// Copyright (c) 2024-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha3

import (
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VirtualMachineTemplateSpec describes the data needed to create a VirtualMachine
// from a template.
type VirtualMachineTemplateSpec struct {
	// +optional
	//
	// ObjectMeta contains the desired Labels and Annotations that must be
	// applied to each replica virtual machine.
	vmopv1common.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	//
	// Specification of the desired behavior of each replica virtual machine.
	Spec VirtualMachineSpec `json:"spec,omitempty"`
}

// VirtualMachineReplicaSetSpec is the specification of a VirtualMachineReplicaSet.
type VirtualMachineReplicaSetSpec struct {
	// +optional
	// +kubebuilder:default=1
	//
	// Replicas is the number of desired replicas.
	// This is a pointer to distinguish between explicit zero and unspecified.
	// Defaults to 1.
	Replicas *int32 `json:"replicas,omitempty"`

	// +optional
	//
	// Minimum number of seconds for which a newly created replica virtual
	// machine should be ready for it to be considered available.
	// Defaults to 0 (virtual machine will be considered available as soon as it
	// is ready).
	MinReadySeconds int32 `json:"minReadySeconds,omitempty"`

	// +optional
	//
	// Selector is a label to query over virtual machines that should match the
	// replica count. A virtual machine's label keys and values must match in order
	// to be controlled by this VirtualMachineReplicaSet.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
	Selector *metav1.LabelSelector `json:"selector"`

	// +optional
	//
	// Template is the object that describes the virtual machine that will be
	// created if insufficient replicas are detected.
	Template VirtualMachineTemplateSpec `json:"template,omitempty"`
}

// VirtualMachineReplicaSetStatus represents the observed state of a
// VirtualMachineReplicaSet resource.
type VirtualMachineReplicaSetStatus struct {
	// +optional
	//
	// Replicas is the most recently observed number of replicas.
	Replicas int32 `json:"replicas"`

	// +optional
	//
	// FullyLabeledReplicas is the number of replicas that have labels matching the
	// labels of the virtual machine template of the VirtualMachineReplicaSet.
	FullyLabeledReplicas int32 `json:"fullyLabeledReplicas,omitempty"`

	// +optional
	//
	// ReadyReplicas is the number of ready replicas for this VirtualMachineReplicaSet. A
	// virtual machine is considered ready when it's "Ready" condition is marked as
	// true.
	Readyreplicas int32 `json:"readyReplicas,omitempty"`

	// +optional
	//
	// AvailableReplicas is the number of available replicas (ready for at
	// least minReadySeconds) for this VirtualMachineReplicaSet.
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed
	// VirtualMachineReplicaSet.
	//
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	//
	// Conditions represents the latest available observations of a
	// VirtualMachineReplicaSet's current state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=vmreplicaset
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas
// +kubebuilder:printcolumn:name="Replicas",type="integer",priority=1,JSONPath=".status.replicas"
// +kubebuilder:printcolumn:name="Ready-Replicas",type="integer",priority=1,JSONPath=".status.readyReplicas"
// +kubebuilder:printcolumn:name="Available-Replicas",type="integer",JSONPath=".status.availableReplicas"

// VirtualMachineReplicaSet is the schema for the virtualmachinereplicasets API
type VirtualMachineReplicaSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineReplicaSetSpec   `json:"spec,omitempty"`
	Status VirtualMachineReplicaSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VirtualMachineList contains a list of VirtualMachine.
type VirtualMachineReplicaSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachineReplicaSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VirtualMachineReplicaSet{}, &VirtualMachineReplicaSetList{})
}
