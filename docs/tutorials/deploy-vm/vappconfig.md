# Deploy a VM with vAppConfig

The vAppConfig bootstrap method is useful for legacy VM images that rely on bespoke, boot-time processes that leverage vAppConfig properties for customizing a guest. This method also supports properties specified with Golang-style template strings in order to use information not known ahead of time, such as the networking configuration, into the guest via vApp properties.

## Example

The following example showcases a `VirtualMachine` resource that specifies one or more vApp properties used to bootstrap a guest.

=== "VirtualMachine"

    ```yaml
    apiVersion: vmoperator.vmware.com/v1alpha1
    kind: VirtualMachine
    metadata:
      name: legacy-vm
      namespace: test-ns
    spec:
      className: best-effort-small
      imageName: haproxy-v0.2.0
      powerState: poweredOn
      storageClass: wcpglobal-storage-profile
      vmMetadata:
        secretName: my-secret
        transport: vAppConfig
    ```

=== "vAppConfig"

    !!! note "The vApp properties below..."

        The vApp properties used in this example are not standard. All images may define their own properties for configuring the guest's network, or anything else. Please review the image's OVF to understand what properties are available and should be assigned.

    ```yaml
    apiVersion: v1
    kind: Secret
    metadata:
      name: my-secret
      namespace: test-ns
    stringData:
      nameservers: "{{ (index .V1alpha1.Net.Nameservers 0) }}"         
      management_ip: "{{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) }}"
      hostname: "{{ .V1alpha1.VM.Name }} "       
      management_gateway: "{{ (index .V1alpha1.Net.Devices 0).Gateway4 }}"
    ```

=== "vAppConfig with templated properties"

    !!! note "The vApp properties below..."

        The vApp properties used in this example are not standard. All images may define their own properties for configuring the guest's network, or anything else. Please review the image's OVF to understand what properties are available and should be assigned.


    ```yaml
    apiVersion: v1
    kind: Secret
    metadata:
      name: my-secret
      namespace: test-ns
    stringData:
      # Please see the following section for more information on these functions.
      nameservers: "{{ V1alpha1_FormatNameservers 2 \",\" }}"
      management_ip: "{{ V1alpha1_FormatIP \"192.168.1.10\" \"255.255.255.0\" }}"
      hostname: "{{ .V1alpha1.VM.Name }} "  
      management_gateway: "{{ (index .V1alpha1.Net.Devices 0).Gateway4 }}"
    ```

## Templating

Properties are templated according to the Golang [`text/template`](https://pkg.go.dev/text/template) package. Please refer to Go's documentation for a full understanding of how to construct template queries.

### Input object

The object provided to the template engine is the [`VirtualMachineTemplate`](https://github.com/vmware-tanzu/vm-operator/blob/70d22b93b3c454809145725d418c4f6cfebb124c/api/v1alpha1/virtualmachinetempl_types.go#L37-L50) data structure.

### Pre-defined functions

The following table lists the functions VM Operator defines and passes into the template engine to make it easier to construct the information required for vApp properties:

| Query name | Signature | Description |
| -------- | -------- | -------- |
| V1alpha1_FirstIP | `func () string` | Get the first, non-loopback IP address (formatted with network length) from the first NIC. |
| V1alpha1_FirstIPFromNIC | `func (index int) string` | Get the first, non-loopback IP address (formatted with network length) from the n'th NIC. If the specified index is out-of-bounds, the template string is not parsed. |
| V1alpha1_FormatIP | `func (IP string, netmask string) string` | This function may be used to format an IP address with or without a network prefix length. If the provided netmask is empty, then the IP address returned does not include a network length. If the provided netmask is non-empty, then it must be either a length, ex. `/24`, or decimal notation, ex. `255.255.255.0`. |
| V1alpha1_FirstNicMacAddr | `func() (string, error)` | Get the MAC address from the first NIC. |
| V1alpha1_FormatNameservers| `func (count int, delimiter string) string` | Format the first occurred count of nameservers with the provided delimiter. Specify a negative number to include all nameservers. |
| V1alpha1_IP | `func(IP string) string` | Format an IP address with the default netmask CIDR. If the specified IP is invalid, the template string is not parsed. |
| V1alpha1_IPsFromNIC | `func (index int) []string` | List all IPs, formatted with the network length, from the n'th NIC. If the specified index is out-of-bounds, the template string is not parsed. |
| V1alpha1_SubnetMask | `func(cidr string) (string, error)` | Get a subnet mask from an IP address formatted with a network length. |

