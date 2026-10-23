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
| `EnumPolicy` | `allowed`, `default`, `required` | Unset `allowed` = all values; `[]` = none. `default` is required when `allowed` has more than one value; with one value, that value is the default. A set `default` must be in `allowed`. A VM that omits the field inherits `default`; an explicit `""` means no value; `required: true` rejects an empty value after defaulting. [NEEDS CLARIFICATION: whether `""` means "no value" for every enum field or only for `reservedProfileName`.] |
| `BoolPolicy` | `allowOverride`, `default` | `allowOverride: false` fixes the value and needs a `default`. Otherwise an unset `default` leaves the value to vpxd. |
| `ListPolicy` | `default`, `allowed`, `denied` | `default` is filled into the VM at admission; the VM may add or remove items within `allowed` and `denied`. Matchers are `{type, value}` with `type` one of `Fixed`, `Regex`, `Glob`. `denied` wins. `allowed` only is closed; `denied` only is open. A class can't require an item. |
| Device `ListPolicy` | as `ListPolicy`, plus `count` per default item | One `ListPolicy` over the VM spec's inline vGPU and DirectPath I/O devices. Dynamic DirectPath I/O matches `{vendorID, deviceID}`. Independent count bounds and required entries are deferred. |

A separate `SetPolicy` shape is deferred; lists taken as one value (`pnumaNodeAffinity`, `cpuFeatureMask`) are not governed in Milestone 1.

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
| `reservedProfileName` | `EnumPolicy` of profile names | Same path as the VM's `spec.reservedProfileName`. Every name must resolve to a `VirtualMachineReservedProfile` in the namespace. Unset `allowed` is not "all profiles": `allowed` is required when `default` is set. See "Reserved capacity". |
| `reservedProfileID` | string | Kept only for conversion and the v1 vAPI; `reservedProfileName` is authoritative. |
| `reservedSlots` | int32 | Deprecated. Same name and meaning as today: the namespace's desired slot total across zones. Preserved through conversion and the namespace copy, including zero; never overwritten with observed totals. |
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
| `spec.reservedProfileName` | string. Filled by the mutation webhook from the class's `reservedProfileName.default` (or the governing class's), like any other default; explicit `""` = no profile. Must be in `allowed`. On a class-backed VM it is class-supplied and can't be edited inline; a classless VM sets or inherits it. Can't change from one set value to another while the VM is powered on; vm-operator may set it from unset (schema upgrade). When set, the VM's CPU count, CPU reservation and limit, memory reservation and limit, and device identities and quantities must equal the profile's; overhead is not compared. vm-operator writes the vCenter profile ID from this field and removes it when the field is empty. |
| `status.governingClassNames` | The classes governing the VM's zone, computed live. |
| `conditions[GovernanceCompliant]` | `True`/`False`; reason `NoGoverningClass` when the zone has none (False for classless VMs, True for class-backed VMs). |
| observed status fields | firmware, change block tracking, CPU `reservationLockedToMax`, NUMA fixed auto-affinity, SEV, SEV-SNP, TDX, CPU hot-remove, SMC, DirectPath I/O devices. |

## `Zone` (`external/tanzu-topology`)

| Field | Rules |
|---|---|
| `status.governingClasses` | Mirror of the classes that govern the zone; written by one controller with an optimistic lock; never read by governance. |

## Reserved capacity

A reserved profile is a vCenter object that holds one VM shape: vCPU count, CPU reservation and limit (equal), memory reservation and limit (equal), and DirectPath profiles. vCenter DRS places slots of a profile on hosts, gives a powered-on VM that names the profile a free slot, and takes it back at power-off. vm-operator never enforces slot counts.

- `VirtualMachineReservedProfile` is not a vm-operator API type. It is owned by the wcpsvc zone controller, which reconciles each profile's shape and per-zone counts from vCenter and backfills existing allocations. vm-operator only reads it (read RBAC), as it reads the environment-browser objects. A profile name that doesn't resolve is a clear admission failure, with no fallback.
- In Milestone 1 a class that lists profiles fixes the profile's fields (`min == max`).
- A VM with a selected profile must match the profile's shape after defaulting, whether or not it names a class. A resize past the profile is rejected at admission.
- Class edits through the v2 path are rejected only when they change an allocated profile's shape or make the class's default VM invalid for it. The v1 vAPI keeps today's block on update and delete while any Supervisor has capacity for the class.

## Conversion

| From v1alpha5 | To v1alpha6 |
|---|---|
| scalar field `v` | `{min: v, max: v, default: v}` |
| `hardware.{cpus,memory}` | `resources.size.{cpu,memory}` |
| `policies.resources.{requests,limits}` | `resources.{requests,limits}` |
| `configSpec` path that has a typed field | that typed field |
| (none) | `externalID` = name; `zones` unset |
| `reservedProfileID: X` | `reservedProfileName: {default: X, allowed: [X], required: true}` |
| `reservedSlots` | `reservedSlots`, unchanged, including zero |

Back to v1alpha5, `reservedProfileID` = `reservedProfileName.default`. Up again, if `reservedProfileID` differs from the default stored in the conversion annotation (an older-API writer such as wcpsvc changed it), the field is reset to `{default: Y, allowed: [Y], required: true}`; otherwise it is restored from the annotation.

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
  reservedProfileName:
    default: gpu-large
    allowed: [gpu-large]
    required: true
  resources:
    size:
      cpu: {min: 8, max: 8, default: 8}
      memory: {min: 64Gi, max: 64Gi, default: 64Gi}
  memoryAdvanced:
    reservationLockedToMax: {allowOverride: false, default: true}
```
