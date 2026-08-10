// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
)

var _ = Describe("CPUQuantityToMhz", func() {

	Context("Convert CPU units from milli-cores to MHz", func() {
		It("return whole number for non-integer CPU quantity", func() {
			q, err := resource.ParseQuantity("500m")
			Expect(err).NotTo(HaveOccurred())
			freq := virtualmachine.CPUQuantityToMhz(q, 3225)
			expectVal := int64(1613)
			Expect(freq).Should(BeNumerically("==", expectVal))
		})

		It("return whole number for integer CPU quantity", func() {
			q, err := resource.ParseQuantity("1000m")
			Expect(err).NotTo(HaveOccurred())
			freq := virtualmachine.CPUQuantityToMhz(q, 3225)
			expectVal := int64(3225)
			Expect(freq).Should(BeNumerically("==", expectVal))
		})
	})
})

var _ = Describe("MbToBytes", func() {

	Context("Convert vSphere MB (binary mebibytes) to bytes", func() {
		It("is the inverse of MemoryQuantityToMb", func() {
			q, err := resource.ParseQuantity("4Gi")
			Expect(err).NotTo(HaveOccurred())
			mb := virtualmachine.MemoryQuantityToMb(q)
			Expect(virtualmachine.MbToBytes(mb)).Should(BeNumerically("==", q.Value()))
		})

		It("returns the binary byte count, not decimal", func() {
			Expect(virtualmachine.MbToBytes(4096)).Should(BeNumerically("==", int64(4096*1024*1024)))
		})
	})
})
