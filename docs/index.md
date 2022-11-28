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

### Demo

View the [demo](./user-guide/examples/demo.md) for more information on ways to use VM Operator to manage VMs.

## Getting Help

Having issues? No worries, let's figure it out together.

### Debug

`// TODO(akutz)`

### Supported Platforms

This section lists the platforms on which VM Operator is supported:

* [Supervisor](./user-guide/platforms/supervisor.md)

### GitHub and Slack

If a little extra help is needed, please don't hesitate to use [GitHub issues](https://github.com/vmware-tanzu/vm-operator/issues).
