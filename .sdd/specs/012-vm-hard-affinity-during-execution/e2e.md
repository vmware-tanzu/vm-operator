# E2E Test Plan: VM Hard Affinity During Execution

- **Spec**: [spec.md](./spec.md) · **Plan**: [plan.md](./plan.md) · **Tasks**: [tasks.md](./tasks.md)

## Suite

- Scenarios extend `test/e2e/vmservice/vmservice/virtualmachine/vm_group.go`, following the existing "Group placement with affinity and anti-affinity" (zonal) and "...at host topology" contexts.
- Coverage is representative, not exhaustive: the strictness × topology × tag × cluster-layout matrix is covered by unit tests, with vcsim coverage as a [Todo] (spec SC-004).
- Registered through `VMGroupSpec` under `Context("VM-GROUP", ...)` in `test/e2e/vmservice/vmservice_test.go`; `TEST_FOCUS="VM-GROUP"` selects it.
- New scenarios sit under `When("VMs specify requiredDuringSchedulingRequiredDuringExecution", Label("experimental"), ...)` and sibling `When` blocks, carrying `"experimental"` until validated on hardware.
- Wait intervals reuse the existing `default/wait-virtual-machine-compute-policy-status-update` key; no new config keys.

## Gating

- `skipper.SkipUnlessStretchSupervisorIsEnabled()` — all scenarios require a multi-zone (stretched) Supervisor.
- `skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.VMHardAffinityDuringExecutionCapabilityName)`.
- Host scenarios additionally use `skipper.SkipUnlessSupervisorHasAtleastOneZoneWithHostCount` with the host count the scenario needs.

## Entitlement setup

The entitlement is provisioned by assigning an InfraPolicy of kind `infrapolicy.PolicyKindRequiredDuringExecutionVMPlacementPolicy` to the test namespace, which creates the `RequiredDuringExecutionVMPlacementPolicy` in it. Revocation scenarios unassign the InfraPolicy.

## Verification

Each placement scenario verifies, as the existing `vm_group.go` tests do:

- Admission (admit / deny with the expected message).
- Placement (host / zone of each VM).
- vCenter policy compliance via `WCPClient.GetVMPolicyCompliance` (`COMPLIANT`).
- The vCenter tags attached to each VM.

## Scenarios

### US1 — RequiredDuringExecution with entitlement

| Scenario | Verifies |
|----------|----------|
| rejects a VM with required-required host AF when the namespace has no entitlement | AC1.3 |
| creates 4 VMs with required-required host AF with each other | AC1.1 |
| creates 3 VMs with required-required host AAF with each other | AC1.1 |
| creates VMs with required-required host AF (2 VMs) and AAF (2 VMs) | AC1.1 |
| creates VMs with required-required zone AF and AAF | AC1.2 |
| rejects a required-required term with an unsupported topology key | AC1.5 |

### US2 — entitlement revoked

| Scenario | Verifies |
|----------|----------|
| unassigns the InfraPolicy; existing required-required VMs stay compliant | AC2.1 |
| updates a label on an existing required-required VM | AC2.2 |
| rejects a new required-required VM after revocation | AC2.3 |

### US3 — host anti-affinity grouping

| Scenario | Verifies |
|----------|----------|
| creates VMs with host AAF against two label selectors; one `VmToVmGroupsAntiAffinity` policy is compliant | AC3.1 |

### US4 — candidate zones

AC4.1 is covered by the existing "VMs are pinned to zones via the topology zone label" scenarios (PR 2000).

### US5 — zonal policies persisted at create

| Scenario | Verifies |
|----------|----------|
| creates VMs with zone AF/AAF; zone policies are attached and compliant after create | AC5.1 |

### US6 — affinity without a group

| Scenario | Verifies |
|----------|----------|
| creates a group-less VM with zone AF to an existing VM; it lands in the same zone | AC6.1 |
| creates a group-less VM with host AAF to existing VMs; it lands on a different host | AC6.3 |

### US7 — tagging unannotated VMs

Covered by spec 006 E2E.
