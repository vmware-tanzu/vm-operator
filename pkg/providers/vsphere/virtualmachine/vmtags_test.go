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
	It("is deterministic for the same pair", func() {
		Expect(virtualmachine.TagResourceName("app", "nginx")).To(
			Equal(virtualmachine.TagResourceName("app", "nginx")))
	})

	It("differs for different pairs", func() {
		Expect(virtualmachine.TagResourceName("app", "nginx")).ToNot(
			Equal(virtualmachine.TagResourceName("app", "apache")))
		Expect(virtualmachine.TagResourceName("app", "nginx")).ToNot(
			Equal(virtualmachine.TagResourceName("env", "nginx")))
	})

	It("is DNS-subdomain safe: \"tag-\" followed by 17 hex characters", func() {
		Expect(virtualmachine.TagResourceName("app", "nginx")).To(
			MatchRegexp(`^tag-[0-9a-f]{17}$`))
	})

	It("handles an empty value", func() {
		name := virtualmachine.TagResourceName("app", "")
		Expect(name).To(MatchRegexp(`^tag-[0-9a-f]{17}$`))
		Expect(name).ToNot(Equal(virtualmachine.TagResourceName("app", "x")))
	})

	It("handles a prefixed key", func() {
		Expect(virtualmachine.TagResourceName("example.com/app", "nginx")).To(
			MatchRegexp(`^tag-[0-9a-f]{17}$`))
	})

	It("distinguishes an empty value from every other value for the same key", func() {
		Expect(virtualmachine.TagResourceName("app", "")).ToNot(
			Equal(virtualmachine.TagResourceName("app", "nginx")))
	})
})

var _ = Describe("VCenterTagName", func() {
	It("joins the key and value with a colon", func() {
		Expect(virtualmachine.VCenterTagName("app", "nginx")).To(Equal("app:nginx"))
	})

	It("handles an empty value", func() {
		Expect(virtualmachine.VCenterTagName("app", "")).To(Equal("app:"))
	})

	It("handles a prefixed key", func() {
		Expect(virtualmachine.VCenterTagName("example.com/app", "nginx")).To(
			Equal("example.com/app:nginx"))
	})
})
