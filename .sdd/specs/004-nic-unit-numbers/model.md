# Data Model: NIC Unit Numbers

## API changes — `api/v1alpha6/virtualmachine_network_types.go`

### `VirtualMachineNetworkInterfaceSpec` — new field (additive)

```go
// +optional
// +kubebuilder:validation:Minimum=7
// +kubebuilder:validation:Maximum=16

// UnitNumber is the desired slot on the virtual PCI bus for this network
// interface. Values 7–16 are valid: the virtual PCI bus is shared with
// other devices (video card, controllers, etc.) that occupy the lower
// slots, and vSphere assigns ethernet cards unit numbers starting at 7.
//
// When set, the network interface device is created with this unit
// number. When omitted, vSphere assigns the slot when the device is
// created, and the observed value is then recorded into this field
// after the VM is created.
//
// This value must be unique among all network interfaces attached to this
// VM. The admission webhook rejects requests where two interfaces share
// the same unit number or where the value is outside the valid range.
//
// Once the VM is powered on this field is immutable; the admission webhook
// rejects changing an already-set unit number while the VM is in the
// powered-on state.
UnitNumber *int32 `json:"unitNumber,omitempty"`
```

### `VirtualMachineNetworkInterfaceStatus` — new field (additive)

```go
// +optional

// UnitNumber is the observed slot on the virtual PCI bus for this network
// interface as reported by vSphere.
UnitNumber *int32 `json:"unitNumber,omitempty"`
```

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

## NIC unit-number range constants — `pkg/util/vmopv1/` (or api validation only)

There is **no** `NICBusSpec` / `NextAvailableUnitNumber` reuse and no slot-assignment helper: the mutation webhook does not assign unit numbers. Unset values are assigned by vSphere at device-creation time and recorded into the spec by the schema-upgrade backfill; explicit values are carried onto the device ConfigSpec. The only shared knowledge of the range is for validation:

```go
const (
    // NICUnitNumberFirst is the first PCI slot vSphere assigns to
    // ethernet cards; the lower slots host other virtual devices.
    NICUnitNumberFirst int32 = 7
    // NICUnitNumberMax is the last valid NIC slot (10 NICs: 7–16).
    NICUnitNumberMax int32 = 16
)
```

These back the CRD `Minimum=7` / `Maximum=16` markers and the Go validation range check; if the CRD markers are deemed sufficient, the Go-side range check (and these constants) can be dropped.

## `NetworkInterfaceResult` — `pkg/providers/vsphere/network/network.go`

```go
type NetworkInterfaceResult struct {
    // ... existing fields ...

    // UnitNumber is the desired PCI slot for this interface, populated from
    // spec.network.interfaces[i].unitNumber when VMNetworkUnitNumbers is enabled.
    // When non-nil it is used as the primary match key in ReconcileNetworkInterfaces
    // and, if vSphere honours explicit placement (confirmed by research), it is set
    // on the Add VirtualDeviceConfigSpec payload.
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
      unitNumber: 7          # vSphere-assigned; recorded by post-create backfill
    - name: eth1
      type: VMXNet3
      unitNumber: 8          # vSphere-assigned; recorded by post-create backfill
```

(At admission time both fields were unset — the mutation webhook does not assign unit numbers.)

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
      unitNumber: 7          # vSphere-assigned; recorded by post-create backfill
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

When the `VMNetworkUnitNumbers` flag is off, any interface setting `unitNumber` is rejected with `field.Forbidden(path, featureNotEnabled)` (existing constant, same pattern as `validateNetworkVLANs`).

## Conversion strategy

**The spec field requires conversion restore logic.** This repo preserves hub-only spec fields across old-version round-trips via the annotation-based restore mechanism (`utilconversion.UnmarshalData`); they are not preserved automatically. Without it, an UPDATE submitted through v1alpha5 (or earlier) silently wipes `spec.network.interfaces[i].unitNumber` — the k8s#111703 additive-field hazard.

Required changes:

- `api/v1alpha5/virtualmachine_conversion.go`: extend `restore_v1alpha6_VirtualMachineNetworkInterfaces` to restore `UnitNumber`, exactly as it already does for `VNUMANodeID`.
- Conversion fuzz tests: extend to cover the new field round-tripping through the restore.
- The status field needs no per-field handling: status is fully restored via `dst.Status = restored.Status` in `ConvertTo`.
