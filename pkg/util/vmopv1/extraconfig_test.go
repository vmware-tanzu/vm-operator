// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

var _ = Describe("EthernetExtraConfigPrefix", func() {
	DescribeTable("returns the correct ethernetX. prefix for a device key",
		func(deviceKey int32, expected string) {
			Expect(vmopv1util.EthernetExtraConfigPrefix(deviceKey)).To(Equal(expected))
		},
		Entry("key 4000 → ethernet0.", int32(4000), "ethernet0."),
		Entry("key 4001 → ethernet1.", int32(4001), "ethernet1."),
		Entry("key 4009 → ethernet9.", int32(4009), "ethernet9."),
	)
})
