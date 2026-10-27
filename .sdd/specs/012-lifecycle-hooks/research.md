# Research: Blocking Lifecycle Hooks

- **Lifecycle framework one-pager** — internal design doc. Business problem, goals, and the initial resource model (`LifecycleStages`, `LifecycleHook`, `LifecycleState`). Explicitly states (in its Non-goals) that the customer-integration mechanism is documented separately.
- **Lifecycle framework API design** — internal design doc. The authoritative, detailed API design: full CRD schemas (OpenAPI) for `LifecycleStages`, `LifecycleState`, `LifecycleHook` (group `lifecycle.vcfa.vmware.com`), plus `Subscription` and `Event` (group `eventing.vcfa.vmware.com`) used by a separate eventing/notification system.

## Discrepancy between the two docs

The one-pager's `LifecycleHook` is per-VM: namespaced, owned by the target `VirtualMachine`, embedding a `spec.stages[]` list with `pauseWorkflow`/`workflowPaused` flags directly on the hook. The API-design doc instead scopes `LifecycleHook` per `(target.group, target.kind, stage)` — one hook registers interest in a single stage for an entire target kind in a namespace, not a single VM — and moves the per-VM pause/resume state into a separate `LifecycleState` resource (owned by the target object, one per VM) whose `status.stages[].hooks[]` lists progress for every hook watching that VM's current stage.

Per the one-pager's own cross-reference, the API-design doc is the authoritative mechanism spec. **This spec follows the API-design doc's schema.** The one-pager's `LifecycleHookTemplate`/per-VM-hook shape is treated as an earlier draft, not implemented.

## Ownership boundary

`LifecycleStages`, `LifecycleHook`, `LifecycleSubscribedStages`, and `LifecycleState` (group `lifecycle.vcfa.vmware.com`) are defined and reconciled by a separate **Lifecycle Operator** — not this repository. The Lifecycle Operator:
- Watches `LifecycleHook` create/update/delete and syncs the corresponding entries into every matching `LifecycleState`.
- Runs the hook timeout timer and flips `LifecycleState` status when a hook's deadline passes.
- Has no connection to the separate eventing system (`Subscription`/`Event`, group `eventing.vcfa.vmware.com`).

**VM Operator's role is consumer-only**: at each defined stage checkpoint, look up (or create) the `LifecycleState` for the `VirtualMachine` being reconciled, read `status.stages[].conditions[type=HooksReady]`, and either proceed or pause. VM Operator does not implement the Lifecycle Operator, the CRDs' controllers, or the eventing system. This mirrors how `external/byok`'s `EncryptionClass` is vendored and consumed today without VM Operator owning the BYOK key-management system behind it.

## Stage scope for this spec

Per product decision, v1 covers four VirtualMachine lifecycle checkpoints, not the one-pager's illustrative `Encrypt`/`PowerOn`/`PreDestroy` set:

1. **VM Create** — before the VM is first created in vSphere.
2. **VM power-state change** — before a power-state transition is applied to vSphere.
3. **VM Delete (vSphere-side)** — before the underlying vSphere VM is deleted/unregistered.
4. **Kubernetes resource deletion** — before the `VirtualMachine` CR's finalizer is removed, i.e. before Kubernetes is allowed to garbage-collect the object.

All four stages are `Reentrant` and `blocking=true`. The last evaluated stage with hooks is reported through one `LifecycleHookReady` condition, with a stage-specific reason — see `model.md` "New `VirtualMachine` API surface" and "Stage `type`/`blocking` defaults".

## What G2's "unrelated reconcile work" means, per stage

`spec.md` G2 requires that pausing a stage not pause unrelated reconcile work. The boundary between "the gated action" and "unrelated work" is not uniform across the four stages — it falls out of where each checkpoint sits in the existing `VirtualMachine` reconcile pipeline, rather than being a free design choice. Traced per stage:

| Stage | Held while hooks are pending | Still converges in the same pass | Externally observable state |
|---|---|---|---|
| **Create** | the entire provider dispatch — placement, configuration, power, status sync | nothing VM-specific; only the finalizer and the blocked condition are persisted | no vSphere VM exists; `status.uniqueID` empty, `Created` condition absent |
| **PowerStateChange** | only the power operation (power on/off, suspend, restart) | every other update step — configuration, devices, networks, volumes, location, status sync | VM exists and is **fully configured**; `Created=True`, `status.powerState` still the pre-transition value |
| **Delete** | the vSphere delete/unregister, and everything downstream of it on the delete path (PVC owner-reference release, the `ResourceDelete` checkpoint) | nothing — delete reconciliation exits at the checkpoint | VM still present in vSphere; object carries a `DeletionTimestamp` with the finalizer held |
| **ResourceDelete** | finalizer removal, plus the metrics and prober cleanup that accompany it | nothing — the vSphere VM is already gone and PVC owner references are already released | object stuck in `Terminating` |

So the set of work that is genuinely "unrelated and still converging" is substantial only for `PowerStateChange` — which is why `spec.md` SC-002 singles that stage out. For the other three it is empty, and not because the gate is coarse: a VM that does not exist yet has no state to reconcile, and a VM on the delete path has no desired state left to converge toward. G2 is therefore a real constraint on exactly one stage and a trivially-satisfied one on the rest.

One caveat found while tracing this: **"proceeds" is not the same as "is correct to proceed."** Snapshot creation runs whenever the power-state step leaves the state unconverged, so a snapshot taken while a `PowerStateChange` hook is pending captures the pre-transition power state. That step is reached because the decision keys off which error flavor the power step returned rather than off power-state convergence — the same gap that a genuine power-on failure already falls through, so it is pre-existing rather than introduced here. The `PowerStateSynced` condition is already set correctly on the blocked path, so the fix belongs in the snapshot step (which carries comparable guards already), not in the stage gate: a gate must not distort its return value to steer an unrelated downstream decision.

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
| **StoragePolicyUsage** | 2 — counting the VM as reserved capacity is the intended semantic | 2 — reports actual `Used`, no distortion | 2 — passively checks `DeletionTimestamp` presence each reconcile; identical to today's behavior if deletion is merely slow for any other reason | 2 — same reasoning as Delete |
| **VirtualMachinePublishRequest** | 1 — `SourceValid=False` plus an error until the VM is created | 2 — no power-state check in this controller | 2 — passively checks deletion state each reconcile; identical to today's behavior if deletion is merely slow for any other reason | 2 — vSphere VM already gone, publish fails same as today |
| **Volume** (legacy) | 1 — no-op on empty `BiosUUID`, attaches once populated | 2 — only the CSI attach mode shifts cold/hot, by design | 2 — `ReconcileDelete` is a deliberate no-op | 2 — no-op |
| **VolumeBatch** | 1 — no-op on empty `InstanceUUID`/`BiosUUID`, attaches once populated | 2 — no power-state coupling at all | 2 — `ReconcileDelete` is a deliberate no-op | 2 — no-op |
| **VirtualMachineClass** | 2 — zero coupling | 2 | 2 | 2 |
| **VirtualMachineImageCache** | 2 — never triggered; no VM watch | 2 | 2 | 2 |

For the "2" rows above that carry a pre-existing, hook-independent correctness gap (e.g. `StoragePolicyUsage`'s quota release, `VirtualMachinePublishRequest`'s missing deletion-timestamp check), G4's `LifecycleHookReady` condition provides additional diagnostic context. Consuming it is each controller's own follow-up work, not required by this spec. Its `False` status alone is not a signal that an action is currently held; see the VKS assessment below.

## VKS compatibility assessment

### Scope and responsibility

This assessment concerns VKS as an existing downstream consumer of `VirtualMachine` resources. It does not propose that VKS register or execute lifecycle hooks. A hook registered by another consumer can delay an operation on a VKS-managed VM, so VKS needs notification of the behavior change and an opportunity to assess its existing workflows.

VM Operator remains responsible only for the `VirtualMachine` lifecycle handshake and status. It does not read or reconcile CAPI `Machine` or workload-cluster `Node` objects, drain nodes, enforce disruption budgets, or change VKS remediation policy. Any downstream compatibility changes belong to the VKS, Cluster API Provider vSphere (CAPV), or Cluster API (CAPI) owners as appropriate.



| Source | Observation |
|---|---|
| VKS default ClusterClass, as inspected | Both control-plane and worker health checks configure `nodeStartupTimeoutSeconds: 3600` and Node `Ready=False` / `Ready=Unknown` thresholds of 600 seconds. These are the inspected release's settings, not universal values for every VKS version or deployed cluster. |
| Cluster API `MachineHealthCheck` `nodeStartupTimeoutSeconds` schema | Startup timeout is measured from the greatest available timestamp among Machine creation, Cluster infrastructure readiness, control-plane initialization, and Machine infrastructure readiness. It is not simply elapsed time since a VM operation was requested. |
| VKS Machine controller, deletion path | An existing CAPI pre-terminate hook cleans up workload-cluster `MachineConfig` objects. It releases the hook even when obtaining the workload-cluster client fails; this is existing cleanup policy, not a new lifecycle-framework integration. |
| VKS in-place update handling | In-place update completion checks `MachineConfig` readiness and observed generation. VM waits need to be assessed alongside this existing progress tracking. |
| VKS cluster-deletion storage cleanup | Cluster deletion already has a CAPI `BeforeClusterDelete` storage-cleanup handler. VM lifecycle holds add another potential wait to the overall deletion workflow. |
| CAPV VM Operator machine service | Provisioning already waits for VM creation, power-on, network addressing, and BIOS UUID. Selected VM prerequisite conditions are propagated; the proposed `LifecycleHookReady` condition is not among them. |
| CAPV `VSphereMachine` controller, deletion path | The infrastructure Machine finalizer is removed only after the VM lookup returns `NotFound`; while deletion is pending, reconciliation requeues after ten seconds. |

**Version qualification:** the inspected CAPV source is not the version VKS pins. The CAPV observations establish an existing pattern, not certification of the shipped dependency combination. VKS should verify these paths in its supported builds. A search of the inspected VKS sources found no existing consumer of `lifecycle.vcfa.vmware.com`.

### Impact by stage

| VM stage held | Expected downstream effect | Compatibility concern |
|---|---|---|
| `Create` | The VM object exists but the vSphere VM does not; provisioning and scale-out remain pending. | A long hold can overlap node-startup failure detection and rollout deadlines. Verify whether remediation creates a replacement subject to the same hold. |
| `PowerStateChange` — initial or corrective power-on | A new Node cannot join, or an existing unhealthy Node cannot recover until power-on proceeds. | Startup or unhealthy-Node timers can expire while VM Operator is intentionally waiting. Corrective power-on after an out-of-band shutdown is the most direct recovery/remediation interaction. |
| `PowerStateChange` — power-off, suspend, or restart | The requested operation has not happened; the VM may still be running and its Node healthy. | Hook blockage alone must not be treated as evidence that the Node is unhealthy. Other VM reconciliation continues while the power operation waits. |
| `Delete` | The VM remains in vSphere with its Kubernetes deletion timestamp set. | Scale-in, rollout, remediation, or cluster deletion can wait for infrastructure deletion. Existing CAPI preparation may already have completed. |
| `ResourceDelete` | The backing VM is gone but the VM object and its finalizer remain. | A consumer waiting for VM `NotFound` still cannot finish deleting the infrastructure Machine. Physical VM removal does not imply downstream deletion has completed. |

The existing wait-and-reconcile patterns suggest limited structural changes for basic compatibility. The larger risk is a downstream controller making a timeout or remediation decision during an intentional hold. Such a decision may be valid under VKS policy; this spec cannot promise that hooks never trigger remediation or that every affected rollout completes within its existing deadline.

### Status propagation and remediation policy

The current contract is one `LifecycleHookReady` condition with reasons `CreateStage`, `PowerStateChangeStage`, `DeleteStage`, and `ResourceDeleteStage`, as defined in [model.md](./model.md#new-virtualmachine-api-surface-this-repo-apiv1alpha6). It reports the last evaluated stage with hooks:

- An absent condition is treated as `True`, including on older Supervisors and VMs that have not reached a hooked stage.
- `False` can mean hooks are pending, a hook failed while other hooks remain pending, or a hook failed and the stage has already proceeded. The message distinguishes these cases.
- A later stage without hooks leaves the earlier condition unchanged. A historical `False` must not be interpreted as a current hold or a failure of the VM operation itself.
- `True` reports successful hook completion or an abandoned hold; it does not certify completion of the underlying VM operation or Node readiness.

CAPV/VKS may need to propagate lifecycle context so users can distinguish an intentional wait from generic provisioning, powering-on, or deleting status. This is a likely localized improvement. Adding `LifecycleHookReady` to an existing list that treats every false prerequisite as an error would not preserve its semantics.

Changing automated remediation is a separate, potentially larger decision. If VKS needs a machine-readable indication of an active hold, the condition's boolean and stage reason alone are insufficient under this contract. Agree on the supported signal with the API owners rather than assuming a historical `False` authorizes suppressing remediation. Any change must preserve detection of unrelated failures; blanket suppression or increasing every timeout could mask them.

VM Operator imposes no independent maximum hold duration. The Lifecycle Operator owns timeout policy; an unavailable Lifecycle Operator can leave a stage waiting. Conversely, `HooksReady=True/HookFailed` releases the VM action despite a failed hook. VKS diagnostics and tests need to distinguish these outcomes. Namespace deletion should be tested against the current finalizer protection and accepted races in [spec.md](./spec.md#resolved-decisions), including the case where State creation is rejected because the namespace is already terminating.

### Downstream validation to agree with VKS

These are proposed joint compatibility checks, not additional Node/Machine responsibilities for VM Operator and not completed acceptance results:

1. **Baseline:** capability disabled, and capability enabled with no applicable hooks; existing VKS provisioning, scaling, upgrade, and deletion behavior is unchanged.
2. **Provisioning:** hold `Create` and initial power-on separately, including beyond the configured startup threshold. Record health-check and remediation behavior, replacement counts, and workflow status; release and verify convergence without manually patching the VM.
3. **Recovery:** hold corrective power-on after an existing VM shuts down, across the configured unhealthy-Node threshold. Verify the agreed remediation outcome and recovery after release. Contrast this with a blocked power-off or restart while the Node remains healthy.
4. **Rollout and deletion:** hold `Delete` and `ResourceDelete` independently during worker scale-in, worker and control-plane replacement, and cluster deletion. Verify finalizer handling, rollout progression after release, and useful reporting of the wait. Include namespace teardown with and without an already-protected `LifecycleState`.
5. **Condition semantics:** cover pending hooks, failure while others are pending, failure followed by release, successful release, abandonment, and an old false condition retained through a later unhooked stage. Ensure historical hook failure does not become a permanent provisioning error or an indefinite remediation exemption.
6. **Recovery and repeated holds:** restart controllers during a hold; retry an underlying VM action; interrupt and restore the Lifecycle Operator; disable the capability during a hold. Verify continued reconciliation and finalizer cleanup under the documented contract. Repeated holds can extend the total workflow duration.

### Complexity and discussion outcome

Assessment: **moderate compatibility and qualification effort, with potentially small code changes**. Basic pending-operation handling appears compatible with the inspected CAPV pattern. Status propagation and support diagnostics are likely low-to-medium complexity. A requirement to distinguish lifecycle waits in health remediation is medium-to-high complexity because it crosses VM status, CAPV propagation, CAPI policy, and supported-version testing. These are qualitative estimates, not a delivery commitment.

The first discussion should confirm the supported VKS/CAPV/CAPI versions, acceptable timeout/remediation outcomes, status-reporting needs, and ownership of the validation above. No new VKS hook consumer is implied. A follow-up compatibility goal, if adopted in `spec.md`, should require validation and tracking of downstream changes; it should not promise that VM Operator controls Machine/Node behavior or prevents all VKS remediation.

## Terminal failures (how a not-yet-ready hook should be treated)

Investigated whether vm-operator has any concept of a permanently-terminal, stop-retrying-forever failure, to decide how VM Operator should react when a stage's hooks aren't ready. Finding: **it does not** — every failure path resolves to one of three level-triggered behaviors, all funneled through `pkgerr.ResultFromError`:

1. **Plain error** (e.g. a vSphere create failure, missing `VirtualMachineClass`, encryption class/key not found) — the relevant condition is set `False` via `conditions.MarkError`/`MarkFalse`, and a plain `error` is returned, which triggers controller-runtime's normal exponential-backoff requeue. These paths are typically paired with a `Watches()` on the missing/invalid referenced object (e.g. `VirtualMachineClass`, `byokv1.EncryptionClass`), so fixing the referenced object re-triggers reconciliation immediately rather than waiting out the backoff.
2. **`pkgerr.NoRequeueError`** — becomes `reconcile.TerminalError`, which stops *backoff* retries for that specific attempt but explicitly still reconciles again on the next watch-triggered event (e.g. a vCenter connection-state change). "Pause polling," not "give up forever."
3. **`pkgerr.NoRequeueNoErr`** — not a failure; "this reconcile did its job for now" (e.g. VM just created, task in flight).

**Conclusion applied to this spec**: the Lifecycle Operator owns hook timeout/failure/retry semantics and writes `LifecycleState.status.stages[].conditions[HooksReady]`. VM Operator uses its status to decide whether to proceed, and its reason and message to report pending or failed hooks through `LifecycleHookReady` (see `model.md` "Hooks-not-ready handling"). A non-`True` `HooksReady` is modeled as case 3 above, with no error backoff: Create/Delete return `lifecycle.ErrHooksBlocked` (`NoRequeueNoErr`); PowerStateChange skips only the power operation; ResourceDelete retains the finalizer. `HooksReady=True/HookFailed` releases the action but leaves `LifecycleHookReady=False` with a failure message. VM Operator watches `LifecycleState` (see `plan.md` §4), so status changes re-trigger reconciliation without a VM spec change.

## Zero-hook cost — the no-`LifecycleHook` path is not free under any documented handshake

Resolved: VM Operator satisfies "no hook anywhere → zero measurable behavior change" (`spec.md` G5 / US1 scenario 4) with Candidate 2 below.

Per "Target operator communication to Lifecycle framework" (WIKI page 2826812771), the framework's own design doc lays out four candidate handshakes for how `LifecycleState.spec.stages[]` gets populated — crossing "who populates it" (the Lifecycle framework vs. the target operator) against "what scope" (every declared blocking stage vs. only stages that currently have a subscriber). None of the four is free for a VM with zero `LifecycleHook`s registered anywhere:

- **Populate all declared blocking stages** (either populator, done synchronously — e.g. via a mutating admission webhook — so there's no creation-time wait): an entry always exists for every stage regardless of subscribers, so reaching any stage still costs "pause → framework resolves zero subscribers → `HooksReady=True`/`NoHooksRegistered` → operator releases" — the doc's own numbers are **4 writes, 2 round trips**, on *every* stage reach, forever, even with zero hooks anywhere.
- **Populate only subscribed stages** (either populator, filled in asynchronously by a separate controller after the object is created with `spec.stages: []`): reaching an unsubscribed stage is free once populated (a missing entry means proceed, zero writes) — but the operator must first wait for a `StagesConverged=True` signal before it can trust that "no entry" actually means "no hook" rather than "the async fill hasn't happened yet" (this is also the source of the creation-race the doc calls out explicitly for the operator-populates variant). That wait happens once per object at creation time, even when zero hooks will ever be registered.

In other words: the "all stages" family taxes the **steady-state per-stage-reach path**; the "subscribed-only" family taxes the **one-time creation path**. Every combination on that page costs *something* for the common case (no hook registered), which conflicts with G5 ("no measurable behavior change from today" when no `LifecycleState`/hook exists).

So VM Operator needs a **pre-check that never enters any of the framework's four handshakes at all** for the zero-hook case, evaluated *before* ever creating a `LifecycleState` (finding none is a cached `Get`, not a handshake), since G5 is unsatisfiable by any of the four framework-side contracts alone. Two candidate pre-check designs, and how they compare:

### Candidate 1 — VM Operator maintains its own indexed cache of `LifecycleHook`

VM Operator watches `LifecycleHook` directly and keeps a local indexed lister keyed by `(namespace, group, kind, stage)`, computing the union itself before ever creating a `LifecycleState`. This is genuinely free in the steady state — a local cache read, no API round trip — for both the one-time creation cost and the steady-state per-stage-reach cost, so it satisfies G5 on both axes at once, unlike either family in the four-option comparison above.

Race/consistency-wise, this carries no additional risk versus any other watched dependency already in this codebase (`VirtualMachineClass`, `byokv1.EncryptionClass`): `controller-runtime`'s manager blocks `Reconcile` until `WaitForCacheSync` completes, and ordinary watch propagation (sub-second) keeps the cache current thereafter. A `LifecycleHook` created at the *literal same instant* as the VM it targets is an acceptable, inherent race for any level-triggered controller — the same watch that eventually observes the hook re-triggers reconciliation, and no design (including all four framework-documented options) can or should promise anything stronger for genuinely concurrent writes. This applies identically regardless of whether the cache being read is local to VM Operator or a framework-maintained aggregate (Candidate 2 below) — neither is "safer" than the other on this axis.

The actual cost of this candidate is architectural, not correctness-related: it reverses the decision that VM Operator never reads `LifecycleHook` (`model.md` "`LifecycleHook`"), and it means VM Operator re-implements the framework's target-matching logic locally. If that matching logic (currently a simple `(group, kind, stage)` match) ever grows richer on the framework side, VM Operator's parallel implementation can silently drift from the authoritative one.

### Candidate 2 — Framework maintains a new `LifecycleSubscribedStages` CRD; VM Operator copies the stage union from it

Instead of VM Operator computing the union itself, the Lifecycle Operator maintains and reconciles a new resource — scoped to `(namespace, group, kind)` — that precomputes "which stages currently have at least one subscriber" for that target kind. VM Operator reads it as a pure consumer through a cached `Get` by its deterministic name, without watching it.

This revives a concept the one-pager originally proposed and the API-design doc superseded — see "Discrepancy between the two docs" above, which notes the one-pager's initial resource model included an "Aggregated LifecycleHook"/"Aggregated LifecycleState" concept dropped in favor of the current per-instance `LifecycleState` schema. Reviving it here is deliberate, to close the G5 gap the current schema reopens; the Lifecycle framework has since adopted it.

On race/consistency grounds, Candidate 2 is **no safer or less safe than Candidate 1** — both are reads through a normally-synced `controller-runtime` cache, subject to the same two acceptable properties described above (concurrent-creation races are inherent and self-correcting via watch; already-committed state is reliably visible once synced). The two candidates differ only on ownership: Candidate 2 keeps the matching/aggregation logic where `research.md`'s existing architecture says it belongs — owned by the Lifecycle Operator — so VM Operator never re-implements or risks drifting from it. That is the deciding factor between the two, not correctness or latency.

**Decision**: Candidate 2, on ownership grounds alone. It requires one more vendored CRD; the Lifecycle framework's contract now includes it.

## Prior art in this repo

- `external/byok` (`EncryptionClass`, group `encryption.vmware.com`) is the closest existing pattern for vendoring a CRD this repo doesn't own: own Go module, `AddToScheme` wired into `pkg/manager/manager.go`, generated manifest under `config/crd/external-crds/`, gated by `pkgcfg.Features.BringYourOwnEncryptionKey`.
- `pkg/vmconfig` (`Reconciler` interface, `crypto.New()` for BYOK) is the existing "pluggable reconcile step" abstraction; a lifecycle-hook stage gate is a natural fit for the same shape, though it must be usable from `controllers/virtualmachine` (the ResourceDelete checkpoint) as well as from `pkg/providers/vsphere` (the Create, PowerStateChange and Delete checkpoints), which is wider than `vmconfig`'s current call sites.
- `pkg/util/paused` (`ByDevOps`/`ByAdmin`) is an existing, unrelated "pause reconciliation" mechanism (annotation-driven, not condition-driven) — a different mechanism with the same spirit; do not confuse the two.

## Links

- Upstream SDD methodology: <https://github.com/github/spec-kit/blob/main/spec-driven.md>
- This repo's SDD standards: [`sdd-standards.md`](../../memory/sdd-standards.md)
- Architectural standards (external CRD vendoring pattern): [`architectural-standards.md`](../../memory/architectural-standards.md)
- Operator best practices (requeue/error semantics, VC op IDs): [`operator-best-practices.md`](../../memory/operator-best-practices.md)
