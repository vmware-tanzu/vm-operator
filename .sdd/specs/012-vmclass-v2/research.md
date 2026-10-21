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
| WIKI page 3118091426 — "…: vAPI" | The v2 class vAPI, the namespace association, the vmodl generator, the CRD writers, open items O4–O12 |
| WIKI page 3118085468 — "…: Reserved Capacity" | How reserved profiles and slots work, and the proposal for flex classes |
| WIKI page 3119061412 — "…: Supervisor 1.0 vs 1.5/2.0" | Why v2 is built on the Supervisor 1.5 model |
| WIKI page 3118573642 — "…: Supervisor 1.0 Fallback" | The vcdb-based design, used only if the 1.5 model is rolled back |
| WIKI page 3118085471 — "…: Supervisor Effort" | Work outside vm-operator |
| WIKI page 3118573641 — "Research: VM Service: Flex VM Classes" | Evidence, alternatives considered (including a separate policy kind), design history |

---

## Finding 1: one kind instead of a class plus a policy

`VirtualMachineConfigPolicy`'s enforcement controller and webhook never reached main; only the `Zone` fan-out that creates the object exists. Zone-specific hardware has to be handled on the class anyway, and a fixed class is a ranged class with `min == max`. Folding governance into `VirtualMachineClass` removes almost no shipped behavior and gives users one object. Alternatives are compared on WIKI page 3118573641 §4 and Appendix A.

## Finding 2: v1alpha6 can be reshaped in place

The 9.1 release ends at v1alpha5; v1alpha6 exists only on main. The class type is changed in place and v1alpha5 and older convert to it. The repository already has a round-trip annotation mechanism; it only needs wiring into the class conversion functions, which must land before any lossy field.

## Finding 3: the environment-browser data is cluster-scoped

There is one `ConfigTarget` per cluster. A cluster-level `QueryConfigTarget` reports CPU, core and NUMA counts as cluster totals, and per-host limits and the maximum hardware version as "at least one host"; device lists are aggregated. `QueryConfigOptionEx` answers for one host (the lowest product version). This is why compatibility is reported per zone, asynchronously, and why a class is never rejected for hardware. The three CRDs reveal hardware a namespace user may not have access to, so they are restricted to admins and vm-operator, which also replaces the demand gate spec 001 planned (vmop-3983).

## Finding 4: reserved capacity

From the vCenter and wcpsvc code and the DRS functional spec:

- **Profile.** A vCenter object holding one VM shape: vCPU count, CPU reservation and limit, memory reservation and limit, DirectPath profile IDs, and an overhead reservation vCenter computes. Creating one requires CPU and memory reservation equal to the limit and both above zero. Memory size isn't part of it. Profiles can't be edited, only deleted and recreated, and deletion fails while slots exist.
- **When wcpsvc creates one.** Only when an admin sets zone capacity for a class (`{class, count}` per zone). The profile ID is the class name. No GPU check exists in wcpsvc or vCenter, though the 9.0 design intended reservations for GPU classes.
- **Slots.** Zone capacity reserves slots on the zone's top-level resource pool; namespace reservations move some of them to the namespace's pool. DRS places each slot on a host.
- **Matching.** At placement, a VM whose extraConfig names a profile must match it exactly on vCPU count, CPU reservation and limit, memory reservation and limit, and the number and IDs of DirectPath profiles. There is no check on reconfigure. A powered-off VM gives its slot back. Instant clone and cross-vCenter vMotion are blocked for such VMs. Without a free slot, the pool's expandable flags decide between deny, borrow from the parent, or best effort.
- **Why profile name equals class name in 9.0.** The Supervisor and Tenant Manager think in VM classes, and Tenant Manager and CCI read profile names as class names; the 9.0 design says later designs can relax it.
- **What vm-operator has today.** `VirtualMachineClass.spec.reservedProfileID` and `reservedSlots`, written by wcpsvc only for a class with zone capacity. `reservedSlots` is the namespace's requested count summed across zones and is read by nothing in vm-operator or wcpsvc. vm-operator copies the profile ID into the VM's extraConfig. `VirtualMachineReservedProfile` has no writer.

Decision for Milestone 1 (spec US10): the class carries `reservedProfile {default, allowed}`, the VM carries `spec.reservedProfileName`, mutation fills it like any default, and the VM webhook checks the VM against the class's fixed values. Open items RP3, RP5, RP8, RC3 and RC6 are on WIKI page 3118085468.

## Finding 5: passthrough devices need full memory reservation

A VM with a vGPU or DirectPath I/O device must reserve all its memory to power on. wcpsvc checks this only on the v1 class API, only on its `devices` field and not in `configSpec`, and not on every update. A ranged class has no single size and a governing class attaches no devices, so the check moves to VM admission (spec US8); fixed classes written through the v1 API keep the class check.

## Finding 6: the vmodl comes from the CRD

The v2 class vAPI's vmodl is generated from the v1alpha6 class's OpenAPI schema plus vmodl markers, through the vAPI team's `openapi-compiler` (`--profile vmodl2`). Its gaps and the fallback route are on WIKI page 3118091426 §7. A pre-commit check keeps the generated vmodl in sync (vmop-4392); a second check for new vpxd config fields is still to be designed (vmop-4393).
