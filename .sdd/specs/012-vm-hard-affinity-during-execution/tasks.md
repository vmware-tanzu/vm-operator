# Tasks: VM Hard Affinity During Execution

- **Spec**: [spec.md](./spec.md) · **Plan**: [plan.md](./plan.md)
- **Epic**: vmop-3332

Format: `[T###] [P?] [USx?] [vmop-NNN?] Description (path/to/file)`

## Phase 1 — Setup

- [x] [T001] Capability `supports_vm_service_vm_hard_affinity_during_execution` sets `Features.TaggingAPI` and `Features.VMHardAffinityDuringExecution` (`pkg/config/capabilities/capabilities.go`, `pkg/config/config.go`, `pkg/config/default.go`)
- [x] [T002] Register `RequiredDuringExecutionVMPlacementPolicy` CRD type (`external/vsphere-policy/api/v1alpha1/requiredduringexecutionvmplacementpolicy_types.go`, `pkg/crd/crd.go`)

## Phase 2 — candidateVSphereZone (G3)

- [x] [T003] [US4] PR 2000 — Pass `CandidateVsphereZone` per VM in `PlaceVmsXCluster` when the feature is enabled (`pkg/providers/vsphere/placement/cluster_placement.go`, `pkg/providers/vsphere/placement/group_placement.go`)

## Phase 3 — RequiredDuringExecution and host anti-affinity grouping (G1, G2)

- [ ] [T004] [US1, US3] PR 2012 — Generate RequiredDuringExecution host policies; use `VmToVmGroupsAntiAffinity` for host anti-affinity when the feature is enabled, `VmVmAntiAffinity` otherwise (`pkg/providers/vsphere/virtualmachine/affinity.go`, `pkg/providers/vsphere/virtualmachine/configspec_test.go`)
- [ ] [T005] [US1] PR 2012 — Webhook entitlement check and RBAC for `RequiredDuringExecutionVMPlacementPolicy`; E2E that provisions the entitlement by assigning an InfraPolicy of kind `infrapolicy.PolicyKindRequiredDuringExecutionVMPlacementPolicy` to the test namespace (`webhooks/virtualmachine/validation/virtualmachine_validator.go`, `webhooks/virtualmachine/validation/virtualmachine_validator_unit_test.go`, `config/rbac/role.yaml`, `test/e2e/vmservice/vmservice/virtualmachine/vm_group.go`)
- [ ] [T006] [US2] [vmop-1] Tests for entitlement revocation: existing VM with RequiredDuringExecution terms admits non-affinity updates, new VM is denied; unit + E2E for AC2.1–AC2.3 (`webhooks/virtualmachine/validation/virtualmachine_validator_unit_test.go`, `test/e2e/vmservice/vmservice/virtualmachine/vm_group.go`)
- [ ] [T007] [US1] [vmop-2] Allow `topology.kubernetes.io/zone` for RequiredDuringExecution terms and emit zone policies with `RequiredDuringPlacementRequiredDuringExecution` strictness; unit + E2E for AC1.2 (`webhooks/virtualmachine/validation/virtualmachine_validator.go`, `pkg/providers/vsphere/virtualmachine/affinity.go`, `pkg/providers/vsphere/virtualmachine/configspec_test.go`, `test/e2e/vmservice/vmservice/virtualmachine/vm_group.go`)

## Phase 4 — Persist zonal policies at create (G4)

- [ ] [T008] [US5] [vmop-3] Keep `ConfigureZoneRules` on create when `VMHardAffinityDuringExecution` is enabled; unit + E2E for AC5.1/AC5.2 (`pkg/providers/vsphere/virtualmachine/configspec.go`, `pkg/providers/vsphere/virtualmachine/configspec_test.go`, `test/e2e/vmservice/vmservice/virtualmachine/vm_group.go`)

## Phase 5 — Group-less affinity (G5)

- [ ] [T009] [US6] [vmop-4] Make `spec.groupName` optional for all affinity types when the feature is enabled (`webhooks/virtualmachine/validation/virtualmachine_validator.go`, `webhooks/virtualmachine/validation/virtualmachine_validator_unit_test.go`)
- [ ] [T010] [US6] [vmop-5] Ensure group-less VMs with affinity are placed via `PlaceVmsXCluster` with their placement policies (`pkg/providers/vsphere/vmprovider_vm.go`, `pkg/providers/vsphere/placement/`)
- [ ] [T011] [P] [US6] [vmop-6] E2E for AC6.1/AC6.2 (`test/e2e/vmservice/vmservice/virtualmachine/vm_group.go`)

## Phase 6 — Placement status (US8)

- [ ] [T012] [US8] [vmop-7] **Blocked** on `[NEEDS CLARIFICATION]` in spec US8 — Surface placement policy status on VMs (paths TBD)

## Phase 7 — Test coverage

- [ ] [T015] [P] [vmop-10] [Todo] vcsim coverage for the strictness × topology × tag × cluster-layout matrix (paths TBD)
- [ ] [T016] [vmop-11] Bump govmomi to the v0.57 release when tagged (`go.mod`, `go.sum`)

## Phase Final — Polish

- [ ] [T013] [vmop-8] Fill in "Best practices for mixing placement policies and tags" and user docs for RequiredDuringExecution entitlement (`.sdd/specs/012-vm-hard-affinity-during-execution/spec.md`, `docs/`)
