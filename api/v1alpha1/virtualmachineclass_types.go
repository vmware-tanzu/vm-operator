// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type VirtualMachineClassHardware struct {
	Cpus   int64             `json:"cpus,omitempty"`
	Memory resource.Quantity `json:"memory,omitempty"`
}

type VirtualMachineResourceSpec struct {
	Cpu    resource.Quantity `json:"cpu,omitempty"`
	Memory resource.Quantity `json:"memory,omitempty"`
}

type VirtualMachineClassResources struct {
	Requests VirtualMachineResourceSpec `json:"requests,omitempty"`
	Limits   VirtualMachineResourceSpec `json:"limits,omitempty"`
}

type VirtualMachineClassPolicies struct {
	Resources VirtualMachineClassResources `json:"resources,omitempty"`
}

// VirtualMachineClassSpec defines the desired state of VirtualMachineClass
type VirtualMachineClassSpec struct {
	Hardware VirtualMachineClassHardware `json:"hardware,omitempty"`
	Policies VirtualMachineClassPolicies `json:"policies,omitempty"`
}

// VirtualMachineClassStatus defines the observed state of VirtualMachineClass
type VirtualMachineClassStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=vmclass
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// VirtualMachineClass is the Schema for the virtualmachineclasses API
type VirtualMachineClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineClassSpec   `json:"spec,omitempty"`
	Status VirtualMachineClassStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VirtualMachineClassList contains a list of VirtualMachineClass
type VirtualMachineClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachineClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VirtualMachineClass{}, &VirtualMachineClassList{})
}
