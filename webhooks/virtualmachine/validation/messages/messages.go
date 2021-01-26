// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package messages

const (
	UpdatingImmutableFieldsNotAllowed         = "updates to immutable fields are not allowed: %s"
	UpdatingFieldsNotAllowedInPowerState      = "updates to fields %s are not allowed in the '%s' power state"
	ImageNotSpecified                         = "spec.imageName must be specified"
	ClassNotSpecified                         = "spec.className must be specified"
	NetworkNameNotSpecifiedFmt                = "spec.networkInterfaces[%d].networkName must be specified"
	NetworkTypeNotSupportedFmt                = "spec.networkInterfaces[%d].networkType is not supported"
	MultipleNetworkInterfacesNotSupportedFmt  = "Multiple network interfaces cannot be connected to the same network. Duplicate NetworkName: spec.networkInterfaces[%d].networkName"
	VolumeNameNotSpecifiedFmt                 = "spec.volumes[%d].name must be specified"
	VolumeNameNotValidObjectNameFmt           = "spec.volumes[%d].name prepended with the VirtualMachine.Name must be valid object name: %s"
	PersistentVolumeClaimNameNotSpecifiedFmt  = "spec.volumes[%d].persistentVolumeClaim.claimName must be specified"
	PersistentVolumeClaimNameReadOnlyFmt      = "spec.volumes[%d].persistentVolumeClaim.readOnly must be false"
	VolumeNameDuplicateFmt                    = "spec.volumes[%d].name must be unique"
	MetadataTransportConfigMapNotSpecified    = "spec.vmMetadata.configMapName must be specified"
	MultipleVolumeSpecifiedFmt                = "only one of spec.volumes[%d].persistentVolumeClaim/spec.volumes[%d].vsphereVolume must be specified"
	VolumeNotSpecifiedFmt                     = "one of spec.volumes[%d].persistentVolumeClaim/spec.volumes[%d].vsphereVolume must be specified"
	VsphereVolumeSizeNotMBMultipleFmt         = "spec.volumes[%d].vsphereVolume.capacity.ephemeral-storage must be a multiple of MB"
	EagerZeroedAndThinProvisionedNotSupported = "Volume provisioning cannot have EagerZeroed and ThinProvisioning set. Eager zeroing requires thick provisioning."
	VMImageNotSupported                       = "GuestOS not supported for osType %s on image %s or VMImage is not compatible with v1alpha1 or is not a TKG Image"
	StorageClassNotAssigned                   = "StorageClass %s is not assigned to any ResourceQuotas in namespace %s"
	NoResourceQuota                           = "no ResourceQuotas assigned to namespace %s"
)
