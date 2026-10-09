# Data Model: Blocking Lifecycle Hooks

- **Spec**: [`spec.md`](./spec.md)
- **Research**: [`research.md`](./research.md)

This feature is **consumer-only**: VM Operator does not own the `LifecycleStages`, `LifecycleHook`, `LifecycleState`, or `LifecycleSubscribedStages` CRDs. It vendors read/write client types for them (mirroring `external/byok`) and adds new `VirtualMachine` status conditions plus a static `LifecycleStages` manifest declaring the stages it exposes. The one exception to "consumer-only" is `LifecycleState`: VM Operator creates its own VM's instance exactly once, at the VM's first reconcile and only if `LifecycleSubscribedStages` lists a stage for `VirtualMachine` (owner-referenced, for correct Kubernetes garbage-collection semantics — see "`LifecycleState`" below), and manages a finalizer on it, even though the CRD itself and its `status` are Lifecycle-Operator-owned. Every later creation is the Lifecycle Operator's: a day-2 hook on a VM that had no `LifecycleState` yet.

## Vendored external types (`lifecycle.vcfa.vmware.com/v1alpha1`, owned by the Lifecycle Operator)

### `LifecycleStages` (cluster-scoped)

Declares which stages a target operator exposes for hooking. VM Operator **authors** one instance of this (`vmoperator-stages`, see below), but does not install or reconcile it — see "Static `LifecycleStages` instance" below for how it reaches the cluster.

| Field | Type | Notes |
|---|---|---|
| `spec.objects[].group` | string | `vmoperator.vmware.com` for all VM Operator entries. |
| `spec.objects[].kind` | string | `VirtualMachine`. |
| `spec.objects[].stages[].name` | string | Stage name, referenced by `LifecycleHook.spec.stage`. |
| `spec.objects[].stages[].type` | enum `Reentrant`\|`Single` | `Reentrant` pauses every reconcile cycle it's reached; `Single` pauses at most once per object lifetime. |
| `spec.objects[].stages[].blocking` | bool | Whether VM Operator waits for hooks before proceeding. |
| `spec.objects[].stages[].eventName` | string | K8s event reason emitted on stage transition. |
| `spec.objects[].stages[].conditionName` | string | Condition type set on the `VirtualMachine`. |
| `spec.objects[].stages[].description` | string, optional | Human-readable summary of the point in the workflow this stage pauses before. Informational only: neither VM Operator nor the Lifecycle Operator branches on it. |

### `LifecycleHook` (namespaced, owned by consumers via the Lifecycle Operator)

A consumer's registration of interest in one stage for one `(group, kind)`. VM Operator only **reads** `LifecycleHook` existence indirectly, through `LifecycleState.status.hooks[]` — it does not `Get`/`List` `LifecycleHook` directly in the reconcile path (the Lifecycle Operator is responsible for fan-out into `LifecycleState`).

| Field | Type | Notes |
|---|---|---|
| `spec.target.group` / `spec.target.kind` | string | e.g. `vmoperator.vmware.com` / `VirtualMachine`. |
| `spec.stage` | string, immutable | Must match a stage name in a `LifecycleStages` resource. |

### `LifecycleSubscribedStages` (namespaced, **one instance per namespace**, owned by the Lifecycle Operator)

Resolves spec G5's zero-hook detection. Unlike `LifecycleState`, this is **not** scoped per `(group, kind)` — a single instance covers every consumer kind with hooks registered anywhere in that namespace, so its `status` must disambiguate by target internally. The Lifecycle Operator computes and keeps this current entirely on its own; VM Operator never lists or watches `LifecycleHook` directly.

| Field | Type | Written by | Notes |
|---|---|---|---|
| `status.objects[].group` / `.kind` | string | Lifecycle Operator | Disambiguates which consumer kind a `stages[]` entry belongs to. |
| `status.objects[].stages[]` | []string | Lifecycle Operator | Union of every stage name with ≥1 `LifecycleHook` currently registered for that `(group, kind)` in this namespace. A stage name absent here — or the whole resource being `NotFound` — both mean zero hooks; VM Operator treats them identically. |

VM Operator's `stageHasHook(stageName)` check: a single cached `Get` of the one per-namespace instance, then a local lookup of the `(vmoperator.vmware.com, VirtualMachine)` entry within `status.objects[]` for `stageName`. **This is consulted exactly once in a VM's lifetime for the purpose of deciding whether to create `LifecycleState`** — once that object exists (for any stage, any reason), VM Operator never reads `LifecycleSubscribedStages` again for that VM; see "VM Operator's read/write contract" below.

### `LifecycleState` (namespaced, owned by the target `VirtualMachine` via `ownerReferences`)

The coordination surface VM Operator reads and writes at each stage checkpoint.

| Field | Type | Written by | Notes |
|---|---|---|---|
| `metadata.ownerReferences` | object | **VM Operator** (on create) | Points at the owning `VirtualMachine`, `controller: true` — this is what makes Kubernetes' garbage collector delete `LifecycleState` automatically the moment the VM is deleted. |
| `metadata.finalizers` | []string | **VM Operator** | VM Operator adds its own finalizer at creation and removes it only after the `ResourceDelete` stage resolves. Exists solely so that neither the GC cascade (normal VM delete) nor the namespace controller's direct sweep (namespace delete) can remove the object out from under an in-progress check — see "Namespace-deletion protection" below. |
| `spec.target.{apiVersion,kind,name,namespace,uid}` | object | VM Operator (on create) | Identifies the `VirtualMachine` this state tracks. |
| `spec.stages[].name` | string | VM Operator (at creation), Lifecycle Operator (day 2) | Stage name. VM Operator seeds one entry per stage listed in `LifecycleSubscribedStages` when it creates the object; the Lifecycle Operator adds entries for hooks registered later. **Entry presence is VM Operator's signal that a hook exists for the stage**; a missing entry means no hook and VM Operator proceeds. |
| `spec.stages[].workflowPaused` | bool | **VM Operator** | Set `true` when VM Operator reaches this stage and pauses; set back to `false` in the same write that sets `workflowResumed=true`, so a `Reentrant` stage re-pauses cleanly on its next pass. |
| `spec.stages[].workflowResumed` | bool | **VM Operator** | Set `true` after VM Operator observes `HooksReady=True` and is proceeding — signals the Lifecycle Operator to reset that stage's `status.stages[]` entry (hook states back to `Pending`, `HooksReady` back to `False`) so the stage is ready to pause again the next time its checkpoint is reached. Every stage is declared `Reentrant` (see "Stage `type`/`blocking` defaults"), so this applies uniformly to all four. Resetting is the Lifecycle Operator's job; VM Operator never writes `status`. |
| `status.stages[]` | list | **Lifecycle Operator** | Per-stage status mirrored from `spec.stages[]`. VM Operator reads only `conditions[HooksReady]` from it (below); it does not use entry presence here as a hook signal. |
| `status.stages[].conditions[type=WorkflowPaused]` | condition | Lifecycle Operator | Mirrors `spec.stages[].workflowPaused`. |
| `status.stages[].conditions[type=HooksReady]` | condition | Lifecycle Operator | `True` = every registered hook for this stage has completed — **VM Operator's sole resume signal**. VM Operator treats any non-`True` value identically (waiting); it does not branch on `reason` (`HooksPending`, `HookFailed`, timed-out, etc.) — see "HooksReady handling" below. |
| `status.stages[].hooks[].lifecycleHookRef` / `.state` / `.message` | object | Lifecycle Operator | Per-hook progress (`Pending`\|`InProgress`\|`Succeeded`\|`Failed`), including timeout handling. Entirely internal to the Lifecycle Operator's bookkeeping — VM Operator does not read this field. |

**VM Operator's read/write contract per stage checkpoint** (`InitLifecycleState` and `ReconcileStage`; decision table in `plan.md` §1):

1. **First reconcile only, before the first create:** `InitLifecycleState` creates the `LifecycleState` (owner reference + finalizer) with every stage `LifecycleSubscribedStages` lists for `VirtualMachine` in `spec.stages[]`, each at `workflowPaused=false`, in one write; if none are listed it creates nothing. This is the only place `LifecycleSubscribedStages` is ever read.
2. `Get` the `LifecycleState` for the VM.
3. **`NotFound`, or found with no entry for this stage**: no hook for this stage. Mark the condition `True` and proceed. VM Operator does not read `LifecycleSubscribedStages` here and does not add or repair entries — the Lifecycle Operator creates `LifecycleState` or patches the stage in when a hook is registered (day 2). No `LifecycleState` traffic at all in the common, zero-hook case (spec G5).
4. **Found, entry present**: if `workflowPaused=false`, patch it `true`, set the VM's condition to blocked, exit without error/requeue (rely on the watch below). If `workflowPaused=true`, check `status.stages[stage].conditions[HooksReady]`: `True` → patch `workflowPaused=false` and `workflowResumed=true` together, set the condition `True`, proceed; not `True` → exit without error/requeue.
5. VM Operator's controller watches `LifecycleState` updates (mapped back to the owning `VirtualMachine` via its owner reference) so a `HooksReady` flip, or a new entry from a day-2 hook, promptly re-triggers reconciliation instead of waiting for the next poll.
6. Once the `ResourceDelete` stage resolves (`Proceed=true`), VM Operator removes its own finalizer from `LifecycleState`. VM Operator never issues an explicit `Delete` call on this object anywhere — Kubernetes' garbage collector does that automatically via the owner reference (or, during namespace deletion, the namespace controller does it directly); VM Operator's finalizer only controls *when that already-issued delete is allowed to complete*.

### Namespace-deletion protection

When a `Namespace` is deleted, Kubernetes' namespace controller deletes **every** namespaced object directly and concurrently — `LifecycleHook`, `LifecycleSubscribedStages`, and `LifecycleState` alike — independent of any owner-reference cascade, and none of the first two carry finalizers today. Without protection, this can make a real hook indistinguishable from no hook at all: a hook could be lost if the state that records it were swept away before VM Operator acted on it.

Two things close this for the two stages where a missed hook is irreversible (`Delete`, `ResourceDelete` — not `Create`/`PowerStateChange`, see `plan.md`'s rationale):

- **The finalizer** (above) — protects an *already-existing* `LifecycleState` from being removed mid-check, regardless of whether the delete call came from GC (normal VM delete) or the namespace controller (namespace delete).
- **Seeding at first reconcile, day 2 by the Lifecycle Operator** — `InitLifecycleState` seeds an entry for **every** stage `LifecycleSubscribedStages` lists for `VirtualMachine`, so `Delete`/`ResourceDelete` are declared long before any deletion is in play. A hook registered later is the Lifecycle Operator's to propagate (patch into the existing object, or create it). `LifecycleSubscribedStages` is finalizer-protected by the Lifecycle Operator, so the one read is always safe. Accepted gaps (see `plan.md` §1a): a hook registered the instant a namespace terminates, and a Lifecycle-Operator-created `LifecycleState` that lacks VM Operator's finalizer.

## New `VirtualMachine` API surface (this repo, `api/v1alpha6`)

One new condition type for the entire feature:

| Condition type | Set during | `True` means | `False` reason |
|---|---|---|---|
| `VirtualMachineConditionLifecycleHooksReady` | all reconcile paths | no stage is currently pausing for hooks | `HooksBlocked` (message names the blocked stage, e.g., "blocked on Create stage hooks") |

The condition follows the repo's positive convention (`True` = ready/healthy, like `Created`/`Ready`): `True` whenever no stage is pausing, `False`/`HooksBlocked` while a stage is blocked, never `Unknown`. Stages are sequential (`Delete` fully resolves before `ResourceDelete`) and `Create`, `PowerStateChange` and `Delete` are mutually exclusive by VM phase, so at most one stage is ever blocked and the `False` message names exactly that one.

This condition is additive to `api/v1alpha6`'s existing condition set (see `api/v1alpha6/condition_consts.go` for the current pattern) — no field removal, no version bump required. `v1alpha6` is `main`'s current storage version.

### Hooks-not-ready handling (single reason, no failure-detail parsing)

VM Operator's `LifecycleHooksReady` condition has exactly one `False` reason (`HooksBlocked`) regardless of *why* `LifecycleState.status.stages[stage].conditions[HooksReady]` isn't `True` yet — a hook still running, a hook that errored, or a hook that timed out are all the Lifecycle Operator's concern, tracked in its own `HooksReady` reason/`status.stages[].hooks[]` bookkeeping, which VM Operator does not parse. This is **not** a terminal state on VM Operator's side either way — VM Operator is a level-triggered controller with no concept of a permanent failure (see `research.md` "Terminal failures"): it keeps reconciling at the normal cadence and the condition self-heals to `True` the moment any blocking `HooksReady` does, with no VM Operator-side retry limit or escalation.

### Stage `type`/`blocking` defaults (resolved)

| Stage | `type` | `blocking` | Rationale |
|---|---|---|---|
| `Create` | `Reentrant` | `true` | Typically reached once per VM lifecycle, but the gate re-evaluates fresh on every reach — e.g. a retry prior to the VM existing in vSphere. |
| `PowerStateChange` | `Reentrant` | `true` | Fires on every power-on **and** power-off transition, independently. |
| `Delete` | `Reentrant` | `true` | Typically reached once, at vSphere-side deletion, but re-evaluates fresh on a retried delete call. |
| `ResourceDelete` | `Reentrant` | `true` | Typically reached once, as the terminal step of the same delete flow as `Delete`, but re-evaluates fresh on a retried finalizer-removal patch. |

`ReconcileStage`'s decision table (`plan.md` §1) does not branch on `type` at all — every stage is evaluated identically regardless of this field. `type=Reentrant` for all four is what the Lifecycle Operator needs to know it should reset `status.stages[stage]` back to pending after every `workflowResumed=true`, not something VM Operator's own gate logic consults.

## Capability gating

The Supervisor capability `supports_vm_service_lifecycle_hooks` gates the entire feature, following the same mechanism `supports_telco_vm_service_api` uses today: `pkg/config/capabilities/capabilities.go` reads the `Capability` CR's `Activated` status and sets `pkgcfg.Features.LifecycleHooks` accordingly (see `research.md`'s BYOK/`BringYourOwnEncryptionKey` cross-reference — BYOK is also capability-drivable via this same code path). When the capability is disabled, `Features.LifecycleHooks` is `false` and every stage checkpoint is a pure no-op: no `LifecycleState` `Get`/`Create`, no watch, no pause — behavior is identical to the feature not existing (per `spec.md`'s Platform engineer stories).

## Static `LifecycleStages` instance

VM Operator authors one cluster-scoped `LifecycleStages` CR (`vmoperator-stages`) declaring the four stages above for `(vmoperator.vmware.com, VirtualMachine)`, all four with `type=Reentrant`, `blocking=true` (see "Stage `type`/`blocking` defaults"). This is data, not a CRD definition — the CRD itself is installed by the Lifecycle Operator's own chart. For how this instance is actually packaged and applied to a cluster, see `plan.md`'s "Getting the CRD onto a Supervisor."

## Conversion strategy

Not applicable — no existing field is being changed or removed. The new conditions are additive and version-agnostic (conditions are not versioned per-`apiVersion` the way `spec`/`status` typed fields are).

## Examples

Illustrative instances of the four external kinds for a namespace where two consumers have registered hooks on `VirtualMachine`. Shapes follow the Lifecycle Operator's API design ("[API design] Blocking Lifecycle stages") and "Target operator and Lifecycle framework details"; the Lifecycle Operator's CRDs are the source of truth if they diverge from these.

### `LifecycleStages`

Authored by VM Operator, cluster-scoped, one per target operator. Declares the four stages (see "Static `LifecycleStages` instance").

```yaml
apiVersion: lifecycle.vcfa.vmware.com/v1alpha1
kind: LifecycleStages
metadata:
  name: vmoperator-stages
spec:
  objects:
  - group: vmoperator.vmware.com
    kind: VirtualMachine
    stages:
    - name: Create
      description: Before the VM is created in vSphere.
      type: Reentrant
      blocking: true
      eventName: CreateHooksReady
      conditionName: CreateHooksReady
    - name: PowerStateChange
      description: Before a power-on or power-off is applied.
      type: Reentrant
      blocking: true
      eventName: PowerStateChangeHooksReady
      conditionName: PowerStateChangeHooksReady
    - name: Delete
      description: Before the VM is deleted from vSphere.
      type: Reentrant
      blocking: true
      eventName: DeleteHooksReady
      conditionName: DeleteHooksReady
    - name: ResourceDelete
      description: Before the VM's Kubernetes finalizer is removed.
      type: Reentrant
      blocking: true
      eventName: ResourceDeleteHooksReady
      conditionName: ResourceDeleteHooksReady
```

### `LifecycleHook`

Created by a consumer, namespaced. Each hook targets one stage, so a consumer that needs two stages creates two hooks. The `target-*` and `stage` labels, the `created-by` annotation, and the finalizer are added by the Lifecycle Operator's webhook and controller, not by the consumer.

```yaml
apiVersion: lifecycle.vcfa.vmware.com/v1alpha1
kind: LifecycleHook
metadata:
  name: domain-join-hook
  namespace: workload-ns
  labels:
    lifecycle.vcfa.vmware.com/target-group: vmoperator.vmware.com
    lifecycle.vcfa.vmware.com/target-kind: VirtualMachine
    lifecycle.vcfa.vmware.com/stage: Create
  annotations:
    lifecycle.vcfa.vmware.com/created-by: >-
      system:serviceaccount:workload-ns:domain-join-sa
  finalizers:
  - lifecycle.vcfa.vmware.com/hook-cleanup
spec:
  target:
    group: vmoperator.vmware.com
    kind: VirtualMachine
  stage: Create
---
apiVersion: lifecycle.vcfa.vmware.com/v1alpha1
kind: LifecycleHook
metadata:
  name: cmdb-cleanup-hook
  namespace: workload-ns
spec:
  target:
    group: vmoperator.vmware.com
    kind: VirtualMachine
  stage: Delete
```

### `LifecycleSubscribedStages`

Computed by the Lifecycle Operator from the two hooks above, namespaced, one per `(namespace, target group, target kind)`. It is never written by VM Operator, and it does not exist at all when no hook is registered. `stages` is a set that is never empty; it lists only stages with at least one live hook, so `PowerStateChange` and `ResourceDelete` are absent here.

```yaml
apiVersion: lifecycle.vcfa.vmware.com/v1alpha1
kind: LifecycleSubscribedStages
metadata:
  name: lss-1b52874a51e98527
  namespace: workload-ns
  labels:
    lifecycle.vcfa.vmware.com/target-group: vmoperator.vmware.com
    lifecycle.vcfa.vmware.com/target-kind: VirtualMachine
spec:
  stages:
  - Create
  - Delete
```

### `LifecycleState`

Created by VM Operator at the VM's first reconcile, seeded with every stage listed above, and owned by the VM. `blockOwnerDeletion` is `false` so the state is never required to be deleted before its VM.

**Seeded at creation (nothing paused yet):**

```yaml
apiVersion: lifecycle.vcfa.vmware.com/v1alpha1
kind: LifecycleState
metadata:
  name: vm-windows-abc123
  namespace: workload-ns
  labels:
    lifecycle.vcfa.vmware.com/target-group: vmoperator.vmware.com
    lifecycle.vcfa.vmware.com/target-version: v1alpha6
    lifecycle.vcfa.vmware.com/target-kind: VirtualMachine
  finalizers:
  - lifecycle.vcfa.vmware.com/vm-operator-state
  ownerReferences:
  - apiVersion: vmoperator.vmware.com/v1alpha6
    kind: VirtualMachine
    name: windows-abc123
    uid: 3f1c2b9e-0000-0000-0000-000000000001
    controller: true
    blockOwnerDeletion: false
spec:
  target:
    apiVersion: vmoperator.vmware.com/v1alpha6
    kind: VirtualMachine
    name: windows-abc123
    namespace: workload-ns
    uid: 3f1c2b9e-0000-0000-0000-000000000001
  stages:
  - name: Create
    workflowPaused: false
    workflowResumed: false
  - name: Delete
    workflowPaused: false
    workflowResumed: false
```

**`Create` blocked.** VM Operator reached the checkpoint and set `workflowPaused=true`. One hook is still running, so `HooksReady` is `False` and the VM's `CreateHooksReady` condition is `False`/`HooksBlocked`. Only `spec.stages` and `status` change from the previous block:

```yaml
spec:
  stages:
  - name: Create
    workflowPaused: true
    workflowResumed: false
  - name: Delete
    workflowPaused: false
    workflowResumed: false
status:
  stages:
  - name: Create
    conditions:
    - type: WorkflowPaused
      status: "True"
      reason: StageReached
      message: Stage Create was reached; waiting for hooks
      lastTransitionTime: "2026-03-25T12:00:00Z"
    - type: HooksReady
      status: "False"
      reason: HooksPending
      message: 0 of 1 hooks completed
      lastTransitionTime: "2026-03-25T12:00:00Z"
    hooks:
    - lifecycleHookRef: domain-join-hook
      lifecycleHookUID: 7a0e5c1d-0000-0000-0000-000000000002
      state: InProgress
      message: Joining domain controller dc01.corp.local
      lastTransitionTime: "2026-03-25T12:00:05Z"
```

**`Create` ready to resume.** `HooksReady` is `True`, which is VM Operator's only resume signal. The Lifecycle Operator sets it for success, failure, and timeout alike, so this example is a failed hook; VM Operator does not read `reason` or `hooks[]`. On its next reconcile, VM Operator patches `workflowPaused=false` and `workflowResumed=true` in one write, then creates the VM. Only the `Create` status entry is shown:

```yaml
status:
  stages:
  - name: Create
    conditions:
    - type: WorkflowPaused
      status: "True"
      reason: StageReached
      message: Stage Create was reached; waiting for hooks
      lastTransitionTime: "2026-03-25T12:00:00Z"
    - type: HooksReady
      status: "True"
      reason: HookFailed
      message: >-
        domain-join-hook: timed out connecting to domain controller
      lastTransitionTime: "2026-03-25T12:05:00Z"
    hooks:
    - lifecycleHookRef: domain-join-hook
      lifecycleHookUID: 7a0e5c1d-0000-0000-0000-000000000002
      state: Failed
      message: Timed out connecting to domain controller
      lastTransitionTime: "2026-03-25T12:05:00Z"
```
