# Feature Specification: Host-Local Storage Support for VM Service

- **Feature branch**: `add-support-for-hostlocalstorage`
- **Created**: 2026-07-25
- **Status**: Draft
- **Architecture**: [`architecture.md`](architecture.md)

---

## Summary

VM Service VMs are today always placed onto a *zone/cluster*: `pkg/providers/
vsphere/placement` resolves a namespace/zone to candidate ResourcePools and
lets DRS pick any host within the chosen cluster. This breaks for
**host-local storage** — a direct attached VMFS datastore mounted on a single
ESXi host in the cluster (not a cluster-wide/shared datastore), identified by
the `StorageLocality` capability on its SPBM storage policy. A
VM's disk can only live on the one ESXi host that owns that local datastore,
but VM Operator's placement has no concept of "this specific host." Today, a
VM backed by a host-local PVC can be scheduled onto a host with no access at
all to the datastore the PVC lives on.

This feature adds host-level placement, keyed off the PVC's own CSI/CNS
topology annotations. [`architecture.md`](architecture.md) groups the cases
below into two modes: **host-derived** (case 1, where the host is already known
and the VM follows it) and **operator-selected** (case 2, where no host exists
yet and the volume follows the VM).

1. A PVC whose PV is already `Bound` — the PV's location is fixed, so the VM
   must be pinned to the exact host it lives on.
2. A `Pending` PVC on a `WaitForFirstConsumer` host-local `StorageClass` with
   no host hint yet available anywhere — VM Operator must itself pick a host
   (and, implicitly, a compliant local datastore) the same way it already
   picks a zone, then hand that host to CNS by stamping
   `volume.kubernetes.io/selected-node` on the PVC.
Because a host belongs to exactly one vSphere cluster and one zone, resolving
the host also determines the cluster and zone; no cluster is treated as
"active" or preferred. See `architecture.md` §2.1 and §5.3.

This entire surface remains inert unless the new Supervisor capability
`supports_host_local_storage` (backed by VC FSS `HostLocalStorageSupport`,
wired outside this repository) is activated for the Supervisor.

---

## Reconcile pipeline (big picture)

```
VirtualMachine + PersistentVolumeClaim(s) on a host-local StorageClass
  │
  └─vmCreateDoPlacement─▶ hostLocalPlacementNeeded  (capability checked by caller)
        │
        ├─ A Bound PVC ────────────────────────────────────────▶ its real disk path goes in the ConfigSpec;
        │                                                        DRS returns the one host that can reach it
        ├─ A Pending PVC already told a host ──────────────────▶ no datastore yet, so wait for it to bind
        └─ A Pending/WFFC PVC has no host anywhere ────────────▶ Constraints.NeedHostLocalPlacement = true
                                                                      │
                                                                      ▼
                                          AddPVCPlacementDisks adds a policy-tagged
                                          placement-only disk per PVC (excluding the
                                          VM's own disks, i.e. dataSourceRef → VM)
                                                                      │
                                                                      ▼
                                          placement.Placement forces PlaceVmsXCluster
                                          (HostRecommRequired=true) → Result.HostMoRef
                                                                      │
                                                                      ▼
                                          the VM is created on that host; nothing is
                                          recorded about the host on the VM
                                                                      │
                                                                      ▼
                              (next reconcile) reconcileHostLocalStorage maps the host the
                              VM actually runs on back to its Supervisor node and stamps
                              PVC.selected-node = node, selected-node-is-zone = "false"
```

---

## User stories

### US1 — DevOps user: VM with an already-bound host-local PVC lands on the right host (Priority: P0)

A DevOps user creates a `VirtualMachine` that references an existing,
`Bound` PVC backed by a host-local `StorageClass`. VM Operator creates the
VM on the exact ESXi host the PV's datastore is mounted on.

**Why P0**: This is the concrete bug being fixed — today the VM can land on
a host with no access to the datastore at all, and VM creation/attachment
fails or the VM cannot see its data.

**Independent test**: Create a host-local `StorageClass`, a PV/PVC pair
bound to a specific host's local datastore, and a VM referencing that PVC.
Assert (via govmomi) the VM's actual ESXi host is the single host that mounts
the datastore the volume lives on.

**Acceptance scenarios**:

1. **Given** `supports_host_local_storage` is enabled and a VM references a
   `Bound` PVC on a host-local `StorageClass` whose volume lives on a datastore
   mounted only by host `H`, **when** the VM is created, **then** it is created
   on `H`, and nothing about the host is recorded on the VM — the volume's disk
   path is re-resolved on every reconcile.
2. **Given** two host-local PVCs on the same VM whose volumes live on
   datastores owned by two different hosts, **when** the VM is created,
   **then** placement yields no recommendation and the VM is not created on a
   host that cannot reach both.
3. **Given** `supports_host_local_storage` is disabled, **when** the same VM
   is created, **then** placement behavior is unchanged from today (the
   host-local annotations/StorageClass annotation are ignored entirely).

---

### US2 — DevOps user: VM with a pending, WaitForFirstConsumer host-local PVC gets a compliant host picked automatically (Priority: P0)

A DevOps user creates a `VirtualMachine` referencing a `Pending` PVC on a
`WaitForFirstConsumer` host-local `StorageClass`, with no host hint anywhere
yet. VM Operator selects a host with a datastore compliant with that
`StorageClass`'s storage policy, creates the VM there, and stamps the PVC so
CNS provisions the volume on that same host.

**Why P0**: Without this, VM Operator has no way to satisfy a genuinely
"pick a host for me" WFFC host-local request at all — the PVC's own
annotations carry no host at this point in its lifecycle.

**Independent test**: Create a host-local `WaitForFirstConsumer`
`StorageClass`, a `Pending` PVC on it, and a VM referencing that PVC.
Assert the resulting VM's host has an accessible datastore compliant with
the `StorageClass`'s storage policy, and that the PVC ends up with
`volume.kubernetes.io/selected-node` set to that host's Supervisor node name
and `cns.vmware.com/selected-node-is-zone: "false"`.

**Acceptance scenarios**:

1. **Given** `supports_host_local_storage` is enabled and a VM references a
   `Pending` PVC on a WFFC host-local `StorageClass` with no requested-
   topology hint, **when** the VM is created, **then** VM Operator injects a
   placement-only disk carrying the `StorageClass`'s SPBM policy ID and asks
   `PlaceVm` for a host recommendation, so the host DRS returns is one with a
   compliant datastore.
2. **Given** the VM has been created on a host, **when** the provider next
   reconciles it, **then** it maps that host back to its Supervisor node and
   sets the PVC's `selected-node` to that node with `selected-node-is-zone`
   `"false"` — never `"true"`.
3. **Given** the VM does not yet exist on a host, **when** the same reconcile
   runs, **then** nothing is published to the PVC: the host is not committed
   to anywhere until the VM is actually created on it.
4. **Given** the PVC is on an `Immediate` class, or its data source is the VM
   itself, **when** the provider reconciles, **then** it is left alone — CNS
   needs no node for the former, and the latter is one of the VM's own disks.

---

## Sentinel value semantics

| Annotation | Absent | Set by caller | Set by VM Operator |
|------------|--------|----------------|----------------------|
| `volume.kubernetes.io/selected-node` (on the PVC) | No host published yet | An explicit host for that volume | The host the VM was created on, published once the VM exists; also how a later reconcile recovers the decision |
| `cns.vmware.com/selected-node-is-zone` (on the PVC) | n/a (existing annotation) | n/a — VM Operator only | `"false"` for host-local StorageClasses (vs. the pre-existing `"true"` for zone-based WFFC) |

---

## Edge cases

- A VM with volumes on **both** a host-local `StorageClass` and an ordinary
  zone-based `StorageClass` is unaffected for the zone-based volume. Every
  volume's disk path goes into the placement ConfigSpec regardless; only the
  host-local one narrows the result to a single host.
- Multiple host-local volumes on one VM that already live on different hosts
  leave DRS no host that can reach all of them, so placement fails rather than
  creating a VM that cannot attach its disks — see US1 scenario 2.
- A `Pending` PVC on an **Immediate**-binding-mode host-local `StorageClass`
  does not fail — it **waits**. `GetPVCZoneConstraints`' existing "PVC is not
  bound" error blocks placement and is retried, while CNS provisions the
  volume without needing a consumer and picks the host itself. Once the PVC is
  `Bound`, the next reconcile puts its real disk path in the placement
  ConfigSpec, so DRS lands the VM on whichever host CNS chose. This is the path
  VKS/CAPI node volumes take.
  - Consequently, a requested-topology annotation naming a **zone but no
    hostname** deadlocks: CNS cannot satisfy it for host-local storage, and
    placement is meanwhile waiting for the bind that will never happen. Such
    a PVC must either omit the annotation, letting CNS choose, or name a
    hostname.
  - **More than one host-local volume on a VM requires a
    `WaitForFirstConsumer` StorageClass.** Under `Immediate` with no
    requested topology, each volume is provisioned independently and may land
    on a different host — leaving no host that can reach them all, and the
    bound volumes cannot afterwards be moved. Verified on a Supervisor with
    four candidate hosts: a VKS cluster requesting three additional node disks
    on an `Immediate` host-local class had one machine's volumes co-locate and
    the other machine's land on three different hosts, from the same spec.
    This is **not new to host-local storage**: it is the existing behavior of
    `Immediate`-bound zonal volumes applied at host granularity, where
    `GetPVCZoneConstraints` already rejects PVCs whose zone sets do not
    intersect. Host-local storage only makes the case more likely to be
    reached, since a cluster has many hosts whereas most namespaces have a
    single zone.
- **DRS resolves the host; VM Operator does not.** For a volume that already
  exists, its real datastore path is put on that volume's placement-only disk
  in the ConfigSpec, and DRS returns the one host that can reach it. No host
  and no datastore is passed to placement, and nothing about the host is
  derived, recorded, or made immutable — a VM whose create fails is free to be
  placed elsewhere on the next attempt. Host-local placement is issued through
  `PlaceVm`, because `PlaceVmsXCluster` was measured to return the same host
  regardless of the disk backings in the ConfigSpec.
- **Every PVC contributes a placement disk, not only host-local ones.** A
  placement-only disk carrying the PVC's storage policy is added for each of
  the VM's PVCs, so the recommendation accounts for all of the VM's storage
  rather than only the VM's own files. A path on shared storage names a
  datastore every host can reach and so constrains nothing, which is why the
  volumes need not be classified first.
- A VM with **both** instance storage volumes and a host-local PVC is not
  supported: instance storage needs placement to choose a host freely, while a
  host-local volume admits only one.
- **VM mobility after create** depends on how the VM itself is provisioned. A
  VM provisioned with a host-local storage class has its home and its disks on
  the host-local datastore and cannot move to another host, since that would
  require a storage vMotion. A VM provisioned with a shared zonal policy that
  also requests a host-local `WaitForFirstConsumer` volume carries no such
  guarantee: nothing keeps it on the host chosen at create time, and once CNS
  has provisioned the volume it cannot follow. That configuration is not
  supported.
- **VM Groups.** Group placement issues `PlaceVmsXCluster`, so the disk-path
  mechanism above is unavailable and the host constraint is not honored — a
  real 3-VM batch call of the exact shape it sends, each VM carrying its own
  already-provisioned host-local volume on a distinct host, came back with
  every VM given the same substituted host and datastore, unrelated to any of
  the three real locations, deterministically across three trials. The PVCs
  are still passed so the recommendation accounts for their storage policies.
  This is an observed property of that API rather than a statement from the
  DRS team; an RFE asks DRS to honor ConfigSpec disk backings there, or to
  accept a per-VM host constraint (`vmop-NNNN`). A host-local VM with
  `Spec.GroupName` set falls back to whatever placement its group computes.
- `VirtualMachineStatus.NodeName` (`api/v1alpha6`) already exists but serves a
  different purpose (VKS node identity) and is not reused by this feature. The
  host a pending volume must be provisioned on is published to the PVC, and no
  annotation is written on the VirtualMachine at all.

---

## Review & acceptance checklist

- [ ] All user stories have at least two Given/When/Then scenarios.
- [ ] Each scenario is independently testable.
- [ ] `supports_host_local_storage` opt-in behavior is specified for every
      user story.
- [ ] Conflicting host-local requirements across volumes on one VM are
      specified as a hard error.
- [ ] VM mobility guarantees are stated per storage-class configuration.
- [ ] Out-of-scope items (VM Groups, instance storage) are listed.
