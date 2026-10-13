# Feature Specification: Boot Disk Capacity with Fast Deploy

- **Created**: 2026-10-06
- **Status**: Draft
- **Epic**: None
- **Bug**: vmop-4249 (P0)

---

## Summary

`spec.advanced.bootDiskCapacity` extends a VM's boot disk beyond the image's, but it does not work with Fast Deploy (vmop-4249):

- The default linked-clone boot disk has a parent, vSphere rejects the extend ("Disks with parents cannot be expanded"), and the VM never powers on.
- With `fast-deploy: direct` the VM boots, but the extend is sent after the disk is registered with CNS, and the CSI check `validatePVCCapacityMatchesBackendFCD` then compares the PVC request (image size) with the capacity CNS stored. The PVC and PV stay at the image size, or, if a CSI retry sees the larger disk, the PVC never binds and the VM never powers on.

This spec fixes the bug with the existing mechanisms. The boot disk is promoted (it is no longer a linked clone) before first boot, and then only the boot disk is extended. When the other disks are promoted depends on `spec.promoteDisksMode`, as before. The API does not change.

## Goals

- A Fast Deploy VM with a larger `bootDiskCapacity` powers on with the requested boot disk size, and its PVC and PV match that size.
- Placement sees the requested boot disk size, so the datastore is chosen for it.
- The boot disk is extended before the guest first boots.
- In `Online` mode, promote only the boot disk before first boot. The other disks are promoted later by the normal flow.
- Keep `bootDiskCapacity` mutable, but only to increase it. A later increase works like the first one. A decrease, or removing the field once it is set, is rejected.
- Keep the spec value correct when the boot disk PVC is resized from outside.

## Non-goals

- Changing the API schema (no new or renamed field, no immutability rule, no conversion change). The decrease check is a webhook rule only.
- Changing the Fast Deploy mode. The mode is not forced to `direct`.
- Changing the ISO behavior: the field stays ignored for VMs with CD-ROMs, in every code path.
- Shrinking a boot disk. A smaller value is an error.

## Approach

The VM is created at the image size, as today. The existing update path then promotes and extends the boot disk.

### Placement

The requested size is set only in the placement ConfigSpec. The create ConfigSpec is not changed: it keeps the image size, because the disk files that Fast Deploy attaches are still at the image size, and the boot disk is promoted and extended after create as above.

Today `vmCreateGenConfigSpecChangeBootDiskSize` (`pkg/providers/vsphere/vmprovider_vm.go`) is a stub that runs while the create ConfigSpec is built. Its TODO says the disk device key is unknown at that point, so placement was punted. The placement ConfigSpec is built from the create ConfigSpec (`CreateConfigSpecForPlacement`), so the size must be applied there, on the placement copy, and not in the create ConfigSpec. The stub is removed or changed to do nothing for the create ConfigSpec.

[NEEDS CLARIFICATION: whether this applies to all deployments or only Fast Deploy.]

### Shared boot disk helpers

The boot disk rules were copied in the disk promotion reconciler, the session resize, the Fast Deploy create check and `BootDiskPVCName`. They now live in `pkg/util` and `pkg/util/volumes`:

- `GetBootDisk`: the first disk is the boot disk.
- `GetRequestedBootDiskCapacity`: the requested size in bytes, or zero when it is not set or the VM has CD-ROMs.
- `GetDiskCapacityInBytes`: the disk size, with a fallback to `CapacityInKB`.
- `IsLinkedCloneDisk`: the disk has a parent that is not the result of a snapshot. A snapshot disk is not a linked clone that needs promotion.
- `GetBootDiskAwaitingPromotion`: the boot disk, if the requested size is larger than the disk, the disk is a linked clone, and the disk is not an FCD.

### Promote the boot disk

The disk promotion reconciler (`pkg/vmconfig/diskpromo`) finds the boot disk with `GetBootDiskAwaitingPromotion`.

The behavior depends on `spec.promoteDisksMode` (default `Online`):

- **`Online`**: normally the reconciler waits for the VM to be powered on, then promotes all linked disks. Exception: when the VM is powered off and does not have the `FirstBootDoneAnnotation`, promote only the boot disk now, so the guest boots with the requested size. A VM that has already booted gets no exception: the boot disk is promoted with the other disks after power-on, and the extend waits for it. A VM with a snapshot stays pending, because online promotion does not support snapshots.
- **`Offline`**: all linked disks are promoted while the VM is powered off. A new VM is powered off, so the boot disk is promoted before first boot without an exception. All disks are promoted, not only the boot disk.
- **`Disabled`**: nothing is promoted. If the boot disk needs promotion, the reconciler sets `VirtualMachineDiskPromotionSynced` to false with reason `DiskPromotionDisabled`. The provider also emits a warning event with the same reason, because the condition is easy to miss. Without a pending boot disk extend the condition is removed, as before.
- **Create**: for a linked clone Fast Deploy create, if the requested size is larger than the image's boot disk and `spec.promoteDisksMode` is `Disabled`, the create fails with a `NoRequeueError`. The size can never be applied.

### Extend the boot disk

`updateVirtualDiskDeviceChanges` (`pkg/providers/vsphere/session`) builds the extend:

1. A requested size smaller than the disk is an error ("cannot shrink boot disk"). An equal size is a no-op.
2. If the boot disk is a linked clone, skip the extend until the disk is promoted. This is logged at V(4).
3. If `AllDisksArePVCs` is on and the boot disk has a PVC, skip the extend. The PVC is the source of truth (see below).
4. Otherwise, edit the boot disk to the requested size in the ConfigSpec.

### Boot disk with a PVC

When the boot disk has a PVC, the register unmanaged volumes reconciler (`pkg/vmconfig/volumes/unmanaged/register`):

- Raises the PVC request to `bootDiskCapacity` if the PVC requests less (`ensureBootDiskPVCCapacity`). It never shrinks the PVC. The CSI driver then expands the disk, which needs a storage class that allows volume expansion.
- Raises `spec.advanced.bootDiskCapacity` to the PVC request if the PVC requests more, for example after someone resizes the PVC from outside. The value is always `max(spec, PVC)`. Without this, the disk in vCenter is larger than the spec value and the shrink check in the update path fails on every reconcile. Writing the spec from a controller is not ideal, but the VM controller already does it in the schema upgrade step.
- Waits to register a disk that has a pending extend in the ConfigSpec (`hasPendingDiskExtend`). It sets the condition to false with reason `PendingConfigUpdates` and registers the disk on a later reconcile. A disk is registered with the PV at its current size, and a PVC that requests more than the PV never binds, so the PVC, PV and CNS must see the final size.

### Reject a decrease or removal

Because the controller can raise the value, the field means "at least this size". Removing the field sets the minimum to zero, which is a decrease. Once the field is set, it can be raised but not lowered and not removed. The VM validating webhook rejects an update if the old value is set and the new value is lower or not set. The check compares only the old and new spec values, so it needs no PVC lookup. A webhook is used and not CEL: it handles removal, needs no type cast for the int-or-string schema, and does not need a rule on each of the six API versions. This also stops a fight with GitOps tools: if a manifest sets a value lower than the live one, the apply fails with a clear error and the controller does not write the value again.

If the PVC is resized from outside and the user sets a value between the old spec value and the new PVC size before the copy-back runs, the webhook allows it and the copy-back then sets the PVC size.

This is a new rejection. Before, a decrease was accepted at admission and failed at reconcile only if the value was lower than the disk size, and the field could be removed. After this change, a user cannot clear the field once it is set, and the only way back is to recreate the VM. It needs a release note.

### Order

For a new Fast Deploy VM with a larger boot disk:

1. Create the VM at the image size. The boot disk is a linked clone.
2. Promote the boot disk while the VM is powered off and has not booted. In `Online` mode only the boot disk, in `Offline` mode all linked disks.
3. Extend the boot disk with the ConfigSpec edit, with the VM powered off.
4. Register the disk, so the PVC, PV and CNS are created at the final size.
5. Power on.

### Existing VM: promotion was disabled, then the value is raised

The VM was deployed with `spec.promoteDisksMode: Disabled` and is powered on, so its boot disk is still a linked clone. The user raises `bootDiskCapacity` and changes the mode.

**Mode changed to `Offline`:**

1. While the VM is powered on, nothing happens. Promotion waits with reason `DiskPromotionPending` ("Pending VM powered off"), and the extend is skipped because the boot disk is a linked clone. The `DiskPromotionDisabled` reason and warning event go away.
2. The user sets `spec.powerState: PoweredOff`. A power-off only in vSphere is not enough, because the VM operator powers the VM on again.
3. All linked disks are promoted.
4. The boot disk is extended while the VM is powered off.
5. The disk is registered, so the PVC and PV have the new size.
6. The user sets `spec.powerState: PoweredOn`. The guest must grow its partition and file system.

**Mode changed to `Online`:**

1. The VM stays powered on. The boot-disk exception does not apply, so the normal flow promotes all linked disks, after any guest customization is done.
2. The boot disk is extended while the VM is powered on (hot extend).
3. The disk is registered, so the PVC and PV have the new size.
4. The guest must grow its partition and file system.

**Cases that behave as before this change:**

- In `Online` mode, a VM with a snapshot stays pending ("Cannot online promote disks when VM has a snapshot"). The extend waits until the snapshot is removed.
- vSphere applies the same hot extend rules to every disk, and has no concept of a boot disk. If it refuses the hot extend, the reconfigure fails and is retried on each reconcile, the same as for any other disk.

The only new behavior is that the extend waits for promotion, instead of failing with "Disks with parents cannot be expanded".

## Behavior by case

| Case | Result |
|---|---|
| Fast Deploy linked, promotion enabled | Boot disk is promoted before first boot, then extended. |
| Fast Deploy direct | Disks are full copies. The boot disk is extended before it is registered. |
| Fast Deploy linked, promotion disabled | Create fails with a `NoRequeueError`. An existing VM keeps its size, with the `DiskPromotionDisabled` condition and a warning event. |
| Boot disk has a PVC, spec is larger | The PVC request is raised. The CSI driver extends the disk. |
| Boot disk has a PVC, PVC is larger (resized from outside) | The spec value is raised to the PVC request. |
| Update sets a value lower than the old value, or removes the field | The webhook rejects it. |
| ISO VM with CD-ROMs | `bootDiskCapacity` is ignored. |
| Not Fast Deploy | The disk is extended after create, as before. The PVC copy-back and the webhook rule also apply. |
| Requested size smaller than the disk | Error, no change. |

## Known limits

- The boot disk is a full copy after promotion, so it uses more datastore space than a linked clone.
- In `Online` mode, a VM that has already booted and has a snapshot cannot promote the boot disk, so the extend waits until the snapshot is removed.
- A failed extend leaves the VM at the image size, and the update path retries it on the next reconcile.

## Alternate approaches

These approaches were considered and not chosen.

### Extend before attach

`fastDeploy` copies the cached disks into the VM directory, calls `ExtendVirtualDisk` on the boot disk copy, then `CreateVM` attaches the existing files. This forces `direct` mode when the size is larger, sets the size in the ConfigSpec so that placement sees it, and makes the VM never exist at the image size. `ExtendVirtualDisk_Task` is deprecated, but the FCD extend APIs do not apply to a classic disk that is not yet attached. It was not chosen because it forces direct mode (all disks become full copies), only covers the Fast Deploy path, and needed a field rename to `initialBootDiskCapacity` that is immutable after create. That rename breaks clients on v1alpha1 to v1alpha5 that change the field on day 2.

### Remove day 2 resize from the spec

Remove the update-path resize and the `diskpromo` boot disk case, and size the disk only through its PVC after create. This needs the immutable field above.

## Acceptance criteria

- A VM deployed with default Fast Deploy settings (no annotation) and a larger `bootDiskCapacity` powers on. Its disk, boot-disk PVC request and PV all equal the requested size.
- The placement request has the boot disk at the requested size, and the create ConfigSpec keeps the image size.
- In `Online` mode, only the boot disk is promoted before first boot. The other disks are promoted after power-on by the normal flow.
- In `Offline` mode, all linked disks are promoted before first boot.
- The boot disk is extended before the VM first boots.
- A disk with a pending extend is not registered until the extend is applied.
- An existing powered-on VM with a linked-clone boot disk, a raised value and the mode changed from `Disabled` to `Offline` keeps its size while powered on, reports `DiskPromotionPending`, and after `spec.powerState: PoweredOff` is promoted, extended and registered at the new size.
- The same VM with the mode changed to `Online` is promoted, extended and registered at the new size without a power-off.
- In `Online` mode, a VM with a snapshot does not promote or extend the boot disk until the snapshot is removed.
- A refused hot extend is retried and does not leave the VM in a broken state.
- With `spec.promoteDisksMode: Disabled`, a Fast Deploy create with a larger boot disk fails, and an existing VM reports `DiskPromotionDisabled` and emits a warning event.
- A boot disk that is part of a snapshot is not promoted.
- `bootDiskCapacity` is ignored for ISO VMs with CD-ROMs in the promotion, resize, register and create paths.
- A requested size smaller than the boot disk is rejected with a clear error.
- If the boot disk PVC is resized above `bootDiskCapacity` from outside, the spec value is raised to the PVC request and the update path does not fail with "cannot shrink boot disk".
- The webhook rejects an update that lowers or removes `bootDiskCapacity` once it is set, and the release note says so.
- E2E covers create with a larger boot disk against a real vCenter (vcsim only rejects shrinking), promotion of only the boot disk in `Online` mode, the PVC resize from outside, the rejected decrease and removal, and the ISO case.
- Unit tests cover the code changes.
- `docs/concepts/workloads/vm.md` describes the promotion behavior per mode, the PVC behavior including the copy-back, and that the field cannot be lowered or removed once set.
