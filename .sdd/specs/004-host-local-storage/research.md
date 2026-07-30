# Research: how a VM's host can be derived for host-local storage

- **Spec**: [`spec.md`](spec.md)
- **Architecture**: [`architecture.md`](architecture.md)

---

## Question

The implementation derives a VM's host by reading the PVC's CSI topology annotation and then constraining placement to that host. Review asked whether supplying the **already-provisioned disk** to placement would let DRS work the host out by itself, removing the annotation dependency and the placement plumbing.

Nothing in the repository exercised that, and vcsim cannot answer it, so it was measured against real DRS.

## Environment

vCenter 9.2.0, cluster `domain-c9` (`wcp-sanity-cluster`) with four hosts: `host-19`, `host-25`, `host-31`, `host-37`. Each host has three single-host `local-*` VMFS datastores; `sharedVmfs_0` and `nfs0-1` are mounted on all ten hosts in the datacenter, and there are three vSAN datastores.

The probe volume was a bound host-local PVC, `pvc-263c4bab-54b5-4813-9333-055860bc3d44`.

## Finding 1: the datastore of a bound volume is obtainable

It is **not** present in any Kubernetes object. The PV's `spec.csi.volumeAttributes` carries only `type` and `csiProvisionerIdentity`; `cnsvolumeinfo.spec` carries volumeID, capacity, storageClassName, storagePolicyID and vCenterServer. Neither has a datastore.

However the PV's `volumeHandle` **is the FCD ID**, and CNS answers for it:

```
volumeId:        263c4bab-54b5-4813-9333-055860bc3d44
datastoreUrl:    ds:///vmfs/volumes/6a63b79d-9bbe4a0e-525a-02003c4e06c0/
storagePolicyId: c0ecfefa-1f10-415f-b929-68aa81215d2a
backing path:    [local-2] fcd/144f4bae4cb34cf2bc72109efb181220.vmdk
```

That URL is datastore `local-2` (`datastore-79`), **mounted on exactly one host: `host-19`** — which is `lvn-dvm-10-161-28-9`, precisely the `selected-node` on that PVC. So the CNS-derived host and the annotation-derived host agree.

`cns.NewClient` / `QueryVolume` and `vslm.NewObjectManager` are already used in `test/e2e/vmservice/vmservice/viadmin/registervm.go`, so no new dependency is required.

## Finding 2: DRS does *not* infer the host from a disk's backing datastore

`ClusterComputeResource.PlaceVm` with `PlacementType=Create`, five trials per arm:

| Arm | ConfigSpec / spec | Recommended host |
|---|---|---|
| reference | no disk beyond the VM home | `host-31` (5/5) |
| control | synthetic create-disk with the host-local **policy** only | `host-31` (5/5) |
| **treatment/backing** | the existing FCD's disk with its **real datastore backing**, no file operation | **no host in recommendation** (5/5) |
| **treatment/dsField** | policy-only disk **plus `PlacementSpec.Datastores=[local-2]`** | **`host-19`** (5/5) |
| xcluster treatment | real backing, via `PlaceVmsXCluster` + `HostRecommRequired` | `host-31` (5/5) |

Two conclusions:

- **Supplying the real backing does not work.** `PlaceVm` returns a recommendation with no host at all, and `PlaceVmsXCluster` recommends `host-31` — a host that cannot even see `local-2`. DRS ignores the backing datastore when placing a new VM.
- **`PlacementSpec.Datastores` does work.** Handing DRS the datastore yields the single host that mounts it, consistently, and distinguishably from every other arm (`host-19` vs `host-31`).

Note the control arm was *consistent* rather than varying — DRS is deterministic in this idle cluster — so the discriminating evidence is `treatment/dsField` returning a **different** host from every other arm, not variance within an arm.

## Consequences

The originally proposed design — put the provisioned disk in the placement ConfigSpec with its real backing and let DRS conclude the host — is **disproven** and should not be pursued.

A different mechanism achieves the same intent: query CNS for the volume's datastore and pass it as `PlacementSpec.Datastores` instead of deriving a host and passing `PlacementSpec.Hosts`. Trade-offs:

- **For:** uses the volume's actual location rather than trusting a CSI annotation's semantics; removes the annotation parsing and the Node-object lookup for the bound-volume path.
- **Against:** adds a vCenter round-trip per bound PVC on the create path, and a new failure mode there. It does **not** reduce the change materially — the placement routing, the constraint field on `Constraints`, and the `cluster_placement.go` parameter all remain, with a datastore in place of a host.
- **Unaffected either way:** the WaitForFirstConsumer path. With no volume yet there is nothing to query, so DRS still chooses the host and that host must still be published to the PVC. That is the bulk of the change.

`PlacementSpec` also carries `Hosts`, `StoragePods` and `Rules`; only `Hosts` (used today) and `Datastores` (measured here) were exercised.

## Reproducing

The probe is a standalone govmomi program: connect, resolve the cluster and the host-local datastore, then issue `PlaceVm` per arm and report the recommended host. It performs **no writes** — only `PlaceVm` / `PlaceVmsXCluster` queries and property reads. Credentials came from `wcp-vmop-sa-vc-auth` in `vmware-system-vmop` (the account the provider itself uses) or an SSO administrator.
