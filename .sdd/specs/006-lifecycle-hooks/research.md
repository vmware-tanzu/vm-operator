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

## Terminal failures (how a not-yet-ready hook should be treated)

Investigated whether vm-operator has any concept of a permanently-terminal, stop-retrying-forever failure, to decide how VM Operator should react when a stage's hooks aren't ready. Finding: **it does not** — every failure path resolves to one of three level-triggered behaviors, all funneled through `pkgerr.ResultFromError`:

1. **Plain error** (e.g. a vSphere create failure, missing `VirtualMachineClass`, encryption class/key not found) — the relevant condition is set `False` via `conditions.MarkError`/`MarkFalse`, and a plain `error` is returned, which triggers controller-runtime's normal exponential-backoff requeue. These paths are typically paired with a `Watches()` on the missing/invalid referenced object (e.g. `VirtualMachineClass`, `byokv1.EncryptionClass`), so fixing the referenced object re-triggers reconciliation immediately rather than waiting out the backoff.
2. **`pkgerr.NoRequeueError`** — becomes `reconcile.TerminalError`, which stops *backoff* retries for that specific attempt but explicitly still reconciles again on the next watch-triggered event (e.g. a vCenter connection-state change). "Pause polling," not "give up forever."
3. **`pkgerr.NoRequeueNoErr`** — not a failure; "this reconcile did its job for now" (e.g. VM just created, task in flight).

**Conclusion applied to this spec**: the Lifecycle Operator owns all timeout/failure/retry semantics for a hook — it is the sole writer of `LifecycleState.status.stages[].conditions[HooksReady]` and its `reason` (`HooksPending`, or any failure/timeout reason it chooses). VM Operator does not parse that `reason` at all (see `model.md` "HooksReady handling") — a non-`True` `HooksReady` is modeled the same way as case 1 above: a plain condition update on the VM (`False`/`HooksPending`), no `NoRequeueError`. VM Operator already plans to `Watches()` `LifecycleState` (see `plan.md`), so any change to `HooksReady` — including the Lifecycle Operator's own retry or timeout resolution — re-triggers reconciliation immediately, exactly like the `EncryptionClass`-missing case does today.

## Prior art in this repo

- `external/byok` (`EncryptionClass`, group `encryption.vmware.com`) is the closest existing pattern for vendoring a CRD this repo doesn't own: own Go module, `AddToScheme` wired into `pkg/manager/manager.go`, generated manifest under `config/crd/external-crds/`, gated by `pkgcfg.Features.BringYourOwnEncryptionKey`.
- `pkg/vmconfig` (`Reconciler` interface, `crypto.New()` for BYOK) is the existing "pluggable reconcile step" abstraction; a lifecycle-hook stage gate is a natural fit for the same shape, though it must be usable from `controllers/virtualmachine` (Create/K8s-delete checkpoints) as well as from `pkg/providers/vsphere` (vSphere Create/PowerState/Delete checkpoints), which is wider than `vmconfig`'s current call sites.
- `pkg/util/paused` (`ByDevOps`/`ByAdmin`) is an existing, unrelated "pause reconciliation" mechanism (annotation-driven, not condition-driven) — different mechanism, same spirit; cross-reference in `plan.md` to avoid confusing the two.

## Links

- Upstream SDD methodology: <https://github.com/github/spec-kit/blob/main/spec-driven.md>
- This repo's SDD standards: [`sdd-standards.md`](../../memory/sdd-standards.md)
- Architectural standards (external CRD vendoring pattern): [`architectural-standards.md`](../../memory/architectural-standards.md)
- Operator best practices (requeue/error semantics, VC op IDs): [`operator-best-practices.md`](../../memory/operator-best-practices.md)
