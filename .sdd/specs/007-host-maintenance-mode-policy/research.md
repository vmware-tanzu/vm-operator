# Research: Host Maintenance Mode Infra Policies

- **Spec**: [`spec.md`](./spec.md)

## Source documents

- WIKI page 2535776102 — "One pager: VM Service - Support for Host Maintenance Mode Infra Policies". Business problem, goals/non-goals, design overview, status-condition computation, references (an internal problem-statement doc and a DRS proposal design doc — not reproduced here per this repo's internal-link policy).
- WIKI page 2781824956 — "Host Maintenance Mode Infra Policy CRDs". CRD shape and example manifests for both new policy kinds, plus the `VirtualMachine` spec/status examples.

## Prior art in this repo

### `ControlledRebalancingPolicy` (PR #1787, PR #1842)

This is the direct precedent for how to add a new `vsphere.policy.vmware.com` compute-policy kind:

- **PR #1787** ("Add ControlledRebalancingPolicy & RequiredDuringExecutionVMPlacementPolicy CRDs") added the CRD type only: `external/vsphere-policy/api/v1alpha1/controlledrebalancingpolicy_types.go`, registered in that package's `init()` (`objectTypes`), plus generated deepcopy and manifests.
- **PR #1842** ("Controlled Rebalancing CR Reconciliation") wired the new CRD into the existing generic evaluation pipeline:
  - `pkg/config/config.go`: added `ControlledRebalancingPolicy bool` to `FeatureStates`.
  - `pkg/config/capabilities/capabilities.go`: added `CapabilityKeyControlledRebalancingPolicy = "supports_infrapolicy_controlled_rebalancing"` and a `case` in `updateCapabilitiesFeaturesFromCRD` mapping the Supervisor capability's `Activated` bool onto the feature flag.
  - `pkg/crd/crd.go`: added a `case "ControlledRebalancingPolicy":` in `Install` gating CRD installation on `features.VSpherePolicies && features.ControlledRebalancingPolicy` (previously it was unconditionally installed alongside `ComputePolicy` whenever `VSpherePolicies` was on — this PR split it out so it has its own independent gate).
  - `controllers/vspherepolicy/policyevaluation/policyevaluation_controller.go`: conditionally added a `Watches(&vspherepolv1.ControlledRebalancingPolicy{}, ...)` (only when the feature is enabled), a `reconcileMandatoryControlledRebalancingPolicies` function mirroring `reconcileMandatoryComputePolicies`, `addControlledRebalancingPolicyRef`/`addControlledRebalancingPolicy` mirroring the `ComputePolicy` equivalents, a `controlledRebalancingPolicyKind` constant, a case in `reconcileExplicitPolicies`'s kind switch (feature-flag-checked, logs and skips if disabled), and a `controlledRebalancingPolicyToPolicyEvaluationMapperFn` watch-mapper. It also refactored the previously `ComputePolicy`-specific `matchesPolicy`/policy-result-append helpers (`addPolicyResult`, `resolvePolicyTags`) into kind-agnostic helpers reused by both policy kinds — this refactor is available for reuse by this feature's new kinds too.
  - `config/rbac/role.yaml`: added `get;list;watch` on `controlledrebalancingpolicies` and `get` on `controlledrebalancingpolicies/status`, via the `// +kubebuilder:rbac` markers on the reconciler.
  - `test/builder/fake.go`: registered `&vspherepolv1.ControlledRebalancingPolicy{}` in `KnownObjectTypes()`.
  - `api/v1alpha6/virtualmachine_types.go`: updated the `Policies []PolicySpec` doc comment's "Valid policy types are: ..." list to include `ControlledRebalancingPolicy` (only `v1alpha6` was touched by that PR — `v1alpha5` was not, likely because `ControlledRebalancingPolicy` had no VKS/`v1alpha5` requirement).
  - Unit tests only (`policyevaluation_controller_test.go`, `capabilities_test.go`, `crd_test.go`) — no `test/e2e` changes accompanied either PR. This feature's plan explicitly calls for e2e coverage where PR #1787/#1842 did not add any (see `plan.md`).

### `MatchSpec` / `PolicyEvaluation` framework (`external/vsphere-policy/api/v1alpha1/common_types.go`, `policyevaluation_types.go`)

- `MatchSpec` (workload labels, guest ID/family, image name/labels, boolean composition) is shared across every policy kind and does not need to change for this feature — both `AutomaticHostEvacuationPolicy` and `BestEffortRestartPolicy` reuse it as-is per the example manifests in WIKI page 2781824956.
- `PolicyEvaluationResult`/`PolicyEvaluationStatus.Policies` (on the `PolicyEvaluation` CR, not the `VirtualMachine`) already carries `APIVersion`/`Kind`/`Name`/`Generation`/`Tags` — sufficient for both new kinds without modification.

### VM-side consumption (`pkg/vmconfig/policy/policy_reconciler.go`, `policy_reconciler_util.go`)

- `getPolicyEvaluationResults` (in `policy_reconciler_util.go`) builds/patches a per-VM `PolicyEvaluation` object (name `vm-<vmName>`) from the VM's explicit `spec.policies`, image, and workload/guest criteria, then waits for it to report `Ready` before returning `obj.Status.Policies`. This function is kind-agnostic already — it works unchanged for any policy kind the `PolicyEvaluation` controller knows how to evaluate.
- `Reconcile` (in `policy_reconciler.go`) diffs the vSphere tags implied by the evaluation results against what's currently attached (tracked via the `vmservice.policy.tags` ExtraConfig key) and issues `TagSpec` add/remove operations in the VM's `ConfigSpec`. It only writes `vm.Status.Policies` (`[]vmopv1.PolicyStatus`, copying `APIVersion`/`Kind`/`Generation`/`Name` from each `PolicyEvaluationResult`) once there are no pending tag changes. This is also kind-agnostic already.
- **Conclusion**: no changes are needed in `pkg/vmconfig/policy` itself for either new CRD to be tagged and surfaced in `status.policies` — only the `PolicyEvaluation` controller needs to learn the two new kinds (same shape as the `ControlledRebalancingPolicy` PR #1842 changes).
- `PolicyStatus` (`api/v1alpha5/virtualmachine_policy_types.go`) currently has no `PolicyID` field. WIKI page 2781824956's status example originally included one; the requester has since corrected the wiki page to drop it. **Resolved: no `PolicyID` field is added.**

### VM power-state status condition (`pkg/providers/vsphere/vmlifecycle/update_status.go`)

- `VirtualMachinePowerStateSynced` condition already exists (`api/v1alpha5/virtualmachine_types.go:84-86`, mirrored in `v1alpha6`) and is already computed every reconcile in `reconcileStatusPowerState`:
  ```go
  if vmCtx.VM.Status.PowerState == vmCtx.VM.Spec.PowerState {
      c := conditions.TrueCondition(vmopv1.VirtualMachinePowerStateSynced)
      c.Reason = "Synced"
      ...
  } else {
      conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachinePowerStateSynced, "NotSynced", ...)
  }
  ```
- This feature's only code change to this file is adding a branch inside the `else` (not-synced) case: when the VM's host is in/entering/exiting maintenance mode, use a different reason (see spec.md's open question about the exact string) instead of the generic `"NotSynced"`. This requires resolving the VM's current host (`vmCtx.MoVM.Runtime.Host`) and querying that host's `runtime.inMaintenanceMode` plus its `recentTask` list for an active `EnterMaintenanceMode_Task`/`ExitMaintenanceMode_Task`.
- `pkg/providers/vsphere/vcenter/host.go` already exists as the home for small host-property-collector helpers (currently just `GetESXHostFQDN`, fetching `HostNetworkSystem.dnsConfig` via `object.NewHostSystem(...).ConfigManager().NetworkSystem(ctx)` then a targeted `Properties` call). A new helper in this same file, following the identical pattern but fetching `HostSystem.runtime` and `HostSystem.recentTask` (resolved to `TaskInfo` to check the task's `descriptionId`/name), is the natural place for the new host-maintenance-mode lookup. No existing helper does this today — this is genuinely new code, not a refactor of something existing.
- `ReconcileStatusData` (passed through every `reconcileStatus*` function) currently only carries `NetworkDeviceKeysToSpecIdx`. It does not currently carry host info, so `reconcileStatusPowerState` will need to either take a new parameter/field or fetch the host directly via the `ctrlclient`/`vim25` client already available in its call chain — this is a plan-level decision, not yet made.

### `infra.vmware.com` group (`external/infra/api/v1alpha1/`) — considered and ruled out

- Initially hypothesized that "Infra Policy" in the design-doc titles meant the `infra.vmware.com` API group (home of `StoragePolicy`, reconciled by a dedicated `controllers/storage/storagepolicy` controller, not the `PolicyEvaluation`/`Match` framework). **This was wrong.** Both example manifests in WIKI page 2781824956 use `apiVersion: vsphere.policy.vmware.com/v1alpha1`. "Infra Policy" in the design docs is the one-pager's informal name for the general Supervisor compute-policy/tagging framework (the thing `ComputePolicy`, `ControlledRebalancingPolicy`, `TagPolicy`, etc. already belong to), not a reference to the `infra.vmware.com` CRD group. The new CRDs belong in `external/vsphere-policy/`, as siblings of `ControlledRebalancingPolicy`.

### API version scope: `v1alpha6` on `main`, `v1alpha5` on `release/vc-9.1.0` (two branches, one design)

- WIKI page 2535776102 states as a goal: "VKS requirement: this should be implemented in v1alpha5 CRDs as VKS does not yet consume v1alpha6 for 9.1.2." Initially (in an earlier pass of this SDD session) this was read as "implement against `v1alpha5` only, on `main`." **Superseded**: the requester clarified that `main`'s storage version is `v1alpha6`, and the `v1alpha5`/9.1.2 requirement is actually about a *separate branch* (`release/vc-9.1.0`), not about `main` skipping its own current storage version. The correct shape is: implement once against `v1alpha6` on `main`, then cherry-pick the same commits onto `release/vc-9.1.0`, where the equivalent edits land on `v1alpha5` (that branch's storage version).
- Verified directly by inspecting `release/vc-9.1.0` (`git ls-tree`/`git show`):
  - `api/` on that branch only goes up to `v1alpha5` — no `v1alpha6` tree exists there. Confirms `v1alpha5` is its ceiling/storage version.
  - `controllers/vspherepolicy/policyevaluation/policyevaluation_controller.go` and `external/vsphere-policy/api/v1alpha1/computepolicy_types.go` already exist on that branch — the base `ComputePolicy`/`PolicyEvaluation` framework this feature extends is present.
  - `external/vsphere-policy/api/v1alpha1/controlledrebalancingpolicy_types.go` does **not** exist on that branch (that CRD is `main`-only, added after `release/vc-9.1.0` branched) — irrelevant to this feature, noted only to confirm the branch is behind `main` on unrelated vsphere-policy work, not broken.
  - `pkg/config/capabilities/capabilities.go` on that branch already has `CapabilityKeyVSpherePolicies`/`fs.VSpherePolicies` — the top-level gate our two new capability keys additionally depend on is present.
- `PolicySpec`/`PolicyStatus` are structurally identical across `v1alpha1`..`v1alpha6` today and are converted automatically by generated conversion code — no manual conversion function exists for this field today because no version-specific logic is needed, on either branch. Adding a new valid policy `Kind` string does not require touching the conversion layer (`Kind` is an opaque string in `PolicySpec`/`LocalObjectRef`).
- The "Valid policy types are: ..." doc comment on `VirtualMachineSpec.Policies` needs a manual update on `main` (`api/v1alpha6/virtualmachine_types.go`, same file `ControlledRebalancingPolicy` touched) **and** a second manual update on `release/vc-9.1.0` (`api/v1alpha5/virtualmachine_types.go`) during the backport, since that file path doesn't exist on `main`'s tree at the `v1alpha5` location the cherry-pick would otherwise target — this is the one hunk in the whole change that cannot cherry-pick cleanly.

## Related, likely-unconnected prior work

- Several already-merged commits reference `maintenance.vm.evacuation.poweroff` ExtraConfig handling for VMs with vGPU/passthrough devices and instance storage (`git log --grep`). These predate this spec and appear to be a narrower, ExtraConfig-only signal for a specific device class, not the general policy-driven mechanism described here. Not modified by this spec; flagged here in case a future implementer needs to reconcile the two mechanisms.

## Resolution of open items

All four items originally flagged as open in this research pass were resolved directly by the requester:

- No `PolicyID` field on `PolicyStatus` — confirmed via a wiki-page correction.
- The condition `Reason` string is `InfraInMaintenance`.
- The status-condition branch is gated, but behind a **new, dedicated** capability/flag (`supports_infrapolicy_vm_evacuation` / `Features.VMEvacuation`) rather than reusing `Features.AutomaticHostEvacuationPolicy` — decoupling the status signal's rollout from either policy CRD's rollout.
- Multi-policy-match status surfacing ("surface both") requires no code change — it is already how `pkg/vmconfig/policy` behaves today for any two simultaneously-matching policy kinds.

See `spec.md`'s "Resolved decisions" section for the authoritative statement of each.
