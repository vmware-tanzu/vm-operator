# Data Model: NIC Unit Numbers

## API changes — `api/v1alpha6/virtualmachine_network_types.go`

### `VirtualMachineNetworkInterfaceSpec` — new field (additive)

```go
// +optional
// +kubebuilder:validation:Minimum=7
// +kubebuilder:validation:Maximum=16

// UnitNumber is the slot on the virtual PCI bus for this network
// interface. Values 7–16 are valid: the virtual PCI bus is shared with
// other devices (video card, controllers, etc.) that occupy the lower
// slots, and ethernet cards are allocated units 7–16.
//
// Once set, this value is this interface's identifier for its underlying
// vSphere hardware device — not merely a slot a device happens to occupy.
//
// This field may be left unset, including when the VM is created. An
// interface created without a unit number receives one from vSphere when
// its device is created, and that value is recorded into this field once
// the VM has been reconciled. From that point on, an interface added to
// this VM without a unit number is assigned the next available one when
// the request is admitted.
//
// This value must be unique among all network interfaces attached to this
// VM. The admission webhook rejects requests where two interfaces share
// the same unit number or where the value is outside the valid range.
//
// This field may remain unset even after the VM has been reconciled: the
// observed value is recorded only when doing so yields a valid
// specification. An interface whose observed slot is already claimed by
// another interface, or is outside the valid range, keeps an unset value.
//
// Once the VM is powered on this field is immutable for user-originated
// requests; the admission webhook rejects changing an already-set unit
// number, or clearing one, while the VM is in the powered-on state. An
// unset field may be set on a powered-on VM only by VM Operator itself,
// recording an observed value discovered from vSphere.
//
// Changing this field on an existing interface while the VM is powered off
// is a hardware replacement, not a relocation: the interface's current
// device (if no other interface claims it) is removed, and a new device
// is created at the newly-requested slot on the VM's next reconcile. The
// new device has a new identity — a new key, and, for an automatically
// assigned MAC address, a new MAC address (and therefore possibly a new
// DHCP-assigned IP) — and reflecting the change does not take effect
// while the VM stays powered on.
//
// Note that once this field is set, other changes to the interface — such
// as re-pointing it at a different network — are likewise applied by
// replacing the device at this unit number rather than by modifying it in
// place, and so carry the same new-key/new-MAC caveat.
UnitNumber *int32 `json:"unitNumber,omitempty"`
```

### `VirtualMachineNetworkInterfaceStatus` — new field (additive)

```go
// +optional

// UnitNumber is the observed slot on the virtual PCI bus for this network
// interface as reported by vSphere.
UnitNumber *int32 `json:"unitNumber,omitempty"`
```

Deliberately **no range markers on the status field**: status records what vSphere reports, whatever it is. Constraining an observed value would make an out-of-range observation unwritable rather than visible.

Both fields are `+optional` / `omitempty`, so existing VMs without unit numbers continue to work. The spec field does, however, require conversion restore logic for older API versions — see "Conversion strategy" below.

## Feature flag — `pkg/config/config.go`

```go
// VMNetworkUnitNumbers gates NIC unit-number mutation, validation,
// unit-number-based reconcile matching, and schema-upgrade backfill.
VMNetworkUnitNumbers bool
```

Added to `Features` struct alongside existing flags like `VMSharedDisks`.

## `FeatureVersion` bitmask — `pkg/util/vmopv1/features.go`

```go
// FeatureVersionNICUnitNumbers refers to the NIC unit-number backfill
// (spec.network.interfaces[i].unitNumber from VirtualDevice.UnitNumber).
FeatureVersionNICUnitNumbers // 16
```

Appended to the `1 << iota` block after `FeatureVersionTelcoVMServiceAPI` (8). The order of existing bits is immutable. Must also be added to `FeatureVersionAll` (15 → 31) and to the `FeatureVersions()` slice, and OR'd into `ActivatedFeatureVersion` when `VMNetworkUnitNumbers` is enabled.

## NIC unit-number range and slot assignment — `pkg/util/vmopv1/hardware.go`

The mutation webhook assigns unit numbers using the existing `NextAvailableUnitNumber` helper, so the NIC bus needs a `ControllerSpec` implementation and the helper needs a first-usable-unit notion (it currently scans from 0 and skips a single reserved unit, which cannot express "start at 7").

**The first-unit notion is an *optional* interface, not an addition to `ControllerSpec` (I21).** `ControllerSpec`'s four implementations are shipped API types in a different Go module (`api/v1alpha6/virtualmachine_hardware_types.go:71,104,128,171`), and widening the interface would mean four no-op methods there for a placement helper. Declare the optional interface in `pkg/util/vmopv1` and type-assert it inside the helper instead; `MaxSlots()` also keeps its existing "count" meaning (SCSI returns 64 for units 0–63) rather than becoming an exclusive upper bound:

```go
// firstUnitNumberer is implemented by controllers whose first usable unit
// is not zero. Controllers that do not implement it start at 0, so the
// existing SCSI/SATA/NVME/IDE behaviour is unchanged.
type firstUnitNumberer interface {
    FirstUnitNumber() int32
}

// NICBusSpec describes the single implicit virtual PCI bus that hosts
// ethernet cards. It exists only to satisfy ControllerSpec for slot
// computation — the bus has no API surface.
type NICBusSpec struct{}

func (NICBusSpec) MaxCount() int32           { return 1 }
func (NICBusSpec) ReservedUnitNumber() int32 { return -1 }
func (NICBusSpec) FirstUnitNumber() int32    { return NICUnitNumberFirst }
func (NICBusSpec) MaxSlots() int32           { return NICUnitNumberCount }
```

`NextAvailableUnitNumber` then scans `first .. first+MaxSlots()-1`, where `first` is 0 unless the controller implements `firstUnitNumberer`.

The range constants back the CRD markers, the Go validation range check, and the bus spec above:

```go
const (
    // NICUnitNumberFirst is the first PCI slot vSphere assigns to
    // ethernet cards; the lower slots host other virtual devices.
    NICUnitNumberFirst int32 = 7
    // NICUnitNumberMax is the last valid NIC slot (10 NICs: 7–16).
    NICUnitNumberMax int32 = 16
    // NICUnitNumberCount is the number of slots in the band, i.e. what
    // NICBusSpec.MaxSlots reports, keeping MaxSlots a count as it is for
    // every other controller.
    NICUnitNumberCount int32 = NICUnitNumberMax - NICUnitNumberFirst + 1
)
```

These back the CRD `Minimum=7` / `Maximum=16` markers and the Go validation range check; if the CRD markers are deemed sufficient, the Go-side range check (and these constants) can be dropped.

The 7–16 bound is a platform contract, not an observed convention: PCI units are allocated statically per device class, with ethernet cards owning units 7–16 and every other class holding its own band (`external/vim/api/v1alpha1/testdata/device_keys.txt`). The ten units line up one-for-one with the ten ethernet device keys (4000–4009) and with `MaxItems=10` on the interfaces list. Note the backfill writes an *observed* value into this validated field, so the **G7** admissibility guard still applies — its live case is a duplicate value, with the range check kept as cheap defence.

## `NetworkInterfaceResult` — `pkg/providers/vsphere/network/network.go`

```go
type NetworkInterfaceResult struct {
    // ... existing fields ...

    // UnitNumber is the desired PCI slot for this interface, populated from
    // spec.network.interfaces[i].unitNumber when VMNetworkUnitNumbers is enabled.
    // When non-nil it is used as the EXCLUSIVE match key in ReconcileNetworkInterfaces
    // (no MAC/ExternalID/backing fallback is attempted for a numbered interface, even
    // on a miss), and, if vSphere honours explicit placement (confirmed by research),
    // it is set on the Add VirtualDeviceConfigSpec payload.
    UnitNumber *int32
}
```

This is an internal struct (not part of the CRD API), so no deepcopy generation is required.

## Example YAML

### VM spec after creation and first reconcile (no explicit unit numbers)

```yaml
spec:
  network:
    interfaces:
    - name: eth0
      type: VMXNet3
      unitNumber: 7          # vSphere-assigned; recorded after the first reconcile
    - name: eth1
      type: VMXNet3
      unitNumber: 8          # vSphere-assigned; recorded after the first reconcile
```

(At admission time both fields were unset — the mutation webhook does not assign on create, since a VM being created has not been reconciled yet. A NIC *added* to this VM later is assigned the next free slot at admission.)

### VM spec with one explicit unit number (after first reconcile)

```yaml
spec:
  network:
    interfaces:
    - name: mgmt
      type: VMXNet3
      unitNumber: 9          # user-specified; device created at slot 9
    - name: data
      type: VMXNet3
      unitNumber: 7          # vSphere-assigned; recorded after the first reconcile
```

### VM status after reconcile

```yaml
status:
  network:
    interfaces:
    - name: eth0
      unitNumber: 7
      deviceKey: 4000
      ip:
        addresses:
        - ip: 192.168.1.10/24
    - name: eth1
      unitNumber: 8
      deviceKey: 4001
      ip:
        addresses:
        - ip: 192.168.1.11/24
```

## Validation error messages

New constants in `webhooks/virtualmachine/validation/virtualmachine_validator_network_interfaces.go`:

```go
const (
    invalidNICUnitNumberRangeFmt    = "NIC unit number must be between 7 and 16"
    invalidNICUnitNumberInUse       = "NIC unit number %d is already in use by another interface"
    invalidNICUnitNumberChangePowOn = "NIC unit number cannot be changed while the VM is powered on"
)
```

`invalidNICUnitNumberChangePowOn` covers all three rejected powered-on transitions: set → different value, set → nil (cleared), and — per I3 — nil → set when the request is not made by the VM Operator service account (`ctx.IsVMOperatorAccount`). Only a nil → set change made by that account (the schema-upgrade backfill) is allowed while powered on.

When the `VMNetworkUnitNumbers` flag is off, an interface whose `unitNumber` is **new or changed relative to the old object** is rejected with `field.Forbidden(path, featureNotEnabled)` (existing constant, same pattern as `validateNetworkVLANs`). An unchanged, previously-backfilled value is **not** rejected — see spec.md **G10**: validation runs against the whole object on every UPDATE, so rejecting on presence would block unrelated edits, the operator's own patches, and finalizer removal, leaving backfilled VMs undeletable while the flag is off.


## Operator-visible signals (spec.md G8)

Revised from an earlier three-Event draft. The split follows whether the fact is still observable after the backfill has run — and the steady-state half follows the disk and CD-ROM precedent (`update_status_hardware_validation.go`) rather than inventing an Event vocabulary.

### One Event, at backfill time

| Reason | Type | Raised when |
|---|---|---|
| `NICUnitNumberBackfillAmbiguous` | `Warning` | An interface was claimed by positional zip rather than uniquely matched by MAC / external ID / backing. |

This is the only fact nothing in the resulting spec records: a zipped value is indistinguishable from a matched one afterwards, so it cannot be recomputed and must be emitted when it happens.

### A condition, recomputed every reconcile

Divergence between a numbered interface's declared slot and the observed hardware is reported the way volume placement divergence already is — `checkVolumes` → `reconcileHardwareCondition` → `VirtualMachineHardwareDeviceConfigVerified` false with `VirtualMachineHardwareDeviceConfigMismatchReason`, optionally with a NIC-specific sibling alongside `VirtualMachineHardwareVolumesVerified` / `...CDROMVerified`. It covers:

- an interface's pre-existing explicit value disagreeing with the observed device slot (spec wins; nothing is written);
- an interface whose observed value the backfill skipped per **G7**;
- **a slot the mutation webhook invented for a G7-skipped interface** (I18) — reachable, and structurally invisible to a backfill-time Event;
- an interface whose declared slot holds no device at all (the **G13** state).

A condition is the better mechanism for all of these: it is recomputed from spec-vs-hardware each reconcile, so it catches values from any source and clears once the VM converges.

## Conversion strategy

**The spec field requires conversion restore logic — in every older API version.** This repo preserves hub-only spec fields across old-version round-trips via the annotation-based restore mechanism (`utilconversion.UnmarshalData`); they are not preserved automatically. Without it, an UPDATE submitted through any older version silently wipes `spec.network.interfaces[i].unitNumber` — the k8s#111703 additive-field hazard.

Required changes:

**Matching interfaces during the restore: `Name` only.** Each version's `restore_v1alpha6_VirtualMachineNetworkInterfaces` already builds a `map[name]*iface` from the saved hub object and looks up each down-converted interface by name (`api/v1alpha5/virtualmachine_conversion.go:100-108`) — not by index, which is the failure mode worth guarding against and is already avoided. `Name` is also the API's own identity for these entries (`+listType=map` / `+listMapKey=name`), so it is the strongest key available.

Adding `unitNumber` to that match key would not help, for a structural reason: the restore runs in `ConvertTo`, where `dst` is the hub built from the *old-version* spoke. `unitNumber` is hub-only, so the spoke cannot carry it and `dst`'s value is always nil at that point — which is exactly why it needs restoring. There is nothing on the incoming side to compare against, so a name-plus-unit-number key degenerates to the name alone, and a variant that only restored when the saved entry had a unit number would skip restoring the other hub-only fields (`type`, `vnumaNodeID`, `vmxnet3`, …) for interfaces that have none.

The hazard the question is reaching for — an old-version client deleting `eth1` and adding a *different* `eth1` in the same update, so the restore re-attaches the previous `unitNumber` to a semantically new interface — is real but resolves safely: the reconciler locates the device at that slot, finds its backing/MAC/ExternalID disagree with the new desired state, and replaces it (**G12**). The new interface inherits a slot rather than getting a fresh assignment, and its device is rebuilt to match. Cover it with a conversion test rather than a matching-key change.

- **Each of `api/v1alpha2`, `api/v1alpha3`, `api/v1alpha4`, and `api/v1alpha5` has its own copy** of `restore_v1alpha6_VirtualMachineNetworkInterfaces` in its `virtualmachine_conversion.go` (each restores `VNUMANodeID` today). Extend **all four** to restore `UnitNumber`, exactly as they already do for `VNUMANodeID`.
- Conversion fuzz tests: extend each of the four versions' fuzz tests to cover the new field round-tripping through the restore.
- `v1alpha1` restores the entire interfaces list wholesale when the down-converted list is non-empty, so it needs no per-field change — but its conversion fuzz tests must assert the field survives the round-trip.
- The status field needs no per-field handling: status is fully restored via `dst.Status = restored.Status` in `ConvertTo`.
