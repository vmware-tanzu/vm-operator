# vSphere Policies

vSphere Policies provide a declarative way to manage compute policies, tags, and operational constraints for virtual machines running in a vSphere environment. These policies are part of the `vsphere.policy.vmware.com` API group and integrate with VM Operator to enable policy-driven VM management.

## Overview

The vSphere Policy APIs introduce three main resource types:

- **ComputePolicy**: Defines compute-related policies for workloads
- **TagPolicy**: Defines vSphere tags to be applied to workloads
- **PolicyEvaluation**: Evaluates which policies should apply to a workload

These resources work together to provide flexible, declarative policy management that can be applied either explicitly or automatically based on matching criteria.

## ComputePolicy

A `ComputePolicy` resource defines compute-related policies that control various aspects of VM behavior, placement, and resource allocation in vSphere.

### Specification

```yaml
apiVersion: vsphere.policy.vmware.com/v1alpha1
kind: ComputePolicy
metadata:
  name: production-policy
  namespace: my-namespace
spec:
  description: "Production workload policy"
  policyID: "vsphere-policy-123"  # Optional vSphere policy ID
  enforcementMode: Mandatory       # Or Optional
  match:
    workload:
      labels:
      - key: tier
        operator: In
        values:
        - production
      kind: VirtualMachine
  tags:
  - production-tag-policy
```

### Enforcement Modes

ComputePolicy resources support two enforcement modes:

#### Mandatory Policies

- Automatically applied to all matching workloads in the namespace
- Cannot be overridden or removed by users
- Applied based on the `match` criteria
- If no `match` is specified, applies to all workloads in the namespace

#### Optional Policies

- Must be explicitly referenced in a VM's `spec.policies` field
- Users can choose whether to apply these policies
- Provide flexibility for workload-specific requirements

### Match Specifications

The `match` field allows sophisticated matching based on:

#### Workload Properties
- **Labels**: Match based on workload labels using label selector requirements
- **Kind**: Filter by workload type (`Pod` or `VirtualMachine`)
- **Guest OS**: Match based on guest ID or guest family

#### Image Properties
- **Name**: Match based on image name patterns
- **Labels**: Match based on image labels

#### Boolean Operations
- **And**: All conditions must match (default)
- **Or**: At least one condition must match

Example of complex matching:

```yaml
spec:
  match:
    op: Or
    match:
    - workload:
        labels:
        - key: tier
          operator: In
          values: [production]
    - image:
        name:
          op: HasPrefix
          value: "production-"
```

## TagPolicy

A `TagPolicy` resource defines vSphere tags that should be applied to workloads. These are typically referenced by ComputePolicy objects.

### Specification

```yaml
apiVersion: vsphere.policy.vmware.com/v1alpha1
kind: TagPolicy
metadata:
  name: production-tag-policy
  namespace: my-namespace
spec:
  tags:
  - "urn:vmomi:InventoryServiceTag:12345678-1234-1234-1234-123456789abc:GLOBAL"
  - "urn:vmomi:InventoryServiceTag:87654321-4321-4321-4321-cba987654321:GLOBAL"
```

Tags are specified as UUIDs that correspond to tags configured in vSphere. When a ComputePolicy references a TagPolicy, the tags are applied to activate the policy on the workload.

## PolicyEvaluation

A `PolicyEvaluation` resource is used to evaluate which policies should apply to a theoretical workload configuration. This is useful for testing and validation before actually creating VMs.

### Specification

```yaml
apiVersion: vsphere.policy.vmware.com/v1alpha1
kind: PolicyEvaluation
metadata:
  name: test-evaluation
  namespace: my-namespace
spec:
  workload:
    labels:
      tier: production
    guest:
      guestID: ubuntu64Guest
      guestFamily: Linux
  image:
    name: ubuntu-22.04
    labels:
      os: ubuntu
  policies:
  - apiVersion: vsphere.policy.vmware.com/v1alpha1
    kind: ComputePolicy
    name: specific-policy
```

### Status

After evaluation, the status will contain the list of policies that would apply:

```yaml
status:
  policies:
  - apiVersion: vsphere.policy.vmware.com/v1alpha1
    kind: ComputePolicy
    name: production-policy
    observedGeneration: 2
    tags:
    - "urn:vmomi:InventoryServiceTag:12345678-1234-1234-1234-123456789abc:GLOBAL"
  - apiVersion: vsphere.policy.vmware.com/v1alpha1
    kind: ComputePolicy
    name: namespace-default
    observedGeneration: 1
  conditions:
  - type: Ready
    status: "True"
```

## Applying Policies to VirtualMachines

### Explicit Application

Optional policies can be explicitly applied to a VM using the `spec.policies` field:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachine
metadata:
  name: my-vm
  namespace: my-namespace
spec:
  className: my-vm-class
  imageName: ubuntu-22.04
  storageClass: my-storage-class
  policies:
  - apiVersion: vsphere.policy.vmware.com/v1alpha1
    kind: ComputePolicy
    name: performance-policy
  - apiVersion: vsphere.policy.vmware.com/v1alpha1
    kind: ComputePolicy
    name: compliance-policy
```

### Automatic Application

Mandatory policies are automatically applied based on their match criteria. The VM's status will reflect all applied policies:

```yaml
status:
  policies:
  - apiVersion: vsphere.policy.vmware.com/v1alpha1
    kind: ComputePolicy
    name: performance-policy
    generation: 3
  - apiVersion: vsphere.policy.vmware.com/v1alpha1
    kind: ComputePolicy
    name: compliance-policy
    generation: 1
  - apiVersion: vsphere.policy.vmware.com/v1alpha1
    kind: ComputePolicy
    name: namespace-mandatory-policy
    generation: 2
```

The `generation` field indicates which version of the policy was applied, helping track when policies need to be re-evaluated.

## Match Operations

The policy matching system supports various string and pattern matching operations:

| Operation | Description | Example |
|-----------|-------------|---------|
| `Equal` | Exact match | `value: "production"` |
| `NotEqual` | Does not match | `value: "development"` |
| `HasPrefix` | Starts with | `value: "prod-"` |
| `NotHasPrefix` | Does not start with | `value: "test-"` |
| `HasSuffix` | Ends with | `value: "-prod"` |
| `NotHasSuffix` | Does not end with | `value: "-test"` |
| `Contains` | Contains substring | `value: "production"` |
| `NotContains` | Does not contain | `value: "test"` |
| `Match` | Regex match | `value: "^prod-.*-v[0-9]+$"` |
| `NotMatch` | Does not match regex | `value: "^test-.*"` |

## Guest Family Types

When matching based on guest operating system family, the following values are supported:

- `Darwin`: macOS guests
- `Linux`: Linux-based guests
- `Netware`: NetWare guests
- `Other`: Other guest types
- `Solaris`: Solaris guests
- `Windows`: Windows guests

## Best Practices

### Policy Naming
- Use descriptive names that indicate the policy's purpose
- Include environment or tier indicators (e.g., `production-compute-policy`)
- Be consistent with naming conventions across namespaces

### Match Criteria
- Keep match criteria as specific as needed but not overly complex
- Use labels for flexible, user-controllable matching
- Use image and guest properties for system-enforced matching

### Enforcement Modes
- Use mandatory policies for namespace-wide requirements
- Use optional policies for workload-specific optimizations
- Document which policies are available and their purposes

### Policy Updates
- Monitor the `generation` field in VM status to track policy versions
- Test policy changes with PolicyEvaluation before applying
- Consider the impact on existing VMs when updating policies

## Troubleshooting

### Policies Not Applied
- Check the policy's `enforcementMode` - optional policies need explicit reference
- Verify match criteria using a PolicyEvaluation resource
- Ensure the policy exists in the same namespace as the VM

### Policy Conflicts
- Review all mandatory policies in the namespace
- Check for overlapping match criteria
- Use PolicyEvaluation to test combinations

### Tag Activation
- Verify TagPolicy resources contain valid vSphere tag UUIDs
- Ensure referenced TagPolicy resources exist
- Check vSphere permissions for tag application