// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha4

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Namespaced,shortName=vmim
// +kubebuilder:printcolumn:name="Validated",type=string,JSONPath=`.status.conditions[?(@.type=="Validated")].status`
// +kubebuilder:printcolumn:name="StorageInSync",type=string,JSONPath=`.status.conditions[?(@.type=="StorageInSync")].status`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// VirtualMachineInfraMigration enables migrating a VirtualMachine to target
// infrastructure within its namespace. The VirtualMachineInfraMigration's
// metadata.name is the name of the target VirtualMachine.
type VirtualMachineInfraMigration struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of the resource.
	// +optional
	Spec VirtualMachineInfraMigrationSpec `json:"spec,omitempty,omitzero"`

	// status is the observed state of the resource.
	// +optional
	Status VirtualMachineInfraMigrationStatus `json:"status,omitempty,omitzero"`
}

// TargetStorageVolume identifies a PVC and its target storage policy for migration.
type TargetStorageVolume struct {
	// pvcName is the name of the PersistentVolumeClaim to migrate.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	PVCName string `json:"pvcName,omitempty"`

	// storagePolicyID is the vSphere storage policy ID to apply to the volume.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	StoragePolicyID string `json:"storagePolicyID,omitempty"`
}

// TargetStorageSpec specifies the desired storage policy changes for the VM and its volumes.
type TargetStorageSpec struct {
	// homeStoragePolicyID is the vSphere storage policy ID for the VM home directory.
	// When unset, the current home policy is kept.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	HomeStoragePolicyID string `json:"homeStoragePolicyID,omitempty"`

	// volumes is the list of PVCs attached to this VM whose storage policy should change.
	// +optional
	// +kubebuilder:validation:MaxItems=60
	// +listType=map
	// +listMapKey=pvcName
	Volumes []TargetStorageVolume `json:"volumes,omitempty"`
}

// VirtualMachineInfraMigrationSpec defines the desired state of VirtualMachineInfraMigration.
// +kubebuilder:validation:MinProperties=1
type VirtualMachineInfraMigrationSpec struct {
	// targetResourcePool is the vSphere managed object ID of the target resource pool. For example: "resgroup-103".
	// It must be the VirtualMachineInfraMigration's Supervisor namespace resource pool or a descendant of it (e.g. a
	// VKS cluster's child pool). Otherwise, the VM is not relocated and ComputeInSync is False with reason
	// ResourcePoolNamespaceMismatch.
	// +optional
	//nolint:kubeapilinter
	TargetResourcePool string `json:"targetResourcePool,omitempty"`

	// targetStorage specifies the desired storage policy changes for the VM and its volumes.
	// +optional
	TargetStorage *TargetStorageSpec `json:"targetStorage,omitempty"`
}

// VirtualMachineInfraMigrationStatus defines the observed state of VirtualMachineInfraMigration.
// +kubebuilder:validation:MinProperties=1
type VirtualMachineInfraMigrationStatus struct {
	// conditions is a list of high-level status conditions for this migration.
	// The Ready condition reports the overall outcome. Ready=True means the
	// migration has converged on the target. Ready=False with reason
	// NotSupported or Infeasible means the migration cannot proceed without a
	// spec change. Ready=False with any other reason,
	// Ready=Unknown, or no Ready condition means the migration is still in
	// progress or retrying.
	// A Ready condition whose observedGeneration is less than
	// metadata.generation describes the previous spec; wait for the current one.
	// +optional
	// +kubebuilder:validation:MaxItems=32
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// resourcePool is the vSphere managed object ID of the observed resource pool of the VM.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	ResourcePool string `json:"resourcePool,omitempty"`
}

// GetConditions returns the list of status conditions for the VirtualMachineInfraMigration.
func (v *VirtualMachineInfraMigration) GetConditions() []metav1.Condition {
	return v.Status.Conditions
}

// +kubebuilder:object:root=true

// VirtualMachineInfraMigrationList contains a list of VirtualMachineInfraMigration resources.
type VirtualMachineInfraMigrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []VirtualMachineInfraMigration `json:"items"`
}

func init() {
	objectTypes = append(objectTypes,
		&VirtualMachineInfraMigration{},
		&VirtualMachineInfraMigrationList{},
	)
}
