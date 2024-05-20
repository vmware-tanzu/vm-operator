// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package image

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
)

// SyncStatusToLabels copies the image's capabilities and OS information from
// the image status to well-known labels on the image in order to make it
// easier for clients to search for images based on capabilities or OS details.
func SyncStatusToLabels(
	obj metav1.Object,
	status vmopv1.VirtualMachineImageStatus) {

	var (
		osID      = status.OSInfo.ID
		osType    = status.OSInfo.Type
		osVersion = status.OSInfo.Version
		caps      = status.Capabilities
	)

	if osID == "" && osType == "" && osVersion == "" && len(caps) == 0 {
		return
	}

	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}

	if k, v := vmopv1.VirtualMachineImageOSIDLabel, osID; isValidLabelValue(v) {
		labels[k] = v
	}
	if k, v := vmopv1.VirtualMachineImageOSTypeLabel, osType; isValidLabelValue(v) {
		labels[k] = v
	}
	if k, v := vmopv1.VirtualMachineImageOSVersionLabel, osVersion; isValidLabelValue(v) {
		labels[k] = v
	}
	if kp, v := vmopv1.VirtualMachineImageCapabilityLabel, caps; len(v) > 0 {
		for i := range v {
			k := kp + v[i]
			if len(validation.IsQualifiedName(k)) == 0 {
				labels[k] = "true"
			}
		}
	}

	obj.SetLabels(labels)
}

func isValidLabelValue(v string) bool {
	return v != "" && len(validation.IsValidLabelValue(v)) == 0
}
