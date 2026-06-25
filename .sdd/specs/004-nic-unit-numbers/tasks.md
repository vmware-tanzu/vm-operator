# Tasks: NIC Unit Numbers

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Epic**: vmop-3982

---

## Phase 1 — Research & Setup

- [ ] T001 [P] Write and run govmomi research program (`cmd/vmoperator-vc-research/main.go`) to validate vSphere NIC unit-number behaviour: **primary question — create a VM whose initial `VirtualMachineConfigSpec` includes NICs with explicit `VirtualDevice.UnitNumber` values and verify the system honours them** (explicit spec values are set on the device at create time; the mutation webhook does not assign); repeat for a post-create `ReconfigVM_Task` Add; add NICs without UnitNumber and confirm the expected 7-based assignment (first NIC → 7, subsequent → next available in 7–16); add/remove NICs while powered on and powered off; **hot-add a NIC with an explicit `UnitNumber` to a powered-on VM and verify the slot is honoured** (the validation webhook allows adding a new interface with an explicit value while powered on); attempt an `Edit` config spec to change a NIC's UnitNumber on a powered-off VM; **add an SR-IOV ethernet card (`VirtualSriovEthernetCard`) and confirm it occupies the same 7–16 unit-number space** as other NIC types; **add a NIC whose explicit `UnitNumber` collides with an existing device and record the exact fault returned** (vcsim returns `InvalidDeviceSpec`; the real fault type drives the reconciler's permanent-error vs. retry handling); inspect `moVM.Config.Hardware.Device[i].GetVirtualDevice().UnitNumber` after each operation. **Scope note:** the field of concern is `VirtualDevice.UnitNumber`, NOT the PCI slot number (`VirtualDevice.SlotInfo` / `VirtualDevicePciBusSlotInfo.PciSlotNumber`) — recording `SlotInfo` alongside each observation is fine/useful, but no product behaviour depends on it. Document all findings in `research.md` and resolve all `[NEEDS CLARIFICATION]` items in `spec.md`. **This gates T018, T019, and T029 (whether the Add/Edit/create payloads carry `UnitNumber`).**

- [ ] T002 [P] Add feature flag `VMNetworkUnitNumbers bool` to the `FeatureStates` struct in `pkg/config/config.go` (with inline comment matching the pattern of `VMSharedDisks`); **NOTE: `VMSharedDisks` is capability-driven, not FSS env-var driven** — decide whether `VMNetworkUnitNumbers` is an FSS env var (add constant to `pkg/config/env/env.go` and a `setBool` line in `pkg/config/env.go`) or a capability (add a capability key constant and switch case in `pkg/config/capabilities/capabilities.go`); document the decision in `plan.md`

- [ ] T003 [P] Add `FeatureVersionNICUnitNumbers` to the `1 << iota` block in `pkg/util/vmopv1/features.go` (value 16); update `FeatureVersionAll` to OR in the new bit (15 → 31); add the new bit to the `FeatureVersions()` slice; add the new bit to `ActivatedFeatureVersion(ctx)` when `VMNetworkUnitNumbers` is enabled; update `features_test.go` in `pkg/util/vmopv1/`

---

## Phase 2 — API & Foundational

- [ ] T004 [vmop-TBD] Add `UnitNumber *int32` to `VirtualMachineNetworkInterfaceSpec` in `api/v1alpha6/virtualmachine_network_types.go` — place after the `VNUMANodeID` field; add kubebuilder markers (`+optional`, `+kubebuilder:validation:Minimum=7`, `+kubebuilder:validation:Maximum=16` — NICs occupy PCI slots 7–16; the lower slots belong to other virtual devices) and full godoc; run `make generate-go` to regenerate `zz_generated.deepcopy.go` and `make generate-manifests` to regenerate CRD YAML under `config/crd/`

- [ ] T005 [vmop-TBD] Add `UnitNumber *int32` to `VirtualMachineNetworkInterfaceStatus` in `api/v1alpha6/virtualmachine_network_types.go` — place after `DeviceKey`; add `+optional` marker and full godoc; regenerate deepcopy (`make generate-go`)

- [ ] T006 [vmop-TBD] Add `NICUnitNumberFirst = 7` / `NICUnitNumberMax = 16` constants (location: `pkg/util/vmopv1/` or the validation package) backing the CRD range markers and the Go validation range check. **No `NICBusSpec` / `NextAvailableUnitNumber` slot-assignment helper is needed** — the mutation webhook does not assign unit numbers; unset values come from vSphere via the backfill. If the CRD `Minimum`/`Maximum` markers are deemed sufficient, drop the Go-side range check and these constants entirely

- [ ] T028 [vmop-TBD] Extend `restore_v1alpha6_VirtualMachineNetworkInterfaces` in **each of** `api/v1alpha2/virtualmachine_conversion.go`, `api/v1alpha3/virtualmachine_conversion.go`, `api/v1alpha4/virtualmachine_conversion.go`, and `api/v1alpha5/virtualmachine_conversion.go` to restore the new `UnitNumber` spec field via the annotation-based restore mechanism (exactly as done for `VNUMANodeID` — each version has its own copy of the function); extend each version's conversion fuzz tests to cover the field round-tripping; without this, an UPDATE submitted through that version wipes the field (k8s#111703). `v1alpha1` restores the interfaces list wholesale when non-empty, so it needs no per-field change — add a fuzz-test assertion that the field survives the v1alpha1 round-trip. The status field needs no per-field handling (`dst.Status = restored.Status`)

- [ ] T007 [vmop-TBD] Add `UnitNumber *int32` field to `NetworkInterfaceResult` struct in `pkg/providers/vsphere/network/network.go`; populate it in `reconcileNetworkInterfaces` in `pkg/providers/vsphere/session/session_vm_update.go` — in the indexed loop that iterates over results alongside `networkSpec.Interfaces`, copy `interfaceSpec.UnitNumber` into `result.UnitNumber` when the `VMNetworkUnitNumbers` flag is on; **also** add `UnitNumber *int32` to the `Device` struct (same file) and populate it in `CreateNetworkDevices` from the spec interface — the VM create path consumes `Device`, not `NetworkInterfaceResult` (see T029); add unit tests to `network_test.go` in `pkg/providers/vsphere/network/`

---

## Phase 3 — Mutation Webhook

> **Phase removed by design decision**: the mutation webhook does NOT assign NIC unit numbers. `unitNumber` is optional — explicit user values are carried onto the device `ConfigSpec` at create/add time (T018, T029), and unset values are assigned by vSphere and recorded into the spec by the schema-upgrade backfill (T014/T015). This makes vSphere the single source of derived values and eliminates the webhook-vs-backfill ordering race an auto-assigning webhook would create.

- [x] T008 — **REMOVED**: no `AssignNICUnitNumbers` mutation function (see phase note above)

- [x] T009 — **REMOVED**: no mutation-webhook registration (see phase note above)

- [x] T010 — **REMOVED**: no mutation-webhook tests (see phase note above)

---

## Phase 4 — Validation Webhook

- [ ] T011 [vmop-TBD] Create `webhooks/virtualmachine/validation/virtualmachine_validator_network_interfaces.go` with `validateNICUnitNumbers` method on `validator` (package `validation`): if `VMNetworkUnitNumbers` flag is off, reject any interface that sets `UnitNumber` with `field.Forbidden(path, featureNotEnabled)` (same pattern as `validateNetworkVLANs` — do NOT silently ignore, or stale/duplicate values poison the spec-wins backfill later); range check each interface's `UnitNumber` (7–16, use `field.Invalid` with `invalidNICUnitNumberRangeFmt`); uniqueness check using `occupiedSlots sets.Set[int32]` (use `field.Invalid` with `invalidNICUnitNumberInUse`) — consider a CEL `XValidation` on the interfaces list for the uniqueness rule per the constitution's CEL-first preference; on update when `oldVM.Spec.PowerState == vmopv1.VirtualMachinePowerStateOn`, compare each interface's `UnitNumber` against `oldVM`'s same-named interface using a `map[string]*int32` lookup — reject with `field.Forbidden(path, updatesNotAllowedWhenPowerOn)` when an already-set value changes to a different value or is cleared (set → nil); nil → set MUST be allowed, since the schema-upgrade backfill writes observed values into the spec of running VMs and must pass this webhook; define new error-message constants at the top of the file

- [ ] T012 [vmop-TBD] Wire `validateNICUnitNumbers` into `validateNetwork` in `webhooks/virtualmachine/validation/virtualmachine_validator.go` — call it after the per-interface loop and before the `validateNetworkVLANs` call; pass `vm`, `oldVM`, and a `field.Path` for `spec.network.interfaces`

- [ ] T013 [vmop-TBD] Create `webhooks/virtualmachine/validation/virtualmachine_validator_network_interfaces_test.go` (package `validation_test`) following the `virtualmachine_validator_hardware_controllers_test.go` model: define `func nicUnitNumberTests()` with `Label(testlabels.Create, testlabels.Update, testlabels.Validation, testlabels.Webhook)`; cover: valid single, valid multiple, duplicate rejected, out-of-range rejected (both below 7 and above 16), powered-on change of an already-set value blocked, powered-on set → nil (cleared) blocked, powered-on nil → set allowed (backfill write path), new NIC added while powered-on allowed (UnitNumber change on existing NIC is what's blocked), flag off with field set → `field.Forbidden` featureNotEnabled, flag off with no field set → no error; register in `virtualmachine_validator_suite_test.go`; **also** ensure the validation suite's `pkgcfg.SetContext` enables `VMNetworkUnitNumbers = true`

---

## Phase 5 — Schema Upgrade (Brownfield Backfill)

- [ ] T014 [vmop-TBD] Implement standalone `NICUnitNumbersFromMoVM` function in `pkg/providers/vsphere/upgrade/virtualmachine/backfill/nic.go`: iterate `spec.network.interfaces`; **prefer** the existing hodgepodge matching (MAC / ExternalID / backing, same criteria as `FindMatchingEthCard` / `MapEthernetDevicesToSpecIdx`) to identify each interface's VC ethernet device and, when `iface.UnitNumber` is nil (spec wins), set `iface.UnitNumber = &dev.GetVirtualDevice().UnitNumber`; after the hodgepodge pass, **fall back to a positional zip** of the remaining unmatched spec interfaces against the remaining unclaimed VC ethernet devices so every interface receives its observed unit number (log zip-matched interfaces at V(4); the zip is strictly a last resort for interfaces the hodgepodge could not uniquely claim — decision confirmed 2026-07-27); the backfill runs even when `spec.network.disabled` is true (no devices → nothing recorded, bit still stamped — decision 2026-07-27); spec wins unconditionally with no mismatch guard — when an explicit spec value disagrees with the observed slot, leave it and log the disagreement at V(4) (decision 2026-07-27); update `nic_test.go` in `pkg/providers/vsphere/upgrade/virtualmachine/backfill/` with test cases covering: unique hodgepodge match backfilled, ambiguous devices zip-backfilled, skipped when already set (spec wins), explicit value disagreeing with observed slot left untouched, network disabled (no devices, no writes), mixed unique+zip, more spec interfaces than devices, more devices than spec interfaces

  > **Decision (confirmed)**: the unit-number backfill is a standalone schema-upgrade step with its own `FeatureVersionNICUnitNumbers` bit and its own flag/capability gate. It must NOT live inside `NICConfigFromMoVM` under the `TelcoVMServiceAPI` gate: that gate is one-shot, so any VM already stamped with the Telco bit would never be backfilled, and `TelcoVMServiceAPI` may be disabled while `VMNetworkUnitNumbers` is enabled.

- [ ] T015 [vmop-TBD] Add the `FeatureVersionNICUnitNumbers` gate to `ReconcileSchemaUpgrade` in `pkg/providers/vsphere/upgrade/virtualmachine/vm_schema_upgrade.go`, guarded on `features.VMNetworkUnitNumbers`, calling `backfill.NICUnitNumbersFromMoVM(ctx, vm, moVM)` and stamping the bit (same shape as the existing `TelcoVMServiceAPI` gate). This single mechanism serves **both** greenfield VMs (first post-create reconcile, once the vSphere devices exist — this is how unset unit numbers reach the spec, since the mutation webhook does not assign) and brownfield VMs (first reconcile after operator upgrade). Add vcsim integration test coverage for both flows (VM with NICs and no spec unit numbers → schema upgrade runs → spec carries observed values, annotation carries the bit; explicit spec values untouched; network-disabled VM → no writes, bit still stamped) — **this is the only place the backfill flow is tested end-to-end, since E2E cannot flip the flag/capability mid-run (see Phase 8)**. vcsim viability confirmed: it assigns NIC unit numbers starting at 7, honours explicit `UnitNumber` on Add, and rejects duplicates with `InvalidDeviceSpec`

- [ ] T016 [vmop-TBD] Update `NICConfigFromMoVM` in `pkg/providers/vsphere/upgrade/virtualmachine/backfill/nic.go` to resolve the `TODO(BV)` positional zip: when all spec interfaces have `UnitNumber` non-nil, replace the positional `for i := range vm.Spec.Network.Interfaces` zip with a unit-number-keyed lookup (build a `map[int32]int` from `VirtualDevice.UnitNumber` to device index; for each spec interface look up its `UnitNumber` in the map); fall back to positional zip only when no interface has a `UnitNumber`; add test cases to `nic_test.go`

- [ ] T031 [vmop-TBD] Investigate and cover the snapshot-revert / failover / register-VM interplay (open question — flagged 2026-07-27): `restoreVMSpecFromSnapshot` in `pkg/providers/vsphere/vmprovider_vmsnapshot.go` restores the VM spec **and annotations** (including the feature-version annotation) from the backup yaml stored with the snapshot, so backfilled unit numbers and the `FeatureVersionNICUnitNumbers` bit travel with the snapshot, while the imported-VM fallback (`synthesizeVMSpecForSnapshot`) synthesizes interfaces with **no** unit numbers; the `FailedOverVMAnnotation` path skips interface immutability so vendors can re-point networks. Verify with vcsim integration tests: (a) revert to a pre-backfill snapshot drops the bit and the backfill re-runs and converges; (b) the imported-VM fallback leaves unit numbers nil and the backfill re-records them post-revert; (c) a failed-over/registered brownfield VM is backfilled on its first reconcile. Document findings in `research.md` and resolve the snapshot-revert `[NEEDS CLARIFICATION]` in `spec.md`

---

## Phase 6 — Reconcile Matching

**Dependency on T001**: T018 and T019 decisions (whether to set `UnitNumber` on Add/Edit payload) depend on govmomi research results from T001.

- [ ] T017 [vmop-TBD] Update `FindMatchingEthCard` in `pkg/providers/vsphere/network/reconcile.go`: add a `unitNumber *int32` parameter; when `unitNumber` is non-nil and the `VMNetworkUnitNumbers` flag is on, perform a unit-number-first scan of `currentEthCards` — iterate and compare `curCard.GetVirtualDevice().UnitNumber == *unitNumber`; return that index immediately if found; fall through to the existing MAC/ExternalId/backing logic when unit-number scan finds no match; update **both** call sites to pass `r.UnitNumber`: `ReconcileNetworkInterfaces` (reconcile.go) and `fixupMacAddressMutableNetworks` (`pkg/providers/vsphere/session/session_vm_update.go`, the post-reconfigure MAC fixup for results without a `DeviceKey`). **`ReconcileNetworkInterfaces` must claim in two passes**: first match and claim devices for all results carrying a `UnitNumber`, then run the fallback matching for the remaining results against the remaining unclaimed cards — otherwise an earlier result without a unit number can claim (via backing match) the device whose slot a later result explicitly requests. **A unit-number match identifies the device but does not mean "no change"**: after the match, `ReconcileNetworkInterfaces` must diff the matched device against the desired state and emit `VirtualDeviceConfigSpec` entries to converge — an Edit for backing/property changes on the same device type, or (device type differs, e.g. spec E1000 vs actual vmxnet3, since type cannot be edited in place) a Remove of the matched device plus an Add of the desired device carrying the same `UnitNumber`; add unit tests to `reconcile_test.go` in `pkg/providers/vsphere/network/` covering: unit-number match with identical device (no change), unit-number match with changed backing (Edit), unit-number match with changed device type (Remove+Add at same slot), unit-number match skipped when flag off, unit-number miss falls through to backing match, mixed results where an un-numbered result's backing match would collide with a later result's explicit unit number (two-pass claim prevents the mis-claim), fixup path (`fixupMacAddressMutableNetworks`) matches by unit number

- [ ] T018 [vmop-TBD] Update the Add path in `ReconcileNetworkInterfaces` in `pkg/providers/vsphere/network/reconcile.go`: when `r.UnitNumber` is non-nil and T001 confirms vSphere honours explicit `UnitNumber` on Add operations, set `r.Device.GetVirtualDevice().UnitNumber = *r.UnitNumber` before appending the `Add` config spec; if T001 shows vSphere ignores the field, do NOT ship spec-driven placement (there is no automatic spec/status convergence — the backfill is one-shot and spec-wins; the design must be revisited per the open question in `spec.md`); add unit tests

- [ ] T029 [vmop-TBD] Update the VM **create** path in `pkg/providers/vsphere/vmprovider_vm.go` — **both branches** of the NIC-device construction loop: (a) the class-ConfigSpec branch, where existing class ethernet devices are positionally zipped to `createArgs.NetworkDevices` and mutated via `network.ApplyNetworkDeviceToVirtualEthCard` — when the flag is on and the spec interface carries an explicit `UnitNumber` (via `Device.UnitNumber`, T007), set it on the class device, **overriding any class-ConfigSpec-provided unit number** (spec wins); when the spec has no value, leave a class-provided unit number as-is (vSphere honours or reassigns; the backfill records the observed value); and (b) the fallback branch building devices via `network.CreateDefaultEthCardFromNetworkDevice` — set the explicit `UnitNumber` on the device before it is appended to the create ConfigSpec. Both gated on T001 confirming vSphere honours explicit `UnitNumber` on create. Interfaces without an explicit value are created without a spec-driven unit number (vSphere assigns; the post-create backfill records the observed value — T015). Surface a clear error when vSphere rejects the create ConfigSpec due to a unit-number collision (e.g. spec value vs. class device — the webhook cannot see the class ConfigSpec); add unit tests covering both branches, the class-override case, and the class-value-preserved case

- [ ] T019 [vmop-TBD] Update the orphaned-CR Edit path in `ReconcileNetworkInterfaces` in `pkg/providers/vsphere/network/reconcile.go`: when `r.UnitNumber` is non-nil and T001 confirms vSphere supports unit-number changes on Edit for powered-off VMs, also set `editDev.GetVirtualDevice().UnitNumber = *r.UnitNumber`; otherwise log a warning at V(4) and skip; add unit tests

- [ ] T020 [vmop-TBD] Update `MapEthernetDevicesToSpecIdx` in `pkg/providers/vsphere/network/devices.go` to add a unit-number-first pass for both the mutable and immutable branches: build a `unitNumber → index` map from `ethCards` (using `dev.GetVirtualDevice().UnitNumber`); iterate spec interfaces — for each with `UnitNumber` non-nil, look it up in the map and claim the card; remove claimed cards from the pool; for remaining unmatched interfaces fall through to the existing per-provider CR-based lookup (mutable) or positional zip (immutable); gate the unit-number pass on `VMNetworkUnitNumbers` flag; add unit tests to `devices_test.go` in `pkg/providers/vsphere/network/` covering: unit-number match (mutable path), unit-number match (immutable path), partial match falls through to CR path, flag off uses original logic. **NOTE:** this function has a second caller besides the status path — the boot-options reconciler (`pkg/vmconfig/bootoptions/bootoptions_reconciler.go`) uses it to map boot-order devices; verify/extend the boot-options tests for the new mapping behaviour

---

## Phase 7 — Status

- [ ] T021 [vmop-TBD] Add `NetworkDeviceKeysToUnitNumber map[int32]int32` field to `ReconcileStatusData` in `pkg/providers/vsphere/vmlifecycle/update_status.go`; in `vmprovider_vm.go` (the caller of `MapEthernetDevicesToSpecIdx` that constructs `ReconcileStatusData`), build this map alongside the existing `NetworkDeviceKeysToSpecIdx` map by iterating `moVM.Config.Hardware.Device`, filtering ethernet cards, and storing `deviceKey → VirtualDevice.UnitNumber`; thread the new map into `ReconcileStatusData`; in `updateGuestNetworkStatus`, use the map to populate `status.network.interfaces[i].unitNumber` — for each interface status entry, look up its `DeviceKey` in `NetworkDeviceKeysToUnitNumber` and set `UnitNumber` if found; **gate population on the `VMNetworkUnitNumbers` flag** — when the flag is off, do not build the map and do not write the status field (decision 2026-07-27); add unit tests to `update_status_test.go` (including flag-off → field absent). **NOTE:** interface status entries are built from `guestInfo.Net` (VMware Tools); Tools-less VMs have no interface status entries at all, so `status.unitNumber` only appears for Tools-reported interfaces — this is accepted and documented in `plan.md`

---

## Phase 8 — E2E Tests

**NOTE on E2E file location**: The correct path is `test/e2e/vmservice/vmservice/virtualmachine/` (double `vmservice`). E2E specs are named `vm_<feature>.go` and use a `func VMXxxSpec(ctx, inputGetter)` pattern, registered in `vmservice_test.go` via a `Context("VM-NIC-UNIT-NUMBERS", ...)` block next to `Context("VM-NETWORKING", ...)`. Tests use `skipper.SkipUnlessInfraIs(...)` for infra gating. **Labels are applied inside the spec file on each `It`** (e.g. `vm_networking.go:132,224` apply `Label("smoke")` directly) — every new/changed `It` here also needs `Label("experimental")` per `e2e-testing.md` until validated on real hardware, in addition to its functional label (`smoke` / `core-functional` / `extended-functional`).

- [ ] T022 [vmop-TBD] Create `test/e2e/vmservice/vmservice/virtualmachine/vm_nic_unit_numbers.go` (package `virtualmachine`) with `func VMNICUnitNumbersSpec(ctx context.Context, inputGetter func() VMNICUnitNumbersSpecInput)` following the `vm_networking.go` pattern. Gate the whole spec (or the individual `It`s that need it) on the `VMNetworkUnitNumbers` capability/FSS check — exact constant TBD per T002, add it to `test/e2e/vmservice/consts/consts.go` following the `TelcoVMServiceAPICapabilityName` naming pattern once T002 lands; do not inline the capability string in the spec file. Cover:
  1. **Backfill on create, no explicit values** — create a VM with `spec.network.interfaces` unset `unitNumber` on every interface; assert the spec is unchanged at admission (no webhook assignment); after the first reconcile, assert `spec.network.interfaces[i].unitNumber` holds vSphere-assigned values (range 7–16, unique across interfaces); after `WaitForVirtualMachineIP` (so Tools has reported and status interface entries exist), assert `status.network.interfaces[i].unitNumber` matches the observed `VirtualDevice.UnitNumber` for each interface.
  2. **Explicit placement on create** *(gated on T001 confirming vSphere honours explicit `UnitNumber` on Add)* — create a VM with one interface `unitNumber: 9`; assert the device lands at slot 9, the spec value is unchanged post-reconcile, and status matches.
  3. **Mixed explicit/unset, two-pass matching** *(gated on T001)* — create a VM with 2+ interfaces where one carries an explicit `unitNumber` (e.g. `16`) and at least one other is unset; assert the explicit interface claims slot 16 and the unset interface's fallback match does not steal it (spec MUST #19's two-pass rule), and the unset interface's backfilled value differs and is unique.
  4. **Add NIC to a running VM** — append an interface with no explicit `unitNumber` to a running VM's spec; assert the new device gets a unique vSphere-assigned slot recorded in status, and the pre-existing NICs' spec/status unit numbers are unchanged by the add.
  5. **Remove NIC, verify by device Key, not count** — capture each NIC's `ethCards[i].Key` and unit number before removing one interface from spec; after reconcile, assert the removed device's `Key` is gone (not just that the count decreased) and the surviving NIC(s) retain their original `Key` and unit number in both spec and status. (This also answers the open question of whether vSphere reuses the freed slot on a subsequent add — record the observed behaviour in `research.md`.)
  6. **Steady-state reconcile stability (no churn)** — after unit numbers are settled (backfilled or explicit), capture `(Key, UnitNumber)` pairs for all NICs and `Consistently` (~30–60s, spanning at least one more reconcile) assert they are unchanged — guards the platform-engineer AC that matching by `unitNumber` avoids spurious remove+add cycles.
  7. **Power-cycle stability** — power off then on a VM with settled unit numbers; assert `spec.network.interfaces[i].unitNumber` and `status.network.interfaces[i].unitNumber` are unchanged after the cycle (resolves the spec.md open question on power-cycle stability).
  8. **Conversion round-trip through an older API version** — after a VM has a `unitNumber` set (explicit or backfilled), submit an UPDATE using the `v1alpha2` manifest shape (`GetVirtualMachineYamlA2`, as `vm_networking.go:145` already does) that omits the field; re-read the object as `v1alpha6` and assert `spec.network.interfaces[i].unitNumber` survived the round-trip via the annotation-based restore (spec MUST #22). This is the only place the real API-server conversion chain is exercised end-to-end — the conversion fuzz tests (T028) only exercise the conversion functions directly.

  > **No E2E admission-validation scenarios**: duplicate `unitNumber`, out-of-range `unitNumber` (both bounds), powered-on change-of-an-already-set-value rejection, and powered-on nil→set allowance are pure webhook logic with no vSphere/cluster dependency — they are exhaustively covered by the `nicUnitNumberTests()` unit tests in T013 (`virtualmachine_validator_network_interfaces_test.go`), which already lists all of these cases. E2E would only be re-testing the same in-process validator code through a slower path, so they are intentionally left out of T022.

  > **No E2E brownfield scenario**: the `VMNetworkUnitNumbers` flag/capability is deployment-level and cannot be toggled mid-run on a shared supervisor cluster. The brownfield backfill flow is covered by vcsim integration tests in T015 instead.

  > **No E2E snapshot-revert scenario**: the annotation/feature-version-bit interplay on revert (T031) requires the same kind of state control E2E cannot do reliably on a shared cluster; it is intentionally covered by vcsim integration tests in T031 instead, following the same reasoning as the brownfield exclusion above.

---

## Phase Final — Polish

- [ ] T023 Update `research.md` with all findings from the govmomi program (T001); confirm or revise the Add/Edit unit-number payload decisions in `plan.md`; resolve all `[NEEDS CLARIFICATION]` items in `spec.md`

- [ ] T024 Update `plan.md` "Rollout / migration" section once the FSS vs. capability decision (T002) is finalised

- [ ] T025 Add release note entry to PR description describing `spec.network.interfaces[i].unitNumber`, `status.network.interfaces[i].unitNumber`, and the opt-in `VMNetworkUnitNumbers` feature flag

- [ ] T026 Flip `spec.md` status from `In Progress` to `Implemented` in the final PR

- [ ] T027 File a follow-up spec `.sdd/specs/003-nic-unit-numbers-ga/` to track GA promotion and flag removal once the feature has been validated in production

- [ ] T030 Update user-facing documentation under `docs/` describing `spec.network.interfaces[i].unitNumber` (optional; range 7–16; explicit values honoured at device creation; unset values assigned by vSphere and recorded into the spec after creation; immutable while powered on) and the `status.network.interfaces[i].unitNumber` observed field

---

## Dependency graph

```
T001 (research) ─────────────────────────────────► T018, T019, T029 (Add/Edit/create payload decisions); gates T022 scenarios 2, 3
T002 (feature flag) ─────────────────────────────► all gated tasks
T003 (FeatureVersion) ───────────────────────────► T015 (schema-upgrade gate)
T004 (spec API field) ───────────────────────────► T011 (validation), T014 (backfill), T028 (conversion restore), T029 (create path)
T005 (status API field) ─────────────────────────► T021 (status)
T006 (range constants) ──────────────────────────► T011 (validation range check)
T007 (NetworkInterfaceResult.UnitNumber) ────────► T017 (FindMatchingEthCard param), T018, T019
T011 (validateNICUnitNumbers) ───────────────────► T012 (wire into validateNetwork), T013 (tests)
T014 (NICUnitNumbersFromMoVM) ───────────────────► T015 (schema-upgrade gate + intg tests), T016 (Telco zip upgrade), T031 (snapshot/failover coverage)
T014/T015/T016 (backfill) ───────────────────────► unit numbers present on spec for T017–T020
T017 (FindMatchingEthCard) ──────────────────────► T018, T019 (updated call site)
T020 (MapEthernetDevicesToSpecIdx) ──────────────► T021 (status needs correct DeviceKey map); also affects bootoptions caller
T011–T021, T028, T029 ───────────────────────────► T022 (E2E tests)

(T008–T010 removed: no mutation-webhook assignment.)
```

---

## Files changed per task (quick reference)

| Task | Files changed |
|------|---------------|
| T001 | `cmd/vmoperator-vc-research/main.go` (new), `research.md`, `spec.md` |
| T002 | `pkg/config/config.go`, `pkg/config/env/env.go` or `pkg/config/capabilities/capabilities.go` |
| T003 | `pkg/util/vmopv1/features.go`, `pkg/util/vmopv1/features_test.go` |
| T004 | `api/v1alpha6/virtualmachine_network_types.go`, `api/v1alpha6/zz_generated.deepcopy.go`, `config/crd/` |
| T005 | `api/v1alpha6/virtualmachine_network_types.go`, `api/v1alpha6/zz_generated.deepcopy.go` |
| T006 | `pkg/util/vmopv1/hardware.go` (range constants only, or dropped in favour of CRD markers) |
| T007 | `pkg/providers/vsphere/network/network.go`, `pkg/providers/vsphere/session/session_vm_update.go`, `pkg/providers/vsphere/network/network_test.go` |
| T008–T010 | — (removed: no mutation-webhook assignment) |
| T011 | `webhooks/virtualmachine/validation/virtualmachine_validator_network_interfaces.go` (new) |
| T012 | `webhooks/virtualmachine/validation/virtualmachine_validator.go` |
| T013 | `webhooks/virtualmachine/validation/virtualmachine_validator_network_interfaces_test.go` (new), `webhooks/virtualmachine/validation/virtualmachine_validator_suite_test.go` |
| T014 | `pkg/providers/vsphere/upgrade/virtualmachine/backfill/nic.go`, `pkg/providers/vsphere/upgrade/virtualmachine/backfill/nic_test.go` |
| T015 | `pkg/providers/vsphere/upgrade/virtualmachine/vm_schema_upgrade.go`, `pkg/providers/vsphere/upgrade/virtualmachine/vm_schema_upgrade_test.go` |
| T016 | `pkg/providers/vsphere/upgrade/virtualmachine/backfill/nic.go`, `pkg/providers/vsphere/upgrade/virtualmachine/backfill/nic_test.go` |
| T017 | `pkg/providers/vsphere/network/reconcile.go`, `pkg/providers/vsphere/session/session_vm_update.go` (fixup call site), `pkg/providers/vsphere/network/reconcile_test.go` |
| T018 | `pkg/providers/vsphere/network/reconcile.go`, `pkg/providers/vsphere/network/reconcile_test.go` |
| T019 | `pkg/providers/vsphere/network/reconcile.go`, `pkg/providers/vsphere/network/reconcile_test.go` |
| T020 | `pkg/providers/vsphere/network/devices.go`, `pkg/providers/vsphere/network/devices_test.go` |
| T021 | `pkg/providers/vsphere/vmlifecycle/update_status.go`, `pkg/providers/vsphere/vmprovider_vm.go`, `pkg/providers/vsphere/vmlifecycle/update_status_test.go` |
| T022 | `test/e2e/vmservice/vmservice/virtualmachine/vm_nic_unit_numbers.go` (new), `test/e2e/vmservice/vmservice_test.go` (register `VM-NIC-UNIT-NUMBERS` context), `test/e2e/vmservice/consts/consts.go` (capability constant, once T002 lands) |
| T028 | `api/v1alpha{2,3,4,5}/virtualmachine_conversion.go`, conversion fuzz tests (incl. v1alpha1 assertion) |
| T029 | `pkg/providers/vsphere/vmprovider_vm.go`, `pkg/providers/vsphere/network/network.go` (`ApplyNetworkDeviceToVirtualEthCard`), `pkg/providers/vsphere/vmprovider_vm_test.go` |
| T030 | `docs/` |
| T031 | vcsim integration tests (snapshot revert / failover / register flows), `research.md`, `spec.md` |
| T023–T027 | `.sdd/specs/004-nic-unit-numbers/*.md` |
