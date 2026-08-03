# Tasks: VM Service Class Policy and Resize

- **Input**: `specs/001-class-policy-resize/spec.md` + `specs/001-class-policy-resize/plan.md`
- **Epic**: vmop-3331
- **Format**: `[ID] [P?] [Story] Description — file paths`

Tasks within a Phase that are marked `[P]` may run in parallel. Each task corresponds to a Story or Sub-task; ticket keys are noted where created.

---

## Phase 0 — Architecture (wiki; no code changes)

Dependencies: none. All A-tasks may run in parallel `[P]`.

- [ ] T001 [P] [A1/vmop-3733] Apply draft One Pager content to wiki page ID: 2453059721 — fill Business Problem, Goals, Non-Goals, Big Picture, `ConfigTarget` per-host iteration section (max HW version + per-host SR-IOV with DVX), `VirtualMachineConfigOptions` GC step, capability gate subsection, Test Plan table
- [ ] T002 [P] [A2/vmop-3734] Update wiki pages ID: 2453059723 and 2453060275 — add SR-IOV EB limitation finding; add the `ConfigTarget.status` aggregation pattern (`maxHardwareVersion`, enriched `sriov` list with `hostMoID`); cross-link vmop-3470, vmop-3794, vmop-3926, and the updated Design/TDS
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
- [x] T015c [S1.b] [PR #1672] Extend `config/rbac/role.yaml` with create/get/list/patch/update/watch on `virtualmachineconfigoptions` and `virtualmachineguestoptions` — landed alongside the VirtualMachineConfigOptions controller (S6) rather than as its own change; this checkbox was stale (RBAC already present before S7 started)

### Story S2 — Partner-facing integration doc (vmop-3739)

- [ ] T020 [S2] Author `external/vim/doc/integration-guide.md` — pipeline diagram, role of each CRD, `ConfigTarget.status` capability surface (`maxHardwareVersion`, enriched `sriov`), per-host iteration inside the `ConfigTarget` controller, syncMode semantics, worked example of policy denial
- [ ] T021 [S2] Link doc from public RTD site; review by PM and at least one downstream consumer team

*Gate (superseded): this originally required T021 complete before any Phase 2 code work began. In practice, S3/S5/S6 (and now S7) started and merged while T021 was still open, matching the Phase 2 header's actual rule ("S3/S6/S7/S8/S9 may begin after S1 is merged"). T021 remains a real, outstanding task — just not a blocking gate for the rest of Phase 2.*

---

## Phase 2 — Implementation (code; S5 depends on S1+S2; S3/S6/S7/S8/S9 may begin after S1 is merged)

> *Story S4 was retired.* The previously-planned `HostSystem` CRD work (Story vmop-3741 and its sub-tasks vmop-3752, vmop-3753, vmop-3754, vmop-3755, vmop-3756) is closed as *Won't Implement*. The per-host vSphere queries now happen inside the `ConfigTarget` controller and write directly to `ConfigTarget.status` (see Story S5 for the cluster-scope path and Story S10 for the SR-IOV per-host path). See `research.md` Finding 7 for the rationale.

### Story S3 — Zone controller fan-out (vmop-3740)

- [x] T050 [S3.a] [PR #1695 / vmop-3740] Modify `controllers/infra/zone/zone_controller.go` — derive cluster MoIDs from the Zone's AvailabilityZone (`ClusterComputeResourceMoIDs`); CreateOrPatch ConfigTarget per MoID; CreateOrPatch VirtualMachineConfigPolicy per zone (default syncMode=ConfigTarget on create only)
- [x] T051 [S3.a] [PR #1695 / vmop-3740] Unit tests — `controllers/infra/zone/zone_controller_test.go`
- [x] T052 [S3.b] [PR #1695 / vmop-3740] Integration tests with vcsim — `controllers/infra/zone/zone_controller_test.go`: zone→AvailabilityZone cluster MoID derivation; idempotent create/patch (UID stable); ConfigTarget not deleted when pool MoIDs removed from zone
- [x] T053 [S3.c] [PR #1695 / vmop-3740] E2E test — `test/e2e/vmservice/vmservice/configpolicy/configpolicy.go`: Zone creation materialises ConfigTarget and VirtualMachineConfigPolicy; idempotency verified with Consistently over 30 s

### Story S5 — ConfigTarget controller (vmop-3742)

- [x] T060 [S5.a] [PR #1711 / vmop-3757] Author `webhooks/configtarget/validation/configtarget_validator.go` — immutable spec.id; valid cluster MoID format for metadata.name
- [ ] T061 [S5.a] [PR #1711 / vmop-3757 — webhook half only] Register ConfigTarget webhook in `controllers/controllers.go` and `webhooks/` suite files
- [x] T061a [S5.a] Register ConfigTarget controller in `controllers/controllers.go`, gated on `Features.VirtualMachineConfigPolicy`
- [x] T062 [S5.a] [PR #1711 / vmop-3757] Unit and integration tests — `webhooks/configtarget/validation/configtarget_validator_unit_test.go`, `configtarget_validator_intg_test.go`
- [x] T063 [S5.b] Author `controllers/configtarget/configtarget_controller.go` — cluster-scope path: QueryConfigTarget + QueryConfigOptionDescriptor; populate status; fan out VirtualMachineConfigOptions
- [x] T064 [S5.b] Add `GetVirtualMachineConfigTarget` (QueryConfigTarget + QueryConfigOptionDescriptor) — implemented directly on `pkg/providers/vsphere/vmprovider.go` (see note below) instead of a new `environment_browser.go` file
- [x] T065 [S5.b] Unit tests — `controllers/configtarget/configtarget_controller_test.go` (includes vcsim-backed integration coverage in the same file)

> **Implementation note (T063–T065):** `metadata.name` (not `spec.id`) is the cluster MoID key. This is confirmed correct, not just a convention choice: `spec.md`'s acceptance criteria key every `ConfigTarget` lookup off `metadata.name` (`spec.id` is referenced only as an immutability constraint), and the merged Zone controller (vmop-3740, PR #1695) sets `spec.id.ID` to the same value as `metadata.name` on every `ConfigTarget` it creates — so the two fields are always identical in practice. `GetVirtualMachineConfigTarget` was placed directly on `vSphereVMProvider` in `vmprovider.go` — consistent with other single-purpose vSphere queries in that file (`DoesProfileSupportEncryption`, `GetStoragePolicyStatus`) — rather than the `pkg/providers/vsphere/environment_browser.go` sketched in the plan; that file can still be introduced later once `QueryConfigOptionEx` (S6) and the per-host iteration (S5.c) grow the surface area.
>
- [x] T066b [S5.c] Compute `ConfigTarget.status.maxHardwareVersion` as `max(Key)` among `QueryConfigOptionDescriptor` results with `CreateSupported == true` — data the cluster-scope path already fetches for the `VirtualMachineConfigOptions` fan-out. No host enumeration or `PropertyCollector` calls needed. Implemented in `controllers/configtarget/configtarget_controller.go` (`computeMaxHardwareVersion`).
- [x] T066c [S5.b] Map the 19 non-SR-IOV `ConfigTargetDevices` categories (CDROM, Floppy, Serial, Parallel, Sound, USB, PCIPassthrough, DynamicPassthroughDevices, VGPUDevice, VGPUProfile, SharedGPUPassthroughTypes, SGXTargetInfo, PrecisionClockInfo, VendorDeviceGroupInfo, DVXClassInfo, IDEDisks, SCSIDisks, SCSIPassthrough, VFlashModule) directly from the cluster-scope `QueryConfigTarget` result the controller already fetches. Implemented in `controllers/configtarget/convert.go` (`populateConfigTargetDevices`). SR-IOV (both `ct.Sriov` and any `VirtualMachineSriovInfo` inside the `PciPassthrough` union) is excluded — see Story S10.
- [x] T067 [S5.c] Unit tests for `maxHardwareVersion` — aggregation across mixed `CreateSupported`/malformed-key descriptors, and the vcsim-backed multi-host proof, in `controllers/configtarget/configtarget_controller_test.go` (unit + vcsim `Describe`s).
- [ ] T068 [S5.b] Author `controllers/configtarget/gc.go` — `GCVirtualMachineConfigOptions` helper; runs only after both `QueryConfigTarget` and `QueryConfigOptionDescriptor` succeed. There is no `GCHostSystems` — there are no per-host objects to GC.
- [ ] T069 [S5.b] Unit tests for `GCVirtualMachineConfigOptions` — correct candidate selection from (existing CRs, latest enumeration).
- [ ] T070 [S5.d] [P] Integration tests with vcsim, added to the in-file vcsim-backed `Describe` in `controllers/configtarget/configtarget_controller_test.go` (this repo has no separate `test/intg/configtarget/` suite) — missing cluster; transient cluster-scope error → retry (no GC); `VirtualMachineConfigOptions` fan-out; drop HW version → GC deletes the orphan `VirtualMachineConfigOptions`. The `maxHardwareVersion`-from-mixed-hosts case is already covered there.
- [x] T071a [S5.e] E2E test (cluster-scope subset) — `test/e2e/vmservice/vmservice/configpolicy/configpolicy.go`: `ConfigTarget.status` populated from real EB and `Ready=True`; `VirtualMachineConfigOptions` fanned out per hardware version; a synthetic-hardware-version `VirtualMachineConfigOptions` owned by a real `ConfigTarget` is garbage-collected on the next reconcile (vcsim's EnvironmentBrowser reports a fixed descriptor set per model, so the drop-a-version GC path can't be exercised by shrinking a real cluster's reported versions — this test injects the staleness directly instead).
- [x] T071 [S5.e] E2E test — `test/e2e/vmservice/vmservice/configpolicy/configpolicy.go`: `status.maxHardwareVersion` non-empty/valid and `CDROM` (a universally-present, non-SR-IOV category) non-empty. Per-host SR-IOV E2E coverage is Story S10's T124.

### Story S6 — VirtualMachineConfigOptions controller (vmop-3743)

- [x] T080 [S6.a] [PR #1671 / vmop-3762] Author `webhooks/virtualmachineconfigoptions/validation/virtualmachineconfigoptions_validator.go` — immutable spec.hardwareVersion; format ^vmx-\d+$; metadata.name must equal spec.hardwareVersion
- [x] T081 [S6.a] [PR #1671 / vmop-3762] Unit tests (13 specs) + integration test stubs — `webhooks/virtualmachineconfigoptions/validation/`
- [x] T082 [S6.b] [PR #1672] Author `controllers/virtualmachineconfigoptions/vmconfigoptions_controller.go` — QueryConfigOptionEx; map vim.vm.ConfigOption → status; fan out VirtualMachineGuestOptions per guest OS; update listMap entry keyed by hardwareVersion
- [x] T083 [S6.b] [PR #1672] Add `pkg/providers/vsphere/environment_browser.go` — `QueryConfigOptionEx` wrapper, consistent with T064's note that single-purpose EnvironmentBrowser queries can live in a dedicated file once the surface area grows
- [x] T084 [S6.b] [PR #1672] Unit tests — `controllers/virtualmachineconfigoptions/vmconfigoptions_controller_test.go` (envtest-backed suite; 5 specs, 75.5% coverage)
- [x] T085 [S6.c] [PR #1672] Integration coverage — folded into `controllers/virtualmachineconfigoptions/vmconfigoptions_controller_test.go` per `testing-standards.md` (no separate `test/intg` tree exists in this repo; unit/integration tests are differentiated by Ginkgo `Label()`, not by file path). Covers happy path and idempotent re-reconcile; no golden-file fixture was added.
- [x] T086 [S6.d] [PR #1672] E2E test — `test/e2e/vmservice/vmservice/configpolicy/configpolicy.go`: `VirtualMachineConfigOptions.status.guestOSIdentifiers` populated and `Ready=True`, and a `VirtualMachineGuestOptions` fanned out per reported guest OS (folded into the shared Spec function established by S3/S5, not a standalone `vmconfigoptions_test.go`)

### Story S7 — VirtualMachineGuestOptions plumbing (vmop-3744)

- [x] T090 [S7.a] [vmop-3766] Author `webhooks/virtualmachineguestoptions/validation/virtualmachineguestoptions_validator.go` — immutable spec.id; metadata.name must equal the DNS-safe transform of spec.id (shared helper extracted to `pkg/util.VimGuestOptionsName`, also used by the S6 controller's fan-out); reject empty spec.id
- [x] T091 [S7.a] [vmop-3766] Unit tests + manifest registration (`config/webhook/manifests.yaml` regenerated via `make generate-manifests`); RBAC was already present (see T015c) so no role.yaml change was needed
- [x] T092 [S7.b] Integration tests with envtest — `webhooks/virtualmachineguestoptions/validation/virtualmachineguestoptions_validator_intg_test.go` (no separate `test/intg/` tree exists in this repo, per `testing-standards.md`). The two-hardware-versions-fan-into-one-GuestOptions scenario this task originally described is controller behavior, not webhook validation, and was already covered by S6's `controllers/virtualmachineconfigoptions/vmconfigoptions_controller_test.go` ("two VirtualMachineConfigOptions for different hardware versions report the same guest OS") — this checkbox was stale.
- [x] T093 [S7.c] E2E test — already satisfied by S6's T086 (`test/e2e/vmservice/vmservice/configpolicy/configpolicy.go`, "Should fan out a VirtualMachineGuestOptions object for each guest OS reported by the cluster"); this checkbox was stale. No dedicated E2E test was added for the webhook's rejection paths (immutable spec.id, name/id mismatch, empty spec.id), consistent with S6's ConfigOptions webhook precedent — those rules have unit + envtest integration coverage only.
- [x] T093a [S7.d] [vmop-3932] Author `garbageCollectGuestOptions` / `removeHardwareVersionAndDeleteIfOrphaned` in `controllers/virtualmachineconfigoptions/vmconfigoptions_controller.go` — prunes a `VirtualMachineGuestOptions.status.hardwareVersions` entry once its hardware version's `VirtualMachineConfigOptions` reconcile no longer reports that guest OS, deleting the object once no hardware-version entries remain. Closes the gap noted in `research.md` Finding 3 and the former spec.md edge case/out-of-scope entries.
- [x] T093b [S7.d] [vmop-3932] Unit tests — `controllers/virtualmachineconfigoptions/vmconfigoptions_controller_test.go`: a guest OS dropped from a hardware version's descriptor list deletes the sole-owning `VirtualMachineGuestOptions`; a guest OS still reported by a second hardware version keeps the object and only removes the dropped version's status entry
- [x] T093e [S7.d] [vmop-3932] Rename `VirtualMachineGuestOptionsHardwareVersionStatus.HardwareVersion` to `Version` (`status.hardwareVersions[].hardwareVersion` -> `status.hardwareVersions[].version`) — the old name stuttered with the parent field. Reviewer feedback on PR #1782; `vim.vmware.com/v1alpha1` has never been tagged/released, so this is a clean rename, not a breaking-API-change requiring a conversion webhook. Manifests regenerated via `controller-gen` for `external/vim/api/...`.

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

### Story S10 — ConfigTarget SR-IOV per-host enrichment (vmop-3926)

- [ ] T120 [S10.a] Extend `VirtualMachineSriovInfo` in `external/vim/api/v1alpha1/config_target_devices_types.go` with `HostMoID` (required), `Active`, `MaxVFs`, `NumVFs`, `DVXClass`, `DVXCheckpointSupported`, `DVXSWDMATracingSupported`; add `+listType=map` with `+listMapKey=hostMoID` and `+listMapKey=pciDevice.id` to `status.sriov`; regenerate `zz_generated.deepcopy.go` and `vim.vmware.com_configtargets.yaml`. T121 cannot be merged before this task.
- [ ] T121 [S10.b] Extend `controllers/configtarget/configtarget_controller.go` with SR-IOV per-host enrichment — enumerate cluster hosts via `ClusterComputeResource.host`; for each host, run one `PropertyCollector` RPC for `config.pciPassthruInfo` (filtered to `sriovCapable=true`) and `hardware.dvxClasses` (where `sriovNic=true`); build per-(host, NIC) `VirtualMachineSriovInfo` entries and write them to `ConfigTarget.status.sriov`. Treat per-host RPC failure as a warning event; do not block the rest of the iteration.
- [ ] T122 [S10.b] Unit tests — SR-IOV entries carry `hostMoID`; DVX class enrichment correlates correctly; partial-failure (one host RPC fails) leaves the other hosts' data intact and triggers a re-queue.
- [ ] T123 [S10.c] [P] Integration tests with vcsim, added to the in-file vcsim-backed `Describe` in `controllers/configtarget/configtarget_controller_test.go` — `status.sriov` entries carry `hostMoID` attribution; one host's RPC failure does not block other hosts' data.
- [ ] T124 [S10.d] E2E test — `test/e2e/vmservice/vmservice/configpolicy/configpolicy.go`: per-host SR-IOV entries with `hostMoID` on SR-IOV-capable clusters.

---

## Phase 3 — Cleanup + verification (after all Phase 2 tasks merged)

- [ ] T200 Run `make generate manifests` in vim-api; verify no uncommitted diffs
- [ ] T201 Run `golangci-lint run ./...`; fix all new lint errors
- [ ] T202 Run `make test` (unit + integration); confirm all tests pass
- [ ] T203 Verify `test/e2e/` suite runs green on the test Supervisor cluster
- [ ] T204 Update `external/vim/doc/controller-workflows.md` to reflect any implementation changes from the plan
- [ ] T205 Update TDS (wiki A4) with any implementation deviations discovered during coding
