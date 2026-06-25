# Implementation Plan: NIC Unit Numbers

- **Spec**: [`spec.md`](./spec.md)
- **Epic**: vmop-3982
- **Date**: 2026-06-25

## Summary

Add an optional `unitNumber` to `spec.network.interfaces[i]` and `status.network.interfaces[i]`. Explicit user values are carried onto the NIC device at create/add time; unset values are assigned by vSphere and recorded into the spec by the schema-upgrade backfill (which covers both greenfield VMs post-create and brownfield VMs on operator upgrade). The mutation webhook does not assign unit numbers. Uniqueness, range (7–16), and powered-on immutability are enforced in the validation webhook, and the unit number becomes the primary NIC-to-VC-device match key during reconcile — all gated behind the `VMNetworkUnitNumbers` feature flag.

## Technical context

- **Go version**: 1.23 (as in go.mod)
- **API versions touched**: `v1alpha6` (additive fields only)
- **Modules touched**: root module (`github.com/vmware-tanzu/vm-operator`), `api/` sub-module
- **New dependencies**: none

## Constitution check

| Rule | Status | Notes |
|------|--------|-------|
| API compatibility — additive only | OK | New `+optional` / `omitempty` fields; no removal or rename; deepcopy regeneration required |
| Thin controllers | OK | All new logic in `pkg/`; reconciler delegates unchanged |
| No direct vSphere calls in controllers | OK | vSphere reads in `pkg/providers/vsphere/` only |
| E2E coverage mandatory | Required | NIC unit number assignment, matching, and status are cluster-observable; E2E ships with the code |
| One test file per package | OK | New test code added to existing `_test.go` files per package |
| Webhook kubebuilder markers | OK | `+kubebuilder:validation:Minimum=7` and `Maximum=16` on new spec field (NICs occupy PCI slots 7–16) |
| Feature flag before behaviour ships | OK | `VMNetworkUnitNumbers` defaults to `false`; all new behaviour is behind it |
| SDD artifacts ship with code | OK | This spec/plan/tasks/model/research committed to same branch |

## Project structure

New files:
```
webhooks/virtualmachine/validation/virtualmachine_validator_network_interfaces.go
cmd/vmoperator-vc-research/main.go          (govmomi research program — not shipped in binary)
```

(No new mutation-webhook file: the mutation webhook does not assign unit numbers — see "Mutation webhook" below.)

Modified files:
```
api/v1alpha6/virtualmachine_network_types.go
api/v1alpha6/zz_generated.deepcopy.go       (regenerated via make generate-go)
api/v1alpha2/virtualmachine_conversion.go   (annotation-based restore for the new hub-only field)
api/v1alpha3/virtualmachine_conversion.go   (annotation-based restore for the new hub-only field)
api/v1alpha4/virtualmachine_conversion.go   (annotation-based restore for the new hub-only field)
api/v1alpha5/virtualmachine_conversion.go   (annotation-based restore for the new hub-only field)
config/crd/                                 (regenerated via make generate-manifests)
pkg/config/config.go
pkg/util/vmopv1/features.go
pkg/providers/vsphere/network/network.go    (NetworkInterfaceResult struct)
pkg/providers/vsphere/network/reconcile.go
pkg/providers/vsphere/network/devices.go
pkg/providers/vsphere/session/session_vm_update.go
pkg/providers/vsphere/upgrade/virtualmachine/backfill/nic.go
pkg/providers/vsphere/upgrade/virtualmachine/vm_schema_upgrade.go
pkg/providers/vsphere/vmlifecycle/update_status.go
pkg/providers/vsphere/vmprovider_vm.go      (create-path ConfigSpec UnitNumber; status unit-number map)
webhooks/virtualmachine/validation/virtualmachine_validator.go
docs/                                       (user-facing documentation for the new field)
```

## API / CRD strategy

Additive fields on `VirtualMachineNetworkInterfaceSpec` and `VirtualMachineNetworkInterfaceStatus`. No version bump required. Run `make generate-go` then `make generate-manifests` after the API change.

**Conversion changes ARE required — in every older API version.** This repo preserves hub-only spec fields across old-version round-trips via the annotation-based restore mechanism (`utilconversion.UnmarshalData`), not automatically. Without it, a client submitting an UPDATE through an older version wipes `spec.network.interfaces[i].unitNumber` — the k8s#111703 additive-field hazard the constitution warns about. **Each of `v1alpha2`, `v1alpha3`, `v1alpha4`, and `v1alpha5` has its own `restore_v1alpha6_VirtualMachineNetworkInterfaces`** (each restores `VNUMANodeID` today); the new spec field must be added to all four, and each version's conversion fuzz tests must be extended to cover it. `v1alpha1` restores the entire interfaces list wholesale when the down-converted list is non-empty, so it needs no per-field change — but its fuzz tests should assert the field survives the round-trip. The status field needs no per-field handling — status is fully restored via `dst.Status = restored.Status`.

## Controller / webhook impact

### Mutation webhook — intentionally NOT involved

**Design decision: the mutation webhook does not assign unit numbers.** `unitNumber` is optional. The flow of values is:

1. **Explicit user value** → carried onto the `VirtualEthernetCard` device in the create `ConfigSpec` (greenfield) or the reconcile Add `ConfigSpec` (new NIC on an existing VM). vSphere honours the requested slot (T001).
2. **Unset** → the device is created without a unit number, vSphere assigns the slot, and the post-create schema-upgrade backfill records the observed value into `spec.network.interfaces[i].unitNumber`. The same backfill covers brownfield VMs on operator upgrade.
3. **NIC added after the backfill has already stamped the VM, without an explicit value** → the spec field stays nil for that interface (the backfill is one-shot); status reports the observed value and the interface is matched via the MAC/ExternalID/backing fallback.

This makes vSphere the single source of derived values and eliminates the webhook-vs-backfill ordering race an auto-assigning webhook would create (a webhook guess written before the backfill would be locked in by the spec-wins rule and could cause unit-number-first matching to claim the wrong devices). No `NICBusSpec` / `NextAvailableUnitNumber` slot computation is needed anywhere, and no changes are made to `virtualmachine_mutator.go`.

### Validation webhook

New file `virtualmachine_validator_network_interfaces.go` with function `validateNICUnitNumbers`:

1. If flag `VMNetworkUnitNumbers` is off: reject any interface that sets `unitNumber` with `field.Forbidden(path, featureNotEnabled)` — the same pattern `validateNetworkVLANs` uses for `VMVlanSubinterface`. Do NOT silently ignore the field: values persisted while the flag is off would later poison the spec-wins backfill. Decision (2026-07-27): disabling the feature after the backfill has run is not a supported flow, so no carve-out is made for previously-backfilled values (a VM with backfilled unit numbers fails spec updates while the flag is off — accepted; note this diverges from the `VMSharedDisks` hardware-controllers validator, which silently skips when its flag is off).
2. For each interface with a `unitNumber` set:
   a. Range check: 7 ≤ value ≤ 16.
   b. Uniqueness check: value not already in `occupiedSlots`; insert after checking. (CEL `XValidation` on the interfaces list is an alternative for the uniqueness rule per the constitution's CEL-first preference — evaluate at implementation; the powered-on check below requires Go either way.)
3. On update, if the old VM is powered on and an interface's already-set `unitNumber` has changed to a different value, add a `field.Forbidden` error. nil → set transitions are allowed — the schema-upgrade backfill writes observed values into the spec of running VMs and must pass this webhook. A set → nil transition (user clears the value) is treated as a change and likewise rejected while powered on; cover both with tests.

Wired into `validateNetwork` in `virtualmachine_validator.go`.

### Schema upgrade — brownfield backfill

**Decision (confirmed): the backfill is a standalone schema-upgrade step with its own feature-version bit and its own flag/capability gate.** It must NOT live inside `NICConfigFromMoVM` under the `TelcoVMServiceAPI` gate: that gate is one-shot, so any VM already stamped with the Telco bit would never be backfilled, and `TelcoVMServiceAPI` may be disabled while `VMNetworkUnitNumbers` is enabled.

New `FeatureVersionNICUnitNumbers = 16` bit in `pkg/util/vmopv1/features.go`, included in `FeatureVersionAll`, in the `FeatureVersions()` slice, and in `ActivatedFeatureVersion` when `VMNetworkUnitNumbers` is enabled.

New function `NICUnitNumbersFromMoVM` in `pkg/providers/vsphere/upgrade/virtualmachine/backfill/nic.go`:

1. Iterate `spec.network.interfaces`.
2. For each interface, prefer the existing hodgepodge matching (MAC, ExternalID, backing — same logic as `FindMatchingEthCard` / `MapEthernetDevicesToSpecIdx`) to identify the corresponding VC ethernet device.
3. If a unique match is found and `iface.UnitNumber` is nil (spec wins), set `iface.UnitNumber = &dev.GetVirtualDevice().UnitNumber`.
4. After the hodgepodge pass, positionally zip the remaining unmatched spec interfaces against the remaining unclaimed VC ethernet devices and backfill from those pairs, so every interface receives its observed unit number. Log at V(4) which interfaces were zip-matched rather than uniquely matched. The zip is strictly a last resort — it applies only to interfaces the MAC/ExternalID/backing matching could not uniquely claim (decision confirmed 2026-07-27).

Additional backfill behaviours (decisions 2026-07-27):

- **`spec.network.disabled` does not skip the backfill.** The backfill runs regardless; a disabled-network VM has no ethernet devices, so nothing is recorded and the feature-version bit is still stamped (one-shot semantics preserved).
- **Spec wins unconditionally, no mismatch guard.** When an interface already carries an explicit `unitNumber` that disagrees with the observed device slot, the backfill leaves the spec value in place (unlike the disk backfill's `hasPlacementMismatch`, which skips on disagreement), logging the disagreement at V(4).

New gate in `ReconcileSchemaUpgrade`:
```go
if features.VMNetworkUnitNumbers {
    if f := vmopv1util.FeatureVersionNICUnitNumbers; !vmFeatureVersion.Has(f) {
        backfill.NICUnitNumbersFromMoVM(ctx, vm, moVM)
        vmFeatureVersion.Set(f)
    }
}
```

After `NICUnitNumbersFromMoVM` completes, also update `NICConfigFromMoVM` (TelcoVMServiceAPI path) to resolve its `TODO(BV)`: when unit numbers are now present on the spec interface, use unit-number-based matching instead of the positional zip.

### Reconcile — NIC-to-VC matching

This is the most complex and most important change. There are two distinct matching functions that must be updated independently. See the six-problem analysis below.

#### Problem 1: `FindMatchingEthCard` — desired device has no `UnitNumber`

The desired `VirtualEthernetCard` built in `CreateDefaultEthCard` never has a `UnitNumber` set. To add unit-number matching, the unit number must be threaded separately.

**Solution:** Add `UnitNumber *int32` to `NetworkInterfaceResult` (in `network.go`). Populate it in `reconcileNetworkInterfaces` (session_vm_update.go) from `interfaceSpec.UnitNumber` when the feature flag is on. Update `FindMatchingEthCard` to accept a `unitNumber *int32` parameter. When non-nil and the flag is on, first scan `currentEthCards` for a card whose `VirtualDevice.UnitNumber` matches and return it immediately — before the existing MAC/ExternalID/backing checks.

Rationale for unit-number-first: the unit number is a stable, vSphere-assigned hardware slot. Backing can change during network migrations. ExternalID is provider-specific and absent on older VMs. MAC is generated and may not be on the spec. Unit number is the most reliable stable identifier once assigned.

**Two-pass claim order.** `ReconcileNetworkInterfaces` processes results in a single greedy loop today. With unit-number matching added, a result without a unit number processed earlier could claim — via backing match — the very device whose slot a later result explicitly requests. The loop must therefore run in two passes: first match and claim devices for all results carrying a `UnitNumber`, then run the fallback MAC/ExternalID/backing matching for the remaining results against the remaining unclaimed cards. (This mirrors the unit-number-first pass already planned for `MapEthernetDevicesToSpecIdx` in Problem 4.)

**Second call site.** `FindMatchingEthCard` is also called from `fixupMacAddressMutableNetworks` in `session_vm_update.go` (the post-reconfigure MAC-address fixup for results without a `DeviceKey`). That call site must pass the result's `UnitNumber` too, so the fixup pass matches devices with the same unit-number-first semantics as the reconcile pass.

**A unit-number match identifies the device — it does not mean "no change".** After the match, `ReconcileNetworkInterfaces` must diff the matched device against the desired state and emit the appropriate `VirtualDeviceConfigSpec` entries to converge the hardware:

- Same device type, differing backing/properties → an Edit on the matched device (as the orphaned-CR path does today).
- Differing device type (e.g. spec says E1000, device is vmxnet3) → device type cannot be edited in place, so emit a Remove of the matched device plus an Add of the desired device carrying the same `UnitNumber`, preserving the slot across the type change.

#### Problem 2: Add config spec must carry `UnitNumber` — in BOTH the reconcile and create paths

When no match is found in `ReconcileNetworkInterfaces`, an Add config spec is emitted using `r.Device` as-is. If `r.UnitNumber` is non-nil and vSphere honours explicit placement (confirmed by T001 research), set `r.Device.GetVirtualDevice().UnitNumber = *r.UnitNumber` before appending the Add config spec.

**The VM create path is separate and must be updated too — and it has TWO branches.** The initial VM `ConfigSpec`'s NIC devices are built in `vmprovider_vm.go`, not via `ReconcileNetworkInterfaces`, and the create path consumes `network.Device` (from `CreateNetworkDevices`), not `NetworkInterfaceResult` — so `Device` also needs a `UnitNumber *int32` field, populated from the spec interface in `CreateNetworkDevices` when the flag is on. The two branches:

1. **Class-ConfigSpec devices**: ethernet devices already present in the VM class ConfigSpec are positionally zipped to `createArgs.NetworkDevices` and mutated via `ApplyNetworkDeviceToVirtualEthCard`. These devices may **already carry a `UnitNumber` from the class ConfigSpec**. When the spec interface carries an explicit `unitNumber`, it overrides the class device's value (spec wins, under the T001 gate). When the spec interface has no value, a class-provided unit number is left as-is (vSphere honours or reassigns it) and the post-create backfill records the observed value. Note the validation webhook cannot see the class ConfigSpec, so a spec-vs-class collision only surfaces at create time — vSphere rejects the ConfigSpec (vcsim returns `InvalidDeviceSpec` for a taken slot); T001 confirms the real fault and the create error path must surface it clearly.
2. **Default devices**: spec interfaces beyond the class device count get devices from `network.CreateDefaultEthCardFromNetworkDevice`. When the spec interface carries an explicit `unitNumber`, set it on the device (T001 gate).

Interfaces without an explicit value are created without a spec-driven unit number; vSphere assigns the slot and the post-create backfill records it into the spec on the first update reconcile.

This is gated on research: if vSphere does NOT honour the field, omit it and let vSphere auto-assign. **Note:** in that branch there is no automatic convergence — the schema-upgrade backfill is one-shot and spec-wins, so it cannot correct an already-set spec value that disagrees with the observed slot. If T001 lands on "not honoured", the design must be revisited (e.g. restrict unit-number-first matching to backfilled brownfield values, or define a sanctioned spec-correction write); do not ship spec-driven placement in that case.

#### Problem 3: Edit config spec for orphaned-CR path

The orphaned-CR Edit path edits backing/MAC/ExternalId but not `UnitNumber`. If the spec requests a specific unit number that differs from the orphaned device's current slot, and vSphere supports unit-number changes via Edit on powered-off VMs (confirmed by T001), set `UnitNumber` on the edit device. If not supported, log a warning and leave the unit number unchanged; the status will reflect the actual slot.

#### Problem 4: `MapEthernetDevicesToSpecIdx` — unit-number-first pass

Add a unit-number-first pass at the top of `MapEthernetDevicesToSpecIdx` (both mutable and immutable paths):

1. If the feature flag is on, build a `UnitNumber → VC device` map from the current ethernet cards.
2. For each spec interface with `unitNumber` set, look up the map and claim that card.
3. For any remaining unmatched spec interfaces (no unit number, or unit number not in the VC device list), fall through to the existing per-provider CR-based matching (mutable) or positional zip (immutable).

This:
- Avoids expensive Kubernetes API calls for interfaces that already have a unit number.
- Correctly handles interfaces added or removed since the last schema upgrade.
- Keeps the existing CR-based path as a robust fallback.

Note that `MapEthernetDevicesToSpecIdx` has a second caller besides the status path in `vmprovider_vm.go`: the boot-options reconciler (`pkg/vmconfig/bootoptions/bootoptions_reconciler.go`) uses it to map boot-order devices to spec interfaces. The unit-number-first pass changes behaviour for both callers; boot-options tests must cover the new mapping.

#### Problem 5: Immutable-networks positional zip

The positional zip in `MapEthernetDevicesToSpecIdx` becomes a last-resort fallback: used only when no spec interface has a unit number AND `MutableNetworks` is off. Once unit numbers are backfilled on all existing VMs, this path should be unreachable in normal operation.

#### Problem 6: `Edit` path — unit number drift

When the orphaned-CR Edit emits a device change, the resulting vSphere device retains its old `UnitNumber` implicitly (the Edit operation does not reassign slots). If the spec has a different `unitNumber` for that interface, there is a mismatch. Since the mutation webhook never assigns unit numbers, a "guessed value after the fact" cannot occur — spec values are either backfilled observations (which match the device by construction) or explicit user requests. The residual case is therefore a user explicitly requesting a different slot for an existing interface: if T001 confirms Edit supports unit-number changes on powered-off VMs, emit the change on the Edit device; otherwise reject or log-and-ignore per T001's findings. Uncommon; can be refined as a follow-up.

#### Summary table

| Location | Current behaviour | Change (flag on) |
|---|---|---|
| `NetworkInterfaceResult` struct | No `UnitNumber` field | Add `UnitNumber *int32` |
| `reconcileNetworkInterfaces` | Builds results from spec, no unit number | Populate `result.UnitNumber` from `interfaceSpec.UnitNumber` |
| `FindMatchingEthCard` | Matches on MAC/ExternalId/backing | New `unitNumber *int32` param; unit-number-first scan when non-nil |
| `ReconcileNetworkInterfaces` — matched device | Match means no change | Diff matched device vs desired: Edit for backing/property changes; Remove+Add at same unit number on device-type change |
| `ReconcileNetworkInterfaces` — Add | Sets no `UnitNumber` on device | Set `r.Device.UnitNumber = *r.UnitNumber` when non-nil (gated on T001 research) |
| `ReconcileNetworkInterfaces` — Edit (orphaned) | Updates backing/MAC/ExternalId only | Optionally set `UnitNumber` (gated on T001 research) |
| `fixupMacAddressMutableNetworks` (second `FindMatchingEthCard` call site) | Matches on MAC/ExternalId/backing | Pass `r.UnitNumber` for unit-number-first matching |
| `network.Device` struct / `CreateNetworkDevices` | No `UnitNumber` field | Add `UnitNumber *int32`, populated from spec interface (create path consumes `Device`, not `NetworkInterfaceResult`) |
| VM create path — class-ConfigSpec branch (`ApplyNetworkDeviceToVirtualEthCard`) | Class device keeps its class-provided `UnitNumber` (if any) | Explicit spec value overrides the class device value (spec wins, T001 gate); unset spec leaves class value as-is |
| VM create path — default branch (`CreateDefaultEthCardFromNetworkDevice`) | Sets no `UnitNumber` on initial ConfigSpec devices | Set desired `UnitNumber` on new NIC devices (gated on T001 research) |
| `MapEthernetDevicesToSpecIdx` | CR-based per-provider or positional zip | Unit-number-first pass; CR/zip as fallback (affects status AND boot-options callers) |
| `updateGuestNetworkStatus` | Writes Name/DeviceKey/IP/DNS | Also writes `UnitNumber` from matched VC device |

### Status

`updateGuestNetworkStatus` (vmlifecycle/update_status.go) currently receives only `*vimtypes.GuestInfo` and the `deviceKey → specIdx` map — it has **no access** to the actual hardware device list, so it cannot read `VirtualDevice.UnitNumber` directly.

The fix requires threading a second map through `ReconcileStatusData`:

1. Add `NetworkDeviceKeysToUnitNumber map[int32]int32` to `ReconcileStatusData` in `update_status.go`.
2. In `vmprovider_vm.go`, in the call site that builds `ReconcileStatusData`, iterate `moVM.Config.Hardware.Device`, filter ethernet cards, and build the `deviceKey → VirtualDevice.UnitNumber` map.
3. Pass the new map into `ReconcileStatusData`.
4. In `updateGuestNetworkStatus`, use the map to set `status.network.interfaces[i].unitNumber` for each interface whose `DeviceKey` is in the map.

**Flag-gated (decision 2026-07-27).** Populating `status.network.interfaces[i].unitNumber` is gated on `VMNetworkUnitNumbers`: when the flag is off, the `NetworkDeviceKeysToUnitNumber` map is not built and the status field is not written, so the new API surface does not appear while the feature is disabled.

**VMware Tools dependency.** `updateGuestNetworkStatus` builds interface status entries by iterating `guestInfo.Net`; a VM without Tools running has no `status.network.interfaces` entries at all, even though unit numbers are available from `moVM.Config.Hardware`. The `unitNumber` status field therefore only appears for Tools-reported interfaces — the spec's acceptance criterion is scoped accordingly. Building interface status entries from hardware config for Tools-less VMs would be a broader behaviour change and is out of scope here.

## Test strategy

- **Unit tests**: for all new pkg functions — `validateNICUnitNumbers`, `NICUnitNumbersFromMoVM`, updated `FindMatchingEthCard`, updated `MapEthernetDevicesToSpecIdx`, updated `updateGuestNetworkStatus`, and the create-path device construction. Added to existing `*_test.go` files per package following the `_test` external package convention and `testlabels` Label patterns.
  - Webhook test files use named `func xTests()` functions registered via `suite.Register` — not `var _ = Describe`. The new test file (`virtualmachine_validator_network_interfaces_test.go`) follows the same convention as `virtualmachine_validator_hardware_controllers_test.go`. (No mutation-webhook tests: the mutation webhook is not involved.)
  - Backfill/pkg unit tests use `var _ = Describe(...)` at top level (no `testlabels` on outer `Describe`).
- **Unit/conversion tests**: conversion fuzz tests in `api/v1alpha5` extended to cover round-tripping the new spec field through the annotation restore.
- **Integration tests**: covered by the webhook test suite via `builder.NewTestSuiteFor{Mutating,Validating}WebhookWithContext`; no separate integration test file needed. The **brownfield backfill** is verified here too (vcsim: VM with NICs and no spec unit numbers → schema upgrade runs → spec carries observed values, feature-version bit stamped) because the E2E environment cannot flip the flag/capability mid-run. **vcsim viability confirmed** (govmomi v0.55.0-alpha simulator source): vcsim assigns NIC unit numbers starting at 7 (`AssignController` applies a PCI offset of 7 for ethernet cards), honours an explicit `UnitNumber` on Add (auto-assigns only when nil), and rejects a duplicate unit number on the same controller with `InvalidDeviceSpec` — so backfill, placement, and collision scenarios are all exercisable under vcsim.
- **E2E tests** (mandatory — cluster-observable; `test/e2e/vmservice/vmservice/virtualmachine/` — note double `vmservice`). Full scenario detail lives in `tasks.md` T022; summary:
  - New file `vm_nic_unit_numbers.go` with `func VMNICUnitNumbersSpec(...)` following the `vm_networking.go` pattern, registered alongside `VM-NETWORKING` in `vmservice_test.go`.
  - Backfill on create with no explicit values (spec unchanged at admission; backfilled post-reconcile; status matches after Tools reports).
  - Explicit placement on create, and mixed explicit/unset across multiple NICs to exercise two-pass matching — both gated on T001 (does vSphere honour explicit `UnitNumber` on Add).
  - Add NIC to a running VM (unique slot, pre-existing NICs unaffected); remove a NIC identified by device `Key` (not just count), verifying survivors are unchanged.
  - Steady-state reconcile stability (`Consistently` — no spurious remove+add) and power-cycle stability (unit numbers unchanged across power off/on).
  - Conversion round-trip: set a `unitNumber`, submit an UPDATE via the `v1alpha2` manifest shape, re-read as `v1alpha6`, assert the value survived via the annotation-based restore — this is the only place the real API-server conversion chain (not just the fuzz tests) is exercised.
  - ~~Admission-validation scenarios~~ (duplicate, out-of-range, powered-on change rejection, nil→set allowance) — NOT covered in E2E: pure webhook logic with no cluster dependency, exhaustively covered by the `nicUnitNumberTests()` unit tests in T013 instead.
  - ~~Brownfield backfill scenario~~ — NOT covered in E2E: the deployment-level flag/capability cannot be toggled mid-run on a shared supervisor. Covered by vcsim integration tests instead (see above).
  - ~~Snapshot-revert/failover interplay~~ — NOT covered in E2E for the same reason; covered by vcsim integration tests under T031.

## Rollout / migration

- **Feature flag**: `pkgcfg.Features.VMNetworkUnitNumbers` (in the `FeatureStates` struct), default `false`. Decision needed (T002): `VMSharedDisks` is capability-driven (not an FSS env var) via `CapabilityKeySharedDisks` in `pkg/config/capabilities/capabilities.go`. `TelcoVMServiceAPI` is also capability-driven. Determine whether `VMNetworkUnitNumbers` follows the same capabilities path or if it warrants an FSS env var during development. Document the decision in this section once resolved. Either way the feature has its **own** flag/capability and its **own** schema-upgrade bit — it does not piggyback on `TelcoVMServiceAPI`.
- **Schema upgrade** (decision confirmed): standalone `NICUnitNumbersFromMoVM` step gated on `VMNetworkUnitNumbers` + `FeatureVersionNICUnitNumbers = 16`. Not part of `NICConfigFromMoVM` / the `TelcoVMServiceAPI` gate — that gate is one-shot, so already-stamped VMs would never be backfilled, and the two flags are orthogonal. See "Schema upgrade — brownfield backfill" above.
- **Rollback safety**: the field is `+optional` / `omitempty`. Disabling the flag stops assignment but does not erase existing values. Re-enabling the flag triggers a fresh backfill pass only for VMs that have not yet been processed (their feature-version annotation lacks the bit). **Decision (2026-07-27): disabling the feature after the backfill has run is not a supported flow** — the validation webhook rejects any use of the field while the flag is off, including previously-backfilled values, so spec updates to those VMs fail until the flag is re-enabled. This is accepted; the feature is expected to remain enabled once rolled out.
- **Old API versions**: the spec field round-trips through v1alpha5 and earlier via the annotation-based conversion restore (see "API / CRD strategy"); without that, old-version clients would silently wipe assigned values on UPDATE.
- **Partner comms**: `status.network.interfaces[i].unitNumber` is a new observable field; partners reading network status should treat it as informational. No breaking change.

## Complexity tracking

| Consideration | Resolution |
|---|---|
| NIC PCI bus has no spec object (unlike SCSI controllers) | No new CRD needed; no slot computation needed either — vSphere assigns unset slots and the backfill records them |
| PCI bus shared with non-NIC devices; NIC slots are 7–16, not 0–9 | CRD markers `Minimum=7` / `Maximum=16`; validation range messages use 7–16 |
| Desired NIC device in reconcile has no `UnitNumber` | Extend `NetworkInterfaceResult` with `UnitNumber *int32`; populate from spec |
| Brownfield first-pass must use hodgepodge (no unit number yet) | `NICUnitNumbersFromMoVM` prefers MAC/ExternalID/backing match with positional-zip fallback; unit-number match used on subsequent reconciles |
| Webhook could guess unit numbers before the backfill runs | Eliminated by design: the mutation webhook never assigns unit numbers — values are explicit-user or backfilled observations only |
| vSphere may not honour explicit `UnitNumber` on Add | Gated on T001 research; if not honoured, spec-driven placement does not ship (no automatic spec/status convergence exists — the backfill is one-shot and spec-wins) |
| Unit-number match with a different device type | Match identifies the device; reconciler diffs and emits Edit, or Remove+Add at the same unit number for type changes |
| Old API versions drop the new hub-only spec field on UPDATE | Annotation-based conversion restore in `restore_v1alpha6_VirtualMachineNetworkInterfaces` + fuzz tests |
| `NICConfigFromMoVM` (TelcoVMServiceAPI) uses positional zip | Upgrade to unit-number match after `NICUnitNumbersFromMoVM` backfill completes; resolves the existing `TODO(BV)` |
| Two different matching functions with different call sites | Updated independently; `FindMatchingEthCard` gets unit-number param (both call sites: `ReconcileNetworkInterfaces` and `fixupMacAddressMutableNetworks`); `MapEthernetDevicesToSpecIdx` gets unit-number-first pass (status + boot-options callers) |
| Greedy single-pass matching could mis-claim an explicitly-requested slot | `ReconcileNetworkInterfaces` runs two passes: unit-numbered results claim devices first, then the MAC/ExternalID/backing fallback runs for the rest |
| VM class ConfigSpec may carry NIC devices with their own unit numbers | Create path: explicit spec value overrides the class device value; unset spec leaves it as-is; collisions surface as a vSphere create fault (T001 documents the fault type) |
| Snapshot revert restores spec + feature-version annotation from backup yaml | Investigated under T031; revert to a pre-backfill snapshot drops the bit and re-triggers the backfill |
| E2E cannot toggle the flag/capability mid-run | Brownfield backfill covered by vcsim integration tests instead of E2E |
