// Copyright (c) 2018-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package pkg

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// Base FQDN for the vmoperator
	VmOperatorKey string = "vmoperator.vmware.com"

	// VM Operator version key
	VmOperatorVersionKey string = "vmoperator.vmware.com/version"

	// Annotation Key for VM provider
	VmOperatorVmProviderKey string = "vmoperator.vmware.com/vmprovider"

	// Annotation key for tag information at vmoperator
	ProviderTagsAnnotationKey string = "vsphere-tag"

	// Annotation key for clusterModule group name information at vmoperator
	ClusterModuleNameKey string = "vsphere-cluster-module-group"
)

func AddAnnotations(objectMeta *metav1.ObjectMeta) {
	annotations := objectMeta.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// TODO: Make this version dynamic
	annotations[VmOperatorVersionKey] = "v1"

	objectMeta.SetAnnotations(annotations)
}
