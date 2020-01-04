// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualMachineSetResourcePolicy
// +k8s:openapi-gen=true
// +resource:path=virtualmachinesetresourcepolicies,strategy=VirtualMachineSetResourcePolicyStrategy
type VirtualMachineSetResourcePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineSetResourcePolicySpec   `json:"spec,omitempty"`
	Status VirtualMachineSetResourcePolicyStatus `json:"status,omitempty"`
}

func (res VirtualMachineSetResourcePolicy) NamespacedName() string {
	return res.Namespace + "/" + res.Name
}

// ResourcePoolSpec defines a Resource Group
type ResourcePoolSpec struct {
	Name         string                     `json:"name,omitempty"`
	Reservations VirtualMachineResourceSpec `json:"reservations,omitempty"`
	Limits       VirtualMachineResourceSpec `json:"limits,omitempty"`
}

// Folder defines a Folder
type FolderSpec struct {
	Name string `json:"name,omitempty"`
}

// ClusterModuleSpec defines a ClusterModule in VC.
type ClusterModuleSpec struct {
	GroupName string `json:"groupname"`
}

// VirtualMachineSetResourcePolicySpec defines the desired state of VirtualMachineSetResourcePolicy
type VirtualMachineSetResourcePolicySpec struct {
	ResourcePool   ResourcePoolSpec    `json:"resourcepool,omitempty"`
	Folder         FolderSpec          `json:"folder,omitempty"`
	ClusterModules []ClusterModuleSpec `json:"clustermodules,omitempty"`
}

type ClusterModuleStatus struct {
	GroupName  string `json:"groupname"`
	ModuleUuid string `json:"moduleUUID"`
}

// VirtualMachineSetResourcePolicyStatus defines the observed state of VirtualMachineSetResourcePolicy
type VirtualMachineSetResourcePolicyStatus struct {
	ClusterModules []ClusterModuleStatus `json:"clustermodules"`
}

// DefaultingFunction sets default VirtualMachineSetResourcePolicy field values
func (VirtualMachineSetResourcePolicySchemeFns) DefaultingFunction(o interface{}) {
}
