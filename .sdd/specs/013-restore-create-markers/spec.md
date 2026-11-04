# Feature Specification: Create Markers of Restored, Failed-Over and Imported VMs

- **Feature branch**: `topic/zl018920/restore-create-markers`
  - **Fork**: `zhangheliu/vm-operator`
  - **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-10-28
- **Status**: Implemented
- **Epic**: TBD

---

## Background

The `vmoperator.vmware.com/restored-vm`, `vmoperator.vmware.com/failed-over-vm` and `vmoperator.vmware.com/imported-vm` annotations mark the write of a VirtualMachine by a restore, a failover or an import, so that VM Operator's webhooks relax their image and network checks for that request. The RegisterVM API sets `restored-vm` or `failed-over-vm` on the create of a VirtualMachine and on the update with which it restores or fails over an existing one; the ImportVM API and the restore-based import set their marker on the create. Before this change the markers stayed on the VirtualMachine for its lifetime and were copied into every later backup of it; the annotation guard compared them by value only, so an account that is not privileged could add one with an empty value; the create mutator filled an empty `spec.imageName` from a CD-ROM entry even on a create that carries a marker; and no rule kept an account that is not privileged from changing `vmoperator.vmware.com/backup-version`, which VM Operator compares with the backup version in the vSphere VM's ExtraConfig to detect a restore by a backup vendor.

## Goals

- **MUST** let only a privileged account add, change or remove `vmoperator.vmware.com/backup-version` on an update of a VirtualMachine.
- **MUST** compare the three markers by presence as well as by value when an account that is not privileged creates or updates a VirtualMachine.
- **MUST** leave `spec.imageName` empty, and the CD-ROM entries as given, on a create that carries one of the three markers.
- **MUST** remove the three markers from a VirtualMachine once the request that set them has been admitted and its vSphere VM exists, and keep every other annotation.
- **MUST** leave the three markers out of VM Operator's backup of a VirtualMachine, so that their removal writes no new backup.
- **MUST** state the lifetime of the three markers in the API documentation.

## Non-goals

- No new feature flag: the changes apply to the RegisterVM API, the ImportVM API and the restore-based import alike.
- No change to how the webhooks use a marker on the request that sets it, nor to which accounts are privileged.
- No guard on `backup-version` on a create, and no presence compare for the other annotations that only a privileged account may change.
- No removal from a VirtualMachine that VM Operator does not reconcile, such as one paused by annotation: the markers stay until its reconcile resumes.

## User stories / acceptance criteria

### DevOps user

- **Given** a VirtualMachine, **When** a DevOps user adds, changes or removes its `backup-version` annotation, **Then** the update is rejected with "modifying this annotation is not allowed for non-admin users".
- **Given** a DevOps user, **When** they create a VirtualMachine with a marker whose value is empty, add such a marker on an update, or remove a marker whose value is empty, **Then** the request is rejected with the same message.

### CSP admin

- **Given** an imageless create with a CD-ROM entry that carries a marker, **When** a privileged account submits it, **Then** the VirtualMachine is admitted with an empty `spec.imageName` and its CD-ROM entries unchanged.
- **Given** a VirtualMachine created, restored or failed over with a marker, **When** VM Operator next reconciles it with its vSphere VM present, **Then** `kubectl get vm -o yaml` shows no marker and every other annotation kept.
- **Given** a VirtualMachine that carries a marker, **When** VM Operator writes its backup, **Then** the VM resource in the backup does not carry the marker, and the removal of the marker from the VirtualMachine changes no backup version.

## Open questions

- None.
