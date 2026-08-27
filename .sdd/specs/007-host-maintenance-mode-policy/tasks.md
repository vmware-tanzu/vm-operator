# Tasks: Host Maintenance Mode Infra Policies

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Epic**: vmop-4057, vmop-4058, vmop-4059

> Story/sub-task tickets, split by layer: **vmop-4103** (CRDs and schema — new CRD types, generated manifests/deepcopy, CRD install gating, VM API "Valid policy types" doc comment; linked to epic vmop-4057), **vmop-4104** (reconciler — `policyevaluation` controller wiring for both new CRD kinds; linked to epic vmop-4058), **vmop-4105** (condition — the `VirtualMachinePowerStateSynced`/`VMEvacuation` status-condition work; linked to epic vmop-4059) — each linked via `customfield_10830`. Tasks that touch more than one layer carry multiple tags, comma-separated in ascending order, per `sdd-standards.md`. Trivial tasks that produce no shipping code omit a tag.

## Phase 1 — Setup

- [x] T001 [P] [vmop-4103] Scaffold `external/vsphere-policy/api/v1alpha1/automatichostevacuationpolicy_types.go`, cloning `controlledrebalancingpolicy_types.go`'s shape (spec: `description`, `policyID`, `enforcementMode`, `match`, `tags`; status: `observedGeneration`, `conditions`; same kubebuilder markers and `GetConditions`/`SetConditions` methods; register in `init()`).
- [x] T002 [P] [vmop-4103] Scaffold `external/vsphere-policy/api/v1alpha1/besteffortrestartpolicy_types.go`, identical shape to T001.
- [x] T003 [vmop-4103] Run `make generate-go` to regenerate `external/vsphere-policy/api/v1alpha1/zz_generated.deepcopy.go` for both new types.
- [x] T004 [vmop-4103] Run `make generate-external-manifests` to generate `config/crd/external-crds/vsphere.policy.vmware.com_automatichostevacuationpolicies.yaml` and `..._besteffortrestartpolicies.yaml`.

## Phase 2 — Foundational (feature flags, capabilities, CRD install gating)

- [x] T005 [vmop-4103] Add a single `VMEvacuation bool` to `FeatureStates` in `pkg/config/config.go`, gating both new CRDs and the status-condition behavior.
- [x] T006 [vmop-4103] Add `CapabilityKeyVMEvacuation = "supports_infrapolicy_vm_evacuation"` to `pkg/config/capabilities/capabilities.go`, plus the one `case` entry in `updateCapabilitiesFeaturesFromCRD`.
- [x] T007 [P] [vmop-4103] Unit tests for the new capability key in `pkg/config/capabilities/capabilities_test.go` (`UpdateCapabilitiesFeatures` and `WouldUpdateCapabilitiesFeatures` tables), mirroring the `ControlledRebalancingPolicy` cases.
- [x] T008 [vmop-4103] Add two new `case "AutomaticHostEvacuationPolicy":`/`case "BestEffortRestartPolicy":` blocks in `pkg/crd/crd.go`'s `Install`, both gated on `features.VSpherePolicies && features.VMEvacuation` — the same single flag for both CRDs.
- [x] T009 [P] [vmop-4103] Unit tests for the new CRD-gating matrix in `pkg/crd/crd_test.go`, mirroring the existing `ControlledRebalancingPolicy` `When(...)` blocks (`VSpherePolicies`/`VMEvacuation` off, on, both on — both new CRDs install/don't install together since they share one flag).
- [x] T010 [P] [vmop-4103] Register `&vspherepolv1.AutomaticHostEvacuationPolicy{}` and `&vspherepolv1.BestEffortRestartPolicy{}` in `test/builder/fake.go`'s `KnownObjectTypes()`.

## Phase 3 — User Story: CSP admin applies AutomaticHostEvacuationPolicy

**Design**: the reconciler is refactored from one hand-written function set per kind into a table-driven registry, so that Phase 4's kind is added by appending a table entry, not writing new functions. See `plan.md` "Controller / webhook impact" for the full design and its scope (reconciler only — `pkg/crd/crd.go` install-gating and RBAC markers stay per-kind, unaffected).

- [ ] T011 [US-CSP-admin] [vmop-4104] In `controllers/vspherepolicy/policyevaluation/policyevaluation_controller.go`: add the `matchableComputePolicy` interface (embeds `ctrlclient.Object`; adds `GetMatchSpec() *vspherepolv1.MatchSpec`, `GetEnforcementMode() vspherepolv1.PolicyEnforcementMode`, `GetPolicyTags() []string`), implement it on `ComputePolicy` in `external/vsphere-policy/api/v1alpha1/computepolicy_types.go` (next to its existing `GetConditions`/`SetConditions`), and add the `computePolicyKindDescriptor` table with a single `ComputePolicy` entry (no feature gate, matching today's always-on behavior). Named `matchableComputePolicy` rather than plan.md's `matchablePolicy` — internally all these CRD kinds are referred to as compute policies (the domain started as one generic `ComputePolicy` CRD before splitting into per-kind CRDs), so the umbrella interface name follows that convention.
- [ ] T012 [US-CSP-admin] [vmop-4104] Replace `reconcileMandatoryComputePolicies` and `reconcileExplicitPolicies`'s switch with one generic loop each over `computePolicyKinds`, using `apimeta.ExtractList` (`k8s.io/apimachinery/pkg/api/meta`) to enumerate items from each descriptor's `newList()`; replace `addComputePolicy`/`addComputePolicyRef` with one generic helper operating on `matchableComputePolicy`; replace the single `Watches(&vspherepolv1.ComputePolicy{}, ...)`/`computePolicyToPolicyEvaluationMapperFn` in `AddToManager` with a loop over `computePolicyKinds` too. Confirmed existing `ComputePolicy` unit tests pass unchanged (behavior must not regress) — same pass/fail counts before and after on this environment's pre-existing envtest-infra gap.
- [ ] T013 [US-CSP-admin] [vmop-4103, vmop-4104] Implement `matchableComputePolicy` on `AutomaticHostEvacuationPolicy` (`external/vsphere-policy/api/v1alpha1/automatichostevacuationpolicy_types.go`) and add its `computePolicyKindDescriptor` table entry (`automaticHostEvacuationPolicyKind` constant, `newObj`/`newList` constructors, gated on `Features.VMEvacuation`) — this is the entire per-kind diff the registry is meant to reduce it to.
- [ ] T014 [US-CSP-admin] [vmop-4104] Add the corresponding `// +kubebuilder:rbac` markers for `automatichostevacuationpolicies`/`automatichostevacuationpolicies/status`; run `make generate-manifests` to update `config/rbac/role.yaml`.
- [ ] T015 [P] [US-CSP-admin] [vmop-4104] Unit tests in `policyevaluation_controller_test.go`: mandatory match, feature-disabled (mandatory + explicit-ref), tag resolution via `TagPolicy`, mixed-kind matching (a VM matching `ComputePolicy` and `AutomaticHostEvacuationPolicy` simultaneously, exercising the shared loop), explicit-ref-does-not-match/not-found error paths.
- [ ] T016 [US-CSP-admin] [vmop-4104] E2E: add a `test/e2e/vmservice/` spec verifying a Mandatory `AutomaticHostEvacuationPolicy` tags a matching VM and appears in `status.policies` (see `plan.md` Test strategy item 1). Landed at `test/e2e/vmservice/vmservice/hostmaintenancepolicy/hostmaintenancepolicy.go`, labeled `experimental` per `e2e-testing.md` (excluded from CI until validated on real hardware). `AutomaticHostEvacuationPolicy` has no WCP admin API yet (unlike `ComputePolicy`, which is mirrored from a legacy `InfraPolicy` admin API), so the CRD is created directly via an admin client through two helper seams (`createTagPolicy`/`createAutomaticHostEvacuationPolicy`) meant to be swapped for a WCP admin API call later, if one is added. A second scenario in the same file covers a risk the existing `ComputePolicy` e2e coverage doesn't exercise generically: a policy's `match` widened *after* an already-created, non-matching VM exists, verifying the VM picks up the policy via the `computePolicyToPolicyEvaluationMapperFn` watch/informer path without being touched itself — a fake-client unit test can't catch that watch under-firing.

## Phase 4 — User Story: Tenant opts into BestEffortRestartPolicy

- [ ] T017 [US-tenant-admin] [vmop-4103, vmop-4104] Implement `matchablePolicy` on `BestEffortRestartPolicy` and add its `policyKindDescriptor` table entry, mirroring T013 — the registry from Phase 3 means this is the only reconciler change needed for this kind (no new watch, mandatory-func, or switch case to write by hand).
- [ ] T018 [US-tenant-admin] [vmop-4104] Add the corresponding RBAC markers for `besteffortrestartpolicies`/`besteffortrestartpolicies/status`; regenerate `config/rbac/role.yaml`.
- [ ] T019 [US-tenant-admin] [vmop-4103] Update the `Policies []PolicySpec` doc comment ("Valid policy types are: ...") in `api/v1alpha6/virtualmachine_types.go` to include `BestEffortRestartPolicy`; run `make generate-manifests` to propagate into generated CRD YAML doc strings.
- [ ] T020 [P] [US-tenant-admin] [vmop-4104] Unit tests in `policyevaluation_controller_test.go` for `BestEffortRestartPolicy`: explicit-ref match/no-match/not-found, mandatory match, feature-disabled — mirroring T015.
- [ ] T021 [US-tenant-admin] [vmop-4104] E2E: add a spec verifying explicit `spec.policies` reference to `BestEffortRestartPolicy` tags a matching VM and errors on a non-matching explicit reference (see `plan.md` Test strategy item 2).
- [ ] T022 [P] [US-tenant-admin] [vmop-4104] E2E: add a spec verifying a VM matching both a Mandatory `AutomaticHostEvacuationPolicy` and an Optional `BestEffortRestartPolicy` surfaces both entries in `status.policies` (see `spec.md`'s resolved decision on multi-match surfacing).

## Phase 5 — User Story: Power-state-synced status signal for host maintenance

- [ ] T023 [US-devops-vks] [vmop-4105] Implement a host-maintenance-mode lookup helper (e.g. `GetHostMaintenanceState`) in `pkg/providers/vsphere/vcenter/host.go`, following the `GetESXHostFQDN` pattern: resolve `HostSystem.runtime.inMaintenanceMode` and `HostSystem.recentTask` (resolved to check for an active `EnterMaintenanceMode_Task`/`ExitMaintenanceMode_Task`). Wrap the vCenter call with `pkgctx.WithVCOpID`.
- [ ] T024 [US-devops-vks] [vmop-4105] Extend `reconcileStatusPowerState` in `pkg/providers/vsphere/vmlifecycle/update_status.go`: check `pkgcfg.FromContext(ctx).Features.VMEvacuation` first (no-op, unchanged behavior when off); when on and in the not-synced branch, call the new helper against `vmCtx.MoVM.Runtime.Host` and set `Reason` to `InfraInMaintenance` with a host-non-identifying `Message` when the host is in/entering/exiting maintenance mode.
- [ ] T025 [P] [US-devops-vks] [vmop-4105] Unit tests in `pkg/providers/vsphere/vmlifecycle/update_status_test.go`: `VMEvacuation` off (unchanged `NotSynced` behavior regardless of host state), `VMEvacuation` on with host not in maintenance mode (unchanged `NotSynced`), `VMEvacuation` on with host `inMaintenanceMode=true`, `VMEvacuation` on with host having an active Enter/ExitMaintenanceMode task — each crossed with synced/not-synced power state; assert the message contains no host-identifying text.
- [ ] T026 [P] [vmop-4105] Unit/vcsim test in `pkg/providers/vsphere/vcenter/` for the new host-maintenance-mode helper itself, if T023's implementation needs live vCenter-shaped verification of task `descriptionId` matching.
- [ ] T027 [US-devops-vks] [vmop-4105] E2E: add a spec driving a host into maintenance mode (via whatever host-state test helper `test/e2e` already supports) and asserting the condition/reason transitions as described in `plan.md` Test strategy item 3, including reversion once the host exits maintenance mode. Cover both `VMEvacuation` on and off.

## Phase 6 — Backport to `release/vc-9.1.0`

Do not start until Phases 1–5 are merged to `main`. See `spec.md` "Branch strategy" and `plan.md` "Branch / backport strategy".

- [ ] T028 [vmop-4103, vmop-4104, vmop-4105] Cherry-pick the merged `main` commit(s) for T001–T027 onto `release/vc-9.1.0`. Expect every file except `api/v1alpha6/virtualmachine_types.go` to apply cleanly (verified: that branch already has the base `ComputePolicy`/`PolicyEvaluation` framework and no `v1alpha6` tree).
- [ ] T029 [vmop-4103] Resolve the expected conflict/no-op on the doc-comment hunk by hand: apply the same "Valid policy types are: ..." addition of `BestEffortRestartPolicy` to `api/v1alpha5/virtualmachine_types.go` on `release/vc-9.1.0`; run that branch's `make generate-manifests`.
- [ ] T030 [vmop-4103] Confirm `external/vsphere-policy`'s pinned/vendored version on `release/vc-9.1.0` picks up the two new CRD types (module version bump or in-tree sync, whichever that branch's dependency process uses).
- [ ] T031 [P] [vmop-4103, vmop-4104, vmop-4105] Run the full unit test suite on `release/vc-9.1.0` after the cherry-pick and the manual doc-comment fix; fix any branch-specific fallout (e.g. `v1alpha5`-specific test fixtures that don't exist on `main`).
- [ ] T032 [vmop-4103, vmop-4104, vmop-4105] E2E: port the `main`-branch scenarios from T016, T021, T022, T027, and T034 to `release/vc-9.1.0`'s e2e suite, adjusted for that branch's `apiVersion: vmoperator.vmware.com/v1alpha5`.
- [ ] T033 [vmop-4103, vmop-4104, vmop-4105] Open the backport PR against `release/vc-9.1.0` with its own release note per that branch's process; do not reuse the `main` PR number.

## Phase Final — Polish

- [ ] T034 [vmop-4103] E2E: add CRD-presence-gating coverage for both new kinds (flag on/off), per `plan.md` Test strategy item 4 (on `main`; ported to `release/vc-9.1.0` as part of T032).
- [ ] T035 [vmop-4103, vmop-4104, vmop-4105] Update release notes per `pull-request-standards.md`, covering both new CRD kinds and the new condition reason, noting this is Supervisor-capability-gated behind the single `supports_infrapolicy_vm_evacuation` capability. File on both the `main` PR and, separately, the `release/vc-9.1.0` backport PR (T033).
- [ ] T036 Flip `spec.md`'s `Status` from `Draft` to `Implemented` once Phases 1–6 are checked.
