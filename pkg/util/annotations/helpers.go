// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package annotations

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
)

func HasForceEnableBackup(o metav1.Object) bool {
	return hasAnnotation(o, vmopv1.ForceEnableBackupAnnotation)
}

func HasPaused(o metav1.Object) bool {
	return hasAnnotation(o, vmopv1.PauseAnnotation)
}

func hasAnnotation(o metav1.Object, annotation string) bool {
	annotations := o.GetAnnotations()
	if annotations == nil {
		return false
	}

	_, ok := annotations[annotation]
	return ok
}
