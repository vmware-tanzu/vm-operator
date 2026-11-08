# Implementation Plan: Create Markers of Restored, Failed-Over and Imported VMs

- **Spec**: [`spec.md`](./spec.md)
- **Epic**: TBD
- **Date**: 2026-10-28

## Summary

Guard `backup-version` and the three markers in the validation webhook, skip the CD-ROM image name in the create mutator, remove the markers in the vSphere provider's VM update, and trim them from backups, as [`spec.md`](./spec.md) requires. No API field, CRD schema or feature flag changes.

## Technical context

- **Go version**: as `go.mod` (1.26.10)
- **API version(s) touched**: `v1alpha6`, documentation of three existing annotation constants only
- **Modules touched**: root module and `api/` (comments only)
- **New dependencies**: none

## Constitution check

| Rule | Status | Notes |
|------|--------|-------|
| API compatibility | OK | no schema change; `make generate-manifests` has nothing to regenerate |
| Thin controllers / provider boundary | OK | the removal and the trim live in `pkg/providers/vsphere/`; the controller's deferred patch persists the removal |
| VM update reconcile order | OK | no step is added or moved; the removal runs at the start of the session update, before its power-state switch |
| Feature gating | OK | the spec ships `Implemented` in this one PR, so no in-progress code needs a flag; see Rollout |
| E2E ships with observable behavior | OK | the Veeam suite checks the removal after a create and after an update |

## Changes

- **Guard** (`webhooks/virtualmachine/validation/virtualmachine_validator.go`): `validateAnnotation` compares `restored-vm`, `failed-over-vm` and `imported-vm` with a new `annotationChanged` (presence and value) on a create and an update, and on an update refuses any add, change or remove of `backup-version`, for an account that is not privileged.
- **CD-ROM image name** (`webhooks/virtualmachine/mutation/virtualmachine_mutator.go`): `SetImageNameFromCdrom` returns when the VM carries one of the three markers.
- **Removal** (`pkg/providers/vsphere/session/session_vm_update.go`): `UpdateVirtualMachine` calls `removeCreateMarkerAnnotations` before its power-state switch, so every branch (suspended, running task, paused by an admin, powered on or off) removes the markers.
- **Backup trim** (`pkg/providers/vsphere/virtualmachine/backup.go`): `trimBackupFields` deletes the three markers; `isVMBackupUpToDate` applies the same trim to the VM it compares with the backup, so their removal writes no new backup.
- **Documentation** (`api/v1alpha6/virtualmachine_types.go`): each constant states when it is set, including the RegisterVM update for `restored-vm` and `failed-over-vm`, and that VM Operator removes it and never backs it up.

## Test strategy

- Unit: tables for the guard in `virtualmachine_validator_unit_test.go` (each marker with an empty value added, removed and kept, by an SSO user and a service user; `backup-version` changed, removed, added and kept); `virtualmachine_mutator_unit_test.go` (`SetImageNameFromCdrom` with each marker); `session_vm_update_test.go` (vcsim: a powered-off VM and a VM paused by an admin lose the markers and keep other annotations); `backup_test.go` (both backup variants: the backup YAML lacks the markers, a second backup after their removal is a no-op).
- E2E: `test/e2e/vmservice/vmservice/backuprestore/veeam.go`. The restore of a lost VM as a new VM checks that `restored-vm` is gone once VM Operator manages the restored VM; the in-place restore checks the same after the RegisterVM update. Both specs keep the `experimental` label.

## Rollout / migration

- Feature flag: none. The markers are written by the RegisterVM API, the ImportVM API and the restore-based import, and each change fixes all three writers alike, so a flag would only keep the defects on its off path.
- Upgrade: a VirtualMachine that carries a marker from an earlier release loses it at its next reconcile, and its next backup leaves it out.
- Behavior change: once `failed-over-vm` is removed, an update of that VM's network interfaces is validated as for any other VM.
