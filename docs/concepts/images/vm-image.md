# VirtualMachineImage

`VirtualMachineImage` and `ClusterVirtualMachineImage` resources represent VM templates that serve as the foundation for deploying `VirtualMachine` workloads. These resources provide metadata about available VM images, including operating system information, hardware requirements, and capabilities.

## Overview

VM Operator provides two types of image resources to accommodate different scoping requirements:

- **`VirtualMachineImage`**: Namespace-scoped images available only within a specific namespace
- **`ClusterVirtualMachineImage`**: Cluster-scoped images available across all namespaces

Both resource types share the same specification and status structure, differing only in their scope. This design allows for flexible image management, where organizations can provide cluster-wide base images while allowing individual teams to manage their own namespace-specific images.

## Image Sources and Synchronization

VM images are typically sourced from vSphere Content Libraries, which serve as centralized repositories for VM templates, OVA files, and ISO images. VM Operator automatically discovers and synchronizes images from configured Content Libraries, creating corresponding Kubernetes resources with rich metadata.

### Content Library Integration

When VM Operator discovers an image in a Content Library, it:

1. **Extracts Metadata**: Parses OVF properties, hardware specifications, and OS information
2. **Creates Resources**: Generates VirtualMachineImage or ClusterVirtualMachineImage resources
3. **Synchronizes Updates**: Monitors for changes and updates resource status accordingly
4. **Manages Lifecycle**: Handles creation, updates, and cleanup of image resources

## Image Identification and Naming

### VMI ID Generation

VM Operator uses a deterministic naming system to ensure unique, predictable identifiers for all images. Prior to vSphere 8.0U2, image names were derived directly from Content Library item names, which could cause conflicts when multiple libraries contained items with identical names.

With vSphere 8.0U2 and the introduction of the Image Registry API, VM Operator generates unique **VMI IDs** (Virtual Machine Image IDs) using the following format:

```
vmi-0123456789ABCDEFG
```

#### VMI ID Construction Process

For Content Library-sourced images, the VMI ID is constructed as follows:

1. **Remove Hyphens**: Remove any `-` characters from the Content Library item's UUID
2. **Calculate SHA-1**: Calculate the SHA-1 hash of the resulting string
3. **Extract Prefix**: Take the first 17 characters of the hash
4. **Create VMI ID**: Prepend `vmi-` to create the final identifier

**Example:**
```
Content Library UUID: e1968c25-dd84-4506-8dc7-9beacb6b688e
Step 1: e1968c25dd8445068dc79beacb6b688e
Step 2: 0a0044d7c690bcbea07c9b49efc9f743479490e5
Step 3: 0a0044d7c690bcbea
Step 4: vmi-0a0044d7c690bcbea
```

### Display Name Resolution

In addition to VMI IDs, VM Operator supports image resolution using human-readable display names. When a VirtualMachine specifies an image using a display name, VM Operator resolves it according to these rules:

1. **Namespace Precedence**: Check for VirtualMachineImage resources in the same namespace
2. **Cluster Fallback**: If no namespace-scoped image is found, check ClusterVirtualMachineImage resources
3. **Uniqueness Requirement**: The display name must unambiguously resolve to a single image
4. **Mutation**: If resolution succeeds, a mutation webhook replaces the display name with the VMI ID

**Example Resolution:**
```yaml
# Original specification
spec:
  imageName: photonos-5-x64

# After mutation webhook processing
spec:
  imageName: vmi-0a0044d7c690bcbea
```

!!! warning "Display Name Resolution Availability"
    Display name resolution was introduced in [PR #214](https://github.com/vmware-tanzu/vm-operator/issues/214) but may not be available in all vSphere releases. Check your vSphere version for feature availability.

## API Reference

### VirtualMachineImageSpec

The specification for both VirtualMachineImage and ClusterVirtualMachineImage resources is minimal, primarily containing a reference to the source provider:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachineImage
metadata:
  name: vmi-0a0044d7c690bcbea
  namespace: default
spec:
  providerRef:
    apiVersion: imageregistry.vmware.com/v1alpha1
    kind: ContentLibraryItem
    name: photon-os-item
```

#### Fields

- **`providerRef`** (optional): Reference to the resource containing the source of the image's information
  - **`apiVersion`**: API version of the referenced object
  - **`kind`**: Kind of the referenced object (typically `ContentLibraryItem`)
  - **`name`**: Name of the referenced object

### VirtualMachineImageStatus

The status section contains comprehensive metadata about the image, populated by VM Operator controllers:

```yaml
status:
  name: "Photon OS 5.0"
  type: "OVF"
  firmware: "EFI"
  hardwareVersion: 19
  capabilities:
    - "cloud-init"
    - "nvidia-gpu"
  osInfo:
    id: "photon64Guest"
    type: "Linux"
    version: "5.0"
  productInfo:
    product: "Photon OS"
    vendor: "VMware"
    version: "5.0"
    fullVersion: "5.0.0-20230629"
  disks:
    - capacity: "20Gi"
      size: "2Gi"
  ovfProperties:
    - key: "hostname"
      type: "string"
      default: "photon-vm"
  providerItemID: "e1968c25-dd84-4506-8dc7-9beacb6b688e"
  providerContentVersion: "1"
  conditions:
    - type: "Ready"
      status: "True"
      lastTransitionTime: "2024-01-15T10:00:00Z"
```

#### Status Fields

- **`name`**: Display name of the image
- **`type`**: Content type (typically "OVF" or "ISO")
- **`firmware`**: Firmware type ("BIOS" or "EFI")
- **`hardwareVersion`**: VMware virtual hardware version
- **`capabilities`**: List of image capabilities (e.g., "cloud-init", "nvidia-gpu")
- **`osInfo`**: Operating system information
  - **`id`**: vSphere guest OS identifier
  - **`type`**: OS type (e.g., "Linux", "Windows")
  - **`version`**: OS version
- **`productInfo`**: Product metadata
  - **`product`**: General product descriptor
  - **`vendor`**: Organization that produced the image
  - **`version`**: Short-form version
  - **`fullVersion`**: Long-form version
- **`disks`**: Disk information
  - **`capacity`**: Virtual disk capacity
  - **`size`**: Estimated populated size
- **`ovfProperties`**: User-configurable OVF properties
- **`vmwareSystemProperties`**: VMware-specific system properties
- **`providerItemID`**: Provider-specific item identifier
- **`providerContentVersion`**: Content version from provider
- **`conditions`**: Resource conditions

## Usage Examples

### Listing Available Images

```bash
# List namespace-scoped images
kubectl get vmi

# List cluster-scoped images  
kubectl get cvmi

# Get detailed information about a specific image
kubectl describe vmi vmi-0a0044d7c690bcbea
```

**Example Output:**
```
NAME                    DISPLAY NAME        TYPE   IMAGE VERSION   OS NAME   OS VERSION   HARDWARE VERSION
vmi-0a0044d7c690bcbea  Photon OS 5.0       OVF    5.0            Linux     5.0          19
vmi-1234567890abcdef   Ubuntu 22.04 LTS    OVF    22.04          Linux     22.04        19
```

### Using Images in VirtualMachine Resources

#### Method 1: Using VMI ID (Recommended)

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachine
metadata:
  name: my-vm
  namespace: default
spec:
  image:
    kind: VirtualMachineImage
    name: vmi-0a0044d7c690bcbea
  className: best-effort-medium
  # ... other configuration
```

#### Method 2: Using Display Name

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachine
metadata:
  name: my-vm
  namespace: default
spec:
  imageName: "Photon OS 5.0"  # Will be resolved to VMI ID
  className: best-effort-medium
  # ... other configuration
```

#### Method 3: Using ClusterVirtualMachineImage

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachine
metadata:
  name: my-vm
  namespace: default
spec:
  image:
    kind: ClusterVirtualMachineImage
    name: cvmi-fedcba0987654321
  className: best-effort-medium
  # ... other configuration
```

### Image Capabilities and Selection

Images may expose capabilities that indicate support for specific features:

```bash
# Find images with cloud-init support
kubectl get vmi -l capability.image.vmoperator.vmware.com/cloud-init

# Find images with GPU support
kubectl get vmi -l capability.image.vmoperator.vmware.com/nvidia-gpu

# Find Linux images
kubectl get vmi -l os.image.vmoperator.vmware.com/type=Linux
```

## Image Management

### Image Lifecycle

1. **Discovery**: VM Operator discovers images from configured Content Libraries
2. **Resource Creation**: VirtualMachineImage or ClusterVirtualMachineImage resources are created
3. **Metadata Population**: Image metadata is extracted and populated in the status
4. **Availability**: Images become available for VirtualMachine deployment
5. **Updates**: Changes in source Content Libraries trigger resource updates
6. **Cleanup**: Resources are removed when source images are deleted

### Content Library Configuration

Images are automatically synchronized from Content Libraries configured in the VM Operator deployment. The synchronization process:

- **Monitors** Content Libraries for new, updated, or deleted items
- **Creates** corresponding Kubernetes resources with appropriate scope
- **Updates** metadata when Content Library items change
- **Removes** resources when Content Library items are deleted

### Image Metadata and Labels

VM Operator automatically adds labels to image resources based on their metadata:

```yaml
metadata:
  labels:
    os.image.vmoperator.vmware.com/type: "Linux"
    os.image.vmoperator.vmware.com/version: "5.0"
    capability.image.vmoperator.vmware.com/cloud-init: ""
    capability.image.vmoperator.vmware.com/nvidia-gpu: ""
```

These labels enable powerful selection and filtering capabilities for automated image management.

## Best Practices

### Image Selection

1. **Use VMI IDs**: Prefer VMI IDs over display names for predictable, immutable references
2. **Leverage Capabilities**: Use capability labels to select images with required features
3. **Consider Scope**: Use ClusterVirtualMachineImage for shared base images, VirtualMachineImage for team-specific images
4. **Monitor Versions**: Track image versions and plan upgrades accordingly

### Content Library Organization

1. **Separate Concerns**: Use different Content Libraries for different image types (base OS, applications, etc.)
2. **Version Management**: Maintain clear versioning schemes for image updates
3. **Access Control**: Configure appropriate permissions for Content Library access
4. **Regular Cleanup**: Remove unused images to manage storage consumption

### Automation and CI/CD

1. **Image Validation**: Verify image availability before deploying VirtualMachines
2. **Capability Checking**: Ensure required capabilities are available in selected images
3. **Version Pinning**: Use specific VMI IDs in production deployments
4. **Update Strategies**: Plan for image updates and VM migration strategies

## Troubleshooting

### Common Issues

#### Image Not Found
```bash
# Check if image exists
kubectl get vmi,cvmi | grep <image-name>

# Verify Content Library synchronization
kubectl describe vmi <image-name>
```

#### Display Name Resolution Fails
```bash
# Check for naming conflicts
kubectl get vmi,cvmi -o custom-columns=NAME:.metadata.name,DISPLAY:.status.name

# Verify mutation webhook is functioning
kubectl get mutatingwebhookconfiguration
```

#### Missing Capabilities
```bash
# Check image capabilities
kubectl get vmi <image-name> -o jsonpath='{.status.capabilities}'

# Verify capability labels
kubectl get vmi <image-name> --show-labels
```

### Debugging Commands

```bash
# Get detailed image information
kubectl describe vmi <image-name>

# Check image conditions
kubectl get vmi <image-name> -o jsonpath='{.status.conditions}'

# View image controller logs
kubectl logs -n vmware-system-vmop deployment/vmware-system-vmop-controller-manager

# List all images with metadata
kubectl get vmi,cvmi -o wide
```

## Recommended Images

VM Operator supports any OVF-compatible image, but here are some commonly used images for testing and development:

| Image | Architecture | Type | Download |
|-------|:------------:|:----:|:--------:|
| Photon OS 4, Rev 2 | amd64 | OVA | [Download](https://packages.vmware.com/photon/4.0/Rev2/ova/photon-ova_uefi-4.0-c001795b80.ova) |
| Photon OS 3, Rev 3, Update 1 | amd64 | OVA | [Download](https://packages.vmware.com/photon/3.0/Rev3/ova/Update1/photon-hw13_uefi-3.0-913b49438.ova) |
| Ubuntu 22.10 Server | amd64 | OVA | [Download](https://cloud-images.ubuntu.com/releases/22.10/release-20230716/ubuntu-22.10-server-cloudimg-amd64.ova) |
| Ubuntu 22.04 Server LTS | amd64 | OVA | [Download](https://cloud-images.ubuntu.com/releases/22.04/release-20230302/ubuntu-22.04-server-cloudimg-amd64.ova) |

!!! note "Bring Your Own Images"
    The above list is not exhaustive or restrictive. VM Operator encourages users to bring their own custom images that meet their specific requirements.

## Related Resources

- [Publishing VM Images](pub-vm-image.md) - Learn how to create custom images from existing VMs
- [VirtualMachine](../workloads/vm.md) - Learn how to deploy VMs using images
- [Content Libraries](https://docs.vmware.com/en/VMware-vSphere/8.0/vsphere-vm-administration/GUID-254B2CE8-20A8-43F0-90E8-3F6776C2C896.html) - vSphere Content Library documentation