# E2E Test Plan: VM ExtraConfig Reconciliation

- **Epic**: vmop-3782
- **Tasks**: [`tasks.md`](./tasks.md)

> `spec.md`, `plan.md`, and `model.md` for this feature are not written yet. This
> document specifies the E2E acceptance criteria directly; fold it under a
> `spec.md`'s "User stories" section when that spec is authored, rather than
> duplicating the scenario tables.

This document specifies the end-to-end test suite that guards `spec.advanced`
ExtraConfig reconciliation on a real Supervisor: the set of scenarios the
reconciler and its `VirtualMachineExtraConfigSynced` condition must satisfy,
grouped by the capability each one guards.

## Suite

- `VMExtraConfigSpec`, in `test/e2e/vmservice/vmservice/virtualmachine/vm_extraconfig.go`, registered from `test/e2e/vmservice/vmservice_test.go` under a `Context("VM-EXTRACONFIG", ...)` block.
- Every scenario carries `Label("extraconfig", ...)` plus either `"core-functional"` or `"extended-functional"` — the latter for scenarios that need direct `govmomi` access to vSphere (out-of-band power-off, out-of-band destroy) and are more disruptive to the test VM's lifecycle.
- The suite requires one new wait-interval key in `test/e2e/vmservice/config/wcp.yaml`: `default/wait-vm-extraconfig-synced` (5m/10s), used for every wait on the `ExtraConfigSynced` condition. Power-state transitions reuse the existing `default/wait-virtual-machine-powerstate` key (via `vmoperator.WaitForVirtualMachinePowerState`/`UpdateVirtualMachinePowerState`) — do not add a second, extraconfig-specific power-state interval; those helpers hardcode the shared key and are used by many other suites.
- The suite requires a capability constant in `test/e2e/vmservice/consts/consts.go`: `TelcoVMServiceAPICapabilityName = "supports_telco_vm_service_api"`.
- Each scenario MUST create its own VM and clean it up independently (`DeferCleanup`), so every `It` block is independently runnable via `TEST_FOCUS`/`LABEL_FILTER`.
- Unless a scenario is specifically testing interaction with another reconciler, every VM it creates MUST set `spec.promoteDisksMode: Disabled` and `spec.bootstrap.disabled: true`. This keeps disk-promotion (SvMotion) and cloud-init customization from racing with or delaying ExtraConfig reconciliation, which would make `ExtraConfigSynced` timing non-deterministic. The one deliberate exception is the disk-promotion-concurrency scenario below, which leaves `promoteDisksMode` at its default (`Online`) specifically to exercise that race.

## VMX key reference

First-class `spec.advanced` fields surface in `status.extraConfig` under the VMX key their `vmxmode`/vmx struct tag maps to, not their Go field name. Assertions against `status.extraConfig` MUST use these literal keys (mirror the vmx struct tags in the active `api/v1alphaN` `VirtualMachineAdvancedSpec`; verify against source if the API version has moved on):

| `spec.advanced` field | VMX key | Mode |
|---|---|---|
| `preferHtEnabled` | `numa.vcpu.preferHT` | PowerCycle |
| `timeTrackerLowLatencyEnabled` | `timeTracker.lowLatency` | PowerCycle |
| `cpuAffinityExclusiveNoStatsEnabled` | `sched.cpu.affinity.exclusiveNoStats` | PowerCycle |
| `vmxSwapEnabled` | `sched.swap.vmxSwapEnabled` | PowerCycle |
| `pnumaNodeAffinity` (`[]int32`, comma-separated encoding) | `numa.nodeAffinity` | PowerCycle |
| `hugePages1GEnabled` | `sched.mem.lpage.enable1GPage` | PowerOff |

The condition under test is `vmopv1.VirtualMachineExtraConfigSynced`, with reasons `"PowerCyclePending"` and `"PowerOffRequired"` (mirror the reconciler's condition-reason constants; verify against source if renamed).

## Gating

The suite MUST skip entirely when the `supports_telco_vm_service_api` Supervisor capability is disabled — `spec.advanced`'s first-class VMX fields are inert without it, so there is nothing to observe.

## Scenarios

### Capability: applying first-class VMX fields and bag keys, reflected in status

| Scenario | Verifies |
|----------|----------|
| creates VM with PowerCycle-mode first-class fields and bag keys, syncs immediately, reflects bag key CRUD | On create, first-class VMX fields (`preferHtEnabled`, `timeTrackerLowLatencyEnabled`, `cpuAffinityExclusiveNoStatsEnabled`, `vmxSwapEnabled`) and `spec.advanced.extraConfig` bag keys land in `status.extraConfig` and `ExtraConfigSynced` reaches `True`; a subsequent patch that adds a key, updates a key's value, and omits a previously-set key is reflected in `status.extraConfig` — the omitted key is removed, not merely left stale |
| applies extraConfig correctly while disk promotion runs concurrently (default `PromoteDisksMode=Online`) | `ExtraConfigSynced` reaches `True` and every first-class/bag key is present even when the disk-promotion (SvMotion) reconciler may still be in flight on the same VM — no ordering or resource-conflict regression between the two reconcilers |

### Capability: PowerCycle-mode vs PowerOff-mode field semantics, surfaced via `ExtraConfigSynced`

| Scenario | Verifies |
|----------|----------|
| marks `PowerCyclePending` when a PowerCycle-mode field changes while the VM is powered on | Flipping a `vmxmode:"powercycle"` field (e.g. `vmxSwapEnabled`) on a running VM applies the value immediately but sets `ExtraConfigSynced=False/PowerCyclePending`; a manual power-off then power-on clears the condition to `True` and the new value is confirmed in `status.extraConfig` |
| defers a PowerOff-mode field while the VM is powered on, applies it after power-off | Adding a `vmxmode:"poweroff"` field (e.g. `hugePages1GEnabled`) to a running VM sets `ExtraConfigSynced=False/PowerOffRequired`, names the deferred VMX key in the condition message, and the key MUST NOT appear in `status.extraConfig` while deferred; powering the VM off applies it, and every other first-class/bag key remains intact afterward |
| `PowerOffRequired` takes priority over `PowerCyclePending` when a PowerOff-mode and a PowerCycle-mode field change simultaneously | Patching both a `poweroff`-mode field and a `powercycle`-mode field in the same update yields `ExtraConfigSynced=False/PowerOffRequired` — never `PowerCyclePending` — and the message names the PowerOff-mode field |
| handles the `[]int32` first-class field (`pnumaNodeAffinity`) correctly | A comma-separated encoding (e.g. `"0"`) is visible in `status.extraConfig` after create; clearing the field to `nil` on a running VM is treated as a PowerCycle-mode change (`PowerCyclePending`), and the key is absent from `status.extraConfig` once the clear is applied after a power-off |

### Capability: resilience — extraConfig survives VM recreation and out-of-band vSphere drift

| Scenario | Verifies |
|----------|----------|
| re-applies all extraConfig keys when the VM is deleted via Kubernetes and recreated with an identical spec | First-class fields and bag keys are fully re-applied from spec on the recreated VM — no key is silently skipped because it was already applied to the previous underlying vSphere VM |
| resolves `PowerCyclePending` after an out-of-band vSphere power-off | Powering the VM off directly via `govmomi` (bypassing VM Operator) is detected, the pending PowerCycle-mode change applies while the VM is off, `ExtraConfigSynced` reaches `True`, and the operator's normal drift recovery restores `spec.powerState`'s desired `PoweredOn` state afterward — with extraConfig still intact post-recovery |
| re-applies extraConfig after the vSphere VM is destroyed out-of-band | Destroying the underlying vSphere VM directly (the Kubernetes object survives) and then toggling `spec.powerState` to `PoweredOn` MUST cause the operator to recreate the VM (`status.uniqueID` changes) and re-apply every first-class/bag key from spec on the new VM — not silently leave the object orphaned or extraConfig-less |

## Scope boundaries (not E2E-tested by design or as a follow-up)

- NIC-level network ExtraConfig (`pkg/vmconfig/networkextraconfig`, reconciling `spec.network.interfaces[].vmxnet3` VMX properties) has no E2E coverage in this suite today. It is exercised at the unit/integration layer only. Per `e2e-sync-with-changes.md`, this is a gap to close in a follow-up spec/task, not a deliberate scope exclusion — call it out explicitly rather than letting it read as covered.
- The `supports_telco_vm_service_api` webhook-rejection behavior when the capability is disabled is not exercised here; this suite's whole-suite skip (see "Gating" above) is the only E2E-level signal for the disabled case. Rejection itself belongs at the webhook unit-test layer.
