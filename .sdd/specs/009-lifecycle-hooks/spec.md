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

- **G1**: VM Operator MUST expose exactly four lifecycle stages for `VirtualMachine`: **Create**, **PowerStateChange**, **Delete** (vSphere-side deletion), and **ResourceDelete** (Kubernetes CR finalizer removal).
- **G2**: VM Operator MUST pause the corresponding underlying action for a stage — and MUST NOT pause unrelated reconcile work — whenever that VM's `LifecycleState` indicates the stage is registered for blocking and hooks are not yet ready.
- **G3**: VM Operator MUST automatically resume the paused action as soon as the `LifecycleState`'s `HooksReady` condition for that stage becomes `True`, without requiring any VM spec change or manual intervention.
- **G4**: VM Operator MUST surface, via a `VirtualMachine` status condition, whether any lifecycle stage is currently blocked by hooks, including which stage in the message.
- **G5**: VM Operator MUST continue to function (create/update/delete VMs normally) when no `LifecycleState`/hook exists for a VM, with no measurable behavior change from today.
- **G6**: The entire feature MUST be gated behind a single Supervisor capability, `supports_vm_service_lifecycle_hooks` — no stage ever pauses on a Supervisor where the capability is disabled, and no `LifecycleState` is ever created or patched while it is disabled.

## Non-goals

- Implementing the Lifecycle Operator, or any controller that reconciles `LifecycleStages`, `LifecycleHook`, or `LifecycleState` — those CRDs and their controllers are owned elsewhere.
- Defining or enforcing hook timeout policy, or distinguishing *why* hooks aren't ready (pending vs. failed vs. timed out) — that is entirely the Lifecycle Operator's responsibility. VM Operator only reads `LifecycleState.status.stages[].conditions[HooksReady]`, a single boolean-like signal.
- Stages beyond the four listed above (e.g. `Encrypt`, as illustrated in the one-pager) are out of scope for this spec — exposing any additional stage requires its own spec and implementation plan.
- Any customer-facing UI or CLI for registering `LifecycleHook`s.

---

## User scenarios & testing *(mandatory)*

### User Story 1 — VM Create can be paused for external setup (Priority: P1)

A DevOps user creates a `VirtualMachine` in a namespace where a `LifecycleHook` is registered for the `Create` stage (e.g. so an AD pre-registration step can run before the VM exists in vSphere). The VM is not created in vSphere until that hook reports done.

**Why this priority**: This is the earliest and simplest checkpoint — without it, no later stage's design would be validated end to end. It delivers a demonstrable MVP on its own.

**Independent test**: Create a VM in a namespace with a `Create`-stage `LifecycleHook` registered, reconcile it, and verify no vSphere VM is created and the VM's `Create` condition is blocked; flip the hook's readiness and verify the VM is then created and the condition clears.

**Acceptance scenarios**:

1. **Given** a `VirtualMachine` with no `LifecycleState` yet and a `LifecycleHook` registered for the `Create` stage on `(vmoperator.vmware.com, VirtualMachine)` in its namespace, **When** VM Operator reconciles the VM for the first time, **Then** VM Operator pauses before creating the VM — no vSphere VM is created, the VM's `LifecycleHooksReady` condition is `False` with a message naming the `Create` stage, and the stage's event is emitted.
2. **Given** the `Create` stage is paused and its hooks have reported ready (`HooksReady=True`), **When** VM Operator reconciles the VM, **Then** VM Operator resumes, the `LifecycleHooksReady` condition becomes `True`, and the VM is created in vSphere on that same or a subsequent reconcile.
3. **Given** no `LifecycleHook` exists for the `Create` stage in the VM's namespace, **When** VM Operator reconciles a newly created VM, **Then** VM Operator proceeds to create the VM in vSphere without pausing and without creating any lifecycle resource, identical to today's behavior.

---

### User Story 2 — Power-state changes can be paused for external coordination (Priority: P1)

A DevOps user's VM is about to power on or off, and a `LifecycleHook` on `PowerStateChange` needs to run first (e.g. a rebalancing step). `PowerStateChange` is `Reentrant`: both `PoweredOff → PoweredOn` and `PoweredOn → PoweredOff` transitions are in scope, and each transition is paused independently.

**Why this priority**: Power-state transitions are the most frequent lifecycle event after create/delete, so this is the highest-value checkpoint after US1.

**Independent test**: Transition a VM `PoweredOff → PoweredOn` with a `PowerStateChange` hook registered; verify only the power-on is held (other reconcile work proceeds) and the condition is blocked. Flip readiness, verify the power-on applies. Repeat for the reverse transition and verify it pauses independently of the first.

**Acceptance scenarios**:

1. **Given** a `VirtualMachine` transitioning `PoweredOff` → `PoweredOn` and a blocking `LifecycleHook` on `PowerStateChange`, **When** VM Operator would otherwise apply the power-on to vSphere, **Then** VM Operator pauses that step only — config/device reconciliation unrelated to power state continues to converge — and sets the `PowerStateChange` condition to blocked.
2. **Given** the same VM once `HooksReady=True`, **When** VM Operator next reconciles, **Then** the power-on is applied and the condition flips to ready.
3. **Given** the `PowerStateChange` stage is `Reentrant`, **When** the VM later transitions `PoweredOn` → `PoweredOff` with the same hook still registered, **Then** VM Operator pauses again for the new transition (power-off), independent of and unaffected by the prior power-on pause/resume.

---

### User Story 3 — VM deletion can be paused for external cleanup, both in vSphere and in Kubernetes (Priority: P1)

A DevOps user deletes a VM that needs external cleanup (e.g. an external CMDB entry) before the VM is deleted from vSphere (`Delete` stage), and/or one more checkpoint after the vSphere VM is gone but before the Kubernetes object disappears (e.g. to finish writing an audit record keyed by the VM's UID) (`ResourceDelete` stage). `Delete` and `ResourceDelete` remain two distinct, independently-hookable stages — a hook may register on either or both — but `Delete` MUST fully resolve (vSphere VM gone) before `ResourceDelete` is ever evaluated; they are never evaluated in parallel.

**Why this priority**: Deletion is the one path where a missed hook is irreversible — once the vSphere VM or the Kubernetes object is gone, there's no second chance to run cleanup.

**Independent test**: Delete a VM with a `Delete`-stage hook registered; verify the vSphere VM is not removed and the finalizer stays in place until the hook reports done. Then verify `ResourceDelete` is evaluated the same way before the finalizer is actually removed.

**Acceptance scenarios**:

1. **Given** a `VirtualMachine` with `DeletionTimestamp` set and a blocking `LifecycleHook` on `Delete`, **When** VM Operator's delete reconciliation would otherwise call into the provider to delete/unregister the vSphere VM, **Then** VM Operator pauses before that provider call, sets the `Delete` condition to blocked, and does not remove the VM Operator finalizer.
2. **Given** `HooksReady=True` for the `Delete` stage, **When** VM Operator next reconciles the deleting VM, **Then** the vSphere VM is deleted, and VM Operator then evaluates the `ResourceDelete` stage the same way — pausing before finalizer removal if a hook is registered and not ready, or removing the finalizer immediately if none is.
3. **Given** `HooksReady=True` for `ResourceDelete` (or no hook registered for it), **When** VM Operator next reconciles, **Then** the finalizer is removed and Kubernetes garbage-collects the object.
4. **Given** `Delete` and `ResourceDelete` are two distinct, independently-hookable stages, **When** a hook is registered on either or both, **Then** `Delete` fully resolves (vSphere VM gone) before `ResourceDelete` is ever evaluated — the two stages are never evaluated in parallel.
5. **Given** a `Delete`-stage hook whose `LifecycleState` carries VM Operator's finalizer (created at the VM's first reconcile, or created by the Lifecycle Operator with that finalizer), **When** the entire namespace is deleted (not just the VM individually), **Then** the `Delete`/`ResourceDelete` hooks are still honored — the namespace remains in `Terminating` until `HooksReady` resolves for both, the same as any other stuck finalizer. A `LifecycleState` created day-2 by the Lifecycle Operator *without* VM Operator's finalizer is not protected — see the accepted-race note in "Resolved decisions."

---

### User Story 4 — Diagnosing a blocked VM from status alone (Priority: P2)

A DevOps user needs to tell, from the VM object alone, whether any stage is currently holding up reconciliation, without inspecting the `LifecycleState` or any hook resource directly.

**Why this priority**: Diagnosability. Without it, a DevOps user waiting on a stuck VM has no way to distinguish "waiting on a hook" from any other stalled reconcile.

**Independent test**: With a stage currently blocked, run `kubectl get vm <name> -o yaml` and confirm the corresponding condition is `False` with a reason indicating hooks are pending. Resolve the hook and confirm the condition flips.

**Acceptance scenarios**:

1. **Given** any of the four stages is currently blocked, **When** a DevOps user runs `kubectl get vm <name> -o yaml`, **Then** the `VirtualMachineConditionLifecycleHooksReady` condition is `False` with reason `HooksBlocked` and a message like "blocked on Create stage hooks", without needing to inspect the `LifecycleState` or any hook resource directly.
2. **Given** no stage is currently blocked, **When** status is read, **Then** the condition is `True`, never `Unknown`.
3. **Given** the `Delete` stage is blocked, **When** status is read, **Then** the condition message names `Delete` only — `Delete` and `ResourceDelete` are sequential and never blocked together, and `Create`, `PowerStateChange` and `Delete` are mutually exclusive by VM phase, so at most one stage is ever named. The condition reads inverted (`True` means *not* blocked); see `model.md`.

---


## Resolved decisions

All open questions from the prior draft have been resolved:

- **Power-off inclusion**: `PowerStateChange` covers both `PoweredOff → PoweredOn` and `PoweredOn → PoweredOff`, and is `Reentrant`.
- **Stage `type`/`blocking` defaults**: `Create`=`Single`, `PowerStateChange`=`Reentrant`, `Delete`=`Single`, `ResourceDelete`=`Single`; all four are `blocking=true`.
- **Hooks-not-ready handling**: VM Operator does not distinguish pending/failed/timed-out; it only mirrors the `HooksReady` boolean via a single `False` reason, `HooksBlocked`. That detail lives in the Lifecycle Operator and is out of scope here. This is not a terminal state — VM Operator is a level-triggered controller with no concept of permanent failure (see [`research.md`](./research.md) "Terminal failures"); it keeps reconciling at the normal cadence and self-heals the moment `HooksReady` flips `True`.
- **Supervisor-level opt-in gating**: a dedicated Supervisor capability, `supports_vm_service_lifecycle_hooks`, gates the entire feature (see G6, `model.md`/`plan.md`), not deferred.
- **Condition type/reason names**: one new `VirtualMachine` condition type, `VirtualMachineConditionLifecycleHooksReady`. When `False`, reason is `HooksBlocked` and the message includes which stage is currently pausing (e.g., "blocked on Create stage hooks"); when `True`, no hooks are blocking any stage (see `model.md`).
- **`LifecycleState` deleted out-of-band**: VM Operator does not re-create it. A missing `LifecycleState` (or a missing stage entry) means "no hook," so the checkpoint proceeds until the Lifecycle Operator re-creates it. Keeping `LifecycleState` accurate after the VM's first reconcile is the Lifecycle Operator's responsibility.
- **VM Operator restart mid-pause**: state lives in the `LifecycleState` CR, not in-memory, so a restart does not lose the pause — the next reconcile re-evaluates from the CR as normal (level-triggered reconciliation).
- **Zero-hook detection (`HookExists` in Diagram D)**: the Lifecycle Operator owns and fully maintains a new `AggregatedLifecycleHooks` resource (`lifecycle.vcfa.vmware.com/v1alpha1`, one per namespace), computed and kept current by the Lifecycle Operator. VM Operator's `HookExists` check is a single cached read of it, read exactly once, at the VM's first reconcile, to decide whether a `LifecycleState` is needed at all and to seed its stages; it never lists or watches `LifecycleHook` (see `model.md` "`AggregatedLifecycleHooks`").
- **Namespace deletion must not silently bypass `Delete`/`ResourceDelete` hooks**: a `Delete`/`ResourceDelete` hook is honored across a namespace teardown **whenever its `LifecycleState` carries VM Operator's finalizer** — which VM Operator adds when it creates the object at the VM's first reconcile (seeding every stage the namespace's `AggregatedLifecycleHooks` then lists). `Create`/`PowerStateChange` do not need this: a missed `Create` hook is recoverable in spirit, and `PowerStateChange` self-heals on every future transition. Two races are accepted: (1) a hook registered in the same instant teardown begins; (2) a `Delete`/`ResourceDelete` hook registered *day-2* against a VM that had no `LifecycleState` at first reconcile — the Lifecycle Operator creates the object, and unless it adds VM Operator's finalizer the namespace controller can remove it mid-check. Closing (2) requires the Lifecycle Operator to create day-2 `LifecycleState`s with VM Operator's finalizer (see `plan.md` "Confirmations needed from the Lifecycle Operator team"). Mechanics: `model.md` "Namespace-deletion protection" and `plan.md` §1a.

---

## Success criteria *(mandatory)*

### Measurable outcomes

- **SC-001**: A `Create`-stage hook registered on a VM's namespace prevents that VM's vSphere creation until `HooksReady=True`, verifiable across create/hook-resolve sequences with no vSphere VM created prematurely.
- **SC-002**: A `PowerStateChange`-stage hook holds only the power-state application, not any other in-flight reconcile step, and re-triggers independently on every subsequent power transition.
- **SC-003**: A `Delete`-stage hook prevents vSphere-side VM deletion, and a `ResourceDelete`-stage hook prevents finalizer removal, until each resolves in sequence — never in parallel.
- **SC-004**: A VM with no `LifecycleHook` registered anywhere in its namespace shows zero behavior change from today — no `LifecycleState` created, no pause, no extra reconcile latency.
- **SC-005**: With the `supports_vm_service_lifecycle_hooks` capability disabled, behavior is identical to the feature not existing — verifiable by the pre-existing test suites passing unchanged.
- **SC-006**: A DevOps user can determine, from the `VirtualMachineConditionLifecycleHooksReady` condition alone, whether any stage is blocked by hooks and which stage.
- **SC-007**: Deleting the namespace containing a VM whose `LifecycleState` carries VM Operator's finalizer (the hook was present at the VM's first reconcile, or the Lifecycle Operator created the object with that finalizer) does not bypass a `Delete`/`ResourceDelete` hook — the namespace stays in `Terminating` until `HooksReady` resolves for the affected stage(s), verifiable by deleting a namespace with such a VM and confirming both the namespace and the VM remain present (with the `LifecycleHooksReady` condition `False`) until the hook is resolved. The day-2/no-finalizer case in "Resolved decisions" is out of scope for this criterion.

## Open questions

None.


## Review & acceptance checklist

- [x] All user stories have at least two Given/When/Then scenarios.
- [x] Each scenario is independently testable.
- [x] The no-hook-registered case is specified as a no-op/no-regression path.
- [x] Hooks-not-ready handling is specified (mirrors `HooksReady` only, no failure-detail parsing).
- [x] Stage `type`/`blocking` defaults are specified.
- [x] Condition/reason names and the capability name are specified.
- [x] Out-of-scope items (Lifecycle Operator, eventing system, additional stages, hook-failure-detail parsing) are listed.
- [x] Feature-flag/capability-off behavior is specified (G6, SC-005).
- [x] The reconcile pipeline and the per-checkpoint decision logic (shared `ReconcileStage`) are diagrammed in `plan.md` "Reconcile flow" and decision table.
