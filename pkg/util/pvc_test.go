// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

var _ = DescribeTable("GeneratePVCName",
	func(vmName, diskUUID, expectedName string) {
		Expect(pkgutil.GeneratePVCName(vmName, diskUUID)).To(Equal(expectedName))
	},
	Entry("normal VM name and disk UUID", "test-vm", "disk-uuid-123", "test-vm-3b1bcd0b"),
	Entry("VM name exactly 54 characters", strings.Repeat("a", 54), "disk-uuid-123", strings.Repeat("a", 54)+"-3b1bcd0b"),
	Entry("VM name longer than 54 characters gets truncated", strings.Repeat("a", 60), "disk-uuid-123", strings.Repeat("a", 54)+"-3b1bcd0b"),
	Entry("VM name exactly 55 characters gets truncated", strings.Repeat("a", 55), "disk-uuid-123", strings.Repeat("a", 54)+"-3b1bcd0b"),
	Entry("empty VM name", "", "disk-uuid-123", "-3b1bcd0b"),
	Entry("empty disk UUID", "test-vm", "", "test-vm-d41d8cd9"),
	Entry("both empty strings", "", "", "-d41d8cd9"),
	Entry("VM name with special characters", "test-vm_123", "disk-uuid-456", "test-vm_123-62861efd"),
	Entry("VM name with hyphens", "test-vm-name", "disk-uuid-789", "test-vm-name-14c657fe"),
	Entry("VM name with dots", "test.vm.name", "disk-uuid-abc", "test.vm.name-1666914f"),
	Entry("very long VM name", strings.Repeat("a", 100), "disk-uuid-xyz", strings.Repeat("a", 54)+"-358413e3"),
	Entry("VM name with unicode characters", "测试虚拟机", "disk-uuid-123", "测试虚拟机-3b1bcd0b"),
	Entry("disk UUID with special characters", "test-vm", "disk-uuid_123!@#", "test-vm-cdc3d582"),
	Entry("very long disk UUID", "test-vm", strings.Repeat("a", 1000), "test-vm-cabe45dc"),
	Entry("disk UUID with newlines", "test-vm", "disk-uuid\nwith\nnewlines", "test-vm-67237cc8"),
	Entry("disk UUID with tabs", "test-vm", "disk-uuid\twith\ttabs", "test-vm-d6adda28"),
	Entry("single character VM name", "a", "disk-uuid-123", "a-3b1bcd0b"),
	Entry("single character disk UUID", "test-vm", "x", "test-vm-9dd4e461"),
	Entry("VM name with numbers only", "123456789", "disk-uuid-123", "123456789-3b1bcd0b"),
	Entry("disk UUID with numbers only", "test-vm", "123456789", "test-vm-25f9e794"),
	Entry("VM name at boundary 53 characters", strings.Repeat("a", 53), "disk-uuid-123", strings.Repeat("a", 53)+"-3b1bcd0b"),
	Entry("VM name at boundary 54 characters", strings.Repeat("a", 54), "disk-uuid-123", strings.Repeat("a", 54)+"-3b1bcd0b"),
	Entry("VM name at boundary 55 characters", strings.Repeat("a", 55), "disk-uuid-123", strings.Repeat("a", 54)+"-3b1bcd0b"),
)
