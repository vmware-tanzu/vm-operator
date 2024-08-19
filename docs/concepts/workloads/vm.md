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
| `spec.className` | The name of the `VirtualMachineClass` that supplies the VM's virtual hardware | ✓ | ✓ | _NA_ |
| `spec.powerState` | The VM's desired power state | ✓ | ✓ | _NA_ |
| `metadata.labels.topology.kubernetes.io/zone` | The desired availability zone in which to schedule the VM | x | x | ✓ |

Some of a VM's hardware resources are derived from the policies defined by your infrastructure administrator, others may be influenced directly by a user.

## CPU and Memory

CPU and memory of a VM are derived from the `VirtualMachineClass` resource used to deploy the VM. Specifically, the `spec.configSpec.numCPUs` and `spec.configSpec.memoryMB` properties in the `VirtualMachineClass` resource dictate the number of CPUs and amount of memory allocated to the VM.

Additionally, an administrator might also define certain policies in a `VirtualMachineClass` resource in the form of resource [reservations](https://docs.vmware.com/en/VMware-vSphere/7.0/com.vmware.vsphere.resmgmt.doc/GUID-8B88D3D8-E9D9-4C05-A065-B3DE1FFFB401.html) or [limits](https://docs.vmware.com/en/VMware-vSphere/7.0/com.vmware.vsphere.resmgmt.doc/GUID-117972E3-F5D3-4641-9EAC-F9DD2B0761C3.html). These are vSphere constructs reserve or set a ceiling on resources consumed by a VM. Users may view these policies by inspecting the `spec.configSpec.cpuAllocation` and `spec.configSpec.memoryAllocation` fields of a `VirtualMachineClass` resource.

For an example, consider the following VM Class:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha3
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

A VM created with this VM class will have 2 CPUs and 4 GiB of memory. Additionally, the VM will be created with a guaranteed CPU bandwidth of 200MHz and 1024 MB of memory. The CPU bandwidth of the VM will never be allowed to exceed 400MHz and the memory will not exceed 2GB.

!!! note "Units for CPU reservations/limits are in MHz"

    Please note that the units for CPU are different in hardware and reservations/limits. Virtual hardware is specified in units of vCPUs, whereas CPU reservations/limits are in MHz. Memory can be specified in MiB, GiB, whereas memory reservations/limits are always in MB.

### Resizing

The VM's `spec.ClassName` field can be edited to point to a different class, causing the VM's virtual hardware configuration to be updated to the new class.

!!! note "CPU/Memory Resize Only"

   Currently, only CPU and Memory, and their associated limits and reservations, are updated during a resize. This may change in the future, but at this time, other fields from the class ConfigSpec are not updated during a resize.

The VM must either be powered off or transitioning from powered off to powered on to be resized.

By default, the VM will be resized once to reflect the new class. That is, if the `VirtualMachineClass` itself is later updated, the VM will not be resized again. The `vmoperator.vmware.com/same-vm-class-resize` annotation can be added to a VM to resize the VM as the class itself changes.

## Networking

The `spec.network` field may be used to configure networking for a `VirtualMachine` resource.

!!! note "Immutable Network Configuration"

    Currently, the VM's network configuration is immutable after the VM is deployed. This may change in the future, but at this time, if the VM's network settings need to be updated, the VM must be redeployed.

### Disable Networking

It is possible to disable a VM's networking entirely by setting `spec.network.disabled` to `true`. This ensures the VM is not configured with any network interfaces. Please note this field currently has no effect on VMs that are already deployed.

### Global Guest Network Configuration

There are several options which may be used to influence the guest's global networking configuration. The term _global_ is in contrast to _per network interface_. Support for these fields depends on the bootstrap provider:

| Field | Description | Cloud-Init | LinuxPrep | Sysprep |
|-------|-------------|:----------:|:---------:|:-------:|
| `spec.network.hostName` | The guest's host name | ✓ | ✓ | ✓ |
| `spec.network.domainName` | The guest's domain name | ✓ | ✓ | ✓ |
| `spec.network.nameservers` | A list DNS server IP addresses |  | ✓ | ✓ |
| `spec.network.searchDomains` | A list of DNS search domains |  | ✓ | ✓ |

### Network Interfaces

The field `spec.network.interfaces` describes one or more network interfaces to add to the VM, ex.:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha3
kind: VirtualMachine
metadata:
  name: my-vm
  namespace: my-namespace
spec:
  className:    my-vm-class
  imageName:    vmi-0a0044d7c690bcbea
  storageClass: my-storage-class
  network:
    interfaces:
    - name: eth0
    - name: eth1
```

#### Default Network Interface

If `spec.network.interfaces` is not specified when deploying a VM, a default network interface is added unless `spec.network.disabled` is `true`.

#### Network Adapter Type

The `VirtualMachineClass` used to deploy a VM determines the type for the VM's network adapters. If the number of interfaces for a `VirtualMachine` exceeds the number of interfaces in a `VirtualMachineClass`, the `VirtualVmxnet3` type is used for the additional interfaces. For example, consider the following `VirtualMachineClass` that specifies two different network interfaces:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha3
kind: VirtualMachineClass
metadata:
  name: my-vm-class
  namespace: my-namespace
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
        _typeName: VirtualVmxnet2
        key: -200
      operation: add
  hardware:
    cpus: 2
    memory: 4Gi
```

What happens when the following VM is deployed with the above class?

```yaml
apiVersion: vmoperator.vmware.com/v1alpha3
kind: VirtualMachine
metadata:
  name: my-vm
  namespace: my-namespace
spec:
  className: my-vm-class
  imageName: vmi-0a0044d7c690bcbea
  storageClass: my-storage-class
  network:
    interfaces:
    - name: eth0
    - name: eth1
    - name: eth2
```

The first two network interfaces in the VM are configured using the VM class. The third interface is configured using `VirtualVmxnet3`. In other words:

| Interface | Type |
|---|---|
| `eth0` | `VirtualE1000` |
| `eth1` | `VirtualVmxnet2` |
| `eth2` | `VirtualVmxnet3` |

#### Per-Interface Guest Network Configuration

There are several options which may be used to influence the guest's per-interface networking configuration. Support for these fields depends on the bootstrap provider.

| Field | Description | Cloud-Init | LinuxPrep | Sysprep |
|-------|-------------|:----------:|:---------:|:-------:|
| `spec.network.interfaces[].guestDeviceName` | The name of the interface in the guest | ✓ |  |  |
| `spec.network.interfaces[].addresses` | The IP4 and IP6 addresses (with prefix length) for the interface | ✓ | ✓ | ✓ |
| `spec.network.interfaces[].dhcp4` | Enables DHCP4 | ✓ | ✓ | ✓ |
| `spec.network.interfaces[].dhcp6` | Enables DHCP6 | ✓ | ✓ | ✓ |
| `spec.network.interfaces[].gateway4` | The IP4 address of the gateway for the IP4 address family | ✓ | ✓ | ✓ |
| `spec.network.interfaces[].gateway6` | The IP6 address of the gateway for the IP6 address family | ✓ | ✓ | ✓ |
| `spec.network.interfaces[].mtu` | The maximum transmission unit size in bytes | ✓ |  |  |
| `spec.network.interfaces[].nameservers` | A list of DNS server IP addresses | ✓ |  |  |
| `spec.network.interfaces[].routes` | A list of static routes | ✓ |  |  |
| `spec.network.interfaces[].searchDomains` | A list of DNS search domains | ✓ |  |  |

!!! note "Underlying Network Support"

    Please note support for the fields `spec.network.interfaces[].addresses`, `spec.network.interfaces[].dhcp4`, and `spec.network.interfaces[].dhcp6` depends on the underlying network.

### Intended Network Config

Deploying a VM also normally means bootstrapping the guest with a valid network configuration. But what if the guest does not include a bootstrap engine, or the one included is not supported by VM Operator? Enter `status.network.config`.  Normally a Kubernetes resource's status contains _observed_ state. However, in the case of the VM's `status.network.config` field, the data represents the _intended_ network configuration. For example, the following YAML illustrates a VM deployed with a single network interface:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha3
kind: VirtualMachine
metadata:
  name: my-vm
  namespace: my-namespace
spec:
  className: my-vm-class
  imageName: vmi-0a0044d7c690bcbea
  storageClass: my-storage-class
  network:
    interfaces:
    - name: eth0
```

After being reconciled, the VM's status may resemble the following:

```yaml
status:
  network:
    config:
      dns:
        hostName: my-vm
        nameservers:
        - 8.8.8.8
        - 8.8.4.4
        searchDomains:
        - hello.world
        - fu.bar
        - example.com
      interfaces:
      - name: eth0
        ip:
          addresses: 192.168.0.3/24
          gateway4:  192.168.0.1
```

The above data does _not_ represent the _observed_ network configuration of the VM, but rather the network configuration that was _provided_ to the VM in order to bootstrap the guest. The information itself is derived from several locations:

| Field | Source |
|-------|--------|
| `status.network.config.dns.hostName` | From `spec.network.hostName` if non-empty, otherwise from `metadata.name` |
| `status.network.config.dns.domainName` | From `spec.network.domainName` if non-empty |
| `status.network.config.dns.nameservers[]` | From `spec.network.nameservers[]` if non-empty, otherwise from the `ConfigMap` used to initialize VM Operator |
| `status.network.config.dns.searchDomains[]` | From `spec.network.searchDomains[]` is used if non-empty, otherwise from the `ConfigMap` used to initialize VM Operator |
| `status.network.config.interfaces[]` | There will be an interface for every corresponding interface in `spec.network.interfaces[]` |
| `status.network.config.interfaces[].name` | From the corresponding `spec.network.interfaces[].name` |
| `status.network.config.interfaces[].dns.nameservers[]` | From the corresponding `spec.network.interfaces[].nameservers[]` |
| `status.network.config.interfaces[].dns.searchDomains[]` | From the corresponding `spec.network.interfaces[].searchDomains[]` |
| `status.network.config.interfaces[].ip.addresses[]` | From the corresponding `spec.network.interfaces[].addresses[]` if non-empty, otherwise from IPAM unless the connected network is configured to use DHCP4 *and* DHCP6, in which case this field will be empty |
| `status.network.config.interfaces[].ip.dhcp.ip4.enabled` | From the corresponding `spec.network.interfaces[].dhcp4` if `true`, otherwise `true` if the connected network is configured to use DHCP4 |
| `status.network.config.interfaces[].ip.dhcp.ip6.enabled` | From the corresponding `spec.network.interfaces[].dhcp6` if `true`, otherwise `true` if the connected network is configured to use DHCP6 |
| `status.network.config.interfaces[].ip.gateway4` | From the corresponding `spec.network.interfaces[].gateway4` if non-empty, otherwise from IPAM unless the connected network is configured to use DHCP4, in which case this field will be empty |
| `status.network.config.interfaces[].ip.gateway6` | From the corresponding `spec.network.interfaces[].gateway6` if non-empty, otherwise from IPAM unless the connected network is configured to use DHCP6, in which case this field will be empty |

## Storage

Deployed VMs inherit the storage defined in the `VirtualMachineImage`. To provide additional storage, users may leverage [PersistentVolumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes):

1. Create a [PersistentVolumeClaim](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims) resource by picking a `StorageClass` associated with the namespace in which the VM exists:

    ```yaml
    apiVersion: v1
    kind: PersistentVolumeClaim
    metadata:
      name: my-pvc
    spec:
      accessModes:
      - ReadWriteOnce
      volumeMode: Filesystem
      resources:
        requests:
          storage: 8Gi
      storageClassName: my-storage-class
    ```

2. Update the VM's `spec.volumes` field with a new volume that references the PVC:

    ```yaml
    apiVersion: vmoperator.vmware.com/v1alpha3
    kind: VirtualMachine
    metadata:
      name: my-vm
      namespace: my-namespace
    spec:
      className:    my-vm-class
      imageName:    vmi-0a0044d7c690bcbea
      storageClass: my-storage-class
      volumes:
      - name: my-disk-1
        persistentVolumeClaim:
          claimName: my-pvc
    ```

3. Wait for the new volume to be provisioned and attached to the VM.

4. SSH into the guest.

5. Format the new disk with desired filesystem.

6. Mount the disk and begin using it.

## Power States

### On, Off, & Suspend

The field `spec.powerState` controls the power state of a VM and may be set to one of the following values:

| Power State | Description |
|-------------|-------------|
| `PoweredOn` | Powers on a powered off VM or resumes a suspended VM |
| `PoweredOff` | Powers off a powered on or suspended VM (controlled by `spec.powerOffMode`) |
| `Suspended` | Suspends a powered on VM (controlled by `spec.suspendMode`) |

### Restart

It is possible to restart a powered on VM by setting `spec.nextRestartTime` to `now` (case-insensitive). A mutating webhook transforms `now` into an RFC3339Nano-formatted string. During the VM's reconciliation, the value of `spec.nextRestartTime` is compared to the last time the VM was restarted by VM Operator. If `spec.nextRestartTime` occurs _after_ the last time a restart occurred, then the VM is restarted in accordance with `spec.restartMode`.

Please note that it is not possible to schedule future restarts by assigning an explicit RFC3339Nano-formatted string to `spec.nextRestartTime`. The only valid values for `spec.nextRestartTime` are an empty string when creating a VM and `now` (case-insensitive) when updating/patching an existing VM.


### Default Power State on Create

When updating a VM's power state, an empty string is not allowed -- the desired power state must be specified explicitly. However, on create, the VM's power state may be omitted. When this occurs, the power state defaults to `PoweredOn`.


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
            <th style="text-align:center;"><code>PoweredOn</code></th>
            <td style="text-align:center;"><code>NA</code></td>
            <td style="text-align:center;">✓</td>
            <td style="text-align:center;">✓</td>
            <td style="text-align:center;">✓</td>
        </tr>
        <tr>
            <th style="text-align:center;"><code>PoweredOff</code></th>
            <td style="text-align:center;">✓</td>
            <td style="text-align:center;"><code>NA</code></td>
            <td style="text-align:center;">✓<br /><em>if <code>spec.powerOffMode: Hard</code></em></td>
            <td style="text-align:center;">❌</td>
        </tr>
        <tr>
            <th style="text-align:center;"><code>Suspended</code></th>
            <td style="text-align:center;">✓</td>
            <td style="text-align:center;">❌</td>
            <td style="text-align:center;"><code>NA</code></td>
            <td style="text-align:center;">❌</td>
        </tr>
    </tbody>
</table>


## Power Op Mode

The fields `spec.powerOffMode`, `spec.suspendMode`, and `spec.restartMode` control how a VM is powered off, suspended, and restarted:

| Mode | Description | Default |
|-------------|-------------|:-----------------:|
| `Hard` | Halts, suspends, or restarts the VM with no interaction with the guest |  |
| `Soft` | The guest is shutdown, suspended, or restarted gracefully (requires VM Tools) |  |
| `TrySoft` | Attempts a graceful shutdown/standby/restart if VM Tools is present, otherwise falls back to a hard operation the VM has not achieved the desired power state after five minutes. | ✓ |

## VM UUIDs

The `spec.instanceUUID` and `spec.biosUUID` fields both default to a random v4 UUID. These fields can only be specified by privileged accounts when creating a VM and cannot be updated.
This instanceUUID is used by VirtualCenter to uniquely identify all virtual machine instances, including those that may share the same BIOS UUID.

The value of `spec.instanceUUID` is used to set the vSphere VM `config.instanceUuid` field, which in turn is used to populate the VM's `status.instanceUUID`. An instanceUUID can be used to find the vSphere VM, for example:
``` bash
govc vm.info -vm.uuid $(k get vm -o jsonpath='{.spec.instanceUUID}' -n $ns $name)
```

The value of `spec.biosUUID` is used to set the vSphere VM `config.uuid` field, which in turn is used to populate the VM's `status.biosUUID`. A biosUUID can be used to find the vSphere VM, for example:
``` bash
govc vm.info -vm.uuid $(k get vm -o jsonpath='{.spec.biosUUID}' -n $ns $name)
```

## Guest IDs

The `spec.guestID` is an optional field that can be used to specify the guest operating system identifier when creating a new VM or powering on an existing VM.

A complete list of supported guest IDs can be found [here](https://dp-downloads.broadcom.com/api-content/apis/API_VWSA_001/8.0U2/html/SDK/vsphere-ws/docs/ReferenceGuide/vim.vm.GuestOsDescriptor.GuestOsIdentifier.html).

The `govc` command can be used to query the currently supported guest IDs for a given vSphere environment:

``` bash
$ govc vm.option.info -cluster CLUSTER_NAME
...
almalinux_64Guest           AlmaLinux (64-bit)
rockylinux_64Guest          Rocky Linux (64-bit)
windows2022srvNext_64Guest  Microsoft Windows Server 2025 (64-bit)
...
```
