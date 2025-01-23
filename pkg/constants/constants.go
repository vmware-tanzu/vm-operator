// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
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

	// VMICacheLabelKey is applied to resources that need to be reconciled when
	// the VirtualMachineImageCache resource specified by the label's value is
	// updated.
	VMICacheLabelKey = "vmicache.vmoperator.vmware.com/name"

	// VMICacheLocationAnnotationKey is applied to resources waiting on a
	// VirtualMachineImageCache's disks to be available at the specified
	// location.
	// The value of this annotation is a comma-delimited string that specifies
	// the datacenter ID and datastore ID, ex. datacenter-50,datastore-42.
	VMICacheLocationAnnotationKey = "vmicache.vmoperator.vmware.com/location"

	// FastDeployAnnotationKey is applied to VirtualMachine resources that want
	// to control the mode of FastDeploy used to create the underlying VM.
	// Please note, this annotation only has any effect if the FastDeploy FSS is
	// enabled.
	// The valid values for this annotation are "direct" and "linked." If the
	// FSS is enabled and:
	//
	//   - the value is "direct," then the VM is deployed from cached disks.
	//   - the value is "linked," then the VM is deployed as a linked clone.
	//   - the value is empty or the annotation is not present, then the mode
	//     is derived from the environment variable FAST_DEPLOY_MODE.
	//   - the value is anything else, then fast deploy is not used to deploy
	//     the VM.
	FastDeployAnnotationKey = "vmoperator.vmware.com/fast-deploy"

	// FastDeployModeDirect is a fast deploy mode. See FastDeployAnnotationKey
	// for more information.
	FastDeployModeDirect = "direct"

	// FastDeployModeLinked is a fast deploy mode. See FastDeployAnnotationKey
	// for more information.
	FastDeployModeLinked = "linked"
)
