# NIC ExtraConfig E2E Test Plan

**Date:** 2026-06-29
**Feature:** NIC ExtraConfig reconciler (`pkg/providers/vsphere/network/extraconfig/`)
**Reference:** `specs/scripts/test-nic-extraconfig-reconciler.py` (7-phase Python validation script)

---

## 1. Goal

Add Ginkgo-based E2E tests for the NIC ExtraConfig reconciler that validate the same 7-phase scenarios as the Python script, integrated into the existing `test/e2e/vmservice/` suite. Tests must be runnable with `make test-e2e TEST_FOCUS="VM-NIC-EXTRA-CONFIG"` or `make e2e-core LABEL_FILTER="nic-extra-config"`.

Every first-class field in `VirtualMachineNetworkInterfaceVMXNet3Spec` and `vnumaNodeID` must have at least one explicit wire-value assertion in the test suite.

---

## 2. Background

### Reconciler responsibility

The NIC ExtraConfig reconciler operates on `spec.network.interfaces[i]` and resolves changes across four mechanism classes:

1. **Live-mode ExtraConfig** — applied immediately via `ReconfigVM_Task` regardless of VM power state:
   - `interfaces[i].vmxnet3.coalescingScheme` → `ethernetX.coalescingScheme`
   - `interfaces[i].vmxnet3.coalescingParams` → `ethernetX.coalescingParams`

2. **PowerCycle-mode ExtraConfig** — written to vSphere immediately; guest must power-cycle for the change to take effect; reconciler injects `vmx.reboot.powerCycle=TRUE`:
   - `interfaces[i].vmxnet3.ctxPerDev` → `ethernetX.ctxPerDev`
   - `interfaces[i].vmxnet3.rssOffloadEnabled` → `ethernetX.rssoffload`
   - `interfaces[i].vmxnet3.udpRssEnabled` → `ethernetX.udpRSS`
   - `interfaces[i].vmxnet3.pnicFeatures` → `ethernetX.pnicfeatures`

3. **DeviceChange — hot-pluggable** (requires `minHardwareVersion ≥ 20`):
   - `interfaces[i].vmxnet3.uptv2Enabled` → `VirtualVmxnet3.Uptv2Enabled`
   - Prerequisite: `spec.memoryAdvanced.reservationLockedToMax = true`

4. **DeviceChange — poweroff-required** (requires `minHardwareVersion ≥ 20`, EFI firmware, and `vnumaNodeCount > 0`):
   - `interfaces[i].vnumaNodeID` → `VirtualDevice.NumaNode`

5. **AdvancedProperties bag** — arbitrary VMX keys tracked via `vmservice.nic.ethernetX.managedKeys`:
   - `interfaces[i].advancedProperties[{key, value}]` → `ethernetX.<key>=<value>`
   - GC: when a key is removed from spec, reconciler emits `ethernetX.<key>=""` to clear it in vSphere

### VMX wire values (sourced from `pkg/util/vmopv1/extraconfig.go`)

| Spec field | Spec value | VMX key | VMX wire value |
|---|---|---|---|
| `coalescingScheme` | `Disabled` | `ethernetX.coalescingScheme` | `"disabled"` |
| `coalescingScheme` | `Adapt` | `ethernetX.coalescingScheme` | `"adapt"` |
| `coalescingScheme` | `Static` | `ethernetX.coalescingScheme` | `"static"` |
| `coalescingScheme` | `RateBasedCoalescing` | `ethernetX.coalescingScheme` | `"rbc"` |
| `coalescingParams` | `"32"` | `ethernetX.coalescingParams` | `"32"` (raw string pass-through) |
| `ctxPerDev` | `PerDevice` | `ethernetX.ctxPerDev` | `"1"` |
| `ctxPerDev` | `PerVM` | `ethernetX.ctxPerDev` | `"2"` |
| `ctxPerDev` | `PerQueue` | `ethernetX.ctxPerDev` | `"3"` |
| `rssOffloadEnabled` | `true` | `ethernetX.rssoffload` | `"TRUE"` (standard `*bool` encoding) |
| `udpRssEnabled` | `UDPRSSModeEnabled` | `ethernetX.udpRSS` | `"1"` (custom integer encoder) |
| `pnicFeatures` | `["ReceiveSideScaling"]` | `ethernetX.pnicfeatures` | `"4"` (bitmask bit 2) |
| `pnicFeatures` | `["LargeReceiveOffload"]` | `ethernetX.pnicfeatures` | `"1"` (bitmask bit 0) |
| `pnicFeatures` | `["LargeReceiveOffload","ReceiveSideScaling"]` | `ethernetX.pnicfeatures` | `"5"` (bits 0+2) |

### Condition lifecycle

```
Spec patch → ReconfigVM_Task submitted
  → OnResult (live-mode ExtraConfig):    condition True  (vmx.reboot.powerCycle absent)
  → OnResult (powercycle-mode):          condition False / PowerCyclePending while flag set
                                         condition True  once ESXi clears flag after power cycle
  → OnResult (DeviceChange hot-plug):    condition True  (applied immediately on vmx-20+)
  → OnResult (DeviceChange poweroff):    condition False / PowerOffRequired while VM is powered on
                                         condition True  after VM powers off and config applied
  → OnResult (prereq not met):           condition False / PrerequisiteNotMet
```

### Feature gate

The reconciler is guarded by the `TelcoVMServiceAPI` supervisor capability. Every `BeforeEach` calls `skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.TelcoVMServiceAPICapabilityName)`.

---

## 3. Files to Create / Modify

| File | Action | Purpose |
|------|---------|---------|
| `test/e2e/vmservice/consts/consts.go` | **Modify** | Add `TelcoVMServiceAPICapabilityName` constant (skip if already added by the compute-config PR) |
| `test/e2e/vmservice/config/wcp.yaml` | **Modify** | Add NIC-extra-config-specific polling interval keys |
| `test/e2e/vmservice/vmservice/virtualmachine/vm_nic_extra_config.go` | **Create** | All 5 `It` blocks + helpers |
| `test/e2e/vmservice/vmservice_test.go` | **Modify** | Wire in `Context("VM-NIC-EXTRA-CONFIG", ...)` |

---

## 4. New Constant

**`test/e2e/vmservice/consts/consts.go`** — append (skip if already present from a sibling PR):

```go
TelcoVMServiceAPICapabilityName = "supports_telco_vm_service_api"
```

---

## 5. New YAML Intervals

**`test/e2e/vmservice/config/wcp.yaml`** — append under `intervals:`:

```yaml
  default/wait-vm-nic-extra-config-synced:     ["5m", "10s"]
  default/wait-vm-nic-extra-config-powerstate: ["3m", "10s"]
```

---

## 6. Test File Structure

### Package and file

```
test/e2e/vmservice/vmservice/virtualmachine/vm_nic_extra_config.go
package virtualmachine
```

Same package as `vm_hardware.go`, `vm_compute_config.go`, etc.

### VMX wire value constants

```go
const (
    // Live-mode ExtraConfig wire values (ethernetX.coalescingScheme).
    vmxCoalescingDisabled = "disabled" // CoalescingSchemeDisabled
    vmxCoalescingAdapt    = "adapt"    // CoalescingSchemeAdapt
    vmxCoalescingStatic   = "static"   // CoalescingSchemeStatic
    vmxCoalescingRBC      = "rbc"      // CoalescingSchemeRateBasedCoalescing

    // PowerCycle-mode ExtraConfig wire values.
    vmxCtxPerDevice = "1"    // TxContextThreadingModePerDevice → ethernetX.ctxPerDev
    vmxCtxPerVM     = "2"    // TxContextThreadingModePerVM
    vmxCtxPerQueue  = "3"    // TxContextThreadingModePerQueue

    vmxRSSOffloadEnabled = "TRUE" // rssOffloadEnabled=true → ethernetX.rssoffload
    vmxRSSOffloadFalse   = "FALSE"

    vmxUDPRSSEnabled  = "1" // UDPRSSModeEnabled → ethernetX.udpRSS (custom integer encoder)
    vmxUDPRSSDisabled = "2" // UDPRSSModeDisabled

    // pnicFeatures bitmask wire values (ethernetX.pnicfeatures).
    vmxPNICFeatureLRO        = "1" // LargeReceiveOffload (bit 0)
    vmxPNICFeatureRSS        = "4" // ReceiveSideScaling  (bit 2)
    vmxPNICFeatureLROAndRSS  = "5" // LRO + RSS           (bits 0+2)

    // Interface name used throughout.
    nicName = "eth0"
)
```

### `VMNICExtraConfigSpecInput` struct

```go
type VMNICExtraConfigSpecInput struct {
    ClusterProxy     wcpframework.WCPClusterProxyInterface
    Config           *e2eConfig.E2EConfig
    WCPClient        wcp.WorkloadManagementAPI
    ArtifactFolder   string
    SkipCleanup      bool
    WCPNamespaceName string
}
```

### `VMNICExtraConfigSpec` function skeleton

```go
func VMNICExtraConfigSpec(ctx context.Context, inputGetter func() VMNICExtraConfigSpecInput) {
    const specName = "vm-nic-extra-config"

    var (
        input           VMNICExtraConfigSpecInput
        config          *e2eConfig.E2EConfig
        clusterProxy    *common.VMServiceClusterProxy
        svClusterClient ctrlclient.Client
        vimClient       *vim25.Client   // govmomi; used in It blocks 7.4 and 7.5
        vmNamespace     string
        linuxVMIName    string
        storageClass    string
        vmClassName     string
    )

    BeforeEach(func() {
        input = inputGetter()
        // validate non-nil inputs...
        skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)
        skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy,
            consts.TelcoVMServiceAPICapabilityName)

        config           = input.Config
        clusterProxy     = input.ClusterProxy.(*common.VMServiceClusterProxy)
        svClusterClient  = clusterProxy.GetClient()
        vmNamespace      = input.WCPNamespaceName
        storageClass     = config.InfraConfig.ManagementClusterConfig.Resources.StorageClassName
        vmClassName      = config.InfraConfig.ManagementClusterConfig.Resources.VMClassName
        vimClient        = vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
        DeferCleanup(vcenter.LogoutVimClient, vimClient)

        var vmiErr error
        linuxVMIName, vmiErr = vmoperator.WaitForVirtualMachineImageName(
            ctx, &config.Config, svClusterClient, vmNamespace,
            config.InfraConfig.ManagementClusterConfig.Resources.PhotonImageDisplayName)
        Expect(vmiErr).NotTo(HaveOccurred())
    })

    // It blocks — see §7
}
```

### Wire into `vmservice_test.go`

```go
Context("VM-NIC-EXTRA-CONFIG", func() {
    virtualmachine.VMNICExtraConfigSpec(context.TODO(), func() virtualmachine.VMNICExtraConfigSpecInput {
        return virtualmachine.VMNICExtraConfigSpecInput{
            ClusterProxy:     svClusterProxy,
            Config:           config,
            WCPClient:        wcpClient,
            ArtifactFolder:   artifactFolder,
            SkipCleanup:      skipCleanup,
            WCPNamespaceName: wcpNamespaceName,
        }
    })
})
```

---

## 7. Test Cases (`It` blocks)

All `It` blocks share the `BeforeEach` guard (capability check + infra check). Each creates its own VM and cleans up via `DeferCleanup`.

---

### 7.1 `"creates VM with live-mode ExtraConfig and verifies both live-mode fields after update"` — Phases 1+2

**Labels:** `nic-extra-config`, `core-functional`

**Fields under test:** `coalescingScheme` (all schemes), `coalescingParams`

**Phase 1 setup:**
- Create `vmopv1a6.VirtualMachine` with:
  - `spec.network.interfaces[0].name = "eth0"`, `type = VMXNet3`
  - `spec.network.interfaces[0].vmxnet3.coalescingScheme = "Disabled"`
  - `spec.bootstrap.disabled = true`, `spec.powerState = PoweredOn`
- Wait for `VirtualMachineCreated=True`
- Composite wait: `VirtualMachineNetworkConfigSynced=True` AND `status.extraConfig["ethernet0.coalescingScheme"]=="disabled"` (5 m)

**Phase 1 assertions:**
```
VirtualMachineNetworkConfigSynced.Status = True
status.extraConfig["ethernet0.coalescingScheme"] = "disabled"
```

**Phase 2 steps — coalescingScheme=Static + coalescingParams (live-mode, applies immediately):**
- Patch `spec.network.interfaces[0].vmxnet3.coalescingScheme = "Static"` + `coalescingParams = "32"`
- Composite wait: `VirtualMachineNetworkConfigSynced=True` AND `status.extraConfig["ethernet0.coalescingScheme"]=="static"` AND `status.extraConfig["ethernet0.coalescingParams"]=="32"` (5 m)

**Phase 2 assertions:**
```
VirtualMachineNetworkConfigSynced.Status = True
status.extraConfig["ethernet0.coalescingScheme"] = "static"
status.extraConfig["ethernet0.coalescingParams"] = "32"
```

> `coalescingParams` is only meaningful for `Static` (`packet queue limit`, range 1–64) and `RateBasedCoalescing` (`interrupts/sec`). `Static` with `"32"` exercises the raw string pass-through encoding and confirms both fields land in `status.extraConfig` simultaneously.
> Live-mode fields call `ReconfigVM_Task` immediately regardless of power state. Condition stays True throughout.

---

### 7.2 `"sets PowerCyclePending for powercycle-mode fields and verifies positive and negative wire values across two power cycles"` — Phases 3+4+5

**Labels:** `nic-extra-config`, `core-functional`

**Fields under test:** `ctxPerDev`, `rssOffloadEnabled` (`true`→`"TRUE"` and `false`→`"FALSE"`), `udpRssEnabled` (`Enabled`→`"1"` and `Disabled`→`"2"`), `pnicFeatures`

**Setup:**
- Create VM with `coalescingScheme = "Adapt"` (live-mode baseline, establishes condition=True)
- Wait for `VirtualMachineCreated=True` and `VirtualMachineNetworkConfigSynced=True`

**Phase 3 steps — set ALL four powercycle-mode fields while powered on:**
- Patch `spec.network.interfaces[0].vmxnet3`:
  - `ctxPerDev = "PerQueue"`
  - `rssOffloadEnabled = true`
  - `udpRssEnabled = UDPRSSModeEnabled` (true)
  - `pnicFeatures = ["ReceiveSideScaling"]`
- Wait for `VirtualMachineNetworkConfigSynced=False / PowerCyclePending` (5 m)

**Phase 3 assertions:**
```
VirtualMachineNetworkConfigSynced.Status = False
VirtualMachineNetworkConfigSynced.Reason = "PowerCyclePending"
VirtualMachineNetworkConfigSynced.Message contains "power cycle"
```

> The reconciler writes all four ExtraConfig entries via a single `ReconfigVM_Task`, injects `vmx.reboot.powerCycle=TRUE`, and sets the condition False/PowerCyclePending. All four entries are written even though the guest has not yet cycled.

**Phase 4 steps — power cycle clears the condition:**
1. Patch `spec.powerState = PoweredOff`; wait for `status.powerState = PoweredOff` (3 m)
2. Patch `spec.powerState = PoweredOn`; wait for `status.powerState = PoweredOn` (5 m)
3. Composite wait: `VirtualMachineNetworkConfigSynced=True` AND `status.extraConfig["ethernet0.ctxPerDev"]=="3"` (5 m)

**Phase 4 assertions:**
```
VirtualMachineNetworkConfigSynced.Status = True
status.extraConfig["ethernet0.ctxPerDev"]   = "3"     (PerQueue wire value)
status.extraConfig["ethernet0.rssoffload"]  = "TRUE"  (rssOffloadEnabled=true; standard *bool)
status.extraConfig["ethernet0.udpRSS"]      = "1"     (UDPRSSModeEnabled; custom integer encoder)
status.extraConfig["ethernet0.pnicfeatures"]= "4"     (ReceiveSideScaling = bitmask bit 2)
```

> After the power cycle ESXi clears `vmx.reboot.powerCycle`. The next `OnResult` sees no pending flag → condition True. All four wire values must be present simultaneously — asserting them together in a single `Eventually` block guards against partial apply.

**Phase 5 steps — flip rssOffloadEnabled and udpRssEnabled to negative values:**
- Patch `spec.network.interfaces[0].vmxnet3`:
  - `rssOffloadEnabled = false`
  - `udpRssEnabled = UDPRSSModeDisabled` (false)
  - (leave `ctxPerDev` and `pnicFeatures` unchanged)
- Wait for `VirtualMachineNetworkConfigSynced=False / PowerCyclePending` (5 m)
- Record the condition `lastTransitionTime` as sentinel T1

**Phase 5 intermediate assertions:**
```
VirtualMachineNetworkConfigSynced.Status = False
VirtualMachineNetworkConfigSynced.Reason = "PowerCyclePending"
```

- Patch `spec.powerState = PoweredOff`; wait for `status.powerState = PoweredOff` (3 m)
- Patch `spec.powerState = PoweredOn`; wait for `status.powerState = PoweredOn` (5 m)
- Composite wait: `VirtualMachineNetworkConfigSynced=True` with `lastTransitionTime > T1` AND `status.extraConfig["ethernet0.rssoffload"]=="FALSE"` (5 m)

**Phase 5 assertions:**
```
VirtualMachineNetworkConfigSynced.Status = True
status.extraConfig["ethernet0.rssoffload"]  = "FALSE"  (rssOffloadEnabled=false; standard *bool)
status.extraConfig["ethernet0.udpRSS"]      = "2"      (UDPRSSModeDisabled; custom integer encoder)
status.extraConfig["ethernet0.ctxPerDev"]   = "3"      (unchanged from Phase 4)
status.extraConfig["ethernet0.pnicfeatures"]= "4"      (unchanged from Phase 4)
```

> `udpRSS="2"` is the critical assertion here: the custom `encodeUDPRSSMode` function maps `Disabled`→`"2"`, not `"FALSE"`. A future refactor that accidentally switches to the standard bool encoder would produce `"FALSE"` and break this assertion. `rssoffload="FALSE"` covers the standard `*bool` false branch via `EncodeVMXBoolField`. The sentinel guard on `lastTransitionTime` ensures we are not re-reading the stale `True` from Phase 4.

---

### 7.3 `"manages AdvancedProperties bag lifecycle — adds key and GC-removes key"` — Phase 5

**Labels:** `nic-extra-config`, `core-functional`

**Fields under test:** `advancedProperties` (bag add, bag GC remove)

**Setup:**
- Create VM with `coalescingScheme = "Adapt"`, `ctxPerDev = "PerQueue"`, `rssOffloadEnabled = true`, `udpRssEnabled = UDPRSSModeEnabled`, `pnicFeatures = ["ReceiveSideScaling"]`
- Wait for `VirtualMachineCreated=True`
- Drive through power cycle so all powercycle-mode fields are live (condition=True)

**Step A — add bag key:**
1. Patch `spec.network.interfaces[0].advancedProperties = [{key: "innerRSS", value: "TRUE"}]`
2. Composite wait: `VirtualMachineNetworkConfigSynced=True` AND `status.extraConfig["ethernet0.innerRSS"]=="TRUE"` (5 m)

**Step A assertions:**
```
VirtualMachineNetworkConfigSynced.Status = True
status.extraConfig["ethernet0.innerRSS"] = "TRUE"
```

> Reconciler writes `ethernet0.innerRSS=TRUE` and tracks `"innerRSS"` in `vmservice.nic.ethernet0.managedKeys`.

**Step B — remove bag key (GC):**
1. Patch `spec.network.interfaces[0].advancedProperties = []`
2. Composite wait: `VirtualMachineNetworkConfigSynced=True` AND `"ethernet0.innerRSS"` absent from `status.extraConfig` (5 m)

**Step B assertions:**
```
VirtualMachineNetworkConfigSynced.Status = True
"ethernet0.innerRSS" absent from status.extraConfig
```

> Reconciler reads `managedKeys="innerRSS"`, sees it removed from spec → emits `ethernet0.innerRSS=""` (clear), updates `managedKeys=""`. `OnResult` only surfaces non-empty values in `status.extraConfig`.

---

### 7.4 `"applies UPTv2Enabled hot-pluggable DeviceChange and checks prerequisite gate"` — Phase 6

**Labels:** `nic-extra-config`, `extended-functional`

**Fields under test:** `uptv2Enabled`

**Step A — prerequisite check (uptv2Enabled without memory reservation):**
1. Create VM with `minHardwareVersion = 20`, `coalescingScheme = "Adapt"` (no `reservationLockedToMax`)
2. Wait for `VirtualMachineCreated=True` and `VirtualMachineNetworkConfigSynced=True`
3. Patch `spec.network.interfaces[0].vmxnet3.uptv2Enabled = true`
4. Assert: either the patch is rejected by the admission webhook, OR the reconciler surfaces `VirtualMachineNetworkConfigSynced=False / PrerequisiteNotMet` with message containing "memory reservation"
5. Delete VM

**Step A assertions (accept either guard):**
```
EITHER: svClusterClient.Patch returns non-nil error (webhook blocked it)
OR:     VirtualMachineNetworkConfigSynced.Status = False
        VirtualMachineNetworkConfigSynced.Reason = "PrerequisiteNotMet"
        VirtualMachineNetworkConfigSynced.Message contains "memory reservation"
```

**Step B — happy path (with memory reservation):**
1. Create VM with `minHardwareVersion = 20`, `coalescingScheme = "Adapt"`, `memoryAdvanced.reservationLockedToMax = true`
2. Wait for `VirtualMachineCreated=True` and `VirtualMachineNetworkConfigSynced=True`
3. Patch `spec.network.interfaces[0].vmxnet3.uptv2Enabled = true`
4. Composite wait: condition reason ≠ `PrerequisiteNotMet` (accept True, PowerCyclePending, or Error — hardware-dependent) (5 m)

**Step B assertions:**
```
VirtualMachineNetworkConfigSynced.Reason ≠ "PrerequisiteNotMet"
```

If network backing present (`status.network.interfaces[eth0]` populated):
```
status.network.interfaces["eth0"].vmxnet3.uptv2Enabled = true
```

If govmomi available (vimClient non-nil):
```
vSphere NIC[0].uptv2Enabled = true   (polled via govmomi using status.uniqueID)
```

> On hardware that does not support UPTv2 hot-add the DeviceChange is accepted by the operator but rejected by vSphere; condition becomes `Error`. The test asserts only that `PrerequisiteNotMet` is NOT the outcome — the prereq guard itself is what It 7.4 exercises.

---

### 7.5 `"applies VNUMANodeID poweroff-required DeviceChange and checks prerequisite gates"` — Phase 7

**Labels:** `nic-extra-config`, `extended-functional`

**Fields under test:** `vnumaNodeID`

**Step A — EFI firmware prerequisite check:**
1. Create VM with `minHardwareVersion = 20`, `cpuAdvanced.topology.vnumaNodeCount = 2`, `coalescingScheme = "Adapt"` (no EFI firmware)
2. Wait for `VirtualMachineCreated=True` and `VirtualMachineNetworkConfigSynced=True`
3. Patch `spec.network.interfaces[0].vnumaNodeID = 1`
4. Wait for `VirtualMachineNetworkConfigSynced=False / PrerequisiteNotMet` (5 m)
5. Delete VM

**Step A assertions:**
```
VirtualMachineNetworkConfigSynced.Status = False
VirtualMachineNetworkConfigSynced.Reason = "PrerequisiteNotMet"
VirtualMachineNetworkConfigSynced.Message contains "EFI firmware"
```

> Step A uses `vnumaNodeCount=2` so the vNUMA-count prerequisite passes and only the EFI check fires. Prerequisite order: (1) vNUMA node count > 0, (2) EFI firmware.

**Step B — happy path (EFI + vNUMA + power-off):**
1. Create VM with:
   - `minHardwareVersion = 20`
   - `bootOptions.firmware = "efi"`
   - `cpuAdvanced.topology.coresPerSocket = 1`, `vnumaNodeCount = 2`
   - `coalescingScheme = "Adapt"`
2. Wait for `VirtualMachineCreated=True` and `VirtualMachineNetworkConfigSynced=True`
3. Patch `spec.network.interfaces[0].vnumaNodeID = 1` while VM is powered on
4. Wait for `VirtualMachineNetworkConfigSynced=False / PowerOffRequired` (5 m)

**Intermediate assertions:**
```
VirtualMachineNetworkConfigSynced.Status = False
VirtualMachineNetworkConfigSynced.Reason = "PowerOffRequired"
```

5. Patch `spec.powerState = PoweredOff`; wait for `status.powerState = PoweredOff` (3 m)
6. Wait for `VirtualMachineNetworkConfigSynced=True` with sentinel (5 m)

**Final assertions:**
```
VirtualMachineNetworkConfigSynced.Status = True
```

If `status.network.interfaces[eth0]` populated:
```
status.network.interfaces["eth0"].vnumaNodeID = 1
```

If govmomi available (vimClient non-nil):
```
vSphere NIC[0].numaNode = 1   (via govmomi using status.uniqueID)
```

> `vnumaNodeID` is poweroff-required. Full prerequisite chain: vmx-20 hardware gate → EFI firmware → vnumaNodeCount > 0. Setting it while powered on → `PowerOffRequired`; power-off apply → `True`.

---

## 8. Helper Functions

```go
// waitForNICExtraConfigSynced polls until VirtualMachineNetworkConfigSynced
// reaches wantStatus, optionally matching wantReason.
// Accepts optional sentinel lastTransitionTimes — re-polls until condition
// lastTransitionTime is strictly after the most recent sentinel.
// Uses "default/wait-vm-nic-extra-config-synced" interval from config.
func waitForNICExtraConfigSynced(
    ctx context.Context,
    svClient ctrlclient.Client,
    config *e2eConfig.E2EConfig,
    key types.NamespacedName,
    wantStatus metav1.ConditionStatus,
    wantReason string,
    sentinels ...metav1.Time,
) *metav1.Condition

// waitForNICExtraConfigSyncedWithExtraConfig polls until:
//   VirtualMachineNetworkConfigSynced=True  AND
//   status.extraConfig[ecKey] == wantValue
// Single-key composite guard — use the plural variant when asserting multiple
// keys simultaneously (It 7.2 Phase 4).
func waitForNICExtraConfigSyncedWithExtraConfig(
    ctx context.Context,
    svClient ctrlclient.Client,
    config *e2eConfig.E2EConfig,
    key types.NamespacedName,
    ecKey string,
    wantValue string,
)

// waitForNICExtraConfigSyncedWithExtraConfigs polls until:
//   VirtualMachineNetworkConfigSynced=True  AND
//   all entries in wantExtraConfig match status.extraConfig exactly.
// Used in It 7.2 Phase 4 to assert all four powercycle-mode wire values
// simultaneously, guarding against partial-apply races.
func waitForNICExtraConfigSyncedWithExtraConfigs(
    ctx context.Context,
    svClient ctrlclient.Client,
    config *e2eConfig.E2EConfig,
    key types.NamespacedName,
    wantExtraConfig map[string]string,
)

// waitForNICExtraConfigSyncedWithExtraConfigAbsent polls until:
//   VirtualMachineNetworkConfigSynced=True  AND
//   ecKey is absent from status.extraConfig
func waitForNICExtraConfigSyncedWithExtraConfigAbsent(
    ctx context.Context,
    svClient ctrlclient.Client,
    config *e2eConfig.E2EConfig,
    key types.NamespacedName,
    ecKey string,
)

// statusExtraConfigMap returns vm.Status.ExtraConfig as a string→string map.
// Safe when ExtraConfig is nil.
func statusExtraConfigMap(vm *vmopv1a6.VirtualMachine) map[string]string

// statusNICInterface returns the VirtualMachineNetworkInterfaceStatus entry
// whose Name matches ifaceName. Returns nil when not found.
func statusNICInterface(
    vm *vmopv1a6.VirtualMachine,
    ifaceName string,
) *vmopv1a6.VirtualMachineNetworkInterfaceStatus

// getNICExtraConfigCondition returns the VirtualMachineNetworkConfigSynced
// condition from vm.Status.Conditions. Returns nil when not found.
func getNICExtraConfigCondition(vm *vmopv1a6.VirtualMachine) *metav1.Condition

// buildNICExtraConfigVM constructs a vmopv1a6.VirtualMachine suitable for
// NIC ExtraConfig testing. Always sets bootstrap.disabled=true.
// opts controls all spec fields that vary across It blocks.
func buildNICExtraConfigVM(
    name, namespace, className, imageName, storageClass string,
    opts nicExtraConfigVMOptions,
) *vmopv1a6.VirtualMachine

// nicExtraConfigVMOptions holds per-test VM spec overrides for buildNICExtraConfigVM.
type nicExtraConfigVMOptions struct {
    PowerState                   vmopv1a6.VirtualMachinePowerState
    // Live-mode ExtraConfig
    CoalescingScheme             vmopv1a6.CoalescingScheme
    CoalescingParams             *string
    // PowerCycle-mode ExtraConfig
    CtxPerDev                    vmopv1a6.TxContextThreadingMode
    RSSOffloadEnabled            *bool
    UDPRSSEnabled                *vmopv1a6.UDPRSSMode
    PNICFeatures                 []vmopv1a6.PNICQueueFeature
    // DeviceChange
    UPTv2Enabled                 *bool
    VNUMANodeID                  *int32
    // AdvancedProperties bag
    AdvancedProperties           []vmopv1common.KeyValuePair
    // VM-level prerequisites
    MinHardwareVersion           *int32
    Firmware                     string   // e.g. "efi"
    VNUMANodeCount               *int32
    MemoryReservationLockedToMax *bool
    NetworkName                  string   // backing network; omit if ""
}

// findVSphereVMByMOIDForNIC locates the govmomi VM object by MOID
// (status.uniqueID). Returns nil (logs a warning) when not found.
// Reuse findVSphereVMByMOID from vm_compute_config.go if already present
// in the package; otherwise define here to avoid duplicate.
func findVSphereVMByMOIDForNIC(vimClient *vim25.Client, moid string) *object.VirtualMachine

// assertVSphereNICState reads the first VMXNet3 device on the VM identified by
// moid and runs optional assertions on Uptv2Enabled and NumaNode.
// Skips silently when vimClient is nil or device not found.
func assertVSphereNICState(
    ctx context.Context,
    vimClient *vim25.Client,
    moid string,
    wantUPTv2 *bool,
    wantNumaNode *int32,
)
```

---

## 9. Imports Required

```go
// Standard library
"context"
"fmt"

// Ginkgo / Gomega (dot-import; allowed in this package per .golangci.yml linter exclusions)
. "github.com/onsi/ginkgo/v2"
. "github.com/onsi/gomega"

// govmomi (for It blocks 7.4 and 7.5)
"github.com/vmware/govmomi/object"
"github.com/vmware/govmomi/property"
"github.com/vmware/govmomi/vim25"
"github.com/vmware/govmomi/vim25/mo"
vimtypes "github.com/vmware/govmomi/vim25/types"

// Kubernetes
metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
"k8s.io/apimachinery/pkg/types"
capiutil "sigs.k8s.io/cluster-api/util"
ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

// vmop API
vmopv1a6 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1common"
"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"

// E2E framework
"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
```

---

## 10. TDD Implementation Checklist

Follow TDD: write the `It` block body as assertion skeletons first (compile-only), then implement helpers until the file compiles and the tests pass on a cluster.

1. [ ] Add `TelcoVMServiceAPICapabilityName` to `consts/consts.go` (if absent)
2. [ ] Add NIC-extra-config intervals to `wcp.yaml`
3. [ ] Create `vm_nic_extra_config.go` with `VMNICExtraConfigSpec` + `VMNICExtraConfigSpecInput` + empty `It` bodies
4. [ ] Wire into `vmservice_test.go` with `Context("VM-NIC-EXTRA-CONFIG", ...)`
5. [ ] Implement helpers: `buildNICExtraConfigVM`, `statusExtraConfigMap`, `statusNICInterface`, `getNICExtraConfigCondition`
6. [ ] Implement helpers: `waitForNICExtraConfigSynced`, `waitForNICExtraConfigSyncedWithExtraConfig`, `waitForNICExtraConfigSyncedWithExtraConfigs`, `waitForNICExtraConfigSyncedWithExtraConfigAbsent`
7. [ ] Implement It block 7.1: live-mode `coalescingScheme=Disabled` create (Phase 1), then `coalescingScheme=Static` + `coalescingParams="32"` update (Phase 2); both fields asserted via composite guard
8. [ ] Implement It block 7.2: Phase 3 — all four powercycle-mode fields patched simultaneously → `PowerCyclePending`; Phase 4 — power cycle → condition True + all four wire values asserted via `waitForNICExtraConfigSyncedWithExtraConfigs`; Phase 5 — flip `rssOffloadEnabled=false` + `udpRssEnabled=UDPRSSModeDisabled` → second power cycle → assert `"FALSE"` and `"2"` with lastTransitionTime sentinel
9. [ ] Implement It block 7.3: AdvancedProperties bag add (`innerRSS=TRUE`) + GC remove (absent from `status.extraConfig`)
10. [ ] Implement helpers: `findVSphereVMByMOIDForNIC`, `assertVSphereNICState` (reuse from `vm_compute_config.go` if present in package)
11. [ ] Implement It block 7.4: UPTv2Enabled prereq check (Step A) + happy path with `status.vmxnet3.uptv2Enabled` and govmomi assertions (Step B)
12. [ ] Implement It block 7.5: VNUMANodeID EFI prereq gate (Step A) + PowerOffRequired → power-off → True with `status.vnumaNodeID` and govmomi assertions (Step B)
13. [ ] Run `go build ./test/e2e/...` to verify compilation
14. [ ] Run `golangci-lint` on the new file

---

## 11. Key Design Decisions vs Python Script

| Python phase | Ginkgo `It` block | Notes |
|---|---|---|
| Phase 1 | 7.1 | Combined with phase 2 — same VM, same `It` |
| Phase 2 | 7.1 | Extended beyond the Python script: also tests `coalescingParams` via `coalescingScheme=Static`+`params="32"` |
| Phase 3 | 7.2 | Combined with phases 4+5 — same VM; Extended: all four powercycle-mode fields patched simultaneously, not just `ctxPerDev` |
| Phase 4 | 7.2 | `waitForNICExtraConfigSyncedWithExtraConfigs` asserts all four wire values in a single composite wait to avoid partial-apply races |
| _(new)_ | 7.2 Phase 5 | Flip `rssOffloadEnabled=false` + `udpRssEnabled=UDPRSSModeDisabled` → second power cycle → asserts `"FALSE"` and `"2"`; guards against encoder regression on the `Disabled` branch of `encodeUDPRSSMode` |
| Phase 5 | 7.3 | Standalone AdvancedProperties lifecycle; setup includes ctxPerDev+RSS fields so the VM state mirrors what it would be after full production use |
| Phase 6 | 7.4 | `extended-functional`; Step A verifies prereq gate; Step B happy path with optional govmomi assertion (hardware-dependent) |
| Phase 7 | 7.5 | `extended-functional`; Step A EFI prereq gate; Step B full lifecycle with optional govmomi assertion |

---

## 12. Status Field Reference

| Assertion | Go path | Notes |
|---|---|---|
| Condition type | `vmopv1a6.VirtualMachineNetworkConfigSynced` | `"VirtualMachineNetworkConfigSynced"` |
| PowerCyclePending reason | `vmopv1a6.VirtualMachineNetworkPowerCyclePendingReason` | `"PowerCyclePending"` |
| PowerOffRequired reason | `vmopv1a6.VirtualMachineNetworkPowerOffRequiredReason` | `"PowerOffRequired"` |
| PrerequisiteNotMet reason | `vmopv1a6.VirtualMachinePrerequisiteNotMetReason` | `"PrerequisiteNotMet"` |
| ExtraConfig entries | `vm.Status.ExtraConfig` (`[]vmopv1common.KeyValuePair`) | Map via `statusExtraConfigMap()` |
| Interface VMXNet3 status | `vm.Status.Network.Interfaces[i].VMXNet3` | `*VirtualMachineNetworkInterfaceVMXNet3Status`; has `UPTv2Enabled *bool` |
| Interface VNUMA node | `vm.Status.Network.Interfaces[i].VNUMANodeID` | `*int32` |

---

## 13. Non-Goals

- Do **not** enable or modify the `supports_telco_vm_service_api` capability from within the test — skip if not enabled.
- Do **not** test webhook validation rules for NIC fields (unit-tested in `webhooks/`).
- Do **not** modify `pkg/` or `api/` from this change set — this is a test-only PR per [`e2e-sync-with-changes.md`](../../memory/e2e-sync-with-changes.md).

---

## 14. Open Questions (deferred)

- Should It 7.4 also assert `status.network.interfaces[eth0].vmxnet3.uptv2Active` once ESXi activates UPTv2? → Deferred: activation depends on driver state inside the guest; not reliably observable in a short test window.
- Should there be a `Consistently` assertion that `VirtualMachineNetworkConfigSynced=True` is stable after live-mode updates in It 7.1? → Nice-to-have; add if flakiness is observed in CI.
- For It 7.5 govmomi assertion: `NIC.NumaNode` or `CoresPerNumaNode`? → `NIC.NumaNode` directly corresponds to `vnumaNodeID`; `CoresPerNumaNode` is compute topology, covered in compute-config tests.
