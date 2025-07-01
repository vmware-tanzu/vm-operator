// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"bytes"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

var _ = Describe("IsNil", func() {

	DescribeTable("value type is not concrete",
		func(f func() any, expected bool) {
			Ω(util.IsNil(f())).Should(Equal(expected))
		},
		Entry("nil", func() any { return nil }, true),
		Entry("a string", func() any { return "hello" }, false),
		Entry("a uint8", func() any { return uint8(1) }, false),
		Entry("a nil *bool", func() any { return (*bool)(nil) }, true),

		Entry("a nil *bytes.Buffer", func() any { return (*bytes.Buffer)(nil) }, true),
		Entry("a nil io.Reader", func() any { return (io.Reader)(nil) }, true),
	)

	When("value type is concrete", func() {
		When("value type is *bytes.Buffer", func() {
			When("nil", func() {
				It("should return false", func() {
					var v *bytes.Buffer = nil
					Ω(util.IsNil(v)).Should(BeTrue())
				})
			})
			When("not-nil", func() {
				It("should return false", func() {
					v := &bytes.Buffer{}
					Ω(util.IsNil(v)).Should(BeFalse())
				})
			})
		})
		When("value type is io.Reader", func() {
			When("nil", func() {
				It("should return false", func() {
					var v io.Reader = nil
					Ω(util.IsNil(v)).Should(BeTrue())
				})
			})
			When("not-nil", func() {
				It("should return false", func() {
					var v io.Reader = &bytes.Buffer{}
					Ω(util.IsNil(v)).Should(BeFalse())
				})
			})
		})
	})
})
