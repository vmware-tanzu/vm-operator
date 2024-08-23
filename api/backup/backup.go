// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// +kubebuilder:object:generate=true

package backup

import (
	corev1 "k8s.io/api/core/v1"
)

// PVCDiskData contains the backup data of a PVC disk attached to VM.
type PVCDiskData struct {
	// Filename is the datastore path to the virtual disk.
	FileName string
	// PVCName is the name of the PVC backed by the virtual disk.
	PVCName string
	// AccessMode is the access modes of the PVC backed by the virtual disk.
	AccessModes []corev1.PersistentVolumeAccessMode
}

// ClassicDiskData contains the backup data of a classic (static) disk attached
// to VM.
type ClassicDiskData struct {
	// Filename is the datastore path to the virtual disk.
	FileName string
}

const (
	// EnableAutoRegistrationExtraConfigKey is the ExtraConfig key that can be
	// set on a virtual machine to opt-into the automatic registration workflow.
	//
	// A "registration" refers to adopting a virtual machine so it is managed by
	// VM operator on Supervisor. Typically, this involves creating a new
	// VirtualMachine resource, or updating an existing VirtualMachine resource on
	// Supervisor to conform to the virtual machine on vSphere.
	//
	// After a restore from a backup/restore vendor, or a failover from a disaster recovery
	// solution, vCenter automatically attempts to register the restored virtual machine with
	// Supervisor. This is referred to as "automatic registration" workflow.
	//
	// Virtual machines can opt-into this workflow by specifying this key. If this key is not
	// set to a positive value, backup/restore or disaster recovery solutions are responsible
	// to register the VM with Supervisor.
	//
	// Any of the following values for this ExtraConfig key result in the virtual
	// machine participating in automatic registration:
	// "1", "on", "t", "true", "y", or "yes".
	EnableAutoRegistrationExtraConfigKey = "vmservice.virtualmachine.enableAutomaticRegistration"
)
