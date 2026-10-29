# Tasks: Create Markers of Restored, Failed-Over and Imported VMs

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Epic**: TBD

## Phase 1 — Webhooks

- [x] T001 [P] [vmop-TBD] Compare the three markers by presence and guard `backup-version` on an update in `webhooks/virtualmachine/validation/virtualmachine_validator.go`, with tests in `webhooks/virtualmachine/validation/virtualmachine_validator_unit_test.go`
- [x] T002 [P] [vmop-TBD] Fill no `spec.imageName` from a CD-ROM entry on a create with a marker in `webhooks/virtualmachine/mutation/virtualmachine_mutator.go`, with tests in `webhooks/virtualmachine/mutation/virtualmachine_mutator_unit_test.go`

## Phase 2 — Provider and E2E

- [x] T003 [P] [vmop-TBD] Remove the markers in the VM update in `pkg/providers/vsphere/session/session_vm_update.go`, with tests in `pkg/providers/vsphere/session/session_vm_update_test.go`
- [x] T004 [P] [vmop-TBD] Leave the markers out of backups in `pkg/providers/vsphere/virtualmachine/backup.go`, with tests in `pkg/providers/vsphere/virtualmachine/backup_test.go`
- [x] T005 [vmop-TBD] Check the removal after a create and after a RegisterVM update in `test/e2e/vmservice/vmservice/backuprestore/veeam.go`

## Phase Final — Polish

- [x] T006 [vmop-TBD] Document the markers' lifetime in `api/v1alpha6/virtualmachine_types.go`
- [x] T007 [vmop-TBD] Add this spec under `.sdd/specs/012-restore-create-markers/` and its row in `.sdd/INDEX.md`
