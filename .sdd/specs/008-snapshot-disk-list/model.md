# Data Model: Expose Disk List in VirtualMachineSnapshot

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)

## CRD Schema — `vmoperator.vmware.com/v1alpha6`

### `VirtualMachineSnapshotStatus`

Added field:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `status.disks` | `[]VirtualMachineSnapshotDiskStatus` | optional | List of disks included in the snapshot. Annotated with `+listType=map` and `+listMapKey=id`. |

### `VirtualMachineSnapshotDiskStatus`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | `string` | required | Unique identifier (UUID) of the disk in the snapshot. |
| `changedBlockTrackingID` | `string` | optional | The Change Block Tracking (CBT) change ID for the disk at snapshot creation time. Omitted if CBT is disabled or unavailable. |

---

## API Conversion Strategy (`v1alpha5` ↔ `v1alpha6`)

- **Hub version**: `v1alpha6`
- **Down-conversion (`v1alpha6` → `v1alpha5`)**:
  - The `status.disks` field does not exist in `v1alpha5`.
  - `utilconversion.MarshalData(src, dst)` stores the full `v1alpha6` object into the `vmoperator.vmware.com/conversion-data` annotation on the `v1alpha5` object.
- **Up-conversion (`v1alpha5` → `v1alpha6`)**:
  - `utilconversion.UnmarshalData(src, restored)` deserializes the stored annotation and restores `dst.Status.Disks = restored.Status.Disks`.

---

## Example YAML

```yaml
apiVersion: vmoperator.vmware.com/v1alpha6
kind: VirtualMachineSnapshot
metadata:
  name: my-vm-snap-1
  namespace: my-namespace
spec:
  vmName: my-vm
status:
  ready: true
  powerState: "PoweredOn"
  quiesced: true
  uniqueID: "snapshot-101"
  disks:
  - id: "6000C29d-47be-a1c2-3e4b-9128374a5f6e"
    changedBlockTrackingID: "52 4b 8f d3 2c 1a 9f 01-44 5e 7d 88 12 34 56 78/1"
  - id: "6000C29d-47be-a1c2-3e4b-9128374a5f6f"
    changedBlockTrackingID: "52 4b 8f d3 2c 1a 9f 01-44 5e 7d 88 12 34 56 78/1"
  conditions:
  - type: Ready
    status: "True"
    lastTransitionTime: "2026-08-18T10:00:00Z"
    reason: SnapshotCreated
```
