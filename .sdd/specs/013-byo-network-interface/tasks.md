# Tasks: Bring Your Own Network Interface CR

- **Spec**: [spec.md](./spec.md)
- **Plan**: [plan.md](./plan.md)
- **Epic**: vmop-2530

All task tickets below are existing children of the epic. Each task is sized for one PR or less; tasks can share a PR. A task covering shipping behavior includes its corresponding tests. The first usable BYO slice must bundle the E2E work even though the test story is separate. No ticket creation or publication is part of this implementation.

## Phase 1 — Resolve contracts

- [x] [T001] [vmop-2531] User confirmed v1alpha6 is still in development and eligible for `externalRef`; record the resolution in spec, plan, and model.
- [x] [T002] [vmop-2532] User confirmed interface-spec overrides remain allowed, except `macAddr`, which the user later directed cannot be combined with `externalRef`; preserve existing device/bootstrap semantics without modifying external resource specifications.
- [x] [T003] [vmop-2533] Record the user's resolution of Q1 and same-VM reassociation in `.sdd/specs/013-byo-network-interface/spec.md`, `.sdd/specs/013-byo-network-interface/research.md`, `.sdd/specs/013-byo-network-interface/plan.md`, and `.sdd/specs/013-byo-network-interface/model.md`: VM Operator sets VPC association metadata, updates the interface name when a reference moves between names on the same VM, and removes it once the SubnetPort is unreferenced and detached or the VM is deleted, while preserving the CR. Specify `vm-name` label-based release discovery, cross-VM conflict rejection, and update/VM-delete release. Decisions documented on 2026-10-28; implementation remains in T010–T013.

## Phase 2 — API and configuration foundations

The user confirmed v1alpha6 is still in development. T004–T006 should land together so the schema never exposes usable unguarded behavior.

- [x] [T004] [US1] [US4] [vmop-2531] Add `externalRef`, godoc, and structural validation in `api/v1alpha6/virtualmachine_network_types.go`; regenerate `api/v1alpha6/zz_generated.deepcopy.go` and `config/crd/bases/vmoperator.vmware.com_virtualmachines.yaml`. Cover AC2–3 and AC10–11.
- [x] [T005] [US4] [vmop-2531] Restore `externalRef` by interface name in `api/v1alpha2/virtualmachine_conversion.go`, `api/v1alpha3/virtualmachine_conversion.go`, `api/v1alpha4/virtualmachine_conversion.go`, and `api/v1alpha5/virtualmachine_conversion.go`; extend the corresponding `api/test/v1alpha2/virtualmachine_conversion_test.go` through `api/test/v1alpha5/virtualmachine_conversion_test.go`. v1alpha1 is unchanged. Regenerate affected conversion files with the repository generator. Conversion never fails; conflicting legacy edits are rejected in T008. Cover AC10. Depends on T004.
- [x] [T006] [US4] [vmop-2531] Add `FeatureStates.BringYourOwnNetworkInterface` and `CapabilityKeyBringYourOwnNetworkInterface` (`supports_VM_service_byo_network_interface`) in `pkg/config/config.go` and `pkg/config/capabilities/capabilities.go`; test absent, inactive, active, and removed capability values in `pkg/config/capabilities/capabilities_test.go`. Add capability-disabled field and dependent-CEL removal in `pkg/crd/crd.go`, with schema and API-server coverage in `pkg/crd/crd_test.go` for first installation and both `CRDCleanupEnabled` update settings. Keep runtime admission protection in `webhooks/virtualmachine/validation/virtualmachine_validator.go` for retained schemas. Cover AC11; no FSS switch or VM-object stripping helper. Depends on T004.

## Phase 3 — Admission and event delivery

- [x] [T007] [US1] [vmop-2532] Skip network defaulting for entries carrying an external reference on create/update in `webhooks/virtualmachine/mutation/virtualmachine_mutator.go` (the default-network lookup still happens); add coverage in the existing `webhooks/virtualmachine/mutation/virtualmachine_mutator_unit_test.go`. Cover AC2 with BYO-only, mixed, invalid, and gate-off requests through a retained schema. Confirm ordinary defaulting for objects with no external reference after API-server pruning; schema removal is tested in T006. Depends on T004–T006.
- [x] [T008] [US1] [US4] [US5] [vmop-2532] In `validateNetworkInterfaceExternalRefs` in `webhooks/virtualmachine/validation/virtualmachine_validator.go`, validate the apiVersion/kind against namespace-supported providers, duplicate references, `network`/`macAddr` exclusivity, managed/external transitions on an existing interface, and capability-off unchanged references through retained schemas; add `macAddr` exclusivity CEL in `api/v1alpha6/virtualmachine_network_types.go`. Add coverage in the existing `webhooks/virtualmachine/validation/virtualmachine_validator_unit_test.go`. Cover AC3, AC10–11, including older-client field loss. Depends on T005–T007.
- [x] [T009] [US3] [vmop-3636] Add index/key/mapper helpers in `pkg/util/kube/network_interface.go`, tests in `pkg/util/kube/network_interface_test.go`, and feature-gated registration/combined owner-reference event routing in `controllers/virtualmachine/virtualmachine/virtualmachine_controller.go`. The mapper enqueues owner VMs, VMs referencing the CR via the field index, and the VM named by the `vm-name` label (for references removed from the spec but pending release). Add an external-interface watch case in the existing `controllers/virtualmachine/virtualmachine/virtualmachine_controller_intg_test.go`. Cover AC7–8: indexer output, referencing VM, same-name different kind, owner-and-reference de-duplication, `vm-name` label mapping, and reconciliation when the referenced CR changes. Depends on T003–T006; complete before enabling no-requeue BYO waits.

## Phase 4 — Provider, mapping, and lifecycle

- [x] [T010] [US1] [US3] [US5] [vmop-2533] Add typed external lookup and metadata-only VPC association helpers in `pkg/providers/vsphere/network/external.go`; wire both entry paths and provider dispatch in `pkg/providers/vsphere/network/network.go`; extend `pkg/providers/vsphere/network/network_test.go`. Cover AC1, AC6–7, AC9, AC11: not-found, VM-owned rejection, association with another VM rejected, association set before readiness without owner reference or spec writes, device built after readiness, and gate-off refusal of attachments. Terminating-resource rejection and same-VM rename for AC13 (interface-name label/annotation update) are implemented in `external.go` without a dedicated test. Depends on T003 and T008–T009.
- [x] [T011] [US1] [US3] [vmop-2533] Resolve external references during device matching in `pkg/providers/vsphere/network/devices.go` using the referenced CR's provider identity; existing bootstrap handling is reused unchanged. Cover AC1 and AC9 in `pkg/providers/vsphere/network/network_test.go`. BYO adds no unit-number handling. Depends on T010 and the T002 field contract.
- [x] [T012] [US2] [vmop-2533] Keep the existing owned-only orphan deletion unchanged. Add `ReleaseExternalSubnetPorts` in `pkg/providers/vsphere/network/external.go`, which lists SubnetPorts by the `vm-name` label and releases unowned ones not referenced by spec whose attachment ID is empty or matches no current NIC; call it before network reconciliation in `pkg/providers/vsphere/session/session_vm_update.go`. Extend `pkg/providers/vsphere/network/network_test.go`. Cover AC4–6: no BYO deletion despite VM labels, retention while attached, last-interface/nil/disabled spec, retryable cleanup, and managed cleanup retained. Depends on T010–T011.
- [x] [T013] [US2] [US3] [vmop-2533] Call `ReleaseAllExternalSubnetPorts` in `pkg/providers/vsphere/vmprovider_vm.go` after VM destruction and on the already-absent VM path; propagate cleanup failures before provider success/finalizer removal. No pre-create release. Release-all behavior is covered in `pkg/providers/vsphere/network/network_test.go`. Keep controller changes limited to delegation. Depends on T009–T012.

## Phase 5 — E2E and release evidence

T014–T015 ship in the same PR as the first usable behavior from Phases 3–4. They are not a deferred test-only follow-up.

- [x] [T014] [US1] [US2] [vmop-2585] Add external-resource setup/assertion helpers in `test/e2e/vmservice/vmservice/virtualmachine/vm_networking.go`; add `BringYourOwnNetworkInterfaceCapabilityName` (`supports_VM_service_byo_network_interface`) in `test/e2e/vmservice/consts/consts.go`. Skip unless the BYO and mutable-networks capabilities are enabled. Depends on T001–T003 and T010–T013.
- [x] [T015] [US1] [US2] [vmop-2585] Add one `experimental` test in the "External network interfaces" Context of `test/e2e/vmservice/vmservice/virtualmachine/vm_networking.go`: clone the VM's managed interface spec into an external CR, add it as `eth1` to a powered-off VM, check two NICs, no owner reference, and VPC association; remove it, check one NIC and release; delete the VM and check the CR still exists. Covers parts of AC1, AC4–5. The full vmop-2585 three-interface workflow, rename/swap, restart, and dependency-recovery E2E scenarios are not covered. Depends on T014.

## Phase 6 — Documentation and completion

- [x] [T016] [US4] [US5] [vmop-2531] Add usage and rollout guidance to `docs/concepts/services-networking/guest-net-config.md`; regenerate the API reference with `make generate-api-docs`; update `.sdd/specs/013-byo-network-interface/model.md` to the implemented API. Include capability discovery, field/CEL removal and CRD cleanup behavior, unknown-field pruning/defaulting, same-namespace scope, provider contract, `macAddr` exclusion, older-client limitations, same-VM rename, powered-off hardware changes, and draining before capability disablement/schema removal/downgrade. Depends on T005–T015.
- [ ] [T017] [US4] [vmop-2585] Add E2E coverage for the full vmop-2585 mixed three-interface workflow and the remaining gaps noted in T015. Run the plan's focused checks and provider E2E matrix; record exact versions, feature combinations, passes and skips in `.sdd/specs/013-byo-network-interface/research.md`. Remove `experimental` only for validated scenarios in `test/e2e/vmservice/vmservice/virtualmachine/vm_networking.go`. Update `.sdd/specs/013-byo-network-interface/spec.md`, `.sdd/specs/013-byo-network-interface/tasks.md`, and `.sdd/INDEX.md` only when the corresponding acceptance criteria are complete. Depends on T015–T016.

## Acceptance coverage

| Criteria | Tasks |
|---|---|
| AC1 | T010–T011, T015 |
| AC2 | T004, T007 |
| AC3 | T004, T008 |
| AC4–5 | T012–T013, T015 |
| AC6 | T010, T012–T013 |
| AC7–8 | T009–T010 |
| AC9 | T010–T011 |
| AC10 | T005, T008 |
| AC11 | T004, T006–T008, T010, T013, T016 |
| AC12 | T015, T017 |
| AC13 | T010, T016 |

## Implementation validation status — 2026-10-28

T004–T016 have implementations and local coverage in this change, as rescoped by the user's simplification. Checkboxes record implementation delivery, not real-provider certification. T017 remains open: the single new E2E scenario is experimental pending a real Supervisor matrix, the full vmop-2585 workflow is not yet covered, and full codegen verification is blocked by the configured npm registry returning HTTP 403. See [research.md](./research.md) for exact local evidence and limitations. No release enablement is asserted.
