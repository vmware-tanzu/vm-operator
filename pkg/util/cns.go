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
// CnsNodeVMBatchAttachment based on the VM name.
func CNSBatchAttachmentNameForVM(vmName string) string {
	return vmName
}

// SanitizeCNSErrorMessage checks if error message contains opId, if yes,
// Only extract the prefix before first ":" and return it.
// The CSI controller sometimes puts the serialized SOAP error into the
// CnsNodeVmAttachment/CnsNodeVMBatchAttachment error field, which contains
// things like OpIds and pointers that change on every failed reconcile attempt.
// Using this error as-is causes VM object churn, so try to avoid that here.
// The full error message is always available
// in the CnsNodeVmAttachment/CnsNodeVMBatchAttachment.
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

// GetCnsDiskModeFromDiskMode maps a volume disk mode to CNS disk mode.
// Returns the converted disk mode or an an error if not supported.
func GetCnsDiskModeFromDiskMode(diskMode vmopv1.VolumeDiskMode) (cnsv1alpha1.DiskMode, error) {
	switch diskMode {
	case vmopv1.VolumeDiskModePersistent:
		return cnsv1alpha1.Persistent, nil
	case vmopv1.VolumeDiskModeIndependentPersistent:
		return cnsv1alpha1.IndependentPersistent, nil
	case vmopv1.VolumeDiskModeIndependentNonPersistent:
		return cnsv1alpha1.IndependentNonPersistent, nil
	case vmopv1.VolumeDiskModeNonPersistent:
		return cnsv1alpha1.NonPersistent, nil
	default:
		return "", fmt.Errorf("unsupported disk mode: %s", diskMode)
	}
}

// GetCnsSharingModeFromSharingMode maps a volume sharing mode to CNS sharing mode.
// Returns the converted sharing mode or an an error if not supported.
func GetCnsSharingModeFromSharingMode(sharingMode vmopv1.VolumeSharingMode) (cnsv1alpha1.SharingMode, error) {
	switch sharingMode {
	case vmopv1.VolumeSharingModeNone:
		return cnsv1alpha1.SharingNone, nil
	case vmopv1.VolumeSharingModeMultiWriter:
		return cnsv1alpha1.SharingMultiWriter, nil
	default:
		return "", fmt.Errorf("unsupported sharing mode: %s", sharingMode)
	}
}
