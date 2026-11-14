# Model: VirtualMachineInfraMigration Status on VirtualMachine

This feature adds a family of status condition types and one status field to `v1alpha6`. The condition types need no schema change because condition types are free-form strings in `status.conditions`. The field changes the `VirtualMachine` CRD schema.

## Status conditions

### `VirtualMachineConditionInfraMigrationPrefix` (`VirtualMachineInfraMigration`)

Defined in `api/v1alpha6/virtualmachine_types.go`. Every condition of the `VirtualMachineInfraMigration` with the same namespace and name as the VirtualMachine is mirrored onto the VirtualMachine as a condition whose type is this prefix followed by the migration condition's type.

| Migration condition type | VirtualMachine condition type |
|--------------------------|-------------------------------|
| `Ready` | `VirtualMachineInfraMigrationReady` (`VirtualMachineConditionInfraMigrationReady`) |
| `Validated` | `VirtualMachineInfraMigrationValidated` |
| `StorageInSync` | `VirtualMachineInfraMigrationStorageInSync` |
| `ComputeInSync` | `VirtualMachineInfraMigrationComputeInSync` |
| any other type `T` | `VirtualMachineInfraMigrationT` |

| Field | Mirrored value |
|-------|----------------|
| `status` | the migration condition's status (`True`, `False`, or `Unknown`) |
| `reason` | the migration condition's reason, or the status if it has none |
| `message` | the migration condition's message followed by `@ VirtualMachineInfraMigration/<name>`, or only `VirtualMachineInfraMigration/<name>` if it has no message |
| `observedGeneration` | not set |
| `lastTransitionTime` | the time the mirrored status, reason, or message last changed on the VirtualMachine |

| State of the migration | VirtualMachine conditions |
|------------------------|---------------------------|
| reports a condition for its current generation | the mirrored condition is set to the migration condition's current state |
| reports a condition for a previous generation (`observedGeneration` less than `metadata.generation`, or unset) | the mirrored condition is removed |
| stops reporting a condition | the mirrored condition is removed |
| no longer exists | the mirrored conditions are left as they were |

Design notes:

- A condition is mirrored whatever its status, but only if it describes the migration's current generation: its own `observedGeneration` must be at least the migration's `metadata.generation`, so one condition can be held back while another is mirrored. A condition that is behind describes the previous spec, so it is not mirrored, and the mirrored condition of that type is removed instead of being left on the VirtualMachine as an out-of-date outcome. A condition without an `observedGeneration` is never mirrored. `status.volumeAttributesClassName` is held back further, until the migration's `Ready` condition is `True` for the current generation.
- `observedGeneration` is not copied, because it refers to the migration's generation and the other VirtualMachine conditions leave it unset.
- The migration's own condition semantics are defined by the mobility operator's API: `Ready=True` means the migration has converged on the target, and a `Ready` condition whose `observedGeneration` is less than `metadata.generation` describes the previous spec.
- Every VirtualMachine condition whose type starts with the prefix is owned by this controller, and is removed if the migration does not report a condition of that type.

## Status fields

### `status.volumeAttributesClassName`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `status.volumeAttributesClassName` | string | No | The observed name of the VolumeAttributesClass (also known as the storage profile name) that the VirtualMachine is configured to. |

Defined in `api/v1alpha6/virtualmachine_types.go` next to `status.storage`. It is the observed counterpart of `spec.volumeAttributesClassName`.

Design notes:

- The migration has no VolumeAttributesClass name. The value is the VolumeAttributesClass on the cluster whose `parameters["storagePolicyID"]` equals the migration's `spec.targetStorage.homeStoragePolicyID`, and it is only written if that is also the VirtualMachine's `spec.volumeAttributesClassName`. When several VolumeAttributesClasses share the policy, the one in the spec decides.
- Nothing is written when the migration has no `homeStoragePolicyID`, when no VolumeAttributesClass refers to the policy, or when the spec's value is unset or not among the matches. The last two cases emit a `VolumeAttributesClassMismatch` warning event on the VirtualMachine.
- It is written only when the migration's `Ready` condition is `True` and its `observedGeneration` is at least the migration's `metadata.generation`. The mirrored conditions follow their own rule, a condition is mirrored when it describes the current generation, so they are written whether or not the migration is ready.
- It is not cleared when the migration is deleted.
- Older API versions do not carry it. Each older version's `ConvertTo` restores the whole hub `status` from the conversion annotation, so no per-field restore function is needed, and the generated conversion code only gains a `does not exist in peer-type` warning line.

## Mapping between the two resources

| VirtualMachineInfraMigration | VirtualMachine |
|------------------------------|----------------|
| `metadata.namespace` / `metadata.name` | `metadata.namespace` / `metadata.name` (identical) |
| `status.conditions[type=T].status` | `status.conditions[type=VirtualMachineInfraMigrationT].status` |
| `status.conditions[type=T].reason` | `status.conditions[type=VirtualMachineInfraMigrationT].reason` |
| `status.conditions[type=T].message` | `status.conditions[type=VirtualMachineInfraMigrationT].message`, followed by a reference to the migration |
| `spec.targetStorage.homeStoragePolicyID`, resolved to the VolumeAttributesClass with the same `parameters["storagePolicyID"]`, and checked against the VirtualMachine's `spec.volumeAttributesClassName` | `status.volumeAttributesClassName` |

A migration condition is only mapped while its `observedGeneration` is at least the migration's `metadata.generation`.
