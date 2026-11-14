# Model: Flex VM Classes (`VirtualMachineClass` v2)

- **Spec**: [`spec.md`](spec.md)
- **Full field list**: WIKI page 3118085467 — "Design: VM Service: Flex VM Classes: Full Schema". This file records the API surface and the rules; the full schema page lists every typed field, its `configSpec` source and the capability data it is checked against.

All types are in `vmoperator.vmware.com/v1alpha6`, which has not shipped and is reshaped in place. v1alpha5 and older convert to it.

---

## Value shapes

Every governable field has one shape. Each shape is a concrete, named Go struct (no generics, so `controller-gen` can build the schema). All sub-fields are optional pointers, so absent is distinct from zero. Each shape has a field set no other shape has, so a client can tell the shape from the CRD schema alone; a schema test checks that every governed field matches exactly one shape.

| Shape | Fields | Rules |
|---|---|---|
| Number (`Range`) | `min`, `max`, `default` | `default` is required when `min` or `max` is set, and must be within them. An unset bound is open: limited by the hardware, or by a parent class in Milestone 2. A fixed value is `min == max == default`. A wholly absent struct leaves the value to vpxd. |
| `CountRange` | `min`, `max` | How many items a list, or an entry of it, may hold. No `default`: the default count is the length of the list's `default`. Every field is optional; an unset `count`, `min` or `max` sets no class limit, and the platform's limits still apply. |
| `EnumPolicy` | `allowed`, `default`, `required` | Unset `allowed` = all values; `[]` = none. `default` is required when `allowed` has more than one value; with one value, that value is the default. A set `default` must be in `allowed`. A VM that omits the field inherits `default`; `required: true` rejects an empty value after defaulting. An explicit `""` is not a value unless the field defines one; only `reservedProfileName` does (`""` = no profile), and its VM field is a pointer so `""` stays distinct from unset. Every other planned enum has an explicit value for "off" (`Normal`, `Disabled`, `PerVM`) or always has a value (NIC `type`, shares `level`). |
| `StringPolicy` | `allowed`, `denied`, `default`, `required` | A free-form string. `allowed` and `denied` are lists of string matchers (below); `denied` is checked first. A set `default` must pass both. `default` and `required` behave as in `EnumPolicy`. |
| `BoolPolicy` | `allowOverride`, `default` | `allowOverride: false` fixes the value and needs a `default`. Otherwise an unset `default` leaves the value to vpxd. |
| `ListPolicy` | `count`, `default`, `allowed`, `denied` | See "List policies". |
| `SetPolicy` | `default`, `allowed`, `required` | The value is one set. `default` is a set; `allowed` is a list of approved sets; sets compare with unique members and order ignored; no matchers and no `denied`. Unset `allowed` = any set of valid members; `required: true` rejects an empty set. Members' type and domain come from the field. Each `allowed` entry is `{members: [...]}`, with `members` a set list (`+listType=set`). Go types are concrete per member type (`Int32SetPolicy`), as with `IntRange` and `LongRange`. |

**String matcher.** One string, matched with Go `path.Match` semantics: a string without `*`, `?` or `[` is an exact match, and `\*` matches a literal `*`. No regex: no requirement needs it, alternation is two entries or a character class (`nvidia-a100-[48]0c`), and a regex can't be rendered as a form or checked for overlaps. The class webhook rejects a malformed pattern. None of the governed key fields (vGPU `profileName`, extraConfig and `advancedProperties` keys) contain wildcard characters.

### List policies

A `ListPolicy` governs a VM list of independent items.

- **`default`** has the VM list's own item shape (a `+listType=map` list with the VM list's keys), so it can be copied from a VM. It is filled in only when the VM omits the list; an explicit list is validated as supplied and never merged with the default. Where the VM list can't keep `[]` apart from unset (a non-pointer slice with `omitempty`), an empty list counts as omitted and the field's own opt-out applies; for NICs that is `network.disabled: true`. A later change to the class's `default` doesn't reach existing VMs.
- **`allowed` and `denied`** hold entries. An entry's matcher is the item's key fields, each optional: an unset key field matches anything, a numeric key field matches exactly, and a string key field is a string matcher. An `allowed` entry also carries an optional `count` and the field policies for the items it matches; a `denied` entry is a matcher only. Entry lists are atomic.
- **Evaluation, per VM item.** `denied` is checked first: a match rejects the item. Then the **first** matching `allowed` entry governs the item: its field policies apply, and the item counts toward its `count`. Entries are never merged. With `allowed` set, an item no entry matches is rejected (`allowed` only is closed); with `allowed` unset, the item is allowed and has no field policies (`denied` only is open). A catch-all is an entry with every key field unset, or `"*"` for a string key.
- **`count`** at the list level bounds the number of items; on an entry, the number of items that entry governs. Every item counts as 1. Required items are expressed only through `count.min`.
- **Where checked.** CEL: `size(default)` within the list-level `count`, and unique keys in `default`. Class webhook (Go), because CEL can't evaluate globs: matcher syntax; every `default` item is not denied, is governed by an `allowed` entry, fits that entry's `count` and its field policies. The webhook also warns when an entry can never govern an item because an earlier entry matches everything it matches.

Field policies sit where the matching happens: on `allowed` entries for lists whose items have a governed key field (devices, extraConfig, `advancedProperties`). NIC types have no governed key field, so NIC policies sit on a type key instead ("Network interfaces").

A collection field's shape follows what its value is. A collection of independent items, each of which can be added or removed on its own, is a `ListPolicy` (`advanced.extraConfig`, devices). A collection that is one setting, whose meaning changes when any member changes, is a `SetPolicy`. The admin chooses the approved sets.

| Collection | Shape | Milestone |
|---|---|---|
| `advanced.extraConfig` | `ListPolicy`; key `key` | 1 |
| `hardware.vGPUs` | `ListPolicy`; key `profileName`; entry field policies: `enableUVM` (`BoolPolicy`) | 1 |
| `hardware.dynamicDirectPathIODevices` | `ListPolicy`; key `vendorID` + `deviceID`. `customLabel` is passed through to the device backing; it is not a key and not governed. | 1 |
| `hardware.pciDeviceGroups` | `ListPolicy`; key `vendorGroupName` | 1 |
| `network.interfaces` | Typed list (see "Network interfaces") | 1 |
| `advanced.pnumaNodeAffinity` | `Int32SetPolicy` of node IDs, e.g. `{default: [0,1], allowed: [{members: [0,1]}, {members: [2,3]}], required: true}`. Unset `allowed` = any set of valid node IDs; `required: false` permits an explicit `[]` (no pinning). Each node ID is checked against `ConfigTarget.numNumaNodes`. | 1 |
| `hardware.cpuFeatureMask` | Preset of the top class only in Milestone 1 (no VM field, so nothing below it to govern); checked against config options `cpuFeatureMaskSupported`. [NEEDS CLARIFICATION: shape for Milestone 2 child classes. Each CPUID leaf's mask is one setting, but the leaves are independent items.] | 2 |

There is no total across device types: no use case needs one.

### Devices

One VM list per device type, each item one device with a required, unique `name`: `hardware.vGPUs[]` and `hardware.dynamicDirectPathIODevices[]`, plus `hardware.pciDeviceGroups[]` for vendor device groups. An item has no `count`; two GPUs of one profile are two items. Per-device fields (the PCI slot `unitNumber`, `deviceGroup`, `enableUVM`, `advancedProperties`) sit on each item. The class mirrors each list at the same path. The VM lists come from the VM PCI device design (WIKI page 2477879529, vmop-3791), which proposes one `pciDevices` union instead; spec 012 asks for one list per type (research.md, Finding 8). [NEEDS CLARIFICATION: agreement on per-type lists with vmop-3791's owner, including per-type or one status list, `unitNumber` assignment by the mutation webhook from the PCI passthrough slot space shared across the lists (and SR-IOV NICs), unit-number uniqueness and the 128-device cap across lists.] A per-type `default` loses nothing because the mutation webhook assigns `unitNumber`, so the order between types doesn't decide PCI slots.

### Network interfaces

`network.interfaces` mirrors the VM's one `network.interfaces[]` list (all NIC types, keyed by `name`):

| Field | Shape | Governs |
|---|---|---|
| `count` | `CountRange` | how many NICs; the VM API caps it at 10 |
| `type` | `EnumPolicy`, `allowed` only, no `default` | `interfaces[].type`. The VM's `type` is optional: it stays unset at create, so a v1alpha5 class's ethernet card still applies by position, and the mutation webhook sets `VMXNet3` after the schema upgrade (T024). SR-IOV doesn't ship in Milestone 1 |
| `default` | one ordered list of VM-shaped NIC items | the VM's NICs when it omits `interfaces` |
| `vmxnet3` | type key | every NIC whose `type` is `VMXNet3` |

The class `default` is applied before the VM webhook's own default NIC, which is added only when `interfaces` is still empty; NIC items with no network get the namespace's default network as today. `default` is one list, not one per type: it matches the VM's list and the vCenter and VCFA adapter tables, needs no rule to keep names unique across lists or to interleave them, and its shape can't change after it ships. Order matters only for the NIC's virtual PCI slot, which vSphere assigns in creation order, until the VM can pin it (`pciSlotNumber`, vmop-3576; a VM-only field the class doesn't govern). Pairing spec NICs with devices, `ethernetN.*` keys and guest customization don't depend on order when `MutableNetworks` is on (research.md, Finding 9).

A type key holds `count` (`CountRange`, NICs of that type) and field policies, flattened, for the common interface fields and for that type's sub-struct. A policy named after a common field governs `interfaces[].<field>`; one named after a sub-struct field governs `interfaces[].<type>.<field>`. The two name sets must not collide (schema test).

| `vmxnet3` policy | Shape | VM field |
|---|---|---|
| `vnumaNodeID` | Number [NEEDS CLARIFICATION: a bound forces a default node for every NIC (T021)] | `interfaces[].vnumaNodeID` |
| `network.name` | `StringPolicy` | `interfaces[].network.name` |
| `advancedProperties` | `ListPolicy`, key `key` | `interfaces[].advancedProperties` |
| `uptv2Enabled`, `rssOffloadEnabled`, `udpRssEnabled` | `BoolPolicy` | `interfaces[].vmxnet3.*` |
| `ctxPerDev`, `coalescingScheme` | `EnumPolicy` | `interfaces[].vmxnet3.*` |
| `pnicFeatures` | `SetPolicy` | `interfaces[].vmxnet3.pnicFeatures` |
| `coalescingParams` | `StringPolicy` | `interfaces[].vmxnet3.coalescingParams` |

Every other interface field is a per-VM input and not governed: `name`, `guestDeviceName`, `macAddr`, `mtu`, the IP and DNS fields and `ipamModes`. SR-IOV has no type key in this spec; the plan for one is in research.md, Finding 9.

**Matching names.** An item's key fields have the same names in the environment-browser entry, the class entry and the VM item, so choosing an option copies the entry's key fields. Environment-browser entries may carry extra display fields (`vendorName`, `deviceName`, `customLabel`).

| Item | Environment-browser entry | Class entry key | VM item |
|---|---|---|---|
| Dynamic DirectPath I/O | ConfigTarget `dynamicPassthroughDevices[]`: `vendorID`, `deviceID` (+ `vendorName`, `deviceName`, `customLabel`) | `vendorID`, `deviceID` | `name`, `vendorID`, `deviceID`, `customLabel`, per-device fields |
| vGPU | ConfigTarget `vgpuProfile[]`: `profileName` (+ display fields) | `profileName` | `name`, `profileName`, `enableUVM`, per-device fields |
| Device group | config options `deviceGroupTypes`: `vendorGroupName` | `vendorGroupName` | `name`, `vendorGroupName` |
| pNUMA node | ConfigTarget `numNumaNodes` (IDs 0..n-1) | node ID | `pnumaNodeAffinity []int32` |

**Mapping to VM fields.** Each class policy governs the VM field at the same path, with two named exceptions: the flattened fields under a NIC type key (above), and `hardware.version`, which governs the VM's actual hardware version rather than its `spec.minHardwareVersion` ("Hardware version"). One reflection-based evaluator applies every policy; a schema test pairs each class policy with exactly one VM field and reserves `count`, `default`, `allowed`, `denied` and `type`.

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
| `resources.size.{cpu,memory}` | Number | CPU is a whole vCPU count; memory in bytes. |
| `resources.{requests,limits}.{cpu,memory}` | Number | Absolute only; no percentage form. CPU in cores, millicores allowed (`"2"`, `"1500m"`), as in v1alpha5; resolved to MHz as cores × the cluster's minimum host frequency. Memory in bytes. An unset limit is unlimited (`-1`), as today. Same unit as the VM's `spec.resources`. See "Size, requests and limits". |
| `resources.shares.{cpu,memory}` | `{level: EnumPolicy, value: Number}` | Same path and shape as the VM's `spec.resources.shares`. `level` is `Low`, `Normal`, `High` or `Custom`; `value` is the custom share count, allowed only when `level` can be `Custom` (CEL on the class defaults and on the VM). [NEEDS CLARIFICATION: the number rule requires a `default` whenever `value` has a bound, so a class can't default to `Normal` while bounding custom values (T021).] Milestone 1 checks `value > 0` and leaves the upper bound to vCenter (`ResourcePool.QueryResourceConfigOption`, not in the environment-browser objects). |
| `cpuAdvanced.*`, `memoryAdvanced.*`, `bootOptions.*`, `advanced.*` | per field | Same paths as `VirtualMachine.spec`, for fields the VM spec has. `bootOptions.firmware`: an image that declares firmware wins over the class default, as today, and must be in `allowed`; a VM value still wins over the image. `bootOptions.networkBootProtocol`: `allowed` only, no class default, because the VM API defaults it to `IP4`. `enterBootSetup` is a one-time request and not a class setting. `cpuAdvanced.topology.exposeVnumaOnCpuHotadd` stays in `configSpec` until the VM has the field [NEEDS CLARIFICATION: whether the observed value is the setting or the effective state]. |
| `advanced.extraConfig` | `ListPolicy` | At the VM's path. |
| `hardware.{vGPUs,dynamicDirectPathIODevices,pciDeviceGroups}` | `ListPolicy` | At the VM's paths. See "Devices". |
| `network.interfaces` | typed list | At the VM's path. See "Network interfaces". |
| `hardware.*` | per field | Settings that aren't in the VM spec but are exposed or checked. |
| `hardware.version` | Number (`int32`) | See "Hardware version". |
| `hardware.instanceStorage` | as today | Preset only. |
| `reservedProfileName` | `EnumPolicy` of profile names | Same path as the VM's `spec.reservedProfileName`. Every name must resolve to a `VirtualMachineReservedProfile` in the namespace. Unset `allowed` is not "all profiles": `allowed` is required when `default` is set. See "Reserved capacity". |
| `reservedSlots` | int32 | Deprecated. Same name and meaning as today: the namespace's desired slot total across zones. Preserved through conversion and the namespace copy, including zero; never overwritten with observed totals. |
| `configSpec` | `VirtualMachineConfigSpec` | Pass-through settings with no typed field; presets only, never governance. An elevated path is moved for older writers and rejected for v1alpha6 writers. Only the top class of a chain may set it. |

Label: the schema-version label records the last CRD version that wrote the class. A write through an older version is rejected once a newer one has written it. The provider's class API shows it as read-only `schemaVersion`.

### Hardware version

`hardware.version: {min, max, default}` is an `int32` number, as the VM's `spec.minHardwareVersion` and `status.hardwareVersion` are; a form shows `vmx-N`. It governs the VM's actual hardware version, not its minimum, so it is not at the VM's path.

- **At create**, the version is the highest of the VM's `minHardwareVersion`, the class `default`, the image's hardware version and the minimum any configured device needs; it must be ≤ `max`. A VM whose `minHardwareVersion` is above `max` is rejected, like any other value outside a policy, and so is an image whose version is above `max`, naming the image's value (the image rule, design §3.2). Today `DetermineHardwareVersion` ignores the image when the class sets a version; the class `default` no longer overrides an image that needs a higher one.
- **Existing VMs** follow `governs.existingVMs` like any other governed field: a VM outside `[min, max]` reports `GovernanceCompliant=False`, and `DenyPowerOn` rejects power-on requests. vm-operator never upgrades a VM on its own. A VM below `min` comes back into compliance when the user raises `spec.minHardwareVersion`, which upgrades it at the next power-off (as today). A VM above a lowered `max` can't: vSphere doesn't downgrade, so it stays non-compliant until it is recreated, and the class webhook warns when a write lowers `max`.
- **Compatibility** per zone compares `max` (and the effective default) with `ConfigTarget`'s maximum hardware version, parsed with `vimtypes.ParseHardwareVersion`.
- **Conversion**: a v1alpha5 `configSpec.version: vmx-N` becomes `{min: N, default: N}` with `max` unset, so the class behaves as today (the higher of the class and the VM's minimum).

### Size, requests and limits

`size` is what the guest sees (vCPU count, memory). `requests` is the host reservation and `limits` the host ceiling. The class and the VM use the same units, so governance compares them directly with no host frequency; only the provider converts CPU cores to MHz when it builds the `ConfigSpec`.

| Field | Unit | `ConfigSpec` |
|---|---|---|
| `size.cpu` | whole vCPU count | `NumCPUs` |
| `size.memory` | bytes | `MemoryMB` |
| `requests.cpu` | cores (millicores allowed) | `CpuAllocation.Reservation` = ceil(cores × minimum host MHz) |
| `limits.cpu` | cores (millicores allowed); unset = unlimited | `CpuAllocation.Limit`; unset = `-1` |
| `requests.memory` | bytes | `MemoryAllocation.Reservation` (MB) |
| `limits.memory` | bytes; unset = unlimited | `MemoryAllocation.Limit` (MB); unset = `-1` |

Relation rules, checked on the effective values (after defaulting):

- **VM**: `requests.cpu ≤ limits.cpu` and `requests.memory ≤ limits.memory` when the limit is set; `requests.memory ≤ size.memory`. A limit above size is allowed and has no effect. `reservationLockedToMax: true` and an explicit `requests.memory` other than `size.memory` are rejected together.
- **Class**: the defaults satisfy the VM rules (`requests.default ≤ limits.default`, `requests.memory.default ≤ size.memory.default`), and the bounds can be met: `requests.min ≤ limits.max` and `requests.memory.min ≤ size.memory.max`.
- [NEEDS CLARIFICATION: whether vCenter rejects a CPU reservation above `size.cpu` × host MHz; if it does, add `requests.cpu ≤ size.cpu`.]

## `VirtualMachineClass.status`

| Field | Meaning |
|---|---|
| `zones[].name` | Covers `spec.zones`, or every zone when unset. |
| `zones[].conditions` | `HardwareCompatible`, `WithinGovernance`, `Governing`, and the informational `DevicePolicyResolved`: `False` when a device `allowed` entry matches nothing in the zone's inventory (likely a typo, or hardware not there yet). It never rejects; a `default` device missing from the zone makes `HardwareCompatible=False`. |
| `zones[]` resolved limits | Per-VM limits and options a VM can obtain in that zone, for every governed field: the class within the governing class (and the parent in Milestone 2), within what the environment browser reports for the zone. Ranges as `{min, max}`; enums, sets and device lists as the available values, with the entry's display fields. A field the class doesn't set is limited by the environment browser alone. Fields without a finite domain (extraConfig) carry their effective `allowed` and `denied`. Updated when the environment-browser objects change. Never pool totals, never what governance forbids. `pnumaNodeAffinity` options are the class's approved sets when set, otherwise the node IDs from `numNumaNodes`; a per-VM limit, not a pool total. |
| `observedGeneration`, `conditions[Ready]` | As required for every controller. |

## `VirtualMachine`

| Field | Rules |
|---|---|
| `spec.resources.{size,requests,limits}` | Same units and relation rules as the class ("Size, requests and limits"). `requests.cpu` and `limits.cpu` change from MHz to cores (millicores allowed) and the CEL rule rejecting the `m` suffix is removed; v1alpha6 has not shipped, so no conversion is needed. |
| `spec.reservedProfileName` | `*string`. Filled by the mutation webhook from the class's `reservedProfileName.default` (or the governing class's), like any other default; explicit `""` = no profile. Must be in `allowed`. On a class-backed VM it is class-supplied and can't be edited inline; a classless VM sets or inherits it. Can't change from one set value to another while the VM is powered on; vm-operator may set it from unset (schema upgrade). When set, the VM's CPU count, CPU reservation and limit, memory reservation and limit, and device identities, with how many devices have each identity, must equal the profile's; overhead is not compared. vm-operator writes the vCenter profile ID from this field and removes it when the field is empty. |
| `status.governingClassNames` | The classes governing the VM's zone, computed live. |
| `conditions[GovernanceCompliant]` | `True`/`False`; reason `NoGoverningClass` when the zone has none (False for classless VMs, True for class-backed VMs). |
| `spec.resources.shares.{cpu,memory}` | `{level, value}`; `value` only with `level: Custom` (CEL). The provider writes them to `CpuAllocation.Shares` and `MemoryAllocation.Shares`; with no value, `Normal`, as today (replacing the hard-coded `Normal`). |
| `spec.hardware.{vGPUs,dynamicDirectPathIODevices,pciDeviceGroups}` | Per-type device lists (see "Devices"); defined with vmop-3791. |
| `spec.network.interfaces[]` | Unchanged; governed by the class's `network.interfaces` (see "Network interfaces"). |
| observed status fields | firmware, change block tracking, SEV, SEV-SNP, TDX, CPU hot-remove, SMC, DirectPath I/O devices. CPU `reservationLockedToMax` and NUMA fixed auto-affinity are added with the vmx-23 fields below. |

**vmx-23 fields.** CPU `reservationLockedToMax` and NUMA fixed auto-affinity (`cpuAdvanced.numaFixedAutoAffinityEnabled`) are requested for release 9.2 but exist in vSphere only from hardware version vmx-23. Each is a VM spec field, a `BoolPolicy` at the same class path, and an observed status field, available only when the hardware version is vmx-23 or later. Tracked separately (vmop-4428) and not part of the rest of the VM API. [NEEDS CLARIFICATION: final VM path name for the CPU lock (`cpuAdvanced.reservationLockedToMax` or `cpuAdvanced.cpuReservationLockedToMax`), and the environment-browser capability names in 9.2.]

## `Zone` (`external/tanzu-topology`)

| Field | Rules |
|---|---|
| `status.governingClasses` | Mirror of the classes that govern the zone; written by one controller with an optimistic lock; never read by governance. |

## Reserved capacity

A reserved profile is a platform object that holds one VM shape: vCPU count, CPU reservation and limit (equal), memory reservation and limit (equal), and passthrough device profiles. The platform places slots of a profile on hosts, gives a powered-on VM that names the profile a free slot, and takes it back at power-off. vm-operator never enforces slot counts.

- `VirtualMachineReservedProfile` is not a vm-operator API type. It is published by the provider with each profile's shape and per-zone counts, backfilled for existing allocations before the capability is enabled (spec, "Provider contract"). vm-operator only reads it (read RBAC), as it reads the environment-browser objects. A profile name that doesn't resolve is a clear admission failure, with no fallback.
- A class that lists profiles: the class's default configuration must match its default profile, and every allowed profile must fit within the class's resource and device policy; listing a profile does not fix the class's bounds.
- A VM with a selected profile must match the profile's shape after defaulting, whether or not it names a class. A resize past the profile is rejected at admission.
- Class edits through v1alpha6 are rejected only when they change an allocated profile's shape or make the class's default VM invalid for it. The provider's older class API keeps today's block on update and delete while capacity is assigned.

## Conversion

| From v1alpha5 | To v1alpha6 |
|---|---|
| scalar field `v` | `{min: v, max: v, default: v}` |
| `hardware.{cpus,memory}` | `resources.size.{cpu,memory}` |
| `configSpec.version: vmx-N` | `hardware.version: {min: N, default: N}` |
| `configSpec` CPU and memory allocation shares | `resources.shares.{cpu,memory}`: `Normal` for a resource that `policies.resources` sets, as today; otherwise the `configSpec` shares (T027) |
| `policies.resources.{requests,limits}` | `resources.{requests,limits}` |
| `configSpec` path that has a typed field | that typed field, except a `configSpec` CPU allocation in MHz (T023) and NIC settings that differ by position (T024), which stay in `configSpec` as today |
| (none) | `externalID` = name; `zones` unset |
| `reservedProfileID: X` | `reservedProfileName: {default: X, allowed: [X], required: true}` |
| `reservedSlots` | `reservedSlots`, unchanged, including zero |

`reservedProfileID` is removed in v1alpha6 and exists only in v1alpha5 and older. Back to v1alpha5, `reservedProfileID` = `reservedProfileName.default`. Up again, if `reservedProfileID` differs from the default stored in the conversion annotation (an older-API writer changed it), the field is reset to `{default: Y, allowed: [Y], required: true}`; otherwise it is restored from the annotation.

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
      allowed: [{key: "guestinfo.*"}]
      denied: [{key: "guestinfo.secret.*"}]
```

A GPU class: at most four vGPUs, A100 profiles only, at most one 80c, which can't use UVM, and no 10c:

```yaml
spec:
  hardware:
    version: {min: 21, default: 21}
    vGPUs:
      count: {max: 4}
      default:
      - {name: gpu0, profileName: nvidia-a100-40c}
      allowed:                    # first match governs
      - profileName: nvidia-a100-80c
        count: {max: 1}
        enableUVM: {allowOverride: false, default: false}
      - profileName: "nvidia-a100-*"
        enableUVM: {default: true}
      denied:
      - profileName: nvidia-a100-10c
    dynamicDirectPathIODevices:
      allowed:
      - {vendorID: 0x8086, deviceID: 0x1593, count: {max: 1}}
  memoryAdvanced:
    reservationLockedToMax: {allowOverride: false, default: true}
```

A NIC class: one to four NICs, VMXNET3 only, two by default:

```yaml
spec:
  network:
    interfaces:
      count: {min: 1, max: 4}
      type: {allowed: [VMXNet3]}
      default:
      - {name: eth0, type: VMXNet3, network: {name: mgmt}}
      - {name: eth1, type: VMXNet3, network: {name: data}}
      vmxnet3:
        ctxPerDev: {allowed: [PerQueue], default: PerQueue}
        uptv2Enabled: {default: true}
        vnumaNodeID: {min: 0, max: 1, default: 0}
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
