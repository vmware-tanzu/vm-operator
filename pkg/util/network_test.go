// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
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
		Expect(util.IsValidHostName(s)).Should(Equal(expected))
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
		Expect(util.IsValidDomainName(s)).Should(Equal(expected))
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

var _ = DescribeTable("Dedupe",
	func(s, expected []string) {
		Expect(util.Dedupe(s)).Should(Equal(expected))
	},
	Entry("empty string list", []string{}, []string{}),
	Entry("unique one string list", []string{"1.1.2.2"}, []string{"1.1.2.2"}),
	Entry("unique two string list", []string{"1.1.2.2", "1.2.3.4"}, []string{"1.1.2.2", "1.2.3.4"}),
	Entry("duplicate two string list", []string{"1.1.2.2", "1.1.2.2", "1.2.3.4", "1.2.3.4"}, []string{"1.1.2.2", "1.2.3.4"}),
	Entry("duplicate three string list", []string{"1.1.2.2", "1.2.3.4", "1.1.2.2", "1.2.3.4", "1.1.2.2", "1.2.3.4"}, []string{"1.1.2.2", "1.2.3.4"}),
)

var _ = DescribeTable("ParseIP",
	func(s, expectedIP, expectedNetwork string, shouldError bool) {
		ip, network, err := util.ParseIP(s)

		if shouldError {
			Expect(err).Should(HaveOccurred())
			Expect(ip).Should(BeNil())
			Expect(network).Should(BeNil())
		} else {
			Expect(err).ShouldNot(HaveOccurred())
			if expectedIP == "" {
				// Expecting nil IP (invalid input)
				Expect(ip).Should(BeNil())
			} else {
				Expect(ip).ShouldNot(BeNil())
				Expect(ip.String()).Should(Equal(expectedIP))
			}
			if expectedNetwork == "" {
				Expect(network).Should(BeNil())
			} else {
				Expect(network).ShouldNot(BeNil())
				Expect(network.String()).Should(Equal(expectedNetwork))
			}
		}
	},
	// CIDR cases (contains "/")
	Entry("valid IPv4 CIDR", "192.168.1.0/24", "192.168.1.0", "192.168.1.0/24", false),
	Entry("valid IPv4 CIDR single host", "192.168.1.1/32", "192.168.1.1", "192.168.1.1/32", false),
	Entry("valid IPv4 CIDR /8", "10.0.0.0/8", "10.0.0.0", "10.0.0.0/8", false),
	Entry("valid IPv4 CIDR /16", "172.16.0.0/16", "172.16.0.0", "172.16.0.0/16", false),
	Entry("valid IPv6 CIDR", "2001:db8::/32", "2001:db8::", "2001:db8::/32", false),
	Entry("valid IPv6 CIDR single host", "2001:db8::1/128", "2001:db8::1", "2001:db8::1/128", false),
	Entry("invalid IPv4 CIDR - bad IP", "256.256.256.256/24", "", "", true),
	Entry("invalid IPv4 CIDR - bad mask", "192.168.1.0/33", "", "", true),
	Entry("invalid IPv6 CIDR - bad IP", "2001:db8::g/32", "", "", true),
	Entry("invalid IPv6 CIDR - bad mask", "2001:db8::/129", "", "", true),
	Entry("invalid CIDR - empty string", "", "", "", false), // Returns nil IP, no error
	Entry("invalid CIDR - just slash", "/", "", "", true),
	Entry("invalid CIDR - slash at start", "/24", "", "", true),
	Entry("invalid CIDR - slash at end", "192.168.1.0/", "", "", true),
	Entry("invalid CIDR - multiple slashes", "192.168.1.0/24/32", "", "", true),
	Entry("invalid CIDR - non-numeric mask", "192.168.1.0/abc", "", "", true),
	Entry("invalid CIDR - negative mask", "192.168.1.0/-1", "", "", true),

	// IP cases (no "/")
	Entry("valid IPv4", "192.168.1.1", "192.168.1.1", "", false),
	Entry("valid IPv4 localhost", "127.0.0.1", "127.0.0.1", "", false),
	Entry("valid IPv4 zero", "0.0.0.0", "0.0.0.0", "", false),
	Entry("valid IPv4 broadcast", "255.255.255.255", "255.255.255.255", "", false),
	Entry("valid IPv6", "2001:db8::1", "2001:db8::1", "", false),
	Entry("valid IPv6 localhost", "::1", "::1", "", false),
	Entry("valid IPv6 zero", "::", "::", "", false),
	Entry("valid IPv6 full", "2001:0db8:0000:0000:0000:0000:0000:0001", "2001:db8::1", "", false),
	Entry("valid IPv4-mapped IPv6", "::ffff:192.168.1.1", "192.168.1.1", "", false), // Returns IPv4 address
	Entry("invalid IPv4 - out of range", "256.1.1.1", "", "", false),                // Returns nil IP, no error
	Entry("invalid IPv4 - too many octets", "1.2.3.4.5", "", "", false),             // Returns nil IP, no error
	Entry("invalid IPv4 - too few octets", "1.2.3", "", "", false),                  // Returns nil IP, no error
	Entry("invalid IPv4 - non-numeric", "192.168.1.a", "", "", false),               // Returns nil IP, no error
	Entry("invalid IPv6 - too many segments", "2001:db8::1::2", "", "", false),      // Returns nil IP, no error
	Entry("invalid IPv6 - invalid characters", "2001:db8::g", "", "", false),        // Returns nil IP, no error
	Entry("invalid IP - empty string", "", "", "", false),                           // Returns nil IP, no error
	Entry("invalid IP - just dots", "...", "", "", false),                           // Returns nil IP, no error
	Entry("invalid IP - just colons", "::::", "", "", false),                        // Returns nil IP, no error
	Entry("invalid IP - mixed v4/v6", "192.168.1.1::1", "", "", false),              // Returns nil IP, no error
	Entry("invalid IP - leading zeros in IPv4", "192.168.001.001", "", "", false),   // Returns nil IP, no error
)
