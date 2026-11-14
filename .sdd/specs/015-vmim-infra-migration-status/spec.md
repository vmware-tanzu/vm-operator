# Feature Specification: VirtualMachineInfraMigration Status on VirtualMachine

- **Feature branch**: `topic/brito-rafa/vmim-status-on-vm`
  - **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-10-30
- **Status**: Draft
- **Epic**: vmop-3432

---

## Summary

The mobility operator migrates a VirtualMachine to target infrastructure (a resource pool and/or storage policies for the VM home and its volumes) through a `VirtualMachineInfraMigration` resource. The migration resource has the same namespace and name as the VirtualMachine it migrates, and reports its overall outcome in a `Ready` condition on its own status.

A DevOps user looking at a VirtualMachine has no way to tell that such a migration has converged without also finding and reading the migration resource. This feature surfaces that outcome on the VirtualMachine itself: VM Operator mirrors every condition of the migration that describes its current generation onto the VirtualMachine's status as a condition whose type is `VirtualMachineInfraMigration` followed by the migration condition's type (for example the migration's `Ready` condition becomes `VirtualMachineInfraMigrationReady`), and, only when the migration is ready, records the VolumeAttributesClass the VirtualMachine is now configured to in a new `status.volumeAttributesClassName` field. The migration reports the storage policy of the VirtualMachine's home directory rather than a VolumeAttributesClass name, so the field is the VolumeAttributesClass on the cluster that refers to that policy, and it must also be the one named in the VirtualMachine's `spec.volumeAttributesClassName`.

The behavior is inert unless the `StoragePolicyMutability` capability is enabled for the Supervisor.

The `VirtualMachineInfraMigration` `v1alpha4` CRD is guaranteed to be installed when the `StoragePolicyMutability` capability is enabled.

---

## Goals

- When the capability is enabled, VM Operator MUST watch `VirtualMachineInfraMigration` resources.
- When a VirtualMachine with the same namespace and name as a `VirtualMachineInfraMigration` exists and is not being deleted, VM Operator MUST mirror every condition of the migration that describes the migration's current generation (the condition's `observedGeneration` has caught up to the migration's `metadata.generation`) onto that VirtualMachine's status, whatever the condition's status (`True`, `False`, or `Unknown`). The VirtualMachine condition's type is `VirtualMachineInfraMigration` followed by the migration condition's type, so the `Ready`, `Validated`, `StorageInSync`, and `ComputeInSync` conditions become `VirtualMachineInfraMigrationReady`, `VirtualMachineInfraMigrationValidated`, `VirtualMachineInfraMigrationStorageInSync`, and `VirtualMachineInfraMigrationComputeInSync`.
- A mirrored condition MUST be removed from the VirtualMachine when the migration stops reporting a condition of that type, or when the migration's condition of that type no longer describes the migration's current generation, so that the VirtualMachine does not keep reporting an outcome of the migration's previous spec.
- Only when a `VirtualMachineInfraMigration` has a `Ready` condition with status `True` that describes its current generation (the `Ready` condition's `observedGeneration` has caught up to the migration's `metadata.generation`), and a VirtualMachine with the same namespace and name exists and is not being deleted, VM Operator MUST set the VirtualMachine's `status.volumeAttributesClassName` to the name of the VolumeAttributesClass whose storage policy ID (`parameters["storagePolicyID"]`) is the migration's `spec.targetStorage.homeStoragePolicyID`, provided that VolumeAttributesClass is also the one in the VirtualMachine's `spec.volumeAttributesClassName`. When several VolumeAttributesClasses refer to the same policy, the one in the VirtualMachine's spec decides.
- VM Operator MUST NOT write `status.volumeAttributesClassName` when the migration has no `spec.targetStorage.homeStoragePolicyID`, when no VolumeAttributesClass refers to that policy, or when the VirtualMachine's `spec.volumeAttributesClassName` is unset or is not one of the VolumeAttributesClasses that refer to it. In the last two cases VM Operator MUST emit a `VolumeAttributesClassMismatch` warning event on the VirtualMachine, and MUST still mirror the conditions. When the migration is not ready, VM Operator MUST NOT emit that event.
- VM Operator does not pause the reconciliation of a VirtualMachine while a `VirtualMachineInfraMigration` for it exists and is not ready (for example its `Ready` condition is `False`), and does not add a validation that rejects a change to the VirtualMachine's `spec.volumeAttributesClassName` while the migration is running. Neither is needed: VM Operator already skips reconciling a VirtualMachine's spec while a task is running on it in vSphere (including DRS activity), which is enough to avoid concurrent operations on the VirtualMachine.
- A mirrored condition's status, reason, and message MUST be copied from the migration's condition (from VMIM .status.conditions to VM .status.conditions) so that a user can read the outcome from the VirtualMachine alone. The mirrored message MUST be followed by a reference to the migration, in the form `<message> @ VirtualMachineInfraMigration/<name>`, so that a user can tell which resource the condition came from. A migration condition without a message is mirrored with only the reference as its message. A migration condition without a reason is mirrored with the status as its reason, because a condition's reason must not be empty. The migration condition's `observedGeneration` MUST NOT be copied, since it refers to the migration's generation rather than the VirtualMachine's.
- Mirroring MUST NOT overwrite or drop any condition on the VirtualMachine that is not a mirrored condition, including conditions written concurrently by the VirtualMachine controller.
- Re-reconciling a migration whose conditions are unchanged MUST NOT change the VirtualMachine (in particular, a mirrored condition's last transition time MUST NOT move).
- If the VirtualMachine is created after the migration's conditions were set, the conditions MUST still be mirrored once the VirtualMachine exists.
- VM Operator does not act when the `VirtualMachineInfraMigration` is deleted: the mirrored conditions and `status.volumeAttributesClassName` are left on the VirtualMachine as they were. It MUST NOT clear `status.volumeAttributesClassName` in any case, because that field describes what the VirtualMachine is configured to, not the migration.
- When the capability is disabled, VM Operator MUST NOT start the controller and MUST NOT require the `VirtualMachineInfraMigration` API to be present.

## Non-goals

- Creating, updating, or deleting `VirtualMachineInfraMigration` resources. The mobility operator owns them.
- Removing the mirrored conditions when the `VirtualMachineInfraMigration` is deleted.
- Updating the VirtualMachine's storage policy usage (quota) after a migration. The mobility operator's design lists it as a separate VM Operator step.
- Performing any migration or calling vSphere. This controller only reads the migration resource and writes VirtualMachine status.
- Surfacing the migration's other status (for example the observed resource pool) on the VirtualMachine.
- Updating `status.volumes[].conditions[]`. The migration has no per-volume status, so there is nothing to mirror.
- Patching the VirtualMachine's spec, including `spec.volumeAttributesClassName`.
- Any VirtualMachine API surface beyond the mirrored conditions and `status.volumeAttributesClassName`.
- E2E coverage. It is scoped as a follow-up (see `tasks.md`), because it needs a Supervisor with the mobility operator installed.

---

## User stories and acceptance criteria

### DevOps user: seeing that a migration has converged

As a DevOps user, I want the VirtualMachine to tell me the state of its infrastructure migration, so that I do not have to look up a second resource.

1. Given a VirtualMachine `ns/vm` and a `VirtualMachineInfraMigration` `ns/vm` whose `Ready` condition is `True` for the migration's current generation, `kubectl get vm vm -o jsonpath='{.status.conditions[?(@.type=="VirtualMachineInfraMigrationReady")].status}'` returns `True`. If the `Ready` condition is `False` or `Unknown`, the same command returns `False` or `Unknown`.
2. Every other condition of the migration (for example `Validated`) is on the VirtualMachine as `VirtualMachineInfraMigration<Type>` (for example `VirtualMachineInfraMigrationValidated`), with the same status and reason as the migration's condition, and its message followed by `@ VirtualMachineInfraMigration/<name>`. The `observedGeneration` is not copied.
3. Given a migration whose `Ready` condition is `False`, `Unknown`, absent, or whose `observedGeneration` is less than the migration's `metadata.generation`, `status.volumeAttributesClassName` is not set by this feature and no `VolumeAttributesClassMismatch` event is emitted. The other conditions that describe the migration's current generation are still mirrored.
4. Given a migration for a VirtualMachine that does not exist yet, creating the VirtualMachine results in the conditions being mirrored without any further change to the migration.
5. All other conditions on the VirtualMachine are unchanged by the feature. When the migration stops reporting a condition, its mirrored condition is removed from the VirtualMachine.
6. Given a ready migration whose `spec.targetStorage.homeStoragePolicyID` is the storage policy of VolumeAttributesClass `vac-a`, and a VirtualMachine whose `spec.volumeAttributesClassName` is `vac-a`, `kubectl get vm vm -o jsonpath='{.status.volumeAttributesClassName}'` returns `vac-a`.
7. Given a ready migration with a `homeStoragePolicyID`, but a VirtualMachine whose `spec.volumeAttributesClassName` is unset or refers to a different storage policy, or no VolumeAttributesClass for the policy, `status.volumeAttributesClassName` is not changed, the `VirtualMachineInfraMigrationReady` condition is still set, and `kubectl describe vm vm` shows a `VolumeAttributesClassMismatch` warning event.
8. Given a ready migration with no `homeStoragePolicyID`, `status.volumeAttributesClassName` is not changed and no warning is emitted.
9. Given the migration is then deleted, the VirtualMachine's mirrored conditions and `status.volumeAttributesClassName` keep their values.
10. A mirrored condition is copied only when its `observedGeneration` has caught up to the migration's `metadata.generation`. When the migration's spec changes and its generation advances, the VirtualMachine's mirrored conditions that describe the previous spec are removed until the migration reports them for the new generation. This avoids a stale condition, such as the VirtualMachine briefly reporting `True` for a condition that describes the migration's previous spec.

### CSP admin: rollout safety

1. With the capability disabled, VM Operator starts and runs on a Supervisor that has no `VirtualMachineInfraMigration` CRD installed.

---
