# Bootstrap Providers

Be our guest, be our guest...

---

## Overview

There are a number of methods that may be used to bootstrap a virtual machine's (VM) guest operating system:

| Provider                    | Supported    | Network Config                | Linux | Windows | Description |
|-----------------------------|:------------:|-------------------------------|:-----:|:-------:|-------------|
| [Cloud-Init](#cloud-init)   |       ✓      | [Cloud-Init Network v2](https://cloudinit.readthedocs.io/en/latest/reference/network-config-format-v2.html) |   ✓   |     ✓    | The industry standard, multi-distro method for cross-platform, cloud instance initialization with modern, VM images |
| [vAppConfig](#vappconfig)   |       ✓      | Bespoke                       |   ✓   |         | For images with bespoke, bootstrap engines driven by vAppConfig properties |
| [OvfEnv](#ovfenv)           | _deprecated_ | [Guest OS Customization](https://vdc-download.vmware.com/vmwb-repository/dcr-public/c476b64b-c93c-4b21-9d76-be14da0148f9/04ca12ad-59b9-4e1c-8232-fd3d4276e52c/SDK/vsphere-ws/docs/ReferenceGuide/vim.vm.customization.Specification.html) (GOSC) |   ✓   |         | A combination of GOSC and Cloud-Init user-data |
| [ExtraConfig](#extraconfig) | _deprecated_ | GOSC                          |   ✓   |         | For images with bespoke, bootstrap engines that rely on Guest Info data |

!!! note "`ConfigMap` or `Secret`"

    When VM Operator was first released, the only way to store bootstrap data was via a `ConfigMap` resource. While this still works, it is not recommended as data stored in a `ConfigMap` is not encrypted at rest. Instead, it is recommended users switch to using `Secret` resources for storing bootstrap data.

    Please note that the choice of a `ConfigMap` or `Secret` _in no way_ impacts the choice of the bootstrap provider. The two resources fulfill the same obligation -- they are simply a way to store the bootstrap data.

## Cloud-Init

[Cloud-Init](https://cloudinit.readthedocs.io/en/latest/) is widely recognized as the de facto method for bootstrapping modern VM instances on hyperscalers, including Tanzu with VM Operator. For example, the following YAML provisions a new VM, using Cloud-Init to:

* add a custom user
* execute commands on boot
* write files

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
  vmMetadata:
    transport: CloudInit
    secretName: my-vm-bootstrap-data
---
apiVersion: v1
kind: Secret
metadata:
  name:      my-vm-bootstrap-data
  namespace: my-namespace
stringData:
  user-data: |
    #cloud-config
    users:
    - default
    - name: akutz
      primary_group: akutz
      groups: users
      ssh_authorized_keys:
      - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDSL7uWGj...
    runcmd:
    - "ls /"
    - [ "ls", "-a", "-l", "/" ]
    write_files:
    - path: /etc/my-plaintext
      permissions: '0644'
      owner: root:root
      content: |
        Hello, world.
```

The data in the above `Secret` is called the Cloud-Init _Cloud Config_. For more information on the Cloud-Init Cloud Config format, please see its [official documentation](https://cloudinit.readthedocs.io/en/latest/reference/examples.html).

!!! note "Windows and Cloud-Init"

    It is possible to use the Cloud-Init bootstrap provider to deploy a Windows image if it contains [Cloudbase-Init](https://cloudbase.it/cloudbase-init/), the Windows port of Cloud-Init.

## vAppConfig

The vAppConfig bootstrap method is useful for legacy, VM images that rely on bespoke, boot-time processes that leverage vAppConfig properties for customizing a guest.

## Deprecated

The following bootstrap providers are still available, but they are deprecated and are not recommended.

### OvfEnv

The `OvfEnv` method is no longer recommended. It relied on a combination of VMware's Guest OS Customization (GOSC) APIs for bootstrapping the guest's network and the Cloud-Init OVF data source for supplying a Cloud-Init Cloud Config. As a result of mixing bootstrap engines (GOSC and Cloud-Init), there was a race condition that meant any image that used `OvfEnv` needed to have a special fix applied. This provider is no longer supported and will be removed in v1alpha2. Any consumers still relying on this provider should switch to Cloud-Init.

### ExtraConfig

When Tanzu Kubernetes was first released, the Cluster API provider that depended upon VM Operator used the `ExtraConfig` provider for supplying bootstrap information. This method was never intended for wide use, and Tanzu now uses Cloud-Init anyway. To that end, this provider is no longer supported and will be removed in v1alpha2. Any consumers still relying on this provider should switch to Cloud-Init.
