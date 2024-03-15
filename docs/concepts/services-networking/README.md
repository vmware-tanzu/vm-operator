# Services, Load Balancing, and Networking

## The VM Operator network model

By default, every virtual machine (VM) receives its own, unique, IP address. It is possible to deploy VM workloads that have no networking capabilities whatsoever, but this would be an explicit choice.

VM Operator imposes the following fundamental requirements on any networking implementation (barring any intentional network segmentation policies):

* VMs in the same namespace are *not* ensured direct network connectivity
* VMs on the same node are *not* ensured direct network connectivity
* VMs in the same namespace, or in different namespaces, can communicate directly if both VMs are connected to the same network
* Depending on the network topology, VMs may not be able to communicate directly with the Kubernetes cluster network, i.e. a VM accessing a pod via its cluster IP

Unlike the [Kubernetes networking model](https://kubernetes.io/docs/concepts/services-networking/), VMs running on a node do not necessarily share a common network with the node, nor are any ports exposed from a VM exposed on the node where the workload is scheduled.

VM Operator networking addresses two concerns:

* The [`VirtualMachineService`](./vm-service.md) API allows users to expose an application running in a VM workload to other VMs in other namespaces or to pod workloads in the same or other namespaces
* The `VirtualMachine` API simplifies bootstrapping the [guest's network configuration](./guest-net-config.md)

## What's Next

This section provides information about networking resources and concepts, such as:

* [`VirtualMachineService`](./vm-service.md)
* [Guest networking](./guest-net-config.md)
