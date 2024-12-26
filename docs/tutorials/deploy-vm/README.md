# Deploy a VM

This page reviews the different components, workflows, and decisions related to deploying a VM with VM Operator:

## The VirtualMachine API

```yaml
apiVersion: vmoperator.vmware.com/v1alpha3 # (1)
kind: VirtualMachine # (2)
metadata:
  name:      my-vm # (3)
  namespace: my-namespace # (4)
spec:
  className:    my-vm-class # (5)
  imageName:    vmi-0a0044d7c690bcbea # (6)
  storageClass: my-storage-class # (7)
  bootstrap: # (8)
    cloudInit:
      cloudConfig: {}
  cdrom: # (9)
    - name: cdrom1
      image:
        name: vmi-0a0044d7c690bcbea
        kind: VirtualMachineImage
      connected: true
      allowGuestControl: true
```

1.  :wave: The field `apiVersion` indicates the resource's schema, ex. `vmoperator.vmware.com`, and version, ex.`v1alpha2`.

2.  :wave: The field `kind` specifies the kind of resource, ex. `VirtualMachine`.

3.  :wave: The field `metadata.name` is used to uniquely identify an instance of an API resource in a given Kubernetes namespace.

4.  :wave: The field `metadata.namespace` denotes in which Kubernetes namespace the API resource is located.

5.  :wave: The field `spec.className` refers to the name of the `VirtualMachineClass` resource that provides the hardware configuration when deploying a VM.

    The `VirtualMachineClass` API is cluster-scoped, and the following command may be used to print all of the VM classes on a cluster:

    ```shell
    kubectl get vmclass
    ```

    However, access to these resources is per-namespace. To determine the names of the VM classes that may be used in a given namespace, use the following command:

    ```shell
    kubectl get -n <NAMESPACE> vmclassbinding
    ```

6.  :wave: The field `spec.imageName` refers to the name of the `ClusterVirtualMachineImage` or `VirtualMachineImage` resource that provides the disk(s) when deploying a VM.

    * If there is a `ClusterVirtualMachineImage` resource with the specified name, the cluster-scoped resource is used, otherwise...
    * If there is a `VirtualMachineImage` resource in the same namespace as the VM being deployed, the namespace-scoped resource is used.

    The following command may be used to print a list of the images available to the entire cluster:

    ```shell
    kubectl get clustervmimage
    ```

    Whereas this command may be used to print a list of images available to a given namespace:

    ```shell
    kubectl get -n <NAMESPACE> vmimage
    ```

7.  :wave: The field `spec.storageClass` refers to the Kubernetes [storage class](https://kubernetes.io/docs/concepts/storage/storage-classes/) used to configure the storage for the VM.

    The following command may be used to print a list of the storage classes available to the entire cluster:

    ```shell
    kubectl get storageclass
    ```

8.  :wave: The field `spec.bootstrap`, and the fields inside of it, are used to configure the VM's [bootstrap provider](#bootstrap-provider).

9.  :wave: The field `spec.cdrom` is used to configure the VM's [CD-ROM](../../concepts/workloads/vm/#cd-rom) devices to mount ISO images.

## Bootstrap Provider

There are a number of methods that may be used to bootstrap a virtual machine's (VM) guest operating system:

| Provider                    | Network Config                | Linux | Windows | Description |
|-----------------------------|-------------------------------|:-----:|:-------:|-------------|
| [Cloud-Init](#cloud-init)   | [Cloud-Init Network v2](https://cloudinit.readthedocs.io/en/latest/reference/network-config-format-v2.html) |   ✓   |     ✓    | The industry standard, multi-distro method for cross-platform, cloud instance initialization with modern, VM images |
| [Sysprep](#sysprep)         | [Guest OS Customization](https://vdc-download.vmware.com/vmwb-repository/dcr-public/c476b64b-c93c-4b21-9d76-be14da0148f9/04ca12ad-59b9-4e1c-8232-fd3d4276e52c/SDK/vsphere-ws/docs/ReferenceGuide/vim.vm.customization.Specification.html) (GOSC) |       |     ✓    | Microsoft Sysprep is used by VMware to customize Windows images on first-boot |
| [vAppConfig](#vappconfig)   | Bespoke                       |   ✓   |         | For images with bespoke, bootstrap engines driven by vAppConfig properties |

Please refer to the documentation for [bootstrap providers](./../../concepts/workloads/guest.md) for more information.