// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
)

// csiErrFindFailPrefix is the substring produced by the vsphere-csi-driver
// batch attachment helper when a PVC in the batch spec is not yet present in
// the CSI controller's volume ID cache.
const csiErrFindFailPrefix = "failed to find volumeID for PVC"

// CnsNodeVMBatchAttachmentReportsCacheMiss returns true when the provided
// CnsNodeVMBatchAttachment reports a PVC volume-ID cache miss in any of its
// status conditions, either at the top level or within per-volume status.
func CnsNodeVMBatchAttachmentReportsCacheMiss(
	ba *cnsv1alpha1.CnsNodeVMBatchAttachment,
) bool {
	if batchAttachConditionsIncludeCacheMissMsg(ba.Status.Conditions) {
		return true
	}
	for _, v := range ba.Status.VolumeStatus {
		if batchAttachConditionsIncludeCacheMissMsg(
			v.PersistentVolumeClaim.Conditions) {
			return true
		}
	}
	return false
}

// batchAttachConditionsIncludeCacheMissMsg reports whether any condition's
// message or reason contains the CSI batch volume-ID cache miss substring.
func batchAttachConditionsIncludeCacheMissMsg(
	conditions []metav1.Condition) bool {
	for _, c := range conditions {
		if strings.Contains(c.Message, csiErrFindFailPrefix) ||
			strings.Contains(c.Reason, csiErrFindFailPrefix) {
			return true
		}
	}
	return false
}

// IsSnapshotVolume reports whether the volume with the given name is a VirtualMachineSnapshot volume,
// either specified in vm.Spec.Volumes or present in vm.Status.Volumes.
func IsSnapshotVolume(vm *vmopv1.VirtualMachine, volName string) bool {
	if vm == nil {
		return false
	}
	for _, vol := range vm.Spec.Volumes {
		if vol.Name == volName && vol.VirtualMachineSnapshot != nil {
			return true
		}
	}
	// Also check if it's in the status but not in the spec (detaching)
	for _, vol := range vm.Status.Volumes {
		if vol.Name == volName && vol.Type == vmopv1.VolumeTypeClassic && vol.Attached {
			return true
		}
	}
	return false
}

// ShouldDeleteVolumeStatus reports whether the volume status entry should be removed
// during volume controller reconciliation of managed volumes.
// Classic volumes and VirtualMachineSnapshot volumes are preserved because they are
// managed by the virtualmachine controller, not CNS.
func ShouldDeleteVolumeStatus(vm *vmopv1.VirtualMachine, e vmopv1.VirtualMachineVolumeStatus) bool {
	if IsSnapshotVolume(vm, e.Name) {
		return false
	}
	if strings.HasSuffix(e.Name, ":detaching") {
		originalName := strings.TrimSuffix(e.Name, ":detaching")
		if IsSnapshotVolume(vm, originalName) {
			return false
		}
	}

	if e.Type == vmopv1.VolumeTypeClassic {
		return false
	}

	return true
}
