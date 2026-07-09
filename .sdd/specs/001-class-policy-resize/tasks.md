# Tasks: VM Service Class Policy and Resize

- **Input**: `specs/001-class-policy-resize/spec.md` + `specs/001-class-policy-resize/plan.md`
- **Epic**: vmop-3331
- **Format**: `[ID] [P?] [Story] Description — file paths`

Tasks within a Phase that are marked `[P]` may run in parallel. Each task corresponds to a Story or Sub-task; ticket keys are noted where created.

---

## Phase 0 — Architecture (wiki; no code changes)

Dependencies: none. All A-tasks may run in parallel `[P]`.

- [ ] T001 [P] [A1/vmop-3733] Apply draft One Pager content to wiki page ID: 2453059721 — fill Business Problem, Goals, Non-Goals, Big Picture, `ConfigTarget` per-host iteration section (max HW version + per-host SR-IOV with DVX), `VirtualMachineConfigOptions` GC step, capability gate subsection, Test Plan table
- [ ] T002 [P] [A2/vmop-3734] Update wiki pages ID: 2453059723 and 2453060275 — add SR-IOV EB limitation finding; add the `ConfigTarget.status` aggregation pattern (`maxHardwareVersion`, enriched `sriov` list with `hostMoID`); cross-link vmop-3470, vmop-3794, vmop-3797, and the updated Design/TDS
- [ ] T003 [P] [A3/vmop-3735] Author new wiki page "Design: VirtualMachineConfigPolicy" as child of 2453059710 — API schemas, reconcile diagram, `ConfigTarget.status` capability surface, webhook decision flow (using `ConfigTarget.status.maxHardwareVersion`), error taxonomy, open questions
- [ ] T004 [P] [A4/vmop-3736] Author new wiki page "TDS: VirtualMachineConfigPolicy" as child of 2453059710 — package layout, controller+webhook designs, RBAC+capability gating, vSphere API mapping per Query* call, `ConfigTarget` per-host iteration design, integration plan, E2E plan, rollback/disable plan, observability plan; each S1..S9 story referenced from the relevant TDS section
- [ ] T005 [A5/vmop-3737] Run stakeholder review for A1+A3+A4; populate Approver+Date columns on each page; move pages under 2453060106 (Design Docs: Approved); update vmop-3331 epic description with approved page URLs

*Gate: T005 must be complete before Phase 1 code work begins.*

---

## Phase 1 — Foundations (code; sequential)

### Story S1 — Capability + feature gate (vmop-3738)

- [x] T010 [S1.a] [PR #1649 / vmop-3747] Add `CapabilityVirtualMachineConfigPolicy = "supports_vm_service_vm_config_policy"` constant — `pkg/config/capabilities/capabilities.go`
- [x] T011 [S1.a] [PR #1649 / vmop-3747 + PR #1667 / vmop-3748] Wire feature gate check — vim.vmware.com scheme registered in `pkg/manager/manager.go` only when capability enabled; `controllers/vim/controllers.go` stub added with kubebuilder RBAC markers
- [ ] T012 [S1.a] Wire webhook short-circuit in `webhooks/virtualmachine/validation_webhook.go` — skip policy + `ConfigTarget`-based capability checks when capability disabled
- [x] T013 [S1.a] [PR #1649 / vmop-3747] Unit tests for capability gate logic — `pkg/config/capabilities/capabilities_test.go`
- [x] T014 [S1.b] [PR #1667 / vmop-3748] Install the four `vim.vmware.com` CRDs (`ConfigTarget`, `VirtualMachineConfigOptions`, `VirtualMachineGuestOptions`, `VirtualMachineConfigPolicy`) via `config/crd/external-crds/` in the Supervisor chart
- [x] T015a [S1.b] [PR #1667 / vmop-3748] Extend `config/rbac/role.yaml` with get/list/watch on `zones`
- [x] T015b [S1.b] [PR #1695 / vmop-3740] Extend `config/rbac/role.yaml` with create/get/list/patch/update/watch on `configtargets` and `virtualmachineconfigpolicies`
- [ ] T015c [S1.b] Extend `config/rbac/role.yaml` with create/get/list/patch/update/watch on `virtualmachineconfigoptions` and `virtualmachineguestoptions` — deferred to S6/S7 controller stories per commit `1698824b`

### Story S2 — Partner-facing integration doc (vmop-3739)

- [ ] T020 [S2] Author `external/vim/doc/integration-guide.md` — pipeline diagram, role of each CRD, `ConfigTarget.status` capability surface (`maxHardwareVersion`, enriched `sriov`), per-host iteration inside the `ConfigTarget` controller, syncMode semantics, worked example of policy denial
- [ ] T021 [S2] Link doc from public RTD site; review by PM and at least one downstream consumer team

*Gate: T021 must be complete before Phase 2 code work begins.*

---

## Phase 2 — Implementation (code; S5 depends on S1+S2; S3/S6/S7/S8/S9 may begin after S1 is merged)

> *Story S4 was retired.* The previously-planned `HostSystem` CRD work (Story vmop-3741 and its sub-tasks vmop-3752, vmop-3753, vmop-3754, vmop-3755, vmop-3756) is closed as *Won't Implement*. The per-host vSphere queries now happen inside the `ConfigTarget` controller and write directly to `ConfigTarget.status` (see Story S5 and the new sub-task vmop-3797). See `research.md` Finding 7 for the rationale.

### Story S3 — Zone controller fan-out (vmop-3740)

- [x] T050 [S3.a] [PR #1695 / vmop-3740] Modify `controllers/infra/zone/zone_controller.go` — derive cluster MoIDs from the Zone's AvailabilityZone (`ClusterComputeResourceMoIDs`); CreateOrPatch ConfigTarget per MoID; CreateOrPatch VirtualMachineConfigPolicy per zone (default syncMode=ConfigTarget on create only)
- [x] T051 [S3.a] [PR #1695 / vmop-3740] Unit tests — `controllers/infra/zone/zone_controller_test.go`
- [x] T052 [S3.b] [PR #1695 / vmop-3740] Integration tests with vcsim — `controllers/infra/zone/zone_controller_test.go`: zone→AvailabilityZone cluster MoID derivation; idempotent create/patch (UID stable); ConfigTarget not deleted when pool MoIDs removed from zone
- [x] T053 [S3.c] [PR #1695 / vmop-3740] E2E test — `test/e2e/vmservice/vmservice/configpolicy/configpolicy.go`: Zone creation materialises ConfigTarget and VirtualMachineConfigPolicy; idempotency verified with Consistently over 30 s

### Story S5 — ConfigTarget controller (vmop-3742)

- [ ] T060 [S5.a] Author `webhooks/configtarget/validation_webhook.go` — immutable spec.id; valid cluster MoID format for metadata.name
- [ ] T061 [S5.a] Register ConfigTarget validation webhook in `webhooks/` suite files
- [x] T061a [S5.a] Register ConfigTarget controller in `controllers/controllers.go`, gated on `Features.VirtualMachineConfigPolicy`
- [ ] T062 [S5.a] Unit tests — `webhooks/configtarget/validation_webhook_test.go`
- [x] T063 [S5.b] Author `controllers/configtarget/configtarget_controller.go` — cluster-scope path: QueryConfigTarget + QueryConfigOptionDescriptor; populate status; fan out VirtualMachineConfigOptions
- [x] T064 [S5.b] Add `GetVirtualMachineConfigTarget` (QueryConfigTarget + QueryConfigOptionDescriptor) — implemented directly on `pkg/providers/vsphere/vmprovider.go` (see note below) instead of a new `environment_browser.go` file
- [x] T065 [S5.b] Unit tests — `controllers/configtarget/configtarget_controller_test.go` (includes vcsim-backed integration coverage in the same file)

> **Implementation note (T063–T065):** `metadata.name` (not `spec.id`) is the cluster MoID key. This is confirmed correct, not just a convention choice: `spec.md`'s acceptance criteria key every `ConfigTarget` lookup off `metadata.name` (`spec.id` is referenced only as an immutability constraint), and the merged Zone controller (vmop-3740, PR #1695) sets `spec.id.ID` to the same value as `metadata.name` on every `ConfigTarget` it creates — so the two fields are always identical in practice. `GetVirtualMachineConfigTarget` was placed directly on `vSphereVMProvider` in `vmprovider.go` — consistent with other single-purpose vSphere queries in that file (`DoesProfileSupportEncryption`, `GetStoragePolicyStatus`) — rather than the `pkg/providers/vsphere/environment_browser.go` sketched in the plan; that file can still be introduced later once `QueryConfigOptionEx` (S6) and the per-host iteration (S5.c) grow the surface area.
>
- [ ] T066 [S5.c / vmop-3759] Extend controller with the per-host iteration — enumerate cluster hosts via `ClusterComputeResource.host`; for each host, run one `PropertyCollector` RPC for `config.defaultHardwareVersion`, `capability.supportedHardwareVersions`, `config.pciPassthruInfo` (filtered to `sriovCapable=true`), and `hardware.dvxClasses` (where `sriovNic=true`); aggregate `max(defaultHardwareVersion)` into `ConfigTarget.status.maxHardwareVersion`; build per-(host, NIC) `VirtualMachineSriovInfo` entries and write them to `ConfigTarget.status.sriov`. Treat per-host RPC failure as a warning event; do not block the rest of the iteration.
- [ ] T066a [S5.c / vmop-3797] Add `ConfigTargetStatus.MaxHardwareVersion` to `external/vim/api/v1alpha1/config_target_types.go`; extend `VirtualMachineSriovInfo` in `external/vim/api/v1alpha1/config_target_devices_types.go` with `HostMoID` (required), `Active`, `MaxVFs`, `NumVFs`, `DVXClass`, `DVXCheckpointSupported`, `DVXSWDMATracingSupported`; add `+listType=map` with `+listMapKey=hostMoID` and `+listMapKey=pciDevice.id` to `status.sriov`; regenerate `zz_generated.deepcopy.go` and `vim.vmware.com_configtargets.yaml`. T066 cannot be merged before T066a.
- [ ] T067 [S5.c] Unit tests for the per-host iteration — `max(defaultHardwareVersion)` aggregation across mixed hosts; SR-IOV entries carry `hostMoID`; DVX class enrichment correlates correctly; partial-failure (one host RPC fails) leaves the other hosts' data intact and triggers a re-queue.
- [ ] T068 [S5.b] Author `controllers/configtarget/gc.go` — `GCVirtualMachineConfigOptions` helper; runs only after both `QueryConfigTarget` and `QueryConfigOptionDescriptor` succeed. There is no `GCHostSystems` — there are no per-host objects to GC.
- [ ] T069 [S5.b] Unit tests for `GCVirtualMachineConfigOptions` — correct candidate selection from (existing CRs, latest enumeration).
- [ ] T070 [S5.d] [P] Integration tests with vcsim — `test/intg/configtarget/configtarget_intg_test.go`: happy path; missing cluster; transient cluster-scope error → retry (no GC); `VirtualMachineConfigOptions` fan-out; per-host iteration writes correct `maxHardwareVersion` from a mixed-host cluster; `status.sriov` entries carry `hostMoID` attribution; one host's RPC failure does not block other hosts' data; drop HW version → GC deletes the orphan `VirtualMachineConfigOptions`.
- [x] T071a [S5.e] E2E test (cluster-scope subset) — `test/e2e/vmservice/vmservice/configpolicy/configpolicy.go`: `ConfigTarget.status` populated from real EB and `Ready=True`; `VirtualMachineConfigOptions` fanned out per hardware version; a synthetic-hardware-version `VirtualMachineConfigOptions` owned by a real `ConfigTarget` is garbage-collected on the next reconcile (vcsim's EnvironmentBrowser reports a fixed descriptor set per model, so the drop-a-version GC path can't be exercised by shrinking a real cluster's reported versions — this test injects the staleness directly instead).
- [ ] T071 [S5.e] E2E test (remaining scope) — `test/e2e/vmservice/vmservice/configpolicy/configpolicy.go`: per-host SR-IOV entries with `hostMoID` on SR-IOV-capable clusters; `ConfigTarget.status.maxHardwareVersion` equals the max of `config.defaultHardwareVersion` across the cluster's hosts. Depends on T066/T066a (vmop-3759/vmop-3797).

### Story S6 — VirtualMachineConfigOptions controller (vmop-3743)

- [x] T080 [S6.a] [PR #1671 / vmop-3762] Author `webhooks/virtualmachineconfigoptions/validation/virtualmachineconfigoptions_validator.go` — immutable spec.hardwareVersion; format ^vmx-\d+$; metadata.name must equal spec.hardwareVersion
- [x] T081 [S6.a] [PR #1671 / vmop-3762] Unit tests (13 specs) + integration test stubs — `webhooks/virtualmachineconfigoptions/validation/`
- [ ] T082 [S6.b] Author `controllers/virtualmachineconfigoptions/vmconfigoptions_controller.go` — QueryConfigOptionEx; map vim.vm.ConfigOption → status; fan out VirtualMachineGuestOptions per guest OS; update listMap entry keyed by hardwareVersion
- [ ] T083 [S6.b] Add `pkg/providers/vsphere/environment_browser.go` QueryConfigOptionEx wrapper (extend T064)
- [ ] T084 [S6.b] Unit tests — `controllers/virtualmachineconfigoptions/vmconfigoptions_controller_test.go`
- [ ] T085 [S6.c] [P] Integration tests with vcsim — `test/intg/virtualmachineconfigoptions/vmconfigoptions_intg_test.go`: happy path on multiple HW versions; idempotent re-reconcile updates one listMap entry; golden-file assertion against testdata/configoption-vmx-22.yaml
- [ ] T086 [S6.d] E2E test — `test/e2e/vmservice/vmservice/configpolicy/vmconfigoptions.go`: at least one HW version reconciled on test cluster. Deferred until the VirtualMachineConfigOptions controller (T082-T084) is implemented; webhook validation for hardwareVersion is already covered by the envtest integration suite in `webhooks/virtualmachineconfigoptions/validation/`

### Story S7 — VirtualMachineGuestOptions plumbing (vmop-3744)

- [ ] T090 [S7.a] Author `webhooks/virtualmachineguestoptions/validation_webhook.go` — immutable spec.id; DNS-safe name
- [ ] T091 [S7.a] Unit tests + RBAC + manifest registration
- [ ] T092 [S7.b] [P] Integration tests with vcsim — `test/intg/virtualmachineguestoptions/`: two VirtualMachineConfigOptions (vmx-21, vmx-22) yield single VirtualMachineGuestOptions with two hardwareVersions listMap entries
- [ ] T093 [S7.c] E2E test — at least one VirtualMachineGuestOptions materialised after install + zone creation

### Story S8 — VirtualMachineConfigPolicy controller (vmop-3745)

- [ ] T100 [S8.a] Author `webhooks/virtualmachineconfigpolicy/defaulting_webhook.go` — default syncMode=ConfigTarget, createMode/updateMode/powerOnMode=Allow, vmClassMode=AsPolicy
- [ ] T101 [S8.a] Author `webhooks/virtualmachineconfigpolicy/validation_webhook.go` — spec.zone references existing Zone; extraConfig allowed/denied entries have non-empty key and valid type enum
- [ ] T102 [S8.a] Unit tests — `webhooks/virtualmachineconfigpolicy/`
- [ ] T103 [S8.b] Author `controllers/virtualmachineconfigpolicy/vmconfigpolicy_controller.go` — syncMode=ConfigTarget: copy ConfigTarget.status → policy spec; syncMode=Disabled: set Ready=True, skip sync; never overwrite extraConfig/latencySensitivityLevels/txRxThreadModels
- [ ] T104 [S8.b] Author `pkg/vmconfig/policy/policy_reconciler.go` — ConfigTarget→policy sync field mapping logic (separate from controller for unit testability)
- [ ] T105 [S8.b] Unit tests — `pkg/vmconfig/policy/policy_reconciler_test.go`
- [ ] T106 [S8.c] [P] Integration tests with vcsim — `test/intg/virtualmachineconfigpolicy/`: syncMode=ConfigTarget happy path; syncMode=Disabled skip; missing ConfigTarget → condition; multi-cluster zone selects correct ConfigTarget
- [ ] T107 [S8.d] E2E test — policy spec filled from real cluster capabilities after Zone creation

### Story S9 — VM admission webhook enforcement (vmop-3746)

- [ ] T110 [S9.a] Modify `webhooks/virtualmachine/validation_webhook.go` — on create/update/powerOn, load namespace VirtualMachineConfigPolicy; apply createMode/updateMode/powerOnMode; respect vmClassMode=AsPolicy vs AsConfig
- [ ] T111 [S9.a] Unit tests for mode enforcement
- [ ] T112 [S9.b] Implement ExtraConfig Allow/Deny enforcement — Fixed (exact), Regex (regexp), Glob (filepath.Match); Denied takes precedence over Allowed
- [ ] T113 [S9.b] Unit tests for all three match types and precedence rules
- [ ] T114 [S9.c] Implement `ConfigTarget`-based hardware-version check — resolve the VM's zone to a cluster MoID (at the moment, there is a single vSphere cluster per zone, though that could change in the future), `Get` the cluster's `ConfigTarget`, reject if the VM's effective hardware version exceeds `ConfigTarget.status.maxHardwareVersion`. No `HostSystem` list, no label selectors.
- [ ] T115 [S9.c] Unit tests for HW-version comparison and the `Zone → cluster MoID → ConfigTarget` resolution path.
- [ ] T116 [S9.d] [P] Integration tests with vcsim — `test/intg/webhook/vm_policy_intg_test.go`: mode-deny, extraConfig-deny, HW-version-deny (via `ConfigTarget.status.maxHardwareVersion`), plus happy path for each.
- [ ] T117 [S9.e] E2E test — `test/e2e/vmservice/configpolicy/vm_policy_test.go`: rejected VM on mode-deny; rejected on extraConfig-deny; rejected on HW-version-deny (driven by `ConfigTarget.status.maxHardwareVersion`); accepted on happy path; all assertions with clear reason strings.

---

## Phase 3 — Cleanup + verification (after all Phase 2 tasks merged)

- [ ] T200 Run `make generate manifests` in vim-api; verify no uncommitted diffs
- [ ] T201 Run `golangci-lint run ./...`; fix all new lint errors
- [ ] T202 Run `make test` (unit + integration); confirm all tests pass
- [ ] T203 Verify `test/e2e/` suite runs green on the test Supervisor cluster
- [ ] T204 Update `external/vim/doc/controller-workflows.md` to reflect any implementation changes from the plan
- [ ] T205 Update TDS (wiki A4) with any implementation deviations discovered during coding
