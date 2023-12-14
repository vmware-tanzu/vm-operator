# Deploy a VM With Cloud-Init

[Cloud-Init](https://cloudinit.readthedocs.io/en/latest/) is widely recognized as the de facto method for bootstrapping modern workloads on hyperscalers, including VM Service on vSphere.


## Example

The example below illustrates a `VirtualMachine` resource that specifies a Cloud-Init Cloud Config via a `Secret` resource (`my-vm-bootstrap-data`). For more information on the Cloud-Init Cloud Config format, please see the [official documentation](https://cloudinit.readthedocs.io/en/latest/reference/examples.html).


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

