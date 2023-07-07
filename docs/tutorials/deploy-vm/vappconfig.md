# Deploy a VM with vAppConfig

This page reviews the bootstrap method vAppConfig.

## Description
The vAppConfig bootstrap method is useful for legacy VM images that rely on bespoke, boot-time processes that leverage vAppConfig properties for customizing a guest.

## Purpose
As a DevOps user, you can assign Golang-based template strings to vAppConfig values to send information into the guest that is not known ahead of time, such as the IPAM data settings used to configure the VM's network stack.

## Example
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

=== "vAppConfig with supporting template"

    ```yaml
    apiVersion: v1
    kind: Secret
    metadata:
    name: my-secret
    namespace: test-ns
    stringData:
    # see more details on below Supporting Template Queries section
    nameservers: "{{ V1alpha1_FormatNameservers 2 \",\" }}"
    management_ip: "{{ V1alpha1_FormatIP \"192.168.1.10\" \"255.255.255.0\" }}"
    hostname: "{{ .V1alpha1.VM.Name }} "  
    management_gateway: "{{ (index .V1alpha1.Net.Devices 0).Gateway4 }}"
    ```

:wave: The Golang based template string is a representation of [vm-operator virtualmachinetempl_types v1alpha1 api](https://github.com/vmware-tanzu/vm-operator/blob/25fb865e615d377192a870583ab32973e9fbd32a/api/v1alpha1/virtualmachinetempl_types.go#L4)

:wave: The fields `nameservers`, `hostname`, `management_ip` and `management_gateway` are derived from image `haproxy-v0.2.0` OVF properties.
```xml
<Property ovf:key="nameservers" ovf:type="string" ovf:userConfigurable="true" ovf:value="1.1.1.1, 1.0.0.1">
      <Label>2.2. DNS</Label>
      <Description>A comma-separated list of IP addresses for up to three DNS servers</Description>
</Property>
<Property ovf:key="hostname" ovf:type="string" ovf:userConfigurable="true" ovf:value="ubuntuguest">
      <Description>Specifies the hostname for the appliance</Description>
</Property>
<Property ovf:key="management_ip" ovf:type="string" ovf:userConfigurable="true">
      <Label>2.3. Management IP</Label>
      <Description>The static IP address for the appliance on the Management Port Group in CIDR format (.For eg. ip/subnet mask bits). This cannot be DHCP.</Description>
</Property>
```

## Supporting Template Queries
To spare users the effort to construct a correct template string, there are some supporting template queries vm-operator provides.

| Query name | Signature | Description |
| -------- | -------- | -------- |
| V1alpha1_FirstIP | `func () string` | Get the first non-loopback IP with CIDR from first NIC. |
| V1alpha1_FirstIPFromNIC | `func (index int) string` | Get non-loopback IP address with CIDR from the ith NIC. if index out of bound, template string won't be parsed. |
| V1alpha1_FormatIP | `func (IP string, netmask string) string` | see below for detailed use case.|
| V1alpha1_FirstNicMacAddr | `func() (string, error)` | Get the first NIC's MAC address. |
| V1alpha1_FormatNameservers| `func (count int, delimiter string) string` | Format the first occurred count of nameservers with specific delimiter (A n.For egative number for count would mean all nameservers). |
| V1alpha1_IP | `func(IP string) string` | Format a static IP address with default netmask CIDR. If IP is not valid, template string won't be parsed. |
| V1alpha1_IPsFromNIC | `func (index int) []string` | List all IPs with CIDR from the ith NIC. if index out of bound, template string won't be parsed. |
| V1alpha1_SubnetMask | `func(cidr string) (string, error)` | Get subnet mask from a CIDR notation IP address and prefix length. |

### `V1alpha1_FormatIP`
1. Format an IP address with network length. A netmask can be either the length, ex. /24, or the decimal notation, ex. 255.255.255.0. Return IP sans CIDR when input is valid.

2. Format an IP address with CIDR: 
    - if input netmask is different with CIDR, replace and return IP with new CIDR. 
    - if input netmask is empty string, return IP sans CIDR. 
    - if input netmask is not valid, return empty string. 

**Note** when OVF only takes in IP sans CIDR, use `V1alpha1_FormatIP` and pass empty string as input netmask:
```yaml
management_ip: '{{ V1alpha1_FormatIP V1alpha1_FirstIP "" }}' # return first non-loopback IP from first NIC without CIDR. For eg, "192.168.1.10".
```

### Examples
```yaml
  management_ip: "{{ V1alpha1_FirstIP }}" # return first non-loopback IP with CIDR from first NIC.
  management_ip: "{{ V1alpha1_FirstIPFromNIC 1 }}" # return first non-loopback IP with CIDR from second NIC.
  management_ip: "{{ V1alpha1_FormatIP \"192.168.1.10\" \"255.255.255.0\" }}" # return IP with CIDR. For eg,"192.168.1.10/24".
  management_ip: "{{ V1alpha1_IP \"192.168.1.37\" }}" # return IP with default netmask CIDR.
  nameservers: "{{ V1alpha1_FormatNameservers 2 \",\" }}" # return 2 nameservers with "," as delimiter. For eg,"10.20.145.1, 10.20.145.2".
  subnet_mask: "{{ V1alpha1_SubnetMask V1alpha1_FirstIP }}" # return the subnet mask of the first non-loopback IP with CIDR from first NIC. For eg, "255.255.255.0".
  mac_address: "{{ v1alpha1_FirstNicMacAddr }}" # return the first NIC's MAC Address.
```

