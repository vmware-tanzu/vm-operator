# Data Model: VM Eviction Compute Policies

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)

## New CRDs — `vsphere.policy.vmware.com/v1alpha1`

Both CRDs reuse the existing `ComputePolicy`/`ControlledRebalancingPolicy` spec and status shape field-for-field. No new field types are introduced by this feature.

### `AutomaticVMEvictionPolicy`

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `spec.description` | `string` | optional | Free-text description. |
| `spec.policyID` | `string` | optional | The ID of the underlying vCenter compute policy, of type `automatic_vm_eviction`. |
| `spec.enforcementMode` | `PolicyEnforcementMode` (`Mandatory`\|`Optional`) | optional, default `Mandatory` | Provider typically applies this policy Mandatorily; the schema does not restrict it to Mandatory-only. |
| `spec.match` | `*MatchSpec` | optional | Existing shared match type (workload labels, guest ID/family, image, boolean composition). Unset + Mandatory = applies to all VMs in namespace. |
| `spec.tags` | `[]string` | optional | Names of `TagPolicy` objects in the same namespace supplying the vSphere tag(s) to apply. |
| `status.observedGeneration` | `int64` | optional | Standard. |
| `status.conditions` | `[]metav1.Condition` | optional | Standard; includes `Ready`. |

Added to the `VirtualMachineSpec.Policies` doc comment's "Valid policy types" list. Nothing in the webhook or `reconcileExplicitPolicies` restricts a `Mandatory`-typed policy from being referenced explicitly via `spec.policies` — the "Valid policy types" doc comment is documentation only, not enforced — and `reconcileExplicitPolicies` already handles `AutomaticVMEvictionPolicy` refs the same as `ComputePolicy`/`BestEffortRestartPolicy`, so the doc now reflects that it is supported.

Example (from WIKI page 2781824956):

```yaml
apiVersion: vsphere.policy.vmware.com/v1alpha1
kind: AutomaticVMEvictionPolicy
metadata:
  name: auto-vm-eviction
  namespace: vm-svc-workloads
spec:
  description: "Automatic eviction of VMs during host maintenance-mode excluding control plane VMs"
  enforcementMode: Mandatory
  policyID: "fc42205a-334b-4e63-85d4-4bb89104e2d1"
  tags:
  - "automatic-eviction"
  match:
    workload:
      labels:
      - key: "capv.vmware.com/cluster.role"
        operator: "NotIn"
        values:
        - "controlplane"
```

### `BestEffortRestartPolicy`

Identical shape to `AutomaticVMEvictionPolicy` (same `ComputePolicy`-derived spec/status). Distinguishing usage: tenants reference this one explicitly and optionally via `spec.policies` on a VM/VKS node to override the provider's default post-maintenance restart behavior.

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
    kind: AutomaticVMEvictionPolicy
    name: auto-vm-eviction
    generation: 1
```

No `PolicyID` field is added to `PolicyStatus`. WIKI page 2781824956 has been corrected to drop that field from its example; confirmed no schema change is needed here.

When a VM matches both a Mandatory `AutomaticVMEvictionPolicy` and an Optional `BestEffortRestartPolicy`, both entries appear in `status.policies` — this already happens today for any two simultaneously-matching policy kinds (no code change required):

```yaml
status:
  policies:
  - apiVersion: vsphere.policy.vmware.com/v1alpha1
    kind: AutomaticVMEvictionPolicy
    name: auto-vm-eviction
    generation: 1
  - apiVersion: vsphere.policy.vmware.com/v1alpha1
    kind: BestEffortRestartPolicy
    name: restart-on-any-host
    generation: 1
```

### `status.conditions` — `VirtualMachinePowerStateSynced` (existing condition type, new `Reason`)

No new condition **type** is introduced, and no host-state lookup is performed. The signal comes from the VM's own power-state-change task failing with a specific vCenter fault, detected at the point VM Operator itself attempts to converge power state.

- `vmCtx.VM.Status.PowerState` continues to be set in `reconcileStatusPowerState` (`pkg/providers/vsphere/vmlifecycle/update_status.go`), unchanged.
- The `VirtualMachinePowerStateSynced` **condition** moves out of `reconcileStatusPowerState` entirely and into `reconcilePowerState` (`pkg/providers/vsphere/vmprovider_vm.go`, the "Reconcile power state" step) — the step that actually calls `vmutil.SetAndWaitOnPowerState` and therefore has the real outcome of this reconcile's power-op attempt (no-op because already synced / succeeded / failed), rather than a pre-op snapshot computed earlier in the same pass.
- A new pure helper, `isInfraMaintenanceFault(ti *vimtypes.TaskInfo) bool` (`pkg/util/vsphere/task/task.go`), matches a `*vimtypes.NoCompatibleHost` fault whose nested fault messages carry the key `com.vmware.cp.autoevac.HostInMaintenanceMode` or `com.vmware.cp.autoevac.RestartOnCurrentHostRequired`.
- `pkg/util/vsphere/vm/power_state.go`'s failure path wraps its returned error with a new sentinel, `vmutil.ErrInfraMaintenanceFault` (via `%w`), when `isInfraMaintenanceFault(ti)` is true for the failed task. This follows the existing `pkgerr` sentinel-error convention (propagates via `errors.Is`/`errors.As` from any call-stack depth) rather than threading the raw `TaskInfo` up through new parameters.
- `reconcilePowerState` checks `errors.Is(err, vmutil.ErrInfraMaintenanceFault)` after the power-op call and sets the condition itself, gated on `Features.VMEviction`:

| Outcome of this reconcile's power-op attempt | `Features.VMEviction` | `Reason` |
|---|---|---|
| already synced (no-op) or op succeeded | any | `Synced` (`status: True`) |
| op failed, not the infra-maintenance fault | any | `NotSynced` (`status: False`) |
| op failed, `errors.Is(err, ErrInfraMaintenanceFault)` | off | `NotSynced` (`status: False`) — unchanged behavior when the feature is off |
| op failed, `errors.Is(err, ErrInfraMaintenanceFault)` | on | **`InfraInMaintenance`** (`status: False`) — confirmed string |

The `Reason` string stays the policy-agnostic `InfraInMaintenance` even though today only `AutomaticVMEvictionPolicy`/`BestEffortRestartPolicy` cause DRS to power a VM off this way: the reason describes the vCenter-side autoevac fault VM Operator observed, not which CRD authorized DRS's action, and VKS only needs "this is infra maintenance," not which policy kind was involved.

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
| `AutomaticVMEvictionPolicy` CRD | `VMEviction` | `supports_infrapolicy_vm_evacuation` |
| `BestEffortRestartPolicy` CRD | `VMEviction` | `supports_infrapolicy_vm_evacuation` |
| `VirtualMachinePowerStateSynced` `InfraInMaintenance` reason | `VMEviction` | `supports_infrapolicy_vm_evacuation` |

All three behaviors share the single `VMEviction` flag / `supports_infrapolicy_vm_evacuation` capability key — there is no per-CRD or per-behavior gate. The two CRDs remain additionally gated by the existing top-level `VSpherePolicies` flag, exactly like `ControlledRebalancingPolicy` (`features.VSpherePolicies && features.VMEviction` in `pkg/crd/crd.go`'s `Install`).

`VMEviction` gates the two CRDs' install/`PolicyEvaluation` wiring **and** the `InfraInMaintenance` branch in `reconcilePowerState` (`pkg/providers/vsphere/vmprovider_vm.go`) together — enabling the capability turns on both CRDs and the status signal at once; there is no way to activate one without the others.

## No conversion-webhook impact

`PolicySpec.Kind` and `LocalObjectRef.Kind` are opaque strings; adding new valid values requires no changes to `zz_generated.conversion.go` for any API version. This feature does not touch any conversion webhook.

## Branch note

Every `VirtualMachine` example above is shown at `v1alpha6` (`main`'s storage version). On the `release/vc-9.1.0` backport, the identical examples apply with `apiVersion: vmoperator.vmware.com/v1alpha5` and file path `api/v1alpha5/...` — the field names, types, and semantics are unchanged; only the version segment differs. See `spec.md` "Branch strategy" and `plan.md` "Branch / backport strategy".
