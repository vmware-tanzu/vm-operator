// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
