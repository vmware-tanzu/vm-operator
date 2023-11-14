// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

var _ = Describe("ParseVirtualHardwareVersion", func() {
	It("empty hardware string", func() {
		vmxHwVersionString := ""
		Expect(util.ParseVirtualHardwareVersion(vmxHwVersionString)).To(BeZero())
	})

	It("invalid hardware string", func() {
		vmxHwVersionString := "blah"
		Expect(util.ParseVirtualHardwareVersion(vmxHwVersionString)).To(BeZero())
	})

	It("valid hardware version string eg. vmx-15", func() {
		vmxHwVersionString := "vmx-15"
		Expect(util.ParseVirtualHardwareVersion(vmxHwVersionString)).To(Equal(int32(15)))
	})
})
