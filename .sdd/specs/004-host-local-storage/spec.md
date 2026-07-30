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
**host-local storage** — a VMFS-L or VMFS datastore that is direct-attached
to, and mounted on, a single ESXi host in the cluster (not a cluster-wide/
shared datastore), identified by the `cns.vmware.com/hostLocalPolicy: "true"`
annotation on a `StorageClass` and a compatible host-local storage policy. A
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
  └─vmCreateDoPlacement─▶ resolveHostLocalStorage  (capability checked by caller)
        │
        ├─ A PVC already has selected-node ────────────────────▶ resolve Node → MoID + zone, pin, DRS bypassed
        ├─ A Bound PVC's accessible-topology names a host ──────▶ resolve Node → MoID + zone, pin, DRS bypassed
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
Assert (via govmomi) the VM's actual ESXi host equals the Node named in the
PVC's `csi.vsphere.volume-accessible-topology` annotation.

**Acceptance scenarios**:

1. **Given** `supports_host_local_storage` is enabled and a VM references a
   `Bound` PVC on a host-local `StorageClass` whose `volume-accessible-
   topology` annotation names host `H`, **when** the VM is created, **then**
   VM Operator constrains VM creation to `H`, and records nothing about the
   host on the VM — it is re-derived from the PVC on every reconcile.
2. **Given** two host-local PVCs on the same VM whose accessible-topology
   annotations name two different hosts, **when** the VM is created,
   **then** VM Operator fails with a clear, actionable error rather than
   picking one arbitrarily.
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
   placement-only phantom disk carrying the `StorageClass`'s SPBM policy ID,
   forces a `PlaceVmsXCluster` host recommendation, and pins the VM to the
   recommended host.
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
  zone-based `StorageClass` is unaffected for the zone-based volume; only
  the host-local volume drives host-level pinning.
- Multiple host-local PVCs on one VM that disagree on host (bound-vs-bound,
  or bound-vs-explicit-override) is a hard error, not a "pick one"
  heuristic — see US1 scenario 2.
- A `Pending` PVC on an **Immediate**-binding-mode host-local `StorageClass`
  does not fail — it **waits**. `GetPVCZoneConstraints`' existing "PVC is not
  bound" error blocks placement and is retried, while CNS provisions the
  volume without needing a consumer and picks the host itself. Once the PVC
  is `Bound`, the next reconcile resolves that host from its topology
  annotation, so the VM follows CNS's choice. This is the path VKS/CAPI node
  volumes take.
  - Consequently, a requested-topology annotation naming a **zone but no
    hostname** deadlocks: CNS cannot satisfy it for host-local storage, and
    placement is meanwhile waiting for the bind that will never happen. Such
    a PVC must either omit the annotation, letting CNS choose, or name a
    hostname.
  - **More than one host-local volume on a VM requires a
    `WaitForFirstConsumer` StorageClass**, which is also satisfied when every
    host-local PVC names the same hostname. Under `Immediate` with no
    requested topology, each volume is provisioned independently and may land
    on a different host — which the conflict check above then rejects, and the
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
- **A resolved host is authoritative.** Once a host has been resolved, no
  placement recommendation may replace it — the VM's disks may only be
  reachable from that host, so a different host leaves the VM unable to
  attach them. `Placement` fails loudly if a recommendation contradicts the
  pin rather than silently placing the VM elsewhere.
- **`FastDeploy` stays enabled for host-local VMs, and placement is
  constrained to the pinned host instead.** Fast deploy is the only create
  path that places the VM's files on an explicit datastore, which is what
  keeps them on the pinned host's host-local datastore. The content-library
  path cannot pin a datastore when a storage profile is set
  (`DeploymentSpec.DefaultDatastoreID` is only honored when there is no
  profile), so vCenter is free to choose a datastore the pinned host cannot
  reach — observed on a real cluster as `Invalid configuration for device
  '2'. Device: VirtualDisk. Provided backing file is not accessible from
  host.` Because fast deploy needs a datastore recommendation, a pinned VM
  cannot take the placement early return; instead the recommendation is
  obtained from the legacy `PlaceVm` API constrained via
  `PlacementSpec.Hosts` to the pinned host, so the datastore it returns is
  reachable from that host. The modern `PlaceVmsXCluster` cannot express this
  constraint for a create.
- **Host-local placement always requests datastore recommendations.** The
  placement ConfigSpec carries a phantom disk tagged with the host-local
  storage policy purely so the recommended host is one with a compliant
  datastore, and DRS only evaluates that policy when it is asked to recommend
  a datastore. This must not be conditioned on the fast deploy feature.
- A VM with **both** instance storage volumes and a host-local PVC is not
  supported: instance storage needs placement to choose a host while
  host-local requires a specific one. This surfaces as the
  authoritative-host error above rather than a silently mis-placed VM.
- VM Groups: host-local storage placement (explicit override, bound-PVC
  pinning, and auto-placement alike) is not supported when a VM is placed
  via its `VirtualMachineGroup`, since that batch, multi-VM, cross-cluster
  DRS flow is independent of the per-VM placement path this feature
  extends. A host-local VM with a `Spec.GroupName` set falls back to
  whatever zone-only placement its group computes.
- `VirtualMachineStatus.NodeName` (`api/v1alpha6`) already exists but serves
  a different purpose (VKS node identity) and is not reused by this
  feature — host info lives in annotations only, consistent with the
  existing Instance Storage feature's own precedent.

---

## Review & acceptance checklist

- [ ] All user stories have at least two Given/When/Then scenarios.
- [ ] Each scenario is independently testable.
- [ ] `supports_host_local_storage` opt-in behavior is specified for every
      user story.
- [ ] The explicit-request annotation's validation/immutability rules are
      specified (existence on Create, immutability on Update).
- [ ] Conflicting host-local requirements across volumes on one VM are
      specified as a hard error.
- [ ] Out-of-scope items (FastDeploy interaction) are listed.
