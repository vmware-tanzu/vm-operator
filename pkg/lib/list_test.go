/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package lib_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"vmware.com/kubevsphere/pkg/lib"
)

var _ = Describe("List Utilities", func() {

	Context("filter", func() {
		It("a nil list", func() {
			var list []string = nil
			Expect(lib.Filter(list, "")).Should(BeEmpty())
		})

		It("an empty list", func() {
			list := make([]string, 0)
			Expect(lib.Filter(list, "")).Should(BeEmpty())
		})

		It("one element list contains match", func() {
			list := []string{"one"}
			Expect(lib.Filter(list, "one")).Should(BeEmpty())
		})

		It("multiple element list contains match", func() {
			list := []string{"one", "two"}
			Expect(lib.Filter(list, "one")).Should(ConsistOf("two"))
		})

		It("multiple element list does not contain match", func() {
			list := []string{"one", "two"}
			Expect(lib.Filter(list, "three")).Should(ConsistOf("one", "two"))
		})
	})

	Context("contains", func() {
		It("a nil list", func() {
			var list []string = nil
			Expect(lib.Contains(list, "")).To(BeFalse())
		})

		It("an empty list", func() {
			list := make([]string, 0)
			Expect(lib.Contains(list, "")).To(BeFalse())
		})

		It("list contains match", func() {
			list := []string{"one", "two"}
			Expect(lib.Contains(list, "one")).Should(BeTrue())
		})

		It("list does not contain match", func() {
			list := []string{"one", "two"}
			Expect(lib.Contains(list, "three")).Should(BeFalse())
		})

		// Test table example:
		DescribeTable("contains (table)",
			func(list []string, strToSearch string, expected bool) {
				Expect(lib.Contains(list, strToSearch)).To(Equal(expected))
			},
			Entry("a nil list", nil, "", false),
			Entry("an empty list", make([]string, 0), "", false),
			Entry("list contains match", []string{"one", "two"}, "one", true),
			Entry("list does not contain match", []string{"one", "two"}, "three", false),
		)
	})

})
