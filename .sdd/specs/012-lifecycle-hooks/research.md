# Research: Blocking Lifecycle Hooks

- **One Pager: VM Service: Lifecycle Stages and Hooks** — internal design doc. Business problem, goals, and the initial resource model (`LifecycleStages`, `LifecycleHook`, `LifecycleState`, an "Aggregated LifecycleHook"/"Aggregated LifecycleState" concept). Explicitly states (in its Non-goals) that the customer-integration mechanism is documented separately.
- **[API design] Blocking Lifecycle stages** — internal design doc. The authoritative, detailed API design: full CRD schemas (OpenAPI) for `LifecycleStages`, `LifecycleState`, `LifecycleHook` (group `lifecycle.vcfa.vmware.com`), plus `Subscription` and `Event` (group `eventing.vcfa.vmware.com`) used by a separate eventing/notification system.

## Discrepancy between the two docs

The one-pager's `LifecycleHook` is per-VM: namespaced, owned by the target `VirtualMachine`, embedding a `spec.stages[]` list with `pauseWorkflow`/`workflowPaused` flags directly on the hook. The API-design doc instead scopes `LifecycleHook` per `(target.group, target.kind, stage)` — one hook registers interest in a single stage for an entire target kind in a namespace, not a single VM — and moves the per-VM pause/resume state into a separate `LifecycleState` resource (owned by the target object, one per VM) whose `status.hooks[]` lists progress for every hook watching that VM's current stage.

Per the one-pager's own cross-reference, the API-design doc is the authoritative mechanism spec. **This spec follows the API-design doc's schema.** The one-pager's `LifecycleHookTemplate`/per-VM-hook shape is treated as an earlier draft, not implemented.

## Ownership boundary

`LifecycleStages`, `LifecycleHook`, and `LifecycleState` (group `lifecycle.vcfa.vmware.com`) are defined and reconciled by a separate **Lifecycle Operator** — not this repository. The Lifecycle Operator:
- Copies `LifecycleHookTemplate`/CRD-driven defaults into per-namespace `LifecycleHook` resources (per the one-pager) — out of scope here regardless, since that flow lives entirely in the Lifecycle Operator.
- Watches `LifecycleHook` create/update/delete and syncs the corresponding entries into every matching `LifecycleState`.
- Runs the hook timeout timer and flips `LifecycleState` status when a hook's deadline passes.
- Talks to the separate eventing system (`Subscription`/`Event`, group `eventing.vcfa.vmware.com`) to notify hook owners — entirely out of scope for VM Operator.

**VM Operator's role is consumer-only**: at each defined stage checkpoint, look up (or create) the `LifecycleState` for the `VirtualMachine` being reconciled, read `status.stages[].conditions[type=HooksReady]`, and either proceed or pause. VM Operator does not implement the Lifecycle Operator, the CRDs' controllers, or the eventing system. This mirrors how `external/byok`'s `EncryptionClass` is vendored and consumed today without VM Operator owning the BYOK key-management system behind it.

## Stage scope for this spec

Per product decision, v1 covers four VirtualMachine lifecycle checkpoints, not the one-pager's illustrative `Encrypt`/`PowerOn`/`PreDestroy` set:

1. **VM Create** — before the VM is first created in vSphere.
2. **VM power-state change** — before a power-state transition is applied to vSphere.
3. **VM Delete (vSphere-side)** — before the underlying vSphere VM is deleted/unregistered.
4. **Kubernetes resource deletion** — before the `VirtualMachine` CR's finalizer is removed, i.e. before Kubernetes is allowed to garbage-collect the object.

Exact `LifecycleStages` `type` (`Single` vs `Reentrant`), `blocking` default, and condition/event naming per stage are open — see `spec.md` "Open questions".

## What G2's "unrelated reconcile work" means, per stage

`spec.md` G2 requires that pausing a stage not pause unrelated reconcile work. The boundary between "the gated action" and "unrelated work" is not uniform across the four stages — it falls out of where each checkpoint sits in the existing `VirtualMachine` reconcile pipeline, rather than being a free design choice. Traced per stage:

| Stage | Held while hooks are pending | Still converges in the same pass | Externally observable state |
|---|---|---|---|
| **Create** | the entire provider dispatch — placement, configuration, power, status sync | nothing VM-specific; only the finalizer and the blocked condition are persisted | no vSphere VM exists; `status.uniqueID` empty, `Created` condition absent |
| **PowerStateChange** | only the power-on / power-off task | every other update step — configuration, devices, networks, volumes, location, status sync | VM exists and is **fully configured**; `Created=True`, `status.powerState` still the pre-transition value |
| **Delete** | the vSphere delete/unregister, and everything downstream of it on the delete path (PVC owner-reference release, the `ResourceDelete` checkpoint) | nothing — delete reconciliation exits at the checkpoint | VM still present in vSphere; object carries a `DeletionTimestamp` with the finalizer held |
| **ResourceDelete** | finalizer removal, plus the metrics and prober cleanup that accompany it | nothing — the vSphere VM is already gone and PVC owner references are already released | object stuck in `Terminating` |

So the set of work that is genuinely "unrelated and still converging" is substantial only for `PowerStateChange` — which is why `spec.md` SC-002 singles that stage out. For the other three it is empty, and not because the gate is coarse: a VM that does not exist yet has no state to reconcile, and a VM on the delete path has no desired state left to converge toward. G2 is therefore a real constraint on exactly one stage and a trivially-satisfied one on the rest.

One caveat found while tracing this: **"proceeds" is not the same as "is correct to proceed."** Snapshot creation runs whenever the power-state step leaves the state unconverged, so a snapshot taken while a `PowerStateChange` hook is pending captures the pre-transition power state. That step is reached because the decision keys off which error flavor the power step returned rather than off power-state convergence — the same gap that a `VirtualMachineGroup` boot-order delay and a genuine power-on failure already fall through, so it is pre-existing rather than introduced here. The `PowerStateSynced` condition is already set correctly on the blocked path, so the fix belongs in the snapshot step (which carries comparable guards already), not in the stage gate: a gate must not distort its return value to steer an unrelated downstream decision.

## Which other controllers a blocked stage affects, per stage

Per G2, pausing a stage must not break unrelated reconcile work. For each `(controller, stage)` pair, does that controller's own reconcile workflow get affected by a blocked stage, and how:

1. **Delayed, resumes** — the workflow waits, then completes correctly once the hook resolves. Self-healing, no defect.
2. **Unaffected** — the controller passively reads current VM state each reconcile, with no forward-looking commitment baked in. It behaves identically whether the VM's transient state is caused by a hook, a slow guest shutdown, a stuck task, or anything else — the window a hook can hold it open for is irrelevant to the controller's own logic.
3. **Breaks the feature** — the controller actively commits to a decision (schedules, stamps, or otherwise acts on an assumption about bounded transition time), and a hook violates that assumption, producing output that is actively wrong, not merely a longer-lived accurate read.

| Controller | Create | PowerStateChange | Delete | ResourceDelete |
|---|---|---|---|---|
| **VirtualMachineReplicaSet** | 1 — counts the member regardless of readiness (`:305-314,472`); no over-provisioning, converges once created | 1 — same counting; converges once powered on | 1 — `DeletionTimestamp`'d members still count (`:321`), no premature replacement | 1 — same as Delete |
| **VirtualMachineService** | 1 — no primary IP, omitted from Endpoints until created | 2 — passively reflects current VM state each reconcile (power-on: no IP, correctly omitted; power-off: VM still running, correctly still in Endpoints) — identical to today's behavior if a power transition is merely slow for any other reason. Not a drain barrier; hook owners must not treat it as one | 2 — dropped from Endpoints immediately on `DeletionTimestamp`, no hook coupling | 2 — already dropped |
| **VirtualMachineSnapshot** | 1 — bare error on empty `Status.UniqueID` (`:200-202`), succeeds once the VM is created | 2 — passively reads current power state and passes `spec.Memory`/`spec.Quiesce` through to `CreateSnapshotEx` as-is (`virtualmachine/snapshot.go:83-96`), identical to today's behavior if a power-on task is merely slow for any other reason. (A `PoweredOff` VM silently ignores both flags and succeeds with a disk-only snapshot — pre-existing vSphere behavior, not a new failure mode. The separate `status.quiesced`-set-from-request-not-outcome defect is pre-existing and unrelated to hooks.) | 2 — no VM-deletion-timestamp check at all; this controller's workflow doesn't look at VM delete state, hook or not | 2 — resolves cleanly once the VM is gone |
| **VirtualMachineGroup** | 1 — placement simply isn't requested yet, same shape as any slow create | 3 — the group **commits** a forward-looking schedule (stamps tier *N+1*'s absolute time assuming tier *N* converges quickly); a hook violates that assumption and the group acts on the stale commitment it already wrote, firing tier *N+1* before tier *N* actually converges. `Ready=True` reported throughout. Tracked as spec G7, not deferred (`plan.md` §7, TBD) | 2 — keeps patching a `Terminating` member's spec; pre-existing, hook-independent | 2 — same mechanism as Delete |
| **StoragePolicyUsage** | 2 — counting the VM as reserved capacity is the intended semantic | 2 — reports actual `Used`, no distortion | 2 — passively checks `DeletionTimestamp` presence each reconcile; identical to today's behavior if deletion is merely slow for any other reason | 2 — same reasoning as Delete |
| **VirtualMachinePublishRequest** | 1 — `SourceValid=False` plus an error until the VM is created | 2 — no power-state check in this controller | 2 — passively checks deletion state each reconcile; identical to today's behavior if deletion is merely slow for any other reason | 2 — vSphere VM already gone, publish fails same as today |
| **Volume** (legacy) | 1 — no-op on empty `BiosUUID`, attaches once populated | 2 — only the CSI attach mode shifts cold/hot, by design | 2 — `ReconcileDelete` is a deliberate no-op | 2 — no-op |
| **VolumeBatch** | 1 — no-op on empty `InstanceUUID`/`BiosUUID`, attaches once populated | 2 — no power-state coupling at all | 2 — `ReconcileDelete` is a deliberate no-op | 2 — no-op |
| **VirtualMachineClass** | 2 — zero coupling | 2 | 2 | 2 |
| **VirtualMachineImageCache** | 2 — never triggered; no VM watch | 2 | 2 | 2 |

Exactly one cell in this table is a "3": `VirtualMachineGroup`/`PowerStateChange`. It is the only controller that makes an active forward-looking commitment rather than passively reading current state, which is what makes it the one row this spec tracks as a goal (G7) rather than leaving to another controller's own follow-up spec. The lever this feature contributes to every "2" row above that still carries a pre-existing, hook-independent correctness gap (e.g. `StoragePolicyUsage`'s quota release, `VirtualMachinePublishRequest`'s missing deletion-timestamp check) is G4's per-stage condition set, giving each controller a first-class signal to key off if it ever chooses to — consuming it is each controller's own follow-up work, not required by this spec.

## Terminal failures (how a not-yet-ready hook should be treated)

Investigated whether vm-operator has any concept of a permanently-terminal, stop-retrying-forever failure, to decide how VM Operator should react when a stage's hooks aren't ready. Finding: **it does not** — every failure path resolves to one of three level-triggered behaviors, all funneled through `pkgerr.ResultFromError`:

1. **Plain error** (e.g. a vSphere create failure, missing `VirtualMachineClass`, encryption class/key not found) — the relevant condition is set `False` via `conditions.MarkError`/`MarkFalse`, and a plain `error` is returned, which triggers controller-runtime's normal exponential-backoff requeue. These paths are typically paired with a `Watches()` on the missing/invalid referenced object (e.g. `VirtualMachineClass`, `byokv1.EncryptionClass`), so fixing the referenced object re-triggers reconciliation immediately rather than waiting out the backoff.
2. **`pkgerr.NoRequeueError`** — becomes `reconcile.TerminalError`, which stops *backoff* retries for that specific attempt but explicitly still reconciles again on the next watch-triggered event (e.g. a vCenter connection-state change). "Pause polling," not "give up forever."
3. **`pkgerr.NoRequeueNoErr`** — not a failure; "this reconcile did its job for now" (e.g. VM just created, task in flight).

**Conclusion applied to this spec**: the Lifecycle Operator owns all timeout/failure/retry semantics for a hook — it is the sole writer of `LifecycleState.status.stages[].conditions[HooksReady]` and its `reason` (`HooksPending`, or any failure/timeout reason it chooses). VM Operator does not parse that `reason` at all (see `model.md` "HooksReady handling") — a non-`True` `HooksReady` is modeled the same way as case 1 above: a plain condition update on the VM (`False`/`HooksPending`), no `NoRequeueError`. VM Operator already plans to `Watches()` `LifecycleState` (see `plan.md`), so any change to `HooksReady` — including the Lifecycle Operator's own retry or timeout resolution — re-triggers reconciliation immediately, exactly like the `EncryptionClass`-missing case does today.

## Zero-hook cost — the no-`LifecycleHook` path is not free under any documented handshake

[NEEDS CLARIFICATION: how does VM Operator satisfy the "no hook anywhere → zero measurable behavior change" requirement (`spec.md` G6 / US1 scenario 3) under the Lifecycle framework's actual create-time contract?]

Per "Target operator communication to Lifecycle framework" (Confluence page 2826812771), the framework's own design doc lays out four candidate handshakes for how `LifecycleState.spec.stages[]` gets populated — crossing "who populates it" (the Lifecycle framework vs. the target operator) against "what scope" (every declared blocking stage vs. only stages that currently have a subscriber). None of the four is free for a VM with zero `LifecycleHook`s registered anywhere:

- **Populate all declared blocking stages** (either populator, done synchronously — e.g. via a mutating admission webhook — so there's no creation-time wait): an entry always exists for every stage regardless of subscribers, so reaching any stage still costs "pause → framework resolves zero subscribers → `HooksReady=True`/`NoHooksRegistered` → operator releases" — the doc's own numbers are **4 writes, 2 round trips**, on *every* stage reach, forever, even with zero hooks anywhere.
- **Populate only subscribed stages** (either populator, filled in asynchronously by a separate controller after the object is created with `spec.stages: []`): reaching an unsubscribed stage is free once populated (a missing entry means proceed, zero writes) — but the operator must first wait for a `StagesConverged=True` signal before it can trust that "no entry" actually means "no hook" rather than "the async fill hasn't happened yet" (this is also the source of the creation-race the doc calls out explicitly for the operator-populates variant). That wait happens once per object at creation time, even when zero hooks will ever be registered.

In other words: the "all stages" family taxes the **steady-state per-stage-reach path**; the "subscribed-only" family taxes the **one-time creation path**. Every combination on that page costs *something* for the common case (no hook registered), which conflicts with G6 as currently written ("no measurable behavior change from today" when no `LifecycleState`/hook exists).

This suggests VM Operator may need a **pre-check that never enters any of the framework's four handshakes at all** for the zero-hook case, evaluated *before* ever creating or reading a `LifecycleState` — rather than relying on any signal the Lifecycle Operator side of the contract produces. `model.md`'s existing "VM Operator does not `Get`/`List` `LifecycleHook` directly in the reconcile path" statement and the edge case in `spec.md`'s prior draft ("exact mechanism — direct `LifecycleHook` list vs. relying solely on `LifecycleState` absence — is a `plan.md` concern") both need to be revisited in light of this: it may no longer be a deferrable `plan.md` detail, since G6 appears unsatisfiable by any of the four framework-side contracts alone. Two candidate pre-check designs, and how they compare:

### Candidate 1 — VM Operator maintains its own indexed cache of `LifecycleHook`

VM Operator watches `LifecycleHook` directly and keeps a local indexed lister keyed by `(namespace, group, kind, stage)`, computing the union itself before ever touching `LifecycleState`. This is genuinely free in the steady state — a local cache read, no API round trip — for both the one-time creation cost and the steady-state per-stage-reach cost, so it satisfies G6 on both axes at once, unlike either family in the four-option comparison above.

Race/consistency-wise, this carries no additional risk versus any other watched dependency already in this codebase (`VirtualMachineClass`, `byokv1.EncryptionClass`): `controller-runtime`'s manager blocks `Reconcile` until `WaitForCacheSync` completes, and ordinary watch propagation (sub-second) keeps the cache current thereafter. A `LifecycleHook` created at the *literal same instant* as the VM it targets is an acceptable, inherent race for any level-triggered controller — the same watch that eventually observes the hook re-triggers reconciliation, and no design (including all four framework-documented options) can or should promise anything stronger for genuinely concurrent writes. This applies identically regardless of whether the cache being read is local to VM Operator or a framework-maintained aggregate (Candidate 2 below) — neither is "safer" than the other on this axis.

The actual cost of this candidate is architectural, not correctness-related: it reverses `research.md`'s "Ownership boundary" decision that VM Operator does not `List`/`Get` `LifecycleHook` directly, and it means VM Operator re-implements the framework's target-matching logic locally. If that matching logic (currently a simple `(group, kind, stage)` match) ever grows richer on the framework side, VM Operator's parallel implementation can silently drift from the authoritative one.

### Candidate 2 — Framework maintains a new `LifecycleSubscribedStages` CRD; VM Operator copies the stage union from it

Instead of VM Operator computing the union itself, the Lifecycle Operator maintains and reconciles a new resource — scoped to `(namespace, group, kind)` — that precomputes "which stages currently have at least one subscriber" for that target kind. VM Operator watches this one (or few) object(s) per `(group, kind)` and reads it as a pure consumer, exactly as it already does for `LifecycleState` and `EncryptionClass`.

This revives a concept the one-pager originally proposed and the API-design doc superseded — see "Discrepancy between the two docs" above, which notes the one-pager's initial resource model included an "Aggregated LifecycleHook"/"Aggregated LifecycleState" concept dropped in favor of the current per-instance `LifecycleState` schema. Reviving it here is deliberate, to close the G6 gap the current schema reopens; it needs the Lifecycle framework team's buy-in, and specifically needs surfacing *why* the aggregated-resource idea was dropped originally, in case that reason still applies.

On race/consistency grounds, Candidate 2 is **no safer or less safe than Candidate 1** — both are reads through a normally-synced `controller-runtime` cache, subject to the same two acceptable properties described above (concurrent-creation races are inherent and self-correcting via watch; already-committed state is reliably visible once synced). The two candidates differ only on ownership: Candidate 2 keeps the matching/aggregation logic where `research.md`'s existing architecture says it belongs — owned by the Lifecycle Operator — so VM Operator never re-implements or risks drifting from it. That is the deciding factor between the two, not correctness or latency.

**Current lean**: Candidate 2, on ownership grounds alone. It requires one new vendored CRD (same shape of work as `LifecycleStages`/`LifecycleHook`/`LifecycleState` already planned) and sign-off from the Lifecycle framework team, since it is not one of the four options that team has documented.

## Prior art in this repo

- `external/byok` (`EncryptionClass`, group `encryption.vmware.com`) is the closest existing pattern for vendoring a CRD this repo doesn't own: own Go module, `AddToScheme` wired into `pkg/manager/manager.go`, generated manifest under `config/crd/external-crds/`, gated by `pkgcfg.Features.BringYourOwnEncryptionKey`.
- `pkg/vmconfig` (`Reconciler` interface, `crypto.New()` for BYOK) is the existing "pluggable reconcile step" abstraction; a lifecycle-hook stage gate is a natural fit for the same shape, though it must be usable from `controllers/virtualmachine` (Create/K8s-delete checkpoints) as well as from `pkg/providers/vsphere` (vSphere Create/PowerState/Delete checkpoints), which is wider than `vmconfig`'s current call sites.
- `pkg/util/paused` (`ByDevOps`/`ByAdmin`) is an existing, unrelated "pause reconciliation" mechanism (annotation-driven, not condition-driven) — different mechanism, same spirit; cross-reference in `plan.md` to avoid confusing the two.

## Links

- Upstream SDD methodology: <https://github.com/github/spec-kit/blob/main/spec-driven.md>
- This repo's SDD standards: [`sdd-standards.md`](../../memory/sdd-standards.md)
- Architectural standards (external CRD vendoring pattern): [`architectural-standards.md`](../../memory/architectural-standards.md)
- Operator best practices (requeue/error semantics, VC op IDs): [`operator-best-practices.md`](../../memory/operator-best-practices.md)
