# Feature Specification: Blocking Lifecycle Hooks

- **Feature branch**: [`lifecyclehooks-sdd`](../../../../tree/lifecyclehooks-sdd)
  - **Fork**: N/A (branch on `vmware-tanzu/vm-operator`)
  - **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-08-24
- **Status**: Draft
- **Epic**: vmop-3377

---

## Summary

Customers migrating from vRA, and internal VCF services, need to pause specific points in a `VirtualMachine`'s reconciliation — before it's created in vSphere, before a power-state change, or before it's deleted (from vSphere and, subsequently, from Kubernetes) — to run work outside VM Operator's control (AD registration, resource rebalancing, external cleanup) before the workflow resumes.

---

## Goals

- VM Operator MUST expose exactly four lifecycle stages for `VirtualMachine` in v1: **Create**, **PowerStateChange**, **Delete** (vSphere-side deletion), and **ResourceDelete** (Kubernetes CR finalizer removal) — the latter two are presented as a single user story (US3) since they're one sequential deletion flow from a consumer's point of view, but remain distinct, independently-hookable stages.
- For each stage, VM Operator MUST pause the corresponding underlying action (and MUST NOT pause unrelated reconcile work) whenever a `LifecycleState` for that VM indicates the stage is registered for blocking and hooks are not yet ready.
- VM Operator MUST resume the paused action once the `LifecycleState`'s `HooksReady` condition for that stage becomes `True`, without requiring a full VM spec change to trigger the resume.
- VM Operator MUST surface, via a `VirtualMachine` status condition per stage, whether that stage is currently blocked and whether it has ever been reached.
- VM Operator MUST support multiple hooks registered on the same stage for the same VM (multiplexed through `LifecycleState.status.stages[].hooks[]`) without requiring changes to how VM Operator itself pauses/resumes.
- VM Operator MUST continue to function (create/update/delete VMs normally) when no `LifecycleState`/hook exists for a VM, with no measurable behavior change from today.
- The entire feature MUST be gated behind a Supervisor capability (see US4) — no stage ever pauses on a Supervisor where the capability is disabled.

## Non-goals

- Implementing the Lifecycle Operator, or any controller that reconciles `LifecycleStages`, `LifecycleHook`, or `LifecycleState` — those CRDs and their controllers are owned elsewhere.
- Implementing the eventing/notification system (`Subscription`/`Event`, group `eventing.vcfa.vmware.com`) that notifies external hook owners that a stage was reached.
- Defining or enforcing hook timeout policy, and distinguishing *why* hooks aren't ready (pending vs. failed vs. timed out) — that is entirely the Lifecycle Operator's responsibility. It updates `LifecycleState.status.stages[].conditions[HooksReady]`; VM Operator only reads that single boolean-like condition (see US3/Edge cases). This is worth calling out explicitly since it's a natural place to *want* more detail, but it is out of scope for this design.
- Stages beyond the four listed above (e.g. `Encrypt`, as illustrated in the one-pager) — may be added in a follow-up spec.
- Any customer-facing UI or CLI for registering `LifecycleHook`s.

---

## User stories / acceptance criteria

### US1 — Platform engineer: VM Create can be paused for external setup (Priority: P1)

An internal VCF service or customer integration needs to run work (e.g. AD pre-registration) before a `VirtualMachine` is created in vSphere.

**Given** a `VirtualMachine` with no `LifecycleState` yet and a `LifecycleHook` registered for the `Create` stage on `(vmoperator.vmware.com, VirtualMachine)` in its namespace,
**When** VM Operator reconciles the VM for the first time,
**Then** VM Operator creates a `LifecycleState` for the VM, sets `spec.stages[Create].workflowPaused=true`, sets the VM's `Create` stage condition to blocked, emits the stage's event, and does not create the underlying vSphere VM.

**Given** a `LifecycleState` with `spec.stages[Create].workflowPaused=true` and `status.stages[Create].conditions[HooksReady]=True`,
**When** VM Operator reconciles the VM,
**Then** VM Operator sets `workflowResumed=true`, updates the VM's `Create` condition to ready, and proceeds to create the VM in vSphere on that same or a subsequent reconcile.

**Given** no `LifecycleHook` exists for the `Create` stage in the VM's namespace,
**When** VM Operator reconciles a newly created VM,
**Then** VM Operator proceeds to create the VM in vSphere without creating a `LifecycleState` or pausing, identical to today's behavior.

### US2 — DevOps user: power-state changes can be paused for external coordination (Priority: P1)

A flex-namespace rebalancing service needs to reposition resources before a VM powers on or off. `PowerStateChange` is a `Reentrant` stage: both `PoweredOff → PoweredOn` and `PoweredOn → PoweredOff` transitions are in scope, and each transition is paused independently.

**Given** a `VirtualMachine` transitioning `PoweredOff` → `PoweredOn` and a blocking `LifecycleHook` on `PowerStateChange`,
**When** VM Operator would otherwise apply the power-on to vSphere,
**Then** VM Operator pauses that step only — config/device reconciliation unrelated to power state continues to converge — and sets the `PowerStateChange` condition to blocked.

**Given** the same VM once `HooksReady=True`,
**When** VM Operator next reconciles,
**Then** the power-on is applied and the condition flips to ready.

**Given** the `PowerStateChange` stage is `Reentrant`,
**When** the VM later transitions `PoweredOn` → `PoweredOff` with the same hook still registered,
**Then** VM Operator pauses again for the new transition (power-off), independent of and unaffected by the prior power-on pause/resume.

### US3 — Tenant admin: VM deletion can be paused for external cleanup, both in vSphere and in Kubernetes (Priority: P1)

A customer integration needs to clean up resources outside VCF (e.g. an external CMDB entry) before the VM is deleted from vSphere (`Delete` stage), and/or needs one more checkpoint after the vSphere VM is gone but before the Kubernetes object disappears (e.g. to finish writing an audit record keyed by the VM's UID) (`ResourceDelete` stage). `Delete` and `ResourceDelete` remain two distinct, independently-hookable stages — a consumer may register on either or both — but `Delete` MUST fully resolve (vSphere VM gone) before `ResourceDelete` is ever evaluated; they are never evaluated in parallel.

**Given** a `VirtualMachine` with `DeletionTimestamp` set and a blocking `LifecycleHook` on `Delete`,
**When** VM Operator's delete reconciliation would otherwise call into the provider to delete/unregister the vSphere VM,
**Then** VM Operator pauses before that provider call, sets the `Delete` condition to blocked, and does not remove the VM Operator finalizer.

**Given** `HooksReady=True` for the `Delete` stage,
**When** VM Operator next reconciles the deleting VM,
**Then** the vSphere VM is deleted, and VM Operator then evaluates the `ResourceDelete` stage the same way — pausing before finalizer removal if a hook is registered and not ready, or removing the finalizer immediately if none is.

**Given** `HooksReady=True` for `ResourceDelete` (or no hook registered for it),
**When** VM Operator next reconciles,
**Then** the finalizer is removed and Kubernetes garbage-collects the object.

### US4 — Platform engineer: the entire feature is gated by a Supervisor capability (Priority: P0)

This feature ships behind a new Supervisor capability (matching the existing `supports_telco_vm_service_api`-style gating, e.g. `pkg/config/capabilities`), not an always-on default.

**Given** the capability is disabled for a Supervisor,
**When** any `VirtualMachine` reconciles, regardless of any `LifecycleHook`/`LifecycleState` present in its namespace,
**Then** VM Operator never pauses any stage and never creates/patches a `LifecycleState` — behavior is identical to the feature not existing.

**Given** the capability is enabled,
**When** a VM's next reconcile runs,
**Then** stage gating begins applying from that reconcile onward, per US1-US3.

### US5 — DevOps user: diagnosing a blocked VM from status alone (Priority: P0)

**Given** any of the four stages is currently blocked,
**When** a DevOps user runs `kubectl get vm <name> -o yaml`,
**Then** the corresponding stage condition is `False` with a reason indicating hooks are pending, without needing to inspect the `LifecycleState` or any hook resource directly.

**Given** a stage has never been reached (e.g. a `Single`-type stage already resumed once),
**When** status is read,
**Then** its condition reflects "resumed"/ready, not "never evaluated" — no condition is left `Unknown` after the stage's first reconcile pass.

---

## Edge cases

- A VM with no `LifecycleHook` registered anywhere in its namespace MUST see zero behavior change and zero extra API calls beyond a best-effort check that no hook applies (exact mechanism — direct `LifecycleHook` list vs. relying solely on `LifecycleState` absence — is a `plan.md` concern, but the **spec-level requirement is no regression for the common no-hook case**).
- VM Operator MUST treat `LifecycleState.status.stages[].conditions[HooksReady]` as its only signal for a stage: `True` means proceed, anything else (`False`, pending, failed, timed out) means keep waiting. VM Operator MUST NOT attempt to distinguish *why* `HooksReady` is `False` — that detail (a hook still running, a hook that errored, a hook that timed out) is entirely internal to the Lifecycle Operator, which is the component that updates `HooksReady` accordingly. VM Operator's own stage condition simply mirrors "waiting on hooks" while `False` and "ready" once `True`; it is never treated as terminal — VM Operator, as a level-triggered controller, has no concept of a permanent failure state (see [`research.md`](./research.md) "Terminal failures") and keeps reconciling at the normal cadence, resuming automatically the moment `HooksReady` flips `True`.
- A `LifecycleState` deleted out-of-band (e.g. by mistake) while a stage is paused: VM Operator MUST re-create it and re-enter the paused state on the next reconcile, rather than treating its absence as "resume" (matches the one-pager's "for safety, patch/create" guidance).
- VM Operator restart mid-pause: state lives in the `LifecycleState` CR, not in-memory, so a restart MUST NOT lose the pause — the next reconcile re-evaluates from the CR as normal (level-triggered reconciliation).

## Open questions

Resolved:

- ~~Power-off inclusion~~ — **Resolved**: `PowerStateChange` covers both `PoweredOff → PoweredOn` and `PoweredOn → PoweredOff`, and is `Reentrant` (see US2).
- ~~Stage `type` defaults~~ — **Resolved**: `Create`=`Single`, `PowerStateChange`=`Reentrant`, `Delete`=`Single` for each of its two sequential checkpoints, all `blocking=true`.
- ~~Hooks-not-ready reason detail~~ — **Resolved**: VM Operator does not distinguish pending/failed/timed-out; it only mirrors the `HooksReady` boolean (see Edge cases above). That detail lives in the Lifecycle Operator and is out of scope here.
- ~~Supervisor-level opt-in gating~~ — **Resolved**: a dedicated Supervisor capability gates the entire feature (see US4, `model.md`, `plan.md`), not deferred.

Still open:

- [NEEDS CLARIFICATION: exact condition type/reason names for the four new `VirtualMachine` conditions, and the exact capability name — see `model.md` proposal. Not a blocker for Phase 1/2 scaffolding.]

## Review & acceptance checklist

- [x] All user stories have at least two Given/When/Then scenarios.
- [x] Each scenario is independently testable.
- [x] The no-hook-registered case is specified as a no-op/no-regression path.
- [x] Hooks-not-ready handling is specified (mirrors `HooksReady` only, no failure-detail parsing).
- [x] Stage `type`/`blocking` defaults are specified.
- [ ] Condition/reason names and the capability name are specified (currently open).
- [x] Out-of-scope items (Lifecycle Operator, eventing system, additional stages, hook-failure-detail parsing) are listed.
