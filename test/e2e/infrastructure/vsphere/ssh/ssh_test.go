// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package ssh_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	e2essh "github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/ssh"
)

var _ = Describe("GetGatewayHost", func() {
	DescribeTable("resolving the gateway VM address",
		func(gatewayIP, httpProxy, expected, expectedErr string) {
			GinkgoT().Setenv("GATEWAY_IP", gatewayIP)
			GinkgoT().Setenv("HTTP_PROXY", httpProxy)

			host, err := e2essh.GetGatewayHost()
			if expectedErr != "" {
				Expect(err).To(MatchError(ContainSubstring(expectedErr)))
				return
			}
			Expect(err).ToNot(HaveOccurred())
			Expect(host).To(Equal(expected))
		},

		Entry("GATEWAY_IP is authoritative",
			"172.21.1.2", "", "172.21.1.2", ""),
		Entry("GATEWAY_IP wins over a SOCKS HTTP_PROXY (isolated testbed)",
			"172.21.1.2", "socks5h://127.0.0.1:1080", "172.21.1.2", ""),
		Entry("falls back to the host of a bare host:port HTTP_PROXY",
			"", "10.0.0.5:3128", "10.0.0.5", ""),
		Entry("a scheme-bearing HTTP_PROXY is not mistaken for the gateway",
			"", "socks5h://127.0.0.1:1080", "", "is a proxy URL"),
		Entry("neither variable set",
			"", "", "", "should be set"),
	)
})
