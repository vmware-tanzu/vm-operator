// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/fault"
	"github.com/vmware/govmomi/pbm"
	pbmsim "github.com/vmware/govmomi/pbm/simulator"
	pbmtypes "github.com/vmware/govmomi/pbm/types"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/session/keepalive"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/simulator/sim25"
	"github.com/vmware/govmomi/vapi/rest"
	_ "github.com/vmware/govmomi/vapi/simulator" // load VAPI simulator
	"github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
)

// This file is in package client, not client_test, because the re-login
// wrappers and the session keeper are package-private. SOAP observations use
// vcsim dispatch hooks; REST header and concurrency checks use net/http.

const (
	reloginSimUsername = "valid"
	reloginSimPassword = "valid"
	reloginSimBadPass  = "invalid"

	reloginSimCategoryPath = "/rest/com/vmware/cis/tagging/category"
	reloginFaultPath       = "/rest/relogin-test"
)

// soapRecorder observes protocol method names through vcsim's dispatch hook.
// It does not depend on generated request-body type names or client wrappers.
type soapRecorder struct {
	mu   sync.Mutex
	seen []string
}

// recordSOAP installs an observer before the fixture starts issuing requests.
func recordSOAP(registry *simulator.Registry) *soapRecorder {
	r := &soapRecorder{}
	previous := registry.Handler
	registry.Handler = func(ctx *simulator.Context, method *simulator.Method) (mo.Reference, vimtypes.BaseMethodFault) {
		r.mu.Lock()
		r.seen = append(r.seen, method.Name)
		r.mu.Unlock()
		if previous != nil {
			return previous(ctx, method)
		}
		return nil, nil
	}
	return r
}

func (s *soapRecorder) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen = nil
}

func (s *soapRecorder) recorded() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.seen...)
}

func (s *soapRecorder) count(body string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, b := range s.seen {
		if b == body {
			n++
		}
	}
	return n
}

// spyHTTPRequest records the session header of one REST round trip.
type spyHTTPRequest struct {
	method        string
	path          string
	query         string
	session       string
	contentLength int64
}

// reloginHTTPRT delays responses for deterministic concurrency tests.
// vcsim serializes REST handlers, so barriers must run outside the server.
type reloginHTTPRT func(*http.Request) (*http.Response, error)

func (r reloginHTTPRT) RoundTrip(req *http.Request) (*http.Response, error) {
	return r(req)
}

// spyHTTPRT records HTTP headers and attempts that vcsim does not expose.
// It uses only the public net/http transport contract.
type spyHTTPRT struct {
	inner http.RoundTripper
	mu    sync.Mutex
	seen  []spyHTTPRequest
}

func (s *spyHTTPRT) RoundTrip(req *http.Request) (*http.Response, error) {
	s.mu.Lock()
	s.seen = append(s.seen, spyHTTPRequest{
		method:        req.Method,
		path:          req.URL.Path,
		query:         req.URL.RawQuery,
		session:       req.Header.Get(restSessionHeader),
		contentLength: req.ContentLength,
	})
	s.mu.Unlock()
	return s.inner.RoundTrip(req)
}

func (s *spyHTTPRT) recorded() []spyHTTPRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]spyHTTPRequest(nil), s.seen...)
}

func (s *spyHTTPRT) countPath(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.seen {
		if r.path == path {
			n++
		}
	}
	return n
}

// loginCount excludes logout and session probes, which share the login path.
func (s *spyHTTPRT) loginCount() int {
	n := 0
	for _, req := range s.recorded() {
		if req.path == restSessionPath && req.method == http.MethodPost && req.query == "" {
			n++
		}
	}
	return n
}

// reloginAdminSession returns a session manager over a second client that can
// terminate the primary client's session.
func reloginAdminSession(
	ctx context.Context,
	u *url.URL) *session.Manager {

	c, err := vim25.NewClient(ctx, soap.NewClient(u, true))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	m := session.NewManager(c)
	ExpectWithOffset(1, m.Login(ctx, url.UserPassword(reloginSimUsername, reloginSimPassword))).To(Succeed())
	return m
}

// terminateSession terminates the primary client's current session from the
// admin session, emulating a NotAuthenticated fault the way the existing
// keepalive tests do.
func terminateSession(
	ctx context.Context,
	sm *session.Manager,
	admin *session.Manager) {

	sess, err := sm.UserSession(ctx)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, sess).NotTo(BeNil())
	ExpectWithOffset(1, admin.TerminateSession(ctx, []string{sess.Key})).To(Succeed())
}

// startReloginSimServer starts a VPX simulator that only accepts the given
// credentials. Used by the specs that need real credential failures or fault
// injection; the simulator.Test helper accepts any non-empty credentials.
func startReloginSimServer(
	username, password string) (*simulator.Model, *simulator.Server) {

	model := simulator.VPX()
	Expect(model.Create()).To(Succeed())
	DeferCleanup(model.Remove)
	model.Service.TLS = &tls.Config{}
	model.Service.RegisterEndpoints = true

	// Let vcsim bind an ephemeral port atomically. A distinct Userinfo
	// enables its credential checks instead of DefaultLogin's permissive mode.
	model.Service.Listen = &url.URL{
		Host: "127.0.0.1:0",
		User: url.UserPassword(username, password),
	}

	server := model.Service.NewServer()
	DeferCleanup(server.Close)

	return model, server
}

var _ = Describe("Relogin", Label(testlabels.VCSim), func() {

	Describe("session keeper", func() {
		It("short-circuits a stale generation without logging in", func() {
			simulator.Test(func(ctx context.Context, c *vim25.Client) {
				spy := recordSOAP(simulator.Map(ctx))
				sm := session.NewManager(c)
				keeper := newSessionKeeper(ctx, sm, simulator.DefaultLogin)

				// vcsim rejects Login while a valid session cookie is
				// attached, so terminate the session first.
				admin := reloginAdminSession(ctx, c.URL())
				terminateSession(ctx, sm, admin)
				spy.reset()

				gen := keeper.soapGeneration()
				Expect(keeper.reloginSOAP(ctx, gen, "test")).To(Succeed())
				Expect(spy.count("Login")).To(Equal(1))

				// Another goroutine already refreshed: no second login.
				Expect(keeper.reloginSOAP(ctx, gen, "test")).To(Succeed())
				Expect(spy.count("Login")).To(Equal(1))
			})
		})

		It("produces one login for N concurrent faults", func() {
			const goroutines = 20

			simulator.Test(func(ctx context.Context, c *vim25.Client) {
				spy := recordSOAP(simulator.Map(ctx))
				sm := session.NewManager(c)
				keeper := newSessionKeeper(ctx, sm, simulator.DefaultLogin)

				admin := reloginAdminSession(ctx, c.URL())
				terminateSession(ctx, sm, admin)
				spy.reset()

				gen := keeper.soapGeneration()

				var (
					wg   sync.WaitGroup
					errs = make([]error, goroutines)
				)
				for i := range goroutines {
					wg.Add(1)
					go func(i int) {
						defer wg.Done()
						errs[i] = keeper.reloginSOAP(ctx, gen, "test")
					}(i)
				}
				wg.Wait()

				for _, err := range errs {
					Expect(err).NotTo(HaveOccurred())
				}
				Expect(spy.count("Login")).To(Equal(1))
			})
		})

		It("propagates a login error without bumping the generation", func() {
			ctx := context.Background()
			model, server := startReloginSimServer(reloginSimUsername, reloginSimPassword)

			c, err := vim25.NewClient(ctx, soap.NewClient(server.URL, true))
			Expect(err).NotTo(HaveOccurred())

			spy := recordSOAP(model.Map())
			sm := session.NewManager(c)
			keeper := newSessionKeeper(ctx,
				sm,
				url.UserPassword(reloginSimUsername, reloginSimPassword))
			Expect(sm.Login(ctx, url.UserPassword(reloginSimUsername, reloginSimPassword))).To(Succeed())
			admin := reloginAdminSession(ctx, server.URL)
			terminateSession(ctx, sm, admin)
			spy.reset()

			keeper.userInfo = url.UserPassword(reloginSimUsername, reloginSimBadPass)
			gen := keeper.soapGeneration()

			Expect(keeper.reloginSOAP(ctx, gen, "test")).NotTo(Succeed())
			Expect(spy.count("Login")).To(Equal(1))
			Expect(keeper.soapGeneration()).To(Equal(gen))
		})

		It("soapKeepAlive tolerates a transient ping error", func() {
			ctx := context.Background()
			_, server := startReloginSimServer(reloginSimUsername, reloginSimPassword)
			c, err := vim25.NewClient(ctx, soap.NewClient(server.URL, true))
			Expect(err).NotTo(HaveOccurred())
			keeper := newSessionKeeper(ctx, session.NewManager(c), simulator.DefaultLogin)
			rt := newReloginSOAP(c.RoundTripper, keeper)

			// Exercise a real transport failure without synthesizing SOAP errors.
			server.Close()
			Expect(keeper.soapKeepAlive(rt)()).To(Succeed())
		})

		It("soapKeepAlive surfaces a persistent credential failure", func() {
			ctx := context.Background()
			model, server := startReloginSimServer(reloginSimUsername, reloginSimPassword)

			c, err := vim25.NewClient(ctx, soap.NewClient(server.URL, true))
			Expect(err).NotTo(HaveOccurred())

			spy := recordSOAP(model.Map())
			sm := session.NewManager(c)
			keeper := newSessionKeeper(ctx,
				sm,
				url.UserPassword(reloginSimUsername, reloginSimPassword))
			rt := newReloginSOAP(c.RoundTripper, keeper)
			c.RoundTripper = rt

			Expect(sm.Login(ctx, url.UserPassword(reloginSimUsername, reloginSimPassword))).To(Succeed())

			admin := reloginAdminSession(ctx, c.URL())
			terminateSession(ctx, sm, admin)
			spy.reset()

			keeper.userInfo = url.UserPassword(reloginSimUsername, reloginSimBadPass)

			// The ping faults with NotAuthenticated, the wrapper's
			// re-login fails with InvalidLogin, and the send func
			// surfaces the joined error so the keepalive handler
			// stops its goroutine, as the legacy handler does.
			err = keeper.soapKeepAlive(rt)()
			Expect(err).To(HaveOccurred())
			Expect(IsNotAuthenticatedError(err)).To(BeTrue())
			Expect(IsInvalidLogin(err)).To(BeTrue())
			Expect(spy.count("Login")).To(Equal(1))
		})
	})

	Describe("SOAP wrapper", func() {
		It("re-logs in and replays once on a terminated session", func() {
			simulator.Test(func(ctx context.Context, c *vim25.Client) {
				spy := recordSOAP(simulator.Map(ctx))
				sm := session.NewManager(c)
				keeper := newSessionKeeper(ctx, sm, simulator.DefaultLogin)
				c.RoundTripper = newReloginSOAP(c.RoundTripper, keeper)

				admin := reloginAdminSession(ctx, c.URL())
				terminateSession(ctx, sm, admin)
				spy.reset()

				_, err := methods.GetCurrentTime(ctx, c)
				Expect(err).NotTo(HaveOccurred())
				Expect(spy.recorded()).To(Equal([]string{
					"CurrentTime",
					"Login",
					"CurrentTime",
				}))
			})
		})

		It("returns the retried result, not the stale fault", func() {
			// This is the resetResponse regression: without zeroing the
			// response, the stale fault from the first attempt makes the
			// successful replay report NotAuthenticated.
			simulator.Test(func(ctx context.Context, c *vim25.Client) {
				spy := recordSOAP(simulator.Map(ctx))
				sm := session.NewManager(c)
				keeper := newSessionKeeper(ctx, sm, simulator.DefaultLogin)
				c.RoundTripper = newReloginSOAP(c.RoundTripper, keeper)

				admin := reloginAdminSession(ctx, c.URL())
				terminateSession(ctx, sm, admin)
				spy.reset()

				res, err := methods.GetCurrentTime(ctx, c)
				Expect(err).NotTo(HaveOccurred())
				Expect(res).NotTo(BeNil())
			})
		})

		It("does not recurse on a faulting login body", func() {
			ctx := context.Background()
			model, server := startReloginSimServer(reloginSimUsername, reloginSimPassword)

			c, err := vim25.NewClient(ctx, soap.NewClient(server.URL, true))
			Expect(err).NotTo(HaveOccurred())

			spy := recordSOAP(model.Map())
			sm := session.NewManager(c)
			keeper := newSessionKeeper(ctx,
				sm,
				url.UserPassword(reloginSimUsername, reloginSimPassword))
			c.RoundTripper = newReloginSOAP(c.RoundTripper, keeper)
			Expect(sm.Login(ctx, url.UserPassword(reloginSimUsername, reloginSimPassword))).To(Succeed())
			spy.reset()

			admin := reloginAdminSession(ctx, server.URL)
			terminateSession(ctx, sm, admin)
			spy.reset()

			keeper.userInfo = url.UserPassword(reloginSimUsername, reloginSimBadPass)

			_, err = methods.GetCurrentTime(ctx, c)
			Expect(err).To(HaveOccurred())

			// The original fault and the login failure are both visible.
			Expect(IsNotAuthenticatedError(err)).To(BeTrue())
			Expect(IsInvalidLogin(err)).To(BeTrue())

			// Exactly one login attempt, with no recursion and no replay.
			Expect(spy.recorded()).To(Equal([]string{
				"CurrentTime",
				"Login",
			}))
		})

		It("re-logs in but does not replay WaitForUpdatesEx", func() {
			simulator.Test(func(ctx context.Context, c *vim25.Client) {
				spy := recordSOAP(simulator.Map(ctx))
				sm := session.NewManager(c)
				keeper := newSessionKeeper(ctx, sm, simulator.DefaultLogin)
				c.RoundTripper = newReloginSOAP(c.RoundTripper, keeper)

				collector, err := property.DefaultCollector(c).Create(ctx)
				Expect(err).NotTo(HaveOccurred())

				admin := reloginAdminSession(ctx, c.URL())
				terminateSession(ctx, sm, admin)
				spy.reset()

				_, err = methods.WaitForUpdatesEx(ctx, c, &vimtypes.WaitForUpdatesEx{
					This: collector.Reference(),
				})
				Expect(err).To(HaveOccurred())
				Expect(IsNotAuthenticatedError(err)).To(BeTrue())

				// The login happened, but the fault is still returned and
				// there is no second WaitForUpdatesEx.
				Expect(spy.recorded()).To(Equal([]string{
					"WaitForUpdatesEx",
					"Login",
				}))
			})
		})

		It("re-logs in but does not replay DestroyPropertyFilter", func() {
			simulator.Test(func(ctx context.Context, c *vim25.Client) {
				spy := recordSOAP(simulator.Map(ctx))
				sm := session.NewManager(c)
				keeper := newSessionKeeper(ctx, sm, simulator.DefaultLogin)
				c.RoundTripper = newReloginSOAP(c.RoundTripper, keeper)

				// Create the property filter on the current session. CreateFilter
				// is deny-listed, but it runs on a live session here, so no fault
				// and no re-login are involved.
				propertyCollector := property.DefaultCollector(c)
				filter, cerr := propertyCollector.CreateFilter(ctx, vimtypes.CreateFilter{
					Spec: vimtypes.PropertyFilterSpec{
						PropSet: []vimtypes.PropertySpec{{
							Type:    "SessionManager",
							PathSet: []string{"currentSession"},
						}},
						ObjectSet: []vimtypes.ObjectSpec{{
							Obj: *c.ServiceContent.SessionManager,
						}},
					},
				})
				Expect(cerr).NotTo(HaveOccurred())

				admin := reloginAdminSession(ctx, c.URL())
				terminateSession(ctx, sm, admin)
				spy.reset()
				gen := keeper.soapGeneration()

				_, err := methods.DestroyPropertyFilter(ctx, c, &vimtypes.DestroyPropertyFilter{
					This: filter.Reference(),
				})
				Expect(err).To(HaveOccurred())
				Expect(IsNotAuthenticatedError(err)).To(BeTrue())

				// The destroy is re-login-only: the login happened, the
				// fault is still returned, and there is no second destroy.
				Expect(spy.recorded()).To(Equal([]string{
					"DestroyPropertyFilter",
					"Login",
				}))
				Expect(keeper.soapGeneration()).To(Equal(gen + 1))
			})
		})

		It("re-logs in but does not replay under WithNoReplay", func() {
			simulator.Test(func(ctx context.Context, c *vim25.Client) {
				spy := recordSOAP(simulator.Map(ctx))
				sm := session.NewManager(c)
				keeper := newSessionKeeper(ctx, sm, simulator.DefaultLogin)
				c.RoundTripper = newReloginSOAP(c.RoundTripper, keeper)

				admin := reloginAdminSession(ctx, c.URL())
				terminateSession(ctx, sm, admin)
				spy.reset()

				_, err := methods.GetCurrentTime(WithNoReplay(ctx), c)
				Expect(err).To(HaveOccurred())
				Expect(IsNotAuthenticatedError(err)).To(BeTrue())

				Expect(spy.recorded()).To(Equal([]string{
					"CurrentTime",
					"Login",
				}))
			})
		})

		It("produces one login for N concurrent replayed calls", func() {
			const goroutines = 20

			simulator.Test(func(ctx context.Context, c *vim25.Client) {
				spy := recordSOAP(simulator.Map(ctx))
				sm := session.NewManager(c)
				keeper := newSessionKeeper(ctx, sm, simulator.DefaultLogin)
				c.RoundTripper = newReloginSOAP(c.RoundTripper, keeper)

				admin := reloginAdminSession(ctx, c.URL())
				terminateSession(ctx, sm, admin)
				spy.reset()

				var (
					wg   sync.WaitGroup
					errs = make([]error, goroutines)
				)
				for i := range goroutines {
					wg.Add(1)
					go func(i int) {
						defer wg.Done()
						_, errs[i] = methods.GetCurrentTime(ctx, c)
					}(i)
				}
				wg.Wait()

				for _, err := range errs {
					Expect(err).NotTo(HaveOccurred())
				}
				Expect(spy.count("Login")).To(Equal(1))
			})
		})

		It("passes a NoPermission fault through untouched", func() {
			ctx := context.Background()
			model, server := startReloginSimServer(reloginSimUsername, reloginSimPassword)

			model.Service.AddFaultRule(&simulator.FaultInjectionRule{
				MethodName:  "CurrentTime",
				ObjectType:  "*",
				ObjectName:  "*",
				Probability: 1.0,
				FaultType:   simulator.FaultTypeNoPermission,
				Enabled:     true,
				MaxCount:    1,
			})

			c, err := vim25.NewClient(ctx, soap.NewClient(server.URL, true))
			Expect(err).NotTo(HaveOccurred())

			spy := recordSOAP(model.Map())
			keeper := newSessionKeeper(ctx, session.NewManager(c), simulator.DefaultLogin)
			c.RoundTripper = newReloginSOAP(c.RoundTripper, keeper)

			// The client is unauthenticated; log in so the injected fault is
			// what surfaces, not the no-session fault.
			Expect(session.NewManager(c).Login(
				ctx,
				url.UserPassword(reloginSimUsername, reloginSimPassword))).To(Succeed())
			spy.reset()

			_, err = methods.GetCurrentTime(ctx, c)
			Expect(err).To(HaveOccurred())
			Expect(fault.Is(err, &vimtypes.NoPermission{})).To(BeTrue())
			Expect(spy.recorded()).To(Equal([]string{"CurrentTime"}))
		})

		It("passes an InvalidArgument fault through untouched", func() {
			ctx := context.Background()
			model, server := startReloginSimServer(reloginSimUsername, reloginSimPassword)

			model.Service.AddFaultRule(&simulator.FaultInjectionRule{
				MethodName:  "CurrentTime",
				ObjectType:  "*",
				ObjectName:  "*",
				Probability: 1.0,
				FaultType:   simulator.FaultTypeInvalidArgument,
				Enabled:     true,
				MaxCount:    1,
			})

			c, err := vim25.NewClient(ctx, soap.NewClient(server.URL, true))
			Expect(err).NotTo(HaveOccurred())

			spy := recordSOAP(model.Map())
			keeper := newSessionKeeper(ctx, session.NewManager(c), simulator.DefaultLogin)
			c.RoundTripper = newReloginSOAP(c.RoundTripper, keeper)

			Expect(session.NewManager(c).Login(
				ctx,
				url.UserPassword(reloginSimUsername, reloginSimPassword))).To(Succeed())
			spy.reset()

			_, err = methods.GetCurrentTime(ctx, c)
			Expect(err).To(HaveOccurred())
			Expect(fault.Is(err, &vimtypes.InvalidArgument{})).To(BeTrue())
			Expect(spy.recorded()).To(Equal([]string{"CurrentTime"}))
		})

		It("passes a transport error through untouched", func() {
			ctx := context.Background()
			_, server := startReloginSimServer(reloginSimUsername, reloginSimPassword)

			c, err := vim25.NewClient(ctx, soap.NewClient(server.URL, true))
			Expect(err).NotTo(HaveOccurred())

			// The server cannot observe failed connections. Count HTTP attempts
			// through the same public transport observer used by REST tests.
			spy := &spyHTTPRT{inner: c.Transport}
			c.Transport = spy
			keeper := newSessionKeeper(ctx, session.NewManager(c), simulator.DefaultLogin)
			c.RoundTripper = newReloginSOAP(c.RoundTripper, keeper)

			server.Close()

			_, err = methods.GetCurrentTime(ctx, c)
			Expect(err).To(HaveOccurred())
			Expect(IsNotAuthenticatedError(err)).To(BeFalse())
			Expect(spy.countPath(c.URL().Path)).To(Equal(1))
		})
	})

	Describe("PBM", func() {
		It("recovers a PBM call on a terminated session", func() {
			model := simulator.VPX()
			Expect(model.Create()).To(Succeed())
			registry := pbmsim.New()
			pbmSpy := recordSOAP(registry)
			model.Service.RegisterSDK(registry)
			simulator.Test(func(ctx context.Context, c *vim25.Client) {
				spy := recordSOAP(simulator.Map(ctx))
				sm := session.NewManager(c)
				keeper := newSessionKeeper(ctx, sm, simulator.DefaultLogin)
				c.RoundTripper = newReloginSOAP(c.RoundTripper, keeper)

				pbmClient, err := pbm.NewClient(ctx, c)
				Expect(err).NotTo(HaveOccurred())

				pbmClient.RoundTripper = newReloginSOAP(pbmClient.Client, keeper)

				admin := reloginAdminSession(ctx, c.URL())
				terminateSession(ctx, sm, admin)
				spy.reset()
				pbmSpy.reset()

				_, err = pbmClient.QueryProfile(
					ctx,
					pbmtypes.PbmProfileResourceType{
						ResourceType: string(pbmtypes.PbmProfileResourceTypeEnumSTORAGE),
					},
					"")
				Expect(err).NotTo(HaveOccurred())

				// One fault, one login, one replay. The re-login goes
				// through the vim client's chain; the fault and the replay
				// go through the PBM chain.
				Expect(pbmSpy.count("PbmQueryProfile")).To(Equal(2))
				Expect(spy.count("Login")).To(Equal(1))
			}, model)
		})
	})

	Describe("REST", func() {
		// newRestChain installs the spy, the re-login wrapper and the
		// keepalive handler on the REST client. The wrapper is returned so
		// specs can drive it directly for the branches rest.Client cannot
		// produce.
		newRestChain := func(ctx context.Context, vc *vim25.Client) (
			*spyHTTPRT, *rest.Client, *sessionKeeper, *reloginREST) {

			restClient := rest.NewClient(vc)
			keeper := newSessionKeeper(ctx, session.NewManager(vc), simulator.DefaultLogin)
			keeper.setRestClient(restClient)

			spy := &spyHTTPRT{inner: restClient.Transport}
			wrapper := newReloginREST(spy, keeper)
			restClient.Transport = wrapper
			handler := keepalive.NewHandlerREST(
				restClient,
				keepAliveIdleTime,
				keeper.restKeepAlive)
			restClient.Transport = handler
			DeferCleanup(handler.Stop)

			Expect(restClient.Login(ctx, simulator.DefaultLogin)).To(Succeed())
			return spy, restClient, keeper, wrapper
		}

		It("re-logs in and replays a GET with a rewritten session header", func() {
			simulator.Test(func(ctx context.Context, vc *vim25.Client) {
				spy, restClient, _, _ := newRestChain(ctx, vc)
				manager := tags.NewManager(restClient)

				Expect(restClient.Logout(ctx)).To(Succeed())

				_, err := manager.ListCategories(ctx)
				Expect(err).NotTo(HaveOccurred())

				gets := []spyHTTPRequest{}
				for _, r := range spy.recorded() {
					if r.path == reloginSimCategoryPath {
						gets = append(gets, r)
					}
				}
				Expect(gets).To(HaveLen(2))

				// The replay carried the new session id, not the old one.
				sid := restClient.SessionID()
				Expect(sid).NotTo(BeEmpty())
				Expect(gets[0].session).ToNot(Equal(sid))
				Expect(gets[1].session).To(Equal(sid))
			})
		})

		It("reuses the renewed session for requests stamped before re-login", func() {
			simulator.Test(func(ctx context.Context, vc *vim25.Client) {
				spy, restClient, _, wrapper := newRestChain(ctx, vc)
				defer func() { Expect(restClient.Logout(ctx)).To(Succeed()) }()

				// Model calls paused after Do stamps their headers but before
				// they enter the re-login transport and read its generation.
				const callers = 20
				requests := make([]*http.Request, callers)
				oldSession := restClient.SessionID()
				for i := range requests {
					requests[i] = restClient.Resource("/com/vmware/cis/tagging/category").
						Request(http.MethodGet)
					requests[i].Header.Set(restSessionHeader, oldSession)
				}
				Expect(restClient.Logout(ctx)).To(Succeed())

				// Another call finishes recovery before the delayed calls enter.
				_, err := tags.NewManager(restClient).ListCategories(ctx)
				Expect(err).NotTo(HaveOccurred())
				newSession := restClient.SessionID()
				Expect(newSession).NotTo(Equal(oldSession))

				results := make(chan error, callers)
				for _, req := range requests {
					go func() {
						res, err := wrapper.RoundTrip(req.WithContext(ctx))
						if err == nil {
							_ = res.Body.Close()
							if res.StatusCode != http.StatusOK {
								err = fmt.Errorf("unexpected status: %s", res.Status)
							}
						}
						results <- err
					}()
				}
				for range callers {
					Eventually(results).Should(Receive(BeNil()))
				}

				logins := 0
				for _, req := range spy.recorded() {
					if req.path == restSessionPath && req.method == http.MethodPost {
						logins++
					}
				}
				Expect(logins).To(Equal(2), "initial login plus one recovery")
				Expect(restClient.SessionID()).To(Equal(newSession))
				for _, req := range requests {
					Expect(req.Header.Get(restSessionHeader)).To(Equal(oldSession))
				}
			})
		})

		It("shares one REST login across simultaneous authentication failures", func() {
			simulator.Test(func(ctx context.Context, vc *vim25.Client) {
				spy, restClient, _, _ := newRestChain(ctx, vc)
				defer func() { Expect(restClient.Logout(ctx)).To(Succeed()) }()
				Expect(restClient.Logout(ctx)).To(Succeed())
				oldSession := restClient.SessionID()
				initialLogins := spy.loginCount()

				const callers = 20
				arrived := make(chan struct{}, callers)
				release := make(chan struct{})
				requestCtx, cancel := context.WithCancel(ctx)
				var wg sync.WaitGroup
				defer func() {
					cancel()
					wg.Wait()
				}()
				inner := spy.inner
				spy.inner = reloginHTTPRT(func(req *http.Request) (*http.Response, error) {
					res, err := inner.RoundTrip(req)
					if err == nil && req.URL.Path == reloginSimCategoryPath &&
						res.StatusCode == http.StatusUnauthorized {
						// Every initial request has faulted before any can log in.
						arrived <- struct{}{}
						select {
						case <-release:
						case <-requestCtx.Done():
							_ = res.Body.Close()
							return nil, requestCtx.Err()
						}
					}
					return res, err
				})

				results := make(chan error, callers)
				for range callers {
					wg.Add(1)
					go func() {
						defer wg.Done()
						_, err := tags.NewManager(restClient).ListCategories(requestCtx)
						results <- err
					}()
				}
				for range callers {
					Eventually(arrived).Should(Receive())
				}
				Expect(spy.loginCount()).To(Equal(initialLogins))
				close(release)
				for range callers {
					Eventually(results).Should(Receive(BeNil()))
				}

				Expect(spy.loginCount()).To(Equal(initialLogins + 1))
				Expect(spy.countPath(reloginSimCategoryPath)).To(Equal(2 * callers))
				newSession := restClient.SessionID()
				Expect(newSession).NotTo(Equal(oldSession))
				requests := spy.recorded()
				var sessions []string
				for _, req := range requests {
					if req.path == reloginSimCategoryPath {
						sessions = append(sessions, req.session)
					}
				}
				Expect(sessions[:callers]).To(HaveEach(Equal(oldSession)))
				Expect(sessions[callers:]).To(HaveEach(Equal(newSession)))
			})
		})

		DescribeTable("shares REST recovery between keepalive and application traffic",
			func(keepaliveFirst bool) {
				simulator.Test(func(ctx context.Context, vc *vim25.Client) {
					spy, restClient, keeper, _ := newRestChain(ctx, vc)
					defer func() { Expect(restClient.Logout(ctx)).To(Succeed()) }()
					Expect(restClient.Logout(ctx)).To(Succeed())
					initialLogins := spy.loginCount()
					oldSession := restClient.SessionID()

					arrived := make(chan struct{}, 2)
					releaseApp := make(chan struct{})
					releaseProbe := make(chan struct{})
					requestCtx, cancel := context.WithCancel(ctx)
					var wg sync.WaitGroup
					defer func() {
						cancel()
						wg.Wait()
					}()
					inner := spy.inner
					spy.inner = reloginHTTPRT(func(req *http.Request) (*http.Response, error) {
						res, err := inner.RoundTrip(req)
						if err != nil || res.StatusCode != http.StatusUnauthorized {
							return res, err
						}
						var release <-chan struct{}
						switch {
						case req.URL.Path == reloginSimCategoryPath:
							release = releaseApp
						case req.URL.Path == restSessionPath && req.URL.Query().Get("~action") == "get":
							release = releaseProbe
						default:
							return res, nil
						}
						arrived <- struct{}{}
						select {
						case <-release:
							return res, nil
						case <-requestCtx.Done():
							_ = res.Body.Close()
							return nil, requestCtx.Err()
						}
					})

					appResult := make(chan error, 1)
					probeResult := make(chan error, 1)
					wg.Add(2)
					go func() {
						defer wg.Done()
						_, err := tags.NewManager(restClient).ListCategories(requestCtx)
						appResult <- err
					}()
					go func() {
						defer wg.Done()
						probeResult <- keeper.restKeepAlive()
					}()
					for range 2 {
						Eventually(arrived).Should(Receive())
					}
					Expect(spy.loginCount()).To(Equal(initialLogins))

					// Complete the selected recovery while the other caller
					// still holds a 401 from the same expired session.
					if keepaliveFirst {
						close(releaseProbe)
						Eventually(probeResult).Should(Receive(BeNil()))
						close(releaseApp)
						Eventually(appResult).Should(Receive(BeNil()))
					} else {
						close(releaseApp)
						Eventually(appResult).Should(Receive(BeNil()))
						close(releaseProbe)
						Eventually(probeResult).Should(Receive(BeNil()))
					}
					Expect(spy.loginCount()).To(Equal(initialLogins + 1))
					Expect(spy.countPath(reloginSimCategoryPath)).To(Equal(2))
					Expect(restClient.SessionID()).NotTo(Equal(oldSession))
					sess, err := restClient.Session(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(sess).NotTo(BeNil())
				})
			},
			Entry("when keepalive completes recovery first", true),
			Entry("when the application completes recovery first", false),
		)

		It("recovers again when the replacement REST session expires", func() {
			simulator.Test(func(ctx context.Context, vc *vim25.Client) {
				spy, restClient, _, _ := newRestChain(ctx, vc)
				defer func() { Expect(restClient.Logout(ctx)).To(Succeed()) }()
				initialLogins := spy.loginCount()
				for cycle := 1; cycle <= 2; cycle++ {
					oldSession := restClient.SessionID()
					Expect(restClient.Logout(ctx)).To(Succeed())
					_, err := tags.NewManager(restClient).ListCategories(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(restClient.SessionID()).NotTo(Equal(oldSession))
					Expect(spy.loginCount()).To(Equal(initialLogins + cycle))
					Expect(spy.countPath(reloginSimCategoryPath)).To(Equal(2 * cycle))
				}
			})
		})

		It("preserves the original REST fault when re-login is rejected", func() {
			model := simulator.VPX()
			Expect(model.Create()).To(Succeed())
			model.Service.HandleFunc(reloginFaultPath, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, "original session expired")
			})
			simulator.Test(func(ctx context.Context, vc *vim25.Client) {
				spy, restClient, keeper, wrapper := newRestChain(ctx, vc)
				defer func() { Expect(restClient.Logout(ctx)).To(Succeed()) }()
				// vcsim rejects empty credentials even in its default permissive mode.
				keeper.userInfo = url.UserPassword(reloginSimUsername, "")
				initialLogins := spy.loginCount()
				gen := keeper.restGeneration()
				oldSession := restClient.SessionID()

				req := restClient.Resource("/relogin-test").Request(http.MethodGet)
				req.Header.Set(restSessionHeader, oldSession)
				res, err := wrapper.RoundTrip(req.WithContext(ctx))
				Expect(err).NotTo(HaveOccurred())
				defer func() { Expect(res.Body.Close()).To(Succeed()) }()
				Expect(res.StatusCode).To(Equal(http.StatusUnauthorized))
				body, err := io.ReadAll(res.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(body)).To(Equal("original session expired"))
				Expect(spy.loginCount()).To(Equal(initialLogins + 1))
				Expect(spy.countPath(reloginFaultPath)).To(Equal(1))
				Expect(keeper.restGeneration()).To(Equal(gen))
				Expect(restClient.SessionID()).To(Equal(oldSession))
			}, model)
		})

		It("stops REST recovery when the replay also returns unauthorized", func() {
			model := simulator.VPX()
			Expect(model.Create()).To(Succeed())
			var attempts atomic.Int32
			model.Service.HandleFunc(reloginFaultPath, func(w http.ResponseWriter, _ *http.Request) {
				body := "original session expired"
				if attempts.Add(1) > 1 {
					body = "replacement session rejected"
				}
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, body)
			})
			simulator.Test(func(ctx context.Context, vc *vim25.Client) {
				spy, restClient, _, wrapper := newRestChain(ctx, vc)
				defer func() { Expect(restClient.Logout(ctx)).To(Succeed()) }()
				initialLogins := spy.loginCount()
				oldSession := restClient.SessionID()

				req := restClient.Resource("/relogin-test").Request(http.MethodGet)
				req.Header.Set(restSessionHeader, oldSession)
				res, err := wrapper.RoundTrip(req.WithContext(ctx))
				Expect(err).NotTo(HaveOccurred())
				defer func() { Expect(res.Body.Close()).To(Succeed()) }()
				Expect(res.StatusCode).To(Equal(http.StatusUnauthorized))
				body, err := io.ReadAll(res.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(body)).To(Equal("replacement session rejected"))
				Expect(spy.loginCount()).To(Equal(initialLogins + 1))
				Expect(spy.countPath(reloginFaultPath)).To(Equal(2))
				Expect(req.Header.Get(restSessionHeader)).To(Equal(oldSession))
			}, model)
		})

		DescribeTable("passes non-authentication REST failures through without recovery",
			func(status int) {
				model := simulator.VPX()
				Expect(model.Create()).To(Succeed())
				model.Service.HandleFunc(reloginFaultPath, func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(status)
					_, _ = io.WriteString(w, "original failure")
				})
				simulator.Test(func(ctx context.Context, vc *vim25.Client) {
					spy, restClient, keeper, wrapper := newRestChain(ctx, vc)
					defer func() { Expect(restClient.Logout(ctx)).To(Succeed()) }()
					initialLogins := spy.loginCount()
					gen := keeper.restGeneration()
					oldSession := restClient.SessionID()

					req := restClient.Resource("/relogin-test").Request(http.MethodGet)
					req.Header.Set(restSessionHeader, oldSession)
					res, err := wrapper.RoundTrip(req.WithContext(ctx))
					Expect(err).NotTo(HaveOccurred())
					defer func() { Expect(res.Body.Close()).To(Succeed()) }()
					Expect(res.StatusCode).To(Equal(status))
					body, err := io.ReadAll(res.Body)
					Expect(err).NotTo(HaveOccurred())
					Expect(string(body)).To(Equal("original failure"))
					Expect(spy.loginCount()).To(Equal(initialLogins))
					Expect(spy.countPath(reloginFaultPath)).To(Equal(1))
					Expect(keeper.restGeneration()).To(Equal(gen))
					Expect(restClient.SessionID()).To(Equal(oldSession))
				}, model)
			},
			Entry("forbidden", http.StatusForbidden),
			Entry("server error", http.StatusInternalServerError),
		)

		It("passes a REST transport failure through without recovery", func() {
			ctx := context.Background()
			password, _ := simulator.DefaultLogin.Password()
			_, server := startReloginSimServer(simulator.DefaultLogin.Username(), password)
			vc, err := vim25.NewClient(ctx, soap.NewClient(server.URL, true))
			Expect(err).NotTo(HaveOccurred())
			spy, restClient, keeper, wrapper := newRestChain(ctx, vc)
			initialLogins := spy.loginCount()
			gen := keeper.restGeneration()
			oldSession := restClient.SessionID()
			server.Close()

			req := restClient.Resource("/relogin-test").Request(http.MethodGet)
			req.Header.Set(restSessionHeader, oldSession)
			res, err := wrapper.RoundTrip(req.WithContext(ctx))
			var networkErr *net.OpError
			Expect(errors.As(err, &networkErr)).To(BeTrue())
			Expect(res).To(BeNil())
			Expect(spy.loginCount()).To(Equal(initialLogins))
			Expect(spy.countPath(reloginFaultPath)).To(Equal(1))
			Expect(keeper.restGeneration()).To(Equal(gen))
			Expect(restClient.SessionID()).To(Equal(oldSession))
		})

		It("re-logs in and replays a JSON POST via GetBody", func() {
			simulator.Test(func(ctx context.Context, vc *vim25.Client) {
				spy, restClient, _, _ := newRestChain(ctx, vc)
				manager := tags.NewManager(restClient)

				Expect(restClient.Logout(ctx)).To(Succeed())

				id, err := manager.CreateCategory(ctx, &tags.Category{
					Name: "relogin-test",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(id).NotTo(BeEmpty())

				posts := []spyHTTPRequest{}
				for _, r := range spy.recorded() {
					if r.method == http.MethodPost && r.path == reloginSimCategoryPath {
						posts = append(posts, r)
					}
				}
				Expect(posts).To(HaveLen(2))
				sid := restClient.SessionID()
				Expect(posts[0].session).ToNot(Equal(sid))
				Expect(posts[1].session).To(Equal(sid))
			})
		})

		It("replays a non-session DELETE", func() {
			simulator.Test(func(ctx context.Context, vc *vim25.Client) {
				spy, restClient, _, _ := newRestChain(ctx, vc)
				manager := tags.NewManager(restClient)

				Expect(restClient.Logout(ctx)).To(Succeed())

				// A DELETE to a nonexistent category: the replay happens
				// (the point of this spec), and the second attempt surfaces
				// the error the replayed request earns.
				err := manager.DeleteCategory(ctx, &tags.Category{ID: "does-not-exist"})
				Expect(err).To(HaveOccurred())

				deletes := []spyHTTPRequest{}
				for _, r := range spy.recorded() {
					if r.method == http.MethodDelete &&
						strings.HasPrefix(r.path, reloginSimCategoryPath) {
						deletes = append(deletes, r)
					}
				}
				Expect(deletes).To(HaveLen(2))
				sid := restClient.SessionID()
				Expect(deletes[0].session).ToNot(Equal(sid))
				Expect(deletes[1].session).To(Equal(sid))
			})
		})

		It("does not replay a streaming body", func() {
			simulator.Test(func(ctx context.Context, vc *vim25.Client) {
				spy, restClient, _, wrapper := newRestChain(ctx, vc)

				Expect(restClient.Logout(ctx)).To(Succeed())

				// An upload-shaped request: an arbitrary stream with
				// GetBody == nil and ContentLength > 0, like soap.Client
				// Upload builds. It must be surfaced unretried and never
				// buffered.
				req, err := http.NewRequest(
					http.MethodPut,
					restClient.Resource("/com/vmware/cis/tagging/category/id:test").String(),
					io.NopCloser(strings.NewReader("body")))
				Expect(err).NotTo(HaveOccurred())
				req.ContentLength = 4

				// Match the authentication header normally stamped by rest.Client.
				req.Header.Set(restSessionHeader, restClient.SessionID())
				res, err := wrapper.RoundTrip(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(res.StatusCode).To(Equal(http.StatusUnauthorized))
				Expect(spy.countPath(reloginSimCategoryPath + "/id:test")).To(Equal(1))

				// The re-login still runs, so the session is healed for the
				// next caller rather than staying dead until the keepalive
				// ticks.
				sess, err := restClient.Session(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(sess).NotTo(BeNil())
			})
		})

		It("replays an action POST with an empty body", func() {
			simulator.Test(func(ctx context.Context, vc *vim25.Client) {
				spy, restClient, _, wrapper := newRestChain(ctx, vc)

				Expect(restClient.Logout(ctx)).To(Succeed())

				// An action-style POST carrying the empty io.MultiReader
				// body: ContentLength is 0, so replayBody treats it as
				// provably empty and the request is replayed.
				req := restClient.Resource("/com/vmware/cis/tagging/tag-association/id:test").
					WithAction("attach").
					Request(http.MethodPost)

				// Match the authentication header normally stamped by rest.Client.
				req.Header.Set(restSessionHeader, restClient.SessionID())
				res, err := wrapper.RoundTrip(req)
				Expect(err).NotTo(HaveOccurred())

				// The replayed request reaches the handler, which answers
				// 404 for the nonexistent tag.
				Expect(res.StatusCode).To(Equal(http.StatusNotFound))
				Expect(spy.countPath("/rest/com/vmware/cis/tagging/tag-association/id:test")).To(Equal(2))
			})
		})

		It("does not replay a bodiless POST", func() {
			simulator.Test(func(ctx context.Context, vc *vim25.Client) {
				spy, restClient, _, wrapper := newRestChain(ctx, vc)

				Expect(restClient.Logout(ctx)).To(Succeed())

				// A bodiless POST with no GetBody and a non-zero
				// ContentLength falls through rule 3, which applies only to
				// GET, HEAD, OPTIONS and DELETE, and into rule 4: it must be
				// surfaced unretried, never replayed.
				req, err := http.NewRequest(
					http.MethodPost,
					restClient.Resource("/com/vmware/cis/tagging/category/id:test").String(),
					io.NopCloser(strings.NewReader("body")))
				Expect(err).NotTo(HaveOccurred())
				req.ContentLength = 4

				// Match the authentication header normally stamped by rest.Client.
				req.Header.Set(restSessionHeader, restClient.SessionID())
				res, err := wrapper.RoundTrip(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(res.StatusCode).To(Equal(http.StatusUnauthorized))
				Expect(spy.countPath(reloginSimCategoryPath + "/id:test")).To(Equal(1))
			})
		})

		It("replays a bodiless GET with a zeroed ContentLength", func() {
			simulator.Test(func(ctx context.Context, vc *vim25.Client) {
				spy, restClient, _, wrapper := newRestChain(ctx, vc)

				Expect(restClient.Logout(ctx)).To(Succeed())

				// A GET whose body has no GetBody and a non-zero
				// ContentLength, like govmomi's vapi/rest builds for verbs
				// that carry the empty io.MultiReader body when a length is
				// set. Rule 3 replays it as http.NoBody, and the stale
				// ContentLength must be zeroed on the replay.
				req, err := http.NewRequest(
					http.MethodGet,
					restClient.Resource("/com/vmware/cis/tagging/category").String(),
					io.NopCloser(strings.NewReader("body")))
				Expect(err).NotTo(HaveOccurred())
				req.ContentLength = 4

				// Match the authentication header normally stamped by rest.Client.
				req.Header.Set(restSessionHeader, restClient.SessionID())
				res, err := wrapper.RoundTrip(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(res.StatusCode).To(Equal(http.StatusOK))

				gets := []spyHTTPRequest{}
				for _, r := range spy.recorded() {
					if r.method == http.MethodGet && r.path == reloginSimCategoryPath {
						gets = append(gets, r)
					}
				}
				Expect(gets).To(HaveLen(2))
				Expect(gets[0].contentLength).To(Equal(int64(4)))
				Expect(gets[1].contentLength).To(Equal(int64(0)))
			})
		})

		It("never retries the session path", func() {
			simulator.Test(func(ctx context.Context, vc *vim25.Client) {
				spy, restClient, _, _ := newRestChain(ctx, vc)

				Expect(restClient.Logout(ctx)).To(Succeed())

				// The session probe swallows the 401 and returns
				// (nil, nil); the wrapper must not retry it.
				s, err := restClient.Session(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(s).To(BeNil())

				// One login, one logout, one probe -- no second probe and
				// no additional login.
				Expect(spy.countPath(restSessionPath)).To(Equal(3))
			})
		})

		It("restKeepAlive heals a dead session with no application traffic", func() {
			simulator.Test(func(ctx context.Context, vc *vim25.Client) {
				_, restClient, keeper, _ := newRestChain(ctx, vc)

				Expect(restClient.Logout(ctx)).To(Succeed())

				Expect(keeper.restKeepAlive()).To(Succeed())

				s, err := restClient.Session(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(s).NotTo(BeNil())
			})
		})
	})

	Describe("Keepalive", func() {
		It("survives a session termination and heals the session with no application traffic", func() {
			var (
				sessionIdleTimeout = time.Second
				keepAliveIdle      = 250 * time.Millisecond
			)

			simulator.Test(func(ctx context.Context, c *vim25.Client) {
				Expect(sim25.SetSessionTimeout(ctx, c, sessionIdleTimeout)).To(Succeed())

				spy := recordSOAP(simulator.Map(ctx))
				sm := session.NewManager(c)
				keeper := newSessionKeeper(ctx, sm, simulator.DefaultLogin)
				c.RoundTripper = keepalive.NewHandlerSOAP(newReloginSOAP(c.RoundTripper, keeper), keepAliveIdle, nil)

				// vcsim rejects Login while a valid session cookie is
				// attached: terminate the pre-existing session first, then
				// log in to start the handler.
				admin := reloginAdminSession(ctx, c.URL())
				terminateSession(ctx, sm, admin)
				Expect(sm.Login(ctx, simulator.DefaultLogin)).To(Succeed())

				terminateSession(ctx, sm, admin)
				spy.reset()

				// No application traffic from here on: the keepalive ping
				// travels through the re-login wrapper and heals the
				// session on its own.
				Eventually(func() []string {
					// Assert on the first three bodies only once they are
					// all recorded, so the poll never observes the window
					// between the login and the replayed ping.
					rec := spy.recorded()
					if len(rec) < 3 {
						return rec
					}
					return rec[:3]
				}, 10*time.Second, 100*time.Millisecond).Should(Equal([]string{
					"CurrentTime",
					"Login",
					"CurrentTime",
				}))
				Expect(spy.count("Login")).To(Equal(1))

				// The session is valid again.
				sess, err := sm.UserSession(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(sess).NotTo(BeNil())
			})
		})
	})

	Describe("NewClient", func() {
		// newAssembledClient assembles the full client against a simulator
		// that only accepts the given credentials.
		newAssembledClient := func(ctx context.Context, inline bool) *Client {
			model, server := startReloginSimServer(reloginSimUsername, reloginSimPassword)
			datacenter := model.Map().Any("Datacenter").Reference().Value
			caFile, err := server.CertificateFile()
			Expect(err).NotTo(HaveOccurred())

			c, err := NewClient(ctx, Config{
				Host:                 server.URL.Hostname(),
				Port:                 server.URL.Port(),
				Username:             reloginSimUsername,
				Password:             reloginSimPassword,
				CAFilePath:           caFile,
				Insecure:             false,
				Datacenter:           datacenter,
				InlineReloginEnabled: inline,
			})
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			DeferCleanup(c.Logout, ctx)
			return c
		}

		It("with the flag on recovers a terminated session inline", func() {
			ctx := context.Background()
			c := newAssembledClient(ctx, true)

			vc := c.VimClient()
			sm := session.NewManager(vc)
			admin := reloginAdminSession(ctx, vc.URL())
			terminateSession(ctx, sm, admin)

			_, err := methods.GetCurrentTime(ctx, vc)
			Expect(err).NotTo(HaveOccurred())
		})

		DescribeTable("derived PBM clients honor inline re-login",
			func(inline bool) {
				ctx := context.Background()
				c := newAssembledClient(ctx, inline)
				vc := c.VimClient()
				pbmClient, err := NewPbmClient(ctx, vc)
				Expect(err).NotTo(HaveOccurred())
				admin := reloginAdminSession(ctx, vc.URL())
				terminateSession(ctx, session.NewManager(vc), admin)

				_, err = pbmClient.QueryProfile(ctx, pbmtypes.PbmProfileResourceType{
					ResourceType: string(pbmtypes.PbmProfileResourceTypeEnumSTORAGE),
				}, "")
				if inline {
					Expect(err).NotTo(HaveOccurred())
				} else {
					Expect(IsNotAuthenticatedError(err)).To(BeTrue())
				}
			},
			Entry("enabled", true),
			Entry("disabled", false),
		)

		It("with the flag on recovers a dead REST session inline", func() {
			ctx := context.Background()
			c := newAssembledClient(ctx, true)

			Expect(c.RestClient().Logout(ctx)).To(Succeed())

			_, err := tags.NewManager(c.RestClient()).ListCategories(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("with the flag off does not recover inline", func() {
			ctx := context.Background()
			c := newAssembledClient(ctx, false)

			vc := c.VimClient()
			sm := session.NewManager(vc)
			admin := reloginAdminSession(ctx, vc.URL())
			terminateSession(ctx, sm, admin)

			_, err := methods.GetCurrentTime(ctx, vc)
			Expect(err).To(HaveOccurred())
			Expect(IsNotAuthenticatedError(err)).To(BeTrue())
		})
	})
})
