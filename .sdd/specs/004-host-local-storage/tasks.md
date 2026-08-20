# Tasks: Host-Local Storage Support for VM Service

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Architecture**: [`architecture.md`](./architecture.md)

Tasks are listed against the design as implemented. `[P]` marks work that could proceed in parallel with its siblings.

## Phase 1 — Feature gate

- [x] T001 Add `CapabilityKeyHostLocalStorage` and its `updateCapabilitiesFeaturesFromCRD` mapping in `pkg/config/capabilities/capabilities.go`
- [x] T002 [P] Add `FeatureStates.HostLocalStorage` in `pkg/config/config.go`
- [x] T003 [P] Add capability unit tests in `pkg/config/capabilities/capabilities_test.go`

## Phase 2 — Detect a host-local storage policy from SPBM

A policy is host-local when SPBM reports the `StorageLocality` capability for it. Nothing about the StorageClass itself identifies host-local storage, so detection reads the policy.

- [x] T004 Add `IsHostLocalStorageCapabilityPolicy` in `pkg/util/vsphere/storage/storage_policy.go`, and set `StoragePolicyStatus.HostLocal` when building the policy status
- [x] T005 Add the `HostLocal` field to `StoragePolicyStatus` in `external/infra/api/v1alpha1/storagepolicy_types.go`, and regenerate `config/crd/external-crds/infra.vmware.com_storagepolicies.yaml`
- [x] T006 Add `IsHostLocalStorageProfile` in `pkg/util/kube/storage.go`, which resolves a policy ID to its `StoragePolicy` CR and reads that status
- [x] T007 [P] Add unit tests for both in `pkg/util/kube/storage_test.go` and the storage-policy suite

## Phase 3 — User Story 1: a VM using an already-provisioned host-local volume runs on that volume's host

DRS derives the host itself. Each `Bound` PVC's real datastore path is put in the placement `ConfigSpec`, and DRS returns the only host that can reach it. VM Operator never computes, passes, or stores a host.

- [x] T008 [US1] Add `AddPVCPlacementDisks` in `pkg/providers/vsphere/vmprovider_vm_hostlocal_storage.go` — one placement-only disk per PVC carrying that PVC's storage policy, closing the pre-existing `TODO: PVC volumes` in `CreateConfigSpecForPlacement`. PVCs whose data source is the VM itself are skipped, since `vmCreateGenConfigSpecImagePVCDataSourceRefs` already put those disks in the ConfigSpec with their policy and size; counting them again would double-count the same storage. The shared predicate `kubeutil.HasVirtualMachineDataSourceRef` states that rule once, replacing the inline check in `GetPVCZoneConstraints`. This leaves `configspec.go` and `devices.go` byte-identical to `main`
- [x] T009 [US1] Add `pvcDiskPaths` to resolve each `Bound` PVC's real datastore path — the PV's `volumeHandle` is the FCD ID, and `CnsQueryVolume` returns the backing disk path — and set it as the `FileName` of that PVC's placement disk. Applied to **every** bound PVC, host-local or shared: a shared volume's path constrains nothing, so there is no reason to branch
- [x] T010 [US1] Call both from `vmCreateDoPlacement` in `pkg/providers/vsphere/vmprovider_vm.go`, with the capability checked at the call site per the project pattern
- [x] T011 [US1] Route host-local placement through `PlaceVm` in `pkg/providers/vsphere/placement/zone_placement.go`. `PlaceVmsXCluster` was measured to return the same host regardless of the ConfigSpec's disks, so it cannot honor this constraint
- [x] T012 [P] [US1] Add placement tests asserting host-local placement uses `PlaceVm` and returns a host, in `pkg/providers/vsphere/placement/zone_placement_test.go`
- [x] T013 [P] [US1] Add vcsim tests for the disk-path and placement-disk construction in `pkg/providers/vsphere/vmprovider_vm_hostlocal_storage_test.go` and `pkg/providers/vsphere/virtualmachine/configspec_test.go`

## Phase 4 — User Story 2: a pending `WaitForFirstConsumer` host-local volume is provisioned on the host the VM lands on

With no volume yet, there is no disk path and nothing to constrain. DRS picks the host, and VM Operator publishes that choice to the PVC so CSI provisions there.

- [x] T014 [US2] Add `hostLocalPlacementNeeded` — requests a host recommendation when the VM has an unprovisioned host-local PVC. It only needs to know *that* a host is required, not which one
- [x] T015 [US2] Add `NeedHostLocalPlacement` to `placement.Constraints` and `HostLocalPlacement` to `Result`, and honor them in `doesVMNeedPlacement`
- [x] T016 [US2] Add the `reconcileHostLocalStorage` update step: map the host the VM actually runs on back to its Supervisor node via `getNodeNameForESXHostMoID`, and stamp `selected-node` plus `selected-node-is-zone=false` on the VM's unprovisioned host-local PVCs. It runs every reconcile, which makes it self-correcting. The reverse lookup reads the same Node annotation as the placement side, so it is guaranteed to agree with `kubernetes.io/hostname`
- [x] T017 [US2] Wait for a bind rather than racing it: a PVC already stamped with a `selected-node` but not yet `Bound` returns `pkgerr.RequeueError`, so DRS is not asked to choose a host that could differ from the one CSI is provisioning on
- [x] T018 [US2] Skip the zone stamp for host-local PVCs in `handlePVCWithWFFC` in `controllers/virtualmachine/volume/volume_controller.go`. Without this, a `Pending` host-local PVC is stamped with a zone, and CSI can act on that before the provider stamps the host — putting the volume on the wrong host
- [x] T019 [US2] Apply the same guard in `controllers/virtualmachine/volumebatch/volumebatch_controller.go`, so there is exactly one writer of a host-local PVC's selected node
- [x] T020 [P] [US2] Add unit tests for both reconcilers' guard (host-local / not host-local / feature off)
- [x] T021 [P] [US2] Add a per-test `NumClusterHosts` override to `builder.VCSimTestConfig`. Needed because govmomi's simulator indexes `hosts[rand.Intn(len(c.Host))]` in `PlaceVm`. File the simulator bug upstream and drop the override on the next govmomi bump
- [ ] T022 [P] [US2] Add a vcsim test covering the create flow end to end — pending WFFC PVC → DRS-chosen host → PVC stamped. The mechanism is covered at the unit level (T015, T016) and verified on a real cluster (T024), but no automated test exercises the two together

## Phase 5 — RBAC

- [x] T023 Grant `get;list;watch` on `persistentvolumes` to the manager, and regenerate `config/rbac/role.yaml`. Required by `pvcDiskPaths`, which reads a `PersistentVolume` to resolve a PVC's CNS volume ID — a type nothing in this repository read before. Found by the failure it caused, not by review: the cached client's watch failed `Forbidden` and retried indefinitely, so every VM with a bound PVC, host-local or not, silently stalled on `PlacementReady`

## Phase 6 — Verification

- [x] T024 Real-cluster verification on a Supervisor backed by real DRS, since vcsim cannot answer whether DRS honors a disk path:
      - A VM referencing an already-`Bound` host-local PVC landed on exactly the ESXi host that mounts that PVC's datastore. No host and no datastore was passed anywhere in the placement call; DRS derived it purely from the volume's disk path in the ConfigSpec
      - A VM with **two** `WaitForFirstConsumer` host-local PVCs: both were stamped with the same node's `selected-node` (`selected-node-is-zone=false`), CSI provisioned both volumes there (`ProvisioningSucceeded` for each), and the VM was created on that host — so the multi-volume co-location guarantee holds under this design, not only under the annotation-derived one it replaced
      - Both VMs reached `PoweredOn` with no host-local-related reconcile errors. The one transient failure was an ordinary async CNS-attach race that resolved on retry
      - This run is also what surfaced the missing RBAC grant (T023)

## Phase 7 — Polish

- [x] T025 Add a "Host-Local Storage" section to `docs/concepts/workloads/vm-placement.md`
- [ ] T026 Add E2E coverage in `test/e2e/vmservice/vmservice/virtualmachine/vm_hostlocal_storage.go`, registered from the vmservice E2E suite entrypoint, skipped unless `supports_host_local_storage` is active
- [ ] T027 Add release notes (per `pull-request-standards.md`)
- [ ] T028 Flip `spec.md` status to `Implemented` once every acceptance criterion is covered

## Known limitations

- [ ] T029 Host-local placement is not honored for VMs placed via a `VirtualMachineGroup`. `vmCreateDoPlacementByGroup` computes a batch, multi-VM, cross-cluster recommendation that has no notion of a per-VM host constraint. The per-PVC placement disks from T008 *do* apply to group placement, so storage policies are still accounted for; only the host constraint is missing. Documented as unsupported in `spec.md`
- [ ] T030 A host recommendation is requested only when a feature needs one — instance storage or host-local — rather than always. Making it unconditional is not self-contained: `processPlacementResult` copies any recommended host into `createArgs.HostMoID`, and a non-empty `HostMoID` *pins the create*, so an unconditional request would pin every VM in the system to a specific host. It needs "a host was recommended" separated from "the VM must be created there", as its own change with its own testing. A `TODO` at the call site in `zone_placement.go` records this
