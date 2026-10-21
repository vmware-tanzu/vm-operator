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
- [x] T004 Reserved capacity in Milestone 1: `reservedProfile {default, allowed}` on the class, `spec.reservedProfileName` on the VM, filled by mutation like any default and checked against the class's fixed values; `reservedSlots` kept as is (WIKI page 3118085468)
- [x] T005 The environment-browser demand gate is dropped; the CRDs are restricted to admins instead (T107)
- [x] T006 `configSpec` keeps its name
- [ ] T010 Decide who rejects an association zone outside the namespace or the class. Blocks T305 (decided so far: the class in `vmware-system-vmop` carries `spec.zones` and `governs.zones`, the association only narrows them, and `status.zones` covers `spec.zones`)
- [ ] T011 Decide the hardware version shape: a range with a default, its path, and whether a VM below the minimum is upgraded or rejected. Blocks T103 for that field
- [ ] T012 Confirm with its owners that the zone's unreserved-DirectPath setting applies to classless VMs and non-reserved classes with devices (working assumption: it does). Affects only T202's reason for that case
- [ ] T013 Decide whether set is its own value shape or a form of list. Blocks T104 for `SetPolicy`
- [ ] T014 Reserved capacity follow-ups: RP5 (may `reservedProfileName` change while powered off), RP3, RP8, RC3, RC6, and whether anything outside vm-operator reads `reservedSlots`. RP5 affects T408
- [ ] T015 Get wcp-namespace-operator's sign-off on vAPI page §6.2, and the Supervisor team's decision on default classes (vAPI page O12)
- [ ] T016 Run the `configSpec` usage survey and choose which leftover fields to elevate first
- [ ] T017 Design the vpxd config field drift check. Blocks T310

---

## Phase 1 — API, upgrade and capability (vmop-4038)

### Story — Flex VM Classes capability (vmop-4313)

- [ ] T100 [US12] [vmop-4313] Add the Flex VM Classes capability and gate every new behavior on it — `pkg/config/capabilities`, `pkg/config`

### Story — Quarantined capability pipeline (vmop-4314)

- [ ] T101 [P] [US12] [vmop-4314] A CI pipeline that runs the Flex VM Classes E2E suites with the capability enabled, quarantined until vmop-4337 — `test/e2e`, CI config

### Story — VM Class v1alpha6 schema (vmop-4315)

- [ ] T102 [US1/US9] [vmop-4315] Wire the round-trip annotation into the `VirtualMachineClass` conversion functions before the rest of the schema lands — `api/utilconversion`, `api/v1alpha*/virtualmachineclass_conversion.go`
- [ ] T103 [US1/US2/US10] [vmop-4315] Reshape `VirtualMachineClass` v1alpha6 in place: `resources` (size, requests, limits), `cpuAdvanced`, `memoryAdvanced`, `bootOptions`, `advanced` (incl. `extraConfig`), `hardware.devices`, `hardware.instanceStorage`, the hardware version (per T011), leftover-only `configSpec`, `zones`, `governs` (`zones`, `enforcement`, `existingVMs`), `externalID`, `reservedProfile`, `parentClassRefs` (reserved, rejected in Milestone 1), `status.zones[]`, the schema-version label — `api/v1alpha6/virtualmachineclass_types.go`
- [ ] T104 [P] [US1] [vmop-4315] The value types (number range, `EnumPolicy`, `BoolPolicy`, `ListPolicy` with the device form, `SetPolicy` per T013) and their CEL rules, with envtest coverage of every rule — `api/v1alpha6/`
- [ ] T105 [vmop-4315] `make generate-go generate-manifests`; commit generated files

### Story — VM Class upgrade and conversion (vmop-4316)

- [ ] T106 [US1/US9/US10] [vmop-4316] Conversion v1alpha5 and older ↔ v1alpha6: scalars to complete ranges, `hardware.{cpus,memory}` to `resources.size`, `policies.resources` to `resources.requests/limits`, elevated `configSpec` paths moved to typed fields, `externalID` = name, `reservedProfileID: X` ↔ `reservedProfile: {default: X, allowed: [X]}` (including the v1alpha1 path wcpsvc writes through) — `api/v1alpha5/virtualmachineclass_conversion.go`, older versions
- [ ] T106a [US9] [vmop-4316] Three-state zone test: unset, `[]` and a list survive JSON, conversion, `CreateOrUpdate` and back — envtest
- [ ] T106b [US1] [vmop-4316] E2E: an existing class and its VMs behave as before after upgrade

### Story — VM and Zone API fields (vmop-4317)

- [ ] T108 [P] [US3/US6/US10] [vmop-4317] VM API: `spec.reservedProfileName`, `status.governingClassNames`, the `GovernanceCompliant` condition, and the observed fields (firmware, change block tracking, CPU `reservationLockedToMax`, NUMA fixed auto-affinity, SEV, SEV-SNP, TDX, CPU hot-remove, SMC, DirectPath I/O devices) — `api/v1alpha6/virtualmachine_types.go`
- [ ] T109 [P] [US2] [vmop-4317] `Zone.status.governingClasses` — `external/tanzu-topology/api/v1alpha1/zone.go`; `make generate-external-manifests`
- [ ] T110 [US10] [vmop-4317] Schema upgrade fills `spec.reservedProfileName` on existing VMs from their class's `reservedProfileID` — `pkg/vmopv1util` schema-upgrade step

### Story — Remove VirtualMachineConfigPolicy (vmop-4318)

- [ ] T111 [P] [US11] [vmop-4318] Remove the `Zone` controller's policy fan-out and its tests — `controllers/infra/zone/zone_controller.go`, `zone_controller_test.go`
- [ ] T112 [P] [US11] [vmop-4318] Remove the policy types and CRD — `external/vim/api/v1alpha1/virtualmachine_config_policy_types.go`, `config/crd/external-crds/`, the embedded CRD list
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

- [ ] T201 [US7] [vmop-4321] Class controller: per-zone `HardwareCompatible` from `ConfigTarget` and the options objects, asynchronous, re-checked as hardware and zones change; never rejects (zones per T010); watch `ConfigTarget` and `Zone` — `controllers/virtualmachineclass`
- [ ] T201a [US7] [vmop-4321] E2E: a class with a zone-specific vGPU profile is accepted and reports per-zone `HardwareCompatible`

### Story — Class resolved limits (vmop-4322)

- [ ] T202 [US7] [vmop-4322] Class status: resolved per-zone limits, per-VM only, only what governance permits; the unreserved-DirectPath reason per T012
- [ ] T202a [US7] [vmop-4322] E2E: resolved limits never exceed the governing class

### Story — Class governance status (vmop-4323)

- [ ] T203 [US2] [vmop-4323] Class controller: per-zone `WithinGovernance` and `Governing`, re-checked on every reconcile
- [ ] T204 [US2] [vmop-4323] Zone mirror controller: `Zone.status.governingClasses` from classes; optimistic-lock patch, skip unchanged writes; never read by governance
- [ ] T205 [vmop-4323] Re-base the `ClassInstance` hash onto typed fields — `controllers/virtualmachineclass`
- [ ] T205a [US2] [vmop-4323] E2E: narrowing a governing class marks a selectable class `WithinGovernance=False`

---

## Phase 3 — Class webhooks and v2 vAPI (vmop-4201)

### Story — Class mutating webhook (vmop-4324)

- [ ] T300 [US1/US8] [vmop-4324] Fill complete number ranges, `governs.enforcement: Deny`, and the memory lock on a preset class that attaches devices — `webhooks/virtualmachineclass/mutation`
- [ ] T300a [US1] [vmop-4324] E2E: a written class reads back with its defaults filled

### Story — Class validating webhook (vmop-4325)

- [ ] T301 [US1/US2/US9] [vmop-4325] Value-shape rules (T002), elevated `configSpec` paths, `externalID` immutable, `governs.zones` within `spec.zones`, `existingVMs` required (T003), `parentClassRefs` rejected in Milestone 1 — `webhooks/virtualmachineclass/validation`
- [ ] T302 [US2] [vmop-4325] One governing class per zone; a selectable class must fit inside the zone's governing class, including `advanced.extraConfig.default` against `allowed`/`denied`
- [ ] T303 [US10] [vmop-4325] A class that lists reserved profiles fixes the profile's fields (`min == max` on vCPU, CPU reservation = limit, memory size, reservation = limit, devices)
- [ ] T304 [US9] [vmop-4325] Sticky v2 writes: reject a write through a CRD version older than the class's last-writer label
- [ ] T305 [US9] [vmop-4325] Namespace-copy checks: the zones belong to the namespace; zones with reserved capacity are included (per T010)
- [ ] T306 [US2/US9] [vmop-4325] E2E: a second governing class for a zone and a selectable class outside the ceiling are rejected; a v1 write to a v2 class is rejected

### Story — vmodl markers (vmop-4326)

- [ ] T307 [US9] [vmop-4326] The vmodl markers the generator reads on the v1alpha6 class types, and a CI check that every field has a mapping or an explicit exclusion

### Story — vmodl generator hookup and drift pipeline (vmop-4392)

- [ ] T308 [US9] [vmop-4392] Run the OpenAPI → vmodl generator on the v1alpha6 class, check in the generated vmodl with golden-file tests, and add a pre-commit job that regenerates it and fails on drift. Depends on T103 and T307

### Story — vpxd config field drift pipeline (vmop-4393)

- [ ] T310 [vmop-4393] A pre-commit job that fails when vpxd adds a VM config field the class doesn't cover or exclude, naming the field and the owner. Blocked on T017

### Story — v2 class vAPI (vmop-4327)

- [ ] T311 [US9] [vmop-4327] The per-Supervisor v2 class vAPI on the generated vmodl: create, update, get, list, read-only `schemaVersion`. Depends on T308

### Story — wcpsvc class API changes (vmop-4328)

- [ ] T312 [P] [US9] [vmop-4328] The v1 class API fans out to every Supervisor, rejects edits to classes last written through v2, and the namespace API carries per-class zones

### Story — Supervisor and nsop integration (vmop-4329)

- [ ] T313 [P] [US9] [vmop-4329] Track the Supervisor 1.5 vAPI proxy, wcp-namespace-operator copying classes through the new CRD version (unset and `[]` kept apart), and the vCenter UI hiding edit for v2 classes (per T015)
- [ ] T314 [US9] [vmop-4329] E2E: a class written through the v2 API reaches the namespace copy with every field, and three-state zones survive end to end

---

## Phase 4 — VM create and edit with governing classes (vmop-4040)

### Story — VM defaults from classes (vmop-4330)

- [ ] T400 [US4/US3/US10] [vmop-4330] VM mutating webhook: on create and on `className` change, write the class's values into the VM spec; for unset governed fields use the image value or the governing default; fill `reservedProfileName` from the class default — `webhooks/virtualmachine/mutation`
- [ ] T401 [US4] [vmop-4330] VM reconciler: rewrite class-supplied fields for VMs with the same-class resize annotation, with an optimistic-lock patch — `pkg/providers/vsphere`
- [ ] T402 [US1/US4] [vmop-4330] E2E: a VM that names a ranged class gets its defaults; a classless VM gets the governing defaults

### Story — VM governance validation (vmop-4331)

- [ ] T403 [US2/US3/US4] [vmop-4331] VM validating webhook: governance through T200; reject inline edits of class-supplied fields while `className` is set; reject a classless VM in an ungoverned zone; reject an image value the governing class doesn't allow; messages follow the visibility rule — `webhooks/virtualmachine/validation`
- [ ] T404 [US2/US3/US4] [vmop-4331] E2E: a classless VM beyond the ceiling, an ExtraConfig key the class denies, an inline edit and a classless VM in an ungoverned zone are rejected; removing `className` keeps the values

### Story — Device memory reservation (vmop-4332)

- [ ] T405 [US8] [vmop-4332] VM validating webhook: memory fully reserved when the effective configuration has a vGPU or DirectPath I/O device (create, and spec changes while attached; not metadata, status or vm-operator writes)
- [ ] T406 [US8] [vmop-4332] E2E: a device VM without full reservation is rejected; a VM of a fixed v1 class is unaffected

### Story — Zone selection rule (vmop-4333)

- [ ] T407 [US5/US7] [vmop-4333] VM validating webhook: require `spec.zone` when the possible zones don't all have the same governing class, naming the zones and their classes; reject a VM in a zone where its class is `HardwareCompatible=False`, with that zone's reason
- [ ] T407a [US5/US7] [vmop-4333] E2E: a VM without a zone is rejected when its zones are governed differently, and a VM in an incompatible zone is rejected

### Story — Reserved capacity (vmop-4334)

- [ ] T408 [US10] [vmop-4334] VM validating webhook: `reservedProfileName` must be in the class's (or governing class's) `allowed`; the profile's fields must equal the class's fixed values; the name can't change while the VM is powered on (per T014)
- [ ] T409 [US10] [vmop-4334] Write the extraConfig profile ID from `spec.reservedProfileName` instead of the class — `pkg/providers/vsphere/virtualmachine/configspec.go`
- [ ] T410 [US10] [vmop-4334] E2E: a VM of a reserved class keeps its reservation after upgrade; a VM whose shape differs from its profile is rejected

---

## Phase 5 — VM compliance, placement and enablement (vmop-4041)

### Story — VM governance compliance (vmop-4335)

- [ ] T500 [US6] [vmop-4335] VM reconciler: re-run the governance check once the zone is known, set `GovernanceCompliant` and `status.governingClassNames`, withhold power-on for `DenyPowerOn`; never change power state itself; populate the observed status fields — `pkg/providers/vsphere`
- [ ] T501 [US6] [vmop-4335] VM validating webhook: reject a user-requested power-on of a non-compliant VM under `DenyPowerOn`
- [ ] T502 [US6] [vmop-4335] E2E: `AllowOnViolation` and `DenyPowerOn` after a governing class is narrowed; deleting the governing class reports `NoGoverningClass`

### Story — Governance-aware placement (vmop-4336)

- [ ] T503 [US5] [vmop-4336] Placement: send only the zones whose governing class admits the VM and whose hardware is compatible; a group placed into one zone uses the zones every member admits — `pkg/providers/vsphere/placement`
- [ ] T504 [US5] [vmop-4336] E2E: a VM without a zone is placed only in a zone that admits it

### Story — Flex VM Classes enablement (vmop-4337)

- [ ] T505 [US12] [vmop-4337] Take the capability pipeline out of quarantine, run the full E2E sweep, enable the capability

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

## Dependencies on other teams (not vm-operator code)

- [ ] T900 Supervisor team: the 1.5 capability, per-class zones on `vmClassRefs`, the vcdb → catalog migrator, backup and restore of catalog classes, one list of default classes
- [ ] T901 vAPI team: `openapi-compiler` support for the vmodl annotations, or the fallback route
- [ ] T902 VCFA: move to the v2 API; whether region quota reads class sizes
- [ ] T903 vCenter UI: hide edit actions for classes last written through v2
