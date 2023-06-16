# VirtualMachineClass

// TODO ([github.com/vmware-tanzu/vm-operator#95](https://github.com/vmware-tanzu/vm-operator/issues/95))

_VirtualMachineClasses_ (VM Class) represent a collection of virtual hardware that is used to realize a new VirtualMachine (VM).

## ControllerName

The field `spec.controllerName` in a VM Class indicates the name of the controller responsible for reconciling `VirtualMachine` resources created from the VM Class. The value `vmoperator.vmware.com/vsphere` indicates the default `VirtualMachine` controller is used. VM Classes that do not have this field set or set to an empty value behave as if the field is set to the value returned from the environment variable `DEFAULT_VM_CLASS_CONTROLLER_NAME`. If this environment variable is empty, it defaults to `vmoperator.vmware.com/vsphere`.
