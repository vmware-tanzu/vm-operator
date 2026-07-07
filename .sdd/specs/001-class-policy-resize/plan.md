# Implementation Plan: VM Service Class Policy and Resize

- **Branch**: [`feature/vm-config-options-api`](https://github.com/akutz/vm-operator/tree/feature/vm-config-options-api/)
  - **Fork**: `akutz/vm-operator`
  - **PR target**: `vmware-tanzu/vm-operator`
- **Date**: 2026-05-13
- **Spec**: [`./sdd/specs/001-class-policy-resize/spec.md`](spec.md)
- **Epic**: vmop-3331

---

## Summary

Implement the four-CRD policy + environment-browser pipeline within the `github.com/vmware-tanzu/vm-operator` codebase, developed on branch `feature/vm-config-options-api` of the `akutz/vm-operator` fork. The `Zone` controller fans out to `ConfigTarget` and `VirtualMachineConfigPolicy`; the `ConfigTarget` controller fans out to `VirtualMachineConfigOptions` (per HW version), walks the cluster's ESX hosts in-place to aggregate per-host data onto `ConfigTarget.status`, and garbage-collects stale `VirtualMachineConfigOptions`. The `VirtualMachineConfigOptions` controller fans out to `VirtualMachineGuestOptions`. The VM admission webhook enforces the namespace policy and reads `ConfigTarget.status.maxHardwareVersion` / `status.sriov` for hardware-version and SR-IOV feasibility checks.

---

## Technical context

| Field | Value |
|-------|-------|
| **Language** | Go 1.22+ |
| **Primary dependencies** | `controller-runtime` v0.17, `govmomi` v0.36, `kubebuilder` v3 |
| **API server** | Kubernetes 1.30+ (vSphere Supervisor) |
| **Testing** | Ginkgo v2 + Gomega; `vcsim` for integration; real WCP Supervisor for E2E |
| **Code generation** | `controller-gen` (deepcopy, CRD manifests, RBAC markers) |
| **Target platform** | VMware vSphere Supervisor (WCP); namespace-isolated multi-tenancy |

---

## Constitution check

- [x] New CRDs use `+kubebuilder:object:root=true` and companion deepcopy generation.
- [x] `spec.id` fields are immutable (webhook enforcement).
- [x] Controllers are thin; vSphere calls go through the provider abstraction.
- [x] `status.observedGeneration` and `Ready` condition on every new CRD controller.
- [x] All observable changes have E2E coverage per `e2e-sync-with-changes.mdc`.
- [x] Acceptance Criteria in `customfield_10100`; Epic Link set post-create.

---

## Repository layout (this feature)

### Documentation (specs/)

```
specs/001-class-policy-resize/
├── spec.md       — this feature's functional spec (what & why)
├── plan.md       — this file (how)
├── tasks.md      — ordered task checklist
└── research.md   — research findings (EB API limitations, GC design, label contract)
```

### Source

```
external/vim/api/v1alpha1/
├── config_target_types.go               MODIFY — add status.maxHardwareVersion
├── config_target_devices_types.go       MODIFY — extend VirtualMachineSriovInfo with
│                                                  hostMoID, active, maxVFs, numVFs,
│                                                  dvxClass{,CheckpointSupported,SwDmaTracingSupported}
├── virtualmachine_config_options_*.go   EXISTS — no changes to types
├── virtualmachine_config_policy_types.go EXISTS — no changes to types
├── virtualmachine_guest_options_types.go EXISTS — no changes to types
└── zz_generated.deepcopy.go            REGEN — pick up the ConfigTarget extensions

config/crd/external-crds/
└── vim.vmware.com_configtargets.yaml    REGEN — pick up the ConfigTarget extensions

external/vim/doc/
└── integration-guide.md                NEW — partner-facing engineering doc (S2)

controllers/
├── infra/zone/
│   └── zone_controller.go              MODIFY — fan out ConfigTarget + VMConfigPolicy
├── configtarget/
│   ├── configtarget_controller.go      NEW — cluster-scope + per-host iteration
│   └── gc.go                           NEW — GC stale VirtualMachineConfigOptions
├── virtualmachineconfigoptions/
│   └── vmconfigoptions_controller.go   NEW
├── virtualmachineconfigpolicy/
│   └── vmconfigpolicy_controller.go    NEW
└── controllers.go                      MODIFY — register new controllers

webhooks/
├── configtarget/
│   ├── webhooks.go                      NEW — thin wrapper delegating to validation subpackage (DONE, PR #1711 / vmop-3757)
│   └── validation/
│       └── configtarget_validator.go   NEW — immutable spec.id; valid cluster MoID format for metadata.name (DONE, PR #1711 / vmop-3757)
├── virtualmachineconfigoptions/
│   └── validation_webhook.go           NEW
├── virtualmachineconfigpolicy/
│   ├── validation_webhook.go           NEW
│   └── defaulting_webhook.go           NEW
└── virtualmachine/
    └── validation_webhook.go           MODIFY — add policy + ConfigTarget capability checks

pkg/
├── config/capabilities/
│   └── capabilities.go                 MODIFY — add CapabilityVirtualMachineConfigPolicy
├── providers/vsphere/
│   └── environment_browser.go          NEW — QueryConfigTarget, QueryConfigOptionDescriptor,
│                                              QueryConfigOptionEx, per-host PropertyCollector
└── vmconfig/policy/
    └── policy_reconciler.go            NEW — ConfigTarget→policy spec sync logic

config/rbac/
└── role.yaml                           MODIFY — add new resources

test/
├── unit/
│   ├── configtarget/                   NEW
│   ├── virtualmachineconfigoptions/    NEW
│   └── virtualmachineconfigpolicy/     NEW
└── intg/
    ├── configtarget/                   NEW
    ├── virtualmachineconfigoptions/    NEW
    └── virtualmachineconfigpolicy/     NEW

test/e2e/vmservice/
└── configpolicy/
    └── configpolicy_test.go            NEW
```

There is no `hostsystem_types.go`, no `controllers/hostsystem/`, no `webhooks/hostsystem/`, no `config/rbac/hostsystem_*.yaml`, no `config/crd/external-crds/vim.vmware.com_hostsystems.yaml`, and no `test/intg/hostsystem/`. The earlier draft of this plan included those files; they were removed when the `HostSystem` CRD was consolidated into the `ConfigTarget` controller (see `research.md` Finding 7).

---

## Implementation phases

### Phase 0 — Architecture (wiki; no code)

Before any code begins, the following wiki documents must be updated or created and approved. Code reviews will reference these documents.

| Story | Target | Action |
|-------|--------|--------|
| A1 (vmop-3733) | Wiki page ID: 2453059721 | Apply Appendix A from `vim-api-jira.md` |
| A2 (vmop-3734) | Wiki pages ID: 2453059723, 2453060275 | Add SR-IOV EB limitation + `ConfigTarget.status` aggregation pattern (`maxHardwareVersion`, enriched `sriov`) |
| A3 (vmop-3735) | New child of 2453059710 | Author Design: VirtualMachineConfigPolicy |
| A4 (vmop-3736) | New child of 2453059710 | Author TDS: VirtualMachineConfigPolicy |
| A5 (vmop-3737) | Pages under 2453059710 | Stakeholder sign-off; move to Approved folder |

### Phase 1 — Foundations (code)

#### F1. Capability + feature gate (Story S1 / vmop-3738)

- Add `CapabilityVirtualMachineConfigPolicy` to `pkg/config/capabilities/capabilities.go`.
- Wire a feature gate in `pkg/config/capabilities/` that controls controller registration and webhook short-circuit.
- Install the four `vim.vmware.com` CRDs (`ConfigTarget`, `VirtualMachineConfigOptions`, `VirtualMachineGuestOptions`, `VirtualMachineConfigPolicy`) via the Supervisor chart install hook and `config/crd/external-crds/`. No `HostSystem` CRD exists.

**Key contracts**:
```go
// pkg/config/capabilities/capabilities.go
const CapabilityVirtualMachineConfigPolicy = "supports_vm_config_policy"
```

```go
// controllers/controllers.go — gated registration
if featuregate.VirtualMachineConfigPolicy.Enabled() {
    must(controllers.AddConfigTargetController(mgr))
    must(controllers.AddVirtualMachineConfigOptionsController(mgr))
    must(controllers.AddVirtualMachineConfigPolicyController(mgr))
    // ...
}
```

#### F2. Partner integration doc (Story S2 / vmop-3739)

Authored at `external/vim/doc/integration-guide.md`. Covers:
- Reconcile pipeline diagram (reuse mermaid from `controller-workflows.md`).
- Role of each CRD (scope, name conventions, owner).
- `ConfigTarget.status` capability surface: `maxHardwareVersion`, `sriov` (per-host entries with `hostMoID`, DVX class fields), and how the admission webhook reads it.
- Per-host iteration inside the `ConfigTarget` controller (PropertyCollector RPC strategy, partial-failure handling).
- `VirtualMachineConfigPolicy.spec.syncMode` semantics.
- Worked example: tenant policy denying an unsupported HW version.

### Phase 2 — Implementation (code; may proceed in parallel once Phase 1 is merged)

#### I1. ConfigTarget API extensions for per-cluster aggregation (Sub-task / vmop-3797)

The only API change for this consolidation is additive on `ConfigTarget`:

**Modify** `external/vim/api/v1alpha1/config_target_types.go` — add a new field on `ConfigTargetStatus`:

```go
// MaxHardwareVersion is the highest virtual hardware version available
// in this cluster, computed as the max of each host's
// HostConfigInfo.defaultHardwareVersion (queried per host during reconcile).
// A VM whose effective hardware version is <= MaxHardwareVersion can be
// created on at least one host in the cluster. The VM admission webhook
// uses this field to reject hardware-version-infeasible VMs without
// needing to read per-host objects or labels.

// +optional
MaxHardwareVersion string `json:"maxHardwareVersion,omitempty"`
```

**Modify** `external/vim/api/v1alpha1/config_target_devices_types.go` — extend `VirtualMachineSriovInfo` so the cluster-aggregated list still attributes each NIC to a specific host and exposes the same DVX-related capabilities the dropped `HostSRIOVNIC` type carried:

```go
// HostMoID is the ManagedObjectID of the ESX host whose
// HostConfigInfo.pciPassthruInfo contributed this entry. The same SR-IOV
// NIC observed on multiple hosts produces multiple entries that differ
// in HostMoID.

// +required
HostMoID string `json:"hostMoID"`

// Active indicates SR-IOV is both enabled and in effect on this NIC,
// meaning the host has been rebooted since SR-IOV was enabled
// (HostSriovInfo.sriovActive).

// +optional
Active bool `json:"active,omitempty"`

// MaxVFs is the maximum number of Virtual Functions this NIC can expose,
// as reported by HostSriovInfo.maxVirtualFunctionSupported.

// +optional
MaxVFs int32 `json:"maxVFs,omitempty"`

// NumVFs is the number of Virtual Functions currently present on this
// NIC, as reported by HostSriovInfo.numVirtualFunction.

// +optional
NumVFs int32 `json:"numVFs,omitempty"`

// DVXClass is the Device Virtualization Extensions class name for this
// NIC, populated when the NIC's device class appears in
// HostHardwareInfo.dvxClasses with sriovNic=true (vSphere 8.0.0.1+).
// Empty when this NIC does not belong to any DVX class.

// +optional
DVXClass string `json:"dvxClass,omitempty"`

// DVXCheckpointSupported indicates whether live migration with
// checkpoint is supported for VMs using this NIC via DVX
// (HostDvxClass.checkpointSupported). Only meaningful when DVXClass is set.

// +optional
DVXCheckpointSupported bool `json:"dvxCheckpointSupported,omitempty"`

// DVXSWDMATracingSupported indicates whether software DMA tracing is
// supported for this NIC via DVX (HostDvxClass.swDMATracingSupported).
// Only meaningful when DVXClass is set.

// +optional
DVXSWDMATracingSupported bool `json:"dvxSwDmaTracingSupported,omitempty"`
```

`ConfigTarget.status.sriov` gains `+listType=map` with `+listMapKey=hostMoID` plus `+listMapKey=id` so that the same physical NIC on different hosts is patchable as distinct entries.

Regenerate `zz_generated.deepcopy.go` and the `vim.vmware.com_configtargets.yaml` CRD manifest via `make generate manifests`.

**No new CRD, no new controller, no new webhook, no new RBAC.** All per-host data acquisition happens in the existing `ConfigTarget` controller; see I3.

#### I2. Zone controller fan-out (Story S3 / vmop-3740)

Modify `controllers/infra/zone/zone_controller.go`:

1. Derive unique cluster MoIDs from `zone.spec.namespace.poolMoIDs`.
2. For each cluster MoID: `CreateOrPatch` a `ConfigTarget` with `metadata.name = clusterMoID`.
3. `CreateOrPatch` a `VirtualMachineConfigPolicy` with `spec.zone = zone.metadata.name`, defaulting `spec.syncMode = ConfigTarget` **only on create**.
4. Do **not** delete `ConfigTarget` or `VirtualMachineConfigPolicy` when a pool is removed from `spec.namespace.poolMoIDs` (deletion is deferred to a follow-up).

#### I3. ConfigTarget controller (Story S5 / vmop-3742)

New controller `controllers/configtarget/configtarget_controller.go`:

**Cluster-scope path** (S5.b):
1. Resolve `spec.id` to a cluster via `pkg/providers/vsphere`.
2. Call `EnvironmentBrowser.QueryConfigTarget` → populate `status.*`.
3. Call `EnvironmentBrowser.QueryConfigOptionDescriptor` → enumerate HW version keys.
4. For each HW version key: `CreateOrPatch` a `VirtualMachineConfigOptions`.

**Per-host iteration** (S5.c, vmop-3759):
1. Enumerate cluster hosts via `ClusterComputeResource.host`.
2. For each host, issue a single `PropertyCollector` RPC fetching:
   - `config.defaultHardwareVersion`
   - `capability.supportedHardwareVersions`
   - `config.pciPassthruInfo` (filtered for `HostSriovInfo` entries where `sriovCapable == true`)
   - `hardware.dvxClasses` (entries where `sriovNic == true`)
3. Aggregate `max(config.defaultHardwareVersion)` across the responding hosts and write it to `ConfigTarget.status.maxHardwareVersion`.
4. Build per-host `VirtualMachineSriovInfo` entries from the SR-IOV / DVX data and write them to `ConfigTarget.status.sriov`. Each entry carries `hostMoID` for attribution.
5. Per-host RPC failures are logged + emitted as warning events; they are not fatal. The reconcile is requeued so the absent host's data lands on a later pass. The successful hosts' data is still written.

No per-host CRD is created; the per-host data is owned entirely by `ConfigTarget.status`. There is no second controller and no watch wiring between `ConfigTarget` and any per-host kind.

**Garbage collection**: Each `VirtualMachineConfigOptions` object carries an owner reference pointing back to the `ConfigTarget` that created it. When a hardware version is no longer returned by `QueryConfigOptionDescriptor` for a given cluster, the `ConfigTarget` reconciler removes its owner reference from the corresponding `VirtualMachineConfigOptions` object. Kubernetes garbage-collects the object only once it has no remaining owner references, which naturally handles the case where multiple `ConfigTarget` objects (e.g. from different clusters that share the same hardware-version key) co-own the same `VirtualMachineConfigOptions`. This prevents a race where a manual delete could conflict with another `ConfigTarget` that still depends on the object.

The ownership invariant is maintained as follows:

- On each successful reconcile of `QueryConfigOptionDescriptor`, the reconciler calls `EnsureOwnerRef` to add its owner reference to every `VirtualMachineConfigOptions` it currently manages.
- If a hardware-version key disappears from the live set, the reconciler removes its owner reference from the corresponding object. If that removal leaves the object with no remaining owner references, the reconciler deletes it. If other `ConfigTarget` objects still own it, it is left untouched.
- This GC path runs only when both `QueryConfigTarget` and `QueryConfigOptionDescriptor` succeed; per-host RPC failures do not affect it (per-host data lives inline on `ConfigTarget.status` and is overwritten on each reconcile, so it produces no stale Kubernetes objects).

#### I4. VirtualMachineConfigOptions controller (Story S6 / vmop-3743)

New controller `controllers/virtualmachineconfigoptions/`:

1. Call `EnvironmentBrowser.QueryConfigOptionEx` for `spec.hardwareVersion`.
2. Map `vim.vm.ConfigOption` → `VirtualMachineConfigOptionsStatus`.
3. For each guest OS descriptor: `CreateOrPatch` a `VirtualMachineGuestOptions` with `spec.id = guestOsID`, `metadata.name = dnsSafe(guestOsID)`.
4. Update `VirtualMachineGuestOptions.status.hardwareVersions` listMap (keyed by `hardwareVersion`) with this HW version's guest OS data.

#### I5. VirtualMachineGuestOptions plumbing (Story S7 / vmop-3744)

No standalone controller; reconciled by the `VirtualMachineConfigOptions` controller. Deliverables:
- Validation webhook: immutable `spec.id`; DNS-safe `metadata.name`.
- RBAC, manifest registration.
- Integration + E2E tests for the listMap merge.

#### I6. VirtualMachineConfigPolicy controller (Story S8 / vmop-3745)

New controller `controllers/virtualmachineconfigpolicy/`:

1. Watch `VirtualMachineConfigPolicy` in tenant namespaces.
2. If `spec.syncMode = ConfigTarget`:
   - Find the `ConfigTarget` for the policy's zone's cluster MoID.    - Copy relevant status fields (capacity limits, security flags, devices) into
     `policy.spec.*`.
3. If `spec.syncMode = Disabled`: set `Ready=True` with reason `SyncDisabled`; do not modify `spec`.
4. Never overwrite `spec.extraConfig`, `spec.latencySensitivityLevels`, or other non-ConfigTarget-derived fields during sync.

**Defaulting webhook** (`webhooks/virtualmachineconfigpolicy/defaulting_webhook.go`):
- `spec.syncMode` → `ConfigTarget`
- `spec.createMode / updateMode / powerOnMode` → `Allow`
- `spec.vmClassMode` → `AsPolicy`

**Validation webhook**:
- `spec.zone` must reference an existing `Zone`.
- `extraConfig.allowed / denied` entries must have non-empty `key` and valid `type` enum.

#### I7. VM admission webhook policy enforcement (Story S9 / vmop-3746)

Modify `webhooks/virtualmachine/validation_webhook.go`:

1. **Mode enforcement** (S9.a): On create/update/power-on, load the namespace's `VirtualMachineConfigPolicy`; apply `createMode`, `updateMode`, `powerOnMode`. Respect `vmClassMode`: if `AsPolicy`, bypass policy for VM Class-derived config.
2. **ExtraConfig enforcement** (S9.b): Walk `vm.spec.advancedProperties` (or the relevant ExtraConfig field); apply `Denied` list first, then `Allowed` list. `Fixed` → exact match; `Regex` → regexp; `Glob` → filepath.Match semantics.
3. **Hardware-version check** (S9.c): Resolve the VM's zone to a cluster MoID (same derivation the policy controller uses), `Get` the cluster's `ConfigTarget`, and reject if the VM's effective hardware version exceeds `ConfigTarget.status.maxHardwareVersion`.

**Key API flow** (S9.c):
```go
var ct vim1a1.ConfigTarget
clusterMoID := zoneToClusterMoID(vm.Namespace, vm.Spec.Zone)
if err := client.Get(ctx, client.ObjectKey{Name: clusterMoID}, &ct); err != nil {
    return admission.Errored(http.StatusInternalServerError, err)
}
if vmHardwareVersion(vm) > ct.Status.MaxHardwareVersion {
    return admission.Denied(fmt.Sprintf(
        "hardware version %s exceeds cluster maximum %s",
        vmHardwareVersion(vm), ct.Status.MaxHardwareVersion))
}
```

Single informer-cached `Get`. No list across per-host objects, no label selector match.

---

## vSphere API mapping

| Action | vSphere API call | Notes |
|--------|-----------------|-------|
| Cluster capacity + security flags | `EnvironmentBrowser.QueryConfigTarget` | Cluster-scope; SR-IOV not returned |
| Supported HW version keys | `EnvironmentBrowser.QueryConfigOptionDescriptor` | Returns `[]VirtualMachineConfigOptionDescriptor` |
| Per-HW-version config | `EnvironmentBrowser.QueryConfigOptionEx` | One call per HW version key |
| Per-host SR-IOV + DVX + default HW version | `PropertyCollector` on `vim.HostSystem` (MoR): `config.pciPassthruInfo`, `hardware.dvxClasses`, `config.defaultHardwareVersion`, `capability.supportedHardwareVersions` | Per-host iteration inside the `ConfigTarget` controller |

---

## RBAC (ClusterRole additions)

| Resource | Verbs |
|----------|-------|
| `configtargets` | get, list, watch, create, update, patch, delete |
| `virtualmachineconfigoptions` | get, list, watch, create, update, patch, delete |
| `virtualmachineconfigpolicy` | get, list, watch, create, update, patch |
| `virtualmachineguestoptions` | get, list, watch, create, update, patch, delete |
| `zones` | get, list, watch |

---

## Testing strategy

| Layer | Mechanism | Location |
|-------|-----------|----------|
| Unit | `*_test.go` beside source | beside each controller/webhook/pkg package |
| Integration | `*_intg_test.go` + `vcsim` | `test/intg/` |
| E2E | Ginkgo, real Supervisor | `test/e2e/vmservice/configpolicy/` |

**GC must be tested at all three layers** (see `spec.md` US4 scenarios).

---

## Risk and rollback

| Risk | Mitigation |
|------|-----------|
| GC deletes live objects after transient error | GC is gated on successful cluster-scope queries only; per-host failures never trigger GC |
| Per-host RPC fails on older hosts | Per-host error is logged + recorded as an event; that host's SR-IOV entries are omitted this pass; reconcile is requeued so missing data lands later |
| Capability disabled mid-deployment | Controllers deregister cleanly; webhook short-circuits |
| CRD schema break | `spec.id` immutability enforced at webhook, not only at API server level |

**Disable path**: Set `CapabilityVirtualMachineConfigPolicy` to `false` in the capabilities ConfigMap. Controllers stop reconciling. CRDs remain installed (safe to list); existing objects are preserved. Re-enabling resumes reconciliation from current state.
