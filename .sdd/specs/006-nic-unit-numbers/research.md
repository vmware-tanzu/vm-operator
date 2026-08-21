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

## govmomi research program (T001)

A standalone Go program using govmomi should be written and run against a real vCenter to answer all open questions in `spec.md`.

**Scope note:** the field of concern is `VirtualDevice.UnitNumber` — NOT the PCI slot number (`VirtualDevice.SlotInfo` / `VirtualDevicePciBusSlotInfo.PciSlotNumber`, the guest-visible physical slot). Matching, backfill, and status all key on `UnitNumber` only. It is fine (and useful) for the program to also record `SlotInfo` alongside each observation, but no product behaviour depends on it.

The program should:

1. Create a VM via govmomi using a `VirtualMachineConfigSpec` (no image, just hardware).
2. **Create a VM whose initial `ConfigSpec` includes NICs with explicit `VirtualDevice.UnitNumber` values and verify vSphere honours them** — the primary question, and it now applies to every interface, since each one carries a user-specified or webhook-assigned value by the time the ConfigSpec is built. Repeat for a post-create `ReconfigVM_Task` Add.
3. Add a NIC without a `UnitNumber` and confirm the expected assignment (first NIC → slot 7; subsequent NICs → next available in 7–16). These vSphere-assigned values are what the post-create backfill records into the spec.
4. Power on the VM. Add a NIC while powered on. Observe assigned slot.
5. Power off. Remove the NIC at unit 9. Add a new NIC. Observe: does vSphere reuse slot 9 or pick the next available?
6. Power on and off the VM. Re-inspect `moVM.Config.Hardware.Device[i].GetVirtualDevice().UnitNumber` — verify stability.
7. On a powered-off VM, attempt to change a NIC's `UnitNumber` via an Edit config spec. Observe whether vSphere accepts it.
8. Via the vCenter UI, add a NIC to a running VM. Re-inspect all unit numbers — do existing NICs shift?
9. Hot-add a NIC **with an explicit `UnitNumber`** to a powered-on VM. Observe whether the slot is honoured (the validation webhook allows adding a new interface with an explicit value while powered on).
10. Add an SR-IOV ethernet card (`VirtualSriovEthernetCard`). Confirm it occupies the same 7–16 unit-number space on the virtual PCI bus as other NIC types (the spec asserts SR-IOV NICs are covered by the same assignment/validation/matching).
11. Add a NIC whose explicit `UnitNumber` collides with an existing device (both at create and via `ReconfigVM_Task` Add). Record the exact fault type returned — vcsim returns `InvalidDeviceSpec`; the real fault drives the reconciler's permanent-error (`NoRequeueError`) vs. retry handling. A collision is possible despite webhook uniqueness validation, e.g. against a class-ConfigSpec device or a NIC added out-of-band via the vCenter UI.

The program lives at `hack/vcresearch/nic-unit-numbers/main.go` — under the dev-tooling tree rather than `cmd/`, which holds shipped binaries (`vmclass`, `web-console-validator`), and in a directory named for the feature it investigates so it is obviously scoped and disposable. It stays in the root module so it compiles against the same pinned govmomi the product uses, which is the point of a program characterising behaviour the product will depend on. Delete it once T023 has recorded the findings here.

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
interfaces[1]: no device at unit 8 — the interface has no hardware yet, so it takes the ordinary Add path with `UnitNumber=8` on the payload. vSphere places the NIC at slot 8 (pending Q1), and status reports `unitNumber=8`.

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
- **SR-IOV cards are ethernet NICs for allocation purposes** — `VirtualSriovEthernetCard` is a `VirtualEthernetCard`, so it draws from the same 4000-series keys and the same 7–16 band. T001's SR-IOV experiment confirms rather than discovers this.

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
