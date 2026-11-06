# Tasks: Blocking Lifecycle Hooks

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Epic**: vmop-3377

<!--
This file lists only the tasks implemented by the change set that contains it.
Each implementation PR adds its own section(s) here, so the spec PR carries no
tasks.md. Remaining work is derived from plan.md's "Controller / webhook impact"
and "Test strategy" sections.

TODO: fill in [vmop-NNN] tags once story/sub-task tickets are filed under vmop-3377.
-->

## Setup

- [x] T001 Scaffold `external/lifecycle` module (`go.mod`, `api/v1alpha1/{doc.go,groupversion_info.go}`) mirroring `external/byok`'s structure
- [x] T002 [P] Add `LifecycleState` types (`api/v1alpha1/lifecyclestate_types.go`) matching the Lifecycle Operator's published schema (target identity via owner reference and labels, no `spec.target`), plus generated deepcopy
- [x] T003 [P] Add `pkgcfg.Features.LifecycleHooks` in `pkg/config/config.go`
- [x] T004 Wire the `supports_vm_service_lifecycle_hooks` capability to `pkgcfg.Features.LifecycleHooks` in `pkg/config/capabilities/capabilities.go`, mirroring the existing `BringYourOwnEncryptionKey` capability-driven wiring
- [x] T005 [P] Add the single `VirtualMachineConditionLifecycleHookReady` condition type and its four stage reason constants (`VirtualMachineLifecycleHookReady{Create,PowerStateChange,Delete,ResourceDelete}StageReason`) to `api/v1alpha6/condition_consts.go`, matching the Lifecycle Operator framework's collapsed single-condition model
- [x] T006 Register `lifecyclev1.AddToScheme` in `pkg/manager/manager.go` and `test/builder/fake.go`
- [x] T007 Generate the external CRD manifests (`make generate-external-manifests`) into `config/crd/external-crds/lifecycle.vcfa.vmware.com_{lifecyclestates,lifecyclesubscribedstages,lifecyclehooks}.yaml`; envtest only — VM Operator does not install these CRDs in production, the Lifecycle Operator's package does
- [x] T008 Vendor the `LifecycleHook` and `LifecycleSubscribedStages` types (`lifecyclehook_types.go`, `lifecyclesubscribedstages_types.go`, plus generated deepcopy) in `external/lifecycle/api/v1alpha1/`; `LifecycleSubscribedStages` is the `Get`-only data source for `GetOrCreateLifecycleState` — types are vendor-only; VM Operator does not watch or list either type
- [x] T009 [P] Vendor `LifecycleStateName(kind, uid)` and `SubscribedStagesName(group, kind)` naming helpers (`external/lifecycle/api/v1alpha1/naming.go`) so VM Operator and the Lifecycle Operator derive the same object names independently
- [x] T010 Vendor the `LifecycleStages` type (`lifecyclestages_types.go`, plus generated deepcopy) in `external/lifecycle/api/v1alpha1/` and generate `config/crd/external-crds/lifecycle.vcfa.vmware.com_lifecyclestages.yaml` (envtest only); VM Operator writes this type at startup (`plan.md` §7)

## Library (`pkg/lifecycle`)

Nothing outside `pkg/lifecycle` calls the library in this change set, so it has no runtime effect with the capability on or off. Tests set `Features.LifecycleHooks` in the context and use the fake client; no cluster or capability rollout is needed.

- [x] T011 Add `pkg/lifecycle/stage.go` basics: `lifecycleStateFinalizer`, stage name constants, `ErrHooksBlocked` (`pkgerr.NoRequeueNoErr`), and `Result{Proceed}`; use the vendored `LifecycleStateName` and `SubscribedStagesName` helpers
- [x] T012 Add `GetOrCreateLifecycleState` (`plan.md` §3): capability off → `nil`; cached `Get` with binding verification and finalizer adoption; `LifecycleSubscribedStages` lookup; owner-referenced `Create` with every listed stage at `(false, false)`; `AlreadyExists` → `RequeueError`; namespace-terminating rejection → `nil`
- [x] T013 Add `ReconcileStage` (`plan.md` §1): the decision table with optimistic-lock pause and release, the single `LifecycleHookReady` condition, `HooksReady=True/HookFailed` released with `Proceed=false` and the `LifecycleState` finalizer kept, the hook-failed message retained while the old status entry lingers, ResourceDelete finalizer removal only when proceeding, and capability-off passthrough
- [x] T014 [P] Add `AbandonStage`: release a paused stage without running its action and set the condition `True`; no-op when `nil`, not paused, or capability off
- [x] T015 [P] Add `MarkUnknown`: set `LifecycleHookReady` `Unknown` with the stage reason and `Could not evaluate <Stage> hooks: <error>`; write nothing for a `409`
- [x] T016 [P] Add `ReleaseLifecycleState`: remove only VM Operator's finalizer and release any other paused stage in one optimistic-lock write; with the capability off, one `APIReader` `Get`, no-op on `NotFound` or no-match; never delete the object
- [x] T017 [P] Add exported `StatusChangedPredicate` for the `LifecycleState` watch (`plan.md` §4)
- [x] T018 [P] Add `pkg/lifecycle/stages.go`: `PublishStages` with the desired `vmoperator-stages` defined in Go, applied with `CreateOrPatch`, a no-op when the capability is off, short bounded backoff, returning the error so the caller can log and continue; add the `+kubebuilder:rbac` marker for `lifecyclestages` (get, create, patch) and regenerate manifests. Not wired into `main.go` here
- [x] T019 Add `pkg/lifecycle/{lifecycle_suite_test.go,stage_test.go}` covering every decision-table row including the `HookFailed` release-and-retry loop and ResourceDelete keeping the finalizer on `HookFailed`, optimistic-lock conflicts, evaluation failure, abandon, release, capability-off call counts, and the predicate
- [x] T020 [P] Add `pkg/lifecycle/stages_test.go` covering create, drift overwrite, unchanged object not written, capability off, and publish failure

## Controller: watch, adopt, release (`controllers/virtualmachine/virtualmachine`)

An independently testable slice. With the capability on and a `LifecycleState` bound to a VM, VM Operator reconciles the VM when the `LifecycleState` status changes, adopts it by adding its finalizer, logs what it read, and removes its finalizer when the VM is deleted. No stage is paused or held: the Create, PowerStateChange, Delete and ResourceDelete gates are not part of this change set, so no VM operation is blocked.

- [x] T021 Add `+kubebuilder:rbac` markers for `lifecyclestates` (get, list, watch, create, patch, delete) and `lifecyclesubscribedstages` (get, list, watch) to `virtualmachine_controller.go`; regenerate `config/rbac/role.yaml` with `make generate-manifests`
- [x] T022 Add an `APIReader client.Reader` field to `Reconciler`, set from `mgr.GetAPIReader()` through a new `NewReconciler` parameter; update the existing unit-test caller
- [x] T023 Add the `LifecycleState` watch to `AddToManager`, gated on `Features.LifecycleHooks`: `EnqueueRequestForOwner` with `lifecycle.StatusChangedPredicate{}`
- [x] T024 `ReconcileNormal`: after the pause-annotation and VM-finalizer-add returns, call `GetOrCreateLifecycleState` once so a `LifecycleState` is adopted (finalizer added); return its error. Log the `LifecycleState` name and its stages' `workflowPaused`/`workflowResumed` and `HooksReady` at info level
- [x] T025 `ReconcileDelete`: after the pause and `delete.check` early returns and before the VM finalizers are removed, call `GetOrCreateLifecycleState` and then `ReleaseLifecycleState` (with `APIReader`), and log the release. It is also called with the capability off. A `ReleaseLifecycleState` error is returned and the VM finalizers are kept
- [x] T026 Add unit tests to `virtualmachine_controller_unit_test.go` (`testlabels.Controller`): adoption adds the finalizer; delete removes only VM Operator's finalizer and keeps others; capability off does only the `APIReader` read; error keeps the VM finalizers
- [x] T027 [P] Add an envtest case (`testlabels.EnvTest`) in the controller's intg test: a `LifecycleState` status change re-queues the owning VM; VM Operator's own `spec` write does not

## Not in this change set

Tracked for follow-up PRs, derived from `plan.md`'s "Controller / webhook impact" and "Test strategy" sections:

- `vmutil.IsRestartDue` split out of `RestartAndWait`
  (`pkg/util/vsphere/vm/power_state.go`)
- Create, PowerStateChange and Delete gate call sites in
  `pkg/providers/vsphere/vmprovider_vm.go`
- Create, PowerStateChange, Delete and ResourceDelete stage gates, `AbandonStage`
  call sites, `ReconcileStage` use in the controller, and Delete event
  suppression (`hooksBlocked`)
- `initLifecycleStages()` in `main.go` after `initCRDs()`
- `test/builder/fakelifecycle/` (test-only fake Lifecycle Operator)
- `test/e2e/vmservice/vmservice/virtualmachine/vm_lifecycle_hooks.go`,
  including the `vmoperator-stages` startup check (SC-011)
