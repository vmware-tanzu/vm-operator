// Copyright (c) 2018-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package pkg

const (
	// VMOperatorKey is the base FQDN for VM operator.
	VMOperatorKey string = "vmoperator.vmware.com"

	// VMOperatorVMProviderKey is the annotation Key for VM provider.
	VMOperatorVMProviderKey string = "vmoperator.vmware.com/vmprovider"

	// ProviderTagsAnnotationKey is the annotation key for tag information at VM operator.
	ProviderTagsAnnotationKey string = "vsphere-tag"

	// ClusterModuleNameKey is the annotation key for clusterModule group name information at VM operator.
	ClusterModuleNameKey string = "vsphere-cluster-module-group"
)
