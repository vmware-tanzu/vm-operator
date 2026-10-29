# Tasks: Inline vCenter Session Re-login

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Research**: [`research.md`](./research.md)
- **Epic**: TBD

> Every task that produces shipping code needs a `[vmop-NNN]` story or sub-task linked to the epic via `customfield_10830` (set post-create via PUT). File them once the epic exists and fill in the tags.

## How to work these tasks

Phases are ordered; tasks inside a phase marked `[P]` touch disjoint files and may run in parallel. Each task states the exact file, the exact symbols, and a **Verify** line. Do not move to the next phase until the current phase's Verify passes.

Repo-wide checks, run from the repo root:

```bash
make lint-go
```

```bash
go test ./pkg/util/vsphere/client/... ./pkg/config/...
```

### Read these before writing code

1. [`research.md`](./research.md) — the govmomi mechanics. Sections §3 (PBM), §4 (watcher), §5 (the vcsim property-collector trap) and §7 (REST) each describe a way to get this wrong that is not obvious from the source.
2. `pkg/util/vsphere/client/client.go` — the file being changed.
3. `.golangci.yml` `linters.settings.importas.alias` — import aliases are enforced, not optional.

### Standing rules for every task here

- Copyright header on every new file, copied verbatim from `pkg/util/vsphere/client/client.go` lines 1-3.
- Comments are complete sentences ending in a period (`architectural-standards.md`).
- Test files are `package client_test` (external). The `depguard` linter forbids `testing`/ginkgo/gomega outside `_test.go`. Approved deviation: `relogin_test.go` is `package client` (internal) because focused tests directly exercise package-private wrappers and the session keeper.
- Never import `github.com/vmware/govmomi/vapi/internal` — Go's `internal` rule makes it uncompilable from this repo. Redeclare the constants you need.
- Never call `pkgcfg.FromContext` from inside `pkg/util/vsphere/client`. It panics when the context has no config, and this package's own tests use a bare `context.Background()`.

---

## Phase 1 — Capability plumbing

No behavior change. At the end of this phase the feature state exists and is readable, and nothing consumes it.

- [ ] **T001** Add the feature state field.
  - `pkg/config/config.go`: add to `type FeatureStates struct`, after `VMEviction`:
    ```go
    VCSessionInlineRelogin bool
    ```
  - No `default.go` change: like the other capability-only fields (`VMGroups`, `VMEviction`, ...), the zero value (`false`) is the default and `Default()` does not list it explicitly.
  - **Verify**: `go build ./pkg/config/...`

- [ ] **T002** Add the capability key and wire it into the CRD switch.
  - `pkg/config/capabilities/capabilities.go`: add, after `CapabilityKeyVMEviction`:
    ```go
    // CapabilityKeyVCSessionInlineRelogin is the name of the capability key
    // defined in the Supervisor capabilities CRD. It gates the vCenter
    // client's inline session re-login: when active, an authentication
    // failure re-authenticates and replays the request in place instead of
    // waiting for the next timer-driven keepalive tick.
    CapabilityKeyVCSessionInlineRelogin = "supports_vc_session_inline_relogin"
    ```
  - In `updateCapabilitiesFeaturesFromCRD`, add to the `switch capName` block:
    ```go
    case CapabilityKeyVCSessionInlineRelogin:
        fs.VCSessionInlineRelogin = capStatus.Activated
    ```
  - This capability predates `SVAsyncUpgrade` support in `updateCapabilitiesFeaturesFromMap`, so it is **not** wired into the legacy ConfigMap path — matching the newer capabilities (`VMGroups`, `VMEviction`, ...) rather than the `TKGMultipleCL`/`WorkloadIsolation` bootstrap-era ones.
  - **Verify**: `go build ./pkg/config/...`

- [ ] **T003** `[P]` Extend the capability tests.
  - `pkg/config/capabilities/capabilities_test.go`: add `CapabilityKeyVCSessionInlineRelogin` alongside `CapabilityKeyVMEviction` everywhere it appears — the `capv1.Capabilities` `Status.Supervisor` fixtures (true and false), the corresponding `config.Features` assertions, and the `Specify`/`Context` blocks in `Describe("UpdateCapabilities")`, `Describe("UpdateCapabilitiesFeatures")`, and `Describe("WouldUpdateCapabilitiesFeatures")`. The combined diff-string assertion in the last of these is alphabetically ordered — `VCSessionInlineRelogin` sorts immediately before `VMAffinityDuringExecution` (`"C"` < `"M"` byte-wise).
  - **Verify**: `go test ./pkg/config/...`

- [ ] **T004** Add the client-side switch (still unused).
  - `pkg/util/vsphere/client/client.go`: add to `type Config struct`:
    ```go
    // InlineReloginEnabled selects the inline re-login round trippers
    // instead of the timer-driven keepalive handlers.
    InlineReloginEnabled bool
    ```
  - `test/builder/vcsim_test_context.go` around line 783: the `pkgclient.Config{...}` literal gains `InlineReloginEnabled: false,` so tests can flip it per-suite.
  - Add `pkg/providers/vsphere/client/client_test.go` (plus its `client_suite_test.go`) once T017 wires the feature state: a vcsim-backed spec asserting `c.Client.Config().InlineReloginEnabled` in both directions. This is the only production read of the feature state, and every other suite sets `client.Config.InlineReloginEnabled` directly, so nothing else would catch its loss.
  - **Verify**: `go build ./... && go vet ./pkg/util/vsphere/client/...`

---

## Phase 2 — The session keeper

The shared object all three wrappers depend on. Nothing wires it up yet.

- [ ] **T005** Create `pkg/util/vsphere/client/relogin.go`.

  Package-private type, roughly:

  ```go
  type sessionKeeper struct {
      sm       *session.Manager
      rest     *rest.Client // set after the REST client is built
      userInfo *url.Userinfo

      muSOAP  sync.Mutex
      genSOAP atomic.Uint64
      muREST  sync.Mutex
      genREST atomic.Uint64
  }
  ```

  Methods:

  - `func newSessionKeeper(ctx context.Context, sm *session.Manager, userInfo *url.Userinfo) *sessionKeeper` — capture `log: pkglog.FromContextOrDefault(ctx)` here. The keepalive send funcs run on a background context and would otherwise log through the global default sink instead of the manager's logger.
  - `func (k *sessionKeeper) setRestClient(c *rest.Client)` / `func (k *sessionKeeper) restClient() *rest.Client` — both under `muREST`. Read the field directly only where the lock is already held.
  - `func (k *sessionKeeper) soapGeneration() uint64` / `restGeneration() uint64` — plain `Load()`.
  - `func (k *sessionKeeper) reloginSOAP(ctx context.Context, gen uint64) error`
  - `func (k *sessionKeeper) reloginREST(ctx context.Context, gen uint64, sessionID, trigger string) error` — under `muREST`, skip login if either the generation or the failed request's session ID differs from the current state.
  - `func (k *sessionKeeper) restKeepAlive() error`

  The SOAP re-login body is below. REST follows the same shape with its own mutex and generation, plus the session ID comparison described above:

  ```go
  k.muSOAP.Lock()
  defer k.muSOAP.Unlock()

  // Another goroutine already refreshed while we were faulting.
  if k.genSOAP.Load() != gen {
      return nil
  }

  log := pkglog.FromContextOrDefault(ctx).WithName("vcSessionRelogin")
  log.Info("Re-authenticating vim client", "generation", gen)

  if err := k.sm.Login(ctx, k.userInfo); err != nil {
      return err
  }
  k.genSOAP.Add(1)
  return nil
  ```

  `restKeepAlive` is the `send` func for `keepalive.NewHandlerREST`. It probes and heals, because `rest.Client.Session` swallows a 401 and returns `(nil, nil)` — the wrapper never sees a failure to act on:

  ```go
  ctx := k.backgroundContext() // context.Background() + the captured logger
  gen := k.restGeneration()
  sessionID := k.restClient().SessionID()
  s, err := k.restClient().Session(ctx)
  if err != nil {
      return nil // Transport hiccup; do not kill the ticker.
  }
  if s != nil {
      return nil
  }
  if err := k.reloginREST(ctx, gen, sessionID, "keepalive"); err != nil {
      if rest.IsStatusError(err, http.StatusUnauthorized) {
          return err // Invalid credentials; let the handler stop.
      }
      k.log.WithName("vcSessionRelogin").
          Error(err, "Error in rest client's keepalive handler")
      return nil
  }
  return nil
  ```

  Also in this file, the context opt-out:

  ```go
  type noReplayContextKey struct{}

  // WithNoReplay returns a context that suppresses request replay after an
  // inline re-login. Re-login still happens; the original fault is still
  // returned. Use it for callers that own session-scoped server state --
  // property collectors, property filters, container and list views -- which
  // a re-login destroys.
  func WithNoReplay(ctx context.Context) context.Context

  func isNoReplay(ctx context.Context) bool
  ```

  **Do not** return an error from `restKeepAlive` for a transport failure *or a transient re-login failure* — govmomi's handler calls `Stop()` and the goroutine exits permanently on any error (`session/keepalive/handler.go`). Only a `401` on the login itself is persistent. Note `rest.Client.Login` fails with govmomi's `*statusError`, not a vim `InvalidLogin` fault, so `IsInvalidLogin` would never match here; use `rest.IsStatusError(err, http.StatusUnauthorized)`.

  **Verify**: `go build ./pkg/util/vsphere/client/...`

- [ ] **T006** `[P]` Unit-test the keeper in a new `pkg/util/vsphere/client/relogin_test.go`.
  - Reuse the existing suite bootstrap — do **not** add a second `_suite_test.go`; `client_suite_test.go` already calls `RunSpecs`.
  - Top-level `Describe("Relogin", Label(testlabels.VCSim), func() {...})`, matching `client_test.go`.
  - Cases: a stale generation short-circuits without calling `Login`; N=20 goroutines calling `reloginSOAP` with the same starting generation produce exactly one `Login`; a `Login` error propagates and does **not** bump the generation.
  - Drive these against `simulator.Test` with a real `session.Manager`, counting logins through `simulator.Registry.Handler` (see T011).
  - **Verify**: `go test ./pkg/util/vsphere/client/...`

---

## Phase 3 — SOAP and PBM

- [ ] **T007** Create `pkg/util/vsphere/client/relogin_soap.go`.

  ```go
  type reloginSOAP struct {
      rt     soap.RoundTripper
      keeper *sessionKeeper
  }

  func newReloginSOAP(
      rt soap.RoundTripper,
      keeper *sessionKeeper) *reloginSOAP
  ```

  `RoundTrip` follows `plan.md` "SOAP / PBM wrapper" exactly. Three details that are easy to get wrong:

  1. **Read the generation before the first attempt**, not after the fault. A login that lands while the call was in flight must be observable.
  2. **Match the fault with the package's existing `IsNotAuthenticatedError`** (`client.go:251`), which delegates to `fault.Is(err, &vimtypes.NotAuthenticated{})`. Do **not** write `switch err.VimFault().(type) { case *vimtypes.NotAuthenticated: }` — the decoder stores the **value**, not the pointer, so a pointer arm never matches (`research.md` §2).
  3. **Leave the context alone.** The replay reuses the same `ctx`, so `pkgctx.WithVCOpID`'s value rides along unchanged. Do not derive a `-retry` operation ID.

  `classify(ctx, req) action` returns one of `actionPassThrough`, `actionReloginOnly`, `actionReloginAndReplay`:

  ```go
  switch req.(type) {
  // Re-entrancy guard: these are how a session is established or torn
  // down. Acting on their faults can recurse.
  case *methods.LoginBody,
      *methods.LoginByTokenBody,
      *methods.LoginExtensionByCertificateBody,
      *methods.LogoutBody,
      *methods.CloneSessionBody,
      *methods.SessionIsActiveBody:
      return actionPassThrough

  // "This" names session-scoped state that the re-login just destroyed.
  // Replaying is guaranteed to fail; the caller's own restart logic is
  // the correct recovery. See research.md section 4.
  case *methods.WaitForUpdatesExBody,
      *methods.WaitForUpdatesBody,
      *methods.CancelWaitForUpdatesBody,
      *methods.CreateFilterBody,
      *methods.DestroyPropertyFilterBody,
      *methods.DestroyPropertyCollectorBody,
      *methods.DestroyViewBody,
      *methods.ModifyListViewBody:
      return actionReloginOnly
  }

  if isNoReplay(ctx) {
      return actionReloginOnly
  }
  return actionReloginAndReplay
  ```

  Note the creation calls — `CreatePropertyCollectorBody`, `CreateContainerViewBody`, `CreateListViewBody` — are deliberately **absent**: they build fresh state on the new session and are safe to replay.

  `resetResponse(res soap.HasFault) error` zeroes the whole response struct before the replay:

  ```go
  v := reflect.ValueOf(res)
  if v.Kind() != reflect.Ptr || v.IsNil() {
      return fmt.Errorf("response is not a non-nil pointer: %T", res)
  }
  e := v.Elem()
  if e.Kind() != reflect.Struct || !e.CanSet() {
      return fmt.Errorf("response is not a settable struct: %T", res)
  }
  e.Set(reflect.Zero(e.Type()))
  return nil
  ```

  This is not cosmetic: govmomi returns `WrapSoapFault(f)` whenever `resBody.Fault() != nil`, and a successful second response has no `Fault` element to overwrite the first attempt's — so without this the caller gets `NotAuthenticated` from a retry that actually succeeded. Zeroing the whole struct (rather than just `Fault_`) also drops any partial `Res` and needs no field-name knowledge. It is safe because generated methods pass **distinct** `reqBody` and `resBody` values (`research.md` §2).

  On re-login failure return `errors.Join(originalFault, loginErr)` so the caller can see why recovery failed.

  **Verify**: `go build ./pkg/util/vsphere/client/... && make lint-go`

- [ ] **T008** Branch the SOAP assembly in `NewVimClient` (`pkg/util/vsphere/client/client.go`).

  Replace the single `vimClient.RoundTripper = keepalive.NewHandlerSOAP(...)` assignment with a branch on `config.InlineReloginEnabled`. The inline arm:

  ```go
  keeper := newSessionKeeper(ctx, sm, userInfo)
  rt := newReloginSOAP(soapClient, keeper)
  vimClient.RoundTripper = &keeperRoundTripper{
      RoundTripper: keepalive.NewHandlerSOAP(
          rt,
          keepAliveIdleTime,
          keeper.soapKeepAlive(rt)),
      keeper: keeper,
  }
  ```

  `keeperRoundTripper` forwards every request untouched; it exists so `keeperFromVimClient` can find the keeper from the `*vim25.Client` alone (T009). Keep it outermost — that is where the lookup reads it.

  Five things this ordering depends on. Changing any of them breaks recovery silently:

  - The re-login wrapper wraps **`soapClient`**, the raw `*soap.Client` — never `vimClient.RoundTripper`, which would be an infinite loop.
  - The keepalive handler is **outermost**, so its ping travels through the wrapper and a dead session heals with no application traffic (`research.md` §6, arrangement B).
  - The `send` argument is **`keeper.soapKeepAlive(rt)`**: the ping is `GetCurrentTime` through the wrapper, and the send tolerates transient ping errors (logging and returning nil) because govmomi's keepalive goroutine stops permanently on the first non-nil send error. It returns an error only on a persistent `InvalidLogin`, matching the legacy `SoapKeepAliveHandlerFn`. Passing `SoapKeepAliveHandlerFn` here would defeat the point.
  - The whole chain is installed **before** `sm.Login(ctx, userInfo)`. The ticker starts only when a login body traverses the handler.

  `NewVimClient` must also return the keeper (or accept one) so `NewClient` can share it with the REST and PBM wrappers. Change the signature to return `(*vim25.Client, *session.Manager, *sessionKeeper, error)` and update the one caller, `NewClient`. **Verified**: the function is exported but has no callers outside this package — the `NewVimClient` hits under `test/e2e/` are a different function, `vcenter.NewVimClient`.

  Leave the legacy arm byte-for-byte as it is today.

  **Verify**: `go build ./... && go test ./pkg/util/vsphere/client/...` — the existing keepalive specs must still pass, since the default is `InlineReloginEnabled: false`.

- [ ] **T009** Add the PBM constructor, and make the keeper discoverable.

  In `relogin.go`, alongside the `keeperRoundTripper` from T008:

  ```go
  // keeperFromVimClient returns the session keeper driving the given vim25
  // client's session, or nil when that client is in legacy keepalive mode.
  func keeperFromVimClient(c *vim25.Client) *sessionKeeper
  ```

  It type-asserts `c.RoundTripper` to `*keeperRoundTripper` and returns that link's keeper. In `client.go`:

  ```go
  // NewPbmClient returns a new PBM client that shares the given vim25
  // client's vCenter session, including its inline re-login behavior when
  // that session has it.
  func NewPbmClient(ctx context.Context, vimClient *vim25.Client) (*pbm.Client, error)

  // Convenience for callers holding the *Client; one line, delegates.
  func (c *Client) NewPbmClient(ctx context.Context) (*pbm.Client, error)
  ```

  The package-level form wraps when `keeperFromVimClient(vimClient) != nil`. Use it inside `NewClient` too, replacing the inline `pbm.NewClient` + `if config.InlineReloginEnabled` block, so there is exactly one construction path.

  `pbmClient.Client` is the derived raw `*soap.Client`; `pbm.Client.RoundTrip` dispatches through the `RoundTripper` field, so this is the correct injection point. Replay works because `pbm.NewClient` sets `sc.Cookie = c.SessionCookie`, a method value that reads the vim25 client's cookie jar **live** on every request — so once the vim25 session is refreshed the replay carries the new cookie (`research.md` §3).

  **Why the lookup and not a parameter**: see `plan.md` §5. Do **not** thread a `*client.Client` down to callers that only have a `*vim25.Client` — that costs six signature changes and a pointer-equality guard that silently returns an unwrapped client when it misses.

  **Verify**: `go build ./pkg/util/vsphere/client/...`

- [ ] **T010** `[P]` Route the ad-hoc PBM constructions through `NewPbmClient`.
  - `pkg/providers/vsphere/vmprovider_vm.go:733` — `*client.Client` in scope; use the method.
  - `pkg/providers/vsphere/storage/provisioning.go:70` — `*client.Client` in scope; use the method.
  - `pkg/vmconfig/volumes/unmanaged/register/unmanagedvolumes_register.go:351` — only a `*vim25.Client` (the `vmconfig.Reconciler` interface gives it nothing else); use the package-level `pkgclient.NewPbmClient(ctx, vimClient)`. Nothing else in the file or its callers changes.

  **Verify**: `go build ./... && go test ./pkg/providers/... ./pkg/vmconfig/...`

- [ ] **T011** SOAP specs in `pkg/util/vsphere/client/relogin_test.go`.

  **The trap that will cost you an afternoon if you skip this paragraph**: do **not** drive the retry with a property-collector call — `finder.*`, `property.Collector.Retrieve*`, `object.*.Properties`, `mo.RetrieveProperties`. `RetrieveProperties` and `RetrievePropertiesEx` are on vcsim's no-session allow-list (`simulator/simulator.go:196-203`), so an unauthenticated call returns **HTTP 200** with `NotAuthenticated` inside `ObjectContent.MissingSet`. govmomi converts that to a `vimFaultError` **above** the round tripper, `soap.IsSoapFault` is false, and the retry never fires. The test then fails for a reason that has nothing to do with the code under test. Drive it with `methods.GetCurrentTime`, or inject with `simulator.FaultTypeNotAuthenticated` (`simulator/fault_injection.go:180`).

  Record SOAP method names through `simulator.Registry.Handler`, guarding the observations with a mutex. This uses vcsim's dispatch hook and protocol names rather than a custom SOAP transport or generated Go type strings. Preserve any existing registry handler. See the test strategy in `plan.md` for REST fault injection and the limited HTTP observers retained for headers, connection failures, and concurrency barriers.

  To exercise a real expiry rather than "never logged in", log in, read `sm.UserSession(ctx).Key`, then terminate it from a second manager: `session.NewManager(other).TerminateSession(ctx, []string{key})`. `client_test.go` already uses this two-manager technique — copy its shape.

  Cases:
  - Terminated session, then `methods.GetCurrentTime`: returns **success**, and the observed sequence is
    `["CurrentTime", "Login", "CurrentTime"]`.
  - Same, asserting no error — this is the `resetResponse` regression. Delete `resetResponse` locally and confirm this case fails; that is how you know it is testing what it claims.
  - A faulting `*methods.LoginBody` does not recurse: bounded observed call count.
  - `*methods.WaitForUpdatesExBody` on a dead session: a `LoginBody` appears in the sequence, the fault is still returned to the caller, and there is no second `WaitForUpdatesExBody`.
  - `WithNoReplay(ctx)`: same shape as above for an ordinary body.
  - Wrong credentials in the keeper: the returned error satisfies both `IsNotAuthenticatedError` and `errors.Is`/`MatchError` for the login failure.
  - 20 goroutines faulting concurrently: exactly one `Login` in the observed sequence.
  - `NoPermission` / `InvalidArgument` faults and a transport error pass through untouched, and no `LoginBody` appears.

  **Verify**: `go test ./pkg/util/vsphere/client/... -run TestClient`

- [ ] **T012** `[P]` PBM spec: terminate the session, then call a PBM method (e.g. `pbmClient.QueryProfile`) and assert it succeeds. Requires the PBM simulator, already blank-imported in `client_test.go` (`_ "github.com/vmware/govmomi/pbm/simulator"`).
  - **Verify**: `go test ./pkg/util/vsphere/client/...`

---

## Phase 4 — REST

- [ ] **T013** Create `pkg/util/vsphere/client/relogin_rest.go`.

  ```go
  const (
      // restSessionHeader mirrors vapi/internal.SessionCookieName, which is
      // in an internal package and cannot be imported from this repository.
      restSessionHeader = "vmware-api-session-id"

      // restSessionPath mirrors vapi/rest.Path + vapi/internal.SessionPath.
      // Login (POST), logout (DELETE) and the session probe
      // (POST ?~action=get) all share this path.
      restSessionPath = "/rest/com/vmware/cis/session"
  )

  type reloginREST struct {
      rt     http.RoundTripper
      keeper *sessionKeeper
  }
  ```

  `RoundTrip` follows `plan.md` "REST wrapper". Five details:

  1. **Exclude the session path first**, before anything else. That is the re-entrancy guard, and it also preserves `rest.Client.Session`'s documented `(nil, nil)`-on-401 contract.
  2. **Rewrite the header on the replay.** `rest.Client.Do` stamps the *old* session id before the transport sees the request; without `req2.Header.Set(restSessionHeader, k.rest.SessionID())` the replay fails identically.
  3. **Clone the request.** `http.RoundTripper`'s contract forbids mutating the one you were given.
  4. **Drain the 401 response body** before replaying, so the connection can be reused — and restore it with a reader over the drained bytes, because `res` is returned to the caller on every path that does not replay.
  5. **Only replay a safely repeatable body.** `replayBody` implements an ordered rule and returns non-nil only then: `req.GetBody != nil` — the `*bytes.Buffer` that `rest.Resource.Request` produces via `encode()` for JSON POST/PATCH/PUT; else a provably empty body (`req.Body == nil`, `http.NoBody`, or `ContentLength == 0`) replayed as `http.NoBody` — this covers the empty `io.MultiReader()` body (`GetBody == nil`, `ContentLength == 0`) that `rest.Resource.Request` puts on no-body requests, including action POSTs; else GET/HEAD/OPTIONS/DELETE replayed as `http.NoBody`, because servers ignore bodies on these verbs; else nil, so streaming POST/PATCH/PUT with `ContentLength > 0` — `rest.Client.Upload` of a content-library item — is neither buffered into memory nor retried. Decide the replay **after** the re-login, not before: a body that cannot be repeated must still heal the session, or an upload-only workload stays 401'ing until the keepalive ticks.

  **Verify**: `go build ./pkg/util/vsphere/client/... && make lint-go`

- [ ] **T014** Branch the REST assembly in `newRestClient` (`pkg/util/vsphere/client/client.go`).

  ```go
  keeper.setRestClient(restClient)
  restClient.Transport = newReloginREST(restClient.Transport, keeper)
  restClient.Transport = keepalive.NewHandlerREST(
      restClient, keepAliveIdleTime, keeper.restKeepAlive)
  ```

  Order matters: the first line makes the re-login wrapper inner, the second captures it as the handler's downstream (`keepalive.NewHandlerREST` reads `c.Transport` at construction). Then `restClient.Login(ctx, userInfo)` as today, which starts the ticker.

  `newRestClient` needs the keeper passed in. Leave the legacy arm unchanged.

  **Verify**: `go build ./... && go test ./pkg/util/vsphere/client/...`

- [ ] **T015** REST specs in `relogin_test.go`.
  - Requires the VAPI simulator, already blank-imported in `client_test.go`.
  - Cases: log in, log out (or terminate) to force a 401, then a tags/content-library call succeeds and the replayed request carried the **new** session id; a request to `restSessionPath` is never retried; a request with a streaming body and `GetBody == nil` returns the 401 unretried; `keeper.restKeepAlive` heals a dead session with no application traffic.
  - Assert the header rewrite with a counting `http.RoundTripper` spliced under `newReloginREST`, recording `req.Header.Get(restSessionHeader)` per attempt.
  - **Verify**: `go test ./pkg/util/vsphere/client/...`

---

## Phase 5 — Callers

- [ ] **T016** Read the feature state at the provider boundary.
  - `pkg/providers/vsphere/client/client.go`, in `NewClient`: add
    `InlineReloginEnabled: pkgcfg.FromContext(ctx).Features.VCSessionInlineRelogin,` to the `client.Config{...}` literal.
  - This is the **only** place the feature state is read. `pkg/util/vsphere/client` must stay free of `pkgcfg`.
  - **Verify**: `go build ./... && go test ./pkg/providers/vsphere/...`

- [ ] **T017** Mark the watcher's traffic no-replay.
  - `pkg/util/vsphere/watcher/watcher.go`, in `Start` (around line 400-440): wrap the context the goroutine uses — `ctx = pkgclient.WithNoReplay(ctx)` — before `ctx, cancel = context.WithCancel(ctx)`, so both `newWatcher`'s setup calls and the `WaitForUpdatesEx` loop inherit it.
  - **Verified**: no import cycle. `go list -deps ./pkg/util/vsphere/client | grep vsphere/watcher` prints nothing today, so `watcher` may import `client`. Re-run it if you add imports to `client` in an earlier phase; if it ever prints something, move `WithNoReplay`/`isNoReplay` into a tiny leaf package and import that from both.
  - `services/vm-watcher/vm_watcher_service.go` needs **no** change: its `IsNotAuthenticatedError` / `IsInvalidLogin` restart loop (lines 110-125) is still the correct recovery, and now runs against an already-re-authenticated session.
  - **Verify**: `go build ./... && go test ./pkg/util/vsphere/watcher/... ./services/vm-watcher/...`

- [ ] **T018** `[P]` Keepalive spec in `relogin_test.go`: with a short idle (`sim25.SetSessionTimeout` plus a sub-second `keepAliveIdleTime`, as `client_test.go` does), terminate the session from a second manager and assert — with **no** application call — that the session becomes valid again and pings resume. This is the arrangement-B property; it fails if the wrapper is placed outside the keepalive.
  - **Verify**: `go test ./pkg/util/vsphere/client/...`

- [ ] **T019** `[P]` Pin and extend the legacy coverage.
  - `pkg/util/vsphere/client/client_test.go`: the existing `Describe("Keepalive")` specs construct handlers by hand and are unaffected, but the `Describe("NewClient")` specs now run with `InlineReloginEnabled: false` implicitly. Make that explicit in the `client.Config` literal so a future default flip does not silently repoint them.
  - Add one spec asserting flag-off yields the legacy chain: assert `vimClient.RoundTripper` is a `*keepalive.HandlerSOAP` and that no `*reloginSOAP` is in the chain (compare against the inline case).
  - **Verify**: `go test ./pkg/util/vsphere/client/...`

---

## Phase 6 — Deployment and E2E

- [ ] **T020** No manager-overlay change. A capability-backed feature state has no env var to add to `config/wcp/vmoperator/manager_env_var_patch.yaml` or `config/local/vmoperator/local_env_var_patch.yaml` — the capability CR/ConfigMap in the target cluster is the only source of truth, per `pkg/config/capabilities/capabilities.go`. Keep this task number reserved so the phase numbering below does not shift.

- [ ] **T021** Run the existing vcsim suites in both modes.
  - `services/vm-watcher/vm_watcher_service_test.go` and `controllers/virtualmachine/virtualmachine/virtualmachine_controller_intg_test.go` both build clients via `vsclient.NewClient(ctx, vcSimCtx.VCClientConfig)`. Add a variant that sets `VCClientConfig.InlineReloginEnabled = true` before the call.
  - **Verify**: `go test ./services/vm-watcher/... ./controllers/virtualmachine/...`

- [ ] **T022** New E2E spec file `test/e2e/vmservice/vmservice/virtualmachine/vm_vcsession.go`.

  Full design — including both `It` blocks, the timing rationale, and the caveats — is in [`plan.md`](./plan.md) "E2E design". Implement it exactly as written there. Structural notes:

  - Follow the shape of `test/e2e/vmservice/vmservice/virtualmachine/vm_extraconfig.go`: a `VCSessionRecoverySpecInput` struct, an exported `VCSessionRecoverySpec(ctx, inputGetter)` func, `input`-validation `Expect`s in `BeforeEach`, `framework.WatchPodLogsAndEventsInNamespaces` with `DeferCleanup`.
  - Both `It` blocks carry `Label("experimental")` until hardware validation and the `Serial` decorator. They take VM Operator's VC session away from every other spec.
  - Register it in `test/e2e/vmservice/vmservice_test.go` under `Context("VIRTUAL-MACHINE", ...)` as `Context("VM-VCSESSION", ...)`, following the `VMExtraConfigSpec` registration block.
  - Verify the restart invalidates an admin session confirmed active beforehand, in addition to polling fresh-login availability; neither a transport error nor availability alone proves disruption.
  - `const vpxdRestartCmd = "vmon-cli -r vpxd"` — a single constant, because **this command needs validating on a real testbed**; `service-control --restart vmware-vpxd` is the documented fallback.
  - **Verify**: `cd test/e2e && go build ./...`, then on a testbed:
    ```bash
    make test-e2e TEST_FOCUS="VM-VCSESSION"
    ```

- [ ] **T023** `[P]` E2E gating constant.
  - `test/e2e/vmservice/consts/consts.go`: add `VCSessionInlineReloginCapabilityName = "supports_vc_session_inline_relogin"` alongside the other `*CapabilityName` constants.
  - No new skipper function: `vm_vcsession.go` calls the existing `skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.VCSessionInlineReloginCapabilityName)`, the same helper `vm_group.go` and `configpolicy.go` already use for capability-gated specs.
  - **Verify**: `cd test/e2e && go build ./...`

- [ ] **T024** Confirm the E2E actually discriminates. Run T022 against a build where the capability is **not activated** and confirm both blocks **fail**. A recovery test that passes in both modes is testing nothing. Record the observed timings in this file.

---

## Phase Final — Polish

- [ ] **T025** Re-login logging: `Info` level, with the generation and the triggering method name, so a burst of logins is visible as a burst rather than as one line repeated.
- [x] **T026** Move `vc-client-retry-login.md` (repo root, untracked) into this directory as `reference-wcpsvc-retry-login.md`.
- [ ] **T027** Update `docs/` if the capability is operator-facing, and write the release note per `pull-request-standards.md`.
- [ ] **T028** WCP activates the `supports_vc_session_inline_relogin` capability — separate change, after soak. Record the soak criteria in this file before requesting activation.
- [ ] **T029** Remove `SoapKeepAliveHandlerFn`, `RestKeepAliveHandlerFn`, and the `InlineReloginEnabled` field — separate spec, after the capability has been activated everywhere.
