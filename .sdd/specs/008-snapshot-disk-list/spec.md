# Feature Specification: Expose Disk List in VirtualMachineSnapshot

- **Feature branch**: `disk_list_in_vm_snapshot`
  - **Fork**: `cc005511/vm-operator`
  - **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-08-18
- **Status**: In Review
- **Epic**: vmop-52730
- **Design docs**: WIKI page 2781824956

---

## Background

Data protection and backup services (such as CSI Backup API / Velero plugins) running on Supervisor clusters require awareness of the exact virtual disks and their Changed Block Tracking (CBT) identifiers captured at the moment a virtual machine snapshot is created.

Prior to this feature, `VirtualMachineSnapshotStatus` exposed high-level snapshot metadata (`powerState`, `quiesced`, `uniqueID`, `children`, `conditions`, `storage`), but did not expose the underlying disk devices or their CBT change IDs. As a result, backup solutions could not determine which disk backings corresponded to the snapshot, nor could they query incremental changes for backup and restore operations without out-of-band vCenter lookups.

This feature adds `status.disks` to `VirtualMachineSnapshot` in `v1alpha6` (with lossless round-trip conversion to/from `v1alpha5`), populated during snapshot reconciliation when the `supports_CSI_Backup_API` capability is enabled on the Supervisor.

---

## Goals

- **MUST** introduce `Disks []VirtualMachineSnapshotDiskStatus` to `VirtualMachineSnapshotStatus` in API version `v1alpha6`.
- **MUST** define `VirtualMachineSnapshotDiskStatus` with required `ID` (disk UUID) and optional `ChangedBlockTrackingID` (CBT change ID at snapshot creation time).
- **MUST** gate the population of `status.disks` behind the Supervisor capability `supports_CSI_Backup_API` and corresponding feature flag `pkgcfg.Features.CSIBackupAPI`.
- **MUST** inspect the snapshot's hardware devices from vCenter and extract disk UUID and ChangeId for supported disk backing types (`VirtualDiskFlatVer2BackingInfo`, `VirtualDiskSeSparseBackingInfo`, `VirtualDiskSparseVer2BackingInfo`, `VirtualDiskRawDiskMappingVer1BackingInfo`).
- **MUST** ensure lossless round-trip conversion between `v1alpha5` and `v1alpha6` via annotation marshaling in conversion webhooks.
- **MUST** maintain the snapshot reconciliation invariant: if retrieving disk details fails, the snapshot MUST be marked as failed via `markSnapshotFailed` to prevent the snapshot from remaining permanently stuck in `CreationInProgressReason`.
- **MUST** provide end-to-end (E2E) testing validating snapshot disk list and CBT extraction on a live Supervisor cluster when the capability is enabled.

---

## Non-goals

- VM Operator does not perform disk backup, data transfer, or snapshot export itself — it only surfaces the disk metadata on the snapshot status for backup controllers and CSI plugins to consume.
- This feature does not mount snapshot disks as volume sources to VMs; that capability is designed and delivered in a subsequent feature (`mount_vm_snapshot_disk`).
- Backporting `status.disks` as a native typed field to `v1alpha5` CRD schema is not required; `v1alpha5` down-converts by dropping the field and preserves it across round-trips via the standard `utilconversion` annotation mechanism.

---

## User stories / acceptance criteria

### CSI Backup Service / Partner Engineer

- **Given** a Supervisor cluster with `supports_CSI_Backup_API` capability activated, **When** a `VirtualMachineSnapshot` is taken for a VM with disks, **Then** `VirtualMachineSnapshot.status.disks` is populated with the list of disks, each containing the disk UUID (`id`) and the CBT change ID (`changedBlockTrackingID`).
- **Given** a VM with CBT enabled (`vm.spec.advanced.changeBlockTracking: true`), **When** a snapshot is created, **Then** each disk in `status.disks` contains a non-empty `changedBlockTrackingID`.

### Tenant / DevOps User

- **Given** a cluster without `supports_CSI_Backup_API` capability activated, **When** a snapshot is created, **Then** the snapshot creation completes successfully and `status.disks` remains empty/omitted, preserving backward compatibility.
- **Given** a failure occurs while querying disk hardware from vSphere for a snapshot, **When** reconciliation runs, **Then** the snapshot is marked with `Ready=False` and reason `SnapshotCreationFailed`, clearing the in-progress condition so the CR does not hang forever.

---

## Open questions

- None. Implementation and E2E validation completed.
