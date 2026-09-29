# Implementation Plan: Blocking Lifecycle Hooks

- **Spec**: [`spec.md`](./spec.md)
- **Model**: [`model.md`](./model.md)
- **Research**: [`research.md`](./research.md)
- **Epic**: vmop-3377
- **Date**: 2026-08-24
- **Status**: Draft

## Summary

Add a consumer-side integration with the externally-owned `lifecycle.vcfa.vmware.com` CRDs so VM Operator can pause and resume four `VirtualMachine` reconcile checkpoints — Create, PowerStateChange, Delete, ResourceDelete — based on a per-VM `LifecycleState` resource, without owning or reconciling any of the Lifecycle Operator's CRDs itself. A single shared routine, `pkg/lifecycle.ReconcileStage`, implements the pause/resume decision table once (see "Shared helper" below) — including creating the `LifecycleState` on first read, since that creation is itself part of the same decision the rest of the routine makes — and is called identically from all four checkpoints — two in the VM controller (`ReconcileNormal`'s Create gate, `ReconcileDelete`'s ResourceDelete gate) and two in the vSphere provider (`DeleteVirtualMachine`'s Delete gate, `session_vm_update.go`'s PowerStateChange gate). `LifecycleState` is owned **1:1** by the `VirtualMachine` it tracks — every fan-out and query in this plan is shaped by that fact: no field index, no custom mapper, just the controller-runtime built-in `handler.EnqueueRequestForOwner`.

`ReconcileStage` answers "does this stage have a hook" with a single cached read of `AggregatedLifecycleHooks` (one per namespace, Lifecycle-Operator-owned, cached by the controller-runtime client but never watched/fanned-out on), consulted only once per VM — the moment `LifecycleState` exists (for any stage, any reason), it is never consulted again for that VM. When VM Operator does create `LifecycleState` for the first time, it copies in *every* stage `AggregatedLifecycleHooks` currently lists for `VirtualMachine` — not just the one stage whose checkpoint triggered the creation. See "Zero-hook pre-check" and "`LifecycleState` creation and day-2 hook additions."

`Delete`/`ResourceDelete` hooks registered *after* a VM's initial reconcile (day 2) are the Lifecycle Operator's responsibility to propagate, not VM Operator's: if the VM already has a `LifecycleState` (for any reason), the Lifecycle Operator patches the new stage directly into it; if it doesn't yet have one, the Lifecycle Operator creates it. This — combined with the full-stage-copy above — is what makes a dedicated VM-Operator-side proactive mechanism unnecessary: `LifecycleState` ends up existing well before any deletion is ever in play, driven by whichever side first learns a hook exists, not by VM Operator's own reconcile cadence.

VM Operator's watch on `LifecycleState` reacts only to `status` changes — not `Create` events (frequently its own) or spec-only updates — since those are the only events carrying information VM Operator doesn't already know. Writes to `spec.stages[].workflowPaused`/`workflowResumed` use optimistic locking, because `LifecycleState` now has two writers on that object (VM Operator and, for day-2 additions, the Lifecycle Operator), not one.

## Technical context

- **Go version**: repo default (see root `go.mod`).
- **API version(s) touched**: `api/v1alpha6` (additive conditions only — no field removal, no version bump; `v1alpha6` is `main`'s current storage version per `model.md`).
- **Modules touched**: root module (`controllers/`, `pkg/`, `api/`, `config/`) plus a new `external/lifecycle` sub-module.
- **New dependencies**: none beyond the new `external/lifecycle` module (own `go.mod`, no third-party deps).
- **Feature flag**: `pkgcfg.FromContext(ctx).Features.LifecycleHooks`, gated behind the Supervisor capability `supports_vm_service_lifecycle_hooks` (spec G6). No independently-toggleable env-var default — the capability is the sole gate (spec "Resolved decisions").
- **Depends on**: no other feature flag. The fan-out is a `Watches(&lifecyclev1.LifecycleState{}, handler.EnqueueRequestForOwner(...))`, backed by the informer cache, not a `cource` channel — it does **not** depend on `AsyncSignalEnabled`
- **Interaction with pre-existing flags**: independent of `BringYourOwnEncryptionKey`, `TelcoVMServiceAPI`, and `FastDeploy` — none of `ReconcileStage`'s four call sites touch the `vmconfig.Reconciler` registry those flags gate. One interaction is load-bearing rather than incidental: the Create-stage gate in `ReconcileNormal` must run identically whichever create path is taken — `r.VMProvider.CreateOrUpdateVirtualMachine` or its `Async` sibling, chosen by `AsyncSignalEnabled && AsyncCreateEnabled` — because spec G1 makes no create-path distinction; the gate is placed before that branch, not duplicated into both arms.

## Constitution check

| Rule | Status | Notes |
|---|---|---|
| API compatibility (additive only) | OK | New condition types only; no field removal/rename. |
| Controllers are thin | OK | Stage-gate logic lives in a new `pkg/lifecycle` package, called from controllers/provider; controllers only orchestrate. |
| No controller calls vSphere directly | OK | Stage gate reads/writes `LifecycleState` via the k8s client, not vSphere; vSphere-side checkpoints (Create, PowerStateChange, Delete) are still invoked only from `pkg/providers/vsphere`. |
| Controllers for non-`vmoperator.vmware.com` groups don't live in `controllers/` | OK | This feature adds **no new controller** — it adds watches/logic to the existing `controllers/virtualmachine/virtualmachine` controller, which already reconciles `vmoperator.vmware.com`. `LifecycleHook`/`LifecycleState` are read/patched, never reconciled by a VM-Operator-owned controller loop. |
| External vendored APIs live under `external/` | OK | New `external/lifecycle` module, mirroring `external/byok`. |
| Mapper functions use a field indexer, not an unfiltered `List` | OK, and simpler than the rule anticipates | `LifecycleState` is owned 1:1 by its `VirtualMachine`, so the fan-out uses `handler.EnqueueRequestForOwner` — which resolves the owning VM straight from the `LifecycleState` object's own `ownerReferences`, no `List` and no field index at all. See "Fan-out" below for why this is a strictly cheaper case than the indexed-mapper rule the constitution is guarding against. |
| `+kubebuilder:rbac` markers document permissions | OK | New markers for `lifecycle.vcfa.vmware.com` `lifecyclestates`/`lifecyclestates/status` (get/list/watch/create/patch) and `aggregatedlifecyclehooks` (get/list/watch, cache-only, no fan-out) — `LifecycleHook` and `LifecycleStages` are never read directly by VM Operator (see `model.md`), so no RBAC needed for either. |
| E2E ships with cluster-observable behavior | OK (mandatory) | All four stages are cluster-observable (pausing reconciliation, new conditions) — E2E required per `e2e-sync-with-changes.md`, tracked in `tasks.md`. |
| Feature flag default / rollout documented | OK | Gated by a Supervisor capability, not a bare always-on flag — see "Rollout / migration" below. |

No complexity-tracking entries for constitutional rules — no rule is being bent. "Complexity tracking" below records design points that deviate from a repository *default* rather than a *rule*.

## Project structure

### New files

```
external/lifecycle/                              # NEW module — vendored client types only
  go.mod
  api/v1alpha1/
    doc.go
    groupversion_info.go
    lifecyclestate_types.go                       # LifecycleState, LifecycleStateList (Get/Create/Patch target)
    lifecyclehook_types.go                         # LifecycleHook, LifecycleHookList — types only; VM Operator
                                                    #   never Gets/Lists this kind (model.md), vendored for
                                                    #   completeness/documentation of the schema only
    aggregatedlifecyclehooks_types.go              # AggregatedLifecycleHooks, AggregatedLifecycleHooksList —
                                                    #   Get-only, the zero-hook pre-check's data source (model.md)
    zz_generated.deepcopy.go

pkg/lifecycle/                                     # NEW — stage-gate helper, reusable from controller + provider
  stage.go                                          # ReconcileStage(ctx, k8sClient, obj, stageName) (Result, error),
                                                     #   ReleaseLifecycleState, ensureFinalizer, hookedStagesFor
  stage_test.go

config/crd/external-crds/
  lifecycle.vcfa.vmware.com_lifecyclestates.yaml   # generated via make generate-external-manifests

config/lifecycle/
  vmoperator-stages.yaml                            # static LifecycleStages *instance* (data, not a CRD
                                                     #   definition — model.md "Static LifecycleStages instance")

test/e2e/vmservice/vmservice/virtualmachine/
  vm_lifecycle_hooks.go                             # E2E suite for all four stages
```

### Modified files

```
config/crd/crd.go                                   # + //go:embed external-crds/lifecycle.vcfa.vmware.com_*.yaml
pkg/crd/crd.go                                       # + case "LifecycleState": updateOrDeleteUnstructured(...,
                                                     #   features.LifecycleHooks, ...) — see "Getting the CRD
                                                     #   onto a Supervisor" below
config/default/kustomization.yaml                   # + ../crd/external-crds/lifecycle.vcfa.vmware.com_lifecyclestates.yaml
config/crd/external-crds/README.md                   # + LifecycleState entry under "Production CRDs"
pkg/manager/manager.go                               # + lifecyclev1 import, + lifecyclev1.AddToScheme(opts.Scheme)
pkg/config/config.go                                 # + Features.LifecycleHooks bool
pkg/config/capabilities/capabilities.go             # + CapabilityKeyLifecycleHooks constant, + case in
                                                     #   updateCapabilitiesFeaturesFromCRD
api/v1alpha6/condition_consts.go                    # + VirtualMachineConditionLifecycleHooksBlocked
                                                     #   constant
controllers/virtualmachine/virtualmachine/
  virtualmachine_controller.go                      # + RBAC markers
                                                     # + Watches(&lifecyclev1.LifecycleState{},
                                                     #   handler.EnqueueRequestForOwner(...)) gated by
                                                     #   Features.LifecycleHooks
                                                     # ReconcileNormal: Create-stage gate before first-create path
                                                     # ReconcileDelete: ResourceDelete-stage gate before
                                                     #   finalizer removal

pkg/providers/vsphere/vmprovider_vm.go              # DeleteVirtualMachine: Delete-stage gate before the
                                                     #   vSphere delete/unregister call
pkg/providers/vsphere/session/session_vm_update.go  # reconcilePoweredOffOrPoweredOnVM: PowerStateChange-stage
                                                     #   gate inside the PowerState switch, before the
                                                     #   power-on/off task is issued

test/builder/fake.go                                # + lifecyclev1.LifecycleState{} in KnownObjectTypes
```

`external/lifecycle` is a brand-new module, so the root `Makefile`'s `generate-external-manifests` target needs a new `paths=github.com/vmware-tanzu/vm-operator/external/lifecycle/...` entry added, not just run against an existing path list.

No envtest wiring is needed for the new CRD beyond that: `test/builder/test_suite.go` loads the whole `config/crd/external-crds` directory into envtest, so the generated manifest is picked up automatically once it exists.

## API / CRD strategy

Additive only. No `vmoperator.vmware.com` CRD schema changes beyond a new condition type constant (conditions are not per-version typed fields, so no conversion webhook work is needed). `external/lifecycle` vendors the schema published in the "[API design] Blocking Lifecycle stages" design doc (see `research.md`) — VM Operator does not modify or extend it.

Two generation steps, both outputs checked in:

- `make generate-go` — deepcopy for `LifecycleState`/`LifecycleHook` in the new `external/lifecycle` module.
- `make generate-external-manifests` — `config/crd/external-crds/lifecycle.vcfa.vmware.com_lifecyclestates.yaml`, after the Makefile path-list addition noted above. Note this generates a manifest **only for `LifecycleState`** (the kind VM Operator actually `Get`/`Create`/`Patch`es), not for `LifecycleHook` or `LifecycleStages` — see below for why those two are different.

### Getting the CRD onto a Supervisor

Only one of the four vendored kinds needs VM Operator to manage its own CRD install, and that shapes the two install paths below:

- **`LifecycleState`** is the one kind VM Operator itself creates and patches (model.md "VM Operator's read/write contract"), so VM Operator must ensure its CRD exists on any Supervisor where the feature might run. Both install paths apply here:
  1. **The install kustomization** — `config/default/kustomization.yaml` gains a `../crd/external-crds/lifecycle.vcfa.vmware.com_lifecyclestates.yaml` entry, alongside the repo's other vendored external CRD manifest entries.
  2. **Runtime install by the manager** — `main.go`'s `initCRDs` calls `pkgcrd.Install` (`pkg/crd/crd.go`), which walks every CRD manifest embedded via the `//go:embed` directives in `config/crd/crd.go` and, for each one, creates or deletes it through `updateOrDeleteUnstructured(ctx, k8sClient, enabled, c, k, mutateFn)` — `enabled` decides whether that kind's CRD should exist right now. A kind with no explicit `case` in `pkgcrd.Install`'s `switch` falls through to `default: enabled = true` and is installed unconditionally. `LifecycleState` needs its own `case` so `enabled` tracks the feature flag instead of defaulting to always-on:
     ```go
     case "LifecycleState":
         if err := updateOrDeleteUnstructured(
             ctx,
             k8sClient,
             features.LifecycleHooks,
             c,
             k,
             nil); err != nil {

             return err
         }
     ```
     Without this `case`, `LifecycleState` would fall through to the unconditional-install default, leaving its CRD present on a Supervisor with the capability off — a small but real deviation from spec G6's "no stage ever pauses... and no `LifecycleState` is ever created" on a disabled Supervisor. The `case` closes that gap for the *install*; `ReconcileStage`'s own flag check closes it for *usage*.
  3. `config/crd/crd.go` additionally needs a `//go:embed external-crds/lifecycle.vcfa.vmware.com_*.yaml` line alongside its existing `//go:embed` directives, or the manifest is never loaded into `pkgcrd.External` in the first place.
- **`LifecycleStages`, `LifecycleHook`, and `AggregatedLifecycleHooks`** are **not** installed by VM Operator at all. Per `model.md` "Static `LifecycleStages` instance," their CRDs are installed by the Lifecycle Operator's own chart; VM Operator only ever writes one `LifecycleStages` *instance* (`vmoperator-stages`, `config/lifecycle/vmoperator-stages.yaml`) into a CRD it assumes is already present, never touches `LifecycleHook` at all, and only ever `Get`s `AggregatedLifecycleHooks` (one per namespace) against a CRD it likewise assumes the Lifecycle Operator has already installed. This is why the project structure above generates a manifest only for `LifecycleState` — generating one for the other three kinds would falsely imply VM Operator owns their installation.

`CRDCleanupEnabled` defaults to `false` (`pkg/config/default.go`), so turning `LifecycleHooks` off leaves an already-created `LifecycleState` CRD (and any existing `LifecycleState` instances) in place rather than deleting them. This is a low-risk default here: with the flag off, `ReconcileStage` is never called at all (every call site is wrapped in the flag check), so a lingering `LifecycleState` CRD is simply unused, not a source of drift.

`test/builder/fake.go`'s `KnownObjectTypes` must gain `&lifecyclev1.LifecycleState{}` so the fake client enforces the status subresource split in unit tests (per `operator-best-practices.md`).

## Controller / webhook impact

### 1. Shared helper — `pkg/lifecycle.ReconcileStage`

The one routine every checkpoint calls (model.md "VM Operator's read/write contract per stage checkpoint"). It lives in its own `pkg/lifecycle` package rather than beside either caller, because this routine is called from **both** `controllers/virtualmachine/virtualmachine` and `pkg/providers/vsphere`, and those two packages do not import each other. A new leaf package with no dependency on either caller is the only placement that avoids a cycle.

Its two parameters worth calling out explicitly:

- **`k8sClient ctrlclient.Client`** — the controller-runtime client used to `Get`/`Create`/`Patch` the `LifecycleState`. `ReconcileStage` takes it as a parameter rather than holding one, because its two call sites already carry their own with different lifetimes: the controller passes `r.Client`, the provider passes `vs.k8sClient`. `ReconcileStage` is a plain function, not a controller or a struct with a client field, so it has no independent way to obtain one.
- **`obj *vmopv1.VirtualMachine`** — the VM being reconciled. `ReconcileStage` keys the `LifecycleState` off `obj.Namespace`/`obj.Name`/`obj.UID` and calls `conditions.MarkTrue`/`MarkFalse` directly on `obj.Status.Conditions`, exactly like `pkgcond.MarkError(ctx.VM, ...)` at `vmprovider_vm.go:849` does today. It mutates the caller's in-memory object; it does **not** patch `obj` back to the API server — the caller's own patch helper (`patch.NewHelper`'s deferred patch in the controller, or the provider's own status patch) persists that.

`ReconcileStage` no longer takes a `conditionType` parameter — the feature now has a single condition (`VirtualMachineConditionLifecycleHooksBlocked`), so every call site marks the same one, with the stage name folded into the message rather than into a distinct condition type per stage.

`LifecycleState` existing is *not* the same thing as "this stage has a hook." A different stage's hook may be the reason the object exists at all (e.g. `Create` was hooked, `Delete` was not). So the zero-hook check (`hookedStagesFor`, against `AggregatedLifecycleHooks`) must run any time this stage has no entry of its own yet — whether the whole object is missing or just this one stage — never inferred from "the object happens to exist."

Rough sketch, not applied to the real source file:

```go
func ReconcileStage(
    ctx context.Context,
    k8sClient ctrlclient.Client,
    obj *vmopv1.VirtualMachine,
    stageName string) (Result, error) {

    var ls lifecyclev1.LifecycleState
    err := k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &ls)

    switch {
    case apierrors.IsNotFound(err):
        return reconcileMissingStage(ctx, k8sClient, obj, nil, stageName)
    case err != nil:
        return Result{}, fmt.Errorf("failed to get LifecycleState for %s: %w", obj.Name, err)
    }

    // The object may have been created by either side -- VM Operator's own
    // getOrCreateLifecycleState below, or the Lifecycle Operator (a day-2
    // hook on a VM that had none yet, see "LifecycleState creation and
    // day-2 hook additions"). Only VM Operator can add VM Operator's own
    // finalizer, so do it opportunistically here rather than assuming it's
    // only ever needed at our own creation time.
    if err := ensureFinalizer(ctx, k8sClient, &ls); err != nil {
        return Result{}, fmt.Errorf("failed to ensure finalizer on LifecycleState for %s: %w", obj.Name, err)
    }

    // CORE FIX: don't treat "LifecycleState exists" as "this stage has a
    // hook" -- a different stage's hook may be why the object exists at all.
    if stage := findStage(&ls, stageName); stage == nil {
        return reconcileMissingStage(ctx, k8sClient, obj, &ls, stageName)
    }

    return evaluateStage(ctx, k8sClient, obj, &ls, stageName)
}

// reconcileMissingStage handles a stage with no entry yet, sourcing the
// answer from a fresh AggregatedLifecycleHooks read rather than from
// whatever LifecycleState happens to contain. ls may be nil (object doesn't
// exist at all) or non-nil (object exists, but not for this stage --
// ensureFinalizer has already run on it by the time we get here).
func reconcileMissingStage(
    ctx context.Context,
    k8sClient ctrlclient.Client,
    obj *vmopv1.VirtualMachine,
    ls *lifecyclev1.LifecycleState,
    stageName string) (Result, error) {

    hookedStages, err := hookedStagesFor(ctx, k8sClient, obj)
    if err != nil {
        return Result{}, err
    }
    if !slices.Contains(hookedStages, stageName) {
        // G5: zero-hook no-op for this stage.
        conditions.MarkTrue(obj, vmopv1.VirtualMachineConditionLifecycleHooksBlocked)
        return Result{Proceed: true}, nil
    }

    if ls == nil {
        // First hook ever discovered for this VM: create LifecycleState and
        // seed it with EVERY stage AggregatedLifecycleHooks currently lists
        // -- not just stageName. This is what captures a Delete/
        // ResourceDelete hook that already existed at this moment for
        // free, with no dedicated proactive mechanism (see "LifecycleState
        // creation and day-2 hook additions").
        created, err := getOrCreateLifecycleState(ctx, k8sClient, obj, hookedStages)
        if err != nil {
            return Result{}, err
        }
        ls = created
    }

    if err := patchStageEntry(ctx, k8sClient, ls, stageName, true /* workflowPaused */); err != nil {
        return Result{}, err
    }
    conditions.MarkFalse(obj, vmopv1.VirtualMachineConditionLifecycleHooksBlocked,
        "HooksBlocked", "blocked on %q stage hooks", stageName)
    return Result{Proceed: false}, nil
}

// evaluateStage runs the pause/resume decision for a stage that already has
// an entry -- unchanged in shape from the original design.
func evaluateStage(
    ctx context.Context,
    k8sClient ctrlclient.Client,
    obj *vmopv1.VirtualMachine,
    ls *lifecyclev1.LifecycleState,
    stageName string) (Result, error) {

    stage := findStage(ls, stageName)

    if !stage.WorkflowPaused {
        if err := patchStageEntry(ctx, k8sClient, ls, stageName, true); err != nil {
            return Result{}, err
        }
        conditions.MarkFalse(obj, vmopv1.VirtualMachineConditionLifecycleHooksBlocked,
            "HooksBlocked", "blocked on %q stage hooks", stageName)
        return Result{Proceed: false}, nil
    }
    if !hooksReady(ls, stageName) {
        conditions.MarkFalse(obj, vmopv1.VirtualMachineConditionLifecycleHooksBlocked,
            "HooksBlocked", "blocked on %q stage hooks", stageName)
        return Result{Proceed: false}, nil
    }
    if err := patchWorkflowResumed(ctx, k8sClient, ls, stageName); err != nil {
        return Result{}, err
    }
    conditions.MarkTrue(obj, vmopv1.VirtualMachineConditionLifecycleHooksBlocked)
    return Result{Proceed: true}, nil
}
```

Every write to `LifecycleState.spec.stages[]` (`patchStageEntry`/`patchWorkflowResumed` above) uses `client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})` — the Lifecycle Operator also writes into `spec.stages[]` directly, for day-2 stage additions to a `LifecycleState` it didn't create in the same call VM Operator is patching in (see "LifecycleState creation and day-2 hook additions"). This is precisely the *shared list, multiple writers* scenario `operator-best-practices.md`'s "Fan-out to Child Objects" rule requires an optimistic lock for — without one, a merge patch computed from a stale local read can silently drop whichever side wrote most recently, since a JSON merge patch on a plain (non-map-keyed) list field replaces the whole array rather than merging per-element. The pattern throughout `pkg/lifecycle` is: read, mutate the in-memory copy, patch with that read's `resourceVersion` as a precondition; a `409 Conflict` just propagates as an error, and the caller's normal reconcile retry re-reads fresh state — no dedicated conflict-handling logic needed beyond that. 

### 1a. `LifecycleState` creation and day-2 hook additions

Two mechanisms, one on each side, together ensure `LifecycleState` exists before any deletion is ever in play — without a dedicated VM-Operator-side proactive check.

**Full-stage snapshot at creation.** When `reconcileMissingStage` decides to create `LifecycleState` because `AggregatedLifecycleHooks` shows a hook for the stage currently being checked, it seeds the new object with *every* stage `AggregatedLifecycleHooks` currently lists for `VirtualMachine` in that namespace — not just the one that triggered creation. A VM whose first-ever hooked checkpoint happens to be `Create`, in a namespace that already has a `Delete` hook registered too, gets both stages seeded into `LifecycleState` in that same call. `Delete`'s entry exists long before the VM is ever deleted, with no separate mechanism required.

**Day-2 propagation is the Lifecycle Operator's responsibility, not VM Operator's.** When a new `LifecycleHook` is registered against a VM's kind *after* that VM's initial reconcile:
- If the VM already has a `LifecycleState` (for any reason), the Lifecycle Operator patches the new stage directly into it.
- If it doesn't yet have one — a VM that had zero hooks at its own initial reconcile — the Lifecycle Operator creates the `LifecycleState`.

This propagation runs on the Lifecycle Operator's own reconcile cadence, reacting to the new `LifecycleHook` directly — not on VM Operator's `ReconcileNormal` cadence (bounded by the 30-minute default manager `SyncPeriod` in the worst case, per `pkg/manager/constants.go`). Whichever side learns about a hook first is the side that ensures `LifecycleState` reflects it, and the Lifecycle Operator's own reaction to a new `LifecycleHook` is materially faster than waiting for a given VM to happen to reconcile again — this is what makes a dedicated proactive mechanism on VM Operator's side unnecessary.

**Why `Create`/`PowerStateChange` need none of this**: `Create` being interrupted by a namespace delete is a conflict of intention (the namespace deletion is the user's later, more explicit action), not a gap to protect against — see spec.md US3. `PowerStateChange` self-heals by re-evaluating on every future transition regardless.

**Consequence for `LifecycleState`'s finalizer**: since the object can now be created by either side, VM Operator cannot assume its own finalizer is only ever added at its own creation time. `ReconcileStage` checks for and adds it opportunistically (`ensureFinalizer`, shown above) on any `LifecycleState` it finds already existing, regardless of who created it:

```go
// ensureFinalizer adds VM Operator's finalizer to ls if not already present.
// LifecycleState may have been created by the Lifecycle Operator (a day-2
// hook on a VM with none yet) rather than by getOrCreateLifecycleState
// below, so this cannot be assumed to have happened at creation time.
func ensureFinalizer(ctx context.Context, k8sClient ctrlclient.Client, ls *lifecyclev1.LifecycleState) error {
    if controllerutil.ContainsFinalizer(ls, lifecycleStateFinalizer) {
        return nil
    }
    base := ls.DeepCopy()
    controllerutil.AddFinalizer(ls, lifecycleStateFinalizer)
    return k8sClient.Patch(ctx, ls, ctrlclient.MergeFrom(base))
}
```

**Residual, accepted gap**: a hook registered for `Delete`/`ResourceDelete` against a VM with no existing `LifecycleState`, in the same instant its namespace begins terminating — before the Lifecycle Operator's own reconcile has a chance to react and before `Namespace.status.phase` flips to `Terminating` (at which point the `NamespaceLifecycle` admission controller starts rejecting new `Create` calls into that namespace, `LifecycleState` included). This window is now bounded by however fast the Lifecycle Operator's own controller reacts to a new `LifecycleHook`, not by VM Operator's reconcile cadence — materially narrower than what an `EnsureTerminalStagesDeclared`-style mechanism on VM Operator's side would have offered, but not literally zero. Documented, not solved further — the same category of unwinnable race as a hook registered in the exact instant teardown begins.

### 2. VM path — the four call sites and their ordering

Each of the four checkpoints wraps its `ReconcileStage` call in `pkgcfg.FromContext(ctx).Features.LifecycleHooks`, so a disabled capability makes every one a true no-op — no `Get`, no `Create`, nothing (spec G6). `ReleaseLifecycleState` (below) is gated the same way.

- **Create**, `ReconcileNormal`, immediately before the existing `r.VMProvider.CreateOrUpdateVirtualMachine`/`Async` dispatch (`virtualmachine_controller.go:657-668`), guarded additionally on `ctx.VM.Status.UniqueID == ""` so the gate is only consulted before the *first* create, matching the stage's `Single` type (model.md):
  ```go
  if pkgcfg.FromContext(ctx).Features.LifecycleHooks && ctx.VM.Status.UniqueID == "" {
      if res, err := lifecycle.ReconcileStage(ctx, r.Client, ctx.VM,
          lifecyclestages.Create); err != nil {
          return err
      } else if !res.Proceed {
          return nil // returns nil just similar pause annotation guard
      }
  }
  ```
- **PowerStateChange**, `session_vm_update.go`'s `reconcilePoweredOffOrPoweredOnVM`, inside the existing `switch vmCtx.MoVM.Runtime.PowerState` (lines 300-338), guarding only the branch that would apply a transition — the network/volume/guest-customization reconcile at lines 295-352 sits outside that switch and runs regardless, satisfying spec G2/SC-002's "pause only this step":
  ```go
  if pkgcfg.FromContext(vmCtx).Features.LifecycleHooks {
      if res, err := lifecycle.ReconcileStage(vmCtx, s.K8sClient, vmCtx.VM,
          lifecyclestages.PowerStateChange); err != nil {
          return err
      } else if !res.Proceed {
          break // skip only the power-state apply
      }
  }
  ```
  Because the stage is `Reentrant` (model.md), the same call runs for both the `PoweredOff→PoweredOn` and `PoweredOn→PoweredOff` branches of that switch, and `ReconcileStage`'s own get-or-create logic against `LifecycleState.spec.stages[PowerStateChange]` is what makes each transition pause independently (spec US2 scenario 3) — no extra bookkeeping is needed at the call site to distinguish the two directions.
- **Delete**, `vmprovider_vm.go`'s `DeleteVirtualMachine`, immediately before the existing terminal call to `virtualmachine.DeleteVirtualMachine(vmCtx, vcVM)` (line 460):
  ```go
  if pkgcfg.FromContext(vmCtx).Features.LifecycleHooks {
      if res, err := lifecycle.ReconcileStage(vmCtx, vs.k8sClient, vm,
          lifecyclestages.Delete); err != nil {
          return err
      } else if !res.Proceed {
          return pkgerr.NoRequeueNoErr("waiting for Delete stage hooks")
      }
  }
  ```
  `pkgerr.NoRequeueNoErr` rather than a plain `nil`/error return, because `DeleteVirtualMachine`'s caller — `ReconcileDelete` — treats its own error return as blocking finalizer removal either way, but `research.md`'s "Terminal failures" survey is explicit that a not-yet-ready hook is not a failure worth backoff retry; `NoRequeueNoErr` is the sentinel this codebase already uses for exactly that shape ("this reconcile did its job for now").
- **ResourceDelete**, `ReconcileDelete`, immediately before the existing `controllerutil.RemoveFinalizer(ctx.VM, finalizerName)` (line 598) — placed **after** the `Delete`-stage provider call above has already returned successfully, which is what gives spec US3 scenario 4's "never evaluated in parallel" for free: `ReconcileDelete` is straight-line code, so `ResourceDelete`'s `ReconcileStage` call is simply unreachable until `DeleteVirtualMachine` returns without pausing.
  ```go
  if pkgcfg.FromContext(ctx).Features.LifecycleHooks {
      if res, err := lifecycle.ReconcileStage(ctx, r.Client, ctx.VM,
          lifecyclestages.ResourceDelete); err != nil {
          return err
      } else if !res.Proceed {
          return nil // finalizer stays; watch re-triggers on HooksReady flip
      }
      // ResourceDelete resolved -- release VM Operator's own finalizer on
      // LifecycleState. Whatever DeletionTimestamp is already on that object
      // (set automatically by GC's owner-reference cascade, or by the
      // namespace controller during a namespace delete) lets the API server
      // finish removing it the moment the finalizer list is empty -- VM
      // Operator never calls Delete on it directly, here or anywhere else.
      if err := lifecycle.ReleaseLifecycleState(ctx, r.Client, ctx.VM); err != nil {
          return err
      }
  }

  controllerutil.RemoveFinalizer(ctx.VM, finalizerName)
  controllerutil.RemoveFinalizer(ctx.VM, deprecatedFinalizerName)
  ```

### 3. Zero-hook pre-check (spec G5, `research.md` "Zero-hook cost")

This plan follows `research.md`'s **Candidate 2**: the Lifecycle Operator owns `AggregatedLifecycleHooks`, **one instance per namespace** (not per `(namespace, group, kind)` — a single instance covers every consumer kind registered for hooks in that namespace, disambiguated internally by a `(group, kind)` field inside its `status`). VM Operator reads it as a pure consumer, the same relationship it already has with `LifecycleState` itself, and — this is the key simplification over earlier drafts — **consults it at most once per VM**: the moment `LifecycleState` exists for any reason, `hookedStagesFor` is never called again for that VM.

```go
const aggregatedHooksName = "vmoperator-hooks"

// hookedStagesFor answers, from the informer cache, which stages currently
// have at least one LifecycleHook for (obj.Namespace, vmoperator.vmware.com,
// VirtualMachine). AggregatedLifecycleHooks is one object per namespace and
// is finalizer-protected by the Lifecycle Operator (confirmed directly with
// them), so this Get is always safe -- it never races a namespace-teardown
// sweep the way an unprotected resource would. It is a single cached Get,
// filtered locally to VM Operator's own (group, kind) entry -- never a List
// or Watch against LifecycleHook itself.
func hookedStagesFor(
    ctx context.Context,
    k8sClient ctrlclient.Client,
    obj *vmopv1.VirtualMachine) ([]string, error) {

    var agg lifecyclev1.AggregatedLifecycleHooks
    key := ctrlclient.ObjectKey{Namespace: obj.Namespace, Name: aggregatedHooksName}
    if err := k8sClient.Get(ctx, key, &agg); err != nil {
        if apierrors.IsNotFound(err) {
            return nil, nil
        }
        return nil, fmt.Errorf("failed to get AggregatedLifecycleHooks in %s: %w", obj.Namespace, err)
    }

    for _, target := range agg.Status.Objects {
        if target.Group == vmopv1.GroupVersion.Group && target.Kind == "VirtualMachine" {
            return target.Stages, nil
        }
    }
    return nil, nil
}

// getOrCreateLifecycleState creates a finalizer-protected LifecycleState
// owned by obj, seeded with a declared (workflowPaused=false) entry for
// every stage in hookedStages -- a snapshot of AggregatedLifecycleHooks at
// the moment of creation, not just the one stage that triggered it (see
// "LifecycleState creation and day-2 hook additions") -- or returns the
// existing one on a create/get race.
func getOrCreateLifecycleState(
    ctx context.Context,
    k8sClient ctrlclient.Client,
    obj *vmopv1.VirtualMachine,
    hookedStages []string) (*lifecyclev1.LifecycleState, error) {

    stages := make([]lifecyclev1.Stage, 0, len(hookedStages))
    for _, s := range hookedStages {
        stages = append(stages, lifecyclev1.Stage{Name: s})
    }

    ls := &lifecyclev1.LifecycleState{
        ObjectMeta: metav1.ObjectMeta{
            Name:       obj.Name,
            Namespace:  obj.Namespace,
            Finalizers: []string{lifecycleStateFinalizer},
        },
        Spec: lifecyclev1.LifecycleStateSpec{
            Target: lifecyclev1.TargetReference{
                APIVersion: vmopv1.GroupVersion.String(),
                Kind:       "VirtualMachine",
                Name:       obj.Name,
                Namespace:  obj.Namespace,
                UID:        obj.UID,
            },
            Stages: stages,
        },
    }
    if err := controllerutil.SetControllerReference(obj, ls, k8sClient.Scheme()); err != nil {
        return nil, fmt.Errorf("failed to set owner reference on LifecycleState for %s: %w", obj.Name, err)
    }

    if err := k8sClient.Create(ctx, ls); err != nil {
        if !apierrors.IsAlreadyExists(err) {
            return nil, fmt.Errorf("failed to create LifecycleState for %s: %w", obj.Name, err)
        }
        existing := &lifecyclev1.LifecycleState{}
        if getErr := k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(ls), existing); getErr != nil {
            return nil, fmt.Errorf("failed to get existing LifecycleState for %s: %w", obj.Name, getErr)
        }
        return existing, nil
    }
    return ls, nil
}

// ReleaseLifecycleState removes VM Operator's finalizer once the
// ResourceDelete stage has fully resolved. Called from ReconcileDelete
// immediately before the VM's own finalizers are removed.
func ReleaseLifecycleState(
    ctx context.Context,
    k8sClient ctrlclient.Client,
    obj *vmopv1.VirtualMachine) error {

    var ls lifecyclev1.LifecycleState
    if err := k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &ls); err != nil {
        if apierrors.IsNotFound(err) {
            return nil
        }
        return fmt.Errorf("failed to get LifecycleState for %s: %w", obj.Name, err)
    }
    if !controllerutil.ContainsFinalizer(&ls, lifecycleStateFinalizer) {
        return nil
    }

    base := ls.DeepCopy()
    controllerutil.RemoveFinalizer(&ls, lifecycleStateFinalizer)
    return k8sClient.Patch(ctx, &ls, ctrlclient.MergeFrom(base))
}
```

`lifecycleStateFinalizer` is a single constant (`"lifecycle.vcfa.vmware.com/vm-operator-state"`), added by both `getOrCreateLifecycleState` and the opportunistic `ensureFinalizer`, and removed by `ReleaseLifecycleState` — see "`LifecycleState` creation and day-2 hook additions" above for why it exists and why it can't be assumed to only ever be added at VM Operator's own creation time.

This design point is fully resolved and requires only one thing from the Lifecycle Operator team: that `AggregatedLifecycleHooks` is exactly as described above (one per namespace, `status.objects[].{group,kind,stages[]}`). It does not require them to build anything new beyond what `research.md`'s Candidate 2 already proposed.

### 4. Fan-out — the VM controller's `LifecycleState` watch

In `virtualmachine_controller.go`'s `AddToManager`, gated the same way as every other feature-flagged watch in that function:

```go
if pkgcfg.FromContext(ctx).Features.LifecycleHooks {
    builder = builder.Watches(
        &lifecyclev1.LifecycleState{},
        handler.EnqueueRequestForOwner(
            mgr.GetScheme(),
            mgr.GetRESTMapper(),
            &vmopv1.VirtualMachine{}),
        builder.WithPredicates(statusChangedPredicate{}),
    )
}
```

This is the exact shape of the existing `PolicyEvaluation` watch (`virtualmachine_controller.go:197-205`, gated on `Features.VSpherePolicies`) — both types are owned 1:1 by a `VirtualMachine` via `ownerReferences`, so both use the built-in `handler.EnqueueRequestForOwner` rather than a hand-written mapper. This is a deliberately simple fan-out, and worth spelling out why it's sufficient: a custom mapper plus a dedicated field index is only needed when a child object must reach *multiple* interested parents with no static path back to them — a many-to-many relationship. `LifecycleState` has no such relationship: `EnqueueRequestForOwner` reads the owning VM's identity straight out of the `LifecycleState` object's own `ownerReferences` field and issues a single `Get`, no `List` and no field index at all. `HooksReady` flipping on any stage therefore re-triggers exactly the one VM it belongs to, promptly, with none of the workqueue-deduplication reasoning a many-to-many fan-out would need to justify staying cheap.

`LifecycleState` can be created by either side (VM Operator itself, or the Lifecycle Operator for a day-2 hook), so its `Create` event is frequently VM Operator's own write reflected back at it — reconciling on that is pure waste. Likewise, VM Operator's own `spec.stages[]` patches (`workflowPaused`/`workflowResumed`) would otherwise re-trigger a reconcile of the exact VM that just made that patch, for no new information. The only events actually worth reacting to are `status` changes — `HooksReady` flipping, or a new day-2 entry appearing in `status.stages[]`. A custom predicate filters to exactly that:

```go
type statusChangedPredicate struct{}

func (statusChangedPredicate) Create(event.CreateEvent) bool { return false }
func (statusChangedPredicate) Delete(event.DeleteEvent) bool { return true }
func (statusChangedPredicate) Generic(event.GenericEvent) bool { return false }

func (statusChangedPredicate) Update(e event.UpdateEvent) bool {
    oldLS, ok1 := e.ObjectOld.(*lifecyclev1.LifecycleState)
    newLS, ok2 := e.ObjectNew.(*lifecyclev1.LifecycleState)
    if !ok1 || !ok2 {
        return false
    }
    return !apiequality.Semantic.DeepEqual(oldLS.Status, newLS.Status)
}
```

This does **not** reduce memory usage — the informer backing this watch still fully caches every `LifecycleState` that exists, regardless of the predicate; the predicate only decides whether an already-cached event gets enqueued as a reconcile request. It saves reconciles/API traffic, not cache footprint.

**`AggregatedLifecycleHooks` is never watched, deliberately.** It is one instance per namespace and covers every consumer kind registered for hooks in that namespace — watching it and fanning out to "every VM in the namespace" on each change would be the many-to-many problem this section's `EnqueueRequestForOwner` approach exists to avoid, and would fire on hook activity for kinds that have nothing to do with `VirtualMachine`. Instead, `hookedStagesFor` (see "Zero-hook pre-check" above) reads it via a plain, informer-cache-backed `Get` — the RBAC below grants `list`/`watch` on it purely so that cache stays populated, not to register any event handler.

**The CRD must exist when the manager starts**, since a watch on an unserved kind fails to start. Per "Getting the CRD onto a Supervisor" above, `main.go`'s `initCRDs()` runs `pkgcrd.Install` — which creates the `LifecycleState` CRD whenever `Features.LifecycleHooks` is on — before `controllers.AddToManager` registers this watch, so there is no configuration where the watch starts without its CRD. `AggregatedLifecycleHooks`'s CRD is installed by the Lifecycle Operator's own chart (see "Getting the CRD onto a Supervisor" below), so it must be present before VM Operator's manager starts issuing `Get`s against it — the same install-ordering dependency `LifecycleStages`' static instance write already has.

### 5. Webhook impact

None. Stage gating is a reconcile-time concern; there is no admission-time decision to make (the `LifecycleState` schema itself is validated by the Lifecycle Operator's own webhook, not VM Operator's).

### 6. RBAC

New markers on the `VirtualMachine` controller for `lifecycle.vcfa.vmware.com`:

- `lifecyclestates` (get, list, watch, create, patch) and `lifecyclestates/status` (get, patch) — no `update` (every write is a `Patch`, per `operator-best-practices.md`'s reconcile-loop convention).
- `aggregatedlifecyclehooks` (get, list, watch) — `list`/`watch` are needed even though there is no dedicated `Watches()` fan-out registered against this kind (see "Fan-out" above), because the informer cache backing every `hookedStagesFor` `Get` needs them to stay populated.

No verbs at all for `lifecyclehooks` or `lifecyclestages`, since VM Operator never reads either kind directly (model.md).

## Reconcile flow

```mermaid
flowchart TD
    subgraph Normal["ReconcileNormal"]
        A{Features.LifecycleHooks &&<br/>Status.UniqueID empty?}
        A -- no --> A1[Skip straight to<br/>CreateOrUpdateVirtualMachine#40;Async#41;]
        A -- yes --> B[ReconcileStage#40;Create#41;]
        B --> C{Proceed?}
        C -- no --> C1([Exit — condition=False/HooksBlocked,<br/>no vSphere VM created])
        C -- yes --> A1
        A1 --> D[Config/device/status reconcile<br/>#40;unaffected#41;]
    end

    subgraph PowerSwitch["session_vm_update.go — PowerState switch"]
        D --> E{desired != observed<br/>power state?}
        E -- no --> Done1([Reconcile complete])
        E -- yes --> F{Features.LifecycleHooks?}
        F -- no --> G[Apply power-state change]
        F -- yes --> H[ReconcileStage#40;PowerStateChange#41;<br/>independent per direction — Reentrant]
        H --> I{Proceed?}
        I -- no --> I1([break — power apply skipped only,<br/>condition=False/HooksBlocked])
        I -- yes --> G
        G --> Done1
    end

    subgraph Delete["ReconcileDelete"]
        J{Features.LifecycleHooks?} -- no --> K1[DeleteVirtualMachine call]
        J -- yes --> K[ReconcileStage#40;Delete#41;<br/>in DeleteVirtualMachine]
        K --> L{Proceed?}
        L -- no --> L1([NoRequeueNoErr — finalizer kept,<br/>condition=False/HooksBlocked])
        L -- yes --> K1
        K1 --> M{Features.LifecycleHooks?}
        M -- no --> N1[RemoveFinalizer]
        M -- yes --> N[ReconcileStage#40;ResourceDelete#41;<br/>— only reached once Delete has<br/>fully resolved, never in parallel]
        N --> O{Proceed?}
        O -- no --> O1([Exit nil — finalizer kept,<br/>condition=False/HooksBlocked])
        O -- yes --> N2[ReleaseLifecycleState<br/>removes VM Operator's finalizer]
        N2 --> N1
        N1 --> GC([Kubernetes garbage-collects the VM])
    end

    subgraph ReconcileStageBox["pkg/lifecycle.ReconcileStage — shared by all four call sites"]
        P0{LifecycleState<br/>exists at all?}
        P0 -- yes --> FIN[ensureFinalizer<br/>— may have been created<br/>by either side]
        FIN --> P
        P0 -- no --> P
        P{entry for this<br/>stage exists?<br/>#40;CORE FIX: checked per-stage,<br/>not per-object#41;}
        P -- no --> Q{hookedStagesFor<br/>#40;AggregatedLifecycleHooks#41;<br/>— finalizer-protected, always safe}
        Q -- no --> QA[MarkTrue#40;LifecycleHooksBlocked#41;<br/>Proceed=true — zero LS traffic]
        Q -- yes --> R[Create LifecycleState if absent —<br/>seed ALL currently-hooked stages<br/>at once, not just this one]
        R --> T1
        P -- yes --> S{spec.stages#91;stage#93;<br/>.workflowPaused?}
        S -- false --> T[Patch workflowPaused=true<br/>MarkFalse#40;HooksBlocked#41;<br/>#40;optimistic lock#41;]
        T --> T1[Proceed=false]
        S -- true --> U{status.stages#91;stage#93;<br/>.conditions#91;HooksReady#93;<br/>== True?}
        U -- no --> U1[MarkFalse#40;HooksBlocked#41;<br/>Proceed=false]
        U -- yes --> V[Patch workflowResumed=true<br/>MarkTrue#40;LifecycleHooksBlocked#41;<br/>Proceed=true<br/>#40;optimistic lock#41;]
    end

    B -.-> P0
    H -.-> P0
    K -.-> P0
    N -.-> P0

    subgraph LCOp["Lifecycle Operator #40;external#41;"]
        DAY2{New LifecycleHook<br/>registered day-2?}
        DAY2 -- LS exists --> PATCH[Patch new stage into<br/>existing LifecycleState]
        DAY2 -- LS absent --> CREATE[Create LifecycleState,<br/>patch the new stage]
    end

    PATCH -.->|status.stages#91;#93; entry appears| W1
    CREATE -.->|new object, VM Operator adds<br/>its finalizer on next reconcile| FIN

    subgraph Watch["Fan-out — VM controller's Watches#40;&LifecycleState{}, statusChangedPredicate#41;"]
        W1[status changed:<br/>HooksReady flip, or a new<br/>day-2 status.stages#91;#93; entry] --> W2[handler.EnqueueRequestForOwner<br/>— reads ownerReferences directly,<br/>no List, no index]
        W2 --> W3[owning VM re-reconciles<br/>immediately]
        WX[Create event, or<br/>spec-only update] -.->|filtered out,<br/>no reconcile| W2
    end

    V -.->|writes status.stages HooksReady, observed by| W1
    W3 -.->|re-enters| A
    W3 -.->|re-enters| F
    W3 -.->|re-enters| J
```

There is deliberately no path into `hookedStagesFor`'s `AggregatedLifecycleHooks` `Get` from a `Namespace`-deletion actor in this diagram — that read is always safe, since `AggregatedLifecycleHooks` is itself finalizer-protected by the Lifecycle Operator and cannot be swept away mid-check. What this diagram *cannot* depict is timing: the `LCOp` subgraph's reaction to a new `LifecycleHook` runs on the Lifecycle Operator's own reconcile cadence, independent of anything VM Operator does — see "`LifecycleState` creation and day-2 hook additions" for why that timing, not a VM-Operator-side proactive check, is what closes the namespace-deletion race for `Delete`/`ResourceDelete`.

## Test strategy

Per `testing-standards.md`: one `_test.go` and one `_suite_test.go` per package, external `_test` package, labels on the top-level `Describe`.

### Unit (`testlabels.Controller`)

- `pkg/lifecycle/stage_test.go` — the full `ReconcileStage` decision table against a fake client with `lifecyclev1.AddToScheme` registered:
  - No hook anywhere → `Proceed=true`, no `LifecycleState` `Get`/`Create` beyond the initial lookup (G5).
  - Hook registered, no `LifecycleState` yet → created (with finalizer + owner reference) + `workflowPaused=true` + `Proceed=false`.
  - **Full-stage snapshot at creation**: `AggregatedLifecycleHooks` lists multiple stages (e.g. `Create` and `Delete`) when the *first* one (`Create`) is checked and `LifecycleState` doesn't exist yet → assert the created object's `spec.stages[]` contains entries for **both** stages, `Delete`'s at `workflowPaused=false` (declared, not paused) and `Create`'s at `workflowPaused=true` (the one actually being checked).
  - `LifecycleState` already exists (created for a *different* stage's hook) but has no entry for *this* stage → `hookedStagesFor` is re-consulted fresh for this stage rather than assuming the object's existence means it's hooked; asserted both ways — hooked (entry gets added, pauses) and not hooked (proceeds, no entry ever added for this stage).
  - **`ensureFinalizer` runs opportunistically**: a `LifecycleState` fixture created *without* VM Operator's finalizer (simulating one the Lifecycle Operator created for a day-2 hook) → `ReconcileStage` patches the finalizer in before evaluating the stage, regardless of which branch (missing-stage or already-present) it takes next; a fixture that already has the finalizer → no patch call issued.
  - `workflowPaused=true` + `HooksReady` absent/`False`/any non-`True` reason → `Proceed=false`, condition stays `HooksBlocked` regardless of the underlying `HooksReady` reason (model.md "Hooks-not-ready handling" — VM Operator does not branch on it).
  - `workflowPaused=true` + `HooksReady=True` → `workflowResumed=true` patched, `Proceed=true`, condition flips `True`.
  - **Optimistic-lock conflict**: a patch attempt where the fake client's object was concurrently modified (simulating a Lifecycle Operator day-2 write) between read and patch → the patch fails with a conflict, propagated as a plain error rather than silently retried inside `ReconcileStage` itself (retry is the caller's normal reconcile-requeue behavior, not new logic here).
  - A `LifecycleState` deleted out-of-band while paused → re-created and re-enters the paused state on the next call (spec "Resolved decisions"), not treated as an implicit resume.
- `pkg/lifecycle/stage_test.go` (continued) — `ReleaseLifecycleState`: finalizer present → removed via a bare `MergeFrom` patch; finalizer already absent, or object already gone (`NotFound`) → no-op, no patch call issued.
- `controllers/virtualmachine/virtualmachine/*_test.go` — Create-stage gate: hook absent (no-op, proceeds to create); hook present and blocking (no vSphere create call reaches the fake provider, condition blocked); `Status.UniqueID` already set skips the gate entirely (post-create reconciles never re-consult `Create`). ResourceDelete-stage gate: finalizer retained while paused; `Delete`-stage completion is a precondition the test constructs explicitly (fake provider's delete call already returned) so the "never in parallel" ordering is exercised, not merely assumed; `ReleaseLifecycleState` is asserted to run exactly once, only after `ResourceDelete` resolves with `Proceed=true`, and before `RemoveFinalizer` on the VM.
- `pkg/providers/vsphere/vmprovider_vm.go`/`vmprovider_vm_test.go` — Delete-stage gate: paused → `pkgerr.NoRequeueNoErr` returned, `virtualmachine.DeleteVirtualMachine` (the vCenter call) never invoked; resumed or no hook → vCenter delete proceeds unchanged from today's behavior (spec SC-004 baseline); a fixture where `LifecycleState` already carries an unpaused `Delete` entry (simulating one seeded by the full-stage-snapshot behavior at an earlier `Create` checkpoint, or patched in by the Lifecycle Operator) is asserted to produce the identical pause behavior as the entry being discovered lazily for the first time here.
- `pkg/providers/vsphere/session/session_vm_update_test.go` — PowerStateChange-stage gate: paused power-on does not issue the power task, but volume/network/guest-customization reconcile in the same call still runs (spec SC-002, asserted via the fake's other reconcile side effects still firing); the same for power-off; the two directions pause independently within one test given the `Reentrant` stage type (spec US2 scenario 3).
- Capability wiring — with `Features.LifecycleHooks=false`, every one of the above call sites (including `ReleaseLifecycleState`) makes zero calls into the fake client for `LifecycleState`/`AggregatedLifecycleHooks` — asserted with a call-counting fake, not just "no error," since the no-op requirement (G5/G6) is specifically about absence of API traffic, not just absence of pausing.
- `pkg/lifecycle` watch predicate unit test (no envtest needed — `statusChangedPredicate` is a plain function): `Create` events always filtered out; `Update` events with only `spec` changed filtered out; `Update` events with `status.stages[]`/`conditions` changed pass through; `Delete` events pass through.

### Integration (`testlabels.EnvTest`)

vcsim gives VM Operator a fake vSphere; it does not give VM Operator a fake Lifecycle Operator. The Lifecycle Operator's own business logic (hook fan-out, timeout, matching) is out of scope per spec's non-goals, and a real Lifecycle Operator binary is unnecessary weight for envtest — VM Operator's contract is fully defined by what it reads/writes on `LifecycleState`/`AggregatedLifecycleHooks`, not by how the Lifecycle Operator arrives at those values. Two tiers, in increasing realism:

1. **Direct test-code manipulation (primary, for most scenarios)** — envtest + real API server, `LifecycleState`/`AggregatedLifecycleHooks` objects created/patched directly by test code standing in for the Lifecycle Operator (already the existing plan's approach for the watch-wiring test below). Sufficient for anything that only needs "the Lifecycle Operator eventually writes X" — no sequencing between multiple Lifecycle-Operator-side writes is needed.
2. **A minimal fake Lifecycle Operator reconciler (new, for compound/day-2 sequencing scenarios)** — a small test-only controller, `test/builder/fakelifecycle` (mirroring the shape of `test/builder/fake.go`'s `VMProvider` fake), registered only in envtest suites that need it. It watches `LifecycleHook` create/delete and mechanically mirrors the minimum needed for these tests: adding/removing the corresponding entry in `AggregatedLifecycleHooks.status.objects[].stages[]`, and — this now includes the day-2 responsibility described in "`LifecycleState` creation and day-2 hook additions" — patching a matching entry into `status.stages[]` on an existing `LifecycleState`, or **creating** `LifecycleState` (with an owner reference to the target VM) and patching it if none exists yet. It does **not** implement timeout, eventing, or multi-hook aggregation (`status.stages[].hooks[]`) — those stay entirely out of scope, per spec's non-goals; it exists purely to remove the hand-choreographed, multi-step test setup these sequencing scenarios would otherwise require. Flipping `HooksReady` itself stays a manual test-code patch in both tiers — that boundary (readiness computation) is never faked, only existence/propagation is.
- `controllers/virtualmachine/virtualmachine/*_test.go` (envtest, tier 1) — the watch wiring itself: with `Features.LifecycleHooks` on, patching `LifecycleState.status.stages[Create].conditions[HooksReady]=True` on a real object causes the owning VM to reconcile promptly (`Eventually`), exercising `handler.EnqueueRequestForOwner` and `statusChangedPredicate` together through a real manager. Also assert the predicate's actual filtering behavior end-to-end: a spec-only patch to `LifecycleState` (e.g. VM Operator's own `workflowPaused` write) does **not** cause a second, redundant reconcile of the same VM; a `status.stages[]` addition with no `HooksReady` change (a day-2 hook just registered, not yet resolved) **does** trigger one.
- `controllers/virtualmachine/virtualmachine/*_test.go` (envtest, tier 1) — full stage sequencing across a real object lifecycle: create a VM with hooks on all four stages, drive each `HooksReady` flip in order, and assert the VM only ever proceeds past `Delete` after that stage's `HooksReady` flip, never before, and that `ResourceDelete` is not evaluated (no `LifecycleState.spec.stages[ResourceDelete]` entry appears) until `Delete` has resolved. Also assert `ReleaseLifecycleState` actually results in the `LifecycleState` object disappearing from the API server once both the VM's and `LifecycleState`'s finalizers clear.
- `controllers/virtualmachine/virtualmachine/*_test.go` (envtest, tier 1) — full-stage-snapshot behavior against a real object: patch `AggregatedLifecycleHooks` to list both `Create` and `Delete` before a fresh VM's first reconcile; assert the `LifecycleState` created at the `Create` checkpoint already carries a declared (`workflowPaused=false`) `Delete` entry, with no separate write needed later.
- `controllers/virtualmachine/virtualmachine/*_test.go` (envtest, tier 2, using `fakelifecycle`) — the day-2, no-prior-`LifecycleState` case end-to-end: VM created and reconciled with zero hooks anywhere (no `LifecycleState` created); register a *new* `LifecycleHook` targeting `Delete` against the running VM; assert `fakelifecycle` creates `LifecycleState` (with `ensureFinalizer` picking up VM Operator's finalizer on the VM's next reconcile) and patches in the `Delete` entry; then delete the VM and assert the `Delete` stage blocks correctly. This is the scenario that most directly stands in for the residual-gap window discussed in "`LifecycleState` creation and day-2 hook additions."

### E2E (mandatory, `e2e-sync-with-changes.md`)

New suite `test/e2e/vmservice/vmservice/virtualmachine/vm_lifecycle_hooks.go`, registered from `test/e2e/vmservice/vmservice_test.go`. Unlike the unit/integration tiers above, E2E scenarios that exercise namespace deletion or genuine Lifecycle-Operator reconcile timing need the **real** Lifecycle Operator installed in the E2E environment — a hand-crafted `LifecycleState` fixture cannot reproduce actual concurrent-delete timing or actual hook fan-out, and that's precisely what these scenarios are validating.

- **Baseline, all four stages**: Create pause/resume, PowerStateChange pause/resume in both directions, the sequential Delete-then-ResourceDelete flow including the case where only one of the two carries a hook, and a capability-disabled run confirming zero behavior change with hooks still registered (spec SC-005).
- **Compound/day-2 sequencing** : create a VM with a `Create`-stage hook only; once resolved, register a *new* `LifecycleHook` on `Delete` against the running VM; delete the VM and confirm `Delete` blocks correctly — validating the real Lifecycle Operator's own day-2 propagation into an already-existing `LifecycleState`, which no lower test tier can fully validate since tier 2's `fakelifecycle` deliberately doesn't implement the real matching/propagation logic.
- **Namespace deletion** (spec US3 scenario 5, SC-007): a VM with a `Delete`-stage hook registered, in a namespace that is then deleted wholesale (not the VM individually) — assert the namespace stays in `Terminating`, the VM and its `LifecycleState` remain present with the condition `False`/`HooksBlocked`, until the hook resolves; then resolve it and confirm both the VM and the namespace complete deletion. This is the one scenario that **cannot** be meaningfully exercised below E2E — it depends on the real namespace controller's concurrent-delete semantics, which envtest's control plane does technically have, but pairing it with a genuine Lifecycle Operator reacting to the same teardown is what actually exercises the race this feature defends against.
- **Normal single-VM delete, for contrast**: the same `Delete`-stage-hooked VM, deleted individually with the namespace left alone — confirms the ordinary GC-cascade-plus-finalizer path (no namespace teardown involved) behaves identically to today's baseline scenario, establishing that the namespace-deletion scenario above is testing an *additional* case, not a different code path for the common one.
- **Day-2 hook removed before it ever resolves**: register a `Delete`-stage hook, let it block a VM delete, then delete the `LifecycleHook` itself while the VM is still paused — confirms VM Operator's own side (condition, finalizer) reacts sanely to whatever the real Lifecycle Operator does with `HooksReady` in that situation (documented as the Lifecycle Operator's own decision per model.md, not VM Operator's — this scenario is about confirming VM Operator doesn't do anything surprising in response, not about dictating what the Lifecycle Operator should do).

## Rollout / migration

- **Capability gate**: `supports_vm_service_lifecycle_hooks` is the sole gate for `pkgcfg.Features.LifecycleHooks` — no independently-toggleable env-var default, matching spec US4's "entire feature gated by a capability" requirement. `pkg/config/capabilities/capabilities.go` needs a new `CapabilityKeyLifecycleHooks` constant and a `case` in `updateCapabilitiesFeaturesFromCRD`, the same two-step wiring `CapabilityKeyBringYourOwnKeyProvider` already uses there — see "Controller / webhook impact" above for the call sites that consume the resulting flag.
- **No schema upgrade / backfill**: nothing in `api/` changes beyond the four additive condition types, and no existing VM field is backfilled. On a Supervisor where the capability is turned on for the first time, every VM's next reconcile simply starts consulting `ReconcileStage` — the feature is level-triggered, so no migration job or one-time pass is needed.
- **Turning the capability off** makes every `ReconcileStage` call site an immediate no-op on the VM's very next reconcile (each is gated inline, not just at controller-startup like the watch registration is) — a stage paused when the capability was on stays paused only until the next reconcile un-pauses evaluation entirely, per spec's edge case ("Enabling the capability on a Supervisor with no `LifecycleHook`s registered anywhere MUST still be a no-op") applied in reverse. Any already-created `LifecycleState` objects and the CRD itself are left in place (`CRDCleanupEnabled` defaults `false`), but since nothing reads or writes them with the flag off, they are simply inert rather than a source of drift.
- **Partner comms**: announce via the same channel/design-doc-review process as other new-condition, capability-gated features (e.g. `supports_telco_vm_service_api`), once the Lifecycle Operator team confirms the two items noted in "Blocking items before implementation starts."
- **Release notes**: ship with the first PR that turns on any stage gate, referencing the new conditions and the capability name.

## Complexity tracking

| Deviation | Why needed | Simpler alternative rejected because |
|---|---|---|
| `ReconcileStage` lives in a brand-new leaf package (`pkg/lifecycle`) rather than beside either caller's existing helpers | It is called from both `controllers/virtualmachine/virtualmachine` and `pkg/providers/vsphere`, and those two packages do not import each other | Placing it in either caller's package (the repository default for a single-consumer helper) would force the other caller to import a controller package or vice versa, which either doesn't compile or violates "controllers are thin" |
| `hookedStagesFor` depends on a Lifecycle-Operator-owned resource (`AggregatedLifecycleHooks`, `research.md` Candidate 2) rather than a locally-computed cache | Satisfying G5 without it requires either a per-stage-reach round trip forever (unacceptable per `research.md`'s measurement) or VM Operator re-implementing the framework's own hook-matching logic locally (Candidate 1), which risks silent drift from the authoritative matching logic if it ever grows richer | Candidate 1 was rejected on ownership grounds in `research.md`, not correctness — building it locally would duplicate matching logic the Lifecycle Operator already owns and risk drift the moment that logic grows richer (e.g. label selectors) |
| `LifecycleState` carries a VM-Operator-owned finalizer, added opportunistically (`ensureFinalizer`) on any existing object rather than only at VM Operator's own creation time | The object can now be created by either side — VM Operator's own `getOrCreateLifecycleState`, or the Lifecycle Operator for a day-2 hook on a VM that had none yet (see "`LifecycleState` creation and day-2 hook additions"). Only VM Operator can add its own finalizer, so it cannot assume creation-time is the only opportunity | Requiring the Lifecycle Operator to add VM Operator's finalizer on its behalf would couple the two controllers' write paths in a way neither team's RBAC model anticipates; checking and adding it opportunistically on every read is a small, local cost that avoids that coupling entirely |
| `spec.stages[]` writes use `client.MergeFromWithOptimisticLock`, reversing this plan's earlier assumption that the field had exactly one writer | The Lifecycle Operator now also writes into `spec.stages[]` for day-2 stage additions to an object it didn't just create, making this a genuinely shared list with two writers — exactly the case `operator-best-practices.md`'s "Fan-out to Child Objects" rule requires a lock for | Keeping the unlocked patch (this plan's original position) risks a silent lost update: a JSON merge patch on a plain list field replaces the whole array, so a stale local read from either side can drop the other's concurrent addition with no error to signal it |
| `LifecycleState`'s watch carries a custom `statusChangedPredicate` rather than no predicate at all | `LifecycleState` can now be created by either side, so its `Create` event is frequently VM Operator's own write reflected back — and VM Operator's own `spec.stages[]` patches would otherwise re-trigger a reconcile of the VM that just made them, for no new information | No predicate (this plan's original position) was correct only while VM Operator was the sole writer of everything except `status`; once the Lifecycle Operator's day-2 writes and VM Operator's own `Create`/spec writes both flow through the same watched object, filtering to `status`-only changes is what keeps the fan-out from generating self-inflicted reconcile noise |

## Blocking items before implementation starts


Confirming `AggregatedLifecycleHooks`'s exact shape (one per namespace, `status.objects[].{group,kind,stages[]}`) and that they are willing to patch a new `status.stages[]` entry onto an already-existing `LifecycleState` when a day-2 `LifecycleHook` targets a stage that object doesn't yet track (see model.md "`LifecycleState`"). Neither blocks starting implementation — both are consistent with `research.md`'s Candidate 2 and with capabilities the Lifecycle Operator already has (writing `LifecycleState.status`), not new capabilities being requested.
