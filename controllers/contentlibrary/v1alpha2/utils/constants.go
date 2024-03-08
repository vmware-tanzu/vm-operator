// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package utils

const (
	ItemFieldNamePrefix           = "clitem"
	ImageFieldNamePrefix          = "vmi"
	ClusterContentLibraryKind     = "ClusterContentLibrary"
	ClusterContentLibraryItemKind = "ClusterContentLibraryItem"
	ContentLibraryKind            = "ContentLibrary"
	ContentLibraryItemKind        = "ContentLibraryItem"

	ContentLibraryItemVmopFinalizer        = "contentlibraryitem.vmoperator.vmware.com"
	ClusterContentLibraryItemVmopFinalizer = "clustercontentlibraryitem.vmoperator.vmware.com"

	// TKGServiceTypeLabelKeyPrefix is a label prefix used to identify
	// labels that contain information about the type of service provided
	// by a ClusterContentLibrary.
	// This label prefix is replaced by MultipleCLServiceTypeLabelKeyPrefix
	// when Features.TKGMultipleCL is enabled.
	TKGServiceTypeLabelKeyPrefix = "type.services.vmware.com/"
	// MultipleCLServiceTypeLabelKeyPrefix is a label prefix used to identify
	// labels that contain information about the type of service provided
	// by a ClusterContentLibrary.
	// This label prefix is used only when Features.TKGMultipleCL is enabled,
	// otherwise TKGServiceTypeLabelKeyPrefix is used.
	MultipleCLServiceTypeLabelKeyPrefix = "services.supervisor.vmware.com/"
)
