// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentlibrary_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/contentlibrary"
)

var _ = Describe("ParseVirtualHardwareVersion", func() {
	It("empty hardware string", func() {
		vmxHwVersionString := ""
		Expect(contentlibrary.ParseVirtualHardwareVersion(vmxHwVersionString)).To(BeZero())
	})

	It("invalid hardware string", func() {
		vmxHwVersionString := "blah"
		Expect(contentlibrary.ParseVirtualHardwareVersion(vmxHwVersionString)).To(BeZero())
	})

	It("valid hardware version string eg. vmx-15", func() {
		vmxHwVersionString := "vmx-15"
		Expect(contentlibrary.ParseVirtualHardwareVersion(vmxHwVersionString)).To(Equal(int32(15)))
	})
})
