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

## Create gate (`pkg/providers/vsphere`)

With the capability on and a hook registered for `Create`, VM creation in vCenter is held until the hooks resolve, for both the blocking and the async create paths. With the capability off, or no hook registered, behavior is unchanged.

- [x] T036 [vmop-3565] `createOrUpdateVirtualMachine`: after `getVM` returns `foundVM == nil` and `getCreateArgs` succeeds, and before the vSphere create (the blocking `createVirtualMachine` call, or the async goroutine start), call the private helper `reconcileLifecycleStage` with `lifecycle.StageCreate`; blocked returns `lifecycle.ErrHooksBlocked`, which the controller emits as `CreatePending`, not `CreateFailure`. Gating after the prerequisites keeps a transient network, policy or image cache failure from re-running the hooks. Key off `getVM`, not `Status.UniqueID`, because snapshot revert wipes `Status` for an existing VM
- [x] T037 [vmop-3565] Add unit tests to `pkg/providers/vsphere/vmprovider_vm_test.go` (`testlabels.VCSim`): no hook creates; blocked returns `ErrHooksBlocked` and issues no vSphere create; `LifecycleHookReady` is `False` with reason `CreateStage` while paused and `True` after release; `foundVM != nil`, including wiped `Status` after revert, never reaches the gate; a `getCreateArgs` failure does not pause; a failed vSphere create after release pauses again; capability off is unchanged
- [x] T038 [P] [vmop-3565] Add envtest cases (`testlabels.EnvTest`) in the VM controller's intg test: the VM is not created before the Create `HooksReady` flip; the `LifecycleState` is created with every subscribed stage at `(false, false)`; a `LifecycleState` carrying `Delete` is created at the Create checkpoint
- [ ] T039 [P] [vmop-3565] Add E2E coverage in `test/e2e/vmservice/vmservice/virtualmachine/vm_lifecycle_hooks.go`: a VM with a Create hook stays uncreated with `LifecycleHookReady=False` until the hook resolves, then is created with no other trigger; with no hooks, behavior is unchanged

## Delete gate (`pkg/providers/vsphere`, VM controller)

With the capability on and a hook registered for `Delete`, deletion of the vSphere VM is held until the hooks resolve. With the capability off, or no hook registered, behavior is unchanged.

- [x] T040 [vmop-3850] `DeleteVirtualMachine`: after the `paused.ByAdmin` handling and before `virtualmachine.DeleteVirtualMachine`, call the private helper `reconcileLifecycleStage` with `lifecycle.StageDelete`; blocked returns `lifecycle.ErrHooksBlocked`. No vSphere VM keeps the existing early `nil` return before the gate, so Delete passes. With `SkipDeletePlatformResourceKey` the VM is only unregistered, so Delete is not evaluated
- [x] T041 [vmop-3850] `ReconcileDelete`: set a local `hooksBlocked` flag when `DeleteVirtualMachine` returns `ErrHooksBlocked` and suppress the deferred `EmitEvent(..."Delete"...)` while it is set; other delete errors still emit `DeleteFailure` and each VM gets exactly one `DeleteSuccess`. Do not use `filterNoRequeueErr`
- [x] T042 [vmop-3850] Add unit tests to `pkg/providers/vsphere/vmprovider_vm_lifecyclehooks_test.go` (`testlabels.VCSim`): blocked returns `ErrHooksBlocked` and `virtualmachine.DeleteVirtualMachine` is not called; released or no entry is unchanged; no vSphere VM returns `nil` before the gate and creates no `LifecycleState`; capability off is unchanged; `LifecycleHookReady` is `False` with reason `DeleteStage` while paused
- [x] T043 [vmop-3850] Add unit tests to `virtualmachine_controller_unit_test.go` (`testlabels.Controller`): no events while blocked; one `DeleteSuccess` across the blocked-then-released sequence; an unsupported-connection-state `NoRequeueError` still emits `DeleteFailure`; the existing `SkipDeletePlatformResourceKey` case already shows the provider, and so the gate, is not called
- [x] T044 [P] [vmop-3850] Add an envtest case (`testlabels.EnvTest`) in the VM controller's intg test: a day-2 `Delete` hook created after the VM exists blocks deletion until `HooksReady`
- [ ] T045 [P] [vmop-3850] Add E2E coverage in `test/e2e/vmservice/vmservice/virtualmachine/vm_lifecycle_hooks.go`: deleting a VM with a `Delete` hook keeps the vSphere VM until the hook resolves, then deletes it; `Delete` hooked alone

## ResourceDelete gate (VM controller)

With the capability on and a hook registered for `ResourceDelete`, removal of VM Operator's finalizer is held until the hooks resolve, after Delete has resolved. With the capability off, or no hook registered, behavior is unchanged. The `APIReader` field and its use by `ReleaseLifecycleState` with the capability off already exist (T022, T025).

- [x] T046 [vmop-3851] `ReconcileDelete`: after the Delete step and the stale-PVC OwnerRef cleanup, and before `ReleaseLifecycleState` and the finalizer removal, get the state and call `lifecycle.ReconcileStage(StageResourceDelete)`; blocked returns `nil` with the VM finalizers kept. Straight-line ordering means it is never evaluated before Delete resolves, and it runs also when Delete is skipped (`SkipDeletePlatformResourceKey`) or the vSphere VM is already gone. Blocked also sets `hooksBlocked`, so the deferred `Delete` event is not emitted and the VM still gets one `DeleteSuccess`. Remove the T025 `TODO`
- [x] T047 [vmop-3851] Proceed: `ReconcileStage` already resumes and releases in one write (T013), so `ReleaseLifecycleState` takes the state and uses it as is when not nil (nil reads as before), then remove the VM finalizers. Re-reading from the cache is stale and conflicts with the resume write. Capability off still reads through `APIReader`
- [x] T048 [P] [vmop-3851] Add unit tests to `pkg/lifecycle/stage_test.go` for the release with a supplied state: no read, no extra write after `ReconcileStage` resumed `ResourceDelete`, only VM Operator's finalizer removed, no `APIReader` read with the capability off
- [x] T049 [vmop-3851] Add unit tests to `virtualmachine_controller_unit_test.go` (`testlabels.Controller`): VM finalizer kept and no event while blocked; evaluated only after Delete resolves; resume and release in one `LifecycleState` write, before the VM finalizer is removed; other finalizers kept and one `DeleteSuccess` across the sequence; a failed finalizer patch after resume re-pauses. Capability off releasing via `APIReader` is already covered by T026
- [x] T050 [P] [vmop-3851] Add an envtest case (`testlabels.EnvTest`) in the VM controller's intg test: with hooks on all four stages none passes early, and once both finalizers clear the VM is gone and the `LifecycleState` no longer carries VM Operator's finalizer (envtest has no garbage collector to delete it)
- [ ] T051 [P] [vmop-3851] Add E2E coverage in `test/e2e/vmservice/vmservice/virtualmachine/vm_lifecycle_hooks.go`: the VM and the Kubernetes resource remain after the vSphere VM is deleted until the `ResourceDelete` hook resolves; `ResourceDelete` hooked alone

## Not in this change set

Tracked for follow-up PRs, derived from `plan.md`'s "Controller / webhook impact" and "Test strategy" sections:

- The remaining `AbandonStages` call sites (`ReconcileDelete`)
- `initLifecycleStages()` in `main.go` after `initCRDs()`
- `test/builder/fakelifecycle/` (test-only fake Lifecycle Operator)
- `test/e2e/vmservice/vmservice/virtualmachine/vm_lifecycle_hooks.go`, including the `vmoperator-stages` startup check (SC-011)
