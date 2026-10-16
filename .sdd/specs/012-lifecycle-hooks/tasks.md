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
- [x] T005 [P] Add the four per-stage condition type constants to `api/v1alpha6/virtualmachine_types.go`: `VirtualMachineConditionCreateHooksReady`, `VirtualMachineConditionPowerStateChangeHooksReady`, `VirtualMachineConditionDeleteHooksReady`, `VirtualMachineConditionResourceDeleteHooksReady` (replaces the single shared `VirtualMachineConditionLifecycleHooksReady`), plus the shared `HooksBlocked` reason
- [x] T006 Register `lifecyclev1.AddToScheme` in `pkg/manager/manager.go` and `test/builder/fake.go`
- [x] T007 Generate the external CRD manifest (`make generate-external-manifests`) into `config/crd/external-crds/lifecycle.vcfa.vmware.com_lifecyclestates.yaml`
- [x] T008 Vendor the `LifecycleHook` and `LifecycleSubscribedStages` types (`lifecyclehook_types.go`, `lifecyclesubscribedstages_types.go`, plus generated deepcopy) in `external/lifecycle/api/v1alpha1/`; `LifecycleSubscribedStages` is the `Get`-only data source for `ReconcileLifecycleState` — types are vendor-only; VM Operator does not watch or list either type
