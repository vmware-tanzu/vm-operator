# Implementation Plan: Host Maintenance Mode Infra Policies

- **Spec**: [`spec.md`](./spec.md)
- **Epic**: vmop-4057, vmop-4058, vmop-4059
- **Date**: 2026-08-25

## Summary

Add two new `vsphere.policy.vmware.com/v1alpha1` compute-policy CRDs (`AutomaticHostEvacuationPolicy`, `BestEffortRestartPolicy`), cloning `ControlledRebalancingPolicy`'s CRD *type* shape (PR #1787) for the schema, and add one new `Reason` to the existing `VirtualMachinePowerStateSynced` condition on `api/v1alpha5.VirtualMachine` so tenants and VKS can tell a maintenance-mode-driven power-off apart from any other unsynced power state. VM Operator does not gain any new evacuation, restart, or power-management logic — it only exposes the policy CRDs into the existing tag-application pipeline and computes one explanatory status field.

**Reconciler wiring deliberately does not follow PR #1842's per-kind-function precedent.** #1842 wires `ControlledRebalancingPolicy` into `policyevaluation_controller.go` by hand-adding a dedicated `Watches(...)`, `reconcileMandatory<Kind>Policies`, `add<Kind>PolicyRef`/`add<Kind>Policy`, mapper function, and switch case per kind (sharing only the innermost match/tag-resolve logic via kind-agnostic `addPolicyResult`/`resolvePolicyTags` helpers). This feature instead refactors the controller into a **table-driven registry**: a `matchableComputePolicy` interface plus a `computePolicyKindDescriptor` table that `AddToManager`, `reconcileMandatoryPolicies`, and `reconcileExplicitPolicies` all iterate over, so that adding a new policy kind is (mostly) appending one entry to the table rather than writing a new function per kind for each of these three call sites. See "Controller / webhook impact" below for the design and its limits.

## Technical context

- **Go version**: `1.26.6` (root module); `external/vsphere-policy` module currently targets `go 1.23.0` / toolchain `go1.24.4` — unaffected by this change, no bump needed.
- **API version(s) touched**: `v1alpha6` on `main` (`api/v1alpha6/virtualmachine_types.go` — current storage version). The identical change is cherry-picked onto `release/vc-9.1.0`, where the equivalent edit lands on `api/v1alpha5/virtualmachine_types.go` instead (see `spec.md` "Branch strategy" and this plan's "Branch / backport strategy" below). No change to `virtualmachine_policy_types.go` on either branch — confirmed no `PolicyID` field is needed on `PolicyStatus`.
- **Modules touched**: root module (`controllers/`, `pkg/`, `config/`) and `external/vsphere-policy` (new CRD types + generated deepcopy/manifests).
- **New dependencies**: none.

## Constitution check

| Rule | Status | Notes |
|------|--------|-------|
| API compatibility (additive only) | OK | New CRD kinds; new condition `Reason` string `InfraInMaintenance` (reasons are not enumerated/validated, so this is additive). No field removal/rename/type change; no `PolicyStatus` change. |
| Every new CRD has `+kubebuilder:object:root=true`, `+groupversion`, `+groupName`, deepcopy | OK | Both new types follow `ControlledRebalancingPolicy`'s existing markers verbatim; `make generate-go` regenerates deepcopy. |
| CRD manifests checked in, regenerated via `make generate-manifests` | OK | New files under `config/crd/external-crds/vsphere.policy.vmware.com_*.yaml`. |
| Controllers are thin; no controller calls vSphere APIs directly | OK | New host-maintenance-mode lookup lives in `pkg/providers/vsphere/vcenter/host.go` (existing home for host property-collector helpers), called from `pkg/providers/vsphere/vmlifecycle/update_status.go`; the `policyevaluation` controller's new per-kind functions are pure additions of the existing pattern and touch only the Kubernetes API via the controller-runtime client, same as today. |
| `status.observedGeneration` + `Ready` condition on new CRDs | OK | Inherited from the reused `ComputePolicy`-shaped status type. |
| Fan-out to child objects: patch vs. `CreateOrPatch` | OK / not applicable | No new child-object fan-out is introduced; `PolicyEvaluation` per-VM object creation (`controllerutil.CreateOrPatch`, single-writer per VM) is unchanged, existing code. |
| Testing standards: one `_test.go` per package, Ginkgo `Label()` | OK | New test cases added to existing `policyevaluation_controller_test.go`, `capabilities_test.go`, `crd_test.go`, `update_status_test.go` — no new test files needed since no new packages are introduced. |
| E2E coverage ships with cluster-observable behavior | Deviation, tracked | See "Complexity tracking" below — this SDD pass documents required e2e scenarios and tasks but does not add `test/e2e` code yet, matching PR #1787/#1842's own precedent of no e2e coverage. The implementing PR(s) **must** close this gap; `tasks.md` enumerates the required specs. |
| Import aliases / grouping | OK | No new packages requiring new `importas` aliases; `vspherepolv1`, `pkgcfg`, `pkgctx` etc. already covered. |

## Project structure

```
external/vsphere-policy/api/v1alpha1/
  automatichostevacuationpolicy_types.go   (new)
  besteffortrestartpolicy_types.go         (new)
  zz_generated.deepcopy.go                 (regenerated)

config/crd/external-crds/
  vsphere.policy.vmware.com_automatichostevacuationpolicies.yaml  (new, generated)
  vsphere.policy.vmware.com_besteffortrestartpolicies.yaml        (new, generated)

config/rbac/role.yaml                                             (modified, generated from markers)

pkg/config/config.go                                              (modified: 1 new FeatureStates field —
                                                                     VMEvacuation — gates both new CRDs and
                                                                     the status-condition branch)
pkg/config/capabilities/capabilities.go                           (modified: 1 new capability key +
                                                                     switch case —
                                                                     supports_infrapolicy_vm_evacuation)

pkg/crd/crd.go                                                     (modified: 2 new gated Install cases,
                                                                     both gated on features.VSpherePolicies
                                                                     && features.VMEvacuation)

controllers/vspherepolicy/policyevaluation/policyevaluation_controller.go
                                                                    (restructured: new matchableComputePolicy
                                                                     interface + computePolicyKindDescriptor
                                                                     table; AddToManager,
                                                                     reconcileMandatoryPolicies, and
                                                                     reconcileExplicitPolicies each become
                                                                     one loop over the table via
                                                                     apimeta.ExtractList, replacing the
                                                                     existing ComputePolicy-only
                                                                     reconcileMandatoryComputePolicies/
                                                                     addComputePolicy/addComputePolicyRef/
                                                                     computePolicyToPolicyEvaluationMapperFn
                                                                     and adding table entries — not new
                                                                     per-kind functions — for the two new
                                                                     kinds; see "Controller / webhook
                                                                     impact")

external/vsphere-policy/api/v1alpha1/*_types.go                    (modified: ComputePolicy and both new
                                                                     types each get 3 small accessor
                                                                     methods — GetMatchSpec,
                                                                     GetEnforcementMode, GetPolicyTags —
                                                                     satisfying matchableComputePolicy, alongside
                                                                     their existing GetConditions/
                                                                     SetConditions methods)

api/v1alpha6/virtualmachine_types.go                               (modified: doc-comment update to
                                                                     "Valid policy types are: ..." — adds
                                                                     BestEffortRestartPolicy only, per
                                                                     model.md. Re-applied by hand to
                                                                     api/v1alpha5/virtualmachine_types.go
                                                                     on release/vc-9.1.0 during backport —
                                                                     see "Branch / backport strategy")

pkg/providers/vsphere/vcenter/host.go                              (modified: new helper resolving a
                                                                     host's maintenance-mode state, e.g.
                                                                     GetHostMaintenanceState)
pkg/providers/vsphere/vmlifecycle/update_status.go                 (modified: reconcileStatusPowerState
                                                                     gains the new not-synced branch)

test/builder/fake.go                                                (modified: register both new types
                                                                     in KnownObjectTypes)

test/e2e/vmservice/...                                              (new — see Test strategy; exact path
                                                                     TBD at implementation time, likely a
                                                                     new file alongside
                                                                     virtualmachinelcm.go's policy-related
                                                                     coverage)
```

## API / CRD strategy

- Purely additive: two new CRD kinds in an existing, already-versioned external API group (`vsphere.policy.vmware.com/v1alpha1`). No conversion webhook exists or is needed for this group (single version).
- On the `vmoperator.vmware.com` side, no new fields on `VirtualMachineSpec`/`VirtualMachineStatus` are required — `PolicySpec`/`PolicyStatus`/`Policies`/`Conditions` are reused as-is. Confirmed no `PolicyID` field is needed on `PolicyStatus`.
- CEL vs. Go validation: none needed. Both new CRDs reuse `ComputePolicySpec`'s existing validation markers verbatim (`+kubebuilder:validation:Enum` on `PolicyEnforcementMode`, etc.) — no new cross-field or vSphere-data-dependent validation is introduced.
- `Kind` is an opaque string in `PolicySpec`/`LocalObjectRef`, so adding new valid values requires no `zz_generated.conversion.go` changes on either branch (see `research.md`).

## Controller / webhook impact

- **`controllers/vspherepolicy/policyevaluation`**: restructured into a table-driven registry, not just extended. On `main` today, `ComputePolicy` is the only kind wired in, via `ComputePolicy`-specific `addComputePolicy`/`addComputePolicyRef`/`matchesPolicy`. Rather than repeat that per-kind shape twice more (or the PR #1842 variant of it, which still hand-adds a `Watches(...)`/mandatory-func/mapper/switch-case per kind), this feature introduces:
  - **`matchableComputePolicy` interface**, embedding `ctrlclient.Object` (for `GetName()`/`GetGeneration()` for free) plus `GetMatchSpec() *vspherepolv1.MatchSpec`, `GetEnforcementMode() vspherepolv1.PolicyEnforcementMode`, `GetPolicyTags() []string`. Implemented by `ComputePolicy`, `AutomaticHostEvacuationPolicy`, and `BestEffortRestartPolicy` — three trivial methods each, living next to their existing `GetConditions`/`SetConditions` methods in `external/vsphere-policy/api/v1alpha1/*_types.go`.
  - **`computePolicyKindDescriptor` table**: one entry per kind (`kind` string constant, `newObj func() matchableComputePolicy`, `newList func() ctrlclient.ObjectList`, and the feature-gate check for that kind). `ComputePolicy`'s existing entry has no gate (always on, same as today); the two new kinds gate on `Features.VMEvacuation`.
  - **`apimeta.ExtractList`** (`k8s.io/apimachinery/pkg/api/meta`) is the mechanism that makes the `client.List` step kind-agnostic — it extracts `[]runtime.Object` from any concrete `ctrlclient.ObjectList` via the standard k8s reflection-based accessor (the same one informers/generic clients use), which is what removes the need for a hand-written `reconcileMandatory<Kind>Policies` per kind.
  - `AddToManager`, `reconcileMandatoryPolicies` (replacing `reconcileMandatoryComputePolicies`), and `reconcileExplicitPolicies` (replacing its kind switch) each become one loop over `computePolicyKinds` instead of one function/case per kind. `matchesPolicy`/`evaluateMatchSpec` already only need a `*MatchSpec` (no change needed there), and the existing `addPolicyResult`/`resolvePolicyTags`-shaped append logic collapses into one generic helper operating on `matchableComputePolicy` directly instead of on extracted scalar params.
  - **Net effect on the stated goal** ("a new CRD comes, it should be just adding to the list of types this controller watches and reacts"): true for the reconciler — a 4th policy kind means one new `computePolicyKindDescriptor` entry (plus implementing `matchableComputePolicy` on its type) and nothing else in this file. **Not fully true end-to-end**: `pkg/crd/crd.go`'s install-gating switch and the `+kubebuilder:rbac` markers are still per-kind by nature (static markers processed by `controller-gen`, no dynamic registration hook exists for either) — those still need one new `case`/marker line per kind, same as today.
  - This folds `ComputePolicy` into the same table rather than leaving it as a special case, so this PR touches `reconcileMandatoryComputePolicies`/`addComputePolicyRef`/`addComputePolicy`/`computePolicyToPolicyEvaluationMapperFn` (removing them in favor of the generic loop) in addition to adding the two new kinds. `ControlledRebalancingPolicy` remains CRD-install-gated only (unchanged, out of scope) since it still isn't wired into this reconciler on `main`.
  - No extension hook is added to `matchableComputePolicy` speculatively. If a kind later needs behavior beyond match+tag, add a narrow optional interface checked via a type assertion in the generic loop (e.g. `if h, ok := pol.(postMatchHook); ok { ... }`, mirroring `io.ReaderFrom`/`http.Flusher`-style optional interfaces) rather than growing `matchableComputePolicy` or the descriptor table's shape.
  - Both new kinds' watches/table entries gate on the same single `Features.VMEvacuation` flag (checked inside each descriptor's gate function, evaluated once when building the table/watch list) — matching `ControlledRebalancingPolicy`'s existing CRD-gating behavior, except both new kinds share one flag instead of each having its own.
- **No new controller**. No new webhook.
- **`pkg/providers/vsphere/vmlifecycle` (VM update/status pipeline)**: `reconcileStatusPowerState` gains a host lookup in its not-synced branch, itself gated behind `pkgcfg.Features.VMEvacuation` (checked first, before doing any host lookup — when off, behavior is byte-for-byte identical to today). This is the one place in the entire feature where new non-CRD-plumbing logic is added. It needs:
  - The VM's current host `ManagedObjectReference` — already available via `vmCtx.MoVM.Runtime.Host` (properties already collected for the VM per the existing "fetch properties" step in the VM update reconcile order).
  - A new property-collector call against that `HostSystem` for `runtime.inMaintenanceMode` and `recentTask`, then resolving `recentTask` entries to `TaskInfo` to check for an active `EnterMaintenanceMode_Task`/`ExitMaintenanceMode_Task`. This is new work (see `research.md` — no existing helper does this), added to `pkg/providers/vsphere/vcenter/host.go` following the existing `GetESXHostFQDN` pattern (`object.NewHostSystem(vimClient, hostMoRef)` + a targeted `Properties` call).
  - Per `operator-best-practices.md`, this new vCenter call must be wrapped with `pkgctx.WithVCOpID` inside the provider-layer helper, not in the controller.
  - This lookup fires **only** when both `Features.VMEvacuation` is on and `status.powerState != spec.powerState` (i.e. only in the already-existing not-synced branch, never on the happy path), so no additional vCenter round-trip is added to the common case where power state is already synced or the feature is disabled.
- **New RBAC**: `get;list;watch` on `automatichostevacuationpolicies`/`besteffortrestartpolicies`, `get` on their `/status` subresources — added via `+kubebuilder:rbac` markers on the `policyevaluation` reconciler, regenerated into `config/rbac/role.yaml`. No new RBAC is needed for the host-maintenance-mode lookup — `HostSystem` property reads go through the existing vCenter service-account session, not the Kubernetes RBAC surface.
- **New feature flag**: a single `pkgcfg.Features.VMEvacuation` (capability key `supports_infrapolicy_vm_evacuation`), default `false`, gating everything in this feature — both new CRDs' install/`PolicyEvaluation` wiring **and** the `reconcileStatusPowerState` `InfraInMaintenance` branch. **Superseding an earlier plan decision in this session**, which proposed three independent flags/capabilities (one per CRD, plus a separate one for the status condition) so a Supervisor could roll each out independently. The requester has since simplified this to one capability for the whole feature — enabling it turns on both CRDs and the status signal together; there is no way to activate one without the other.

## Test strategy

- **Unit** (`testlabels.Controller`, table-driven):
  - `controllers/vspherepolicy/policyevaluation/policyevaluation_controller_test.go`: mandatory-match, feature-disabled (mandatory and explicit-ref variants), mixed-kind matching (including all three kinds matching the same VM at once, since they now share one loop), tag resolution via `TagPolicy`, explicit-ref-does-not-match error path, explicit-ref-not-found error path — same matrix as today, parameterized by `computePolicyKindDescriptor` entry instead of copy-pasted per kind. Plus a regression check that `ComputePolicy` behavior is byte-for-byte unchanged after folding it into the registry.
  - `pkg/config/capabilities/capabilities_test.go`: `UpdateCapabilitiesFeatures`/`WouldUpdateCapabilitiesFeatures` cases for the single new capability key, following the existing table shape.
  - `pkg/crd/crd_test.go`: gating matrix (`VSpherePolicies` on/off × `VMEvacuation` on/off) mirroring the existing `ControlledRebalancingPolicy` `When(...)` blocks — both new CRDs install/don't install together since they share one flag.
  - `pkg/providers/vsphere/vmlifecycle/update_status_test.go` (existing file, new `Context`s): host in maintenance mode / host has active Enter or Exit task / host not in maintenance mode, crossed with powerState synced/not-synced, verifying the `Reason` string and that the message contains no host-identifying text.
- **Integration** (`testlabels.EnvTest`/`testlabels.VCSim`): none anticipated beyond what unit tests with the fake client already cover, since no new controller or watch topology beyond the existing `policyevaluation` controller is introduced. If the host-property-collector helper needs live vCenter semantics validated (e.g. confirming `EnterMaintenanceMode_Task`'s exact `descriptionId`), a `vcsim`-labeled test in `pkg/providers/vsphere/vcenter` is the right place — flagged as a task, not assumed necessary until the helper is implemented.
- **E2E** (mandatory per `e2e-sync-with-changes.md` — cluster-observable behavior): **not written in this SDD pass** (per the requester's explicit choice this session — plan/tasks only, no test files yet). Required scenarios to cover in the implementing PR, under `test/e2e/vmservice/`:
  1. Applying a Mandatory `AutomaticHostEvacuationPolicy` results in the expected vSphere tag on a matching VM and the policy appearing in `status.policies`.
  2. Explicitly referencing an Optional `BestEffortRestartPolicy` on a VM's `spec.policies` tags the VM when it matches, and surfaces an error condition/event when it does not.
  3. A VM whose host is placed into maintenance mode (via vcsim or a real host, per whatever the suite's existing host-state test helpers support) shows `VirtualMachinePowerStateSynced=False` with the new reason while `spec.powerState=PoweredOn` and `status.powerState=PoweredOff`; the condition reverts once the host exits maintenance mode and the VM powers back on.
  4. Both CRDs are absent from the cluster when the shared capability/feature flag is off, present when on (mirrors the existing `ControlledRebalancingPolicy` CRD-gating e2e gap — note PR #1787/#1842 didn't add e2e for this either; this plan does not treat that as a blocker inherited from prior art, but flags it as good hygiene to add now while touching the same code).

## Branch / backport strategy

- **`main` (this PR)**: the full change against `v1alpha6`, per "Project structure" above.
- **`release/vc-9.1.0` (follow-up PR)**: cherry-pick the same commit(s). Verified directly against that branch:
  - It has no `api/v1alpha6` — `v1alpha5` is its storage version.
  - It already has the base `ComputePolicy`/`PolicyEvaluation` framework (`controllers/vspherepolicy/policyevaluation`, `pkg/config/capabilities` with `CapabilityKeyVSpherePolicies`, etc.) this feature extends — it is only missing `ControlledRebalancingPolicy`, which is irrelevant here.
  - Every touched file except one cherry-picks with no manual version edits, because `vmopv1` is a per-branch import alias resolved to that branch's own storage version — the diffs to `pkg/providers/vsphere/vmlifecycle/update_status.go`, `pkg/providers/vsphere/vcenter/host.go`, the `policyevaluation` controller, `pkg/config/*`, and `pkg/crd/crd.go` never spell out a version string.
  - The one exception: the `VirtualMachineSpec.Policies` "Valid policy types" doc-comment edit targets `api/v1alpha6/virtualmachine_types.go` on `main`, a path that doesn't exist on `release/vc-9.1.0`. The cherry-pick will conflict/no-op on that hunk; it must be re-applied by hand to `api/v1alpha5/virtualmachine_types.go` on that branch.
  - `external/vsphere-policy` is a single-version (`v1alpha1`) module consumed identically by both branches' `go.mod` replace directives — no version-skew handling needed there beyond making sure `release/vc-9.1.0`'s pinned version of that module (or in-tree copy, however that branch vendors it) picks up the two new CRD types.
- This backport is tracked as `tasks.md` Phase 6, not a separate spec — see `spec.md` "Branch strategy".

## Rollout / migration

- The single feature flag `VMEvacuation` defaults `false`; activated per-Supervisor via the one capability key `supports_infrapolicy_vm_evacuation`, identically in mechanism to `ControlledRebalancingPolicy`, on both branches. There is no independent rollout of the CRDs vs. the status signal — they ship and activate together.
- No backfill/schema upgrade needed — both CRDs are new, and no existing VM has ever had a policy of these kinds, on either branch.
- Partner comms: VKS needs to know (a) both CRDs and (b) the new `InfraInMaintenance`-style condition reason are available in `release/vc-9.1.0`'s `v1alpha5` for 9.1.2 once the backport lands, and (c) that suppressing their own health-check remediation during the signaled window is their responsibility, not automatic.
- Release note: should mention the two new CRD kinds and the new status-condition reason, flagged as a capability-gated Supervisor feature (not directly user-enablable). File on both the `main` PR and the `release/vc-9.1.0` backport PR per that branch's own release-note process.

## Complexity tracking

| Violation | Why needed | Simpler alternative rejected because |
|-----------|------------|--------------------------------------|
| E2E coverage not added in this change set, despite `e2e-sync-with-changes.md` requiring it ship with cluster-observable behavior | This is an SDD-artifacts-only pass by explicit user request this session; no product code exists yet for e2e tests to exercise | Writing e2e test skeletons against non-existent CRDs/controller code would either not compile or be pure placeholder Ginkgo blocks with no assertions — `tasks.md` instead enumerates the exact scenarios so the implementing PR cannot skip them. This deviation must be closed in the same PR(s) that add the CRDs and controller wiring — see `e2e-sync-with-changes.md`'s "do not land product-only changes that clearly require E2E updates without adjusting tests in the same effort" rule. |
