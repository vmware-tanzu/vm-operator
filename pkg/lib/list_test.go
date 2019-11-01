// *******************************************************************************
// Copyright 2018 - 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
// *******************************************************************************

package lib_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
)

var _ = Describe("List Utilities", func() {

	Context("remove string", func() {
		It("a nil list", func() {
			var list []string = nil
			Expect(lib.RemoveString(list, "")).Should(BeEmpty())
		})

		It("an empty list", func() {
			list := make([]string, 0)
			Expect(lib.RemoveString(list, "")).Should(BeEmpty())
		})

		It("one element list contains match", func() {
			list := []string{"one"}
			Expect(lib.RemoveString(list, "one")).Should(BeEmpty())
		})

		It("multiple element list contains match", func() {
			list := []string{"one", "two"}
			Expect(lib.RemoveString(list, "one")).Should(ConsistOf("two"))
		})

		It("multiple element list does not contain match", func() {
			list := []string{"one", "two"}
			Expect(lib.RemoveString(list, "three")).Should(ConsistOf("one", "two"))
		})
	})

	Context("contains string", func() {
		It("a nil list", func() {
			var list []string = nil
			Expect(lib.ContainsString(list, "")).To(BeFalse())
		})

		It("an empty list", func() {
			list := make([]string, 0)
			Expect(lib.ContainsString(list, "")).To(BeFalse())
		})

		It("list contains match", func() {
			list := []string{"one", "two"}
			Expect(lib.ContainsString(list, "one")).Should(BeTrue())
		})

		It("list does not contain match", func() {
			list := []string{"one", "two"}
			Expect(lib.ContainsString(list, "three")).Should(BeFalse())
		})

		// Test table example:
		DescribeTable("contains (table)",
			func(list []string, strToSearch string, expected bool) {
				Expect(lib.ContainsString(list, strToSearch)).To(Equal(expected))
			},
			Entry("a nil list", nil, "", false),
			Entry("an empty list", make([]string, 0), "", false),
			Entry("list contains match", []string{"one", "two"}, "one", true),
			Entry("list does not contain match", []string{"one", "two"}, "three", false),
		)
	})

})
