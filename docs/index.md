# VM Operator

Self-service manage your virtual infrastructure...

---

## Getting Started

Deploying a VM and customizing its guest is easy with VM Operator:

=== "VirtualMachine"

    ```yaml
    apiVersion: vmoperator.vmware.com/v1alpha1
    kind: VirtualMachine
    metadata:
      name:      my-vm
      namespace: my-namespace
    spec:
      className:    small # (1)
      imageName:    ubuntu-2210 # (2)
      storageClass: iscsi # (3)
      vmMetadata:
        transport: CloudInit
        secretName: my-vm-bootstrap-data
    ```

    1.  :wave: Use the following command to determine the name(s) of the available classes on your cluster:
        
        ```shell
        kubectl get virtualmachineclass
        ```

    2.  :wave: Use the following command to determine the name(s) of the available images on your cluster:
        
        ```shell
        kubectl get virtualmachineimage
        ```

    3.  :wave: Use the following command to determine the name(s) of the available [storage classes](https://kubernetes.io/docs/concepts/storage/storage-classes/) on your cluster:
        
        ```shell
        kubectl get storageclass
        ```

=== "CloudConfig"

    ``` yaml
    apiVersion: v1
    kind: Secret
    metadata:
      name:      my-vm-bootstrap-data
      namespace: my-namespace
    stringData:
      user-data: | # (1)
        #cloud-config
        users:
        - default
      ssh-public-keys: | # (2)
        ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDSL7uWGj...
        ssh-rsa bHJlYWR5IHNhdGlzZmllZDogTWFya3VwU2FmZT...
    ```

    1. :wave: Ensures the guest's default user is enabled.

    2. :wave: A [line-delimited](https://github.com/canonical/cloud-init/blob/main/cloudinit/sources/__init__.py#L931) list of public, SSH keys that will be added to the guest's default user.

For those interested in learning more about the VM deployment and customization workflow, please see the [bootstrap documentation](./user-guide/bootstrap.md). 

## Bootstrap Providers

| Provider                    | Supported    | Network Config                | Linux | Windows | Description |
|-----------------------------|:------------:|-------------------------------|:-----:|:-------:|-------------|
| [Cloud-Init](./user-guide/deployvm.md#cloud-init)   |       ✓      | [Cloud-Init Network v2](https://cloudinit.readthedocs.io/en/latest/reference/network-config-format-v2.html) |   ✓   |     ✓    | The industry standard, multi-distro method for cross-platform, cloud instance initialization with modern, VM images |
| [vAppConfig](./user-guide/deployvm.md#vappconfig)   |       ✓      | Bespoke                       |   ✓   |         | For images with bespoke, bootstrap engines driven by vAppConfig properties |

## Recommended Images

There are no restrictions on the images that can be deployed by VM Operator. However, for users wanting to try things out for themselves, here are a few images the project's developers use on a daily basis:

| Image | Arch | Download |
|-------|:----:|:--------:|
| :fontawesome-solid-clone: Photon OS 4, Rev 2 | amd64 | [OVA :fontawesome-solid-cloud-arrow-down:](https://packages.vmware.com/photon/4.0/Rev2/ova/photon-ova_uefi-4.0-c001795b80.ova) |
| :fontawesome-solid-clone: Photon OS 3, Rev 3, Update 1 | amd64 | [OVA :fontawesome-solid-cloud-arrow-down:](https://packages.vmware.com/photon/3.0/Rev3/ova/Update1/photon-hw13_uefi-3.0-913b49438.ova) |
| :fontawesome-brands-ubuntu: Ubuntu 22.10 Server (Kinetic Kudo) | amd64 | [OVA :fontawesome-solid-cloud-arrow-down:](https://cloud-images.ubuntu.com/releases/22.10/release-20230302/ubuntu-22.10-server-cloudimg-amd64.ova) |
| :fontawesome-brands-ubuntu: Ubuntu 22.04 Server (Jammy Jellyfish) | amd64 | [OVA :fontawesome-solid-cloud-arrow-down:](https://cloud-images.ubuntu.com/releases/22.04/release-20230302/ubuntu-22.04-server-cloudimg-amd64.ova) |

!!! note "Bring your own image..."

    The above list is by no means exhaustive or restrictive -- we _want_ users to bring their own images!

## Documentation

For those interested in learning more about VM Operator, this site is divided into two areas:

1. [User Guide](./user-guide/deployvm.md) how to deploy and customize VMs
2. [Developer Guide](./dev-guide/project-guidelines.md) how to contribute docs, fixes, features, & more

## Getting Help

Having issues? No worries, let's figure it out together. Please don't hesitate to use [GitHub issues](https://github.com/vmware-tanzu/vm-operator/issues).
