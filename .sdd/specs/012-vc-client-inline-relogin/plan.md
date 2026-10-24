# Implementation Plan: Inline vCenter Session Re-login

- **Spec**: [`spec.md`](./spec.md)
- **Research**: [`research.md`](./research.md)
- **Epic**: TBD
- **Date**: 2026-09-03

## Summary

Wrap the vCenter client's SOAP, PBM and REST round trippers so an authentication failure is turned into *re-login + replay the same request* in place, instead of waiting up to a 5-minute keepalive tick. The new chain is selected by the Supervisor `supports_vc_session_inline_relogin` capability (`pkgcfg.Features.VCSessionInlineRelogin`, default off) and lives beside the existing timer-driven handlers until it has soaked.

## Technical context

- **Go version**: as per root `go.mod`.
- **API version(s) touched**: none. No CRD, no status, no webhook.
- **Modules touched**: root module only.
- **Primary dependency**: `github.com/vmware/govmomi v0.56.0-alpha.0.0.20260720221020-d993be43fe66`. No new dependencies. All mechanics re-verified against this version in [`research.md`](./research.md) §2.
- **Blast radius**: one package (`pkg/util/vsphere/client`) plus a capability-backed feature state, a read of that state at the provider boundary, and a context opt-out threaded into the watcher.

## Constitution check

| Rule | Status | Notes |
|------|--------|-------|
| API compatibility | N/A | No API group, CRD, or field changes. |
| Thin controllers | OK | Nothing lands in `controllers/`. Logic is in `pkg/util/vsphere/client`. |
| No direct vSphere calls from controllers | OK | Unchanged; this is below the provider abstraction. |
| `+kubebuilder:rbac` | N/A | No new Kubernetes access. |
| Import aliases / grouping (`.golangci.yml`) | OK | New files follow `importas`; `vimtypes`, `pkgcfg`, `pkglog` aliases. |
| Forbidden imports (`depguard`) | OK | No `io/ioutil`, `github.com/pkg/errors`, `k8s.io/utils`. **Note**: `github.com/vmware/govmomi/vapi/internal` is unusable for a different reason — Go's `internal` rule (see [`research.md`](./research.md) §7). |
| Copyright header | OK | On every new file. |
| One suite bootstrap per package | OK | `client_suite_test.go` already exists and is reused. |
| Test labels from `pkg/constants/testlabels` | OK | New specs carry `testlabels.VCSim`, matching the existing `client_test.go`. |
| E2E ships with cluster-observable behavior | OK | See "Test strategy"; a session-termination E2E is part of this change set. |
| New capability ⇒ SDD artifacts | OK | This directory. |
| One test file per package | **Deviation** | See "Complexity tracking". |

## Project structure

```
pkg/util/vsphere/client/
  client.go              — MODIFIED: Config gains a mode field; assembly branches
  relogin.go             — NEW: session keeper (login mutex + generation), ctx opt-out
  relogin_soap.go        — NEW: soap.RoundTripper wrapper (vim25 + pbm)
  relogin_rest.go        — NEW: http.RoundTripper wrapper (vapi/rest)
  relogin_test.go        — NEW: unit + vcsim coverage for the new chain
  client_test.go         — MODIFIED: existing keepalive specs pinned to legacy mode

pkg/config/
  config.go              — MODIFIED: FeatureStates.VCSessionInlineRelogin
  capabilities/capabilities.go
                         — MODIFIED: capability key + CRD switch case

pkg/providers/vsphere/client/client.go
                         — MODIFIED: read the feature state, populate client.Config

pkg/util/vsphere/watcher/watcher.go
                         — MODIFIED: mark the watcher goroutine's ctx no-replay

test/builder/vcsim_test_context.go
                         — MODIFIED: expose the mode on VCClientConfig

test/e2e/vmservice/...   — NEW: session-termination E2E
```

## Design

### 1. The chain

Today (kept verbatim when the flag is off):

```
vimClient.RoundTripper  = keepalive.NewHandlerSOAP(soapClient, 5m, SoapKeepAliveHandlerFn(...))
restClient.Transport    = keepalive.NewHandlerREST(restClient, 5m, RestKeepAliveHandlerFn(...))
pbmClient.RoundTripper  = <raw derived soap.Client>   // no wrapper at all
```

With the flag on — arrangement **B**, keepalive outermost, per [`research.md`](./research.md) §6:

```
vimClient.RoundTripper  = keeperRoundTripper{                 // makes the keeper
    keepalive.NewHandlerSOAP(reloginSOAP(soapClient, keeper), 5m,   // discoverable
                             keeper.soapKeepAlive(<the same reloginSOAP wrapper>)),
    keeper}
pbmClient.RoundTripper  = reloginSOAP(pbmClient.Client, keeper)
restClient.Transport    = keepalive.NewHandlerREST(restClient, 5m, keeper.restKeepAlive)
                          // where restClient.Transport was first set to
                          //   reloginREST(<original transport>, keeper)
```

Three things this ordering buys, and one it demands:

- The keepalive ping travels **through** the re-login wrapper, so a dead session heals even with zero application traffic, and the ticker never dies permanently: the keeper's SOAP send (`keeper.soapKeepAlive`) swallows transient ping errors so a transport hiccup does not stop the goroutine, and only a persistent credential failure (`InvalidLogin`) is surfaced, matching legacy `SoapKeepAliveHandlerFn`.
- The re-login wrapper wraps the **raw** `soap.Client`, never `vimClient.RoundTripper` — wrapping the latter is an infinite loop.
- `session.Manager.Login` re-enters at the **top** of the chain (it holds the `*vim25.Client`), so the login body traverses the keepalive handler and (re)starts the ticker. That is desirable, and it is also the recursion hazard the deny-list below closes.
- **Demanded**: the handler must be installed before the first login, or the ticker never starts.
- `keeperRoundTripper` is a pass-through link whose only job is to make the keeper reachable from the `*vim25.Client` — see §5.

REST needs its own `send` because `rest.Client.Session()` swallows a 401 and returns `(nil, nil)` — the wrapper never sees a failure to act on. `keeper.restKeepAlive` is today's `RestKeepAliveHandlerFn` routed through the shared mutex, with the same tolerance as the SOAP send: it returns an error only when the login itself is answered `401` (`rest.IsStatusError`, the REST equivalent of `InvalidLogin`), and logs-and-continues on a transport hiccup or a transient re-login failure, so a vCenter that is answering but not yet fully up cannot kill the ticker for the life of the process. SOAP needs its own `send` for the mirror reason: with a `nil` send, govmomi's default `keepAliveSOAP` returns any error from the ping, and the keepalive handler stops its goroutine permanently on the first error — so a transient transport hiccup would silently kill the keepalive. `keeper.soapKeepAlive` mirrors the legacy `SoapKeepAliveHandlerFn` tolerance instead.

### 2. The session keeper

One object, shared by all three wrappers, owning everything session-scoped:

```go
type sessionKeeper struct {
    sm       *session.Manager
    rest     *rest.Client
    userInfo *url.Userinfo

    mu  sync.Mutex
    gen atomic.Uint64   // bumped on every successful login
}

// generation() is read BEFORE the first attempt.
// relogin(ctx, gen) takes mu, and if gen != k.gen.Load() another goroutine
// already refreshed -- return nil without logging in. Otherwise log in and
// bump gen.
```

This is the fix for reference-implementation gap §10.1: N goroutines faulting at once produce **one** login and **one** VC session. Read the generation before the call, not after the fault, so a login that lands while the call was in flight is still observed.

SOAP and REST have independent generations (`genSOAP`, `genREST`) — they are separate sessions with separate lifetimes. REST also passes the failed request's session ID to the keeper and compares it with the current session under `muREST`. `rest.Client.Do` stamps the header before the transport reads the generation, so a request can carry an old session ID with a new generation. Either a changed generation or a changed session ID suppresses another login. The keepalive captures the generation and session ID before probing.

### 3. SOAP / PBM wrapper

```go
func (r *reloginSOAP) RoundTrip(ctx context.Context, req, res soap.HasFault) error {
    gen := r.keeper.genSOAP()

    err := r.rt.RoundTrip(ctx, req, res)
    if err == nil || !soap.IsSoapFault(err) {
        return err                       // transport/TLS/HTTP errors pass through
    }
    if !IsNotAuthenticatedError(err) {
        return err                       // NoPermission, InvalidLogin, ... pass through
    }

    action := classify(ctx, req)
    if action == passThrough {           // login/logout/session bodies
        return err
    }
    if lerr := r.keeper.reloginSOAP(ctx, gen); lerr != nil {
        return errors.Join(err, lerr)    // gap §10.2: keep BOTH
    }
    if action == reloginOnly {           // property collector / view bodies
        return err
    }
    if cerr := resetResponse(res); cerr != nil {
        return err
    }
    return r.rt.RoundTrip(ctx, req, res) // exactly one replay
}
```

Notes:

- **Fault matching** reuses the package's existing `IsNotAuthenticatedError` → `fault.Is(err, &vimtypes.NotAuthenticated{})`, which normalizes the value-vs-pointer trap described in [`research.md`](./research.md) §2. Do not hand-roll a type switch.
- **`resetResponse`** zeroes the whole response struct by reflection (`reflect.ValueOf(res).Elem().Set(reflect.Zero(...))`) rather than only the `Fault_` field. Generated methods pass distinct `reqBody`/`resBody` values, so this cannot clobber the request; and it drops any partial `Res` as well as the stale fault. If the response is not a settable struct pointer, bail out and return the original fault rather than retrying blind.
- **One retry, no backoff, no cap** — there is no loop to bound. The fault is raised before the method dispatches, so the server did not act and the replay is safe even for task-creating methods.
- **The VC operation ID is left alone.** The replay reuses the same `ctx`, so `pkgctx.WithVCOpID`'s value rides along unchanged and the fault and its recovery share one op ID in `vpxd` logs. Do not derive a `-retry` suffix — decided in `spec.md`.

`classify` — the deny-list, in one place, with a comment per entry:

| Bodies | Action | Why |
|---|---|---|
| `LoginBody`, `LoginByTokenBody`, `LoginExtensionByCertificateBody`, `LogoutBody`, `CloneSessionBody`, `SessionIsActiveBody` | pass through | Re-entrancy guard. Closes reference gap §10.4 rather than relying on `vpxd` never answering a rejected login with `NotAuthenticated`. |
| `WaitForUpdatesExBody`, `WaitForUpdatesBody`, `CancelWaitForUpdatesBody`, `CreateFilterBody`, `DestroyPropertyFilterBody`, `DestroyPropertyCollectorBody`, `DestroyViewBody`, `ModifyListViewBody` | re-login, do **not** replay | `This` names session-scoped state that the re-login just destroyed. The vm-watcher's existing restart loop is the correct recovery ([`research.md`](./research.md) §4). |
| ctx carries `WithNoReplay` | re-login, do **not** replay | Explicit opt-out for callers that own session-scoped state; the watcher marks its goroutine's ctx. |
| everything else | re-login **and** replay | Including PBM, which reads the vim25 cookie live ([`research.md`](./research.md) §3). |

The deny-list and the context opt-out are deliberately redundant: the list cannot be forgotten by a new caller, the context flag is explicit at the site that knows why.

### 4. REST wrapper

```go
func (r *reloginREST) RoundTrip(req *http.Request) (*http.Response, error) {
    if isSessionPath(req.URL.Path) {
        return r.rt.RoundTrip(req)       // login / logout / session probe
    }
    replay := replayBody(req)            // nil when the body cannot be re-read
    gen := r.keeper.restGeneration()
    sessionID := req.Header.Get(restSessionHeader)

    res, err := r.rt.RoundTrip(req)
    if err != nil || res.StatusCode != http.StatusUnauthorized {
        return res, err
    }
    drain(res)                           // read + close, then restore the body
    if lerr := r.keeper.reloginREST(req.Context(), gen, sessionID, req.Method + " " + req.URL.Path); lerr != nil {
        return res, nil                  // surface the original 401
    }
    if replay == nil {
        return res, nil                  // healed, but this one is not retried
    }
    req2 := req.Clone(req.Context())
    req2.Body = replay()
    req2.Header.Set(restSessionHeader, r.keeper.rest.SessionID())
    return r.rt.RoundTrip(req2)
}
```

- `restSessionHeader = "vmware-api-session-id"` and the session path `"/rest/com/vmware/cis/session"` are **redeclared locally** — `github.com/vmware/govmomi/vapi/internal` is not importable from this repo. Comment each constant with its govmomi source.
- The header rewrite is not optional: `rest.Client.Do` stamps the *old* session id before the transport sees the request.
- `replayBody` implements an ordered rule and returns non-nil only when the body is safely repeatable: `req.GetBody != nil` (which `http.NewRequest` populates for the `*bytes.Buffer` that `rest.Resource.Request` produces via `encode()`); else a provably empty body (`req.Body == nil`, `http.NoBody`, or `ContentLength == 0`) replayed as `http.NoBody`; else `GET`/`HEAD`/`OPTIONS`/`DELETE` replayed as `http.NoBody` (govmomi's vapi/rest only ever puts the empty `io.MultiReader()` body on those verbs — `GetBody == nil` — and servers ignore bodies on them), with the stale `ContentLength` zeroed on any `http.NoBody` replay; else nil — streaming bodies with `ContentLength > 0` (`rest.Client.Upload`) are never buffered into memory and never retried. Action-style POSTs (`?~action=`) carry the empty MultiReader body with `ContentLength == 0`, so rule 2 replays them; only a streaming action POST with `ContentLength > 0` is excluded, which is the desired upload behavior.
- **The re-login is decided before the replay is.** A request whose body cannot be repeated still re-authenticates; only its own 401 is surfaced unretried. Skipping the re-login there would leave the session dead until some later call happened to fault with a replayable body or the keepalive ticked — the latency this feature exists to remove.
- **Draining restores the body.** `res` is handed back to the caller on every path that does not replay, so `drain` re-installs an equivalent reader over the bytes it read rather than leaving a consumed, closed body behind.
- `http.RoundTripper`'s contract forbids mutating the request; hence `Clone`.
- Path matching, not method matching, is what keeps login/logout/probe out of the retry — all three share the path.

### 5. Ad-hoc PBM clients

Three call sites build a PBM client directly off the vim25 client and would silently miss the wrapper ([`research.md`](./research.md) §3). Add a **package-level** `func NewPbmClient(ctx, vimClient *vim25.Client) (*pbm.Client, error)` that constructs and wraps consistently, and route those three through it. In legacy mode it returns an unwrapped client, exactly as today. `(*Client).NewPbmClient(ctx)` remains as a one-line convenience for callers that hold the `*Client`.

Whether to wrap is read **off `vimClient` itself**, via `keeperFromVimClient`, which type-asserts the `keeperRoundTripper` at the top of the chain. That matters because one of the three sites — the unmanaged-volumes `vmconfig.Reconciler` — only ever receives a `*vim25.Client`; its interface gives it nothing else. Threading a `*client.Client` down to it instead would mean six extra parameters, a reconciler field, and a pointer-equality guard whose failure silently returns an unwrapped client. Reading the keeper from the client the PBM client is *derived from* cannot name the wrong session and has no fallback branch to get wrong.

## Controller / webhook impact

- **Controllers**: none directly. `controllers/virtualmachineimagecache` consumes `c.RestClient()` and benefits transparently.
- **Services**: `services/vm-watcher` keeps its `IsNotAuthenticatedError` / `IsInvalidLogin` restart loop unchanged — with this change it fires on a session that has *already* been re-authenticated, so the restart succeeds on the first try instead of racing the keepalive.
- **Provider**: `pkg/providers/vsphere/client.NewClient` reads `pkgcfg.FromContext(ctx).Features.VCSessionInlineRelogin` and sets it on `client.Config`. This is the only production site that reads the feature state, so it carries its own vcsim-backed spec asserting both directions; the mode-parameterized suites elsewhere set `client.Config.InlineReloginEnabled` directly and would not catch its loss. Deliberately **not** read inside `pkg/util/vsphere/client` — that package must stay usable from contexts without a `pkgcfg` (its own `client_test.go` uses a bare `context.Background()`, which `pkgcfg.FromContext` would panic on).
- **Watcher**: `watcher.Start` wraps its goroutine ctx with `client.WithNoReplay`.
- **New RBAC**: none.

## Test strategy

Unit and vcsim, in `pkg/util/vsphere/client/relogin_test.go`, `Label(testlabels.VCSim)`. The file is deliberately `package client` (internal), not `package client_test`: the focused tests directly exercise package-private wrappers and the session keeper, which an external test package cannot reach.

- **The vcsim trap** — do **not** drive the retry with a property-collector call (`finder.*`, `property.Collector.Retrieve*`, `object.*.Properties`). Those are on vcsim's no-session allow-list and deliver `NotAuthenticated` via `MissingSet`, above the round tripper; the retry never fires and the test fails for reasons unrelated to the code ([`research.md`](./research.md) §5). Drive it with `methods.GetCurrentTime`, or inject with `simulator.FaultTypeNotAuthenticated`.
- **Native SOAP observations** — use `simulator.Registry.Handler` to record `simulator.Method.Name` under a mutex. Assert the protocol sequence `["CurrentTime", "Login", "CurrentTime"]`, not generated Go type strings. PBM uses a registry from `pbm/simulator.New`, installed with `Service.RegisterSDK` before startup. Keep `Service.AddFaultRule` for typed SOAP faults.
- **Native REST failures** — use `Service.HandleFunc` on a test-only endpoint for 401/403/500 bodies and vcsim's actual credential validator for rejected logins. Close a simulator server to exercise real connection failures. Do not synthesize `http.Response` or SOAP errors.
- **Retained HTTP observer** — a small `http.RoundTripper` records headers and attempt counts, including connections that never reach the server. vcsim does not expose these observations. Concurrency barriers remain outside the REST server because its handlers run under a shared lock; blocking inside a handler would prevent the competing call from reaching the barrier. These adapters depend only on the public `net/http` interface.
- **Fixture lifecycle and behavior assertions** — let `Service.NewServer` allocate the port (`127.0.0.1:0`), create actual property collectors with `property.DefaultCollector(c).Create`, and test enabled/disabled recovery through API calls instead of reflecting into keepalive internals or asserting wrapper types. Close servers before removing models and stop keepalive handlers even after failed logins or connection failures.
- **Session expiry, not "never logged in"** — log in, read `sm.UserSession(ctx).Key`, terminate it from a second admin client via `session.NewManager(admin).TerminateSession(ctx, []string{key})`. The existing `client_test.go` already uses this two-manager technique.
- **Cases**:
  - SOAP: terminated session → one fault, one login, one replay, caller sees success.
  - SOAP: the stale-fault regression — assert the caller gets success, which only holds if the response was reset.
  - SOAP: `LoginBody` faulting does not recurse (bounded call count).
  - SOAP: `WaitForUpdatesExBody` → login happened, fault still returned, not replayed.
  - SOAP: `WithNoReplay` ctx → same.
  - SOAP: bad credentials → returned error joins the original fault **and** the login failure.
  - SOAP: concurrency — N goroutines faulting together produce exactly one `LoginBody` on the wire.
  - SOAP: non-auth faults (`NoPermission`, `InvalidArgument`) and transport errors pass through untouched, no login.
  - PBM: a PBM call on a dead session recovers.
  - REST: 401 → login → replay with a **rewritten** `vmware-api-session-id`.
  - REST: requests stamped with the old session ID before another call completes recovery reuse that recovery, even when they reach the wrapper with the new generation; concurrent delayed calls create no additional sessions.
  - REST: simultaneous 401 responses are held behind a channel barrier until every caller has faulted; releasing them produces one login and one replay per caller.
  - REST: keepalive and application calls both observe a dead session; exercise each recovery order with channel barriers and assert one shared login.
  - REST: expiration of the replacement session triggers another recovery, with one login per expiration.
  - REST: a rejected login does not recurse or replay the application request, does not advance the generation, and preserves the original 401 body.
  - REST: a replay that also returns 401 stops after two application attempts and one login, returning the replay's response body.
  - REST: HTTP 403, HTTP 500, and transport errors pass through unchanged without login or replay.
  - REST: session-path requests are never retried.
  - REST: a streaming body is not buffered and not retried.
  - REST: keepalive `send` heals a dead session with no application traffic.
  - Keepalive: with a short idle, the ticker survives a session termination (arrangement B), and pings resume.
- **Legacy mode**: the existing `client_test.go` keepalive specs are pinned to flag-off and must keep passing unchanged. Assert flag-off surfaces authentication faults without inline recovery, including derived PBM clients.

Integration: no new envtest suite. `test/builder/vcsim_test_context.go` gains the mode on `VCClientConfig` so existing `VSphereClientFn` overrides in `controllers/**` and `services/vm-watcher` can exercise either mode; run the vm-watcher suite in both.

E2E: mandatory per `e2e-sync-with-changes.md` — this is cluster-observable. Full design in the next section.

## E2E design

Two `It` blocks in one new spec file, `test/e2e/vmservice/vmservice/virtualmachine/vm_vcsession.go`, exported as `VCSessionRecoverySpec(ctx, inputGetter)` and registered from `test/e2e/vmservice/vmservice_test.go` under a `Context("VM-VCSESSION", ...)`. Both carry `experimental` until validated on a real WCP cluster. Both are `Serial` (they take VM Operator's VC session away from every other spec) and both `Skip` unless the `supports_vc_session_inline_relogin` capability is activated, so a legacy-mode testbed is unaffected.

### Shared setup

- Gate: `skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.VCSessionInlineReloginCapabilityName)` — the standard capability-gating helper already used throughout `test/e2e/vmservice` (see `vm_group.go`, `configpolicy.go`, ...). Add `VCSessionInlineReloginCapabilityName = "supports_vc_session_inline_relogin"` to `test/e2e/vmservice/consts/consts.go`, per `e2e-testing.md`'s "capability constants belong in `consts.go`" rule.
- Admin VC client: `vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())` (logs in as `testbed.AdminUsername`), with `DeferCleanup(func() { vcenter.LogoutVimClient(c) })`.
- A pre-created, powered-on VM per block, built the way `vm_extraconfig.go` does it, so the recovery assertion measures VM Operator's reaction and not first-boot latency.
- **Restart-count baseline**: record `.status.readyReplicas` and the manager pod's `restartCount` before the disruption, and assert they are unchanged afterwards. This is what proves recovery happened *in process* rather than via a crash-loop, and it holds regardless of timing.

### Block 1 — terminated session (`core-functional`, `experimental`)

Deterministic, non-disruptive, and the direct test of the inline path. No VC downtime, so nothing else on the testbed is harmed.

1. Read the VM Operator solution-user name from the `wcp-vmop-sa-vc-auth` secret in `vmware-system-vmop` (`utils.GetSecret`, `.Data["username"]`).
2. With the admin client, retrieve `mo.SessionManager` (`property.DefaultCollector(admin).RetrieveOne(ctx, *admin.ServiceContent.SessionManager, []string{"sessionList"}, &sm)`) and collect every `sm.SessionList[i].Key` whose `UserName` matches that solution user.
3. `session.NewManager(admin).TerminateSession(ctx, keys)`.
4. Immediately drive a VM operation — `vmoperator.UpdateVirtualMachinePowerState(...)` to `PoweredOff`, then `vmoperator.WaitForVirtualMachinePowerState(...)`.
5. Assert it completes within `vcSessionRecoveryTimeout` (below), and that the manager pod's `restartCount` did not change.

### Block 2 — vpxd restart (`extended-functional`, `disruptive`, `experimental`)

The scenario that motivated this work: the service goes away, connections drop, every session dies server-side.

1. Create an admin client and confirm `session.NewManager(admin).UserSession(ctx)` returns a non-nil session. Retain this client without re-authenticating it to verify disruption. SSH to the VC as root: `e2essh.NewSSHCommandRunner(vcenter.GetVCPNIDFromKubeconfig(ctx, kubeconfigPath), vcenter.VCSSHPort, testbed.RootUsername, []ssh.AuthMethod{ssh.Password(testbed.RootPassword)})`. This is exactly the runner `supervisor.GetControlPlaneVMConnectionDetails` builds at `test/e2e/infrastructure/vsphere/supervisor/apiserver.go:112`.
2. Restart vpxd: `vmon-cli -r vpxd`. **Validate this command on a testbed before merging** — `service-control --restart vmware-vpxd` is the documented fallback if `vmon-cli` is unavailable or restarts too much. Put it in a single `const vpxdRestartCmd` so correcting it is a one-line change.
3. Do **not** trust the command's exit status. Poll for VC to serve again with a *fresh* client — `vcenter.NewVimClient(host, admin, pass)` in an `Eventually` with a generous timeout (vpxd restart can take several minutes on a loaded testbed). In the same poll, require the pre-restart admin client's `UserSession(ctx)` to return `(nil, nil)`, proving its previously active session was invalidated; a transport error is not proof. A failed or no-op restart must not pass just because a fresh login succeeds. Record the wall-clock instant both checks succeed; that is `t0`.
4. From `t0`, drive the same VM operation as block 1 and assert it completes within `vcSessionRecoveryTimeout`.
5. Assert the manager pod's `restartCount` did not change.
6. Log out each fresh polling client, and make the pre-restart admin client's `DeferCleanup` logout tolerant of a session that is already gone.

### What actually distinguishes the two modes

Both blocks would eventually pass with the capability unactivated — the keepalive re-logs in within `keepAliveIdleTime` (5 minutes). **The timing bound is the whole assertion**, so it has to be stated as such rather than left implicit:

```go
// A VM power-state change normally completes in well under a minute. With
// inline re-login the session is restored on the first faulting call, so the
// only added cost is one round trip. With the legacy timer-driven keepalive
// the operation cannot succeed until the next 5m tick. 90s is comfortably
// above the former and comfortably below the latter.
const vcSessionRecoveryTimeout = 90 * time.Second
```

Two honest caveats to carry into review:

- The bound is testbed-speed dependent. It is the reason both blocks are `Serial` — a parallel spec competing for VC would widen the operation's own latency and erode the margin.
- Block 2 measures from "VC answers again", not from "vpxd was restarted", so vpxd's own startup time is excluded from the budget.

Run either block against a legacy-mode build to confirm it actually fails there. A recovery test that passes in both modes is testing nothing.

## Rollout / migration

- **Flag**: `pkgcfg.Features.VCSessionInlineRelogin`, sourced from the Supervisor capability `supports_vc_session_inline_relogin` (`pkg/config/capabilities/capabilities.go`), **default `false`** until the capability is activated.
- **Why `Features.*` and not `Config`** (decided, see `spec.md` "Decisions"): this behavior is meant to roll out the same way every other WCP-gated behavior change does — activated per-Supervisor by WCP, not by an operator-managed env var. `Features.*` is where capability-backed state already lives (`VMGroups`, `VMEviction`, ...); `Config` is reserved for operational knobs like `AsyncSignalEnabled` that have no Supervisor capability behind them.
- **Ordering**: land the wrapper + tests with the capability unactivated everywhere (no behavior change in any environment), then WCP activates the capability once it has soaked, then delete the legacy handlers in a third change under its own spec. `SoapKeepAliveHandlerFn` and `RestKeepAliveHandlerFn` stay exported until that third step — `client_test.go` calls both directly.
- **Observability**: log at `Info` on every re-login with the session generation and the triggering method name, so a burst of logins is visible as a burst. This is the signal an operator uses to tell "recovered once" from "flapping".
- **Rollback**: deactivate the capability on Supervisor. The capability controller (`controllers/infra/capability`) detects the change and restarts the manager to pick up the new value. No persisted state, no schema change.
- **Release note**: user-visible only as "VM Operator now recovers from a lost vCenter session on the next call instead of on the next keepalive interval."

## Complexity tracking

| Violation | Why needed | Simpler alternative rejected because |
|-----------|------------|--------------------------------------|
| Constitution says one test file per package (`<package>_test.go`); this adds `relogin_test.go` alongside `client_test.go` | The new specs are ~600 lines with their own spy/simulator scaffolding, and `client_test.go` is already ~470 lines of unrelated keepalive coverage | Appending to `client_test.go` produces a file where the legacy and inline suites are hard to tell apart. The repo's actual practice is one test file per source file (`pkg/util/vsphere/vm/{guest_id,hardware_version,power_state}_test.go`), which this matches. If the reviewer prefers the literal rule, fold `relogin_test.go` into `client_test.go` — no other change is needed. |
| Two mechanisms for "do not replay" (body-type deny-list **and** a context flag) | The deny-list is the safety net a future caller cannot forget; the context flag is the explicit, greppable marker at the one site that knows why | Deny-list only: a new session-scoped call site silently gets replayed. Context only: every existing property-collector path has to be found and annotated correctly, with no backstop when one is missed. |
| Reflection to zero the SOAP response before replay | `soap.HasFault` exposes no way to clear a fault, and a successful second response does not overwrite `Fault_` | Allocating a fresh response and copying it back is the same reflection with more steps. A `ClearFault()` interface would need an upstream govmomi change. |
