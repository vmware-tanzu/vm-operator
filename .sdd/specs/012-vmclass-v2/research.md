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

The provider generates its v2 class API definition (vmodl) from the v1alpha6 class's OpenAPI schema plus `+vmodl` markers on vm-operator's Go types. vm-operator owns the markers and a check that every field is mapped or explicitly excluded (vmop-4326); the generator, its drift check (vmop-4392) and the vpxd config field drift check (vmop-4393) are the provider's (WIKI page 3118091426 §7).
