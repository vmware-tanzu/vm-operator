# Tasks: NIC ExtraConfig Reconciler — E2E Test Coverage

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Epic**: TBD — file before this branch merges; see `spec.md` header

Tasks marked `[P]` within a phase may run in parallel (no file-level conflicts between them). All tasks touch only `test/e2e/` — this is a test-only change set; do **not** modify `pkg/` or `api/`.

---

## Phase 1 — Setup (no test logic, compilation gate)

Dependencies: none. T001–T004 may run in parallel.

- [ ] T001 [P] Add `TelcoVMServiceAPICapabilityName = "supports_telco_vm_service_api"` constant — `test/e2e/vmservice/consts/consts.go` (skip if already present from a sibling PR)
- [ ] T002 [P] Add NIC-extra-config polling interval keys to the `intervals:` block — `test/e2e/vmservice/config/wcp.yaml`
  ```
  default/wait-vm-nic-extra-config-synced:     ["5m", "10s"]
  default/wait-vm-nic-extra-config-powerstate: ["3m", "10s"]
  ```
- [ ] T003 [P] Create `test/e2e/vmservice/vmservice/virtualmachine/vm_nic_extra_config.go` — package declaration, VMX wire-value constants block, `VMNICExtraConfigSpecInput` struct, `VMNICExtraConfigSpec` function with `BeforeEach` guard only (no `It` blocks yet); file must compile
- [ ] T004 [P] Wire the new spec into `test/e2e/vmservice/vmservice_test.go` — add `Context("VM-NIC-EXTRA-CONFIG", func() { virtualmachine.VMNICExtraConfigSpec(...) })` alongside the existing Context blocks
- [ ] T005 Compilation gate — run `go build ./test/e2e/...` from the repo root; resolve any errors before proceeding to Phase 2

---

## Phase 2 — Shared Helpers

T006–T009 may run in parallel once T005 passes; all touch only `vm_nic_extra_config.go`.

- [ ] T006 [P] Implement VM builder — `nicExtraConfigVMOptions` struct (all fields: `CoalescingScheme`, `CoalescingParams`, `CtxPerDev`, `RSSOffloadEnabled`, `UDPRSSEnabled`, `PNICFeatures`, `UPTv2Enabled`, `VNUMANodeID`, `AdvancedProperties`, `MinHardwareVersion`, `Firmware`, `VNUMANodeCount`, `MemoryReservationLockedToMax`, `NetworkName`, `PowerState`) and `buildNICExtraConfigVM` function — `test/e2e/vmservice/vmservice/virtualmachine/vm_nic_extra_config.go`
- [ ] T007 [P] Implement status accessor helpers — `statusExtraConfigMap`, `statusNICInterface`, `getNICExtraConfigCondition` — `test/e2e/vmservice/vmservice/virtualmachine/vm_nic_extra_config.go`
- [ ] T008 [P] Implement wait helpers — `waitForNICExtraConfigSynced` (condition status + reason + optional lastTransitionTime sentinel), `waitForNICExtraConfigSyncedWithExtraConfig` (single key), `waitForNICExtraConfigSyncedWithExtraConfigs` (map[string]string for multi-key composite), `waitForNICExtraConfigSyncedWithExtraConfigAbsent` — `test/e2e/vmservice/vmservice/virtualmachine/vm_nic_extra_config.go`
- [ ] T009 [P] Implement govmomi helpers — `findVSphereVMByMOIDForNIC` (reuse or thin-wrap `findVSphereVMByMOID` from `vm_compute_config.go` if already in package) and `assertVSphereNICState` (reads first VMXNet3 device, asserts `Uptv2Enabled` and `NumaNode` when non-nil; no-ops when `vimClient` is nil) — `test/e2e/vmservice/vmservice/virtualmachine/vm_nic_extra_config.go`

---

## Phase 3 — Core-Functional `It` Blocks (7.1, 7.2, 7.3)

T010–T012 each create their own VM and are independent; they may run in parallel after Phase 2 helpers compile.

- [ ] T010 [P] Implement It 7.1 — live-mode ExtraConfig for `coalescingScheme` and `coalescingParams`:
  - Phase 1: create VM with `coalescingScheme=Disabled`; composite wait for condition=True AND `ethernet0.coalescingScheme="disabled"`
  - Phase 2: patch to `coalescingScheme=Static` + `coalescingParams="32"`; composite wait for condition=True AND both `ethernet0.coalescingScheme="static"` AND `ethernet0.coalescingParams="32"` via `waitForNICExtraConfigSyncedWithExtraConfigs`
  - Labels: `nic-extra-config`, `core-functional`
  - File: `test/e2e/vmservice/vmservice/virtualmachine/vm_nic_extra_config.go`
- [ ] T011 [P] Implement It 7.2 — powercycle-mode ExtraConfig for all four fields (`ctxPerDev`, `rssOffloadEnabled`, `udpRssEnabled`, `pnicFeatures`), positive and negative wire values across two power cycles:
  - Phase 3: patch all four fields (positive values) → wait for condition=False/PowerCyclePending
  - Phase 4: power cycle → `waitForNICExtraConfigSyncedWithExtraConfigs` asserts `ctxPerDev="3"`, `rssoffload="TRUE"`, `udpRSS="1"`, `pnicfeatures="4"` simultaneously
  - Phase 5: patch `rssOffloadEnabled=false` + `udpRssEnabled=UDPRSSModeDisabled` → power cycle → assert `rssoffload="FALSE"` and `udpRSS="2"` (sentinel guards stale-True read)
  - Labels: `nic-extra-config`, `core-functional`
  - File: `test/e2e/vmservice/vmservice/virtualmachine/vm_nic_extra_config.go`
- [ ] T012 [P] Implement It 7.3 — AdvancedProperties bag lifecycle:
  - Setup: create VM and drive through a power cycle so all powercycle-mode fields are live
  - Step A: add `advancedProperties=[{key:"innerRSS",value:"TRUE"}]`; assert `ethernet0.innerRSS="TRUE"` in status.extraConfig
  - Step B: clear `advancedProperties=[]`; assert `ethernet0.innerRSS` absent from status.extraConfig (GC)
  - Labels: `nic-extra-config`, `core-functional`
  - File: `test/e2e/vmservice/vmservice/virtualmachine/vm_nic_extra_config.go`

---

## Phase 4 — Extended-Functional `It` Blocks (7.4, 7.5)

T013–T014 are independent; they may run in parallel after Phase 3 compiles.

- [ ] T013 [P] Implement It 7.4 — UPTv2Enabled DeviceChange (hot-pluggable, vmx-20+):
  - Step A: VM without `reservationLockedToMax` → patch `uptv2Enabled=true` → assert webhook rejection OR condition=False/PrerequisiteNotMet with message containing "memory reservation"; delete VM
  - Step B: VM with `reservationLockedToMax=true` + `minHardwareVersion=20` → patch `uptv2Enabled=true` → assert condition reason ≠ "PrerequisiteNotMet"; optionally assert govmomi NIC `Uptv2Enabled=true`
  - Labels: `nic-extra-config`, `extended-functional`
  - File: `test/e2e/vmservice/vmservice/virtualmachine/vm_nic_extra_config.go`
- [ ] T014 [P] Implement It 7.5 — VNUMANodeID DeviceChange (poweroff-required, vmx-20+/EFI/vNUMA):
  - Step A: VM with `minHardwareVersion=20` + `vnumaNodeCount=2` but no EFI firmware → patch `vnumaNodeID=1` → assert condition=False/PrerequisiteNotMet with message containing "EFI firmware"; delete VM
  - Step B: VM with `minHardwareVersion=20` + EFI firmware + `vnumaNodeCount=2` → patch `vnumaNodeID=1` while powered on → assert condition=False/PowerOffRequired; power off → assert condition=True; optionally assert govmomi NIC `NumaNode=1`
  - Labels: `nic-extra-config`, `extended-functional`
  - File: `test/e2e/vmservice/vmservice/virtualmachine/vm_nic_extra_config.go`

---

## Phase 5 — Polish

Sequential; run after all Phase 4 tasks are merged.

- [ ] T015 Run `golangci-lint run ./test/e2e/...` from the repo root; fix all lint errors in `vm_nic_extra_config.go` (import grouping, importas aliases, copyright header, comment grammar)
- [ ] T016 Final compilation + vet gate — `go build ./test/e2e/...` and `go vet ./test/e2e/...` pass clean
- [ ] T017 Update `test/e2e/README.md` — add `nic-extra-config` to the feature-label table and note the `TelcoVMServiceAPI` capability guard
- [ ] T018 Update `spec.md` status from `In Progress` to `Implemented` and set `Epic: vmop-NNN` (replace TBD with real ticket number before merging)
