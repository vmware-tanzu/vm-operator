/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package v1alpha1

import (
	"context"
	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/apis/vmoperator"
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
	Cpu    int64             `json:"cpu,omitempty"`
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

// Validate checks that an instance of VirtualMachineClass is well formed
func (VirtualMachineClassStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	o := obj.(*vmoperator.VirtualMachineClass)
	glog.V(4).Infof("Validating fields for VirtualMachineClass %s", o.Name)
	errors := field.ErrorList{}
	// perform validation here and add to errors using field.Invalid
	return errors
}

// DefaultingFunction sets default VirtualMachineClass field values
func (VirtualMachineClassSchemeFns) DefaultingFunction(o interface{}) {
	obj := o.(*VirtualMachineClass)
	// set default field values here
	glog.V(4).Infof("Defaulting fields for VirtualMachineClass %s", obj.Name)
}
