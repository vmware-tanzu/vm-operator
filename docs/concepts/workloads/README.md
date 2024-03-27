# Workloads

A workload is an application running on Kubernetes. Whether your workload is a single component or several that work together, on Kubernetes you run it inside a set of [VirtualMachines](./vm.md). In Kubernetes, a `VirtualMachine` represents a Virtual Machine running on an underlying, VMware hypervisor, such as vSphere.

VM Operator virtual machines (VM) have a defined lifecycle. For example, on vSphere Supervisor, VMs are scheduled on nodes that are represent underlying ESXi hosts. Once a VM is scheduled and running on a node, a critical fault on the node where that VM is running does not necessarily cause the VMs scheduled on that node to fail. If the underlying platform is vSphere, for example, the VMs on that node will be vMotioned to other, compatible nodes in the Supervisor. 

## What's Next

This section provides information about workload resources, such as:

* [`VirualMachine`](./vm.md)
* [`VirtualMachine` controller](./vm-controller.md)
* [`VirualMachineClass`](./vm-class.md)
* [`WebConsoleRequest`](./vm-web-console.md)

In addition to the workload resources themselves, there is documentation related to broader topics related to workloads:

* [Customizing a Guest](./guest.md)
