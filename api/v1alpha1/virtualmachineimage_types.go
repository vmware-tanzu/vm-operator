// Copyright (c) 2018 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VirtualMachineImageSpec defines the desired state of VirtualMachineImage
type VirtualMachineImageSpec struct {
	// Type describes the type of the VirtualMachineImage. Currently, the only supported image is "OVF"
	Type string `json:"type"`

	// ImageSourceType describes the type of content source of the VirtualMachineImage.  The only Content Source
	// supported currently is the vSphere Content Library.
	ImageSourceType string `json:"imageSourceType,omitempty"`
}

// VirtualMachineImageStatus defines the observed state of VirtualMachineImage
type VirtualMachineImageStatus struct {
	Uuid       string `json:"uuid,omitempty"`
	InternalId string `json:"internalId"`
	PowerState string `json:"powerState,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=vmimage
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// VirtualMachineImage is the Schema for the virtualmachineimages API
// A VirtualMachineImage represents a VirtualMachine image (e.g. VM template) that can be used as the base image
// for creating a VirtualMachine instance.  The VirtualMachineImage is a required field of the VirtualMachine
// spec.  Currently, VirtualMachineImages are immutable to end users.  They are created and managed by a
// VirtualMachineImage controller whose role is to discover available images in the backing infrastructure provider
// that should be surfaced as consumable VirtualMachineImage resources.
type VirtualMachineImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineImageSpec   `json:"spec,omitempty"`
	Status VirtualMachineImageStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VirtualMachineImageList contains a list of VirtualMachineImage
type VirtualMachineImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachineImage `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VirtualMachineImage{}, &VirtualMachineImageList{})
}
