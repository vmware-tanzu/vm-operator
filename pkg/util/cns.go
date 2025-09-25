// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"fmt"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
)

// CNSAttachmentNameForVolume returns the name of the CnsNodeVmAttachment based
// on the VM and Volume name.
// This matches the naming used in previous code but there are situations where
// we may get a collision between VMs and Volume names. I'm not sure if there is
// an absolute way to avoid that: the same situation can happen with the
// claimName.
// Ideally, we would use GenerateName, but we lack the back-linkage to match
// Volumes and CnsNodeVmAttachment up.
// The VM webhook validate that this result will be a valid k8s name.
func CNSAttachmentNameForVolume(vmName, volumeName string) string {
	return vmName + "-" + volumeName
}

// CNSBatchAttachmentNameForVolume returns the name of the
// CnsNodeVmBatchAttachment based on the VM name.
func CNSBatchAttachmentNameForVolume(vmName string) string {
	return vmName
}

// BuildControllerKey builds a controller key that CNS can understand
// Format: <type>:<busNumber> or just <type> if bus number not specified.
func BuildControllerKey(controllerType vmopv1.VirtualControllerType, busNumber *int32) string {
	if busNumber != nil {
		return fmt.Sprintf("%s:%d", controllerType, *busNumber)
	}

	return string(controllerType)
}
