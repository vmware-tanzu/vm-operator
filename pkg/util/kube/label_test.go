// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

var _ = Describe("Label", func() {

	Context("HasCAPILabels", func() {

		It("should return true if the VM has a CAPW label", func() {
			vmLabels := map[string]string{
				kubeutil.CAPWClusterRoleLabelKey: "",
			}
			Expect(kubeutil.HasCAPILabels(vmLabels)).To(BeTrue())
		})

		It("should return true if the VM has a CAPV label", func() {
			vmLabels := map[string]string{
				kubeutil.CAPVClusterRoleLabelKey: "",
			}
			Expect(kubeutil.HasCAPILabels(vmLabels)).To(BeTrue())
		})

		It("should return false if the VM has no Cluster API related labels", func() {
			vmLabels := map[string]string{}
			Expect(kubeutil.HasCAPILabels(vmLabels)).To(BeFalse())
		})
	})

	Context("HasZoneLabel", func() {

		It("should return true if the VM has a zone label", func() {
			vmLabels := map[string]string{
				corev1.LabelTopologyZone: "us-west-1a",
			}
			Expect(kubeutil.HasZoneLabel(vmLabels)).To(BeTrue())
		})

		It("should return false if the VM has a zone label with empty value", func() {
			vmLabels := map[string]string{
				corev1.LabelTopologyZone: "",
			}
			Expect(kubeutil.HasZoneLabel(vmLabels)).To(BeFalse())
		})

		It("should return true if the VM has a zone label along with other labels", func() {
			vmLabels := map[string]string{
				"app":                    "my-app",
				corev1.LabelTopologyZone: "us-west-1a",
				"version":                "1.0",
			}
			Expect(kubeutil.HasZoneLabel(vmLabels)).To(BeTrue())
		})

		It("should return false if the VM has no zone label", func() {
			vmLabels := map[string]string{
				"app":     "my-app",
				"version": "1.0",
			}
			Expect(kubeutil.HasZoneLabel(vmLabels)).To(BeFalse())
		})

		It("should return false if the VM has an empty labels map", func() {
			vmLabels := map[string]string{}
			Expect(kubeutil.HasZoneLabel(vmLabels)).To(BeFalse())
		})

		It("should return false if the VM labels map is nil", func() {
			Expect(kubeutil.HasZoneLabel(nil)).To(BeFalse())
		})
	})

	Context("HasVMOperatorLabels", func() {
		It("should return true if the labels map has a VM Operator managed label", func() {
			labels := map[string]string{
				"vmoperator.vmware.com/paused": "",
			}
			Expect(kubeutil.HasVMOperatorLabels(labels)).To(BeTrue())

			labels = map[string]string{
				"vmicache.vmoperator.vmware.com/paused": "",
			}
			Expect(kubeutil.HasVMOperatorLabels(labels)).To(BeTrue())
		})

		It("should return false if the labels map has no VM Operator managed labels", func() {
			labels := map[string]string{}
			Expect(kubeutil.HasVMOperatorLabels(labels)).To(BeFalse())

			labels["foo"] = "bar"
			Expect(kubeutil.HasVMOperatorLabels(labels)).To(BeFalse())
		})
	})

	Context("RemoveVMOperatorLabels", func() {
		It("should remove all VM Operator managed labels", func() {
			labels := map[string]string{
				"vmoperator.vmware.com/paused":          "",
				"vmicache.vmoperator.vmware.com/paused": "",
			}
			Expect(kubeutil.RemoveVMOperatorLabels(labels)).To(BeEmpty())
		})

		It("should keep non-VM Operator managed labels", func() {
			labels := map[string]string{
				"foo": "bar",
			}
			Expect(kubeutil.RemoveVMOperatorLabels(labels)).To(Equal(labels))
		})
	})
})
