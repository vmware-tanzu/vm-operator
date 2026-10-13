# Tasks: Boot Disk Capacity with Fast Deploy

- **Spec**: [spec.md](./spec.md)
- **Bug**: vmop-4249

Task format: `[T###] [P?] [vmop-NNN?] Description (path)`. `[x]` means done.

## Phase 1 — Shared helpers

- [x] T001 [vmop-4249] Add `GetBootDisk`, `GetRequestedBootDiskCapacity`, `GetDiskCapacityInBytes`, `IsLinkedCloneDisk`, `GetBootDiskAwaitingPromotion` with tests (`pkg/util/devices.go`, `pkg/util/devices_test.go`)
- [x] T002 [P] [vmop-4249] Add `VolumeInfo.BootDisk` and `BootDiskPVCName` (`pkg/util/volumes/volumes_util.go`, `pkg/util/volumes/volumes_util_test.go`)

## Phase 2 — Promote and extend

- [x] T003 [vmop-4249] Promote the boot disk before first boot in `Online` mode, all linked disks in `Offline` mode; set `DiskPromotionDisabled` in `Disabled` mode (`pkg/vmconfig/diskpromo/diskpromo_reconciler.go`)
- [x] T004 [vmop-4249] Emit a warning event for `DiskPromotionDisabled` (`pkg/providers/vsphere/vmprovider_vm.go`)
- [x] T005 [vmop-4249] Fail a linked clone Fast Deploy create with `NoRequeueError` when the boot disk is extended and promotion is disabled (`pkg/providers/vsphere/vmlifecycle/create_fastdeploy.go`)
- [x] T006 [vmop-4249] Extend the boot disk: reject shrink, skip while linked clone, skip when the boot disk has a PVC (`pkg/providers/vsphere/session/session_vm.go`)
- [x] T007 [vmop-4249] Unit tests for T003 to T006, including boot disks in a snapshot and VMs with a snapshot (`pkg/vmconfig/diskpromo/diskpromo_reconciler_test.go`, `pkg/providers/vsphere/session/session_vm_update_test.go`, `pkg/providers/vsphere/vmprovider_vm_fast_deploy_test.go`, `pkg/util/devices_test.go`)

## Phase 3 — Boot disk with a PVC

- [x] T008 [vmop-4249] Raise the PVC request to `bootDiskCapacity`; wait to register a disk with a pending extend (`pkg/vmconfig/volumes/unmanaged/register/unmanagedvolumes_register.go`)
- [x] T009 [vmop-4249] Raise `spec.advanced.bootDiskCapacity` to the PVC request when the PVC is larger (`pkg/vmconfig/volumes/unmanaged/register/unmanagedvolumes_register.go`, `unmanagedvolumes_register_test.go`)

## Phase 4 — Placement

- [x] T010 [vmop-4249] Size the boot disk in the placement ConfigSpec only, and remove the `vmCreateGenConfigSpecChangeBootDiskSize` stub (`pkg/providers/vsphere/virtualmachine/configspec.go`, `configspec_test.go`, `pkg/providers/vsphere/vmprovider_vm.go`). The size is applied for all deployments, not only Fast Deploy. Open: confirm this resolves the `[NEEDS CLARIFICATION]` in the spec.

## Phase 5 — Webhook

- [x] T011 [vmop-4249] Reject an update that lowers or removes `bootDiskCapacity` once set, with tests (`webhooks/virtualmachine/validation/virtualmachine_validator.go`, `virtualmachine_validator_unit_test.go`)

## Phase 6 — E2E

- [x] T012 [vmop-4249] E2E: create with a larger boot disk, promote and extend before first boot, Disabled mode, booted VM, no register annotation (`test/e2e/vmservice/vmservice/virtualmachine/vm_hardware.go`, `bootdisk_capacity.go`)
- [x] T013 [P] [vmop-4249] E2E: rejected decrease and removal (`test/e2e/vmservice/vmservice/virtualmachine/vm_hardware.go`)
- [ ] T014 [P] [vmop-4249] E2E: PVC resized from outside raises the spec value (`test/e2e/vmservice/vmservice/virtualmachine/vm_hardware.go`)
- [ ] T015 [P] [vmop-4249] E2E: `Online` mode promotes only the boot disk before first boot (`test/e2e/vmservice/vmservice/virtualmachine/vm_hardware.go`)
- [ ] T016 [P] [vmop-4249] E2E: ISO VM with CD-ROMs ignores the field (`test/e2e/vmservice/vmservice/virtualmachine/vm_hardware.go`)

## Phase Final — Polish

- [x] T017 [vmop-4249] Document promotion per mode, PVC behavior with copy-back, and no lower or remove (`docs/concepts/workloads/vm.md`)
- [ ] T018 [vmop-4249] Add the release note for the new rejection of a lower or removed value in the PR description
- [ ] T019 Create `plan.md` and set the spec Epic and Status
