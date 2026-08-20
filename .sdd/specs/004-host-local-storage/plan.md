# Implementation Plan: Host-Local Storage Support for VM Service

- **Branch**: `add-support-for-hostlocalstorage`
- **Date**: 2026-07-25
- **Spec**: [`spec.md`](spec.md)
- **Architecture**: [`architecture.md`](architecture.md)

---

## Summary

Add host-level VM placement, gated by the `supports_host_local_storage`
Supervisor capability, for VMs backed by host-local `StorageClass` PVCs.

VM Operator never derives the host. Each of the VM's provisioned volumes is
named in the placement ConfigSpec by its real datastore path, and DRS works the
host out from those paths, since only one host can reach a host-local datastore.
For a volume that is not provisioned yet there is no path, so DRS picks a host
whose storage satisfies the volume's policy; once the VM exists on that host it
is published back to the PVC as `selected-node` so CNS provisions there.

> **This plan records the original intent and has been overtaken in places.**
> The implementation diverged where reality disagreed: the fast-deploy
> interaction (tasks.md Phase 7–8), the ordering of the PVC handoff (Phase 9),
> the removal of the caller-supplied annotation (Phase 10), and finally the
> move from deriving the host ourselves to letting DRS derive it from the
> ConfigSpec (Phase 11). Where this file and tasks.md disagree, tasks.md is
> correct.

---

## Technical context

| Field | Value |
|-------|-------|
| **Language** | Go (this repo's toolchain) |
| **Primary dependencies** | `govmomi`/`vim25` (`vimtypes`), `controller-runtime` — no new dependencies added |
| **API server** | Kubernetes (vSphere Supervisor) |
| **Testing** | Ginkgo v2 + Gomega; `envtest`/`vcsim` for integration; real WCP Supervisor for E2E |
| **Target platform** | VMware vSphere Supervisor (WCP) |

### Feature gate

`pkgcfg.Features.HostLocalStorage`, mirrored from the Supervisor capability
`supports_host_local_storage` (`Capabilities.Status.Supervisor` map),
exactly like the existing `VirtualMachineConfigPolicy` capability
(`pkg/config/capabilities/capabilities.go`). No FSS env var exists in this
repo for it — the VC FSS `HostLocalStorageSupport` → WCP capability
activation wiring lives outside this repository. Every new code path below
is inert when this is `false`.

---

## Constitution check

- [x] Business logic lives in `pkg/providers/vsphere/`, `pkg/util/kube/`,
      and `pkg/providers/vsphere/virtualmachine/`; the `VirtualMachine`
      controller and the volume/volumebatch controllers stay thin — they
      only call into provider/util functions.
- [x] No controller calls vSphere APIs directly; host resolution reads a
      Kubernetes `Node` object (`pkg/util/kube`) and vCenter's `HostSystem`
      FQDN (existing `vcenter.GetESXHostFQDN`) only from within the
      provider package.
- [x] `supports_host_local_storage` gates the entire surface; disabled
      behavior is byte-for-byte identical to today.
- [x] No new CRD; only new annotations on the existing `VirtualMachine` and
      `PersistentVolumeClaim` resources.
- [x] Test files use the single `_test.go` + `Label()` convention.
- [x] Any change observable on a Supervisor cluster ships with E2E coverage
      in the same change set (`e2e-sync-with-changes.md`).

---

## Repository layout (this feature)

### Documentation (specs/)

```
specs/004-host-local-storage/
├── spec.md   — this feature's functional spec (what & why)
├── plan.md   — this file (how)
└── tasks.md  — ordered task checklist
```

### Source

```
pkg/config/capabilities/capabilities.go        MODIFY — CapabilityKeyHostLocalStorage
pkg/config/config.go                            MODIFY — FeatureStates.HostLocalStorage
pkg/config/capabilities/capabilities_test.go    MODIFY — new capability test cases

external/infra/api/v1alpha1/storagepolicy_types.go  MODIFY — StoragePolicyStatus.HostLocal
pkg/util/vsphere/storage/storage_policy.go      MODIFY — IsHostLocalStorageCapabilityPolicy

pkg/util/kube/storage.go                        MODIFY — IsHostLocalStorageProfile
pkg/util/kube/storage_test.go                   MODIFY

pkg/providers/vsphere/placement/zone_placement.go       MODIFY — Constraints.NeedHostLocalPlacement,
                                                                   Result.HostLocalPlacement,
                                                                   doesVMNeedPlacement, getPlacementRecommendation
pkg/providers/vsphere/placement/zone_placement_test.go  MODIFY

pkg/providers/vsphere/virtualmachine/configspec.go      MODIFY — phantom disk injection in
                                                                   CreateConfigSpecForPlacement
pkg/providers/vsphere/virtualmachine/configspec_test.go MODIFY

pkg/providers/vsphere/vmprovider_vm_hostlocal_storage.go NEW   — resolveHostLocalStorage,
                                                                 AddPVCPlacementDisks,
                                                                 reconcileHostLocalStorage
pkg/providers/vsphere/vmprovider_vm_hostlocal_test.go   NEW    — vcsim end-to-end coverage
pkg/providers/vsphere/vmprovider_vm.go                  MODIFY — call site in vmCreateDoPlacement,
                                                                   new branch in processPlacementResult
pkg/providers/vsphere/vmprovider_vmgroup.go             MODIFY — thread new CreateConfigSpecForPlacement param

controllers/virtualmachine/volume/volume_controller.go            MODIFY — handlePVCWithWFFC
controllers/virtualmachine/volume/volume_controller_unit_test.go  MODIFY
controllers/virtualmachine/volumebatch/volumebatch_controller.go            MODIFY — handlePVCWithWFFC
controllers/virtualmachine/volumebatch/volumebatch_controller_unit_test.go  MODIFY

controllers/virtualmachine/virtualmachine/virtualmachine_controller.go  MODIFY — +kubebuilder:rbac nodes marker
config/rbac/role.yaml                                                    REGEN (no diff expected)

test/e2e/vmservice/vmservice/virtualmachine/vm_hostlocal_storage.go  NEW
```

---

## Implementation phases

### Phase 1 — Capability & foundations

1. Add `CapabilityKeyHostLocalStorage = "supports_host_local_storage"` and
   its `updateCapabilitiesFeaturesFromCRD` mapping arm
   (`pkg/config/capabilities/capabilities.go`), and `FeatureStates.
   HostLocalStorage bool` (`pkg/config/config.go`).
2. Surface the policy's host-local SPBM capability on
   `StoragePolicyStatus.HostLocal` (`external/infra/api/v1alpha1/
   storagepolicy_types.go`, `pkg/util/vsphere/storage/storage_policy.go`) and
   add `IsHostLocalStorageProfile` to read it back
   (`pkg/util/kube/storage.go`).

### Phase 2 — Placement bypass (US1)

1. Extend `placement.Constraints`/`Result` and `doesVMNeedPlacement`
   (`pkg/providers/vsphere/placement/zone_placement.go`) to honor
   `constants.HostLocalSelectedNodeMOIDAnnotationKey` exactly like the
   existing `InstanceStorageSelectedNodeMOIDAnnotationKey` check, and to
   thread a new `NeedHostLocalPlacement` flag into `getPlacementRecommendation`.
2. Add `resolveHostLocalPlacement` (`pkg/providers/vsphere/
   vmprovider_vm_hostlocal_storage.go`) implementing the priority order
   from `plan`'s design doc (already-resolved → explicit override → bound
   PVC → needs auto-placement), called from `vmCreateDoPlacement` before
   `CreateConfigSpecForPlacement`/`placement.Placement`.

### Phase 3 — Auto-placement for the pending case (US2)

1. Extend `CreateConfigSpecForPlacement` (`pkg/providers/vsphere/
   virtualmachine/configspec.go`) to accept the VM's PVCs and inject one
   placement-only phantom `VirtualDisk` per unresolved `Pending` host-local
   PVC, carrying that PVC's `StorageClass`'s SPBM policy ID.
2. Force the modern `PlaceVmsXCluster` path with `HostRecommRequired: true`
   when `NeedHostLocalPlacement` is set (`getPlacementRecommendation`).
3. Add the `processPlacementResult` branch that resolves the DRS-chosen
   host's FQDN (`vcenter.GetESXHostFQDN`) and writes both host-local
   annotations onto the VM.

### Phase 4 — PVC annotation stamping (US2)

1. Extend `handlePVCWithWFFC` in both `controllers/virtualmachine/volume/
   volume_controller.go` and `controllers/virtualmachine/volumebatch/
   volumebatch_controller.go`: when the PVC's `StorageClass` is host-local,
   read `HostLocalSelectedNodeAnnotationKey` off the VM and stamp
   `selected-node`/`selected-node-is-zone: "false"`, erroring (retryable)
   when the annotation isn't resolved yet.

### Phase 5 — Webhook validation (US3)

1. Add `validateHostLocalSelectedNode` to `webhooks/virtualmachine/
   validation/virtualmachine_validator.go`: existence-on-Create for the node
   annotation (mirrors `validateAvailabilityZone`'s zone-existence check) and
   immutability-on-Update with the same privileged-account carve-out. The
   annotation is a caller input only; VM Operator never writes it.

### Phase 6 — RBAC & manifests

1. Add the `+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch`
   marker to the `VirtualMachine` controller (the aggregate ClusterRole
   already grants this — added purely for self-documentation at the call
   site). Run `make generate-manifests` and confirm no diff.

### Phase 7 — Tests

Unit, integration, and E2E coverage per the Testing strategy table below;
see `tasks.md` for the per-file breakdown.

---

## RBAC

`nodes: get;list;watch` is already granted cluster-wide
(`config/rbac/role.yaml`), exercised today only by
`controllers/infra/node/infra_node_controller.go`. No role changes are
required; Phase 6 above only adds a documentation marker.

---

## Testing strategy

| Layer | Mechanism | Location |
|-------|-----------|----------|
| Unit | `*_test.go` + Ginkgo `Label()` | `pkg/config/capabilities/capabilities_test.go`, `pkg/util/kube/node_test.go`, `pkg/util/kube/storage_test.go`, `pkg/providers/vsphere/placement/zone_placement_test.go`, `pkg/providers/vsphere/virtualmachine/configspec_test.go`, `webhooks/virtualmachine/validation/virtualmachine_validator_unit_test.go` |
| Integration | `*_test.go` + `testlabels.EnvTest`/`testlabels.VCSim` | `pkg/providers/vsphere/vmprovider_vm_hostlocal_test.go`, `controllers/virtualmachine/volume/volume_controller_unit_test.go`, `controllers/virtualmachine/volumebatch/volumebatch_controller_unit_test.go` |
| E2E | Ginkgo, real Supervisor, skipped unless capability active | `test/e2e/vmservice/vmservice/virtualmachine/vm_hostlocal_storage.go` |

---

## Risk and rollback

| Risk | Mitigation |
|------|-----------|
| DRS recommends a host that cannot reach a provisioned volume | The volume's real datastore path is in the placement ConfigSpec, and DRS validates every disk against the host it picks, so an unreachable host yields no recommendation rather than a wrong one |
| `PlaceVmsXCluster` ignores the disks in the ConfigSpec | Host-local placement routes through `PlaceVm`, which does not |
| A volume told which host to use but not yet bound | It has no datastore for the ConfigSpec, so placement waits for it to bind rather than letting DRS pick a different host |
| A tentative DRS host choice becoming permanently stuck on the VM | Nothing about the host is recorded on the VM; it is derived afresh each reconcile and only published to the PVC once the VM exists |

**Disable path**: set `supports_host_local_storage` to `false` on the
Supervisor. All new code paths become no-ops; VMs revert to zone-only
placement exactly as before this feature existed.
