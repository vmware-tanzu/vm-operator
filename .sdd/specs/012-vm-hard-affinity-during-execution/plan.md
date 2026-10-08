# Implementation Plan: VM Hard Affinity During Execution

- **Spec**: [spec.md](./spec.md)
- **Epic**: vmop-3332
- **Date**: 2026-10-07

## Summary

Expose DRS 9.2 placement capabilities (RequiredDuringExecution strictness, host `VmToVmGroupsAntiAffinity`, per-VM `candidateVSphereZone`, group-less affinity, persisted zonal policies) behind `Features.VMHardAffinityDuringExecution`, plus the entitlement check on `RequiredDuringExecutionVMPlacementPolicy`.

## Technical context

- **Go version**: 1.26.8 (`go.mod`).
- **Modules touched**: root module only.
- **API**: `vmoperator.vmware.com/v1alpha6`. No new fields; `VMAffinitySpec.RequiredDuringSchedulingRequiredDuringExecution` and `VMAntiAffinitySpec.RequiredDuringSchedulingRequiredDuringExecution` already exist (`api/v1alpha6/virtualmachine_affinity_types.go`). v1alpha5 restores them on round-trip (`api/v1alpha5/virtualmachine_conversion.go`, `restore_v1alpha6_VirtualMachineAffinityRequiredDuringExecution`).
- **External API**: `vsphere.policy.vmware.com/v1alpha1` `RequiredDuringExecutionVMPlacementPolicy` (`external/vsphere-policy/api/v1alpha1/requiredduringexecutionvmplacementpolicy_types.go`), already registered in `pkg/crd/crd.go`.
- **Capability / features**: `CapabilityKeyVMHardAffinityDuringExecution = "supports_vm_service_vm_hard_affinity_during_execution"` sets both `Features.TaggingAPI` and `Features.VMHardAffinityDuringExecution` (`pkg/config/capabilities/capabilities.go`). Both default to `false` (`pkg/config/default.go`).
- **Dependencies**: govmomi `vimtypes.VmToVmGroupsAntiAffinity`, `VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementRequiredDuringExecution`, `PlaceVmsXCluster` `VmPlacementSpecs[].CandidateVsphereZone`.

### Feature gates

| Gate | Source | Controls |
|------|--------|----------|
| `Features.VMHardAffinityDuringExecution` | capability `supports_vm_service_vm_hard_affinity_during_execution` | G1–G5 |
| `Features.TaggingAPI` | same capability | G6 (spec 006) |
| `Features.VMPlacementPolicies` | existing | `ConfigureZoneRules` base |
| `Features.VMAffinityDuringExecution` | existing | `ConfigureHostRules` base |

## Constitution check

| Rule | Status |
|------|--------|
| API compatibility | No API change; existing field becomes admissible. |
| Thin controllers | All changes in `pkg/providers/vsphere/` and `webhooks/`. |
| Provider abstraction | vSphere policy objects built only in `pkg/providers/vsphere/virtualmachine/affinity.go` and `placement/`. |
| Webhooks: Go validation for cross-object rules | Entitlement check requires listing another resource, so Go validation (not CEL). |
| Feature flags | Every behavior gated on `pkgcfg.FromContext(ctx).Features.VMHardAffinityDuringExecution`. |
| Testing | Unit tests in existing `_test.go` files; E2E in `test/e2e/vmservice/vmservice/virtualmachine/vm_group.go`. |

## Project structure

| Path | Change |
|------|--------|
| `pkg/providers/vsphere/virtualmachine/affinity.go` | RequiredDuringExecution policy generation; host `VmToVmGroupsAntiAffinity` vs `VmVmAntiAffinity`; zonal RequiredDuringExecution. |
| `pkg/providers/vsphere/virtualmachine/configspec.go` | `CalculateAffinityConstraints`: keep `ConfigureZoneRules` on create when feature enabled. |
| `pkg/providers/vsphere/placement/cluster_placement.go`, `group_placement.go` | `CandidateVsphereZone` per VM (done, PR 2000). |
| `pkg/providers/vsphere/vmprovider_vm.go` | Placement path for VMs with affinity but no `spec.groupName`. |
| `webhooks/virtualmachine/validation/virtualmachine_validator.go` | `validateRequiredDuringExecutionTerms`, entitlement check on change only, zone key allowed, `groupName` optional. |
| `config/rbac/role.yaml` | `get;list;watch` on `requiredduringexecutionvmplacementpolicies` (done, PR 2012). |
| `test/e2e/vmservice/vmservice/virtualmachine/vm_group.go` | E2E per user story. |

## API / CRD strategy

Additive admission only: the v1alpha6 field already exists and was previously always `Forbidden`. No version bump or conversion change.

## Controller / webhook impact

### RequiredDuringExecution (G1)

- **Provider** (`affinity.go`): when the feature is enabled, `extractAffinityLabelsFromVM` includes RequiredDuringExecution terms so their labels are tagged. `processVMAffinity` emits `VmVmAffinity`, and `processVMAntiAffinity` emits grouped anti-affinity, with strictness `RequiredDuringPlacementRequiredDuringExecution`. Host terms are emitted under `ConfigureHostRules`. Zone terms are emitted under `ConfigureZoneRules`.
- **Webhook**:
  - `validateRequiredDuringExecutionTerms` returns `Forbidden` if the feature is off or `hasRequiredDuringExecutionPolicy` (namespace `List` with `Limit(1)`) is false.
  - Topology key must be `kubernetes.io/hostname` or `topology.kubernetes.io/zone`, otherwise `NotSupported`.
  - `validateVMAffinity` runs only on create, and `validateImmutableVMAffinity` forbids any update to `spec.affinity`. The entitlement is therefore only evaluated at create, and revoking it does not block other updates (G1.5). No code change is needed; T006 adds the tests.
- **Delta vs PR 2012**: PR 2012 allows only the host key. T007 adds the zone key.

### Host anti-affinity grouping (G2) — PR 2012

`addHostAntiAffinityPolicies` passes `grouped = Features.VMHardAffinityDuringExecution` to `addAntiAffinityPolicies`: one `VmToVmGroupsAntiAffinity` with all tags, or one `VmVmAntiAffinity` per tag. Zone anti-affinity is always grouped.

### candidateVSphereZone (G3) — PR 2000

`cluster_placement.go` sets `VmPlacementSpecs[i].CandidateVsphereZone` and `group_placement.go` uses per-VM zones only when the feature is enabled.

### Persist zonal policies at create (G4)

`CalculateAffinityConstraints` currently sets `ConfigureZoneRules = false` when `isCreateVM`. Change it so this applies only when `VMHardAffinityDuringExecution` is disabled.

### Cluster modules (G1.6)

`CalculateAffinityConstraints` disables both `ConfigureHostRules` and `ConfigureZoneRules` when the VM has a cluster module, so no placement policies are emitted for it. VKS node VMs without a cluster module are not excluded.

### Group-less affinity (G5)

- Webhook: `validateVMAffinity` requires `spec.groupName` only when the feature is disabled.
- Provider: `vmprovider_vm.go` currently places by group only when `spec.groupName != ""`. A group-less VM with affinity must reach `PlaceVmsXCluster` with its placement policies in the `ConfigSpec` so DRS evaluates them against existing workloads. Verify the non-group placement path forwards the policies; adjust if not.

### Tagging (G6)

No new work here; see spec 006 (`controllers/controllers.go` and `webhooks/webhooks.go` gate on `Features.TaggingAPI`).

## RBAC

| Component | Group | Resource | Verbs |
|-----------|-------|----------|-------|
| VM validating webhook | `vsphere.policy.vmware.com` | `requiredduringexecutionvmplacementpolicies` | `get;list;watch` |

Generated from the `+kubebuilder:rbac` marker on the VM validator into `config/rbac/role.yaml` (PR 2012).

## Test strategy

- **Unit**:
  - `pkg/providers/vsphere/virtualmachine/configspec_test.go`: policy shape per feature state, strictness, topology, cluster module exclusion, VKS node VM without cluster module, zone policies on create.
  - `webhooks/virtualmachine/validation/virtualmachine_validator_unit_test.go`: feature on/off, entitlement present/absent, topology keys, update with no entitlement, `groupName` optional.
- **Placement**: `pkg/providers/vsphere/placement` tests for `CandidateVsphereZone` (PR 2000).
- **vcsim**: [Todo] combination matrix (spec SC-004).
- **E2E**: see [e2e.md](./e2e.md). Stretched Supervisor required; entitlement provisioned via InfraPolicy assignment.

## Reconcile flow

```
VM webhook (create)
  validateVMAffinity
    RDE terms -> feature + entitlement (List, Limit 1) + topology key
    groupName required only when feature off
VM webhook (update)
  validateImmutableVMAffinity -> spec.affinity unchanged

VM controller -> vSphere provider createVirtualMachine
  CalculateAffinityConstraints(isCreateVM=true)
    ConfigureZoneRules: on at create when feature on
    Host/Zone rules: off when VM has a cluster module
  extractAffinityLabelsFromVM -> tags (incl. RDE terms)
  processVMAffinity / processVMAntiAffinity -> placement policies
  placement
    groupName set -> vmCreateDoPlacementByGroup
                     (CandidateVsphereZone per VM)
    groupName empty -> single-VM PlaceVmsXCluster with policies
```

## Branch / backport strategy

`main` only; no backport.

## Rollout / migration

- Capability-driven; defaults off. No backfill: existing VMs keep their 9.1.x policies until reconfigured.
- On upgrade with the feature on, existing VMs keep their 9.1.1 `VmVmAntiAffinity` host policies; only new VMs get `VmToVmGroupsAntiAffinity`.
- govmomi is pinned to ToT (`v0.57.0-alpha`); move to the v0.57 release when it is tagged.
- Release note: RequiredDuringExecution affinity requires a namespace `RequiredDuringExecutionVMPlacementPolicy`; host RequiredDuringExecution can block host maintenance mode.

## Risks

| Risk | Mitigation |
|------|------------|
| Host RequiredDuringExecution blocks host maintenance mode. | Namespace entitlement assigned by the CSP admin (G1.3). |
| Placement policies cannot be removed from a VM, so a mis-specified policy persists for the VM's lifetime. | `spec.affinity` immutable; webhook validation at create; delete and recreate the VM to change it. |
| Unsatisfiable hard affinity fails placement. | Placement error surfaced in VM status (spec E8). |
| govmomi types only on ToT. | Track v0.57 release. |
| E2E matrix too large and slow. | Representative E2E; unit + vcsim for the matrix. |

## Complexity tracking

No constitutional deviations.
