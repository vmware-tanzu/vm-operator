// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

var _ = DescribeTable("VimGuestOptionsName",
	func(in, out string) {
		Expect(util.VimGuestOptionsName(in)).To(Equal(out))
	},
	Entry("already DNS-safe", "otherlinux64guest", "otherlinux64guest"),
	Entry("mixed case", "otherLinux64Guest", "otherlinux64guest"),
	Entry("spaces and punctuation", "Windows 11 (64-bit)", "windows-11-64-bit"),
	Entry("leading and trailing invalid characters", "_Foo_", "foo"),
	Entry("empty string", "", ""),
	Entry("string longer than 63 characters", strings.Repeat("a", 70), strings.Repeat("a", 63)),
	Entry("truncation lands on a hyphen", strings.Repeat("a", 62)+"-bcdef", strings.Repeat("a", 62)),
)
