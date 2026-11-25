# Feature Specification: Flex VM Classes (`VirtualMachineClass` v2)

- **Feature branch**: `faisal/vmop-4034-flex-vm-classes-spec`; amended by `faisal/vmop-4034-vmclass-v2-no-classless`
- **Created**: 2026-10-08
- **Amended**: 2026-11-25 — VMs must name a class (no classless VMs for DevOps users) and `governs` is removed; a VM edits its fields within its class's bounds; fields a class doesn't list are closed unless the class allows them (research.md, Finding 13)
- **Status**: In Progress (Milestone 1 specified and scheduled; Milestone 2 designed, scheduled after Milestone 1)
- **Epic**: vmop-4034 (design). Implementation epics: vmop-4038, vmop-4039, vmop-4201, vmop-4040, vmop-4041 (Milestone 1) and vmop-4312 (Milestone 2). The epic split and order are in [`plan.md`](plan.md).
- **Design**: WIKI page 3119061398 — "One Pager: VM Service: Flex VM Classes"; WIKI page 3118085463 — "Design: VM Service: Flex VM Classes" (Kubernetes side, authoritative), with its child pages for the full schema, the vAPI, reserved capacity and the Supervisor models. Summaries and pointers are in [`research.md`](research.md); the API surface is in [`model.md`](model.md).
- **Plan**: [`plan.md`](plan.md)
- **Tasks**: [`tasks.md`](tasks.md)
- **Supersedes**: the `VirtualMachineConfigPolicy` work formerly in spec 001 (Stories S8, S9.a, S9.b and S11); spec [`001-environment-browser`](../001-environment-browser/) now covers environment-browser discovery only

---

## Summary

`VirtualMachineClass` stays the object a VM names, and its fields become ranges instead of fixed values:

1. Every configuration field a class can bound has a typed value shape: number (`min`, `max`, `default`), enum, boolean or list. A fixed size is `min == max == default`, so one class can offer one size or a range. CPU requests and limits stay in vCPUs, as in today's class. A list policy bounds how many items a VM list holds and which ones, and the first matching entry applies to an item; each device type has its own list, and NICs have one list with per-type policies.
2. A VM names a class, as today. It inherits the class's defaults at admission and may set or change any field within the class's bounds. Only privileged accounts can create a VM without a class, as today (import, restore, failover).
3. A field a class doesn't list is closed: a VM that names the class can't set it. `spec.unlistedFieldPolicy: Allow` opens every unlisted field, bounded by the hardware (or by a parent class in Milestone 2), for a class that exposes the hardware without listing each field.
4. A class change rewrites what a class change resizes today (CPU count, memory, CPU and memory requests, limits and shares) and keeps the rest of the VM spec, which must fit the new class. A class edit never changes existing VMs; a VM that no longer fits reports it and keeps running.
5. Classes are zonal: `spec.zones` says where a class is available.
6. `spec.externalID` holds the client's identity for the class, separate from the Kubernetes name.
7. `configSpec` keeps only pass-through fields that have no typed field.
8. Classes live in each Supervisor's `vmware-system-vmop` namespace and are copied into namespaces; they are written through the provider's class APIs, and vm-operator's webhooks validate every write.
9. Reserved capacity follows the VM: a class offers reserved profiles as a bounded field, the VM carries the profile it uses and must match its shape, and the profiles themselves are read from `VirtualMachineReservedProfile` objects that the provider publishes.
10. `VirtualMachineConfigPolicy` is removed.

Milestone 1 (provider classes) ships all of the above behind the `supports_vm_service_classless_vm` Supervisor capability. Milestone 2 (tenant classes) adds `parentClassRefs`, so a tenant can define a class that narrows another class.

---

## Goals

- Existing classes and their clients, including v1alpha5 clients such as VKS, **MUST** keep working unchanged after upgrade, with no manual step. A converted class **MUST NOT** let a VM set anything a VM of that class couldn't set before the upgrade.
- A VM **MUST** name a class; only privileged accounts can create a VM without one, as today.
- A VM **MUST** get its unset fields from its class's defaults at admission, and **MAY** set or change any field within its class's bounds. A VM is checked against the one class it names; classes are never merged.
- A field a class doesn't list **MUST** be closed to the VM unless the class sets `unlistedFieldPolicy: Allow`.
- A class edit **MUST NOT** change existing VMs. A VM that no longer fits its class **MUST** report it and keep running.
- Hardware compatibility **MUST** be reported per zone on the class, asynchronously; a class **MUST NOT** be rejected for hardware.
- A namespace user **MUST NOT** be able to read more about hardware or other classes than they could obtain.
- A VM that holds reserved capacity **MUST** match its reserved profile's shape.
- The Milestone 1 schema **MUST** allow Milestone 2 without a breaking change.
- Flex behavior, Telco VM APIs, and the configuration-policy/environment-browser pipeline **MUST** share the disabled-by-default `supports_vm_service_classless_vm` Supervisor capability (vmop-4313), which replaces the Telco and configuration-policy capabilities. Enablement is tracked by vmop-4337. The key keeps its name although DevOps users can't create classless VMs.

## Non-goals

- VMs without a class for DevOps users.
- A zone- or namespace-wide policy that bounds every VM. An admin who doesn't want a field set restricts it in every class assigned to the namespace.
- Creating or editing classes through the Kubernetes API by namespace users; classes are written by platform components only.
- Redesigning placement; placement only receives a narrower zone list.
- Bringing VCFA's provider and tenant concepts into vm-operator. Which classes are copied into which namespaces is the provider's choice.
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
5. **Given** an existing v1alpha5 class, **when** it is read as v1alpha6, **then** each scalar field is `{min: v, max: v, default: v}` and the class behaves exactly as before (US17).
6. **Given** a class or a VM, **when** CPU requests or limits are written, **then** they are in cores (millicores allowed) on both, an unset limit means unlimited, and the provider converts cores to MHz with the cluster's minimum host frequency; there is no percentage form.
7. **Given** a VM whose effective requests exceed its limits, or whose memory request exceeds its memory size, **when** it is written, **then** it is rejected, naming the fields; **given** a class whose defaults break those rules or whose bounds can't be met (`requests.min` above `limits.max`), **then** the class is rejected.
8. **Given** an existing class with `hardware.instanceStorage`, **when** it is read as v1alpha6, **then** instance storage is kept as a preset value and VMs that name the class get it as today.

### US2 — CSP admin: a class bounds what a VM that names it can set (P1)

**Acceptance scenarios**:

1. **Given** a class `flex` with `resources.size.cpu: {min: 1, max: 16, default: 2}`, **when** a VM that names `flex` requests 32 CPUs, **then** it is rejected, naming the field and the bounds.
2. **Given** a class with `advanced.extraConfig.denied: [{key: "guestinfo.*"}]`, **when** a VM of that class sets `guestinfo.foo`, **then** it is rejected; **given** an `allowed` entry `{key: "sched.cpu.affinity", value: {allowed: ["0-3"]}}`, **when** the VM sets that key to another value, **then** it is rejected.
3. **Given** a class with `unlistedFieldPolicy: Deny` that doesn't list `cpuAdvanced.latencySensitivity`, **when** a VM of that class sets it, **then** it is rejected, naming the field; **given** the class lists it as `{}`, **then** any value is admitted and the class supplies no default.
4. **Given** a class with `unlistedFieldPolicy: Allow`, **when** a VM of that class sets a field the class doesn't list, **then** it is admitted if the value is within what the zone's hardware reports (and within the parent class in Milestone 2); fields the class lists are still checked against their policies.
5. **Given** a class written without `unlistedFieldPolicy`, **when** it is stored, **then** it is `Deny`.
6. **Given** a class with `unlistedFieldPolicy: Deny` that doesn't list `resources.size.cpu` or `resources.size.memory`, **when** it is written, **then** it is rejected: a VM must have CPU and memory and couldn't set them; **given** `{}` for them, **then** a VM of the class must set them. **Given** a class with `unlistedFieldPolicy: Allow` that doesn't list them, **then** it is accepted and a VM of the class must set them.
7. **Given** a DevOps user, **when** they create a VM without `spec.className`, **then** it is rejected, as today; **given** a privileged account creating an imported, restored or failed-over VM without a class, **then** it is admitted as today, is not checked against a class, and reports `ClassCompliant=False`.
8. **Given** an admin who doesn't want a field set in a namespace, **when** they configure it, **then** they restrict the field in every class assigned to the namespace; there is no policy above the classes.

### US3 — DevOps user: a VM inherits its class's defaults and edits within its bounds (P1)

**Acceptance scenarios**:

1. **Given** a VM created with `spec.className: large`, **when** it is admitted, **then** every default `large` sets is written into the VM spec for fields the VM leaves unset; fields the VM sets are kept and checked against `large`.
2. **Given** that VM, **when** an update changes a field within `large`'s bounds, **then** it is admitted; vm-operator applies it as it applies a resize today, and `VirtualMachineClassConfigurationSynced` is `False` until it is applied. [NEEDS CLARIFICATION: the condition's reason for an edit that isn't a class change (T039).]
3. **Given** that VM, **when** an update sets a value outside `large`'s bounds, **then** it is rejected, naming the field and the bounds.
4. **Given** `large` is edited, **when** VMs of `large` reconcile, **then** their specs are unchanged; a VM that no longer fits reports `ClassCompliant=False` (US6).
5. **Given** a VM of `large`, **when** the same-class resize annotation is added or its value changes, **then** the mutation webhook rewrites the fields a class change rewrites (US4.1) from `large`'s current values, once; a later edit of `large` doesn't reach the VM until the annotation changes again. In the 9.1 release, the annotation made the VM follow every later edit of the class.
6. **Given** a class edit that changes a typed field, **when** the class controller reconciles, **then** the class's `ClassInstance` hash changes.
7. **Given** a DevOps user, **when** an update removes `spec.className`, **then** it is rejected, as today.

### US4 — DevOps user: a VM changes its class (P1)

**Acceptance scenarios**:

1. **Given** a VM of class `A`, **when** `spec.className` changes to `B` (allowed where resize is allowed today), **then** the mutation webhook writes `B`'s values for CPU count, memory, and CPU and memory requests, limits and shares; one that `B` doesn't set is cleared, so the VM gets the vSphere default (no reservation, unlimited, `Normal` shares), as in the 9.1 release (research.md, Finding 14).
2. **Given** that change, **when** it is validated, **then** every other field of the VM spec is kept, `B`'s other defaults (for example a default vGPU) are not added, and the whole spec must fit `B`; a field that doesn't (for example a vGPU `B` doesn't allow) is rejected, naming it, and the user fixes it in the same update.
3. **Given** the same update also changes a field `B`'s values would rewrite (for example CPU), **when** it is admitted, **then** the user's value is kept and must fit `B`.
4. **Given** a powered-on VM, **when** its class changes, **then** the spec is written at once and vm-operator applies it at the next power-off, as a resize today; `VirtualMachineClassConfigurationSynced` is `False` with reason `ClassNameChanged` until then.
5. **Given** `B` isn't available in the VM's zone, or is `HardwareCompatible=False` there, **when** the class changes, **then** it is rejected, naming the zone's reason.
6. **Given** a VM with a reserved profile, **when** its class changes, **then** US10.9 and US10.10 apply.

### US5 — DevOps user: a VM without a zone is placed where its class fits (P1)

**Acceptance scenarios**:

1. **Given** a VM without `spec.zone`, **when** it is created, **then** it is admitted with its class's defaults; they are the same in every zone.
2. **Given** a VM without `spec.zone`, **when** it is placed, **then** placement receives only the zones where its class is available and `HardwareCompatible`, and where the VM's values are within the zone's resolved limits (US7.7).
3. **Given** a VM placed in `zone-a`, **when** it is later updated, **then** it is validated against `zone-a` only; it is never moved to another zone.

### US6 — CSP admin: VMs that no longer fit their class (P1)

**Acceptance scenarios**:

1. **Given** a VM that falls outside its class after the class is narrowed, **when** the VM reconciles, **then** it reports `ClassCompliant=False`, naming the fields, and keeps running; a power cycle succeeds, and vm-operator changes neither its spec nor its power state.
2. **Given** a VM that reports `ClassCompliant=False`, **when** an update changes some fields, **then** only the changed fields (and rules that involve them, such as requests within limits) are checked against the class; fields left as they were are not. A class change checks the whole spec (US4.2). [NEEDS CLARIFICATION: confirm this ratcheting rule (T036).]
3. **Given** a VM's class is deleted, **when** the VM reconciles, **then** it keeps its values and running, and reports `ClassCompliant=False`, as for any VM that doesn't fit its class.
4. **Given** an existing VM after upgrade, **when** the schema upgrade fills its spec from vSphere (US17.4), **then** a value that doesn't fit its class makes it report `ClassCompliant=False`; it keeps running, as an imported VM does.
5. **Given** a VM, **when** it reconciles, **then** its status reports the observed firmware, change block tracking, SEV, SEV-SNP, TDX, CPU hot-remove, SMC and DirectPath I/O devices, so compliance can be read from the VM.

### US7 — CSP admin: hardware compatibility is reported, not enforced on the class (P1)

**Acceptance scenarios**:

1. **Given** a class requesting a vGPU profile that exists only in `zone-a`, **when** it is created in `vmware-system-vmop` or copied into a namespace, **then** the write is accepted.
2. **Given** that class, **when** the class controller reconciles, **then** `status.zones` reports `HardwareCompatible=True` for `zone-a` and `False` with a reason for the other zones, and re-checks as hardware changes.
3. **Given** that class, **when** a VM that names it is created in a zone where it is not compatible, **then** the VM is rejected with that zone's reason.
4. **Given** a namespace user, **when** they read the class status or a rejection message, **then** they see per-VM limits and what the class permits, never pool totals or devices the class doesn't allow.
5. **Given** a DevOps user, **when** they read `ConfigTarget`, `VirtualMachineConfigOptions` or `VirtualMachineGuestOptions`, **then** the request is forbidden; only admins and vm-operator can read them.
6. **Given** a resync with no cluster change, **when** the environment-browser controllers reconcile, **then** vCenter is not queried again; a cluster change triggers a new query.
7. **Given** a class, **when** the class controller reconciles, **then** `status.zones[]` lists, for every field a VM of the class can set, the values a VM can get in that zone: what the environment browser reports, within the class (and the parent in Milestone 2), with the entry's display fields. With `unlistedFieldPolicy: Allow`, an unlisted field is limited by the environment browser alone; with `Deny`, it is not listed. A form renders its options from this status, or, for a new class with nothing above it, from the environment-browser objects an admin can read; it never combines restrictions itself. [NEEDS CLARIFICATION: status size and write rate at scale (T029).]

### US8 — Admin: a VM with a passthrough device has its memory fully reserved (P1)

**Acceptance scenarios**:

1. **Given** a VM whose effective configuration has a vGPU or DirectPath I/O device, **when** its memory is not fully reserved, **then** it is rejected at create, and on spec changes while the device is attached, naming the fix.
2. **Given** such a VM, **when** only metadata, finalizers or status change, or vm-operator writes it, **then** the write is not rejected.
3. **Given** a class whose defaults attach a device, **when** it is written, **then** the class mutating webhook sets the memory reservation lock; a VM that names it with less than full reservation is rejected at VM admission.
4. **Given** a fixed class written through the existing class API, **when** it attaches a device, **then** the class itself still requires full memory reservation, as today.

### US9 — Admin and automation: classes keep their shape across API versions (P1)

**Acceptance scenarios**:

1. **Given** a class created or last updated through the v1alpha6 CRD version, **when** a client writes it through an older CRD version, **then** the write is accepted and converted (T025): a rendered value the writer changed replaces the default, and the rest is restored from the conversion annotation. **When** an admin edits it through the provider's older class API, **then** the edit is rejected; reads still work.
2. **Given** a class in `vmware-system-vmop`, **when** the provider copies it into a namespace through v1alpha6, **then** every v1alpha6 field reaches the namespace copy.
3. **Given** an association entry with `zones` unset, `[]` or a list, **when** the namespace copy is written, **then** the three states stay distinct through every layer.
4. **Given** an existing class, **when** it is migrated, **then** its `metadata.name` is unchanged and `spec.externalID` equals the name.
5. **Given** a change to the v1alpha6 class types, **when** a field has neither a provider API mapping marker nor an explicit exclusion, **then** the field-coverage check fails.
6. **Given** the v1alpha6 class CRD schema, **when** the schema test runs, **then** every field a class can bound matches exactly one value shape by its field set, every class policy pairs with exactly one VM field (the NIC type-key fields and `hardware.version` by their named mappings), the names under a NIC type key don't collide, and every device list is a map list keyed by `name` whose class entries use the same key fields as its environment-browser entry.

### US10 — CSP admin: reserved capacity works with flex classes (P1)

**Acceptance scenarios**:

1. **Given** an existing class with `reservedProfileID: X`, **when** it is read as v1alpha6, **then** it has `reservedProfileName: {default: X, allowed: [X], required: true}` and no `reservedProfileID` (removed in v1alpha6), and VMs that name it keep using the reservation.
2. **Given** a v1alpha6 class whose `reservedProfileName` has several allowed names, **when** it is read through v1alpha5 (where `reservedProfileID` = `default`) and written back unchanged, **then** the v1alpha6 value is restored; **when** an older-API writer changes `reservedProfileID` to `Y`, **then** the class becomes `{default: Y, allowed: [Y], required: true}`.
3. **Given** a class whose `reservedProfileName` has a default, **when** a VM that names it is created without `spec.reservedProfileName`, **then** the mutation webhook fills the default; an explicit `""` selects no profile, and is rejected when `required: true`.
4. **Given** a class whose `reservedProfileName` sets `default` without `allowed`, or names a profile with no `VirtualMachineReservedProfile` in the namespace, or whose default configuration doesn't match its default profile, or lists a profile that doesn't fit within its resource and device policy, **when** it is written, **then** it is rejected. Listing a profile does not fix the class's bounds; VMs are validated against the selected profile's shape (scenario 6).
5. **Given** a VM that sets `spec.reservedProfileName`, **when** the name is not in its class's `allowed` list, **then** it is rejected.
6. **Given** a VM with a selected profile after defaulting, including a privileged VM without a class, **when** its CPU count, CPU reservation or limit, memory reservation or limit, or device identities (with how many devices have each) differ from the profile, **then** it is rejected, naming the field and the profile. Overhead is not compared. A resize past the profile is rejected the same way.
7. **Given** a powered-off VM, **when** an update changes `spec.reservedProfileName` to another name its class allows, **then** it is admitted if the VM matches the new profile's shape.
8. **Given** a powered-on VM with `spec.reservedProfileName` set, **when** an update changes it to another set value, **then** it is rejected; vm-operator setting it from unset (schema upgrade) is allowed.
9. **Given** a powered-on VM with a reserved profile, **when** its class changes, **then** it is rejected; the class can change only while the VM is powered off. [NEEDS CLARIFICATION: whether the platform honors a changed or removed profile on a VM without a restart of its management service; until it does, this stays powered-off only. Tracked in the provider's spec.]
10. **Given** a VM whose class changes, **when** the change is admitted, **then** the mutation webhook sets `spec.reservedProfileName` from the new class's default, or clears it when the new class has none, and vm-operator writes or removes the profile ID in the VM's extraConfig.
11. **Given** an existing VM of a reserved class, **when** it is upgraded, **then** `spec.reservedProfileName` is filled from its class, and vm-operator sets the VM's reservation from the VM field, falling back to the class while the field is unset.
12. **Given** a class with VMs holding an allocated profile, **when** it is edited through v1alpha6, **then** only changes that alter the profile's shape or make the class's default VM invalid for it are rejected. (The provider's older class API keeps today's block on update and delete while capacity is assigned; see "Provider contract".)
13. **Given** a class with `reservedSlots`, **when** it is converted or copied into a namespace, **then** the value, including zero, is kept and never replaced with observed totals.

### US11 — Platform engineer: `VirtualMachineConfigPolicy` is removed (P2)

**Acceptance scenarios**:

1. **Given** a Supervisor upgraded to this release, **when** a `Zone` reconciles, **then** no `VirtualMachineConfigPolicy` is created, and the CRD is not installed.
2. **Given** the environment-browser pipeline from spec 001, **when** the policy is removed, **then** `ConfigTarget`, `VirtualMachineConfigOptions` and `VirtualMachineGuestOptions` keep working unchanged.

### US12 — Platform engineer: flex behavior is gated by `supports_vm_service_classless_vm` (P1)

**Acceptance scenarios**:

1. **Given** a Supervisor without the `supports_vm_service_classless_vm` capability, **when** a class or VM is written, **then** VMs behave as they do today, and the class write is not rejected for using ranges or `unlistedFieldPolicy`. v1alpha6 `VirtualMachineClass` objects always use the new shape, whether or not the capability is enabled. The capability gates webhook behavior (defaults from ranged classes, edits within bounds, the class-change rewrite, the new VM and class rules) and controller reconciliation (compatibility, compliance, placement).
2. **Given** the capability is enabled or disabled, **when** the capability is read by vm-operator, **then** the new webhook rules and the class controller turn on or off without a restart.
3. **Given** the quarantined E2E pipeline, **when** it runs, **then** it runs the Flex VM Classes suites with the capability enabled, separately from the main pipeline.

### US13 — Tenant admin: a class that narrows another class (Milestone 2, P3)

A class lists one parent in `spec.parentClassRefs`; it can only narrow its parent: a child that exceeds its parent is rejected, fields it doesn't list come from the chain, and a referenced parent can't be deleted. A child's `unlistedFieldPolicy: Allow` opens its unlisted fields only as far as the parent allows them. Only the top class of a chain may set `configSpec`. Parent and child live in `vmware-system-vmop`, and vm-operator resolves the parent there, so a namespace copy of a child works whether or not its parent is copied into the namespace. Which classes reach a namespace is the provider's choice; a child narrows what a DevOps user can do only when its parent isn't also copied into the namespace. [NEEDS CLARIFICATION: what happens to a child, and its VMs, when its parent is narrowed below it; proposed: the parent edit is accepted, the child reports it, and VMs of the child are checked against the child within its parent (T042).] Designed in the design page §8; behind its own capability, which depends on the Milestone 1 capability.

### US14 — CSP admin: a class bounds device and NIC lists (P1)

**Acceptance scenarios**:

1. **Given** a class with `hardware.vGPUs.allowed: [{profileName: nvidia-a100-80c, count: {max: 1}}, {profileName: "nvidia-a100-*"}]`, **when** a VM asks for two `nvidia-a100-80c` vGPUs, **then** it is rejected, naming the entry's count; one 80c and two 40c are admitted.
2. **Given** the same class, **when** a VM asks for an `nvidia-h100-80c` vGPU, **then** it is rejected because no `allowed` entry matches; **given** `denied: [{profileName: nvidia-a100-10c}]`, **when** a VM asks for a 10c, **then** it is rejected even though `nvidia-a100-*` matches it.
3. **Given** entries that both match an item, **when** the VM is validated, **then** only the first applies to it: its field policies apply (for example `enableUVM: {allowOverride: false, default: false}` on the 80c entry) and the later entry's do not.
4. **Given** a class `hardware.vGPUs.default` and a VM that omits `hardware.vGPUs`, **when** it is admitted, **then** the default items are filled in; **given** a VM that sets the list, including `[]`, **then** its list is validated as supplied and nothing is merged.
5. **Given** a class whose `default` exceeds its `count`, **when** it is written, **then** it is rejected by CEL; **given** a `default` item that is denied, matched by no `allowed` entry, or exceeds its entry's `count`, **then** the class webhook rejects it; **given** a malformed glob, **then** it is rejected.
6. **Given** `count` unset at the list or entry level, **when** VMs are validated, **then** the class sets no count limit and only the platform's limits apply.
7. **Given** a class with `network.interfaces: {count: {min: 1, max: 4}, type: {allowed: [VMXNet3]}, vmxnet3: {ctxPerDev: {allowed: [PerQueue], default: PerQueue}}}`, **when** a VM omits `ctxPerDev` on a VMXNET3 NIC, **then** it gets `PerQueue`; **when** a VM has five NICs or an SR-IOV NIC, **then** it is rejected.
8. **Given** a class with an ordered `network.interfaces.default`, **when** a VM omits `interfaces`, **then** it gets those NICs in that order.
9. **Given** a device `allowed` entry that matches nothing a zone has, **when** the class controller reconciles, **then** that zone reports `DevicePolicyResolved=False`, and nothing is rejected.

### US15 — CSP admin: a class bounds the hardware version (P1)

**Acceptance scenarios**:

1. **Given** a class with `hardware.version: {min: 20, max: 22, default: 21}`, **when** a VM that sets no minimum is created, **then** it gets vmx-21; **when** a VM sets `minHardwareVersion: 23`, **then** it is rejected.
2. **Given** a device or an image that needs a higher version than the class default, **when** the VM is created, **then** it gets that version if it is ≤ `max`, and is rejected otherwise, naming the image's or the device's version.
3. **Given** an existing VM below a raised `min`, **when** it reconciles, **then** it reports `ClassCompliant=False` and is not upgraded; **when** the user raises its `minHardwareVersion`, **then** it is upgraded at the next power-off and becomes compliant.
4. **Given** a class write that lowers `max` below a version some VMs already have, **when** it is written, **then** it is accepted with a warning, and those VMs report `ClassCompliant=False`.
5. **Given** a v1alpha5 class with `configSpec.version: vmx-21`, **when** it is read as v1alpha6, **then** it has `hardware.version: {min: 21, default: 21}` and no `max`.
6. **Given** a zone whose maximum hardware version is below the class's `max` or default, **when** the class controller reconciles, **then** that zone reports `HardwareCompatible=False` with the reason.

### US16 — CSP admin: CPU and memory shares (P1)

**Acceptance scenarios**:

1. **Given** a VM with `resources.shares.cpu: {level: Custom, value: 4000}`, **when** it is created, **then** vCenter shows custom CPU shares of 4000; **given** a `value` with any other level, or `value` ≤ 0, **then** it is rejected.
2. **Given** a class with `resources.shares.cpu.level: {allowed: [Normal, High], default: High}`, **when** a VM omits shares, **then** it gets `High`; **when** it sets `Custom`, **then** it is rejected.
3. **Given** a VM with no shares and a class that sets none, **when** it is created, **then** it gets `Normal`, as today.
4. **Given** a v1alpha5 class whose `configSpec` sets CPU or memory allocation shares, **when** it is read as v1alpha6, **then** they are in `resources.shares`.

### US17 — Admin and automation: upgrade changes nothing a client can see (P1)

**Acceptance scenarios**:

1. **Given** an existing v1alpha5 class, **when** it is read as v1alpha6, **then** it has `unlistedFieldPolicy: Deny`; every value the class sets is fixed to that value, unless a v1alpha5 VM could already set the field; and every field a v1alpha5 VM could set while naming a class is listed as open, with the class's value as its default when it has one (model.md, "Conversion").
2. **Given** a v1alpha5 client such as VKS, **when** it creates or updates a VM that names a converted class, setting the fields it sets today (for example network interfaces, `minHardwareVersion` or boot options), **then** the VM is admitted as in the 9.1 release.
3. **Given** a converted class whose `configSpec` attaches a vGPU, **when** a VM of that class changes the vGPU profile or adds a device, **then** it is rejected, as the 9.1 release gives the VM no way to do it.
4. **Given** an existing VM, **when** it first reconciles after upgrade, **then** the schema upgrade fills its CPU count, memory, CPU and memory requests and limits, shares and device lists from vSphere, as the telco backfill does today; **given** its class changed and the resize hasn't been applied yet (the last-resized annotation differs from `spec.className`), **then** the resize fields are filled from the class instead, so the pending resize isn't lost.

---

## Scope of work

**vm-operator**:

- The v1alpha6 `VirtualMachineClass` schema (reshaped in place; v1alpha6 has not shipped), value types, `unlistedFieldPolicy`, conversion from v1alpha5 and older, and round-trip annotations.
- The VM API fields: the `ClassCompliant` condition, observed status fields, `spec.reservedProfileName`, `spec.resources.shares`; the per-type device lists with vmop-3791.
- The class's list policies (devices, NICs, extraConfig), `hardware.version` and `resources.shares`, applied by one evaluator.
- Class mutating and validating webhooks.
- VM mutating and validating webhooks: defaults at admission, the check against the VM's class, the class-change rewrite, the one-shot same-class resize, ratcheting, device memory reservation, reserved profiles.
- The `VirtualMachineClass` controller: per-zone `HardwareCompatible` and `DevicePolicyResolved`, and resolved limits.
- The VM reconciler: compliance, observed status, the schema-upgrade backfill, and the reservation from the VM field.
- Moving `VirtualMachineReservedProfile` out of the vm-operator API; vm-operator only reads it.
- Placement: the zone filter for single VMs and groups.
- The environment-browser completion from spec 001: maximum hardware version, admin-only access, no re-query on resync.
- The provider API mapping markers on the v1alpha6 class Go types, which the provider's API generation reads, and a check that every field has a mapping or an explicit exclusion.
- The capability, the quarantined pipeline, enablement and docs.
- Removing `VirtualMachineConfigPolicy`.

**Provider (tracked in the provider's spec)**: the v2 class API and the older class API, the namespace API with per-class zones, generating the class API definition from the CRD and its drift checks, publishing `VirtualMachineReservedProfile` and building reserved profiles from class defaults, advertising the capability, and the Supervisor and namespace-copy integration, including which classes are copied into which namespaces (plan.md, "Provider work"; tasks.md T910–T916).

**Other teams**: the Supervisor team (the 1.5 capability, the class migrator), VCFA (move to the v2 API; whether region quota reads class sizes; which classes it copies into namespaces), VKS (classes without a CPU or memory default), the vCenter UI (hide edit actions for classes written through v1alpha6).

---

## Provider contract

vm-operator runs on a Supervisor whose provider supplies the pieces below. vm-operator states what it assumes, not how the provider does it; the provider's spec tracks the implementation (plan.md, "Provider work").

- **Reserved profiles are published.** The provider publishes a namespaced `VirtualMachineReservedProfile` for every profile a namespace can use, with the profile's shape (CPU count, CPU reservation and limit, memory reservation and limit, device identities and quantities) and per-zone counts. vm-operator only reads it.
- **Backfilled before the capability.** Objects exist for every existing allocation before the `supports_vm_service_classless_vm` capability is enabled. A profile name that doesn't resolve is a clear admission failure in vm-operator, with no fallback.
- **Older writers keep working.** The provider may keep writing `reservedProfileID` and `reservedSlots` through older API versions; vm-operator converts them (US10.1, US10.2, US10.13).
- **Capability.** The provider advertises the `supports_vm_service_classless_vm` capability; vm-operator only reads it (US12).
- **Platform enforcement.** The platform rejects, at power-on, a VM whose configuration doesn't match its named profile. vm-operator rejects the same mismatch earlier, at admission.
- **Class and namespace APIs.** The v2 class API, the older class API (including its block on editing a class with assigned capacity) and the namespace API with per-class zones are the provider's. Every class they write lands in `vmware-system-vmop` and is validated by vm-operator's webhooks; the provider copies classes into namespaces through v1alpha6, keeping unset and `[]` zone lists apart. The v2 class API exposes `unlistedFieldPolicy`.
- **Which classes reach a namespace.** The provider decides which classes it copies into each namespace. vm-operator only checks a VM against the class it names; in Milestone 2 it resolves a child's parent in `vmware-system-vmop`, so the parent doesn't have to be copied.

---

## Edge cases

- A client that drops `[]` turns "no zones" into "all zones"; for availability this fails open. Mitigated by pointer types, a three-state round-trip test and an E2E test.
- A class edit between admission and placement can leave a VM in a zone where it no longer fits; it reports `ClassCompliant=False` like any VM that no longer fits.
- An elevated field arriving in `configSpec` from an older writer is moved to the typed field; from a writer on the version where the field is typed, it is rejected.
- The platform checks a VM against its reserved profile only when it is placed and powered on, and rejects a mismatch with no fallback. The VM webhook enforces the profile's shape so the same failure surfaces at admission instead. [NEEDS CLARIFICATION: what happens to a powered-on VM's slot after a resize; tracked in the provider's spec.]
- A VM moved to an unreserved class must lose its extraConfig profile ID; vm-operator writes or removes it from `spec.reservedProfileName`.
- A VM whose class changed in the 9.1 release while powered on still has its resize pending at upgrade; the schema upgrade fills the resize fields from the class, not from vSphere (US17.4).
- A converted v1alpha5 VM whose device came from its old class's `configSpec` has the device in its spec after the backfill; a class change to a class that doesn't allow it is rejected, naming the device (US4.2).
- A VM that sets the same-class resize annotation through a v1alpha5 client expects the 9.1 behavior (following every class edit); it gets one rewrite per annotation change (US3.5). Release notes say so.
- A class whose `unlistedFieldPolicy` changes from `Allow` to `Deny` leaves VMs that set unlisted fields non-compliant; they keep running (US6.1).

---

## Open questions

- An association zone outside the namespace or the class is reported per zone in `status.zones[]` (`ZoneNotInNamespace`, `ZoneNotInClass`), not rejected, because a zone can leave the namespace or the class after the association is written (T010). [NEEDS CLARIFICATION: implementation: whether the provider's namespace API also rejects such a zone up front.]
- [NEEDS CLARIFICATION: Agreement from the owner of the VM PCI device design (WIKI page 2477879529, vmop-3791) on one VM list per device type instead of one `pciDevices` union, and on its status list, `unitNumber` assignment across the lists, and the 128-device cap across them (model.md, "Devices").]
- [NEEDS CLARIFICATION: Confirmation from its owners that the zone's unreserved-DirectPath setting applies to VMs of non-reserved classes with devices and to privileged VMs without a class.] Working assumption: it does.
- [NEEDS CLARIFICATION: Reserved capacity: RP3 profiles not tied to a class and how VCFA discovers them; RP8 read access to reservations for a later per-zone status.] Findings in WIKI page 3118085468.
- [NEEDS CLARIFICATION: Number fields where a bound has no meaningful default (shares `value`, NIC `vnumaNodeID`): whether their `Range` makes `default` optional, against the number rule (T002). Decided when T104 and T120 are implemented (T021).]
- Provider-side open items (platform behavior on a changed profile or a resize, profile naming across Supervisors, external readers of `reservedSlots`, the class API generator, namespace-copy sign-off, default classes, the vpxd config field drift check) are tracked in the provider's spec; vm-operator tasks that depend on them say so.
- [NEEDS CLARIFICATION: CPU cores to MHz (research.md, Finding 12): which frequency the provider uses and when it resolves, so a frequency change doesn't reconfigure existing VMs. Decided when vmop-4481 is implemented (T022). It also decides whether a v1alpha5 class's `configSpec` CPU allocation in MHz can be elevated into `resources.requests/limits.cpu` (T023).]
- [NEEDS CLARIFICATION: the name of `unlistedFieldPolicy` (T034).]
- [NEEDS CLARIFICATION: ratcheting for VMs that don't fit their class (T036).]
- [NEEDS CLARIFICATION: the `VirtualMachineClassConfigurationSynced` reason for an edit within bounds (T039).]
- [NEEDS CLARIFICATION: class status size and write rate at scale, and how to bound them (T029).]
- [NEEDS CLARIFICATION: Milestone 2: a child whose parent is narrowed below it (T042).]

---

## Review & acceptance checklist

- [ ] Every user story has Given/When/Then scenarios that can be tested independently.
- [ ] Upgrade with no manual step, and no behavior change for v1alpha5 clients, is specified (US1.5, US9.4, US10.1, US10.11, US11, US17).
- [ ] Defaults, edits within bounds and the unlisted-field rule are specified (US2, US3).
- [ ] The class change and the same-class resize are specified, matching the 9.1 release (US3.5, US4).
- [ ] Compliance without enforcement is specified (US6).
- [ ] The zone rule for VMs without a zone is specified (US5).
- [ ] Hardware compatibility is asynchronous and never rejects a class (US7).
- [ ] Visibility limits are specified (US7.4, US7.5).
- [ ] List policies (counts, first match, defaults only for an omitted list), hardware version and shares are specified (US14–US16).
- [ ] Every open question is mirrored as a blocked task in `tasks.md`.
