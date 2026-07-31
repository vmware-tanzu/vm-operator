# vim.vmware.com

This Go module contains the following directories:

* [`./api`](./api) -- A Go module that contains the VIM Kubernetes APIs.
* [`./pkg`](./pkg) -- Utilities for converting to/from the VIM Kubernetes APIs and the traditional VIM types.

## Documentation

* [Integration guide](./doc/integration-guide.md) -- the contract for teams consuming these APIs from outside VM Operator: what each resource guarantees, what is not populated yet, how `VirtualMachineConfigPolicy` is enforced, and what will reject a request.
* [Deploying a VM](./doc/deploying-a-vm.md) -- how `kubectl` users and graphical applications leverage the VIM Kubernetes environment browser APIs to deploy workloads.
* [Controller workflows](./doc/controller-workflows.md) -- how zone discovery fans out into cluster-scoped config metadata and guest option resources.