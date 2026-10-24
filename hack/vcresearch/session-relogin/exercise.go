// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/object"
	pbmtypes "github.com/vmware/govmomi/pbm/types"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/session"
	ssomethods "github.com/vmware/govmomi/ssoadmin/methods"
	ssotypes "github.com/vmware/govmomi/ssoadmin/types"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vsclient "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/client"
)

// Observers count actual login attempts without replacing recovery behavior.
type soapObserver struct {
	soap.RoundTripper
	logins atomic.Int64
}

func (s *soapObserver) RoundTrip(ctx context.Context, req, res soap.HasFault) error {
	if _, ok := req.(*methods.LoginBody); ok {
		s.logins.Add(1)
	}
	return s.RoundTripper.RoundTrip(ctx, req, res)
}

type restObserver struct {
	http.RoundTripper
	logins atomic.Int64
}

func (r *restObserver) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodPost && req.URL.Path == "/rest/com/vmware/cis/session" && req.URL.Query().Get("~action") == "" {
		r.logins.Add(1)
	}
	return r.RoundTripper.RoundTrip(req)
}

type candidate struct {
	*vsclient.Client
	soap *soapObserver
	rest *restObserver
}

type exercise struct {
	*testbed
	name, principal string
	clients         []*candidate
	secrets         []string
}

func (e *exercise) redact(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	for _, secret := range e.secrets {
		if secret != "" {
			s = strings.ReplaceAll(s, secret, "[REDACTED]")
		}
	}
	return s
}

func (e *exercise) step(name string, fn func() error) error {
	start := time.Now()
	// Reporting failures must not prevent the cleanup step from running.
	startErr := writeReport(map[string]any{"starting": name})
	err := fn()
	reportErr := writeReport(map[string]any{
		"case": name, "passed": err == nil, "duration_ms": time.Since(start).Milliseconds(), "error": e.redact(err),
	})
	if err != nil {
		return errors.Join(fmt.Errorf("%s: %s", name, e.redact(err)), startErr, reportErr)
	}
	return errors.Join(startErr, reportErr)
}

func writeReport(report any) error {
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		return fmt.Errorf("write JSON report: %w", err)
	}
	return nil
}

func randomString() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func randomPassword() string {
	// This testbed's policy caps passwords at 20 characters.
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return "Qz!7" + base64.RawURLEncoding.EncodeToString(b[:])
}

func run(ctx context.Context, path, journal string) (retErr error) {
	if journal == "" {
		return fmt.Errorf("-journal is required with -exercise")
	}
	b, err := connect(ctx, path)
	if err != nil {
		return err
	}
	defer b.logout()
	e := &exercise{testbed: b, name: "vmop-relogin-" + randomString()[:12]}
	e.principal = e.name + "@" + b.sso.Domain
	p0, p1 := randomPassword(), randomPassword()
	e.secrets = []string{p0, p1, b.credentials.Password}
	state := map[string]any{"principal": e.principal, "host": b.credentials.Host, "cleaned_up": false}
	f, err := os.OpenFile(journal, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600) //nolint:gosec // G304: The CLI operator explicitly chooses the cleanup journal path.
	if err != nil {
		return fmt.Errorf("create cleanup journal: %w", err)
	}
	err = json.NewEncoder(f).Encode(state)
	err = errors.Join(err, f.Close())
	if err != nil {
		return fmt.Errorf("write cleanup journal: %w", err)
	}
	if err := writeReport(map[string]any{
		"version": b.vc.ServiceContent.About.Version, "build": b.vc.ServiceContent.About.Build,
		"principal": e.principal, "lockout_policy": b.policy,
	}); err != nil {
		return err
	}
	// Register cleanup before creation, including an ambiguous create response.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(logr.NewContext(context.Background(), logr.Discard()), 2*time.Minute)
		defer cancel()
		cleanupErr := e.step("cleanup", func() error { return e.cleanup(cleanupCtx) })
		state["cleaned_up"] = cleanupErr == nil
		data, marshalErr := json.MarshalIndent(state, "", "  ")
		writeErr := os.WriteFile(journal, data, 0600)
		retErr = errors.Join(retErr, cleanupErr, marshalErr, writeErr)
	}()
	if err := e.step("create read-only test account", func() error {
		if err := b.sso.CreatePersonUser(ctx, e.name, ssotypes.AdminPersonDetails{
			FirstName: "VMOperator", LastName: "Relogin", Description: "Disposable session recovery validation account",
		}, p0); err != nil {
			return err
		}
		am := object.NewAuthorizationManager(b.vc.Client)
		roles, err := am.RoleList(ctx)
		if err != nil {
			return err
		}
		role := roles.ByName("ReadOnly")
		if role == nil {
			return fmt.Errorf("ReadOnly role not found")
		}
		return am.SetEntityPermissions(ctx, b.vc.ServiceContent.RootFolder, []vimtypes.Permission{
			{Principal: e.principal, RoleId: role.RoleId, Propagate: true},
		})
	}); err != nil {
		return err
	}
	var c *candidate
	if err := e.step("production client baseline SOAP REST PBM", func() error {
		var err error
		c, err = e.newClient(ctx, p0, true)
		if err != nil {
			return err
		}
		return baseline(ctx, c)
	}); err != nil {
		return err
	}
	for _, tc := range []struct {
		name string
		soap bool
		call func(context.Context) error
	}{
		{"SOAP terminated session", true, c.callSOAP},
		{"SOAP CurrentTime keepalive request after termination", true, func(ctx context.Context) error {
			_, err := methods.GetCurrentTime(ctx, c.VimClient())
			return err
		}},
		{"PBM terminated SOAP session", true, c.callPBM},
		{"REST expired session", false, c.callREST},
		{"REST replacement session expires again", false, c.callREST},
		{"20 concurrent SOAP callers", true, concurrent(c.callSOAP)},
		{"20 concurrent REST callers", false, concurrent(c.callREST)},
	} {
		if err := e.step(tc.name, func() error {
			return e.recover(ctx, c, tc.soap, tc.call)
		}); err != nil {
			return err
		}
	}
	if err := e.step("WithNoReplay recovers without hiding original fault", func() error {
		return e.recover(ctx, c, true, func(ctx context.Context) error {
			if err := c.callSOAP(vsclient.WithNoReplay(ctx)); !vsclient.IsNotAuthenticatedError(err) {
				return fmt.Errorf("expected NotAuthenticated, got %w", err)
			}
			return c.callSOAP(ctx)
		})
	}); err != nil {
		return err
	}
	if err := e.step("WaitForUpdatesEx returns fault for expired collector", func() error {
		pc, err := property.DefaultCollector(c.VimClient()).Create(ctx)
		if err != nil {
			return err
		}
		// Session termination destroys this collector on the server.
		return e.recover(ctx, c, true, func(ctx context.Context) error {
			zero := int32(0)
			_, err := methods.WaitForUpdatesEx(ctx, c.VimClient(), &vimtypes.WaitForUpdatesEx{
				This: pc.Reference(), Options: &vimtypes.WaitOptions{MaxWaitSeconds: &zero},
			})
			if !vsclient.IsNotAuthenticatedError(err) {
				return fmt.Errorf("expected NotAuthenticated, got %w", err)
			}
			return c.callSOAP(ctx)
		})
	}); err != nil {
		return err
	}
	return e.exerciseCredentials(ctx, c, p1)
}

func (e *exercise) exerciseCredentials(ctx context.Context, c *candidate, password string) error {
	if err := e.step("password rotation rejects cached credentials once per call", func() error {
		if err := e.sso.ResetPersonPassword(ctx, e.name, password); err != nil {
			return err
		}
		return e.reject(ctx, c)
	}); err != nil {
		return err
	}
	if err := e.step("rebuilt client uses rotated password", func() error {
		if err := e.unlock(ctx); err != nil {
			return err
		}
		var err error
		c, err = e.newClient(ctx, password, true)
		if err != nil {
			return err
		}
		return baseline(ctx, c)
	}); err != nil {
		return err
	}
	if err := e.step("disabled account rejects recovery", func() error {
		_, err := ssomethods.DisableUserAccount(ctx, e.sso, &ssotypes.DisableUserAccount{
			This: e.sso.ServiceContent.PrincipalManagementService, UserId: e.id(),
		})
		if err != nil {
			return err
		}
		u, err := e.sso.FindPersonUser(ctx, e.principal)
		if err != nil || u == nil || !u.Disabled {
			return fmt.Errorf("account not confirmed disabled: %w", err)
		}
		return e.reject(ctx, c)
	}); err != nil {
		return err
	}
	if err := e.step("re-enabled account recovers with same client", func() error {
		_, err := ssomethods.EnableUserAccount(ctx, e.sso, &ssotypes.EnableUserAccount{
			This: e.sso.ServiceContent.PrincipalManagementService, UserId: e.id(),
		})
		if err != nil {
			return err
		}
		if err := e.unlock(ctx); err != nil {
			return err
		}
		return baseline(ctx, c)
	}); err != nil {
		return err
	}
	if err := e.step("lockout rejects correct cached credentials", func() error {
		if err := e.lock(ctx); err != nil {
			return err
		}
		return e.reject(ctx, c)
	}); err != nil {
		return err
	}
	if err := e.step("unlocked account recovers with same client", func() error {
		if err := e.unlock(ctx); err != nil {
			return err
		}
		return baseline(ctx, c)
	}); err != nil {
		return err
	}
	return e.step("legacy mode returns auth faults without inline login", func() error {
		legacy, err := e.newClient(ctx, password, false)
		if err != nil {
			return err
		}
		if err := baseline(ctx, legacy); err != nil {
			return err
		}
		if err := e.expireSOAP(ctx, legacy); err != nil {
			return err
		}
		if err := e.expireREST(ctx, legacy); err != nil {
			return err
		}
		if err := legacy.callSOAP(ctx); !vsclient.IsNotAuthenticatedError(err) {
			return fmt.Errorf("expected SOAP NotAuthenticated, got %w", err)
		}
		if err := legacy.callREST(ctx); !rest.IsStatusError(err, http.StatusUnauthorized) {
			return fmt.Errorf("expected REST 401, got %w", err)
		}
		return checkCounts(legacy, 0, 0)
	})
}

func (e *exercise) newClient(ctx context.Context, password string, inline bool) (*candidate, error) {
	c, err := vsclient.NewClient(ctx, vsclient.Config{
		Host: e.credentials.Host, Port: "443", Username: e.principal, Password: password,
		Insecure: true, Datacenter: e.datacenter, InlineReloginEnabled: inline,
	})
	if err != nil {
		return nil, err
	}
	r := &candidate{Client: c, soap: &soapObserver{RoundTripper: c.VimClient().RoundTripper},
		rest: &restObserver{RoundTripper: c.RestClient().Transport}}
	// PBM was assembled before observation, retaining its shared keeper.
	c.VimClient().RoundTripper = r.soap
	c.RestClient().Transport = r.rest
	e.clients = append(e.clients, r)
	return r, nil
}

func (c *candidate) callSOAP(ctx context.Context) error {
	// A container view requires System.View and changes no inventory.
	v, err := view.NewManager(c.VimClient()).CreateContainerView(ctx,
		c.VimClient().ServiceContent.RootFolder, []string{"VirtualMachine"}, true)
	if err != nil {
		return err
	}
	return v.Destroy(ctx)
}

func (c *candidate) callREST(ctx context.Context) error {
	_, err := tags.NewManager(c.RestClient()).ListCategories(ctx)
	return err
}

func (c *candidate) callPBM(ctx context.Context) error {
	_, err := c.PbmClient().QueryProfile(ctx, pbmtypes.PbmProfileResourceType{ResourceType: "STORAGE"}, "")
	return err
}

func baseline(ctx context.Context, c *candidate) error {
	return errors.Join(c.callSOAP(ctx), c.callREST(ctx), c.callPBM(ctx))
}

func concurrent(call func(context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make([]error, 20)
		for i := range errs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				errs[i] = call(ctx)
			}()
		}
		close(start)
		wg.Wait()
		return errors.Join(errs...)
	}
}

func checkCounts(c *candidate, soapCount, restCount int64) error {
	if s, r := c.soap.logins.Load(), c.rest.logins.Load(); s != soapCount || r != restCount {
		return fmt.Errorf("login attempts SOAP=%d REST=%d, expected SOAP=%d REST=%d", s, r, soapCount, restCount)
	}
	return nil
}

func (e *exercise) recover(ctx context.Context, c *candidate, isSOAP bool, call func(context.Context) error) error {
	var err error
	if isSOAP {
		err = e.expireSOAP(ctx, c)
	} else {
		err = e.expireREST(ctx, c)
	}
	if err != nil {
		return err
	}
	s0, r0 := c.soap.logins.Load(), c.rest.logins.Load()
	s, r := s0, r0
	if isSOAP {
		s++
	} else {
		r++
	}
	start := time.Now()
	if err := call(ctx); err != nil {
		return err
	}
	if err := writeReport(map[string]any{
		"recovery_ms": time.Since(start).Milliseconds(),
		"soap_logins": c.soap.logins.Load() - s0, "rest_logins": c.rest.logins.Load() - r0,
	}); err != nil {
		return err
	}
	return checkCounts(c, s, r)
}

func (e *exercise) expireSOAP(ctx context.Context, c *candidate) error {
	// Query the victim through a raw session manager, bypassing recovery.
	raw := *c.VimClient()
	raw.RoundTripper = raw.Client
	s, err := session.NewManager(&raw).UserSession(ctx)
	if err != nil {
		return err
	}
	if s != nil {
		if !e.owns(s.UserName) {
			return fmt.Errorf("refusing to terminate session owned by %q (expected %q)", s.UserName, e.principal)
		}
		if err := session.NewManager(e.vc.Client).TerminateSession(ctx, []string{s.Key}); err != nil {
			return err
		}
	}
	// Real vCenter can acknowledge termination before it becomes observable.
	probeCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		current, err := session.NewManager(&raw).UserSession(probeCtx)
		if err != nil {
			return err
		}
		if current == nil {
			v, probeErr := view.NewManager(&raw).CreateContainerView(probeCtx, raw.ServiceContent.RootFolder, []string{"VirtualMachine"}, true)
			switch {
			case probeErr == nil:
				_ = v.Destroy(probeCtx)
			case vsclient.IsNotAuthenticatedError(probeErr):
				return writeReport(map[string]any{"observation": "SOAP termination confirmed by protected request"})
			default:
				return probeErr
			}
		}
		select {
		case <-probeCtx.Done():
			return fmt.Errorf("terminated SOAP session is still active: %w", probeCtx.Err())
		case <-ticker.C:
		}
	}
}

func (e *exercise) expireREST(ctx context.Context, c *candidate) error {
	r := rest.NewClient(e.vc.Client)
	r.SessionID(c.RestClient().SessionID())
	if err := r.Logout(ctx); err != nil && !rest.IsStatusError(err, http.StatusUnauthorized) {
		return err
	}
	return nil
}

func (e *exercise) reject(ctx context.Context, c *candidate) error {
	if err := e.expireSOAP(ctx, c); err != nil {
		return err
	}
	if err := e.expireREST(ctx, c); err != nil {
		return err
	}
	s, r := c.soap.logins.Load(), c.rest.logins.Load()
	if err := c.callSOAP(ctx); !vsclient.IsNotAuthenticatedError(err) || !vsclient.IsInvalidLogin(err) {
		return fmt.Errorf("expected original SOAP auth fault and InvalidLogin, got %w", err)
	}
	if err := c.callREST(ctx); !rest.IsStatusError(err, http.StatusUnauthorized) {
		return fmt.Errorf("expected REST 401, got %w", err)
	}
	return checkCounts(c, s+1, r+1)
}

func (e *exercise) id() ssotypes.PrincipalId {
	return ssotypes.PrincipalId{Name: e.name, Domain: e.sso.Domain}
}

func (e *exercise) owns(name string) bool {
	return strings.EqualFold(name, e.principal) || strings.EqualFold(name, e.sso.Domain+`\`+e.name)
}

func (e *exercise) unlock(ctx context.Context) error {
	_, err := ssomethods.UnlockUserAccount(ctx, e.sso, &ssotypes.UnlockUserAccount{
		This: e.sso.ServiceContent.PrincipalManagementService, UserId: e.id(),
	})
	if err != nil {
		return err
	}
	u, err := e.sso.FindPersonUser(ctx, e.principal)
	if err != nil || u == nil || u.Locked {
		return fmt.Errorf("account not confirmed unlocked: %w", err)
	}
	return nil
}

func (e *exercise) lock(ctx context.Context) error {
	if e.policy.MaxFailedAttempts <= 0 || e.policy.MaxFailedAttempts > 10 {
		return fmt.Errorf("refusing to induce lockout with threshold %d", e.policy.MaxFailedAttempts)
	}
	if err := e.unlock(ctx); err != nil {
		return err
	}
	u := &url.URL{Scheme: "https", Host: e.credentials.Host, Path: "/sdk"}
	c, err := vim25.NewClient(ctx, soap.NewClient(u, true))
	if err != nil {
		return err
	}
	for range e.policy.MaxFailedAttempts + 2 {
		err := session.NewManager(c).Login(ctx, url.UserPassword(e.principal, "Wrong!"+randomString()))
		if !vsclient.IsInvalidLogin(err) {
			return fmt.Errorf("expected deliberate invalid login, got %w", err)
		}
		u, err := e.sso.FindPersonUser(ctx, e.principal)
		if err != nil {
			return err
		}
		if u != nil && u.Locked {
			return nil
		}
	}
	return fmt.Errorf("account did not report Locked after bounded login failures")
}

func (e *exercise) cleanup(ctx context.Context) error {
	for _, c := range e.clients {
		c.Logout(ctx)
	}
	// Also remove sessions left by a partially constructed production client.
	var sm mo.SessionManager
	var errs []error
	if err := property.DefaultCollector(e.vc.Client).RetrieveOne(ctx,
		*e.vc.ServiceContent.SessionManager, []string{"sessionList"}, &sm); err != nil {
		errs = append(errs, err)
	} else {
		var keys []string
		for _, s := range sm.SessionList {
			if e.owns(s.UserName) {
				keys = append(keys, s.Key)
			}
		}
		if len(keys) != 0 {
			errs = append(errs, session.NewManager(e.vc.Client).TerminateSession(ctx, keys))
		}
	}
	am := object.NewAuthorizationManager(e.vc.Client)
	permissions, err := am.RetrieveEntityPermissions(ctx, e.vc.ServiceContent.RootFolder, false)
	if err != nil {
		errs = append(errs, err)
	} else {
		for _, p := range permissions {
			if !p.Group && e.owns(p.Principal) {
				errs = append(errs, am.RemoveEntityPermission(ctx, e.vc.ServiceContent.RootFolder, p.Principal, false))
			}
		}
	}
	u, err := e.sso.FindPersonUser(ctx, e.principal)
	if err != nil {
		errs = append(errs, err)
	} else if u != nil {
		errs = append(errs, e.sso.DeletePrincipal(ctx, e.name))
	}
	u, err = e.sso.FindPersonUser(ctx, e.principal)
	if err != nil {
		errs = append(errs, err)
	} else if u != nil {
		errs = append(errs, fmt.Errorf("test principal still exists"))
	}
	return errors.Join(errs...)
}

func cleanupJournal(ctx context.Context, credentialsPath, journal string) error {
	data, err := os.ReadFile(journal) //nolint:gosec // G304: The CLI operator explicitly chooses the cleanup journal path.
	if err != nil {
		return err
	}
	var state struct {
		Principal string `json:"principal"`
		Host      string `json:"host"`
		CleanedUp bool   `json:"cleaned_up"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	b, err := connect(ctx, credentialsPath)
	if err != nil {
		return err
	}
	defer b.logout()
	name, domain, ok := strings.Cut(state.Principal, "@")
	if !ok || !strings.HasPrefix(name, "vmop-relogin-") || domain != b.sso.Domain || state.Host != b.credentials.Host {
		return fmt.Errorf("journal does not identify a test account on this VC")
	}
	e := &exercise{testbed: b, name: name, principal: state.Principal, secrets: []string{b.credentials.Password}}
	err = e.step("cleanup", func() error { return e.cleanup(ctx) })
	state.CleanedUp = err == nil
	data, marshalErr := json.MarshalIndent(state, "", "  ")
	return errors.Join(err, marshalErr, os.WriteFile(journal, data, 0600))
}
