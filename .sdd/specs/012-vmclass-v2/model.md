# Model: Flex VM Classes (`VirtualMachineClass` v2)

- **Spec**: [`spec.md`](spec.md)
- **Full field list**: WIKI page 3118085467 — "Design: VM Service: Flex VM Classes: Full Schema". This file records the API surface and the rules; the full schema page lists every typed field, its `configSpec` source and the capability data it is checked against.

All types are in `vmoperator.vmware.com/v1alpha6`, which has not shipped and is reshaped in place. v1alpha5 and older convert to it.

---

## Value shapes

Every governable field has one shape. Each shape is a concrete, named Go struct (no generics, so `controller-gen` can build the schema). All sub-fields are optional pointers, so absent is distinct from zero.

| Shape | Fields | Rules |
|---|---|---|
| Number (`Range`) | `min`, `max`, `default` | `default` is required when `min` or `max` is set, and must be within them. An unset bound is open: limited by the hardware, or by a parent class in Milestone 2. A fixed value is `min == max == default`. A wholly absent struct leaves the value to vpxd. |
| `EnumPolicy` | `allowed`, `default` | Unset `allowed` = all values; `[]` = none. `default` is required when `allowed` has more than one value; with one value, that value is the default. A set `default` must be in `allowed`. |
| `BoolPolicy` | `allowOverride`, `default` | `allowOverride: false` fixes the value and needs a `default`. Otherwise an unset `default` leaves the value to vpxd. |
| `ListPolicy` | `default`, `allowed`, `denied` | `default` is filled into the VM at admission; the VM may add or remove items within `allowed` and `denied`. Matchers are `{type, value}` with `type` one of `Fixed`, `Regex`, `Glob`. `denied` wins. `allowed` only is closed; `denied` only is open. A class can't require an item. |
| Device `ListPolicy` | as `ListPolicy`, plus `count` per default item and a `count` range on the total | Dynamic DirectPath I/O matches `{vendorID, deviceID}`. |
| `SetPolicy` | `default`, `allowed` | For a list taken as one value (`pnumaNodeAffinity`, `cpuFeatureMask`); the VM's set must be a subset of `allowed`. [NEEDS CLARIFICATION: whether set stays its own shape or becomes a form of list.] |

## `VirtualMachineClass.spec`

| Field | Type | Rules |
|---|---|---|
| `description` | string | As today. |
| `externalID` | string | The client's identity for the class; immutable after create. Upgrade sets it to the name. |
| `zones` | `*[]string` | Availability. Unset = every zone of the namespace, including later ones; `[]` = none; a list = only those. |
| `governs` | struct | Absent = the class doesn't govern. |
| `governs.zones` | `*[]string` | Unset = same as `zones`; `[]` = none; a list must be a subset of `zones`. |
| `governs.enforcement` | enum | One value, `Deny`, set by the mutating webhook. |
| `governs.existingVMs` | enum | `AllowOnViolation` or `DenyPowerOn`. Required on a governing class with no parent. In Milestone 2 only the top class sets it; children inherit it. |
| `parentClassRefs` | list of `{name}` | Milestone 2. Rejected in Milestone 1. One entry; same namespace. |
| `resources.size.{cpu,memory}` | Number | CPU in vCPUs. |
| `resources.{requests,limits}.{cpu,memory}` | Number | CPU in vCPUs; compared with a VM's MHz using the cluster's minimum host frequency. |
| `cpuAdvanced.*`, `memoryAdvanced.*`, `bootOptions.*`, `advanced.*` | per field | Same paths as `VirtualMachine.spec`. |
| `advanced.extraConfig` | `ListPolicy` | At the VM's path. |
| `hardware.devices.{vgpuDevices,dynamicDirectPathIODevices}` | device `ListPolicy` | |
| `hardware.*` | per field | Settings that aren't in the VM spec but are exposed or checked. |
| hardware version | Number | [NEEDS CLARIFICATION: shape, path, and upgrade or reject below the minimum.] |
| `hardware.instanceStorage` | as today | Preset only. |
| `reservedProfile` | `EnumPolicy` of profile names | See "Reserved capacity". |
| `reservedProfileID`, `reservedSlots` | string, int32 | Kept for conversion and the v1 vAPI; written by wcpsvc. `reservedSlots` is the namespace's requested slot count summed across zones, for information only. |
| `configSpec` | `VirtualMachineConfigSpec` | Pass-through settings with no typed field; presets only, never governance. An elevated path is moved for older writers and rejected for v1alpha6 writers. Only the top class of a chain may set it. |

Label: the schema-version label records the last CRD version that wrote the class. A write through an older version is rejected once a newer one has written it. The vAPI shows it as read-only `schemaVersion`.

## `VirtualMachineClass.status`

| Field | Meaning |
|---|---|
| `zones[].name` | Covers `spec.zones`, or every zone when unset. |
| `zones[].conditions` | `HardwareCompatible`, `WithinGovernance`, `Governing`. |
| `zones[]` resolved limits | Per-VM limits a VM can obtain in that zone: the class within the governing class, clamped by hardware. Never pool totals, never what governance forbids. |
| `observedGeneration`, `conditions[Ready]` | As required for every controller. |

## `VirtualMachine`

| Field | Rules |
|---|---|
| `spec.reservedProfileName` | Filled by the mutation webhook from the class's `reservedProfile.default` (or the governing class's), like any other default. Must be in `allowed`. Can't change while the VM is powered on. When set, the VM's CPU count, CPU reservation and limit, memory size, reservation and limit, and devices must equal the profile's. vm-operator writes the vCenter profile ID from this field. |
| `status.governingClassNames` | The classes governing the VM's zone, computed live. |
| `conditions[GovernanceCompliant]` | `True`/`False`; reason `NoGoverningClass` when the zone has none (False for classless VMs, True for class-backed VMs). |
| observed status fields | firmware, change block tracking, CPU `reservationLockedToMax`, NUMA fixed auto-affinity, SEV, SEV-SNP, TDX, CPU hot-remove, SMC, DirectPath I/O devices. |

## `Zone` (`external/tanzu-topology`)

| Field | Rules |
|---|---|
| `status.governingClasses` | Mirror of the classes that govern the zone; written by one controller with an optimistic lock; never read by governance. |

## Reserved capacity

A reserved profile is a vCenter object that holds one VM shape: vCPU count, CPU reservation and limit (equal), memory reservation and limit (equal), and DirectPath profiles. vCenter DRS places slots of a profile on hosts, gives a powered-on VM that names the profile a free slot, and takes it back at power-off. vm-operator never enforces slot counts.

- In Milestone 1 a class that lists profiles fixes the profile's fields (`min == max`), so checking a VM against the class's fixed values is the same as checking it against the profile.
- `VirtualMachineReservedProfile` is unchanged in Milestone 1. Later, it can carry the profile's shape and per-zone reserved and used counts, derived from vCenter, without a breaking change.

## Conversion

| From v1alpha5 | To v1alpha6 |
|---|---|
| scalar field `v` | `{min: v, max: v, default: v}` |
| `hardware.{cpus,memory}` | `resources.size.{cpu,memory}` |
| `policies.resources.{requests,limits}` | `resources.{requests,limits}` |
| `configSpec` path that has a typed field | that typed field |
| (none) | `externalID` = name; `zones` unset |
| `reservedProfileID: X` | `reservedProfile: {default: X, allowed: [X]}` (and back) |

Fields with no v1alpha5 home round-trip through the conversion annotation.

## Examples

A fixed class after upgrade:

```yaml
spec:
  externalID: best-effort-small
  resources:
    size:
      cpu: {min: 2, max: 2, default: 2}
      memory: {min: 4Gi, max: 4Gi, default: 4Gi}
```

A governing class for a zone:

```yaml
spec:
  zones: [zone-a]
  governs:
    existingVMs: AllowOnViolation
  resources:
    size:
      cpu: {min: 1, max: 16, default: 2}
      memory: {min: 1Gi, max: 64Gi, default: 4Gi}
  advanced:
    extraConfig:
      allowed: [{type: Glob, value: "guestinfo.*"}]
      denied: [{type: Glob, value: "guestinfo.secret.*"}]
```

A reserved GPU class:

```yaml
spec:
  reservedProfile:
    default: gpu-large
    allowed: [gpu-large]
  resources:
    size:
      cpu: {min: 8, max: 8, default: 8}
      memory: {min: 64Gi, max: 64Gi, default: 64Gi}
  memoryAdvanced:
    reservationLockedToMax: {allowOverride: false, default: true}
```
