// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

var _ = DescribeTable("IsValidHostName",
	func(s string, expected bool) {
		Ω(util.IsValidHostName(s)).Should(Equal(expected))
	},
	Entry("ip4", "1.2.3.4", true),
	Entry("ip6", "2001:db8:3333:4444:5555:6666:7777:8888", true),
	Entry("single letter", "a", true),
	Entry("two letters", "ab", true),
	Entry("single digit", "1", true),
	Entry("two digits", "12", true),
	Entry("letter-dash-letter", "a-b", true),
	Entry("digit-dash-digit", "1-2", true),
	Entry("letter-dash-digit", "a-1", true),
	Entry("digit-dash-letter", "1-a", true),
	Entry("alphanumeric", "abc123", true),
	Entry("begins with dash", "-a", false),
	Entry("ends with dash", "a-", false),
	Entry("single dash", "-", false),
	Entry("unicode ✓", "✓", true),
	Entry("alphanumeric and unicode ✓", "ch✓ck", true),
	Entry("two segments", "a.b", false),
	Entry("three segments", "a.b.c", false),
	Entry("exceeds 63 chars", strings.Repeat("a", 64), false),
)

var _ = DescribeTable("IsValidDomainName",
	func(s string, expected bool) {
		Ω(util.IsValidDomainName(s)).Should(Equal(expected))
	},
	Entry("ip4", "1.2.3.4", false),
	Entry("ip6", "2001:db8:3333:4444:5555:6666:7777:8888", false),
	Entry("single letter", "a", false),
	Entry("two letters", "ab", true),
	Entry("single digit", "1", false),
	Entry("two digits", "12", true),
	Entry("letter-dash-letter", "a-b", true),
	Entry("digit-dash-digit", "1-2", true),
	Entry("letter-dash-digit", "a-1", true),
	Entry("digit-dash-letter", "1-a", true),
	Entry("alphanumeric", "abc123", true),
	Entry("begins with dash", "-a", false),
	Entry("ends with dash", "a-", false),
	Entry("single dash", "-", false),
	Entry("unicode ✓ in top-level domain", "✓", false),
	Entry("alphanumeric and unicode ✓ in top-level domain", "ch✓ck", false),
	Entry("unicode ✓ in sub-domain", "✓.com", true),
	Entry("alphanumeric and unicode ✓ in sub-domain", "ch✓ck.com", true),
	Entry("two segments with single-character top-level domain", "a.b", false),
	Entry("three segments with single-character top-level domain", "a.b.c", false),
	Entry("two segments", "a.com", true),
	Entry("three segments", "a.b.com", true),
	Entry("exceeds 253 chars", strings.Repeat("a", 254), false),
)
