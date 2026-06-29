# Tasks: NIC ExtraConfig Reconciler — E2E Coverage

- **E2E plan**: [`e2e.md`](./e2e.md)
- **Epic**: vmop-3782

<!--
TODO: fill in per-task [vmop-NNN] story/sub-task tags once filed under the epic.
spec.md and plan.md for the underlying networkextraconfig reconciler are
tracked separately and not required to complete this task list; this file
only decomposes the E2E suite described in e2e.md.
-->

This task list produces the E2E suite specified in `e2e.md`: coverage for
per-NIC ExtraConfig and device-spec reconciliation — live-mode and
PowerCycle-mode ExtraConfig fields, AdvancedProperties bag CRUD, and the
DeviceChange prerequisite gates for `uptv2Enabled` and `vnumaNodeID`. It
assumes the `VirtualMachineNetworkConfigSynced` condition and its
`PrerequisiteNotMet`/`PowerOffRequired`/`PowerCyclePending` reasons already
exist as reconciler surface (`pkg/vmconfig/networkextraconfig/`) to test
against; no product-code task is included here.

## Phase 1 — Setup

- [ ] T001 [P] Add `TelcoVMServiceAPICapabilityName = "supports_telco_vm_service_api"` to `test/e2e/vmservice/consts/consts.go` (skip if already added by a sibling PR for the `spec.advanced` VM-level ExtraConfig suite)
- [ ] T002 [P] Add `default/wait-vm-nic-extra-config-synced` (`5m`/`10s`) interval to `test/e2e/vmservice/config/wcp.yaml`; power-state waits reuse the existing `default/wait-virtual-machine-powerstate` interval

## Phase 2 — Foundational

- [ ] T003 Scaffold `VMNICExtraConfigSpecInput`/`VMNICExtraConfigSpec` and the `BeforeEach` (infra check, capability skip, namespace/class/image lookup, vim client) in `test/e2e/vmservice/vmservice/virtualmachine/vm_nic_extra_config.go`
- [ ] T004 [P] Add the VM builder in the same file: `nicExtraConfigVMOptions` (live-mode, PowerCycle-mode, DeviceChange, AdvancedProperties, and VM-level prerequisite fields) and `buildNICExtraConfigVM`
- [ ] T005 [P] Add status/condition accessors in the same file: `statusExtraConfigMap`, `statusNICInterface`, `getNICExtraConfigCondition`
- [ ] T006 [P] Add wait helpers in the same file: `waitForNICExtraConfigSynced` (status/reason, with an optional `LastTransitionTime` sentinel to guard against reading a stale `True`), `waitForNICExtraConfigSyncedWithExtraConfig`, `waitForNICExtraConfigSyncedWithExtraConfigs` (multi-key composite guard — required whenever more than one wire value must be asserted atomically), `waitForNICExtraConfigSyncedWithExtraConfigAbsent`
- [ ] T007 [P] Add `deleteNICExtraConfigVM` and `retryUpdateVMA6` (get-mutate-update retried on any error, including `409 Conflict` races against the reconciler) in the same file
- [ ] T008 Register `Context("VM-NIC-EXTRA-CONFIG", ...)` calling `virtualmachine.VMNICExtraConfigSpec` from `test/e2e/vmservice/vmservice_test.go`

## Phase 3 — User Story: live-mode ExtraConfig applies immediately, independent of power state and concurrent disk promotion

- [ ] T009 [US1] Add scenario: create VM with `coalescingScheme=Disabled`, wait for `NetworkConfigSynced=True` with `ethernet0.coalescingScheme=disabled`, then patch `coalescingScheme=Static`+`coalescingParams=32`, assert both land in `status.extraConfig` via the multi-key composite wait (`vm_nic_extra_config.go`)
- [ ] T010 [P] [US1] Add scenario: create VM with `promoteDisksMode=Online` and a live-mode field, patch the field, assert `NetworkConfigSynced=True` and the field present despite concurrent disk promotion, and assert `spec.promoteDisksMode` is unchanged afterward (`vm_nic_extra_config.go`)

## Phase 4 — User Story: PowerCycle-mode fields defer to `PowerCyclePending`, wire values encode correctly in both directions

- [ ] T011 [US2] Add scenario: patch all four PowerCycle-mode fields to positive values on a running VM, assert `NetworkConfigSynced=False/PowerCyclePending`, power-cycle, assert all four wire values (`ctxPerDev="3"`, `rssoffload="TRUE"`, `udpRSS="1"`, `pnicFeatures="4"`) simultaneously via `waitForNICExtraConfigSyncedWithExtraConfigs`; then flip `rssOffloadEnabled=false`+`udpRssEnabled=Disabled`, assert `PowerCyclePending` again (using the first power-cycle's `LastTransitionTime` as a sentinel), power-cycle again, and assert the negative-branch wire values (`rssoffload="FALSE"`, `udpRSS="2"`) while `ctxPerDev`/`pnicFeatures` remain unchanged (`vm_nic_extra_config.go`)

## Phase 5 — User Story: AdvancedProperties bag keys are added and garbage-collected

- [ ] T012 [US3] Add scenario: create a VM with baseline PowerCycle-mode fields already live, add `advancedProperties=[{key:"innerRSS",value:"TRUE"}]`, assert `ethernet0.innerRSS=TRUE` in `status.extraConfig`; then clear `advancedProperties` to empty and assert `ethernet0.innerRSS` is absent from `status.extraConfig` via `waitForNICExtraConfigSyncedWithExtraConfigAbsent` (`vm_nic_extra_config.go`)

## Phase 6 — User Story: DeviceChange fields are gated by prerequisites and, when met, apply per their power-state mode

- [ ] T013 [US4] Add scenario: patch `uptv2Enabled=true` on a VM without `memoryAdvanced.reservationLockedToMax`, accept either a webhook rejection or `NetworkConfigSynced=False/PrerequisiteNotMet` as the outcome, delete the VM; then create a second VM with `reservationLockedToMax=true`+`minHardwareVersion=20`, patch `uptv2Enabled=true`, and assert the condition reason is never `PrerequisiteNotMet` (`vm_nic_extra_config.go`)
- [ ] T014 [P] [US4] Add scenario: patch `vnumaNodeID=1` on a VM with `minHardwareVersion=20`+`vnumaNodeCount=2` but no EFI firmware, assert `NetworkConfigSynced=False/PrerequisiteNotMet` and assert the condition `Message` contains `"EFI firmware required"`, delete the VM; then create a second VM with `minHardwareVersion=20`+`bootOptions.firmware=EFI`+`cpuAdvanced.topology.vnumaNodeCount=2`+`cpuAdvanced.topology.coresPerSocket=1`, patch `vnumaNodeID=1` while powered on, assert `PowerOffRequired`, power off, assert `NetworkConfigSynced=True`, and independently verify via govmomi that the `VirtualVmxnet3` device's `NumaNode` equals `1` (`vm_nic_extra_config.go`)

## Phase Final — Polish

- [ ] T015 Add `Label("experimental", ...)` to every `It` block in `vm_nic_extra_config.go` per `e2e-testing.md` — this suite does not carry it today
- [ ] T016 Run every scenario in this file against a real Supervisor at least once; drop the `"experimental"` label only after that run passes
- [ ] T017 File a follow-up spec/task for resilience coverage (VM recreation, out-of-band vSphere drift) for `pkg/vmconfig/networkextraconfig/`, mirroring the `spec.advanced` VM-level ExtraConfig suite's resilience scenarios (see `e2e.md` "Scope boundaries") — not part of this task list's scope
