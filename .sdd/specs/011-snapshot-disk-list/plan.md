# Implementation Plan: Expose Disk List in VirtualMachineSnapshot

- **Spec**: [`spec.md`](./spec.md)
- **Epic**: vmop-52730
- **Date**: 2026-08-18

## Summary

Extend `VirtualMachineSnapshotStatus` in `api/v1alpha6` with a new `Disks []VirtualMachineSnapshotDiskStatus` field that reports the UUID and Changed Block Tracking (CBT) ID of each disk captured in the snapshot. When the Supervisor capability `supports_CSI_Backup_API` is enabled, the vSphere snapshot reconciler queries the snapshot's hardware devices from vCenter, extracts the backing UUID and `ChangeId`, and populates `status.disks`. Lossless conversion between `v1alpha5` and `v1alpha6` is supported via conversion webhooks.

## Technical context

- **Go version**: `1.26.8`
- **API version(s) touched**: `v1alpha6` (storage version), `v1alpha5` (conversion)
- **Modules touched**: root module (`api/`, `config/`, `pkg/`, `test/`)
- **New dependencies**: none

## Constitution check

| Rule | Status | Notes |
|------|--------|-------|
| API compatibility | OK | Additive field on `VirtualMachineSnapshotStatus` in `v1alpha6`. Down-conversion to `v1alpha5` preserves data via annotation marshaling (`utilconversion`). |
| CRD manifests regenerated | OK | `config/crd/bases/vmoperator.vmware.com_virtualmachinesnapshots.yaml` regenerated with controller-gen. Conversion webhook manifests updated. |
| Controllers are thin / Provider boundaries | OK | Provider logic resides in `pkg/providers/vsphere/vmprovider_vmsnapshot.go` and `pkg/util/kube/vmsnapshot.go`. No vCenter calls in controllers. |
| In-progress state recovery invariant | OK | When `getDisksFromSnapshot` encounters an error, `markSnapshotFailed` is called before returning to ensure the `CreationInProgressReason` condition is cleared. |
| Feature gating | OK | Gated by `pkgcfg.Features.CSIBackupAPI` sourced from Supervisor capability `supports_CSI_Backup_API`. |
| E2E coverage ships with observable behavior | OK | E2E spec in `test/e2e/vmservice/vmservice/virtualmachine/vm_snapshot.go` validates disk UUID and CBT extraction on live cluster. |
| Testing standards | OK | Unit tests in `capabilities_test.go`, `vmprovider_vmsnapshot_test.go`, `vmsnapshot_test.go`, and conversion tests in `virtualmachinesnapshot_conversion_test.go`. |

## Project structure

```
api/v1alpha6/
  virtualmachinesnapshot_types.go                     (modified: add Disks and VirtualMachineSnapshotDiskStatus)
  zz_generated.deepcopy.go                            (regenerated)

api/v1alpha5/
  virtualmachinesnapshot_conversion.go                (modified: round-trip conversion helper for Disks)
  zz_generated.conversion.go                          (regenerated)

api/test/v1alpha5/
  virtualmachinesnapshot_conversion_test.go           (new: conversion unit tests)

config/crd/
  bases/vmoperator.vmware.com_virtualmachinesnapshots.yaml (regenerated)
  patches/webhook_in_virtualmachinesnapshots.yaml      (new: conversion webhook patch)
  patches/cainjection_in_virtualmachinesnapshots.yaml  (new: CA injection patch)

pkg/config/
  config.go                                           (modified: add CSIBackupAPI to FeatureStates)
  capabilities/capabilities.go                        (modified: add CapabilityKeyCSIBackupAPI)
  capabilities/capabilities_test.go                   (modified: unit test capability mapping)

pkg/providers/vsphere/
  vmprovider_vmsnapshot.go                            (modified: getDisksFromSnapshot and error handling)
  vmprovider_vmsnapshot_test.go                       (modified: tests for disk population)

pkg/util/kube/
  vmsnapshot.go                                       (modified: accept disks in PatchSnapshotSuccessStatus)
  vmsnapshot_test.go                                  (modified: unit test status patching)

test/e2e/vmservice/
  consts/consts.go                                    (modified: add CSIBackupAPICapabilityName)
  vmservice/virtualmachine/vm_snapshot.go             (modified: e2e test for CBT and snapshot disks)
```

## API / CRD strategy

- Add `Disks []VirtualMachineSnapshotDiskStatus` to `VirtualMachineSnapshotStatus` in `api/v1alpha6`:
  ```go
  type VirtualMachineSnapshotDiskStatus struct {
      ID                     string `json:"id"`
      ChangedBlockTrackingID string `json:"changedBlockTrackingID,omitempty"`
  }
  ```
- Annotated with `+listType=map` and `+listMapKey=id`.
- `v1alpha5` down-conversion drops `Disks` and serializes the hub object into `utilconversion` annotation. Up-conversion restores `Disks` from annotation when present.
- Configured CRD conversion webhook for `VirtualMachineSnapshot` across versions.

## Controller / provider impact

- `getDisksFromSnapshot(vmCtx, vcVM, snapRef)`:
  - Fetches `config.hardware.device` property of the `VirtualMachineSnapshot` MoRef.
  - Iterates devices, filters `*vimtypes.VirtualDisk`, and extracts `Uuid` and `ChangeId` from `VirtualDiskFlatVer2BackingInfo`, `VirtualDiskSeSparseBackingInfo`, `VirtualDiskSparseVer2BackingInfo`, or `VirtualDiskRawDiskMappingVer1BackingInfo`.
  - Appends to `[]VirtualMachineSnapshotDiskStatus`.
- In `ReconcileCurrentSnapshot`:
  - When `pkgcfg.FromContext(vmCtx).Features.CSIBackupAPI` is enabled, calls `getDisksFromSnapshot`.
  - If `getDisksFromSnapshot` fails, calls `markSnapshotFailed(vmCtx, k8sClient, snapshotToProcess, err)` to prevent snapshot from hanging in `CreationInProgressReason`.
  - Passes `disks` to `kubeutil.PatchSnapshotSuccessStatus`.

## Test strategy

- **Unit tests**:
  - `api/test/v1alpha5/virtualmachinesnapshot_conversion_test.go`: Verifies round-trip fidelity between `v1alpha5` and `v1alpha6`.
  - `pkg/config/capabilities/capabilities_test.go`: Verifies capability key `supports_CSI_Backup_API` activates `Features.CSIBackupAPI`.
  - `pkg/providers/vsphere/vmprovider_vmsnapshot_test.go`: Verifies snapshot reconcile loop with `CSIBackupAPI` enabled.
  - `pkg/util/kube/vmsnapshot_test.go`: Verifies `PatchSnapshotSuccessStatus` correctly sets disks in CR status.
- **E2E tests**:
  - `test/e2e/vmservice/vmservice/virtualmachine/vm_snapshot.go`: Gated by `skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, vmSvcClusterProxy, consts.CSIBackupAPICapabilityName)`. Creates VM with CBT enabled, triggers snapshot, and asserts that `status.disks` contains non-empty disk `id` and `changedBlockTrackingID`.

## Rollout / migration

- Gated by `supports_CSI_Backup_API` Supervisor capability.
- Default: disabled unless WCP signals capability activation.
- Non-breaking change: existing snapshots retain empty `status.disks`.
