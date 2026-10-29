# Research: Inline vCenter Session Re-login

- **Spec**: [`spec.md`](./spec.md)
- **Date**: 2026-09-03

Prior art: the wcpsvc "re-login and retry in place" pattern, written up in [`reference-wcpsvc-retry-login.md`](./reference-wcpsvc-retry-login.md) (moved here from the repo root when this spec merged). That document was verified against **govmomi v0.55.0**. Everything below re-verifies its load-bearing claims against the version this repo actually builds with, **`github.com/vmware/govmomi v0.56.0-alpha.0.0.20260720221020-d993be43fe66`** (`go.mod:54`), and adds the VM Operator-specific findings the reference document could not know about.

---

## 1. What ships today

`pkg/util/vsphere/client/client.go` builds four clients from one config:

| Client | Field | Auth carrier | Round-trip path |
|---|---|---|---|
| `*vim25.Client` | `vimClient` | `vmware_soap_session` cookie in the `soap.Client` jar | `vimClient.RoundTripper` (wrappable) |
| `*rest.Client` | `restClient` | `vmware-api-session-id` header, cached in `rest.Client.sessionID` | `restClient.Transport` (`http.RoundTripper`, wrappable) |
| `*pbm.Client` | `pbmClient` | SOAP `Header.Cookie`, read live from the **vim25** jar | `pbmClient.RoundTripper` (wrappable) |
| `*find.Finder` / `*object.Datacenter` | | none of their own | ride on `vimClient` |

Both sessions are kept alive by `keepalive` handlers with **custom** `send` funcs — `SoapKeepAliveHandlerFn` (`client.go:91`) and `RestKeepAliveHandlerFn` (`client.go:121`) — that probe and re-login on a 5-minute timer (`keepAliveIdleTime`, `client.go:84`).

Two structural problems, both confirmed in the govmomi source:

1. **The timer is the only trigger.** Nothing re-authenticates between ticks.
2. **The keepalive goroutine is one error from dying.** `session/keepalive/handler.go` `Start()`: `if err := h.send(); err != nil { h.notifyWaitGroup.Done(); h.Stop(); return }`. It is revived only when a login body next traverses the handler.

## 2. Load-bearing govmomi mechanics — re-verified at v0.56.0-alpha

- **`vim25.Client.RoundTripper` is the injection point.** `vim25/client.go`: `func (c *Client) RoundTrip(ctx, req, res) error { return c.RoundTripper.RoundTrip(ctx, req, res) }`. Unchanged.
- **`session.NewManager` stores the `*vim25.Client` pointer** (`session/manager.go:49-60`), and `Manager.Login` issues `methods.Login(ctx, sm.client, &req)` (`session/manager.go:92`). So a re-login triggered from inside a wrapper **re-enters at the top of the chain**, above the wrapper. This is what makes the keepalive ticker restart on re-login, and it is the recursion hazard.
- **Generated method bodies use distinct request and response values.** `vim25/methods/methods.go`: `var reqBody, resBody CurrentTimeBody; reqBody.Req = req; r.RoundTrip(ctx, &reqBody, &resBody)`. Zeroing `res` before a replay therefore cannot clobber the request.
- **`CurrentTimeBody` (and every generated `*Body`) is `{ Req; Res; Fault_ *soap.Fault }`.** A successful second response does not clear `Fault_`, and `soap.Client.RoundTrip` returns `WrapSoapFault(f)` whenever `resBody.Fault() != nil` — so the stale fault from attempt 1 is re-reported unless the response is cleared. The reference implementation clears only `Fault_` by reflection; **zeroing the whole response struct is strictly better** — it also drops any partial `Res` and needs no field-name knowledge.
- **Fault matching.** `soap.Fault.Detail.Fault` is `types.AnyType` (i.e. `any`), so the XML decoder stores the **value** `types.NotAuthenticated`, not the pointer. `soap.ToVimFault` yields the pointer form. This repo already has the normalizing helper: `client.IsNotAuthenticatedError` → `fault.Is(err, &vimtypes.NotAuthenticated{})`, and `govmomi/fault.As` walks both forms plus wrapped errors (`fault/fault.go:35`). **Use the existing helpers; do not hand-roll a type switch.**
- **`simulator.FaultTypeNotAuthenticated` exists** (`simulator/fault_injection.go:180`) for injecting an auth fault into a chosen method.
- **The vcsim no-session allow-list is still there** (`simulator/simulator.go:196-203`): `Login`, `LoginByToken`, `LoginExtensionByCertificate`, `CloneSession`, `RetrieveServiceContent`, `RetrieveInternalContent`, `PbmRetrieveServiceContent`, `Fetch`, `RetrieveProperties`, `RetrievePropertiesEx`, `List`, `GetTrustedCertificates` — marked `// ok for now, TODO: authz`. Everything else faults with `NotAuthenticated` when there is no session.

## 3. VM Operator-specific finding: PBM is recoverable, unlike EAM

The reference document's `(retryLogin=true, retryRequest=false)` case exists because wcpsvc's EAM client caches the session cookie in private state at construction. **PBM does not.** `pbm/client.go:38-41`:

```go
sc := c.Client.NewServiceClient(Path, Namespace)
sc.Cookie = c.SessionCookie // method value bound to the vim25 soap.Client
```

`sc.Cookie` is a `func() *HeaderElement` that `soap.Client` calls on **every** request (`vim25/soap/client.go:736`), and `SessionCookie()` reads the vim25 client's live cookie jar (`:215`). So once the vim25 session is refreshed, a PBM replay carries the new cookie. PBM is therefore `(true, true)` — re-login **and** replay — provided we wrap `pbmClient.RoundTripper` (which dispatches through `pbm.Client.RoundTrip`, `pbm/client.go:53`) rather than leaving it pointed at the raw derived `soap.Client`.

Caveat: three call sites build **ad-hoc** PBM clients off the vim25 client and would not get the wrapper:

- `pkg/providers/vsphere/vmprovider_vm.go:733`
- `pkg/providers/vsphere/storage/provisioning.go:70`
- `pkg/vmconfig/volumes/unmanaged/register/unmanagedvolumes_register.go:351`

## 4. VM Operator-specific finding: the watcher must **not** be replayed

`pkg/util/vsphere/watcher/watcher.go` builds session-scoped server state on the shared `*vim25.Client` — container views (`:346`), a list view (`:354`), a property collector (`:359`), a property filter (`:365`) — and then loops on `methods.WaitForUpdatesEx(ctx, w.client, &req)` (`:464`) with a `This` pointing at that collector and a `Version` token that is only meaningful within that session.

Because `w.client` is the `*vim25.Client`, this traffic **does** go through the round-tripper chain. Replaying `WaitForUpdatesEx` after a re-login would target a collector that no longer exists. The correct behavior already exists: the fault propagates to `services/vm-watcher/vm_watcher_service.go:110-125`, which recognizes it via `IsNotAuthenticatedError` / `IsInvalidLogin` and restarts the watcher. Re-login is still desirable (the restart's `CreateContainerView` then succeeds); the **replay** is not.

Bodies bound to session-scoped MoRefs, i.e. re-login-but-do-not-replay:

`WaitForUpdatesExBody`, `WaitForUpdatesBody`, `CancelWaitForUpdatesBody`, `CreateFilterBody`, `DestroyPropertyFilterBody`, `DestroyPropertyCollectorBody`, `DestroyViewBody`, `ModifyListViewBody`.

Creation calls (`CreatePropertyCollectorBody`, `CreateContainerViewBody`, `CreateListViewBody`) are safe to replay — they build fresh state on the new session.

## 5. VM Operator-specific risk: property-collector `MissingSet`

`RetrievePropertiesEx` can return **HTTP 200 with no SOAP fault**, carrying `NotAuthenticated` inside `ObjectContent.MissingSet`; govmomi converts that to a `vimFaultError` in `vim25/mo/retrieve.go`, **above** the round tripper. `soap.IsSoapFault` is false for it and the wrapper never sees it.

This matters here more than in wcpsvc because VM Operator leans hard on the property collector (`property.DefaultCollector(...)`, `find.Finder`, `object.*.Properties`, `mo.RetrieveProperties`). Under vcsim it is guaranteed (those methods are on the allow-list in §2); under real `vpxd` an expired session is expected to be rejected at the session layer with a top-level fault, but that was **not** reproduced here and should not be assumed.

Two consequences:

1. **Tests must not drive the retry with a property-collector call** — they will pass through the allow-list and the retry will never fire. Use a non-exempt method (`CurrentTime`) or `simulator.FaultTypeNotAuthenticated`.
2. **Keep the keepalive.** It bounds the exposure of this hole to one keepalive period, which is exactly the status quo. Dropping it in favour of "pure inline" would make this hole unbounded.

## 6. Keepalive placement: measured, and why arrangement B

The reference document measured both orderings. Restated in this repo's terms, with `KA` = keepalive handler, `RL` = re-login wrapper:

| Arrangement | Ping travels through | Behavior after the session dies |
|---|---|---|
| `RL(KA(soap))` — what wcpsvc builds | raw `soap.Client` | ping faults, `send` returns error, goroutine `Stop()`s and **exits**; nothing pings again until a real request faults, recovers, and its login body restarts the ticker |
| `KA(RL(soap))` — **recommended here** | the re-login wrapper | the ping itself faults, re-logs in, returns success; session heals with **no application request**, ticker survives |

Arrangement B is the right default for VM Operator: it keeps a warm session (fewer VC session-list churn events for CSP admins), it keeps the ticker alive as the backstop for the `MissingSet` hole in §5, and it still recovers instantly on the application path.

Both arrangements require the handler to be **in the chain before the first login** — the ticker is started only by a `LoginBody` traversing `HandlerSOAP.RoundTrip` (`session/keepalive/handler.go`, `Start()` on `*methods.LoginBody | LoginExtensionByCertificateBody | LoginByTokenBody`).

## 7. REST is a different animal

- Auth is the `vmware-api-session-id` **header**, set by `rest.Client.Do` from the **cached** `c.sessionID` (`vapi/rest/client.go:163-165`, `:60`). The header is on the request before our `http.RoundTripper` sees it, so a replay **must** rewrite it after re-login.
- `internal.SessionCookieName` (`"vmware-api-session-id"`) and `internal.SessionPath` (`"/com/vmware/cis/session"`) live under `github.com/vmware/govmomi/vapi/internal` — **not importable from this repo**. Both constants have to be redeclared locally with a comment pointing at the govmomi source.
- Login, logout and the session probe all hit the same path — `POST /rest/com/vmware/cis/session`, `DELETE` on it, and `POST …?~action=get` — so a single path check excludes all three from the retry logic. That is the REST re-entrancy guard, and it is the same guard `keepalive.HandlerREST.RoundTrip` uses.
- **Body replay is conditional.** `rest.Resource.Request` builds bodies with `encode()` → `*bytes.Buffer`, which `http.NewRequest` special-cases into a non-nil `GetBody`; those replay cleanly. Empty bodies (`io.MultiReader()`) replay trivially. But `rest.Client.Upload` → `soap.Client.Upload` (`vim25/soap/client.go`) passes an arbitrary `io.Reader` with `GetBody == nil` — that must **not** be buffered into memory, and must **not** be retried.
- `rest.Client.Session()` swallows a 401 and returns `(nil, nil)` (`vapi/rest/client.go:337-343`). So the REST keepalive's default `send` cannot heal itself through the wrapper the way the SOAP one can — REST needs a keeper-aware `send`, essentially today's `RestKeepAliveHandlerFn` routed through the shared login mutex.

## 8. Gaps in the reference implementation this plan closes

Numbered as in `reference-wcpsvc-retry-login.md` §10:

1. **No serialization around `Login`** — N faulting goroutines produce N logins and N VC sessions. Closed here with a mutex plus a session generation counter, so late arrivals observe that someone already re-logged in and skip straight to the replay.
2. **Login errors discarded** in favour of the original fault. Closed with `errors.Join`.
3. **Exactly one retry.** Kept. A `NotAuthenticated` fault is raised at the session layer before the method dispatches, so the server did not act and the replay is safe; a second attempt would only cover a login race that the generation counter already covers.
4. **Recursion bounded only by fault-type coincidence.** Closed with an explicit body-type deny-list for login/logout/session bodies, rather than relying on `vpxd` never answering a rejected login with `NotAuthenticated`.
5. **No direct unit test of the retry path.** Closed by the test plan in `plan.md`.

## 9. Live vCenter validation — 2026-10-24

Validated the production client from this branch against vCenter **9.2.0, build 25863067**, using the pinned govmomi dependency and Go's race detector. The standalone command in [`hack/vcresearch/session-relogin`](../../../hack/vcresearch/session-relogin/main.go) imports `pkg/util/vsphere/client`; it does not copy its recovery implementation. Public round-tripper observers count login attempts. The sanitized results are in [`live-validation.json`](./live-validation.json).

All 19 checks passed, including setup and cleanup:

- SOAP, REST, and PBM recover after confirmed session loss. A second REST expiration triggers a second recovery. The `CurrentTime` request used by SOAP keepalive also recovers.
- Twenty concurrent SOAP callers and twenty concurrent REST callers each produce exactly one recovery login, and all calls succeed. Measured recovery calls, including these concurrent batches, take approximately 0.2–0.6 seconds on this testbed.
- `WithNoReplay` and `WaitForUpdatesEx` preserve the original authentication fault while refreshing the session; subsequent calls work.
- Password rotation rejects the client's cached credentials with exactly one SOAP and one REST login attempt. SOAP retains both `NotAuthenticated` and `InvalidLogin`; REST returns 401. A newly built client succeeds with the new password.
- Disabling the account and inducing lockout both reject recovery. Re-enabling or unlocking it lets the same client recover. SSO account state is checked explicitly. The existing policy (five failures within 180 seconds, automatic unlock after 300 seconds) is unchanged.
- Legacy mode returns authentication faults with zero inline login attempts.
- The temporary SSO user, its inventory permission, and its sessions are cleaned up. Each run journals its principal before creation; `-cleanup` can retry cleanup after interruption.

**Disruption must be observed, not assumed.** On this VC, `TerminateSession` returns and `UserSession` becomes nil roughly three seconds before protected requests begin returning `NotAuthenticated`. Immediate calls can therefore succeed without exercising recovery. The command waits until a raw, non-retrying `CreateContainerView` request faults before timing the production client's recovery. An early inference that `CurrentTime` did not require authentication was disproved once this propagation delay was accounted for.

To rerun, create a private JSON file containing `host`, `username`, and `password`, then run:

```sh
go run -race ./hack/vcresearch/session-relogin \
  -credentials /private/vc.json \
  -exercise -journal /private/new-run-state.json
```

Omit `-exercise` for inspection. Use `-cleanup -journal /private/new-run-state.json` to retry cleanup. Credentials and generated test passwords are excluded from reports and journals. The command creates a local SSO person user with the built-in ReadOnly role; it does not alter the operator's account or SSO policies.

This validates the password-authenticated client library, not deployed Supervisor reconciliation, certificate-based solution users, natural idle expiration, or the five-minute background keepalive tick. No vpxd restart was performed. Both cluster E2E tests remain experimental until their full workflows are validated on a WCP testbed.
