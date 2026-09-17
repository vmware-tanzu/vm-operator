# Research: NIC Unit Numbers

## vSphere NIC placement model

In vSphere, all `VirtualEthernetCard` devices (vmxnet3, e1000, SR-IOV, etc.) sit on a single implicit `VirtualPCIBus` (device key 100, always present). The `VirtualDevice.UnitNumber` field identifies the NIC's slot on that bus. vSphere allows at most 10 NICs per VM.

**The PCI bus is shared with other virtual devices** (video card, VMCI device, storage controllers, …), which occupy the lower slots. vSphere assigns ethernet cards unit numbers starting at **7**, so valid NIC unit numbers are **7–16**, not 0–9. Any slot-assignment logic must treat slots 0–6 as reserved for non-NIC devices, and the CRD range markers, webhook assignment base, and validation messages must all use the 7–16 range.

Unlike storage controllers (SCSI/SATA/NVME/IDE), which are explicit API objects in `spec.hardware`, the PCI bus for NICs has no representation in the vm-operator API. NICs are managed as a flat list in `spec.network.interfaces` (max 10, enforced by `+kubebuilder:validation:MaxItems=10` kubebuilder marker).

## Existing NIC matching (the hodgepodge)

There are two distinct matching functions, each serving a different purpose:

### `FindMatchingEthCard` — `pkg/providers/vsphere/network/reconcile.go`

Called during reconfigure (`ReconfigVM_Task`) to decide whether a desired NIC already exists on the VC VM (match → no change), needs editing (orphaned-CR path), or needs to be added (no match). Matching criteria:

1. **MAC address** — only checked when `AddressType == Manual`. If the desired MAC is `Generated`, MAC is ignored entirely for matching.
2. **ExternalId** — checked only if the desired device's `ExternalId` is non-empty. Must be an exact string match.
3. **Backing** — always checked (the primary discriminator). Type-dispatched:
   - `VirtualEthernetCardNetworkBackingInfo` → matches on `DeviceName`
   - `VirtualEthernetCardDistributedVirtualPortBackingInfo` → matches on `SwitchUuid` AND `PortgroupKey`
   - `VirtualEthernetCardOpaqueNetworkBackingInfo` → matches on `OpaqueNetworkId` AND `OpaqueNetworkType`

`UnitNumber` is never consulted. The desired device (constructed in `CreateDefaultEthCard`) never has a `UnitNumber` set.

**`FindMatchingEthCard` has TWO call sites**: the reconcile loop in `ReconcileNetworkInterfaces` (reconcile.go), and the post-reconfigure MAC-address fixup `fixupMacAddressMutableNetworks` in `session_vm_update.go` (re-matches results without a `DeviceKey` against the VM's live network devices). Both must pass the unit number when the parameter is added.

### `MapEthernetDevicesToSpecIdx` — `pkg/providers/vsphere/network/devices.go`

Called after reconcile to build a `DeviceKey → SpecIndex` map for status population. Two modes:

- **Immutable networks** (`MutableNetworks` flag off): simple positional zip — `ethCards[i]` → spec index `i`. No semantic matching.
- **Mutable networks**: per-provider Kubernetes CR lookup:
  - VDS (NetOP): fetches `NetworkInterface` CR; matches on `CR.Status.ExternalID` and `CR.Status.NetworkID` (portgroup key), optionally MAC.
  - NSXT (NCP): fetches `VirtualNetworkInterface` CR; requires both `ExternalId == CR.Status.InterfaceID` AND `MacAddress == CR.Status.MacAddress`.
  - VPC: fetches `SubnetPort` CR; matches on `CR.Status.Attachment.ID == ExternalId`, optionally MAC.
  - Named (test-only): backing device name, optionally MAC.

`UnitNumber` is never consulted here either.

### The orphaned-CR path — `findExistingEthCardForOrphanedCR`

When `FindMatchingEthCard` fails (backing changed, e.g. network migration), the reconciler checks whether there is an "orphaned" network CR with the same interface name whose backing still matches one of the VM's current ethernet cards. If so, an `Edit` config spec is emitted instead of Remove+Add, which preserves the device type and implicitly preserves the vSphere-assigned `UnitNumber`. This path is only active for MutableNetworks on powered-off VMs.

### `defaultNICMatcher` — the fourth matcher, in `pkg/vmconfig/networkextraconfig`

`defaultNICMatcher` (`nic_matcher.go:29-44`) is an **unconditional positional zip** of `spec.network.interfaces` against `collectManagedEthernetDevices(moVM.Config.Hardware.Device)`, wrapped in a `NICDeviceMatcher` func type (`:20`) that is clearly meant as an injection point. It is consulted twice per reconcile — `reconciler.go:111,122` (config path) and `:251,256` (status path) — and unlike the other three matchers it **writes to devices**:

- per-NIC ExtraConfig overlay (`reconciler.go:137`);
- per-device field Edits — `NumaNode`, `Uptv2Enabled` — through `reconcileNICFields` → `findOrCreateDeviceEdit` (`nic_fields.go:123-125`, `:206-224`);
- per-interface status — `vnumaNodeID`, `vmxnet3` — through `updateInterfaceStatus` (`reconciler.go:330-375`), which locates its status entry **by interface name**, making it a third consumer of the G13 empty-name miss.

The whole reconciler is gated on `TelcoVMServiceAPI` (`reconciler.go:98-100`), which does **not** make it a two-flag corner case: this spec's own argument for a standalone feature-version bit is that Telco and `VMNetworkUnitNumbers` are independent.

Its ConfigSpec is shared with the ethernet device changes, which is what makes the Remove/Edit collision reachable: `getConfigSpecForPoweredOffVM` appends `ethCardDeviceChanges` (`session_vm_update.go:740-744`), `poweredOffReconfigure` passes that same spec into `doReconfigure` (`:473-509`), and `doReconfigure` runs `reconcileNetworkExtraConfig` against it (`:1563-1575`). The resize path repeats the shape at `:1023-1027`. `findOrCreateDeviceEdit` dedupes only against existing **Edit** entries, never checking for a Remove.

## The disk and CD-ROM placement contract — what NICs copy, and what they cannot

Disks already have an end-to-end unit-number story here, and it is worth recording in full because three of its four pieces transfer and the fourth does not.

1. **Backfill skips on disagreement, leaving nils.** `needsPlacementBackfill` (`vm_schema_upgrade.go:669`) touches a volume only when a placement field is missing; `hasPlacementMismatch` (`:676`) then abandons the write for that volume entirely — all three fields — if any already-set field disagrees with observed hardware. CD-ROMs mirror it at `:794-818`.
2. **The mutator fills the nils later.** `AddControllersForVolumes` classifies a nil-unit-number volume as implicit placement (`virtualmachine_mutator_hardware_controllers.go:61-71`) and assigns a slot on the next update past the `IsObjectUpgraded` gate — **including the backfill's own patch**, since the mutating webhook has no privileged-account bypass (`virtualmachine_mutator.go:255-420`; the package's only account check is `:692`) and that patch carries the annotation stamp.
3. **Identity lives elsewhere.** PVC name → disk UUID in the backfill (`pvcNameToDiskUUID`), disk UUID → volume name in the hardware check (`update_status_hardware_validation.go:324`). The unit number is requested placement, never a lookup key.
4. **Divergence is reported, not converged.** `checkVolumes` diffs expected-from-spec against actual-from-hardware and `reconcileHardwareCondition` marks `VirtualMachineHardwareDeviceConfigVerified` false with `VirtualMachineHardwareDeviceConfigMismatchReason` (`:289`, `:196-228`).

NICs adopt 1, 2 and 4 unchanged (G7, G8). **Piece 3 is what G11 overturns**, and it is the piece that makes 1 and 2 safe for disks: a wrong unit number on a disk is inert metadata that produces a false condition, while on a NIC it is the identity itself, so the reconciler acts on it and can replace the device. The divergence is deliberate — see `plan.md` "Consistency with the disk placement model".

## Key structural insight: `NetworkInterfaceResult` has no `UnitNumber`

The `NetworkInterfaceResult` struct (network.go) that flows through the entire reconcile pipeline has no `UnitNumber` field. The desired `VirtualEthernetCard` device is constructed without a unit number. This means adding unit-number matching and placement to `FindMatchingEthCard` and the Add config spec requires either:

- Extending `NetworkInterfaceResult` with a `UnitNumber *int32` field populated from `interfaceSpec.UnitNumber` in `reconcileNetworkInterfaces`, OR
- Passing `unitNumber` as a separate parameter to `FindMatchingEthCard`.

Extending `NetworkInterfaceResult` is preferred: it flows the unit number through naturally and lets `ReconcileNetworkInterfaces` set it on the Add device payload without needing to pass extra parameters everywhere.

## Disk unit number prior art — the pattern NICs follow

`pkg/util/vmopv1/hardware.go` defines the `ControllerSpec` interface and the `NextAvailableUnitNumber` helper used by the disk, CD-ROM, and controller mutation webhooks to assign slots. **NICs follow the same pattern.** The pieces, and where to copy each from:

1. **Two-phase assignment** (`virtualmachine_mutator_hardware_controllers.go:200-215`): insert every explicitly-specified unit number into `occupiedSlots` first, then call `NextAvailableUnitNumber` for the entries that have none. An explicit value is never overwritten — it only reserves its slot.
2. **The upgrade gate** (`virtualmachine_mutator_cdrom.go:50`): `vmopv1util.IsObjectUpgraded(ctx, vm)` — skip the mutation entirely, with a log line, until the object's build, schema, and feature versions are all current. This is what prevents the webhook from assigning slots before the backfill has recorded what the VM's hardware actually uses. It gates on the **new** object, so assignment resumes on the very request that carries the schema upgrade's annotation stamp.
   **These mutations are update-only, and that is a consequence of the gate, not an independent choice.** The upgrade annotations are written by `ReconcileSchemaUpgrade` during the VM's reconcile (`vm_schema_upgrade.go:100-112`) — nothing in the admission path sets them — so a VM being created can never satisfy `IsObjectUpgraded`. `MutateCdromControllerOnUpdate` therefore returns early on `oldVM == nil`, and the volume/controller mutation block sits under `case admissionv1.Update`. For NICs the same holds: a VM is created with whatever the user submitted, the backfill supplies values on the first post-create reconcile, and the mutator numbers interfaces added after that.
3. **The mirrored gate on validation** (`virtualmachine_validator_hardware_controllers.go:60`, and `:186` for the powered-on/hot-add rules). The comment at `:180-186` explains the asymmetry worth copying: validation of hot-add rules checks **`oldVM`** "because the patch on schema upgrade will contain the annotation along with any backfilled controllers" — judging that patch by the post-upgrade rules would reject the backfill itself.
4. **No slot available** → the mutator logs and leaves the field unset (`skippedNoSlotMessage`); the validating webhook is what turns that into a user-visible error.
5. **Validation rules** carried over from `virtualmachine_validator_hardware_controllers.go`: range check, uniqueness via `occupiedSlots`, immutability while powered on, and a feature-gate check (`validateNetworkVLANs` is the precedent for rejecting rather than silently ignoring a flag-gated field).

**One helper change is needed.** `NextAvailableUnitNumber` scans `0..MaxSlots()-1` and skips a single `ReservedUnitNumber()`. Ethernet cards start at 7, which cannot be expressed as one reserved unit, so `ControllerSpec` gains a first-usable-unit notion — 0 for the existing controllers, 7 for the NIC bus. A `NICBusSpec` then implements `ControllerSpec` for the single implicit PCI bus. Forking a NIC-local copy of the helper was rejected: sharing the helper is the point.

**Why webhook assignment needs the upgrade gate.** Without it, a webhook-guessed slot written before the backfill would be locked in by spec-wins and could make unit-number-first matching claim the wrong device. That hazard is not NIC-specific — it is exactly what `IsObjectUpgraded` exists for, and disks, CD-ROMs and controllers all rely on it. Note also that dropping assignment entirely would not be free: because the backfill is one-shot, every NIC added after a VM was stamped would keep a nil `unitNumber` forever, permanently preserving the un-numbered fallback path and the positional zip.

**What genuinely differs for NICs, and does not change the conclusion:** for disks the operator chooses the unit number and vSphere honours it, whereas for NICs "does vSphere honour an explicit `UnitNumber` on Add" is still open (spec.md **Q1**). Assigning to every interface means every Add requests a slot, so a "not honoured" answer costs more. That raises the stakes on Q1 — which already gates the create and add paths — rather than arguing for a different assignment model.

## Schema upgrade prior art

`FeatureVersionVMSharedDisks` (bit 2) in `vm_schema_upgrade.go` is the closest prior art. It reads `moVM.Config.Hardware.Device`, identifies disk controllers and volumes by device type and key, and backfills `spec.hardware.*Controllers` and per-volume `{controllerType, controllerBusNumber, unitNumber}`. The NIC unit number backfill follows the identical structure in `pkg/providers/vsphere/upgrade/virtualmachine/backfill/nic.go`.

The existing `NICConfigFromMoVM` function (TelcoVMServiceAPI gate) already uses `collectEthernetDevicesFromMoVM`. The new `NICUnitNumbersFromMoVM` backfill **must** run as a separate feature-version step (`FeatureVersionNICUnitNumbers`), not inside `NICConfigFromMoVM`, because:

- **The Telco gate is one-shot.** `ReconcileSchemaUpgrade` runs `NICConfigFromMoVM` only when the VM's feature-version annotation lacks `FeatureVersionTelcoVMServiceAPI`, then stamps the bit. Any VM already stamped with the Telco bit would never re-run it, so a backfill living there would never execute for those VMs. Only a new bit re-triggers the upgrade pass for already-upgraded VMs (`ActivatedFeatureVersion` grows, `IsOrSuperset` fails, the pass re-runs).
- `TelcoVMServiceAPI` may be disabled while `VMNetworkUnitNumbers` is enabled; the two flags are orthogonal.
- The TelcoVMServiceAPI step uses positional zip (with a `TODO(BV)` for real matching).
- The unit-number backfill must match spec interfaces to VC devices using the existing hodgepodge (since `unitNumber` is not yet on the spec during the first pass), then write the observed `VirtualDevice.UnitNumber` back. For devices the hodgepodge cannot uniquely match, it falls back to positional zip so every interface still receives its observed value.
- After the unit-number backfill completes, the `NICConfigFromMoVM` positional zip can be upgraded to use unit-number-based matching (resolves the `TODO(BV)`).

## FeatureVersion bitmask

Current bits (order is immutable once in production):

| Bit | Value | Name |
|-----|-------|------|
| 1   | 1     | `FeatureVersionBase` |
| 2   | 2     | `FeatureVersionVMSharedDisks` |
| 3   | 4     | `FeatureVersionAllDisksArePVCs` |
| 4   | 8     | `FeatureVersionTelcoVMServiceAPI` |

New bit: `FeatureVersionNICUnitNumbers = 16` (bit 5, produced automatically by the `1 << iota` block). Three places in `pkg/util/vmopv1/features.go` must be updated together: the constant itself, `FeatureVersionAll` (currently 15), and the `FeatureVersions()` slice. `ActivatedFeatureVersion` must OR the bit in when `VMNetworkUnitNumbers` is enabled.

## Additional findings from code review

- **Mutation-webhook assignment, gated on `IsObjectUpgraded`** — see "Disk unit number prior art" above. The gate sequences the backfill ahead of any webhook assignment for a given VM, which is the same ordering guarantee the disk path relies on.
- **VM create path is separate from reconcile — and has two branches.** The initial VM `ConfigSpec` builds NIC devices in `pkg/providers/vsphere/vmprovider_vm.go`, not via `ReconcileNetworkInterfaces`, and it consumes `network.Device` (from `CreateNetworkDevices`), not `NetworkInterfaceResult` — so `Device` also needs a `UnitNumber` field. Branch (a): ethernet devices already present in the **VM class ConfigSpec** are positionally zipped to `createArgs.NetworkDevices` and mutated via `ApplyNetworkDeviceToVirtualEthCard`; these devices may already carry a `UnitNumber` from the class ConfigSpec — an explicit spec value overrides it (spec wins), an unset spec leaves it as-is. Branch (b): remaining spec interfaces get default devices via `CreateDefaultEthCardFromNetworkDevice`. Both branches must set the explicit spec value (gated on the T001 result), otherwise greenfield VMs start life with a spec/device mismatch. The validation webhook cannot see the class ConfigSpec, so a spec-vs-class unit-number collision only surfaces as a vSphere create fault.
- **Conversion restore is required in every older version.** This repo preserves hub-only spec fields across old-version round-trips via the annotation-based restore mechanism, not automatically. **Each of `v1alpha2`, `v1alpha3`, `v1alpha4`, and `v1alpha5` has its own `restore_v1alpha6_VirtualMachineNetworkInterfaces`** (each restores `VNUMANodeID` today) and all four must be extended for `UnitNumber`. `v1alpha1` restores the interfaces list wholesale when the down-converted list is non-empty, so it is incidentally covered — assert it in fuzz tests. Without these, an UPDATE submitted through an older version wipes the field — exactly the k8s#111703 hazard the constitution warns about.
- **vcsim models NIC unit numbers faithfully enough for integration tests** (verified in the govmomi v0.55.0-alpha simulator source): `AssignController` assigns ethernet-card unit numbers with a PCI offset of 7; an explicit `UnitNumber` on an Add is honoured (auto-assignment only happens when nil); and a duplicate unit number on the same controller is rejected with `InvalidDeviceSpec`. The backfill, placement, and collision scenarios in T015 are therefore exercisable under vcsim. Real-VC behaviour still needs T001 confirmation.
- **Snapshot revert restores spec + annotations from the backup yaml** (`restoreVMSpecFromSnapshot` in `vmprovider_vmsnapshot.go`): backfilled unit numbers and the feature-version annotation travel with the snapshot, so reverting to a pre-backfill snapshot drops the `FeatureVersionNICUnitNumbers` bit and re-triggers the backfill. The imported-VM fallback (`synthesizeVMSpecForSnapshot`) synthesizes interfaces with no unit numbers. Covered by T031.
- **`MapEthernetDevicesToSpecIdx` has a second caller.** Besides the status path in `vmprovider_vm.go`, `pkg/vmconfig/bootoptions/bootoptions_reconciler.go` calls it to map boot-order devices. The unit-number-first pass changes behaviour for both callers; boot-option tests should cover the new mapping.
- **Status depends on VMware Tools.** `updateGuestNetworkStatus` builds `status.network.interfaces` entries by iterating `guestInfo.Net`; a VM without Tools (or before Tools starts) has no interface status entries at all, even though unit numbers are available in `moVM.Config.Hardware`. The `unitNumber` status field therefore only appears for Tools-reported interfaces.
- **Immutable-networks interface comparison is per-field.** When `MutableNetworks` is off, the validator compares only `Name`, `Network`, and `MACAddr` per interface (not `DeepEqual`), so a mutation-webhook-assigned `unitNumber` on update does not trip the interfaces-immutable check.

## govmomi research program (T001) — results

**Run against a real vCenter on 2026-08-23 and 2026-08-24 (E18 extended and re-run again on 2026-08-24; E19 added and run 2026-08-31, see below).** The program lives at `hack/vcresearch/nic-unit-numbers/main.go` (still present pending T023's deletion once this section is reviewed). It ran all 15 experiments (`E01`–`E15`) tasks.md T001 originally enumerates, plus four more (`E16`, `E17`, `E18`, `E19`) added afterward on request — see "Extension experiments" below. Every VM it created was destroyed on exit (verified via `govc find` after each run); none of the results below required `-keep`. **Raw output** (the full requested-vs-observed device dumps and fault detail per R7) is committed under [`t001-results/`](./t001-results/): `full-run.md`/`.json` is the complete E01–E15 run (E13 included via an out-of-band-add automation); `e02-discriminator.md`/`.json` is the standalone rerun that resolved the OVF-descriptor-interaction question below; `e06-fixed.md`/`.json` is the standalone rerun with the corrected finding text described in the methodology notes; `e16-e17-e18-suspend-vmotion-upgrade.md`/`.json` is the suspend/resume, vMotion, and hardware-version-upgrade extension run (each of E16/E17/E18 was also run standalone at least once before this combined run, with matching results); `e19-out-of-range-unit-number.md`/`.json` is the standalone out-of-range-unit-number run against the same testbed's vCenter (10.162.38.193, same build). The synthesis below is derived from those runs (E01–E12/E14/E15 from `full-run`, E02/E06 cross-checked against their standalone reruns, E16/E17/E18 from the combined extension run, E19 from its own standalone run).

### Environment

| Property | Value |
|---|---|
| vCenter | VMware vCenter Server 9.2.0, build 25689988, API 9.1.2.0.rc0 |
| ESX host | 9.2.0, build 25690016 |
| VM hardware version | vmx-23 for directly-created VMs (observed from E01's VM; not pinned via `-hardware-version`); vmx-15 for the OVF-deployed VMs in E02, whose hardware version comes from the `photon-5.0` OVF descriptor rather than the environment default — see note below |
| Testbed | `wcp-4-esx-fullInstall` (nested/nimbus ESXi) — no SR-IOV-capable pNIC and no vGPU/DVX passthrough device available |
| govmomi | `v0.56.0-alpha.0.0.20260720221020-d993be43fe66` |
| Content library | subscribed `vmsvc` library, OVF item `photon-5.0` (used for E02) |

**Single-vCenter run (R6).** This characterises the builds above only; it does not by itself answer cross-VC-version stability. No other vCenter build was available in this pass. **It does, incidentally, cover two hardware versions rather than one**: the program's `hardware()` helper records the first observed `moVM.Config.Version` and does not re-check it per experiment, so the table above reflects E01's directly-created VM (vmx-23) — but E02's OVF-deployed VMs actually came up at **vmx-15**, the version the `photon-5.0` OVF descriptor declares, not the environment default. Q1 and Q2 are therefore each confirmed HONOURED on a different hardware version (vmx-23 for the direct-create/reconfigure paths, vmx-15 for the OVF path) rather than both on the same one — a strengthening of the result, not a caveat, though still short of a genuine multi-VC-build matrix.

### Summary

| Experiment | Question(s) | Status | Title |
|---|---|---|---|
| E01 | Q1 | HONOURED | `folder.CreateVM` honours explicit NIC unit numbers in the create ConfigSpec |
| E02 | Q1, Q2 | HONOURED | OVF content-library deploy honours explicit NIC unit numbers |
| E03 | Q1 | HONOURED | `ReconfigVM_Task` Add honours explicit NIC unit numbers on a powered-off VM |
| E04 | — | RECORDED | NICs added with no unit number are assigned from 7 upward |
| E05 | — | RECORDED | Add/remove NICs powered off (primary) and powered on (informational) |
| E06 | — | HONOURED | Remove at unit N and Add at unit N are accepted in one `ReconfigVM_Task` |
| E07 | — | RECORDED | Edit an existing NIC's unit number on a powered-off VM (informational) |
| E08 | Q4 | RECORDED | Fault returned when an explicit NIC unit number collides |
| E09 | — | RECORDED | `ControllerKey` on operator-built Add payloads is unset and resolved by vSphere |
| E10 | — | SKIPPED | No vGPU/DVX passthrough device available in this testbed |
| E11 | Q5 | RECORDED | A freed unit number is reused by the next auto-assigned NIC |
| E12 | Q6 | RECORDED | NIC unit numbers are stable across a power cycle |
| E13 | Q7 | RECORDED | Out-of-band NIC add does not shift existing NICs' unit numbers |
| E14 | Q3 | SKIPPED | No SR-IOV-capable pNIC/network available in this testbed |
| E15 | — | HONOURED | Hot-add a NIC with an explicit unit number (informational) |
| E16 | — | RECORDED | Suspend/resume — not in T001's original scope (extension) |
| E17 | — | RECORDED | vMotion to another host — not in T001's original scope (extension) |
| E18 | — | RECORDED | Hardware-version (VM Compatibility) upgrades through intermediate versions, with post-upgrade power-on — not in T001's original scope (extension) |
| E19 | — | RECORDED | An explicit unit number outside the 7-16 band: silently renumbered below the band, rejected above it — not in T001's original scope (extension) |

### Answers to the gating questions (Q1, Q2, Q4)

- **Q1 — HONOURED.** vSphere honours an explicit `VirtualDevice.UnitNumber` on every Add path this feature depends on: the initial create ConfigSpec via `folder.CreateVM` (E01), a post-create `ReconfigVM_Task` Add on a powered-off VM (E03), and — informationally — a hot-add to a powered-on VM (E15). Every requested unit number was observed on the resulting hardware in every case. **This releases the T001 gate on T008, T009, T010, T017, T018, T029, and T022 scenarios 2/3/5** — the "If Q1 fails" fallback in `plan.md` was not taken; implementation proceeds on the identity-model design as written, and `unitNumber`-carrying Add payloads may be built with confidence they will land where requested on this support matrix.
- **Q2 — HONOURED, plus a load-bearing interaction finding.** The OVF content-library deploy path (`deployOVF` → vAPI `DeployLibraryItem` with the ConfigSpec marshalled to XML as `deploymentSpec.VmConfigSpec`) honours explicit unit numbers identically to `folder.CreateVM` (E02). **How the OVF descriptor's own NICs interact with the ConfigSpec's Add entries — confirmed by two runs, not just one:**
  - With explicit units `[7, 10, 16]` (the program's default set, which includes the descriptor's own auto-assigned slot 7): the baseline deploy (no ConfigSpec) produced exactly 1 NIC, at unit 7. The ConfigSpec-carrying deploy produced exactly 3 NICs — at 7, 10, 16 — not 4. That alone is ambiguous: it is consistent with either "the descriptor's NIC and the unit-7 ConfigSpec Add merged into one" or "the descriptor's NIC was never created at all."
  - **Discriminator rerun with units `[10, 16]` (excluding 7 entirely) resolved it**: the resulting VM carried exactly 2 NICs — at 10 and 16 — with **no NIC at unit 7 at all**. The descriptor's own NIC was not created.
  - **Conclusion: when `deploymentSpec.VmConfigSpec` carries any ethernet-card device Add entries, the OVF descriptor's own network-card declaration is suppressed outright — the ConfigSpec's device list is authoritative for NICs, not additive to the descriptor's.** This is good news for T029's OVF branch: there is no descriptor-vs-ConfigSpec NIC collision to guard against, because the descriptor's NIC contribution is not created once a device-carrying ConfigSpec is supplied. The operator's own create path always supplies a ConfigSpec with the interfaces it wants, so this is the path it will always take through `deployOVF`.
- **Q4 — `InvalidDeviceSpec`, `property=unitNumber`, `deviceIndex` populated.** A colliding explicit `UnitNumber` — both on `CreateVM` (two NICs requesting the same unit) and on a `ReconfigVM_Task` Add against an already-occupied unit — is rejected with fault type `InvalidDeviceSpec`, `Property: "unitNumber"`, and a populated `DeviceIndex` naming the offending device in the request (E08). **This matches vcsim's simulated fault type exactly**, which confirms `plan.md`'s I26 disposition: vcsim's fault *type* is representative even though its duplicate-unit check cannot fire for operator-shaped Add payloads (unset `ControllerKey`). T018/T029 should treat `InvalidDeviceSpec` with `Property == "unitNumber"` as the permanent-error signal (`pkgerr.NoRequeueError`), not a retryable condition — retrying an unchanged colliding payload will fault identically forever.

### Answers to the non-gating questions (Q3, Q5, Q6, Q7)

- **Q3 — NOT confirmed live; this testbed cannot exercise it, and a follow-up attempt to fake the hardware confirmed why not.** E14 was skipped: this testbed (nested/nimbus ESXi VMs) has no SR-IOV-capable physical NIC or SR-IOV-enabled network to attach a `VirtualSriovEthernetCard` to, and the program recorded an explicit skip with reason rather than omitting the result (R5). The spec's existing decision — SR-IOV cards are `VirtualEthernetCard`s and draw from the same 4000-series device keys and 7–16 band, per the platform's static per-device-class allocation in `external/vim/api/v1alpha1/testdata/device_keys.txt` — still stands, but it rests on the documented platform contract plus the corroboration below, not on a live NIC-add observation from this run. A future run against a testbed with real SR-IOV hardware should still confirm it end to end.
  - **No SR-IOV-capable PCI hardware exists on any of this testbed's 4 hosts**, confirmed by querying each host's `HostPciPassthruSystem` (`pciPassthruInfo`, `sriovDevicePoolInfo`) via the VC API: every PCI device on every host reports `PassthruCapable: false`, and the SR-IOV device pool is empty everywhere. The only pNICs present are `nvmxnet3` (nested-ESXi virtual NICs). Scope this precisely: the finding is "absent on this testbed, and not fakeable by the mechanism tried below," not "SR-IOV is architecturally impossible on nested ESXi" in general.
  - **The vGPU-faking trick (`EnsureVGPUConfiguration` in `test/e2e/vmservice/vmservice/util.go`, which installs a `test-vmx.vib` matched to the ESX build) does not also fake SR-IOV, despite a rumor that it might.** The vib was installed on host `10.162.36.16` (build `bora-25690016`, derived from this testbed's observed ESX build `25690016` the same way `EnsureVGPUConfiguration` derives it, then confirmed as a real, fetchable 57 MB file before installing) via `esxcli software vib install`, followed by `esxcli graphics host refresh`, mirroring that function exactly. It genuinely works for its stated purpose: `esxcli graphics device list` afterward shows a new "Mockup Vmiop Device" (`SharedPassthru`, vendor VMware) that wasn't there before. But the network-domain equivalent of that check — `esxcli network sriovnic list`, the namespace where a real or mocked SR-IOV-capable pNIC would appear — stayed empty, `esxcli network nic list` still shows only the same 3 `nvmxnet3` pNICs, and VC's `HostPciPassthruSystem` was unchanged before and after. The vib installs no new vmkernel module (`esxcli system module list` is unchanged), and neither `pciPassthru`'s nor `nvmxnet3`'s module parameters expose anything SR-IOV- or mock-related. **The mechanistic reason all of those checks came back clean: `test-vmx` (per its own `esxcli software vib get` description, "Unit tests for the VMX") ships no vmkernel module at all — it is a VMX-userworld payload, not a vmkernel driver.** That is exactly why its mock vGPU device surfaces only in the userworld-facing `esxcli graphics device list` and never in vmkernel-level views (`hardware pci list`, `system module list`, or VC's `HostPciPassthruSystem`, which all read vmkernel-side PCI enumeration) — a mock SR-IOV pNIC, if this vib provided one, would need to appear at that same vmkernel layer (`network sriovnic list` reads it the same way `hardware pci list` does), and nothing appeared there either. **Conclusion: `test-vmx.vib` mocks a vGPU device only, at the VMX-userworld layer; the rumor that it also mocks SR-IOV — which would require faking a vmkernel-visible pNIC, a different layer entirely — is false on this build.** The vib remains installed on `10.162.36.16` (a deliberate choice — it is a harmless, zero-VM mock device and `LiveRemoveAllowed: True`/`RebootRequired: false` makes it trivial to remove later if it ever matters); it was not installed on the other two `dc-cluster` hosts since a second identical negative result would have added nothing.
  - **A second, hardware-independent check corroborates a narrower but still useful part of the spec's premise, without needing any SR-IOV hardware at all.** `EnvironmentBrowser.QueryConfigOption`, queried per host for hardware version `vmx-23`, lists `VirtualSriovEthernetCardOption` as a declared device option on all three `dc-cluster` hosts — live, primary-source confirmation that the platform's own config-option model recognises `VirtualSriovEthernetCard` as an ethernet-card option class at this hardware version, which is the premise `device_keys.txt` keys off. **This does not corroborate the specific 7–16 sub-band claim, and an earlier draft of this note overstated it by pointing at `ControllerType`:** `VirtualSriovEthernetCardOption` and `VirtualVmxnet3Option` do share `ControllerType: "VirtualPCIController"`, but so does `VirtualPCIPassthroughOption` in the same query output — and PCI passthrough demonstrably does *not* draw from the ethernet 7–16 band (that's E10's whole premise). `ControllerType` only establishes "attached to the virtual PCI controller," true of the video card and HBAs too, so it cannot discriminate the sub-band. It does not close Q3 on its own either way: declaring the device class is not the same as a live Add against a real physical function, which still needs actual SR-IOV hardware to observe.
- **Q5 — Reused.** With NICs at units 7, 8, 9, removing the one at 8 and then adding a new NIC with no explicit `UnitNumber` landed the new NIC back at unit 8 — vSphere's own auto-assignment reuses the lowest freed slot rather than continuing past the previous high-water mark (E11).
- **Q6 — Stable.** Units [7, 9, 12] were unchanged across a full power-off → on → off cycle, with no re-inspection surprises (E12). See the scope caveat under "Extension experiments" below — this checks the platform's config record of the slot, not guest-visible NIC enumeration.
- **Q7 — Existing NICs unshifted; the out-of-band NIC takes the next free slot.** `tasks.md` item 13 authorizes either "the vCenter UI (or an out-of-band API call)"; this run used a second, independent API session issuing the add — a `ReconfigVM_Task` from an actor other than the operator's own reconcile, which is the property under test. Against a VM with NICs at units 7 and 9, the add left both unchanged and placed the new NIC at unit 8 (E13). No design change follows (I11), but operators can be told existing NICs are not disturbed by an out-of-band add.

### Two non-obvious platform behaviours found beyond the enumerated questions

1. **On this build, an ethernet card's `Key` is deterministically derived from its `UnitNumber`, not from creation order.** Every observation in every experiment fits `Key == 4000 + (UnitNumber − 7)` — e.g. unit 7 → key 4000, unit 9 → key 4002, unit 16 → key 4009, with no exception across E01–E15. **Consequence: `Key` equality/inequality is not reliable replacement evidence on this platform.** E06's same-slot Remove+Add at unit 9 produced a device with the *same* key (4002) as the one removed, precisely because the unit number was unchanged — yet the MAC address changed (`...8d:11` → `...23:bf` in one run, `...aa:29` → `...47:b0` in the rerun), confirming it genuinely is new hardware, not a no-op. **This means T017's I4 bookkeeping requirement (leave `DeviceKey=0`, do not adopt the removed device's `Key`/MAC after a replace) is correct as specified, but the rationale should read "the MAC goes stale and `Key` is not identity-bearing on at least this build" rather than "the old key no longer resolves to anything"** — on this build the old key value is simply reissued to the new device, so the risk I4 guards against is stale-MAC leakage into bootstrap args/status, not a dangling key reference. Do not add logic anywhere in the implementation that treats an unchanged `Key` as evidence a device was *not* replaced.
2. **An Edit that only changes `UnitNumber` is accepted and genuinely relocates the device with its MAC preserved (E07, informational).** Requesting `Operation: Edit` on the existing NIC at unit 7 with `UnitNumber` set to 11 succeeded; the resulting device sat at unit 11 with the *same* MAC address it had before the edit (only its derived `Key` changed, consistent with finding 1 above — 4000 → 4004 — not with a new device being created). This design never issues such an Edit (`unitNumber` is an identity, not a relocatable slot — see `plan.md` Design point 7), so nothing here changes this MR. It is, however, useful and encouraging context for the deferred Edit-instead-of-replace follow-on (T032): on this build, relocating a numbered interface's slot via Edit does preserve device identity (MAC, and presumably guest-visible state) rather than forcing a MAC/DHCP churn — worth re-confirming when that follow-on is scoped.

### Extension experiments: suspend/resume, vMotion, and hardware-version upgrade (E16–E18 — not in T001's original scope)

None of suspend/resume, cross-host migration (vMotion), or a hardware-version (VM Compatibility) upgrade was among tasks.md T001's 15 enumerated experiments, and none appears in spec.md's Q1–Q8. They were added to `main.go` and run against the same testbed after the initial research pass, on request, because all three are common VM lifecycle transitions the feature will encounter in production and none had been characterised. Raw output: [`t001-results/e16-e17-e18-suspend-vmotion-upgrade.md`](./t001-results/e16-e17-e18-suspend-vmotion-upgrade.md).

- **E16 — suspend/resume: stable.** A VM with NICs at units 7, 9, 12 was powered on, suspended (`SuspendVM_Task`), and resumed (`PowerOnVM_Task` — resuming a suspended VM uses the same op as a cold power-on). Unit numbers, `Key`s, and MACs were all unchanged across the whole cycle, every time this experiment ran across this research effort — clean on every run, no exceptions. The committed artifact reflects the latest run: `[7 9 12]` before, suspended, and after resume.
- **E17 — vMotion: stable.** The same VM shape was powered on, then migrated to a different host in the same cluster via `MigrateVM_Task` (host change only, no storage move — this *is* what a vMotion is when disks stay on shared storage, which they did: the datastore is `sharedVmfs-0`, mounted on all three `dc-cluster` hosts). Every run migrated the VM to a different host in the same cluster and left unit numbers, `Key`s, and MACs unchanged; the host pair varies per run because the target is chosen dynamically (see the environment note), and several different pairs have been observed across this research effort's runs. The committed artifact's run migrated 10.162.36.16 → 10.162.37.144.
- **E18 — hardware-version upgrade, including intermediate versions and post-upgrade power-on: stable at every step.** A VM was created explicitly at `vmx-15` with NICs at units 7, 9, 12, then walked through a sequence of `UpgradeVM_Task` calls — by default `vmx-17`, then `vmx-20`, then an empty-target upgrade to the host's maximum (`vmx-23` here) — rather than a single jump straight to the host's maximum. After each upgrade the VM was observed powered off, then powered on (`PowerOnVM_Task`) and observed again, then powered back off before the next upgrade in the sequence. Every step genuinely took effect — `config.version` read back `vmx-15` → `vmx-17` → `vmx-20` → `vmx-23` in order, reproduced across three full runs with the default target list — and unit numbers were unchanged at every single observation and asserted as such by the program itself: after each upgrade step while still powered off (compared against the immediately preceding step, not only the original baseline), after powering on at that version, and end-to-end from the original `vmx-15` baseline through the last power-on at `vmx-23`. `Key`s and MACs were likewise unchanged throughout, confirmed by inspection of the raw device dumps in the committed artifact — the program itself only asserts `UnitNumber` equality at each of these points, the same scope as E16/E17's own assertions. Powering on at an intermediate version specifically confirms the platform doesn't reassign a NIC's slot when it first boots a VM at a hardware version it has never run at before — a case a single jump straight to the host's maximum wouldn't have exercised for the intermediate versions. Unlike E12/E16/E17, a hardware-version upgrade genuinely rewrites the VM's virtual hardware definition rather than merely changing power/runtime state, so a stable `UnitNumber`, `Key`, and MAC here is a real invariant rather than a near-tautological one. It is also the extension experiment with a direct consumer: `plan.md`'s T014/T015 backfill targets brownfield VMs, and those VMs are exactly the ones most likely to sit at an old hardware version and get upgraded around the same window an operator upgrade backfills their `status`/`spec` NIC identity — E18 says that backfilled identity is not invalidated by a compatibility upgrade (to an intermediate or to the maximum version) happening before or after, nor by the VM being powered on again afterward.
- **A tangential, non-unit-number observation from E18's raw output**: the reported `pciSlot` field (`VirtualDevicePciBusSlotInfo.PciSlotNumber`) is absent from every powered-off observation and present in every powered-on one, for the same device at the same `UnitNumber`. That is expected — the PCI slot address is a runtime placement the platform derives at power-on, not something persisted in the powered-off config — and is unrelated to `UnitNumber` stability (this program tracks `pciSlot` only because it is "cheap and occasionally illuminating," per the file's header comment; no product behaviour depends on it).
- **None of the three results changes anything in `plan.md` or `spec.md`.** All three are further evidence that `UnitNumber` is stable under transitions the reconciler doesn't itself drive, consistent with E12's power-cycle and E13's out-of-band-add results. No new experiment surfaced a case where matching by unit number would need to account for suspend/resume, vMotion, or a hardware-version upgrade (to an intermediate or a maximum version, with or without a subsequent power-on) specifically.
- **Environment note:** the target host for E17 is picked as the first host in the VM's cluster (resolved via the resource pool's owning `ComputeResource`) that isn't the VM's current host — not a specific pinned host — so a re-run may pick a different pair of hosts than shown above; that's expected and doesn't change the result. E18's starting version defaults to `vmx-15` — already known-good on this environment (it's what the `photon-5.0` OVF descriptor itself declares, per E02) and deliberately older than this environment's create-time default (`vmx-23`, per E01), so there is always somewhere for the upgrade sequence to go — but is a `-e18-start-version` flag, not a hardcoded constant, precisely because that "older than the environment's max" requirement is environment-dependent; a different environment should pass a lower value rather than require a code edit. The sequence of intermediate/final targets is itself a `-e18-target-versions` flag (default `vmx-17,vmx-20,max`, where `max` stands in for `UpgradeVM_Task`'s empty-string "upgrade to the host's latest supported" target); a target the VM has already reached or passed (verified against this environment by requesting the already-current `vmx-15` as a target, which recorded the skip below rather than erroring) is recorded as an explicit skip step and the sequence continues with the next target, rather than erroring out. If *every* configured target is already reached, E18 as a whole records a skip rather than a false "stable" result. Skip detection matches the `AlreadyUpgraded` fault by type-name prefix, since govmomi exposes both an `AlreadyUpgraded` fault-info type and a distinct `AlreadyUpgradedFault` wrapper type and neither spelling was actually observed against this environment's targets outside that one deliberate redundant-target check.
- **Scope caveat, applying equally to E12, E16, E17, and E18: all four read `VirtualDevice.UnitNumber` from `moVM.Config.Hardware.Device` — the persisted VM hardware configuration — because that field has no other representation in the vSphere API.** This is the correct and complete way to answer "does the platform's own record of this NIC's slot change across the transition," which is what these four experiments claim, and it is exactly what the feature's `status.network.interfaces[i].unitNumber` is populated from (`vmprovider_vm.go`, per `plan.md` "Status"). It is *not* a guest-OS-visible check — none of E01–E18 inspects VMware-Tools-reported interface enumeration or any other in-guest signal, so these results say nothing about whether a guest OS's own device ordering (or its network driver's) is similarly stable across suspend/resume, vMotion, or a hardware-version upgrade. That question, if it matters, needs a separate experiment with a real guest OS running (every research VM in this program is diskless/OS-less) — and a hardware-version upgrade is the one of the three most likely to actually perturb guest-visible state, since it can change which virtual hardware (and therefore which guest driver) backs a device; this program's diskless VMs cannot speak to that.

### Extension experiment: an explicit unit number outside the 7-16 band (E19 — not in T001's original scope)

Neither T001's original 15 experiments nor E16-E18 asked what vSphere itself does with an explicit `UnitNumber` outside the platform's 7-16 ethernet-card band — the design (spec.md G9, tasks.md T013) assumes the CRD/webhook rejects such a value before it ever reaches vSphere, and E08's collision test only covers a duplicate *in-range* value. This gap was closed by adding `E19` to `main.go` and running it against the same testbed (vCenter 10.162.38.193, build 25689988) on 2026-08-31. Raw output: [`t001-results/e19-out-of-range-unit-number.md`](./t001-results/e19-out-of-range-unit-number.md) / `.json`. Every VM E19 created was destroyed on exit (verified via `govc find` after the run).

**The answer is neither "always honoured" nor "always rejected" — it splits on which side of the band the value is on, and the two outcomes are qualitatively different.** Three values were tried, each on both `folder.CreateVM` and a `ReconfigVM_Task` Add against an already-created VM, to also check for a create-vs-reconfigure difference the way Q1/Q2 found one between `folder.CreateVM` and the OVF path:

| Requested unit | Band it falls in (`device_keys.txt`) | CreateVM result | Reconfigure Add result |
|---|---|---|---|
| 3 | SCSI HBA (3-6), unoccupied on this diskless VM | **Silently placed at unit 7 instead** — task succeeded | **Silently placed at unit 8 instead** (unit 7 already held by the base NIC) — task succeeded |
| 17 | VMCI's own reserved unit | **Rejected**: `InvalidArgument`, "A specified parameter was not correct: unitNumber" | Same rejection |
| 200 | No PCI-unit meaning on this platform | **Rejected**: `InvalidArgument`, "A specified parameter was not correct: unitNumber" | Same rejection |

- **Below the band (3): vSphere does not honour it and does not fault — it silently renumbers the NIC into the ethernet band instead**, exactly the "changed to a valid one under the covers" outcome the question in this section was asked to rule in or out. Neither the create nor the reconfigure path returned an error; both simply placed the device at the next free ethernet slot (7, then 8) as if no `UnitNumber` had been requested at all.
- **Above the band (17, 200): vSphere rejects outright**, with the same fault type and message in both cases, regardless of how far above 16 the value is (one unit over vs. wildly out of range) and regardless of path (create vs. reconfigure).
- **This is the opposite failure mode from a duplicate (Q4/E08), and worth telling apart operationally.** A collision (E08) always faults with `InvalidDeviceSpec`, `Property: "unitNumber"`. An out-of-range value either faults with a *different* fault type (`InvalidArgument`, no `Property`/`DeviceIndex` — compare `deviceIndexSuffix` in `main.go`, which renders nothing for this fault) or does not fault at all. Code that pattern-matches on `InvalidDeviceSpec`/`unitNumber` for the collision case (T018/T029) will not catch a rejected out-of-range Add, and must not assume a lack of fault means the requested unit was honoured.
- **Consequence for the design: vSphere is not a backstop for an out-of-range value that reaches it, so the CRD/webhook rejection (G9) is not redundant defense-in-depth — it is the only layer that reliably prevents a low out-of-range value from silently landing on hardware with a different unit number than the spec claims.** If a webhook bug, a service-account bypass, or a future code path ever sent an unvalidated low value (e.g., a stray 0-6) to vSphere, the result would not be a clean rejection to catch and log — it would be a spec/hardware divergence that looks exactly like the ordinary "no unit number requested" case, undetectable from the fault alone. This does not change G7's backfill skip (the backfill only ever writes *observed* hardware values, which vSphere itself only ever places in 7-16, so it can't be fed a low out-of-range value this way) or G9 itself (already a MUST), but it sharpens the rationale: G9 is the sole guarantee, not a redundant check on top of a vSphere-side one.
- **Scope note:** this run did not characterise every value on either side of the band (e.g., 0, negative, or 4-6 specifically) — 3 is one representative sample of the "below 7" case. The clean, consistent 17/200 rejection suggests the upper-bound check is a hard cap rather than value-dependent, but the lower side was only sampled once; a future run wanting to fully map the boundary should also try 0, 6, and a negative value (govmomi never sends -1 itself — see the file header comment — but nothing stops a raw SOAP/vAPI caller from doing so).

### Methodology notes

- **E13 used the out-of-band-API-call form `tasks.md` item 13 explicitly authorizes ("the vCenter UI (or an out-of-band API call)")**: a second, independent govmomi/govc session issued the `ReconfigVM_Task` Add, rather than a browser click. The property under test — does an add from outside the operator's own reconcile shift other NICs' unit numbers — is a server-side allocation behavior the VIM API enforces identically regardless of which client issued the request, so this is a like-for-like exercise of the authorized alternative, not a substitution requiring justification.
- **E10 and E14 are explicit, reasoned skips (R5), not gaps in coverage.** This testbed is a nested/nimbus-provisioned set of ESXi VMs with no physical SR-IOV NIC and no GPU/DVX passthrough device to attach. Re-run both against a testbed with that hardware before treating Q3 as fully closed.
- **Two gaps found in the research program itself during this run, both fixed in `main.go` before the results above were finalised:**
  1. The environment table's "VM hardware version" was only ever populated from the `-hardware-version` flag (which this run did not pass), never from an actual created VM — so it silently rendered `_(not recorded)_` despite R6 asking for it. Fixed to record the first observed `moVM.Config.Version` from any research VM.
  2. E06's finding text unconditionally asserted "a changed key confirms..." without checking whether the key actually changed — which produced a misleading finding on this build (see non-obvious finding 1 above, where the key does *not* change). Fixed to check `Key`/`MAC` independently and to call out explicitly when `Key` is unchanged, so the report doesn't assert something the data contradicts.

## Scenarios for the reconcile matching change

### Normal steady-state (NIC already exists, flag on, unit numbers present)

```
Spec: interfaces[0].unitNumber = 9  (eth0, backed by portgroup PG-A)
VC:   ethCards[0] = vmxnet3, UnitNumber=9, ExternalId=abc, backing=PG-A
```

**Matching is two steps, and the unit number only does the first one.** The unit number answers *"is there a device in this interface's slot?"* — it locates the hardware. It does **not** answer *"is that device what this spec interface describes?"* That second question is still answered the way it always has been: compare the located device's **backing**, its **MAC when the desired state pins one** (`AddressType: Manual`), and its **ExternalId when the desired state specifies one**. Fields the desired state leaves unspecified are not compared.

Here both steps pass — a device exists at unit 9, and its backing/ExternalId agree with the desired state — so no config change is emitted and the interface adopts the device's `Key` and MAC.

Without unit-number matching (current behaviour): the backing matches PG-A, so this also resolves correctly today — but only because the backing happened not to change. That fragility is the reason for the change, not the steady state itself.

### NIC added with an explicit unit number, flag on

```
Spec: interfaces[0].unitNumber = 7, interfaces[1].unitNumber = 8 (new, user-specified)
VC:   ethCards[0] = vmxnet3, UnitNumber=7
```

interfaces[0]: a device exists at unit 7; its properties agree; claim it, no change.
interfaces[1]: no device at unit 8 — the interface has no hardware yet, so it takes the ordinary Add path with `UnitNumber=8` on the payload. vSphere places the NIC at slot 8 (Q1: confirmed HONOURED — see the T001 results above), and status reports `unitNumber=8`.

### NIC added to an upgraded VM without specifying a unit number, flag on

```
Spec (as submitted):  interfaces[1] = eth1, no unitNumber
Spec (as admitted):   interfaces[1].unitNumber = 8   <- assigned by the mutation webhook
VC:                   ethCards[0] = vmxnet3, UnitNumber=7
```

**With the disk-pattern assignment this case no longer produces an un-numbered interface.** The mutation webhook assigns the next free slot at admission, so by the time the reconciler sees the VM, interfaces[1] is numbered like every other interface: no device at unit 8, so it is added there, and it is matched by unit number on every subsequent reconcile. The MAC/ExternalId/backing path is not involved.

The genuinely un-numbered cases that remain are narrow, and both are transitional:

- **A VM that has not yet been schema-upgraded — including every VM at creation time.** The mutator's `IsObjectUpgraded` gate is closed, so it assigns nothing; the backfill has not run yet either. Users cannot edit `spec.network.interfaces` at all in this window (see "User updates are blocked until the schema upgrade completes", below), so the interfaces are exactly as they were, matched by MAC/ExternalId/backing as they are today.
- **An interface the backfill had to skip** because its observed value would have made the spec inadmissible (G7). It keeps a nil `unitNumber` **only until the next admission request** — the mutating webhook has no privileged-account bypass, so it numbers the interface on the backfill's own patch (which carries the annotation stamp) or on any later update. This mirrors the disk path exactly (`hasPlacementMismatch` skips → `AddControllersForVolumes` assigns); the difference is that under G11 the invented slot is load-bearing. So this case is genuinely transitional too, and the un-numbered fallback path is not where it ends up — see `plan.md` "Consistency with the disk placement model" and I18.

### NIC removed, flag on

```
Spec: interfaces[0].unitNumber = 7  (eth1 at unit 8 has been removed from spec)
VC:   ethCards[0] = vmxnet3, UnitNumber=7; ethCards[1] = vmxnet3, UnitNumber=8
```

`FindMatchingEthCard` for interfaces[0]: unit 7 matches ethCards[0]. Claim.
No more desired interfaces. Remaining: ethCards[1] (unit 8). → Remove config spec for ethCards[1].
Correct NIC removed by unit number, not by backing.

### Backing change during network migration (flag on, unit numbers present)

```
Spec: interfaces[0].unitNumber = 7, backed by PG-B (just migrated)
VC:   ethCards[0] = vmxnet3, UnitNumber=7, ExternalId=abc, backing=PG-A (old)
```

The unit number identifies the device immediately (which is the real improvement: identification is now stable across a backing change, where the backing-first matcher would have missed). But convergence for this MR is **compare-then-replace**: the located device's backing disagrees with the desired PG-B, so the reconciler emits a Remove of the device at unit 7 plus an Add of the desired device at unit 7, in the same `ReconfigVM_Task` (vSphere permits freeing and reusing a unit number within one Reconfigure). The interface keeps its unit number; the *device* does not survive — it gets a new key, and a new MAC when the provider does not pin one.

Two consequences worth recording here because they are easy to rediscover as "bugs":

- **A numbered interface no longer reaches the orphaned-CR Edit path** (`findExistingEthCardForOrphanedCR`), because pass 1 finds a device at its declared unit number and therefore never falls into the `else` branch that consults orphaned CRs. Today that path can absorb this exact migration by editing the device in place; for numbered interfaces it stops applying. Emitting a device-preserving Edit instead of a replacement is deferred follow-on work — see `plan.md` Design point 7 and the type-support/Edit follow-ons in `tasks.md` T032.
- **The replacement device's type comes from `spec.network.interfaces[i].type`**, whose documented contract is "if omitted, VMXNet3 will be used for new network interfaces" (`api/v1alpha6/virtualmachine_network_types.go:200-204`). So a VM whose actual device is E1000/SR-IOV while `spec.type` is empty — a divergence the class-ConfigSpec path can create — gets a VMXNet3 replacement. Setting `type` explicitly is the sanctioned way to pin this, and the enum already includes `SRIOV`, `E1000`, `E1000e`, `VMXNet2`, `PCNet32` (`virtualmachine_network_advanced_types.go:11-35`). **Interim gap:** the reconcile path does not consult `type` yet (see the next section), so between this MR and the type-support MR that pinning is not yet effective — see `plan.md` "Rollout / migration" for the resulting ordering constraint. Note the Telco `NICConfigFromMoVM` backfill already populates `type` *from the observed device*, so for VMs it has touched the spec data needed to preserve the type is already present; only the consumer is missing.

### Device type differences (flag on, unit numbers present)

```
Spec: interfaces[0].unitNumber = 7, type = E1000
VC:   ethCards[0] = vmxnet3, UnitNumber=7
```

**Type-changing convergence is OUT OF SCOPE for this MR** (see `spec.md` Non-goals, review finding I2). Device type is deliberately excluded from the compare step, so a located device whose type differs from the spec is **left alone** — no Edit, no replacement. The reason is that the update path's desired device is always built by `CreateDefaultEthCard`, which hardcodes `defaultEthernetCardType = "vmxnet3"` (`pkg/providers/vsphere/network/network.go:108`, `:1332`) and never reflects `spec.type`; comparing type against that hardcoded default would churn every class-ConfigSpec E1000/SR-IOV NIC to vmxnet3 on the first reconcile after the flag flips. Honouring `spec.type` when constructing the desired device — which is what makes both this convergence *and* a type-preserving replacement possible — is the deferred type-support MR (`tasks.md` T032).

### Brownfield backfill failure modes

If the hodgepodge cannot uniquely match a spec interface to a VC device (e.g., two NICs on the same portgroup with generated MACs), `NICUnitNumbersFromMoVM` falls back to a positional zip for the otherwise-unmatched (spec interface, VC device) pairs, so every interface still receives an observed unit number. The zip is only applied to the leftovers after hodgepodge matching has claimed everything it can uniquely identify. The mutator's `IsObjectUpgraded` gate means the backfill never races webhook-assigned values: for a VM that has not been upgraded, the only spec values the backfill can encounter are explicit user values, which win.

## Unit-number allocation on the PCI bus is static per device class

`external/vim/api/v1alpha1/testdata/device_keys.txt` records the VIM API's static device-key and PCI-unit allocation. The relevant rows:

```
DeviceKey                            Unit Index
  4000    4009  Ethernet NIC   10    4000+dev    pci:7..16

PCI Controller Unit Allocation:
Start  End  Count  Device
  0     0      1    Video Card
  2     2      1    Audio
  3     6      4    SCSI HBA
  7    16     10    Ethernet NIC
 17    17      1    VMCI
 18    21      4    PCI Passthrough-0..3
 22    22      1    UHCI/EHCI
 23    23      1    XHCI
 24    27      4    SATA HBA
 30    33      4    NVMe HBA
 38   161    124    PCI Passthrough-4..127
```

This is authoritative for the range and settles three things:

- **Ethernet NIC unit numbers are exactly 7–16.** The allocation is static per device class, not first-fit: every other PCI-bus occupant has its own reserved band (VMCI at 17, passthrough at 18–21 and 38–161, SATA at 24–27, NVMe at 30–33). A vGPU / DirectPath I/O device cannot displace a NIC or push one above 16, so the CRD `Minimum=7` / `Maximum=16` markers match the platform contract exactly.
- **The 10-NIC cap is the same constraint from the other side.** Device keys 4000–4009 are ten slots, matching `+kubebuilder:validation:MaxItems=10` on `spec.network.interfaces` and the ten units 7–16 one-for-one.
- **SR-IOV cards are ethernet NICs for allocation purposes** — `VirtualSriovEthernetCard` is a `VirtualEthernetCard`, so it draws from the same 4000-series keys and the same 7–16 band. **T001's SR-IOV experiment (E14) was run but recorded an explicit skip** — this test environment has no SR-IOV-capable hardware — so this line is still sourced from the device-key table above, not from a live observation; re-run E14 on a testbed with SR-IOV hardware to actually confirm it (see Q3 above).

**Do not reason about NIC unit numbers from govmomi's `VirtualDeviceList.AssignController` / `newUnitNumber` (`object/virtual_device_list.go:480-548`).** That helper picks the first free unit ≥ 7 counting every device on the controller — a client-side convenience for constructing a config spec, not a model of the platform's allocation. Reading it as the allocation rule produces a false conclusion that passthrough occupants can push a NIC above 16.

## Where NIC device changes are actually computed

`ReconcileNetworkInterfaces` is reached from `UpdateEthCardDeviceChanges`, which has two call sites in `session_vm_update.go`, and the dispatch in `reconcilePoweredOffOrPoweredOnVM` does not reach both on every reconcile:

| Situation | Path | NIC device changes? |
|---|---|---|
| Powered on | `poweredOnReconfigure` | No |
| Powered off, staying off, `VMResize`/`VMResizeCPUMemory` on | `resizeVMWhenPoweredStateOff` | Only inside `if Features.MutableNetworks` |
| Powered off → on, `VMResize` on | `getResizeConfigSpecForPoweredOffVM` | No — this builder emits no ethernet device changes |
| Powered off → on, `VMResize` off | `getConfigSpecForPoweredOffVM` | Yes, unconditional |

None of this is introduced by this feature — it is the existing shape of NIC convergence — but it determines where any of this feature's device-level behaviour is observable, and therefore which flag combination the E2E scenarios need. See `plan.md`, "Reconcile entry points and flag interactions".

## What a device→spec-interface mapping miss costs today

The exact-only matching rule means a numbered interface with no device at its declared slot gets no entry in `MapEthernetDevicesToSpecIdx`'s output. Both callers handle a missing entry poorly today:

- `updateGuestNetworkStatus` (`update_status.go:1194`) takes the interface name from that map; with no entry it emits the status entry with an **empty `name`**.
- `bootoptions_reconciler.go:235-249` scans the map for the interface's index and, finding none, returns `unable to locate network interface matching name %q` — failing the reconcile.

The state is reachable and long-lived because a `unitNumber` change is admitted on a powered-on VM but not applied until the next power-off. Hence spec.md **G13**, T021, and T033.

## The backfill's write is subject to admission

`ReconcileSchemaUpgrade` mutates `vm.Spec` in memory; the VM controller's patch helper persists it, so the write passes through the same CRD schema validation and the same validating webhook as a user's write. `ValidateUpdate` (`virtualmachine_validator.go:306`) validates the **whole object** on every UPDATE and has no deletion or no-change early-out. Two design consequences follow, both normative in spec.md:

- **G7** — the backfill must not write a value the schema or the webhook would reject (duplicate, or out of range), or the VM stops converging entirely.
- **G10** — a flag-off rule that rejects the field's *presence* rather than its *change* would block every update to a backfilled VM, including finalizer removal, making the VM undeletable while the flag is off.

## User updates are blocked until the schema upgrade completes

The repo already has the mechanism for this, and it already covers the exact field set this feature cares about. `ValidateUpdate` calls `validateSchemaUpgrade`, which — for anyone other than the VM Operator service account or `system:masters` — calls `validateFieldsDuringSchemaUpgrade` when `IsObjectUpgraded(ctx, oldVM)` reports the object is not yet upgraded (`virtualmachine_validator.go:2437-2443`). The comment states the intent directly: *"Prevent most users from modifying the VM spec fields, that are backfilled by the schema upgrade and mutable, before the schema upgrade is completed."*

Inside it, `validateNICBackfilledFieldsNotChanged` (`:3604`) already forbids **any** change to `spec.network.interfaces` in that window:

```go
if !equality.Semantic.DeepEqual(newIfaces, oldIfaces) {
    allErrs = append(allErrs, field.Forbidden(
        specPath.Child("network").Child("interfaces"), notUpgraded))
}
```

**The only gap is its gate:** it runs under `Features.TelcoVMServiceAPI` only (`:3502-3509`), and `VMNetworkUnitNumbers` is orthogonal to Telco — the same argument this spec already makes for keeping the unit-number backfill out of `NICConfigFromMoVM`. So the guard must also apply when `VMNetworkUnitNumbers` is enabled (T034).

This closes the ordering story with the same three-part shape disks use, and it is worth stating as one sequence because each part alone looks incomplete:

1. **Users cannot touch `spec.network.interfaces`** until the VM is schema-upgraded — so no user-supplied unit number can precede the backfill's observation.
2. **The mutation webhook assigns nothing** until the VM is schema-upgraded (`IsObjectUpgraded` on the new object) — so no *generated* value can precede it either.
3. **The backfill records observed hardware** and stamps the feature-version bit; from that request onward both 1 and 2 open up.

The VM Operator service account bypasses (1) — `validateSchemaUpgrade` returns nil for it before any field checks — which is what lets the backfill's own patch through.
