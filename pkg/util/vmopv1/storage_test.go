// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

var _ = Describe("CnsNodeVMBatchAttachmentReportsCacheMiss", func() {
	const csiErrMsg = "failed to find volumeID for PVC pvc-0"

	It("should return false for an attachment with no conditions", func() {
		ba := &cnsv1alpha1.CnsNodeVMBatchAttachment{}
		Expect(vmopv1util.CnsNodeVMBatchAttachmentReportsCacheMiss(ba)).To(BeFalse())
	})

	It("should return false when no condition contains the cache miss substring", func() {
		ba := &cnsv1alpha1.CnsNodeVMBatchAttachment{}
		ba.Status.Conditions = []metav1.Condition{
			{Type: "Ready", Status: metav1.ConditionFalse, Reason: "AttachFailed", Message: "some other error"},
		}
		Expect(vmopv1util.CnsNodeVMBatchAttachmentReportsCacheMiss(ba)).To(BeFalse())
	})

	It("should return true when a top-level condition message contains the cache miss substring", func() {
		ba := &cnsv1alpha1.CnsNodeVMBatchAttachment{}
		ba.Status.Conditions = []metav1.Condition{
			{Type: "Ready", Status: metav1.ConditionFalse, Reason: "Failed", Message: csiErrMsg},
		}
		Expect(vmopv1util.CnsNodeVMBatchAttachmentReportsCacheMiss(ba)).To(BeTrue())
	})

	It("should return true when a top-level condition reason contains the cache miss substring", func() {
		ba := &cnsv1alpha1.CnsNodeVMBatchAttachment{}
		ba.Status.Conditions = []metav1.Condition{
			{Type: "Ready", Status: metav1.ConditionFalse, Reason: csiErrMsg, Message: ""},
		}
		Expect(vmopv1util.CnsNodeVMBatchAttachmentReportsCacheMiss(ba)).To(BeTrue())
	})

	It("should return true when a per-volume condition message contains the cache miss substring", func() {
		ba := &cnsv1alpha1.CnsNodeVMBatchAttachment{}
		ba.Status.VolumeStatus = []cnsv1alpha1.VolumeStatus{
			{
				Name: "vol-0",
				PersistentVolumeClaim: cnsv1alpha1.PersistentVolumeClaimStatus{
					ClaimName: "pvc-0",
					Conditions: []metav1.Condition{
						{Type: "VolumeAttached", Status: metav1.ConditionFalse, Reason: "AttachFailed", Message: csiErrMsg},
					},
				},
			},
		}
		Expect(vmopv1util.CnsNodeVMBatchAttachmentReportsCacheMiss(ba)).To(BeTrue())
	})

	It("should return false when per-volume conditions do not contain the cache miss substring", func() {
		ba := &cnsv1alpha1.CnsNodeVMBatchAttachment{}
		ba.Status.VolumeStatus = []cnsv1alpha1.VolumeStatus{
			{
				Name: "vol-0",
				PersistentVolumeClaim: cnsv1alpha1.PersistentVolumeClaimStatus{
					ClaimName: "pvc-0",
					Conditions: []metav1.Condition{
						{Type: "VolumeAttached", Status: metav1.ConditionFalse, Reason: "AttachFailed", Message: "disk is busy"},
					},
				},
			},
		}
		Expect(vmopv1util.CnsNodeVMBatchAttachmentReportsCacheMiss(ba)).To(BeFalse())
	})
})
