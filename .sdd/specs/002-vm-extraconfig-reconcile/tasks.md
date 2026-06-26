# Tasks: VM ExtraConfig Reconciliation — E2E Coverage

- **E2E plan**: [`e2e.md`](./e2e.md)
- **Epic**: vmop-3782

<!--
TODO: fill in per-task [vmop-NNN] story/sub-task tags once filed under the epic.
spec.md and plan.md for the underlying reconciler are tracked separately and
not required to complete this task list; this file only decomposes the E2E
suite described in e2e.md.
-->

This task list produces the E2E suite specified in `e2e.md`: coverage for
`spec.advanced` ExtraConfig reconciliation — first-class VMX fields, bag key
CRUD, PowerCycle/PowerOff-mode semantics and their condition precedence, and
resilience to VM recreation and out-of-band vSphere drift. It assumes the
`VirtualMachineExtraConfigSynced` condition and its `PowerOffRequired`/
`PowerCyclePending` reasons already exist as reconciler surface to test
against; no product-code task is included here.

## Phase 1 — Setup

- [ ] T001 [P] Add `TelcoVMServiceAPICapabilityName = "supports_telco_vm_service_api"` to `test/e2e/vmservice/consts/consts.go`
- [ ] T002 [P] Add `default/wait-vm-extraconfig-synced` (`5m`/`10s`) interval to `test/e2e/vmservice/config/wcp.yaml`; power-state waits reuse the existing `default/wait-virtual-machine-powerstate` key

## Phase 2 — Foundational

- [ ] T003 Scaffold `VMExtraConfigSpecInput`/`VMExtraConfigSpec` and the `BeforeEach` (capability skip, namespace/class/image lookup, pod-log watch) in `test/e2e/vmservice/vmservice/virtualmachine/vm_extraconfig.go`
- [ ] T004 [P] Add shared helpers in the same file: `buildExtraConfigVM`, `waitForExtraConfigSynced`, `statusExtraConfigValue`, `statusExtraConfigKeys`, `getExtraConfigVM`, `deleteExtraConfigVM`
- [ ] T005 [P] Add out-of-band vSphere helpers in the same file: `waitForBiosUUID`, `findVSphereVMByBiosUUID` (used by Phase 4's out-of-band scenarios)
- [ ] T006 Register `Context("VM-EXTRACONFIG", ...)` calling `virtualmachine.VMExtraConfigSpec` from `test/e2e/vmservice/vmservice_test.go`

## Phase 3 — User Story: applying first-class VMX fields and bag keys, reflected in status

- [ ] T007 [US1] Add scenario: create VM with PowerCycle-mode first-class fields + two bag keys, wait for `ExtraConfigSynced=True`, assert both are visible in `status.extraConfig`, then patch bag keys (add/update/omit) and assert the change lands in `status.extraConfig` (`vm_extraconfig.go`)
- [ ] T008 [P] [US1] Add scenario: create VM with default `PromoteDisksMode=Online` plus first-class fields and a bag key, assert `ExtraConfigSynced=True` and all keys present despite concurrent disk promotion (`vm_extraconfig.go`)

## Phase 4 — User Story: PowerCycle-mode vs PowerOff-mode semantics and condition precedence

- [ ] T009 [US2] Add scenario: flip a PowerCycle-mode field on a running VM, assert `ExtraConfigSynced=False/PowerCyclePending`, power-cycle the VM, assert `True` and the new value in `status.extraConfig` (`vm_extraconfig.go`)
- [ ] T010 [P] [US2] Add scenario: add a PowerOff-mode field to a running VM, assert `ExtraConfigSynced=False/PowerOffRequired` naming the field, assert the key is absent from `status.extraConfig` while deferred, power off, assert `True` and every key present (`vm_extraconfig.go`)
- [ ] T011 [P] [US2] Add scenario: change a PowerOff-mode and a PowerCycle-mode field simultaneously on a running VM, assert `ExtraConfigSynced=False/PowerOffRequired` (not `PowerCyclePending`) and the message names the PowerOff-mode field (`vm_extraconfig.go`)
- [ ] T012 [P] [US2] Add scenario: create VM with `pnumaNodeAffinity` set, assert the comma-separated encoding in `status.extraConfig`; clear the field on a running VM, assert `PowerCyclePending`; power off, assert the key is removed from `status.extraConfig` (`vm_extraconfig.go`)

## Phase 5 — User Story: resilience to VM recreation and out-of-band vSphere drift

- [ ] T013 [US3] Add scenario: power off, delete the VM via the Kubernetes API, recreate with an identical spec, assert `ExtraConfigSynced=True` and every first-class/bag key re-applied (`vm_extraconfig.go`)
- [ ] T014 [P] [US3] Add scenario: create a `PowerCyclePending` condition, power the VM off out-of-band via `govmomi`, assert the pending change applies and `ExtraConfigSynced=True`, assert the operator restores `PoweredOn` afterward with extraConfig intact (`vm_extraconfig.go`)
- [ ] T015 [P] [US3] Add scenario: power off cleanly, destroy the vSphere VM out-of-band via `govmomi` (Kubernetes object survives), set `spec.powerState=PoweredOn`, assert `status.uniqueID` changes (proving recreation), assert `VirtualMachineCreated=True` and `ExtraConfigSynced=True` with every key re-applied (`vm_extraconfig.go`)

## Phase Final — Polish

- [ ] T016 Run every scenario in this file against a real Supervisor at least once; drop the `"experimental"` label per `e2e-testing.md` only after that run passes
- [ ] T017 File a follow-up spec/task for E2E coverage of `pkg/vmconfig/networkextraconfig` (see `e2e.md` "Scope boundaries") — not part of this task list's scope
