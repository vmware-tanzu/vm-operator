// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha3

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
)

type VirtualMachineConfigSource struct {

	// +optional

	// Kind is the API kind, in CamelCase, of the source of this config.
	// For example, a VirtualMachineClass or VirtualMachine.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	Kind string `json:"kind,omitempty"`

	// +optional

	// Name is the unique value used to identify the source of the this
	// VirtualMachineConfig.
	Name string `json:"name,omitempty"`

	// +optional

	// UID is the unique in time and space value for this object.
	UID apitypes.UID `json:"uid,omitempty"`

	// +optional

	// ResourceVersion is an opaque value that represents the internal version
	// of this object that can be used by clients to determine when objects have
	// changed. May be used for optimistic concurrency, change detection, and
	// the watch operation on a resource or set of resources. Clients must treat
	// these values as opaque and passed unmodified back to the server. They may
	// only be valid for a particular resource or set of resources.
	ResourceVersion string `json:"resourceVersion,omitempty"`

	// +optional

	// Generation is a sequence number representing a specific generation of the
	// desired state.
	Generation int64 `json:"generation,omitempty"`
}

// VirtualMachineConfigSpec defines the desired state of
// VirtualMachineConfig.
type VirtualMachineConfigSpec struct {
	// +optional
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:Type=object
	// +kubebuilder:pruning:PreserveUnknownFields

	// Config describes the desired configuration for a VirtualMachine.
	//
	// The contents of this field are determined by the underlying platform on
	// which the VM is deployed. For vSphere, the value of spec.config is the
	// VirtualMachineConfigSpec data object (https://bit.ly/3HDtiRu) marshaled
	// to JSON using the discriminator field "_typeName" to preserve type
	// information.
	Config json.RawMessage `json:"config,omitempty"`

	// +optional

	// InstanceStorage describes the instance storage volumes associated with
	// this VirtualMachineConfig.
	InstanceStorage InstanceStorage `json:"instanceStorage,omitempty"`

	// Source describes the Kubernetes resource that is the source of this
	// VirtualMachineConfig, ex. a VirtualMachineClass or VirtualMachine.
	Source VirtualMachineConfigSource `json:"source,omitempty"`
}

// VirtualMachineConfigStatus defines the observed state of
// VirtualMachineConfig.
type VirtualMachineConfigStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=vmconfig
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// VirtualMachineConfig is the schema for the virtualmachineconfigs API and
// represents the desired state and observed status of a virtualmachineconfigs
// resource.
type VirtualMachineConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineConfigSpec   `json:"spec,omitempty"`
	Status VirtualMachineConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VirtualMachineConfigList contains a list of VirtualMachineConfig.
type VirtualMachineConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachineConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&VirtualMachineConfig{},
		&VirtualMachineConfigList{},
	)
}
