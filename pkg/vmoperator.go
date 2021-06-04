// Copyright (c) 2018-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package pkg

const (
	// Base FQDN for the vmoperator
	VmOperatorKey string = "vmoperator.vmware.com"

	// Annotation Key for VM provider
	VmOperatorVmProviderKey string = "vmoperator.vmware.com/vmprovider"

	// Annotation key for tag information at vmoperator
	ProviderTagsAnnotationKey string = "vsphere-tag"

	// Annotation key for clusterModule group name information at vmoperator
	ClusterModuleNameKey string = "vsphere-cluster-module-group"
)
