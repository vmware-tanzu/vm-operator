# E2E Test Plan: NIC ExtraConfig Reconciliation

- **Epic**: vmop-3782
- **Tasks**: [`tasks.md`](./tasks.md)

> `spec.md`, `plan.md`, and `model.md` for the underlying `networkextraconfig`
> reconciler are not written yet. This document specifies the E2E acceptance
> criteria directly; fold it under a `spec.md`'s "User stories" section when
> that spec is authored, rather than duplicating the scenario tables.

This document specifies the end-to-end test suite that guards per-NIC
ExtraConfig and device-spec reconciliation on a real Supervisor: the set of
scenarios the `networkextraconfig` reconciler
(`pkg/vmconfig/networkextraconfig/`) and its
`VirtualMachineNetworkConfigSynced` condition must satisfy, grouped by the
capability each one guards.

## Suite

- `VMNICExtraConfigSpec`, in `test/e2e/vmservice/vmservice/virtualmachine/vm_nic_extra_config.go`, registered from `test/e2e/vmservice/vmservice_test.go` under a `Context("VM-NIC-EXTRA-CONFIG", ...)` block.
- Every scenario carries `"experimental"` (required for new/changed scenarios per `e2e-testing.md` until validated against a real Supervisor) plus either `"core-functional"` or `"extended-functional"` — the latter for scenarios that exercise DeviceChange fields gated by hardware/firmware prerequisites (`uptv2Enabled`, `vnumaNodeID`) and are more disruptive to set up. There is no additional feature-scoped label (e.g. `"nic-extra-config"`): `export TEST_FOCUS="VM-NIC-EXTRA-CONFIG"` already selects just this suite by matching the `Context` name, so a redundant label would be one more spot to keep in sync (see review discussion on [PR #1688](https://github.com/vmware-tanzu/vm-operator/pull/1688), which applies the same reasoning to the sibling VM-level ExtraConfig suite).
- The suite requires one new wait-interval key in `test/e2e/vmservice/config/wcp.yaml`: `default/wait-vm-nic-extra-config-synced` (5m/10s). Power-state waits reuse the existing `default/wait-virtual-machine-powerstate` interval — do not add a second interval key for that.
- The suite requires a capability constant in `test/e2e/vmservice/consts/consts.go`: `TelcoVMServiceAPICapabilityName = "supports_telco_vm_service_api"` (shared with the `spec.advanced` VM-level ExtraConfig suite when both exist in the same tree).
- Each scenario MUST create its own VM (single VMXNet3 interface named `eth0`) and clean it up independently (`DeferCleanup`), so every `It` block is independently runnable via `TEST_FOCUS`/`LABEL_FILTER`. Exception: the two DeviceChange scenarios (`uptv2Enabled`, `vnumaNodeID`) each create a second, throwaway VM to exercise the prerequisite-failure path first; that VM is deleted inline (not via `DeferCleanup`) before the happy-path VM is created, since it must not outlive the assertion that provoked it.
- Spec patches MUST retry the get-mutate-update sequence on any error, including `409 Conflict` when the reconciler's own write races the test's read-modify-write. Waits for the condition to reach a target status/reason MUST accept an optional `LastTransitionTime` sentinel and require the new transition to be strictly after it — otherwise a second assertion for the same status/reason (e.g. `PowerCyclePending` after a second power cycle) can pass against a stale, un-refreshed condition from the first transition.

## Background

The `networkextraconfig` reconciler operates on `spec.network.interfaces[i]`
and resolves changes across three mechanism classes, each with distinct
power-state semantics:

1. **ExtraConfig — live and PowerCycle mode.** Written via `ReconfigVM_Task`
   regardless of power state. Live-mode keys (`coalescingScheme`,
   `coalescingParams`) take effect immediately. PowerCycle-mode keys
   (`ctxPerDev`, `rssOffloadEnabled`, `udpRssEnabled`, `pnicFeatures`) are
   written immediately but the reconciler also injects
   `vmx.reboot.powerCycle=TRUE` when the VM is powered on, and the condition
   reports `PowerCyclePending` until the next power cycle clears the flag.
2. **DeviceChange — hot-pluggable vs. poweroff-required.** `uptv2Enabled`
   applies immediately (subject to `spec.memoryAdvanced.reservationLockedToMax`
   and `minHardwareVersion ≥ 20`). `vnumaNodeID` requires the VM to be
   powered off (subject to EFI firmware and `minHardwareVersion ≥ 20`); while
   the VM is on, the condition reports `PowerOffRequired`.
3. **AdvancedProperties bag.** Arbitrary VMX keys tracked per-device via
   `vmservice.nic.ethernetX.managedKeys`. Removing a key from spec emits an
   empty-value ExtraConfig entry to clear it in vSphere and updates the
   managed-keys tracking entry — the key MUST NOT reappear in
   `status.extraConfig` once cleared.

Prerequisite gates surface as `PrerequisiteNotMet` and take priority over
`PowerOffRequired`/`PowerCyclePending`. The condition is computed by
`reconcileStatusNetworkExtraConfig`
(`pkg/providers/vsphere/vmlifecycle/update_status.go`) as part of status
reconciliation, fresh from the VM's confirmed vSphere state on every
reconcile — not by `networkextraconfig.OnResult`, which only ever marks the
condition `False`/`NetworkConfigError` on a genuine Reconfigure task failure;
every other case below is decided independently of whether `OnResult` runs in
the same pass:

```
len(Blocked) > 0             → False / PrerequisiteNotMet
len(BlockedPowerOff) > 0     → False / PowerOffRequired
PowerCyclePending flag set   → False / PowerCyclePending
observed == desired          → True
otherwise (pending, but      → False / NetworkConfigMismatch
  neither blocked nor
  power-cycle — e.g. a bag
  key just added; resolves
  to True once the next
  reconcile observes it)
```

## Gating

The suite MUST skip entirely when the `supports_telco_vm_service_api`
Supervisor capability is disabled — the reconciler's `Reconcile`,
`OnResult`, and `reconcileStatusNetworkExtraConfig` are all no-ops without it
(`pkgcfg.FromContext(ctx).Features.TelcoVMServiceAPI`), so there is nothing
to observe.

## Scenarios

### Capability: live-mode ExtraConfig applies immediately, independent of power state and concurrent disk promotion

| Scenario | Verifies |
|----------|----------|
| creates VM with `coalescingScheme=Disabled`, then patches to `coalescingScheme=Static` with `coalescingParams=32` | On create, the live-mode field lands in `status.extraConfig["ethernet0.coalescingScheme"]` and `NetworkConfigSynced` reaches `True` without a power cycle; a subsequent patch that sets both `coalescingScheme` and `coalescingParams` together lands both keys simultaneously (`static`/`32`), exercising the raw string pass-through encoding for `coalescingParams` |
| applies a live-mode field update while `spec.promoteDisksMode=Online` | `NetworkConfigSynced` reaches `True` and the field is present in `status.extraConfig` even with the disk-promotion (SvMotion) reconciler enabled on the same VM — no ordering or resource-conflict regression between the two reconcilers; `spec.promoteDisksMode` remains `Online` after the NIC ExtraConfig reconcile |

### Capability: PowerCycle-mode fields defer to `PowerCyclePending`, wire values encode both directions correctly

| Scenario | Verifies |
|----------|----------|
| sets all four PowerCycle-mode fields (`ctxPerDev`, `rssOffloadEnabled`, `udpRssEnabled`, `pnicFeatures`) to positive values on a running VM, power-cycles, then flips two of them to negative values and power-cycles again | Patching the four fields together sets `NetworkConfigSynced=False/PowerCyclePending` immediately (values are written to vSphere but not yet active); after power-off/power-on all four wire values are visible simultaneously in `status.extraConfig` (`ctxPerDev="3"`, `rssoffload="TRUE"`, `udpRSS="1"`, `pnicFeatures="4"`); flipping `rssOffloadEnabled=false` and `udpRssEnabled=Disabled` re-enters `PowerCyclePending`, and a second power cycle confirms the negative-branch encodings (`rssoffload="FALSE"`, `udpRSS="2"` — the custom `UDPRSSMode` encoder, not the standard bool encoder) without disturbing the two fields left unchanged (`ctxPerDev`, `pnicFeatures`) |

Concrete spec values that reach the wire values above: `ctxPerDev=TxContextThreadingModePerQueue` → `"3"`; `pnicFeatures=[PNICQueueFeatureReceiveSideScaling]` → `"4"`; `udpRssEnabled=UDPRSSModeEnabled`/`UDPRSSModeDisabled` → `"1"`/`"2"`; `rssOffloadEnabled=true`/`false` → `"TRUE"`/`"FALSE"`. These are the specific enum members the test must pick — other members of the same enums encode to different wire strings, so re-deriving the scenario without pinning them will not reproduce the asserted values.

### Capability: AdvancedProperties bag keys are added and garbage-collected

| Scenario | Verifies |
|----------|----------|
| adds an `advancedProperties` bag key, then clears it | Adding `{key: "innerRSS", value: "TRUE"}` surfaces `ethernet0.innerRSS=TRUE` in `status.extraConfig`; clearing `advancedProperties` to empty causes the reconciler to emit an empty-value clear and drop the managed-keys entry, and the key MUST be absent from `status.extraConfig` afterward — not merely left stale |

### Capability: DeviceChange fields are gated by prerequisites and, when met, apply per their power-state mode

| Scenario | Verifies |
|----------|----------|
| sets `uptv2Enabled=true` without `spec.memoryAdvanced.reservationLockedToMax`, then again with it set | Without full memory reservation, the change is rejected either at admission (webhook) or reconciled to `NetworkConfigSynced=False/PrerequisiteNotMet` — both are valid terminal outcomes and the test accepts either; with `reservationLockedToMax=true` and `minHardwareVersion=20`, the condition reason is never `PrerequisiteNotMet` (hot-plug applies immediately; final state may vary by hardware) |
| sets `vnumaNodeID=1` on a VM without EFI firmware, then again on a VM with EFI firmware and `vnumaNodeCount=2` | Without EFI firmware, the reconciler reports `NetworkConfigSynced=False/PrerequisiteNotMet` with `status.conditions[].message` containing the exact string `"EFI firmware required"` (asserted via substring match, not just the `Reason`); with EFI firmware and hardware version ≥ 20, setting `vnumaNodeID` on a running VM instead defers with `PowerOffRequired`, and powering off applies it — `NetworkConfigSynced` reaches `True` |

The happy-path VM for `vnumaNodeID` must set all of `minHardwareVersion=20`, `bootOptions.firmware=EFI`, `cpuAdvanced.topology.vnumaNodeCount=2`, and `cpuAdvanced.topology.coresPerSocket=1` together — omitting `coresPerSocket` or using a hardware version other than exactly `20` does not reliably unlock the prerequisite in the current reconciler (`vmopv1util.FirmwareIsEFI`, `vmopv1util.GetVNUMANodeCount`). After powering off, verify `vnumaNodeID=1` two ways: (1) `status.network.interfaces[i].vnumaNodeID` on the VM object, and (2) independently via govmomi — retrieve `config.hardware.device`, find the `VirtualVmxnet3` device, and assert its `NumaNode` field equals `1`. Fetching the device list without checking `NumaNode` is not a real verification of NUMA placement.

## Scope boundaries (not E2E-tested by design or as a follow-up)

- Resilience to VM recreation and out-of-band vSphere drift (VM deleted/recreated with an identical spec, out-of-band power-off, out-of-band VM destroy) is not exercised here, unlike the `spec.advanced` VM-level ExtraConfig suite. This reconciler's per-device matching (`defaultNICMatcher`) has not been proven stable across a device re-add; this is a gap to close in a follow-up task, not a deliberate scope exclusion — call it out explicitly rather than letting it read as covered.
- `AdvancedProperties` entries that shadow a first-class VMXNet3 key are rejected by the reconciler (`IsFirstClassVMXnet3NICKey`) but this rejection path has no E2E scenario; it is exercised at the unit layer only.
- Webhook-level rejection of NIC fields (as opposed to reconciler-level `PrerequisiteNotMet`) is not asserted precisely — It "sets `uptv2Enabled`..." accepts either outcome. A dedicated webhook unit test, not this suite, should pin down which layer enforces the memory-reservation prerequisite.
- Per `e2e-testing.md`, new/changed E2E scenarios must carry `Label("experimental", ...)` until validated against a real Supervisor; every scenario in this suite carries it. It must be dropped only after a real-Supervisor run passes (see `tasks.md` T016) — do not close the story before then.
