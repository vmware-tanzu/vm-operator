# Inline `NotAuthenticated` Retry for govmomi VC Clients

**Read this when porting wcpsvc's "re-login and retry the request in place" behaviour into another
service, or when debugging why a session-expiry did (or did not) recover on its own.**

Audience: an agent working in a *different* codebase that talks to vCenter over govmomi. The
mechanics below are govmomi-level and portable; wcpsvc paths are cited as the reference
implementation.

Verified against **govmomi v0.55.0** (the version in `src/server/go.mod`). Every "observed" claim in
this document was reproduced against `vcsim` with a spy `soap.RoundTripper`; the measured sequences
are quoted inline. Line numbers are as of this writing — re-grep if they drift.

---

## 1. The goal

A long-lived VC client holds a SOAP session cookie. Sessions expire (idle timeout, `vpxd` restart,
an admin terminating the session, a service-account token rolling over). When that happens, the
next call fails with a `NotAuthenticated` fault. Without help, every caller in the process has to
know how to re-authenticate.

The pattern: wrap the client's `RoundTripper` so a `NotAuthenticated` fault is turned into
*re-login + replay the same request*, invisibly to the caller. Callers see one successful response,
not an error.

Reference implementation:

| Piece | File |
|---|---|
| Chain assembly | `src/server/vclib/vc_client_factory.go:244` (`newVcClient`) |
| The round tripper | `src/server/pkg/vsphere/vmodl1/roundtrip_retry.go` |
| Login + fault classification | `src/server/pkg/vsphere/vmodl1/session.go` |
| Token acquisition (the env-specific seam) | `src/server/pkg/ssolib/sts.go:85` (`GetHOKSigner`) |

---

## 2. The object graph, and the one mechanism everything rests on

```
soap.Client                      // owns http.Transport, TLS config, cookie jar; IS a soap.RoundTripper
  └─ vim25.Client                // embeds *soap.Client + ServiceContent + a *separate* RoundTripper field
       └─ govmomi.Client         // embeds *vim25.Client + *session.Manager
```

`vim25.Client.RoundTrip` does nothing but dispatch to its own `RoundTripper` field
(govmomi `vim25/client.go`). That field is the injection point.

**The load-bearing fact:** `session.NewManager(vim25Client)` stores the *pointer* to that
`vim25.Client` (govmomi `session/manager.go`). So when you later mutate
`vim25Client.RoundTripper`, you retroactively change what the SessionManager's *own* calls
(`LoginByToken`, `Logout`, `UserSession`) travel through. `session.Manager.LoginByToken` calls
`methods.LoginByToken(ctx, sm.client, &req)` — i.e. through `sm.client.RoundTripper`, the wrapped
chain.

Consequence: **a re-login triggered from inside your round tripper re-enters at the *top* of the
chain, not below itself.** Everything in §4 follows from this.

In wcpsvc, `newGovmomiClient` (`vc_client_factory.go:228`) builds the graph and `newVcClient`
(`:244`) mutates `vcClient.RoundTripper` — which, through the embedded `*vim25.Client`, is the same
field the SessionManager will use.

---

## 3. Assembling the chain

```go
// vc_client_factory.go:244-256
func newVcClient(ctx context.Context, vcClient *govmomi.Client, sess vmodl1.LoginInvoker) (*VimClient, error) {
	if err := vcClient.Client.UseServiceVersion(); err != nil {   // plain HTTP GET, no session needed
		return nil, err
	}

	keepAliveRoundTripper := session.KeepAlive(vcClient.Client.Client, sessionIdleTime) // wraps the *soap.Client*
	vcClient.RoundTripper = vmodl1.NewRetryLoginRoundTripper(sess, keepAliveRoundTripper)

	return &VimClient{Client: vcClient}, sess.Login(ctx)          // note: non-nil client AND a possible error
}
```

Four things to carry over:

1. `UseServiceVersion()` must run **before** login. It is an unauthenticated `GET
   /sdk/vimServiceVersions.xml` that sets the client's namespace/version; doing it after login is
   harmless but pointless, doing it never leaves you pinned to govmomi's compiled-in version.
2. `session.KeepAlive(rt, idle)` is `keepalive.NewHandlerSOAP`. Its argument does **double duty**:
   it is both the downstream RT for application requests *and* the RT its background ping uses.
   Passing `vcClient.Client.Client` (the raw `soap.Client`) means the ping bypasses the retry
   wrapper. See §4.
3. The **keepalive handler must already be in the chain before the first login**, because its
   goroutine is started only by a login body traversing it (`keepalive/handler.go`: `Start()` on
   `*methods.LoginBody | LoginExtensionByCertificateBody | LoginByTokenBody`, `Stop()` on
   `LogoutBody`). Log in first and wrap afterwards and you get a client whose session is never kept
   alive. Where the handler sits *relative to the retry wrapper* is a separate question — §4.
4. `newVcClient` returns a **non-nil client alongside a possibly non-nil error** (`return
   &VimClient{...}, sess.Login(ctx)`). Callers must check the error — `InitPersistentVCClient`
   (`vclib/client.go:95`) loops on it — and must not treat "client is non-nil" as success.

`sessionIdleTime` is 10 minutes (`vclib/client.go:60`); the keepalive ping period, not a session
lifetime. Note the ticker fires every `idle` unconditionally — it is not reset by application
traffic.

---

## 4. What the "ordering matters" comment actually buys (measured)

The code comment at `vc_client_factory.go:249` says the KeepAlive RT must stay inside so it keeps
pinging "even in the event we need to reauthenticate". The precise behaviour is more specific than
that, and one plausible reading of it is **wrong**:

**Not the reason:** "keepalive must be inner so the re-login starts the ticker." Observed: with
keepalive *outside* (`keepAlive(retryLogin(soap))`) the ticker starts too — because
`SessionManager.Login` re-enters at the top of the chain (§2), so the login body traverses the
keepalive handler in *either* arrangement.

**The real difference is where the ping goes**, and it decides how a dead session heals:

| Arrangement | Ping travels through | Behaviour after the session dies |
|---|---|---|
| `retryLogin(keepAlive(soapClient))` — what wcpsvc builds | raw `soap.Client` | Ping gets `NotAuthenticated`, `h.send` returns an error, the goroutine calls `Stop()` and **exits permanently**. Nothing pings again until some real request faults, re-logs in, and that login body restarts the ticker. |
| `keepAlive(retryLogin(soapClient))` | the retry wrapper | The ping itself faults, re-logs in, and returns success — the session heals with **no application request at all**, and the ticker keeps running. |

Measured with a 60 ms idle and an admin-terminated session (spy counts of bodies on the wire):

- Arrangement A: pings `4 → 5 → 5` (goroutine dead), then one application call re-logs in and pings
  resume `5 → 11`.
- Arrangement B: with no application call at all, `LoginByToken` count `1 → 2` and pings `4 → 11`.

So wcpsvc's recovery loop is: **keepalive dies quietly → the next real request eats one
`NotAuthenticated` and recovers inline → the recovery's login body revives the keepalive
goroutine.** That is self-healing, but it is *request-driven*, and between the ping failing and the
next request there is no live session. If your service has long idle stretches and cares about
holding a warm session, prefer arrangement B (or pass a custom keepalive handler that re-logs in —
which is exactly what the sibling LS client does, `pkg/lshelper/keepalive_handler.go:62`, wired at
`vclib/client.go:1694`).

Either way, the handler has to be in the chain before the first login (§3.3) — that requirement is
independent of the arrangement you pick.

---

## 5. The `LoginInvoker` contract — two booleans, not one

```go
// roundtrip_retry.go:16
type LoginInvoker interface {
	Login(ctx context.Context) error
	IsLoginRetryRequired(fault *soap.Fault) (retryLogin, retryRequest bool)
}
```

The two booleans are independent on purpose:

| Return | Meaning | Used by |
|---|---|---|
| `(true, true)` | Re-authenticate **and** replay the request. | VIM/SOAP calls on `NotAuthenticated` (`session.go:30`) |
| `(true, false)` | Re-authenticate but **do not** replay. | EAM (`eamlib/client.go:76`) — a derived service client (`eam.NewClient(...)`) caches the session cookie in private state at construction time. Logging in on the underlying transport does not update it, so replaying now would fail again with the stale cookie. The request can only succeed after the *service client* is recreated, which happens on the caller's next attempt. |
| `(false, false)` | Not an auth problem; pass the fault through. | default |

If you are multiplexing several service clients (EAM, PBM, VSAN, lookup…) over one vim25 transport,
you almost certainly need the `(true, false)` case. Getting this wrong produces a confusing symptom:
a "retry" that always fails identically.

The dispatch loop itself (`roundtrip_retry.go:44`):

```go
err := rl.rt.RoundTrip(ctx, req, res)
if err == nil || !soap.IsSoapFault(err) { return err }        // (a)

retryLogin, retryRequest := rl.invoker.IsLoginRetryRequired(soap.ToSoapFault(err))
if retryLogin && rl.invoker.Login(ctx) != nil { return err }  // (b) original fault, not the login error
if retryRequest {
	if err := resetFault(res); err != nil { return err }       // (c) see §7
	err = rl.rt.RoundTrip(ctx, req, res)
}
return err
```

- (a) Only SOAP faults are considered. Network errors, TLS failures and non-200 HTTP statuses pass
  straight through — deliberately, since those are not auth problems.
- (b) When login fails, the caller sees the **original** `NotAuthenticated`, not the login failure.
  The login error is only logged (`session.go:67`, `:75`). Budget for this when debugging: the
  interesting error is in the log line above, not in the returned error.
- (c) Exactly **one** retry. No backoff, no attempt cap needed because there is no loop.

SOAP needs no request-body bookkeeping for the replay: `req` is a Go struct that the soap client
re-marshals on each round trip. This is the one concrete thing that differs when adapting the
pattern to an `http.RoundTripper` — there you must clone the request and buffer/restore the body
before the first attempt (`vapi/rest/client.go:120-157`).

---

## 6. Matching the fault: value, not pointer

```go
// session.go:30
switch err.VimFault().(type) {
case govmomiTypes.NotAuthenticated:      // value type — correct
	loginRequired, retryRequired = true, true
}
```

`soap.Fault.Detail.Fault` is typed `types.AnyType`, which is `any` (govmomi `vim25/types/base.go`).
When govmomi's XML decoder resolves an `xsi:type` into an empty-interface field it stores the
**struct value**, because the concrete type trivially satisfies `any` (`vim25/xml/read.go`, the
`typ.Implements(val.Type())` branch). Observed: `soap.ToSoapFault(err).VimFault()` has dynamic type
`types.NotAuthenticated`; a `case *types.NotAuthenticated:` arm does **not** match.

Contrast `soap.ToVimFault(err)`, whose field is typed `types.BaseMethodFault` — an interface
satisfied only by the pointer — so *that* helper yields `*types.NotAuthenticated`. Mixing the two
up is the classic way to write a retry that silently never fires. (`eamlib/client.go:78` lists both
`eamtypes.EamInvalidLogin` and `*eamtypes.EamInvalidLogin`; harmless belt-and-braces.)

Portable guidance: match both forms, or use `fault.Is(err, &types.NotAuthenticated{})` from
govmomi's `fault` package, which normalises this.

Also note the switch matches **exactly** `NotAuthenticated`. Its parent `NoPermission`, and
`InvalidLogin` / `ExpiredFault` / `SecurityError`, do **not** trigger a retry. That is intentional
for `NoPermission` (a real authz failure must not loop) and it is what keeps a *failed login* from
recursing in practice: VC does not answer a rejected `LoginByToken` with `NotAuthenticated` (it
faults with `InvalidLogin` and friends), so the login attempt falls to `default`. Treat that last
sentence as unverified inference about real `vpxd` behaviour — it was not reproduced here — and rely
on the rule rather than the fault name. Recursion is a latent hazard: since `Login` re-enters the top of the chain
(§2), any fault type you classify as retry-worthy that VC can *also* return from `LoginByToken`
gives you unbounded recursion. Keep the retryable set disjoint from the set of faults login itself
can produce, or add an explicit re-entrancy guard.

---

## 7. `resetFault`, and the constraint it imposes on response types

`roundtrip_retry.go:71` clears the response's fault field by reflection before the replay:

```go
faultVal := reflect.ValueOf(res).Elem().FieldByName("Fault_")
faultVal.Set(reflect.Zero(faultVal.Type()))
```

This is not cosmetic. govmomi's soap client returns `WrapSoapFault(f)` whenever
`resBody.Fault() != nil` after decoding, and XML unmarshalling of a *successful* second response
does not clear a field that the successful response has no element for. So the stale fault from
attempt 1 survives into attempt 2's response struct and is re-reported.

Observed, with `resetFault` removed: the wire sequence is
`CurrentTime → LoginByToken → CurrentTime` — the retry really happened and really succeeded — yet
the caller gets `ServerFaultCode: NotAuthenticated`. With `resetFault` in place the same sequence
returns success.

Constraint: this only works on generated `methods.*Body` structs, which all have a settable
`Fault_` field. Hand-rolled or wrapped response types will hit the
`fault missing Fault_ prefix` / `unable to set fault` error paths and the request will *not* be
retried. If you introduce your own `soap.HasFault` implementations, either give them a `Fault_`
field or add an interface (`ClearFault()`) and prefer it over reflection.

---

## 8. `Login`, and the environment-specific seam

```go
// session.go:40
func (s *SessionManager) Login(ctx context.Context) error {
	c := &vim25.Client{ /* soap.Client as its own RoundTripper */ }
	token, err := s.Signer(ctx, true)                       // force-refresh the token
	if err != nil { return err }
	header := soap.Header{Security: token}
	return s.Client.SessionManager.LoginByToken(c.WithHeader(ctx, header))
}
```

**The `Signer` is the seam you replace.** wcpsvc injects `ssolib.GetHOKSigner`
(`vc_client_factory.go:128`), which acquires a Hold-of-Key SAML token for the `wcp` solution user
from the service-account token client, with a 10-minute lifetime and an in-process cache
(`pkg/ssolib/sts.go:85`). Two details worth copying:

- Re-login passes `forceRefresh = true`. A stale-but-unexpired cached token is exactly what you do
  *not* want when recovering from `NotAuthenticated`.
- The signer is a field (`Signer func(ctx, forceRefresh) (*sts.Signer, error)`), not a package
  call — that is what makes the round tripper testable against `vcsim` (issue a bearer token from
  `sts/simulator`). Keep the indirection.

If your service authenticates differently (username/password, extension certificate, an existing
session cookie), only `Login` changes; `IsLoginRetryRequired` and the round tripper are unaffected.

**Do not copy the comment at `session.go:47-64`.** It says a separate `vim25.Client` is used for
login "to avoid recursion", and that is not what the code achieves: the login is issued via
`s.Client.SessionManager`, whose own `*vim25.Client` is the wrapped one, so the login body goes
through the full chain. Observed: the spy sees `*methods.LoginByTokenBody` on the wrapped chain, and
the keepalive handler (which sits inside the chain) starts its ticker as a result — which would be
impossible if login bypassed the chain. The locally-built `c` is used only for
`c.WithHeader(ctx, header)`, a `context.WithValue` on the *same* embedded `soap.Client`; it is
inert. Recursion is prevented by fault-type disjointness (§6), not by this client.

---

## 9. Paths that bypass the retry (know these before trusting it)

- **Non-SOAP failures.** Network/TLS/HTTP-status errors are passed through untouched (§5a). A VC
  behind a restarting Envoy sidecar surfaces as a transport error, not a fault — no retry.
- **Property-collector "missing property" faults.** `RetrievePropertiesEx` can return **HTTP 200
  with no SOAP fault**, carrying `NotAuthenticated` inside `ObjectContent.MissingSet`. govmomi
  converts the first such entry into a `vimFaultError` *above* the round tripper
  (`vim25/mo/retrieve.go`, `ObjectContentToType` → `soap.WrapVimFault`). The round tripper never
  sees a fault, so **no re-login and no retry happen**. Observed under vcsim: an unauthenticated
  `property.Collector.RetrieveOne` returns `soap.vimFaultError` wrapping
  `*types.NotAuthenticated`, `soap.IsSoapFault(err) == false`, and the spy records a single
  `*methods.RetrievePropertiesExBody`.
  In vcsim this is partly a simulator simplification — `RetrieveProperties`/`RetrievePropertiesEx`
  are on an explicit no-session allow-list (`simulator/simulator.go`, marked `TODO: authz`) and the
  per-property fault is synthesised in `simulator/property_collector.go` — so do not conclude that
  production VC behaves identically for an expired session. But do treat "auth fault arrives as a
  vim fault, not a soap fault" as a real shape your code can meet, and handle it at the call site
  (or with `fault.Is`) rather than assuming the round tripper caught it.
- **Derived service clients** (EAM/PBM/etc.) — see the `(true, false)` case in §5. The retry cannot
  help them; the caller must recreate the client.
- **`vAPI`/REST traffic** does not go through this chain at all. It has its own wrappers (§11).

---

## 10. Gaps in the reference implementation — fix these while adopting

1. **No serialisation around `Login`.** `retryLogin.RoundTrip` has no mutex. N goroutines that hit
   `NotAuthenticated` at once each fetch a token and each call `LoginByToken`, producing N sessions
   on VC (all but the last orphaned until they idle out) and N STS round trips. The fix already
   exists in the sibling REST client — a `loginMu sync.Mutex` held across the re-login
   (`vapi/rest/client.go:50`, `:140-146`). Add the same, and consider collapsing concurrent waiters
   (re-check whether another goroutine already refreshed before logging in again).
2. **Login errors are discarded** in favour of the original fault (§5b). Prefer wrapping/joining so
   the caller can see *why* recovery failed.
3. **Exactly one retry.** Fine for session expiry; insufficient if you also want to cover a
   `vpxd` restart window. If you add attempts, add backoff and a cap — and remember each attempt
   re-marshals the same `req`, so it must be idempotent-safe for your call set. (Replaying a
   *task-creating* method after a fault the server may have partially processed is a business
   decision, not a transport one.)
4. **Recursion is bounded only by fault-type coincidence** (§6).
5. **`retryLogin.RoundTrip` has no direct unit test** in wcpsvc; `vc_client_factory_test.go` uses a
   `fakeLoginInvoker` that returns `(false, false)`, so the retry path is never exercised there, and
   `pkg/vsphere/vmodl1/session_test.go` only covers `Login`. Write the test (§12) — the whole
   feature is invisible when it silently stops working.

---

## 11. Sibling patterns in this repo (pick the closest analogue)

| Transport | Where | Trigger | Notable |
|---|---|---|---|
| SOAP / VIM | `pkg/vsphere/vmodl1/roundtrip_retry.go` + `session.go` | `NotAuthenticated` soap fault | re-login + replay; `resetFault`; no mutex |
| SOAP / EAM | `eamlib/client.go:76` | `EamInvalidLogin` | re-login **without** replay (stale cached cookie in the derived client) |
| vAPI REST (`http.RoundTripper`) | `vapi/rest/client.go:116` | HTTP 401 | clones request + backs up/restores body; `loginMu`; rewrites `vmware-api-session-id` before replay |
| vAPI REST (session-id oriented) | `pkg/vcrestlib/client.go:120` | HTTP 401, plus a `InvalidGrant` 500 special case | renews session id with compare-and-set (`setSessionIDIfEqual`) and deletes the loser session — the cleanest concurrency story of the four |
| Lookup Service SOAP | `pkg/lshelper/keepalive_handler.go:62`, wired at `vclib/client.go:1694` | `NotAuthenticated` seen *by the keepalive ping* | re-logins from inside the keepalive handler, i.e. heals without an application request |

---

## 12. Testing it (and the vcsim trap)

Test against `vcsim` with a counting `soap.RoundTripper` spliced in as the wrapper's *underlying*
RT; assert on the sequence of body types. The recipe that works:

```go
func TestRetryLogin(t *testing.T) {
	simulator.Run(func(ctx context.Context, c *vim25.Client) error {
		c2, err := vim25.NewClient(ctx, soap.NewClient(c.URL(), true)) // fresh, unauthenticated
		require.NoError(t, err)
		g := &govmomi.Client{Client: c2, SessionManager: session.NewManager(c2)}
		spy := &spyRT{inner: c2.Client}
		sm := &SessionManager{Client: g, Signer: bearerSigner(c)}
		c2.RoundTripper = NewRetryLoginRoundTripper(sm, spy)

		// GetCurrentTime is NOT on vcsim's no-session allow-list, so it returns a
		// top-level SOAP NotAuthenticated fault -- the production shape.
		_, err = methods.GetCurrentTime(ctx, c2)
		require.NoError(t, err)
		require.Equal(t, []string{
			"*methods.CurrentTimeBody", "*methods.LoginByTokenBody", "*methods.CurrentTimeBody",
		}, spy.seq())
		return nil
	})
}

// spyRT records fmt.Sprintf("%T", req) for every body it forwards; seq() returns
// a copy of those strings (guard the slice with a mutex -- the keepalive
// goroutine calls RoundTrip too).
func bearerSigner(c *vim25.Client) func(context.Context, bool) (*sts.Signer, error) {
	return func(ctx context.Context, _ bool) (*sts.Signer, error) {
		stsClient, err := sts.NewClient(ctx, c)
		if err != nil {
			return nil, err
		}
		return stsClient.Issue(ctx, sts.TokenRequest{Userinfo: simulator.DefaultLogin})
	}
}
```

Notes:

- Blank-import `_ "github.com/vmware/govmomi/sts/simulator"` and
  `_ "github.com/vmware/govmomi/lookup/simulator"` so the simulator serves STS; the signer above is
  the same trick `pkg/vsphere/vmodl1/session_test.go:29` uses.
- **The trap:** do *not* drive the test with a property-collector call (`finder.*`,
  `property.Collector.Retrieve*`, `object.*.Properties`). Under vcsim those succeed at the SOAP
  layer and deliver `NotAuthenticated` via `MissingSet`, so your retry never fires and the test
  fails for a reason that has nothing to do with your code (§9).
- To exercise session *expiry* rather than "never logged in", log in, read
  `sm.UserSession(ctx).Key`, then terminate it from a second admin client:
  `session.NewManager(admin).TerminateSession(ctx, []string{key})`.
- To exercise keepalive behaviour, build the handler with a short idle
  (`session.KeepAliveHandler(rt, 60*time.Millisecond, nil)`) and count `CurrentTimeBody` entries in
  the spy over a few hundred milliseconds.
- `simulator.FaultTypeNotAuthenticated` (`simulator/fault_injection.go`) can inject an auth fault
  into a specific method if you need one on a call that vcsim would otherwise allow.

---

## 13. Client lifecycle context (why this matters more than it looks)

Creating one of these clients is expensive: each `soap.NewClient` owns its own `http.Transport` and
therefore its own connection pool, and construction does a service-content fetch, a version probe,
an STS token acquisition and a login. wcpsvc creates VC clients in exactly two situations:

- at startup, retrying until success — `InitPersistentVCClient` (`vclib/client.go:95`), registered
  as a process-wide singleton in the service registry;
- on a trusted-roots change notification, and only when TLS is actually in use —
  `refreshPersistentVCClient` (`vapi/impl/wcp/notifications/certificates.go:87`), which builds a
  *new* client and swaps the singleton.

Two rules follow, and they are the reason the inline retry has to exist at all:

- **Consumers must not cache the client**; they fetch the current singleton per operation, so a CA
  rotation is picked up without restarting anything (in-flight calls finish on the old client).
- **Session recovery cannot be "recreate the client"** — that is far too expensive for an expiry, so
  it has to happen inside the round tripper.

If your service creates clients per request, most of this pattern is unnecessary; if it holds
long-lived clients, all of it applies.

---

## 14. Adoption checklist

1. Build `soap.Client → vim25.Client → govmomi.Client`, keeping the `session.Manager` bound to the
   same `*vim25.Client` you will mutate.
2. `UseServiceVersion()`.
3. Implement `Login` against your credential source; keep the token getter injectable and
   force-refresh on re-login.
4. Implement `IsLoginRetryRequired`: exact `NotAuthenticated` (value *and* pointer forms, or
   `fault.Is`) → `(true, true)`; every fault your login call can itself return → `(false, false)`;
   any derived-service-client auth fault → `(true, false)`.
5. Decide the keepalive arrangement using §4 — request-driven healing (wcpsvc) vs ping-driven
   healing (LS client). Install the chain **before** the first login either way.
6. Add the login mutex from day one (§10.1).
7. Assign `vim25Client.RoundTripper = NewRetryLoginRoundTripper(invoker, keepAliveRT)`, then log in,
   and propagate the login error.
8. Test the retry path with the recipe in §12, avoiding the property-collector trap.
9. Audit call sites that can receive an auth fault as a *vim* fault rather than a soap fault (§9).
