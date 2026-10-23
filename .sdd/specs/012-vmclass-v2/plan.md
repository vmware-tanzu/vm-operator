# Implementation Plan: Flex VM Classes (`VirtualMachineClass` v2)

- **Spec**: [`spec.md`](spec.md)
- **Epic**: vmop-4034 (design); implementation epics below
- **Date**: 2026-10-21

The technical design is WIKI page 3118085463 and its child pages (full schema, vAPI, reserved capacity, Supervisor models); [`research.md`](research.md) points at each. This plan records the constitution check, the code areas, the epic split and the order of work. The task list is in [`tasks.md`](tasks.md); the API surface is in [`model.md`](model.md).

---

## Summary

Reshape the unreleased `VirtualMachineClass` v1alpha6 with typed value shapes, `governs`, zones, `externalID` and reserved profiles; enforce governance in the class and VM webhooks; report compatibility and compliance from the class and VM controllers; filter placement by governance; ship the v2 class vAPI; remove `VirtualMachineConfigPolicy`. All of it is behind the Flex VM Classes capability until the enablement story. Milestone 2 adds `parentClassRefs` behind a second capability.

## Technical context

| Field | Value |
|---|---|
| Language | Go (repo toolchain) |
| API | `vmoperator.vmware.com/v1alpha6` (hub, reshaped in place: it exists only on main, and the 9.1 release ends at v1alpha5); conversion from v1alpha1–v1alpha5 |
| Inputs | `ConfigTarget`, `VirtualMachineConfigOptions`, `VirtualMachineGuestOptions` from spec 001 |
| Testing | Ginkgo/Gomega; envtest for CRD, CEL and conversion behavior; vcsim; E2E on a Supervisor in the quarantined capability pipeline |
| Feature gate | The Flex VM Classes Supervisor capability (vmop-4313); Milestone 2 adds the Tenant VM Classes capability (vmop-4339), which depends on it |
| Other components | v2 class vAPI, wcpsvc class API, Supervisor and wcp-namespace-operator integration, built in-house (vmop-4327, vmop-4328, vmop-4329) |

## Constitution check

- [ ] **API compatibility.** v1alpha6 has not shipped, so its class type is changed in place; v1alpha5 and older convert to it, and no field is removed from a shipped version. The round-trip annotation wiring lands first.
- [ ] **Unset vs `[]`.** `*[]string` for every zone list, a deliberate exception to Kubernetes list conventions, recorded on the vAPI page §5.1.
- [ ] **CRDs.** Root, groupversion and deepcopy markers; `make generate-go generate-manifests`; manifests checked in.
- [ ] **Controllers are thin.** The governance check is one exported package in `pkg/`, called by the VM webhook, the VM reconciler and placement.
- [ ] **No direct vSphere calls in controllers.** Compatibility reads only `ConfigTarget` and the options objects.
- [ ] **Webhooks.** CEL for structural rules (value shapes, `governs.zones` within `spec.zones`); Go for cross-object rules (one governing class per zone, class-vs-class bounds, VM governance, reserved profiles).
- [ ] **Controllers modify only status (deviation).** The VM reconciler writes VM spec in three places: the same-class resize rewrite, schema upgrade, and the re-default after a class change. Precedent: `pkg/providers/vsphere/upgrade/virtualmachine/vm_schema_upgrade.go`. The spec patch is a separate write with an optimistic lock, not part of the deferred patch helper, so a concurrent user edit fails with a conflict and is retried. Recorded in "Complexity tracking".
- [ ] **Status.** `observedGeneration` and `Ready` on the class; per-zone conditions in `status.zones`.
- [ ] **Fan-in writes.** `Zone.status.governingClasses` is written by one controller from many classes: patch with an optimistic lock and skip unchanged writes.
- [ ] **Feature flags.** Every new behavior checks the capability through `pkgcfg.FromContext(ctx)`. v1alpha6 `VirtualMachineClass` objects always use the new shape, whether or not the Flex VM Classes capability is enabled. The capability gates webhook behavior (governance, defaulting from governing classes, the new VM and class rules) and controller reconciliation (compatibility, compliance, placement); toggling it takes effect without a restart.
- [ ] **E2E.** Every user-visible behavior ships with E2E coverage in the same story, in the quarantined capability pipeline until enablement.
- [ ] **Public repo.** No internal links or ticket IDs other than `vmop-NNN` and `WIKI page NNN`.

## Code areas

| Area | Paths |
|---|---|
| Class API | `api/v1alpha6/virtualmachineclass_types.go`, value types in `api/v1alpha6/`, conversion in `api/v1alpha5` and older, `api/utilconversion` |
| VM API | `api/v1alpha6/virtualmachine_types.go`: `spec.reservedProfileName`, `status.governingClassNames`, conditions, observed fields |
| Zone mirror | `external/tanzu-topology/api/v1alpha1/zone.go`: `status.governingClasses` |
| Capability | `pkg/config/capabilities`, `pkg/config` |
| Governance | new package under `pkg/`: resolve the governing class, compare a VM or a class with it |
| Class webhooks | `webhooks/virtualmachineclass/{mutation,validation}` |
| VM webhooks | `webhooks/virtualmachine/{mutation,validation}` |
| Class controller | `controllers/virtualmachineclass` (including the `ReservedProfileID` reader in `virtualmachineclass_controller.go`) |
| VM reconciler | `pkg/providers/vsphere` (compliance step before power state; reservation from the VM field in `virtualmachine/configspec.go`), `pkg/providers/vsphere/upgrade/virtualmachine/vm_schema_upgrade.go` (backfill of `spec.reservedProfileName`), `controllers/virtualmachine` |
| Reserved profiles | remove `VirtualMachineReservedProfile` from `api/v1alpha6`; read RBAC in `config/rbac`; the writer is the wcpsvc zone controller |
| Placement | `pkg/providers/vsphere/placement` |
| Zone mirror controller | `controllers/infra/zone` |
| Environment browser | `controllers/vim/*`, `controllers/virtualmachineconfigoptions`, `config/rbac` |
| Removal | `external/vim/api/v1alpha1/virtualmachine_config_policy_types.go`, `config/crd/external-crds`, `controllers/infra/zone`, `config/rbac`, `pkg/config` (`Features.VirtualMachineConfigPolicy`), `test/e2e/vmservice/vmservice/configpolicy` |
| E2E | `test/e2e/vmservice/` suites, labeled for the quarantined capability pipeline |
| Docs | `docs/` |

## API strategy

- **Class.** Every configuration field the class bounds is a typed value: number `{min, max, default}` (`default` required when `min` or `max` is set; an unset bound is open, limited by hardware or a parent class), enum `{allowed, default}`, boolean `{allowOverride, default}`, list `{default, allowed, denied}` with `{type, value}` matchers. `configSpec` keeps only pass-through fields; an elevated path in it is moved for older writers and rejected for writers on v1alpha6.
- **Reserved capacity.** `reservedProfileName` is an `EnumPolicy` `{default, allowed, required}` of profile names, at the same path as the VM's `spec.reservedProfileName`; `EnumPolicy` was chosen over `ListPolicy` because the VM value is one name and list add/remove/match semantics don't fit a single reservation. Names must resolve to a `VirtualMachineReservedProfile` in the namespace, and `allowed` is required when `default` is set. v1alpha5 `reservedProfileID: X` converts to `{default: X, allowed: [X], required: true}`; down, `reservedProfileID` = `default`; up, a `reservedProfileID` that differs from the annotation's default (an older-API writer such as wcpsvc through the v1alpha1 client changed it) resets the field to `{default: Y, allowed: [Y], required: true}`, otherwise the annotation is restored. `reservedProfileName` is authoritative in the hub; `reservedProfileID` stays only for conversion and the v1 vAPI. `reservedSlots` stays an `int32` with the same name and meaning, deprecated, preserved through conversion and the namespace copy (including zero), never overwritten with observed totals.
- **Reserved profiles.** `VirtualMachineReservedProfile` leaves vm-operator's v1alpha6 (unshipped, so this is safe) and is owned by the wcpsvc zone controller, which reconciles each profile's shape and per-zone counts from vCenter and backfills existing allocations. vm-operator only reads it. The backfill must land before VM admission relies on it; an unknown profile is a clear failure with no fallback.
- **VM.** `spec.reservedProfileName` is filled by the mutation webhook from the class default, like any other default; omission inherits the default, explicit `""` selects no profile, and `required: true` rejects an empty value after defaulting. The extraConfig profile ID is written from the VM field, falling back to the class while the field is unset, and removed when the field is empty. Existing VMs get the field from their class during schema upgrade; that unset-to-set write is exempt from the powered-on lock.
- **Upgrade.** Only conversion fills ranges (`min == max == default`); `spec.zones` stays unset (all zones); `externalID` = name; no class governs until an admin adds `governs`.

## Controller and webhook impact

- **Class mutating webhook**: `governs.enforcement: Deny`, the memory lock on a preset class that attaches devices.
- **Class validating webhook**: value-shape rules not expressible in CEL, elevated `configSpec` paths, `externalID` immutable, `existingVMs` required on a governing class with no parent, one governing class per zone, class-vs-class bounds, sticky v2 writes, `parentClassRefs` rejected until Milestone 2, reserved profiles resolve to a `VirtualMachineReservedProfile`, the class's default configuration must match its default profile, and every allowed profile must fit within the class's resource and device policy; listing a profile does not fix the class's bounds, and on edit only changes that alter an allocated profile's shape or invalidate the class's default VM are rejected (the v1 vAPI keeps today's block on update and delete while any Supervisor has capacity for the class). `governs.zones` within `spec.zones` is a CEL rule on the type.
- **VM mutating webhook**: fill unset fields from the class, from the image, or from the governing class; fill `reservedProfileName`; on a class change (allowed when `VMResize` or `VMResizeCPUMemory` is on, with no annotation), replace every class-supplied field from the new class and clear and re-default the fields it doesn't set.
- **VM validating webhook**: governance, inline-edit rule while `className` is set (including `reservedProfileName`), the zone rule, hardware compatibility per zone, device memory reservation, reserved-profile match (after defaulting, for class-backed and classless VMs; overhead excluded; a resize past the profile rejected), the powered-on lock on changing one set profile to another, class change of a reserved VM only while powered off. The `DenyPowerOn` power-on rejection ships with compliance (vmop-4041).
- **Class controller**: per-zone `HardwareCompatible`, `WithinGovernance`, `Governing`, resolved limits; watches `ConfigTarget` and `Zone`.
- **VM reconciler**: `GovernanceCompliant`, `status.governingClassNames`, withheld power-on for `DenyPowerOn`, same-class resize rewrite, observed fields, the reservation from the VM field (written or removed).
- **Placement**: only zones that admit the VM and are compatible.
- **Environment browser**: maximum hardware version; RBAC restricted to admins and vm-operator; no vCenter re-query on resync when the cluster is unchanged.
- **RBAC**: remove `virtualmachineconfigpolicies`; restrict the three environment-browser CRDs; read-only access to `virtualmachinereservedprofiles`.

## Epics and order

Milestone 1 enables governing classes for providers; Milestone 2 adds tenant classes.

| Epic | Scope | Depends on |
|---|---|---|
| vmop-4038 — API, upgrade and capability | capability, quarantined pipeline, v1alpha6 schema (including removing `VirtualMachineReservedProfile`), conversion, VM and Zone API fields, policy removal, environment-browser completion | — |
| vmop-4039 — Governance engine and class compatibility | governance package, class `HardwareCompatible`, resolved limits, governance status and Zone mirror | the schema (vmop-4315) |
| vmop-4201 — Class webhooks and v2 vAPI | class webhooks, vmodl markers, generator hookup and drift checks, v2 class vAPI, wcpsvc changes, the wcpsvc zone controller's `VirtualMachineReservedProfile` writer, Supervisor and nsop integration | the schema; the governance package for class-vs-class bounds |
| vmop-4040 — VM create and edit | VM defaults, VM governance validation, device memory reservation, zone rule, reserved capacity | the governance package; the VM API fields; the reserved profile writer (vmop-TBD) for reserved capacity |
| vmop-4041 — Compliance, placement and enablement | `GovernanceCompliant`, the `DenyPowerOn` power-on rejection, placement filter, enablement, docs | the VM webhooks |
| vmop-4312 — Tenant VM Classes (Milestone 2) | second capability, `parentClassRefs`, class families, v2 vAPI round-trip, enablement | all of Milestone 1 |

**Sequential**: the schema → the governance package → the VM webhooks → compliance and placement → enablement.

**Parallel**:

- Within vmop-4038: the capability and pipeline, policy removal and environment-browser completion run beside the schema.
- vmop-4039's class status stories run beside vmop-4201's class webhooks once the governance package lands.
- vmop-4201's vAPI, wcpsvc and integration work runs beside vmop-4040, because it needs only the schema and markers.
- Docs run beside vmop-4041.

## Timeline

Two-week sprints from 2026-10-20; the sprint covering 2026-12-15 to 2027-01-11 includes the holidays.

| Epic | Three engineers | Two engineers |
|---|---|---|
| vmop-4038 | 2026-10-20 → 2026-11-16 | 2026-10-20 → 2026-11-16 |
| vmop-4039 | 2026-11-03 → 2026-11-30 | 2026-11-03 → 2026-11-30 |
| vmop-4201 | 2026-11-17 → 2026-12-14 | 2026-11-17 → 2027-01-11 |
| vmop-4040 | 2026-12-01 → 2027-01-11 | 2026-12-01 → 2027-01-25 |
| vmop-4041 | 2026-12-15 → 2027-01-25 | 2026-12-15 → 2027-02-08 |
| vmop-4312 | 2027-01-26 → 2027-03-08 | 2027-01-26 → 2027-03-22 |

## Test strategy

- Unit tests with `testlabels` category labels for every webhook rule and the governance package; envtest for every CEL rule and for conversion, including the three-state zone round trip.
- vcsim for the class controller, the VM reconciler, placement and the environment-browser re-query rule (call counts on the fake provider).
- E2E ships inside each story, in the quarantined capability pipeline (vmop-4314); the enablement story (vmop-4337) takes it out of quarantine after a full sweep.

## Rollout

- Existing classes convert with no manual step; no class governs until an admin adds `governs`, so behavior doesn't change on upgrade.
- The capability stays off until vmop-4337. The v2 vAPI is offered only on Supervisors with the VM class migration capability (Supervisor 1.5); the vcdb fallback (WIKI page 3118573642) applies only if that model is rolled back.
- The policy is removed in the same release; its enforcement never shipped.
- Release notes: v1alpha6 class fields, classless VMs in governed zones, `GovernanceCompliant`, `spec.reservedProfileName`.

## Complexity tracking

| Deviation | Why | Simpler alternative rejected |
|---|---|---|
| `*[]string` zone lists distinguish unset from `[]` | "No zones" and "all zones" must not collapse | A plain list: dropping `[]` would make a class available everywhere |
| The VM reconciler writes VM spec (same-class resize, schema upgrade, class change re-default) | These values come from the class and must follow it; precedent `vm_schema_upgrade.go`. A separate spec patch with an optimistic lock, apart from the deferred status patch | Asking users to re-apply the class: drift between the class and its VMs, and no backfill on upgrade |
