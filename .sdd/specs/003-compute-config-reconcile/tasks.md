# Tasks: VM Compute Configuration Reconciliation

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Epic**: vmop-3388

<!--
TODO: fill in [vmop-NNN] tags once story/sub-task tickets are filed under the epic. Check off each task as it lands, and add any task discovered mid-implementation that isn't listed yet.
-->

## Phase 1 — Setup

- [ ] T001 Add `spec.resources` / `spec.cpuAdvanced` / `spec.memoryAdvanced` types in `api/v1alpha6/virtualmachine_compute_types.go`
- [ ] T002 [P] Add `VirtualMachineConditionComputeConfigSynced` and the new `ComputeConfigMismatch` reason in `api/v1alpha6/virtualmachine_types.go` — reuse the existing `PrerequisiteNotMet`/`PowerOffRequired` reason constants, do not redeclare them
- [ ] T003 Regenerate deepcopy and CRD manifests (`make generate-go generate-manifests`) — `api/v1alpha6/zz_generated.deepcopy.go`, `config/crd/bases/vmoperator.vmware.com_virtualmachines.yaml`, `config/crd/bases/vmoperator.vmware.com_virtualmachinereplicasets.yaml`

## Phase 2 — Foundational

- [ ] T004 Implement the `computeFieldDef` table and `OverwriteSpecComputeConfig` in `pkg/util/vmopv1/compute_overwrite.go`; see `plan.md` "Field mapping" for spec-field-to-`ConfigSpec` mappings
- [ ] T005 [P] Implement `SyncClassComputeToSpec` and `SyncClassSizeAndAllocationToSpec` in `pkg/util/vmopv1/resize_class_sync.go`
- [ ] T006 [P] Add the schema-upgrade backfill for the new fields in `pkg/providers/vsphere/upgrade/virtualmachine/backfill/compute.go`, per `model.md` "Backfill strategy"
- [ ] T007 Wire `OverwriteSpecComputeConfig` and the class-sync calls into `pkg/providers/vsphere/session/session_vm_update.go` (powered-off, powered-on, and both resize entry points — see Diagrams A-D in `spec.md`)
- [ ] T008 Implement `reconcileComputeConfigSynced` (dry-run diff against live config) in `pkg/providers/vsphere/vmlifecycle/update_status.go`
- [ ] T009 [P] Extend `DefaultWatchedPropertyPaths` in `pkg/util/vsphere/watcher/watcher.go` with every vSphere property this feature reads, so an out-of-band vCenter change triggers a prompt reconcile (see `plan.md` "Repository layout" for the full property list)
- [ ] T010 [P] Add `GetVNUMANodeCount`, `FullMemReservationSpecMet`, and `FirmwareIsEFI` helpers to `pkg/util/vmopv1/vm.go`

## Phase 3 — User Story 1 (DevOps user: setting CPU/memory/flags)

- [ ] T011 [US1] Add webhook validation for the new fields in `webhooks/virtualmachine/validation/virtualmachine_validator_compute.go`, including the `supports_telco_vm_service_api` gate and every cross-field rule listed in `model.md`
- [ ] T012 [P] [US1] Add unit tests per `computeFieldDef` (differs/apply/hotPluggable/prerequisite, across the sentinel matrix) in `pkg/util/vmopv1/compute_overwrite_test.go`
- [ ] T013 [P] [US1] Add unit tests per `syncClass*ToSpec` helper in `pkg/util/vmopv1/resize_class_sync_test.go`
- [ ] T014 [P] [US1] Add unit tests for every webhook validation rule, including the `-1` unlimited sentinel and its interaction with the requests/limits ordering checks, in `webhooks/virtualmachine/validation/virtualmachine_validator_compute_test.go`
- [ ] T015 [US1] Add integration tests for the powered-off/powered-on apply paths in `pkg/providers/vsphere/session/session_vm_update_test.go`
- [ ] T016 [US1] Add E2E coverage for a DevOps user setting compute fields on a VM in `test/e2e/vmservice/vmservice/virtualmachine/vm_compute_config.go`, registered from `test/e2e/vmservice/vmservice_test.go` — see [`e2e.md`](./e2e.md) for the scenario breakdown

## Phase 4 — User Story 2 (DevOps user: diagnosing sync status via condition)

- [ ] T017 [US2] Implement condition reason/message assembly (hwVer gate vs prerequisite vs power-off-required vs mismatch, in that precedence order) in `pkg/providers/vsphere/vmlifecycle/update_status.go`
- [ ] T018 [P] [US2] Add unit/integration tests for condition reason precedence and message content in `pkg/providers/vsphere/vmlifecycle/update_status_test.go`
- [ ] T019 [US2] Extend `vm_compute_config.go` (from T016) so condition reason/message assertions cover every scenario, not just the happy path — see [`e2e.md`](./e2e.md)

## Phase 5 — User Story 3 (DevOps user: class-based resize applies compute intent atomically)

- [ ] T020 [P] [US3] Unit tests per `syncClass*ToSpec` helper are covered by T013; no additional unit-level task needed
- [ ] T021 [US3] Add E2E coverage for the full and narrow class-resize paths (class change + spec overlay applied together; guaranteed↔best-effort reservation clear/restore) in `vm_compute_config.go` — see [`e2e.md`](./e2e.md)

## Phase Final — Polish

- [ ] T022 Update `docs/concepts/workloads/vm.md`'s "Compute Reconfiguration" section (under "Resizing") with the new fields and the `VirtualMachineComputeConfigSynced` condition — one section only; do not leave a second, duplicate "Compute Reconfiguration" heading elsewhere in the file
- [ ] T023 Add release notes (per pull-request-standards.md)
- [ ] T024 Flip `spec.md` status to `Implemented` once every acceptance criterion is covered
