# Tasks: Flex VM Classes (`VirtualMachineClass` v2)

- **Input**: [`spec.md`](spec.md) + [`plan.md`](plan.md) + [`model.md`](model.md); design in WIKI page 3118085463 and its child pages
- **Epic**: vmop-4034 (design); one phase per implementation epic below
- **Format**: `[ID] [P?] [USx] [vmop-NNN] Description — file paths`
- **E2E**: every story's E2E task ships in the same story and runs in the quarantined capability pipeline (vmop-4314) until enablement (vmop-4337)

Tasks marked `[P]` within a story can run in parallel. The order of epics and what can run in parallel is in [`plan.md`](plan.md).

---

## Phase 0 — Decisions

Each open item changes the shape of an API or a check. A task that depends on one says so.

- [x] T000 CPU requests and limits on the class stay in vCPUs, as today
- [x] T001 A passthrough device requires full memory reservation: the VM webhook enforces it (T405), and fixed classes written through the existing class API keep today's class check
- [x] T002 Number rule: `default` is required when `min` or `max` is set; an unset bound is open; a fixed size is `min == max == default`
- [x] T003 `governs.existingVMs` is required on a governing class with no parent; in Milestone 2 only the top class sets it and children inherit it
- [x] T004 Reserved capacity in Milestone 1: `reservedProfileName {default, allowed, required}` (`EnumPolicy`, not `ListPolicy`, because the VM value is one name) on the class, `spec.reservedProfileName` on the VM, filled by mutation like any default; omission inherits the default, `""` selects no profile, `required: true` rejects empty after defaulting; names must resolve to a `VirtualMachineReservedProfile`, and `allowed` is required when `default` is set (WIKI page 3118085468)
- [x] T007 Conversion of reserved profiles: `reservedProfileID: X` ↔ `{default: X, allowed: [X], required: true}`; down = `default`; up resets to `{Y, [Y], required: true}` when the ID differs from the annotation's default, otherwise restores the annotation. `reservedProfileID` is removed from v1alpha6 (v1alpha5 and older only); the provider may keep writing it through older versions
- [x] T008 A VM with a selected profile, class-backed or classless, must match the profile's shape after defaulting: CPU count, CPU and memory reservation and limit, device identities and quantities; overhead is not compared. A resize past the profile is rejected at admission
- [x] T009 `reservedProfileName` is class-supplied on a class-backed VM (inline-edit rule); it can't change from one set value to another while powered on; vm-operator's unset-to-set backfill is exempt. A class change of a reserved VM is allowed only while powered off (per T018)
- [x] T009a Class change is allowed when `VMResize` or `VMResizeCPUMemory` is on, with no annotation: class-supplied fields are replaced from the new class, fields it doesn't set are cleared and re-defaulted, and the extraConfig profile ID is written or removed from the VM field
- [x] T009b Class edits: the provider's older class API keeps today's block on update and delete while capacity is assigned; edits through v1alpha6 reject only changes that alter an allocated profile's shape or invalidate the class's default VM (the exact field boundary is a test deliverable of T303a)
- [x] T009c `reservedSlots` stays `int32` with the same name and meaning, deprecated, preserved through conversion and the namespace copy including zero, never overwritten with observed totals; no annotation migration
- [x] T009d `VirtualMachineReservedProfile` moves out of vm-operator's API; the provider publishes and backfills it (T910); vm-operator only reads it (T103a)
- [x] T009e Devices are `ListPolicy`s over the VM's inline vGPU and DirectPath I/O devices. Items are flat and mirror the environment-browser entry: `{profileName, count}` and `{vendorID, deviceID, customLabel, count}`, keyed by `profileName` and by `vendorID` + `deviceID`, with the same key names in the environment browser, the class and the VM. Matchers are the key fields, each optional; string key fields take `{fixed, glob, regex}`. Independent count bounds and required entries are deferred
- [x] T009f A collection of independent items is a `ListPolicy`; a collection that is one setting is a `SetPolicy` (`default`, `allowed` sets, `required`; no `denied`). `pnumaNodeAffinity` is an `Int32SetPolicy` in Milestone 1; `cpuFeatureMask` is a top-class preset until Milestone 2
- [x] T005 The environment-browser demand gate is dropped; the CRDs are restricted to admins instead (T107)
- [x] T006 `configSpec` keeps its name
- [ ] T010 Decide who rejects an association zone outside the namespace or the class. Blocks T305 (decided so far: the class in `vmware-system-vmop` carries `spec.zones` and `governs.zones`, the association only narrows them, and `status.zones` covers `spec.zones`)
- [ ] T011 Decide the hardware version shape: a range with a default, its path, and whether a VM below the minimum is upgraded or rejected. Blocks T103 for that field
- [ ] T012 Confirm with its owners that the zone's unreserved-DirectPath setting applies to classless VMs and non-reserved classes with devices (working assumption: it does). Affects only T202's reason for that case
- [ ] T014 Reserved capacity follow-ups: RP3 and RP8 (profile naming across Supervisors and external readers of `reservedSlots` are tracked in the provider's spec)
- [ ] T018 Get the platform's answer (tracked in the provider's spec) on whether a changed or removed profile on a VM is honored without a restart, and what happens to a powered-on VM's slot after a resize. Affects T408 (class change of a reserved VM while powered on)
- [ ] T019 Decide whether an explicit `""` means "no value" for every enum field, as for `reservedProfileName`. Affects T104 and T400

---

## Phase 1 — API, upgrade and capability (vmop-4038)

### Story — Flex VM Classes capability (vmop-4313)

- [ ] T100 [US12] [vmop-4313] Add the Flex VM Classes capability and gate every new behavior on it; v1alpha6 `VirtualMachineClass` objects always use the new shape, whether or not the Flex VM Classes capability is enabled. The capability gates webhook behavior (governance, defaulting from governing classes, the new VM and class rules) and controller reconciliation (compatibility, compliance, placement); toggling it on or off takes effect without a restart (test both directions) — `pkg/config/capabilities`, `pkg/config`

### Story — Quarantined capability pipeline (vmop-4314)

- [ ] T101 [P] [US12] [vmop-4314] A CI pipeline that runs the Flex VM Classes E2E suites with the capability enabled, quarantined until vmop-4337 — `test/e2e`, CI config

### Story — VM Class v1alpha6 schema (vmop-4315)

- [ ] T102 [US1/US9] [vmop-4315] Wire the round-trip annotation into the `VirtualMachineClass` conversion functions before the rest of the schema lands — `api/utilconversion`, `api/v1alpha*/virtualmachineclass_conversion.go`
- [ ] T103 [US1/US2/US10] [vmop-4315] Reshape `VirtualMachineClass` v1alpha6 in place: `resources` (size, requests, limits), `cpuAdvanced`, `memoryAdvanced`, `bootOptions`, `advanced` (incl. `extraConfig`), `hardware.devices`, `hardware.instanceStorage`, the hardware version (per T011), leftover-only `configSpec`, `zones`, `governs` (`zones`, `enforcement`, `existingVMs`), `externalID`, `reservedProfileName`, `parentClassRefs` (reserved, rejected in Milestone 1), `status.zones[]`, the schema-version label — `api/v1alpha6/virtualmachineclass_types.go`
- [ ] T103a [US10] [vmop-4315] Remove `VirtualMachineReservedProfile` from the vm-operator API and its CRD, and add read-only RBAC for it — `api/v1alpha6/`, `config/crd/`, `config/rbac`
- [ ] T103b [US10] [vmop-4315] Remove `ReservedProfileID` from `VirtualMachineClassSpec` in v1alpha6 (replaced by `reservedProfileName`); keep it in v1alpha5 and older for conversion; regenerate deepcopy and CRDs — `api/v1alpha6/virtualmachineclass_types.go`, `config/crd/`
- [ ] T104 [P] [US1/US2] [vmop-4315] The value types (number range, `EnumPolicy` with `required`, `BoolPolicy`, `ListPolicy` with the device form) and their CEL rules, including `governs.zones` ⊆ `spec.zones`, with envtest coverage of every rule — `api/v1alpha6/`
- [ ] T105 [vmop-4315] `make generate-go generate-manifests`; commit generated files

### Story — VM Class upgrade and conversion (vmop-4316)

- [ ] T106 [US1/US9/US10] [vmop-4316] Conversion v1alpha5 and older ↔ v1alpha6: scalars to complete ranges, `hardware.{cpus,memory}` to `resources.size`, `policies.resources` to `resources.requests/limits`, elevated `configSpec` paths moved to typed fields, `externalID` = name, reserved profiles per T007 (`reservedProfileID` in v1alpha5 and older ↔ `reservedProfileName` in v1alpha6, which has no `reservedProfileID`; including the v1alpha1 path the provider writes through), `reservedSlots` preserved including zero. Round-trip test including a multi-entry `allowed` and a changed `reservedProfileID` — `api/v1alpha5/virtualmachineclass_conversion.go`, older versions
- [ ] T106a [US9] [vmop-4316] Three-state zone test: unset, `[]` and a list survive JSON, conversion, `CreateOrUpdate` and back — envtest
- [ ] T106b [US1] [vmop-4316] E2E: an existing class and its VMs behave as before after upgrade

### Story — VM and Zone API fields (vmop-4317)

- [ ] T108 [P] [US3/US6/US10] [vmop-4317] VM API: `spec.reservedProfileName`, `status.governingClassNames`, the `GovernanceCompliant` condition, and the observed fields (firmware, change block tracking, CPU `reservationLockedToMax`, NUMA fixed auto-affinity, SEV, SEV-SNP, TDX, CPU hot-remove, SMC, DirectPath I/O devices) — `api/v1alpha6/virtualmachine_types.go`
- [ ] T108a [US8/US10] [vmop-4317] VM API: inline `hardware.devices.vgpuDevices` and `hardware.devices.dynamicDirectPathIODevices`, with the same item fields and key names as the class items (T009e) — `api/v1alpha6/virtualmachine_types.go`
- [ ] T109 [P] [US2] [vmop-4317] `Zone.status.governingClasses` — `external/tanzu-topology/api/v1alpha1/zone.go`; `make generate-external-manifests`
- [ ] T110 [US10] [vmop-4317] Schema upgrade fills `spec.reservedProfileName` on existing VMs from their class's `reservedProfileName.default`; this unset-to-set write is exempt from the powered-on lock — `pkg/providers/vsphere/upgrade/virtualmachine/vm_schema_upgrade.go`

### Story — Remove VirtualMachineConfigPolicy (vmop-4318)

- [ ] T111 [P] [US11] [vmop-4318] Remove the `Zone` controller's policy fan-out and its tests — `controllers/infra/zone/zone_controller.go`, `zone_controller_test.go`
- [ ] T112 [P] [US11] [vmop-4318] Remove the policy types, CRD and feature flag — `external/vim/api/v1alpha1/virtualmachine_config_policy_types.go`, `config/crd/external-crds/`, the embedded CRD list, `Features.VirtualMachineConfigPolicy` in `pkg/config`
- [ ] T113 [P] [US11] [vmop-4318] Remove the `virtualmachineconfigpolicies` RBAC rules — `config/rbac/role.yaml` (from markers)
- [ ] T114 [US11] [vmop-4318] E2E: remove the policy assertions, keep the environment-browser specs, and assert no policy exists after upgrade — `test/e2e/vmservice/vmservice/configpolicy/`

### Story — Environment browser completion (vmop-4319, includes vmop-3964)

- [ ] T107 [US7] [vmop-4319] Restrict `ConfigTarget`, `VirtualMachineConfigOptions` and `VirtualMachineGuestOptions` to admins and the vm-operator service account — `config/rbac`
- [ ] T115 [US7] [vmop-4319] Maximum hardware version per cluster, and a conservative cluster choice for options co-owned by several `ConfigTarget`s (vmop-3964) — `controllers/vim/*`, `controllers/virtualmachineconfigoptions`
- [ ] T116 [US7] [vmop-4319] No EnvironmentBrowser query on resync when the cluster is unchanged; a cluster change re-queries. Unit test on fake-provider call counts
- [ ] T117 [US7] [vmop-4319] E2E: a DevOps user gets 403 on the three CRDs; admins succeed

---

## Phase 2 — Governance engine and class compatibility (vmop-4039)

### Story — Governance engine (vmop-4320)

- [ ] T200 [US2/US3] [vmop-4320] Resolve the governing class for a zone, and compare a VM's effective configuration, or a class's ranges, with it, field by field, including vCPU against MHz using the cluster's minimum host frequency — new package under `pkg/`, exported for the webhooks, the reconciler and placement

### Story — Class hardware compatibility (vmop-4321)

- [ ] T201 [US7] [vmop-4321] Class controller: per-zone `HardwareCompatible` from `ConfigTarget` and the options objects, asynchronous, re-checked as hardware and zones change; never rejects (zones per T010); watch `ConfigTarget` and `Zone`. Depends on T115 — `controllers/virtualmachineclass`
- [ ] T201a [US7] [vmop-4321] E2E: a class with a zone-specific vGPU profile is accepted and reports per-zone `HardwareCompatible`

### Story — Class resolved limits (vmop-4322)

- [ ] T202 [US7] [vmop-4322] Class status: resolved per-zone limits and options for every governed field (ranges as `{min, max}`; enums, sets and devices as the available values with display fields), from the environment-browser objects within the governing class; per-VM only, only what governance permits; updated when the environment-browser objects change; the unreserved-DirectPath reason per T012
- [ ] T202a [US7] [vmop-4322] E2E: resolved limits never exceed the governing class, and a device list shows only the devices the zone has

### Story — Class governance status (vmop-4323)

- [ ] T203 [US2] [vmop-4323] Class controller: per-zone `WithinGovernance` and `Governing`, re-checked on every reconcile — `controllers/virtualmachineclass`
- [ ] T204 [US2] [vmop-4323] Zone mirror controller: `Zone.status.governingClasses` from classes; optimistic-lock patch, skip unchanged writes; never read by governance. Depends on T109 — `controllers/infra/zone`
- [ ] T205 [US4] [vmop-4323] Re-base the `ClassInstance` hash onto typed fields — `controllers/virtualmachineclass`
- [ ] T205a [US2] [vmop-4323] E2E: narrowing a governing class marks a selectable class `WithinGovernance=False`

---

## Phase 3 — Class webhooks and provider API markers (vmop-4201)

The class APIs, their generation and the namespace-copy integration are provider work (T912–T916).

### Story — Class mutating webhook (vmop-4324)

- [ ] T300 [US2/US8] [vmop-4324] Fill `governs.enforcement: Deny` and set the memory reservation lock on a preset class that attaches devices; number ranges are not completed — `webhooks/virtualmachineclass/mutation`
- [ ] T300a [US2/US8] [vmop-4324] E2E: a written governing class reads back with `enforcement: Deny`, and a preset device class with the memory lock set

### Story — Class validating webhook (vmop-4325)

- [ ] T301 [US1/US2/US9/US12] [vmop-4325] Value-shape rules (T002) not covered by CEL, elevated `configSpec` paths, `externalID` immutable, `existingVMs` required (T003), `parentClassRefs` rejected in Milestone 1; the new class rules are gated on the capability — `webhooks/virtualmachineclass/validation`
- [ ] T302 [US2] [vmop-4325] One governing class per zone; a selectable class must fit inside the zone's governing class, including `advanced.extraConfig.default` against `allowed`/`denied` — `webhooks/virtualmachineclass/validation`
- [ ] T303 [US10] [vmop-4325] A class's default configuration must match its default profile, and every allowed profile must fit within the class's resource and device policy (bounds are not fixed by listing a profile); every name in `reservedProfileName` resolves to a `VirtualMachineReservedProfile` in the namespace; `allowed` is required when `default` is set — `webhooks/virtualmachineclass/validation`
- [ ] T303a [US10] [vmop-4325] On class update through the v2 path, reject only changes that alter an allocated profile's shape or make the class's default VM invalid for it; tests pin the exact field boundary (T009b) — `webhooks/virtualmachineclass/validation`
- [ ] T304 [US9] [vmop-4325] Sticky v2 writes: reject a write through a CRD version older than the class's last-writer label
- [ ] T305 [US9] [vmop-4325] Namespace-copy checks: the zones belong to the namespace; zones with reserved capacity are included (per T010)
- [ ] T306 [US2/US9] [vmop-4325] E2E: a second governing class for a zone and a selectable class outside the ceiling are rejected; a v1 write to a v2 class is rejected

### Story — Provider API mapping markers (vmop-4326)

- [ ] T307 [US9] [vmop-4326] The mapping markers the provider's API generation reads on the v1alpha6 class types, and a CI check that every field has a mapping or an explicit exclusion — `api/v1alpha6/virtualmachineclass_types.go`, value types in `api/v1alpha6/`
- [ ] T308 [US9] [vmop-4326] Schema test on the generated CRD: every governed field matches exactly one value shape by its field set; device lists are map lists keyed by the same key fields as their environment-browser entries; matcher lists are atomic — test under `api/`

---

## Phase 4 — VM create and edit with governing classes (vmop-4040)

### Story — VM defaults from classes (vmop-4330)

- [ ] T400 [US4/US3/US10] [vmop-4330] VM mutating webhook: on create and on `className` change (when `VMResize` or `VMResizeCPUMemory` is on, no annotation), write the class's values into the VM spec, clearing and re-defaulting fields the new class doesn't set; for unset governed fields use the image value or the governing default; fill `reservedProfileName` from the class default (omission inherits, `""` = none, per T019 for other enums) — `webhooks/virtualmachine/mutation`
- [ ] T401 [US4] [vmop-4330] VM reconciler: rewrite class-supplied fields for VMs with the same-class resize annotation, as a spec patch with an optimistic lock separate from the deferred patch helper — `pkg/providers/vsphere`
- [ ] T402 [US1/US4] [vmop-4330] E2E: a VM that names a ranged class gets its defaults; a classless VM gets the governing defaults; a class change replaces and re-defaults the class-supplied fields

### Story — VM governance validation (vmop-4331)

- [ ] T403 [US2/US3/US4] [vmop-4331] VM validating webhook: governance through T200; reject inline edits of class-supplied fields while `className` is set; reject a classless VM in an ungoverned zone; reject an image value the governing class doesn't allow; messages follow the visibility rule — `webhooks/virtualmachine/validation`
- [ ] T404 [US2/US3/US4] [vmop-4331] E2E: a classless VM beyond the ceiling, an ExtraConfig key the class denies, an inline edit and a classless VM in an ungoverned zone are rejected; removing `className` keeps the values

### Story — Device memory reservation (vmop-4332)

- [ ] T405 [US8] [vmop-4332] VM validating webhook: memory fully reserved when the effective configuration has a vGPU or DirectPath I/O device (create, and spec changes while attached; not metadata, status or vm-operator writes) — `webhooks/virtualmachine/validation`
- [ ] T406 [US8] [vmop-4332] E2E: a device VM without full reservation is rejected; a VM of a fixed v1 class is unaffected

### Story — Zone selection rule (vmop-4333)

- [ ] T407 [US5/US7] [vmop-4333] VM validating webhook: require `spec.zone` when the possible zones don't all have the same governing class, naming the zones and their classes; reject a VM in a zone where its class is `HardwareCompatible=False`, with that zone's reason. Depends on T201 — `webhooks/virtualmachine/validation`
- [ ] T407a [US5/US7] [vmop-4333] E2E: a VM without a zone is rejected when its zones are governed differently, and a VM in an incompatible zone is rejected

### Story — Reserved capacity (vmop-4334)

Depends on the provider's reserved profile publication (T910).

- [ ] T408 [US10] [vmop-4334] VM validating webhook: `reservedProfileName` must be in the class's (or governing class's) `allowed`; after defaulting, every VM with a selected profile must match the `VirtualMachineReservedProfile` shape (T008), naming the field and the profile; a resize past the profile is rejected; inline edits on a class-backed VM are rejected; a set value can't change to another while powered on, unset-to-set by vm-operator is allowed; a class change of a reserved VM only while powered off (per T018) — `webhooks/virtualmachine/validation`
- [ ] T409 [US10] [vmop-4334] Write the extraConfig profile ID from `spec.reservedProfileName`, falling back to the class's `reservedProfileName` (not the removed `reservedProfileID`) while the VM field is unset, and remove the key when the field is empty (fixes the stale `resourcepool.vmResourceProfileId` after moving to an unreserved class). Runs after T110 — `pkg/providers/vsphere/virtualmachine/configspec.go`
- [ ] T411 [US10] [vmop-4334] Move the class controller's `ReservedProfileID` reader (including the `ClassInstance` hash `Reserved` bool) to `reservedProfileName`, since v1alpha6 has no `reservedProfileID` — `controllers/virtualmachineclass/virtualmachineclass_controller.go`
- [ ] T410 [US10] [vmop-4334] E2E: a VM of a reserved class keeps its reservation after upgrade; a VM whose shape differs from its profile is rejected; a VM moved to an unreserved class loses the profile ID

---

## Phase 5 — VM compliance, placement and enablement (vmop-4041)

### Story — VM governance compliance (vmop-4335)

- [ ] T500 [US6] [vmop-4335] VM reconciler: re-run the governance check once the zone is known, set `GovernanceCompliant` and `status.governingClassNames`, withhold power-on for `DenyPowerOn`; never change power state itself; populate the observed status fields — `pkg/providers/vsphere`, `controllers/virtualmachine`
- [ ] T501 [US6] [vmop-4335] VM validating webhook: reject a user-requested power-on of a non-compliant VM under `DenyPowerOn` — `webhooks/virtualmachine/validation`
- [ ] T502 [US6] [vmop-4335] E2E: `AllowOnViolation` and `DenyPowerOn` after a governing class is narrowed; deleting the governing class reports `NoGoverningClass`

### Story — Governance-aware placement (vmop-4336)

- [ ] T503 [US5] [vmop-4336] Placement: send only the zones whose governing class admits the VM and whose hardware is compatible; a group placed into one zone uses the zones every member admits — `pkg/providers/vsphere/placement`
- [ ] T504 [US5] [vmop-4336] E2E: a VM without a zone is placed only in a zone that admits it

### Story — Flex VM Classes enablement (vmop-4337)

- [ ] T505 [US12] [vmop-4337] Take the capability pipeline out of quarantine, run the full E2E sweep, enable the capability. Depends on T910 and T911

### Story — Flex VM Classes docs (vmop-4338, absorbs vmop-3739)

- [ ] T506 [P] [vmop-4338] User and developer docs for v1alpha6 classes, governance, reserved profiles and the environment-browser pipeline, linked from the docs index — `docs/`

---

## Phase 6 — Tenant VM Classes, Milestone 2 (vmop-4312)

### Story — Tenant VM Classes capability (vmop-4339)

- [ ] T600 [US13] [vmop-4339] A second capability that requires the Flex VM Classes capability

### Story — Parent class references (vmop-4340)

- [ ] T601 [US13] [vmop-4340] `parentClassRefs`: one parent, no depth limit, cycles rejected, ranges within the parent's, live per-field resolution; only the top class carries `configSpec` and `existingVMs`
- [ ] T602 [US13] [vmop-4340] Reject deleting a referenced parent; fail closed if one disappears; report a rejected platform-driven delete
- [ ] T603 [US13] [vmop-4340] E2E: a child outside its parent is rejected; a referenced parent can't be deleted

### Story — Class families per zone (vmop-4341)

- [ ] T604 [US13] [vmop-4341] One family per zone (R1–R5); branches behind a flag; `existingVMs` from the top class
- [ ] T605 [US13] [vmop-4341] E2E: a VM is checked against its family

### Story — Tenant classes through v2 vAPI (vmop-4342)

- [ ] T606 [US13] [vmop-4342] `parentClassRefs` round-trips through the v2 vAPI; VCFA can build its tenant portal on it

### Story — Tenant VM Classes enablement (vmop-4343)

- [ ] T607 [US13] [vmop-4343] E2E sweep, docs, enable the capability

---

## Provider work (tracked in the provider's spec)

Dependency tasks; vm-operator does not implement them. What vm-operator assumes from them is in spec.md, "Provider contract".

- [ ] T910 [US10] [vmop-TBD] Publish a namespaced `VirtualMachineReservedProfile` per profile with its shape and per-zone counts, backfilled for existing allocations before the capability is enabled. Blocks vmop-4334 and T505
- [ ] T911 [US12] [vmop-TBD] Advertise the Flex VM Classes capability once T910 and T914 hold. Blocks T505
- [ ] T912 [US9] [vmop-4327] The per-Supervisor v2 class API. Depends on T103 and T307 (through T915)
- [ ] T913 [US9/US10] [vmop-4328] The older class API (rejects writes to v1alpha6 classes per T304, keeps today's block on classes with capacity, reports the schema version) and the namespace API with per-class zones; reserved profiles built from a flex class's defaults
- [ ] T914 [US9] [vmop-4329] Supervisor and namespace-copy integration: copy classes through v1alpha6 with unset and `[]` zones kept apart, report rejected copies; end-to-end E2E of a class written through the v2 API
- [ ] T915 [US9] [vmop-4392] Generate the class API definition from the v1alpha6 CRD and its mapping markers, with a drift check. Depends on T103 and T307
- [ ] T916 [vmop-4393] A drift check for new vpxd VM config fields the class neither covers nor excludes

## Dependencies on other teams (not vm-operator code)

- [ ] T900 Supervisor team: the 1.5 capability, the class migrator, backup and restore of classes, one list of default classes and whether they can be edited
- [ ] T901 vAPI team: generator support for the class API definition (needed by T915)
- [ ] T902 VCFA: move to the v2 API; whether region quota reads class sizes
- [ ] T903 vCenter UI: hide edit actions for classes last written through v1alpha6
