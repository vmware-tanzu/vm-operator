# Research & Prior Art: Expose Disk List in VirtualMachineSnapshot

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)

## Background & Prior Art

Data protection solutions in Kubernetes (such as Velero and external backup operators) interacting with Supervisor clusters rely on the Container Storage Interface (CSI) Backup API to perform crash-consistent and application-consistent backup and restore operations for virtual machines.

To take differential/incremental backups, backup applications must discover:
1. Which specific virtual disks were captured in a snapshot.
2. The Changed Block Tracking (CBT) ChangeID associated with each virtual disk at the point in time the snapshot was created.

In vSphere, Changed Block Tracking (CBT) assigns a unique ChangeID to each snapshot point of a virtual disk when CBT is enabled on the VM (`ctkEnabled = true`). The backup utility compares the change ID between two snapshots to query only the modified disk extents via VDDK / CBT APIs.

## vCenter Snapshot Object Representation

When a VM snapshot is taken in vCenter, the snapshot node retains a virtual hardware configuration snapshot:
- `VirtualMachineSnapshot.config.hardware.device` contains the slice of `vimtypes.BaseVirtualDevice`.
- Virtual disks are represented as `*vimtypes.VirtualDisk`.
- Different storage and provisioning types utilize different backings:
  - `VirtualDiskFlatVer2BackingInfo`: Standard flat disk backing (VMFS / vSAN).
  - `VirtualDiskSeSparseBackingInfo`: Space-efficient sparse backing.
  - `VirtualDiskSparseVer2BackingInfo`: Monolithic/split sparse backing.
  - `VirtualDiskRawDiskMappingVer1BackingInfo`: Raw Device Mapping (RDM).
- Each backing contains:
  - `Uuid`: Global unique disk identifier.
  - `ChangeId`: CBT change identifier string.

## Reconcile Error Handling Invariant

In `pkg/providers/vsphere/vmprovider_vmsnapshot.go`, `ReconcileCurrentSnapshot` marks a snapshot as in-progress (`CreationInProgressReason`) before taking the vCenter snapshot.

If any subsequent step in `ReconcileCurrentSnapshot` fails without clearing this status (via `markSnapshotFailed`), future reconciliations will detect the in-progress status and short-circuit:
```go
if Reason == CreationInProgressReason {
    return nil // wait for in-progress snapshot to finish
}
```
This causes the snapshot to hang permanently. Therefore, any error in `getDisksFromSnapshot` MUST invoke `markSnapshotFailed` before returning to guarantee that failures transition the condition to `Ready=False` with `SnapshotCreationFailed`.
