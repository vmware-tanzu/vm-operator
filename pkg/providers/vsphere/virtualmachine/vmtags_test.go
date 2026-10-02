// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
)

var _ = Describe("TagResourceName", func() {
	It("is deterministic for the same triple", func() {
		Expect(virtualmachine.TagResourceName("ns", "app", "nginx")).To(
			Equal(virtualmachine.TagResourceName("ns", "app", "nginx")))
	})

	It("differs for different pairs", func() {
		Expect(virtualmachine.TagResourceName("ns", "app", "nginx")).ToNot(
			Equal(virtualmachine.TagResourceName("ns", "app", "apache")))
		Expect(virtualmachine.TagResourceName("ns", "app", "nginx")).ToNot(
			Equal(virtualmachine.TagResourceName("ns", "env", "nginx")))
	})

	It("differs for different namespaces with the same key/value pair", func() {
		Expect(virtualmachine.TagResourceName("ns-a", "app", "nginx")).ToNot(
			Equal(virtualmachine.TagResourceName("ns-b", "app", "nginx")))
	})

	It("is DNS-subdomain safe: \"tag-\" followed by 16 hex characters", func() {
		Expect(virtualmachine.TagResourceName("ns", "app", "nginx")).To(
			MatchRegexp(`^tag-[0-9a-f]{16}$`))
	})

	It("handles an empty value", func() {
		name := virtualmachine.TagResourceName("ns", "app", "")
		Expect(name).To(MatchRegexp(`^tag-[0-9a-f]{16}$`))
		Expect(name).ToNot(Equal(virtualmachine.TagResourceName("ns", "app", "x")))
	})

	It("handles a prefixed key", func() {
		Expect(virtualmachine.TagResourceName("ns", "example.com/app", "nginx")).To(
			MatchRegexp(`^tag-[0-9a-f]{16}$`))
	})

	It("distinguishes an empty value from every other value for the same key", func() {
		Expect(virtualmachine.TagResourceName("ns", "app", "")).ToNot(
			Equal(virtualmachine.TagResourceName("ns", "app", "nginx")))
	})
})

var _ = Describe("TagName", func() {
	It("joins the key and value with a colon", func() {
		Expect(virtualmachine.TagName("app", "nginx")).To(Equal("app:nginx"))
	})

	It("handles an empty value", func() {
		Expect(virtualmachine.TagName("app", "")).To(Equal("app:"))
	})

	It("handles a prefixed key", func() {
		Expect(virtualmachine.TagName("example.com/app", "nginx")).To(
			Equal("example.com/app:nginx"))
	})
})
