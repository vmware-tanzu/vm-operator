// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package messages

const (
	UpdatingImmutableFieldsNotAllowed         = "updates to immutable fields are not allowed: %s"
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
	MetadataTransportNotSupported             = "spec.vmmetadata.transport is not supported"
	MetadataTransportConfigMapNotSpecified    = "spec.vmmetadata.configmapname must be specified"
	MultipleVolumeSpecifiedFmt                = "only one of spec.volumes[%d].persistentVolumeClaim/spec.volumes[%d].vsphereVolume must be specified"
	VolumeNotSpecifiedFmt                     = "one of spec.volumes[%d].persistentVolumeClaim/spec.volumes[%d].vsphereVolume must be specified"
	VsphereVolumeSizeNotMBMultipleFmt         = "spec.volumes[%d].vsphereVolume.capacity.ephemeral-storage must be a multiple of MB"
	EagerZeroedAndThinProvisionedNotSupported = "Volume provisioning cannot have EagerZeroed and ThinProvisioning set. Eager zeroing requires thick provisioning."
	GuestOSNotSupported                       = "GuestOS not supported for osType %s on image %s"
	StorageClassNotAssigned                   = "StorageClass %s is not assigned to any ResourceQuotas in namespace %s"
	NoResourceQuota                           = "no ResourceQuotas assigned to namespace %s"
)
