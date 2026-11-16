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

- [x] T011 Add `pkg/lifecycle/stage.go` basics: `lifecycleStateFinalizer`, stage name constants, `ErrHooksBlocked` (`pkgerr.NoRequeueNoErr`), and `Result{Proceed}`; use `vmopv1.GroupName` for `TargetGroup` and the vendored `LifecycleStateName` and `SubscribedStagesName` helpers
- [x] T012 Add `GetOrCreateLifecycleState` (`plan.md` §3): capability off → `nil`; cached `Get` with binding verification (target labels and `metav1.IsControlledBy`) and finalizer adoption; `LifecycleSubscribedStages` lookup; owner-referenced `Create` with every listed stage at `(false, false)`; `AlreadyExists` → `RequeueError`; namespace-terminating rejection → `nil`
- [x] T013 Add `ReconcileStage` (`plan.md` §1): the decision table with optimistic-lock pause and release, the single `LifecycleHookReady` condition, `HooksReady=True/HookFailed` released with `Proceed=false` and the `LifecycleState` finalizer kept, the hook-failed message retained while the old status entry lingers, ResourceDelete finalizer removal only when proceeding, and capability-off passthrough
- [x] T014 [P] Add `AbandonStages`: read the VM's existing `LifecycleState` once, never creating it, and release every named paused stage in a single write without running its action, setting the condition `True`; no-op when there is no `LifecycleState`, a stage is not paused, or the capability is off
- [x] T015 [P] Add `MarkUnknown`: set `LifecycleHookReady` `Unknown` with the stage reason and `Could not evaluate <Stage> hooks: <error>`; write nothing for a `409`
- [x] T016 [P] Add `ReleaseLifecycleState`: read the VM's existing `LifecycleState` itself (never creating it), remove only VM Operator's finalizer and release any other paused stage in one optimistic-lock write; with the capability off, one `APIReader` `Get`, no-op on `NotFound` or no-match; never delete the object
- [x] T017 [P] Add exported `StatusChangedPredicate` for the `LifecycleState` watch (`plan.md` §4)
- [x] T018 [P] Add `pkg/lifecycle/stages.go`: `PublishStages` with the desired `vmoperator-stages` defined in Go, applied with `CreateOrPatch`, a no-op when the capability is off, short bounded backoff, returning the error so the caller can log and continue; add the `+kubebuilder:rbac` marker for `lifecyclestages` (get, create, patch) and regenerate manifests. Not wired into `main.go` here
- [x] T019 Add `pkg/lifecycle/{lifecycle_suite_test.go,stage_test.go}` covering every decision-table row including the `HookFailed` release-and-retry loop and ResourceDelete keeping the finalizer on `HookFailed`, optimistic-lock conflicts, evaluation failure, abandon, release, capability-off call counts, and the predicate
- [x] T020 [P] Add `pkg/lifecycle/stages_test.go` covering create, drift overwrite, unchanged object not written, capability off, and publish failure

## Controller: watch, adopt, release (`controllers/virtualmachine/virtualmachine`)

An independently testable slice. With the capability on and a `LifecycleState` bound to a VM, VM Operator reconciles the VM when the `LifecycleState` status changes, adopts it by adding its finalizer, logs its name, and removes its finalizer when the VM is deleted. No stage is paused or held: the Create, PowerStateChange, Delete and ResourceDelete gates are not part of this change set, so no VM operation is blocked.

- [x] T021 Add `+kubebuilder:rbac` markers for `lifecyclestates` (get, list, watch, create, patch, delete) and `lifecyclesubscribedstages` (get, list, watch) to `virtualmachine_controller.go`; regenerate `config/rbac/role.yaml` with `make generate-manifests`
- [x] T022 Add an `APIReader client.Reader` field to `Reconciler`, set from `mgr.GetAPIReader()` through a new `NewReconciler` parameter; update the existing unit-test caller
- [x] T023 Add the `LifecycleState` watch to `AddToManager`, gated on `Features.LifecycleHooks`: `EnqueueRequestForOwner` with `lifecycle.StatusChangedPredicate{}`
- [x] T024 `ReconcileNormal`: after the pause-annotation and VM-finalizer-add returns, call `GetOrCreateLifecycleState` once so a `LifecycleState` is adopted (finalizer added) before a checkpoint is reached, since namespace deletion removes objects without it; return its error, with a comment saying so
- [x] T025 `ReconcileDelete`: after the pause and `delete.check` early returns and before the VM finalizers are removed, call `ReleaseLifecycleState` (with `APIReader`), which reads the existing `LifecycleState` and never creates one, and log the release with a comment that the log is temporary and for testing; add a `TODO` that the gate PR evaluates the Delete and ResourceDelete stages before this release. It is also called with the capability off. A `ReleaseLifecycleState` error is returned and the VM finalizers are kept
- [x] T026 Add unit tests to `virtualmachine_controller_unit_test.go` (`testlabels.Controller`): adoption adds the finalizer; delete removes only VM Operator's finalizer and keeps others; capability off does only the `APIReader` read; error keeps the VM finalizers
- [x] T027 [P] Add an envtest case (`testlabels.EnvTest`) in the controller's intg test: a `LifecycleState` status change re-queues the owning VM; VM Operator's own `spec` write does not

## PowerStateChange gate (`pkg/providers/vsphere`)

With the capability on and a hook registered for `PowerStateChange`, power-on, power-off, suspend and a due restart are held until the hooks resolve; only the power operation is skipped, so the rest of `updateVirtualMachine`, including snapshot create, still runs. With the capability off, or no hook registered, behavior is unchanged.

- [x] T028 [vmop-3849] Split `vmutil.IsRestartDue` out of `RestartAndWait` (`pkg/util/vsphere/vm/power_state.go`) so `restart()` and the gate share one definition of "due" (VM powered on, `desiredLastRestartTime` newer than the ExtraConfig last restart, not in the future)
- [x] T029 [P] [vmop-3849] Add `IsRestartDue` unit tests to `pkg/util/vsphere/vm/power_state_test.go`: due, not due (older and equal), future time, powered off, nil config; confirm `RestartAndWait` behavior is unchanged
- [x] T030 [vmop-3849] `reconcilePowerState`: inside `if setPowerState` and before `SetPowerState`, call the private helper `reconcileLifecycleStage` (get-or-create, `ReconcileStage(PowerStateChange)`, `MarkUnknown` on a non-409 error); blocked returns `nil` (not `ErrHooksBlocked`). The existing VMGroups apply-power-state-time block moves unchanged into `reconcileApplyPowerStateTime` to keep `reconcilePowerState` under the `gocyclo` limit
- [x] T031 [vmop-3849] `reconcilePowerState`: before `RestartAndWait`, call `IsRestartDue` and run the same helper only when a restart is due; a restart that is not due never pauses
- [x] T032 [vmop-3849] `reconcilePowerState`: one `defer`, registered after the `isVMPaused` and `HasVMRunningTask` returns, calls `abandonPowerStateChange` (`AbandonStages(PowerStateChange)`) unless the local `powerChangeWanted` is set (at `setPowerState`, when a restart is due, and at the `CheckAnnotationPowerOn` skip) or `retErr` is already non-nil, so an error return keeps the hold; its own error becomes `retErr`
- [x] T033 [vmop-3849] Add unit tests to `pkg/providers/vsphere/vmprovider_vm_lifecyclehooks_test.go`, registered from the `vmprovider_vm_test.go` index (`testlabels.VCSim`): blocked power-on, power-off, suspend and due restart issue no power task while the rest of the update, including snapshot create, still runs; restart not due does not pause; on, off and restart pause independently; release lets the operation proceed; reverting `spec.powerState` abandons the hold and the next request pauses afresh; early returns and a return with an error, such as an unparsable `nextRestartTime`, keep the hold; capability off is unchanged; `LifecycleHookReady` is `False` with reason `PowerStateChangeStage` while paused
- [x] T034 [P] [vmop-3849] Add an envtest case (`testlabels.EnvTest`) in the VM controller's intg test: a power state change waits for the `HooksReady` flip and re-pauses on the next change
- [ ] T035 [P] [vmop-3849] Add E2E coverage in `test/e2e/vmservice/vmservice/virtualmachine/vm_lifecycle_hooks.go`: power-on, power-off and restart are each held until the hook resolves

## Not in this change set

Tracked for follow-up PRs, derived from `plan.md`'s "Controller / webhook impact" and "Test strategy" sections:

- Create and Delete gate call sites in `pkg/providers/vsphere/vmprovider_vm.go`
- Create, Delete and ResourceDelete stage gates, the remaining `AbandonStages` call sites (`ReconcileDelete`), `ReconcileStage` use in the controller, and Delete event suppression (`hooksBlocked`)
- `initLifecycleStages()` in `main.go` after `initCRDs()`
- `test/builder/fakelifecycle/` (test-only fake Lifecycle Operator)
- `test/e2e/vmservice/vmservice/virtualmachine/vm_lifecycle_hooks.go`, including the `vmoperator-stages` startup check (SC-011)
