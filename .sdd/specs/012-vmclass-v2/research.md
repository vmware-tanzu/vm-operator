# Research: Flex VM Classes (`VirtualMachineClass` v2)

- **Spec**: [`spec.md`](spec.md)

The design and its evidence live in the wiki. This file points at each page and keeps the findings that shaped decisions in [`spec.md`](spec.md) and [`plan.md`](plan.md).

---

## Design pages

| Page | What it holds |
|---|---|
| WIKI page 3119061398 — "One Pager: VM Service: Flex VM Classes" | Problem, goals, the class and VM behavior, vCenter and upgrade summary, dependencies |
| WIKI page 3118085463 — "Design: VM Service: Flex VM Classes" | The authoritative Kubernetes-side design: schema rules, governance, zones, compatibility, migration, Milestone 2 hierarchy, key decisions (§9) |
| WIKI page 3118085467 — "…: Full Schema" | Every typed field, its `configSpec` source and capability data; what stays in `configSpec` |
| WIKI page 3118091426 — "…: vAPI" | The class APIs, the namespace association, how the class API is generated from the CRD, the CRD writers, open items (provider-side) |
| WIKI page 3118085468 — "…: Reserved Capacity" | How reserved profiles and slots work, and the proposal for flex classes |
| WIKI page 3119061412 — "…: Supervisor 1.0 vs 1.5/2.0" | Why v2 is built on the Supervisor 1.5 model |
| WIKI page 3118573642 — "…: Supervisor 1.0 Fallback" | The vcdb-based design, used only if the 1.5 model is rolled back |
| WIKI page 3118085471 — "…: Supervisor Effort" | Work outside vm-operator, tracked in the provider's spec |
| WIKI page 3118573641 — "Research: VM Service: Flex VM Classes" | Evidence, alternatives considered (including a separate policy kind), design history |

---

## Finding 1: one kind instead of a class plus a policy

`VirtualMachineConfigPolicy`'s enforcement controller and webhook never reached main; only the `Zone` fan-out that creates the object exists. Zone-specific hardware has to be handled on the class anyway, and a fixed class is a ranged class with `min == max`. Folding governance into `VirtualMachineClass` removes almost no shipped behavior and gives users one object. Alternatives are compared on WIKI page 3118573641 §4 and Appendix A.

## Finding 2: v1alpha6 can be reshaped in place

The 9.1 release ends at v1alpha5; v1alpha6 exists only on main. The class type is changed in place and v1alpha5 and older convert to it. The repository already has a round-trip annotation mechanism; it only needs wiring into the class conversion functions, which must land before any lossy field.

## Finding 3: the environment-browser data is cluster-scoped

There is one `ConfigTarget` per cluster. The cluster-level config target reports CPU, core and NUMA counts as cluster totals, and per-host limits and the maximum hardware version as "at least one host"; device lists are aggregated. The config options answer for one host (the lowest product version). This is why compatibility is reported per zone, asynchronously, and why a class is never rejected for hardware. The three CRDs reveal hardware a namespace user may not have access to, so they are restricted to admins and vm-operator, which also replaces the demand gate spec 001 planned (vmop-3983).

## Finding 4: reserved capacity

Behavior that shaped the design; the platform and provider details, with their evidence, are on WIKI page 3118085468:

- **Profile.** A reserved profile holds one VM shape: vCPU count, CPU reservation and limit, memory reservation and limit, and passthrough device profiles, plus an overhead the platform computes. It can't be edited, only replaced, and can't be removed while slots exist. Reservation must equal the limit for CPU and memory. Memory size isn't part of the shape.
- **How profiles are created.** The provider creates a profile for a class when an admin assigns zone capacity to it, from the class's configuration; today the profile's name is the class name. For a flex class it uses the class's resolved defaults, so a fixed class gets the same profile as today.
- **Slots.** Zone capacity reserves slots on the zone; namespace reservations move some of them to the namespace. The platform places each slot on a host.
- **Matching.** When a VM that names a profile is placed or powered on, the platform matches it against the profile's shape (overhead excluded); a mismatch fails with no fallback to an unreserved placement. Power-off releases the slot; power-on without a free slot fails. There is no check on reconfigure.
- **Why profile name equals class name today.** Clients outside vm-operator read profile names as class names; later designs can relax this.
- **What vm-operator has today.** `VirtualMachineClass.spec.reservedProfileID` and `reservedSlots`, written by the provider only for a class with zone capacity. `reservedSlots` is the namespace's desired count summed across zones and is read by nothing in vm-operator. vm-operator copies the profile ID into the VM's extraConfig (`resourcepool.vmResourceProfileId`) from the class, so a VM moved to an unreserved class keeps a stale key. `VirtualMachineReservedProfile` is defined in v1alpha6 but has no writer.

Decision for Milestone 1 (spec US10): the class carries `reservedProfileName {default, allowed, required}`, the VM carries `spec.reservedProfileName`, mutation fills it like any default, and the VM webhook checks the VM against the profile's shape. `VirtualMachineReservedProfile` leaves vm-operator's API; the provider publishes it and vm-operator only reads it (spec, "Provider contract"). Platform and provider questions still open are tracked in the provider's spec (WIKI page 3118085468 §5).

## Finding 5: passthrough devices need full memory reservation

A VM with a vGPU or DirectPath I/O device must reserve all its memory to power on. Today this is checked only by the older class API, only on its `devices` field and not in `configSpec`, and not on every update. A ranged class has no single size and a governing class attaches no devices, so the check moves to VM admission (spec US8); fixed classes written through the older class API keep the class check.

## Finding 6: the class API definition comes from the CRD

The provider generates its v2 class API definition from the v1alpha6 class's OpenAPI schema plus mapping markers on vm-operator's Go types. vm-operator owns the markers and a check that every field is mapped or explicitly excluded (vmop-4326); the generator, its drift check (vmop-4392) and the vpxd config field drift check (vmop-4393) are the provider's (WIKI page 3118091426 §7).

## Finding 7: list policies

- **One string matcher, no regex.** A glob with no wildcard is an exact match (Go `path.Match`). The earlier `{fixed, glob, regex}` matcher had no requirement for regex, and a regex can't be rendered as a form or checked for overlaps.
- **First match, not every match.** With every matching entry applied, two overlapping entries with different defaults made the class invalid, and an item's policy depended on all entries at once. With `denied` first and then the first matching `allowed` entry, each item has one governing entry and nothing is merged. The cost: an earlier, more specific entry must repeat any policy it shares with a later catch-all; the class webhook warns when an entry is shadowed.
- **Counts.** `count` is a `CountRange {min, max}` without `default`, because the number rule (T002) would otherwise require a default count that is really the length of `default`. Each item counts 1. There is no total across device types: no use case asks for one. The NIC total stays (telco "Number of vNICs").
- **Defaults.** A list's `default` fills only an omitted list, as the Full Schema page already says; an explicit list is validated as supplied.
- **Where field policies sit.** On `allowed` entries when items have a governed key field (a vGPU's UVM support depends on its profile); on a type key when they don't (NICs). Per-network NIC field policies (different VMXNET3 settings per network) were considered and dropped because no requirement asks for them. Network names in a class (`network.name`, `default` items) only work where the namespaces the class is copied into use the same network names.

## Finding 8: one VM list per device type

The VM had no device spec field. The `{profileName, count}` items came from this spec. The VM PCI device design (WIKI page 2477879529, vmop-3791) needs one entry per device with a stable `name`: a fixed PCI slot (`unitNumber`), device-group membership, per-device UVM and advanced properties, and status per device. It does not need one union list:

- Every per-device field fits on a per-type item.
- Group members point at the group (`deviceGroup` → `pciDeviceGroups[].name`), so groups already span lists.
- Unit-number uniqueness and the 128-device cap must be checked across lists anyway, because SR-IOV NICs in `network.interfaces` share the same slot space.
- The mutation webhook assigns `unitNumber`, so the order between device types decides nothing, and per-type defaults lose nothing.

Per-type lists remove the type-key level from the class, show a type's fields in its own struct instead of a CEL rule pairing `type` with a sub-struct, and follow the per-type controller lists. Rule for splitting VM lists: one list per identity and numbering space. Controllers are per type because each has its own bus numbers; NICs stay one list because the VM already has one. Names are required: a mutating webhook can't infer them, because `kubectl apply` and server-side apply merge keyed lists before webhooks run. Open with vmop-3791's owner (T020).

## Finding 9: NICs

- **Shape.** The VM already has typed telco NIC fields (`type`, `vnumaNodeID`, `vmxnet3.*`, `advancedProperties`), so the class mirrors them under `network.interfaces`: `count`, `type`, one ordered `default`, and a `vmxnet3` type key with the common and VMXNET3 fields flattened.
- **Why one `default`.** With `MutableNetworks` on, spec NICs pair with devices by identity (`pkg/providers/vsphere/network/devices.go`), per-NIC `ethernetN.*` keys use the matched device's own index (`pkg/vmconfig/networkextraconfig/nic_matcher.go`), and guest customization maps NICs by MAC (`pkg/providers/vsphere/network/gosc.go`). Only the NIC's virtual PCI slot still follows creation order, which matters to guests that name NICs by PCI order and to DPDK workloads bound to a PCI address. `pciSlotNumber` (vmop-3576) removes that. Even then one list is better: it is the VM's own shape, matches the vCenter and VCFA adapter tables, needs no cross-list name or interleaving rule, and can't change shape after it ships.
- **SR-IOV, plan only.** Nothing in this spec's schema or tasks. When SR-IOV is built end to end, its class policy is a `network.interfaces.sriov` type key under the same rules as `vmxnet3`, with `StringPolicy` for `network.name` and `dvx.deviceClass`. Until then a class can only allow or forbid SR-IOV NICs through `network.interfaces.type`, and those NICs have no tuning policy, including `vnumaNodeID`.

## Finding 10: hardware version and shares

- **Hardware version as a number.** The VM's `minHardwareVersion`, `status.hardwareVersion` and the image's hardware version are `int32`; public docs say `vmx-N`, which a form renders. `ConfigTarget` reports strings, parsed with `vimtypes.ParseHardwareVersion`. Raising `minHardwareVersion` upgrades a VM at power-off today (`ReconcileMinHardwareVersion`, `pkg/providers/vsphere/session/session_vm_update.go`), which is the way back into compliance below `min`.
- **Shares.** The provider writes `Normal` shares whenever a class sets requests or limits (`pkg/providers/vsphere/virtualmachine/configspec.go`). The valid custom range comes from `ResourcePool.QueryResourceConfigOption`, which isn't in the environment-browser objects, so Milestone 1 checks only `value > 0` (vmop-4479).

## Finding 11: telco requirement coverage

Against the telco gap analysis and the v1alpha6 telco field reference:

| Requirement | Covered by |
|---|---|
| Hardware version | `hardware.version` |
| Number of vNICs | `network.interfaces.count` |
| NIC to network | `vmxnet3.network.name` and the `default` items |
| pnicFeatures, ctxPerDev, RSS offload, UDP RSS, coalescing, UPTv2 | `network.interfaces.vmxnet3.*` |
| NIC vNUMA affinity | `vmxnet3.vnumaNodeID`, VMXNET3 NICs only; SR-IOV NICs, the main telco case, wait for the SR-IOV type key |
| VMXNET3 and SR-IOV NIC types | `network.interfaces.type` |
| vGPU count and UVM | `hardware.vGPUs` counts and `enableUVM` |
| Latency sensitivity `HighWithHyperthreading` | The VM enum already has it |
| Shares | `resources.shares` (vmop-4479) |
| `pciSlotNumber` | A VM field (vmop-3576); not governed |
| C-states | Out of scope in the field reference; set through `advanced.extraConfig` |
| Percentage reservations and limits | Not covered: decided against (T000) |
| Number and size of disks | Not covered: open with product management |
| ISO | Not a class setting: the VM's `cdrom` |

## Finding 12: upgrade paths

- **What shipped.** The 9.1 release ends at v1alpha5. Its VM has none of the telco fields and no extraConfig field; every telco setting comes from the class: `configSpec.extraConfig`, `configSpec.deviceChange`, typed `configSpec` fields, or `hardware.devices`. v1alpha6, the VM typed fields, the VM backfill and the capability exist only on main.
- **VM fields: the telco pattern.** `ReconcileSchemaUpgrade` (`pkg/providers/vsphere/upgrade/virtualmachine/vm_schema_upgrade.go`) runs a backfill once per VM per feature-version bit (`FeatureVersionTelcoVMServiceAPI` for the classless capability), from the live vSphere config, filling only unset fields. It ends the reconcile (`ErrUpgradeObject`) so the backfill is saved before any config change, and the VM webhook blocks user edits until the VM is upgraded. New VMs go through it too. Every new VM field needs one: the reconcilers treat an unset field as the vSphere default and enforce it (`desiredCPUAllocation` gives reservation 0 and limit -1; unset extraConfig keys are cleared), so a field without a backfill resets existing VMs.
- **Class fields: conversion only.** The class has no schema-upgrade reconciler. v1alpha6 is the storage version, and stored v1alpha5 objects convert on read. Conversion is pure: it can't call vSphere (no host frequency, T023) and can't fail. The `api` module has no govmomi dependency, so it reads `configSpec` with local JSON types.
- **Gaps found.** The backfill writes the live CPU reservation and limit in MHz (`backfill/compute.go`), as does the resize sync (`pkg/util/vmopv1/resize_class_sync.go`); both change with T108b (vmop-4481). Shares (T120) and device lists (T108a) have no backfill yet. The NIC backfill matches NICs to devices by position, so a NIC added outside vm-operator can shift settings onto the wrong interface. The conversion table has no row for `hardware.devices`.
