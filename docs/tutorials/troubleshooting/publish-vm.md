# Publish Virtual Machine

This page provides comprehensive troubleshooting guidance for VM publishing issues. VM publishing involves capturing a VirtualMachine's state and creating a reusable VirtualMachineImage in a Content Library. Refer to the [Publish VM Image documentation](../../concepts/images/pub-vm-image.md) for detailed information about the publishing process.

## Overview

The VirtualMachinePublishRequest workflow involves several stages, each with potential failure points:

1. **Source Validation**: Ensuring the source VM exists and is ready
2. **Target Validation**: Verifying the target Content Library is accessible and writable
3. **VM Capture**: Creating an OVF template from the VM
4. **Upload**: Transferring the template to the Content Library
5. **Image Creation**: Generating a VirtualMachineImage resource
6. **Completion**: Finalizing the publish request

## Best Practices

Before troubleshooting, ensure you follow these best practices:

- **Source VM State**: Ensure the source VM is in a stable state (powered off or consistent running state)
- **Content Library Access**: Verify the target Content Library exists and is writable
- **Storage Space**: Ensure sufficient storage space in the Content Library
- **Permissions**: Confirm appropriate RBAC permissions for VirtualMachinePublishRequest resources
- **Network Connectivity**: Ensure stable connectivity to vCenter during the publish process

```console
# Verify source VM exists and is ready
$ kubectl get vm <vm-name> -n <namespace>

# Check Content Library availability
$ kubectl get contentlibrary -n <namespace>

# Verify publish request permissions
$ kubectl auth can-i create virtualmachinepublishrequest -n <namespace>
```

## Troubleshooting Steps

### 1. Check VirtualMachinePublishRequest Status

Start by examining the publish request status and conditions:

```console
$ kubectl describe vmpub <publish-request-name> -n <namespace>
```

Look for the `Status.Conditions` section which shows the current state:

```yaml
Status:
  Conditions:
  - Type: SourceValid
    Status: "True"
    Reason: ""
    Message: ""
  - Type: TargetValid
    Status: "False"
    Reason: "TargetContentLibraryNotExist"
    Message: "ContentLibrary 'my-library' not found"
  - Type: Uploaded
    Status: "False"
    Reason: "HasNotBeenUploaded"
    Message: "item hasn't been uploaded yet"
  - Type: ImageAvailable
    Status: "False"
    Reason: "ImageUnavailable"
    Message: "VirtualMachineImage is not available"
  - Type: Complete
    Status: "False"
    Reason: "HasNotBeenUploaded"
    Message: "item hasn't been uploaded yet"
```

### 2. Analyze Status Fields

Check these important status fields:

```console
# View detailed status
$ kubectl get vmpub <publish-request-name> -n <namespace> -o yaml
```

Key fields to examine:
- `status.ready`: Overall completion status
- `status.attempts`: Number of publish attempts
- `status.startTime`: When the request was initiated
- `status.lastAttemptTime`: Most recent attempt timestamp
- `status.imageName`: Name of the created VirtualMachineImage (when successful)

## Common Issues and Solutions

### Source Validation Issues

#### Source VM Not Found

**Condition:**
```yaml
- Type: SourceValid
  Status: "False"
  Reason: "SourceVirtualMachineNotExist"
  Message: "VirtualMachine 'missing-vm' not found"
```

**Causes and Solutions:**
- **VM name mismatch**: Verify the VM name in the publish request spec
- **Wrong namespace**: Ensure the VM exists in the same namespace as the publish request
- **VM was deleted**: Check if the source VM still exists

```console
# Verify VM exists
$ kubectl get vm <vm-name> -n <namespace>

# Check publish request source reference
$ kubectl get vmpub <publish-request-name> -n <namespace> -o jsonpath='{.status.sourceRef.name}'
```

#### Source VM Not Created

**Condition:**
```yaml
- Type: SourceValid
  Status: "False"
  Reason: "SourceVirtualMachineNotCreated"
  Message: "VM hasn't been created and has no uniqueID"
```

**Causes and Solutions:**
- **VM still deploying**: Wait for the VM to complete deployment
- **VM deployment failed**: Check the VM's status and resolve deployment issues
- **VM stuck in pending**: Investigate VM deployment problems

```console
# Check VM deployment status
$ kubectl describe vm <vm-name> -n <namespace>

# Wait for VM to be ready
$ kubectl wait --for=condition=Ready vm/<vm-name> -n <namespace> --timeout=300s
```

### Target Validation Issues

#### Content Library Not Found

**Condition:**
```yaml
- Type: TargetValid
  Status: "False"
  Reason: "TargetContentLibraryNotExist"
  Message: "ContentLibrary 'missing-library' not found"
```

**Causes and Solutions:**
- **Library name mismatch**: Verify the Content Library name in the publish request
- **Library not imported**: Ensure the Content Library is properly imported into the namespace
- **Library deleted**: Check if the Content Library resource still exists

```console
# List available Content Libraries
$ kubectl get contentlibrary -n <namespace>

# Check publish request target reference
$ kubectl get vmpub <publish-request-name> -n <namespace> -o jsonpath='{.spec.target.location.name}'
```

#### Content Library Not Writable

**Condition:**
```yaml
- Type: TargetValid
  Status: "False"
  Reason: "TargetContentLibraryNotWritable"
  Message: "target location 'my-library' is not writable"
```

**Causes and Solutions:**
- **Read-only library**: The Content Library is configured as read-only
- **Insufficient permissions**: The service account lacks write permissions to the library
- **Library configuration**: Check the Content Library configuration in vCenter

```console
# Check Content Library writability
$ kubectl get contentlibrary <library-name> -n <namespace> -o jsonpath='{.spec.writable}'

# Verify Content Library status
$ kubectl describe contentlibrary <library-name> -n <namespace>
```

#### Content Library Not Ready

**Condition:**
```yaml
- Type: TargetValid
  Status: "False"
  Reason: "TargetContentLibraryNotReady"
  Message: "target location 'my-library' is not ready"
```

**Causes and Solutions:**
- **Library synchronizing**: Wait for Content Library synchronization to complete
- **Network issues**: Check connectivity between vCenter and storage
- **Storage problems**: Verify storage backend is accessible

```console
# Monitor Content Library readiness
$ kubectl get contentlibrary <library-name> -n <namespace> -w

# Check Content Library conditions
$ kubectl describe contentlibrary <library-name> -n <namespace>
```

#### Target Item Already Exists

**Condition:**
```yaml
- Type: TargetValid
  Status: "False"
  Reason: "TargetItemAlreadyExists"
  Message: "item with name 'my-image' already exists in content library"
```

**Causes and Solutions:**
- **Name collision**: Choose a different target item name
- **Previous publish**: Remove or rename the existing item
- **Stale resources**: Clean up incomplete previous publish attempts

```console
# Check existing items in Content Library (via vCenter)
# Or use a different target item name
$ kubectl patch vmpub <publish-request-name> -n <namespace> --type='merge' -p='{"spec":{"target":{"item":{"name":"my-image-v2"}}}}'
```

### Upload Issues

#### Upload Task Not Started

**Condition:**
```yaml
- Type: Uploaded
  Status: "False"
  Reason: "NotStarted"
  Message: "VM Publish task hasn't started"
```

**Causes and Solutions:**
- **vCenter connectivity**: Check connection to vCenter
- **Task queue backlog**: Wait for vCenter task queue to process
- **Resource constraints**: Verify vCenter has sufficient resources

```console
# Check VM Operator controller logs
$ kubectl logs -n vmware-system-vmop deployment/vmware-system-vmop-controller-manager

# Monitor publish request attempts
$ kubectl get vmpub <publish-request-name> -n <namespace> -o jsonpath='{.status.attempts}'
```

#### Upload Task Queued

**Condition:**
```yaml
- Type: Uploaded
  Status: "False"
  Reason: "Queued"
  Message: "VM Publish task is queued"
```

**Causes and Solutions:**
- **High vCenter load**: Wait for the task to start processing
- **Resource limits**: Check vCenter task limits and resource usage
- **Task priority**: Monitor vCenter task manager for progress

This is typically a temporary state - the task should progress to "Uploading" status.

#### Upload in Progress

**Condition:**
```yaml
- Type: Uploaded
  Status: "False"
  Reason: "Uploading"
  Message: "Uploading item to content library"
```

**Causes and Solutions:**
- **Normal progress**: This indicates the upload is proceeding normally
- **Slow progress**: Large VMs may take significant time to upload
- **Monitor progress**: Check vCenter task manager for detailed progress

```console
# Monitor upload progress
$ kubectl get vmpub <publish-request-name> -n <namespace> -w

# Check last attempt time for stalled uploads
$ kubectl get vmpub <publish-request-name> -n <namespace> -o jsonpath='{.status.lastAttemptTime}'
```

#### Upload Failure

**Condition:**
```yaml
- Type: Uploaded
  Status: "False"
  Reason: "UploadFailure"
  Message: "failed to publish source VM: insufficient storage"
```

**Causes and Solutions:**
- **Storage space**: Ensure sufficient space in the Content Library datastore
- **Network issues**: Check network connectivity during upload
- **VM state changes**: Avoid modifying the source VM during publish
- **Timeout issues**: Large VMs may exceed timeout limits

```console
# Check Content Library storage usage
$ kubectl describe contentlibrary <library-name> -n <namespace>

# Review controller logs for detailed error information
$ kubectl logs -n vmware-system-vmop deployment/vmware-system-vmop-controller-manager | grep -i publish
```

#### Invalid Item ID

**Condition:**
```yaml
- Type: Uploaded
  Status: "False"
  Reason: "ItemIDInvalid"
  Message: "VM publish task result returns an invalid Item id"
```

**Causes and Solutions:**
- **vCenter API issues**: Temporary vCenter API problems
- **Task result corruption**: Retry the publish operation
- **Version compatibility**: Check vCenter and VM Operator version compatibility

### Image Availability Issues

#### VirtualMachineImage Not Found

**Condition:**
```yaml
- Type: ImageAvailable
  Status: "False"
  Reason: "VirtualMachineImageNotFound"
  Message: "VirtualMachineImage not found"
```

**Causes and Solutions:**
- **Image sync delay**: Wait for the VirtualMachineImage resource to be created
- **Controller issues**: Check VM Operator controller health
- **Content Library sync**: Verify Content Library synchronization

```console
# Check for VirtualMachineImage creation
$ kubectl get vmi -n <namespace> --watch

# Look for images with matching provider item ID
$ kubectl get vmi -n <namespace> -o jsonpath='{range .items[*]}{.metadata.name}{": "}{.status.providerItemID}{"\n"}{end}'
```

### Completion Issues

#### Has Not Been Uploaded

**Condition:**
```yaml
- Type: Complete
  Status: "False"
  Reason: "HasNotBeenUploaded"
  Message: "item hasn't been uploaded yet"
```

This indicates the upload phase hasn't completed successfully. Refer to the Upload Issues section above.

#### Image Unavailable

**Condition:**
```yaml
- Type: Complete
  Status: "False"
  Reason: "ImageUnavailable"
  Message: "VirtualMachineImage is not available"
```

This indicates the VirtualMachineImage resource hasn't been created yet. Refer to the Image Availability Issues section above.

## Advanced Troubleshooting

### Checking vCenter Tasks

For detailed task information, check the vCenter task manager:

1. Log into vCenter UI
2. Navigate to Menu > Tasks & Events > Tasks
3. Filter by task type: "Capture virtual machine to library item"
4. Look for tasks with activation IDs matching your publish request

### Controller Logs Analysis

Monitor VM Operator controller logs for detailed error information:

```console
# Follow controller logs
$ kubectl logs -n vmware-system-vmop deployment/vmware-system-vmop-controller-manager -f

# Search for specific publish request logs
$ kubectl logs -n vmware-system-vmop deployment/vmware-system-vmop-controller-manager | grep <publish-request-name>

# Look for publish-related errors
$ kubectl logs -n vmware-system-vmop deployment/vmware-system-vmop-controller-manager | grep -i "publish\|upload\|content.library"
```

### Resource Cleanup

If a publish request gets stuck, you may need to clean up resources:

```console
# Delete the stuck publish request
$ kubectl delete vmpub <publish-request-name> -n <namespace>

# Check for orphaned VirtualMachineImage resources
$ kubectl get vmi -n <namespace>

# Clean up Content Library items (via vCenter if necessary)
```

### Retry Strategies

For transient failures, the controller automatically retries. You can also:

1. **Delete and recreate**: Remove the failed request and create a new one
2. **Modify the request**: Change target item name to avoid conflicts
3. **Wait and monitor**: Some issues resolve themselves with time

## Performance Considerations

### Large VM Publishing

For large VMs:
- **Expect longer times**: Multi-GB VMs can take hours to publish
- **Monitor storage**: Ensure adequate space throughout the process
- **Network stability**: Maintain stable network connections
- **Resource planning**: Schedule during low-usage periods

### Concurrent Publishing

When publishing multiple VMs:
- **Limit concurrency**: Avoid overwhelming vCenter with simultaneous requests
- **Stagger requests**: Space out publish requests to reduce resource contention
- **Monitor resources**: Watch vCenter CPU, memory, and storage usage

## Prevention Strategies

### Pre-publish Validation

Before creating a publish request:

```console
# Verify source VM is ready
$ kubectl get vm <vm-name> -n <namespace> -o jsonpath='{.status.ready}'

# Check Content Library status
$ kubectl get contentlibrary <library-name> -n <namespace> -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}'

# Validate target item name availability
$ kubectl get vmi -n <namespace> | grep <target-item-name>
```

### Monitoring and Alerting

Set up monitoring for:
- Publish request duration and success rates
- Content Library storage usage
- vCenter task queue length
- VM Operator controller health

### Regular Maintenance

- **Clean up**: Remove completed publish requests with TTL settings
- **Monitor storage**: Keep Content Library storage below 80% capacity
- **Update images**: Regularly refresh base images to include security updates
- **Test workflows**: Periodically test the publish process with sample VMs

## Related Resources

- [Publish VM Image Concepts](../../concepts/images/pub-vm-image.md)
- [VirtualMachineImage Documentation](../../concepts/images/vm-image.md)
- [Content Library Management](../../concepts/images/README.md)
- [Deploy VM Tutorial](../deploy-vm/README.md)
