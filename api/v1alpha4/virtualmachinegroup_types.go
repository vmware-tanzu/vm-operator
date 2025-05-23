// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha4

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// VirtualMachineGroupConditionPlacementReady indicates placement is ready
	// for all the initial members of the group.
	VirtualMachineGroupConditionPlacementReady = "PlacementReady"

	// VirtualMachineGroupConditionMembersOwnerRefReady indicates that
	// all the members of the group have an owner reference set to the group.
	VirtualMachineGroupConditionMembersOwnerRefReady = "MembersOwnerRefReady"

	// VirtualMachineGroupConditionPowerStateReady indicates that all desired
	// group members have their power state set to the group's power state.
	VirtualMachineGroupConditionPowerStateReady = "PowerStateReady"
)

const (
	// LastUpdatedPowerStateTimeAnnotation is the annotation key for the last
	// updated time of the power state.
	LastUpdatedPowerStateTimeAnnotation = GroupName + "/last-updated-power-state-time"

	// ScheduledPowerStateTimeAnnotation is the annotation key for the scheduled
	// time when a power state change should be applied.
	ScheduledPowerStateTimeAnnotation = GroupName + "/scheduled-power-state-time"

	// ScheduledPowerStateAnnotation is the annotation key for the power state
	// that should be applied at the scheduled time.
	ScheduledPowerStateAnnotation = GroupName + "/scheduled-power-state"
)

type GroupMember struct {
	// Name is the name of member of this group.
	Name string `json:"name"`

	// +optional
	// +kubebuilder:default=VirtualMachine
	// +kubebuilder:validation:Enum=VirtualMachine;VirtualMachineGroup

	// Kind is the kind of member of this group, which can be either
	// VirtualMachine or VirtualMachineGroup.
	//
	// If omitted, it defaults to VirtualMachine.
	Kind string `json:"kind,omitempty"`
}

type VirtualMachineGroupPowerOp struct {
	// Name is the name of member of this group.
	Name string `json:"name"`

	// +optional
	// +kubebuilder:default=VirtualMachine
	// +kubebuilder:validation:Enum=VirtualMachine;VirtualMachineGroup
	//
	// Kind is the kind of member of this group, which can be either
	// VirtualMachine or VirtualMachineGroup.
	//
	// If omitted, it defaults to VirtualMachine.
	Kind string `json:"kind,omitempty"`

	// +optional

	// Delay is the amount of time to wait before performing the power
	// operation.
	//
	// If omitted, the power operation will be applied immediately.
	Delay *metav1.Duration `json:"delay,omitempty"`
}

// VirtualMachineGroupSpec defines the desired state of VirtualMachineGroup.
type VirtualMachineGroupSpec struct {
	// +optional

	// GroupName describes the name of the group that this group belongs to.
	//
	// If omitted, this group is not a member of any other group.
	GroupName string `json:"groupName,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=kind
	// +listMapKey=name

	// Members describes the names of VirtualMachine or VirtualMachineGroup
	// objects that are members of this group. The VM or VM Group objects must
	// be in the same namespace as this group.
	Members []GroupMember `json:"members,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=kind
	// +listMapKey=name

	// PowerOnOp describes the order in which members of this group are
	// powered on.
	//
	// If this field is empty, all members of the group will be powered on
	// immediately when the group's power state changes to PoweredOn.
	//
	// If this field is not empty, only the listed members will be powered on,
	// each after the delay specified for that member. Members not included in
	// this list will not be powered on when the group's power state changes.
	PowerOnOp []VirtualMachineGroupPowerOp `json:"powerOnOp,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=kind
	// +listMapKey=name

	// PowerOffOp describes the order in which members of this group are
	// powered off.
	//
	// If this field is empty, all members of the group will be powered off
	// immediately when the group's power state changes to PoweredOff.
	//
	// If this field is not empty, only the listed members will be powered off,
	// each after the delay specified for that member. Members not included in
	// this list will not be powered off when the group's power state changes.
	PowerOffOp []VirtualMachineGroupPowerOp `json:"powerOffOp,omitempty"`

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

	// +optional
	// +kubebuilder:default=TrySoft

	// PowerOffMode describes the desired behavior when powering off a VM Group.
	//
	// There are three, supported power off modes: Hard, Soft, and
	// TrySoft. The first mode, Hard, is the equivalent of a physical
	// system's power cord being ripped from the wall. The Soft mode
	// requires the VM's guest to have VM Tools installed and attempts to
	// gracefully shutdown the VM. Its variant, TrySoft, first attempts
	// a graceful shutdown, and if that fails or the VM is not in a powered off
	// state after five minutes, the VM is halted.
	//
	// Please note this field is only propagated to the group's members when
	// the group's power state is changed.
	//
	// If omitted, the mode defaults to TrySoft.
	PowerOffMode VirtualMachinePowerOpMode `json:"powerOffMode,omitempty"`

	// +optional
	// +kubebuilder:default=TrySoft

	// SuspendMode describes the desired behavior when suspending a VM Group.
	//
	// There are three, supported suspend modes: Hard, Soft, and
	// TrySoft. The first mode, Hard, is where vSphere suspends the VM to
	// disk without any interaction inside of the guest. The Soft mode
	// requires the VM's guest to have VM Tools installed and attempts to
	// gracefully suspend the VM. Its variant, TrySoft, first attempts
	// a graceful suspend, and if that fails or the VM is not in a put into
	// standby by the guest after five minutes, the VM is suspended.
	//
	// Please note this field is only propagated to the group's members when
	// the group's power state is changed.
	//
	// If omitted, the mode defaults to TrySoft.
	SuspendMode VirtualMachinePowerOpMode `json:"suspendMode,omitempty"`
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

type VirtualMachinePlacementStatus struct {
	// Name is the name of VirtualMachine member of this group.
	Name string `json:"name"`

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

type VirtualMachinePowerStateStatus struct {
	// Name is the name of VirtualMachine member of this group.
	Name string `json:"name"`

	// +optional

	// PowerState describes the observed power state of the VirtualMachine
	// group member.
	PowerState VirtualMachinePowerState `json:"powerState,omitempty"`
}

// VirtualMachineGroupStatus defines the observed state of VirtualMachineGroup.
type VirtualMachineGroupStatus struct {
	// +optional
	// +listType=map
	// +listMapKey=name

	// Placement describes the placement results for the group members.
	Placement []VirtualMachinePlacementStatus `json:"placement,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=name

	// PowerState describes the observed power state of all direct or indirect
	// VirtualMachine members of this group.
	PowerState []VirtualMachinePowerStateStatus `json:"powerState,omitempty"`

	// +optional

	// LastUpdatedPowerStateTime describes the observed time when the power
	// state of the group was last updated.
	LastUpdatedPowerStateTime *metav1.Time `json:"lastUpdatedPowerStateTime,omitempty"`

	// +optional

	// Conditions describes any conditions associated with this VM Group.
	//
	// - The MembersOwnerReferenceReady condition is True when all of the
	//   members of the group exist and have an owner reference to the group.
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

func (vm *VirtualMachineGroup) GetConditions() []metav1.Condition {
	return vm.Status.Conditions
}

func (vm *VirtualMachineGroup) SetConditions(conditions []metav1.Condition) {
	vm.Status.Conditions = conditions
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
