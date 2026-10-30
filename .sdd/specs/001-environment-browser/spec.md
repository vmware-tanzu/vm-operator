# Feature Specification: Environment Browser

- **Feature branch**: [`feature/vm-config-options-api`](https://github.com/akutz/vm-operator/tree/feature/vm-config-options-api/)
  - **Fork**: `akutz/vm-operator`
  - **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-05-13
- **Status**: In Progress
- **Epic**: vmop-3331
- **Wiki parent**: VM Service: Class Policy and Resize
- **Spike (done)**: vmop-3470
- **Scope update (vmop-4318)**: `VirtualMachineConfigPolicy` and its admission enforcement are superseded by Flex VM Classes (`VirtualMachineClass` v2, vmop-4034; specification proposed in PR #2067). This spec covers environment-browser discovery and hardware feasibility; the class spec owns governance, ranges, ExtraConfig rules, and create/update/power-on enforcement. Policy removal (vmop-4318) is independent of the class spec and environment-browser completion (vmop-4319).

---

## Summary

Introduce a Kubernetes-native pipeline that:

1. Queries each vSphere cluster's `EnvironmentBrowser` for cluster-scope data and walks the cluster's ESX hosts for the per-host fields (max default hardware version, SR-IOV NICs with DVX details) that `QueryConfigTarget` does not return at cluster scope.
2. Materializes that discovery as a set of cluster-scoped CRDs: `ConfigTarget`, `VirtualMachineConfigOptions`, and `VirtualMachineGuestOptions`. There is no per-host CRD; all per-host data is aggregated onto `ConfigTarget.status`.
3. Checks hardware-version feasibility at VM admission via `ConfigTarget.status.maxHardwareVersion` (remaining admission work is tracked separately).

The whole pipeline shares the `vm_service_classless_vm` Supervisor capability with the Telco VM Service APIs (vmop-4313). This disabled-by-default capability replaces `supports_vm_service_vm_config_policy` and `supports_telco_vm_service_api`; activating or deactivating it updates the shared classless VM feature state. The retired keys alone do not enable the feature.

---

## Reconciliation pipeline (big picture)

```
Zone (existing)
  │
  ├─per cluster─▶ ConfigTarget (cluster-scoped)
  │                  │   (reconcile also walks the cluster's ESX hosts
  │                  │    and writes max default HW version + per-host
  │                  │    SR-IOV NIC details onto ConfigTarget.status)
  │                  │
  │                  └─per HW version─▶ VirtualMachineConfigOptions (cluster-scoped)
  │                                        └─per guest OS─▶ VirtualMachineGuestOptions (cluster-scoped)
```

Hardware-feasibility admission reads the cluster's `ConfigTarget.status` (in particular `maxHardwareVersion` and `sriov`). VM Class governance consumes the same discovery objects under its separate spec.

---

## User stories

### US1 — CSP admin: hardware discovery is automatic (Priority: P1)

A CSP admin creates a `Zone` on a Supervisor. Without any further action, the system automatically discovers what that cluster's vSphere EnvironmentBrowser reports and reflects it as structured Kubernetes objects.

**Why P1**: This is the root of the entire pipeline. Nothing else works without it.

**Independent test**: After a `Zone` is created, `kubectl get configtargets` and `kubectl get virtualmachineconfigoptions` return objects reflecting the cluster's real capabilities, including the per-host aggregations on `ConfigTarget.status`.

**Acceptance scenarios**:

1. **Given** a `Zone` with a valid `spec.namespace.poolMoIDs`, **when** the Zone controller reconciles, **then** a `ConfigTarget` with `metadata.name` equal to the derived cluster MoID exists.
2. **Given** a `ConfigTarget` exists, **when** the ConfigTarget controller reconciles, **then** `status.numCPUs`, `status.maxCPUsPerVM`, and security flags (`sevSupported`, `tdxSupported`, etc.) are populated from `QueryConfigTarget`.
3. **Given** a cluster with N supported hardware versions, **when** the ConfigTarget controller reconciles, **then** N `VirtualMachineConfigOptions` objects exist, one per version key (e.g. `vmx-21`, `vmx-22`).
4. **Given** a cluster with M ESX hosts whose `config.defaultHardwareVersion` values differ (e.g., `vmx-21`, `vmx-22`, `vmx-23`), **when** the ConfigTarget controller reconciles, **then** `ConfigTarget.status.maxHardwareVersion` equals the largest value across those hosts (`vmx-23` in the example), and an additional `Ready=True` reconcile does not change it.
5. **Given** a cluster with SR-IOV-capable hosts, **when** the ConfigTarget controller reconciles, **then** `ConfigTarget.status.sriov` contains one entry per (host, SR-IOV NIC) pair, populated by walking each host's `HostConfigInfo.pciPassthruInfo` (entries where `sriovCapable=true`), with each entry enriched from `HostHardwareInfo.dvxClasses` where `sriovNic=true`, and each entry carrying `hostMoID`, `active`, `maxVFs`, `numVFs`, and `dvxClass*` fields. The cluster-scope `QueryConfigTarget` call is never used for SR-IOV discovery.
6. **Given** the per-host RPC for one host transiently fails while the others succeed, **when** the ConfigTarget controller reconciles, **then** the failing host's SR-IOV entries are absent from `ConfigTarget.status.sriov` for this pass, the controller records a warning event referencing the host, and the reconcile is requeued so the missing data lands on a later pass. The successful hosts' data is still written.

---

### US2 — CSP admin: cluster capabilities are queryable from a single object (Priority: P1)

A CSP admin or an admission webhook needs to know the hardware capabilities of a zone's cluster from a single object lookup — no per-host fan-out, no list-and-label match.

**Why P1**: This is the basis for the webhook capability check in US5 and for per-zone hardware compatibility in the Flex VM Classes spec.

**Independent test**: After the pipeline stabilises, `kubectl get configtarget <clusterMoID> -o yaml` shows `status.maxHardwareVersion` populated and `status.sriov` containing one entry per (host, SR-IOV NIC) pair, with `hostMoID` attribution and DVX class fields filled where applicable.

**Acceptance scenarios**:

1. **Given** the cluster's hosts default to `vmx-21`, `vmx-22`, and `vmx-23`, **when** the ConfigTarget controller reconciles, **then** `ConfigTarget.status.maxHardwareVersion = vmx-23`.
2. **Given** a host's `config.defaultHardwareVersion` changes in vSphere, **when** the next ConfigTarget reconcile completes, **then** `ConfigTarget.status.maxHardwareVersion` is recomputed and reflects the new max.
3. **Given** a host is removed from the cluster, **when** the next ConfigTarget reconcile completes, **then** its SR-IOV entries are gone from `ConfigTarget.status.sriov` and any reduction in `maxHardwareVersion` is reflected.

---

### US4 — Tenant admin: stale objects are garbage-collected automatically (Priority: P2)

When a hardware version is dropped from a cluster's supported list, or when an ESX host is removed from a cluster, the corresponding Kubernetes objects are deleted without manual intervention.

**Why P2**: Correctness guarantee; prevents stale inventory from misleading admission.

**Independent test**: After a simulated cluster mutation (drop a HW version; remove a host), the matching `VirtualMachineConfigOptions` is gone within one ConfigTarget reconcile cycle and the host-derived data on `ConfigTarget.status` (max HW version, per-host SR-IOV entries) no longer references the removed host.

**Acceptance scenarios**:

1. **Given** a `VirtualMachineConfigOptions` for `vmx-19` exists, **when** `QueryConfigOptionDescriptor` no longer returns `vmx-19` in its result, **then** the ConfigTarget controller deletes the `vmx-19` object on its next successful reconcile.
2. **Given** a transient `QueryConfigTarget` error, **when** the controller retries, **then** no objects are garbage-collected (GC only runs on fully successful cluster-scope queries).
3. **Given** an ESX host has been evicted from the cluster, **when** the ConfigTarget controller completes its next reconcile, **then** the host's SR-IOV entries no longer appear in `ConfigTarget.status.sriov`, and any reduction in `max(perHostDefaultHardwareVersion)` is reflected in `ConfigTarget.status.maxHardwareVersion`.
4. **Given** a `VirtualMachineGuestOptions` has a `status.hardwareVersions` entry for `vmx-19`, **when** the `vmx-19` `VirtualMachineConfigOptions`'s next successful reconcile no longer reports that guest OS in `QueryConfigOptionEx`'s result, **then** the `vmx-19` entry is removed from `status.hardwareVersions`, and the `VirtualMachineGuestOptions` itself is deleted once that removal leaves it with no remaining hardware-version entries.

---

### US5 — DevOps user: VM admission reflects host capabilities (Priority: P1)

A VM requesting a hardware version unavailable in its zone is rejected with an actionable reason. This hardware-feasibility work remains separate from vmop-4318.

**Acceptance scenarios**:

1. **Given** the cluster's `ConfigTarget.status.maxHardwareVersion = vmx-21`, **when** a VM requests features requiring `vmx-23`, **then** admission rejects the request with the maximum available hardware version.
2. **Given** the cluster's `ConfigTarget` is missing or not ready, **when** feasibility is checked, **then** admission fails closed.

---

### US6 — Platform engineer: the pipeline is opt-in per Supervisor (Priority: P2)

A Supervisor can be brought up or upgraded without enabling the environment-browser pipeline; enabling or disabling the feature leaves the cluster in a clean state.

**Why P2**: Operational safety; allows phased rollout.

**Acceptance scenarios**:

1. **Given** the capability `CapabilityKeyVMServiceClasslessVM` (`"vm_service_classless_vm"`) is `false`, **when** a `Zone` reconciles, **then** no `ConfigTarget` objects are created.
2. **Given** the capability is disabled, **when** a `VirtualMachine` is admitted, **then** the webhook skips all `ConfigTarget`-based capability checks.
3. **Given** the capability is re-enabled after being disabled, **when** the first `Zone` reconcile completes, **then** the pipeline restores the full set of objects.

---

### US7 — Platform admin: restrict environment-browser access (vmop-4319)

Implementation is tracked by vmop-4319, separately from the policy-removal change.

Environment-browser discovery exposes cluster hardware and inventory. Access to `ConfigTarget`, `VirtualMachineConfigOptions`, and `VirtualMachineGuestOptions` is restricted to platform administrators and the service accounts required by the discovery controllers and admission webhooks. Tenant users, including namespace administrators, must not receive access through namespace roles.

**Acceptance scenarios**:

1. **Given** a platform administrator, **when** they get, list, or watch discovery objects, **then** access succeeds.
2. **Given** a tenant user or namespace administrator, **when** they get, list, or watch any of the three discovery resources, **then** access is forbidden.
3. **Given** the environment-browser capability is enabled, **when** its controllers and admission webhooks access discovery objects, **then** their service accounts retain the permissions required for normal operation.

## Edge cases

- `spec.id` on `ConfigTarget` is immutable after create; attempts to mutate it are rejected by the validation webhook.
- A `Zone` that references a cluster MoID for which vSphere returns no EB result causes the `ConfigTarget` to enter `Ready=False` with reason `ClusterNotFound`; the Zone reconcile is requeued.
- A user enables a hardware-version-gated feature (e.g., auto-NUMA, auto-placed SR-IOV) on an existing VM whose `spec.minHardwareVersion` is unset and whose placement has not yet converged. Because `spec.minHardwareVersion` cannot be decreased once raised, accepting the edit on an unsupported zone produces a VM that cannot be powered on or reconciled. The exact rejection rule (and the carve-out for "edit-to-fix-stuck-placement") is the subject of Spike vmop-3794; this spec records the edge case and defers the rule to that Spike's output.
- `PlaceVmsXCluster` is **not** assumed to honor auto-NUMA or auto-placed-SR-IOV compatibility at create time. The admission webhook must make those determinations from K8s objects alone (per Findings 1 and 6 in `research.md`) — specifically from `ConfigTarget.status.maxHardwareVersion` and `ConfigTarget.status.sriov`. See Spike vmop-3794.

---

## Out of scope

- VM governance, hardware ranges, ExtraConfig allow/deny rules, and create/update/power-on enforcement belong to the Flex VM Classes spec (vmop-4034, PR #2067).
- Sequence diagrams for the per-host iteration *(TDS A4)*.
- Automatic resize of running VMs *(separate epic)*.
- Telco-specific extra config fields (VMXNET3, latency sensitivity) *(vmop-3388)*.
- The 9.2 auto-NUMA and auto-placed-SR-IOV admission rules themselves *(open Spike vmop-3794; resulting Stories will land under vmop-3331)*. This spec captures only the edge case and the design constraint that the answer must be derivable from K8s objects.

---

## Review & acceptance checklist

- [ ] All user stories have at least two Given/When/Then scenarios.
- [ ] Each scenario is independently testable.
- [ ] Garbage collection behaviour is specified with a failure mode (transient error → no GC).
- [ ] Opt-in capability behaviour is specified.
- [ ] Immutability constraints on `spec.id` are called out.
- [ ] Out-of-scope items are listed.
- [ ] Open Spike vmop-3794 (`PlaceVmsXCluster` compatibility for auto-NUMA / auto-placed SR-IOV) is referenced from edge cases and out-of-scope; its conclusions will land as a separate spec/plan/tasks update.
