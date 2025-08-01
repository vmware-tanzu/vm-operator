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

	upgradedToPrefix = "vmoperator.vmware.com/upgraded-to-"

	// UpgradedToBuildVersionAnnotationKey is set on VirtualMachine
	// objects when the function "ReconcileSchemaUpgrade" from
	// pkg/providers/vsphere/upgrade/virtualmachine/vm_schema_upgrade.go is
	// executed.
	//
	// Please note, a validation webhook will deny any patch/update operations
	// on VirtualMachine objects from unprivileged users until such time the
	// object has this annotation with a value that matches the current build
	// version.
	UpgradedToBuildVersionAnnotationKey = upgradedToPrefix + "build-version"

	// UpgradedToSchemaVersionAnnotationKey is set on VirtualMachine
	// objects when the function "ReconcileSchemaUpgrade" from
	// pkg/providers/vsphere/upgrade/virtualmachine/vm_schema_upgrade.go is
	// executed.
	//
	// Please note, a validation webhook will deny any patch/update operations
	// on VirtualMachine objects from unprivileged users until such time the
	// object has this annotation with a value that matches the current build
	// version.
	UpgradedToSchemaVersionAnnotationKey = upgradedToPrefix + "schema-version"

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

	// LastRestartTimeAnnotationKey is applied to a Deployment's pod template
	// spec when the pod needs to restart itself, ex. the capabilities change.
	// The application of this annotation causes the Deployment to do a rollout
	// of new pods, ensuring at least one pod is online at all times.
	// The value is an RFC3339Nano formatted timestamp.
	LastRestartTimeAnnotationKey = "vmoperator.vmware.com/last-restart-time"

	// LastRestartReasonAnnotationKey is applied to a Deployment's pod template
	// spec when the pod needs to restart itself, ex. the capabilities change.
	// The application of this annotation causes the Deployment to do a rollout
	// of new pods, ensuring at least one pod is online at all times.
	// The value is the reason for the restart.
	LastRestartReasonAnnotationKey = "vmoperator.vmware.com/last-restart-reason"

	// VCCredsSecretName is the name of the secret in the pod namespace
	// that contains the VC credentials.
	VCCredsSecretName = "wcp-vmop-sa-vc-auth" //nolint:gosec

	// ClusterModuleNameAnnotationKey is the annotation key for cluster module group name for
	// the VM. The VM must have a VirtualMachineSetResourcePolicy assigned.
	ClusterModuleNameAnnotationKey string = "vsphere-cluster-module-group"

	// ClusterModuleUUIDAnnotationKey is the annotation key for cluster module UUID for
	// the VM. The VM must have a VirtualMachineSetResourcePolicy assigned.
	ClusterModuleUUIDAnnotationKey string = "vsphere-cluster-module-group-uuid"

	// SkipValidationAnnotationKey is a privileged annotation that may be used
	// to skip the validation webhooks for a given object.
	SkipValidationAnnotationKey string = "vmoperator.vmware.com.protected/skip-validation"

	// BootstrapHashConfigSpecAnnotationKey is the annotation used to track the
	// config spec used to bootstrap a VM's guest information.
	BootstrapHashConfigSpecAnnotationKey = "vmoperator.vmware.com/bootstrap-hash-configspec"

	// BootstrapHashCustomSpecAnnotationKey is the annotation used to track the
	// customization spec used to bootstrap a VM's guest information.
	BootstrapHashCustomSpecAnnotationKey = "vmoperator.vmware.com/bootstrap-hash-customspec"

	// SkipDeletePlatformResourceKey is a privileged annotation that may be used
	// to skip the deletion of a Kubernetes object's underlying platform
	// resource. For example, when applied to a VM, deleting the VirtualMachine
	// object in Kubernetes will not result in the deletion of the vSphere VM.
	SkipDeletePlatformResourceKey string = "vmoperator.vmware.com.protected/skip-delete-platform-resource"

	// LastUpdatedPowerStateTimeAnnotation is the annotation key for the last
	// updated time of the power state.
	LastUpdatedPowerStateTimeAnnotation = "vmoperator.vmware.com.protected/last-updated-power-state-time"

	// ApplyPowerStateTimeAnnotation is the annotation key for the time to apply
	// a power state change to a VirtualMachine or a VirtualMachineGroup object
	// scheduled from its parent group.
	ApplyPowerStateTimeAnnotation = "vmoperator.vmware.com.protected/apply-power-state-time"

	// VirtualMachineClassHashAnnotationKey is the annotation key for the VM Class hash
	// used to generate VirtualMachineClassInstances.
	VirtualMachineClassHashAnnotationKey = "vmoperator.vmware.com/vmclass-hash"

	// VirtualMachineSnapshotRevertInProgressAnnotationKey is the
	// annotation key to indicate that a VM snapshot revert is in
	// progress.
	VirtualMachineSnapshotRevertInProgressAnnotationKey = "vmoperator.vmware.com/snapshot-revert-in-progress"
)
