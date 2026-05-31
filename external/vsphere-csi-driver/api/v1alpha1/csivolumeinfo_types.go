/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	// CVIProtectionFinalizer is held by a CsiVolumeInfo while the volume has
	// an active VM attachment or is snapshot-retained. It prevents garbage
	// collection of the CsiVolumeInfo (and, via blockOwnerDeletion, the owning
	// PersistentVolume) while the volume has outstanding work.
	CVIProtectionFinalizer = "csi.vsphere.vmware.com/cvi-protection"

	// CVIDiskUUIDLabel is the label key whose value is the disk UUID
	// (VirtualDisk.Backing.Uuid). The label is set at CsiVolumeInfo creation
	// and is immutable. It enables an API-server-indexed O(1) lookup of a
	// CsiVolumeInfo by disk UUID.
	CVIDiskUUIDLabel = "cns.vmware.com/disk-uuid"
)

// OwnershipState describes who currently owns a volume's lifecycle.
//
// +kubebuilder:validation:Enum=CSI_MANAGED;TRANSFERRING_TO_VM;VM_MANAGED;TRANSFERRING_TO_CSI
type OwnershipState string

const (
	// OwnershipStateCSIManaged is the steady detached state. The FCD is
	// registered in CNS; the volume is CSI-owned. No VM owns the disk.
	OwnershipStateCSIManaged OwnershipState = "CSI_MANAGED"

	// OwnershipStateTransferringToVM is the transient state between a
	// successful CnsUnregisterVolume call and the disk being confirmed on
	// the target VM.
	OwnershipStateTransferringToVM OwnershipState = "TRANSFERRING_TO_VM"

	// OwnershipStateVMManaged is the steady attached state. No FCD exists;
	// the disk is a plain VMDK on a VM. Also used for snapshot-retained
	// volumes where vmName is empty but vCenter snapshots still reference
	// the disk.
	OwnershipStateVMManaged OwnershipState = "VM_MANAGED"

	// OwnershipStateTransferringToCSI is the transient state after the disk
	// has been removed from the VM but before it has been re-registered as
	// an FCD.
	OwnershipStateTransferringToCSI OwnershipState = "TRANSFERRING_TO_CSI"
)

// CsiVolumeInfoSpec defines the desired state of CsiVolumeInfo.
type CsiVolumeInfoSpec struct {
	// VolumeID is the CNS volume identifier. It is immutable and matches
	// PersistentVolume.spec.csi.volumeHandle.
	VolumeID string `json:"volumeID"`

	// PVCName is the name of the PersistentVolumeClaim bound at CsiVolumeInfo
	// creation, or the most recently observed bound PVC.
	PVCName string `json:"pvcName"`

	// PVName is the name of the owning PersistentVolume.
	PVName string `json:"pvName"`
}

// CsiVolumeInfoStatus defines the observed state of CsiVolumeInfo.
type CsiVolumeInfoStatus struct {
	// OwnershipState is the current lifecycle state of the volume.
	OwnershipState OwnershipState `json:"ownershipState"`

	// VMName is the name of the VirtualMachine that owns this volume, or
	// empty when the volume is detached or snapshot-retained.
	// +optional
	VMName string `json:"vmName,omitempty"`

	// VMInstanceUUID is the instance UUID of the owning VM, or empty when
	// the volume is detached or snapshot-retained.
	// +optional
	VMInstanceUUID string `json:"vmInstanceUUID,omitempty"`

	// DiskUUID is the backing disk UUID (VirtualDisk.Backing.Uuid). Populated
	// at CsiVolumeInfo creation and immutable thereafter.
	DiskUUID string `json:"diskUUID"`

	// DiskPath is the datastore path to the VMDK file. This is an
	// informational cache; it is updated just-in-time at each consumption
	// point and may be stale between updates.
	// +optional
	DiskPath string `json:"diskPath,omitempty"`

	// Conditions describes the observed conditions of the CsiVolumeInfo.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=cvi
// +kubebuilder:printcolumn:name="OwnershipState",type="string",JSONPath=".status.ownershipState"
// +kubebuilder:printcolumn:name="VMName",type="string",JSONPath=".status.vmName"
// +kubebuilder:printcolumn:name="DiskUUID",type="string",JSONPath=".status.diskUUID"

// CsiVolumeInfo carries durable per-volume identity across attach/detach
// lifecycle transitions. It is created by CSI at PVC provisioning time and
// lives for the lifetime of the owning PersistentVolume.
type CsiVolumeInfo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CsiVolumeInfoSpec   `json:"spec,omitempty"`
	Status CsiVolumeInfoStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CsiVolumeInfoList contains a list of CsiVolumeInfo.
type CsiVolumeInfoList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CsiVolumeInfo `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &CsiVolumeInfo{}, &CsiVolumeInfoList{})
}
