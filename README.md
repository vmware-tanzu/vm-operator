# VM Operator for Kubernetes

#### Useful links
- [Quick Start](docs/quick-start-guide.md)

## What is VM Operator?

The VM Operator project is an Operator to add Virtual Machine management to Kubernetes, focused on vSphere as the initial target.

VM Operator is currently the lowest plumbing layer in the vSphere 7 with Tanzu release and it manages all VM lifecycle
for Tanzu Kubernetes Grid (TKG) clusters. It uses a declarative model in which custom resource types such as
VirtualMachine, VirtualMachineClass and VirtualMachineImage are reconciled by controllers which then drive the 
vSphere APIs to ensure that the desired state matches the actual state. 

The API for VM Operator is in a separate project [here](https://github.com/vmware-tanzu/vm-operator-api)

Having demonstrated its value in vSphere with Tanzu, we are committed to making VM Operator available as its own service
so that it can be used in conjunction with other Kubernetes services that need to manage VMs.

## Contributing

If you are passionate about VMs in Kubernetes, are interested in learning about our roadmap or have
ideas about the project, we'd love to hear from you!

More information [here](CONTRIBUTING.md)

## Getting Started

Please see our [Quick Start Guide](docs/quick-start-guide.md) for details on building and testing the project

## Maintainers

Current list of project maintainers [here](MAINTAINERS.md)

## License

VM Operator API is licensed under the [Apache License, version 2.0](LICENSE.txt)
