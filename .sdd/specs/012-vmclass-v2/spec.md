# Feature Specification: Flex VM Classes (`VirtualMachineClass` v2)

- **Feature branch**: `faisal/vmop-4034-flex-vm-classes-spec`
- **Created**: 2026-10-08
- **Status**: In Progress (Milestone 1 specified and scheduled; Milestone 2 designed, scheduled after Milestone 1)
- **Epic**: vmop-4034 (design). Implementation epics: vmop-4038, vmop-4039, vmop-4201, vmop-4040, vmop-4041 (Milestone 1) and vmop-4312 (Milestone 2). The epic split and order are in [`plan.md`](plan.md).
- **Design**: WIKI page 3119061398 — "One Pager: VM Service: Flex VM Classes"; WIKI page 3118085463 — "Design: VM Service: Flex VM Classes" (Kubernetes side, authoritative), with its child pages for the full schema, the vAPI, reserved capacity and the Supervisor models. Summaries and pointers are in [`research.md`](research.md); the API surface is in [`model.md`](model.md).
- **Plan**: [`plan.md`](plan.md)
- **Tasks**: [`tasks.md`](tasks.md)
- **Supersedes**: the `VirtualMachineConfigPolicy` work in spec [`001-class-policy-resize`](../001-class-policy-resize/) (Stories S8, S9.a, S9.b and S11)

---

## Summary

`VirtualMachineClass` becomes the one kind for VM presets and VM governance:

1. Every configuration field the class can bound has a typed value shape: number (`min`, `max`, `default`), enum, boolean or list. A fixed size is `min == max == default`, so one class can offer one size or a range. CPU requests and limits stay in vCPUs, as in today's class. Devices are one list; a separate set shape is deferred.
2. A class can govern: with `spec.governs`, its constraints are the ceiling for every VM in the zones it governs, whether or not the VM names it. A zone with a governing class accepts classless (free-form) VMs; a zone without one requires a class, as today.
3. Classes are zonal: `spec.zones` and `spec.governs.zones` say where a class is available and where it governs.
4. `spec.externalID` holds the client's identity for the class, separate from the Kubernetes name.
5. `configSpec` keeps only pass-through fields that have no typed field.
6. Classes live in each Supervisor's `vmware-system-vmop` namespace and are copied into namespaces; they are written through the provider's class APIs, and vm-operator's webhooks validate every write.
7. Reserved capacity follows the VM: a class offers reserved profiles as a governed field, the VM carries the profile it uses and must match its shape, and the profiles themselves are read from `VirtualMachineReservedProfile` objects that the provider publishes.
8. `VirtualMachineConfigPolicy` is removed.

Milestone 1 (provider classes) ships all of the above with one governing class per zone, behind the `vm_service_classless_vm` Supervisor capability. Milestone 2 (tenant classes) adds `parentClassRefs`, so a class can be bounded by another class.

---

## Goals

- Existing classes **MUST** keep working unchanged after upgrade, with no manual step.
- A class **MUST** be able to be a selectable preset, a governing ceiling, both, or neither, without a separate policy object.
- A VM **MUST** be checked against exactly one complete governing class per zone in Milestone 1; ranges are never merged across classes.
- A VM **MUST** get its unset fields filled from its class, or from the governing class when it has none, at admission.
- Hardware compatibility **MUST** be reported per zone on the class, asynchronously; a class **MUST NOT** be rejected for hardware.
- A namespace user **MUST NOT** be able to read more about hardware or other classes than they could obtain.
- A VM that holds reserved capacity **MUST** match its reserved profile's shape.
- The Milestone 1 schema **MUST** allow Milestone 2 without a breaking change.
- Flex behavior **MUST** be off unless the Supervisor capability is enabled.

## Non-goals

- Creating or editing classes through the Kubernetes API by namespace users; classes are written by platform components only.
- Redesigning placement; placement only receives a narrower zone list.
- Bringing VCFA's provider and tenant concepts into vm-operator.
- Moving the vCenter UI to the v2 API (a later milestone).
- Implementing the class and namespace APIs, publishing reserved profiles, or advertising the capability; these are the provider's (see "Provider contract").
- Enforcing reserved slot counts; the platform owns slots.

---

## User stories

Personas are those in [`constitution.md`](../../memory/constitution.md). Everything below is Milestone 1 unless marked.

### US1 — CSP admin: a class offers a range of sizes (P1)

**Acceptance scenarios**:

1. **Given** a class with `resources.size.cpu: {min: 2, max: 16, default: 4}`, **when** a VM names it without setting CPU, **then** the VM spec gets 4 CPUs at admission.
2. **Given** a class that sets `min` or `max` for a number field, **when** it is written without `default`, **then** it is rejected because `default` is required.
3. **Given** a class that sets only `min` and `default`, **when** it is stored, **then** `max` stays unset and is bounded only by the hardware (or by a parent class in Milestone 2).
4. **Given** a fixed size, **when** it is written, **then** it is expressed as `min == max == default`; the class mutating webhook does not fill missing bounds.
5. **Given** an existing v1alpha5 class, **when** it is read as v1alpha6, **then** each scalar field is `{min: v, max: v, default: v}` and the class behaves exactly as before.
6. **Given** an existing class with `hardware.instanceStorage`, **when** it is read as v1alpha6, **then** instance storage is kept as a preset value and VMs that name the class get it as today.

### US2 — CSP admin: a class governs the VMs in its zones (P1)

**Acceptance scenarios**:

1. **Given** a class `flex` with `governs: {existingVMs: AllowOnViolation}` and `spec.zones: [zone-a]`, **when** a classless VM in `zone-a` requests more CPU than `flex`'s `max`, **then** it is rejected, naming the field.
2. **Given** the same class, **when** a VM that names another class in `zone-a` gets values outside `flex`'s ranges, **then** it is rejected.
3. **Given** a governing class with `advanced.extraConfig.denied: [{type: Glob, value: "guestinfo.*"}]`, **when** a VM sets `guestinfo.foo`, **then** it is rejected.
4. **Given** a class already governs `zone-a`, **when** a second class is written so that it also governs `zone-a`, **then** the write is rejected.
5. **Given** a governing class in `zone-a`, **when** a selectable class available in `zone-a` is written with a range outside the governing class's range, **then** the write is rejected.
6. **Given** a governing class is narrowed after a selectable class was admitted, **when** the class controller reconciles, **then** the selectable class reports `WithinGovernance=False` for that zone and is not otherwise changed.
7. **Given** a governing class with no parent, **when** it is written without `governs.existingVMs`, **then** it is rejected because `existingVMs` is required.
8. **Given** a governing class written without `governs.enforcement`, **when** it is stored, **then** `enforcement` is `Deny`.
9. **Given** a class with `governs.zones` that is not a subset of `spec.zones`, **when** it is written, **then** it is rejected by the schema.

### US3 — DevOps user: a classless VM is sized within the ceiling (P1)

**Acceptance scenarios**:

1. **Given** a zone with no governing class, **when** a classless VM is created in it, **then** it is rejected.
2. **Given** a governing class in `zone-a`, **when** a classless VM omits a governed field, **then** the field is filled from the image if the governing class allows the image's value, or from the governing class's default if the image reports none.
3. **Given** the image's value for a governed field is not allowed, **when** the VM is created, **then** it is rejected, naming the field, the image's value and the allowed values.
4. **Given** a classless VM, **when** it is admitted, **then** `spec.className` is never filled in, and `status.governingClassNames` names the governing class.

### US4 — DevOps user: a VM that names a class takes the class's values (P1)

**Acceptance scenarios**:

1. **Given** a VM created with `spec.className: large`, **when** it is admitted, **then** every field `large` sets is written into the VM spec.
2. **Given** that VM, **when** an update changes a field `large` supplies, **then** it is rejected with a message to remove `spec.className` first.
3. **Given** that VM, **when** `spec.className` is removed, **then** the VM keeps its values and is governed as a classless VM.
4. **Given** `large` is edited, **when** the VM carries the same-class resize annotation, **then** the VM reconciler rewrites the class-supplied fields; without the annotation, the VM keeps its values.
5. **Given** VM resize (`VMResize` or `VMResizeCPUMemory`) is enabled, **when** `spec.className` changes from `large` to `small`, **then** no annotation is needed: every class-supplied field is replaced from `small`, and fields `small` doesn't set are cleared and re-defaulted from the next source (the governing class, the image, or vpxd).
6. **Given** a class edit that changes a typed field, **when** the class controller reconciles, **then** the class's `ClassInstance` hash changes, so VMs of the class see the new instance.

### US5 — DevOps user: a VM without a zone gets one only when it is unambiguous (P1)

**Acceptance scenarios**:

1. **Given** a VM without `spec.zone` whose possible zones are all governed by the same class, **when** it is created, **then** it is admitted with that class's defaults for unset governed fields.
2. **Given** a VM without `spec.zone` whose possible zones have different governing classes, or some governed and some not, **when** it is created, **then** it is rejected, naming the zones and their governing classes.
3. **Given** a VM placed in `zone-a`, **when** it is later updated, **then** it is validated against `zone-a` only; it is never moved to another zone.
4. **Given** a VM without `spec.zone`, **when** it is placed, **then** placement receives only the zones whose governing class admits the VM's values and whose hardware is compatible.

### US6 — CSP admin: existing VMs that drift out of compliance (P1)

**Acceptance scenarios**:

1. **Given** `governs.existingVMs: AllowOnViolation` and a VM that falls outside a narrowed range, **when** the VM reconciles, **then** it reports `GovernanceCompliant=False` and keeps running; a power cycle succeeds.
2. **Given** `governs.existingVMs: DenyPowerOn`, **when** a user requests power-on of a non-compliant VM, **then** the request is rejected; vm-operator never changes the power state itself.
3. **Given** the governing class of a zone is deleted, **when** VMs there reconcile, **then** classless VMs report `GovernanceCompliant=False` with reason `NoGoverningClass`, and VMs that name a class report `GovernanceCompliant=True` with the same reason.
4. **Given** a VM, **when** it reconciles, **then** its status reports the observed firmware, change block tracking, CPU `reservationLockedToMax`, NUMA fixed auto-affinity, SEV, SEV-SNP, TDX, CPU hot-remove, SMC and DirectPath I/O devices, so compliance can be read from the VM.

### US7 — CSP admin: hardware compatibility is reported, not enforced on the class (P1)

**Acceptance scenarios**:

1. **Given** a class requesting a vGPU profile that exists only in `zone-a`, **when** it is created in `vmware-system-vmop` or copied into a namespace, **then** the write is accepted.
2. **Given** that class, **when** the class controller reconciles, **then** `status.zones` reports `HardwareCompatible=True` for `zone-a` and `False` with a reason for the other zones, and re-checks as hardware changes.
3. **Given** that class, **when** a VM that names it is created in a zone where it is not compatible, **then** the VM is rejected with that zone's reason.
4. **Given** a namespace user, **when** they read the class status or a rejection message, **then** they see per-VM limits and what governance permits, never pool totals or devices governance forbids.
5. **Given** a DevOps user, **when** they read `ConfigTarget`, `VirtualMachineConfigOptions` or `VirtualMachineGuestOptions`, **then** the request is forbidden; only admins and vm-operator can read them.
6. **Given** a resync with no cluster change, **when** the environment-browser controllers reconcile, **then** vCenter is not queried again; a cluster change triggers a new query.
7. **Given** a class, **when** the class controller reconciles, **then** `status.zones[]` lists, for every governed field, the values a VM can get in that zone: what the environment browser reports, within the governing class (and the parent in Milestone 2), with the entry's display fields. A field the class doesn't set is limited by the environment browser alone. A form renders its options from this status, or, for a new class with nothing above it, from the environment-browser objects an admin can read; it never combines restrictions itself.

### US8 — Admin: a VM with a passthrough device has its memory fully reserved (P1)

**Acceptance scenarios**:

1. **Given** a VM whose effective configuration has a vGPU or DirectPath I/O device, **when** its memory is not fully reserved, **then** it is rejected at create, and on spec changes while the device is attached, naming the fix.
2. **Given** such a VM, **when** only metadata, finalizers or status change, or vm-operator writes it, **then** the write is not rejected.
3. **Given** a preset class that attaches a device, **when** it is written, **then** the class mutating webhook sets the memory reservation lock; a VM that names it with less than full reservation is rejected at VM admission.
4. **Given** a fixed class written through the existing class API, **when** it attaches a device, **then** the class itself still requires full memory reservation, as today.

### US9 — Admin and automation: classes keep their shape across API versions (P1)

**Acceptance scenarios**:

1. **Given** a class created or last updated through the v1alpha6 CRD version, **when** a client writes its spec through an older CRD version, **then** the write is rejected; reads still work.
2. **Given** a class in `vmware-system-vmop`, **when** the provider copies it into a namespace through v1alpha6, **then** every v1alpha6 field reaches the namespace copy.
3. **Given** an association entry with `zones` unset, `[]` or a list, **when** the namespace copy is written, **then** the three states stay distinct through every layer.
4. **Given** an existing class, **when** it is migrated, **then** its `metadata.name` is unchanged and `spec.externalID` equals the name.
5. **Given** a change to the v1alpha6 class types, **when** a field has neither a provider API mapping marker nor an explicit exclusion, **then** the field-coverage check fails.
6. **Given** the v1alpha6 class CRD schema, **when** the schema test runs, **then** every governed field matches exactly one value shape by its field set, and every device list is a map list keyed by the same key fields as its environment-browser entry.

### US10 — CSP admin: reserved capacity works with flex classes (P1)

**Acceptance scenarios**:

1. **Given** an existing class with `reservedProfileID: X`, **when** it is read as v1alpha6, **then** it has `reservedProfileName: {default: X, allowed: [X], required: true}` and no `reservedProfileID` (removed in v1alpha6), and VMs that name it keep using the reservation.
2. **Given** a v1alpha6 class whose `reservedProfileName` has several allowed names, **when** it is read through v1alpha5 (where `reservedProfileID` = `default`) and written back unchanged, **then** the v1alpha6 value is restored; **when** an older-API writer changes `reservedProfileID` to `Y`, **then** the class becomes `{default: Y, allowed: [Y], required: true}`.
3. **Given** a class whose `reservedProfileName` has a default, **when** a VM that names it, or a classless VM in a zone it governs, is created without `spec.reservedProfileName`, **then** the mutation webhook fills the default; an explicit `""` selects no profile, and is rejected when `required: true`.
4. **Given** a class whose `reservedProfileName` sets `default` without `allowed`, or names a profile with no `VirtualMachineReservedProfile` in the namespace, or whose default configuration doesn't match its default profile, or lists a profile that doesn't fit within its resource and device policy, **when** it is written, **then** it is rejected. Listing a profile does not fix the class's bounds; VMs are validated against the selected profile's shape (scenario 6).
5. **Given** a VM that sets `spec.reservedProfileName`, **when** the name is not in its class's (or governing class's) `allowed` list, **then** it is rejected.
6. **Given** a VM, class-backed or classless, with a selected profile after defaulting, **when** its CPU count, CPU reservation or limit, memory reservation or limit, or device identities or quantities differ from the profile, **then** it is rejected, naming the field and the profile. Overhead is not compared. A resize past the profile is rejected the same way.
7. **Given** a VM that names a class, **when** an update changes `spec.reservedProfileName` directly, **then** it is rejected by the inline-edit rule; a classless VM may set it.
8. **Given** a powered-on VM with `spec.reservedProfileName` set, **when** an update changes it to another set value, **then** it is rejected; vm-operator setting it from unset (schema upgrade) is allowed.
9. **Given** a powered-on VM with a reserved profile, **when** its class changes, **then** it is rejected; the class can change only while the VM is powered off. [NEEDS CLARIFICATION: whether the platform honors a changed or removed profile on a VM without a restart of its management service; until it does, this stays powered-off only. Tracked in the provider's spec.]
10. **Given** a VM moved to a class without a reserved profile, **when** it reconciles, **then** `spec.reservedProfileName` is cleared and vm-operator removes the profile ID from the VM's extraConfig.
11. **Given** an existing VM of a reserved class, **when** it is upgraded, **then** `spec.reservedProfileName` is filled from its class, and vm-operator sets the VM's reservation from the VM field, falling back to the class while the field is unset.
12. **Given** a class with VMs holding an allocated profile, **when** it is edited through v1alpha6, **then** only changes that alter the profile's shape or make the class's default VM invalid for it are rejected. (The provider's older class API keeps today's block on update and delete while capacity is assigned; see "Provider contract".)
13. **Given** a class with `reservedSlots`, **when** it is converted or copied into a namespace, **then** the value, including zero, is kept and never replaced with observed totals.

### US11 — Platform engineer: `VirtualMachineConfigPolicy` is removed (P2)

**Acceptance scenarios**:

1. **Given** a Supervisor upgraded to this release, **when** a `Zone` reconciles, **then** no `VirtualMachineConfigPolicy` is created, and the CRD is not installed.
2. **Given** the environment-browser pipeline from spec 001, **when** the policy is removed, **then** `ConfigTarget`, `VirtualMachineConfigOptions` and `VirtualMachineGuestOptions` keep working unchanged.

### US12 — Platform engineer: flex behavior is behind a capability (P1)

The capability is `vm_service_classless_vm` (vmop-4313). It replaces both `supports_telco_vm_service_api` and `supports_vm_service_vm_config_policy`, so the Telco VM APIs and the configuration-policy/environment-browser pipeline share one switch. It defaults to disabled until vmop-4337; the old keys do not enable the new capability. Removing the policy API remains separate work.

**Acceptance scenarios**:

1. **Given** a Supervisor without the Flex VM Classes capability, **when** a class or VM is written, **then** there is no governance and no classless VMs, and the class write is not rejected for using ranges or `governs`. v1alpha6 `VirtualMachineClass` objects always use the new shape, whether or not the Flex VM Classes capability is enabled. The capability gates webhook behavior (governance, defaulting from governing classes, the new VM and class rules) and controller reconciliation (compatibility, compliance, placement).
2. **Given** the capability is enabled or disabled, **when** the capability is read by vm-operator, **then** governance, the new webhooks and the class controller turn on or off without a restart.
3. **Given** the quarantined E2E pipeline, **when** it runs, **then** it runs the Flex VM Classes suites with the capability enabled, separately from the main pipeline.

### US13 — Tenant admin: a class bounded by another class (Milestone 2, P3)

A class lists one parent in `spec.parentClassRefs`; its ranges must fit inside the parent's (a child that exceeds its parent is rejected), fields it doesn't declare come from the chain, and a referenced parent can't be deleted. Only the top class of a chain may set `governs.existingVMs` and `configSpec`; children inherit `existingVMs`. Designed in the design page §8; behind its own capability, which depends on the Milestone 1 capability.

---

## Scope of work

**vm-operator**:

- The v1alpha6 `VirtualMachineClass` schema (reshaped in place; v1alpha6 has not shipped), value types, conversion from v1alpha5 and older, and round-trip annotations.
- The VM and Zone API fields: `status.governingClassNames`, `GovernanceCompliant`, observed status fields, `spec.reservedProfileName`, `Zone.status.governingClasses`.
- Class mutating and validating webhooks.
- VM mutating and validating webhooks: defaults at admission, governance, the zone rule, device memory reservation, reserved profiles, `DenyPowerOn`.
- The `VirtualMachineClass` controller: per-zone `HardwareCompatible`, `WithinGovernance` and `Governing`, and resolved limits.
- The VM reconciler: compliance, the same-class resize rewrite, observed status, and the reservation from the VM field.
- Moving `VirtualMachineReservedProfile` out of the vm-operator API; vm-operator only reads it.
- Placement: the zone filter for single VMs and groups.
- The environment-browser completion from spec 001: maximum hardware version, admin-only access, no re-query on resync.
- The provider API mapping markers on the v1alpha6 class Go types, which the provider's API generation reads, and a check that every field has a mapping or an explicit exclusion.
- The capability, the quarantined pipeline, enablement and docs.
- Removing `VirtualMachineConfigPolicy`.

**Provider (tracked in the provider's spec)**: the v2 class API and the older class API, the namespace API with per-class zones, generating the class API definition from the CRD and its drift checks, publishing `VirtualMachineReservedProfile` and building reserved profiles from class defaults, advertising the capability, and the Supervisor and namespace-copy integration (plan.md, "Provider work"; tasks.md T910–T916).

**Other teams**: the Supervisor team (the 1.5 capability, the class migrator), VCFA (move to the v2 API; whether region quota reads class sizes), the vCenter UI (hide edit actions for classes written through v1alpha6).

---

## Provider contract

vm-operator runs on a Supervisor whose provider supplies the pieces below. vm-operator states what it assumes, not how the provider does it; the provider's spec tracks the implementation (plan.md, "Provider work").

- **Reserved profiles are published.** The provider publishes a namespaced `VirtualMachineReservedProfile` for every profile a namespace can use, with the profile's shape (CPU count, CPU reservation and limit, memory reservation and limit, device identities and quantities) and per-zone counts. vm-operator only reads it.
- **Backfilled before the capability.** Objects exist for every existing allocation before the Flex VM Classes capability is enabled. A profile name that doesn't resolve is a clear admission failure in vm-operator, with no fallback.
- **Older writers keep working.** The provider may keep writing `reservedProfileID` and `reservedSlots` through older API versions; vm-operator converts them (US10.1, US10.2, US10.13).
- **Capability.** The provider advertises the Flex VM Classes capability; vm-operator only reads it (US12).
- **Platform enforcement.** The platform rejects, at power-on, a VM whose configuration doesn't match its named profile. vm-operator rejects the same mismatch earlier, at admission.
- **Class and namespace APIs.** The v2 class API, the older class API (including its block on editing a class with assigned capacity) and the namespace API with per-class zones are the provider's. Every class they write lands in `vmware-system-vmop` and is validated by vm-operator's webhooks; the provider copies classes into namespaces through v1alpha6, keeping unset and `[]` zone lists apart.

---

## Edge cases

- A client that drops `[]` turns "no zones" into "all zones"; for availability this fails open. Mitigated by pointer types, a three-state round-trip test and an E2E test.
- A class edit between admission and placement can leave a VM in a non-compliant zone; it is handled as drift by `existingVMs`.
- A governing class with `governs.zones: []` doesn't count when deciding whether a VM must set a zone.
- An elevated field arriving in `configSpec` from an older writer is moved to the typed field; from a writer on the version where the field is typed, it is rejected.
- The platform checks a VM against its reserved profile only when it is placed and powered on, and rejects a mismatch with no fallback. The VM webhook enforces the profile's shape so the same failure surfaces at admission instead. [NEEDS CLARIFICATION: what happens to a powered-on VM's slot after a resize; tracked in the provider's spec.]
- A VM moved to an unreserved class must lose its extraConfig profile ID; vm-operator writes or removes it from `spec.reservedProfileName`.

---

## Open questions

- [NEEDS CLARIFICATION: Who rejects an association zone outside the namespace or the class.] The class in `vmware-system-vmop` carries `spec.zones` and `governs.zones`, the association only narrows them, and its `status.zones` covers its `spec.zones`.
- [NEEDS CLARIFICATION: Hardware version as a range with a default, its path, and whether a VM below the minimum is upgraded or rejected.]
- [NEEDS CLARIFICATION: Confirmation from its owners that the zone's unreserved-DirectPath setting applies to classless VMs and non-reserved classes with devices.] Working assumption: it does.
- [NEEDS CLARIFICATION: Reserved capacity: RP3 profiles not tied to a class and how VCFA discovers them; RP8 read access to reservations for a later per-zone status.] Findings in WIKI page 3118085468.
- [NEEDS CLARIFICATION: Whether an explicit `""` means "no value" for every enum field, as it does for `reservedProfileName`.]
- Provider-side open items (platform behavior on a changed profile or a resize, profile naming across Supervisors, external readers of `reservedSlots`, the class API generator, namespace-copy sign-off, default classes, the vpxd config field drift check) are tracked in the provider's spec; vm-operator tasks that depend on them say so.
- [NEEDS CLARIFICATION: Milestone 2: whether a child's branches need one `existingVMs` answer when a VM satisfies none of them.]

---

## Review & acceptance checklist

- [ ] Every user story has Given/When/Then scenarios that can be tested independently.
- [ ] Upgrade with no manual step is specified (US1.5, US9.4, US10.1, US10.11, US11).
- [ ] One governing class per zone, and the class-vs-class bound, are specified (US2).
- [ ] The zone rule for VMs without a zone is specified (US5).
- [ ] Hardware compatibility is asynchronous and never rejects a class (US7).
- [ ] Visibility limits are specified (US7.4, US7.5).
- [ ] Every open question is mirrored as a blocked task in `tasks.md`.
