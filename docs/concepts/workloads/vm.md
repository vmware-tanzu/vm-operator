# VirtualMachine

_VirtualMachines_ (VM) represent a collection of virtual hardware combined with one or more virtual disk images that can be managed using a Kubernetes cluster, such as vSphere Supervisor, and are scheduled on a VMware hypervisor, such as vSphere.

## What is a VM?

VMware [defines](https://www.vmware.com/topics/glossary/content/virtual-machine.html) a VM as:

> a compute resource that uses software instead of a physical computer to run programs and deploy apps. One or more virtual “guest” machines run on a physical “host” machine. Each virtual machine runs its own operating system and functions separately from the other VMs, even when they are all running on the same host.

The VM Operator `VirtualMachine` API enables the lifecycle-management of a VM on an underlying hypervisor. A VM resource's virtual hardware is derived from a [`VirtualMachineClass`](./vm-class.md) (VM Class) and a [`VirtualMachineImage`](../images/vm-image.md) (VM Image) supplies the VM's disk(s).

## Using VMs

The following is an example of a `VirtualMachine` resource that bootstraps a Photon OS image using [Cloud-Init](https://cloudinit.readthedocs.io/):

```yaml title="vm-example.yaml"
--8<-- "./docs/concepts/workloads/vm-example.yaml"
```

!!! note "Customizing a Guest"

    Please see [this section](./guest.md) for more information on the available bootstrap providers tha may be used to bootstrap a guest's host name, network, and further customize the operating system.

To create the VM shown above, run the following command (replacing `<NAMESPACE>` with the name of the [`Namespace`](https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/) in which to create the VM):

```shell
kubectl apply -n <NAMESPACE> -f ${{ config.repo_url_raw }}/main/docs/concepts/workloads/vm-example.yaml
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

For more information on Storage Classes, please see the documentation for [`StorageClass`](https://kubernetes.io/docs/concepts/storage/storage-classes/).


## Updating a VM

It is possible to update parts of an existing `VirtualMachine` resource. Some fields are completely immutable while some _can_ be modified depending on the VM's power state and whether or not the field has already been set to a non-empty value. The following table highlights what fields may or may not be updated and under what conditions:

| Update | Description | While Powered On | While Powered Off | Not Already Set |
|--------|-------------|:----------------:|:-----------------:|:---------------:|
| `spec.imageName` | The name of the `VirtualMachineImage` that supplies the VM's disk(s) | ✗ | ✗ | _NA_ |
| `spec.className` | The name of the `VirtualMachineClass` that supplies the VM's virtual hardware | ✗ | ✗ | _NA_ |
| `spec.powerState` | The VM's desired power state | ✓ | ✓ | _NA_ |
| `metadata.labels.topology.kubernetes.io/zone` | The desired availability zone in which to schedule the VM | ✓ | ✓ | ✓ |

## Resources

Some of a VM's hardware resources are derived from the policies defined by your infrastructure administrator, others may be influenced directly by a user.

### CPU and Memory
CPU and memory of a VM are derived form the `VirtualMachineClass` that is used to create the VM. Specifically, the `memoryMB` and `numCPUs` properties in the `configSpec` field of the `VirtualMachineClass` resource dictate the exact number of virtual CPUs, and the virtual memory that the VM will be created with.

Additionally, your administrator might also define certain policies in your VM class in form of resource [reservations](https://docs.vmware.com/en/VMware-vSphere/7.0/com.vmware.vsphere.resmgmt.doc/GUID-8B88D3D8-E9D9-4C05-A065-B3DE1FFFB401.html) or [limits](https://docs.vmware.com/en/VMware-vSphere/7.0/com.vmware.vsphere.resmgmt.doc/GUID-117972E3-F5D3-4641-9EAC-F9DD2B0761C3.html). These are vSphere constructs which are used to reserve, or set a ceiling on resources consumed by a VM. Users can view these policies by inspecting the `ConfigSpec.cpuAllocation` and `ConfigSpec.memoryAllocation` fields of the VM Class for CPU and memory reservation/limits information respectively.

For an example, consider the following VM Class:
```
apiVersion: vmoperator.vmware.com/v1alpha1
kind: VirtualMachineClass
metadata:
  name: my-vm-class
spec:
  configSpec:
    _typeName: VirtualMachineConfigSpec
    cpuAllocation:
      _typeName: ResourceAllocationInfo
      limit: 400
      reservation: 200
    memoryAllocation:
      _typeName: ResourceAllocationInfo
      limit: 2048
      reservation: 1024
  hardware:
    cpus: 2
    memory: 4Gi
```
A VM created with this VM class will have 2 vCPUs, 4 GiB of memory. Additionally, the VM will be created with a guaranteed CPU bandwidth of 200MHz and 1024 MB of memory. The CPU bandwidth of the VM will never be allowed to exceed 400MHz and the memory will not exceed 2GB.

!!! note "Units for CPU reservations/limits are in MHz"

    Please note that the units for CPU are different in hardware and reservations/limits.  Virtual hardware is specified in units of vCPUs, whereas CPU reservations/limits are in MHz.  Memory can be specified in MiB, GiB, whereas memory reservations/limits are always in MB.

### Networking
Users can optionally specify a list of network interfaces for their VM in the `spec.NetworkInterfaces` field. Each interface must specify the network type which can be either `nsx-t` or `vsphere-distributed`, depending on the network provider being used. Users can further customize each interface with optional fields such as the Ethernet card type, network name etc. If a VM does not specify any network interface, VM operator creates an interface that is connected to the default network available in the namespace. The IP address of the network interface is handled by the available network provider (configured by the administrator).

#### How are a VM's Network Interfaces Configured?
VM operator uses the network devices specified in the VM Class to configure the network interfaces specified in the VM spec. If it cannot find sufficient number of devices in the VM class, a set of defaults are used to configure the network interfaces.

As an example, consider the following VM Class that specifies two network devices, each with a different Ethernet card type.

```yaml
apiVersion: vmoperator.vmware.com/v1alpha1
kind: VirtualMachineClass
metadata:
  name: class-with-two-nics
spec:
  configSpec:
    _typeName: VirtualMachineConfigSpec
    deviceChange:
    - _typeName: VirtualDeviceConfigSpec
      device:
        _typeName: VirtualE1000
        key: -100
      operation: add
    - _typeName: VirtualDeviceConfigSpec
      device:
        _typeName: VirtualVmxnet3
        key: -200
      operation: add
  hardware:
    cpus: 2
    memory: 4Gi
```

If the following VM is created using the class defined above:
```yaml
apiVersion: vmoperator.vmware.com/v1alpha1
kind: VirtualMachine
metadata:
  name: my-vm
  namespace: my-namespace
spec:
  className: class-with-two-nics
  imageName: ubuntu-kinetic
  networkInterfaces:
  - networkType: nsx-t
    ethernetCardType: eth1000
  - networkType: nsx-t
    etherNetCardType: vmxNet3
  - networkType: nsx-t
    ethernetCardType: vmxnet2
  - networkType: nsx-t
  powerState: poweredOn
  storageClass: wcpglobal-storage-profile
```
The first two network interfaces of the VM are configured using the VM class. So, even though they specify a card type, they inherit the `VirtualE1000` and `VirtualVmxnet3` types respectively. The third interface is configured using the card type it specifies - `VirtualVmxnet2`. The fourth interface does not specify any type, so the default Ethernet card type of `VirtualVmxnet3` is used.


### Storage
A VM deployed using VM operator inherits the storage defined in the `VirtualMachineImage`.  However, developers can also provision and manage additional storage dynamically by leveraging [PersistentVolumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes). To do this, a user would create a [PersistentVolumeClaim](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims) resource by picking a `StorageClass` associated with their namespace, along with other properties such as the disk size, mode etc. VM operator then dynamically provisions a first class disks which is exposed to the guest as a block volume. Users can start using the disk after formatting and mounting it at a mountpoint. VM operator also supports resizing these volumes to increase their size.

## Power States

### On, Off, & Suspend

The field `spec.powerState` controls the power state of a VM and may be set to one of the following values:

| Power State | Description |
|-------------|-------------|
| `poweredOn` | Powers on a powered off VM or resumes a suspended VM |
| `poweredOff` | Powers off a powered on or suspended VM (controlled by `spec.powerOffMode`) |
| `suspended` | Suspends a powered on VM (controlled by `spec.suspendMode`) |

### Restart

It is possible to restart a powered on VM by setting `spec.nextRestartTime` to `now` (case-insensitive). A mutating webhook transforms `now` into an RFC3339Nano-formatted string. During the VM's reconciliation, the value of `spec.nextRestartTime` is compared to the last time the VM was restarted by VM Operator. If `spec.nextRestartTime` occurs _after_ the last time a restart occurred, then the VM is restarted in accordance with `spec.restartMode`.

Please note that it is not possible to schedule future restarts by assigning an explicit RFC3339Nano-formatted string to `spec.nextRestartTime`. The only valid values for `spec.nextRestartTime` are an empty string when creating a VM and `now` (case-insensitive) when updating/patching an existing VM.


### Default Power State on Create

When updating a VM's power state, an empty string is not allowed -- the desired power state must be specified explicitly. However, on create, the VM's power state may be omitted. When this occurs, the power state defaults to `poweredOn`.


### Transitions

Please note that there are supported power state transitions, and if a power state is requested that is not a supported transition, an error will be returned from a validating webhook.

<table>
    <tbody>
        <tr>
            <th></th>
            <th style="text-align:center;" colspan="4">Desired Power Operations</th>
        </tr>
        <tr>
            <th style="text-align:center;">Observed Power State</th>
            <th style="text-align:center;">Power On / Resume</th>
            <th style="text-align:center;">Power Off</th>
            <th style="text-align:center;">Suspend</th>
            <th style="text-align:center;">Restart</th>
        </tr>
        <tr>
            <th style="text-align:center;"><code>poweredOn</code></th>
            <td style="text-align:center;"><code>NA</code></td>
            <td style="text-align:center;">✓</td>
            <td style="text-align:center;">✓</td>
            <td style="text-align:center;">✓</td>
        </tr>
        <tr>
            <th style="text-align:center;"><code>poweredOff</code></th>
            <td style="text-align:center;">✓</td>
            <td style="text-align:center;"><code>NA</code></td>
            <td style="text-align:center;">✓<br /><em>if <code>spec.powerOffMode: hard</code></em></td>
            <td style="text-align:center;">❌</td>
        </tr>
        <tr>
            <th style="text-align:center;"><code>suspended</code></th>
            <td style="text-align:center;">✓</td>
            <td style="text-align:center;">❌</td>
            <td style="text-align:center;"><code>NA</code></td>
            <td style="text-align:center;">❌</td>
        </tr>
    </tbody>
</table>


## Power Op Mode

The fields `spec.powerOffMode`, `spec.suspendMode`, and `spec.restartMode` control how a VM is powered off, suspended, and restarted:

!!! note "Default Power Op Mode"

    Please note that the default power op mode is changing in v1alpha2 to `TrySoft`. This should not impact users still managing resources using v1alpha1.

| Mode | Description | Default |
|-------------|-------------|:-----------------:|
| `hard` | Halts, suspends, or restarts the VM with no interaction with the guest | ✓ |
| `soft` | The guest is shutdown, suspended, or restarted gracefully (requires VM Tools) |
| `trySoft` | Attempts a graceful shutdown/standby/restart if VM Tools is present, otherwise falls back to a hard operation the VM has not achieved the desired power state after five minutes. |

