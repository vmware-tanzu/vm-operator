// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VGPUDevice contains the configuration corresponding to a vGPU device.
type VGPUDevice struct {
	ProfileName string `json:"profileName"`
}

// DynamicDirectPathIODevice contains the configuration corresponding to a Dynamic DirectPath I/O device.
type DynamicDirectPathIODevice struct {
	VendorID    int    `json:"vendorID"`
	DeviceID    int    `json:"deviceID"`
	// +optional
	CustomLabel string `json:"customLabel,omitempty"`
}

// InstanceStorage provides information used to configure instance
// storage volumes for a VirtualMachine.
type InstanceStorage struct {
	// StorageClass refers to the name of a StorageClass resource used to
	// provide the storage for the configured instance storage volumes.
	// The value of this field has no relationship to or bearing on the field
	// virtualMachine.spec.storageClass. Please note the referred StorageClass
	// must be available in the same namespace as the VirtualMachineClass that
	// uses it for configuring instance storage.
	StorageClass string `json:"storageClass,omitempty"`

	// Volumes describes instance storage volumes created for a VirtualMachine
	// instance that use this VirtualMachineClass.
	Volumes []InstanceStorageVolume `json:"volumes,omitempty"`
}

// InstanceStorageVolume contains information required to create an
// instance storage volume on a VirtualMachine.
type InstanceStorageVolume struct {
	Size resource.Quantity `json:"size"`
}

// VirtualDevices contains information about the virtual devices associated with a VirtualMachineClass.
type VirtualDevices struct {
	// +optional
	VGPUDevices                []VGPUDevice                `json:"vgpuDevices,omitempty" patchStrategy:"merge" patchMergeKey:"profileName"`
	// +optional
	DynamicDirectPathIODevices []DynamicDirectPathIODevice `json:"dynamicDirectPathIODevices,omitempty"`
}

// VirtualMachineClassHardware describes a virtual hardware resource specification.
type VirtualMachineClassHardware struct {
	Cpus    int64             `json:"cpus,omitempty"`
	Memory  resource.Quantity `json:"memory,omitempty"`
	// +optional
	Devices VirtualDevices    `json:"devices,omitempty"`
	// +optional
	InstanceStorage InstanceStorage `json:"instanceStorage,omitempty"`
}

// VirtualMachineResourceSpec describes a virtual hardware policy specification.
type VirtualMachineResourceSpec struct {
	Cpu    resource.Quantity `json:"cpu,omitempty"`
	Memory resource.Quantity `json:"memory,omitempty"`
}

// VirtualMachineClassResources describes the virtual hardware resource reservations and limits configuration to be used
// by a VirtualMachineClass.
type VirtualMachineClassResources struct {
	Requests VirtualMachineResourceSpec `json:"requests,omitempty"`
	Limits   VirtualMachineResourceSpec `json:"limits,omitempty"`
}

// VirtualMachineClassPolicies describes the policy configuration to be used by a VirtualMachineClass.
type VirtualMachineClassPolicies struct {
	Resources VirtualMachineClassResources `json:"resources,omitempty"`
}

// VirtualMachineClassSpec defines the desired state of VirtualMachineClass
type VirtualMachineClassSpec struct {
	// Hardware describes the configuration of the VirtualMachineClass attributes related to virtual hardware.  The
	// configuration specified in this field is used to customize the virtual hardware characteristics of any VirtualMachine
	// associated with this VirtualMachineClass.
	Hardware VirtualMachineClassHardware `json:"hardware,omitempty"`

	// Policies describes the configuration of the VirtualMachineClass attributes related to virtual infrastructure
	// policy.  The configuration specified in this field is used to customize various policies related to
	// infrastructure resource consumption.
	Policies VirtualMachineClassPolicies `json:"policies,omitempty"`

	// Description describes the configuration of the VirtualMachineClass which is not related to virtual hardware
	// or infrastructure policy. This field is used to address remaining specs about this VirtualMachineClass.
	// +optional
	Description string `json:"description,omitempty"`
}

// VirtualMachineClassStatus defines the observed state of VirtualMachineClass.  VirtualMachineClasses are immutable,
// non-dynamic resources, so this status is currently unused.
type VirtualMachineClassStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=vmclass
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="CPU",type="string",JSONPath=".spec.hardware.cpus"
// +kubebuilder:printcolumn:name="Memory",type="string",JSONPath=".spec.hardware.memory"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="VGPUDevicesProfileNames",type="string",priority=1,JSONPath=".spec.hardware.devices.vgpuDevices[*].profileName"
// +kubebuilder:printcolumn:name="PassthroughDeviceIDs",type="string",priority=1,JSONPath=".spec.hardware.devices.dynamicDirectPathIODevices[*].deviceID"

// VirtualMachineClass is the Schema for the virtualmachineclasses API.
// A VirtualMachineClass represents the desired specification and the observed status of a VirtualMachineClass
// instance.  A VirtualMachineClass represents a policy and configuration resource which defines a set of attributes to
// be used in the configuration of a VirtualMachine instance.  A VirtualMachine resource references a
// VirtualMachineClass as a required input.
type VirtualMachineClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineClassSpec   `json:"spec,omitempty"`
	Status VirtualMachineClassStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VirtualMachineClassList contains a list of VirtualMachineClass.
type VirtualMachineClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachineClass `json:"items"`
}

func init() {
	RegisterTypeWithScheme(&VirtualMachineClass{}, &VirtualMachineClassList{})
}
