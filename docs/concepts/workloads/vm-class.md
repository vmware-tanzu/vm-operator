# VirtualMachineClass

VirtualMachineClass resources define the virtual hardware specifications and policies used to deploy VirtualMachine workloads. They act as templates that specify CPU, memory, storage, and device configurations, along with resource management policies for virtual machines.

## Overview

VirtualMachineClass serves as a blueprint for virtual machine hardware and policy configuration. When deploying a VirtualMachine, you reference a VirtualMachineClass to define:

- **Hardware Resources**: CPU count, memory allocation, and specialized devices
- **Resource Policies**: CPU and memory reservations, limits, and quality of service
- **Storage Configuration**: Instance storage volumes and storage classes
- **Advanced Settings**: vSphere-specific configuration through ConfigSpec
- **Controller Assignment**: Which controller manages VMs using this class

## Hardware Configuration

### Basic Hardware Specification

The `hardware` section defines the fundamental virtual hardware characteristics:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachineClass
metadata:
  name: my-vm-class
  namespace: my-namespace
spec:
  hardware:
    cpus: 4
    memory: 8Gi
```

### Virtual Devices

VirtualMachineClass supports specialized virtual devices for advanced workloads:

#### vGPU Devices

```yaml
spec:
  hardware:
    cpus: 4
    memory: 16Gi
    devices:
      vgpuDevices:
        - profileName: "grid_v100d-2q"
        - profileName: "grid_v100d-4q"
```

#### Dynamic DirectPath I/O Devices

```yaml
spec:
  hardware:
    cpus: 8
    memory: 32Gi
    devices:
      dynamicDirectPathIODevices:
        - vendorID: 0x10de  # NVIDIA
          deviceID: 0x1db4  # V100
          customLabel: "GPU-0"
        - vendorID: 0x8086  # Intel
          deviceID: 0x0d58  # Network Controller
          customLabel: "NIC-0"
```

### Instance Storage

Instance storage provides ephemeral, high-performance storage volumes:

```yaml
spec:
  hardware:
    cpus: 4
    memory: 16Gi
    instanceStorage:
      storageClass: "local-ssd"
      volumes:
        - size: 100Gi
        - size: 200Gi
```

## Resource Policies

Resource policies control how VMs consume infrastructure resources and implement quality of service guarantees.

### Resource Requests and Limits

```yaml
spec:
  hardware:
    cpus: 4
    memory: 16Gi
  policies:
    resources:
      requests:
        cpu: 2000m      # 2 CPU cores reserved
        memory: 8Gi     # 8GB memory reserved
      limits:
        cpu: 4000m      # Maximum 4 CPU cores
        memory: 16Gi    # Maximum 16GB memory
```

### Quality of Service Classes

VM Operator provides predefined classes with different QoS characteristics:

#### Best Effort Classes
No resource reservations; VMs compete for available resources:

```yaml
# Best effort - no guarantees
spec:
  hardware:
    cpus: 2
    memory: 4Gi
  # No policies section means best effort
```

#### Guaranteed Classes
Full resource reservations ensure predictable performance:

```yaml
# Guaranteed - full reservation
spec:
  hardware:
    cpus: 4
    memory: 16Gi
  policies:
    resources:
      requests:
        cpu: 4000m    # Reserve all CPUs
        memory: 16Gi  # Reserve all memory
```

## Advanced Configuration with ConfigSpec

The `configSpec` field allows direct vSphere configuration using VirtualMachineConfigSpec:

### Basic ConfigSpec Example

```yaml
spec:
  configSpec:
    _typeName: VirtualMachineConfigSpec
    numCPUs: 4
    memoryMB: 8192
    firmware: "efi"
```

### ExtraConfig Parameters

```yaml
spec:
  configSpec:
    _typeName: VirtualMachineConfigSpec
    extraConfig:
      - _typeName: OptionValue
        key: "my-custom-setting"
        value:
          _typeName: string
          _value: "my-value"
      - _typeName: OptionValue
        key: "another-setting"
        value:
          _typeName: string
          _value: "another-value"
```

### vGPU Configuration via ConfigSpec

```yaml
spec:
  configSpec:
    _typeName: VirtualMachineConfigSpec
    deviceChange:
      - _typeName: VirtualDeviceConfigSpec
        operation: add
        device:
          _typeName: VirtualPCIPassthrough
          key: 0
          backing:
            _typeName: VirtualPCIPassthroughVmiopBackingInfo
            vgpu: "grid_v100d-4q"
```

### Dynamic DirectPath I/O via ConfigSpec

```yaml
spec:
  configSpec:
    _typeName: VirtualMachineConfigSpec
    deviceChange:
      - _typeName: VirtualDeviceConfigSpec
        operation: add
        device:
          _typeName: VirtualPCIPassthrough
          key: 0
          backing:
            _typeName: VirtualPCIPassthroughDynamicBackingInfo
            deviceName: ""
            allowedDevice:
              - _typeName: VirtualPCIPassthroughAllowedDevice
                vendorId: 0x10de
                deviceId: 0x1db4
```

## Controller Assignment

The `controllerName` field specifies which controller manages VMs using this class:

```yaml
spec:
  controllerName: "vmoperator.vmware.com/vsphere"
  hardware:
    cpus: 2
    memory: 4Gi
```

**Default Behavior**: When omitted, the controller name defaults to the value of the environment variable `DEFAULT_VM_CLASS_CONTROLLER_NAME`, or `vmoperator.vmware.com/vsphere` if the environment variable is empty.

## Predefined VirtualMachineClasses

VM Operator provides a comprehensive set of predefined classes for common use cases:

### Best Effort Classes

| Class Name | CPUs | Memory | Use Case |
|------------|:----:|:------:|----------|
| `best-effort-xsmall` | 2 | 2Gi | Development, testing |
| `best-effort-small` | 2 | 4Gi | Small applications |
| `best-effort-medium` | 2 | 8Gi | Standard workloads |
| `best-effort-large` | 4 | 16Gi | Medium applications |
| `best-effort-xlarge` | 4 | 32Gi | Large applications |
| `best-effort-2xlarge` | 8 | 64Gi | High-memory workloads |
| `best-effort-4xlarge` | 16 | 128Gi | Compute-intensive tasks |
| `best-effort-8xlarge` | 32 | 128Gi | Massive parallel workloads |

### Guaranteed Classes

| Class Name | CPUs | Memory | CPU Reservation | Memory Reservation |
|------------|:----:|:------:|:---------------:|:------------------:|
| `guaranteed-xsmall` | 2 | 2Gi | 2000m | 2Gi |
| `guaranteed-small` | 2 | 4Gi | 2000m | 4Gi |
| `guaranteed-medium` | 2 | 8Gi | 2000m | 8Gi |
| `guaranteed-large` | 4 | 16Gi | 4000m | 16Gi |
| `guaranteed-xlarge` | 4 | 32Gi | 4000m | 32Gi |
| `guaranteed-2xlarge` | 8 | 64Gi | 8000m | 64Gi |
| `guaranteed-4xlarge` | 16 | 128Gi | 16000m | 128Gi |
| `guaranteed-8xlarge` | 32 | 128Gi | 32000m | 128Gi |

## Usage Examples

### Listing Available Classes

```bash
# List all VirtualMachineClasses in the current namespace
kubectl get vmclass

# List classes with detailed information
kubectl get vmclass -o wide

# Describe a specific class
kubectl describe vmclass best-effort-medium
```

**Example Output:**
```
NAME                   CPU   MEMORY   CAPABILITIES
best-effort-small      2     4Gi      
best-effort-medium     2     8Gi      
guaranteed-large       4     16Gi     
my-custom-class        8     32Gi     gpu,instance-storage
```

### Using VirtualMachineClass in VirtualMachine

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachine
metadata:
  name: my-vm
  namespace: default
spec:
  className: guaranteed-medium
  imageName: vmi-0a0044d7c690bcbea
  # ... other configuration
```

### Creating Custom VirtualMachineClass

#### Simple Custom Class

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachineClass
metadata:
  name: custom-web-server
  namespace: production
spec:
  description: "Optimized for web server workloads"
  hardware:
    cpus: 4
    memory: 8Gi
  policies:
    resources:
      requests:
        cpu: 2000m
        memory: 4Gi
```

#### Advanced Custom Class with GPU

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachineClass
metadata:
  name: ml-training-class
  namespace: ai-workloads
spec:
  description: "Machine learning training with GPU acceleration"
  hardware:
    cpus: 16
    memory: 64Gi
    devices:
      vgpuDevices:
        - profileName: "grid_v100d-8q"
    instanceStorage:
      storageClass: "fast-ssd"
      volumes:
        - size: 500Gi
  policies:
    resources:
      requests:
        cpu: 8000m
        memory: 32Gi
```

#### Class with Advanced vSphere Configuration

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachineClass
metadata:
  name: high-performance-class
  namespace: hpc
spec:
  description: "High-performance computing with custom vSphere settings"
  hardware:
    cpus: 32
    memory: 128Gi
  configSpec:
    _typeName: VirtualMachineConfigSpec
    numCPUs: 32
    memoryMB: 131072
    firmware: "efi"
    extraConfig:
      - _typeName: OptionValue
        key: "numa.nodeAffinity"
        value:
          _typeName: string
          _value: "0"
      - _typeName: OptionValue
        key: "numa.vcpu.preferHT"
        value:
          _typeName: string
          _value: "TRUE"
  policies:
    resources:
      requests:
        cpu: 16000m
        memory: 64Gi
```

## Resource Reservations and Profiles

### Reserved Profiles

VirtualMachineClass supports resource reservation profiles for guaranteed capacity:

```yaml
spec:
  hardware:
    cpus: 4
    memory: 16Gi
  reservedProfileID: "gold-tier-profile"
  reservedSlots: 10
  policies:
    resources:
      requests:
        cpu: 4000m
        memory: 16Gi
```

This configuration reserves 10 slots of the specified resources under the "gold-tier-profile" reservation profile.

## JSON Discriminators and Encoding

VirtualMachineClass uses JSON discriminators to encode vSphere API types in the `configSpec` field:

### Discriminator Properties

| Property | Description |
|----------|-------------|
| `_typeName` | The vSphere type name for the JSON object |
| `_value` | Wrapped value for interface types |

### Encoding Rules

1. **Type Names**: Always include `_typeName` for vSphere API objects
2. **Interface Values**: Wrap interface values with `_typeName` and `_value`
3. **Operations**: Only `add` operations are valid for device changes
4. **Sparse Data**: Omit unneeded fields (ConfigSpec is sparse)

## Best Practices

### Class Design

1. **Use Predefined Classes**: Start with predefined classes for common use cases
2. **Meaningful Names**: Use descriptive names that indicate purpose and characteristics
3. **Resource Alignment**: Align CPU and memory ratios with workload requirements
4. **Documentation**: Always include descriptions for custom classes

### Resource Management

1. **Right-Sizing**: Size classes appropriately to avoid resource waste
2. **QoS Selection**: Choose between best-effort and guaranteed based on requirements
3. **Reservation Strategy**: Use reservations for critical workloads only
4. **Instance Storage**: Use instance storage for temporary, high-performance needs

### Advanced Configuration

1. **ConfigSpec Sparingly**: Use ConfigSpec only when hardware/policies fields are insufficient
2. **Test Thoroughly**: Validate ConfigSpec changes in development environments
3. **Version Control**: Track ConfigSpec changes in version control systems
4. **Documentation**: Document custom ConfigSpec settings and their purposes

## Troubleshooting

### Common Issues

#### Class Not Found
```bash
# Check if class exists
kubectl get vmclass <class-name>

# List all available classes
kubectl get vmclass -A
```

#### Insufficient Resources
```bash
# Check cluster resource availability
kubectl describe node

# Check resource reservations
kubectl describe vmclass <class-name>
```

#### ConfigSpec Validation Errors
```bash
# Validate ConfigSpec syntax
kubectl describe vmclass <class-name>

# Check VM operator logs
kubectl logs -n vmware-system-vmop deployment/vmware-system-vmop-controller-manager
```

### Debugging Commands

```bash
# Get detailed class information
kubectl describe vmclass <class-name>

# Check class usage across VMs
kubectl get vm -o custom-columns=NAME:.metadata.name,CLASS:.spec.className

# View class with full ConfigSpec
kubectl get vmclass <class-name> -o yaml

# Check controller assignment
kubectl get vmclass <class-name> -o jsonpath='{.spec.controllerName}'
```

## Validation and Constraints

### Hardware Constraints

- **CPU Count**: Must be positive integer
- **Memory**: Must be valid Kubernetes quantity (e.g., "4Gi", "8192Mi")
- **Device IDs**: Must be valid hexadecimal values for DirectPath I/O devices

### Policy Constraints

- **Requests ≤ Limits**: Resource requests cannot exceed limits
- **Valid Quantities**: CPU and memory must be valid Kubernetes quantities
- **Reservation Profiles**: ReservedSlots requires ReservedProfileID

### ConfigSpec Constraints

- **Valid JSON**: ConfigSpec must be valid JSON with proper discriminators
- **Add Operations Only**: Device changes must use "add" operation
- **Type Names Required**: All vSphere objects must include `_typeName`

## Migration and Compatibility

### API Version Migration

When migrating between API versions:

1. **Export Current Classes**: `kubectl get vmclass -o yaml > classes.yaml`
2. **Update API Version**: Change `apiVersion` in exported YAML
3. **Validate Changes**: Review field mappings and deprecated features
4. **Test Deployment**: Deploy in development environment first

### Backward Compatibility

- **Field Deprecation**: Deprecated fields continue to work but emit warnings
- **Default Values**: New fields have sensible defaults for existing classes
- **Controller Names**: Existing classes without controllerName use defaults

## Related Resources

- [VirtualMachine](vm.md) - Learn how to deploy VMs using classes
- [VirtualMachineImage](../images/vm-image.md) - Understand VM image management
- [Storage Classes](https://kubernetes.io/docs/concepts/storage/storage-classes/) - Kubernetes storage configuration
- [vSphere ConfigSpec](https://vdc-download.vmware.com/vmwb-repository/dcr-public/184bb3ba-6fa8-4574-a767-d0c96e2a38f4/ba9422ef-405c-47dd-8553-e11b619185b2/SDK/vsphere-ws/docs/ReferenceGuide/vim.vm.ConfigSpec.html) - vSphere API reference

