/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

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
)

func AddAnnotations(objectMeta *metav1.ObjectMeta) {
	// Add vSphere provider annotations to the object meta
	annotations := objectMeta.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// TODO: Make this version dynamic
	annotations[VmOperatorVersionKey] = "v1"

	objectMeta.SetAnnotations(annotations)
}
