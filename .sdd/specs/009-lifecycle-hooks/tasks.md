# Tasks: Blocking Lifecycle Hooks

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Epic**: vmop-3377

<!--
TODO: fill in [vmop-NNN] tags once story/sub-task tickets are filed under vmop-3377.
spec.md's open items are resolved except exact condition/reason and capability naming
(not a blocker — see plan.md "Blocking items before implementation starts").
-->

## Phase 1 — Setup

- [ ] T001 Scaffold `external/lifecycle` module (`go.mod`, `api/v1alpha1/{doc.go,groupversion_info.go}`) mirroring `external/byok`'s structure
- [ ] T002 [P] Add `LifecycleState` types (`api/v1alpha1/lifecyclestate_types.go`) per `model.md`'s field table, plus generated deepcopy
- [ ] T003 [P] Add `pkgcfg.Features.LifecycleHooks` in `pkg/config/config.go` (no independent env-var default — see T004)
- [ ] T004 Wire the new capability (name TBD) to `pkgcfg.Features.LifecycleHooks` in `pkg/config/capabilities/capabilities.go`, mirroring the existing `BringYourOwnEncryptionKey` capability-driven wiring
- [ ] T005 [P] Add the four new condition type constants to `api/v1alphaN/condition_consts.go` (names finalized once spec's open question resolves)
- [ ] T006 Register `lifecyclev1.AddToScheme` in `pkg/manager/manager.go` and `test/builder/fake.go`
- [ ] T007 Generate the external CRD manifest (`make generate-external-manifests`) into `config/crd/external-crds/lifecycle.vcfa.vmware.com_lifecyclestates.yaml`

## Phase 2 — Foundational

- [ ] T008 Implement `pkg/lifecycle/stage.go` — `CheckStage(ctx, cli, obj, stageName) (Result, error)` implementing the get-or-create / pause / resume decision table from `model.md`, mirroring `HooksReady` as the sole non-`True`/`True` signal (no failure-detail parsing)
- [ ] T009 [P] Unit tests for `CheckStage`'s full decision table in `pkg/lifecycle/stage_test.go`
- [ ] T010 Add RBAC markers (`lifecyclestates`, `lifecyclestates/status`) to `controllers/virtualmachine/virtualmachine/virtualmachine_controller.go`
- [ ] T011 Add the `Features.LifecycleHooks`-gated `Watches(&lifecyclev1.LifecycleState{}, ...)` (owner-reference-mapped) to `controllers/virtualmachine/virtualmachine/virtualmachine_controller.go`'s `AddToManager`
- [ ] T012 Author the static `LifecycleStages` manifest (`config/lifecycle/vmoperator-stages.yaml`) per `model.md`'s resolved stage `type`/`blocking` table (`Create`=`Single`, `PowerStateChange`=`Reentrant`, `Delete`=`Single`, `ResourceDelete`=`Single`, all `blocking=true`)

## Phase 3 — User Story 1: VM Create (P1)

- [ ] T013 [US1] Wire `pkg/lifecycle.CheckStage` for the `Create` stage into `controllers/virtualmachine/virtualmachine/virtualmachine_controller.go`'s `ReconcileNormal`, before the provider's create path
- [ ] T014 [P] [US1] Unit tests for the Create-stage gate (hook absent, paused, resumed) in the controller's unit test file
- [ ] T015 [US1] Integration test: hook present blocks vSphere VM creation; `HooksReady` flip (via T011's watch) triggers creation on next reconcile
- [ ] T016 [US1] E2E scenario for VM Create pause/resume in `test/e2e/vmservice/vmservice/virtualmachine/vm_lifecycle_hooks.go`, registered from `test/e2e/vmservice/vmservice_test.go`

## Phase 4 — User Story 2: PowerStateChange (P1)

- [ ] T017 [US2] Wire `CheckStage` for `PowerStateChange` into `pkg/providers/vsphere/session/session_vm_update.go`'s power-state branch
- [ ] T018 [P] [US2] Unit/integration tests confirming unrelated config reconciliation proceeds while the power-state step is paused, and that power-on/power-off pause independently (`Reentrant`)
- [ ] T019 [US2] Extend `vm_lifecycle_hooks.go` (T016) with power-on and power-off pause/resume scenarios

## Phase 5 — User Story 3: VM deletion, vSphere-side then Kubernetes (P1)

- [ ] T020 [US3] Wire `CheckStage` for `Delete` into `pkg/providers/vsphere/vmprovider_vm.go`'s `DeleteVirtualMachine`, before the vSphere delete/unregister call
- [ ] T021 [US3] Wire `CheckStage` for `ResourceDelete` into `controllers/virtualmachine/virtualmachine/virtualmachine_controller.go`'s `ReconcileDelete`, immediately before `controllerutil.RemoveFinalizer`
- [ ] T022 [P] [US3] Unit tests confirming the finalizer is not removed while `Delete` is paused, and that `Delete` must fully resolve before `ResourceDelete` is evaluated (sequential, not parallel)
- [ ] T023 [US3] Extend `vm_lifecycle_hooks.go` with the full vSphere-delete-then-resource-delete sequential scenario, including the case where only one of the two stages has a hook registered

## Phase 6 — User Story 4: capability gating (P0)

- [ ] T024 [US4] Unit tests confirming `Features.LifecycleHooks=false` makes every `CheckStage` call in T013/T017/T020/T021 a pure no-op (no `LifecycleState` `Get`/`Create`, no pause)
- [ ] T025 [US4] Integration test: toggling the capability CR off mid-pause immediately un-pauses stage evaluation on the next reconcile (no `LifecycleState` cleanup required — it's simply no longer consulted)
- [ ] T026 [US4] E2E scenario in `vm_lifecycle_hooks.go` (or a capability-focused sibling file) confirming the capability-disabled Supervisor sees zero behavior change with hooks registered

## Phase 7 — User Story 5: status diagnosability (P0, cuts across US1-3)

- [ ] T027 [US5] Ensure every stage gate sets its condition to a terminal ready/blocked state on every reconcile pass (no `Unknown` left behind) — add the assertion to each stage's unit tests (T014/T018/T022)

## Phase Final — Polish

- [ ] T028 Update `docs/concepts/workloads/vm.md` with a "Lifecycle Hooks" section describing the four stages, their conditions, and the gating capability
- [ ] T029 Add release notes (per `pull-request-standards.md`) referencing the new capability
- [ ] T030 Flip `spec.md` status to `Implemented` once every acceptance criterion is covered and the condition/reason and capability naming open question is resolved
