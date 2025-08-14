# Deploy a VM With Cloud-Init

[Cloud-Init](https://cloudinit.readthedocs.io/en/latest/) is widely recognized as the de facto method for bootstrapping modern workloads on hyperscalers, including VM Service on vSphere.


## Example

The example below illustrates a `VirtualMachine` resource that specifies a Cloud-Init Cloud Config via a `Secret` resource (`my-vm-bootstrap-data`).

=== "VirtualMachine"

    ``` yaml
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
        cloudInit:
          cloudConfig:
            users:
            - name: jdoe
              primary_group: jdoe
              groups: users
              passwd:
                name: my-vm-bootstrap-data
                key:  jdoe-passwd
              ssh_authorized_keys:
              - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDSL7uWGj...
            runcmd:
            - "ls /"
            - [ "ls", "-a", "-l", "/" ]
            write_files:
            - path: /etc/my-plain-text
              permissions: '0644'
              owner: root:root
              content: |
                Hello, world.
            - path: /etc/my-secret-data
              permissions: '0644'
              owner: root:root
              content:
                name: my-vm-bootstrap-data
                key:  etc-my-secret-data
    ```

=== "Secret"

    ``` yaml
    apiVersion: v1
    kind: Secret
    metadata:
      name:      my-vm-bootstrap-data
      namespace: my-namespace
    stringData:
      jdoe-passwd: my-password
      etc-my-secret-data: |
        My super secret message.
    ```

For more information, please refer to the documentation [documentation](./../../concepts/workloads/guest.md#cloud-init) for the Cloud-Init bootstrap provider.
