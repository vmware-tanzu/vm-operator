# Research: VM Service Class Policy and Resize

- **Feature**: `specs/001-class-policy-resize/`
- **Epic**: vmop-3331
- **Spike**: vmop-3470 (completed; produced `controller-workflows.md` and the v1alpha1 type set)

---

## Finding 1: `QueryConfigTarget` does not return SR-IOV NICs at cluster scope

### Background

The vSphere `EnvironmentBrowser.QueryConfigTarget` API, when called against a `ClusterComputeResource`, returns a `ConfigTarget` structure that includes most device categories (datastores, networks, CD-ROMs, serial ports, etc.) but **does not** include SR-IOV-capable physical NICs in the `sriov` field when called at the cluster scope.

### Evidence

Empirically confirmed during vmop-3470 spike: the `sriov` field in `vim.ConfigTarget` is only populated when `QueryConfigTarget` is called against an individual `vim.HostSystem` MoR (not against the `ClusterComputeResource`).

### Impact on design

SR-IOV discovery (and per-host hardware-version aggregation) is handled inside a **single controller**:

The `ConfigTarget` controller, during the same reconcile that calls cluster-scope `QueryConfigTarget` and `QueryConfigOptionDescriptor`, also enumerates the cluster's hosts via `ClusterComputeResource.host` and uses `PropertyCollector` against each host to read:

- `config.defaultHardwareVersion` — aggregated as `max(perHost)` into the new `ConfigTarget.status.maxHardwareVersion` field.
- `config.pciPassthruInfo` — filtered for `HostSriovInfo` entries where `sriovCapable == true` — to build per-host SR-IOV entries.
- `hardware.dvxClasses` — entries where `sriovNic == true` — to enrich each SR-IOV entry with its DVX class name and capability flags (`dvxCheckpointSupported`, `dvxSwDmaTracingSupported`).

The per-host results are written into `ConfigTarget.status.sriov` as one entry per (host, NIC) pair, carrying a `hostMoID` field for attribution plus `active`, `maxVFs`, `numVFs`, and the DVX fields. This means a single object — the cluster's `ConfigTarget` — exposes every per-cluster and per-host capability that downstream consumers need.

> **Earlier draft (superseded).** This file previously described a two-controller, two-hop design with a per-host `HostSystem` CRD that owned the per-host vSphere queries; `ConfigTarget` re-aggregated `HostSystem.status` via a watch. That design was removed in favour of the consolidation above — see Finding 7 for the rationale. The corresponding JIRA work (vmop-3741 and its sub-tasks) has been closed as *Won't Implement*; the new API surface is captured in vmop-3797.

### vSphere API references

| API | MOR type | Returns SR-IOV at cluster scope? | Used by |
|-----|----------|----------------------------------|---------|
| `EnvironmentBrowser.QueryConfigTarget` | `ClusterComputeResource` | **No** | `ConfigTarget` controller (cluster path) |
| `EnvironmentBrowser.QueryConfigTarget` | `HostSystem` (vSphere MoR) | Yes | **Not used** — `PropertyCollector` is cheaper and more targeted |
| `PropertyCollector` on `config.pciPassthruInfo` | `HostSystem` (vSphere MoR) | Yes | `ConfigTarget` controller (per-host iteration) |
| `PropertyCollector` on `hardware.dvxClasses` | `HostSystem` (vSphere MoR) | Yes | `ConfigTarget` controller (per-host iteration) |
| `PropertyCollector` on `config.defaultHardwareVersion` | `HostSystem` (vSphere MoR) | Yes | `ConfigTarget` controller (per-host iteration) |

(`HostSystem` in the table above is the vSphere managed-object type, not a VM-Operator CRD; no Kubernetes CRD for hosts is being introduced.)

**Recommendation**: use `PropertyCollector` with a multi-property fetch (`config.defaultHardwareVersion`, `capability.supportedHardwareVersions`, `config.pciPassthruInfo`, `hardware.dvxClasses`) in a single RPC per host. One round trip per host, no second controller, no second CRD.

---

## Finding 2: cluster capabilities live on `ConfigTarget.status`; admission webhook reads one object

### Background

The VM admission webhook (S9) needs to know the maximum hardware version available in the zone's cluster to validate that a VM's effective hardware version is achievable. With the per-host CRD removed, the natural — and cheaper — place to expose that is on the cluster's `ConfigTarget` status, where every other per-cluster capability already lives.

### Design decision: a single per-cluster object

The `ConfigTarget` controller's per-host iteration (Finding 1) computes `max(perHostDefaultHardwareVersion)` and writes it to `ConfigTarget.status.maxHardwareVersion`. The same iteration writes the per-host SR-IOV entries to `ConfigTarget.status.sriov`. Downstream consumers do exactly one `Get` per cluster.

### Consumer pattern for admission webhooks

```go
// Admission webhook — hardware-version check
var ct vim1a1.ConfigTarget
clusterMoID := zoneToClusterMoID(vm.Namespace, vm.Spec.Zone)  // existing helper
if err := client.Get(ctx, client.ObjectKey{Name: clusterMoID}, &ct); err != nil {
    return admission.Errored(http.StatusInternalServerError, err)
}
if vmEffectiveHardwareVersion(vm) > ct.Status.MaxHardwareVersion {
    return admission.Denied(fmt.Sprintf(
        "hardware version %s exceeds cluster maximum %s",
        vmEffectiveHardwareVersion(vm), ct.Status.MaxHardwareVersion))
}
```

This is a single `Get` (informer-cached), no list and no per-host label match. Earlier drafts of this section described a `HostSystem` label index — that approach has been retired (see Finding 7).

---

## Finding 3: Garbage collection design

### Problem

After the `ConfigTarget` controller reconciles, the following stale objects may exist in Kubernetes but no longer correspond to real vSphere inventory:

- `VirtualMachineConfigOptions` for a hardware version that `QueryConfigOptionDescriptor` no longer returns (e.g., the vSphere cluster has been upgraded past `vmx-17`).

If these objects are not deleted, downstream components (policies, admission webhooks) may act on stale data. Per-host data (max default HW version, SR-IOV NIC entries) lives inline on `ConfigTarget.status` and is overwritten each reconcile, so it does not require GC.

### Decision: GC runs only on a fully-successful cluster-scope reconcile

The GC step is the **final** step of a `ConfigTarget` reconcile, executed only when both `QueryConfigTarget` and `QueryConfigOptionDescriptor` succeeded. The per-host iteration's success or partial-failure is irrelevant to GC — per-host data is overwritten inline and does not produce stale K8s objects.

If either of the cluster-scope queries fails, the controller requeues without running GC. This ensures transient vSphere errors never cause spurious object deletions.

### Implementation sketch

```go
// controllers/configtarget/gc.go

// GCVirtualMachineConfigOptions deletes any VirtualMachineConfigOptions whose
// metadata.name is not in liveHWVersionKeys. Called only after
// QueryConfigOptionDescriptor succeeds.
func GCVirtualMachineConfigOptions(ctx context.Context, c client.Client,
    liveHWVersionKeys sets.Set[string]) error { ... }
```

### What GC does NOT cover

- `VirtualMachineGuestOptions.status.hardwareVersions` listMap entries for garbage-collected hardware versions. These become orphaned (the entry is no longer updated) but not actively wrong. Clean-up is deferred to a follow-up.
- `VirtualMachineConfigPolicy` objects when their owning `Zone` is deleted. This involves namespace-scoped objects and Zone finaliser semantics; deferred.

---

## Finding 4: `QueryConfigOptionDescriptor` vs `QueryConfigOptionEx` semantics

### `QueryConfigOptionDescriptor`

Returns a list of `VirtualMachineConfigOptionDescriptor` objects, each with:
- `key` — the hardware version string (e.g. `vmx-22`).
- `description` — human-readable name.
- `host` — list of hosts that support this version.
- `createSupported` / `runSupported` / etc. — creation and runtime support flags.

This is the source for enumerating which `VirtualMachineConfigOptions` objects to create/maintain.

### `QueryConfigOptionEx`

Returns a full `VirtualMachineConfigOption` (alias: `vim.vm.ConfigOption`) for a specific hardware version key. This is a large object including:
- `hardwareOptions` — virtual device count limits, memory limits.
- `capabilities` — VM feature capability flags.
- `defaultDevice` — list of default virtual devices.
- `guestOSDescriptor` — list of supported guest OS identifiers with descriptions.

This is the source for `VirtualMachineConfigOptionsStatus` and for enumerating which `VirtualMachineGuestOptions` to create.

### Implication

The `ConfigTarget` controller calls `QueryConfigOptionDescriptor` to get the set of supported HW version keys. The `VirtualMachineConfigOptions` controller then calls `QueryConfigOptionEx` once per key. This division of responsibility avoids one giant `QueryConfigOptionEx` call per cluster on every `ConfigTarget` reconcile.

---

## Finding 5: existing in-tree code (no re-implementation needed)

The following work is already checked in on the repository's `main` branch:

| Artifact | Status |
|----------|--------|
| Go types for `ConfigTarget`, `VirtualMachineConfigOptions`, `VirtualMachineGuestOptions`, `VirtualMachineConfigPolicy` | **Done** |
| `zz_generated.deepcopy.go` | **Done** (regen required for the `ConfigTarget` additions in vmop-3797) |
| CRD manifests for the four above | **Done** (`config_target_*.yaml` regen required for the `ConfigTarget` additions in vmop-3797) |
| `external/vim/doc/controller-workflows.md` | **Done** |
| `external/vim/pkg/convert/vim2crd.go` stub | Done (panics "Not Implemented") |
| `controllers/infra/zone/zone_controller.go` skeleton | Done (177 LOC; does not yet emit child CRs) |

**No new CRD types** are introduced. The only API change is the additive set of `ConfigTarget.status` fields tracked by vmop-3797 (`maxHardwareVersion` plus per-host fields on `VirtualMachineSriovInfo`).

---

## Finding 6: `PlaceVmsXCluster` compatibility for auto-NUMA and auto-placed SR-IOV is open (vmop-3794)

### Background

vSphere 9.2 introduces auto-NUMA and auto-placement for SR-IOV NICs. Both features are gated on virtual hardware version `vmx-23` and require host-side capabilities that are not uniform across clusters in a Supervisor.

VM Service must answer two enforcement questions before users can rely on these features:

- **Create path** — refuse a VM whose effective configuration requires auto-NUMA or auto-placed SR-IOV when no zone in the namespace contains a cluster that supports these features.
- **Update path** — refuse a transition from `Pinned` to `Auto` when the VM's currently-scheduled cluster does not support the new mode. This is more critical than the create path because `spec.minHardwareVersion` cannot be decreased once raised; an accepted transition that no host can satisfy leaves the VM in a state that cannot be powered on or reconciled.

### Open question

A scenario raised with the platform team (paraphrased):

- Two clusters, A and B, both with SR-IOV-capable hosts, all NICs supporting 9.2 auto placement.
- Only two NICs on two hosts in cluster A have access to network N-1.
- `PlaceVmsXCluster` is invoked with a `ConfigSpec` requesting an SR-IOV NIC, an SR-IOV backing with a PF set to `auto`, and a network connection to N-1.

Will `PlaceVmsXCluster` deterministically pick cluster A, or are the auto-placement constraints only consulted at power-on?

The DRS team's preliminary answer is that compatibility tests during VM creation are not guaranteed to be honored: when the VM is powered off, compatibility checks emit *warnings* (not errors), and DRS is free to ignore them.

If that answer is confirmed, the VM admission webhook cannot rely on `PlaceVmsXCluster` to fence off incompatible clusters at admission time. The webhook must instead make the determination from K8s objects alone (the design principle established in Findings 1 and 2): per-cluster capability summaries collected by the `ConfigTarget` controller and exposed on `ConfigTarget.status` (`maxHardwareVersion` for hardware-version checks, `sriov` for SR-IOV capability checks).

### Update-path edge case

The investigation must also account for the following sequence:

1. User creates a VM with `spec.minHardwareVersion` unset.
2. The VM is persisted in etcd but not yet placed by the placement controller.
3. User decides to enable auto-NUMA and is therefore forced to set `spec.minHardwareVersion = vmx-23`.

A naive rule of "refuse any hardware-version-sensitive update until placement converges" breaks the case where placement is currently failing *because* of the existing desired state and the user is editing the spec to fix it. The investigation needs to take a position on this, with rationale.

### Impact on this design

This Spike (vmop-3794) is intentionally scoped to *answer the question*, not to implement the resulting enforcement. Outputs feed back into:

- `ConfigTarget.status` — possible new boolean(s) summarising auto-NUMA and auto-placed-SR-IOV support per cluster, computed from `pciPassthruInfo`, `dvxClasses`, and the existing supported-hardware-version set. The cluster-aggregation pattern is already established by `maxHardwareVersion` and the enriched `sriov` list landed under vmop-3797.
- The VM admission webhook (Story S9 / vmop-3746) — added create- and update-path checks that read those K8s-object summaries.
- The `VirtualMachineConfigPolicy` — possibly a tenant-controlled mode for the unplaced-VM edit window.

Follow-up Stories will be filed against vmop-3331 once the Spike concludes.

### Why this is captured here, not deferred

Recording the open question alongside the rest of the policy/EB pipeline research keeps the SDD honest about what is decided vs. what is being investigated, and stops the existing design from quietly drifting toward an answer the spike has not yet validated.

---

## Finding 7: HostSystem CRD consolidated into the ConfigTarget controller (vmop-3797)

### Decision

The previously-proposed `HostSystem` CRD, its dedicated controller, its validation webhook, its well-known label contract, its RBAC, and its GC entry are all removed from the design. The `ConfigTarget` controller is the single authority that talks to vSphere for both cluster-scope and per-host data, and it writes every per-cluster aggregate to `ConfigTarget.status`.

### Why the original two-CRD design was rejected

The two-CRD, two-hop SR-IOV design was an artefact of trying to separate "who owns the vSphere RPC" from "who owns the cluster-level summary". Reviewing it against the data being moved:

- A `HostSystem` CRD adds a second cluster-internal Kubernetes object kind (RBAC, manifests, CRD installation, namespace placement, scope ambiguity between namespaced and cluster-scoped) for data whose only consumer was a single `Get` from the admission webhook and a single read by the `ConfigTarget` controller.
- The "well-known label" contract for hardware-version queries introduced a second source of truth (label vs `status.defaultHardwareVersion`) that had to be kept in sync inside one Patch. A single field on `ConfigTarget.status.maxHardwareVersion` removes the synchronisation requirement entirely.
- The watch-driven convergence (`HostSystem` status change re-enqueues `ConfigTarget`) and the second GC entry (stale `HostSystem` deletion) added moving parts that the consolidated design simply does not have.

### What the new design looks like

See Finding 1 for the per-host iteration mechanics and Finding 2 for the admission-webhook consumer pattern. The API surface is:

- `ConfigTarget.status.maxHardwareVersion string` — max of per-host `config.defaultHardwareVersion`.
- `ConfigTarget.status.sriov []VirtualMachineSriovInfo` — one entry per (host, NIC) pair, with the existing PCI passthrough fields plus the new `hostMoID`, `active`, `maxVFs`, `numVFs`, `dvxClass`, `dvxCheckpointSupported`, `dvxSwDmaTracingSupported` fields.

Both additions are tracked by the new sub-task vmop-3797 under vmop-3742.

### What was retired

- vmop-3741 (`HostSystem` Story) and its sub-tasks vmop-3752, vmop-3753, vmop-3754, vmop-3755, vmop-3756 — closed as *Won't Implement*.
- `HostSystem` API types, the `controllers/hostsystem/` package, the `webhooks/hostsystem/` package, the RBAC editor/viewer roles, the CRD manifest, the well-known-label contract, the GC step for stale `HostSystem` objects, and the watch wiring between `ConfigTarget` and `HostSystem`.

### Knock-on updates

- vmop-3742 (Story) — per-host iteration moves *into* this controller; no fan-out.
- vmop-3758 (Sub-task) — cluster-scope path; only `VirtualMachineConfigOptions` GC remains.
- vmop-3759 (Sub-task) — re-scoped to per-host iteration writing to `ConfigTarget.status`; no `HostSystem` fan-out.
- vmop-3760, vmop-3761 (Sub-tasks) — vcsim/E2E scenarios adjusted; `HostSystem` assertions replaced with `ConfigTarget.status.maxHardwareVersion` + `status.sriov` assertions.
- vmop-3746 (Story) — admission webhook reads `ConfigTarget.status.maxHardwareVersion`; no label list.
- vmop-3735, vmop-3736 (Stories) — design / TDS titles drop *"and HostSystem"*.
- vmop-3733, vmop-3734, vmop-3738, vmop-3739 (Stories) — One Pager, Research pages, capability/feature-gate, and partner integration doc all drop the `HostSystem` text and replace it with the `ConfigTarget.status` pattern.

---

## References

- Spike: vmop-3470
- Spike: vmop-3794 (`PlaceVmsXCluster` compatibility for auto-NUMA and auto-placed SR-IOV)
- Sub-task: vmop-3797 (`ConfigTarget.status.maxHardwareVersion` + per-host `sriov` enrichments)
- Architecture doc: `external/vim/doc/controller-workflows.md`

- Wiki: Research: VM Service: Class Policy and Resize
- vSphere API: `vim.EnvironmentBrowser.QueryConfigTarget`, `QueryConfigOptionDescriptor`, `QueryConfigOptionEx`
- govmomi: `github.com/vmware/govmomi/vim25/methods`, `github.com/vmware/govmomi/property`
