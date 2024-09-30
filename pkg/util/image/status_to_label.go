// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package image

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
)

// SyncStatusToLabels copies the image's type, capabilities and OS information
// from the image status to well-known labels on the image in order to make it
// easier for clients to search for images based on capabilities or OS details.
// If the image status does not have the info or is invalid, remove the label.
func SyncStatusToLabels(
	obj metav1.Object,
	status vmopv1.VirtualMachineImageStatus) {

	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
		obj.SetLabels(labels)
	}

	setLabel := func(k, v string) {
		if isValidLabelValue(v) {
			labels[k] = v
		} else {
			delete(labels, k)
		}
	}

	// Set image type label.
	setLabel(vmopv1.VirtualMachineImageTypeLabel, status.Type)

	// Set OS labels.
	setLabel(vmopv1.VirtualMachineImageOSIDLabel, status.OSInfo.ID)
	setLabel(vmopv1.VirtualMachineImageOSTypeLabel, status.OSInfo.Type)
	setLabel(vmopv1.VirtualMachineImageOSVersionLabel, status.OSInfo.Version)

	// Set capability labels.
	kp := vmopv1.VirtualMachineImageCapabilityLabel
	capKeys := map[string]struct{}{}
	for _, cap := range status.Capabilities {
		k := kp + cap
		if len(validation.IsQualifiedName(k)) == 0 {
			labels[k] = "true"
			capKeys[k] = struct{}{}
		} else {
			delete(labels, k)
		}
	}
	// Delete any stale capability labels.
	for k := range labels {
		if _, ok := capKeys[k]; !ok && strings.HasPrefix(k, kp) {
			delete(labels, k)
		}
	}
}

func isValidLabelValue(v string) bool {
	return v != "" && len(validation.IsValidLabelValue(v)) == 0
}
