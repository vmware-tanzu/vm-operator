# Tasks: Blocking Lifecycle Hooks

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Epic**: vmop-3377

<!--
This file lists only the tasks implemented by the change set that contains it.
Each implementation PR adds its own phase(s) here, so the spec PR carries no
tasks.md. Remaining work is derived from plan.md's "Controller / webhook impact"
and "Test strategy" sections.

TODO: fill in [vmop-NNN] tags once story/sub-task tickets are filed under vmop-3377.
-->

## Phase 1 — Setup

- [x] T001 Scaffold `external/lifecycle` module (`go.mod`, `api/v1alpha1/{doc.go,groupversion_info.go}`) mirroring `external/byok`'s structure
- [x] T002 [P] Add `LifecycleState` types (`api/v1alpha1/lifecyclestate_types.go`) matching the Lifecycle Operator's published schema (target identity via owner reference and labels, no `spec.target`), plus generated deepcopy
- [x] T003 [P] Add `pkgcfg.Features.LifecycleHooks` in `pkg/config/config.go`
- [x] T004 Wire the `supports_vm_service_lifecycle_hooks` capability to `pkgcfg.Features.LifecycleHooks` in `pkg/config/capabilities/capabilities.go`, mirroring the existing `BringYourOwnEncryptionKey` capability-driven wiring
- [x] T005 [P] Add the single `VirtualMachineConditionLifecycleHookReady` condition type and its four stage reason constants (`VirtualMachineLifecycleHookReady{Create,PowerStateChange,Delete,ResourceDelete}StageReason`) to `api/v1alpha6/condition_consts.go`, matching the Lifecycle Operator framework's collapsed single-condition model
- [x] T006 Register `lifecyclev1.AddToScheme` in `pkg/manager/manager.go` and `test/builder/fake.go`
- [x] T007 Generate the external CRD manifests (`make generate-external-manifests`) into `config/crd/external-crds/lifecycle.vcfa.vmware.com_{lifecyclestates,lifecyclesubscribedstages,lifecyclehooks}.yaml`; envtest only — VM Operator does not install these CRDs in production, the Lifecycle Operator's package does
- [x] T008 Vendor the `LifecycleHook` and `LifecycleSubscribedStages` types (`lifecyclehook_types.go`, `lifecyclesubscribedstages_types.go`, plus generated deepcopy) in `external/lifecycle/api/v1alpha1/`; `LifecycleSubscribedStages` is the `Get`-only data source for `GetOrCreateLifecycleState` — types are vendor-only; VM Operator does not watch or list either type
- [x] T009 [P] Vendor `LifecycleStateName(kind, uid)` and `SubscribedStagesName(group, kind)` naming helpers (`external/lifecycle/api/v1alpha1/naming.go`) so VM Operator and the Lifecycle Operator derive the same object names independently

## Phase 2+ — Not in this change set

Tracked for a follow-up PR once Phase 1 lands; derived from `plan.md`'s
"Controller / webhook impact" and "Test strategy" sections:

- `pkg/lifecycle` package: `GetOrCreateLifecycleState`, `ReconcileStage`,
  `AbandonStage`, `MarkUnknown`, `ReleaseLifecycleState`, `ErrHooksBlocked`,
  `statusChangedPredicate`
- `vmutil.IsRestartDue` split out of `RestartAndWait`
  (`pkg/util/vsphere/vm/power_state.go`)
- Create, PowerStateChange, Delete and ResourceDelete gate call sites in
  `pkg/providers/vsphere/vmprovider_vm.go` and
  `controllers/virtualmachine/virtualmachine/virtualmachine_controller.go`
  (RBAC, `LifecycleState` watch, `APIReader` field, ResourceDelete gate,
  Delete event suppression)
- `config/lifecycle/vmoperator-stages.yaml` (the `LifecycleStages` instance)
  and `test/builder/fakelifecycle/` (test-only fake Lifecycle Operator)
- `test/e2e/vmservice/vmservice/virtualmachine/vm_lifecycle_hooks.go`
