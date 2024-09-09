// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha3

import (
	"slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// +kubebuilder:validation:Enum=Thin;Thick;ThickEagerZero

// VirtualMachineVolumeProvisioningMode is the type used to express the
// desired or observed provisioning mode for a virtual machine disk.
type VirtualMachineVolumeProvisioningMode string

const (
	VirtualMachineVolumeProvisioningModeThin           VirtualMachineVolumeProvisioningMode = "Thin"
	VirtualMachineVolumeProvisioningModeThick          VirtualMachineVolumeProvisioningMode = "Thick"
	VirtualMachineVolumeProvisioningModeThickEagerZero VirtualMachineVolumeProvisioningMode = "ThickEagerZero"
)

// VirtualMachineVolume represents a named volume in a VM.
type VirtualMachineVolume struct {
	// Name represents the volume's name. Must be a DNS_LABEL and unique within
	// the VM.
	Name string `json:"name"`

	// VirtualMachineVolumeSource represents the location and type of a volume
	// to mount.
	VirtualMachineVolumeSource `json:",inline"`
}

// VirtualMachineVolumeSource represents the source location of a volume to
// mount. Only one of its members may be specified.
type VirtualMachineVolumeSource struct {
	// +optional

	// PersistentVolumeClaim represents a reference to a PersistentVolumeClaim
	// in the same namespace.
	//
	// More information is available at
	// https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims.
	PersistentVolumeClaim *PersistentVolumeClaimVolumeSource `json:"persistentVolumeClaim,omitempty"`
}

// PersistentVolumeClaimVolumeSource is a composite for the Kubernetes
// corev1.PersistentVolumeClaimVolumeSource and instance storage options.
type PersistentVolumeClaimVolumeSource struct {
	corev1.PersistentVolumeClaimVolumeSource `json:",inline" yaml:",inline"`

	// +optional

	// InstanceVolumeClaim is set if the PVC is backed by instance storage.
	InstanceVolumeClaim *InstanceVolumeClaimVolumeSource `json:"instanceVolumeClaim,omitempty"`
}

// InstanceVolumeClaimVolumeSource contains information about the instance
// storage volume claimed as a PVC.
type InstanceVolumeClaimVolumeSource struct {
	// StorageClass is the name of the Kubernetes StorageClass that provides
	// the backing storage for this instance storage volume.
	StorageClass string `json:"storageClass"`

	// Size is the size of the requested instance storage volume.
	Size resource.Quantity `json:"size"`
}

// +kubebuilder:validation:Enum=Classic;Managed

// VirtualMachineVolumeType describes the type of a VirtualMachine volume.
type VirtualMachineVolumeType string

const (
	// VirtualMachineStorageDiskTypeClassic describes a classic virtual disk,
	// such as the boot disk for a VirtualMachine deployed from a VM Image of
	// type OVF.
	VirtualMachineStorageDiskTypeClassic VirtualMachineVolumeType = "Classic"

	// VirtualMachineStorageDiskTypeManaged describes a managed virtual disk,
	// such as persistent volumes.
	VirtualMachineStorageDiskTypeManaged VirtualMachineVolumeType = "Managed"
)

// VirtualMachineVolumeStatus defines the observed state of a
// VirtualMachineVolume instance.
type VirtualMachineVolumeStatus struct {
	// Name is the name of the attached volume.
	Name string `json:"name"`

	// +kubebuilder:default=Managed

	// Type is the type of the attached volume.
	Type VirtualMachineVolumeType `json:"type"`

	// +optional

	// Limit describes the storage limit for the volume.
	Limit *resource.Quantity `json:"limit,omitempty"`

	// +optional

	// Used describes the observed, non-shared size of the volume on disk.
	// For example, if this is a linked-clone's boot volume, this value
	// represents the space consumed by the linked clone, not the parent.
	Used *resource.Quantity `json:"used,omitempty"`

	// +optional

	// Attached represents whether a volume has been successfully attached to
	// the VirtualMachine or not.
	Attached bool `json:"attached,omitempty"`

	// +optional

	// DiskUUID represents the underlying virtual disk UUID and is present when
	// attachment succeeds.
	DiskUUID string `json:"diskUUID,omitempty"`

	// +optional

	// Error represents the last error seen when attaching or detaching a
	// volume.  Error will be empty if attachment succeeds.
	Error string `json:"error,omitempty"`
}

// SortVirtualMachineVolumeStatuses sorts the provided list of
// VirtualMachineVolumeStatus objects.
func SortVirtualMachineVolumeStatuses(s []VirtualMachineVolumeStatus) {
	slices.SortFunc(s, func(a, b VirtualMachineVolumeStatus) int {
		switch {
		case a.DiskUUID < b.DiskUUID:
			return -1
		case a.DiskUUID > b.DiskUUID:
			return 1
		default:
			return 0
		}
	})
}

// VirtualMachineStorageStatus defines the observed state of a VirtualMachine's
// storage.
type VirtualMachineStorageStatus struct {

	// +optional

	// Committed is the total storage space committed to this VirtualMachine.
	Committed *resource.Quantity `json:"committed,omitempty"`

	// +optional

	// Uncommitted is the total storage space potentially used by this
	// VirtualMachine.
	Uncommitted *resource.Quantity `json:"uncommitted,omitempty"`

	// +optional

	// Unshared is the total storage space occupied by this VirtualMachine that
	// is not shared with any other VirtualMachine.
	Unshared *resource.Quantity `json:"unshared,omitempty"`
}
