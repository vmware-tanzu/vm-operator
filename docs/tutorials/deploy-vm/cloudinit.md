# Deploy a VM With Cloud-Init

This page reviews deploying a VM with the bootstrap method Cloud-Init.

## Description

[Cloud-Init](https://cloudinit.readthedocs.io/en/latest/) is widely recognized as the de facto method for bootstrapping modern VM instances on hyperscalers, including VM Service on vSphere with Tanzu.

## Purpose
As a DevOps user leveraging Cloud-Init, you have the capability to bootstrap the guest within a VM, enabling seamless execution of operations during the boot process. Some tasks you can accomplish include:

* Adding a custom user,
* Executing commands on boot, and
* Writing files.


## Example
=== "VirtualMachine"

    ``` yaml
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
    ```

=== "CloudConfig"

    ``` yaml
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

The above example shows a `VirtualMachine` resource that specifies user-data using a `Secret` resource (`my-vm-bootstrap-data`), which will be used by `CloudInit` to bootstrap and customize the guest.

The data in the above `Secret` has the Cloud-Init _Cloud Config_. For more information on the Cloud-Init Cloud Config format, please see its [official documentation](https://cloudinit.readthedocs.io/en/latest/reference/examples.html).
