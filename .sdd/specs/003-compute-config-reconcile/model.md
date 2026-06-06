# Data Model: VM Compute Configuration Reconciliation

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)

This feature adds three optional `VirtualMachine.spec` fields (`resources`, `cpuAdvanced`, `memoryAdvanced`) and one status condition (`VirtualMachineConditionComputeConfigSynced`) to `v1alpha6`.  Note that the fields are optional since the fields are backfilled during creation.

## API surface

Most of the fields below are mutable and some are conditionally mutable. None of the fields should be mutable till VM schema upgrade completes - i.e. before the one-time backfill populates these fields from the live vSphere config for newly created VMs or imported VMs or during schema upgrage. Once the backfill completes, the fields are normal mutable spec fields.

All validation in this document, including the fields themselves, is gated by the `supports_telco_vm_service_api` supervisor capability. When the capability is disabled, setting any of `spec.resources`, `spec.cpuAdvanced`, or `spec.memoryAdvanced` MUST be rejected outright.

### `VirtualMachine.spec.resources` (`VirtualMachineResourcesSpec`)

| Field | Type | +optional/+required | Validation | Notes |
|-------|------|----------------------|------------|-------|
| `size.cpu` | `*resource.Quantity` | optional | must be `> 0` when set; whole number only (no `m` suffix) | guest-visible vCPU count; maps to `ConfigSpec.NumCPUs`. `nil` defers to the VM Class value. An increase can be applied while the VM is powered on when cpuAdvanced.hotAddEnabled is already enabled on the live VM; a decrease always requires power-off.|
| `size.memory` | `*resource.Quantity` | optional | must be `> 0` when set | guest memory; maps to `ConfigSpec.MemoryMB`. `nil` defers to the VM Class value. An increase can be applied while the VM is powered on when memoryAdvanced.hotAddEnabled is already enabled on the live VM; a decrease always requires power-off. |
| `requests.cpu` | `*resource.Quantity` | optional | must not be negative when set (`0` is valid); whole number only (no `m` suffix); `<= limits.cpu` when both set and `limits.cpu` is not the `-1` unlimited sentinel | host MHz reservation; maps to `CpuAllocation.Reservation`. Can be reconfigured while powered on. |
| `requests.memory` | `*resource.Quantity` | optional | must not be negative when set (`0` is valid); `<= size.memory` when both set; `<= limits.memory` when both set and `limits.memory` is not the `-1` unlimited sentinel; must be unset when `memoryAdvanced.reservationLockedToMax=true` | guaranteed amount of memory for the VM; maps to `MemoryAllocation.Reservation`. |
| `limits.cpu` | `*resource.Quantity` | optional | must be `> 0` or `-1` when set; whole number only (no `m` suffix) | absolute maximum cap on CPU a VM can use; maps to `CpuAllocation.Limit`. `nil` or `-1` = unlimited. |
| `limits.memory` | `*resource.Quantity` | optional | must be `> 0` or `-1` when set; when `memoryAdvanced.reservationLockedToMax=true`, must be `>= size.memory` unless `limits.memory` is the `-1` unlimited sentinel | absolute maximum cap on Memory a VM can use; maps to `MemoryAllocation.Limit`. `nil` or `-1` = unlimited. |

### `VirtualMachine.spec.cpuAdvanced` (`VirtualMachineCPUAdvancedSpec`)

| Field | Type | +optional/+required | Validation | Notes |
|-------|------|----------------------|------------|-------|
| `latencySensitivity` | `*VirtualMachineLatencySensitivityLevel` (enum: `Normal`\|`High`\|`HighWithHyperthreading`; `Low` is not a supported value) | optional | `High`/`HighWithHyperthreading` require full memory reservation (`requests.memory == size.memory`, or `memoryAdvanced.reservationLockedToMax=true`) AND an explicit, non-zero `requests.cpu`; the webhook cannot verify `requests.cpu` equals exactly 100% of vCPU capacity (host-speed-dependent), so it only checks that a reservation was explicitly set | can be reconfigured while powered on; `HighWithHyperthreading` also sets `ConfigSpec.SimultaneousThreads=2` |
| `topology.coresPerSocket` | `*int32` | optional | `Minimum=0`; `0`/unset = auto | requires power-off to apply |
| `topology.vnumaNodeCount` | `*int32` | optional | `Minimum=0`; requires `coresPerSocket > 0` when non-zero; when set and `size.cpu` set, `size.cpu` must divide evenly by `vnumaNodeCount` and the derived `coresPerNumaNode` must be a multiple/divisor of `coresPerSocket` | requires VM hardware version >= 20; requires power-off |
| `hotAddEnabled` | `*bool` | optional | — | requires VM hardware version  >= 11; requires power-off |
| `iommuEnabled` | `*bool` | optional | — | requires VM hardware version >= 14 (Intel) / 18 (AMD); requires power-off |
| `nestedHardwareVirtualizationEnabled` | `*bool` | optional | — | requires VM hardware version >= 9; requires power-off |
| `performanceCountersEnabled` | `*bool` | optional | — | requires VM hardware version >= 9; requires power-off |

### `VirtualMachine.spec.memoryAdvanced` (`VirtualMachineMemoryAdvancedSpec`)

| Field | Type | +optional/+required | Validation | Notes |
|-------|------|----------------------|------------|-------|
| `hotAddEnabled` | `*bool` | optional | — | requires VM hardware version >= 7; requires power-off |
| `reservationLockedToMax` | `*bool` | optional | when `true`: `requests.memory` must be unset; `limits.memory`, if set, must be `>= size.memory` | maps to `ConfigSpec.MemoryReservationLockedToMax`; can be reconfigured while powered on |

### CEL validation rules

Two rules are structural enough to enforce as CEL directly on the CRD schema rather than in the webhook's Go validation, per `architectural-standards.md`:

- **CPU quantities must be a whole number** (applies to `size.cpu`, `requests.cpu`, and `limits.cpu` — all three share the `CPU` field on `VirtualMachineResourceQuantity`): `rule="type(self) != string || !self.endsWith('m')"`, `message="CPU must be a whole number (e.g. '4' for vCPUs or '2000' for 2000 MHz). The 'm' (milli) suffix is not supported."`
- **`vnumaNodeCount` requires `coresPerSocket`**: `rule="!has(self.vnumaNodeCount) || self.vnumaNodeCount == 0 || (has(self.coresPerSocket) && self.coresPerSocket > 0)"`, `message="vnumaNodeCount requires coresPerSocket to be set to an explicit (non-zero) value"`

### Cross-field validation not tied to a single field

- A VM MUST NOT set latencySensitivity to High/HighWithHyperthreading unless the VM satisfies full CPU reservation (`requests.cpu == size.cpu (in Mhz)`) AND full memory reservation (`requests.memory == size.memory`, or `memoryAdvanced.reservationLockedToMax=true`).
- A VM network interface MUST NOT set `spec.network.interfaces[].vmxnet3.uptv2Enabled=true` unless the VM satisfies full guest memory reservation (the same memory rule used for `latencySensitivity` above).
- All validation for this API surface, including the `supports_telco_vm_service_api` gate, is cross-field depends on spec state outside these three fields.

## Status conditions

### `VirtualMachineConditionComputeConfigSynced` (`VirtualMachineComputeConfigSynced`)

This condition reflects whether `spec.resources` / `spec.cpuAdvanced` / `spec.memoryAdvanced` are fully applied to the live vSphere configuration. It should be recomputed on every reconcile by dry-running the same field-by-field diff/apply logic used for the real reconfigure, regardless of whether a reconfigure actually happened this cycle. Exactly one of the following four outcomes applies per reconcile, evaluated in this precedence order — first match wins:

| Order | Reason | Status | Trigger | Message |
|-------|--------|--------|---------|---------|
| 1 | `PrerequisiteNotMet` | False | one or more differing fields fail their hardware-version gate or a runtime prerequisite (full CPU/memory reservation, vNUMA topology) | `"Prerequisites not met: <field> (<reason>); ..."` — one entry per blocked field |
| 2 | `PowerOffRequired` | False | no prerequisite failures, but one or more differing fields are not currently hot-pluggable while the VM is powered on | `"VM power off required to apply: <field>, ..."` — one entry per deferred field |
| 3 | `ComputeConfigMismatch` | False | no blocked fields, but at least one field still needs to be written to converge spec and live config | `"Spec compute fields differ from the live vSphere configuration"` |
| 4 | _(none)_ | True | every field is converged | — |

Design notes:

- `PrerequisiteNotMet` and `PowerOffRequired` are never reported together in the same condition message — a hardware-version or prerequisite block on any field takes full precedence over reporting power-off-required fields, even if both categories exist on the same reconcile. If a future requirement needs both surfaced simultaneously, this precedence rule and its message format need to change together.

## Backfill strategy

For a VM that predates this feature, `spec.resources`, `spec.cpuAdvanced`, and `spec.memoryAdvanced` are populated once from the live vSphere `ConfigInfo` during schema upgrade, using the reverse of the `Field mapping` table in `plan.md`. The governing rule is **spec wins**: backfill MUST only write a leaf field (e.g. `size.cpu`, `hotAddEnabled`) when it is currently `nil` on the VM; any value the DevOps user has already set — including one that happens to equal the vSphere default — is left untouched. This applies per leaf field, not per top-level struct: a VM with `spec.cpuAdvanced.hotAddEnabled` already set but `spec.cpuAdvanced.topology` unset gets only `topology` backfilled, not the whole `cpuAdvanced` struct replaced.

## Conversion strategy

`spec.resources`, `spec.cpuAdvanced`, and `spec.memoryAdvanced` are defined only on `v1alpha6`. Older API versions (`v1alpha1`-`v1alpha5`) MUST NOT carry any equivalent representation of these fields, but the values MUST survive a down-convert/up-convert round trip, same as every other `v1alpha6`-only field:
- Down-converting (`ConvertFrom`, hub → spoke) MUST drop these fields from the visible spoke spec (the older type has no field to hold them), but MUST preserve them by having `utilconversion.MarshalData` encode the full hub object into an annotation on the spoke object.
- Up-converting (`ConvertTo`, spoke → hub) MUST restore them from that annotation via `restore_v1alpha6_VirtualMachineResources`, `restore_v1alpha6_VirtualMachineCPUAdvanced`, and `restore_v1alpha6_VirtualMachineMemoryAdvanced`, following the same `MarshalData`/`UnmarshalData` + `restore_*` pattern used for other hub-only fields (e.g. `Spec.Bootstrap.Disabled`, `Spec.VolumeAttributesClassName`, `Spec.Network.VLANs`).
- No new conversion webhook logic is needed beyond wiring those three `restore_*` functions into each spoke version's `ConvertTo` (`v1alpha1` through `v1alpha5`) — the existing hub-spoke round-trip machinery covers the rest.

## Example YAML

```yaml
# DevOps user: explicit CPU/memory size, CPU hot-add enabled so a future
# size increase can apply without powering off.
apiVersion: vmoperator.vmware.com/v1alpha6
kind: VirtualMachine
metadata:
  name: telco-vnf-01
  namespace: my-namespace
spec:
  className: my-vm-class
  imageName: my-vm-image
  resources:
    size:
      cpu: "4"
      memory: "8Gi"
    requests:
      cpu: "2000"      # MHz
      memory: "4Gi"
  cpuAdvanced:
    hotAddEnabled: true
  memoryAdvanced:
    hotAddEnabled: true
```

```yaml
# DevOps user: latency-sensitive VNF workload. High latency sensitivity
# requires full CPU + memory reservation (requests == size, or
# reservationLockedToMax for memory).
apiVersion: vmoperator.vmware.com/v1alpha6
kind: VirtualMachine
metadata:
  name: telco-vnf-latency-sensitive
  namespace: my-namespace
spec:
  className: my-vm-class
  imageName: my-vm-image
  resources:
    size:
      cpu: "8"
      memory: "16Gi"
    requests:
      cpu: "16000"     # 100% of an 8-vCPU class at 2GHz/core, illustrative
      memory: "16Gi"   # equals size.memory to satisfy full reservation
  cpuAdvanced:
    latencySensitivity: HighWithHyperthreading
    topology:
      coresPerSocket: 4
      vnumaNodeCount: 2  # 8 vCPU / 2 nodes = 4 coresPerNumaNode, divides coresPerSocket evenly
```

```yaml
# Platform engineer debugging: status excerpt when a field is blocked by a
# hardware-version gate (VM is on vmx-9, iommuEnabled requires vmx-14+).
status:
  conditions:
    - type: VirtualMachineComputeConfigSynced
      status: "False"
      reason: PrerequisiteNotMet
      message: "Prerequisites not met: cpuAdvanced.iommuEnabled (requires hwVer >= 14)"
```
