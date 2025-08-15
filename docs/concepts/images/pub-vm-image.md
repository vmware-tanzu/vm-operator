# Publish Virtual Machine Image

Publishing a VirtualMachine as a new VM image enables you to capture the current state of a VM and make it available as a reusable image template. VM Operator provides the `VirtualMachinePublishRequest` API to facilitate this process, allowing you to create custom VM images from running or powered-off VirtualMachines.

## Overview

The VirtualMachinePublishRequest API enables you to:

- **Capture VM State**: Create a snapshot of a VirtualMachine's current configuration and disk state
- **Publish to Content Library**: Store the captured image in a vSphere Content Library for reuse
- **Automate Image Creation**: Integrate with tools like Packer for automated image building workflows
- **Version Control**: Maintain multiple versions of VM images with descriptive metadata

## How VM Publishing Works

The VM publishing process follows these steps:

1. **Request Creation**: A VirtualMachinePublishRequest is created specifying the source VM and target location
2. **Validation**: The controller validates that the source VM exists and the target Content Library is accessible
3. **VM Capture**: The VM is captured as an OVF template using vSphere APIs
4. **Upload**: The captured template is uploaded to the specified Content Library
5. **Image Creation**: A new VirtualMachineImage resource is created referencing the published template
6. **Completion**: The request is marked as complete and optionally cleaned up based on TTL settings

## VirtualMachinePublishRequest API

### Basic Structure

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachinePublishRequest
metadata:
  name: my-vm-publish-request
  namespace: my-namespace
spec:
  source:
    name: source-vm-name
    apiVersion: vmoperator.vmware.com/v1alpha5
    kind: VirtualMachine
  target:
    item:
      name: my-custom-image
      description: "Custom VM image created from my-vm"
    location:
      name: my-content-library
      apiVersion: imageregistry.vmware.com/v1alpha1
      kind: ContentLibrary
  ttlSecondsAfterFinished: 3600
```

### Field Reference

#### Source Configuration

- **`source.name`**: Name of the VirtualMachine to publish (defaults to the VirtualMachinePublishRequest name)
- **`source.apiVersion`**: API version of the source object (default: `vmoperator.vmware.com/v1alpha5`)
- **`source.kind`**: Kind of the source object (default: `VirtualMachine`)

#### Target Configuration

- **`target.item.name`**: Display name for the published image (defaults to `{source.name}-image`)
- **`target.item.description`**: Optional description for the published image
- **`target.location.name`**: Name of the target ContentLibrary resource
- **`target.location.apiVersion`**: API version of the target (default: `imageregistry.vmware.com/v1alpha1`)
- **`target.location.kind`**: Kind of the target (default: `ContentLibrary`)

#### TTL Configuration

- **`ttlSecondsAfterFinished`**: Time-to-live for the request after completion (optional)

## Examples

### Basic VM Publishing

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachinePublishRequest
metadata:
  name: ubuntu-golden-image
  namespace: default
spec:
  source:
    name: ubuntu-vm-template
  target:
    item:
      name: ubuntu-22.04-golden
      description: "Ubuntu 22.04 golden image with security patches"
    location:
      name: default-content-library
```

### Publishing with Automatic Cleanup

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachinePublishRequest
metadata:
  name: temp-build-publish
  namespace: ci-cd
spec:
  source:
    name: build-vm
  target:
    item:
      name: app-server-v2.1.0
      description: "Application server image v2.1.0"
    location:
      name: production-images
  ttlSecondsAfterFinished: 300  # Clean up after 5 minutes
```

### Minimal Configuration (Using Defaults)

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachinePublishRequest
metadata:
  name: web-server
  namespace: default
# Uses defaults:
# - source.name: web-server (same as metadata.name)
# - target.item.name: web-server-image
# - target.location: ContentLibrary with label "imageregistry.vmware.com/default"
```

## Integration with Packer

VM Operator integrates seamlessly with [HashiCorp Packer's vSphere Supervisor builder](https://developer.hashicorp.com/packer/integrations/hashicorp/vsphere/latest/components/builder/vsphere-supervisor), enabling automated image building workflows.

### Packer Configuration Example

```hcl
source "vsphere-supervisor" "ubuntu-image" {
  # Source image configuration
  image_name = "ubuntu-22.04-base"
  class_name = "best-effort-large"
  storage_class = "wcplocal-storage-profile"
  
  # Bootstrap configuration for guest customization
  bootstrap_provider = "CloudInit"
  bootstrap_data_file = "cloud-init-config.yaml"
  
  # Publish configuration
  publish_location_name = "golden-images-library"
}

build {
  sources = ["source.vsphere-supervisor.ubuntu-image"]
  
  # Provisioning steps
  provisioner "shell" {
    scripts = [
      "scripts/install-security-updates.sh",
      "scripts/install-monitoring-agent.sh",
      "scripts/cleanup.sh"
    ]
  }
  
  # Post-processing
  post-processor "manifest" {
    output = "manifest.json"
  }
}
```

### Bootstrap Provider Support

The Packer vSphere Supervisor builder supports all VM Operator bootstrap providers:

#### CloudInit Bootstrap
```hcl
source "vsphere-supervisor" "linux-vm" {
  bootstrap_provider = "CloudInit"
  bootstrap_data_file = "cloud-init.yaml"
  # ... other configuration
}
```

#### Sysprep Bootstrap (Windows)
```hcl
source "vsphere-supervisor" "windows-vm" {
  bootstrap_provider = "Sysprep"
  bootstrap_data_file = "sysprep.xml"
  # ... other configuration
}
```

#### vAppConfig Bootstrap
```hcl
source "vsphere-supervisor" "appliance-vm" {
  bootstrap_provider = "vAppConfig"
  bootstrap_data_file = "vapp-properties.json"
  # ... other configuration
}
```

## Status and Monitoring

### Checking Publication Status

```bash
# Check the status of a publish request
kubectl get vmpub ubuntu-golden-image -o yaml

# Watch the progress
kubectl get vmpub ubuntu-golden-image -w
```

### Status Conditions

The VirtualMachinePublishRequest reports its progress through status conditions:

- **`SourceValid`**: Source VM exists and is accessible
- **`TargetValid`**: Target Content Library exists and is accessible
- **`Uploaded`**: VM has been successfully uploaded to the Content Library
- **`ImageAvailable`**: VirtualMachineImage resource has been created
- **`Complete`**: All operations completed successfully

### Example Status

```yaml
status:
  ready: true
  imageName: ubuntu-22.04-golden
  startTime: "2024-01-15T10:00:00Z"
  completionTime: "2024-01-15T10:15:00Z"
  attempts: 1
  conditions:
  - type: SourceValid
    status: "True"
    lastTransitionTime: "2024-01-15T10:00:30Z"
  - type: TargetValid
    status: "True"
    lastTransitionTime: "2024-01-15T10:01:00Z"
  - type: Uploaded
    status: "True"
    lastTransitionTime: "2024-01-15T10:12:00Z"
  - type: ImageAvailable
    status: "True"
    lastTransitionTime: "2024-01-15T10:14:00Z"
  - type: Complete
    status: "True"
    lastTransitionTime: "2024-01-15T10:15:00Z"
```

## Best Practices

### VM Preparation

1. **Power Off VMs**: While publishing powered-on VMs is supported, powering off ensures consistency
2. **Clean Up Temporary Files**: Remove logs, temporary files, and sensitive data before publishing
3. **Generalize the Image**: Remove machine-specific configurations (hostnames, SSH keys, etc.)
4. **Install VMware Tools**: Ensure VMware Tools are installed for optimal performance

### Content Library Management

1. **Use Descriptive Names**: Choose meaningful names for published images
2. **Add Metadata**: Include comprehensive descriptions and version information
3. **Organize by Purpose**: Use separate Content Libraries for different image types (base images, application images, etc.)
4. **Regular Cleanup**: Remove unused images to manage storage consumption

### Automation Considerations

1. **Use TTL**: Set appropriate `ttlSecondsAfterFinished` values for automated workflows
2. **Monitor Progress**: Implement proper status checking in CI/CD pipelines
3. **Handle Failures**: Plan for retry logic and error handling
4. **Version Control**: Maintain consistent naming conventions for image versions

## Troubleshooting

### Common Issues

#### Source VM Not Found
```yaml
conditions:
- type: SourceValid
  status: "False"
  reason: "SourceNotFound"
  message: "VirtualMachine 'missing-vm' not found in namespace 'default'"
```

**Solution**: Verify the source VM name and namespace are correct.

#### Target Content Library Unavailable
```yaml
conditions:
- type: TargetValid
  status: "False"
  reason: "TargetLocationNotFound"
  message: "ContentLibrary 'missing-library' not found"
```

**Solution**: Ensure the ContentLibrary resource exists and is accessible.

#### Upload Failures
```yaml
conditions:
- type: Uploaded
  status: "False"
  reason: "UploadFailure"
  message: "Failed to upload VM to content library: insufficient storage"
```

**Solution**: Check Content Library storage capacity and vCenter connectivity.

### Debugging Commands

```bash
# Get detailed status
kubectl describe vmpub my-publish-request

# Check related VirtualMachineImage
kubectl get vmi -l "virtualmachinepublishrequest.vmoperator.vmware.com/name=my-publish-request"

# View controller logs
kubectl logs -n vmware-system-vmop deployment/vmware-system-vmop-controller-manager
```

## Related Resources

- [VirtualMachine Images](vm-image.md) - Learn about VM image management
- [Content Libraries](https://docs.vmware.com/en/VMware-vSphere/8.0/vsphere-vm-administration/GUID-254B2CE8-20A8-43F0-90E8-3F6776C2C896.html) - vSphere Content Library documentation
- [Packer vSphere Supervisor Builder](https://developer.hashicorp.com/packer/integrations/hashicorp/vsphere/latest/components/builder/vsphere-supervisor) - Packer integration guide
