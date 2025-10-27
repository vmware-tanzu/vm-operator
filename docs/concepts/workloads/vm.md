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

### VM Placement

When a VM is created, the placement system determines the optimal location for the workload based on available resources, policies, and constraints. The placement process considers:

* **Zone placement** for availability zone assignment
* **Host placement** for instance storage requirements
* **Datastore placement** for storage optimization

The placement system uses different strategies depending on requirements:
* Single-cluster placement for detailed host and datastore selection
* Multi-cluster placement for zone-level decisions across clusters
* Implied placement when only one viable option exists

For detailed information about VM placement, including configuration options, troubleshooting, and advanced topics, see [VirtualMachine Placement](./vm-placement.md).

### VM Image

The `VirtualMachineImage` is a namespace-scoped resource from which a VM's disk image(s) is/are derived. This is why the name of a `VirtualMachineImage` resource must be specified when creating a new VM from OVF. It is also possible to deploy a new VM with the cluster-scoped `ClusterVirtualMachineImage` resource. The following commands may be used to discover the available images:

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

## Hardware

### Configuration

Virtual machines deployed and managed by VM Operator support _all_ the same hardware and configuration options as any other VM on vSphere. The only difference is from where the hardware/configuration information is derived:

* **Disks** come from an admin or DevOps curated [VM image](#vm-image).
* **Hardware** comes from an admin-curated [VM class](#vm-class) as well as the DevOps-provided VM specification.

As long as they are specified in the VM class, features such as virtual GPUs, device groups, and SR-IOV NICs are all supported.

### Status

Information about the hardware being used (ex. CPU, memory, vGPUs, etc.) can be gleamed from the VM's status, ex.:

```yaml
status:
  hardware:
    cpu:
      reservation: 1000
      total: 4
    memory:
      reservation: 1024M
      total: 2048M
    vGPUs:
    - migrationType: None
      profile: grid_p40-1q
      type: Nvidia
    - migrationType: Enhanced
      profile: grid_p40-2q
      type: Nvidia
```

Examples of other hardware (ex. storage, vTPMs, etc.) in the status are illustrated throughout this page.

## Updating a VM

It is possible to update parts of an existing `VirtualMachine` resource. Some fields are completely immutable while some _can_ be modified depending on the VM's power state and whether or not the field has already been set to a non-empty value. The following table highlights what fields may or may not be updated and under what conditions:

| Update | Description | While Powered On | While Powered Off | Not Already Set |
|--------|-------------|:----------------:|:-----------------:|:---------------:|
| `spec.imageName` | The name of the `VirtualMachineImage` that supplies the VM's disk(s) | ✗ | ✗ | _NA_ |
| `spec.className` | The name of the `VirtualMachineClass` that supplies the VM's virtual hardware | ✓ | ✓ | _NA_ |
| `spec.powerState` | The VM's desired power state | ✓ | ✓ | _NA_ |
| `metadata.labels.topology.kubernetes.io/zone` | The desired availability zone in which to schedule the VM | x | x | ✓ |
| `spec.hardware.cdrom[].name` | The name of the CD-ROM device to mount ISO in the VM | x | ✓ | _NA_ |
| `spec.hardware.cdrom[].image` | The reference to an ISO type `VirtualMachineImage` or `ClusterVirtualMachineImage` to mount in the VM | x | ✓ | _NA_ |
| `spec.hardware.cdrom[].connected` | The desired connection state of the CD-ROM device | ✓ | ✓ | _NA_ |
| `spec.hardware.cdrom[].allowGuestControl` | Whether the guest OS is allowed to connect/disconnect the CD-ROM device | ✓ | ✓ | _NA_ |

Some of a VM's hardware resources are derived from the policies defined by your infrastructure administrator, others may be influenced directly by a user.

## Deleting a VM

### Delete Check

The annotation `delete.check.vmoperator.vmware.com/<COMPONENT>: <REASON>` may be applied to new or existing VMs in order to prevent the VM from being deleted until the annotation is removed.

This allows external components to participate in a VM's delete event. For example, there may be an external service that watches all `VirtualMachine` objects and deletes Computer objects in Active Directory for VMs being deleted, and needs to ensure the VM is not removed on the underlying hypervisor until its Computer object is deleted. This service can use a mutation webhook to apply the annotation to new VMs, only removing it once the Computer object is deleted in Active Directory.

There may be multiple instances of the annotation on a VM, hence its format:

```yaml
delete.check.vmoperator.vmware.com/<COMPONENT>: <REASON>
```

For example:

```yaml
annotations:
  delete.check.vmoperator.vmware.com/ad: "waiting on computer object"
  delete.check.vmoperator.vmware.com/net: "waiting on net auth"
  delete.check.vmoperator.vmware.com/app1: "waiting on app auth"
```

All check annotations must be removed before a VM can be deleted.

This annotation may only be added, modified, or removed by privileged users. This is to prevent a non-privileged from creating a situation where the VM cannot be deleted.

Non-privileged users cannot remove the annotation is because it is designed to be used by external services that want to _ensure_ the underlying VM is not deleted until some external condition is met. If a non-privileged user could bypass this, it would defeat the purpose of the annotation.

## CPU and Memory

### Configuration

CPU and memory of a VM are derived from the `VirtualMachineClass` resource used to deploy the VM. Specifically, the `spec.configSpec.numCPUs` and `spec.configSpec.memoryMB` properties in the `VirtualMachineClass` resource dictate the number of CPUs and amount of memory allocated to the VM.

Additionally, an administrator might also define certain policies in a `VirtualMachineClass` resource in the form of resource [reservations](https://docs.vmware.com/en/VMware-vSphere/7.0/com.vmware.vsphere.resmgmt.doc/GUID-8B88D3D8-E9D9-4C05-A065-B3DE1FFFB401.html) or [limits](https://docs.vmware.com/en/VMware-vSphere/7.0/com.vmware.vsphere.resmgmt.doc/GUID-117972E3-F5D3-4641-9EAC-F9DD2B0761C3.html). These are vSphere constructs reserve or set a ceiling on resources consumed by a VM. Users may view these policies by inspecting the `spec.configSpec.cpuAllocation` and `spec.configSpec.memoryAllocation` fields of a `VirtualMachineClass` resource.

For an example, consider the following VM Class:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
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

### Status

The configured CPU and memory may be gleamed from the VM's status as well:

```yaml
status:
  hardware:
    cpu:
      reservation: 1000
      total: 4
    memory:
      reservation: 1024M
      total: 2048M
```

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
apiVersion: vmoperator.vmware.com/v1alpha5
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
apiVersion: vmoperator.vmware.com/v1alpha5
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

* Using the default key provider, if one exists, by removing `spec.crypto.encryptionClassName`.
* Switching to a different `EncryptionClass` by changing the value of `spec.crypto.encryptionClassName`.

!!! note "Rekeying with a different `EncryptionClass`"

    Please note that picking a different `EncryptionClass` to rekey a VM requires the new `EncryptionClass` resource to either:

    * Have a different provider ID than the one currently used by the VM
    * Have a different key ID than the one currently used by the VM

    For example, imagine the following, two `EncryptionClass` resources:


    === "`EncryptionClass 1`"

        ```yaml
        apiVersion: encryption.vmware.com/v1alpha1
        kind: EncryptionClass
        metadata:
          name: my-encryption-class-1
          namespace: my-namespace-1
        spec:
          keyProvider: local
        ```

    === "EncryptionClass 2"

        ```yaml
        apiVersion: encryption.vmware.com/v1alpha1
        kind: EncryptionClass
        metadata:
          name: my-encryption-class-2
          namespace: my-namespace-1
        spec:
          keyProvider: local
        ```

    Switching a VM from `my-encryption-class-1` to `my-encryption-class-2` will have no effect since both `EncryptionClass` resources:

    * Reference the same, underlying key provider
    * Do not specify the key ID, thus rely on key generation

    Even though the lack of an explicit key ID means one will be generated, the logic to rekey a VM depends on comparing the key ID from the `EncryptionClass` with the one currently used by the VM. If the one from the `EncryptionClass` is empty, i.e. use a generated key, then it does not cause a rekey to occur since the VM already has a key.

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
| `status.crypto.hasVTPM` | True if the VM has a vTPM. |

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

The following is an example of a VM with a vTPM:

```yaml
status:
  crypto:
    encrypted:
    - Config
    providerID: my-key-provider-id
    keyID: my-key-id
    hasVTPM: true
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
apiVersion: vmoperator.vmware.com/v1alpha5
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
apiVersion: vmoperator.vmware.com/v1alpha5
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
apiVersion: vmoperator.vmware.com/v1alpha5
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
apiVersion: vmoperator.vmware.com/v1alpha5
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
| `status.storage.other.used` | The total storage space used by all of the VM's non-disk files, ex. snapshots, logs, etc. |
| `status.storage.other.disks` | The total storage space used by all of the VM's non-PVC disks. |
| `status.storage.requested.disks` | The total storage space requested by all of the VM's non-PVC disks. |
| `status.storage.total` | A sum of `status.storage.other.used` and `status.storage.requested.disks`. |

The value of `status.storage.total` is what is reported against the namespace's storage quota.

For example, the following illustrates the storage usage for a `VirtualMachine`:

```yaml
status:
  storage:
    total: 11Gi
    used:
      other: 1Gi
      disks: 5Gi
    requested:
      disks: 10Gi
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

Deployed `VirtualMachine` resources contribute to the total storage usage in a given namespace. The usage of a `VirtualMachine` is calculated based on the total usage of a VM's non disk files (ex. snapshots, logs, etc.) and the requested capacity of a VM's non-PVC disks. For example, consider the following VM:

| File | Used        | Requested | Counted against quota |
|------|:-----------:|:---------:|:---------------------:|
| Snapshots | `5Gi` | | Yes |
| Logs | `1Gi` | | Yes |
| Disk 1 | `1Gi` | | No |
| Disk 1 | | `10Gi` | Yes |
| PVC 1 | `5Gi` | | No |
| PVC 1 | | `50Gi` | No |

The value of `status.storage.total` will be `16Gi`, which is what will be counted against the namespace's storage quota.

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
      volumeMode: Block
      resources:
        requests:
          storage: 8Gi
      storageClassName: my-storage-class
    ```

2. Update the VM's `spec.volumes` field with a new volume that references the PVC:

    ```yaml
    apiVersion: vmoperator.vmware.com/v1alpha5
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
    | `name` | The name of the volume. For managed disks this is the name from `spec.volumes` and for classic disks this is the name of the underlying disk. |
    | `type` | The [type](#volume-type) of the attached volume, i.e. either `Classic` or `Managed` |
    | `limit` | The maximum amount of space that may be used by this volume. |
    | `requested` | The minimum amount of space that may be used by this volume. |
    | `used` | The total storage space occupied by the volume on disk. |

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
            <td style="text-align:center;">❌</td>
            <td style="text-align:center;">❌</td>
        </tr>
        <tr>
            <th style="text-align:center;"><code>Suspended</code></th>
            <td style="text-align:center;">✓</td>
            <td style="text-align:center;">✓<br /><em>if <code>spec.powerOffMode: Hard</code></em></td>
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

### Power On Check

The annotation `poweron.check.vmoperator.vmware.com/<COMPONENT>: <REASON>` may be applied when creating a new VM in order to prevent the VM from being powered on until the annotation is removed. The VM's `spec.powerState` field may still be set to `PoweredOn`, but the VM will not be powered on until the annotation is removed.

This allows external components to participate in a VM's power-on event. For example, there may be an external service that watches all `VirtualMachine` objects and creates new Computer objects in Active Directory, and needs to ensure the VM does not power on until its Computer object is created. This service can use a mutation webhook to apply the annotation to new VMs, only removing it once the Computer object is created in Active Directory.

There may be multiple instances of the annotation on a VM, hence its format:

```yaml
poweron.check.vmoperator.vmware.com/<COMPONENT>: <REASON>
```

For example:

```yaml
annotations:
  poweron.check.vmoperator.vmware.com/ad: "waiting on computer object"
  poweron.check.vmoperator.vmware.com/net: "waiting on net auth"
  poweron.check.vmoperator.vmware.com/app1: "waiting on app auth"
```

All check annotations must be removed before a VM can be powered on. This annotation is also subject to the following rules:

* May be applied by any user when creating a new VM.
* May be applied by privileged users when updating existing VMs.
* May be removed by privileged users when updating existing VMs.

Please note, non-privileged users cannot remove this annotation. If a non-privileged user accidentally creates a _new_ VM with this annotation, it would mean the VM cannot be powered on until a _privileged_ user removes the annotation. In this case, the non-privileged user's best recourse is to delete the VM and redeploy it without the annotation. The reasons non-privileged users can only _add_ the annotation and not remove it are:

* External services that want to ensure new VMs are subject to this annotation would use mutation webhooks, which act in the context of the end-user.
* External services also want to prevent the end-user from _removing_ the annotation until such time that some external condition is met that allows the VM to be powered on, at which point the external service can remove the annotation.

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

### Configuration

The optional field `spec.guestID` that may be used when deploying a VM to specify its guest operating system identifier. This field may also be updated when a VM is powered off. The value of this field is derived from the list of [supported guest identifiers](https://developer.broadcom.com/xapis/vsphere-web-services-api/latest/vim.vm.GuestOsDescriptor.GuestOsIdentifier.html). The following command may be used to query the currently supported guest identifiers for a given vSphere environment:

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

### Status

The configured guest OS ID and name may be gleamed from the VM's status as well:

```yaml
status:
  guest:
    guestFullName: Ubuntu Linux (64-bit)
    guestID: ubuntu64Guest
```

## CD-ROM

The `spec.hardware.cdrom` field may be used to mount one or more ISO images in a VM. Each entry in the `spec.hardware.cdrom` field must reference a unique `VirtualMachineImage` or `ClusterVirtualMachineImage` resource as backing. Multiple CD-ROM devices using the same backing image, regardless of image kind (namespace or cluster scope), are not allowed.

### CD-ROM Name

The `spec.hardware.cdrom[].name` field consists of at least two lowercase letters or digits of this CD-ROM device. It must be unique among all CD-ROM devices attached to the VM.

### CD-ROM Image

The `spec.hardware.cdrom[].image` field is the Kubernetes object reference to the ISO type `VirtualMachineImage` or `ClusterVirtualMachineImage` resource. The following commands may be used to discover the available ISO type images:

=== "Get ISO images for a namespace"

    ```shell
    kubectl get -n <NAMESPACE> vmi -l image.vmoperator.vmware.com/type=ISO
    ```

=== "Get ISO images for a cluster"

    ```shell
    kubectl get cvmi -l image.vmoperator.vmware.com/type=ISO
    ```

### CD-ROM Connection State

The `spec.hardware.cdrom[].connected` field controls the connection state of the CD-ROM device. When set to `true`, the device is added and connected to the VM, or updated to a connected state if already present but disconnected. When explicitly set to `false`, the device is added but remains disconnected from the VM, or updated to a disconnected state if already connected.

### CD-ROM Guest Control

The `spec.hardware.cdrom[].allowGuestControl` field controls the guest OS's ability to connect/disconnect the CD-ROM device. If set to `true` (default value), a web console connection may be used to connect/disconnect the CD-ROM device from within the guest OS.

For more information on the ISO VM workflow, please refer to the [Deploy a VM with ISO](../../../tutorials/deploy-vm/iso/) tutorial.

## Affinity

The optional field `spec.affinity` that may be used to define a set of affinity/anti-affinity scheduling rules for VMs.

**Important**: The `spec.affinity` field can only be used by VMs that belong to a VirtualMachineGroup. To use affinity rules, the VM must have a non-empty `spec.groupName` value that references a valid VirtualMachineGroup in the same namespace.

### Virtual Machine Affinity/Anti-affinity

The `spec.affinity.vmAffinity` and `spec.affinity.vmAntiAffinity` fields are used to define scheduling rules related to other VMs.

### Affinity/Anti-affinity verbs

Note: Please refer to the [Validation rules](#validation-rules) section below for supported affinity/anti-affinity verb combinations in the context of a zone/host.

#### RequiredDuringSchedulingPreferredDuringExecution

RequiredDuringSchedulingPreferredDuringExecution describes affinity requirements that must be met or the VM will not be scheduled. Additionally, it also describes the affinity requirements that should be met during run-time, but the VM can still be run if the requirements cannot be satisfied. This setting is available via `vmAffinity` and `vmAntiAffinity` fields in `spec.affinity`.

#### PreferredDuringSchedulingPreferredDuringExecution

PreferredDuringSchedulingPreferredDuringExecution describes affinity requirements that should be met, but the VM can still be scheduled if the requirement cannot be satisfied. The scheduler will prefer to schedule VMs that satisfy the affinity expressions specified by this field, but it may choose to violate one or more of the expressions. Additionally, it also describes the affinity requirements that should be met during run-time, but the VM can still be run if the requirements cannot be satisfied. This setting is available via `vmAffinity` and `vmAntiAffinity` fields in `spec.affinity`.

#### TopologyKey 
The `topologyKey` field is specified with the VM Affinity/Anti-Affinity based scheduling constraints to designate the scope of the rule. Commonly used values include:

* `kubernetes.io/hostname` -- The rule is executed in the context of a node/host.
* `topology.kubernetes.io/zone` -- This rule is executed in the context of a zone.

### Validation rules

The above affinity/anti-affinity settings are available with the following rules in place:

* When topology key is in the context of a zone, the only supported verbs are PreferredDuringSchedulingPreferredDuringExecution and RequiredDuringSchedulingPreferredDuringExecution.
* When topology key is in the context of a host, the only supported verbs are PreferredDuringSchedulingPreferredDuringExecution and RequiredDuringSchedulingPreferredDuringExecution for VM-VM node-level anti-affinity scheduling.


### Examples

The following is an example of VM affinity that uses the topologyKey field to indicate the VM affinity rule applies to the Zone scope:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachine
metadata:
  name: my-vm-1
  namespace: my-namespace-1
  labels:
    app: my-app-1
spec:
  groupName: my-vm-group  # Required for affinity rules
  affinity:
    vmAffinity:
      requiredDuringSchedulingPreferredDuringExecution:
      - labelSelector:
          matchLabels:
            app: my-app-1
        # The topology key is what designates the scope of the rule.
        # For example, "topologyKey: kubernetes.io/hostname" would
        # indicate the rule applies to nodes and not zones.
        # For more detail, please refer to
        # https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/.
        topologyKey: topology.kubernetes.io/zone
```

#### VM Anti-Affinity

The following example shows VM anti-affinity at the host level, ensuring VMs with the label `tier: database` are distributed across different hosts:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachine
metadata:
  name: database-vm-1
  namespace: my-namespace-1
  labels:
    tier: database
spec:
  groupName: database-group  # Required for affinity rules
  affinity:
    vmAntiAffinity:
      requiredDuringSchedulingPreferredDuringExecution:
      - labelSelector:
          matchLabels:
            tier: database
        topologyKey: kubernetes.io/hostname
```

## VirtualMachine Groups

VirtualMachine Groups provide a way to manage multiple VMs as a single unit, enabling coordinated operations and advanced placement capabilities.

### Group Membership

A VM becomes a member of a group by setting its `spec.groupName` field to the name of a VirtualMachineGroup in the same namespace:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachine
metadata:
  name: web-server-1
  namespace: my-namespace
spec:
  groupName: web-tier-group  # Join the web-tier-group
  className: my-vm-class
  imageName: ubuntu-22.04
  storageClass: my-storage-class
```

When a VM joins a group:

- An owner reference to the group is automatically added to the VM
- The VM appears in the group's `status.members` list
- The VM inherits the group's power state management
- The VM can use affinity rules (when `spec.groupName` is set)

### VirtualMachineGroup Resource

The VirtualMachineGroup resource defines how a collection of VMs should be managed:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachineGroup
metadata:
  name: web-tier-group
  namespace: my-namespace
spec:
  # Optional: This group can itself belong to another group
  groupName: application-group
  # Power state management for all group members
  powerState: PoweredOn
  # Boot order defines startup sequence
  bootOrder:
  - members:
    - name: database-vm
      kind: VirtualMachine
    powerOnDelay: 10s
  - members:
    - name: app-server-1
      kind: VirtualMachine
    - name: app-server-2
      kind: VirtualMachine
    powerOnDelay: 5s
  - members:
    - name: web-server-1
      kind: VirtualMachine
    - name: web-server-2
      kind: VirtualMachine
```

### Boot Ordering

Boot ordering allows you to control the startup sequence of VMs within a group:

- **Boot groups**: VMs are organized into sequential boot groups
- **Parallel startup**: All members within a boot group start simultaneously
- **Delays**: Optional delays between boot groups ensure dependencies are ready
- **Power off**: When powering off, all members stop immediately without delays

Example use cases:
- Starting database servers before application servers
- Ensuring infrastructure services are ready before dependent workloads
- Implementing multi-tier application startup sequences

### Nested Groups

VirtualMachineGroups can be hierarchical - a group can belong to another group by setting its own `spec.groupName` field:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachineGroup
metadata:
  name: database-group
  namespace: my-namespace
spec:
  groupName: application-group  # This group belongs to application-group
  powerState: PoweredOn
```

This enables:

- Multi-level organizational structures
- Cascading power management
- Complex application topologies

### Power State Management

Groups provide coordinated power state control:

- **Group power state**: Setting `spec.powerState` on a group affects all members
- **Synchronized operations**: Members are powered on/off according to boot order
- **State inheritance**: New members added to a powered-on group will be powered on

### Placement Decisions

Groups can influence VM placement through:

- **Affinity rules**: VMs in a group can define affinity/anti-affinity rules (requires `spec.groupName` to be set)
- **Placement status**: The group's `status.members[].placement` shows placement recommendations
- **Datastore recommendations**: Optimal datastore placement for group members

### Group Status

The VirtualMachineGroup status provides visibility into:

```yaml
status:
  members:
  - name: web-server-1
    kind: VirtualMachine
    conditions:
    - type: GroupLinked
      status: "True"
    - type: PowerStateSynced
      status: "True"
    - type: PlacementReady
      status: "True"
    placement:
      datastores:
      - name: datastore1
        id: datastore-123
  - name: database-vm
    kind: VirtualMachine
    conditions:
    - type: GroupLinked
      status: "True"
  lastUpdatedPowerStateTime: "2024-01-15T10:30:00Z"
  conditions:
  - type: Ready
    status: "True"
```

### Conditions

Groups and their members report various conditions:

**Group Conditions**:
- `Ready`: The group and all its members are in the desired state

**Member Conditions**:
- `GroupLinked`: Member exists and has `spec.groupName` set to this group
- `PowerStateSynced`: Member's power state matches the group's desired state
- `PlacementReady`: Placement decision is available for this member

### Best Practices

1. **Naming conventions**: Use descriptive group names that indicate purpose (e.g., `web-tier-group`, `database-cluster`)

1. **Boot order design**:
    - Place independent services in the same boot group
    - Use delays sparingly to avoid slow startup times
    - Test boot sequences to ensure dependencies are met

1. **Affinity planning**:
    - Only VMs with `spec.groupName` set can use affinity rules
    - Design affinity rules at the group level for consistency
    - Consider both zone and host-level distribution

1. **Power management**:
    - Avoid setting individual VM power states when using groups
    - Use group power state for coordinated control
    - Monitor member conditions to ensure synchronization

1. **Group hierarchy**:
    - Keep nesting levels reasonable (2-3 levels max)
    - Use hierarchy to model real application relationships
    - Ensure parent groups account for child group dependencies

## VirtualMachine Snapshots

VirtualMachine Snapshots can be taken by creating a VirtualMachineSnapshot CR.
This uses the provider APIs to take a snapshot of the VM

### Creating a Snapshot

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachineSnapshot
metadata:
  name: snap-2
spec:
  description: "Snapshot of my-vm with memory"
  vmName: my-vm
  memory: true
```

### Reverting to a Snapshot

Once a snapshot is created, the VM can be reverted to the snapshot by
specifying `spec.currentSnapshotName` to the desired snapshot.

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachine
metadata:
  name: my-vm
spec:
  currentSnapshotName: snap-1
```

The VM would be reverted to the snapshotted state if the revert operation
is succesful. After a succesful revert operation, `spec.currentSnapshotName`
would become unset.

### Further Information

See more information about the VirtualMachineSnapshot resource:

- [VirtualMachineSnapshot concept](./vm-snapshot.md) documentation
- VirtualMachineSnapshot API reference
