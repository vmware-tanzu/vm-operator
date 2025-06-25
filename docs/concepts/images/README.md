# Images

## The VM Operator image model

VM Operator uses virtual machine images as the foundation for deploying VirtualMachine workloads. Images provide the base operating system, applications, and configurations that define the initial state of a VM. VM Operator supports both consuming existing images and creating new images from running VMs.

VM Operator provides two types of image resources:

* **`ClusterVirtualMachineImage`**: Cluster-scoped images available to all namespaces
* **`VirtualMachineImage`**: Namespace-scoped images available only within a specific namespace

Images are typically sourced from vSphere Content Libraries, which serve as centralized repositories for VM templates, ISO files, and other virtual machine artifacts. VM Operator automatically discovers and synchronizes images from configured Content Libraries, making them available as Kubernetes resources.

## Image identification and resolution

VM Operator uses a unique identification system for images:

* **VMI ID**: A deterministic, unique identifier (e.g., `vmi-0a0044d7c690bcbea`) generated from the source Content Library item
* **Display Name**: A human-readable name that can be used for image resolution when unambiguous

When specifying an image in a VirtualMachine resource, you can use either the VMI ID for precise identification or the display name for convenience, provided it uniquely resolves to a single image.

## Image lifecycle management

VM Operator supports the complete image lifecycle:

* **Discovery**: Automatic synchronization of images from Content Libraries
* **Publishing**: Creating new images from existing VirtualMachine instances using the VirtualMachinePublishRequest API
* **Versioning**: Managing multiple versions of images with descriptive metadata
* **Distribution**: Sharing images across namespaces and clusters

## What's Next

This section provides information about image resources and concepts, such as:

* [`VirtualMachineImage`](./vm-image.md) - Core image resource types and management
* [Publishing VM Images](./pub-vm-image.md) - Creating custom images from VirtualMachines using the VirtualMachinePublishRequest API
