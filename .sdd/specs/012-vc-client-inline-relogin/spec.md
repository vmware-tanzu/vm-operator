# Feature Specification: Inline vCenter Session Re-login

- **Feature branch**: `bryanv/vclient-inline-relogin`
  - **Fork**: `bryanv/vm-operator`
  - **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-09-03
- **Status**: Draft
- **Epic**: TBD
- **Design docs**: n/a

---

## Problem

VM Operator holds one long-lived vCenter client per process (`pkg/util/vsphere/client`). Its SOAP and REST sessions are kept alive by a **timer-driven** keepalive handler that pings VC every `keepAliveIdleTime` (5 minutes) and re-logs in only if that ping observes a dead session.

The session can die at any moment — `vpxd` restart, an admin terminating the session, an idle timeout, a VC upgrade. Between the moment the session dies and the moment the next timer fires, **every** VM Operator call to vCenter fails with `NotAuthenticated`. In the worst case that window is a full keepalive period. During it:

- every `VirtualMachine` reconcile fails and is rate-limited by controller-runtime;
- the vm-watcher service spins restarting its watcher;
- image-cache and content-library work stalls;
- the failures are indistinguishable, in logs and conditions, from a real authz problem.

The timer can also stop entirely: govmomi's keepalive goroutine calls `Stop()` and exits permanently the first time its `send` func returns an error, and nothing restarts it until a login body traverses the round-tripper chain again.

## Goals

- VM Operator **MUST** recover from an expired or terminated vCenter session on the **next call that needs it**, without waiting for a timer, for both SOAP/VIM and vAPI/REST traffic.
- Recovery **MUST** be invisible to callers: a call made against a dead session returns the successful result of the retried call, not an error.
- Recovery **MUST** be safe under concurrency: N reconcile threads faulting simultaneously **MUST** produce at most one re-login and one new VC session, not N.
- Recovery **MUST NOT** replay a request whose success depends on session-scoped server state (property collectors, property filters, container/list views). Those callers **MUST** continue to receive the fault so their own restart logic runs.
- Recovery **MUST NOT** recurse: a failing login **MUST NOT** be able to trigger another login attempt.
- When re-login fails, the caller **MUST** be able to see why — both the original fault and the login failure.
- The new behavior **MUST** be selectable by a Supervisor capability, defaulting to the existing timer-driven behavior while the capability is not activated, so both implementations coexist while the new one bakes.
- With the capability not activated, behavior **MUST** be byte-for-byte the behavior shipping today.
- Both modes **MUST** be covered by tests that fail if the recovery path silently stops working.

## Non-goals

- Retrying **transport-level** failures (connection refused, `EOF`, TLS errors, non-200 HTTP statuses). Those are not authentication problems and replaying them raises idempotency questions that this spec does not answer. A `vpxd` restart typically produces one transport error on the in-flight call and then a `NotAuthenticated` fault on the next call; only the second half is in scope.
- Changing how VC credentials are sourced, rotated, or how `UpdateVcCreds` / `UpdateVcPNID` tear down and rebuild the client.
- Changing the client's lifecycle (one cached client per process, rebuilt on credential/PNID change).
- Removing the timer-driven keepalive. It stays, in both modes.
- Recovering auth faults that arrive as **property-collector `MissingSet`** entries rather than SOAP faults (see `research.md` §5). Bounded by the keepalive that this spec keeps; a call-site fix is out of scope.
- Removing the capability gate. That is a follow-up spec once the new mode has soaked.

## User stories / acceptance criteria

### DevOps user

- **Given** a `VirtualMachine` is reconciling normally, **When** an administrator terminates VM Operator's vCenter session (or `vpxd` restarts), **Then** the next reconcile completes successfully and the VM's `Ready` condition never goes false for an authentication reason.
- **Given** the same scenario, **When** the user inspects the VM Operator logs, **Then** they see a single re-login entry, not a burst of one per reconcile thread.

### CSP admin

- **Given** VM Operator is running with the capability activated, **When** the admin lists vCenter sessions for the VM Operator solution user after a forced session loss, **Then** they see one session, not one per concurrent reconcile.
- **Given** VM Operator is running with the capability not activated, **When** the admin exercises the same scenario, **Then** VM Operator behaves exactly as it does in the currently shipping release.

### Platform engineer

- **Given** the vm-watcher service is watching VMs, **When** the session is terminated, **Then** `WaitForUpdatesEx` returns the authentication fault to the service loop (rather than being replayed against a property collector that no longer exists), the service restarts the watcher, and the watcher's first call succeeds against a freshly re-authenticated session.
- **Given** VM Operator credentials are invalid, **When** a call faults with `NotAuthenticated` and the re-login is rejected, **Then** the returned error carries both the original fault and the login failure, and no further login attempt is made for that call.

## Decisions

- **Flag placement**: a `pkgcfg.Features.*` entry (`VCSessionInlineRelogin`), sourced from the Supervisor `supports_vc_session_inline_relogin` capability — the same pattern the repo already uses for other capability-gated behavior (e.g. `VMGroups`, `VMEviction`), not an operational `pkgcfg.Config` bool like `AsyncSignalEnabled`.
- **VC operation ID**: the replayed request reuses the original operation ID unchanged. The replay runs on the same context, so this is the no-code-change outcome; it also means the fault and its recovery correlate to one op ID in `vpxd` logs. Do not add a retry suffix.

## Open questions

- [NEEDS CLARIFICATION: Epic ticket number. Required before this spec merges.]
