/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualMachineClass
// +k8s:openapi-gen=true
// +resource:path=virtualmachineclasses,strategy=VirtualMachineClassStrategy
type VirtualMachineClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineClassSpec   `json:"spec,omitempty"`
	Status VirtualMachineClassStatus `json:"status,omitempty"`
}

type VirtualMachineClassHardware struct {
	Cpus   int64             `json:"cpus,omitempty"`
	Memory resource.Quantity `json:"memory,omitempty"`
}

type VirtualMachineClassResourceSpec struct {
	Cpu    resource.Quantity `json:"cpu,omitempty"`
	Memory resource.Quantity `json:"memory,omitempty"`
}

type VirtualMachineClassResources struct {
	Requests VirtualMachineClassResourceSpec `json:"requests,omitempty"`
	Limits   VirtualMachineClassResourceSpec `json:"limits,omitempty"`
}

type VirtualMachineClassPolicies struct {
	Resources    VirtualMachineClassResources `json:"resources,omitempty"`
	StorageClass string                       `json:"storageClass,omitempty"`
}

// VirtualMachineClassSpec defines the desired state of VirtualMachineClass
type VirtualMachineClassSpec struct {
	Hardware VirtualMachineClassHardware `json:"hardware,omitempty"`
	Policies VirtualMachineClassPolicies `json:"policies,omitempty"`
}

// VirtualMachineClassStatus defines the observed state of VirtualMachineClass
type VirtualMachineClassStatus struct {
}

// DefaultingFunction sets default VirtualMachineClass field values
func (VirtualMachineClassSchemeFns) DefaultingFunction(o interface{}) {
	//obj := o.(*VirtualMachineClass)
	// set default field values here
}
