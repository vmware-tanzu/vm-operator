//go:build !race

// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	"context"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/session/keepalive"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"

	. "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/client"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/config"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/credentials"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/test"
)

func testConfig(vcpnid string, vcport string, user string, pass string) *config.VSphereVMProviderConfig {
	providerConfig := &config.VSphereVMProviderConfig{
		VcPNID:     vcpnid,
		VcPort:     vcport,
		Datacenter: "datacenter-2",
		VcCreds: &credentials.VSphereVMProviderCredentials{
			Username: user,
			Password: pass,
		},
		// Let the tests run without TLS validation by default.
		InsecureSkipTLSVerify: true,
	}
	return providerConfig
}

var _ = Describe("keepalive handler", func() {

	var (
		sessionIdleTimeout = time.Second / 2
		keepAliveIdle      = sessionIdleTimeout / 2
		sessionCheckPause  = 3 * sessionIdleTimeout // Avoid unit test thread waking up before session expires
		simulatorIdleTime  time.Duration
	)

	assertSoapSessionValid := func(ctx context.Context, m *session.Manager) {
		s, err := m.UserSession(ctx)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		ExpectWithOffset(1, s).NotTo(BeNil())
	}

	assertSoapSessionExpired := func(ctx context.Context, m *session.Manager) {
		s, err := m.UserSession(ctx)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		ExpectWithOffset(1, s).To(BeNil())
	}

	BeforeEach(func() {
		// Sharing this variable causes the race detector to trip.
		simulator.SessionIdleTimeout = sessionIdleTimeout
	})

	AfterEach(func() {
		simulator.SessionIdleTimeout = simulatorIdleTime
	})

	Context("for SOAP sessions", func() {
		Context("with a logged out session)", func() {
			It("re-logins the session", func() {
				simulator.Test(func(ctx context.Context, c *vim25.Client) {
					m := session.NewManager(c)

					// Session should be valid since simulator logs in the client by default
					assertSoapSessionValid(ctx, m)

					// Sleep for time > sessionIdleTimeout, so the session expires
					time.Sleep(sessionCheckPause)

					// Session should be expired now
					assertSoapSessionExpired(ctx, m)

					// Set the keepalive handler
					c.RoundTripper = keepalive.NewHandlerSOAP(c.RoundTripper, keepAliveIdle, SoapKeepAliveHandlerFn(c.Client, m, simulator.DefaultLogin))

					// Start the handler
					Expect(m.Login(ctx, simulator.DefaultLogin)).To(Succeed())

					time.Sleep(sessionCheckPause)

					// Session should not have been expired
					assertSoapSessionValid(ctx, m)

					Expect(m.Logout(ctx)).To(Succeed())
					assertSoapSessionExpired(ctx, m)
				})
			})
		})

		getNewSessionManager := func(url *url.URL) *session.Manager {
			c2, err := vim25.NewClient(ctx, soap.NewClient(url, true))
			Expect(err).NotTo(HaveOccurred())
			Expect(c2).NotTo(BeNil())

			// With default keepalive handler
			m2 := session.NewManager(c2)

			c2.RoundTripper = keepalive.NewHandlerSOAP(c2.RoundTripper, keepAliveIdle, nil)

			// Start the handler
			Expect(m2.Login(ctx, simulator.DefaultLogin)).To(Succeed())

			return m2
		}

		// To test NotAuthenticated Fault:
		// We cannot "self-terminate" a session. So, this following two tests creates _two_ session managers. One with
		// the custom keepalive handler, and another session to orchestrate the "NotAuthenticated" fault.
		// We terminate the first session using the second session's manager.

		Context("Re-login on NotAuthenticated: with a session that is terminated", func() {
			Context("when handler is called with correct userInfo", func() {
				It("re-logins the session", func() {
					simulator.Test(func(ctx context.Context, c *vim25.Client) {

						// With custom keepalive handler
						m1 := session.NewManager(c)
						// Orchestrator session
						m2 := getNewSessionManager(c.URL())

						// Session should be valid
						assertSoapSessionValid(ctx, m1)

						// Sleep for time > sessionIdleTimeout
						time.Sleep(sessionCheckPause)

						// Session should be expired now
						assertSoapSessionExpired(ctx, m1)

						// set the keepalive handler
						c.RoundTripper = keepalive.NewHandlerSOAP(c.RoundTripper, keepAliveIdle, SoapKeepAliveHandlerFn(c.Client, m1, simulator.DefaultLogin))

						// Start the handler
						Expect(m1.Login(ctx, simulator.DefaultLogin)).To(Succeed())

						// Wait for the keepalive handler to get called
						time.Sleep(sessionCheckPause)

						// Session should be valid since Login starts a new one
						assertSoapSessionValid(ctx, m1)

						// Terminate session to emulate NotAuthenticated Error
						sess, err := m1.UserSession(ctx)
						Expect(err).NotTo(HaveOccurred())
						Expect(sess).NotTo(BeNil())

						By("terminating the session")
						Expect(m2.TerminateSession(ctx, []string{sess.Key})).To(Succeed())

						// session expired since it is terminated
						assertSoapSessionExpired(ctx, m1)

						time.Sleep(sessionCheckPause)

						// keepalive handler must have re-logged in
						assertSoapSessionValid(ctx, m1)
					})
				})
			})
		})

		Context("Relogin on NotAuthenticated: with a session that is terminated", func() {
			Context("when handler is called with wrong userInfo", func() {
				It("re-logins the session", func() {
					simulator.Test(func(ctx context.Context, c *vim25.Client) {

						// With custom keepalive handler
						m1 := session.NewManager(c)
						// With default keepalive handler
						m2 := getNewSessionManager(c.URL())

						// Session should be valid
						assertSoapSessionValid(ctx, m1)

						// Sleep for time > sessionIdleTimeout
						time.Sleep(sessionCheckPause)

						// Session should be expired now
						assertSoapSessionExpired(ctx, m1)

						// set the keepalive handler with wrong userInfo
						c.RoundTripper = keepalive.NewHandlerSOAP(c.RoundTripper, keepAliveIdle, SoapKeepAliveHandlerFn(c.Client, m1, nil))

						// Start the handler
						Expect(m1.Login(ctx, simulator.DefaultLogin)).To(Succeed())

						// Wait for the keepalive handler to get called
						time.Sleep(sessionCheckPause)

						// Session should be valid since Login starts a new one
						assertSoapSessionValid(ctx, m1)

						// Terminate session to emulate NotAuthenticated Error
						sess, err := m1.UserSession(ctx)
						Expect(err).NotTo(HaveOccurred())
						Expect(sess).NotTo(BeNil())

						By("terminating the session")
						Expect(m2.TerminateSession(ctx, []string{sess.Key})).To(Succeed())

						// session expired since it is terminated
						assertSoapSessionExpired(ctx, m1)

						// Wait for keepalive handler to be called
						time.Sleep(sessionCheckPause)

						// keepalive handler should error out, session still invalid
						assertSoapSessionExpired(ctx, m1)
					})
				})
			})
		})
	})

	assertRestSessionValid := func(ctx context.Context, c *rest.Client) {
		s, err := c.Session(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(s).NotTo(BeNil())
	}

	assertRestSessionExpired := func(ctx context.Context, c *rest.Client) {
		s, err := c.Session(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(s).To(BeNil())
	}

	Context("REST sessions", func() {
		Context("When a REST session is logged out)", func() {
			It("re-logins the session", func() {
				simulator.Test(func(ctx context.Context, vc *vim25.Client) {
					c := rest.NewClient(vc)

					Expect(c.Login(ctx, simulator.DefaultLogin)).To(Succeed())

					// Session should be valid
					assertRestSessionValid(ctx, c)

					// Sleep for time > sessionIdleTimeout
					time.Sleep(sessionCheckPause)

					// Session should be expired now
					assertRestSessionExpired(ctx, c)

					// Set the keepalive handler
					c.Transport = keepalive.NewHandlerREST(c, keepAliveIdle, RestKeepAliveHandlerFn(c, simulator.DefaultLogin))

					// Start the handler
					Expect(c.Login(ctx, simulator.DefaultLogin)).To(Succeed())

					// Session should not have been expired
					assertRestSessionValid(ctx, c)

					Expect(c.Logout(ctx)).To(Succeed())
					assertRestSessionExpired(ctx, c)
				})
			})
		})
	})
})

var _ = Describe("NewClient", func() {
	Context("When called with valid config", func() {
		Specify("returns a valid client and no error", func() {
			client, err := NewClient(ctx, testConfig(server.URL.Hostname(), server.URL.Port(), "some-username", "some-password"))
			Expect(err).ToNot(HaveOccurred())
			Expect(client).ToNot(BeNil())
		})
	})

	Context("When called with invalid host and port", func() {
		Specify("soap.ParseURL should fail", func() {
			failConfig := testConfig("test%test", "", "test-user", "test-pass")
			client, err := NewClient(ctx, failConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(HavePrefix("failed to parse"))
			Expect(client).To(BeNil())
		})
	})

	Context("When called with invalid VC PNID", func() {
		Specify("returns failed to parse error", func() {
			failConfig := testConfig("test-pnid", "test-port", "test-user", "test-pass")
			client, err := NewClient(ctx, failConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid"))
			Expect(err.Error()).To(ContainSubstring("port"))
			Expect(err.Error()).To(ContainSubstring("test-port"))
			Expect(client).To(BeNil())
		})
	})

	DescribeTable("Should fail if given wrong username and/or wrong password",
		func(expectedUsername, expectedPassword, username, password string) {
			server.URL.User = url.UserPassword(expectedUsername, expectedPassword)
			model.Service.Listen = server.URL
			config := testConfig(server.URL.Hostname(), server.URL.Port(), username, password)
			client, err := NewClient(ctx, config)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(HavePrefix("login failed for url"))
			Expect(client).To(BeNil())
		},
		Entry("with wrong username and password", "correct-username", "correct-password", "username", "password"),
		Entry("with wrong username and correct password", "correct-username", "correct-password", "username", "correct-password"),
		Entry("with correct username and wrong password", "correct-username", "correct-password", "correct-username", "password"),
	)
})

// Most of the other VM Operator tests run without TLS verification. Start up a separate simulator with a fresh TLS key/cert
//
//	and ensure the client can connect to it.
var _ = Describe("Tests for client TLS", func() {
	Context("when the client recognizes the certificate presented by the VC", func() {
		It("successfully connects to the VC", func() {
			tlsServer.URL.User = url.UserPassword("some-username", "some-password")
			config := testConfig(tlsServer.URL.Hostname(), tlsServer.URL.Port(), "some-username", "some-password")
			config.InsecureSkipTLSVerify = false
			config.CAFilePath = tlsServerCertPath
			_, err := NewClient(context.Background(), config)

			Expect(err).NotTo(HaveOccurred())
		})
	})
	Context("when the CA bundle referred to does not exist", func() {
		It("returns an error about loading the CA bundle", func() {
			tlsServer.URL.User = url.UserPassword("some-username", "some-password")
			config := testConfig(tlsServer.URL.Hostname(), tlsServer.URL.Port(), "some-username", "some-password")
			config.CAFilePath = "/a/nonexistent/ca-bundle.crt"
			config.InsecureSkipTLSVerify = false
			_, err := NewClient(context.Background(), config)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to set root CA"))
		})

	})
	Context("when the client does not recognize the certificate presented by the VC", func() {
		Context("when TLS verification is used", func() {
			It("returns an error about certificate validation", func() {
				tlsServer.URL.User = url.UserPassword("some-username", "some-password")
				config := testConfig(tlsServer.URL.Hostname(), tlsServer.URL.Port(), "some-username", "some-password")
				_, randomCANotForServer := test.GenerateSelfSignedCert()
				config.CAFilePath = randomCANotForServer
				config.InsecureSkipTLSVerify = false
				_, err := NewClient(context.Background(), config)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("x509: certificate signed by unknown authority"))
			})
		})
	})
})
