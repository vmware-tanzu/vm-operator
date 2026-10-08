# Feature Specification: Blocking Lifecycle Hooks

- **Feature branch**: [`lifecyclehooks-sdd`](../../../../tree/lifecyclehooks-sdd)
  - **Fork**: N/A (branch on `vmware-tanzu/vm-operator`)
  - **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-08-24
- **Status**: Draft
- **Epic**: vmop-3377

---

## Background

A DevOps user's `VirtualMachine` reconciliation passes through several points — before it's created in vSphere, before a power-state change, and before it's deleted (from vSphere and, subsequently, from Kubernetes) — where work outside VM Operator's control needs to run first (AD registration, resource rebalancing, external cleanup) before the workflow resumes. Today, VM Operator offers no way to pause at any of these points; reconciliation always runs straight through.

VM Operator does not own the coordination mechanism itself. A separate Lifecycle Operator owns the `LifecycleStages`, `LifecycleHook`, and `LifecycleState` CRDs (group `lifecycle.vcfa.vmware.com`) and all hook fan-out, timeout, and eventing logic. VM Operator's role is limited to declaring which stages it exposes, and at each stage checkpoint, reading a single readiness signal off `LifecycleState` and pausing or proceeding accordingly — the same "consumer-only" relationship this repo already has with `external/byok`'s `EncryptionClass`.

---

## Goals

- **G1**: VM Operator MUST expose exactly four pre-stage lifecycle stages for `VirtualMachine`: **Create**, **PowerStateChange**, **Delete** (vSphere-side deletion), and **ResourceDelete** (Kubernetes CR finalizer removal).
- **G2**: Whenever a VM's `LifecycleState` indicates a stage is registered for blocking and its hooks are not yet ready, VM Operator MUST pause only that stage's underlying action. The rest of the reconcile workflow MUST remain exactly as it is today — no other step is skipped, delayed, or otherwise altered because one stage is paused.
- **G3**: VM Operator MUST automatically resume the paused action as soon as the `LifecycleState`'s `HooksReady` condition for that stage becomes `True`, without requiring any VM spec change or manual intervention.
- **G4**: VM Operator MUST surface, via `VirtualMachine` status conditions, whether each lifecycle stage is currently blocked by hooks — **one condition per stage** (`Create`, `PowerStateChange`, `Delete`, `ResourceDelete`), not a single condition shared across all four. A consumer needing to know whether one *specific* stage is blocking a given VM (e.g. `VirtualMachineGroup` sequencing boot order on `PowerStateChange`, see `research.md`) must be able to check that stage's condition directly, without parsing a free-text message to determine which stage it refers to.
- **G5**: VM Operator MUST continue to function (create/update/delete VMs normally) when no `LifecycleState`/hook exists for a VM, with no measurable behavior change from today.
- **G6**: The entire feature MUST be gated behind a single Supervisor capability, `supports_vm_service_lifecycle_hooks` — no stage ever pauses on a Supervisor where the capability is disabled, and no `LifecycleState` is ever created or patched while it is disabled.

## Non-goals

- Implementing the Lifecycle Operator, or any controller that reconciles `LifecycleStages`, `LifecycleHook`, or `LifecycleState` — those CRDs and their controllers are owned elsewhere.
- Defining or enforcing hook timeout policy, or distinguishing *why* hooks aren't ready (pending vs. failed vs. timed out) — that is entirely the Lifecycle Operator's responsibility. VM Operator only reads `LifecycleState.status.stages[].conditions[HooksReady]`, a single boolean-like signal.
- Stages beyond the four listed above (e.g. `Encrypt`, as illustrated in the one-pager) are out of scope for this spec — exposing any additional stage requires its own spec and implementation plan.
- Any customer-facing UI or CLI for registering `LifecycleHook`s.
- Every lifecycle hook is a pre-stage gate: it runs before the stage's underlying action and holds that action until the hooks are ready. VM Operator does not pause or notify after a stage completes.
- Evaluating more than one stage at a time. Stages are evaluated one at a time, each at its own checkpoint, and a later stage is never evaluated while an earlier one is unresolved (e.g. `ResourceDelete` is not evaluated until `Delete` has resolved). Evaluating stages in parallel is out of scope.

---

## Key Entities

All four entities are in the `lifecycle.vcfa.vmware.com` API group and are owned by the Lifecycle Operator; VM Operator only consumes them.

- **LifecycleHook**: A namespaced registration that a customer or internal VCF service creates to pause `VirtualMachine` workflows before one or more stages, optionally with a timeout. Every consumer registers its own, so one VM can have many.
- **LifecycleStages**: A cluster-scoped catalog, one per CRD, that VM Operator ships to declare the stages it exposes (`Create`, `PowerStateChange`, `Delete`, `ResourceDelete`) along with each stage's type. It is what a `LifecycleHook` refers to.
- **LifecycleSubscribedStages**: A per-namespace summary that the Lifecycle Operator keeps current by aggregating all `LifecycleHook`s into the list of stages that have at least one hook. VM Operator reads it once at a VM's first reconcile to decide whether any hooks exist.
- **LifecycleState**: A per-VM runtime object, owned by the VM, through which VM Operator and the Lifecycle Operator coordinate pause and resume. VM Operator sets `workflowPaused` when it reaches a hooked stage, and the Lifecycle Operator sets `HooksReady=True` once all consumers have acknowledged, failed, or timed out.

---

## User scenarios & testing *(mandatory)*

### User Story 1 — VM Create can be paused for external setup (Priority: P1)

A DevOps user creates a `VirtualMachine` in a namespace where a `LifecycleHook` is registered for the `Create` stage (e.g. so an AD pre-registration step can run before the VM exists in vSphere). The VM is not created in vSphere until that hook reports done.

**Why this priority**: This is the earliest and simplest checkpoint — without it, no later stage's design would be validated end to end. It delivers a demonstrable MVP on its own.

**Independent test**: Create a VM in a namespace with a `Create`-stage `LifecycleHook` registered, reconcile it, and verify no vSphere VM is created and the VM's `Create` condition is blocked; flip the hook's readiness and verify the VM is then created and the condition clears.

**Acceptance scenarios**:

1. **Given** a `VirtualMachine` with no `LifecycleState` yet and a `LifecycleHook` registered for the `Create` stage on `(vmoperator.vmware.com, VirtualMachine)` in its namespace, **When** VM Operator reconciles the VM for the first time, **Then** VM Operator pauses before creating the VM — no vSphere VM is created, the VM's `Create`-stage condition is `False`/`HooksBlocked`, and the stage's event is emitted.
2. **Given** the `Create` stage is paused and its hooks have reported ready (`HooksReady=True`), **When** VM Operator reconciles the VM, **Then** VM Operator resumes, the `Create`-stage condition becomes `True`, and the VM is created in vSphere on that same or a subsequent reconcile.
3. **Given** no `LifecycleHook` exists for the `Create` stage in the VM's namespace, **When** VM Operator reconciles a newly created VM, **Then** VM Operator proceeds to create the VM in vSphere without pausing and without creating any lifecycle resource, identical to today's behavior.

---

### User Story 2 — Power-state changes can be paused for external coordination (Priority: P1)

A DevOps user's VM is about to power on or off, and a `LifecycleHook` on `PowerStateChange` needs to run first (e.g. a rebalancing step). `PowerStateChange` is `Reentrant`: both `PoweredOff → PoweredOn` and `PoweredOn → PoweredOff` transitions are in scope, and each transition is paused independently.

**Why this priority**: Power-state transitions are the most frequent lifecycle event after create/delete, so this is the highest-value checkpoint after US1.

**Independent test**: Transition a VM `PoweredOff → PoweredOn` with a `PowerStateChange` hook registered; verify only the power-on is held (other reconcile work proceeds) and the condition is blocked. Flip readiness, verify the power-on applies. Repeat for the reverse transition and verify it pauses independently of the first.

**Acceptance scenarios**:

1. **Given** a `VirtualMachine` transitioning `PoweredOff` → `PoweredOn` and a blocking `LifecycleHook` on `PowerStateChange`, **When** VM Operator would otherwise apply the power-on to vSphere, **Then** VM Operator pauses that step only — config/device reconciliation unrelated to power state continues to converge — and sets the `PowerStateChange`-stage condition to `False`/`HooksBlocked`.
2. **Given** the same VM once `HooksReady=True`, **When** VM Operator next reconciles, **Then** the power-on is applied and the `PowerStateChange`-stage condition flips to `True`.
3. **Given** the `PowerStateChange` stage is `Reentrant`, **When** the VM later transitions `PoweredOn` → `PoweredOff` with the same hook still registered, **Then** VM Operator pauses again for the new transition (power-off), independent of and unaffected by the prior power-on pause/resume.

---

### User Story 3 — VM deletion can be paused for external cleanup, both in vSphere and in Kubernetes (Priority: P1)

A DevOps user deletes a VM that needs external cleanup (e.g. an external CMDB entry) before the VM is deleted from vSphere (`Delete` stage), and/or one more checkpoint after the vSphere VM is gone but before the Kubernetes object disappears (e.g. to finish writing an audit record keyed by the VM's UID) (`ResourceDelete` stage). `Delete` and `ResourceDelete` remain two distinct, independently-hookable stages — a hook may register on either or both — but `Delete` MUST fully resolve (vSphere VM gone) before `ResourceDelete` is ever evaluated; they are never evaluated in parallel.

**Why this priority**: Deletion is the one path where a missed hook is irreversible — once the vSphere VM or the Kubernetes object is gone, there's no second chance to run cleanup.

**Independent test**: Delete a VM with a `Delete`-stage hook registered; verify the vSphere VM is not removed and the finalizer stays in place until the hook reports done. Then verify `ResourceDelete` is evaluated the same way before the finalizer is actually removed.

**Acceptance scenarios**:

1. **Given** a `VirtualMachine` with `DeletionTimestamp` set and a blocking `LifecycleHook` on `Delete`, **When** VM Operator's delete reconciliation would otherwise call into the provider to delete/unregister the vSphere VM, **Then** VM Operator pauses before that provider call, sets the `Delete`-stage condition to `False`/`HooksBlocked`, and does not remove the VM Operator finalizer.
2. **Given** `HooksReady=True` for the `Delete` stage, **When** VM Operator next reconciles the deleting VM, **Then** the vSphere VM is deleted, and VM Operator then evaluates the `ResourceDelete` stage the same way — pausing before finalizer removal if a hook is registered and not ready, or removing the finalizer immediately if none is.
3. **Given** `HooksReady=True` for `ResourceDelete` (or no hook registered for it), **When** VM Operator next reconciles, **Then** the finalizer is removed and Kubernetes garbage-collects the object.
4. **Given** `Delete` and `ResourceDelete` are two distinct, independently-hookable stages, **When** a hook is registered on either or both, **Then** `Delete` fully resolves (vSphere VM gone) before `ResourceDelete` is ever evaluated — the two stages are never evaluated in parallel.
5. **Given** a `Delete`-stage hook whose `LifecycleState` carries VM Operator's finalizer (created at the VM's first reconcile, or created by the Lifecycle Operator with that finalizer), **When** the entire namespace is deleted (not just the VM individually), **Then** the `Delete`/`ResourceDelete` hooks are still honored — the namespace remains in `Terminating` until `HooksReady` resolves for both, the same as any other stuck finalizer. A `LifecycleState` created day-2 by the Lifecycle Operator *without* VM Operator's finalizer is not protected — see the accepted-race note in "Resolved decisions."

---

### User Story 4 — Diagnosing a blocked VM from status alone (Priority: P2)

A DevOps user needs to tell, from the VM object alone, whether any stage is currently holding up reconciliation — and *which* stage, unambiguously, without parsing a condition message — without inspecting the `LifecycleState` or any hook resource directly.

**Why this priority**: Diagnosability. Without it, a DevOps user waiting on a stuck VM has no way to distinguish "waiting on a hook" from any other stalled reconcile, nor to tell which of several possible stages is responsible.

**Independent test**: With a stage currently blocked, run `kubectl get vm <name> -o yaml` and confirm that stage's own condition is `False` with a reason indicating hooks are pending, and that the other three stages' conditions are unaffected (either `True` or absent). Resolve the hook and confirm that stage's condition flips.

**Acceptance scenarios**:

1. **Given** a stage is currently blocked, **When** a DevOps user runs `kubectl get vm <name> -o yaml`, **Then** that stage's own condition — one of `VirtualMachineConditionCreateHooksReady`, `...PowerStateChangeHooksReady`, `...DeleteHooksReady`, `...ResourceDeleteHooksReady` — is `False` with reason `HooksBlocked`, identifying the blocked stage by **condition type**, not by parsing a message, without needing to inspect the `LifecycleState` or any hook resource directly.
2. **Given** no stage is currently blocked, **When** status is read, **Then** each stage's condition is either `True` (the stage was evaluated and is not blocked) or **absent** (the stage has not been reached yet this VM's lifecycle, e.g. `PowerStateChange` before any transition is attempted) — never `False`, and never `Unknown`. A DevOps user, and any other controller consuming these conditions, must treat "absent" and "`True`" identically: not blocked.
3. **Given** `Delete` and `ResourceDelete` are sequential and never blocked together, and `Create`, `PowerStateChange`, and `Delete` are mutually exclusive by VM phase, **When** status is read, **Then** at most one of the four conditions is ever `False` at a time.

---


## Resolved decisions

- **Stages**: all four stages are `Reentrant` and `blocking=true`; the gate treats them uniformly, so hooks must be idempotent, and `Delete` resolves before `ResourceDelete`.
- **Create trigger**: "VM exists" is decided by the vCenter lookup (`getVM`), not `Status.UniqueID`, so a `Status` reset never re-runs `Create`.
- **Readiness and conditions**: VM Operator mirrors only `HooksReady` and exposes four per-stage conditions (`False`/`HooksBlocked`, `True`, or absent, with absent treated as `True`).
- **`LifecycleState` ownership**: VM Operator creates it once at first reconcile from `LifecycleSubscribedStages`; the Lifecycle Operator owns it afterwards, and a missing entry means "no hook".
- **Namespace deletion**: `Delete`/`ResourceDelete` hooks survive namespace teardown only while `LifecycleState` carries VM Operator's finalizer; two races are accepted.
- **Packaging and scope**: gated by `supports_vm_service_lifecycle_hooks`, `vmoperator-stages` ships with the Lifecycle Operator's package, and other controllers were verified unaffected (`VirtualMachineGroup` is G7).

---

## Success criteria *(mandatory)*

### Measurable outcomes

- **SC-001**: A `Create`-stage hook registered on a VM's namespace prevents that VM's vSphere creation until `HooksReady=True`, verifiable across create/hook-resolve sequences with no vSphere VM created prematurely.
- **SC-002**: A `PowerStateChange`-stage hook holds only the power-state application, not any other in-flight reconcile step, and re-triggers independently on every subsequent power transition.
- **SC-003**: A `Delete`-stage hook prevents vSphere-side VM deletion, and a `ResourceDelete`-stage hook prevents finalizer removal, until each resolves in sequence — never in parallel.
- **SC-004**: A VM with no `LifecycleHook` registered anywhere in its namespace shows zero behavior change from today — no `LifecycleState` created, no pause, no extra reconcile latency.
- **SC-005**: With the `supports_vm_service_lifecycle_hooks` capability disabled, behavior is identical to the feature not existing — verifiable by the pre-existing test suites passing unchanged.
- **SC-006**: A DevOps user can determine, from the VM's four per-stage conditions alone (`VirtualMachineConditionCreateHooksReady`, `...PowerStateChangeHooksReady`, `...DeleteHooksReady`, `...ResourceDeleteHooksReady`), whether any stage is blocked by hooks and which one — by condition type, not by parsing a message.
- **SC-007**: Deleting the namespace containing a VM whose `LifecycleState` carries VM Operator's finalizer (the hook was present at the VM's first reconcile, or the Lifecycle Operator created the object with that finalizer) does not bypass a `Delete`/`ResourceDelete` hook — the namespace stays in `Terminating` until `HooksReady` resolves for the affected stage(s), verifiable by deleting a namespace with such a VM and confirming both the namespace and the VM remain present (with the affected stage's condition `False`) until the hook is resolved. The day-2/no-finalizer case in "Resolved decisions" is out of scope for this criterion.
- **SC-008**: A `VirtualMachineReplicaSet` whose member has a blocked `Create` stage never creates more VMs than `spec.replicas` — verifiable by registering a blocking `Create` hook, scaling a ReplicaSet up, and confirming the VM count never exceeds `spec.replicas` for as long as the hook remains unresolved.
- **SC-009**: A `VirtualMachineService` never routes traffic to a VM whose power-on is blocked by a `PowerStateChange` hook — verifiable by registering a blocking `PowerStateChange` hook on a `PoweredOff` VM targeted by a service, requesting power-on, and confirming the VM never appears in the service's `Endpoints` `Addresses` while the hook remains unresolved.
- **SC-010**: A `VirtualMachineSnapshot` request against a VM blocked on the `Create` stage fails safely (no vSphere call attempted) rather than creating a snapshot of a VM that doesn't yet exist — verifiable by registering a blocking `Create` hook, requesting a snapshot of the not-yet-created VM, and confirming no snapshot is attempted until the VM exists.

## Open questions

- **G7**: How does `VirtualMachineGroup` preserve its boot-order guarantee when a tier member's `PowerStateChange` hook blocks past its stamped delay? Mechanism is unresolved — see `plan.md` §7 (marked TBD).


## Review & acceptance checklist

- [x] All user stories have at least two Given/When/Then scenarios.
- [x] Each scenario is independently testable.
- [ ] G7 (`VirtualMachineGroup` boot-order preservation) has a resolved mechanism — currently blocked on `plan.md` §7.
- [x] The no-hook-registered case is specified as a no-op/no-regression path.
- [x] Hooks-not-ready handling is specified (mirrors `HooksReady` only, no failure-detail parsing).
- [x] Stage `type`/`blocking` defaults are specified.
- [x] Condition/reason names and the capability name are specified.
- [x] Out-of-scope items (Lifecycle Operator, eventing system, additional stages, hook-failure-detail parsing) are listed.
- [x] Feature-flag/capability-off behavior is specified (G6, SC-005).
- [x] The reconcile pipeline and the per-checkpoint decision logic (shared `ReconcileStage`) are diagrammed in `plan.md` "Reconcile flow" and decision table.
