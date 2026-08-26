// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
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

var _ = Describe("IsSnapshotVolume", func() {
	It("should return false for nil VM", func() {
		Expect(vmopv1util.IsSnapshotVolume(nil, "snap-vol")).To(BeFalse())
	})

	It("should return true when volume has VirtualMachineSnapshot in spec", func() {
		vm := &vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Volumes: []vmopv1.VirtualMachineVolume{
					{
						Name: "snap-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
								Name: "my-snap",
							},
						},
					},
				},
			},
		}
		Expect(vmopv1util.IsSnapshotVolume(vm, "snap-vol")).To(BeTrue())
		Expect(vmopv1util.IsSnapshotVolume(vm, "other-vol")).To(BeFalse())
	})

	It("should return false when volume in spec is a PVC", func() {
		vm := &vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Volumes: []vmopv1.VirtualMachineVolume{
					{
						Name: "pvc-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "my-pvc",
								},
							},
						},
					},
				},
			},
		}
		Expect(vmopv1util.IsSnapshotVolume(vm, "pvc-vol")).To(BeFalse())
	})

	It("should return true when volume is in status as attached classic volume (detaching)", func() {
		vm := &vmopv1.VirtualMachine{
			Status: vmopv1.VirtualMachineStatus{
				Volumes: []vmopv1.VirtualMachineVolumeStatus{
					{
						Name:     "snap-vol",
						Type:     vmopv1.VolumeTypeClassic,
						Attached: true,
					},
				},
			},
		}
		Expect(vmopv1util.IsSnapshotVolume(vm, "snap-vol")).To(BeTrue())
	})
})

var _ = Describe("ShouldDeleteVolumeStatus", func() {
	It("should return false for snapshot volume in spec", func() {
		vm := &vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Volumes: []vmopv1.VirtualMachineVolume{
					{
						Name: "snap-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
								Name: "my-snap",
							},
						},
					},
				},
			},
		}
		volStatus := vmopv1.VirtualMachineVolumeStatus{
			Name: "snap-vol",
			Type: vmopv1.VolumeTypeManaged, // even if temporarily misclassified as Managed
		}
		Expect(vmopv1util.ShouldDeleteVolumeStatus(vm, volStatus)).To(BeFalse())
	})

	It("should return false for snapshot volume with detaching suffix", func() {
		vm := &vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Volumes: []vmopv1.VirtualMachineVolume{
					{
						Name: "snap-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
								Name: "my-snap",
							},
						},
					},
				},
			},
		}
		volStatus := vmopv1.VirtualMachineVolumeStatus{
			Name: "snap-vol:detaching",
			Type: vmopv1.VolumeTypeManaged,
		}
		Expect(vmopv1util.ShouldDeleteVolumeStatus(vm, volStatus)).To(BeFalse())
	})

	It("should return false for classic volume", func() {
		vm := &vmopv1.VirtualMachine{}
		volStatus := vmopv1.VirtualMachineVolumeStatus{
			Name: "classic-vol",
			Type: vmopv1.VolumeTypeClassic,
		}
		Expect(vmopv1util.ShouldDeleteVolumeStatus(vm, volStatus)).To(BeFalse())
	})

	It("should return true for managed PVC volume", func() {
		vm := &vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Volumes: []vmopv1.VirtualMachineVolume{
					{
						Name: "pvc-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "my-pvc",
								},
							},
						},
					},
				},
			},
		}
		volStatus := vmopv1.VirtualMachineVolumeStatus{
			Name: "pvc-vol",
			Type: vmopv1.VolumeTypeManaged,
		}
		Expect(vmopv1util.ShouldDeleteVolumeStatus(vm, volStatus)).To(BeTrue())
	})
})
