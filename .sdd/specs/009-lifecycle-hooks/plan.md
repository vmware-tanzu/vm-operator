# Implementation Plan: Blocking Lifecycle Hooks

- **Spec**: [`spec.md`](./spec.md)
- **Model**: [`model.md`](./model.md)
- **Epic**: vmop-3377
- **Date**: 2026-08-24
- **Status**: Draft (only condition/reason naming remains open; not a blocker for Phase 1/2)

## Summary

Add a consumer-side integration with the externally-owned `lifecycle.vcfa.vmware.com` CRDs so VM Operator can pause and resume four `VirtualMachine` reconcile checkpoints (Create, PowerStateChange, Delete, ResourceDelete) based on a per-VM `LifecycleState` resource, without owning or reconciling the Lifecycle Operator's CRDs itself.

## Technical context

- **Go version**: repo default (see root `go.mod`).
- **API version(s) touched**: current `vmopv1` alias target (additive conditions only — no field removal, no version bump).
- **Modules touched**: root module (`controllers/`, `pkg/`, `api/`) plus a new `external/lifecycle` sub-module.
- **New dependencies**: none beyond the new `external/lifecycle` module (own `go.mod`, no third-party deps).

## Constitution check

| Rule | Status | Notes |
|---|---|---|
| API compatibility (additive only) | OK | New condition types only; no field removal/rename. |
| Controllers are thin | OK | Stage-gate logic lives in a new `pkg/lifecycle` package, called from controllers/provider; controllers only orchestrate. |
| No controller calls vSphere directly | OK | Stage gate reads/writes `LifecycleState` via the k8s client, not vSphere; vSphere-side checkpoints (Create, PowerStateChange, Delete) are still invoked only from `pkg/providers/vsphere`. |
| Controllers for non-`vmoperator.vmware.com` groups don't live in `controllers/` | OK | This feature adds **no new controller** — it adds watches/logic to the existing `controllers/virtualmachine/virtualmachine` controller, which already reconciles `vmoperator.vmware.com`. `LifecycleHook`/`LifecycleState` are read/patched, never reconciled by a VM-Operator-owned controller loop. |
| External vendored APIs live under `external/` | OK | New `external/lifecycle` module, mirroring `external/byok`. |
| `+kubebuilder:rbac` markers document permissions | OK | New markers for `lifecycle.vcfa.vmware.com` `lifecyclestates`/`lifecyclestates/status` (get/list/watch/create/patch) only — `LifecycleHook` is never read directly by VM Operator (see `model.md`), so no RBAC needed for it. |
| E2E ships with cluster-observable behavior | OK (mandatory) | All four stages are cluster-observable (pausing reconciliation, new conditions) — E2E required per `e2e-sync-with-changes.md`, tracked in `tasks.md`. |
| Feature flag default / rollout documented | OK | Gated by a Supervisor capability, not a bare always-on flag — see "Rollout / migration" below and `model.md` "Capability gating". |

No complexity-tracking entries — no constitutional rule is being bent.

## Project structure

```
external/lifecycle/                              # NEW module — vendored client types only
  go.mod
  api/v1alpha1/
    doc.go
    groupversion_info.go
    lifecyclestate_types.go                       # LifecycleState (Get/Create/Patch target)
    lifecyclehook_types.go                         # LifecycleHook (types only; read via LifecycleState.status.hooks, not listed directly — pending open question)
    zz_generated.deepcopy.go

pkg/lifecycle/                                     # NEW — stage-gate helper, reusable from controller + provider
  stage.go                                          # CheckStage(ctx, cli, obj, stageName) (Result, error)
  stage_test.go

pkg/config/config.go                                # + Features.LifecycleHooks
pkg/config/capabilities/capabilities.go             # + new capability -> Features.LifecycleHooks wiring (mirrors BringYourOwnEncryptionKey)

api/v1alphaN/condition_consts.go                    # + 4 new condition type constants (name TBD, see spec open questions)

controllers/virtualmachine/virtualmachine/
  virtualmachine_controller.go                      # + RBAC markers, + Watches(&lifecyclev1.LifecycleState{}, ...) gated by Features.LifecycleHooks
                                                     # ReconcileNormal: Create-stage gate before first-create path
                                                     # ReconcileDelete: ResourceDelete-stage gate before finalizer removal

pkg/providers/vsphere/vmprovider_vm.go              # Delete-stage gate before DeleteVirtualMachine's vSphere call
pkg/providers/vsphere/session/session_vm_update.go  # PowerStateChange-stage gate before the power-on/off task is issued

config/crd/external-crds/lifecycle.vcfa.vmware.com_lifecyclestates.yaml   # generated via make generate-external-manifests
config/lifecycle/vmoperator-stages.yaml             # static LifecycleStages instance shipped via helm chart

test/builder/fake.go                                # + lifecyclev1.AddToScheme(scheme)
```

## API / CRD strategy

Additive only. No `vmoperator.vmware.com` CRD schema changes beyond new condition type constants (conditions are not per-version typed fields, so no conversion webhook work is needed). `external/lifecycle` vendors the Lifecycle Operator's schema as published in the "[API design] Blocking Lifecycle stages" design doc (see `research.md`) — VM Operator does not modify or extend that schema.

## Controller / webhook impact

- **`controllers/virtualmachine/virtualmachine`**: gains a `Watches(&lifecyclev1.LifecycleState{}, handler.EnqueueRequestsFromMapFunc(...))` (via owner-reference mapping, since `LifecycleState` is owned by the `VirtualMachine`), gated by `pkgcfg.FromContext(ctx).Features.LifecycleHooks`, so a `HooksReady` flip promptly re-triggers reconciliation. `ReconcileNormal` gains the Create-stage gate before the provider's create path is invoked; `ReconcileDelete` gains the ResourceDelete-stage gate immediately before `controllerutil.RemoveFinalizer`.
- **No new webhook** — stage gating is a reconcile-time concern, not an admission-time one.
- **`pkg/providers/vsphere`**: `vmprovider_vm.go`'s `DeleteVirtualMachine` gains the Delete-stage gate before issuing the vSphere delete/unregister call; `session_vm_update.go`'s power-state branch gains the PowerStateChange-stage gate before issuing the power task.
- **New RBAC**: `lifecycle.vcfa.vmware.com` `lifecyclestates` (get, list, watch, create, patch) and `lifecyclestates/status` (get, patch) in the VM Operator controller-manager role.
- **New capability-gated flag**: `pkgcfg.Features.LifecycleHooks`, driven entirely by a new Supervisor capability (name TBD) via `pkg/config/capabilities/capabilities.go`, the same mechanism that can drive `BringYourOwnEncryptionKey` today — no independent env-var default (see Rollout).

## Test strategy

- **Unit** (`testlabels.Controller`): `pkg/lifecycle/stage_test.go` covers the `CheckStage` decision table (no `LifecycleState` → proceed; `workflowPaused=false` → pause+patch; `workflowPaused=true` + `HooksReady=False` → wait; `HooksReady=True` → resume) using a fake client with `lifecyclev1.AddToScheme` registered per `test/builder/fake.go`.
- **Integration** (`testlabels.EnvTest`): extend `controllers/virtualmachine/virtualmachine`'s existing suite with cases per stage — hook absent (no-op), hook present and blocking, `HooksReady` flip triggers resume via the new watch.
- **E2E** (mandatory, `e2e-sync-with-changes.md`): new scenarios under `test/e2e/vmservice/vmservice/virtualmachine/` exercising all four stages end-to-end against a real (or vcsim-backed) `LifecycleState` fixture — see `tasks.md` for the concrete file.

## Rollout / migration

- **Capability gate**: a new Supervisor capability (name TBD) is the sole gate for `pkgcfg.Features.LifecycleHooks` — there is no independently-toggleable env-var default, matching `spec.md` US4's "entire feature gated by a capability" requirement. Enabling the capability on a Supervisor with no `LifecycleHook`s registered anywhere MUST still be a no-op (per spec's edge cases) — that's the rollout safety net once the capability itself is on.
- **Schema upgrade**: none — no existing VM spec/status field changes.
- **Partner comms**: announce via the same channel/design-doc-review process as other new-condition, capability-gated features (e.g. `supports_telco_vm_service_api`) once the spec's remaining open question (condition/reason and capability naming) is resolved.
- **Release notes**: ship with the first PR that turns on any stage gate, referencing the new conditions and the capability name.

## Complexity tracking

None.

## Blocking items before implementation starts

Resolved: power-off inclusion (both directions, `Reentrant`), per-stage `type`/`blocking` defaults, `HooksReady`-only handling with no failure-detail parsing (see `research.md` "Terminal failures"), and capability-based gating for the whole feature (`spec.md` US4, `model.md` "Capability gating") — no longer deferred. Still open: exact condition/reason and capability names, which is a naming-only detail that doesn't block Phase 1/2 (vendoring, capability wiring, scaffolding) or the shape of Phase 3+ — it only needs to land before `api/v1alphaN/condition_consts.go` (T004) and the capability definition (T003) are finalized for review.
