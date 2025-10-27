# VirtualMachineSnapshot

VirtualMachineSnapshot resources enable consumers to take Snapshot of their VirtualMachines. It leverages the providers Snapshot capabilities to create VM snapshots.

## Overview

A VirtualMachineSnapshots is a namespace-scoped Kubernetes resource that:

- Creates snapshots in the provider for the referred VM
- Supports taking a snapshot with or without memory
- Supports quiescing the guest when creating a snapshot

## Creating a VirtualMachineSnapshot

### Snapshot without memory

A snapshot which doesn't include VM memory:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachineSnapshot
metadata:
  name: snap-1
spec:
  description: "Snapshot of my-vm without memory"
  vmName: my-vm
```

### Snapshot with memory

A snapshot which includes VM memory:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachineSnapshot
metadata:
  name: snap-2
spec:
  description: "Snapshot of my-vm with memory"
  vmName: my-vm
  memory: true
```

### Snapshot while queiscing

A snapshot which specifies a Quiesce timeout:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachineSnapshot
metadata:
  name: snap-3
spec:
  description: "Snapshot of my-vm with a consistent state of the guest filesystem"
  vmName: my-vm
  memory: true
  queisce:
    timeout: 10min
```

> Note: VMware Tools must be installed on the Guest OS.

## Reverting to a snapshot

Set the `spec.currentSnapshotName` on the VM to revert to a snapshot:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachine
metadata:
  name: my-vm
spec:
  currentSnapshotName: snap-1
```

Once the VM is reverted, `status.currentSnapshot` should refer to the snapshot set in `spec.currentSnapshotName` and `spec.currentSnapshotName` would be removed if the VM was succesfully reverted.

```yaml
status:
  currentSnapshot:
    type: Managed
    name: snap-1
```

Please refer to the [Troubleshooting](#troubleshooting) section below if the operation fails.

## Status and Conditions

### Status

#### VirtualMachineSnapshot

```yaml
status:
  children:
  - name: snap-2
    type: Managed
  conditions:
  - lastTransitionTime: "2025-10-27T14:55:26Z"
    message: ""
    reason: "True"
    status: "True"
    type: VirtualMachineSnapshotCSISynced
  - lastTransitionTime: "2025-10-27T14:55:26Z"
    message: ""
    reason: "True"
    status: "True"
    type: VirtualMachineSnapshotCreated
  - lastTransitionTime: "2025-10-27T14:55:26Z"
    message: ""
    reason: "True"
    status: "True"
    type: VirtualMachineSnapshotReady
  powerState: PoweredOn
  storage:
    requested:
    - storageClass: wcpglobal-storage-profile
      total: 25Gi
    used: "12226581330"
  uniqueID: snapshot-226
```

#### VirtualMachine

```yaml
status:
...
  currentSnapshot:
    name: snap-2
    type: Managed
...
  rootSnapshots:
  - name: snap-1
    type: Managed
...
```

### Conditions

#### VirtualMachineSnapshot

| Condition                         | Description                               |
| --------------------------------- | ----------------------------------------- |
| `VirtualMachineSnapshotCSISynced` | CSI volume has been synced                |
| `VirtualMachineSnapshotCreated`   | Snapshot has been created on the provider |
| `VirtualMachineSnapshotReady`     | Snapshot has been succesfully reconciled  |

#### VirtualMachine

| Condition                               | Description                                                                                                                                                                 |
| --------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `VirtualMachineSnapshotRevertSucceeded` | Snapshot is succesfully reverted. This condition can be seen only when the snapshot revert operation fails for some reason. Steady state is for the condition to not exist. |

### Unmanaged Snapshots

Snapshots directly created via the provider can't be managed through VM Operator.
Those snapshots will be marked as `Unamanaged` in VM's `status.currentSnapshot`
and `status.rootSnapshots`.

## Best Practices

### Design Considerations

1. **Meaningful Names**: Use descriptive names that indicate the purpose of the snapshot
2. **Use Description**: Use the description field to add more context
3. **Number of snapshots**: Prefer limiting the number of snapshots to less than 32
4. **Unmanaged Snapshots**: Avoid creating unmanaged snapshots directly on the provider

### Troubleshooting

Common issues and solutions:

| Issue                           | Cause                            | Solution                                                                                             |
| ------------------------------- | -------------------------------- | ---------------------------------------------------------------------------------------------------- |
| Snapshot doesn't include memory | Missing `spec.memory`            | Set VM Snapshot's `spec.memory` field to `true`                                                      |
| VM reverts to a PoweredOff mode | Snapshot taken without memory    | Set VM Snapshot's `spec.memory` field to `true`                                                      |
| Snapshot create failed          | Might be a VKS node              | VKS nodes can't be snapshotted or reverted                                                           |
| VM not reverting                | Might be a VKS node              | VKS nodes can't be snapshotted or reverted                                                           |
| Snapshot revert failed          | CSI Snapshot exists for a volume | Delete any CSI volume snapshot which exists between current state and the snapshot being reverted to |


## API Reference

For detailed API specifications, see:

- VirtualMachineSnapshot CRD documentation
- VirtualMachine `spec.currentSnapshotName`, `status.currentSnapshot` and `status.rootSnapshots` field documentation

## Related Resources

- [VirtualMachine](./vm.md) - Core VM resource documentation
