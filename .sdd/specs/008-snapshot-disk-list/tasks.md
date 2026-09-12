# Tasks: Expose Disk List in VirtualMachineSnapshot

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Epic**: vmop-52730

## Phase 1 — Setup & API Types

- [x] T001 [vmop-52730] Define `VirtualMachineSnapshotDiskStatus` struct and `Disks` slice on `VirtualMachineSnapshotStatus` in `api/v1alpha6/virtualmachinesnapshot_types.go`.
- [x] T002 [vmop-52730] Run `make generate-go` to regenerate `api/v1alpha6/zz_generated.deepcopy.go`.
- [x] T003 [vmop-52730] Run `make generate-manifests` to update `config/crd/bases/vmoperator.vmware.com_virtualmachinesnapshots.yaml`.

## Phase 2 — Foundational (Capabilities & Feature Flags)

- [x] T004 [vmop-52730] Add `CSIBackupAPI bool` to `FeatureStates` in `pkg/config/config.go`.
- [x] T005 [vmop-52730] Add `CapabilityKeyCSIBackupAPI = "supports_CSI_Backup_API"` to `pkg/config/capabilities/capabilities.go` and map to `Features.CSIBackupAPI`.
- [x] T006 [P] [vmop-52730] Add unit test cases for `CapabilityKeyCSIBackupAPI` in `pkg/config/capabilities/capabilities_test.go`.

## Phase 3 — API Conversion & Webhook

- [x] T007 [vmop-52730] Implement `ConvertTo` and `ConvertFrom` methods with `utilconversion.MarshalData`/`UnmarshalData` in `api/v1alpha5/virtualmachinesnapshot_conversion.go`.
- [x] T008 [vmop-52730] Run `make generate-go-conversions` to update `api/v1alpha5/zz_generated.conversion.go`.
- [x] T009 [vmop-52730] Configure CRD conversion webhook patches in `config/crd/patches/webhook_in_virtualmachinesnapshots.yaml`, `config/crd/patches/cainjection_in_virtualmachinesnapshots.yaml`, and `config/crd/kustomization.yaml`.
- [x] T010 [P] [vmop-52730] Add conversion unit test suite in `api/test/v1alpha5/virtualmachinesnapshot_conversion_test.go`.

## Phase 4 — Provider Implementation & Status Patching

- [x] T011 [vmop-52730] Implement `getDisksFromSnapshot` helper in `pkg/providers/vsphere/vmprovider_vmsnapshot.go` to retrieve disk UUID and CBT Change ID from snapshot hardware device properties.
- [x] T012 [vmop-52730] Update `kubeutil.PatchSnapshotSuccessStatus` in `pkg/util/kube/vmsnapshot.go` to accept and patch `disks`.
- [x] T013 [vmop-52730] Update `ReconcileCurrentSnapshot` in `pkg/providers/vsphere/vmprovider_vmsnapshot.go` to invoke `getDisksFromSnapshot` when `Features.CSIBackupAPI` is active, and call `markSnapshotFailed` on error to avoid leaving snapshot in permanent in-progress state.
- [x] T014 [P] [vmop-52730] Unit tests for `PatchSnapshotSuccessStatus` in `pkg/util/kube/vmsnapshot_test.go`.
- [x] T015 [P] [vmop-52730] Unit tests for `ReconcileCurrentSnapshot` with CSI backup feature flag in `pkg/providers/vsphere/vmprovider_vmsnapshot_test.go`.

## Phase 5 — E2E Coverage

- [x] T016 [vmop-52730] Add `CSIBackupAPICapabilityName` constant in `test/e2e/vmservice/consts/consts.go`.
- [x] T017 [vmop-52730] Implement E2E test verifying CBT enablement and snapshot `status.disks` population in `test/e2e/vmservice/vmservice/virtualmachine/vm_snapshot.go`.

## Phase Final — Polish & SDD

- [x] T018 Document spec, plan, tasks, model, and research under `.sdd/specs/008-snapshot-disk-list/`.
- [x] T019 Update `.sdd/INDEX.md` with the new spec entry.
