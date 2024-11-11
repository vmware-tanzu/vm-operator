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

VMs are scheduled to run on a [`Node`](https://kubernetes.io/docs/concepts/architecture/nodes/) in the cluster. Regardless where VM Operator is running, the `Node` represents an underlying hypervisor. For example, on vSphere Supervisor a `Node` represents an ESXi host.

The VM remains on that `Node` until the VM resource is deleted or, if supported by the underlying platform, the VM is rescheduled on another `Node` via live-migration (ex. vMotion). There may also be situations where a VM cannot be rescheduled with live-migration due to some hardware dependency. In these instances, depending on how the infrastructure administrator has configured the platform, the VM may be automatically or manually powered off and then powered on again on a compatible `Node` if one is available. If no other `Nodes` can support the VM, its controller will continue to try powering on the VM until such time a compatible `Node` becomes available.

The name of a VM must be a valid [DNS subdomain](https://kubernetes.io/docs/concepts/overview/working-with-objects/names#dns-subdomain-names) value, but this can produce unexpected results for the VM's host name. For best compatibility, the name should follow the more restrictive rules for a [DNS label](https://kubernetes.io/docs/concepts/overview/working-with-objects/names#dns-label-names).

### VM Image

The `VirtualMachineImage` is a namespace-scoped resource from which a VM's disk image(s) is/are derived. This is why the name of a `VirtualMachineImage` resource must be specified when creating a new VM. It is also possible to deploy a new VM with the cluster-scoped `ClusterVirtualMachineImage` resource. The following commands may be used to discover the available images:

=== "Get images for a namespace"

    ```shell
    kubectl get -n <NAMESPACE> vmimage
    ```

=== "Get images for a cluster"

    ```shell
    kubectl get clustervmimage
    ```

For more information on `VirtualMachineImage` and `ClusterVirtualMachineImage` resources, please see the documentation for [`VirtualMachineImage`](../images/vm-image.md).

### VM Class

A `VirtualMachineClass` is a namespace-scoped resource from which a VM's virtual hardware is derived. This is why the name of a `VirtualMachineClass` is required when creating a VM. The `VirtualMachineClass` resources available in a given namespace may be discovered with:

```shell
kubectl get -n <NAMESPACE> vmclass
```

For more information on `VirtualMachineClass` resources, please see the documentation for [`VirtualMachineClass`](./vm-class.md).

### Storage Class

A `StorageClass` is a cluster-scoped resource that defines a VM's storage policy and is required to create a new VM. The following command may be used to discover a cluster's available `StorageClass` resources:

```shell
kubectl get storageclass
```

For more information on `StorageClass` resources, please see the documentation for [`StorageClass`](https://kubernetes.io/docs/concepts/storage/storage-classes/).

### Encryption Class

An `EncryptionClass`  is a namespace-scoped resource that defines the key provider and key used to encrypt a VM. The following command may be used to discover a namespace's available `EncryptionClass` resources:

```shell
kubectl get -n <NAMESPACE> encryptionclass
```

For more information on `EncryptionClass` resources, please see the [Encryption](#encryption) section below.

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

#### ClassConfigurationSynced Condition

The condition `VirtualMachineClassConfigurationSynced` is used to report whether or not the VM's configuration was synchronized to the class.
If the configuration it, then the condition will be:

```yaml
status:
  conditions:
  - type: VirtualMachineClassConfigurationSynced
    status: True
```
When `VirtualMachineClassConfigurationSynced` has `status: False`, the `reason` field may be set to one of the following values:

| Reason             | Description                                                                                                                                  |
|--------------------|----------------------------------------------------------------------------------------------------------------------------------------------|
| `ClassNotFound`    | The resource specified with the `spec.className` field cannot be found.                                                                      |
| `ClassNameChanged` | The resource specified with the `spec.className` field has been changed, and a resize will be performed at the next appropriate power state. |
| `SameClassResize`  | The resource pointed to by the `spec.className` field has been updated, and a resize will be performed at the next appropriate power state.   |

If the condition is ever false, please refer first to the condition's `reason` field and then `message` for more information.

## Encryption

The field `spec.crypto` may be used in conjunction with a VM's storage class and/or virtual trusted platform module (vTPM) to control a VM's encryption level.

### Encrypting a VM

A VM may be encrypted with either:

* the default key provider
* an `EncryptionClass` resource

#### Default Key Provider

By default, `spec.crypto.useDefaultKeyProvider` is true, meaning that if there is a default key provider and the VM has an encryption storage class and/or vTPM, the VM will be subject to some level of encryption. For example, if there was a default key provider present, it would be used to encrypt the following VM:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha3
kind: VirtualMachine
metadata:
  name: my-vm
  namespace: my-namespace
spec:
  className:    my-vm-class
  imageName:    vmi-0a0044d7c690bcbea
  storageClass: my-encrypted-storage-class
```

#### EncryptionClass

It is also possible to set `spec.crypto.encryptionClassName` to the name of an `EncryptionClass` resource in the same namespace as the VM. This resource specifies the provider and key ID used to encrypt workloads. For example, the following VM would be deployed and encrypted using the `EncryptionClass` named `my-encryption-class`:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha3
kind: VirtualMachine
metadata:
  name: my-vm
  namespace: my-namespace
spec:
  className:    my-vm-class
  imageName:    vmi-0a0044d7c690bcbea
  storageClass: my-encrypted-storage-class
  crypto:
    encryptionClassName: my-encryption-class
```

### Rekeying a VM

Users can rekey VMs and their [classic disks](#volume-type) by:

* Switching to a different `EncryptionClass` by changing the value of `spec.crypto.encryptionClassName`.
* Using the default key provider, if one exists, by removing `spec.crypto.encryptionClassName`.

Either change results in the VM and its [classic disks](#volume-type) being rekeyed using the new key provider.

### Decrypting a VM

It is not possible to decrypt an encrypted VM using VM Operator.

### Encryption Status

The following fields may be used to determine the encryption status of a VM:

| Name | Description |
|------|-------------|
| `status.crypto.encrypted` | The observed state of the VM's encryption. May be `Config`, `Disks`, or both. |
| `status.crypto.providerID` | The provider ID used to encrypt the VM. |
| `status.crypto.keyID` | The key ID used to encrypt the VM. |

For example, the following is an example of the status of a VM encrypted with an encryption storage class:

```yaml
status:
  crypto:
    encrypted:
    - Config
    - Disks
    providerID: my-key-provider-id
    keyID: my-key-id
```

#### Encryption Type

The type of encryption used by the VM is reported in the list `status.crypto.encrypted`. The list may contain the values `Config` and/or `Disks`, depending on the storage class and hardware present in the VM as the chart below illustrates:

|                          | Config | Disks |
|--------------------------|:------:|:-----:|
| Encryption storage class |    ✓   |   ✓   |
| vTPM                     |    ✓   |       |

The `Config` type refers to all files related to a VM except for virtual disks. The `Disks` type indicates at least one of a VM's virtual disks is encrypted. To determine which of the VM's disks are encrypted, please refer to [`status.volumes[].crypto`](#volume-status). 

#### EncryptionSynced Condition

The condition `VirtualMachineEncryptionSynced` is also used to report whether or not the VM's desired encryption state was synchronized. For example, a VM that should be encrypted but is powered on may have the following condition:

```yaml
status:
  conditions:
  - type: VirtualMachineEncryptionSynced
    status: False
    reason: InvalidState
    message: Must be powered off to encrypt VM.
```

If the VM requested encryption and it was successful, then the condition will be:

```yaml
status:
  conditions:
  - type: VirtualMachineEncryptionSynced
    status: True
```

When `VirtualMachineEncryptionSynced` has `status: False`, the `reason` field may be set to one of or combination of the following values:

| Reason | Description |
|--------|-------------|
| `EncryptionClassNotFound` | The resource specified with `spec.crypto.encryptionClassName` cannot be found. |
| `EncryptionClassInvalid` |  The resource specified with `spec.crypto.encryptionClassName` contains an invalid provider and/or key ID. |
| `InvalidState` | The VM cannot be encrypted, rekeyed, or updated in its current state. |
| `InvalidChanges` | The VM has pending changes that would prohibit an encrypt, rekey, or update operation. |
| `ReconfigureError` |  An encrypt, rekey, or update operation was attempted but failed due to a reason related to encryption. |

If the condition is ever false, please refer first to the condition's `reason` field and then `message` for more information.

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

A VM deployed from a `VirtualMachineImage` or `ClusterVirtualMachineImage` inherit the disk(s) from those images. Additional storage may also be provided by using [PersistentVolumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes).

### Storage Usage

The following fields summarize a `VirtualMachine` resource's overall storage usage:

| Name | Description |
|------|-------------|
| `status.storage.committed` | The total storage space committed to this `VirtualMachine`. |
| `status.storage.uncommitted` | The total storage space potentially used by this `VirtualMachine` |
| `status.storage.unshared` | The total storage space occupied by this VirtualMachine that is not shared with any other `VirtualMachine`. |

For example, the following illustrates the storage usage for a `VirtualMachine`:

```yaml
status:
  storage:
    committed: 6Gi
    uncommitted: 7Gi
    unshared: 2Gi
```

Individual volume usage may be determined by inspecting [`status.volumes`](#volume-status).


### Storage Quota

`VirtualMachine` resources on a vSphere Supervisor must adhere to the namespace's storage quota:

#### Storage Quota Reservation

Deploying a `VirtualMachine` resource will fail an admission check unless the total capacity required by the disk(s) from the `VirtualMachineImage` or `ClusterVirtualMachineImage` is less than or equal to the amount of free space available in the namespace according to its storage quota. For example, consider the following:

* A VM is deployed from an image that has a disk which is only `2Gi` in size, but has the capacity to grow to `100Gi`.
* The storage quota for the namespace is `500Gi` and there is currently only `50Gi` available.

The VM in the above scenario will fail the admission check. While the image's disk is only `2Gi` in size and there are `50Gi` of free space in the namespace, the disk's _total_ capacity is `100Gi`, which exceeds the quota.

#### Storage Quota Usage

Deployed `VirtualMachine` resources contribute to the total storage usage in a given namespace. The usage of a `VirtualMachine` is not calculated based on the total possible space the VM _can_consume, rather usage is the total number of unshared bytes on disk used by a `VirtualMachine` less the space used by any of the VM's [managed volumes](#volume-type). For example, consider the following:

* A VM is using `2Gi` of unshared space.
* The VM has three volumes, two of which are managed that, combined, consume `1Gi`.

In the above scenario, the VM's total usage will be reported as `1Gi`.


### Volumes

A `VirtualMachine` resource's disks are referred to as _volumes_.

#### Volume Type

There are two types of volumes: _Managed_ and _Classic_:

* **Managed** volumes refer to storage that is provided by [PersistentVolumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes).
* **Classic** volumes refer to the disk(s) that came from the `VirtualMachineImage` or `ClusterVirtualMachineImage` from which the `VirtualMachine` was deployed.

#### Adding Volumes

##### Adding Classic Volumes

There is no support for adding classic vSphere disks directly to `VirtualMachine` resources. Additional storage _must_ be provided by managed volumes, i.e. [PersistentVolumeClaims](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims).

##### Adding Managed Volumes

The following steps describe how to provide additional storage with [PersistentVolumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes):

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

#### Volume Status

The field `status.volumes` described the observed state of a `VirtualMachine` resource's volumes, including information about the volume's usage and encryption properties:

=== "Volume Properties"

    | Name | Description |
    |------|-------------|
    | `attached` | Whether or not the volume has been successfully attached to the `VirtualMachine`. |
    | `crypto` | An optional field set only if the volume is encrypted. |
    | `diskUUID` | The unique identifier of the volume's underlying disk. |
    | `error` | The last observed error that may have occurred when attaching/detaching the disk. |
    | `limit` | The maximum amount of space that may be used by this volume. |
    | `name` | The name of the volume. For managed disks this is the name from `spec.volumes` and for classic disks this is the name of the underlying disk. |
    | `type` | The [type](#volume-type) of the attached volume, i.e. either `Classic` or `Managed` |
    | `used` | The total storage space occupied by this VirtualMachine that is not shared with any other `VirtualMachine`. |

=== "Encryption Properties"

    If the field `crypto` is set, it will contain the following information:

    | Name | Description |
    |------|-------------|
    | `keyID` | The identifier of the key used to encrypt the volume.  |
    | `providerID` | The identifier of the provider used to encrypt the volume. |

The following example shows the status for a single, classic, encrypted volume:

```yaml
status:
  volumes:
  - attached: true
    crypto:
      keyID: "19"
      providerID: gce2e-standard
    diskUUID: 6000C295-760e-e736-3a10-1395a8749300
    limit: 10Gi
    name: my-vm
    type: Classic
    used: 2Gi
```

The status may also reflect multiple volumes, both classic and managed:

```yaml
status:
  volumes:
  - attached: true
    crypto:
      keyID: "19"
      providerID: gce2e-standard
    diskUUID: 6000C295-760e-e736-3a10-1395a8749300
    limit: 10Gi
    name: my-vm
    type: Classic
    used: 2Gi
  - attached: true
    diskUUID: 6000C299-8a21-f2ad-7084-2195c255f905
    limit: 1Gi
    name: my-disk-1
    type: Managed
    used: "0"
```


## Power Management

### Power States

#### On, Off, & Suspend

The field `spec.powerState` controls the power state of a VM and may be set to one of the following values:

| Power State | Description |
|-------------|-------------|
| `PoweredOn` | Powers on a powered off VM or resumes a suspended VM |
| `PoweredOff` | Powers off a powered on or suspended VM (controlled by `spec.powerOffMode`) |
| `Suspended` | Suspends a powered on VM (controlled by `spec.suspendMode`) |

#### Restart

It is possible to restart a powered on VM by setting `spec.nextRestartTime` to `now` (case-insensitive). A mutating webhook transforms `now` into an RFC3339Nano-formatted string. During the VM's reconciliation, the value of `spec.nextRestartTime` is compared to the last time the VM was restarted by VM Operator. If `spec.nextRestartTime` occurs _after_ the last time a restart occurred, then the VM is restarted in accordance with `spec.restartMode`.

Please note that it is not possible to schedule future restarts by assigning an explicit RFC3339Nano-formatted string to `spec.nextRestartTime`. The only valid values for `spec.nextRestartTime` are an empty string when creating a VM and `now` (case-insensitive) when updating/patching an existing VM.


#### Default Power State on Create

When updating a VM's power state, an empty string is not allowed -- the desired power state must be specified explicitly. However, on create, the VM's power state may be omitted. When this occurs, the power state defaults to `PoweredOn`.


#### Transitions

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


### Power Op Mode

The fields `spec.powerOffMode`, `spec.suspendMode`, and `spec.restartMode` control how a VM is powered off, suspended, and restarted:

| Mode | Description | Default |
|-------------|-------------|:-----------------:|
| `Hard` | Halts, suspends, or restarts the VM with no interaction with the guest |  |
| `Soft` | The guest is shutdown, suspended, or restarted gracefully (requires VM Tools) |  |
| `TrySoft` | Attempts a graceful shutdown/standby/restart if VM Tools is present, otherwise falls back to a hard operation the VM has not achieved the desired power state after five minutes. | ✓ |

## Identifiers

In addition to the `VirtualMachine` resource's object name, i.e. `metadata.name`, there are several other methods by which a VM can be identified:

### BIOS UUID

The field `spec.biosUUID` defaults to a random V4 UUID. This field is immutable and may only be set manually when creating a VM by a privileged user, such as a service account. The value of `spec.biosUUID` is generally unique across all VMs running in a hypervisor, but there may be cases where it is duplicated, such as when a backed-up VM is restored to a new VM. The value of `spec.biosUUID` is assigned to a vSphere VM's `config.uuid` property, which is why it is possible to discover a vSphere VM with this value using the following command:

```shell
govc vm.info -vm.uuid $(k get vm -o jsonpath='{.spec.biosUUID}' -n $ns $name)
```

### Instance UUID

The field `spec.biosUUID` defaults to a random V4 UUID. This field is immutable and may only be set manually when creating a VM by a privileged user, such as a service account. The instance UUID is used by vSphere to uniquely identify all VMs, including those that may share the same BIOS UUID. The value of `spec.instanceUUID` is assigned to a vSphere VM's `config.instanceUUID` property, which is why it is possible to discover a vSphere VM with this value using the following command:

```shell
govc vm.info -vm.uuid $(k get vm -o jsonpath='{.spec.instanceUUID}' -n $ns $name)
```

## Guest OS

### Guest ID

The optional field `spec.guestID` that may be used when deploying a VM to specify its guest operating system identifier. This field may also be updated when a VM is powered off. The value of this field is derived from the list of [supported guest identifiers](https://dp-downloads.broadcom.com/api-content/apis/API_VWSA_001/8.0U2/html/SDK/vsphere-ws/docs/ReferenceGuide/vim.vm.GuestOsDescriptor.GuestOsIdentifier.html). The following command may be used to query the currently supported guest identifiers for a given vSphere environment:

```shell
govc vm.option.info -cluster CLUSTER_NAME
```

The above command will emit output similar to the following:

```shell
almalinux_64Guest           AlmaLinux (64-bit)
rockylinux_64Guest          Rocky Linux (64-bit)
windows2022srvNext_64Guest  Microsoft Windows Server 2025 (64-bit)
...
```
