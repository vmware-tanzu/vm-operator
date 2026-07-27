# Tasks: Host-Local Storage Support for VM Service

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Architecture**: [`architecture.md`](./architecture.md)

## Phase 1 — Setup

- [x] T001 Add `CapabilityKeyHostLocalStorage` + `updateCapabilitiesFeaturesFromCRD` mapping in `pkg/config/capabilities/capabilities.go`
- [x] T002 [P] Add `FeatureStates.HostLocalStorage` in `pkg/config/config.go`
- [x] T003 [P] Add unit tests for the new capability in `pkg/config/capabilities/capabilities_test.go`
- [x] T004 [P] Add `HostLocalPolicyStorageClassAnnotationKey`, `HostLocalSelectedNodeMOIDAnnotationKey`, `HostLocalSelectedNodeAnnotationKey` in `pkg/providers/vsphere/constants/constants.go`

## Phase 2 — Foundational (kube helpers)

- [x] T005 Add `GetESXHostInfoForNode` in `pkg/util/kube/node.go`
- [x] T006 [P] Add unit tests for `GetESXHostInfoForNode` in `pkg/util/kube/node_test.go`
- [x] T007 Add `IsHostLocalStorageClass` and `GetPVCHostLocalHostname` in `pkg/util/kube/storage.go`
- [x] T008 [P] Add unit tests for both in `pkg/util/kube/storage_test.go`

## Phase 3 — User Story 1 (bound host-local PVC pins VM to host)

- [x] T009 [US1] Extend `placement.Constraints`/`Result` and `doesVMNeedPlacement` in `pkg/providers/vsphere/placement/zone_placement.go` to honor the MoID annotation
- [x] T010 [US1] Add `resolveHostLocalPlacement` in `pkg/providers/vsphere/vmprovider_vm_hostlocal.go` (already-resolved and bound-PVC branches)
- [x] T011 [US1] Wire `resolveHostLocalPlacement` into `vmCreateDoPlacement` in `pkg/providers/vsphere/vmprovider_vm.go`
- [x] T012 [P] [US1] Add unit tests for the annotation-bypass branch in `pkg/providers/vsphere/placement/zone_placement_test.go`
- [x] T013 [P] [US1] Add vcsim end-to-end tests (bound PVC → correct host, conflicting hostnames → error) in `pkg/providers/vsphere/vmprovider_vm_hostlocal_test.go`

## Phase 4 — User Story 2 (pending WFFC host-local PVC auto-placement)

- [x] T014 [US2] Extend `CreateConfigSpecForPlacement` in `pkg/providers/vsphere/virtualmachine/configspec.go` to inject the policy-tagged phantom disk; thread the new `pvcs` param through `vmprovider_vm.go` and `vmprovider_vmgroup.go` call sites
- [x] T015 [P] [US2] Add unit tests for the phantom-disk injection in `pkg/providers/vsphere/virtualmachine/configspec_test.go`
- [x] T016 [US2] Force `PlaceVmsXCluster` with `HostRecommRequired=true` when `NeedHostLocalPlacement` is set, in `getPlacementRecommendation` (`zone_placement.go`)
- [x] T017 [US2] Add the `result.HostLocalPlacement` branch to `processPlacementResult` in `pkg/providers/vsphere/vmprovider_vm.go`
- [x] T018 [P] [US2] Add unit tests for the forced-DRS-host-recommendation branch in `zone_placement_test.go`
- [x] T019 [US2] Extend `handlePVCWithWFFC` in `controllers/virtualmachine/volume/volume_controller.go` for the host-local branch
- [x] T020 [US2] Extend `handlePVCWithWFFC` in `controllers/virtualmachine/volumebatch/volumebatch_controller.go` for the host-local branch
- [x] T021 [P] [US2] Add unit tests for both controllers' new branch (annotation present / absent / feature off)
- [ ] T022 [P] [US2] Add vcsim end-to-end test (pending WFFC PVC, no hint → DRS-chosen host, annotations written) in `vmprovider_vm_hostlocal_test.go` — not yet done; the forced-DRS-recommendation mechanism is covered at the `zone_placement_test.go` (T018) and `configspec_test.go` (T015) unit level, but no full create-flow vcsim test exercises the two together end-to-end

## Phase 5 — User Story 3 (explicit host-override annotation)

- [x] T023 [US3] Add the explicit-override branch to `resolveHostLocalPlacement`
- [x] T024 [US3] Add `validateHostLocalSelectedNode` (existence-on-Create, immutability-on-Update, MoID-annotation rejection) in `webhooks/virtualmachine/validation/virtualmachine_validator.go`
- [x] T025 [P] [US3] Add unit tests for all three validation rules in `webhooks/virtualmachine/validation/virtualmachine_validator_unit_test.go`
- [x] T026 [P] [US3] Add a vcsim end-to-end test for the explicit-override path in `vmprovider_vm_hostlocal_test.go`

## Phase 6 — RBAC & manifests

- [x] T027 Add the `nodes` RBAC marker to `controllers/virtualmachine/virtualmachine/virtualmachine_controller.go`
- [x] T028 Run `make generate-manifests` and confirm no diff to `config/rbac/role.yaml`

## Phase 7 — Fix: pinned host discarded when FastDeploy is enabled

Found during real-cluster testing. A VM correctly resolved and recorded its
pinned host but was created on a different host, so CNS volume attach failed
with `Invalid configuration for device '0'`. Root cause:
`doesVMNeedPlacement` sets `needDatastorePlacement = Features.FastDeploy`
unconditionally, and `Placement`'s DRS-bypass early return requires it to be
false — so on a FastDeploy-enabled cluster the bypass never fired and DRS's
fresh host recommendation silently replaced the pin.

- [x] T033 ~~Add `HasHostLocalStorage` to `pkg/util/kube/storage.go`~~ -
      reverted, see T042
- [x] T034 ~~Add unit tests for `HasHostLocalStorage`~~ - reverted, see T042
- [x] T035 ~~Disable fast deploy for host-local VMs in the `VirtualMachine`
      controller~~ - reverted, see T042
- [x] T036 Make a resolved host authoritative in
      `pkg/providers/vsphere/placement/zone_placement.go`: error when a
      recommendation contradicts the pin, and always prefer the pin when
      building `Result`. The error is conditioned on a non-nil recommendation
      host, since a nil host is legitimate (implied placement, or
      `PlaceVmsXCluster` without `HostRecommRequired`) and is covered by
      preferring the pin
- [x] T037 [P] Add placement tests: pin honored with fast deploy on; pin
      preserved on the early-return path; and the authoritative-host error
      (triggered via instance storage, the one path that always returns a DRS
      host recommendation)
- [x] T038 Correct the comment in `zone_placement.go` that claimed the
      function disables fast deploy — reworded again in Phase 8, since fast
      deploy is no longer disabled for host-local VMs
- [x] T039 ~~Real-cluster verification of this phase~~ — superseded by T047,
      since Phase 8 replaced this phase's approach
- [ ] T048 Still outstanding from this phase: verify a **VKS/CAPI cluster**
      with a host-local volume. T047 validated a standalone VM; the
      originally-reported failure was a VKS cluster VM, whose PVC is
      Immediate-bound before the VM exists (the bound-PVC resolution path
      rather than the WFFC auto-placement path)

## Phase 8 — Fix: keep fast deploy, constrain placement to the pinned host

Disabling fast deploy (T035) turned out to be the wrong fix. It moved
host-local VMs onto the content-library path, which cannot pin a datastore
when a storage profile is set, so vCenter chose a datastore unreachable from
the pinned host: `Invalid configuration for device '2'. Device: VirtualDisk.
Provided backing file is not accessible from host.` Fast deploy is in fact
the only create path that places the VM's files on an explicit datastore.

- [x] T042 Revert T033-T035: drop the controller guard and the
      `HasHostLocalStorage` helper, so host-local VMs keep fast deploy
- [x] T043 Add an optional `hostMoRef` to `PlaceVMForCreate`
      (`cluster_placement.go`) that sets `PlacementSpec.Hosts`, and thread it
      through `getPlaceVMRecommendation`
- [x] T044 Route the pinned-and-needs-datastore case to that constrained
      `PlaceVm` call in `getPlacementRecommendation`, fused with the existing
      instance-storage case so a VM that is both keeps working. Pass
      `curResult` rather than three positional bools
- [x] T045 [P] Add a per-test `NumClusterHosts` override to
      `builder.VCSimTestConfig`, needed because govmomi's simulator indexes
      `hosts[rand.Intn(len(c.Host))]` in `PlaceVm`, so a single-element
      `Hosts` list panics unless the cluster has one host. File the simulator
      bug upstream and drop the override on the next govmomi bump
- [x] T046 [P] Add placement tests: the constrained call carries the pinned
      host in `PlacementSpec.Hosts` and returns a datastore for fast deploy;
      and a response that contradicts the pin still errors (via a
      `RoundTripper` spy, since vcsim otherwise echoes the constrained host)
- [x] T047 Real-cluster verification of the pinned + fast deploy path.
      Confirmed on a WCP Supervisor: a VM with `spec.storageClass:
      host-local-vmfs` plus a WFFC host-local PVC was created on its pinned
      host (`status.nodeName` 10.161.28.9 == host-19 ==
      `hostlocal-selected-node-moid`), with
      `VirtualMachineConditionImageCacheReady` proving fast deploy was used,
      both host-local volumes bound on that host, and the guest booted with
      tools running. This also confirms **real DRS honors
      `PlacementSpec.Hosts` for `PlacementType=Create`**, so the
      `RelocateSpec.Host` fallback noted during design is not needed.

### Deferred
- [ ] T041 Host-local placement is not honored for VMs placed via a
      `VirtualMachineGroup`: `vmCreateDoPlacementByGroup` returns before
      `resolveHostLocalPlacement` runs, and `GroupPlacement` has no notion of
      a pinned host. Documented as unsupported in `spec.md`.

## Phase Final — Polish

- [ ] T029 Add E2E coverage in `test/e2e/vmservice/vmservice/virtualmachine/vm_hostlocal_storage.go`, registered from the vmservice E2E suite entrypoint, skipped unless `supports_host_local_storage` is active
- [x] T030 Update `docs/concepts/workloads/vm-placement.md` with a "Host-Local Storage" section describing the three resolution sources and the annotation contract
- [ ] T031 Add release notes (per `pull-request-standards.md`)
- [ ] T032 Flip `spec.md` status to `Implemented` once every acceptance criterion is covered
