# VirtualMachineGroup

VirtualMachineGroup resources enable coordinated management of multiple VirtualMachines, providing capabilities for grouped power operations, boot sequencing, placement decisions, and affinity rules.

## Overview

A VirtualMachineGroup is a namespace-scoped Kubernetes resource that:
- Groups VMs together for coordinated management
- Controls power states across multiple VMs
- Defines boot order and startup sequences
- Enables affinity/anti-affinity rules for member VMs
- Provides placement recommendations and status tracking
- Supports hierarchical group structures

## Creating a VirtualMachineGroup

### Basic Group

A simple group that manages multiple VMs:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha4
kind: VirtualMachineGroup
metadata:
  name: my-application-group
  namespace: production
spec:
  powerState: PoweredOn
```

### Group with Boot Order

Define a startup sequence for multi-tier applications:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha4
kind: VirtualMachineGroup
metadata:
  name: three-tier-app
  namespace: production
spec:
  powerState: PoweredOn
  bootOrder:
  # First: Start the database tier
  - members:
    - name: postgres-primary
      kind: VirtualMachine
    - name: postgres-replica
      kind: VirtualMachine
    delay: 30s  # Wait 30 seconds before starting next tier

  # Second: Start the application tier
  - members:
    - name: app-server-1
      kind: VirtualMachine
    - name: app-server-2
      kind: VirtualMachine
    - name: app-server-3
      kind: VirtualMachine
    delay: 20s  # Wait 20 seconds before starting next tier

  # Third: Start the web tier
  - members:
    - name: nginx-1
      kind: VirtualMachine
    - name: nginx-2
      kind: VirtualMachine
```

## Joining VMs to a Group

VirtualMachines join a group by setting their `spec.groupName` field:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachine
metadata:
  name: app-server-1
  namespace: production
spec:
  groupName: three-tier-app  # Join the group
  className: my-vm-class
  imageName: ubuntu-22.04
  storageClass: fast-ssd
  # Affinity rules can only be used when groupName is set
  affinity:
    vmAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
        labelSelector:
        - matchLabels:
            tier: app
        topologyKey: kubernetes.io/hostname
```

## Boot Order Specification

The `bootOrder` field defines how VMs should be started:

### Boot Groups
- VMs are organized into sequential boot groups
- Each group contains one or more members
- All members in a group start simultaneously

### Delays
- Optional delay after each boot group
- Ensures dependencies are ready before proceeding
- Specified as a duration string (e.g., "30s", "1m", "90s")

### Member Types
Boot order members can be:
- **VirtualMachine**: Individual VM instances
- **VirtualMachineGroup**: Nested groups (for hierarchical structures)

Example with mixed member types:

```yaml
bootOrder:
- members:
  - name: infrastructure-group
    kind: VirtualMachineGroup  # Another group
  delay: 1m
- members:
  - name: database-vm
    kind: VirtualMachine
  - name: cache-vm
    kind: VirtualMachine
  delay: 30s
```

## Power State Management

### Group Power Operations

Setting `spec.powerState` on a group affects all members:

```yaml
spec:
  powerState: PoweredOn   # Powers on all members following boot order
  # or
  powerState: PoweredOff  # Powers off all members immediately
  # or
  powerState: Suspended   # Suspends all members
```

### Power State Synchronization

- Members automatically sync to the group's power state
- New members added to a powered-on group will be powered on
- Power state changes follow the defined boot order (for power on)
- Power off operations happen immediately without delays

### Individual VM Power Control

When a VM belongs to a group:
- Group power state takes precedence
- Individual VM power state changes may be overridden
- Best practice: manage power at the group level

## Hierarchical Groups

Groups can belong to other groups, creating hierarchies:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha4
kind: VirtualMachineGroup
metadata:
  name: database-tier
  namespace: production
spec:
  groupName: parent-application-group  # This group belongs to another
  powerState: PoweredOn
  bootOrder:
  - members:
    - name: postgres-primary
      kind: VirtualMachine
    delay: 10s
  - members:
    - name: postgres-replica-1
      kind: VirtualMachine
    - name: postgres-replica-2
      kind: VirtualMachine
```

### Parent Group Example

```yaml
apiVersion: vmoperator.vmware.com/v1alpha4
kind: VirtualMachineGroup
metadata:
  name: parent-application-group
  namespace: production
spec:
  powerState: PoweredOn
  bootOrder:
  - members:
    - name: database-tier
      kind: VirtualMachineGroup  # Child group
    delay: 1m
  - members:
    - name: application-tier
      kind: VirtualMachineGroup
    delay: 30s
  - members:
    - name: web-tier
      kind: VirtualMachineGroup
```

## Affinity and Anti-Affinity

VMs in a group can use affinity rules, but **only when `spec.groupName` is set**:

### Zone Anti-Affinity Example

Distribute group members across availability zones:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachine
metadata:
  name: web-server-1
  namespace: production
  labels:
    app: nginx
    tier: web
spec:
  groupName: web-tier  # Required for affinity
  className: my-vm-class
  imageName: nginx-image
  storageClass: fast-ssd
  affinity:
    zoneAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        labelSelector:
        - matchLabels:
            tier: web
        topologyKey: topology.kubernetes.io/zone
```

### Host Affinity Example

Keep related VMs on the same host for performance:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachine
metadata:
  name: app-cache
  namespace: production
  labels:
    component: cache
spec:
  groupName: app-tier  # Required for affinity
  className: my-vm-class
  imageName: redis-image
  storageClass: fast-ssd
  affinity:
    vmAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        labelSelector:
        - matchLabels:
            component: app-server
        topologyKey: kubernetes.io/hostname
```

## Status and Conditions

### Group Status

Monitor the group's operational state:

```yaml
status:
  members:
  - name: database-vm
    kind: VirtualMachine
    conditions:
    - type: GroupLinked
      status: "True"
      reason: MemberLinked
      message: Member is linked to this group
    - type: PowerStateSynced
      status: "True"
      reason: PowerStateMatches
      message: Member power state matches group specification
    - type: PlacementReady
      status: "True"
      reason: PlacementComputed
      message: Placement decision is ready
    placement:
      datastores:
      - name: vsanDatastore
        id: datastore-123

  lastUpdatedPowerStateTime: "2024-01-15T10:30:00Z"

  conditions:
  - type: Ready
    status: "True"
    reason: AllMembersReady
    message: All group members are in desired state
```

### Member Conditions

Each member reports these conditions:

| Condition | Description |
|-----------|-------------|
| `GroupLinked` | Member VM has `spec.groupName` set to this group |
| `PowerStateSynced` | Member's power state matches group's desired state |
| `PlacementReady` | Placement decision is available for the member |

### Group Conditions

| Condition | Description |
|-----------|-------------|
| `Ready` | All members are in their desired state |

## Use Cases

### 1. Multi-Tier Applications

Manage complex applications with dependencies:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha4
kind: VirtualMachineGroup
metadata:
  name: e-commerce-platform
spec:
  bootOrder:
  - members:
    - name: mysql-primary
    - name: redis-cache
    delay: 45s
  - members:
    - name: catalog-service
    - name: inventory-service
    - name: payment-service
    delay: 30s
  - members:
    - name: api-gateway
    - name: web-frontend
```

### 2. Development Environments

Quick spin-up/down of complete dev stacks:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha4
kind: VirtualMachineGroup
metadata:
  name: dev-environment
spec:
  powerState: PoweredOff  # Default off to save resources
  bootOrder:
  - members:
    - name: dev-db
    - name: dev-cache
    delay: 20s
  - members:
    - name: dev-app
    - name: dev-worker
```

### 3. Disaster Recovery

Coordinate failover scenarios:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha4
kind: VirtualMachineGroup
metadata:
  name: dr-site
spec:
  powerState: PoweredOff  # Standby state
  bootOrder:
  - members:
    - name: dr-database-primary
    delay: 1m
  - members:
    - name: dr-app-1
    - name: dr-app-2
    delay: 30s
  - members:
    - name: dr-load-balancer
```

## Best Practices

### Design Considerations

1. **Group Size**: Keep groups manageable (10-20 VMs max for optimal performance)
2. **Boot Delays**: Use minimum necessary delays to reduce startup time
3. **Hierarchy Depth**: Limit to 2-3 levels for maintainability
4. **Naming**: Use clear, descriptive names indicating purpose

### Operational Guidelines

1. **Power Management**:
    - Always use group-level power control when possible
    - Monitor member conditions for sync issues
    - Test boot sequences in non-production first

2. **Affinity Rules**:
    - Remember: VMs must have `spec.groupName` set to use affinity
    - Design rules considering group membership changes
    - Balance between performance and availability

3. **Boot Order**:
    - Group independent services together
    - Account for service initialization time in delays
    - Document dependencies clearly

### Troubleshooting

Common issues and solutions:

| Issue | Cause | Solution |
|-------|-------|----------|
| VM won't use affinity rules | Missing `spec.groupName` | Set VM's `spec.groupName` field |
| Boot order not followed | Members not in group | Verify all VMs have correct `groupName` |
| Power state not syncing | Conflicting individual settings | Check member conditions and remove individual power settings |
| Placement not optimal | Missing affinity rules | Add appropriate affinity/anti-affinity rules |

## Migration from Individual VMs

To migrate existing VMs to group management:

1. Create the VirtualMachineGroup resource
2. Update VM specs to include `groupName`
3. Remove individual power state settings
4. Add affinity rules as needed
5. Test boot order in staging
6. Apply power state at group level

## API Reference

For detailed API specifications, see:
- VirtualMachineGroup CRD documentation
- VirtualMachine `spec.groupName` field documentation
- Affinity rules documentation

## Related Resources

- [VirtualMachine](./vm.md) - Core VM resource documentation
- [VirtualMachine Affinity](./vm.md#affinity) - Detailed affinity rules
- [VirtualMachine Power Management](./vm.md#power-management) - Power state details