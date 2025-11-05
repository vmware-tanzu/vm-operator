// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"fmt"
	"strings"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
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

// GetCnsNodeVMAttachmentsForVM returns the existing CnsNodeVmAttachments for
// the specified VM by filtering attachments that match the VM's BiosUUID.
//
// The function queries CnsNodeVmAttachment objects in the VM's namespace
// using the VM's BiosUUID (spec.nodeuuid field in the attachment) to find
// all attachments belonging to this VM. This includes both active and
// orphaned attachments.
//
// Returns a map where the key is the attachment name and the value is the
// CnsNodeVmAttachment object.
func GetCnsNodeVMAttachmentsForVM(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
) (map[string]cnsv1alpha1.CnsNodeVmAttachment, error) {

	if vm.Status.BiosUUID == "" {
		return nil, nil
	}

	list := &cnsv1alpha1.CnsNodeVmAttachmentList{}
	err := k8sClient.List(
		ctx,
		list,
		ctrlclient.InNamespace(vm.Namespace),
		ctrlclient.MatchingFields{"spec.nodeuuid": vm.Status.BiosUUID})
	if err != nil {
		return nil, fmt.Errorf("failed to list CnsNodeVmAttachments: %w", err)
	}

	attachments := make(
		map[string]cnsv1alpha1.CnsNodeVmAttachment,
		len(list.Items))
	for _, attachment := range list.Items {
		attachments[attachment.Name] = attachment
	}

	return attachments, nil
}
