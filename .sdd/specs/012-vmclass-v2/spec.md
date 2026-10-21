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

1. Every configuration field the class can bound has a typed value shape: number (`min`, `max`, `default`), enum, boolean or list. A fixed size is `min == max == default`, so one class can offer one size or a range. CPU requests and limits stay in vCPUs, as in today's class. [NEEDS CLARIFICATION: whether set is its own value shape or a form of list.]
2. A class can govern: with `spec.governs`, its constraints are the ceiling for every VM in the zones it governs, whether or not the VM names it. A zone with a governing class accepts classless (free-form) VMs; a zone without one requires a class, as today.
3. Classes are zonal: `spec.zones` and `spec.governs.zones` say where a class is available and where it governs.
4. `spec.externalID` holds the client's identity for the class, separate from the Kubernetes name.
5. `configSpec` keeps only pass-through fields that have no typed field.
6. Classes live in each Supervisor's `vmware-system-vmop` namespace, are written through a new per-Supervisor v2 class vAPI, and are copied into namespaces by wcp-namespace-operator.
7. Reserved capacity follows the VM: a class offers reserved profiles as a governed field, and the VM carries the profile it uses.
8. `VirtualMachineConfigPolicy` is removed.

Milestone 1 (provider classes) ships all of the above with one governing class per zone, behind a Supervisor capability. Milestone 2 (tenant classes) adds `parentClassRefs`, so a class can be bounded by another class.

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
- Enforcing reserved slot counts; vCenter DRS owns slots.

---

## User stories

Personas are those in [`constitution.md`](../../memory/constitution.md). Everything below is Milestone 1 unless marked.

### US1 — CSP admin: a class offers a range of sizes (P1)

**Acceptance scenarios**:

1. **Given** a class with `resources.size.cpu: {min: 2, max: 16, default: 4}`, **when** a VM names it without setting CPU, **then** the VM spec gets 4 CPUs at admission.
2. **Given** a class that sets `min` or `max` for a number field, **when** it is written without `default`, **then** it is rejected because `default` is required.
3. **Given** a class that sets only `min` and `default`, **when** it is stored, **then** `max` stays unset and is bounded only by the hardware (or by a parent class in Milestone 2).
4. **Given** a fixed size, **when** it is written, **then** it is expressed as `min == max == default`.
5. **Given** an existing v1alpha5 class, **when** it is read as v1alpha6, **then** each scalar field is `{min: v, max: v, default: v}` and the class behaves exactly as before.

### US2 — CSP admin: a class governs the VMs in its zones (P1)

**Acceptance scenarios**:

1. **Given** a class `flex` with `governs: {existingVMs: AllowOnViolation}` and `spec.zones: [zone-a]`, **when** a classless VM in `zone-a` requests more CPU than `flex`'s `max`, **then** it is rejected, naming the field.
2. **Given** the same class, **when** a VM that names another class in `zone-a` gets values outside `flex`'s ranges, **then** it is rejected.
3. **Given** a governing class with `advanced.extraConfig.denied: [{type: Glob, value: "guestinfo.*"}]`, **when** a VM sets `guestinfo.foo`, **then** it is rejected.
4. **Given** a class already governs `zone-a`, **when** a second class is written so that it also governs `zone-a`, **then** the write is rejected.
5. **Given** a governing class in `zone-a`, **when** a selectable class available in `zone-a` is written with a range outside the governing class's range, **then** the write is rejected.
6. **Given** a governing class is narrowed after a selectable class was admitted, **when** the class controller reconciles, **then** the selectable class reports `WithinGovernance=False` for that zone and is not otherwise changed.
7. **Given** a governing class with no parent, **when** it is written without `governs.existingVMs`, **then** it is rejected because `existingVMs` is required.

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
4. **Given** `large` is edited, **when** the VM carries the same-class resize annotation, **then** the VM reconciler rewrites the class-supplied fields with an optimistic-lock patch; without the annotation, the VM keeps its values.

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

### US7 — CSP admin: hardware compatibility is reported, not enforced on the class (P1)

**Acceptance scenarios**:

1. **Given** a class requesting a vGPU profile that exists only in `zone-a`, **when** it is created in `vmware-system-vmop` or copied into a namespace, **then** the write is accepted.
2. **Given** that class, **when** the class controller reconciles, **then** `status.zones` reports `HardwareCompatible=True` for `zone-a` and `False` with a reason for the other zones, and re-checks as hardware changes.
3. **Given** that class, **when** a VM that names it is created in a zone where it is not compatible, **then** the VM is rejected with that zone's reason.
4. **Given** a namespace user, **when** they read the class status or a rejection message, **then** they see per-VM limits and what governance permits, never pool totals or devices governance forbids.
5. **Given** a DevOps user, **when** they read `ConfigTarget`, `VirtualMachineConfigOptions` or `VirtualMachineGuestOptions`, **then** the request is forbidden; only admins and vm-operator can read them.
6. **Given** a resync with no cluster change, **when** the environment-browser controllers reconcile, **then** vCenter is not queried again; a cluster change triggers a new query.

### US8 — Admin: a VM with a passthrough device has its memory fully reserved (P1)

**Acceptance scenarios**:

1. **Given** a VM whose effective configuration has a vGPU or DirectPath I/O device, **when** its memory is not fully reserved, **then** it is rejected at create, and on spec changes while the device is attached, naming the fix.
2. **Given** such a VM, **when** only metadata, finalizers or status change, or vm-operator writes it, **then** the write is not rejected.
3. **Given** a fixed class written through the existing class API, **when** it attaches a device, **then** the class itself still requires full memory reservation, as today.

### US9 — Admin and automation: classes are written through the v2 vAPI (P1)

**Acceptance scenarios**:

1. **Given** a class created or last updated through the v2 API, **when** a client writes it through the existing class API, **then** the write is rejected with an error pointing at the v2 API; reads still work.
2. **Given** a class written through the v2 API, **when** it is copied into a namespace, **then** every v2 field reaches the namespace copy.
3. **Given** an association entry with `zones` unset, `[]` or a list, **when** the namespace copy is written, **then** the three states stay distinct through every layer.
4. **Given** an existing class, **when** it is migrated, **then** its `metadata.name` is unchanged and `spec.externalID` equals the name.
5. **Given** a change to the v1alpha6 class types, **when** the vmodl is not regenerated, **then** the pre-commit drift check fails.

### US10 — CSP admin: reserved capacity works with flex classes (P1)

**Acceptance scenarios**:

1. **Given** an existing class with a reserved profile, **when** it is read as v1alpha6, **then** it has `reservedProfile: {default: <profile>, allowed: [<profile>]}`, and VMs that name it keep using the reservation.
2. **Given** a class whose `reservedProfile` has a default, **when** a VM that names it, or a classless VM in a zone it governs, is created without `spec.reservedProfileName`, **then** the mutation webhook fills it, like any other default.
3. **Given** a VM that sets `spec.reservedProfileName`, **when** the name is not in its class's (or governing class's) `allowed` list, **then** it is rejected.
4. **Given** a VM with `spec.reservedProfileName`, **when** its CPU count, CPU reservation or limit, memory size, reservation or limit, or devices differ from the reserved profile, **then** it is rejected, naming the field.
5. **Given** a class that lists reserved profiles, **when** it is written with ranges on the profile's fields, **then** it is rejected; in Milestone 1 those fields are fixed (`min == max`).
6. **Given** a powered-on VM with `spec.reservedProfileName`, **when** an update changes the profile name, **then** it is rejected. [NEEDS CLARIFICATION: whether the profile name may change while the VM is powered off (RP5).]
7. **Given** an existing VM of a reserved class, **when** it is upgraded, **then** `spec.reservedProfileName` is filled from its class, and vm-operator sets the vCenter reservation from the VM field.

### US11 — Platform engineer: `VirtualMachineConfigPolicy` is removed (P2)

**Acceptance scenarios**:

1. **Given** a Supervisor upgraded to this release, **when** a `Zone` reconciles, **then** no `VirtualMachineConfigPolicy` is created, and the CRD is not installed.
2. **Given** the environment-browser pipeline from spec 001, **when** the policy is removed, **then** `ConfigTarget`, `VirtualMachineConfigOptions` and `VirtualMachineGuestOptions` keep working unchanged.

### US12 — Platform engineer: flex behavior is behind a capability (P1)

**Acceptance scenarios**:

1. **Given** a Supervisor without the Flex VM Classes capability, **when** a class or VM is written, **then** the v1alpha6 fields behave as fixed classes do today: no governance, no classless VMs.
2. **Given** the capability is enabled, **when** the capability is read by vm-operator, **then** governance, the new webhooks and the class controller turn on without a restart.
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
- Placement: the zone filter for single VMs and groups.
- The environment-browser completion from spec 001: maximum hardware version, admin-only access, no re-query on resync.
- The vmodl markers, the generator hookup and the drift checks.
- The capability, the quarantined pipeline, enablement and docs.
- Removing `VirtualMachineConfigPolicy`.

**Built in-house in other components**: the v2 class vAPI and the wcpsvc class API changes, the Supervisor and wcp-namespace-operator integration (plan.md, Milestone 1, vmop-4201).

**Other teams**: the Supervisor team (the 1.5 capability, per-class zones on `vmClassRefs`, the vcdb → catalog migrator), wcp-namespace-operator owners (sign-off), the vAPI team (generator support), VCFA (move to the v2 API; whether region quota reads class sizes), the vCenter UI (hide edit actions for classes written through v2).

---

## Edge cases

- A client that drops `[]` turns "no zones" into "all zones"; for availability this fails open. Mitigated by pointer types, a three-state round-trip test and an E2E test.
- A class edit between admission and placement can leave a VM in a non-compliant zone; it is handled as drift by `existingVMs`.
- A governing class with `governs.zones: []` doesn't count when deciding whether a VM must set a zone.
- An elevated field arriving in `configSpec` from an older writer is moved to the typed field; from a writer on the version where the field is typed, it is rejected.
- vCenter DRS checks a VM against its reserved profile only at placement; a reserved VM resized while it holds a slot would keep the slot with the wrong shape, which is why the VM webhook enforces the profile's fields.

---

## Open questions

- [NEEDS CLARIFICATION: Who rejects an association zone outside the namespace or the class.] The class in `vmware-system-vmop` carries `spec.zones` and `governs.zones`, the association only narrows them, and its `status.zones` covers its `spec.zones`.
- [NEEDS CLARIFICATION: Hardware version as a range with a default, its path, and whether a VM below the minimum is upgraded or rejected.]
- [NEEDS CLARIFICATION: Confirmation from its owners that the zone's unreserved-DirectPath setting applies to classless VMs and non-reserved classes with devices.] Working assumption: it does.
- [NEEDS CLARIFICATION: Whether set is its own value shape or a form of list.]
- [NEEDS CLARIFICATION: Reserved capacity: RP3 profiles not tied to a class and how VCFA discovers them; RP5 whether `reservedProfileName` may change while the VM is powered off; RP8 read access to vCenter reservations for a later per-zone status; RC3 profile naming across Supervisors on one vCenter; RC6 whether memory overhead is part of the DRS match; whether anything outside vm-operator reads `reservedSlots`.] Findings in WIKI page 3118085468.
- [NEEDS CLARIFICATION: Whether default classes can be updated and deleted on Supervisor 1.5 (vAPI page O12).]
- [NEEDS CLARIFICATION: vAPI generator route, `Quantity` mapping, generator ownership, `externalID` index, v2 `name` uniqueness, and creating a class before any Supervisor exists (vAPI page O4–O8, O11).]
- [NEEDS CLARIFICATION: Sign-off from wcp-namespace-operator on vAPI page §6.2.]
- [NEEDS CLARIFICATION: The design of the vpxd config field drift check (vmop-4393).]
- [NEEDS CLARIFICATION: Milestone 2: whether a child's branches need one `existingVMs` answer when a VM satisfies none of them.]

---

## Review & acceptance checklist

- [ ] Every user story has Given/When/Then scenarios that can be tested independently.
- [ ] Upgrade with no manual step is specified (US1.5, US9.4, US10.1, US10.7, US11).
- [ ] One governing class per zone, and the class-vs-class bound, are specified (US2).
- [ ] The zone rule for VMs without a zone is specified (US5).
- [ ] Hardware compatibility is asynchronous and never rejects a class (US7).
- [ ] Visibility limits are specified (US7.4, US7.5).
- [ ] Every open question is mirrored as a blocked task in `tasks.md`.
