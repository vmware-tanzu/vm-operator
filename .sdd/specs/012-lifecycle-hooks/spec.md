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

VM Operator does not own the coordination mechanism itself. A separate Lifecycle Operator provides the framework for coordination and owns the `LifecycleStages`, `LifecycleHook`, `LifecycleSubscribedStages`,  and `LifecycleState` CRDs (group `lifecycle.vcfa.vmware.com`) and all hook fan-out and timeout logic. VM Operator's role is limited to declaring which stages it exposes, and at each stage checkpoint, reading a single readiness signal off `LifecycleState` and pausing or proceeding accordingly — the same "consumer-only" relationship this repo already has for example with `external/byok`'s `EncryptionClass`.

---

## Goals

- **G1**: VM Operator MUST expose exactly four pre-stage lifecycle stages for `VirtualMachine`: **Create**, **PowerStateChange**, **Delete** (vSphere-side deletion), and **ResourceDelete** (Kubernetes CR finalizer removal) for this release.
- **G2**: Whenever a VM's `LifecycleState` indicates a stage is registered for blocking and its hooks are not yet ready, VM Operator MUST pause only that stage's underlying action. The rest of the reconcile workflow MUST remain exactly as it is today — no other step is skipped, delayed, or otherwise altered because one stage is paused.
- **G3**: VM Operator MUST automatically resume the paused action as soon as the `LifecycleState`'s `HooksReady` condition for that stage becomes `True`, without requiring any VM spec change or manual intervention.
- **G4**: VM Operator MUST reflect the hook status of the last evaluated stage that has hooks in one `VirtualMachine` condition, `LifecycleHookReady`: reason is the stage (`CreateStage`, `PowerStateChangeStage`, `DeleteStage`, `ResourceDeleteStage`); `True` when the stage's hooks finished without failure or the hold was abandoned; `False` while blocked or when a hook failed; `Unknown` when VM Operator could not evaluate the stage's hooks; message is the stage's `HooksReady` message, prefixed to say whether hooks are pending or failed. A stage with no hook writes no condition, so a VM in a namespace with no hooks never has one.
- **G5**: VM Operator MUST continue to function (create/update/delete VMs normally) when no `LifecycleState`/hook exists for a VM, with no measurable behavior change from today.
- **G6**: The entire feature MUST be gated behind a single Supervisor capability, `supports_vm_service_lifecycle_hooks`. It depends on another capability the Lifecycle Framework enables when its package, including its CRDs, is installed, so the capability being enabled means the CRDs are present — no stage ever pauses on a Supervisor where the capability is disabled, and no `LifecycleState` is ever created or patched while it is disabled, with one exception: when a VM is deleted, VM Operator still removes its finalizer from that VM's `LifecycleState`, so that disabling the capability never blocks deleting a VM or its namespace.
- **G7**: Controllers that scale `VirtualMachine` replicas like VirtualMachineReplicaSet, VirtualMachineDeployment MUST scale up and down to their desired replica count as they would without hooks. 
- **G8**: `VirtualMachineGroup` MUST be able to apply boot order and power-on/off delays as it does today. A member's `PowerStateChange` hook is evaluated once its delay has elapsed and holds only that member.
- **G9**: `VirtualMachineService` MUST be able to discover and drain traffic to the exposed `VirtualMachines` as it does today even if the individual `VirtualMachine` members has hooks for `Create`, `Delete` or `PowerStateChange`.

## Non-goals

- Implementing the Lifecycle Operator, or any controller that reconciles `LifecycleStages`, `LifecycleHook`, `LifecycleSubscribedStages`, or `LifecycleState` — those CRDs and their controllers are owned elsewhere.
- Propagating results from `LifecycleHook` to `LifecycleState` is owned entirely by Lifecycle Operator.
- Defining or enforcing hook timeout policy, or reading per-hook results — that is entirely the Lifecycle Operator's responsibility. Of the hook results, VM Operator reads only `LifecycleState.status.stages[].conditions[HooksReady]` (status, reason, message), never `status.stages[].hooks[]`.
- Stages beyond the four listed above (e.g. `Encrypt`, as illustrated in the one-pager) are out of scope for this spec — exposing any additional stage requires its own spec and implementation plan.
- Any customer-facing UI or CLI for registering `LifecycleHook`s.
- Every lifecycle hook is a pre-stage gate: it runs before the stage's underlying action and holds that action until the hooks are ready. VM Operator does not pause or notify after a stage completes.
- Evaluating more than one stage at a time. Stages are evaluated one at a time, each at its own checkpoint, and a later stage is never evaluated while an earlier one is unresolved (e.g. `ResourceDelete` is not evaluated until `Delete` has resolved). Evaluating stages in parallel is out of scope.
- Holding later `VirtualMachineGroup` boot-order groups until an earlier group's hooks resolve. Boot order is time-based today and does not wait for members to reach their power state.

---

## Key Entities

All four entities are in the `lifecycle.vcfa.vmware.com` API group and are owned by the Lifecycle Operator. VM Operator authors the `LifecycleStages` instance and creates `LifecycleState` when absent.

- **LifecycleHook**: A namespaced registration, by a customer or internal VCF service, to pause a `VirtualMachine` stage. One VM can have many.
- **LifecycleStages**: The cluster-scoped catalog for the `VirtualMachine` CRD, authored by VM Operator and shipped with the Lifecycle Operator's package, declaring the stages it exposes (`Create`, `PowerStateChange`, `Delete`, `ResourceDelete`).
- **LifecycleSubscribedStages**: One per namespace for the `VirtualMachine` CRD, listing the stages that have at least one hook. VM Operator reads it when a VM has no `LifecycleState`, to decide whether to create one.
- **LifecycleState**: One per VM, owned by the VM. VM Operator sets `workflowPaused` when it reaches a hooked stage, and resumes once the Lifecycle Operator sets the stage's `HooksReady=True`.

---

## User scenarios & testing *(mandatory)*

### User Story 1 — VM Create can be paused for external setup (Priority: P1)

A DevOps user creates a `VirtualMachine` in a namespace where a `LifecycleHook` is registered for the `Create` stage (e.g. so an AD pre-registration step can run before the VM exists in vSphere). The VM is not created in vSphere until that hook reports done.

**Why this priority**: This is the earliest and simplest checkpoint — without it, no later stage's design would be validated end to end. It delivers a demonstrable MVP on its own.

**Independent test**: Create a VM in a namespace with a `Create`-stage `LifecycleHook` registered, reconcile it, and verify no vSphere VM is created and the VM's `LifecycleHookReady` condition is `False`/`CreateStage`; flip the hook's readiness and verify the VM is then created and the condition is `True`.

**Acceptance scenarios**:

1. **Given** a `VirtualMachine` with no `LifecycleState` yet and a `LifecycleHook` registered for the `Create` stage on `(vmoperator.vmware.com, VirtualMachine)` in its namespace, **When** VM Operator reconciles the VM for the first time, **Then** VM Operator pauses before creating the VM — no vSphere VM is created, the VM's `LifecycleHookReady` condition is `status=False`, and `reason=CreateStage`.
2. **Given** the `Create` stage is paused and its hooks have all succeeded (`HooksReady=True/AllHooksSucceeded`), **When** VM Operator reconciles the VM, **Then** VM Operator resumes, `LifecycleHookReady` becomes `True`, and the VM is created in vSphere on that same or a subsequent reconcile.
3. **Given** the `Create` stage was released and the vSphere create then fails, **When** VM Operator retries the create, **Then** it pauses again and the hooks re-run, so hooks must be idempotent.
4. **Given** no `LifecycleHook` exists for the `Create` stage in the VM's namespace, **When** VM Operator reconciles a newly created VM, **Then** VM Operator proceeds to create the VM in vSphere without pausing, without creating any lifecycle resource, and without writing `LifecycleHookReady`, identical to today's behavior.

---

### User Story 2 — Power-state changes can be paused for external coordination (Priority: P1)

A DevOps user's VM is about to change power state, and a `LifecycleHook` on `PowerStateChange` needs to run first (e.g. a rebalancing step). `PowerStateChange` is `Reentrant`, and each operation is paused independently.

- **In scope**: every power operation VM Operator applies, hard or soft: power on, power off, suspend (`PoweredOn → Suspended`), resume (`Suspended → PoweredOn`), `Suspended → PoweredOff`, and restart (`spec.nextRestartTime`). This includes the corrective operation after an out-of-band change (guest shutdown, vCenter UI).
- **Out of scope**: the out-of-band change itself, and the power-off before vSphere deletion (gated by `Delete`).

**Why this priority**: Power-state transitions are the most frequent lifecycle event after create/delete, so this is the highest-value checkpoint after US1.

**Independent test**: Transition a VM `PoweredOff → PoweredOn` with a `PowerStateChange` hook registered; verify only the power-on is held (other reconcile work proceeds) and the condition is blocked. Flip readiness, verify the power-on applies. Repeat for the reverse transition and verify it pauses independently of the first.

**Acceptance scenarios**:

1. **Given** a `VirtualMachine` transitioning `PoweredOff` → `PoweredOn` and a blocking `LifecycleHook` on `PowerStateChange`, **When** VM Operator would otherwise apply the power-on to vSphere, **Then** VM Operator pauses that step only — config/device reconciliation unrelated to power state continues to converge — and sets `LifecycleHookReady` to `status=False`, `reason=PowerStateChangeStage`.
2. **Given** the same VM once `HooksReady=True`, **When** VM Operator next reconciles, **Then** the power-on is applied and `LifecycleHookReady` flips to `True` (or stays `False` with a hook-failed message if a hook failed; see US4).
3. **Given** the `PowerStateChange` stage is `Reentrant`, **When** the VM later transitions `PoweredOn` → `PoweredOff` with the same hook still registered, **Then** VM Operator pauses again for the new transition (power-off), independent of and unaffected by the prior power-on pause/resume.
4. **Given** a powered-on VM with the same hook, **When** `spec.powerState` is set to `Suspended` or a due `spec.nextRestartTime` is set, **Then** VM Operator pauses the suspend or restart in the same way.
5. **Given** `spec.powerState: PoweredOn` and the guest shuts itself down, **When** VM Operator would power the VM back on, **Then** VM Operator pauses that power-on in the same way.

---

### User Story 3 — VM deletion can be paused for external cleanup, both in vSphere and in Kubernetes (Priority: P1)

A DevOps user deletes a VM that needs external cleanup (e.g. an external CMDB entry) before the VM is deleted from vSphere (`Delete` stage), and/or one more checkpoint after the vSphere VM is gone but before the Kubernetes object disappears (e.g. to finish writing an audit record keyed by the VM's UID) (`ResourceDelete` stage). `Delete` and `ResourceDelete` remain two distinct, independently-hookable stages — a hook may register on either or both — but `Delete` MUST fully resolve (vSphere VM gone) before `ResourceDelete` is ever evaluated; they are never evaluated in parallel.

**Why this priority**: Deletion is the one path where a missed hook is irreversible — once the vSphere VM or the Kubernetes object is gone, there's no second chance to run cleanup.

**Independent test**: Delete a VM with a `Delete`-stage hook registered; verify the vSphere VM is not removed and the finalizer stays in place until the hook reports done. Then verify `ResourceDelete` is evaluated the same way before the finalizer is actually removed.

**Acceptance scenarios**:

1. **Given** a `VirtualMachine` with `DeletionTimestamp` set and a blocking `LifecycleHook` on `Delete`, **When** VM Operator's delete reconciliation would otherwise call into the provider to delete/unregister the vSphere VM, **Then** VM Operator pauses before that provider call, sets `LifecycleHookReady` to `status=False`, `reason=DeleteStage`, and does not remove the VM Operator finalizer.
2. **Given** `HooksReady=True` for the `Delete` stage, **When** VM Operator next reconciles the deleting VM, **Then** the vSphere VM is deleted, and VM Operator then evaluates the `ResourceDelete` stage the same way — pausing before finalizer removal if a hook is registered and not ready, or removing the finalizer immediately if none is.
3. **Given** `HooksReady=True` for `ResourceDelete` (or no hook registered for it), **When** VM Operator next reconciles, **Then** the finalizer is removed and Kubernetes garbage-collects the object.
4. **Given** `Delete` and `ResourceDelete` are two distinct, independently-hookable stages, **When** a hook is registered on either or both, **Then** `Delete` fully resolves (vSphere VM gone) before `ResourceDelete` is ever evaluated — the two stages are never evaluated in parallel.
5. **Given** a `Delete`-stage hook whose `LifecycleState` carries VM Operator's finalizer (added at creation, or when VM Operator adopts a State the Lifecycle Operator created), **When** the entire namespace is deleted (not just the VM individually), **Then** the `Delete`/`ResourceDelete` hooks are still honored — the namespace remains in `Terminating` until `HooksReady` resolves for each hooked stage, the same as any other stuck finalizer. A Lifecycle-Operator-created `LifecycleState` deleted before VM Operator adopts it is not protected — see the accepted-race note in "Resolved decisions."

---

### User Story 4 — Diagnosing a blocked VM from status alone (Priority: P2)

A DevOps user needs to tell, from the VM object alone, which stage with hooks was last evaluated and whether its hooks are pending, failed, or done, without inspecting the `LifecycleState` or any hook resource directly.

**Why this priority**: Diagnosability. Without it, a DevOps user waiting on a stuck VM has no way to distinguish "waiting on a hook" from any other stalled reconcile, nor to tell which of several possible stages is responsible.

**Independent test**: With a stage currently blocked, run `kubectl get vm <name> -o yaml` and confirm `LifecycleHookReady` is `False` with that stage's reason and a waiting message. Fail one hook and confirm the message says the hook failed. Resolve all hooks successfully and confirm the condition is `True`.

**Acceptance scenarios**:

1. **Given** a stage is blocked with hooks pending (`HooksReady=False/HooksPending`), **When** status is read, **Then** `LifecycleHookReady` is `status=False`, `reason=<Stage>Stage`, message `Waiting for <Stage> hooks: <HooksReady message>`.
2. **Given** a stage is blocked and a hook failed while others still run (`HooksReady=False/HookFailed`), **When** status is read, **Then** `LifecycleHookReady` is `status=False`, `reason=<Stage>Stage`, message `<Stage> hook failed, waiting for remaining hooks: <HooksReady message>`.
3. **Given** a stage's hooks all finished and at least one failed (`HooksReady=True/HookFailed`), **When** VM Operator releases the stage, **Then** the action runs and `LifecycleHookReady` is `status=False`, `reason=<Stage>Stage`, message `<Stage> hook failed, <Stage> proceeded: <HooksReady message>`.
4. **Given** a stage's hooks all succeeded, **When** VM Operator proceeds, **Then** `LifecycleHookReady` is `status=True`, `reason=<Stage>Stage`.
5. **Given** a stage has no hook, **When** VM Operator proceeds, **Then** `LifecycleHookReady` is not written; any condition left by an earlier stage with hooks is unchanged.
6. **Given** no stage with hooks has been reached yet, including any VM in a namespace with no hooks, **When** status is read, **Then** `LifecycleHookReady` is absent, which consumers treat as `True`.
7. **Given** `Delete` and `ResourceDelete` are sequential, and `Create`, `PowerStateChange`, and `Delete` are mutually exclusive by VM phase, **When** status is read, **Then** at most one stage is blocked at a time, so one condition is enough.
8. **Given** a VKS-managed VM backing a workload-cluster `Machine` whose initial or corrective power-on is held by a `PowerStateChange` hook, **When** VKS or CAPV reads the VM's status, **Then** `LifecycleHookReady` is `status=False`, `reason=PowerStateChangeStage`, with a waiting message, so the hold is distinguishable from a generic power-on failure. VM Operator does not read or change the `Machine`, its `Node` or VKS remediation, and applies the power-on once `HooksReady=True` with no VM spec change.
9. **Given** VM Operator cannot evaluate a stage's hooks, for example because reading the VM's `LifecycleState` fails or its binding does not match the VM, **When** status is read, **Then** `LifecycleHookReady` is `status=Unknown`, `reason=<Stage>Stage`, message `Could not evaluate <Stage> hooks: <error>`, the stage's action stays held, and VM Operator retries. Consumers must not treat `Unknown` as `True`.
---


## Resolved decisions

- **Stages**: all four stages are `Reentrant` and `blocking=true`, and `Delete` resolves before `ResourceDelete`. VM Operator never branches on the stage: every reach of a checkpoint after a release is a new hold, including a retry of a failed action, so all hooks must be idempotent.
- **Create trigger**: "VM exists" is decided by the vCenter lookup (`getVM`), not `Status.UniqueID`, so a `Status` reset never re-runs `Create`.
- **Abandon**: a held stage whose action VM Operator will no longer run (power change no longer wanted, VM deleted while Create or a power change is held, Delete skipped because the vSphere VM is gone) is released without waiting for `HooksReady`. Hook owners treat the stage's status entry disappearing as the end of the hold and abort in-flight work.
- **Readiness and conditions**: VM Operator mirrors only `HooksReady` into one condition, `LifecycleHookReady`, with `reason=<Stage>Stage`. `False` does not by itself mean blocked: the message says whether hooks are pending or failed. A failed hook does not block the action: a release on `HooksReady=True/HookFailed` runs the action and leaves the condition `False` with a hook-failed message.
- **`LifecycleState` ownership**: every checkpoint runs the same get-or-create, so it doesn't matter which controller reaches the VM first. If absent, VM Operator creates it from `LifecycleSubscribedStages`; the Lifecycle Operator may create it too, under the same name. A missing entry means "no hook". VM Operator adds its finalizer to any `LifecycleState` without it (adoption).
- **Namespace deletion**: `Delete`/`ResourceDelete` hooks survive namespace teardown only while `LifecycleState` carries VM Operator's finalizer. Three races are accepted: a hook registered the instant a namespace begins terminating, before the Lifecycle Operator reacts; a Lifecycle-Operator-created `LifecycleState` deleted before VM Operator adopts it; and a namespace deleted before either operator creates the `LifecycleState`, where the create is rejected and VM Operator proceeds.
- **Packaging and scope**: gated by `supports_vm_service_lifecycle_hooks`, `vmoperator-stages` ships with the Lifecycle Operator's package, and other controllers were verified unaffected.

---

## Success criteria *(mandatory)*

### Measurable outcomes

- **SC-001**: A `Create`-stage hook registered on a VM's namespace prevents that VM's vSphere creation until `HooksReady=True`, verifiable across create/hook-resolve sequences with no vSphere VM created prematurely.
- **SC-002**: A `PowerStateChange`-stage hook holds only the power-state application, not any other in-flight reconcile step, and re-triggers independently on every subsequent power transition.
- **SC-003**: A `Delete`-stage hook prevents vSphere-side VM deletion, and a `ResourceDelete`-stage hook prevents finalizer removal, until each resolves in sequence — never in parallel.
- **SC-004**: A VM with no `LifecycleHook` registered anywhere in its namespace shows no change in how it is created, powered, or deleted — no `LifecycleState` created, no pause, no extra reconcile latency, and no `LifecycleHookReady` condition written.
- **SC-005**: With the `supports_vm_service_lifecycle_hooks` capability disabled, behavior is identical to the feature not existing — verifiable by the pre-existing test suites passing unchanged.
- **SC-006**: A DevOps user can determine, from the VM's `LifecycleHookReady` condition alone, which stage with hooks was last evaluated (its reason), whether its hooks are done without failure (its status), and whether they are pending or failed (its message).
- **SC-007**: Deleting the namespace containing a VM whose `LifecycleState` carries VM Operator's finalizer (added at creation or on adoption) does not bypass a `Delete`/`ResourceDelete` hook — the namespace stays in `Terminating` until `HooksReady` resolves for the affected stage(s), verifiable by deleting a namespace with such a VM and confirming both the namespace and the VM remain present (with `LifecycleHookReady` `False` and the affected stage's reason) until the hook is resolved. 
- **SC-008**: A `VirtualMachineReplicaSet` whose member has a blocked `Create` stage never creates more VMs than `spec.replicas` — verifiable by registering a blocking `Create` hook, scaling a ReplicaSet up, and confirming the VM count never exceeds `spec.replicas` for as long as the hook remains unresolved.
- **SC-009**: A `VirtualMachineService` never routes traffic to a VM whose power-on is blocked by a `PowerStateChange` hook — verifiable by registering a blocking `PowerStateChange` hook on a `PoweredOff` VM targeted by a service, requesting power-on, and confirming the VM never appears in the service's `Endpoints` `Addresses` while the hook remains unresolved.
- **SC-010**: A `VirtualMachineSnapshot` request against a VM blocked on the `Create` stage fails safely (no vSphere call attempted) rather than creating a snapshot of a VM that doesn't yet exist — verifiable by registering a blocking `Create` hook, requesting a snapshot of the not-yet-created VM, and confirming no snapshot is attempted until the VM exists.

## Open questions

None.

## Review & acceptance checklist

- [x] All user stories have at least two Given/When/Then scenarios.
- [x] Each scenario is independently testable.
- [x] The no-hook-registered case is specified as a no-op/no-regression path.
- [x] Hooks-not-ready handling is specified (mirrors `HooksReady`, prefixes pending vs. failed in the message, no per-hook parsing).
- [x] Stage `type`/`blocking` defaults are specified.
- [x] Condition/reason names and the capability name are specified.
- [x] Out-of-scope items (Lifecycle Operator, eventing system, additional stages, hook-failure-detail parsing) are listed.
- [x] Feature-flag/capability-off behavior is specified (G6, SC-005).
- [x] The reconcile pipeline and the per-checkpoint decision logic (shared `ReconcileStage`) are diagrammed in `plan.md` "Reconcile flow" and decision table.
