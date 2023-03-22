// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	corev1 "k8s.io/api/core/v1"
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
	// PersistentVolumeClaim represents a reference to a PersistentVolumeClaim
	// in the same namespace.
	//
	// More information is available at
	// https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims.
	//
	// +optional
	PersistentVolumeClaim *corev1.PersistentVolumeClaimVolumeSource `json:"persistentVolumeClaim,omitempty"`
}

// VirtualMachineVolumeProvisioningOptions specifies the provisioning options
// for a VirtualMachineVolume.
type VirtualMachineVolumeProvisioningOptions struct {
	// ThinProvision indicates whether to allocate space on demand for the
	// volume.
	//
	// +optional
	ThinProvision *bool `json:"thinProvision,omitempty"`

	// EagerZero indicates whether to write zeroes to the volume, wiping clean
	// any previous contents. Please note this causes a volume's capacity to be
	// allocated all at once.
	//
	// Please note this option may not be used concurrently with
	// ThinProvisioned.
	//
	// +optional
	EagerZero *bool `json:"eagerZero,omitempty"`
}

// VirtualMachineVolumeStatus defines the observed state of a
// VirtualMachineVolume instance.
type VirtualMachineVolumeStatus struct {
	// Name is the name of the attached volume.
	Name string `json:"name"`

	// Attached represents whether a volume has been successfully attached to
	// the VirtualMachine or not.
	// +optional
	Attached bool `json:"attached,omitempty"`

	// DiskUUID represents the underlying virtual disk UUID and is present when
	// attachment succeeds.
	// +optional
	DiskUUID string `json:"diskUUID,omitempty"`

	// Error represents the last error seen when attaching or detaching a
	// volume.  Error will be empty if attachment succeeds.
	// +optional
	Error string `json:"error,omitempty"`
}
