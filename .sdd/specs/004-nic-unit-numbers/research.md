# Research: NIC Unit Numbers

## vSphere NIC placement model

In vSphere, all `VirtualEthernetCard` devices (vmxnet3, e1000, SR-IOV, etc.) sit on a single implicit `VirtualPCIBus` (device key 100, always present). The `VirtualDevice.UnitNumber` field identifies the NIC's slot on that bus. vSphere allows at most 10 NICs per VM.

**The PCI bus is shared with other virtual devices** (video card, VMCI device, storage controllers, …), which occupy the lower slots. vSphere assigns ethernet cards unit numbers starting at **7**, so valid NIC unit numbers are **7–16**, not 0–9. Any slot-assignment logic must treat slots 0–6 as reserved for non-NIC devices, and the CRD range markers, webhook assignment base, and validation messages must all use the 7–16 range. (An earlier draft of this research incorrectly stated 0–9 with no reserved slots; that was wrong and everything downstream of it has been corrected.)

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

## Key structural insight: `NetworkInterfaceResult` has no `UnitNumber`

The `NetworkInterfaceResult` struct (network.go) that flows through the entire reconcile pipeline has no `UnitNumber` field. The desired `VirtualEthernetCard` device is constructed without a unit number. This means adding unit-number matching and placement to `FindMatchingEthCard` and the Add config spec requires either:

- Extending `NetworkInterfaceResult` with a `UnitNumber *int32` field populated from `interfaceSpec.UnitNumber` in `reconcileNetworkInterfaces`, OR
- Passing `unitNumber` as a separate parameter to `FindMatchingEthCard`.

Extending `NetworkInterfaceResult` is preferred: it flows the unit number through naturally and lets `ReconcileNetworkInterfaces` set it on the Add device payload without needing to pass extra parameters everywhere.

## Disk unit number prior art (webhook layer) — and why NICs differ

`pkg/util/vmopv1/hardware.go` defines the `ControllerSpec` interface and `NextAvailableUnitNumber` helper used by the disk-controller mutation webhook to auto-assign slots. **NICs deliberately do NOT reuse the mutation half of this pattern.** `unitNumber` is optional and the mutation webhook never assigns it: explicit user values flow onto the device `ConfigSpec` at create/add time, and unset values are assigned by vSphere and then recorded into the spec by the post-create/brownfield schema-upgrade backfill. This makes vSphere the single source of derived values, eliminates any webhook-vs-backfill ordering race, and means no `NICBusSpec` / `NextAvailableUnitNumber` reuse is needed.

Only the validation pattern from `virtualmachine_validator_hardware_controllers.go` carries over:
1. Range check: 7 ≤ `unitNumber` ≤ 16.
2. Uniqueness check: collision detection via `occupiedSlots`.
3. Immutability check on powered-on VM: block changing an already-set `unitNumber` to a different value. nil → set transitions are allowed (the schema-upgrade backfill writes observed values into the spec of running VMs).
4. Feature-gate check: when `VMNetworkUnitNumbers` is off, any use of the field is rejected with `field.Forbidden` — the precedent is `validateNetworkVLANs`, which rejects `spec.network.vlans` with `featureNotEnabled` when `VMVlanSubinterface` is off. Silently ignoring the field would let users persist arbitrary or duplicate values that later poison the spec-wins backfill.

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

- **No mutation-webhook assignment (design decision).** An earlier draft had the mutation webhook auto-assign unit numbers (disk pattern), which required gating the update path on `vmopv1util.IsObjectUpgraded` to avoid the webhook guessing slots ahead of the backfill (guesses would be locked in by the spec-wins rule and cause unit-number-first matching to claim the wrong devices). The final design removes webhook assignment entirely: explicit values go onto the device ConfigSpec, everything else is vSphere-assigned and recorded by the backfill. The race — and the gate — no longer exist.
- **VM create path is separate from reconcile — and has two branches.** The initial VM `ConfigSpec` builds NIC devices in `pkg/providers/vsphere/vmprovider_vm.go`, not via `ReconcileNetworkInterfaces`, and it consumes `network.Device` (from `CreateNetworkDevices`), not `NetworkInterfaceResult` — so `Device` also needs a `UnitNumber` field. Branch (a): ethernet devices already present in the **VM class ConfigSpec** are positionally zipped to `createArgs.NetworkDevices` and mutated via `ApplyNetworkDeviceToVirtualEthCard`; these devices may already carry a `UnitNumber` from the class ConfigSpec — an explicit spec value overrides it (spec wins), an unset spec leaves it as-is. Branch (b): remaining spec interfaces get default devices via `CreateDefaultEthCardFromNetworkDevice`. Both branches must set the explicit spec value (gated on the T001 result), otherwise greenfield VMs start life with a spec/device mismatch. The validation webhook cannot see the class ConfigSpec, so a spec-vs-class unit-number collision only surfaces as a vSphere create fault.
- **Conversion restore is required in every older version.** This repo preserves hub-only spec fields across old-version round-trips via the annotation-based restore mechanism, not automatically. **Each of `v1alpha2`, `v1alpha3`, `v1alpha4`, and `v1alpha5` has its own `restore_v1alpha6_VirtualMachineNetworkInterfaces`** (each restores `VNUMANodeID` today) and all four must be extended for `UnitNumber`. `v1alpha1` restores the interfaces list wholesale when the down-converted list is non-empty, so it is incidentally covered — assert it in fuzz tests. Without these, an UPDATE submitted through an older version wipes the field — exactly the k8s#111703 hazard the constitution warns about.
- **vcsim models NIC unit numbers faithfully enough for integration tests** (verified in the govmomi v0.55.0-alpha simulator source): `AssignController` assigns ethernet-card unit numbers with a PCI offset of 7; an explicit `UnitNumber` on an Add is honoured (auto-assignment only happens when nil); and a duplicate unit number on the same controller is rejected with `InvalidDeviceSpec`. The backfill, placement, and collision scenarios in T015 are therefore exercisable under vcsim. Real-VC behaviour still needs T001 confirmation.
- **Snapshot revert restores spec + annotations from the backup yaml** (`restoreVMSpecFromSnapshot` in `vmprovider_vmsnapshot.go`): backfilled unit numbers and the feature-version annotation travel with the snapshot, so reverting to a pre-backfill snapshot drops the `FeatureVersionNICUnitNumbers` bit and re-triggers the backfill. The imported-VM fallback (`synthesizeVMSpecForSnapshot`) synthesizes interfaces with no unit numbers. Covered by T031.
- **`MapEthernetDevicesToSpecIdx` has a second caller.** Besides the status path in `vmprovider_vm.go`, `pkg/vmconfig/bootoptions/bootoptions_reconciler.go` calls it to map boot-order devices. The unit-number-first pass changes behaviour for both callers; boot-option tests should cover the new mapping.
- **Status depends on VMware Tools.** `updateGuestNetworkStatus` builds `status.network.interfaces` entries by iterating `guestInfo.Net`; a VM without Tools (or before Tools starts) has no interface status entries at all, even though unit numbers are available in `moVM.Config.Hardware`. The `unitNumber` status field therefore only appears for Tools-reported interfaces.
- **Immutable-networks interface comparison is per-field.** When `MutableNetworks` is off, the validator compares only `Name`, `Network`, and `MACAddr` per interface (not `DeepEqual`), so a mutation-webhook-assigned `unitNumber` on update does not trip the interfaces-immutable check.

## govmomi research program (T001)

A standalone Go program using govmomi should be written and run against a real vCenter to answer all open questions in `spec.md`.

**Scope note:** the field of concern is `VirtualDevice.UnitNumber` — NOT the PCI slot number (`VirtualDevice.SlotInfo` / `VirtualDevicePciBusSlotInfo.PciSlotNumber`, the guest-visible physical slot). Matching, backfill, and status all key on `UnitNumber` only. It is fine (and useful) for the program to also record `SlotInfo` alongside each observation, but no product behaviour depends on it.

The program should:

1. Create a VM via govmomi using a `VirtualMachineConfigSpec` (no image, just hardware).
2. **Create a VM whose initial `ConfigSpec` includes NICs with explicit `VirtualDevice.UnitNumber` values and verify vSphere honours them** — this is the primary question, since explicit spec values are set on the device at create time (the mutation webhook does not assign values). Repeat for a post-create `ReconfigVM_Task` Add.
3. Add a NIC without a `UnitNumber` and confirm the expected assignment (first NIC → slot 7; subsequent NICs → next available in 7–16). These vSphere-assigned values are what the post-create backfill records into the spec.
4. Power on the VM. Add a NIC while powered on. Observe assigned slot.
5. Power off. Remove the NIC at unit 9. Add a new NIC. Observe: does vSphere reuse slot 9 or pick the next available?
6. Power on and off the VM. Re-inspect `moVM.Config.Hardware.Device[i].GetVirtualDevice().UnitNumber` — verify stability.
7. On a powered-off VM, attempt to change a NIC's `UnitNumber` via an Edit config spec. Observe whether vSphere accepts it.
8. Via the vCenter UI, add a NIC to a running VM. Re-inspect all unit numbers — do existing NICs shift?
9. Hot-add a NIC **with an explicit `UnitNumber`** to a powered-on VM. Observe whether the slot is honoured (the validation webhook allows adding a new interface with an explicit value while powered on).
10. Add an SR-IOV ethernet card (`VirtualSriovEthernetCard`). Confirm it occupies the same 7–16 unit-number space on the virtual PCI bus as other NIC types (the spec asserts SR-IOV NICs are covered by the same assignment/validation/matching).
11. Add a NIC whose explicit `UnitNumber` collides with an existing device (both at create and via `ReconfigVM_Task` Add). Record the exact fault type returned — vcsim returns `InvalidDeviceSpec`; the real fault drives the reconciler's permanent-error (`NoRequeueError`) vs. retry handling. A collision is possible despite webhook uniqueness validation, e.g. against a class-ConfigSpec device or a NIC added out-of-band via the vCenter UI.

The program lives at `cmd/vmoperator-vc-research/main.go` (not shipped as part of the binary, used for research only). Findings are written back into this `research.md`.

## Scenarios for the reconcile matching change

### Normal steady-state (NIC already exists, flag on, unit numbers backfilled)

```
Spec: interfaces[0].unitNumber = 9  (eth0, backed by portgroup PG-A)
VC:   ethCards[0] = vmxnet3, UnitNumber=9, ExternalId=abc, backing=PG-A
```

With unit-number matching: `FindMatchingEthCard` finds `UnitNumber==9` immediately. Match. No config change emitted. Correct.

Without unit-number matching (current): backs match on PG-A. Match. Also correct — but fragile if backing changes during network migration.

### NIC added with explicit unit number, flag on

```
Spec: interfaces[0].unitNumber = 7, interfaces[1].unitNumber = 8 (new, user-specified)
VC:   ethCards[0] = vmxnet3, UnitNumber=7
```

`FindMatchingEthCard` for interfaces[0]: unit number 7 matches VC card. Claim. No change.
`FindMatchingEthCard` for interfaces[1]: no VC card at unit 8. No match → Add config spec with `UnitNumber=8`.
vSphere places NIC at slot 8 (confirmed by research). Status reflects `unitNumber=8`.

### NIC added without a unit number, flag on (post-backfill VM)

```
Spec: interfaces[0].unitNumber = 7, interfaces[1].unitNumber = nil (new, no explicit value)
VC:   ethCards[0] = vmxnet3, UnitNumber=7
```

interfaces[1] has no unit number: the Add config spec carries none, vSphere assigns the next available slot (8), and status reports `unitNumber=8`. The spec field stays nil — the backfill is one-shot and does not re-run — so this interface continues to be matched by the MAC/ExternalID/backing fallback. This is fine: the field is optional and the fallback path is retained precisely for this case.

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

Unit-number match finds the card at slot 7 immediately. Edit (via `ReconcileNetworkInterfaces` orphaned or direct backing update) updates the backing to PG-B. The NIC keeps its unit number. No spurious Remove+Add cycle. This is the key improvement over the current backing-first match.

### Device type change (flag on, unit numbers present)

```
Spec: interfaces[0].unitNumber = 7, type = E1000
VC:   ethCards[0] = vmxnet3, UnitNumber=7
```

The unit number identifies the device; the type differs. Device type cannot be edited in place, so the reconciler emits a Remove of the matched vmxnet3 device plus an Add of the desired E1000 device carrying `UnitNumber=7`, converging the hardware to the spec while keeping the slot. A unit-number match therefore does not mean "no change" — the matched device must still be diffed against the desired state.

### Brownfield backfill failure modes

If the hodgepodge cannot uniquely match a spec interface to a VC device (e.g., two NICs on the same portgroup with generated MACs), `NICUnitNumbersFromMoVM` falls back to a positional zip for the otherwise-unmatched (spec interface, VC device) pairs, so every interface still receives an observed unit number. The zip is only applied to the leftovers after hodgepodge matching has claimed everything it can uniquely identify. Because the mutation webhook never assigns unit numbers, there is no window in which guessed values can precede the backfill — the only spec values the backfill ever encounters are explicit user values (which win).
