// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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
