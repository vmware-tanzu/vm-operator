# Implementation Plan: VM Compute Configuration Reconciliation

- **Branch**: [`compute-config-reconcile`](https://github.com/hpannem/vm-operator/tree/compute-config-reconcile)
  - **Fork**: `hpannem/vm-operator`
  - **PR target**: `vmware-tanzu/vm-operator`
- **Date**: 2026-07-08
- **Spec**: [`spec.md`](spec.md)
- **Epic**: vmop-3388

---

## Summary

Implement the compute configuration surface described in `spec.md` within the `github.com/vmware-tanzu/vm-operator` codebase: three new `v1alpha6` spec fields (`spec.resources`, `spec.cpuAdvanced`, `spec.memoryAdvanced`), a single table-driven reconcile function that both a direct spec edit and a class-based resize funnel through, and a new `VirtualMachineConditionComputeConfigSynced` status condition. All per-field behavior — hardware-version gates, runtime prerequisites, hot-pluggability — is centralized in one function table so no call site duplicates that logic.

---

## Technical context

| Field | Value |
|-------|-------|
| **Language** | Go 1.26.4 |
| **Primary dependencies** | `govmomi`/`vim25` (`vimtypes`), `controller-runtime` — no new dependencies added |
| **API server** | Kubernetes (vSphere Supervisor); `v1alpha6` only |
| **Testing** | Ginkgo v2 + Gomega; `envtest`/`vcsim` for integration; real WCP Supervisor for E2E |
| **Code generation** | `controller-gen` (deepcopy, CRD manifests) |
| **Target platform** | VMware vSphere Supervisor (WCP) |

### Feature gates

Three independent boolean flags interact here:

- `pkgcfg.Features.TelcoVMServiceAPI` gates the entire compute surface — whether `OverwriteSpecComputeConfig` runs at all, and whether the webhook accepts `spec.resources`/`cpuAdvanced`/`memoryAdvanced` in the first place. Externally this is the `supports_telco_vm_service_api` Supervisor capability.
- `pkgcfg.Features.VMResize` gates the *full* class-resize path (device changes included). When enabled, it takes precedence over `VMResizeCPUMemory` at every call site in `session_vm_update.go` — the two are mutually exclusive by construction (`if VMResize {...} else {... checks VMResizeCPUMemory ...}`), not independently combinable.
- `pkgcfg.Features.VMResizeCPUMemory` gates the *narrow* CPU/memory-only resize path, and only takes effect when `VMResize` is disabled.
- Regardless of which resize flag (if any) is active, `TelcoVMServiceAPI` independently decides whether the compute-specific sync/overwrite calls run within that path.

---

## Constitution check

- [x] Business logic lives entirely in `pkg/util/vmopv1/` and `pkg/providers/vsphere/`; no controller in `controllers/` gains compute-config logic.
- [x] `OverwriteSpecComputeConfig` operates only on `vimtypes.VirtualMachineConfigInfo`/`ConfigSpec` values passed in by the session/provider layer; it is never called from `controllers/`.
- [x] `reconcileComputeConfigSynced` runs during "Reconcile status," strictly before "Reconcile config" applies the real change, per the VM Update Reconcile Order in `operator-best-practices.md`.
- [x] `VirtualMachineConditionComputeConfigSynced` is a pure read-only signal; no code path branches on it to decide whether to reconcile.
- [x] `spec.resources`, `spec.cpuAdvanced`, and `spec.memoryAdvanced` are new, wholly optional `v1alpha6` fields — no existing field is renamed, retyped, or removed.
- [x] No new `RequeueError`/`NoRequeueError` is needed; this feature relies on the existing `updateVirtualMachine` flow's `ErrReconfigure`/requeue handling for the actual vSphere task.
- [x] Test files use the single `_test.go` + `Label()` convention (`testlabels.Controller`, `testlabels.EnvTest`, etc.), not the retired `_unit_test.go`/`_intg_test.go` split.

---

## Repository layout (this feature)

### Documentation (specs/)

```
specs/002-compute-config-reconcile/
├── spec.md       — this feature's functional spec (what & why)
├── plan.md       — this file (how)
├── tasks.md      — ordered task checklist
└── model.md      — API surface, validation rules, condition semantics, conversion strategy
```

### Source

```
api/v1alpha6/
├── virtualmachine_compute_types.go      NEW    — spec.resources/cpuAdvanced/memoryAdvanced types
├── virtualmachine_types.go              MODIFY — VirtualMachineConditionComputeConfigSynced + reasons;
│                                                  reference the new types from VirtualMachineSpec
└── zz_generated.deepcopy.go             REGEN  — pick up the above

pkg/util/vmopv1/
├── compute_overwrite.go                 NEW    — computeFieldDef table + OverwriteSpecComputeConfig
├── resize_class_sync.go                 NEW    — SyncClassComputeToSpec / SyncClassSizeAndAllocationToSpec
└── vm.go                                MODIFY — GetVNUMANodeCount, FullMemReservationSpecMet, FirmwareIsEFI

pkg/providers/vsphere/
├── session/session_vm_update.go         MODIFY — call OverwriteSpecComputeConfig (powered-off + powered-on
│                                                  paths); call the class-sync functions at each
│                                                  ResizeNeeded branch (see Diagrams A-D in spec.md)
├── vmlifecycle/update_status.go         MODIFY — reconcileComputeConfigSynced (dry-run, sets the condition)
└── upgrade/virtualmachine/backfill/compute.go
                                          NEW    — backfill from live vSphere state during schema upgrade

pkg/util/vsphere/watcher/watcher.go      MODIFY — extend DefaultWatchedPropertyPaths with the vSphere
                                                   properties this feature reads (see below)

webhooks/virtualmachine/validation/virtualmachine_validator_compute.go
                                          NEW    — Go validation for the new fields (cross-field rules +
                                                   the supports_telco_vm_service_api gate); simple structural
                                                   rules (vnumaNodeCount/coresPerSocket co-presence,
                                                   latencySensitivity enum) are CEL on the type instead

config/crd/bases/
├── vmoperator.vmware.com_virtualmachines.yaml            REGEN
└── vmoperator.vmware.com_virtualmachinereplicasets.yaml  REGEN

test/e2e/vmservice/vmservice/virtualmachine/
└── vm_compute_config.go                 NEW    — E2E coverage (see Testing strategy)
```

`watcher.go` gains these property paths: `config.cpuAllocation`, `config.memoryAllocation`, `config.cpuHotAddEnabled`, `config.memoryHotAddEnabled`, `config.flags.vvtdEnabled`, `config.nestedHVEnabled`, `config.hardware.{numCPU,memoryMB,numCoresPerSocket,autoCoresPerSocket,simultaneousThreads}`, `config.numaInfo`, `config.latencySensitivity`, `config.vPMCEnabled`, `config.memoryReservationLockedToMax` — so an out-of-band vCenter change to any of them triggers a reconcile promptly instead of waiting for the next periodic resync.

---

## Implementation phases

### Phase 1 — API surface (Foundations)

1. Add `spec.resources`, `spec.cpuAdvanced`, and `spec.memoryAdvanced` to `VirtualMachineSpec`, each optional and rejected outright by the webhook when `supports_telco_vm_service_api` is disabled.
2. Add `VirtualMachineConditionComputeConfigSynced`. Of its three reasons, only `ComputeConfigMismatch` is new — `PrerequisiteNotMet` and `PowerOffRequired` are pre-existing, generic "used on synced conditions" reason constants already shared by other conditions (e.g. `pkg/vmconfig/networkextraconfig`). Reuse the existing declarations; do not redeclare them.
3. Regenerate deepcopy and CRD manifests.

### Phase 2 — Reconcile core

1. Implement the `computeFieldDef` table and `OverwriteSpecComputeConfig`, covering every field in `spec.resources`/`cpuAdvanced`/`memoryAdvanced` behind one shared diff/apply loop (see Diagrams A and B in `spec.md`).

   **Key contract**:
   ```go
   // pkg/util/vmopv1/compute_overwrite.go
   type computeFieldDef struct {
       fieldName    string
       minHWVer     vimtypes.HardwareVersion
       prerequisite func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo) (blocked bool, reason string)
       hotPluggable func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo) bool
       differs      func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) bool
       apply        func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec)
   }
   ```
2. Implement `SyncClassComputeToSpec` (full resize) and `SyncClassSizeAndAllocationToSpec` (narrow resize).

   **Why the sync must run before `OverwriteSpecComputeConfig`**: the class is authoritative during a resize — `ResizeNeeded` firing means the user (or an automated policy) asked to move the VM onto a different class's compute shape. Without syncing first, `OverwriteSpecComputeConfig` would diff the *current* `vm.Spec` — which may still hold stale values from before the resize, or backfilled values from schema upgrade — against the live config. Because spec wins over class in the normal (non-resize) reconcile path, the resize's class-derived values would never make it into the `ConfigSpec` without this step. Syncing into spec first makes the class's intent visible to the same diff-and-apply function used for every other reconcile, so there is one code path for applying compute config, not two.

   **Why the narrow path omits topology, hot-add flags, and latency sensitivity**: `VMResizeCPUMemory` is the narrower, earlier resize feature, scoped explicitly to CPU count, memory size, and their host-level allocations — that is the resize contract it was designed for, and this plan does not widen it. Topology (`coresPerSocket`/`vnumaNodeCount`), hot-add flags, and `latencySensitivity` are newer surface introduced under `supports_telco_vm_service_api`; pulling them into a CPU/memory-only resize would silently change fields the user never asked that narrower resize to touch. As noted under "Feature gates" above, `VMResize` and `VMResizeCPUMemory` are mutually exclusive by construction, so the narrow path's limited scope only ever applies when `VMResize` is off.
3. Wire both `OverwriteSpecComputeConfig` and the class-sync calls into `session_vm_update.go` (powered-off, powered-on, and both resize entry points — see Diagrams A-D in `spec.md`).
4. Add the schema-upgrade backfill in `pkg/providers/vsphere/upgrade/virtualmachine/backfill/compute.go`, populating only fields the DevOps user has not already set.
5. Extend `DefaultWatchedPropertyPaths` in `watcher.go`.

### Phase 3 — Status condition & observability

Implement `reconcileComputeConfigSynced`: a dry-run of `OverwriteSpecComputeConfig` against the live config, evaluated in this precedence order (first match wins):

```go
switch {
case len(blocked) > 0:
    // PrerequisiteNotMet — hardware-version gate or runtime prerequisite failed
case len(blockedPowerOff) > 0:
    // PowerOffRequired — powered on, field not currently hot-pluggable
case !reflect.DeepEqual(dryRunCS, vimtypes.VirtualMachineConfigSpec{}):
    // ComputeConfigMismatch — no blocks, but a field still needs to be written
default:
    // True — every field converged
}
```

### Phase 4 — Webhook validation

Implement `validateComputeConfig` and its sub-validators in `virtualmachine_validator_compute.go`: the `supports_telco_vm_service_api` gate, resource ordering (including the `-1` unlimited sentinel and its interaction with the ordering checks), `reservationLockedToMax` mutual exclusion, `latencySensitivity` full-reservation requirement, vNUMA topology co-presence, and the UPTv2 memory-reservation requirement. Every rule is enumerated in `model.md`. The `vnumaNodeCount`/`coresPerSocket` co-presence rule and the `latencySensitivity` enum are expressed as CEL directly on the CRD schema instead of Go, per `architectural-standards.md`.

### Phase 5 — Tests

Unit, integration, and E2E coverage per the Testing strategy table below; see `tasks.md` for the per-file breakdown.

---

## Field mapping

| Spec field | vSphere `ConfigSpec` path |
|------------|---------------------------|
| `resources.size.{cpu,memory}` | `NumCPUs`, `MemoryMB` |
| `resources.requests.{cpu,memory}` | `CpuAllocation.Reservation`, `MemoryAllocation.Reservation` |
| `resources.limits.{cpu,memory}` | `CpuAllocation.Limit`, `MemoryAllocation.Limit` (`-1` = unlimited) |
| `cpuAdvanced.latencySensitivity` | `LatencySensitivity.Level`, `SimultaneousThreads` (for `HighWithHyperthreading`) |
| `cpuAdvanced.topology.coresPerSocket` | `NumCoresPerSocket` |
| `cpuAdvanced.topology.vnumaNodeCount` | `VirtualNuma.CoresPerNumaNode` (derived) |
| `cpuAdvanced.hotAddEnabled` | `CpuHotAddEnabled` |
| `cpuAdvanced.iommuEnabled` | `Flags.VvtdEnabled` |
| `cpuAdvanced.nestedHardwareVirtualizationEnabled` | `NestedHVEnabled` |
| `cpuAdvanced.performanceCountersEnabled` | `VPMCEnabled` |
| `memoryAdvanced.hotAddEnabled` | `MemoryHotAddEnabled` |
| `memoryAdvanced.reservationLockedToMax` | `MemoryReservationLockedToMax` |

Implementation note: `requests` and `limits` for a given resource type are always hot-pluggable with no hardware-version gate, so they may be diffed/applied as either one combined reconcile unit per resource type or two independent ones — either is a valid implementation choice with no observable difference today. Only pick one deliberately if a future change adds independent gating to `requests` vs. `limits`.

Full validation rules and sentinel semantics for each of these fields live in `model.md`, not duplicated here.

---

## RBAC

No RBAC changes. This feature adds no new Kubernetes resource kind — only new spec fields and one new status condition on the existing `VirtualMachine` resource.

---

## Testing strategy

| Layer | Mechanism | Location |
|-------|-----------|----------|
| Unit | `*_test.go` + Ginkgo `Label()` | `pkg/util/vmopv1/compute_overwrite_test.go`, `resize_class_sync_test.go`, `webhooks/virtualmachine/validation/virtualmachine_validator_compute_test.go` |
| Integration | `*_test.go` + `testlabels.EnvTest`/`testlabels.VCSim` | `pkg/providers/vsphere/session/session_vm_update_test.go`, `pkg/providers/vsphere/vmlifecycle/update_status_test.go` |
| E2E | Ginkgo, real Supervisor | `test/e2e/vmservice/vmservice/virtualmachine/vm_compute_config.go` |

Condition precedence (`PrerequisiteNotMet` > `PowerOffRequired` > `ComputeConfigMismatch` > `True`) and the full sentinel matrix (`nil`/`0`/explicit/`-1`) must be covered at the unit layer; end-to-end condition reporting is covered at the integration layer per `e2e-sync-with-changes.md`.

---

## Risk and rollback

| Risk | Mitigation |
|------|-----------|
| A reconfigure fails for a reason none of the webhook/prerequisite gates catch (e.g. a vSphere-side resource constraint) | Surfaces as a persistent `ComputeConfigMismatch` across reconciles rather than a silent `True`, per `model.md` "Status conditions" |
| A client strips unknown annotations on an older API version | The hub-only fields are lost on that round trip; this is the same exposure every other `v1alpha6`-only field already has, not new to this feature |
| `VMResize` and `VMResizeCPUMemory` both enabled unexpectedly | Mutually exclusive by construction — `VMResize` always takes precedence, so behavior is deterministic even if both flags are set |
| CRD schema drift between hand-written doc comments and generated manifests | `make generate-manifests` is run after every type/doc-comment change; manifests are never hand-edited |

**Disable path**: set `supports_telco_vm_service_api` to `false` on the Supervisor. The webhook reverts to rejecting `spec.resources`/`cpuAdvanced`/`memoryAdvanced` outright; the schema-upgrade backfill stops running for VMs that haven't already been backfilled; existing values already written to a VM's spec are preserved but no longer reconciled against the live config until the capability is re-enabled.
