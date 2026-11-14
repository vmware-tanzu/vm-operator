# Tasks: VirtualMachineInfraMigration Status on VirtualMachine

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Epic**: vmop-3432
- **Story**: vmop-3875

<!--
Check off each task as it lands. Tasks that produce shipping code carry the story tag [vmop-3875].
User stories: US1 is "DevOps user: seeing that a migration has converged", US2 is "CSP admin: rollout safety".
-->

## Phase 1 — Setup

- [x] T001 [vmop-3875] Add the condition constants `VirtualMachineConditionInfraMigrationPrefix` (`VirtualMachineInfraMigration`) and `VirtualMachineConditionInfraMigrationReady` (`VirtualMachineInfraMigrationReady`) in `api/v1alpha6/virtualmachine_types.go`
- [x] T002 [vmop-3875] Add `status.volumeAttributesClassName` in `api/v1alpha6/virtualmachine_types.go`, and regenerate with `make generate-go-conversions` and `make generate-manifests` (`api/v1alpha{1..5}/zz_generated.conversion.go`, `config/crd/bases/vmoperator.vmware.com_virtualmachines.yaml`)
- [x] T003 Wire `external/mobility-operator` into the root module: `replace` and `require` in `go.mod`, and the `mobilityv1a4` alias in `.golangci.yml`
- [x] T004 Add `external/mobility-operator/api/v1alpha4` with only the `VirtualMachineInfraMigration` API, generate its deepcopy from `make generate-go`, and generate the test-only CRD from `make generate-external-manifests` (`Makefile`, `external/mobility-operator/api/v1alpha4/`, `config/crd/external-crds/`, `config/crd/external-crds/README.md`)

## Phase 2 — Foundational

- [x] T005 Register the mobility v1alpha4 scheme in `pkg/manager/manager.go` (behind `StoragePolicyMutability`) and in `test/builder/fake.go`, including the status subresource for the fake client
- [x] T006 Regenerate RBAC for the new `virtualmachineinframigrations` rule in `config/rbac/role.yaml`
- [x] T007 [P] [US1] [vmop-3875] Add `IsReady(from Getter, options ...ReadyOption)` with the `WithGeneration` option, and `GetObservedAt(from Getter, generation int64)`, which returns the conditions whose `observedGeneration` has caught up to the generation, with unit tests (`pkg/conditions/conditions.go`, `pkg/conditions/getter.go`, `pkg/conditions/getter_test.go`)
- [x] T008 [P] [US1] [vmop-3875] Add `GetVACNamesForStoragePolicyID` and its unit tests (`pkg/util/kube/storage.go`, `pkg/util/kube/storage_test.go`)

## Phase 3 — User Story 1 (DevOps user: seeing that a migration has converged)

- [x] T009 [US1] [vmop-3875] Implement the reconciler, the `VirtualMachine` watch mapper (dropped when no migration has the same namespace and name), and `AddToManager` (`controllers/mobility/virtualmachineinframigration/virtualmachineinframigration_controller.go`). For a VirtualMachine that exists and is not being deleted, mirror every condition of the migration that describes its current generation (its own `observedGeneration` is at least the migration's `metadata.generation`), whatever its status, as `VirtualMachineInfraMigration<Type>` with the migration condition's status, its reason (the status when the reason is empty), and its message followed by `@ VirtualMachineInfraMigration/<name>` (the reference alone when the message is empty), without copying `observedGeneration`. Remove a mirrored condition the migration stops reporting or reports for a previous generation, leave every other condition alone by patching through `pkg/patch.Helper` with `patch.WithOwnedConditions`, and take no action when the migration is deleted. Add no pause of the VirtualMachine's reconciliation and no validation of `spec.volumeAttributesClassName`, because the existing skip while a vSphere task is running is enough. Covers criteria 1–5, 9, and 10
- [x] T010 [US1] [vmop-3875] Record `status.volumeAttributesClassName` in the same reconciler, only when `conditions.IsReady(obj, conditions.WithGeneration(obj.Generation))` is true: resolve the migration's `spec.targetStorage.homeStoragePolicyID` to VolumeAttributesClasses, write the one named in the VirtualMachine's `spec.volumeAttributesClassName` when it is among them, never clear the field, and emit a `VolumeAttributesClassMismatch` warning event when no class refers to the policy or the spec's class is unset or not among them. The conditions are mirrored in every case, and a migration that is not ready neither records the field nor emits the event (`controllers/mobility/virtualmachineinframigration/virtualmachineinframigration_controller.go`). Covers criteria 3 and 6–8
- [x] T011 [US1] [vmop-3875] Unit specs for T009 and T010 with the fake client (`controllers/mobility/virtualmachineinframigration/virtualmachineinframigration_controller_test.go`), labeled `testlabels.Controller` and `testlabels.API`: the mirroring of one and several conditions with their status, reason, message, and reference, the empty reason and empty message fallbacks, no `observedGeneration`, a changed and a removed condition, a condition that describes a previous generation (not mirrored, and a copy mirrored earlier removed), current and previous generations mixed, a condition without an `observedGeneration`, other conditions preserved, an unchanged last transition time on re-reconcile, a missing or deleting VirtualMachine, a missing migration, and the VolumeAttributesClass cases for a ready, not ready, and stale-generation migration
- [x] T012 [US1] [vmop-3875] Integration specs for T009 and T010 against envtest in the same file, labeled `testlabels.Controller` and `testlabels.EnvTest`, with the suite bootstrap in `controllers/mobility/virtualmachineinframigration/virtualmachineinframigration_controller_suite_test.go`: conditions changing over time, the migration's spec changing so that its generation advances, a migration whose conditions were set before the VirtualMachine exists, the status field recorded only when the migration is ready and the spec agrees, and the conditions and the status field surviving the migration's deletion
- [ ] T013 [US1] [vmop-3875] Add E2E coverage on a Supervisor with the mobility operator and the `StoragePolicyMutability` capability, in `test/e2e/vmservice/vmservice/virtualmachine/`, per `e2e-sync-with-changes.md`. Scoped as a follow-up to this change, as stated in the spec's non-goals

## Phase 4 — User Story 2 (CSP admin: rollout safety)

- [x] T014 [US2] [vmop-3875] Start the controller only when `StoragePolicyMutability` is enabled, in `controllers/mobility/controllers.go`, and register it in `controllers/controllers.go`. With the capability disabled the `VirtualMachineInfraMigration` API is not required. Covers the CSP admin criterion
