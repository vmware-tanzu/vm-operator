# Deploy VM
This page is dedicated to diagnose issues that can arise during the deployment of virtual machines. Refer to [Deploy VM tutorial](https://vm-operator.readthedocs.io/en/stable/tutorials/deploy-vm/) on how to deploy a virtual machine.

## Best practice
- Create a namespace
- Make sure the VM image is available in the namespace.
```console
$ kubectl get virtualmachineimage -n <namespace-name>
$ kubectl get contentlibrary -n <namespace-name>
```
- Make sure storage class is specified in the namespace
```console
$ kubectl get storageclass -n <namespace-name>
```
- Make sure VM Class is attached to the namespace
```console
$ kubectl get vmclass -n <namespace-name>
```
- Make sure appropriate transport is chosen
- Deploy VM

## Troubleshoot step

### 1. Access your namespace in the Kubernetes environment.

```console
$ kubectl config use-context <context-name>
```

See [Get and Use the Supervisor Context](https://docs.vmware.com/en/VMware-vSphere/8.0/vsphere-with-tanzu-services-workloads/GUID-63A1C273-DC75-420B-B7FD-47CB25A50A2C.html#GUID-63A1C273-DC75-420B-B7FD-47CB25A50A2C) if you need help accessing Supervisor clusters.

### 2. Describe Virtual Machine k8s resource
Run `kubectl describe vm <vm-name> -n <namespace-name>` command. Check `VM.Status.Conditions` as seen in sample output below.

```console
$ kubectl describe vm <vm-name> -n <namespace-name>

...
Status:
  Conditions:
    Last Transition Time:  2022-07-06T00:43:47Z
    Status:                Unknown
    Type:                  GuestCustomization
    Last Transition Time:  2022-07-06T00:42:41Z
    Status:                True
    Type:                  VirtualMachinePrereqReady
    Last Transition Time:  2022-07-06T00:43:47Z
    Message:               VMware Tools is not running
    Reason:                VirtualMachineToolsNotRunning
    Severity:              Error
    Status:                False
    Type:                  VirtualMachineTools
...
```

## Error and fix
This section focuses on leveraging the information provided by the `VM.Status.Conditions` field to diagnose the root cause of errors and provide solutions. By analyzing which conditions showed `Status` as `False`/`Unknown` and `Reason`, you can troubleshoot issues depending on below scenarios.

### VirtualMachinePrereqReady
- Reason `VirtualMachineClassBindingNotFound`(*deprecated*)/`VirtualMachineClassNotFound`: this means that the VM class has not been associated to the Supervisor namespace.
    - **Fix**: Attach VM Class to your namespace. Refer to [VMClass](https://vm-operator.readthedocs.io/en/stable/concepts/workloads/vm-class/) for details.
- Reason `VirtualMachineImageNotFound`: this means that the VirtualMachine Image doesn't exist in the supervisor cluster, which means that this VM image doesn't exist in the vCenter, or the content library which it belongs to has not been associated to any of the supervisor namespaces. 
    - **Fix**: Upload mentioned VM Image to content library and associate content library to your namespace. Refer to [VM Image](https://vm-operator.readthedocs.io/en/stable/concepts/images/) for details.
- Reason `ContentSourceBindingNotFound` (*deprecated*): this means that the content library which contains the specified VM Image has not been associated to the Supervisor namespace. 
    - **Fix**: Associate content library to your namespace. Refer to [VM Image](https://vm-operator.readthedocs.io/en/stable/concepts/images/) for details.

### GuestCustomization/VirtualMachineTools
- Reason `VirtualMachineToolsNotRunning`:
This could be happening when VM tools is not installed on the image at use.
    - **Fix**: Make sure image has VM tools installed. Then [get a console session](https://vm-operator.readthedocs.io/en/stable/tutorials/troubleshooting/get-console-session/) to debug further.
- Reason `GuestCustomizationFailed`/`VirtualMachineToolsNotRunning`:
This could be happening when using transport `OvfEnv/ExtraConfig` to deploy a VM. These two transports use VMTools for network configuration, using incompatible virtual machine images could cause race with cloud-init user-data configuration. 
    - **Fix**: [Get a console session](https://vm-operator.readthedocs.io/en/stable/tutorials/troubleshooting/get-console-session/) to debug further. And consider using transport `CloudInit` for wider VM Image support. Refer to [Deploy with CloudInit](https://vm-operator.readthedocs.io/en/stable/tutorials/deploy-vm/cloudinit/) for details.
