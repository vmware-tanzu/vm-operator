/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package v1alpha1

import (
	"context"

	"github.com/golang/glog"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const (
	VirtualMachineFinalizer string = "virtualmachine.vmoperator.vmware.com"
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
	// controller has received a request to create the VM
	Creating VMStatusPhase = "Creating"
	// VM is created
	Created VMStatusPhase = "Created"
	// VM is Deleted
	Deleted VMStatusPhase = "Deleted"
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

func (v VirtualMachineStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	// Invoke the parent implementation to strip the Status
	v.DefaultStorageStrategy.PrepareForCreate(ctx, obj)

	o := obj.(*vmoperator.VirtualMachine)

	// Add a finalizer so that our controllers can process deletion
	finalizers := append(o.GetFinalizers(), VirtualMachineFinalizer)
	o.SetFinalizers(finalizers)
}

// Validate checks that an instance of VirtualMachine is well formed
func (v VirtualMachineStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	vm := obj.(*vmoperator.VirtualMachine)
	glog.V(4).Infof("Validating fields for VirtualMachine %s", vm.Name)
	errors := field.ErrorList{}

	// Confirm that the required fields are present and within valid ranges, if applicable
	if vm.Spec.ImageName == "" {
		glog.Errorf("Image empty for VM %s", vm.Name)
		errors = append(errors, field.Required(field.NewPath("spec", "image"), ""))
	}
	if vm.Spec.ClassName == "" {
		glog.Errorf("Class empty for VM %s", vm.Name)
		errors = append(errors, field.Required(field.NewPath("spec", "class"), ""))
	}

	return errors
}

// DefaultingFunction sets default VirtualMachine field values
func (VirtualMachineSchemeFns) DefaultingFunction(o interface{}) {
	obj := o.(*VirtualMachine)
	// set default field values here
	glog.V(4).Infof("Defaulting fields for VirtualMachine %s", obj.Name)
}
