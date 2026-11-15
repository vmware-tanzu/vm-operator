// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package ssh_test

import (
	"net"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/ssh"

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

var _ = Describe("DialSSH", func() {
	var (
		listener net.Listener
		accepted chan net.Conn
	)

	BeforeEach(func() {
		// A SOCKS5 proxy that accepts connections and then never answers.
		var err error
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		Expect(err).ToNot(HaveOccurred(), "listen for the stalling SOCKS5 proxy")
		DeferCleanup(listener.Close)

		accepted = make(chan net.Conn, 8)
		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				accepted <- conn
			}
		}()

		// golang.org/x/net/proxy reads ALL_PROXY once per process, so this
		// must be the first dial in the suite (nothing else here dials).
		GinkgoT().Setenv("ALL_PROXY", "socks5h://"+listener.Addr().String())
		GinkgoT().Setenv("NO_PROXY", "")
	})

	It("returns within the timeout when the proxy stalls the SOCKS5 negotiation", func() {
		config := &ssh.ClientConfig{
			User:            "root",
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         time.Second,
		}

		done := make(chan error, 1)
		go func() {
			_, err := e2essh.DialSSH("192.0.2.1:22", config)
			done <- err
		}()

		// The dial must have gone to the proxy, not directly to the target.
		var conn net.Conn
		Eventually(accepted).WithTimeout(5 * time.Second).Should(Receive(&conn))
		DeferCleanup(conn.Close)

		var err error
		Eventually(done).WithTimeout(5 * time.Second).Should(Receive(&err))
		Expect(err).To(HaveOccurred())
	})
})
