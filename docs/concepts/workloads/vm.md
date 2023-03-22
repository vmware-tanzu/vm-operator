# VirtualMachine

_VirtualMachines_ (VM) represent a collection of virtual hardware combined with one or more virtual disk images that can be managed using a Kubernetes cluster, such as vSphere Supervisor, and are scheduled on a VMware hypervisor, such as vSphere.

## What is a VM?

VMware [defines](https://www.vmware.com/topics/glossary/content/virtual-machine.html) a VM as:

> a compute resource that uses software instead of a physical computer to run programs and deploy apps. One or more virtual “guest” machines run on a physical “host” machine. Each virtual machine runs its own operating system and functions separately from the other VMs, even when they are all running on the same host.

The VM Operator `VirtualMachine` API enables the lifecycle-management of a VM on an underlying hypervisor. A VM resource's virtual hardware is derived from a [`VirtualMachineClass`](./vm-class.md) (VM Class) and a [`VirtualMachineImage`](../images/vm-image.md) (VM Image) supplies the VM's disk(s).

## Using VMs

The following is an example of a `VirtualMachine` resource that bootstraps a Photon OS image using [Cloud-Init](cloudinit.readthedocs.io/):

```yaml title="vm-example.yaml"
--8<-- "./docs/concepts/workloads/vm-example.yaml"
```

!!! note "Customizing a Guest"

    Please see [this section](./guest.md) for more information on the available bootstrap providers tha may be used to bootstrap a guest's host name, network, and further customize the operating system.

To create the VM shown above, run the following command (replacing `<NAMESPACE>` with the name of the [`Namespace`](https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/) in which to create the VM):

```shell
kubectl apply -n <NAMESPACE> -f {{ config.repo_url_raw }}/main/docs/concepts/workloads/vm-example.yaml
```

## Working with VMs

When a VM gets created, it is scheduled to run on a [`Node`](https://kubernetes.io/docs/concepts/architecture/nodes/) in your cluster. Regardless where VM Operator is running, the `Node` represents an underlying hypervisor. For example, on vSphere Supervisor a `Node` represents an ESXi host.

The VM remains on that `Node` until the VM resource is deleted or, if supported by the underlying platform, the VM is rescheduled on another `Node` via live-migration (ex. vMotion) due to an unexpected failure of planned maintenance resulting in a `Node`'s underlying hypervisor going offline. There may also be situations where a VM cannot be rescheduled with live-migration due to some hardware dependency. In these instances the VM is powered off and then is powered back onto a compatible `Node` if one is available. If no other `Nodes` can support the VM, its controller will continue to try powering on the VM until such time a compatible `Node` becomes available.

The name of a VM must be a valid [DNS subdomain](https://kubernetes.io/docs/concepts/overview/working-with-objects/names#dns-subdomain-names) value, but this can produce unexpected results for the VM's hostname. For best compatibility, the name should follow the more restrictive rules for a [DNS label](https://kubernetes.io/docs/concepts/overview/working-with-objects/names#dns-label-names).

### VM Image

A VM's disk(s) are supplied by a VM Image, and thus its name must be specified when creating a new VM. The following commands may be used to discover the available VM Images:

```shell title="Get available VM Images for a Namespace"
kubectl get -n <NAMESPACE> vmimage
```

```shell title="Get available VM Images for cluster"
kubectl get clustervmimage
```

For more information on VM Images, please see the documentation for [`VirtualMachineImage`](../images/vm-image.md).

### VM Class

A VM's virtual hardware is derived from a VM Class, which is why the name of a VM Class is required when creating a VM. The VM Classes available in a given namespace may be discovered with:

```shell
kubectl get -n <NAMESPACE> vmclass
```

For more information on VM Classes, please see the documentation for [`VirtualMachineClass`](./vm-class.md).

### Storage Class

A Storage Class defines a VM's storage policy and is required to create a new VM. Use the following command to discover a cluster's available storage classes:

```shell
kubectl get storageclass
```

For more information on Storage Classes, please see the documentation for [`StorageClass`]([./vm-class.md](https://kubernetes.io/docs/concepts/storage/storage-classes/)).

## Updating a VM

It is possible to update parts of an existing `VirtualMachine` resource. Some fields are completely immutable while some _can_ be modified depending on the VM's power state and whether or not the field has already been set to a non-empty value. The following table highlights what fields may or may not be updated and under what conditions:

| Update | Description | While Powered On | While Powered Off | Not Already Set |
|--------|-------------|:----------------:|:-----------------:|:---------------:|
| `spec.imageName` | The name of the `VirtualMachineImage` that supplies the VM's disk(s) | ✗ | ✗ | _NA_ |
| `spec.className` | The name of the `VirtualMachineClass` that supplies the VM's virtual hardware | ✗ | ✗ | _NA_ |
| `spec.powerState` | The VM's desired power state | ✓ | ✓ | _NA_ |
| `metadata.labels.topology.kubernetes.io/zone` | The desired availability zone in which to schedule the VM | ✓ | ✓ | ✓ |

## Resources

Some of a VM's hardware resources are derived, and there are some that may be influenced directly by a user.

### CPU and Memory

### Networking

### Storage
