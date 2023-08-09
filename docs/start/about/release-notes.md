# Release Notes

## v1.8.4 (2023/08/09)

An addendum to the previous release to fix misaligned headings in the release notes.

## v1.8.3 (2023/08/09)

This release introduces several documentation updates.

### Documentation

* Deploying a VM with the vAppConfig bootstrap provider ([\#175](https://github.com/vmware-tanzu/vm-operator/pull/175))
* Deploying a VM with the Cloud-Init bootstrap provider ([\#183](https://github.com/vmware-tanzu/vm-operator/pull/183))
* Information about a VM resources such as CPU, memory, storage ([\#189](https://github.com/vmware-tanzu/vm-operator/pull/189))
* A call-out to address the incomplete Sysprep state issue ([\#186](https://github.com/vmware-tanzu/vm-operator/pull/186))
* How to use `kubectl vsphere vm web-console` ([\#196](https://github.com/vmware-tanzu/vm-operator/pull/196))


## v1.8.2 (2023/07/28)

This release includes changes related to the upcoming v1alpha2 schema, power state enhancements, support for deploying Windows, and the ability for a `VirtualMachineClass` to contain all of a vSphere VM's hardware and configuration options.

### Coming Soon

* The not-yet-enabled-but-now-in-repo beginnings of the VM Operator v1alpha2 API ([the changes so far...](https://github.com/vmware-tanzu/vm-operator/pulls?q=is%3Apr+v1a2+v1alpha2))!

### New Features

* A `VirtualMachineClass` now supports all of the hardware and configuration options of a vSphere VM!
* Support for deploying Windows VMs using Sysprep ([\#83](https://github.com/vmware-tanzu/vm-operator/pull/83), [\#136](https://github.com/vmware-tanzu/vm-operator/pull/136), [\#149](https://github.com/vmware-tanzu/vm-operator/pull/149)).
* The ability to suspend a VM, either using suspend or standby, by setting the VM's `spec.powerState` field to `suspended` ([\#152](https://github.com/vmware-tanzu/vm-operator/pull/152), [\#154](https://github.com/vmware-tanzu/vm-operator/pull/154)).
* Users can now gracefully shutdown a VM using VM tools instead of halting a VM, the equivalent of yanking a system's power cable, by setting `spec.powerOffMode` to `soft` or `trySoft` ([\#152](https://github.com/vmware-tanzu/vm-operator/pull/152)).
* Restart a VM or reboot its guest by setting `spec.nextRestartTime` to the value `now`. The VM will be power cycled exactly once until the next time `spec.nextRestartTime` is set to `now` ([\#155](https://github.com/vmware-tanzu/vm-operator/pull/155)).
* Support for multiple, concurrent controllers for reconciling a VirtualMachine resource via the new `spec.controllerName` field in a `VirtualMachineClass` ([\#163](https://github.com/vmware-tanzu/vm-operator/pull/163)).

### Bug Fixes

* A `VirtualMachineService` with an empty selector no longer considers all `VirtualMachine` resources ([\#137](https://github.com/vmware-tanzu/vm-operator/pull/137)).

### Known Issues

* Deploying a `VirtualMachine` with an encryption storage policy is not currently supported. This means it is not possible for a VM Service VM to have encrypted boot disks. However, using VM Class that has a vTPM will still result in a VM with encryption less its boot disks.
