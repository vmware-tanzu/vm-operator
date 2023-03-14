# VM Operator

Self-service manage your virtual infrastructure...

---
## Getting Started

A virtual machine may be deployed on vSphere Supervisor by applying the following YAML with `kubectl`:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha1
kind: VirtualMachine
metadata:
  name:      my-vm
  namespace: my-namespace
spec:
  className:    small
  imageName:    ubuntu-2210
  storageClass: iscsi
```

## User Guide
 
The user guide provides information on:

* [Bootstrap Providers](./user-guide/bootstrap.md) -- how to bootstrap the VM's guest
* [Supported Platforms](./user-guide/platforms.md) -- where/how VM Operator can be deployed

## Supported Platforms

This section lists the platforms on which VM Operator is supported:

* [Supervisor](./user-guide/platforms/supervisor.md)

## Getting Help

Having issues? No worries, let's figure it out together. Please don't hesitate to use [GitHub issues](https://github.com/vmware-tanzu/vm-operator/issues).
