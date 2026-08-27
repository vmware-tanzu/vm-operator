# Data Model: Host Maintenance Mode Infra Policies

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)

## New CRDs — `vsphere.policy.vmware.com/v1alpha1`

Both CRDs reuse the existing `ComputePolicy`/`ControlledRebalancingPolicy` spec and status shape field-for-field. No new field types are introduced by this feature.

### `AutomaticHostEvacuationPolicy`

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `spec.description` | `string` | optional | Free-text description. |
| `spec.policyID` | `string` | optional | The ID of the underlying vCenter compute policy, of type `automatic_host_evacuation`. |
| `spec.enforcementMode` | `PolicyEnforcementMode` (`Mandatory`\|`Optional`) | optional, default `Mandatory` | Provider typically applies this policy Mandatorily; the schema does not restrict it to Mandatory-only. |
| `spec.match` | `*MatchSpec` | optional | Existing shared match type (workload labels, guest ID/family, image, boolean composition). Unset + Mandatory = applies to all VMs in namespace. |
| `spec.tags` | `[]string` | optional | Names of `TagPolicy` objects in the same namespace supplying the vSphere tag(s) to apply. |
| `status.observedGeneration` | `int64` | optional | Standard. |
| `status.conditions` | `[]metav1.Condition` | optional | Standard; includes `Ready`. |

Not added to the `VirtualMachineSpec.Policies` doc comment's "Valid policy types" list, matching the design doc ("usually applied by the provider mandatorily"). Confirmed by inspecting `webhooks/` and `reconcileExplicitPolicies`: nothing in the webhook or controller technically prevents a `Mandatory`-typed policy from being referenced explicitly via `spec.policies` — the "Valid policy types" doc comment is documentation only, not enforced. Omitting `AutomaticHostEvacuationPolicy` from that list is a documentation choice reflecting intended usage, not a schema restriction.

Example (from WIKI page 2781824956):

```yaml
apiVersion: vsphere.policy.vmware.com/v1alpha1
kind: AutomaticHostEvacuationPolicy
metadata:
  name: auto-host-evac
  namespace: vm-svc-workloads
spec:
  description: "Automatic evacuation host maintenance policy targeting all worker VMs excluding control plane VMs"
  enforcementMode: Mandatory
  policyID: "fc42205a-334b-4e63-85d4-4bb89104e2d1"
  tags:
  - "automatic-evacuation"
  match:
    workload:
      labels:
      - key: "capv.vmware.com/cluster.role"
        operator: "NotIn"
        values:
        - "controlplane"
```

### `BestEffortRestartPolicy`

Identical shape to `AutomaticHostEvacuationPolicy` (same `ComputePolicy`-derived spec/status). Distinguishing usage: tenants reference this one explicitly and optionally via `spec.policies` on a VM/VKS node to override the provider's default post-maintenance restart behavior.

Example (from WIKI page 2781824956):

```yaml
apiVersion: vsphere.policy.vmware.com/v1alpha1
kind: BestEffortRestartPolicy
metadata:
  name: restart-on-any-host
  namespace: vm-svc-workloads
spec:
  description: "Restart on any available host after maintenance-mode power-off"
  enforcementMode: Mandatory
  policyID: "fc42205a-334b-4e63-85d4-4bb89104e2d1"
  tags:
  - "best-effort-restart"
```

**Is** added to `VirtualMachineSpec.Policies`'s "Valid policy types are: ..." doc comment (`api/v1alpha6/virtualmachine_types.go` on `main`; re-applied by hand to `api/v1alpha5/virtualmachine_types.go` on the `release/vc-9.1.0` backport — see `spec.md` "Branch strategy"), alongside `ComputePolicy`.

## `VirtualMachine` (`api/v1alpha6` on `main`; `api/v1alpha5` on `release/vc-9.1.0`) — consuming side

### `spec.policies` (existing `[]PolicySpec`, no schema change)

```yaml
apiVersion: vmoperator.vmware.com/v1alpha6
kind: VirtualMachine
metadata:
  name: restart-policy
  namespace: vm-svc-workloads
spec:
  policies:
  - kind: BestEffortRestartPolicy
    name: restart-on-any-host
  className: best-effort-xsmall
  imageName: vmi-8fd6733b0b28760f4
  powerState: PoweredOn
  storageClass: wcpglobal-storage-profile
```

### `status.policies` (existing `[]PolicyStatus`)

No schema change required to surface the two new kinds — `PolicyStatus` already has `apiVersion`/`kind`/`name`/`generation`, populated identically to how `ComputePolicy`/`ControlledRebalancingPolicy` matches are surfaced today.

```yaml
status:
  policies:
  - apiVersion: vsphere.policy.vmware.com/v1alpha1
    kind: AutomaticHostEvacuationPolicy
    name: auto-host-evac
    generation: 1
```

No `PolicyID` field is added to `PolicyStatus`. WIKI page 2781824956 has been corrected to drop that field from its example; confirmed no schema change is needed here.

When a VM matches both a Mandatory `AutomaticHostEvacuationPolicy` and an Optional `BestEffortRestartPolicy`, both entries appear in `status.policies` — this already happens today for any two simultaneously-matching policy kinds (no code change required):

```yaml
status:
  policies:
  - apiVersion: vsphere.policy.vmware.com/v1alpha1
    kind: AutomaticHostEvacuationPolicy
    name: auto-host-evac
    generation: 1
  - apiVersion: vsphere.policy.vmware.com/v1alpha1
    kind: BestEffortRestartPolicy
    name: restart-on-any-host
    generation: 1
```

### `status.conditions` — `VirtualMachinePowerStateSynced` (existing condition type, new `Reason`)

No new condition **type** is introduced. `reconcileStatusPowerState` (`pkg/providers/vsphere/vmlifecycle/update_status.go`) gains one new branch in its not-synced case:

| `status.powerState` vs `spec.powerState` | Host state | `Reason` (existing) | `Reason` (new) |
|---|---|---|---|
| equal | — | `Synced` (`status: True`) | unchanged |
| not equal | host not in/entering/exiting maintenance mode, or `Features.VMEvacuation` is off | `NotSynced` (`status: False`) | unchanged |
| not equal | `Features.VMEvacuation` is on, and host `runtime.inMaintenanceMode == true`, or an `EnterMaintenanceMode_Task`/`ExitMaintenanceMode_Task` is active against it | `NotSynced` | **`InfraInMaintenance`** (`status: False`) — confirmed string |

Message text (from WIKI page 2781824956's example): `"VirtualMachine is powered off due to infrastructure is in maintenance"` — this spec proposes cleaning up the grammar (e.g. `"VirtualMachine is powered off because its host is in infrastructure maintenance"`) rather than reproducing the doc's message verbatim; exact wording is an implementation-time copy-editing decision, not a contract.

Deliberately **excluded from the message**: the host's name, Managed Object ID, or any other host-identifying detail (persona/topology-privacy requirement from the one-pager).

```yaml
status:
  powerState: PoweredOff
  conditions:
  - type: VirtualMachinePowerStateSynced
    status: "False"
    reason: InfraInMaintenance
    message: "VirtualMachine is powered off because its host is in infrastructure maintenance"
```

## Feature flags and capability keys

Following the `ControlledRebalancingPolicy` precedent (`pkg/config/config.go`, `pkg/config/capabilities/capabilities.go`):

| Capability-gated behavior | `pkgcfg.Features.*` flag | Capability key |
|---|---|---|
| `AutomaticHostEvacuationPolicy` CRD | `VMEvacuation` | `supports_infrapolicy_vm_evacuation` |
| `BestEffortRestartPolicy` CRD | `VMEvacuation` | `supports_infrapolicy_vm_evacuation` |
| `VirtualMachinePowerStateSynced` `InfraInMaintenance` reason | `VMEvacuation` | `supports_infrapolicy_vm_evacuation` |

All three behaviors share the single `VMEvacuation` flag / `supports_infrapolicy_vm_evacuation` capability key — there is no per-CRD or per-behavior gate. The two CRDs remain additionally gated by the existing top-level `VSpherePolicies` flag, exactly like `ControlledRebalancingPolicy` (`features.VSpherePolicies && features.VMEvacuation` in `pkg/crd/crd.go`'s `Install`).

`VMEvacuation` gates the two CRDs' install/`PolicyEvaluation` wiring **and** the `reconcileStatusPowerState` branch in `pkg/providers/vsphere/vmlifecycle/update_status.go` together — enabling the capability turns on both CRDs and the status signal at once; there is no way to activate one without the others.

## No conversion-webhook impact

`PolicySpec.Kind` and `LocalObjectRef.Kind` are opaque strings; adding new valid values requires no changes to `zz_generated.conversion.go` for any API version. This feature does not touch any conversion webhook.

## Branch note

Every `VirtualMachine` example above is shown at `v1alpha6` (`main`'s storage version). On the `release/vc-9.1.0` backport, the identical examples apply with `apiVersion: vmoperator.vmware.com/v1alpha5` and file path `api/v1alpha5/...` — the field names, types, and semantics are unchanged; only the version segment differs. See `spec.md` "Branch strategy" and `plan.md` "Branch / backport strategy".
