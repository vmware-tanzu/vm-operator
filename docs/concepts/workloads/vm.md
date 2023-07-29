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

Some of a VM's hardware resources are derived, and there are some that may be influenced directly by a user.

// TODO ([github.com/vmware-tanzu/vm-operator#105](https://github.com/vmware-tanzu/vm-operator/issues/105))

### CPU and Memory

### Networking

### Storage

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

