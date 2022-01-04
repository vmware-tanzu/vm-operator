// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"k8s.io/apimachinery/pkg/api/resource"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/virtualmachine"
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
