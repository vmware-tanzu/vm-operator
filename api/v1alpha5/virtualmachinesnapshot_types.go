// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha5

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
)

const (
	// ImportedSnapshotAnnotation on a VirtualMachineSnapshot represents that a snapshot
	// has been imported by the Mobility Service. Mobility service uses this annotation to
	// differentiate such snapshots from external snapshots. This annotation is used to
	// determine whether to approximate a VM spec for revert operations to such snapshots.
	ImportedSnapshotAnnotation = GroupName + "/imported-snapshot"
)

const (
	// VMNameForSnapshotLabel label represents the name of the
	// VirtualMachine for which the snapshot is created.
	VMNameForSnapshotLabel = "snapshot." + GroupName + "/vm-name"
)

// VirtualMachineSnapshotSpec defines the desired state of VirtualMachineSnapshot.
type VirtualMachineSnapshotSpec struct {
	// +optional

	// Memory represents whether the snapshot includes the VM's
	// memory. If true, a dump of the internal state of the virtual
	// machine (a memory dump) is included in the snapshot. Memory
	// snapshots consume time and resources and thus, take longer to
	// create.
	// The virtual machine must support this capability.
	// When set to false, the power state of the snapshot is set to
	// false.
	// For a VM in suspended state, memory is always included
	// in the snashot.
	Memory bool `json:"memory,omitempty"`

	// +optional

	// Quiesce represents the spec used for granular control over
	// quiesce details. If quiesceSpec is set and the virtual machine
	// is powered on when the snapshot is taken, VMware Tools is used
	// to quiesce the file system in the virtual machine. This assures
	// that a disk snapshot represents a consistent state of the guest
	// file systems. If the virtual machine is powered off or VMware
	// Tools are not available, the quiesce spec is ignored.
	Quiesce *QuiesceSpec `json:"quiesce,omitempty"`

	// +optional

	// Description represents a description of the snapshot.
	Description string `json:"description,omitempty"`

	// +optional

	// VMRef represents the name of the virtual machine for which the
	// snapshot is requested.
	VMRef *vmopv1common.LocalObjectRef `json:"vmRef,omitempty"`
}

// QuiesceSpec represents specifications that will be used to quiesce
// the guest when taking a snapshot.
type QuiesceSpec struct {
	// +optional
	// +kubebuilder:validation:format:=duration

	// Timeout represents the maximum time in minutes for snapshot
	// operation to be performed on the virtual machine. The timeout
	// can not be less than 5 minutes or more than 240 minutes.
	Timeout *metav1.Duration `json:"timeout,omitempty"`
}

const (
	// VirtualMachineSnapshotReadyCondition represents the condition
	// that the virtual machine snapshot is ready.
	VirtualMachineSnapshotReadyCondition = "VirtualMachineSnapshotReady"

	// VirtualMachineSnapshotInProgressReason represents that a
	// snapshot is in progress.
	VirtualMachineSnapshotInProgressReason = "VirtualMachineSnapshotInProgress"
)

// VirtualMachineSnapshotStatus defines the observed state of VirtualMachineSnapshot.
type VirtualMachineSnapshotStatus struct {
	// +optional

	// PowerState represents the observed power state of the virtual
	// machine when the snapshot was taken.
	PowerState VirtualMachinePowerState `json:"powerState,omitempty"`

	// +optional

	// Quiesced represents whether or not the snapshot was created
	// with the quiesce option to ensure a snapshot with a consistent
	// state of the guest file system.
	Quiesced bool `json:"quiesced,omitempty"`

	// +optional

	// UniqueID describes a unique identifier provider by the backing
	// infrastructure (e.g., vSphere) that can be used to distinguish
	// this snapshot from other snapshots of this virtual machine.
	UniqueID string `json:"uniqueID,omitempty"`

	// +optional

	// Children represents the snapshots for which this snapshot is
	// the parent.
	Children []vmopv1common.LocalObjectRef `json:"children,omitempty"`

	// +optional

	// Conditions describes the observed conditions of the VirtualMachine.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +optional

	// Storage describes the observed amount of storage used by a
	// VirtualMachineSnapshot, including the space for FCDs.
	Storage *VirtualMachineSnapshotStorageStatus `json:"storage,omitempty"`
}

// VirtualMachineSnapshotStorageStatus defines the observed state of a
// VirtualMachineSnapshot's storage.
type VirtualMachineSnapshotStorageStatus struct {
	// +optional

	// Used describes the observed amount of storage used by a
	// VirtualMachineSnapshot, except the space for FCDs.
	Used *resource.Quantity `json:"used,omitempty"`

	// +optional

	// Requested describes the observed amount of storage requested by a
	// VirtualMachineSnapshot. It's a list of requested storage for each
	// storage class.
	// Since a snapshot can have multiple PVCs, it can point to multiple storage
	// classes.
	Requested []VirtualMachineSnapshotStorageStatusRequested `json:"requested,omitempty"`
}

// VirtualMachineSnapshotStorageStatusRequested describes the observed amount of
// storage requested by a VirtualMachineSnapshot for a storage class.
type VirtualMachineSnapshotStorageStatusRequested struct {
	// StorageClass is the name of the storage class.
	StorageClass string `json:"storageClass"`

	// Total describes the total storage space requested by a
	// VirtualMachineSnapshot for the storage class.
	Total *resource.Quantity `json:"total"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=vmsnapshot
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// VirtualMachineSnapshot is the schema for the virtualmachinesnapshot API.
type VirtualMachineSnapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineSnapshotSpec   `json:"spec,omitempty"`
	Status VirtualMachineSnapshotStatus `json:"status,omitempty"`
}

func (vmSnapshot *VirtualMachineSnapshot) NamespacedName() string {
	return NamespacedName(vmSnapshot)
}

func (vmSnapshot *VirtualMachineSnapshot) GetConditions() []metav1.Condition {
	return vmSnapshot.Status.Conditions
}

func (vmSnapshot *VirtualMachineSnapshot) SetConditions(conditions []metav1.Condition) {
	vmSnapshot.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// VirtualMachineSnapshotList contains a list of VirtualMachineSnapshot.
type VirtualMachineSnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachineSnapshot `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &VirtualMachineSnapshot{}, &VirtualMachineSnapshotList{})
}
