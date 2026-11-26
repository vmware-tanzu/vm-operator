# Model: VirtualMachineInfraMigration Status on VirtualMachine

This feature adds one status condition type and one status field to `v1alpha6`. The condition type needs no schema change because condition types are free-form strings in `status.conditions`. The field changes the `VirtualMachine` CRD schema.

## Status conditions

### `VirtualMachineConditionInfraMigrationReady` (`VirtualMachineInfraMigrationReady`)

Defined in `api/v1alpha6/virtualmachine_types.go`. The `Ready` condition of the `VirtualMachineInfraMigration` with the same namespace and name as the VirtualMachine is mirrored onto the VirtualMachine as this condition, and indicates whether the migration has converged on its target infrastructure. The migration's other conditions (`Validated`, `StorageInSync`, `ComputeInSync`, and any the mobility operator adds) are not mirrored.

| Field | Mirrored value |
|-------|----------------|
| `status` | the migration `Ready` condition's status (`True`, `False`, or `Unknown`) |
| `reason` | the migration `Ready` condition's reason, or the status if it has none |
| `message` | the migration `Ready` condition's message followed by `@ VirtualMachineInfraMigration/<name>`, or only `VirtualMachineInfraMigration/<name>` if it has no message; the migration's message is truncated and ends in `...` when it would not leave room for the reference within the 32768 characters of a condition message |
| `observedGeneration` | not set |
| `lastTransitionTime` | the time the mirrored status, reason, or message last changed on the VirtualMachine |

| State of the migration | VirtualMachine condition |
|------------------------|--------------------------|
| reports `Ready` for its current generation | the mirrored condition is set to the migration condition's current state |
| reports `Ready` for a previous generation (`observedGeneration` less than `metadata.generation`, or unset) | the mirrored condition is removed |
| does not report `Ready` | the mirrored condition is removed |
| no longer exists | the mirrored condition is removed |

Design notes:

- `Ready` is mirrored whatever its status, but only if it describes the migration's current generation: its `observedGeneration` must be at least the migration's `metadata.generation`. A `Ready` condition that is behind describes the previous spec, so it is not mirrored, and the mirrored condition is removed instead of being left on the VirtualMachine as an out-of-date outcome. A `Ready` condition without an `observedGeneration` is never mirrored. `status.volumeAttributesClassName` is held back further, until `Ready` is `True` for the current generation.
- Only `Ready` is mirrored, so the VirtualMachine API has one documented condition type and does not change shape when the mobility operator adds or renames one of its conditions. A user who wants the detail follows the `@ VirtualMachineInfraMigration/<name>` reference to the migration. A failing migration's `Ready` message can already name the failing step, as in `StorageInSync: Task task-419 state is error`.
- `observedGeneration` is not copied, because it refers to the migration's generation and the other VirtualMachine conditions leave it unset.
- The migration's own condition semantics are defined by the mobility operator's API: `Ready=True` means the migration has converged on the target, and a `Ready` condition whose `observedGeneration` is less than `metadata.generation` describes the previous spec.
- The mirrored condition is owned by this controller (`patch.WithOwnedConditions`), so it is written or removed without treating a concurrent change as a conflict, and no other VirtualMachine condition is touched.

## Status fields

### `status.volumeAttributesClassName`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `status.volumeAttributesClassName` | string | No | The observed name of the VolumeAttributesClass (also known as the storage profile name) that the VirtualMachine is configured to. |

Defined in `api/v1alpha6/virtualmachine_types.go` next to `status.storage`. It is the observed counterpart of `spec.volumeAttributesClassName`.

Design notes:

- The migration has no VolumeAttributesClass name. The value is the VolumeAttributesClass on the cluster whose `parameters["storagePolicyID"]` equals the migration's `spec.targetStorage.homeStoragePolicyID`, and it is only written if that is also the VirtualMachine's `spec.volumeAttributesClassName`. When several VolumeAttributesClasses share the policy, the one in the spec decides.
- Nothing is written when the migration has no `homeStoragePolicyID`, when no VolumeAttributesClass refers to the policy, or when the spec's value is unset or not among the matches. The last two cases most likely mean that the spec was edited again and the mobility operator has not picked it up yet, so they are only logged at verbosity 2: the next change to the VirtualMachine's spec, the migration, or the VolumeAttributesClasses reconciles again.
- It is written only when the migration's `Ready` condition is `True` and its `observedGeneration` is at least the migration's `metadata.generation`. The mirrored condition follows its own rule, it is mirrored when `Ready` describes the current generation, so it is written whether or not the migration is ready.
- It is never cleared, including when the migration is deleted. Only VM Operator writes it, and it is the durable record of the outcome, while the migration and the mirrored condition are short-lived.
- Older API versions do not carry it. Each older version's `ConvertTo` restores the whole hub `status` from the conversion annotation, so no per-field restore function is needed, and the generated conversion code only gains a `does not exist in peer-type` warning line.
- The crypto reconciler reads it to tell whether the VirtualMachine's change to `spec.volumeAttributesClassName` is in progress: it is when this field is not empty and differs from the spec. An empty value counts as in sync, because only a completed migration records it, so it stays empty for a VirtualMachine that was created with a VolumeAttributesClass and never migrated. The observed counterpart for a PVC is Kubernetes' `status.currentVolumeAttributesClassName`, read the same way.

## `spec.crypto` under a VolumeAttributesClass

No field is added. The documentation of `spec.crypto.encryptionClassName` now says that the storage policy that must be encryption-enabled is the one of `spec.volumeAttributesClassName` when it is set, and otherwise the one of `spec.storageClass`. The behavior is described in the spec and the plan.

## Mapping between the two resources

| VirtualMachineInfraMigration | VirtualMachine |
|------------------------------|----------------|
| `metadata.namespace` / `metadata.name` | `metadata.namespace` / `metadata.name` (identical) |
| `status.conditions[type=Ready].status` | `status.conditions[type=VirtualMachineInfraMigrationReady].status` |
| `status.conditions[type=Ready].reason` | `status.conditions[type=VirtualMachineInfraMigrationReady].reason` |
| `status.conditions[type=Ready].message` | `status.conditions[type=VirtualMachineInfraMigrationReady].message`, followed by a reference to the migration |
| `spec.targetStorage.homeStoragePolicyID`, resolved to the VolumeAttributesClass with the same `parameters["storagePolicyID"]`, and checked against the VirtualMachine's `spec.volumeAttributesClassName` | `status.volumeAttributesClassName` |

The `Ready` condition is only mapped while its `observedGeneration` is at least the migration's `metadata.generation`.
