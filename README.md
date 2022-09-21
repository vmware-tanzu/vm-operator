# VM Operator for Kubernetes

#### Useful links
- [Quick Start](docs/quick-start-guide.md)
- [vSphere documentation](https://docs.vmware.com/en/VMware-vSphere/7.0/vmware-vsphere-with-tanzu/GUID-152BE7D2-E227-4DAA-B527-557B564D9718.html)

## What is VM Operator?

The VM Operator project is an Operator to add Virtual Machine management to Kubernetes, focused on vSphere as the initial target.

VM Operator is currently the lowest plumbing layer in the vSphere 7 with Tanzu release and it manages all VM lifecycle
for Tanzu Kubernetes Grid (TKG) clusters. It uses a declarative model in which custom resource types such as
VirtualMachine, VirtualMachineClass and VirtualMachineImage are reconciled by controllers which then drive the 
vSphere APIs to ensure that the desired state matches the actual state. 

The API for VM Operator is in a separate project [here](https://github.com/vmware-tanzu/vm-operator-api)

Having demonstrated its value in vSphere with Tanzu, we are committed to making VM Operator available as its own service
so that it can be used in conjunction with other Kubernetes services that need to manage VMs.

## Getting Started

If you are passionate about VMs in Kubernetes, are interested in learning about our roadmap or have
ideas about the project, we'd love to hear from you!

Please see our [Quick Start Guide](docs/quick-start-guide.md) for details on building and testing the project

## Community
- Find us on Slack at [#ug-vmware](https://kubernetes.slack.com/messages/ug-vmware)
- Regular working group [meetings](https://docs.google.com/document/d/1B2oUAuNbYc8nXjRrN353Pt-mDPtwyLrBO3cok7BfV4s/edit?usp=sharing)
- Reach out directly via VMware liaison [Ellen Mei](meie@vmware.com)
