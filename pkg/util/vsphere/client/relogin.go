// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"

	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
)

// noReplayContextKey is the context key used by WithNoReplay. It is a struct
// type so only this package can populate it.
type noReplayContextKey struct{}

// WithNoReplay returns a context that suppresses request replay after an
// inline re-login. Re-login still happens; the original fault is still
// returned. Use it for callers that own session-scoped server state --
// property collectors, property filters, container and list views -- which
// a re-login destroys.
func WithNoReplay(ctx context.Context) context.Context {
	return context.WithValue(ctx, noReplayContextKey{}, struct{}{})
}

// isNoReplay reports whether ctx was marked with WithNoReplay.
func isNoReplay(ctx context.Context) bool {
	_, ok := ctx.Value(noReplayContextKey{}).(struct{})
	return ok
}

// sessionKeeper owns the re-authentication state shared by the SOAP, PBM and
// REST inline re-login round trippers. It serializes logins so that N
// goroutines faulting on the same dead session produce one login and one new
// vCenter session, and hands each wrapper a generation counter that lets a
// late arrival detect that someone else already re-authenticated.
//
// SOAP/vim25 and REST have independent generations because they are separate
// sessions with separate lifetimes.
type sessionKeeper struct {
	sm       *session.Manager
	userInfo *url.Userinfo

	// log is captured at construction so the keepalive send funcs, which
	// run on a background context with no logger of their own, still log
	// through the manager's logger rather than the global default.
	log logr.Logger

	// rest is set after the REST client is built. Read it with restClient,
	// never directly: muREST is what makes attaching it safe.
	rest *rest.Client

	muSOAP  sync.Mutex
	genSOAP atomic.Uint64
	muREST  sync.Mutex
	genREST atomic.Uint64
}

// newSessionKeeper returns a session keeper for the given session manager and
// login credentials. The REST client is attached later, with setRestClient.
// The logger is taken from ctx once, here, because the keepalive paths below
// have no request context to carry one.
func newSessionKeeper(
	ctx context.Context,
	sm *session.Manager,
	userInfo *url.Userinfo) *sessionKeeper {

	return &sessionKeeper{
		sm:       sm,
		userInfo: userInfo,
		log:      pkglog.FromContextOrDefault(ctx),
	}
}

// setRestClient attaches the REST client used by the REST re-login paths.
func (k *sessionKeeper) setRestClient(c *rest.Client) {
	k.muREST.Lock()
	defer k.muREST.Unlock()
	k.rest = c
}

// restClient returns the attached REST client. Reads go through muREST, the
// same lock setRestClient writes under, so attaching or replacing the client
// after the keeper is live stays safe. Callers already holding muREST use the
// field directly.
func (k *sessionKeeper) restClient() *rest.Client {
	k.muREST.Lock()
	defer k.muREST.Unlock()
	return k.rest
}

// soapGeneration returns the current SOAP session generation. Callers read it
// before their first attempt so that a login landing while the call was in
// flight is still observed.
func (k *sessionKeeper) soapGeneration() uint64 {
	return k.genSOAP.Load()
}

// restGeneration returns the current REST session generation. Callers read it
// before their first attempt so that a login landing while the call was in
// flight is still observed.
func (k *sessionKeeper) restGeneration() uint64 {
	return k.genREST.Load()
}

// reloginSOAP re-authenticates the vim25 session unless another goroutine
// already did so since gen was read. It returns nil without logging in when
// the generation has moved, and joins nothing: the caller is responsible for
// combining this error with the original fault.
func (k *sessionKeeper) reloginSOAP(
	ctx context.Context,
	gen uint64,
	trigger string) error {

	k.muSOAP.Lock()
	defer k.muSOAP.Unlock()

	// Another goroutine already refreshed while we were faulting.
	if k.genSOAP.Load() != gen {
		return nil
	}

	log := pkglog.FromContextOrDefault(ctx).WithName("vcSessionRelogin")
	log.Info("Re-authenticating vim client",
		"generation", gen, "method", trigger)

	if err := k.sm.Login(ctx, k.userInfo); err != nil {
		return err
	}
	k.genSOAP.Add(1)
	return nil
}

// reloginREST re-authenticates the REST session unless another goroutine
// already did so since gen was read or the failed request's sessionID was
// stamped. The header can predate the generation snapshot in the transport.
func (k *sessionKeeper) reloginREST(
	ctx context.Context,
	gen uint64,
	sessionID string,
	trigger string) error {

	k.muREST.Lock()
	defer k.muREST.Unlock()

	// Another goroutine already refreshed while we were faulting.
	if k.genREST.Load() != gen || k.rest.SessionID() != sessionID {
		return nil
	}

	log := pkglog.FromContextOrDefault(ctx).WithName("vcSessionRelogin")
	log.Info("Re-authenticating REST client",
		"generation", gen, "path", trigger)

	if err := k.rest.Login(ctx, k.userInfo); err != nil {
		return err
	}
	k.genREST.Add(1)
	return nil
}

// keeperRoundTripper is the outermost link of the inline re-login chain. It
// forwards every request untouched; its only job is to make the session
// keeper reachable from the *vim25.Client it is installed on, so a derived
// client -- PBM, say -- can find the keeper of the session it shares without
// the owner having to hand it down through every intervening call.
type keeperRoundTripper struct {
	soap.RoundTripper
	keeper *sessionKeeper
}

// keeperFromVimClient returns the session keeper driving the given vim25
// client's session, or nil when that client is in legacy keepalive mode.
// The answer comes from the client itself, so it cannot go stale or name the
// keeper of some other session.
func keeperFromVimClient(c *vim25.Client) *sessionKeeper {
	if c == nil {
		return nil
	}
	if rt, ok := c.RoundTripper.(*keeperRoundTripper); ok {
		return rt.keeper
	}
	return nil
}

// soapKeepAlive returns the send func for the SOAP keepalive handler in
// inline mode. It pings the session through the re-login wrapper rt, so a
// dead session heals with no application traffic.
//
// Only a persistent credential failure returns an error: govmomi's keepalive
// handler stops its goroutine permanently the first time send returns an
// error, and a transport hiccup or a transient re-login failure is not worth
// killing the ticker over. This mirrors the legacy SoapKeepAliveHandlerFn,
// which tolerates non-auth errors and fails only on an invalid login.
func (k *sessionKeeper) soapKeepAlive(rt soap.RoundTripper) func() error {
	return func() error {
		ctx := k.backgroundContext()
		_, err := methods.GetCurrentTime(ctx, rt)
		if err == nil {
			return nil
		}
		if IsNotAuthenticatedError(err) && IsInvalidLogin(err) {
			// The re-login inside the wrapper failed with invalid
			// credentials. Let the handler stop its goroutine, as the
			// legacy SoapKeepAliveHandlerFn does.
			return err
		}
		k.log.WithName("vcSessionRelogin").
			Error(err, "Error in vim25 client's keepalive handler")
		return nil
	}
}

// restKeepAlive is the send func for the REST keepalive handler in inline
// mode. It probes the session and re-authenticates when it is gone.
//
// Only a persistent credential failure returns an error, mirroring
// soapKeepAlive above: govmomi's keepalive handler stops its goroutine
// permanently the first time send returns an error, and neither a transport
// hiccup nor a transient re-login failure -- vCenter answering but not yet
// fully up after a vpxd restart, say -- is worth killing the ticker over. A
// 401 on the login itself is the REST equivalent of an invalid login: the
// credentials are wrong, not the moment.
func (k *sessionKeeper) restKeepAlive() error {
	ctx := k.backgroundContext()
	gen := k.restGeneration()
	sessionID := k.restClient().SessionID()

	s, err := k.restClient().Session(ctx)
	if err != nil {
		// Transport hiccup; do not kill the ticker.
		return nil //nolint:nilerr // Keepalive retries transient transport errors on its next tick.
	}
	if s != nil {
		return nil
	}

	if err := k.reloginREST(ctx, gen, sessionID, "keepalive"); err != nil {
		if rest.IsStatusError(err, http.StatusUnauthorized) {
			// Invalid credentials. Let the handler stop its goroutine, as
			// the legacy RestKeepAliveHandlerFn does.
			return err
		}
		k.log.WithName("vcSessionRelogin").
			Error(err, "Error in rest client's keepalive handler")
		return nil
	}
	return nil
}

// backgroundContext returns the context the keepalive paths run on. The
// keepalive ticker has no caller context, so the logger captured at
// construction is attached here and the re-login helpers pick it up through
// pkglog.FromContextOrDefault just as they do on an application call.
func (k *sessionKeeper) backgroundContext() context.Context {
	return logr.NewContext(context.Background(), k.log)
}
