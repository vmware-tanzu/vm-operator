// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha6

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VGPUDevice contains the configuration corresponding to a vGPU device.
type VGPUDevice struct {
	ProfileName string `json:"profileName"`
}

// DynamicDirectPathIODevice contains the configuration corresponding to a
// Dynamic DirectPath I/O device.
type DynamicDirectPathIODevice struct {
	VendorID int64 `json:"vendorID"`
	DeviceID int64 `json:"deviceID"`

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

// VirtualDevices contains information about the virtual devices associated
// with a VirtualMachineClass.
type VirtualDevices struct {
	// +optional
	// +listType=map
	// +listMapKey=profileName
	VGPUDevices []VGPUDevice `json:"vgpuDevices,omitempty"`

	// +optional
	DynamicDirectPathIODevices []DynamicDirectPathIODevice `json:"dynamicDirectPathIODevices,omitempty"`
}

// VirtualMachineClassHardware describes hardware presets for a class.
type VirtualMachineClassHardware struct {
	// Devices describes the class's device configuration.
	// +optional
	Devices VirtualDevices `json:"devices,omitempty"`

	// InstanceStorage describes instance storage volumes.
	// +optional
	InstanceStorage InstanceStorage `json:"instanceStorage,omitempty"`
}

// VirtualMachineClassResourceSpec describes CPU and memory value ranges.
type VirtualMachineClassResourceSpec struct {
	// CPU is measured in vCPUs, including requests and limits.
	// +optional
	CPU *ResourceQuantityRange `json:"cpu,omitempty"`

	// Memory is measured in bytes.
	// +optional
	Memory *ResourceQuantityRange `json:"memory,omitempty"`
}

// VirtualMachineClassResources describes guest size, reservations and limits.
type VirtualMachineClassResources struct {
	// +kubebuilder:validation:XValidation:rule="!has(self.cpu) || !has(self.cpu.min) || (quantity(string(self.cpu.min)).isInteger() && quantity(string(self.cpu.min)).asInteger() >= 0 && quantity(string(self.cpu.min)).asInteger() <= 2147483647)",message="CPU size min must be a whole number between 0 and 2147483647"
	// +kubebuilder:validation:XValidation:rule="!has(self.cpu) || !has(self.cpu.max) || (quantity(string(self.cpu.max)).isInteger() && quantity(string(self.cpu.max)).asInteger() >= 0 && quantity(string(self.cpu.max)).asInteger() <= 2147483647)",message="CPU size max must be a whole number between 0 and 2147483647"
	// +kubebuilder:validation:XValidation:rule="!has(self.cpu) || !has(self.cpu.default) || (quantity(string(self.cpu.default)).isInteger() && quantity(string(self.cpu.default)).asInteger() >= 0 && quantity(string(self.cpu.default)).asInteger() <= 2147483647)",message="CPU size default must be a whole number between 0 and 2147483647"
	// Size describes the guest-visible CPU and memory configuration.
	// +optional
	Size VirtualMachineClassResourceSpec `json:"size,omitempty"`

	// Requests describes CPU and memory reservations.
	// +optional
	Requests VirtualMachineClassResourceSpec `json:"requests,omitempty"`

	// Limits describes CPU and memory limits.
	// +optional
	Limits VirtualMachineClassResourceSpec `json:"limits,omitempty"`
}

// VirtualMachineClassSpec defines the desired state of VirtualMachineClass.
type VirtualMachineClassSpec struct {
	// +optional

	// ControllerName describes the name of the controller responsible for
	// reconciling VirtualMachine resources that are realized from this
	// VirtualMachineClass.
	//
	// When omitted, controllers reconciling VirtualMachine resources determine
	// the default controller name from the environment variable
	// DEFAULT_VM_CLASS_CONTROLLER_NAME. If this environment variable is not
	// defined or empty, it defaults to vmoperator.vmware.com/vsphere.
	//
	// Once a non-empty value is assigned to this field, attempts to set this
	// field to an empty value will be silently ignored.
	ControllerName string `json:"controllerName,omitempty"`

	// +optional

	// Hardware describes the configuration of the VirtualMachineClass
	// attributes related to virtual hardware. The configuration specified in
	// this field is used to customize the virtual hardware characteristics of
	// any VirtualMachine associated with this VirtualMachineClass.
	Hardware VirtualMachineClassHardware `json:"hardware,omitempty"`

	// +optional

	// Resources describes guest size, reservations and limits.
	Resources VirtualMachineClassResources `json:"resources,omitempty"`

	// +optional

	// Description describes the configuration of the VirtualMachineClass which
	// is not related to virtual hardware or infrastructure policy. This field
	// is used to address remaining specs about this VirtualMachineClass.
	Description string `json:"description,omitempty"`

	// +optional
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:Type=object
	// +kubebuilder:pruning:PreserveUnknownFields

	// ConfigSpec describes additional configuration information for a
	// VirtualMachine.
	// The contents of this field are the VirtualMachineConfigSpec data object
	// (https://bit.ly/3HDtiRu) marshaled to JSON using the discriminator
	// field "_typeName" to preserve type information.
	ConfigSpec json.RawMessage `json:"configSpec,omitempty"`

	// +optional

	// ReservedProfileID describes the reservation profile associated with
	// the namespace-scoped VirtualMachineClass object.
	ReservedProfileID string `json:"reservedProfileID,omitempty"`

	// +optional
	// +kubebuilder:validation:Minimum=0

	// ReservedSlots describes the number of slots reserved for VMs that use
	// this VirtualMachineClass.
	// This field is only valid in conjunction with reservedProfileID.
	ReservedSlots int32 `json:"reservedSlots,omitempty"`
}

// VirtualMachineClassStatus defines the observed state of VirtualMachineClass.
type VirtualMachineClassStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=vmclass
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="CPU",type="string",JSONPath=".spec.resources.size.cpu.default"
// +kubebuilder:printcolumn:name="Memory",type="string",JSONPath=".spec.resources.size.memory.default"
// +kubebuilder:printcolumn:name="Capabilities",type="string",priority=1,JSONPath=".status.capabilities"

// VirtualMachineClass is the schema for the virtualmachineclasses API and
// represents the desired state and observed status of a virtualmachineclasses
// resource.
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
	objectTypes = append(objectTypes, &VirtualMachineClass{}, &VirtualMachineClassList{})
}
