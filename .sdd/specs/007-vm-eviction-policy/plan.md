# Implementation Plan: VM Eviction Compute Policies

- **Spec**: [`spec.md`](./spec.md)
- **Epic**: vmop-4057, vmop-4058, vmop-4059
- **Date**: 2026-08-25

## Summary

Add two new `vsphere.policy.vmware.com/v1alpha1` compute-policy CRDs (`AutomaticVMEvictionPolicy`, `BestEffortRestartPolicy`), cloning `ControlledRebalancingPolicy`'s CRD *type* shape (PR #1787) for the schema, and add one new `Reason` to the existing `VirtualMachinePowerStateSynced` condition on `VirtualMachine` (`api/v1alpha6` on `main`; `api/v1alpha5` on `release/vc-9.1.0`) so tenants and VKS can tell a maintenance-mode-driven power-off apart from any other unsynced power state. VM Operator does not gain any new evacuation, restart, or power-management logic — it only exposes the policy CRDs into the existing tag-application pipeline and computes one explanatory status field.

**Reconciler wiring follows PR #1842's per-kind-function precedent for `List`/`Get`, converging on a shared interface for match-evaluation and result-recording.** `policyevaluation_controller.go` keeps one explicit, concrete function per kind at each of the three places `ComputePolicy` already has one — `reconcileMandatory<Kind>Policies`, `add<Kind>PolicyRef`, and a gated `Watches(...)` entry in `AddToManager` — so listing/fetching a policy kind, and dispatching on `ref.Kind` in `reconcileExplicitPolicies`, stays explicit and readable per kind. What's shared across kinds is the kind-agnostic matching/recording logic: `matchesPolicy`/`evaluateMatchSpec` (operate on a `*MatchSpec` alone, already kind-agnostic), a `matchablePolicy` interface (`ctrlclient.Object` plus `GetPolicyEnforcementMode()`, `GetPolicyMatch()`, `GetPolicyTagNames()`) implemented directly by `ComputePolicy`, `AutomaticVMEvictionPolicy`, and `BestEffortRestartPolicy` in `external/vsphere-policy/api/v1alpha1`, and shared `addPolicyResult`/`resolvePolicyTags`/`addExplicitPolicyMatch`/`processMandatoryPolicies` helpers that operate on it (taking the caller's `kind` string as an explicit parameter, since the interface itself carries no kind identity) so the dedup/tag-lookup/append logic is written once. A generic `toMatchablePolicies[T, PT]` helper converts each kind's `List().Items` into `[]matchablePolicy` by taking the address of each item, rather than copying fields into an intermediate struct. See "Controller / webhook impact" below for the full design.

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
| Controllers are thin; no controller calls vSphere APIs directly | OK | No new vCenter call is introduced by this feature at all — the infra-maintenance signal is derived from the `TaskInfo` of a power-op call the provider layer (`pkg/providers/vsphere/vmprovider_vm.go`, `pkg/util/vsphere/vm/power_state.go`) already makes; `isInfraMaintenanceFault` (`pkg/util/vsphere/task/task.go`) is a pure function over that existing data. The `policyevaluation` controller's new per-kind functions are pure additions of the existing pattern and touch only the Kubernetes API via the controller-runtime client, same as today. |
| `status.observedGeneration` + `Ready` condition on new CRDs | OK | Inherited from the reused `ComputePolicy`-shaped status type. |
| Fan-out to child objects: patch vs. `CreateOrPatch` | OK / not applicable | No new child-object fan-out is introduced; `PolicyEvaluation` per-VM object creation (`controllerutil.CreateOrPatch`, single-writer per VM) is unchanged, existing code. |
| Testing standards: one `_test.go` per package, Ginkgo `Label()` | OK | New test cases added to existing `policyevaluation_controller_test.go`, `capabilities_test.go`, `crd_test.go`, `pkg/util/vsphere/task/task_test.go`, `pkg/util/vsphere/vm/power_state_test.go`, and `pkg/providers/vsphere/vmprovider_vm_power_test.go` — no new test files needed since no new packages are introduced. |
| E2E coverage ships with cluster-observable behavior | Partially done, tracked | Scenario 1 (Mandatory `AutomaticVMEvictionPolicy` tagging + match-widening re-evaluation) and scenario 2 (explicit `BestEffortRestartPolicy` reference, plus both-Mandatory and both-Optional-explicit combinations of the two kinds) shipped in `test/e2e/vmservice/vmservice/computepolicies/vmevictionpolicy.go`, wired into `vmservice_test.go`. Scenarios 3–4 (`InfraInMaintenance` condition, CRD-presence gating) remain open — see "Test strategy" and `tasks.md` T029/T030. |
| Import aliases / grouping | OK | No new packages requiring new `importas` aliases; `vspherepolv1`, `pkgcfg`, `pkgctx` etc. already covered. |

## Project structure

```
external/vsphere-policy/api/v1alpha1/
  automaticvmevictionpolicy_types.go   (new)
  besteffortrestartpolicy_types.go         (new)
  zz_generated.deepcopy.go                 (regenerated)

config/crd/external-crds/
  vsphere.policy.vmware.com_automaticvmevictionpolicies.yaml      (new, generated)
  vsphere.policy.vmware.com_besteffortrestartpolicies.yaml        (new, generated)

config/rbac/role.yaml                                             (modified, generated from markers)

pkg/config/config.go                                              (modified: 1 new FeatureStates field —
                                                                     VMEviction — gates both new CRDs and
                                                                     the status-condition branch)
pkg/config/capabilities/capabilities.go                           (modified: 1 new capability key +
                                                                     switch case —
                                                                     supports_infrapolicy_vm_evacuation)

pkg/crd/crd.go                                                     (modified: 2 new gated Install cases,
                                                                     both gated on features.VSpherePolicies
                                                                     && features.VMEviction)

controllers/vspherepolicy/policyevaluation/policyevaluation_controller.go
                                                                    (modified: one explicit
                                                                     reconcileMandatory<Kind>Policies +
                                                                     add<Kind>PolicyRef function per new
                                                                     kind, plus one explicit gated Watches(...)
                                                                     each in AddToManager — mirroring the
                                                                     existing ComputePolicy shape and #1842's
                                                                     ControlledRebalancingPolicy precedent.
                                                                     computePolicyToPolicyEvaluationMapperFn
                                                                     renamed to policyToPolicyEvaluationMapperFn
                                                                     since it is now reused, unmodified, across
                                                                     all three kinds' watches. matchablePolicy
                                                                     is an interface (ctrlclient.Object plus
                                                                     GetPolicyEnforcementMode()/GetPolicyMatch()/
                                                                     GetPolicyTagNames()) implemented directly
                                                                     by the three CRD types; a generic
                                                                     toMatchablePolicies[T, PT] helper converts
                                                                     each kind's List().Items into
                                                                     []matchablePolicy by address, and shared
                                                                     addPolicyResult/resolvePolicyTags/
                                                                     addExplicitPolicyMatch helpers operate on
                                                                     the interface (kind passed as an explicit
                                                                     parameter) so dedup/tag-lookup/append is
                                                                     written once; see "Controller / webhook
                                                                     impact")

external/vsphere-policy/api/v1alpha1/*_types.go                     (modified: ComputePolicy,
                                                                     AutomaticVMEvictionPolicy, and
                                                                     BestEffortRestartPolicy each implement
                                                                     matchablePolicy directly —
                                                                     GetPolicyEnforcementMode()/GetPolicyMatch()/
                                                                     GetPolicyTagNames() — alongside their
                                                                     existing GetConditions/SetConditions
                                                                     methods, so the controller shares match-
                                                                     evaluation/result-recording logic without
                                                                     copying fields into an intermediate struct)

api/v1alpha6/virtualmachine_types.go                               (modified: doc-comment update to
                                                                     "Valid policy types are: ..." — adds
                                                                     AutomaticVMEvictionPolicy and
                                                                     BestEffortRestartPolicy, per model.md.
                                                                     Re-applied by hand to
                                                                     api/v1alpha5/virtualmachine_types.go
                                                                     on release/vc-9.1.0 during backport —
                                                                     see "Branch / backport strategy")

pkg/util/vsphere/task/task.go                                       (modified: new pure helper
                                                                     isInfraMaintenanceFault(ti
                                                                     *vimtypes.TaskInfo) bool, matching
                                                                     a *vimtypes.NoCompatibleHost fault
                                                                     carrying the
                                                                     com.vmware.cp.autoevac.HostInMaintenanceMode
                                                                     / ...RestartOnCurrentHostRequired
                                                                     fault-message keys)
pkg/util/vsphere/vm/power_state.go                                  (modified: failure path wraps its
                                                                     returned error with a new sentinel,
                                                                     ErrInfraMaintenanceFault, via %w,
                                                                     when isInfraMaintenanceFault(ti) is
                                                                     true for the failed task)
pkg/providers/vsphere/vmprovider_vm.go                              (modified: reconcilePowerState sets
                                                                     the VirtualMachinePowerStateSynced
                                                                     condition itself — no-op/success ->
                                                                     Synced, errors.Is(err,
                                                                     ErrInfraMaintenanceFault) with
                                                                     Features.VMEviction on ->
                                                                     InfraInMaintenance, any other
                                                                     failure -> NotSynced; gated on
                                                                     Features.VMEviction)
pkg/providers/vsphere/vmlifecycle/update_status.go                  (modified: reconcileStatusPowerState
                                                                     keeps setting Status.PowerState only
                                                                     — its VirtualMachinePowerStateSynced
                                                                     condition logic is removed, moved to
                                                                     reconcilePowerState above)

test/builder/fake.go                                                (modified: register both new types
                                                                     in KnownObjectTypes)

test/e2e/vmservice/vmservice/computepolicies/vmevictionpolicy.go   (new — see Test strategy item 1;
                                                                     package computepolicies, new home for
                                                                     compute-policy-CRD e2e coverage since
                                                                     none existed before this feature —
                                                                     ComputePolicy itself is otherwise only
                                                                     touched inline in
                                                                     virtualmachinelcm.go's WCP-admin-API
                                                                     mirroring, not a standalone suite)
```

## API / CRD strategy

- Purely additive: two new CRD kinds in an existing, already-versioned external API group (`vsphere.policy.vmware.com/v1alpha1`). No conversion webhook exists or is needed for this group (single version).
- On the `vmoperator.vmware.com` side, no new fields on `VirtualMachineSpec`/`VirtualMachineStatus` are required — `PolicySpec`/`PolicyStatus`/`Policies`/`Conditions` are reused as-is. Confirmed no `PolicyID` field is needed on `PolicyStatus`.
- CEL vs. Go validation: none needed. Both new CRDs reuse `ComputePolicySpec`'s existing validation markers verbatim (`+kubebuilder:validation:Enum` on `PolicyEnforcementMode`, etc.) — no new cross-field or vSphere-data-dependent validation is introduced.
- `Kind` is an opaque string in `PolicySpec`/`LocalObjectRef`, so adding new valid values requires no `zz_generated.conversion.go` changes on either branch (see `research.md`).

## Controller / webhook impact

- **`controllers/vspherepolicy/policyevaluation`**: extended by repeating the existing per-kind `List`/`Get` shape twice more, following #1842's `ControlledRebalancingPolicy` precedent for the explicit-per-kind fetch, while converging the match-evaluation/result-recording logic on a shared interface. `ComputePolicy`, `AutomaticVMEvictionPolicy`, and `BestEffortRestartPolicy` each keep their own concrete `reconcileMandatory<Kind>Policies`/`add<Kind>PolicyRef` functions for `List`/`Get`, and each CRD type implements the `matchablePolicy` interface directly:
  - **`matchablePolicy`**: an interface (`ctrlclient.Object` plus `GetPolicyEnforcementMode() vspherepolv1.PolicyEnforcementMode`, `GetPolicyMatch() *vspherepolv1.MatchSpec`, `GetPolicyTagNames() []string`), implemented by `*ComputePolicy`, `*AutomaticVMEvictionPolicy`, and `*BestEffortRestartPolicy` in `external/vsphere-policy/api/v1alpha1`. It carries no `Kind`-identifying method; the caller's `kind` string (e.g. `computePolicyKind`) is passed explicitly to the shared helpers below instead.
  - **`toMatchablePolicies[T any, PT interface{ *T; matchablePolicy }](items []T) []matchablePolicy`**: a generic helper each `reconcileMandatory<Kind>Policies` calls on its `List().Items`, taking the address of each item in place (`PT(&items[i])`) rather than copying fields into an intermediate struct.
  - **`reconcileMandatoryComputePolicies`/`reconcileMandatoryAutomaticVMEvictionPolicies`/`reconcileMandatoryBestEffortRestartPolicies`**: each does its own explicit `client.List` for its concrete `*List` type, converts `list.Items` via `toMatchablePolicies`, and hands the result plus its `kind` constant to a shared `processMandatoryPolicies` helper that filters to `PolicyEnforcementModeMandatory`, calls the already-kind-agnostic `matchesPolicy`/`evaluateMatchSpec` (unchanged — they only ever took a `*MatchSpec`), and records a match via `addPolicyResult`.
  - **`addComputePolicyRef`/`addAutomaticVMEvictionPolicyRef`/`addBestEffortRestartPolicyRef`**: each does its own explicit `client.Get` for its concrete type into a local variable, then calls shared `addExplicitPolicyMatch` (match-or-error) with its `kind` constant and `&pol` → `addPolicyResult`.
  - **`addPolicyResult`/`resolvePolicyTags`/`addExplicitPolicyMatch`/`processMandatoryPolicies`**: take a `kind string` parameter alongside the `matchablePolicy` value, reading `p.GetName()`/`p.GetGeneration()` via the embedded `ctrlclient.Object`/`metav1.Object` accessors and `p.GetPolicyMatch()`/`GetPolicyEnforcementMode()`/`GetPolicyTagNames()` via the custom interface methods — no field-copying struct needed.
  - **`AddToManager`**: one new explicit, gated `Watches(...)` per new kind (both behind `pkgcfg.FromContext(ctx).Features.VMEviction`), reusing the existing mapper function — renamed `computePolicyToPolicyEvaluationMapperFn` → `policyToPolicyEvaluationMapperFn` since its body was already kind-agnostic and is now shared by all three kinds' watches unmodified.
  - **Net effect**: adding a policy kind to this reconciler means one new `reconcileMandatory<Kind>Policies` function, one new `add<Kind>PolicyRef` function, one new gated `Watches(...)` line, and implementing `matchablePolicy` on the new CRD type — the same granularity as `pkg/crd/crd.go`'s install-gating switch and the `+kubebuilder:rbac` markers. What's shared, and grows by zero code per new kind: `matchesPolicy`/`evaluateMatchSpec`, `toMatchablePolicies`, `addPolicyResult`, `resolvePolicyTags`, `addExplicitPolicyMatch`, `processMandatoryPolicies`, and the mapper function.
  - `ComputePolicy`'s existing behavior is unchanged (confirmed via the existing unit test suite passing unmodified), though it now also implements `matchablePolicy` alongside `AutomaticVMEvictionPolicy`/`BestEffortRestartPolicy` so all three kinds share the same helpers. `ControlledRebalancingPolicy` remains CRD-install-gated only, unchanged, since it isn't wired into this reconciler on `main`.
  - Both new kinds' watches gate on the same single `Features.VMEviction` flag, checked once via `pkgcfg.FromContext(ctx).Features.VMEviction` in `AddToManager` and again per-reconcile in `reconcileMandatoryPolicies`/`reconcileExplicitPolicies` — matching `ControlledRebalancingPolicy`'s existing CRD-gating behavior, except both new kinds share one flag instead of each having its own.
- **No new controller**. No new webhook.
- **`pkg/providers/vsphere/vmprovider_vm.go` / `pkg/util/vsphere/vm/power_state.go` / `pkg/util/vsphere/task/task.go` (power-state reconcile step)**: there is no host lookup at all. The signal comes from the VM's own power-op task failing with a specific vCenter fault, observed at the point VM Operator itself attempts to converge power state — no new vCenter round-trip is added anywhere.
  - `reconcileStatusPowerState` (`pkg/providers/vsphere/vmlifecycle/update_status.go`) keeps setting `vmCtx.VM.Status.PowerState` exactly as it does today; its `VirtualMachinePowerStateSynced` condition logic is removed from this function.
  - The condition instead moves to `reconcilePowerState` (`pkg/providers/vsphere/vmprovider_vm.go`, the "Reconcile power state" step, called unconditionally every reconcile, including the case where power state already matches and no task is issued). This function already has `vmCtx.VM` in scope, so no new plumbing is needed to reach `conditions.MarkTrue`/`MarkFalse`. It runs *after* `reconcileStatusPowerState` in the same pass (per the documented reconcile order), so this is also a correctness fix: the condition now reflects the actual outcome of this reconcile's power-op attempt, not a pre-op snapshot computed earlier in the same pass.
  - A new pure helper, `isInfraMaintenanceFault(ti *vimtypes.TaskInfo) bool` (`pkg/util/vsphere/task/task.go`, alongside the existing `ErrorMessageFromTaskInfo`), matches a `*vimtypes.NoCompatibleHost` fault whose nested fault messages carry the key `com.vmware.cp.autoevac.HostInMaintenanceMode` or `com.vmware.cp.autoevac.RestartOnCurrentHostRequired`.
  - `pkg/util/vsphere/vm/power_state.go`'s failure path (`setPowerState`/`doAndWaitOnHardPowerOp`) already has the failed task's `*vimtypes.TaskInfo` in hand but today collapses it into a plain formatted error string before returning. It's changed to wrap the returned error with a new sentinel, `vmutil.ErrInfraMaintenanceFault` (via `%w`), when `isInfraMaintenanceFault(ti)` is true — following the existing `pkgerr` sentinel-error convention (`RequeueError`/`NoRequeueError`), which propagates via `errors.Is`/`errors.As` from any call-stack depth, rather than adding new return values/params to thread the raw `TaskInfo` up through `resources/vm.go` and `vmutil.SetAndWaitOnPowerState`.
  - `reconcilePowerState` checks `errors.Is(err, vmutil.ErrInfraMaintenanceFault)` after the power-op call and sets `VirtualMachinePowerStateSynced` itself, gated on `Features.VMEviction` checked first (when off, behavior — including the `NotSynced` reason on this same fault — is byte-for-byte identical to today): no-op or success -> `Synced`; this fault with the feature on -> `InfraInMaintenance`; any other failure, or this fault with the feature off -> `NotSynced`.
  - The `Reason` string stays the policy-agnostic `InfraInMaintenance`, not named after either CRD (`AutomaticVMEvictionPolicy`/`BestEffortRestartPolicy`): it describes the vCenter-side autoevac fault observed, not which CRD authorized DRS's action, and downstream consumers (VKS) only need "this is infra maintenance," not which policy kind was involved.
- **New RBAC**: `get;list;watch` on `automaticvmevictionpolicies`/`besteffortrestartpolicies`, `get` on their `/status` subresources — added via `+kubebuilder:rbac` markers on the `policyevaluation` reconciler, regenerated into `config/rbac/role.yaml`. No new RBAC is needed for the `InfraInMaintenance` condition logic — it inspects the `TaskInfo` of a power-op call already made through the existing vCenter service-account session, not the Kubernetes RBAC surface.
- **New feature flag**: a single `pkgcfg.Features.VMEviction` (capability key `supports_infrapolicy_vm_evacuation`), default `false`, gating everything in this feature — both new CRDs' install/`PolicyEvaluation` wiring **and** the `InfraInMaintenance` branch in `reconcilePowerState`. Enabling it turns on both CRDs and the status signal together; there is no way to activate one without the other.

## Test strategy

- **Unit** (`testlabels.Controller`, table-driven):
  - `controllers/vspherepolicy/policyevaluation/policyevaluation_controller_test.go`: mandatory-match, feature-disabled (mandatory and explicit-ref variants), mixed-kind matching (a VM matching more than one kind at once, since each kind runs through the same shared `addPolicyResult`/`processMandatoryPolicies` helpers), tag resolution via `TagPolicy`, explicit-ref-does-not-match error path, explicit-ref-not-found error path — same matrix as `ComputePolicy`'s existing tests, duplicated per new kind. Plus a regression check that `ComputePolicy`'s own behavior is byte-for-byte unchanged (confirmed: its existing tests pass unmodified).
  - `pkg/config/capabilities/capabilities_test.go`: `UpdateCapabilitiesFeatures`/`WouldUpdateCapabilitiesFeatures` cases for the single new capability key, following the existing table shape.
  - `pkg/crd/crd_test.go`: gating matrix (`VSpherePolicies` on/off × `VMEviction` on/off) mirroring the existing `ControlledRebalancingPolicy` `When(...)` blocks — both new CRDs install/don't install together since they share one flag.
  - `pkg/util/vsphere/task/task_test.go` (existing file, new cases): `isInfraMaintenanceFault` matches a `*vimtypes.NoCompatibleHost` fault carrying either autoevac fault-message key, does not match other faults/fault-message keys, and handles `nil` `TaskInfo`/`Error`/non-`NoCompatibleHost` fault types gracefully (mirroring `ErrorMessageFromTaskInfo`'s existing nil-guards).
  - `pkg/util/vsphere/vm/power_state_test.go` (existing file, new cases): a failed power-op task whose `TaskInfo` matches `isInfraMaintenanceFault` returns an error satisfying `errors.Is(err, ErrInfraMaintenanceFault)`; a failed task that doesn't match returns a plain error not satisfying that check.
  - `pkg/providers/vsphere/vmprovider_vm_power_test.go` (existing file, new `Context`s): `reconcilePowerState` sets `VirtualMachinePowerStateSynced` to `Synced` on no-op/success, `NotSynced` on a generic power-op failure, `NotSynced` on the infra-maintenance fault with `Features.VMEviction` off (unchanged behavior), and `InfraInMaintenance` on the infra-maintenance fault with `Features.VMEviction` on — verifying the `Reason` string and that the message contains no host-identifying text.
- **Integration** (`testlabels.EnvTest`/`testlabels.VCSim`): none anticipated beyond what unit tests with the fake client/fake vCenter task already cover, since no new controller or watch topology is introduced and no new vCenter call is made.
- **E2E** (mandatory per `e2e-sync-with-changes.md` — cluster-observable behavior), status per scenario:
  1. **Done.** Applying a Mandatory `AutomaticVMEvictionPolicy` results in the expected vSphere tag on a matching VM and the policy appearing in `status.policies`, plus scenarios for: a policy `match` widened after a non-matching VM already exists (watch/informer re-evaluation); updating the policy's `spec.tags` to reference a different `TagPolicy` re-tags the VM accordingly; and deleting the policy removes both the VM's vSphere tag and its `status.policies` entry — `test/e2e/vmservice/vmservice/computepolicies/vmevictionpolicy.go`, labeled `experimental`, wired into `test/e2e/vmservice/vmservice_test.go` under `Context("VM-EVICTION-POLICY", ...)`.
  2. **Done.** Explicitly referencing an Optional `BestEffortRestartPolicy` on a VM's `spec.policies` tags the VM when it matches, and surfaces a not-ready `PolicyEvaluation` condition when it does not — `vmevictionpolicy.go`. Plus three combinations of `AutomaticVMEvictionPolicy` and `BestEffortRestartPolicy` in the same file: Mandatory `AutomaticVMEvictionPolicy` + Optional `BestEffortRestartPolicy` (`tasks.md` T022); both Mandatory and matching simultaneously with no explicit references; both Optional and explicitly referenced together (`tasks.md` T040). Deliberately scoped to these two kinds only — `ComputePolicy` already has thorough mandatory/optional combination coverage through the real WCP admin API path in `virtualmachinelcm.go` (`Context("IaaS Policies", ...)`, `Context("PLACEMENT-POLICY-COMBINATIONS", ...)`), so this feature's e2e additions don't duplicate it.
  3. **Not done** (`tasks.md` T029): a VM whose host is placed into maintenance mode (via vcsim or a real host, per whatever mechanism the suite's existing host-state test helpers support to trigger a `NoCompatibleHost`/autoevac-faulted power-op against the VM) shows `VirtualMachinePowerStateSynced=False` with the new reason while `spec.powerState=PoweredOn` and `status.powerState=PoweredOff`; the condition reverts once the host exits maintenance mode and the VM powers back on successfully. Blocked on Phase 5 (`isInfraMaintenanceFault`/`ErrInfraMaintenanceFault`/`InfraInMaintenance` condition work), which is not implemented yet.
  4. **Not done** (`tasks.md` T030): both CRDs are absent from the cluster when the shared capability/feature flag is off, present when on (mirrors the existing `ControlledRebalancingPolicy` CRD-gating e2e gap — note PR #1787/#1842 didn't add e2e for this either; this plan does not treat that as a blocker inherited from prior art, but flags it as good hygiene to add now while touching the same code).

## Branch / backport strategy

- **`main` (this PR)**: the full change against `v1alpha6`, per "Project structure" above.
- **`release/vc-9.1.0` (follow-up PR)**: cherry-pick the same commit(s). Verified directly against that branch:
  - It has no `api/v1alpha6` — `v1alpha5` is its storage version.
  - It already has the base `ComputePolicy`/`PolicyEvaluation` framework (`controllers/vspherepolicy/policyevaluation`, `pkg/config/capabilities` with `CapabilityKeyVSpherePolicies`, etc.) this feature extends — it is only missing `ControlledRebalancingPolicy`, which is irrelevant here.
  - Every touched file except one cherry-picks with no manual version edits, because `vmopv1` is a per-branch import alias resolved to that branch's own storage version — the diffs to `pkg/providers/vsphere/vmprovider_vm.go`, `pkg/util/vsphere/vm/power_state.go`, `pkg/util/vsphere/task/task.go`, the `policyevaluation` controller, `pkg/config/*`, and `pkg/crd/crd.go` never spell out a version string.
  - The one exception: the `VirtualMachineSpec.Policies` "Valid policy types" doc-comment edit targets `api/v1alpha6/virtualmachine_types.go` on `main`, a path that doesn't exist on `release/vc-9.1.0`. The cherry-pick will conflict/no-op on that hunk; it must be re-applied by hand to `api/v1alpha5/virtualmachine_types.go` on that branch.
  - `external/vsphere-policy` is a single-version (`v1alpha1`) module consumed identically by both branches' `go.mod` replace directives — no version-skew handling needed there beyond making sure `release/vc-9.1.0`'s pinned version of that module (or in-tree copy, however that branch vendors it) picks up the two new CRD types.
- This backport is tracked as `tasks.md` Phase 6, not a separate spec — see `spec.md` "Branch strategy".

## Rollout / migration

- The single feature flag `VMEviction` defaults `false`; activated per-Supervisor via the one capability key `supports_infrapolicy_vm_evacuation`, identically in mechanism to `ControlledRebalancingPolicy`, on both branches. There is no independent rollout of the CRDs vs. the status signal — they ship and activate together.
- No backfill/schema upgrade needed — both CRDs are new, and no existing VM has ever had a policy of these kinds, on either branch.
- Partner comms: VKS needs to know (a) both CRDs and (b) the new `InfraInMaintenance`-style condition reason are available in `release/vc-9.1.0`'s `v1alpha5` for 9.1.2 once the backport lands, and (c) that suppressing their own health-check remediation during the signaled window is their responsibility, not automatic.
- Release note: should mention the two new CRD kinds and the new status-condition reason, flagged as a capability-gated Supervisor feature (not directly user-enablable). File on both the `main` PR and the `release/vc-9.1.0` backport PR per that branch's own release-note process.

## Complexity tracking

| Violation | Why needed | Simpler alternative rejected because |
|-----------|------------|--------------------------------------|
| Scenarios 3–4 of the required E2E coverage (`e2e-sync-with-changes.md`) are not yet in this change set; scenarios 1 and 2 (`AutomaticVMEvictionPolicy`/`BestEffortRestartPolicy` tagging, plus their combinations) have landed | Scenario 4 (CRD-presence gating) depends only on code that already exists and is tracked as `tasks.md` T030. Scenario 3 (`InfraInMaintenance` condition) additionally depends on Phase 5 product code (`isInfraMaintenanceFault`/`ErrInfraMaintenanceFault`/the condition-reason wiring), which has not been implemented yet, so there is nothing for that scenario's e2e spec to exercise. | Writing an e2e spec for scenario 3 against the not-yet-implemented Phase 5 code would either not compile or be a placeholder Ginkgo block with no real assertions. This deviation must be closed in the same PR(s) that add the Phase 5 product code — see `e2e-sync-with-changes.md`'s "do not land product-only changes that clearly require E2E updates without adjusting tests in the same effort" rule. |
