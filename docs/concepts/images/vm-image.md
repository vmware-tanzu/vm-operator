# VirtualMachineImage

// TODO ([github.com/vmware-tanzu/vm-operator#109](https://github.com/vmware-tanzu/vm-operator/issues/109))


## Image Scope

There are two types of VM image resources, the `ClusterVirtualMachineImage` and `VirtualMachineImage`. The former is a cluster-scoped resource, while the latter is a namespace-scoped resource. Other than that, the two resources are exactly the same.


## Image Names

Prior to vSphere 8.0U2, the name of a VM image resource was derived from the name of a Content Library item. For example, if a Content Library item was named `photonos-5-x64`, then its corresponding  `VirtualMachineImage` resource would also be named `photonos-5-x64`. This caused a problem if there library items with the same name from different libraries. With the exception of the first library item encountered, all subsequent library items would have randomly generated data appended to their corresponding Kubernetes resource names to ensure they were unique. In vSphere 8.0U2+, with the introduction of the Image Registry API and potential for global image catalogs, image names needed to be both unique _and_ deterministic, hence:

```
vmi-0123456789ABCDEFG
```

The above value is referred to as a _VMI ID_, where _VMI_ stands for _Virtual Machine Image_. No matter the source of a VM image, all images have unique, predictable VMI IDs. If the source of a VM image is Content Library, then the VMI ID is constructed using the following steps:

1. Remove any `-` characters from the Content Library item's UUID
2. Calculate the sha1sum of the value from the previous step
3. Take the first 17 characters from the value from the previous step
4. Append the value from the previous step to `vmi-`

For example, if the Content Library item's UUID is `e1968c25-dd84-4506-8dc7-9beacb6b688e`, then the VMI ID is `vmi-0a0044d7c690bcbea`, for example:

1. Remove any `-` characters: `e1968c25dd8445068dc79beacb6b688e`.
1. Get the sha1sum: `0a0044d7c690bcbea07c9b49efc9f743479490e5`.
1. First 17 characters: `0a0044d7c690bcbea`.
1. Create the VMI ID: `vmi-0a0044d7c690bcbea`.


## Name Resolution

When a `VirtualMachine` resource's field `spec.imageName` is set to a VMI ID, the value is resolved to the `VirtualMachineImage` or `ClusterVirtualMachineImage` with that name. It is also possible to specify images based on their _friendly_ name.

!!! warning "Friendly name resolution"

    Please note that while resolving VM images based on their friendly name was merged into VM Operator with [github.com/vmware-tanzu/vm-operator#214](https://github.com/vmware-tanzu/vm-operator/issues/214), the feature is not yet part of a shipping vSphere release.

For example, if `vmi-0a0044d7c690bcbea` refers to an image with a friendly name of `photonos-5-x64`, then a user could also specify that value for `spec.imageName` as long as the following is true:

* There is no other `VirtualMachineImage` in the same namespace with that friendly name.
* There is no other `ClusterVirtualMachineImage` with the same friendly name.

If the friendly name unambiguously resolves to the distinct, VM image `vmi-0a0044d7c690bcbea`, then a mutation webhook replaces `spec.imageName: photonos-5-x64` with `spec.imageName: vmi-0a0044d7c690bcbea`. If the friendly name resolves to multiple or no VM images, then the mutation webhook denies the request and outputs an error message accordingly.


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