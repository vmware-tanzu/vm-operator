# Deploy a VM with vAppConfig

The vAppConfig bootstrap method is useful for legacy VM images that rely on bespoke, boot-time processes that leverage vAppConfig properties for customizing a guest. This method also supports properties specified with Golang-style template strings in order to use information not known ahead of time, such as the networking configuration, into the guest via vApp properties.

## Example

The following example showcases a `VirtualMachine` resource that specifies one or more vApp properties used to bootstrap a guest:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachine
metadata:
  name:      my-vm
  namespace: my-namespace
spec:
  className:    my-vm-class
  imageName:    vmi-0a0044d7c690bcbea
  storageClass: my-storage-class
  bootstrap:
    vAppConfig:
      properties:
      - key: nameservers
        value:
          value: "{{ (index .V1alpha5.Net.Nameservers 0) }}"
      - key: management_ip
        value:
          value: "{{ (index (index .V1alpha5.Net.Devices 0).IPAddresses 0) }}"
      - key: hostname
        value:
          value: "{{ .V1alpha5.VM.Name }}"
      - key: management_gateway
        value:
          value: "{{ (index .V1alpha5.Net.Devices 0).Gateway4 }}"
```

For more information on the templating used, please refer to the [documentation](./../../concepts/workloads/guest.md#vappconfig) for the vAppConfig bootstrap provider.
