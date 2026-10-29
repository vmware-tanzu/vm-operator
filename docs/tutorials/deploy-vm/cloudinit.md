# Deploy a VM With Cloud-Init

[Cloud-Init](https://cloudinit.readthedocs.io/en/latest/) is widely recognized as the de facto method for bootstrapping modern workloads on hyperscalers, including VM Service on vSphere.


## Example

The example below illustrates a `VirtualMachine` resource that specifies a Cloud-Init Cloud Config via a `Secret` resource (`my-vm-bootstrap-data`).

=== "VirtualMachine"

    ``` yaml
    apiVersion: vmoperator.vmware.com/v1alpha6
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

For more information, please refer to the [documentation](./../../concepts/workloads/guest.md#cloud-init) for the Cloud-Init bootstrap provider.

## DHCP overrides

With Cloud-Init, a VM can obtain its IP addresses through DHCP while using your chosen DNS servers and routes. Configure this through the existing nameserver and gateway fields in `spec.network`.

### Choose DNS servers

Set `spec.network.interfaces[].nameservers` to use your chosen DNS servers on a DHCP interface. VM Operator configures that interface to ignore DNS servers from every enabled DHCP family. For example, specifying an IPv4 nameserver also suppresses DNS servers supplied by DHCPv6. This behavior requires Netplan's `networkd` renderer.

### Choose routes

For each interface, `gateway4` controls the IPv4 gateway and `gateway6` controls the IPv6 gateway:

| Value | Behavior |
| --- | --- |
| Omitted | Use the network provider's gateway and retain DHCP routes. |
| An IP address | Override the network provider's gateway. Requires manual IP allocation and DHCP disabled for that address family. |
| `"None"` | Ignore the network provider's gateway and all DHCP routes for that address family. DHCP address allocation continues. |

The interface's `routes` list adds static routes. Those routes are installed even when the matching gateway is `"None"`. Providing a `routes` list alone retains DHCP routes.

The following excerpt obtains an IPv4 address through DHCP, uses `192.0.2.53` for DNS, and ignores all DHCPv4 routes. It adds one static route to `198.51.100.0/24` through `192.0.2.1`:

```yaml
spec:
  bootstrap:
    cloudInit: {}
  network:
    interfaces:
    - name: eth0
      dhcp4: true
      dhcp6: false
      nameservers:
      - 192.0.2.53
      gateway4: "None"
      routes:
      - to: 198.51.100.0/24
        via: 192.0.2.1
```

### Interfaces using both DHCPv4 and DHCPv6

When both DHCP families are enabled on the same interface, either set both `gateway4` and `gateway6` to `"None"` to ignore DHCP routes, or omit both fields to retain them:

```yaml
spec:
  bootstrap:
    cloudInit: {}
  network:
    interfaces:
    - name: eth0
      dhcp4: true
      dhcp6: true
      gateway4: "None"
      gateway6: "None"
```

A request with both DHCP families explicitly enabled and only one gateway set to `"None"` is rejected at admission. When the network provider enables DHCPv4 or DHCPv6 because the corresponding `dhcp4` or `dhcp6` field is omitted, setting only one gateway to `"None"` is accepted but DHCP routes are retained for both families.

VM Operator translates explicit interface nameservers into Netplan's `use-dns: false` and `"None"` gateways into `use-routes: false`, applying each override only to enabled DHCP families. Netplan's `networkd` renderer requires the same override keys and values for both families. See [Netplan DHCP overrides](https://netplan.readthedocs.io/en/stable/netplan-yaml/#dhcp-overrides).

These overrides leave DNS and routes learned from IPv6 router advertisements unchanged.
