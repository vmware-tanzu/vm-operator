// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

// VirtualMachineSetResourcePolicyStatus defines the observed state of VirtualMachineSetResourcePolicy
type VirtualMachineSetResourcePolicyStatus struct {
	ClusterModules []ClusterModuleStatus `json:"clustermodules,omitempty"`
}

type ClusterModuleStatus struct {
	GroupName  string `json:"groupname"`
	ModuleUuid string `json:"moduleUUID"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// VirtualMachineSetResourcePolicy is the Schema for the virtualmachinesetresourcepolicies API
type VirtualMachineSetResourcePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineSetResourcePolicySpec   `json:"spec,omitempty"`
	Status VirtualMachineSetResourcePolicyStatus `json:"status,omitempty"`
}

func (res VirtualMachineSetResourcePolicy) NamespacedName() string {
	return res.Namespace + "/" + res.Name
}

// +kubebuilder:object:root=true

// VirtualMachineSetResourcePolicyList contains a list of VirtualMachineSetResourcePolicy
type VirtualMachineSetResourcePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachineSetResourcePolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VirtualMachineSetResourcePolicy{}, &VirtualMachineSetResourcePolicyList{})
}
