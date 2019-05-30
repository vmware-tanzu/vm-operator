/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualMachineImage
// +k8s:openapi-gen=true
// +resource:path=virtualmachineimages,strategy=VirtualMachineImageStrategy,rest=VirtualMachineImagesREST
type VirtualMachineImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineImageSpec   `json:"spec,omitempty"`
	Status VirtualMachineImageStatus `json:"status,omitempty"`
}

// VirtualMachineImageSpec defines the desired state of VirtualMachineImage
type VirtualMachineImageSpec struct {
	Type            string `json:"type"`
	ImageSource     string `json:"imageSource,omitempty"`
	ImageSourceType string `json:"imageSourceType,omitempty"`
	ImagePath       string `json:"imagePath,omitempty"`
}

// VirtualMachineImageStatus defines the observed state of VirtualMachineImage
type VirtualMachineImageStatus struct {
	Uuid       string `json:"uuid,omitempty"`
	InternalId string `json:"internalId"`
	PowerState string `json:"powerState,omitempty"`
}

// DefaultingFunction sets default VirtualMachineImage field values
func (VirtualMachineImageSchemeFns) DefaultingFunction(o interface{}) {
	//obj := o.(*VirtualMachineImage)
	// set default field values here
}
