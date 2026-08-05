# Tasks: Host-Local Storage Support for VM Service

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Architecture**: [`architecture.md`](./architecture.md)

## Phase 1 — Setup

- [x] T001 Add `CapabilityKeyHostLocalStorage` + `updateCapabilitiesFeaturesFromCRD` mapping in `pkg/config/capabilities/capabilities.go`
- [x] T002 [P] Add `FeatureStates.HostLocalStorage` in `pkg/config/config.go`
- [x] T003 [P] Add unit tests for the new capability in `pkg/config/capabilities/capabilities_test.go`
- [x] T004 [P] Add `HostLocalPolicyStorageClassAnnotationKey` in `pkg/providers/vsphere/constants/constants.go`. The two `HostLocalSelectedNode*` keys added here were later removed — see T050 and T062

## Phase 2 — Foundational (kube helpers)

- [x] T005 Add `getESXHostInfoForNode` (originally `pkg/util/kube/node.go`; folded into the provider by T061)
- [x] T006 [P] Add unit tests for it
- [x] T007 Add `IsHostLocalStorageClass` and `GetPVCHostLocalHostname` in `pkg/util/kube/storage.go`
- [x] T008 [P] Add unit tests for both in `pkg/util/kube/storage_test.go`

## Phase 3 — User Story 1 (bound host-local PVC pins VM to host)

- [x] T009 [US1] Extend `placement.Constraints`/`Result` and `doesVMNeedPlacement` in `pkg/providers/vsphere/placement/zone_placement.go` to honor a caller-supplied `HostLocalHostMoID`
- [x] T010 [US1] Add `resolveHostLocalStorage` in `pkg/providers/vsphere/vmprovider_vm_hostlocal_storage.go` (override, PVC selected-node, and bound-PVC branches)
- [x] T011 [US1] Wire `resolveHostLocalStorage` into `vmCreateDoPlacement` in `pkg/providers/vsphere/vmprovider_vm.go`, behind the capability check at the call site
- [x] T012 [P] [US1] Add unit tests for the host-constraint bypass branch in `pkg/providers/vsphere/placement/zone_placement_test.go`
- [x] T013 [P] [US1] Add vcsim end-to-end tests (bound PVC → correct host, conflicting hostnames → error) in `pkg/providers/vsphere/vmprovider_vm_hostlocal_storage_test.go`

## Phase 4 — User Story 2 (pending WFFC host-local PVC auto-placement)

- [x] T014 [US2] Extend `CreateConfigSpecForPlacement` in `pkg/providers/vsphere/virtualmachine/configspec.go` to inject the policy-tagged phantom disk; thread the new `pvcs` param through `vmprovider_vm.go` and `vmprovider_vmgroup.go` call sites
- [x] T015 [P] [US2] Add unit tests for the phantom-disk injection in `pkg/providers/vsphere/virtualmachine/configspec_test.go`
- [x] T016 [US2] Force `PlaceVmsXCluster` with `HostRecommRequired=true` when `NeedHostLocalPlacement` is set, in `getPlacementRecommendation` (`zone_placement.go`)
- [x] T017 [US2] Add the `result.HostLocalPlacement` branch to `processPlacementResult` in `pkg/providers/vsphere/vmprovider_vm.go`
- [x] T018 [P] [US2] Add unit tests for the forced-DRS-host-recommendation branch in `zone_placement_test.go`
- [x] T019 [US2] Extend `handlePVCWithWFFC` in `controllers/virtualmachine/volume/volume_controller.go` for the host-local branch
- [x] T020 [US2] Extend `handlePVCWithWFFC` in `controllers/virtualmachine/volumebatch/volumebatch_controller.go` for the host-local branch
- [x] T021 [P] [US2] Add unit tests for both controllers' new branch (annotation present / absent / feature off)
- [ ] T022 [P] [US2] Add vcsim end-to-end test (pending WFFC PVC, no hint → DRS-chosen host, annotations written) in `vmprovider_vm_hostlocal_test.go` — still not done. The forced-DRS-recommendation mechanism is covered at the `zone_placement_test.go` (T018) and `configspec_test.go` (T015) unit level, and the path is now verified on a real cluster (T048), but no automated create-flow test exercises the two together end-to-end

## Phase 5 — User Story 3 (explicit host-override annotation) — REMOVED, see T062

- [x] T023 ~~[US3] Add the explicit-override branch~~ — reverted, see T062
- [x] T024 ~~[US3] Add `validateHostLocalSelectedNode`~~ — reverted, see T062
- [x] T025 ~~[P] [US3] Add unit tests for the validation rules~~ — reverted, see T062
- [x] T026 ~~[P] [US3] Add a vcsim test for the explicit-override path~~ — reverted, see T062

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
- [x] T048 Real-cluster verification of a **VKS/CAPI cluster** with host-local
      node volumes, covering both resolution paths:
      - **Immediate class, bound-PVC path.** A cluster with three additional
        node disks on `host-local-vmfs` exposed the multi-volume limitation:
        the control plane's three volumes happened to co-locate and its VM
        came up, while the worker's three landed on three different hosts of
        the four in the zone and its VM was correctly rejected with `VM has
        host-local PVCs bound to conflicting hosts`. Both machines had
        identical inputs, so the successful one does not demonstrate a
        guarantee. Recorded in `architecture.md` §11 item 5
      - **WFFC class, operator-selected path.** The same cluster shape on
        `host-local-vmfs-latebinding` succeeded on both machines. Each VM
        independently selected its own host (control plane `host-31`, worker
        `host-25`), VM Operator stamped `selected-node` plus
        `selected-node-is-zone=false` on every host-local PVC, and CNS
        provisioned each volume on exactly the stamped host. That the two
        machines chose *different* hosts and each still co-located its own
        volumes is what distinguishes this from coincidence
      - The VM's self-referential disk PVC (`dataSourceRef` → the
        `VirtualMachine`) is registered where the disk already lives rather
        than placed, so it always matches the VM's host — consistent with
        §11 item 3

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
      host (`status.nodeName` 10.161.28.9 == host-19, the host its PVC
      named), with
      `VirtualMachineConditionImageCacheReady` proving fast deploy was used,
      both host-local volumes bound on that host, and the guest booted with
      tools running. This also confirms **real DRS honors
      `PlacementSpec.Hosts` for `PlacementType=Create`**, so the
      `RelocateSpec.Host` fallback noted during design is not needed.

## Phase 9 — Code review rework

Addresses code-owner review feedback on the initial implementation.

- [x] T049 Stop recording the host on the VM. The MoID annotation was written
      during placement and persisted by the deferred patch even when the create
      then failed, while `resolveHostLocalPlacement` treated it as final and
      the webhook made it immutable — so a DRS host that could not take the VM
      pinned it permanently, with no recovery. Instance storage has a reset
      path for exactly this (`instanceStoragePlacementFailed` deletes both of
      its annotations); this feature had none. The host is now derived afresh
      every reconcile and nothing is committed before the VM exists
- [x] T050 Drop `HostLocalSelectedNodeMOIDAnnotationKey` entirely, along with
      its admission validation. The remaining node annotation is a caller
      input only
- [x] T051 Thread the derived host to placement through
      `Constraints.HostLocalHostMoID` and the derived zone through
      `Constraints.Zones`, so `doesVMNeedPlacement` no longer reads annotations
      and the zone label is still recorded from the placement result
- [x] T052 Publish the decision on the PVC rather than the VM: a new
      `reconcileHostLocalStorage` update step maps the host the VM actually
      runs on back to its Supervisor node and stamps `selected-node` plus
      `selected-node-is-zone=false` on the VM's unprovisioned host-local PVCs.
      Running every reconcile makes it self-correcting. Add
      `getNodeNameForESXHostMoID` for the reverse
      lookup, which is preferred over `GetESXHostFQDN` because it reads the
      same Node annotation as the forward lookup and so is guaranteed to match
      `kubernetes.io/hostname`
- [x] T053 Remove the host-local branch from both volume reconcilers so there
      is exactly one writer of a host-local PVC's selected node
- [x] T054 Add `AddPVCPlacementDisks` in the **VM provider**, one
      placement-only disk per PVC carrying that PVC's storage policy, closing
      the pre-existing `TODO: PVC volumes` in `CreateConfigSpecForPlacement`.
      PVCs whose data source is the VM itself are skipped, since those disks
      already exist in the ConfigSpec where
      `vmCreateGenConfigSpecImagePVCDataSourceRefs` sets their policy and size
      — counting them again would double-count the same storage. The shared
      predicate `kubeutil.HasVirtualMachineDataSourceRef` now expresses that
      rule once, replacing the inline check in `GetPVCZoneConstraints`. This
      leaves `configspec.go` and `devices.go` byte-identical to `main`.
      **Note:** this still changes placement for every VM with PVCs, not only
      host-local ones
- [ ] T059 Always request a host recommendation rather than only for
      instance storage / host-local, which has been called out as a false
      optimization. **Not done, and not self-contained.** Attempting it broke
      five pre-existing placement specs, because `processPlacementResult`
      copies any recommended host into `createArgs.HostMoID`, and a non-empty
      `HostMoID` *pins the create* — `host` for fast deploy,
      `Target.HostID` for the content library. Making the request
      unconditional therefore also pins every VM in the system to a specific
      host, which is a far wider behavior change than the request itself and
      cuts against the mobility concerns raised in the same review. It needs
      "a host was recommended" separated from "the VM must be created there",
      as its own change with its own testing. A `TODO` at the call site in
      `zone_placement.go` records this
- [x] T055 Move the capability check from `resolveHostLocalStorage` to its
      call site, per the project pattern
- [x] T056 Replace `continue` statements in the resolution helpers with
      positive conditions and small named predicates
- [x] T057 Rename to `vmprovider_vm_hostlocal_storage.go` and its test file
- [ ] T058 Re-verify on a real cluster after this rework. The prior T047/T048
      runs validated the *previous* design; the handoff now happens after
      create and via the PVC, so both the WFFC and bound-PVC paths need
      re-confirming

### Deferred
- [ ] T041 Host-local placement is not honored for VMs placed via a
      `VirtualMachineGroup`: `vmCreateDoPlacementByGroup` returns before
      `resolveHostLocalStorage` runs, and `GroupPlacement` has no notion of
      a pinned host. Documented as unsupported in `spec.md`. Note the
      per-PVC placement disks from T054 *do* now apply to group placement.

## Phase Final — Polish

- [ ] T029 Add E2E coverage in `test/e2e/vmservice/vmservice/virtualmachine/vm_hostlocal_storage.go`, registered from the vmservice E2E suite entrypoint, skipped unless `supports_host_local_storage` is active
- [x] T030 Update `docs/concepts/workloads/vm-placement.md` with a "Host-Local Storage" section describing the three resolution sources and the annotation contract
- [ ] T031 Add release notes (per `pull-request-standards.md`)
- [ ] T032 Flip `spec.md` status to `Implemented` once every acceptance criterion is covered

## Phase 10 — Review follow-up: verify the DRS premise, then simplify

Review asked whether supplying the already-provisioned disk to placement would
let DRS derive the host by itself, removing the PVC-annotation dependency and
simplifying the placement plumbing.

- [x] T061 Spike the DRS premise against real vCenter before changing product
      code, since nothing in the repo exercised it and vcsim cannot answer it.
      **Result: the premise is false.** Supplying the disk with its real
      datastore backing makes `PlaceVm` return a recommendation with *no host*,
      and `PlaceVmsXCluster` recommend a host that cannot even see the
      datastore. A different field, `PlacementSpec.Datastores`, *does* work and
      returns the datastore's single host. Also corrected an earlier wrong
      claim: the datastore of a bound PVC **is** obtainable — the PV's
      `volumeHandle` is the FCD ID and `CnsQueryVolume` returns its
      `datastoreUrl`. Full method and data in [`research.md`](./research.md)
- [x] T062 Drop user story 3, the caller-supplied host override. It was
      modeled on the zone-label escape hatch and was not part of the original
      request. Reverts `webhooks/virtualmachine/validation/` entirely to
      `main`, drops the annotation key, and removes one resolution source — so
      VM Operator now reads and writes **no VM annotation at all**, which is
      the direct answer to the review question "why annotate the VM at all"
- [x] T063 Fold `getESXHostInfoForNode` and `getNodeNameForESXHostMoID` into
      `vmprovider_vm_hostlocal_storage.go` as unexported helpers and delete
      `pkg/util/kube/node.go` plus its test. `IsHostLocalStorageClass` and
      `HasVirtualMachineDataSourceRef` stay in `pkg/util/kube/storage.go`
      because the volume reconcilers and `GetPVCZoneConstraints` respectively
      depend on them; `GetPVCHostLocalHostname` stays too, since it shares the
      CSI topology annotation constants with the zone logic and moving it would
      duplicate those literals across packages
- [x] T064 Real-cluster verification after this round, on a second Supervisor
      (`domain-c9`, cluster `566c6125-5e58-401e-90b4-050f419b3e4c`), confirming
      host derivation still works with the annotation source removed:
      - A VKS cluster with two additional node disks on
        `host-local-vmfs-latebinding` (WFFC) came up with both the control
        plane and worker `PoweredOn`, on **different** hosts
        (`10.161.147.51` and `10.161.151.72`)
      - Neither VM carries any `hostlocal-*` annotation — confirms the
        annotation is genuinely gone, not just unread
      - All four WFFC PVCs are `Bound` with `selected-node` equal to their own
        VM's host and `selected-node-is-zone=false`, and CNS provisioned each
        on exactly that host (`accessible-topology` matches `selected-node`)
      - Each machine's own two volumes are co-located on that machine's host
      - Both machines' self-referential boot-disk PVCs (`dataSourceRef` →
        the VM, `Immediate` class) are correctly left unstamped
        (`selected-node` absent)

      **Supplementary: host-derived path.** Also verified a standalone VM
      (`vm-3z88`) created directly against an already-`Bound` host-local PVC
      (`host-local-pvc`, `Immediate` class). The VM landed on
      `10.161.147.51`, exactly the host named in that PVC's
      `accessible-topology`, with no `hostlocal-*` annotation on the VM. The
      bound PVC itself was correctly left unstamped (`selected-node` absent
      — CNS already placed it, nothing to publish), and the VM's separate
      self-referential boot-disk PVC (`dataSourceRef` → the VM) was excluded
      from host-local resolution regardless of its own (unrelated, async)
      `Pending` phase. This confirms the host-derived path — the other half
      of host resolution not exercised by the WFFC cluster above — on real
      DRS with the reworked, annotation-free code.
