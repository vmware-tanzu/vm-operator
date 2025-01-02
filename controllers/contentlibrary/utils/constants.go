// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package utils

const (
	ItemFieldNamePrefix           = "clitem"
	ImageFieldNamePrefix          = "vmi"
	ClusterContentLibraryKind     = "ClusterContentLibrary"
	ClusterContentLibraryItemKind = "ClusterContentLibraryItem"
	ContentLibraryKind            = "ContentLibrary"
	ContentLibraryItemKind        = "ContentLibraryItem"

	CLItemFinalizer            = "vmoperator.vmware.com/contentlibraryitem"
	DeprecatedCLItemFinalizer  = "contentlibraryitem.vmoperator.vmware.com"
	CCLItemFinalizer           = "clustercontentlibraryitem.vmoperator.vmware.com"
	DeprecatedCCLItemFinalizer = "vmoperator.vmware.com/clustercontentlibraryitem"

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
