// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package messages

const (
	UpdatingImmutableFieldsNotAllowed        = "updates to immutable fields are not allowed"
	ImageNotSpecified                        = "spec.imageName must be specified"
	ClassNotSpecified                        = "spec.className must be specified"
	NetworkNameNotSpecifiedFmt               = "spec.networkInterfaces[%d].networkName must be specified"
	NetworkTypeNotSupportedFmt               = "spec.networkInterfaces[%d].networkType is not supported"
	VolumeNameNotSpecifiedFmt                = "spec.volumes[%d].name must be specified"
	PersistentVolumeClaimNotSpecifiedFmt     = "spec.volumes[%d].persistentVolumeClaim must be specified"
	PersistentVolumeClaimNameNotSpecifiedFmt = "spec.volumes[%d].persistentVolumeClaim.claimName must be specified"
	VolumeNameDuplicateFmt                   = "spec.volumes[%d].name must be unique"
	MetadataTransportNotSupported            = "spec.vmmetadata.transport is not supported"
	MetadataTransportConfigMapNotSpecified   = "spec.vmmetadata.configmapname must be specified"
)
