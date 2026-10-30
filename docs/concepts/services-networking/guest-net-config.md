# Guest Networking Configuration

Guest networking configuration in VM Operator determines how virtual machines (VMs) are configured with network connectivity inside the guest operating system. This configuration is applied during VM bootstrap and varies depending on the bootstrap provider used.

## The VM Operator guest networking model

VM Operator provides a declarative approach to configuring guest networking through the `VirtualMachine` API. The network configuration is built from several components:

* **Network Interfaces**: Each VM can have up to 10 network interfaces, each connected to a specific network and configured with IP addressing, DNS, and routing information.
* **Bootstrap Providers**: Different bootstrap providers handle the network configuration in different ways, supporting various guest operating systems and use cases.
* **Dynamic Configuration**: Network settings can be resolved dynamically through DHCP or configured statically through the VM specification.

The guest network configuration is separate from but coordinated with the underlying vSphere network infrastructure. While the hypervisor provides the virtual network hardware and connectivity, the guest networking configuration determines how the guest operating system uses those network resources.

## Network Interface Configuration

Network interfaces for VMs are defined in the `spec.network.interfaces` field of the `VirtualMachine` resource. Each interface specification includes:

### Basic Interface Configuration

```yaml
apiVersion: vmoperator.vmware.com/v1alpha6
kind: VirtualMachine
metadata:
  name: my-vm
  namespace: my-namespace
spec:
  className: my-vm-class
  imageName: my-vm-image
  storageClass: my-storage-class
  network:
    interfaces:
    - name: eth0
      network:
        name: my-network
```

### Static IP Configuration

For networks that support manual IP allocation, interfaces can be configured with static IP addresses:

```yaml
network:
  interfaces:
  - name: eth0
    network:
      name: my-network
    addresses:
    - "192.168.1.100/24"
    - "2001:db8::100/64"
    gateway4: "192.168.1.1"
    gateway6: "2001:db8::1"
    nameservers:
    - "8.8.8.8"
    - "8.8.4.4"
    searchDomains:
    - "example.com"
    - "local"
```

### DHCP Configuration

Interfaces can be configured to use DHCP for automatic IP assignment:

```yaml
network:
  interfaces:
  - name: eth0
    network:
      name: my-network
    dhcp4: true
    dhcp6: true
```

### Advanced Interface Options

Additional interface configuration options include:

```yaml
network:
  interfaces:
  - name: eth0
    guestDeviceName: ens33  # Custom device name in guest
    mtu: 9000              # Maximum transmission unit
    routes:                # Static routes
    - to: "10.0.0.0/8"
      via: "192.168.1.254"
      metric: 100
```

## Bootstrap Providers

VM Operator supports four different bootstrap providers, each with different capabilities for network configuration:

| Provider | Linux | Windows | Network Configuration Method | Supported Features |
|----------|:-----:|:-------:|------------------------------|-------------------|
| [Cloud-Init](#cloud-init) | ✓ | ✓ | [Cloud-Init Network Config v2](https://cloudinit.readthedocs.io/en/latest/reference/network-config-format-v2.html) | Static IPs, DHCP, DNS, Routes, MTU |
| [LinuxPrep](#linuxprep) | ✓ | | [Guest OS Customization (GOSC)](https://vdc-download.vmware.com/vmwb-repository/dcr-public/c476b64b-c93c-4b21-9d76-be14da0148f9/04ca12ad-59b9-4e1c-8232-fd3d4276e52c/SDK/vsphere-ws/docs/ReferenceGuide/vim.vm.customization.Specification.html) | Static IPs, DHCP, DNS |
| [Sysprep](#sysprep) | | ✓ | [Guest OS Customization (GOSC)](https://vdc-download.vmware.com/vmwb-repository/dcr-public/c476b64b-c93c-4b21-9d76-be14da0148f9/04ca12ad-59b9-4e1c-8232-fd3d4276e52c/SDK/vsphere-ws/docs/ReferenceGuide/vim.vm.customization.Specification.html) | Static IPs, DHCP, DNS |
| [vAppConfig](#vappconfig) | ✓ | | Bespoke (varies by image) | Depends on image implementation |

### Cloud-Init

Cloud-Init is the most feature-rich bootstrap provider for guest networking configuration. It generates [Cloud-Init Network Config v2](https://cloudinit.readthedocs.io/en/latest/reference/network-config-format-v2.html) format configuration that is applied inside the guest.

#### Supported Network Features

Cloud-Init supports the full range of network configuration options:

- Static IPv4 and IPv6 addressing
- DHCP4 and DHCP6 configuration
- Custom DNS nameservers and search domains
- Static routing
- MTU configuration
- Custom device naming in the guest

#### Example Configuration

```yaml
apiVersion: vmoperator.vmware.com/v1alpha6
kind: VirtualMachine
metadata:
  name: cloud-init-vm
spec:
  className: my-vm-class
  imageName: ubuntu-cloud-image
  storageClass: my-storage-class
  bootstrap:
    cloudInit:
      cloudConfig:
        users:
        - name: vmware
          sudo: ALL=(ALL) NOPASSWD:ALL
  network:
    hostName: my-vm
    domainName: example.com
    nameservers:
    - "8.8.8.8"
    - "8.8.4.4"
    searchDomains:
    - "example.com"
    interfaces:
    - name: eth0
      guestDeviceName: ens160
      dhcp4: true
      dhcp6: false
    - name: eth1
      addresses:
      - "10.0.1.100/24"
      gateway4: "10.0.1.1"
      nameservers:
      - "10.0.1.53"
      routes:
      - to: "10.0.2.0/24"
        via: "10.0.1.254"
        metric: 100
```

#### Cloud-Init Network Config Generation

When using Cloud-Init, VM Operator generates a [Netplan](https://netplan.io/) configuration that is consumed by Cloud-Init. The configuration includes:

- Interface matching based on MAC addresses
- IP addressing configuration (static or DHCP)
- Gateway configuration
- DNS configuration
- Static route definitions

### LinuxPrep

LinuxPrep uses VMware's Guest OS Customization (GOSC) to configure Linux guests. GOSC is a vSphere feature that applies network configuration during VM power-on.

#### Supported Network Features

LinuxPrep supports a subset of network configuration options:

- Static IPv4 and IPv6 addressing
- DHCP4 and DHCP6 configuration
- Global DNS nameservers and search domains
- Basic gateway configuration

#### Example Configuration

```yaml
apiVersion: vmoperator.vmware.com/v1alpha6
kind: VirtualMachine
metadata:
  name: linuxprep-vm
spec:
  className: my-vm-class
  imageName: rhel-template
  storageClass: my-storage-class
  bootstrap:
    linuxPrep:
      hardwareClockIsUTC: true
      timeZone: "America/Los_Angeles"
  network:
    hostName: my-linux-vm
    domainName: example.com
    nameservers:
    - "192.168.1.1"
    - "8.8.8.8"
    searchDomains:
    - "example.com"
    - "local"
    interfaces:
    - name: eth0
      addresses:
      - "192.168.1.100/24"
      gateway4: "192.168.1.1"
    - name: eth1
      dhcp4: true
```

#### GOSC Configuration

LinuxPrep generates a vSphere `CustomizationSpec` that includes:

- Identity configuration (hostname, domain, timezone)
- Global IP settings (DNS servers, search domains)
- Per-interface NIC settings mapped by MAC address

### Sysprep

Sysprep uses VMware's Guest OS Customization (GOSC) to configure Windows guests. Like LinuxPrep, it applies configuration during VM power-on but uses Windows-specific mechanisms.

#### Supported Network Features

Sysprep supports network configuration options similar to LinuxPrep:

- Static IPv4 and IPv6 addressing
- DHCP4 and DHCP6 configuration
- Global DNS nameservers and search domains
- Basic gateway configuration
- Domain join configuration

#### Example Configuration

```yaml
apiVersion: vmoperator.vmware.com/v1alpha6
kind: VirtualMachine
metadata:
  name: sysprep-vm
spec:
  className: my-vm-class
  imageName: windows-template
  storageClass: my-storage-class
  bootstrap:
    sysprep:
      sysprep:
        identification:
          domainAdmin: "EXAMPLE\\administrator"
          domainAdminPassword:
            name: domain-credentials
            key: password
          joinDomain: "example.com"
        guiUnattended:
          autoLogon: true
          autoLogonCount: 1
  network:
    hostName: my-windows-vm
    domainName: example.com
    nameservers:
    - "192.168.1.1"
    - "8.8.8.8"
    interfaces:
    - name: ethernet0
      addresses:
      - "192.168.1.101/24"
      gateway4: "192.168.1.1"
```

### vAppConfig

vAppConfig is used for VMs that have custom bootstrap mechanisms built into the VM image. The network configuration is passed to the guest through OVF/vApp properties, and the guest is responsible for applying the configuration.

#### Network Configuration Support

vAppConfig network support depends entirely on the implementation within the VM image:

- Configuration is passed as OVF properties
- Guest must have custom scripts or software to consume the properties
- Network configuration format is image-specific
- Can be combined with GOSC for more standardized network setup

#### Example Configuration

```yaml
apiVersion: vmoperator.vmware.com/v1alpha6
kind: VirtualMachine
metadata:
  name: vapp-vm
spec:
  className: my-vm-class
  imageName: custom-appliance
  storageClass: my-storage-class
  bootstrap:
    vAppConfig:
      properties:
      - key: "network.ip0"
        value:
          value: "192.168.1.102/24"
      - key: "network.gateway"
        value:
          value: "192.168.1.1"
      - key: "network.dns"
        value:
          value: "8.8.8.8,8.8.4.4"
  network:
    interfaces:
    - name: eth0
      network:
        name: my-network
```

## Global Network Settings

In addition to per-interface configuration, VM Operator supports global network settings that apply to the entire VM:

### Hostname and Domain

```yaml
network:
  hostName: my-vm        # VM hostname (max 63 chars, 15 for Windows)
  domainName: example.com # DNS domain name
```

### Global DNS Settings

```yaml
network:
  nameservers:           # Global DNS servers
  - "8.8.8.8"
  - "8.8.4.4"
  searchDomains:         # Global search domains
  - "example.com"
  - "local"
```

These global settings are handled differently by each bootstrap provider:

- **Cloud-Init**: Used as defaults when per-interface settings are not specified
- **LinuxPrep/Sysprep**: Applied globally via GOSC
- **vAppConfig**: Passed as properties; handling depends on guest implementation

## Network Configuration Precedence

When multiple sources specify network configuration, the precedence order is:

1. **Per-interface settings** (highest precedence)
2. **Global VM network settings**
3. **Network provider defaults** (from the connected network)
4. **Bootstrap provider defaults** (lowest precedence)

For example, if both `spec.network.nameservers` and `spec.network.interfaces[0].nameservers` are specified, the per-interface nameservers take precedence for that interface.

## Network Status and Observability

VM Operator provides detailed status information about network configuration and the observed network state:

### Configuration Status

The `status.network.config` field shows the resolved network configuration:

```yaml
status:
  network:
    config:
      interfaces:
      - name: eth0
        ip:
          addresses:
          - "192.168.1.100/24"
          gateway4: "192.168.1.1"
          dhcp:
            ip4:
              enabled: false
        dns:
          nameservers:
          - "8.8.8.8"
          - "8.8.4.4"
```

### Observed Network State

The `status.network` field provides observed information from VMware Tools:

```yaml
status:
  network:
    hostName: my-vm.example.com
    primaryIP4: "192.168.1.100"
    interfaces:
    - name: eth0
      deviceKey: 4000
      ip:
        macAddr: "00:50:56:89:ab:cd"
        addresses:
        - address: "192.168.1.100/24"
          origin: "manual"
          state: "preferred"
```

## Troubleshooting Network Configuration

### Common Issues

1. **IP Address Not Assigned**: Check if the connected network supports the requested addressing method (DHCP vs. static)

2. **DNS Resolution Problems**: Verify nameserver configuration and network connectivity to DNS servers

3. **Bootstrap Failures**: Check the `GuestCustomization` condition on the VM for GOSC-related issues

4. **Interface Not Configured**: Ensure the bootstrap provider supports the requested network features

### Debugging Steps

1. **Check VM Conditions**:
   ```bash
   kubectl get vm my-vm -o jsonpath='{.status.conditions}'
   ```

2. **Examine Network Status**:
   ```bash
   kubectl get vm my-vm -o jsonpath='{.status.network}'
   ```

3. **Review Bootstrap Configuration**:
   ```bash
   kubectl get vm my-vm -o yaml | grep -A 20 bootstrap:
   ```

4. **Check Connected Networks**:
   ```bash
   kubectl get networks -n my-namespace
   ```

## Bring your own network interface

When the Supervisor capability `supports_vm_service_byo_network_interface` is active, v1alpha6 VMs can reference existing interface resources in their own namespace. Check the Supervisor's advertised capabilities before submitting a reference. Supported providers are NetOP `NetworkInterface` (`netoperator.vmware.com/v1alpha1`), NCP `VirtualNetworkInterface` (`vmware.com/v1alpha1`), and VPC `SubnetPort` (`crd.nsx.vmware.com/v1alpha1`), restricted to providers supported in the namespace.

```yaml
spec:
  network:
    interfaces:
    - name: eth0
      externalRef:
        apiVersion: crd.nsx.vmware.com/v1alpha1
        kind: SubnetPort
        name: precreated-port
    - name: eth1
      # The normal default network is selected for this interface.
```

Set either `externalRef` or `network` on each interface; `macAddr` also cannot be set together with `externalRef`. External references receive no default network. The external resource may become ready after the VM is submitted; creation, status changes, and deletion trigger VM reconciliation. A missing resource is reported as a dependency failure and is never replaced with an automatically created interface.

Other interface-spec overrides remain available under their existing validation and feature gates. Addresses, gateways, DNS, routes, MTU, and DHCP settings customize the guest; an explicit `dhcp4: false` or `dhcp6: false` overrides the provider value. No MAC override is applied to external interfaces, and hardware settings such as `unitNumber` follow their existing rules. These settings do not rewrite the external resource's specification or ask its provider to allocate new addresses; its owner remains responsible for provider allocation and compatibility with the requested settings.

VM Operator does not add VM ownership or delete external resources. For VPC it manages the `vmoperator.vmware.com/vm-name` and `vmoperator.vmware.com/vm-interface-name` labels and the `nsx.vmware.com/attachment_ref` annotation. It releases them once the SubnetPort is no longer referenced by the VM spec and is not attached to any of the VM's current NICs, or when the VM is deleted. Unrelated metadata is preserved. Resources owned by any VirtualMachine, and SubnetPorts labeled with or attached to a different VM, are rejected. Callers must coordinate exclusive use across VMs.

With mutable networking enabled, a powered-off VM can rename an interface while retaining its external reference; for VPC, the interface-name label and attachment annotation are updated. Any NIC changes go through the normal mutable-network device reconcile and wait for the supported powered-off path. Guest naming follows normal bootstrap behavior; a spec rename does not promise a live guest rename. Remove an interface and confirm detachment before deleting its external resource.

v1alpha2 through v1alpha5 preserve references by interface name through conversion data. Edits through older clients that produce an invalid combination, such as setting `network` on an external interface, are rejected by validation; use v1alpha6 for source changes. Switching an existing interface name between managed and external is rejected; remove it and add a new interface.

The capability is disabled by default and independent of mutable networking and NIC unit-number capabilities. Disabled fields and dependent validation rules are removed on initial CRD installation and on updates when CRD cleanup is enabled. If the schema is retained, runtime guards reject new or changed references. If the field is removed, API-server unknown-field handling may reject or prune it, after which ordinary network defaulting applies. Drain BYO workloads before disabling the capability, removing the schema field, or downgrading to a version without this lifecycle support.

## What's Next

This section covers guest network configuration concepts. For information about exposing VM services to other workloads, see:

* [VirtualMachineService](./vm-service.md) - Exposing VM applications through Kubernetes Services
* [VM Networking](../workloads/vm.md#networking) - Advanced VM networking topics
