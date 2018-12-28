
/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/


package v1beta1

import (
	"k8s.io/apiserver/pkg/registry/rest"
	"log"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/endpoints/request"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"vmware.com/kubevsphere/pkg/apis/vmoperator"
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
}

// VirtualMachineImageStatus defines the observed state of VirtualMachineImage
type VirtualMachineImageStatus struct {
}

// Validate checks that an instance of VirtualMachineImage is well formed
func (VirtualMachineImageStrategy) Validate(ctx request.Context, obj runtime.Object) field.ErrorList {
	o := obj.(*vmoperator.VirtualMachineImage)
	log.Printf("Validating fields for VirtualMachineImage %s\n", o.Name)
	errors := field.ErrorList{}
	// perform validation here and add to errors using field.Invalid
	return errors
}

// DefaultingFunction sets default VirtualMachineImage field values
func (VirtualMachineImageSchemeFns) DefaultingFunction(o interface{}) {
	obj := o.(*VirtualMachineImage)
	// set default field values here
	log.Printf("Defaulting fields for VirtualMachineImage %s\n", obj.Name)
}

func NewVirtualMachineImageFake() runtime.Object {
	return &VirtualMachineImage{
		metav1.TypeMeta{
			"VirtualMachineImage",
			"v1Beta",
		},
		metav1.ObjectMeta{Name:"Fake"},
		VirtualMachineImageSpec{},
		VirtualMachineImageStatus{},
	}
}

func NewVirtualMachineImagesREST() rest.Storage {
	return &VirtualMachineImagesREST{}
}

