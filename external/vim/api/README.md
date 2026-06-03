# vim.vmware.com APIs

This module contains the VIM Kubernetes APIs.

* [`ConfigTarget`](./v1alpha1/config_target_types.go) is a cluster-scoped resource surfaced for each vSphere cluster visible to Supervisor. The resource's status provides information for all of the cluster's physical attributes and hardware available to provision workloads.
* [`VirtualMachineConfigOptions`](./v1alpha1/virtualmachine_config_options_types.go) is a cluster-scoped resource surfaced for each distinct hardware version. Each resource then contains all of the virtual hardware and configuration options for the object's respective hardware version, for each guest ID supported.
* [`VirtualMachineGuestOptions`](./v1alpha1/virtualmachine_guest_options_types.go) is a cluster-scoped resource surfaced for each distinct guest ID. Each resource then contains all of the information about the guest ID, for each hardware version that supports it.
* [`VirtualMachineConfigPolicy`](./v1alpha1/virtualmachine_config_policy.go) is a namespace-scoped resource surfaced for each zone in a given namespace. It allows administrators to control the physical resources and capabilities available to deploy VMs.
