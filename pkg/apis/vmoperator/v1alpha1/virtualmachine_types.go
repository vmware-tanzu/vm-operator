/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualMachine
// +k8s:openapi-gen=true
// +resource:path=virtualmachines,strategy=VirtualMachineStrategy
type VirtualMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineSpec   `json:"spec,omitempty"`
	Status VirtualMachineStatus `json:"status,omitempty"`
}

func (vm VirtualMachine) NamespacedName() string {
	return vm.Namespace + "/" + vm.Name
}

type VirtualMachinePowerState string

// See govmomi.vim25.types.VirtualMachinePowerState
const (
	VirtualMachinePoweredOff = "poweredOff"
	VirtualMachinePoweredOn  = "poweredOn"
)

type VMStatusPhase string

const (
	Creating VMStatusPhase = "Creating"
	Created  VMStatusPhase = "Created"
	Deleted  VMStatusPhase = "Deleted"
)

type VirtualMachinePort struct {
	Port     int             `json:"port"`
	Ip       string          `json:"ip"`
	Name     string          `json:"name"`
	Protocol corev1.Protocol `json:"protocol"`
}

// VirtualMachineSpec defines the desired state of VirtualMachine
type VirtualMachineSpec struct {
	ImageName  string                  `json:"imageName"`
	ClassName  string                  `json:"className"`
	PowerState string                  `json:"powerState"`
	Env        corev1.EnvVar           `json:"env,omitempty"`
	Ports      []VirtualMachinePort    `json:"ports,omitempty"`
	VmMetadata *VirtualMachineMetadata `json:"vmMetadata,omitempty"`
}

// VirtualMachineMetadata defines the guest customization
type VirtualMachineMetadata struct {
	ConfigMapName string `json:"configMapName,omitempty"`
	Transport     string `json:"transport,omitempty"`
}

// TODO: Make these annotations
/*
type VirtualMachineConfigStatus struct {
	Uuid 		string `json:"uuid,omitempty"`
	InternalId 	string `json:"internalId"`
	CreateDate  	string `json:"createDate"`
	ModifiedDate 	string `json:"modifiedDate"`
}
*/

type VirtualMachineCondition struct {
	LastProbeTime      metav1.Time `json:"lastProbeTime"`
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`
	Message            string      `json:"message"`
	Reason             string      `json:"reason"`
	Status             string      `json:"status"`
	Type               string      `json:"type"`
}

type VirtualMachineStatus struct {
	Conditions []VirtualMachineCondition `json:"conditions"`
	Host       string                    `json:"host"`
	PowerState string                    `json:"powerState"`
	Phase      VMStatusPhase             `json:"phase"`
	VmIp       string                    `json:"vmIp"`
}

// DefaultingFunction sets default VirtualMachine field values
func (VirtualMachineSchemeFns) DefaultingFunction(o interface{}) {
	//obj := o.(*VirtualMachine)
	// set default field values here
}
