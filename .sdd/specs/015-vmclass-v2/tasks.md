# Tasks: Flex VM Classes (`VirtualMachineClass` v2)

- **Input**: [`spec.md`](spec.md) + [`plan.md`](plan.md) + [`model.md`](model.md); design in WIKI page 3118085463 and its child pages
- **Epic**: vmop-4034 (design); one phase per implementation epic below
- **Format**: `[ID] [P?] [USx] [vmop-NNN] Description — file paths`
- **E2E**: every story's E2E task ships in the same story and runs in the quarantined capability pipeline (vmop-4314) until enablement (vmop-4337)
- **Amended 2026-11-25**: classless VMs for DevOps users and `governs` are removed (T030). Superseded decisions keep their IDs and say what replaced them.

Tasks marked `[P]` within a story can run in parallel. The order of epics and what can run in parallel is in [`plan.md`](plan.md).

---

## Phase 0 — Decisions

Each open item changes the shape of an API or a check. A task that depends on one says so.

- [x] T000 CPU requests and limits stay absolute, in cores (millicores allowed), as in v1alpha5, on both the class and the VM; no percentage form. The VM's `spec.resources` CPU requests and limits change from MHz to cores (T108b). An unset limit is unlimited
- [x] T001 A passthrough device requires full memory reservation: the VM webhook enforces it (T405), and fixed classes written through the existing class API keep today's class check
- [x] T002 Number rule: `default` is required when `min` or `max` is set; an unset bound is open; a fixed size is `min == max == default`; `{}` is any value with no default
- [x] T003 Superseded by T030: `governs` and `existingVMs` are removed
- [x] T004 Reserved capacity in Milestone 1: `reservedProfileName {default, allowed, required}` (`EnumPolicy`, not `ListPolicy`, because the VM value is one name) on the class, `spec.reservedProfileName` on the VM, filled by mutation like any default; omission inherits the default, `""` selects no profile, `required: true` rejects empty after defaulting; names must resolve to a `VirtualMachineReservedProfile`, and `allowed` is required when `default` is set (WIKI page 3118085468)
- [x] T007 Conversion of reserved profiles: `reservedProfileID: X` ↔ `{default: X, allowed: [X], required: true}`; down = `default`; up resets to `{Y, [Y], required: true}` when the ID differs from the annotation's default, otherwise restores the annotation. `reservedProfileID` is removed from v1alpha6 (v1alpha5 and older only); the provider may keep writing it through older versions
- [x] T008 A VM with a selected profile, including a privileged VM without a class, must match the profile's shape after defaulting: CPU count, CPU and memory reservation and limit, device identities and quantities; overhead is not compared. A resize past the profile is rejected at admission
- [x] T009 `reservedProfileName` can change within the class's `allowed` names; it can't change from one set value to another while powered on; vm-operator's unset-to-set backfill is exempt. A class change of a reserved VM is allowed only while powered off (per T018). (The inline-edit rule it referred to is superseded by T031.)
- [x] T009a Superseded by T032: a class change rewrites only the resize fields
- [x] T009b Class edits: the provider's older class API keeps today's block on update and delete while capacity is assigned; edits through v1alpha6 reject only changes that alter an allocated profile's shape or invalidate the class's default VM (the exact field boundary is a test deliverable of T303a)
- [x] T009c `reservedSlots` stays `int32` with the same name and meaning, deprecated, preserved through conversion and the namespace copy including zero, never overwritten with observed totals; no annotation migration
- [x] T009d `VirtualMachineReservedProfile` moves out of vm-operator's API; the provider publishes and backfills it (T910); vm-operator only reads it (T103a)
- [x] T009e List policies (model.md, "List policies"): `{count, default, allowed, denied}`; `count` is a `CountRange {min, max}` with no `default`, optional at every level, every item counts 1; a string matcher is one glob (`path.Match`, no wildcard = exact, no regex), numeric keys match exactly; `denied` first, then the first matching `allowed` entry applies to the item and carries its `count` and field policies; `default` has the VM's item shape and fills only an omitted list. No total across device types
- [x] T009g Devices: one VM list per device type, one named item per device with no `count` (`hardware.vGPUs`, `hardware.dynamicDirectPathIODevices`; `hardware.pciDeviceGroups` deferred to vmop-4485), mirrored at the same class paths; replaces the earlier `{profileName, count}` items. The VM side is agreed with vmop-3791 (T020)
- [x] T009h NICs: class `network.interfaces.{count, type, default, vmxnet3}`; one ordered `default` list; the `vmxnet3` type key holds `count` and the flattened policies for the common interface fields and the `vmxnet3` sub-struct; no SR-IOV type key in this spec (research.md, Finding 9); `pciSlotNumber` is a VM-only field (vmop-3576)
- [x] T009i `StringPolicy {allowed, denied, default, required}` for free-form strings (NIC `network.name`, `coalescingParams`)
- [x] T009j Shares: VM and class `resources.shares.{cpu,memory}: {level, value}` (vmop-4479); Milestone 1 checks `value > 0`
- [x] T009f A collection of independent items is a `ListPolicy`; a collection that is one setting is a `SetPolicy` (`default`, `allowed` sets, `required`; no `denied`). `pnumaNodeAffinity` is an `Int32SetPolicy` in Milestone 1; `cpuFeatureMask` is a top-class preset until Milestone 2
- [x] T005 The environment-browser demand gate is dropped; the CRDs are restricted to admins instead (T107)
- [x] T006 `configSpec` keeps its name
- [x] T010 An association zone outside the namespace or the class is reported, not rejected. The class in `vmware-system-vmop` carries `spec.zones`, the association only narrows it, and `status.zones` covers `spec.zones`. A zone can leave the namespace or the class after the association is written, so an admission check can't keep it valid. vm-operator accepts the namespace copy and reports the zone in `status.zones[]` with a condition (`False`, reason `ZoneNotInNamespace` or `ZoneNotInClass`); the class is not available in that zone, as if it weren't listed. Open for implementation: whether the provider's namespace API also rejects such a zone up front, so the admin sees the error at the call. Unblocks T305
- [x] T011 Hardware version: class `hardware.version {min, max, default}` as `int32`; it bounds the actual version; at create the highest of the VM minimum, the class default and the device minimum, ≤ `max`; the image's version counts too, and an image above `max` is rejected; existing VMs outside the range report `ClassCompliant=False`, fixed below `min` by raising `minHardwareVersion` (upgrade at power-off), not fixable above `max`; conversion `vmx-N` → `{min: N, default: N}`, no version → `{}` (model.md, "Hardware version")
- [ ] T012 Confirm with its owners that the zone's unreserved-DirectPath setting applies to VMs of non-reserved classes with devices and to privileged VMs without a class (working assumption: it does). Affects only T202's reason for that case
- [ ] T014 Reserved capacity follow-ups: RP3 and RP8 (profile naming across Supervisors and external readers of `reservedSlots` are tracked in the provider's spec)
- [ ] T018 Get the platform's answer (tracked in the provider's spec) on whether a changed or removed profile on a VM is honored without a restart, and what happens to a powered-on VM's slot after a resize. Affects T408 (class change of a reserved VM while powered on)
- [x] T019 An explicit `""` is not an enum value unless the field defines one; only `reservedProfileName` does (`""` = no profile, a `*string` VM field). Every other planned enum has an explicit "off" value or always has a value (model.md, `EnumPolicy`). The VM's `vmxnet3.coalescingScheme` CEL rule accepts `""` today; fixed by vmop-4482. Affects T104 and T400
- [ ] T021 Number fields whose bound has no meaningful default; decide when T104 and T120 are implemented. The number rule (T002) requires a `default` whenever `min` or `max` is set, so an omitted VM value still lands in range. Two fields break it. Shares `value`: a class that defaults to `Normal` but bounds custom values (`{min: 1000, max: 4000}`) is forced to set `value.default`, which is only valid with `level: Custom`. NIC `vnumaNodeID`: bounding the allowed nodes (`{min: 0, max: 1}`) forces a default node, so every NIC is pinned even when the VM wants vSphere to choose. Proposed: `default` is optional on fields where unset has its own meaning (vSphere decides), and the bounds then apply only when the VM sets the value. Open: which fields qualify (a per-field marker or a list in the schema test); whether a status or form must show that the field is unset by default; whether `vnumaNodeID` should be a `SetPolicy` of allowed node IDs instead. Affects T104 and T120
- [ ] T020 Agree with vmop-3791's owner (WIKI page 2477879529) on one VM list per device type instead of the `pciDevices` union: item fields, per-type or one status list, `unitNumber` assignment from the slot space shared across the lists and SR-IOV NICs, unit-number uniqueness and the 128-device cap across lists, and whether an explicit `[]` must survive (a pointer list) so a VM can opt out of a class's default devices. Blocks T108a
- [ ] T022 CPU reservation and limit in cores: which MHz the provider uses and when it resolves cores to MHz. Open; decided when vmop-4481 is implemented. vSphere takes only MHz, so the provider converts on every write and every drift check. Facts: today a class's cores are resolved once, at create or resize, with one minimum host frequency across all the Supervisor's clusters (`vSphereVMProvider.minCPUFreq`), and the 9.1 release never reconfigures a VM when that frequency changes. On main, `OverwriteSpecComputeConfig` runs on every reconcile and compares MHz with vCenter, so cores in the VM spec converted with the current frequency would reconfigure every VM with a reservation or limit when the frequency changes. Comparing in cores (live MHz / frequency) doesn't avoid this, because the quotient moves just as much. MHz to millicores isn't exactly reversible (one millicore is the frequency / 1000 MHz), so the backfill must not cause a reconfigure on the first reconcile after upgrade. Open: which frequency (the Supervisor-wide minimum, the VM's cluster, or a replacement for the frequency calculation); when to resolve (for example, record the applied MHz with the cores it came from and resolve again only when the spec's cores change). Affects T108b (vmop-4481)
- [x] T023 CPU and memory allocation keep v1alpha5 behavior. `policies.resources` is the class's allocation (CPU in cores) and converts to `resources.requests/limits` as is. Today `configSpec.cpuAllocation` and `memoryAllocation` apply only when `policies.resources` doesn't set that resource; when it does, the provider replaces the whole allocation (`pkg/providers/vsphere/virtualmachine/configspec.go`). Memory from `configSpec` converts (MB to bytes) when `policies.resources` doesn't set memory. CPU from `configSpec` is in MHz, which conversion can't turn into cores without the host frequency, so it stays in `configSpec` as an unchecked preset and reaches vSphere exactly as today; a v1alpha6 write may keep it unchanged but not add or change it, and `resources.requests/limits.cpu` takes precedence when set. Open: whether a later change can elevate the MHz value into `resources.requests/limits.cpu`, like the other elevated `configSpec` paths; blocked by T022, because the conversion needs the frequency. Affects T106
- [x] T024 Per-NIC class settings. Today the class's ethernet cards in `configSpec.deviceChange` are matched to the VM's interfaces by position at create (`vmCreateGenConfigSpecZipNetworkInterfaces`): an interface with no `type` takes the class card as its template, keeping the card's device type and settings; `ethernetN.*` extraConfig keys apply to the Nth NIC. The class `network.interfaces.type` has no `default` (allowed values only); the VM's `type` is not required: it stays unset at create, so a class card still applies, and the mutation webhook sets `VMXNet3` once the schema upgrade has backfilled the live type, as on main. SR-IOV doesn't ship in Milestone 1. Conversion: when every NIC of a type has the same settings, they become that type key's defaults; when they differ by position, they stay in `configSpec` as a preset, exempt from the elevated-path rejection (a v1alpha6 write may keep them unchanged but not add or change them), and the class reports a warning. Affects T106 and T400
- [x] T025 Down-conversion of elevated fields for v1alpha5 and older readers, including the provider's older class API: render the class defaults back into `configSpec` (and `hardware.devices`) so those readers see what they see today, and keep the policy in the conversion annotation; up again, a rendered value an older writer changed replaces the default, otherwise the annotation is restored (the T007 pattern). Risks: every older version with `configSpec` needs the render-back, including the v1alpha1 path the provider writes through; the changed-value path applies to every write through an older CRD version, including classes v1alpha6 has written (only the provider's older class API is blocked, T304); and `configSpec` JSON fidelity, so the moves edit the raw JSON map in place on the moved paths only, never a round trip through typed structs. Tests in T106c. Affects T106
- [x] T026 Superseded by T031: there is no "class-supplied" field and no inline-edit rule; a VM edits any field within its class's bounds
- [x] T028 Superseded by T030: there are no governing classes
- [x] T027 Shares conversion: today the provider writes `Normal` shares whenever the class sets `policies.resources`, ignoring `configSpec` shares. Decided: keep v1alpha5 behavior. A resource whose `policies.resources` is set converts to `Normal` shares; otherwise the `configSpec` shares convert (unset = `Normal`). Affects T120
- [ ] T029 Class status size and write rate. Measure the real environment-browser object sizes on a testbed, then in envtest the class status size and write rate at 1,000 namespaces × 20 classes × 3 zones, including `unlistedFieldPolicy: Allow` classes (whose resolved limits are close to the zone's whole hardware view) and a hardware change that touches every class. Options to bound them: list only fields a VM of the class can set; skip unchanged writes and spread writes; compute status only on classes in `vmware-system-vmop` and copy it; a shared per-zone summary instead of per-class resolved limits. Blocks T202
- [x] T030 VMs must name a class. DevOps users can't create a VM without `spec.className` or remove it, as today; privileged accounts can, as today (import, restore, failover; `VMImportNewNet`), and such a VM is not checked against a class and reports `ClassCompliant=False`. `governs`, governing classes, `existingVMs`, `DenyPowerOn`, `status.governingClassNames`, `GovernanceCompliant`, `WithinGovernance`, `Governing` and `Zone.status.governingClasses` are removed. There is no zone- or namespace-wide policy: an admin restricts a field in every class assigned to the namespace. The capability key keeps its name (research.md, Finding 13)
- [x] T031 A VM inherits its class's defaults at admission (written into the spec) and may set or change any field within its class's bounds. Values stay on the spec with no record of who set them. Supersedes T026 and the inline-edit rule
- [x] T032 A class change (allowed where resize is allowed today) rewrites the resize fields as the 9.1 release does: CPU count, memory, and CPU and memory requests, limits and shares from the new class, a resize field the new class doesn't set cleared (vSphere default: no reservation, unlimited, `Normal`); a resize field the same request changes keeps the user's value; `reservedProfileName` from the new class or cleared; every other field kept, the new class's other defaults not added, and the whole spec checked against the new class, so a kept field it doesn't allow (for example a vGPU from the old class) is rejected, naming it. Written by the mutation webhook; applied as a resize today. Facts in research.md, Finding 14. Supersedes T009a
- [x] T033 A class edit never changes a VM. The `vmoperator.vmware.com/same-vm-class-resize` annotation triggers the T032 rewrite from the class's current values once per add or change, in the mutation webhook; in the 9.1 release it made the VM follow every class edit, which needed a controller to write VM spec
- [ ] T034 `spec.unlistedFieldPolicy: Deny | Allow` (model.md, "Unlisted fields"). Decided: `Deny` is the default and what conversion writes; `Allow` opens unlisted fields within the hardware (and the parent in Milestone 2); a child's `Allow` inherits its parent's policy; a `Deny` class must list `resources.size.{cpu,memory}`. Open: the name. Proposed: `unlistedFieldPolicy`, following the Kubernetes `...Policy` enum convention (`imagePullPolicy`, `fsGroupChangePolicy`, `unhealthyPodEvictionPolicy`) and its advice to use an enum rather than a boolean. Affects T103, T300 and T403
- [x] T035 Conversion lets a VM do exactly what it could do before (model.md, "Conversion"): `unlistedFieldPolicy: Deny`; class values fixed, including device lists and extraConfig keys; every field a v1alpha5 VM could set and the class can bound listed as `{}` (or `{default: v}` when the class sets `v`). The open list is derived by a schema test from the v1alpha5 VM spec and the class policy paths, so a new field can't be missed. Affects T106 and T308
- [ ] T036 Ratcheting for a VM that doesn't fit its class (after a class edit, an import or the upgrade backfill). Proposed, following Kubernetes CRD validation ratcheting: on update, only changed fields, and relation rules that involve them, are checked against the class; unchanged fields are not; a class change checks the whole spec. Affects T403
- [x] T037 Compliance is informational: `ClassCompliant=False` names the fields and nothing is withheld; a deleted class is reported the same way; vm-operator never changes a VM's spec (outside the schema upgrade) or power state because of it
- [x] T038 Existing VMs: the schema upgrade fills the new VM fields from vSphere once per VM (`ReconcileSchemaUpgrade` with `FeatureVersionTelcoVMServiceAPI` already fills `resources.size`, requests and limits through `backfill/compute.go`); shares (T120) and device lists (T108a) are added to it, and a VM with a pending class change gets the resize fields from its class (T110a). A backfilled VM that doesn't fit its class reports `ClassCompliant=False` and keeps running, as an imported VM does
- [ ] T039 The `VirtualMachineClassConfigurationSynced` reason while an edit within bounds is not yet applied; today the reasons are `ClassNameChanged` and `SameClassResize`. Affects T500
- [ ] T042 Milestone 2: a parent narrowed below a child. Proposed: accept the parent edit (it is the provider's, the child may be a tenant's), report it on the child, and check VMs of the child against the child within its parent. Affects T601

---

## Phase 1 — API, upgrade and capability (vmop-4038)

### Story — `supports_vm_service_classless_vm` capability (vmop-4313)

- [ ] T100 [US12] [vmop-4313] Replace the Telco and configuration-policy capability keys with `supports_vm_service_classless_vm` and their internal feature flags with `Features.VMServiceClasslessVM`; update every caller, manifest gate, and E2E capability check to use the shared gate; v1alpha6 `VirtualMachineClass` objects always use the new shape, whether or not the capability is enabled. The capability gates webhook behavior (defaults from ranged classes, edits within bounds, the class-change rewrite, the new VM and class rules) and controller reconciliation (compatibility, compliance, placement); toggling it on or off takes effect without a restart (test both directions) — `pkg/config/capabilities`, `pkg/config`

### Story — Quarantined capability pipeline (vmop-4314)

- [ ] T101 [P] [US12] [vmop-4314] A CI pipeline that runs the Flex VM Classes E2E suites with the capability enabled, quarantined until vmop-4337 — `test/e2e`, CI config

### Story — VM Class v1alpha6 schema (vmop-4315)

- [ ] T102 [US1/US9] [vmop-4315] Wire the round-trip annotation into the `VirtualMachineClass` conversion functions before the rest of the schema lands — `api/utilconversion`, `api/v1alpha*/virtualmachineclass_conversion.go`
- [ ] T103 [US1/US2/US10] [vmop-4315] Reshape `VirtualMachineClass` v1alpha6 in place: `resources` (size, requests, limits), `cpuAdvanced`, `memoryAdvanced`, `bootOptions`, `advanced` (incl. `extraConfig`, with a `value` `StringPolicy` on its `allowed` entries), `hardware.{vGPUs,dynamicDirectPathIODevices}`, `network.interfaces`, `resources.shares` (with T120), `hardware.instanceStorage`, `hardware.version`, leftover-only `configSpec`, `zones`, `unlistedFieldPolicy` (T034), `externalID`, `reservedProfileName`, `parentClassRefs` (reserved, rejected in Milestone 1), `status.zones[]`, the schema-version label — `api/v1alpha6/virtualmachineclass_types.go`
- [ ] T103a [US10] [vmop-4315] Remove `VirtualMachineReservedProfile` from the vm-operator API and its CRD, and add read-only RBAC for it — `api/v1alpha6/`, `config/crd/`, `config/rbac`
- [ ] T103b [US10] [vmop-4315] Remove `ReservedProfileID` from `VirtualMachineClassSpec` in v1alpha6 (replaced by `reservedProfileName`); keep it in v1alpha5 and older for conversion; regenerate deepcopy and CRDs — `api/v1alpha6/virtualmachineclass_types.go`, `config/crd/`
- [ ] T104 [P] [US1/US2/US14] [vmop-4315] The value types (number range, `CountRange`, `EnumPolicy` with `required`, `StringPolicy`, `BoolPolicy`, `ListPolicy` with entry `count` and field policies, `SetPolicy`), the NIC typed list, and their CEL rules: `size(default)` within the list `count`, unique `default` keys, shares `value` only with `Custom`; envtest coverage of every rule, including `{}` for every shape — `api/v1alpha6/`
- [ ] T105 [vmop-4315] `make generate-go generate-manifests`; commit generated files

### Story — VM Class upgrade and conversion (vmop-4316)

- [ ] T106 [US1/US9/US10/US15/US17] [vmop-4316] Conversion v1alpha5 and older ↔ v1alpha6 per T035: `unlistedFieldPolicy: Deny`; scalars to complete ranges, `configSpec.version: vmx-N` to `hardware.version: {min: N, default: N}` (no version: `{}`), `hardware.{cpus,memory}` to `resources.size`, `policies.resources` to `resources.requests/limits`, elevated `configSpec` paths moved to typed fields and fixed to their values, `{}` (or `{default: v}`) for every field a v1alpha5 VM could set, `externalID` = name, reserved profiles per T007 (`reservedProfileID` in v1alpha5 and older ↔ `reservedProfileName` in v1alpha6, which has no `reservedProfileID`; including the v1alpha1 path the provider writes through), `reservedSlots` preserved including zero. `hardware.devices.{vgpuDevices,dynamicDirectPathIODevices}` and `configSpec.deviceChange` vGPU and DDPIO entries to the device lists' `default` with generated names, with `allowed` entries and a `count` that fix them; `configSpec.extraConfig` keys decoded with the backfill's decoder (`auto`, `default` and `dontcare` = unset; a value that doesn't decode stays in `configSpec`) into `advanced.extraConfig.default` with `allowed` entries that fix each value; CPU allocation per T023, NICs per T024, down-conversion per T025, shares per T027. The `api` module has no govmomi dependency, so conversion reads `configSpec` with small local JSON types. Conversion can't fail and can't call vSphere. Round-trip test including a multi-entry `allowed` and a changed `reservedProfileID` — `api/v1alpha5/virtualmachineclass_conversion.go`, older versions
- [ ] T106a [US9] [vmop-4316] Three-state zone test: unset, `[]` and a list survive JSON, conversion, `CreateOrUpdate` and back — envtest
- [ ] T106b [US1/US17] [vmop-4316] E2E: an existing class and its VMs behave as before after upgrade; a VKS-style client creates and updates VMs on converted classes, setting network interfaces, `minHardwareVersion` and boot options, as in the 9.1 release; a VM of a converted device class can't change its devices
- [ ] T106c [US9] [vmop-4316] Conversion tests per T025, for every older version (v1alpha1 through v1alpha5, including the provider's v1alpha1 path): older → v1alpha6 → older is lossless for every elevated path; v1alpha6 → older → v1alpha6 restores ranges, `allowed` and `denied` from the annotation; an older writer that changes, removes or adds a rendered value (hardware version, CPU or memory, an extraConfig key, a vGPU or DDPIO device, a NIC setting, `reservedProfileID`) gets that field reset to the written value while the rest is restored; a class with no annotation (never written through v1alpha6) converts plainly; a class v1alpha6 wrote and an older CRD version then edits keeps every policy the edit didn't touch, including `unlistedFieldPolicy`; policies with no default (a bound or `denied` only) aren't rendered and survive through the annotation; T023 and T024 presets stay in `configSpec` unchanged; unknown `configSpec` fields and non-elevated paths survive semantically unchanged (number formats, field order); unset, `[]` and lists stay distinct; a large class (many extraConfig keys and devices) stays within the annotation size limit; fuzz round trips across every pair of versions — `api/v1alpha5/virtualmachineclass_conversion_test.go`, `api/test`
- [ ] T106d [US17] [vmop-4316] Schema test for T035: every VM field in the v1alpha5 VM spec that has a v1alpha6 class policy path is open (`{}` or `{default: v}`) in a converted class, and no other field is — test under `api/`

### Story — VM API fields and backfill (vmop-4317)

- [ ] T108 [P] [US6/US10] [vmop-4317] VM API: `spec.reservedProfileName`, the `ClassCompliant` condition, and the observed fields (firmware, change block tracking, SEV, SEV-SNP, TDX, CPU hot-remove, SMC, DirectPath I/O devices) — `api/v1alpha6/virtualmachine_types.go`
- [ ] T108a [US8/US10/US14/US17] [vmop-4317] VM API: per-type device lists `hardware.vGPUs` and `hardware.dynamicDirectPathIODevices` (device groups deferred, vmop-4485), keyed by `name`, with the key fields named as in the environment browser (T009g), landed through or with vmop-3791; schema-upgrade backfill of existing vGPU and DDPIO devices into named items with stable generated names. Blocked by T020 — `api/v1alpha6/virtualmachine_hardware_types.go`, `pkg/providers/vsphere/upgrade/virtualmachine/backfill/`
- [ ] T108b [US1] [vmop-4481] VM API: `spec.resources.{requests,limits}.cpu` in cores (millicores allowed) instead of MHz, matching the class; remove the CEL rule rejecting the `m` suffix (kept for `size.cpu`); the provider converts with the cluster's minimum host frequency, as for the class, per T022; the schema-upgrade backfill and the resize sync convert MHz to cores — `api/v1alpha6/virtualmachine_compute_types.go`, `pkg/util/vmopv1/compute_overwrite.go`, `pkg/util/vmopv1/resize_class_sync.go`, `pkg/providers/vsphere/upgrade/virtualmachine/backfill/compute.go`
- [ ] T108c [US1] [vmop-4317] VM validating webhook: size, requests and limits relation rules on the effective values (model.md, "Size, requests and limits") — `webhooks/virtualmachine/validation`
- [x] T109 Removed by T030 (`Zone.status.governingClasses`)
- [ ] T110 [US10] [vmop-4317] Schema upgrade fills `spec.reservedProfileName` on existing VMs from their class's `reservedProfileName.default`; this unset-to-set write is exempt from the powered-on lock — `pkg/providers/vsphere/upgrade/virtualmachine/vm_schema_upgrade.go`
- [ ] T110a [US17] [vmop-4317] Schema upgrade, per T038: when a VM's class resize is pending (the last-resized annotations differ from `spec.className`), fill the resize fields from the class instead of vSphere; test that a VM resized in the 9.1 release while powered on still gets the new class's size at its next power-off — `pkg/providers/vsphere/upgrade/virtualmachine/`

### Story — CPU and memory shares (vmop-4479)

- [ ] T120 [US16] [vmop-4479] VM API `spec.resources.shares.{cpu,memory}: {level, value}` with the CEL rule; class `resources.shares` at the same path; conversion of v1alpha5 `configSpec` allocation shares; the provider writes the VM's shares and keeps `Normal` when none is set (replacing the hard-coded level); schema-upgrade backfill from the live shares first, so an existing `High` or `Custom` VM isn't reset — `api/v1alpha6/`, `api/v1alpha5/virtualmachineclass_conversion.go`, `pkg/providers/vsphere/virtualmachine/configspec.go`, `pkg/providers/vsphere/upgrade/virtualmachine/backfill/compute.go`
- [ ] T121 [US16] [vmop-4479] E2E: a VM with custom CPU shares shows the value in vCenter; a VM outside its class's shares policy is rejected

### Story — Remove VirtualMachineConfigPolicy (vmop-4318)

- [ ] T111 [P] [US11] [vmop-4318] Remove the `Zone` controller's policy fan-out and its tests — `controllers/infra/zone/zone_controller.go`, `zone_controller_test.go`
- [ ] T112 [P] [US11] [vmop-4318] Remove the policy types and CRD; retain the shared `Features.VMServiceClasslessVM` gate for flex classes and the environment-browser pipeline — `external/vim/api/v1alpha1/virtualmachine_config_policy_types.go`, `config/crd/external-crds/`, the embedded CRD list
- [ ] T113 [P] [US11] [vmop-4318] Remove the `virtualmachineconfigpolicies` RBAC rules — `config/rbac/role.yaml` (from markers)
- [ ] T114 [US11] [vmop-4318] E2E: remove the policy assertions, keep the environment-browser specs, and assert no policy exists after upgrade — `test/e2e/vmservice/vmservice/configpolicy/`

### Story — Environment browser completion (vmop-4319, includes vmop-3964)

- [ ] T107 [US7] [vmop-4319] Restrict `ConfigTarget`, `VirtualMachineConfigOptions` and `VirtualMachineGuestOptions` to admins and the vm-operator service account — `config/rbac`
- [ ] T115 [US7] [vmop-4319] Maximum hardware version per cluster, and a conservative cluster choice for options co-owned by several `ConfigTarget`s (vmop-3964) — `controllers/vim/*`, `controllers/virtualmachineconfigoptions`
- [ ] T116 [US7] [vmop-4319] No EnvironmentBrowser query on resync when the cluster is unchanged; a cluster change re-queries. Unit test on fake-provider call counts
- [ ] T117 [US7] [vmop-4319] E2E: a DevOps user gets 403 on the three CRDs; admins succeed

### Story — vmx-23 CPU fields (vmop-4428)

Blocked on a vSphere and govmomi release with the vmx-23 fields. Not a gate for the rest of Milestone 1.

- [ ] T118 [US2/US6] [vmop-4428] VM API and class API: CPU `reservationLockedToMax` and `cpuAdvanced.numaFixedAutoAffinityEnabled` as VM spec fields, `BoolPolicy` class fields at the same paths, and observed status fields — `api/v1alpha6/virtualmachine_types.go`, `api/v1alpha6/virtualmachineclass_types.go`
- [ ] T119 [US2/US6] [vmop-4428] Support check: reject both on a hardware version below vmx-23 and when the environment browser reports them unsupported; map them in the provider and report them in status — `pkg/providers/vsphere`, class check package

---

## Phase 2 — Class check and class compatibility (vmop-4039)

### Story — Class check (vmop-4320)

- [ ] T200 [US2/US3/US4/US14/US15] [vmop-4320] Compare a VM's effective configuration, or (in Milestone 2) a child class's ranges, with a class, field by field, through one reflection-based evaluator over the value shapes (list policies with `denied` first and first-match entries, NIC type keys, `hardware.version` against the actual version), applying `unlistedFieldPolicy` (an `Allow` class's unlisted fields against the zone's environment-browser objects) and the ratcheting rule (T036) — new package under `pkg/`, exported for the webhooks, the reconciler, the class controller and placement

### Story — Class hardware compatibility (vmop-4321)

- [ ] T201 [US7/US14/US15] [vmop-4321] Class controller: per-zone `HardwareCompatible` from `ConfigTarget` and the options objects (including `hardware.version` against the maximum hardware version, and `default` devices the zone lacks), the informational `DevicePolicyResolved` for device `allowed` entries that match nothing in the zone, asynchronous, re-checked as hardware and zones change; never rejects (zones per T010); watch `ConfigTarget` and `Zone`. Depends on T115 — `controllers/virtualmachineclass`
- [ ] T201a [US7] [vmop-4321] E2E: a class with a zone-specific vGPU profile is accepted and reports per-zone `HardwareCompatible`

### Story — Class resolved limits (vmop-4322)

- [ ] T202 [US7] [vmop-4322] Class status: resolved per-zone limits and options for every field a VM of the class can set (ranges as `{min, max}`; enums, sets and devices as the available values with display fields), from the environment-browser objects within the class; per-VM only, only what the class allows; `Allow` classes' unlisted fields from the environment browser alone; updated when the environment-browser objects change; the unreserved-DirectPath reason per T012. Shaped by T029
- [ ] T202a [US7] [vmop-4322] E2E: resolved limits never exceed the class, and a device list shows only the devices the zone has

### Story — Class instance hash (vmop-4323)

- [ ] T205 [US3] [vmop-4323] Re-base the `ClassInstance` hash onto typed fields, including `unlistedFieldPolicy` — `controllers/virtualmachineclass`
- [x] T203, T204, T205a Removed by T030 (`WithinGovernance`, `Governing`, the Zone mirror and its E2E)

---

## Phase 3 — Class webhooks and provider API markers (vmop-4201)

The class APIs, their generation and the namespace-copy integration are provider work (T912–T916).

### Story — Class mutating webhook (vmop-4324)

- [ ] T300 [US2/US8] [vmop-4324] Set `unlistedFieldPolicy: Deny` when unset and the memory reservation lock on a class whose defaults attach devices; number ranges are not completed — `webhooks/virtualmachineclass/mutation`
- [ ] T300a [US2/US8] [vmop-4324] E2E: a class written without `unlistedFieldPolicy` reads back with `Deny`, and a device class with the memory lock set

### Story — Class validating webhook (vmop-4325)

- [ ] T301 [US1/US2/US9/US12/US14/US15] [vmop-4325] Value-shape rules (T002) not covered by CEL: glob syntax, `default` items not denied, matched by an `allowed` entry and within that entry's `count` and field policies, a warning for an entry an earlier one shadows, a warning when `hardware.version.max` is lowered; `resources.size.{cpu,memory}` listed in a `Deny` class (T034); the size, requests and limits relation rules (model.md), elevated `configSpec` paths, `externalID` immutable, `parentClassRefs` rejected in Milestone 1; the new class rules are gated on the capability — `webhooks/virtualmachineclass/validation`
- [x] T302 Removed by T030 (one governing class per zone, class-vs-class bounds)
- [ ] T303 [US10] [vmop-4325] A class's default configuration must match its default profile, and every allowed profile must fit within the class's resource and device policy (bounds are not fixed by listing a profile); every name in `reservedProfileName` resolves to a `VirtualMachineReservedProfile` in the namespace; `allowed` is required when `default` is set — `webhooks/virtualmachineclass/validation`
- [ ] T303a [US10] [vmop-4325] On class update through the v2 path, reject only changes that alter an allocated profile's shape or make the class's default VM invalid for it; tests pin the exact field boundary (T009b) — `webhooks/virtualmachineclass/validation`
- [ ] T304 [US9] [vmop-4325] Schema-version label: record the last CRD version that wrote the class; writes through older CRD versions are accepted and converted (T025); the provider's older class API rejects edits of a class last written through v1alpha6 (provider task)
- [ ] T305 [US9] [vmop-4325] Namespace-copy zone checks: the zones belong to the namespace and to the class; zones with reserved capacity are included; reported per zone in `status.zones[]` with `ZoneNotInNamespace` or `ZoneNotInClass`, never rejected (T010)
- [ ] T306 [US2/US9] [vmop-4325] E2E: a `Deny` class without CPU or memory is rejected; an edit of a v2 class through the provider's older class API is rejected, and a write through an older CRD version is converted

### Story — Provider API mapping markers (vmop-4326)

- [ ] T307 [US9] [vmop-4326] The mapping markers the provider's API generation reads on the v1alpha6 class types, and a CI check that every field has a mapping or an explicit exclusion — `api/v1alpha6/virtualmachineclass_types.go`, value types in `api/v1alpha6/`
- [ ] T308 [US9] [vmop-4326] Schema test on the generated CRD: every field a class can bound matches exactly one value shape by its field set; every class policy pairs with exactly one VM field (with the NIC type-key and `hardware.version` mappings), and `count`, `default`, `allowed`, `denied` and `type` are reserved; names under a NIC type key don't collide; device lists are map lists keyed by `name` whose class entry keys match their environment-browser entries; entry lists are atomic — test under `api/`

---

## Phase 4 — VM create and edit (vmop-4040)

### Story — VM defaults and class changes (vmop-4330)

- [ ] T400 [US3/US4/US10/US14/US15] [vmop-4330] VM mutating webhook: on create, write the class's defaults into every field the VM leaves unset; fill `reservedProfileName` from the class default (omission inherits, `""` = none, per T019 for other enums); fill an omitted list (devices, NICs in order, extraConfig) from its `default` and never merge into an explicit one, applying the class's NIC default before the existing default-NIC mutation; fill field policies' defaults on each item from the entry or type key that applies to it; set the hardware version per model.md. On a `className` change (where resize is allowed today), the T032 rewrite; on an add or change of the same-class resize annotation, the T033 rewrite — `webhooks/virtualmachine/mutation`
- [ ] T401 [US3/US4] [vmop-4330] VM reconciler: apply the spec's resize fields as a resize is applied today (at power-off, before a requested power-on) and report `VirtualMachineClassConfigurationSynced` until applied, with the T039 reason for edits within bounds; the reconciler no longer reads the class to decide what to resize (`pkg/util/vmopv1/resize.go`) and never writes VM spec — `pkg/providers/vsphere`, `pkg/providers/vsphere/session`
- [ ] T402 [US1/US3/US4] [vmop-4330] E2E: a VM that names a ranged class gets its defaults; a class change rewrites CPU, memory, reservations, limits and shares and clears the ones the new class doesn't set, keeping the rest; a class change that keeps a device the new class doesn't allow is rejected; the same-class resize annotation applies the class's current values once; a class edit changes no VM

### Story — VM validation against its class (vmop-4331)

- [ ] T403 [US2/US3/US4/US6/US14/US15/US16] [vmop-4331] VM validating webhook: the VM against its class through T200, including `unlistedFieldPolicy`, list counts and entries, extraConfig values against an entry's `value` policy, NIC type keys, `minHardwareVersion` above `max`, and shares; the whole spec on a class change; ratcheting per T036; `className` required except for privileged accounts, as today; messages follow the visibility rule — `webhooks/virtualmachine/validation`
- [ ] T404 [US2/US3/US6/US14/US15] [vmop-4331] E2E: a VM beyond its class's range, an extraConfig key the class denies, a field a `Deny` class doesn't list, a vGPU beyond its entry's count, a NIC type the class forbids and a minimum hardware version above `max` are rejected; an edit within bounds is admitted; an unlisted field on an `Allow` class is admitted; a VM that no longer fits after a class edit can still change an unrelated field

### Story — Device memory reservation (vmop-4332)

- [ ] T405 [US8] [vmop-4332] VM validating webhook: memory fully reserved when the effective configuration has a vGPU or DirectPath I/O device (create, and spec changes while attached; not metadata, status or vm-operator writes) — `webhooks/virtualmachine/validation`
- [ ] T406 [US8] [vmop-4332] E2E: a device VM without full reservation is rejected; a VM of a fixed v1 class is unaffected

### Story — Zone selection rule (vmop-4333)

- [ ] T407 [US5/US7] [vmop-4333] VM validating webhook: reject a VM in a zone where its class is not available or `HardwareCompatible=False`, with that zone's reason; a VM without a zone is admitted with its class's defaults. Depends on T201 — `webhooks/virtualmachine/validation`
- [ ] T407a [US5/US7] [vmop-4333] E2E: a VM in an incompatible zone is rejected; a VM without a zone is admitted

### Story — Reserved capacity (vmop-4334)

Depends on the provider's reserved profile publication (T910).

- [ ] T408 [US10] [vmop-4334] VM validating webhook: `reservedProfileName` must be in the class's `allowed`; after defaulting, every VM with a selected profile must match the `VirtualMachineReservedProfile` shape (T008), naming the field and the profile; a resize past the profile is rejected; a set value can't change to another while powered on, unset-to-set by vm-operator is allowed; a class change of a reserved VM only while powered off (per T018) — `webhooks/virtualmachine/validation`
- [ ] T409 [US10] [vmop-4334] Write the extraConfig profile ID from `spec.reservedProfileName`, falling back to the class's `reservedProfileName` (not the removed `reservedProfileID`) while the VM field is unset, and remove the key when the field is empty (fixes the stale `resourcepool.vmResourceProfileId` after moving to an unreserved class). Runs after T110 — `pkg/providers/vsphere/virtualmachine/configspec.go`
- [ ] T411 [US10] [vmop-4334] Move the class controller's `ReservedProfileID` reader (including the `ClassInstance` hash `Reserved` bool) to `reservedProfileName`, since v1alpha6 has no `reservedProfileID` — `controllers/virtualmachineclass/virtualmachineclass_controller.go`
- [ ] T410 [US10] [vmop-4334] E2E: a VM of a reserved class keeps its reservation after upgrade; a VM whose shape differs from its profile is rejected; a VM moved to an unreserved class loses the profile ID

---

## Phase 5 — VM compliance, placement and enablement (vmop-4041)

### Story — VM compliance (vmop-4335)

- [ ] T500 [US6/US15] [vmop-4335] VM reconciler: re-run the class check once the zone is known (including the actual hardware version from `status.hardwareVersion`; never upgrade it) and set `ClassCompliant`, naming the fields; `False` for a deleted class and for a privileged VM without a class; never change spec or power state because of it; populate the observed status fields — `pkg/providers/vsphere`, `controllers/virtualmachine`
- [x] T501 Removed by T030 (`DenyPowerOn`)
- [ ] T502 [US6] [vmop-4335] E2E: narrowing a class marks its VMs `ClassCompliant=False` and they keep running through a power cycle; deleting the class does the same; an upgraded VM that doesn't fit its class reports it

### Story — Class-aware placement (vmop-4336)

- [ ] T503 [US5] [vmop-4336] Placement: send only the zones where the VM's class is available and compatible and the VM fits the zone's resolved limits; a group placed into one zone uses the zones every member admits — `pkg/providers/vsphere/placement`
- [ ] T504 [US5] [vmop-4336] E2E: a VM without a zone is placed only in a zone where its class fits

### Story — Flex VM Classes enablement (vmop-4337)

- [ ] T505 [US12] [vmop-4337] Take the capability pipeline out of quarantine, run the full E2E sweep, enable the capability. Depends on T910 and T911

### Story — Flex VM Classes docs (vmop-4338, absorbs vmop-3739)

- [ ] T506 [P] [vmop-4338] User and developer docs for v1alpha6 classes, `unlistedFieldPolicy`, edits within bounds, class changes, reserved profiles and the environment-browser pipeline, linked from the docs index — `docs/`

---

## Phase 6 — Tenant VM Classes, Milestone 2 (vmop-4312)

### Story — Tenant VM Classes capability (vmop-4339)

- [ ] T600 [US13] [vmop-4339] A second capability that requires the `supports_vm_service_classless_vm` capability

### Story — Parent class references (vmop-4340)

- [ ] T601 [US13] [vmop-4340] `parentClassRefs`: one parent, no depth limit, cycles rejected, ranges within the parent's, live per-field resolution in `vmware-system-vmop` (also for a namespace copy of the child), a child's `unlistedFieldPolicy: Allow` limited to its parent's policy; only the top class carries `configSpec`; a parent narrowed below a child per T042
- [ ] T602 [US13] [vmop-4340] Reject deleting a referenced parent; fail closed if one disappears; report a rejected platform-driven delete
- [ ] T603 [US13] [vmop-4340] E2E: a child outside its parent is rejected; a referenced parent can't be deleted; a namespace with only the child copied checks VMs against the child within its parent

### Story — Class families per zone (vmop-4341)

- [x] T604, T605 Removed by T030: one family per zone existed only for governing classes. vmop-4341 is closed or re-scoped

### Story — Tenant classes through v2 vAPI (vmop-4342)

- [ ] T606 [US13] [vmop-4342] `parentClassRefs` round-trips through the v2 vAPI; VCFA can build its tenant portal on it

### Story — Tenant VM Classes enablement (vmop-4343)

- [ ] T607 [US13] [vmop-4343] E2E sweep, docs, enable the capability

---

## Provider work (tracked in the provider's spec)

Dependency tasks; vm-operator does not implement them. What vm-operator assumes from them is in spec.md, "Provider contract".

- [ ] T910 [US10] [vmop-4483] Publish a namespaced `VirtualMachineReservedProfile` per profile with its shape and per-zone counts, backfilled for existing allocations before the capability is enabled. Blocks vmop-4334 and T505
- [ ] T911 [US12] [vmop-4484] Advertise the `supports_vm_service_classless_vm` capability once T910 and T914 hold. Blocks T505
- [ ] T912 [US9] [vmop-4327] The per-Supervisor v2 class API, including `unlistedFieldPolicy`. Depends on T103 and T307 (through T915)
- [ ] T913 [US9/US10] [vmop-4328] The older class API (rejects writes to v1alpha6 classes per T304, keeps today's block on classes with capacity, reports the schema version) and the namespace API with per-class zones; reserved profiles built from a flex class's defaults
- [ ] T914 [US9/US13] [vmop-4329] Supervisor and namespace-copy integration: copy classes through v1alpha6 with unset and `[]` zones kept apart, report rejected copies; which classes are copied into which namespaces (in Milestone 2, whether a parent is copied with its child); end-to-end E2E of a class written through the v2 API
- [ ] T915 [US9] [vmop-4392] Generate the class API definition from the v1alpha6 CRD and its mapping markers, with a drift check. Depends on T103 and T307
- [ ] T916 [vmop-4393] A drift check for new vpxd VM config fields the class neither covers nor excludes

## Dependencies on other teams (not vm-operator code)

- [ ] T900 Supervisor team: the 1.5 capability, the class migrator, backup and restore of classes, one list of default classes and whether they can be edited
- [ ] T901 vAPI team: generator support for the class API definition (needed by T915)
- [ ] T902 VCFA: move to the v2 API; whether region quota reads class sizes; which classes it copies into namespaces
- [ ] T903 vCenter UI: hide edit actions for classes last written through v1alpha6
- [ ] T904 VKS: a class with no CPU or memory default (`{}`, or an `Allow` class without them) gives node VMs no size; VKS requires class CPU and memory defaults, or reads the VM's size instead of the class's
