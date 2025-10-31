// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"strings"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
)

const (
	// IDEControllerMaxSlotCount is the maximum number of slots
	// per IDE controller.
	IDEControllerMaxSlotCount = int32(2)
	// IDEControllerMaxCount is the maximum number of IDE controllers.
	IDEControllerMaxCount = int32(2)
	// SATAControllerMaxSlotCount is the maximum number of slots
	// per SATA controller.
	SATAControllerMaxSlotCount = int32(30)
	// SATAControllerMaxCount is the maximum number of SATA controllers.
	SATAControllerMaxCount = int32(4)
)

// ControllerID represents a unique identifier for a virtual controller in a VM.
// It combines the controller type (IDE, NVME, SCSI, or SATA) with a bus number
// to uniquely identify a specific controller instance within the virtual machine.
type ControllerID struct {
	ControllerType vmopv1.VirtualControllerType
	BusNumber      int32
}

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

// CNSBatchAttachmentNameForVM returns the name of the
// CnsNodeVmBatchAttachment based on the VM name.
func CNSBatchAttachmentNameForVM(vmName string) string {
	return vmName
}

// SanitizeCNSErrorMessage checks if error message contains opId, if yes,
// Only extract the prefix before first ":" and return it.
// The CSI controller sometimes puts the serialized SOAP error into the
// CnsNodeVmAttachment/CnsNodeVmBatchAttachment error field, which contains
// things like OpIds and pointers that change on every failed reconcile attempt.
// Using this error as-is causes VM object churn, so try to avoid that here.
// The full error message is always available
// in the CnsNodeVmAttachment/CnsNodeVmBatchAttachment.
//
// Issue tracked in CSI repo for them to sanitize the error so we could get
// rid of this func:
// https://github.com/kubernetes-sigs/vsphere-csi-driver/issues/3663.
func SanitizeCNSErrorMessage(msg string) string {
	if strings.Contains(msg, "opId:") {
		idx := strings.Index(msg, ":")
		return msg[:idx]
	}

	return msg
}
