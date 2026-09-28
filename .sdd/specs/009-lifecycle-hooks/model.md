# Data Model: Blocking Lifecycle Hooks

- **Spec**: [`spec.md`](./spec.md)
- **Research**: [`research.md`](./research.md)

This feature is **consumer-only**: VM Operator does not own the `LifecycleStages`, `LifecycleHook`, `LifecycleState`, or `AggregatedLifecycleHooks` CRDs. It vendors read/write client types for them (mirroring `external/byok`) and adds new `VirtualMachine` status conditions plus a static `LifecycleStages` manifest declaring the stages it exposes. The one exception to "consumer-only" is `LifecycleState`: VM Operator is the sole **creator** of its own VM's instance (for correct Kubernetes garbage-collection semantics — see "`LifecycleState`" below) and manages a finalizer on it, even though the CRD itself and its `status` are Lifecycle-Operator-owned.

## Vendored external types (`lifecycle.vcfa.vmware.com/v1alpha1`, owned by the Lifecycle Operator)

### `LifecycleStages` (cluster-scoped)

Declares which stages a target operator exposes for hooking. VM Operator **writes** one instance of this (`vmoperator-stages`, see below) via its helm chart; it does not reconcile this type.

| Field | Type | Notes |
|---|---|---|
| `spec.objects[].group` | string | `vmoperator.vmware.com` for all VM Operator entries. |
| `spec.objects[].kind` | string | `VirtualMachine`. |
| `spec.objects[].stages[].name` | string | Stage name, referenced by `LifecycleHook.spec.stage`. |
| `spec.objects[].stages[].type` | enum `Reentrant`\|`Single` | `Reentrant` pauses every reconcile cycle it's reached; `Single` pauses at most once per object lifetime. |
| `spec.objects[].stages[].blocking` | bool | Whether VM Operator waits for hooks before proceeding. |
| `spec.objects[].stages[].eventName` | string | K8s event reason emitted on stage transition. |
| `spec.objects[].stages[].conditionName` | string | Condition type set on the `VirtualMachine`. |

### `LifecycleHook` (namespaced, owned by consumers via the Lifecycle Operator)

A consumer's registration of interest in one stage for one `(group, kind)`. VM Operator only **reads** `LifecycleHook` existence indirectly, through `LifecycleState.status.hooks[]` — it does not `Get`/`List` `LifecycleHook` directly in the reconcile path (the Lifecycle Operator is responsible for fan-out into `LifecycleState`).

| Field | Type | Notes |
|---|---|---|
| `spec.target.group` / `spec.target.kind` | string | e.g. `vmoperator.vmware.com` / `VirtualMachine`. |
| `spec.stage` | string, immutable | Must match a stage name in a `LifecycleStages` resource. |

### `AggregatedLifecycleHooks` (namespaced, **one instance per namespace**, owned by the Lifecycle Operator)

Resolves spec G5's zero-hook detection. Unlike `LifecycleState`, this is **not** scoped per `(group, kind)` — a single instance covers every consumer kind with hooks registered anywhere in that namespace, so its `status` must disambiguate by target internally. The Lifecycle Operator computes and keeps this current entirely on its own; VM Operator never lists or watches `LifecycleHook` directly.

| Field | Type | Written by | Notes |
|---|---|---|---|
| `status.objects[].group` / `.kind` | string | Lifecycle Operator | Disambiguates which consumer kind a `stages[]` entry belongs to. |
| `status.objects[].stages[]` | []string | Lifecycle Operator | Union of every stage name with ≥1 `LifecycleHook` currently registered for that `(group, kind)` in this namespace. A stage name absent here — or the whole resource being `NotFound` — both mean zero hooks; VM Operator treats them identically. |

VM Operator's `stageHasHook(stageName)` check: a single cached `Get` of the one per-namespace instance, then a local lookup of the `(vmoperator.vmware.com, VirtualMachine)` entry within `status.objects[]` for `stageName`. **This is consulted exactly once in a VM's lifetime for the purpose of deciding whether to create `LifecycleState`** — once that object exists (for any stage, any reason), VM Operator never reads `AggregatedLifecycleHooks` again for that VM; see "VM Operator's read/write contract" below.

### `LifecycleState` (namespaced, owned by the target `VirtualMachine` via `ownerReferences`)

The coordination surface VM Operator reads and writes at each stage checkpoint.

| Field | Type | Written by | Notes |
|---|---|---|---|
| `metadata.ownerReferences` | object | **VM Operator** (on create) | Points at the owning `VirtualMachine`, `controller: true` — this is what makes Kubernetes' garbage collector delete `LifecycleState` automatically the moment the VM is deleted. |
| `metadata.finalizers` | []string | **VM Operator** | VM Operator adds its own finalizer at creation and removes it only after the `ResourceDelete` stage resolves. Exists solely so that neither the GC cascade (normal VM delete) nor the namespace controller's direct sweep (namespace delete) can remove the object out from under an in-progress check — see "Namespace-deletion protection" below. |
| `spec.target.{apiVersion,kind,name,namespace,uid}` | object | VM Operator (on create) | Identifies the `VirtualMachine` this state tracks. |
| `spec.stages[].name` | string | VM Operator | Stage name — written by VM Operator when it first reaches (or proactively declares) a stage. |
| `spec.stages[].workflowPaused` | bool | **VM Operator** | Set `true` when VM Operator reaches this stage and pauses. |
| `spec.stages[].workflowResumed` | bool | **VM Operator** | Set `true` after VM Operator observes `HooksReady=True` and is proceeding — signals the Lifecycle Operator to clear/reset the stage entry for future `Reentrant` passes. |
| `status.stages[]` (entry presence) | list | **Lifecycle Operator** | The presence of an entry for a stage — regardless of its `conditions` — is VM Operator's signal that a hook exists for that stage. Lifecycle Operator populates this either when `LifecycleState` is first created (for every stage currently hooked, not just the one that triggered creation) **or later, via a direct patch to the already-existing object**, when a new `LifecycleHook` targets a stage that has no entry yet ("day 2" hook registration). This second capability is what lets VM Operator stop re-consulting `AggregatedLifecycleHooks` once `LifecycleState` exists — see "VM Operator's read/write contract" below. |
| `status.stages[].conditions[type=WorkflowPaused]` | condition | Lifecycle Operator | Mirrors `spec.stages[].workflowPaused`. |
| `status.stages[].conditions[type=HooksReady]` | condition | Lifecycle Operator | `True` = every registered hook for this stage has completed — **VM Operator's sole resume signal**. VM Operator treats any non-`True` value identically (waiting); it does not branch on `reason` (`HooksPending`, `HookFailed`, timed-out, etc.) — see "HooksReady handling" below. |
| `status.stages[].hooks[].lifecycleHookRef` / `.state` / `.message` | object | Lifecycle Operator | Per-hook progress (`Pending`\|`InProgress`\|`Succeeded`\|`Failed`), including timeout handling. Entirely internal to the Lifecycle Operator's bookkeeping — VM Operator does not read this field. |

**VM Operator's read/write contract per stage checkpoint** (`ReconcileStage`, see `plan.md` for the full code):

1. `Get` the `LifecycleState` for the VM.
2. **`NotFound`**: no hook has ever touched this VM, for any stage. Consult `AggregatedLifecycleHooks` (the only time it is ever read for this VM) — if this specific stage is hooked, create `LifecycleState` (owner reference + finalizer), add a paused entry, and pause; otherwise mark the condition `True` and proceed. No `LifecycleState` traffic at all in the common, zero-hook case (spec G5).
3. **Found, but no entry for this specific stage**: do **not** assume "object exists" means "this stage is hooked" — a different stage may be the reason the object exists. Re-derive freshly from `AggregatedLifecycleHooks`, exactly as in step 2's hooked/unhooked branches, just skipping the create (the object already exists).
4. **Found, entry present**: if `workflowPaused=false`, patch it `true`, set the VM's condition to blocked, exit without error/requeue (rely on the watch below). If `workflowPaused=true`, check `status.stages[stage].conditions[HooksReady]`: `True` → patch `workflowResumed=true`, set the condition `True`, proceed; not `True` → exit without error/requeue.
5. VM Operator's controller watches `LifecycleState` updates (mapped back to the owning `VirtualMachine` via its owner reference) so a `HooksReady` flip, or a new `status.stages[]` entry from a day-2 hook, promptly re-triggers reconciliation instead of waiting for the next poll.
6. Once the `ResourceDelete` stage resolves (`Proceed=true`), VM Operator removes its own finalizer from `LifecycleState`. VM Operator never issues an explicit `Delete` call on this object anywhere — Kubernetes' garbage collector does that automatically via the owner reference (or, during namespace deletion, the namespace controller does it directly); VM Operator's finalizer only controls *when that already-issued delete is allowed to complete*.

### Namespace-deletion protection

When a `Namespace` is deleted, Kubernetes' namespace controller deletes **every** namespaced object directly and concurrently — `LifecycleHook`, `AggregatedLifecycleHooks`, and `LifecycleState` alike — independent of any owner-reference cascade, and none of the first two carry finalizers today. Without protection, this can make a real hook indistinguishable from no hook at all: if `LifecycleState` didn't already exist for a VM, and `AggregatedLifecycleHooks` is swept away in the same instant `ReconcileStage` tries to consult it, the check incorrectly concludes "no hook."

Two things close this for the two stages where a missed hook is irreversible (`Delete`, `ResourceDelete` — not `Create`/`PowerStateChange`, see `plan.md`'s rationale):

- **The finalizer** (above) — protects an *already-existing* `LifecycleState` from being removed mid-check, regardless of whether the delete call came from GC (normal VM delete) or the namespace controller (namespace delete).
- **Proactive declaration** — `pkg/lifecycle.EnsureTerminalStagesDeclared`, called from every `ReconcileNormal` pass, commits `Delete`/`ResourceDelete` hook existence into a finalizer-protected `LifecycleState` *while the VM is still alive and `AggregatedLifecycleHooks` is guaranteed reachable* — long before any deletion, namespace-triggered or otherwise, is in play. See `plan.md`'s "Controller / webhook impact" for the mechanics and its accepted residual gap (a hook registered in the same instant as namespace teardown, with no intervening reconcile, is not recoverable — same class of unwinnable race as a hook registered in the exact instant of teardown itself).

## New `VirtualMachine` API surface (this repo, `api/v1alpha6`)

One new condition type for the entire feature:

| Condition type | Set during | `True` means | `False` reason |
|---|---|---|---|
| `VirtualMachineConditionLifecycleHooksBlocked` | all reconcile paths | no stage is currently pausing for hooks | `HooksBlocked` (message includes which stage, e.g., "Create stage") |

This condition is additive to `api/v1alpha6`'s existing condition set (see `api/v1alpha6/condition_consts.go` for the current pattern) — no field removal, no version bump required. `v1alpha6` is `main`'s current storage version.

### Hooks-not-ready handling (single reason, no failure-detail parsing)

VM Operator's `LifecycleHooksBlocked` condition has exactly one `False` reason (`HooksBlocked`) regardless of *why* `LifecycleState.status.stages[stage].conditions[HooksReady]` isn't `True` yet — a hook still running, a hook that errored, or a hook that timed out are all the Lifecycle Operator's concern, tracked in its own `HooksReady` reason/`status.stages[].hooks[]` bookkeeping, which VM Operator does not parse. This is **not** a terminal state on VM Operator's side either way — VM Operator is a level-triggered controller with no concept of a permanent failure (see `research.md` "Terminal failures"): it keeps reconciling at the normal cadence and the condition self-heals to `True` the moment any blocking `HooksReady` does, with no VM Operator-side retry limit or escalation.

### Stage `type`/`blocking` defaults (resolved)

| Stage | `type` | `blocking` | Rationale |
|---|---|---|---|
| `Create` | `Single` | `true` | Fires at most once per VM lifecycle. |
| `PowerStateChange` | `Reentrant` | `true` | Fires on every power-on **and** power-off transition, independently. |
| `Delete` | `Single` | `true` | Fires once, at vSphere-side deletion. |
| `ResourceDelete` | `Single` | `true` | Fires once, as the terminal step of the same delete flow as `Delete`. |

## Capability gating

The Supervisor capability `supports_vm_service_lifecycle_hooks` gates the entire feature, following the same mechanism `supports_telco_vm_service_api` uses today: `pkg/config/capabilities/capabilities.go` reads the `Capability` CR's `Activated` status and sets `pkgcfg.Features.LifecycleHooks` accordingly (see `research.md`'s BYOK/`BringYourOwnEncryptionKey` cross-reference — BYOK is also capability-drivable via this same code path). When the capability is disabled, `Features.LifecycleHooks` is `false` and every stage checkpoint is a pure no-op: no `LifecycleState` `Get`/`Create`, no watch, no pause — behavior is identical to the feature not existing (per `spec.md`'s Platform engineer stories).

## Static `LifecycleStages` instance

VM Operator's helm chart ships one cluster-scoped `LifecycleStages` CR (`vmoperator-stages`) declaring the four stages above for `(vmoperator.vmware.com, VirtualMachine)`. This is data, not a CRD definition — the CRD itself is installed by the Lifecycle Operator's own chart. Exact YAML lands in `plan.md`'s project structure once `type`/`blocking` are finalized per stage.

## Conversion strategy

Not applicable — no existing field is being changed or removed. The new conditions are additive and version-agnostic (conditions are not versioned per-`apiVersion` the way `spec`/`status` typed fields are).
