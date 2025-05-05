// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha4

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
)

const (
	// VirtualMachineGroupConditionPlacementReady indicates placement is ready
	// for all the initial members of the group.
	VirtualMachineGroupConditionPlacementReady = "PlacementReady"
)

type VirtualMachineGroupPowerOp struct {
	// Member references a specific group member to which this power operation
	// applies.
	Member vmopv1common.LocalObjectRef `json:"member"`

	// +optional

	// Delay is the amount of time to wait before performing the power
	// operation.
	Delay *metav1.Duration `json:"delay,omitempty"`
}

// VirtualMachineGroupSpec defines the desired state of VirtualMachineGroup.
type VirtualMachineGroupSpec struct {
	// +optional

	// Members contains references to the VirtualMachine or VirtualMachineGroup
	// objects belonging to this group. All referenced objects must exist in
	// the same namespace as this group.
	Members []vmopv1common.LocalObjectRef `json:"members,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=member

	// PowerOnOrder describes the order in which members of this group are
	// powered on.
	//
	// Please note that not all members need to be present in this list. If
	// omitted, there is no guarantee about the order in which a member's
	// power state is changed.
	PowerOnOrder []VirtualMachineGroupPowerOp `json:"powerOnOrder,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=member

	// PowerOffOrder describes the order in which members of this group are
	// powered off.
	//
	// Please note that not all members need to be present in this list. If
	// omitted, there is no guarantee about the order in which a member's
	// power state is changed.
	PowerOffOrder []VirtualMachineGroupPowerOp `json:"powerOffOrder,omitempty"`

	// +optional

	// PowerState describes the desired power state of a VirtualMachineGroup.
	//
	// Please note this field may be omitted when creating a new VM group. This
	// ensures that the power states of any existing VMs that are added to the
	// group do not have their power states changed until the group's power
	// state is explicitly altered.
	//
	// However, once the field is set to a non-empty value, it may no longer be
	// set to an empty value. This means that if the group's power state is
	// PoweredOn, and a VM whose power state is PoweredOff is added to the
	// group, that VM will be powered on.
	PowerState VirtualMachinePowerState `json:"powerState,omitempty"`
}

type VirtualMachineGroupPlacementDatastoreStatus struct {
	// Name describes the name of a datastore.
	Name string `json:"name"`

	// ID describes the datastore ID.
	ID string `json:"id,omitempty"`

	// URL describes the datastore URL.
	URL string `json:"url,omitempty"`

	// +optional

	// SupportedDiskFormat describes the list of disk formats supported by this
	// datastore.
	SupportedDiskFormats []string `json:"supportedDiskFormats,omitempty"`

	// +optional

	// DiskKey describes the device key to which this recommendation applies.
	// When omitted, this recommendation is for the VM's home directory.
	DiskKey *int32 `json:"diskKey,omitempty"`
}

type VirtualMachineGroupPlacementStatus struct {
	// Member references a specific group member to which this placement
	// status applies.
	Member vmopv1common.LocalObjectRef `json:"member"`

	// +optional

	// Zone describes the recommended zone for this VM.
	Zone string `json:"zoneID,omitempty"`

	// +optional

	// Node describes the recommended node for this VM.
	Node string `json:"node,omitempty"`

	// +optional

	// Pool describes the recommended resource pool for this VM.
	Pool string `json:"pool,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=name

	// Datastores describe the recommended datastores for this VM.
	Datastores []VirtualMachineGroupPlacementDatastoreStatus `json:"datastores,omitempty"`

	// +optional

	// Conditions describes any conditions associated with this placement.
	//
	// Generally this should just include the ReadyType condition.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// VirtualMachineGroupStatus defines the observed state of VirtualMachineGroup.
type VirtualMachineGroupStatus struct {
	// +optional
	// +listType=map
	// +listMapKey=member

	// Placement describes the placement results for the group members.
	Placement []VirtualMachineGroupPlacementStatus `json:"placement,omitempty"`

	// +optional

	// Conditions describes any conditions associated with this placement.
	//
	// - The PlacementReady condition is True when all of the placement results
	//   have a True ReadyType condition.
	// - The ReadyType condition is True when all of the members have a True
	//   ReadyType condition.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=vmgroup
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// VirtualMachineGroup is the schema for the VirtualMachineGroup API and
// represents the desired state and observed status of a VirtualMachineGroup
// resource.
type VirtualMachineGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineGroupSpec   `json:"spec,omitempty"`
	Status VirtualMachineGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VirtualMachineGroupList contains a list of VirtualMachineGroup.
type VirtualMachineGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachineGroup `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &VirtualMachineGroup{}, &VirtualMachineGroupList{})
}
