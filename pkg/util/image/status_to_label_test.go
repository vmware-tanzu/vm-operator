// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package image_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	imgutil "github.com/vmware-tanzu/vm-operator/pkg/util/image"
)

var _ = Describe("SyncStatusToLabels", func() {
	var (
		obj    metav1.Object
		status vmopv1.VirtualMachineImageStatus
	)

	BeforeEach(func() {
		vmi := &vmopv1.VirtualMachineImage{}
		obj = vmi
		status = vmi.Status
	})

	JustBeforeEach(func() {
		imgutil.SyncStatusToLabels(obj, status)
	})

	When("the status does not have any info", func() {
		It("should not set any labels", func() {
			Expect(obj.GetLabels()).To(BeNil())
		})
	})

	When("the status has the OS ID", func() {
		BeforeEach(func() {
			status.OSInfo.ID = "photon64"
		})
		It("should have the OS ID label", func() {
			Expect(obj.GetLabels()).To(HaveLen(1))
			Expect(obj.GetLabels()).To(HaveKeyWithValue(
				vmopv1.VirtualMachineImageOSIDLabel,
				status.OSInfo.ID))
		})
	})

	When("the status has the OS type", func() {
		BeforeEach(func() {
			status.OSInfo.Type = "linux"
		})
		It("should have the OS ID label", func() {
			Expect(obj.GetLabels()).To(HaveLen(1))
			Expect(obj.GetLabels()).To(HaveKeyWithValue(
				vmopv1.VirtualMachineImageOSTypeLabel,
				status.OSInfo.Type))
		})
	})

	When("the status has the OS version", func() {
		BeforeEach(func() {
			status.OSInfo.Version = "5"
		})
		It("should have the OS ID label", func() {
			Expect(obj.GetLabels()).To(HaveLen(1))
			Expect(obj.GetLabels()).To(HaveKeyWithValue(
				vmopv1.VirtualMachineImageOSVersionLabel,
				status.OSInfo.Version))
		})
	})

	When("the status has two capabilities", func() {
		BeforeEach(func() {
			status.Capabilities = []string{"cloud-init", "nvidia-vgpu"}
		})
		It("should have the capability labels", func() {
			Expect(obj.GetLabels()).To(HaveLen(2))
			Expect(obj.GetLabels()).To(HaveKeyWithValue(
				vmopv1.VirtualMachineImageCapabilityLabel+"cloud-init",
				"true"))
			Expect(obj.GetLabels()).To(HaveKeyWithValue(
				vmopv1.VirtualMachineImageCapabilityLabel+"nvidia-vgpu",
				"true"))
		})
	})

	When("the status has OS info and capabilities", func() {
		BeforeEach(func() {
			status.OSInfo.ID = "photon64"
			status.OSInfo.Type = "linux"
			status.OSInfo.Version = "5"
			status.Capabilities = []string{"cloud-init", "nvidia-vgpu"}
		})
		It("should have the capability labels", func() {
			Expect(obj.GetLabels()).To(HaveLen(5))
			Expect(obj.GetLabels()).To(HaveKeyWithValue(
				vmopv1.VirtualMachineImageOSIDLabel,
				status.OSInfo.ID))
			Expect(obj.GetLabels()).To(HaveKeyWithValue(
				vmopv1.VirtualMachineImageOSTypeLabel,
				status.OSInfo.Type))
			Expect(obj.GetLabels()).To(HaveKeyWithValue(
				vmopv1.VirtualMachineImageOSVersionLabel,
				status.OSInfo.Version))
			Expect(obj.GetLabels()).To(HaveKeyWithValue(
				vmopv1.VirtualMachineImageCapabilityLabel+"cloud-init",
				"true"))
			Expect(obj.GetLabels()).To(HaveKeyWithValue(
				vmopv1.VirtualMachineImageCapabilityLabel+"nvidia-vgpu",
				"true"))
		})

		When("the object already has labels", func() {
			BeforeEach(func() {
				obj.SetLabels(map[string]string{"hello": "world", "fu": "bar"})
			})
			It("should not remove the existing labels", func() {
				Expect(obj.GetLabels()).To(HaveLen(7))
				Expect(obj.GetLabels()).To(HaveKeyWithValue("hello", "world"))
				Expect(obj.GetLabels()).To(HaveKeyWithValue("fu", "bar"))
				Expect(obj.GetLabels()).To(HaveKeyWithValue(
					vmopv1.VirtualMachineImageOSIDLabel,
					status.OSInfo.ID))
				Expect(obj.GetLabels()).To(HaveKeyWithValue(
					vmopv1.VirtualMachineImageOSTypeLabel,
					status.OSInfo.Type))
				Expect(obj.GetLabels()).To(HaveKeyWithValue(
					vmopv1.VirtualMachineImageOSVersionLabel,
					status.OSInfo.Version))
				Expect(obj.GetLabels()).To(HaveKeyWithValue(
					vmopv1.VirtualMachineImageCapabilityLabel+"cloud-init",
					"true"))
				Expect(obj.GetLabels()).To(HaveKeyWithValue(
					vmopv1.VirtualMachineImageCapabilityLabel+"nvidia-vgpu",
					"true"))
			})
		})
	})

})
