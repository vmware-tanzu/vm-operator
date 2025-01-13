// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

var _ = DescribeTable("SHA1Sum17",
	func(in, out string) {
		Expect(util.SHA1Sum17(in)).To(Equal(out))
	},
	Entry("empty string", "", "da39a3ee5e6b4b0d3"),
	Entry("a", "a", "86f7e437faa5a7fce"),
	Entry("b", "b", "e9d71f5ee7c92d6dc"),
	Entry("c", "c", "84a516841ba77a5b4"),
	Entry("a/b/c", "a/b/c", "ee2be6da91ae87947"),
)

var _ = DescribeTable("VMIName",
	func(in, out string) {
		Expect(util.VMIName(in)).To(Equal(out))
	},
	Entry("empty string", "", "vmi-da39a3ee5e6b4b0d3"),
	Entry("a", "a", "vmi-86f7e437faa5a7fce"),
	Entry("-b", "-b", "vmi-e9d71f5ee7c92d6dc"),
	Entry("c-", "c-", "vmi-84a516841ba77a5b4"),
	Entry("a/--b/-c", "a/--b-/c", "vmi-ee2be6da91ae87947"),
)
