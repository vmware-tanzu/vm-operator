# Implementation Plan: Flex VM Classes (`VirtualMachineClass` v2)

- **Spec**: [`spec.md`](spec.md)
- **Epic**: vmop-4034 (design); implementation epics below
- **Date**: 2026-10-21; amended 2026-11-25 (no classless VMs for DevOps users, `governs` removed)

The technical design is WIKI page 3118085463 and its child pages (full schema, vAPI, reserved capacity, Supervisor models); [`research.md`](research.md) points at each. This plan records the constitution check, the code areas, the epic split and the order of work. The task list is in [`tasks.md`](tasks.md); the API surface is in [`model.md`](model.md).

---

## Summary

Reshape the unreleased `VirtualMachineClass` v1alpha6 with typed value shapes, `unlistedFieldPolicy`, zones, `externalID` and reserved profiles; fill VM defaults from the VM's class and check every VM write against that class in the VM webhooks; rewrite the resize fields on a class change as the 9.1 release does; report compatibility and compliance from the class and VM controllers; filter placement by the VM's class; add the mapping markers the provider's class API is generated from; remove `VirtualMachineConfigPolicy`. The provider's work is listed under "Provider work". All of it is behind the `supports_vm_service_classless_vm` capability until the enablement story. Milestone 2 adds `parentClassRefs` behind a second capability.

## Technical context

| Field | Value |
|---|---|
| Language | Go (repo toolchain) |
| API | `vmoperator.vmware.com/v1alpha6` (hub, reshaped in place: it exists only on main, and the 9.1 release ends at v1alpha5); conversion from v1alpha1–v1alpha5 |
| Inputs | `ConfigTarget`, `VirtualMachineConfigOptions`, `VirtualMachineGuestOptions` from spec 001 |
| Testing | Ginkgo/Gomega; envtest for CRD, CEL and conversion behavior; vcsim; E2E on a Supervisor in the quarantined capability pipeline |
| Feature gate | `supports_vm_service_classless_vm`, read as `pkgcfg.FromContext(ctx).Features.VMServiceClasslessVM` (vmop-4313), replaces both previous capability keys and feature flags; the name is kept although DevOps users can't create classless VMs; Milestone 2 adds the Tenant VM Classes capability (vmop-4339), which depends on it |
| Provider | Class and namespace APIs, class API generation and drift checks, reserved profile publication, capability advertisement, Supervisor integration and which classes reach which namespaces; tracked in the provider's spec (tasks.md T910–T916) |

## Constitution check

- [ ] **API compatibility.** v1alpha6 has not shipped, so its class type is changed in place; v1alpha5 and older convert to it, and no field is removed from a shipped version. The round-trip annotation wiring lands first. A converted class lets a VM do exactly what it could do before (model.md, "Conversion").
- [ ] **Unset vs `[]`.** `*[]string` for every zone list, a deliberate exception to Kubernetes list conventions, recorded on the vAPI page §5.1.
- [ ] **CRDs.** Root, groupversion and deepcopy markers; `make generate-go generate-manifests`; manifests checked in.
- [ ] **Controllers are thin.** The class check is one exported package in `pkg/`, called by the VM webhook, the VM reconciler, the class controller and placement.
- [ ] **No direct vSphere calls in controllers.** Compatibility reads only `ConfigTarget` and the options objects.
- [ ] **Webhooks.** CEL for structural rules (value shapes); Go for cross-object rules (VM against its class, class against its parent in Milestone 2, reserved profiles).
- [ ] **Controllers modify only status (deviation).** Only the schema upgrade writes VM spec from a controller, as it does today (`pkg/providers/vsphere/upgrade/virtualmachine/vm_schema_upgrade.go`). The class-change rewrite, the same-class resize and `reservedProfileName` on a class change are written by the VM mutation webhook. Recorded in "Complexity tracking".
- [ ] **Status.** `observedGeneration` and `Ready` on the class; per-zone conditions in `status.zones`. Status size and write rate are measured before the resolved-limits story (T029).
- [ ] **Feature flags.** Every new behavior checks `pkgcfg.FromContext(ctx).Features.VMServiceClasslessVM`. v1alpha6 `VirtualMachineClass` objects always use the new shape, whether or not the capability is enabled. The capability gates webhook behavior (defaults from ranged classes, edits within bounds, the class-change rewrite, the new VM and class rules) and controller reconciliation (compatibility, compliance, placement); toggling it takes effect without a restart.
- [ ] **E2E.** Every user-visible behavior ships with E2E coverage in the same story, in the quarantined capability pipeline until enablement. Every story pairs new-behavior tests with tests that v1alpha5 clients and converted classes behave as before.
- [ ] **Public repo.** No internal links or ticket IDs other than `vmop-NNN` and `WIKI page NNN`.

## Code areas

| Area | Paths |
|---|---|
| Class API | `api/v1alpha6/virtualmachineclass_types.go`, value types in `api/v1alpha6/`, conversion in `api/v1alpha5` and older, `api/utilconversion` |
| VM API | `api/v1alpha6/virtualmachine_types.go`: `spec.reservedProfileName`, the `ClassCompliant` condition, observed fields; `spec.resources.shares`; per-type device lists in `virtualmachine_hardware_types.go` with vmop-3791 |
| vmx-23 CPU fields | CPU `reservationLockedToMax` and NUMA fixed auto-affinity on the VM, the class and VM status; a separate story (vmop-4428), blocked on vSphere and govmomi with vmx-23 |
| Capability | `pkg/config/capabilities`, `pkg/config` |
| Class check | new package under `pkg/`: compare a VM, or a child class in Milestone 2, with a class, applying `unlistedFieldPolicy` and the environment-browser bounds |
| Class webhooks | `webhooks/virtualmachineclass/{mutation,validation}` |
| VM webhooks | `webhooks/virtualmachine/{mutation,validation}` (the class-change rewrite and the one-shot same-class resize replace `SetLastResizeAnnotations`-only handling in `virtualmachine_mutator.go`) |
| Class controller | `controllers/virtualmachineclass` (moving the `ReservedProfileID` reader in `virtualmachineclass_controller.go` to `reservedProfileName`) |
| VM reconciler | `pkg/providers/vsphere` (compliance step before power state; reservation from the VM field in `virtualmachine/configspec.go`; applying spec resize fields), `pkg/providers/vsphere/upgrade/virtualmachine/vm_schema_upgrade.go` and `backfill/` (shares, device lists, `spec.reservedProfileName`, pending class changes), `pkg/util/vmopv1/resize.go`, `controllers/virtualmachine` |
| Reserved profiles | remove `VirtualMachineReservedProfile` from `api/v1alpha6`; read RBAC in `config/rbac`; the writer is the provider |
| Provider API mapping markers | `api/v1alpha6/virtualmachineclass_types.go` and the value types; field-coverage check under `api/` |
| Placement | `pkg/providers/vsphere/placement` |
| Environment browser | `controllers/vim/*`, `controllers/virtualmachineconfigoptions`, `config/rbac` |
| Removal | `external/vim/api/v1alpha1/virtualmachine_config_policy_types.go`, `config/crd/external-crds`, `controllers/infra/zone`, `config/rbac`, `test/e2e/vmservice/vmservice/configpolicy` |
| E2E | `test/e2e/vmservice/` suites, labeled for the quarantined capability pipeline |
| Docs | `docs/` |

## API strategy

- **Class.** Every configuration field the class bounds is a typed value: number `{min, max, default}` (`default` required when `min` or `max` is set; an unset bound is open, limited by hardware or a parent class), enum `{allowed, default, required}`, string `{allowed, denied, default, required}`, boolean `{allowOverride, default}`, list `{count, default, allowed, denied}` (model.md, "List policies": glob matchers, `denied` first, the first matching `allowed` entry applies to an item and carries its `count` and field policies, `default` fills only an omitted list), set `{default, allowed, required}`. `{}` lists a field with no bound and no default. A field the class doesn't list follows `unlistedFieldPolicy` (`Deny` by default; `Allow` opens it within the hardware). Devices are one list per type at the VM's paths (agreed with vmop-3791, T020); NICs are `network.interfaces.{count, type, default, vmxnet3}` with one ordered `default`; `hardware.version` is an `int32` range over the actual version; shares are `resources.shares.{cpu,memory}` (vmop-4479). One reflection-based evaluator applies every policy, and a schema test pairs each policy with one VM field. `configSpec` keeps only pass-through fields; an elevated path in it is moved for older writers and rejected for writers on v1alpha6.
- **Reserved capacity.** `reservedProfileName` is an `EnumPolicy` `{default, allowed, required}` of profile names, at the same path as the VM's `spec.reservedProfileName`; `EnumPolicy` was chosen over `ListPolicy` because the VM value is one name and list add/remove/match semantics don't fit a single reservation. Names must resolve to a `VirtualMachineReservedProfile` in the namespace, and `allowed` is required when `default` is set. v1alpha5 `reservedProfileID: X` converts to `{default: X, allowed: [X], required: true}`; down, `reservedProfileID` = `default`; up, a `reservedProfileID` that differs from the annotation's default (an older-API writer changed it) resets the field to `{default: Y, allowed: [Y], required: true}`, otherwise the annotation is restored. `reservedProfileID` is removed from v1alpha6 and exists only in v1alpha5 and older; the class controller reader and the extraConfig write move to `reservedProfileName`. The provider may keep writing `reservedProfileID` through older versions. `reservedSlots` stays an `int32` with the same name and meaning, deprecated, preserved through conversion and the namespace copy (including zero), never overwritten with observed totals.
- **Reserved profiles.** `VirtualMachineReservedProfile` leaves vm-operator's v1alpha6 (unshipped, so this is safe) and is published by the provider with each profile's shape and per-zone counts (spec, "Provider contract"). vm-operator only reads it. The provider's backfill must be complete before VM admission relies on it; an unknown profile is a clear failure with no fallback.
- **VM.** A VM names a class, as today. Defaults are written into the VM spec at admission; the VM may change any field within its class's bounds; values stay on the spec with no record of who set them. A class change rewrites only the resize fields (model.md, "Defaults, edits and checks"); a class edit changes no VM. `spec.reservedProfileName` is filled by the mutation webhook from the class default, like any other default. Existing VMs get the new fields from vSphere during schema upgrade; that unset-to-set write is exempt from the powered-on lock.
- **Upgrade.** Only conversion fills ranges (`min == max == default`); `unlistedFieldPolicy: Deny`, with `{}` for every field a v1alpha5 VM could already set; `spec.zones` stays unset (all zones); `externalID` = name.

## Controller and webhook impact

- **Class mutating webhook**: `unlistedFieldPolicy: Deny` when unset; the memory lock on a class whose defaults attach devices.
- **Class validating webhook**: value-shape rules not expressible in CEL, `resources.size.{cpu,memory}` listed in a `Deny` class, elevated `configSpec` paths, `externalID` immutable, the schema-version label, `parentClassRefs` rejected until Milestone 2, reserved profiles resolve to a `VirtualMachineReservedProfile`, the class's default configuration must match its default profile, and every allowed profile must fit within the class's resource and device policy; listing a profile does not fix the class's bounds, and on edit only changes that alter an allocated profile's shape or invalidate the class's default VM are rejected (the provider's older class API keeps today's block on update and delete while capacity is assigned).
- **VM mutating webhook**: fill unset fields from the VM's class; fill `reservedProfileName`; on a class change (allowed where resize is allowed today), write the new class's resize fields, keeping a resize field the same request changes, and set `reservedProfileName` from the new class; on a change of the same-class resize annotation, write the class's current resize fields once.
- **VM validating webhook**: the VM against its class in its zone (including `unlistedFieldPolicy`), with ratcheting for VMs that don't fit; the whole spec on a class change; `className` required except for privileged accounts, as today; hardware compatibility per zone; device memory reservation; reserved-profile match (after defaulting; overhead excluded; a resize past the profile rejected); the powered-on lock on changing one set profile to another; class change of a reserved VM only while powered off. Cost per request: the check runs on create and on spec changes, not on metadata-, status- or finalizer-only updates (US8). It reads from the informer cache only (the VM's class and, in Milestone 2, its ancestors in `vmware-system-vmop`; the reserved profile; the zone's environment-browser objects, which the existing hardware validation already reads), with no API-server or vCenter calls, and compares each field the class bounds: one pass over the class's policies, plus matching list items against `allowed` and `denied` entries. A VM with no zone is checked once per candidate zone (Design §3.1b).
- **Class controller**: per-zone `HardwareCompatible`, `DevicePolicyResolved`, resolved limits; watches `ConfigTarget` and `Zone`.
- **VM reconciler**: `ClassCompliant`, `VirtualMachineClassConfigurationSynced`, applying the spec's resize fields as a resize today, observed fields, the reservation from the VM field (written or removed), the schema-upgrade backfill. It never writes VM spec outside the schema upgrade and never withholds power-on.
- **Placement**: only zones where the VM's class is available and compatible and the VM fits the zone's resolved limits.
- **Environment browser**: maximum hardware version; RBAC restricted to admins and vm-operator; no vCenter re-query on resync when the cluster is unchanged.
- **RBAC**: remove `virtualmachineconfigpolicies`; restrict the three environment-browser CRDs; read-only access to `virtualmachinereservedprofiles`.

## Epics and order

Milestone 1 enables ranged classes for providers; Milestone 2 adds tenant classes.

| Epic | Scope | Depends on |
|---|---|---|
| vmop-4038 — API, upgrade and capability | capability, quarantined pipeline, v1alpha6 schema (including `unlistedFieldPolicy` and removing `VirtualMachineReservedProfile`), conversion, VM API fields and backfill, shares (vmop-4479), policy removal, environment-browser completion | the VM device lists from vmop-3791 (T020) |
| vmop-4039 — Class check and class compatibility | the class check package, class `HardwareCompatible`, resolved limits (after the status analysis, T029) | the schema (vmop-4315) |
| vmop-4201 — Class webhooks and provider API markers | class mutating and validating webhooks (vmop-4324, vmop-4325), provider API mapping markers (vmop-4326) | the schema |
| vmop-4040 — VM create and edit | VM defaults, edits within bounds, the class change and same-class resize, device memory reservation, zone rule, reserved capacity | the class check package; the VM API fields; the provider's reserved profile publication (T910) for reserved capacity |
| vmop-4041 — Compliance, placement and enablement | `ClassCompliant`, placement filter, enablement, docs | the VM webhooks; the provider's capability advertisement (T911) for enablement |
| vmop-4312 — Tenant VM Classes (Milestone 2) | second capability, `parentClassRefs`, v2 vAPI round-trip, enablement | all of Milestone 1 |

**Sequential**: the schema → the class check package → the VM webhooks → compliance and placement → enablement.

**Parallel**:

- Within vmop-4038: the capability and pipeline, policy removal and environment-browser completion run beside the schema.
- vmop-4039's class status stories run beside vmop-4201's class webhooks once the class check package lands.
- Provider work (T910–T916) runs beside the vm-operator epics; it needs the schema (vmop-4315) and, for the class API, the markers (vmop-4326).
- Docs run beside vmop-4041.

## Timeline

Two-week sprints from 2026-10-20; the sprint covering 2026-12-15 to 2027-01-11 includes the holidays. Dates are from before the 2026-11-25 amendment; removing governance shortens vmop-4039 and vmop-4041.

| Epic | Three engineers | Two engineers |
|---|---|---|
| vmop-4038 | 2026-10-20 → 2026-11-16 | 2026-10-20 → 2026-11-16 |
| vmop-4039 | 2026-11-03 → 2026-11-30 | 2026-11-03 → 2026-11-30 |
| vmop-4201 | 2026-11-17 → 2026-12-14 | 2026-11-17 → 2026-12-28 |
| vmop-4040 | 2026-12-01 → 2027-01-11 | 2026-12-01 → 2027-01-25 |
| vmop-4041 | 2026-12-15 → 2027-01-25 | 2026-12-15 → 2027-02-08 |
| vmop-4312 | 2027-01-26 → 2027-03-08 | 2027-01-26 → 2027-03-22 |

## Provider work

Tracked in the provider's spec and listed in tasks.md as dependency tasks T910–T916 with their vmop keys: reserved profile publication and backfill (T910), capability advertisement (T911), the v2 class API (T912, vmop-4327), the older class API and the namespace API (T913, vmop-4328), Supervisor and namespace-copy integration (T914, vmop-4329), class API generation and drift checks (T915, vmop-4392), and the vpxd config field drift check (T916, vmop-4393). vm-operator work that consumes them: vmop-4334 needs T910; vmop-4337 needs T911; T912 and T915 need vmop-4315 and vmop-4326.

## Test strategy

- Unit tests with `testlabels` category labels for every webhook rule and the class check package; envtest for every CEL rule and for conversion, including the three-state zone round trip and the conversion open-field list (T035).
- vcsim for the class controller, the VM reconciler, placement and the environment-browser re-query rule (call counts on the fake provider).
- A status size and write-rate measurement at scale before the resolved-limits story (T029).
- E2E ships inside each story, in the quarantined capability pipeline (vmop-4314); the enablement story (vmop-4337) takes it out of quarantine after a full sweep. Upgrade E2E covers a VKS-style client creating VMs on converted classes.

## Rollout

- Existing classes convert with no manual step to `Deny` classes that open exactly the fields v1alpha5 VMs could set, so behavior doesn't change on upgrade.
- The capability stays off until vmop-4337. vm-operator turns flex behavior on only when the provider advertises the capability, which it does once its contract is met (spec, "Provider contract"); the Supervisor model choice is on WIKI page 3119061412.
- The policy is removed in the same release; its enforcement never shipped.
- Release notes: v1alpha6 class fields and `unlistedFieldPolicy`, VM edits within class bounds, `ClassCompliant`, `spec.reservedProfileName`, and the same-class resize annotation applying once per change instead of following every class edit.

## Complexity tracking

| Deviation | Why | Simpler alternative rejected |
|---|---|---|
| `*[]string` zone lists distinguish unset from `[]` | "No zones" and "all zones" must not collapse | A plain list: dropping `[]` would make a class available everywhere |
| The VM schema upgrade writes VM spec | New VM fields must hold the live values so the reconciler doesn't reset existing VMs; precedent `vm_schema_upgrade.go`. Every other VM spec write is in the mutation webhook | No backfill: unset fields are treated as the vSphere default and reset existing VMs (research.md, Finding 12) |
