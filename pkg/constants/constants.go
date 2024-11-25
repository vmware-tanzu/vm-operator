// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package constants

import (
	vimtypes "github.com/vmware/govmomi/vim25/types"
)

const (
	createdAtPrefix = "vmoperator.vmware.com/created-at-"

	// CreatedAtBuildVersionAnnotationKey is set on VirtualMachine
	// objects when an object's metadata.generation value is 1.
	// The value of this annotation may be empty, indicating the VM was first
	// created before this annotation was used. This information itself is
	// in fact useful, as it still indicates something about the build version
	// at which an object was first created.
	CreatedAtBuildVersionAnnotationKey = createdAtPrefix + "build-version"

	// CreatedAtSchemaVersionAnnotationKey is set on VirtualMachine
	// objects when an object's metadata.generation value is 1.
	// The value of this annotation may be empty, indicating the VM was first
	// created before this annotation was used. This information itself is
	// in fact useful, as it still indicates something about the schema version
	// at which an object was first created.
	CreatedAtSchemaVersionAnnotationKey = createdAtPrefix + "schema-version"

	// MinSupportedHWVersionForPVC is the supported virtual hardware version for
	// persistent volumes.
	MinSupportedHWVersionForPVC = vimtypes.VMX15

	// MinSupportedHWVersionForVTPM is the supported virtual hardware version
	// for a Virtual Trusted Platform Module (vTPM).
	MinSupportedHWVersionForVTPM = vimtypes.VMX14

	// MinSupportedHWVersionForPCIPassthruDevices is the supported virtual
	// hardware version for NVidia PCI devices.
	MinSupportedHWVersionForPCIPassthruDevices = vimtypes.VMX17
)
