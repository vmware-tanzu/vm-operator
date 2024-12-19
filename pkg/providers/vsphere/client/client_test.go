// Copyright (c) 2019-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	"context"
	"crypto/tls"
	"net"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	_ "github.com/vmware/govmomi/pbm/simulator" // load PBM simulator
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/session/keepalive"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vapi/rest"
	_ "github.com/vmware/govmomi/vapi/simulator" // load VAPI simulator
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/credentials"
	vsphereclient "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/client"
)

const (
	invalid = "invalid"
	valid   = "valid"
)

var _ = Describe("Client", Label(testlabels.VCSim), Ordered /* Avoided race for pkg symbol simulator.SessionIdleTimeout */, func() {

	Describe("NewClient", func() {
		var (
			ctx context.Context
			cfg *config.VSphereVMProviderConfig

			model     *simulator.Model
			server    *simulator.Server
			tlsConfig *tls.Config

			expectedUsername string
			expectedPassword string
			serverCertFile   string
		)

		BeforeEach(func() {
			ctx = logr.NewContext(pkgcfg.NewContextWithDefaultConfig(), GinkgoLogr)
			expectedUsername = valid
			expectedPassword = valid
			tlsConfig = &tls.Config{}
		})

		JustBeforeEach(func() {
			model = simulator.VPX()
			Expect(model.Create()).To(Succeed())

			model.Service.RegisterEndpoints = true

			// Get a free port on localhost and use that for the server.
			addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
			Expect(err).ToNot(HaveOccurred())
			l, err := net.ListenTCP("tcp", addr)
			Expect(err).ToNot(HaveOccurred())
			Expect(l.Close()).To(Succeed())
			model.Service.Listen = &url.URL{
				Host: l.Addr().String(),
				User: url.UserPassword(expectedUsername, expectedPassword),
			}

			// Configure TLS.
			model.Service.TLS = tlsConfig

			// Create the server.
			server = model.Service.NewServer()

			// If the server uses TLS then record the path to the server's cert.
			if tlsConfig != nil {
				f, err := server.CertificateFile()
				Expect(err).ToNot(HaveOccurred())
				serverCertFile = f
			}

			datacenter := simulator.Map.Any("Datacenter")

			cfg = &config.VSphereVMProviderConfig{
				VcPNID: server.URL.Hostname(),
				VcPort: server.URL.Port(),
				VcCreds: &credentials.VSphereVMProviderCredentials{
					Username: expectedPassword,
					Password: expectedPassword,
				},
				CAFilePath:            serverCertFile,
				InsecureSkipTLSVerify: false,
				Datacenter:            datacenter.Reference().Value,
			}
		})

		AfterEach(func() {
			server.Close()
			model = nil
			server = nil
			serverCertFile = ""
			cfg = nil
		})

		When("credentials are valid", func() {
			It("should connect", func() {
				c, err := client.NewClient(ctx, cfg)
				Expect(err).ToNot(HaveOccurred())
				Expect(c).ToNot(BeNil())
			})
		})

		When("username and password are invalid", func() {
			JustBeforeEach(func() {
				cfg.VcCreds.Username = invalid
				cfg.VcCreds.Password = invalid
			})
			It("should fail to login", func() {
				c, err := client.NewClient(ctx, cfg)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(HavePrefix("login failed for url"))
				Expect(c).To(BeNil())
			})
		})

		When("username is invalid", func() {
			JustBeforeEach(func() {
				cfg.VcCreds.Username = invalid
			})
			It("should fail to login", func() {
				c, err := client.NewClient(ctx, cfg)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(HavePrefix("login failed for url"))
				Expect(c).To(BeNil())
			})
		})

		When("password is invalid", func() {
			JustBeforeEach(func() {
				cfg.VcCreds.Password = invalid
			})
			It("should fail to login", func() {
				c, err := client.NewClient(ctx, cfg)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(HavePrefix("login failed for url"))
				Expect(c).To(BeNil())
			})
		})

		When("host is invalid", func() {
			JustBeforeEach(func() {
				cfg.VcPNID = "test-host"
				cfg.VcPort = "test-port"
			})
			Specify("returns failed to parse error", func() {
				c, err := client.NewClient(ctx, cfg)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(invalid))
				Expect(err.Error()).To(ContainSubstring("port"))
				Expect(err.Error()).To(ContainSubstring("test-port"))
				Expect(c).To(BeNil())
			})

			Context("and port", func() {
				JustBeforeEach(func() {
					cfg.VcPNID = "test%test"
					cfg.VcPort = server.URL.Port()
				})
				Specify("returns failed to parse error", func() {
					c, err := client.NewClient(ctx, cfg)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(HavePrefix("failed to parse"))
					Expect(c).To(BeNil())
				})
			})
		})

		When("ca bundle is invalid", func() {
			JustBeforeEach(func() {
				cfg.CAFilePath = "/a/nonexistent/ca-bundle.crt"
			})
			Specify("returns failed to parse error", func() {
				c, err := client.NewClient(ctx, cfg)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to set root CA"))
				Expect(c).To(BeNil())
			})
		})

		When("client cannot verify the server certificate", func() {
			JustBeforeEach(func() {
				cfg.CAFilePath = ""
			})
			Specify("returns unknown authority error", func() {
				c, err := client.NewClient(ctx, cfg)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("x509: certificate signed by unknown authority"))
				Expect(c).To(BeNil())
			})
		})
	})

	// The keepalive tests must be ordered after the NewClient tests so that
	// setting simulator.SessionIdleTimeout does not impact the NewClient tests.
	Describe("Keepalive", Ordered, func() {

		var (
			ctx                context.Context
			sessionIdleTimeout = time.Second / 2
			keepAliveIdle      = sessionIdleTimeout / 2
			sessionCheckPause  = 3 * sessionIdleTimeout // Avoid unit test thread waking up before session expires
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

		BeforeAll(func() {
			simulator.SessionIdleTimeout = sessionIdleTimeout
		})

		BeforeEach(func() {
			ctx = context.Background()
		})

		Context("for SOAP sessions", func() {
			Context("with a logged out session)", func() {
				It("logs back into the session", func() {
					simulator.Test(func(ctx context.Context, c *vim25.Client) {
						m := session.NewManager(c)

						// Session should be valid since simulator logs in the
						// client by default
						assertSoapSessionValid(ctx, m)

						// Sleep for time > sessionIdleTimeout, so the session
						// expires
						time.Sleep(sessionCheckPause)

						// Session should be expired now
						assertSoapSessionExpired(ctx, m)

						// Set the keepalive handler
						c.RoundTripper = keepalive.NewHandlerSOAP(
							c.RoundTripper,
							keepAliveIdle,
							vsphereclient.SoapKeepAliveHandlerFn(ctx, c.Client, m, simulator.DefaultLogin))

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

			//
			// Test NotAuthenticated fault
			//

			// We cannot "self-terminate" a session. So, this following two tests
			// creates _two_ session managers. One with the custom keepalive
			// handler, and another session to orchestrate the "NotAuthenticated"
			// fault. We terminate the first session using the second session's
			// manager.

			When("a session is terminated", func() {
				Context("and handler is called with correct userInfo", func() {
					It("log back into the session", func() {
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
							c.RoundTripper = keepalive.NewHandlerSOAP(
								c.RoundTripper,
								keepAliveIdle,
								vsphereclient.SoapKeepAliveHandlerFn(ctx, c.Client, m1, simulator.DefaultLogin))

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

				Context("and handler is called with wrong userInfo", func() {
					It("fails to log back into the session", func() {
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
							c.RoundTripper = keepalive.NewHandlerSOAP(
								c.RoundTripper,
								keepAliveIdle,
								vsphereclient.SoapKeepAliveHandlerFn(ctx, c.Client, m1, nil))

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
			When("a REST session is logged out)", func() {
				It("logs back into the session", func() {
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
						c.Transport = keepalive.NewHandlerREST(
							c,
							keepAliveIdle,
							vsphereclient.RestKeepAliveHandlerFn(ctx, c, simulator.DefaultLogin))

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
})
