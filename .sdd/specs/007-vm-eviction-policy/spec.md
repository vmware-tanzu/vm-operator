# Feature Specification: VM Eviction Compute Policies

- **Feature branch**: `feature/host-maintenance-mode-policy`
  - **Fork**: `vmware-tanzu/vm-operator`
  - **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-08-25
- **Status**: Draft
- **Epic**: vmop-4057, vmop-4058, vmop-4059
- **Design docs**: WIKI page 2535776102 — "One pager: VM Service - Support for VM Eviction Compute Policies"; WIKI page 2781824956 — "VM Eviction Compute Policies CRDs"

---

## Background

Host maintenance mode cannot today evacuate VMs with physical PCI passthrough devices or multi-GPU workloads using Unified Virtual Memory (UVM): there is no vMotion story for these devices, and forcibly evacuating the host can corrupt in-flight GPU state rather than just fail to migrate. This has already caused a customer-impacting incident (an 8+ day outage during a VCF upgrade because hosts running PAIS model-endpoint VMs could not enter maintenance mode). Live migration support for these device classes is a long-term hardware-vendor roadmap item; controlled shutdown-and-restart is being adopted instead as a first-class resilience pattern for this workload class, driven by a new DRS capability (see the DRS proposal referenced in WIKI page 2535776102).

DRS will gain the ability to power off VMs that cannot be evacuated from a host entering maintenance mode, when a matching compute policy authorizes it. VM Operator's role is limited to:

1. Exposing the two new compute-policy CRDs so CSP admins and tenants can configure and select this behavior, reusing the existing Infra Policy → tag-application pipeline unchanged.
2. Giving the tenant (and consumers built on top of VM Operator, notably VKS) a status signal that explains why a VM's power state is not currently synced with its spec, when that VM was powered off by DRS for host maintenance rather than through any VM Operator-initiated action.

VM Operator does not decide which VMs get powered off, does not suppress its own convergence of `spec.powerState`, and does not special-case its behavior based on policy presence or host state anywhere except when computing this one status condition.

## Branch strategy

This spec targets `main`, where `v1alpha6` is the current storage version. `release/vc-9.1.0` — the branch VKS's 9.1.2 timeframe actually ships from — has no `v1alpha6`; `v1alpha5` is its storage version there. Confirmed by inspecting that branch directly: it already has the base `ComputePolicy`/`PolicyEvaluation` framework this feature builds on (just not `ControlledRebalancingPolicy`, which doesn't matter here), so it's a valid target for the same change.

The implementation is therefore delivered as two PRs from one design:

1. **`main`**: the full change as described in `plan.md`/`tasks.md`, against `v1alpha6`.
2. **`release/vc-9.1.0`**: a cherry-pick of the same commit(s). Every file in this change except one cherry-picks cleanly with no version-specific edits, because `vmopv1` is a per-branch import alias (`api/v1alpha6` on `main`, `api/v1alpha5` on `release/vc-9.1.0`) — the diff never spells out a version string inside those files. The one exception is the `VirtualMachineSpec.Policies` doc-comment edit, whose target path (`api/v1alpha6/virtualmachine_types.go` on `main`) doesn't exist on `release/vc-9.1.0`; that edit must be re-applied by hand to `api/v1alpha5/virtualmachine_types.go` there instead of relying on the cherry-pick.

## Goals

- MUST introduce two new CRDs in the `vsphere.policy.vmware.com/v1alpha1` API group: `AutomaticVMEvictionPolicy` and `BestEffortRestartPolicy`, each reusing the existing `ComputePolicy`/`ControlledRebalancingPolicy` spec shape (`description`, `policyID`, `enforcementMode`, `match`, `tags`) and status shape (`observedGeneration`, `conditions`) verbatim — no new fields on the policy CRDs themselves.
- MUST extend `controllers/vspherepolicy/policyevaluation` to evaluate both new policy kinds using the exact same mandatory-match and explicit-optional-reference semantics already implemented for `ControlledRebalancingPolicy`, both gated by the single feature flag/capability described below (no per-kind flag).
- MUST allow a matching VM to be tagged for `AutomaticVMEvictionPolicy` and/or `BestEffortRestartPolicy` through the existing `pkg/vmconfig/policy` tag-application reconciler, with zero VM-Operator-specific evacuation or restart logic — the vSphere tag is the only mechanism by which DRS acts.
- MUST allow `AutomaticVMEvictionPolicy` and `BestEffortRestartPolicy` to be referenced explicitly and optionally on a VM via `spec.policies` — both kinds are listed in the `VirtualMachineSpec.Policies` "Valid policy types" doc comment, and `reconcileExplicitPolicies` handles an explicit reference to either kind identically to `ComputePolicy` — so a tenant can opt a workload in or out of the provider's default eviction/restart behavior after a maintenance-mode power-off.
- MUST NOT change how VM Operator reconciles `spec.powerState`: it continues to attempt convergence unconditionally, every reconcile, regardless of policy presence or host state.
- MUST add a new, non-host-identifying explanation to the existing `VirtualMachinePowerStateSynced` condition: when VM Operator's own attempt to converge `spec.powerState` fails because the underlying vSphere power-state-change task returns a `NoCompatibleHost` fault whose nested fault messages carry the key `com.vmware.cp.autoevac.HostInMaintenanceMode` or `com.vmware.cp.autoevac.RestartOnCurrentHostRequired`, set the condition `False` with reason `InfraInMaintenance` instead of the generic `NotSynced` reason. No host state is queried directly — the signal comes from the failed task's own fault, not a separate host-status lookup. The condition returns to its normal computation once a subsequent convergence attempt no longer hits this fault (i.e. the host has exited maintenance mode and the VM powers back on).
- MUST gate the entire feature — both new CRDs and the status-condition behavior — behind a single capability, `supports_infrapolicy_vm_evacuation`. There is no independent per-CRD or per-behavior gating; enabling the capability turns on both CRDs and the new condition reason together.
- MUST ship on `main` against API version `v1alpha6` (the current storage version). The identical change MUST also be cherry-picked onto the `release/vc-9.1.0` branch, where it targets `v1alpha5` instead (the storage version there, and the version VKS consumes for the 9.1.2 timeframe) — same code, applied as two separate version-scoped implementations on two branches. See "Branch strategy" below.
- SHOULD surface both new policy kinds in `VirtualMachine.status.policies` via the existing `PolicyStatus` mechanism, identically to how `ComputePolicy` and `ControlledRebalancingPolicy` results are surfaced today.

## Non-goals

- VM Operator does not decide which VMs cannot be evacuated or take the power-off action — that is DRS's decision and action, driven by the vSphere tag this spec's CRDs produce.
- Suppressing VKS's own health-check-triggered node-replacement remediation during the maintenance-mode power-off window is VKS's responsibility, not VM Operator's. This spec only provides the signal VKS needs to build that behavior.
- Resolving conflicts when a VM matches both a Mandatory `AutomaticVMEvictionPolicy` and an Optional policy (e.g. a namespace-opted-in `BestEffortRestartPolicy`) is out of scope — DRS resolves such conflicts implicitly today (see the DRS proposal). A future spec may introduce explicit conflict prevention.
- No vMotion or live-migration support is added for passthrough/vGPU/UVM devices. This is deliberately not framed as a stopgap for that.
- The `RoCH` policy type mentioned as a future follow-on in WIKI page 2535776102 is out of scope for this spec.
- No changes to `v1alpha4` or earlier API versions on either branch.
- The `release/vc-9.1.0` backport is a mechanical cherry-pick of the same commits, not an independent design — it does not get its own spec/plan/tasks set, only its own task entries in this spec's `tasks.md` (see "Branch strategy" below).

## User stories / acceptance criteria

### CSP admin

- **Given** the `supports_infrapolicy_vm_evacuation` Supervisor capability is activated, **When** the CSP admin creates a Mandatory `AutomaticVMEvictionPolicy` in a namespace referencing a vSphere compute-policy ID of type `automatic_vm_eviction`, **Then** every matching VM in that namespace (per the policy's `match` spec, or all VMs if `match` is unset) is tagged with the policy's vSphere tag, and the policy appears in each matching VM's `status.policies`.
- **Given** the capability is not activated, **When** the controller-manager starts, **Then** neither CRD is installed, neither policy kind is evaluated, and the `VirtualMachinePowerStateSynced` condition never reports the `InfraInMaintenance` reason (mirrors `ControlledRebalancingPolicy`'s existing gating behavior, extended to cover the status condition too).

### Tenant admin / DevOps user

- **Given** a VM's namespace has an Optional `BestEffortRestartPolicy`, **When** the DevOps user references it explicitly in the VM's `spec.policies` (`kind: BestEffortRestartPolicy`), **Then** the VM is tagged accordingly if it matches the policy's `match` spec, and an error is surfaced (mirroring the existing explicit-reference behavior for other policy kinds) if it does not match.
- **Given** a VM was powered off by DRS because its host entered maintenance mode and the VM could not be evacuated, **When** the DevOps user inspects the VM's status, **Then** the `VirtualMachinePowerStateSynced` condition is `False` with a reason that communicates the power-off is due to infrastructure maintenance, without naming or otherwise identifying the specific host (persona/topology-privacy requirement).
- **Given** the host has since exited maintenance mode and the VM has been powered back on, **When** the DevOps user inspects the VM's status, **Then** the `VirtualMachinePowerStateSynced` condition returns to its normal `Synced`/not-synced computation with no trace of the prior infra-maintenance reason.

### Partner engineer (VKS)

- **Given** a VKS node VM is powered off for host maintenance, **When** VKS observes the `VirtualMachinePowerStateSynced=False` condition with the infra-maintenance reason, **Then** VKS has the signal it needs to decide whether to suppress its own health-check-triggered node replacement during the power-off window (VKS's own remediation logic is out of scope here; VM Operator only emits the signal).
- **Given** VKS does not yet consume `v1alpha6` and its 9.1.2 timeframe ships from `release/vc-9.1.0`, **When** VKS creates or reads VirtualMachine objects there, **Then** all of the above is available via that branch's `v1alpha5`, delivered by the cherry-pick described in "Branch strategy" above.

### Platform engineer

- **Given** the `PolicyEvaluation` controller already watches `ComputePolicy`, `ControlledRebalancingPolicy`, and `TagPolicy`, **When** this feature ships, **Then** it additionally watches `AutomaticVMEvictionPolicy` and `BestEffortRestartPolicy`, both behind the single `supports_infrapolicy_vm_evacuation` capability, following the identical mapper/watch/RBAC pattern already established.

## Resolved decisions

All open questions from the prior draft have been resolved:

- `PolicyStatus` does **not** need a `PolicyID` field. WIKI page 2781824956 has been updated to drop that field from its status example; `status.policies[]` entries remain `apiVersion`/`kind`/`name`/`generation` only, unchanged from today's `PolicyStatus` type.
- The `Reason` string for the new `VirtualMachinePowerStateSynced=False` case is confirmed as `InfraInMaintenance`.
- The status-condition logic **is** gated: the entire feature sits behind a single, shared capability, `supports_infrapolicy_vm_evacuation`. Both CRDs and the status-condition behavior turn on and off together; there is no way to enable one without the other.
- When a VM matches both the Mandatory `AutomaticVMEvictionPolicy` and an Optional `BestEffortRestartPolicy`, both are surfaced in `status.policies`, regardless of which one DRS ultimately honors. This requires no new code — it is already how multiple simultaneous policy matches (e.g. `ComputePolicy` + `ControlledRebalancingPolicy`) are surfaced today via `pkg/vmconfig/policy`.

## Open questions

None outstanding.
