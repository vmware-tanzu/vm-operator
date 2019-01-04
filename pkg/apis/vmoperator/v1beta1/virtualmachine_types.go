
/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/


package v1beta1

import (
	"context"
	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"vmware.com/kubevsphere/pkg/apis/vmoperator"
)

const VirtualMachineFinalizer string = "virtualmachine.vmoperator.vmware.com"

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

type VirtualMachineResourceSpec struct {
	Cpu int `json:"cpu"`
	Memory int `json:"memory"`
}

type VirtualMachineResourcesSpec struct {
	Capacity 	VirtualMachineResourceSpec `json:"capacity"`
	Limits 		VirtualMachineResourceSpec `json:"limits,omitempty"`
	Requests 	VirtualMachineResourceSpec `json:"requests,omitempty"`
}

// VirtualMachineSpec defines the desired state of VirtualMachine
type VirtualMachineSpec struct {
	Image string `json:"image"`
	Resources VirtualMachineResourcesSpec `json:"resources"`
	PowerState string `json:"powerState"`
}

type VirtualMachineConfigStatus struct {
	Uuid string `json:"uuid,omitempty"`
	InternalId string `json:"internalId"`
	CreateDate   string `json:"createDate"`
	ModifiedDate string `json:"modifiedDate"`
}

// VirtualMachineStatus defines the observed state of VirtualMachine
type VirtualMachineRuntimeStatus struct {
	Host       string `json:"host"`
	PowerState string `json:"powerState"`
}

type VirtualMachineStatus struct {
	State 		string `json:"state"`
	ConfigStatus VirtualMachineConfigStatus `json:"configStatus`
	RuntimeStatus VirtualMachineRuntimeStatus `json:"runtimeStatus"`
}

func (v VirtualMachineStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	// Invoke the parent implementation to strip the Status
	v.DefaultStorageStrategy.PrepareForCreate(ctx, obj)

	// Add a finalizer so that our controllers can process deletion
	o := obj.(*vmoperator.VirtualMachine)
	finalizers := append(o.GetFinalizers(), VirtualMachineFinalizer)
	o.SetFinalizers(finalizers)
}

// Validate checks that an instance of VirtualMachine is well formed
func (v VirtualMachineStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	o := obj.(*vmoperator.VirtualMachine)
	glog.Infof("Validating fields for VirtualMachine %s\n", o.Name)
	errors := field.ErrorList{}
	// perform validation here and add to errors using field.Invalid
	return errors
}

// DefaultingFunction sets default VirtualMachine field values
func (VirtualMachineSchemeFns) DefaultingFunction(o interface{}) {
	obj := o.(*VirtualMachine)
	// set default field values here
	glog.Infof("Defaulting fields for VirtualMachine %s\n", obj.Name)
}
