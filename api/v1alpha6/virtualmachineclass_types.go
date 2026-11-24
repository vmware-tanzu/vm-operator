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

// VirtualMachineClassResourceSpec describes CPU and memory value ranges for
// requests or limits.
type VirtualMachineClassResourceSpec struct {
	// CPU is measured in cores, where millicores are allowed.
	// +optional
	CPU *ResourceQuantityRange `json:"cpu,omitempty"`

	// Memory is measured in bytes.
	// +optional
	Memory *ResourceQuantityRange `json:"memory,omitempty"`
}

// VirtualMachineClassResources describes reservations and limits.
//
// +kubebuilder:validation:XValidation:rule="!self.?requests.?cpu.?default.hasValue() || !self.?limits.?cpu.?default.hasValue() || quantity(string(self.requests.cpu.default)).compareTo(quantity(string(self.limits.cpu.default))) <= 0",message="the CPU request default must not exceed the CPU limit default"
// +kubebuilder:validation:XValidation:rule="!self.?requests.?memory.?default.hasValue() || !self.?limits.?memory.?default.hasValue() || quantity(string(self.requests.memory.default)).compareTo(quantity(string(self.limits.memory.default))) <= 0",message="the memory request default must not exceed the memory limit default"
type VirtualMachineClassResources struct {
	// +kubebuilder:validation:XValidation:rule="[?self.?cpu.?min, ?self.?cpu.?max, ?self.?cpu.?default, ?self.?memory.?min, ?self.?memory.?max, ?self.?memory.?default].all(q, quantity(string(q)).compareTo(quantity('0')) >= 0)",message="requests must not be negative"
	// Requests describes CPU and memory reservations. A zero request is no
	// reservation.
	// +optional
	Requests VirtualMachineClassResourceSpec `json:"requests,omitempty"`

	// +kubebuilder:validation:XValidation:rule="[?self.?cpu.?min, ?self.?cpu.?max, ?self.?cpu.?default, ?self.?memory.?min, ?self.?memory.?max, ?self.?memory.?default].all(q, quantity(string(q)).compareTo(quantity('0')) > 0)",message="limits must be greater than 0"
	// Limits describes CPU and memory limits. A limit without a default is
	// unlimited, and a limit without a max lets a VM choose any limit,
	// including unlimited.
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
	// +kubebuilder:validation:XValidation:rule="[?self.?min, ?self.?max, ?self.?default].all(q, quantity(string(q)).isInteger() && quantity(string(q)).asInteger() >= 1 && quantity(string(q)).asInteger() <= 2147483647)",message="CPU must be a whole number of vCPUs between 1 and 2147483647"

	// CPU is the guest-visible vCPU count.
	CPU *ResourceQuantityRange `json:"cpu,omitempty"`

	// +optional
	// +kubebuilder:validation:XValidation:rule="[?self.?min, ?self.?max, ?self.?default].all(q, quantity(string(q)).compareTo(quantity('0')) > 0)",message="memory must be greater than 0"

	// Memory is the guest-visible memory, in bytes.
	Memory *ResourceQuantityRange `json:"memory,omitempty"`

	// +optional

	// Resources describes reservations and limits.
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
// +kubebuilder:printcolumn:name="CPU",type="string",JSONPath=".spec.cpu.default"
// +kubebuilder:printcolumn:name="Memory",type="string",JSONPath=".spec.memory.default"
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
