// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package api

// GroupName specifies the group name for VM operator.
const GroupName = "vmoperator.vmware.com"

// ToPersistentVolumeAccessModes returns a string slice from a slice of T.
// This is useful when converting a slice of corev1.PersistentVolumeAccessMode
// to a string slice.
func ToPersistentVolumeAccessModes[T ~string](in []T) []string {
	out := make([]string, len(in))
	for i := range in {
		out[i] = string(in[i])
	}
	return out
}

// FromPersistentVolumeAccessModes returns a slice of T from a string slice.
// This is useful when converting a slice of strings to a slice of
// corev1.PersistentVolumeAccessMode.
func FromPersistentVolumeAccessModes[T ~string](in []string) []T {
	out := make([]T, len(in))
	for i := range in {
		out[i] = T(in[i])
	}
	return out
}

// PVCDiskData contains the backup data of a PVC disk attached to VM.
type PVCDiskData struct {
	// Filename is the datastore path to the virtual disk.
	FileName string
	// PVCName is the name of the PVC backed by the virtual disk.
	PVCName string
	// AccessMode is the access modes of the PVC backed by the virtual disk.
	AccessModes []string
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

// VirtualMachine backup/restore related constants.
const (
	// VMResourceYAMLExtraConfigKey is the ExtraConfig key to persist VM
	// Kubernetes resource YAML, compressed using gzip and base64-encoded.
	VMResourceYAMLExtraConfigKey = "vmservice.virtualmachine.resource.yaml"
	// AdditionalResourcesYAMLExtraConfigKey is the ExtraConfig key to persist
	// VM-relevant Kubernetes resource YAML, compressed using gzip and base64-encoded.
	AdditionalResourcesYAMLExtraConfigKey = "vmservice.virtualmachine.additional.resources.yaml"
	// PVCDiskDataExtraConfigKey is the ExtraConfig key to persist the VM's
	// PVC disk data in JSON, compressed using gzip and base64-encoded.
	PVCDiskDataExtraConfigKey = "vmservice.virtualmachine.pvc.disk.data"

	// ClassicDiskDataExtraConfigKey is the ExtraConfig key to persist the VM's
	// classic (static) disk data in JSON, compressed using gzip and base64-encoded.
	ClassicDiskDataExtraConfigKey = "vmservice.virtualmachine.classicDiskData"

	// BackupVersionExtraConfigKey is the ExtraConfig key that indicates
	// the version of the VM's last backup. It is a monotonically increasing counter and
	// is only supposed to be used by IaaS control plane and vCenter for virtual machine registration
	// post a restore operation.
	//
	// The BackupVersionExtraConfigKey on the vSphere VM and the VirtualMachineBackupVersionAnnotation
	// on the VM resource in Supervisor indicate whether the backups are in sync.
	BackupVersionExtraConfigKey = "vmservice.virtualmachine.backupVersion"

	// DisableAutoRegistrationExtraConfigKey is the ExtraConfig key that can be
	// set to "true" (case insensitive) on a virtual machine to opt-out of the
	// automatic registration workflow.
	//
	// A "registration" refers to adopting a virtual machine so it is managed by
	// VM operator on Supervisor. Typically, this involves creating a new
	// VirtualMachine resource, or updating an existing VirtualMachine resource on
	// Supervisor to conform to the virtual machine on vSphere.
	//
	// After a restore from a backup restore vendor, or a failover from a disaster
	// recovery solution, vCenter automatically attempts to register the restored
	// virtual machine with Supervisor. If the automatic registration is skipped by
	// specifying this key, backup/restore and/or disaster recovery solutions are
	// responsible to register the VM with Supervisor.
	DisableAutoRegistrationExtraConfigKey = "vmservice.virtualmachine.disableAutomaticRegistration"

	// TestFailoverLabelKey label signifies that a VirtualMachine is going through
	// a test recovery plan scenario. Typically, this involves failing over a VM
	// to a dedicated temporary network so as to not exhaust, and cause IP address
	// collisions with the production network. This label on a VirtualMachine
	// custom resource allows vendors to change the VM's network interface to
	// connect to the test network after the VM is registered post a failover.
	//
	// VM operator does not support mutable networks, so this label allows only
	// select vendors (e.g., SRM) to support recovery plan test workflows while we
	// build support for true network mutability.
	//
	// The value of this label does not matter. Its presence is considered a
	// sufficient evidence to indicate that the virtual machine is going through
	// test fail-over.
	TestFailoverLabelKey = "virtualmachine." + GroupName + "/test-failover"
)
