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
	Entry("normal VM name and disk UUID", "test-vm", "disk-uuid-123", "test-vm-54cddb33"),
	Entry("VM name exactly 54 characters", strings.Repeat("a", 54), "disk-uuid-123", strings.Repeat("a", 54)+"-54cddb33"),
	Entry("VM name longer than 54 characters gets truncated", strings.Repeat("a", 60), "disk-uuid-123", strings.Repeat("a", 54)+"-54cddb33"),
	Entry("VM name exactly 55 characters gets truncated", strings.Repeat("a", 55), "disk-uuid-123", strings.Repeat("a", 54)+"-54cddb33"),
	Entry("empty VM name", "", "disk-uuid-123", "-54cddb33"),
	Entry("empty disk UUID", "test-vm", "", "test-vm-ef46db37"),
	Entry("both empty strings", "", "", "-ef46db37"),
	Entry("VM name with special characters", "test-vm_123", "disk-uuid-456", "test-vm_123-c6a0a3e7"),
	Entry("VM name with hyphens", "test-vm-name", "disk-uuid-789", "test-vm-name-134e95b6"),
	Entry("VM name with dots", "test.vm.name", "disk-uuid-abc", "test.vm.name-6efba0c7"),
	Entry("very long VM name", strings.Repeat("a", 100), "disk-uuid-xyz", strings.Repeat("a", 54)+"-fe45de1a"),
	Entry("VM name with unicode characters", "测试虚拟机", "disk-uuid-123", "测试虚拟机-54cddb33"), //nolint:gosmopolitan
	Entry("disk UUID with special characters", "test-vm", "disk-uuid_123!@#", "test-vm-918f3a0c"),
	Entry("very long disk UUID", "test-vm", strings.Repeat("a", 1000), "test-vm-56e43b71"),
	Entry("disk UUID with newlines", "test-vm", "disk-uuid\nwith\nnewlines", "test-vm-6d58741c"),
	Entry("disk UUID with tabs", "test-vm", "disk-uuid\twith\ttabs", "test-vm-76fa9f93"),
	Entry("single character VM name", "a", "disk-uuid-123", "a-54cddb33"),
	Entry("single character disk UUID", "test-vm", "x", "test-vm-5c80c096"),
	Entry("VM name with numbers only", "123456789", "disk-uuid-123", "123456789-54cddb33"),
	Entry("disk UUID with numbers only", "test-vm", "123456789", "test-vm-8cb841db"),
	Entry("VM name at boundary 53 characters", strings.Repeat("a", 53), "disk-uuid-123", strings.Repeat("a", 53)+"-54cddb33"),
	Entry("VM name at boundary 54 characters", strings.Repeat("a", 54), "disk-uuid-123", strings.Repeat("a", 54)+"-54cddb33"),
	Entry("VM name at boundary 55 characters", strings.Repeat("a", 55), "disk-uuid-123", strings.Repeat("a", 54)+"-54cddb33"),
)
