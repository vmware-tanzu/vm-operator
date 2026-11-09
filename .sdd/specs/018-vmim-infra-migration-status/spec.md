# Feature Specification: VirtualMachineInfraMigration Status on VirtualMachine

- **Feature branch**: `topic/brito-rafa/vmim-status-on-vm`
  - **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-10-30
- **Status**: Draft
- **Epic**: vmop-3432

---

## Summary

The mobility operator migrates a VirtualMachine to target infrastructure (a resource pool and/or storage policies for the VM home and its volumes) through a `VirtualMachineInfraMigration` resource. The migration resource has the same namespace and name as the VirtualMachine it migrates, and reports its overall outcome in a `Ready` condition on its own status.

A DevOps user looking at a VirtualMachine has no way to tell that such a migration has converged without also finding and reading the migration resource. This feature surfaces that outcome on the VirtualMachine itself: VM Operator mirrors the migration's `Ready` condition, when it describes the migration's current generation, onto the VirtualMachine's status as a `VirtualMachineInfraMigrationReady` condition, and, only when the migration is ready, records the VolumeAttributesClass the VirtualMachine is now configured to in a new `status.volumeAttributesClassName` field. The migration reports the storage policy of the VirtualMachine's home directory rather than a VolumeAttributesClass name, so the field is the VolumeAttributesClass on the cluster that refers to that policy, and it must also be the one named in the VirtualMachine's `spec.volumeAttributesClassName`.

A change to a VirtualMachine's `spec.volumeAttributesClassName` is turned into a `VirtualMachineInfraMigration` by the mobility operator, which watches the VirtualMachine objects, and the migration starts the storage migration right away. The migration could be short-lived and might be eventually deleted: the `VirtualMachineInfraMigrationReady` condition describes it while it exists and is removed when it is gone, while `status.volumeAttributesClassName`, which only VM Operator writes, is the durable record and is kept.

```mermaid
sequenceDiagram
    autonumber
    actor Dev as DevOps user
    participant VMOp as VM Operator
    participant VMIM as VMIM controller

    Dev->>VMOp: edit spec.volumeAttributesClassName on the VM
    VMOp-->>VMIM: VM change (watched)
    VMIM->>VMIM: create the VMIM and start the storage migration
    VMIM-->>VMOp: Ready condition (watched)
    VMOp->>VMOp: mirror it as VirtualMachineInfraMigrationReady
    VMOp->>VMOp: Ready=True, so record status.volumeAttributesClassName
    Note over VMOp,VMIM: the VMIM is eventually deleted
    VMOp->>VMOp: remove VirtualMachineInfraMigrationReady, keep the field status.volumeAttributesClassName
```

The behavior is inert unless the `StoragePolicyMutability` capability is enabled for the Supervisor.

The `VirtualMachineInfraMigration` `v1alpha4` CRD is guaranteed to be installed when the `StoragePolicyMutability` capability is enabled.

---

## Goals

- When the capability is enabled, VM Operator MUST watch `VirtualMachineInfraMigration` resources.
- When a VirtualMachine with the same namespace and name as a `VirtualMachineInfraMigration` exists and is not being deleted, VM Operator MUST mirror the migration's `Ready` condition onto that VirtualMachine's status as the `VirtualMachineInfraMigrationReady` condition, whatever the condition's status (`True`, `False`, or `Unknown`), provided it describes the migration's current generation (the condition's `observedGeneration` has caught up to the migration's `metadata.generation`). The migration's other conditions (`Validated`, `StorageInSync`, `ComputeInSync`, and any the mobility operator adds) are not mirrored, so the VirtualMachine API does not depend on the mobility operator's condition vocabulary.
- The `VirtualMachineInfraMigrationReady` condition MUST be removed from the VirtualMachine when the migration stops reporting a `Ready` condition, or when its `Ready` condition no longer describes the migration's current generation, so that the VirtualMachine does not keep reporting an outcome of the migration's previous spec.
- Only when a `VirtualMachineInfraMigration` has a `Ready` condition with status `True` that describes its current generation (the `Ready` condition's `observedGeneration` has caught up to the migration's `metadata.generation`), and a VirtualMachine with the same namespace and name exists and is not being deleted, VM Operator MUST set the VirtualMachine's `status.volumeAttributesClassName` to the name of the VolumeAttributesClass whose storage policy ID (`parameters["storagePolicyID"]`) is the migration's `spec.targetStorage.homeStoragePolicyID`, provided that VolumeAttributesClass is also the one in the VirtualMachine's `spec.volumeAttributesClassName`. When several VolumeAttributesClasses refer to the same policy, the one in the VirtualMachine's spec decides.
- VM Operator MUST NOT write `status.volumeAttributesClassName` when the migration has no `spec.targetStorage.homeStoragePolicyID`, when no VolumeAttributesClass refers to that policy, or when the VirtualMachine's `spec.volumeAttributesClassName` is unset or is not one of the VolumeAttributesClasses that refer to it. The last two cases most likely mean that the spec was edited again and the mobility operator has not picked it up yet, so VM Operator MUST NOT treat them as an error: it MUST still mirror the `Ready` condition, and it writes nothing until the VirtualMachine's spec, the migration, or the VolumeAttributesClasses change.
- VM Operator does not pause the reconciliation of a VirtualMachine while a `VirtualMachineInfraMigration` for it exists and is not ready (for example its `Ready` condition is `False`), and does not add a validation that rejects a change to the VirtualMachine's `spec.volumeAttributesClassName` while the migration is running. Neither is needed: VM Operator already skips reconciling a VirtualMachine's spec while a task is running on it in vSphere (including DRS activity), which is enough to avoid concurrent operations on the VirtualMachine.
- The mirrored condition's status, reason, and message MUST be copied from the migration's `Ready` condition (from VMIM .status.conditions to VM .status.conditions) so that a user can read the outcome from the VirtualMachine alone. The mirrored message MUST be followed by a reference to the migration, in the form `<message> @ VirtualMachineInfraMigration/<name>`, so that a user can tell which resource the condition came from. A `Ready` condition without a message is mirrored with only the reference as its message. A `Ready` condition without a reason is mirrored with the status as its reason, because a condition's reason must not be empty. If the message followed by the reference would be longer than a condition message can be (32768 characters), the migration's message MUST be truncated so that the reference is always present. The migration condition's `observedGeneration` MUST NOT be copied, since it refers to the migration's generation rather than the VirtualMachine's.
- Mirroring MUST NOT overwrite or drop any condition on the VirtualMachine other than the mirrored `Ready` condition, including conditions written concurrently by the VirtualMachine controller.
- Re-reconciling a migration whose `Ready` condition is unchanged MUST NOT change the VirtualMachine (in particular, the mirrored condition's last transition time MUST NOT move).
- VM Operator SHOULD log at Info level only when the status of the mirrored condition changes (including it being added or removed) or `status.volumeAttributesClassName` is newly recorded, so that reconciling an unchanged migration, or one whose `Ready` condition only changed its reason or message, does not write to the log at the default verbosity.
- If the VirtualMachine is created after the migration's `Ready` condition was set, the condition MUST still be mirrored once the VirtualMachine exists.
- When the `VirtualMachineInfraMigration` is deleted, VM Operator MUST remove the `VirtualMachineInfraMigrationReady` condition from the VirtualMachine, because it would otherwise describe a resource that no longer exists, and this includes a migration that was deleted while VM Operator was not running. It MUST NOT clear `status.volumeAttributesClassName` in any case, because that field describes what the VirtualMachine is configured to, not the migration.
- When the capability is disabled, VM Operator MUST NOT start the controller and MUST NOT require the `VirtualMachineInfraMigration` API to be present.

## Non-goals

- Creating, updating, or deleting `VirtualMachineInfraMigration` resources. The mobility operator owns them.
- Updating the VirtualMachine's storage policy usage (quota) after a migration. The mobility operator's design lists it as a separate VM Operator step.
- Performing any migration or calling vSphere. This controller only reads the migration resource and writes VirtualMachine status.
- Surfacing the migration's other conditions (`Validated`, `StorageInSync`, `ComputeInSync`) or its other status (for example the observed resource pool) on the VirtualMachine.
- Updating `status.volumes[].conditions[]`. The migration has no per-volume status, so there is nothing to mirror.
- Patching the VirtualMachine's spec, including `spec.volumeAttributesClassName`.
- Any VirtualMachine API surface beyond the mirrored `Ready` condition and `status.volumeAttributesClassName`.
- E2E coverage. It will be added after all the components are ready, and is tracked by vmop-4499.

---

## User stories and acceptance criteria

### DevOps user: seeing that a migration has converged

As a DevOps user, I want the VirtualMachine to tell me the state of its infrastructure migration, so that I do not have to look up a second resource.

1. Given a VirtualMachine `ns/vm` and a `VirtualMachineInfraMigration` `ns/vm` whose `Ready` condition is `True` for the migration's current generation, `kubectl get vm vm -o jsonpath='{.status.conditions[?(@.type=="VirtualMachineInfraMigrationReady")].status}'` returns `True`. If the `Ready` condition is `False` or `Unknown`, the same command returns `False` or `Unknown`.
2. The mirrored condition has the same status and reason as the migration's `Ready` condition, and its message followed by `@ VirtualMachineInfraMigration/<name>`. The migration's other conditions (for example `Validated`) are not mirrored, and the `observedGeneration` is not copied.
3. Given a migration whose `Ready` condition is `False`, `Unknown`, absent, or whose `observedGeneration` is less than the migration's `metadata.generation`, `status.volumeAttributesClassName` is not set by this feature.
4. Given a migration for a VirtualMachine that does not exist yet, creating the VirtualMachine results in the condition being mirrored without any further change to the migration.
5. All other conditions on the VirtualMachine are unchanged by the feature. When the migration stops reporting its `Ready` condition, the mirrored condition is removed from the VirtualMachine.
6. Given a ready migration whose `spec.targetStorage.homeStoragePolicyID` is the storage policy of VolumeAttributesClass `vac-a`, and a VirtualMachine whose `spec.volumeAttributesClassName` is `vac-a`, `kubectl get vm vm -o jsonpath='{.status.volumeAttributesClassName}'` returns `vac-a`.
7. Given a ready migration with a `homeStoragePolicyID`, but a VirtualMachine whose `spec.volumeAttributesClassName` is unset or refers to a different storage policy, or no VolumeAttributesClass for the policy, `status.volumeAttributesClassName` is not changed and the `VirtualMachineInfraMigrationReady` condition is still set.
8. Given a ready migration with no `homeStoragePolicyID`, `status.volumeAttributesClassName` is not changed.
9. Given the migration is then deleted, the VirtualMachine no longer has the `VirtualMachineInfraMigrationReady` condition, and `status.volumeAttributesClassName` keeps its value.
10. The mirrored condition is copied only when the `Ready` condition's `observedGeneration` has caught up to the migration's `metadata.generation`. When the migration's spec changes and its generation advances, the VirtualMachine's mirrored condition is removed until the migration reports `Ready` for the new generation. This avoids a stale condition, such as the VirtualMachine briefly reporting `True` for a migration that describes its previous spec.

### CSP admin: rollout safety

1. With the capability disabled, VM Operator starts and runs on a Supervisor that has no `VirtualMachineInfraMigration` CRD installed.

---
