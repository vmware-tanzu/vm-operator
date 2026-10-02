# One-Pager: Guest DNS Defaults

- **Spec**: [`spec.md`](./spec.md) · **Plan**: [`plan.md`](./plan.md)
- **Branch**: `bryanv/network-dns-bootstrap-changes`
- **Epic**: TBD
- **Status**: Draft

## Summary

A Supervisor can be configured with cluster wide default DNS nameservers and search domains, stored in the `vmoperator-network-config` ConfigMap. When VM Operator bootstraps a VM's network and the VM Spec doesn't provide DNS, it falls back to these defaults. The rules that decide where the defaults, and the DNS from the VM Spec, end up were written when VMs typically had a single interface, and they apply DNS far more broadly than they should. Resolvers are configured on interfaces that can't reach them, DNS servers from DHCP are overridden, multi-NIC VKS nodes end up with more nameservers than the guest's resolver will use, and Windows VMs get their nameservers in a setting the guest ignores.

This change adds new behavior behind a new Supervisor capability:

- DNS in the VM Spec always takes precedence over the Supervisor defaults.
- VM-level DNS is applied only to interfaces with a static IP address.
- The Supervisor defaults are applied only to the VM's first interface, and only when that interface has a static IP address and a gateway, and the VM Spec doesn't give it DNS.
- Nameservers are applied to an interface only when they match its IP families. The default nameservers, which can be resolvers on another network or a public resolver such as `1.1.1.1`, are further limited to the families the interface has a gateway for.
- DNS that the network provider reports for an interface is treated like interface-level DNS from the VM Spec.

VMs that already exist when the capability is activated keep the prior behavior, so activating it doesn't change their guest configuration.

## Background

### Where a VM's guest DNS comes from

VM Operator combines DNS from four places when it builds a VM's guest network configuration:

- **VM-level DNS**: `spec.network.nameservers` and `spec.network.searchDomains`.
- **Interface-level DNS**: `spec.network.interfaces[].nameservers` and `searchDomains`.
- **The network provider**: NSX-T, VPC, or a vSphere Distributed Switch network. The provider decides whether an interface gets its address from a static IP pool, from DHCP, or not at all, in which case the network has no IP address management (NoIPAM) and addressing is left to the guest. VPC SubnetPorts are also expected to report nameservers and search domains for each port.
- **The Supervisor defaults**: the `nameservers` and `searchsuffixes` keys of the `vmoperator-network-config` ConfigMap in VM Operator's namespace, each a whitespace-separated list. The checked-in manifest only has a `<worker_dns>` placeholder for `nameservers`, and VM Operator returns an error if the placeholder hasn't been replaced with real values. These resolvers can be on another network, or a public resolver such as `1.1.1.1`, so a VM can usually reach them only through a gateway.

### How each bootstrap method applies DNS

VM Operator configures the guest network through the VM's bootstrap method, and each method can express a different part of the guest's DNS configuration. Cloud-Init receives a netplan configuration in `guestinfo.metadata`, and netplan configures DNS per interface. LinuxPrep and Sysprep use vSphere guest OS customization (GOSC). A GOSC customization spec has a global DNS server list and suffix list, plus a DNS server list for each adapter, but Linux and Windows each honor only part of it: Linux customization uses only the global lists, and Windows uses only the per-adapter server lists and the global suffix list. vAppConfig doesn't configure the guest network itself; its templates can read the resolved nameservers from `.Net.Nameservers`.

The VM validation webhook only accepts the DNS fields the VM's bootstrap method can apply:

| | Cloud-Init | LinuxPrep | Sysprep | vAppConfig only, or no bootstrap |
|---|---|---|---|---|
| **Guest DNS servers** | Per interface (netplan) | Global only; Linux GOSC ignores the per-adapter list | Per adapter only; Windows ignores the global list | Not configured by VM Operator; templates may use `.Net.Nameservers` |
| **Guest search domains** | Per interface (netplan) | Global only | Global only | Not configured by VM Operator |
| **Interaction with DHCP** | Netplan adds the configured nameservers to DHCP's | A non-empty global list overrides DHCP on every interface | A non-empty adapter list overrides DHCP on that adapter | — |
| `spec.network.nameservers` | Allowed when `useGlobalNameserversAsDefault` is unset or true | Allowed | Allowed | Rejected |
| `spec.network.searchDomains` | Allowed when `useGlobalSearchDomainsAsDefault` is unset or true | Allowed | Allowed | Rejected |
| `interfaces[].nameservers` | Allowed | Rejected | Allowed | Rejected |
| `interfaces[].searchDomains` | Allowed | Rejected | Rejected | Rejected |

A VM that is identified as Linux with no bootstrap method is implicitly customized with LinuxPrep (historical VM Operator behavior), but the webhook treats it as having no bootstrap method, so none of its DNS fields can be set. Adding `spec.bootstrap.linuxPrep`, which the webhook allows after creation, makes them available.

Any rule for applying DNS therefore has to work per interface for Cloud-Init and for Sysprep nameservers, and globally for LinuxPrep and for Sysprep search domains.

### How the current behavior came about

The current rules date back to v1alpha1, when most VMs had one interface:

- When any interface that doesn't use DHCP has no nameservers, and the VM Spec has none, VM Operator applies the default nameservers. For Cloud-Init it copies them to every interface that doesn't use DHCP, which includes NoIPAM interfaces. For LinuxPrep and Sysprep it puts them in the GOSC global list.
- The default search domains are applied to the interfaces of VKS VMs, which use Cloud-Init. LinuxPrep and Sysprep VMs never get them, because the v1alpha1 code assumed GOSC couldn't set search suffixes. The exception is a Linux VM with no bootstrap method. Since a 2024 refactor (`6c4fbc485`), this code decides whether a VM uses GOSC from `spec.bootstrap` alone, so it doesn't recognize such a VM as LinuxPrep. Before the refactor, these VMs didn't get the default search domains either.
- For Cloud-Init, VM-level DNS is copied to every interface that doesn't set its own, whatever its addressing, unless `useGlobalNameserversAsDefault` or `useGlobalSearchDomainsAsDefault` is false.

The code's own comment says these rules are probably not correct for every situation.

VKS clusters are a common user of VM Service, and multi-NIC VKS nodes show the problem most clearly. CAPV creates the node VMs, which always use Cloud-Init and carry the `capv.vmware.com/cluster.role` label. A node's first interface is always its primary network, and a node can also have secondary networks. The guest merges the nameservers of every interface into one resolver list. Because VM Operator copies the defaults to the primary and to every static secondary interface, each default nameserver appears once per interface. With two default nameservers and two interfaces, the node has four, one more than the three that glibc's resolver uses.

## Problem

| Bootstrap method | What happens today | Why it is a problem |
|---|---|---|
| Cloud-Init | The default nameservers go on **every** interface that doesn't use DHCP, including NoIPAM interfaces. VM-level DNS is copied to **every** interface, including DHCP and NoIPAM interfaces. | Netplan DNS is per interface, so resolvers are configured on links that can't reach them, and lookups sent over those links can time out. IPv6 resolvers are added to IPv4-only links. |
| Cloud-Init, multi-NIC VKS nodes | Each default nameserver is written to the primary and every static secondary interface. | The guest merges them into one resolver list. With two or more interfaces, the list can exceed glibc's limit of three nameservers, and the extra ones are ignored. |
| LinuxPrep | The default nameservers go into the GOSC **global** DNS server list whenever any interface doesn't use DHCP, even when the first interface does. | GOSC treats a non-empty global list as an override of DHCP. A VM whose primary network uses DHCP loses the DNS servers its DHCP server provides. |
| Sysprep | The default nameservers, **and `spec.network.nameservers`**, go only into the GOSC global DNS server list. | Windows doesn't use the global DNS server list, only the per-adapter lists, so the VM gets no DNS servers from either the defaults or the VM Spec. |
| Implicit LinuxPrep (Linux VM with no bootstrap method) | Since a 2024 refactor, the default search domains are applied, because the code no longer treats the VM as using GOSC. | Explicit LinuxPrep VMs never get them, so the result depends on whether the bootstrap method was spelled out. Before the refactor, implicit LinuxPrep VMs didn't get them either. |

## Goals and non-goals

**Goals**

- DNS from the VM Spec always takes precedence over the Supervisor defaults. For each interface the order is interface-level DNS, then DNS from the network provider, then VM-level DNS, and finally the Supervisor default.
- VM-level DNS and the Supervisor defaults are only applied to interfaces with a static IP address, and nameservers only when they match the interface's IP families.
- Only the first interface is considered when deciding whether the Supervisor defaults apply, and which of the default nameservers: the first interface needs a gateway to reach them. With Cloud-Init, the defaults are applied to that interface only, so a VKS node's resolver list doesn't grow with its number of interfaces.
- VM-level nameservers reach Windows guests.
- The DNS of VMs that are already deployed doesn't change.

**Non-goals**

- New API fields, or changes to which DNS fields the webhook accepts. The only API change is to field documentation.
- DHCP overrides such as nameservers or routes. Support for this will be added later.
- Changing DNS for vAppConfig-only VMs, whose templates configure the guest.

## Proposal

### Terms

- A **static interface** has a static IP address, doesn't use DHCPv4 or DHCPv6, and isn't on a NoIPAM network.
- An interface's **IP families** are the families of its static IP addresses. An interface that accepts IPv6 Router Advertisements also counts as IPv6.
- An interface's **gateway families** are the families it has a gateway for: IPv4 or IPv6 when it has a static address of that family with a gateway, and IPv6 when it accepts Router Advertisements. `gateway4: None` or `gateway6: None` removes that family.
- The **primary interface** is the VM's first interface in `spec.network.interfaces`, when it is static and has a gateway. If the first interface uses DHCP, is on a NoIPAM network, or has no gateway, the VM has no primary interface and gets no Supervisor defaults; later interfaces are never considered. The rule is the same for VKS and other VMs. It fits VKS, where the first interface is always the node's primary network, and it matches the vSphere GOSC primary adapter.
- An interface's **interface DNS** is its interface-level DNS from the VM Spec, or else the DNS its network provider reports for it.

### Per bootstrap method, with the new behavior

- **Cloud-Init**
  - Interface DNS goes on its interface, however the interface is addressed.
  - VM-level DNS is copied to each static interface that has no interface DNS, when the matching `useGlobal*AsDefault` field is unset or true. Nameservers are filtered to the interface's IP families. DHCP and NoIPAM interfaces don't get VM-level DNS; set interface-level DNS on them instead.
  - The primary interface gets the default nameservers of its gateway families when `spec.network.nameservers` is empty and the interface has no interface nameservers.
  - For VKS VMs only, the primary interface also gets the default search domains under the same conditions.
  - The `useGlobal*AsDefault` fields don't affect the Supervisor defaults.
- **LinuxPrep, explicit or implicit**: Linux GOSC only has global DNS settings.
  - The global lists get the VM-level DNS, followed by each interface's provider DNS in interface order, without duplicates.
  - When that leaves no nameservers, and the VM has a primary interface, the global list gets the default nameservers of the primary interface's gateway families. Only the first interface is considered, so a later DHCP interface gets the defaults in place of its DHCP servers' DNS, as with the prior behavior.
  - The default search domains are not applied.
- **Sysprep**: Windows configures DNS servers per adapter and search suffixes globally.
  - The global DNS server list is always empty.
  - Interface nameservers go on their adapter, including a DHCP adapter, where they override DHCP.
  - VM-level nameservers go to each static adapter that has no interface nameservers, filtered to its IP families. DHCP and NoIPAM adapters keep the DNS servers from DHCP.
  - The primary adapter gets the default nameservers of its gateway families when `spec.network.nameservers` is empty and the adapter has no interface nameservers.
  - The global suffix list gets the VM-level search domains, followed by each adapter's provider search domains. The default search domains are still not applied. On Windows, a suffix search list replaces appending the primary and connection-specific DNS suffixes, which could break short-name resolution in a domain-joined VM's Active Directory domain.

### DNS from the network provider

VPC SubnetPorts are expected to optionally report nameservers and search domains for each interface. The SubnetPort API doesn't have these fields yet, so only the plumbing is in place: `bootstrapFromVPC` has a TODO to read them.

With the new behavior, the DNS a provider reports for an interface takes the place of interface-level DNS, unless the interface spec sets its own. Nameservers and search domains are handled separately, so an interface can take its nameservers from the spec and its search domains from the provider. Like interface-level DNS, provider DNS is applied however the interface is addressed, isn't filtered by IP family, and isn't affected by the `useGlobal*AsDefault` fields. The prior behavior ignores provider DNS, so its output doesn't change when SubnetPorts start reporting it.

What happens to provider DNS depends on the bootstrap method:

| Bootstrap method | Provider nameservers | Provider search domains                                                                                                               | How a user overrides them |
|---|---|---------------------------------------------------------------------------------------------------------------------------------------|---|
| Cloud-Init | Go on the interface's netplan `nameservers`. The interface doesn't get the VM-level nameservers, and if it is the primary interface, it doesn't get the default nameservers. On a DHCP interface, netplan adds them to the DNS servers from DHCP. | Go on the interface's netplan `search`, with the same effect on VM-level search domains and, for VKS VMs, the default search domains. | `interfaces[].nameservers` and `interfaces[].searchDomains` replace them. |
| LinuxPrep | Appended to the global DNS server list after the VM-level nameservers, in interface order, without duplicates. If any interface has provider nameservers, the default nameservers aren't applied. Like any global entry, they override DHCP on every interface. | Appended to the global suffix list after the VM-level search domains.                                                                 | Users can't: the webhook rejects interface-level DNS for LinuxPrep. VM-level DNS is placed ahead of them but doesn't remove them. |
| Sysprep | Go on the adapter's DNS server list, including on a DHCP adapter, where they replace the DNS servers from DHCP. The adapter doesn't get the VM-level nameservers, and if it is the primary adapter, it doesn't get the default nameservers. | Appended to the global suffix list after the VM-level search domains.                                                                 | `interfaces[].nameservers` replaces the nameservers. The search domains can't be removed, since the webhook rejects `interfaces[].searchDomains` for Sysprep. |
| vAppConfig only, no bootstrap method, or bootstrap disabled | Reported in `status.network.config.interfaces[].dns`. Not available to templates. | Same as nameservers.                                                                                                                  | — |

Examples 7, 10, and 13 show provider DNS with each bootstrap method. The open questions below cover the cases where users can't override provider DNS, the effect of provider search domains on Windows, and templates.

### Status and templates

- `status.network.config.dns` reports only the global DNS that the bootstrap method applies:
  - Cloud-Init: nothing, since netplan has no global DNS.
  - LinuxPrep: the global lists.
  - Sysprep: the search suffixes, and no nameservers.
  - When no bootstrap method configures the network (vAppConfig only, no bootstrap method, or bootstrap disabled): the resolved DNS, which is the VM-level DNS or else the Supervisor defaults, so users can configure the guest themselves.
- DNS applied to an interface is reported in `status.network.config.interfaces[].dns`.
- Templates keep receiving the resolved nameservers in `.Net.Nameservers`, so existing vAppConfig and Sysprep templates render the same as before.

### Selecting the behavior

The new behavior requires the Supervisor capability `supports_vm_service_scoped_dns_defaults`, a placeholder name until WCP assigns one. While the capability isn't activated, every VM uses the prior behavior whether or not it has the annotation described below, and VM Operator doesn't add or change the annotation.

Once the capability is activated, the internal annotation `vmoperator.vmware.com/dns-defaults` pins each VM to the prior behavior (`legacy`) or the new one (`scoped`). A VM without the annotation gets it on its next reconcile. The value is `legacy` if the VM's guest may already have been configured, which VM Operator infers from a bootstrap hash annotation or from the `first-boot-done`, `restored-vm`, `imported-vm`, or `failed-over-vm` annotation. Otherwise it is `scoped`.

Without this pin, activating the capability would change the bootstrap of existing VMs. VM Operator would rewrite the guestinfo of running Cloud-Init VMs, and re-customize LinuxPrep and Sysprep VMs that don't set `customizeAtNextPowerOn` at their next power-on.

Only privileged users can add, change, or remove the annotation, so an admin can set it to `legacy` to put a single VM back on the prior behavior. The prior behavior produces exactly the customization data VM Operator produces today.

## Examples

Each example shows a VM Spec, the guest configuration VM Operator generates for it with the new behavior, and how the prior behavior differs. They all assume the Supervisor's ConfigMap has:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: vmoperator-network-config
  namespace: vmware-system-vmop
data:
  nameservers: "10.0.0.53 fd00::53"
  searchsuffixes: "corp.local"
```

They also assume a new VM, so the new behavior is selected.

How to read the output:

- For Cloud-Init, the output is the netplan VM Operator writes to `guestinfo.metadata`. The excerpts leave out `match`, `set-name`, `dhcp6`, and `accept-ra`. An interface without DNS has an empty `nameservers` block.
- For LinuxPrep and Sysprep, the output is the relevant part of the vSphere customization spec: `globalIPSettings`, and each adapter in `nicSettingMap` in interface order. An adapter's `ip` is shortened to its fixed address, or `dhcp`.
- Addresses are set in the spec so the examples are self-contained. Addresses from the network's IP pool behave the same.

### Changes at a glance

| Scenario | Prior behavior | New behavior | Example |
|---|---|---|---|
| Cloud-Init, several static interfaces, no DNS in the spec | Defaults on every interface; on VKS nodes, more nameservers than glibc uses | Defaults on the first interface only, filtered to its gateway families | 1, 2 |
| Cloud-Init, NoIPAM interface | Gets the defaults and the VM-level DNS | Gets neither | 3 |
| Cloud-Init, DHCP or NoIPAM first interface | Later static interfaces get the defaults | No defaults | 4 |
| Cloud-Init, VM-level DNS | Copied to every interface, unfiltered | Copied to static interfaces, with nameservers filtered by family | 5 |
| Cloud-Init, VM-level DNS with only DHCP interfaces | Applied to every interface | Not applied | 6 |
| LinuxPrep, DHCP or NoIPAM first interface | Defaults in the global list, overriding DHCP | No defaults | 8 |
| LinuxPrep, static first interface | Defaults of both families in the global list | Only the defaults of the first interface's gateway families | 9 |
| Sysprep, no DNS in the spec | Defaults in the global list, which Windows ignores | Defaults on the first adapter | 11 |
| Sysprep, VM-level nameservers | In the global list, which Windows ignores | On each static adapter, filtered by family | 12 |
| Linux VM with no bootstrap method | Default search suffixes applied | Not applied, like explicit LinuxPrep | 14 |
| DNS from the network provider | Ignored | Treated as interface-level DNS | 7, 10, 13 |

### 1. Cloud-Init, VKS node with a primary and a secondary network

```yaml
metadata:
  labels:
    capv.vmware.com/cluster.role: ...
spec:
  bootstrap:
    cloudInit: {}
  network:
    interfaces:
    - name: eth0   # primary network
      addresses: ["192.168.10.10/24"]
      gateway4: 192.168.10.1
    - name: eth1   # secondary network
      addresses: ["192.168.20.10/24"]
      gateway4: 192.168.20.1
```

New behavior:

```yaml
network:
  ethernets:
    eth0:
      dhcp4: false
      addresses: ["192.168.10.10/24"]
      gateway4: 192.168.10.1
      nameservers:
        addresses: ["10.0.0.53"]
        search: ["corp.local"]
    eth1:
      dhcp4: false
      addresses: ["192.168.20.10/24"]
      gateway4: 192.168.20.1
      nameservers: {}
```

The defaults go only on the primary network. eth0 only has an IPv4 gateway, so it only gets the IPv4 default nameserver. The node ends up with one nameserver, however many secondary networks it has.

- **Prior behavior**: both interfaces get `10.0.0.53, fd00::53` and `corp.local`. The guest merges them into four nameservers, one more than glibc uses.
- **Non-VKS VM**: eth0 gets the same nameservers, without the search domains. With the prior behavior, both interfaces get the nameservers, and neither gets search domains.

### 2. Cloud-Init, dual-stack first interface with `gateway6: None`

```yaml
spec:
  bootstrap:
    cloudInit: {}
  network:
    interfaces:
    - name: eth0
      addresses: ["192.168.10.10/24", "fd00:10::10/64"]
      gateway4: 192.168.10.1
      gateway6: None
```

New behavior:

```yaml
network:
  ethernets:
    eth0:
      dhcp4: false
      addresses: ["192.168.10.10/24", "fd00:10::10/64"]
      gateway4: 192.168.10.1
      nameservers:
        addresses: ["10.0.0.53"]
```

eth0 has an IPv6 address but no IPv6 gateway, so it doesn't get the IPv6 default nameserver. With a `gateway6`, it would get both. If `gateway4` were also `None`, it would get no defaults at all.

- **Prior behavior**: eth0 gets `10.0.0.53, fd00::53`.

### 3. Cloud-Init, static interface and a NoIPAM interface

```yaml
spec:
  bootstrap:
    cloudInit: {}
  network:
    interfaces:
    - name: eth0
      addresses: ["192.168.10.10/24"]
      gateway4: 192.168.10.1
    - name: eth1   # its network has no IP address management (NoIPAM)
```

New behavior:

```yaml
network:
  ethernets:
    eth0:
      dhcp4: false
      addresses: ["192.168.10.10/24"]
      gateway4: 192.168.10.1
      nameservers:
        addresses: ["10.0.0.53"]
    eth1:
      dhcp4: false
      nameservers: {}
```

VM Operator doesn't know what addresses or routes the guest will configure on a NoIPAM interface, so eth1 gets neither the defaults nor `spec.network.nameservers`. If it needs DNS, set `nameservers` on the interface.

- **Prior behavior**: eth1 also gets `10.0.0.53, fd00::53`, and would get any VM-level DNS. If those resolvers can't be reached over eth1, lookups sent over it can time out.

### 4. Cloud-Init, DHCP first interface

```yaml
spec:
  bootstrap:
    cloudInit: {}
  network:
    interfaces:
    - name: eth0
      dhcp4: true
    - name: eth1
      addresses: ["192.168.20.10/24"]
      gateway4: 192.168.20.1
```

New behavior:

```yaml
network:
  ethernets:
    eth0:
      dhcp4: true
      nameservers: {}
    eth1:
      dhcp4: false
      addresses: ["192.168.20.10/24"]
      gateway4: 192.168.20.1
      nameservers: {}
```

The first interface isn't static, so the VM has no primary interface and gets no defaults; later interfaces aren't considered. The guest still gets DNS from DHCP on eth0. A NoIPAM first interface, or one with `gateway4: None`, has the same result. To configure DNS anyway, set `spec.network.nameservers`, which goes to every static interface.

- **Prior behavior**: eth1 gets `10.0.0.53, fd00::53`. So does eth0 if it is NoIPAM or static instead of DHCP.

### 5. Cloud-Init, DNS in the VM Spec

```yaml
spec:
  bootstrap:
    cloudInit: {}
  network:
    nameservers: ["192.168.1.53", "fd00:1::53"]
    searchDomains: ["app.example"]
    interfaces:
    - name: eth0
      addresses: ["192.168.10.10/24"]
      gateway4: 192.168.10.1
    - name: eth1
      addresses: ["192.168.20.10/24", "fd00:20::10/64"]
      gateway4: 192.168.20.1
      gateway6: fd00:20::1
    - name: eth2
      dhcp4: true
    - name: eth3
      dhcp4: true
      nameservers: ["172.16.0.53"]
```

New behavior:

```yaml
network:
  ethernets:
    eth0:
      dhcp4: false
      addresses: ["192.168.10.10/24"]
      gateway4: 192.168.10.1
      nameservers:
        addresses: ["192.168.1.53"]
        search: ["app.example"]
    eth1:
      dhcp4: false
      addresses: ["192.168.20.10/24", "fd00:20::10/64"]
      gateway4: 192.168.20.1
      gateway6: fd00:20::1
      nameservers:
        addresses: ["192.168.1.53", "fd00:1::53"]
        search: ["app.example"]
    eth2:
      dhcp4: true
      nameservers: {}
    eth3:
      dhcp4: true
      nameservers:
        addresses: ["172.16.0.53"]
```

The VM-level DNS goes only to the static interfaces, and each nameserver only to interfaces of its IP family. The spec sets nameservers, so the defaults aren't used. eth3 sets its own nameservers, which netplan adds to the ones from DHCP, while eth2 gets only the DNS from DHCP.

- **Prior behavior**: eth0, eth1, and eth2 all get `192.168.1.53, fd00:1::53` and `app.example`, unfiltered. eth3 gets its own nameservers and `app.example`.

### 6. Cloud-Init, DNS in the VM Spec with only DHCP interfaces

```yaml
spec:
  bootstrap:
    cloudInit: {}
  network:
    nameservers: ["192.168.1.53"]
    searchDomains: ["app.example"]
    interfaces:
    - name: eth0
      dhcp4: true
    - name: eth1
      dhcp4: true
```

New behavior:

```yaml
network:
  ethernets:
    eth0:
      dhcp4: true
      nameservers: {}
    eth1:
      dhcp4: true
      nameservers: {}
```

This is the most visible change for Cloud-Init. The webhook still accepts the VM-level DNS, but the VM has no static interface to apply it to, so the guest only uses the DNS from DHCP. To keep the VM-level DNS, set it on each interface that needs it:

```yaml
    interfaces:
    - name: eth0
      dhcp4: true
      nameservers: ["192.168.1.53"]
      searchDomains: ["app.example"]
```

- **Prior behavior**: both interfaces get `192.168.1.53` and `app.example`, which netplan adds to the DNS from DHCP.

### 7. Cloud-Init, DNS from the network provider

```yaml
spec:
  bootstrap:
    cloudInit: {}
  network:
    nameservers: ["192.168.1.53"]
    interfaces:
    - name: eth0   # its VPC SubnetPort reports nameservers 10.1.0.53
      addresses: ["192.168.10.10/24"]
      gateway4: 192.168.10.1
    - name: eth1
      addresses: ["192.168.20.10/24"]
      gateway4: 192.168.20.1
```

New behavior:

```yaml
network:
  ethernets:
    eth0:
      # ...
      nameservers:
        addresses: ["10.1.0.53"]
    eth1:
      # ...
      nameservers:
        addresses: ["192.168.1.53"]
```

The provider's nameservers take the place of interface-level nameservers on eth0, so eth0 doesn't get the VM-level nameservers. Setting `nameservers` on eth0 would override the provider's.

- **Prior behavior**: provider DNS is ignored, and both interfaces get `192.168.1.53`.

### 8. LinuxPrep, DHCP first interface

```yaml
spec:
  bootstrap:
    linuxPrep: {}
  network:
    interfaces:
    - name: eth0
      dhcp4: true
    - name: eth1
      addresses: ["192.168.20.10/24"]
      gateway4: 192.168.20.1
```

New behavior:

```yaml
globalIPSettings: {}
nicSettingMap:
- adapter:   # eth0
    ip: dhcp
- adapter:   # eth1
    ip: 192.168.20.10
    subnetMask: 255.255.255.0
    gateway: ["192.168.20.1"]
```

The first interface isn't static, so the global DNS server list stays empty and eth0 keeps the DNS servers from DHCP. A NoIPAM first interface gives the same result.

- **Prior behavior**: `globalIPSettings.dnsServerList` is `10.0.0.53, fd00::53`, which replaces eth0's DNS servers from DHCP. A NoIPAM first interface also gets the defaults, since the prior behavior only skips DHCP interfaces.

### 9. LinuxPrep, static first interface

```yaml
spec:
  bootstrap:
    linuxPrep: {}
  network:
    interfaces:
    - name: eth0
      addresses: ["192.168.10.10/24"]
      gateway4: 192.168.10.1
    - name: eth1
      dhcp4: true
```

New behavior:

```yaml
globalIPSettings:
  dnsServerList: ["10.0.0.53"]
nicSettingMap:
- adapter:   # eth0
    ip: 192.168.10.10
    subnetMask: 255.255.255.0
    gateway: ["192.168.10.1"]
- adapter:   # eth1
    ip: dhcp
```

Only the first interface is considered. Because the global list overrides DHCP on every interface, eth1 loses its DNS servers from DHCP, the same as with the prior behavior. The default search domains are never applied to LinuxPrep.

- **Prior behavior**: `dnsServerList` is `10.0.0.53, fd00::53`.

### 10. LinuxPrep, DNS in the VM Spec and from the network provider

```yaml
spec:
  bootstrap:
    linuxPrep: {}
  network:
    nameservers: ["192.168.1.53"]
    searchDomains: ["app.example"]
    interfaces:
    - name: eth0   # its SubnetPort reports 10.1.0.53 / eth0.example
      addresses: ["192.168.10.10/24"]
      gateway4: 192.168.10.1
    - name: eth1   # its SubnetPort reports 10.2.0.53, 10.1.0.53
      dhcp4: true
```

New behavior:

```yaml
globalIPSettings:
  dnsServerList: ["192.168.1.53", "10.1.0.53", "10.2.0.53"]
  dnsSuffixList: ["app.example", "eth0.example"]
nicSettingMap:
- adapter:   # eth0
    ip: 192.168.10.10
    subnetMask: 255.255.255.0
    gateway: ["192.168.10.1"]
- adapter:   # eth1
    ip: dhcp
```

LinuxPrep only has global lists, so they get the VM-level values first, then each interface's provider DNS in interface order, without duplicates. eth1's provider nameservers are included even though it uses DHCP, and the global list overrides DHCP on eth1.

- **Prior behavior**: provider DNS is ignored, so `dnsServerList` is `192.168.1.53` and `dnsSuffixList` is `app.example`.

### 11. Sysprep, no DNS in the spec

```yaml
spec:
  bootstrap:
    sysprep: {...}
  network:
    interfaces:
    - name: eth0
      addresses: ["192.168.10.10/24"]
      gateway4: 192.168.10.1
    - name: eth1
      addresses: ["192.168.20.10/24"]
      gateway4: 192.168.20.1
```

New behavior:

```yaml
globalIPSettings: {}
nicSettingMap:
- adapter:   # eth0
    ip: 192.168.10.10
    subnetMask: 255.255.255.0
    gateway: ["192.168.10.1"]
    dnsServerList: ["10.0.0.53"]
- adapter:   # eth1
    ip: 192.168.20.10
    subnetMask: 255.255.255.0
    gateway: ["192.168.20.1"]
```

Windows only uses the per-adapter DNS server lists, so the defaults go on the first adapter. The default search domains are never applied to Sysprep, so a domain-joined VM keeps resolving short names in its Active Directory domain.

- **Prior behavior**: `globalIPSettings.dnsServerList` is `10.0.0.53, fd00::53`. Windows ignores it, so the guest gets no DNS servers.

### 12. Sysprep, DNS in the VM Spec

```yaml
spec:
  bootstrap:
    sysprep: {...}
  network:
    nameservers: ["192.168.1.53"]
    searchDomains: ["app.example"]
    interfaces:
    - name: eth0
      addresses: ["192.168.10.10/24"]
      gateway4: 192.168.10.1
    - name: eth1
      dhcp4: true
    - name: eth2
      addresses: ["192.168.30.10/24"]
      gateway4: 192.168.30.1
      nameservers: ["172.16.0.53"]
    - name: eth3
      dhcp4: true
      nameservers: ["172.17.0.53"]
```

New behavior:

```yaml
globalIPSettings:
  dnsSuffixList: ["app.example"]
nicSettingMap:
- adapter:   # eth0
    ip: 192.168.10.10
    # ...
    dnsServerList: ["192.168.1.53"]
- adapter:   # eth1
    ip: dhcp
- adapter:   # eth2
    ip: 192.168.30.10
    # ...
    dnsServerList: ["172.16.0.53"]
- adapter:   # eth3
    ip: dhcp
    dnsServerList: ["172.17.0.53"]
```

The VM-level nameservers go to every static adapter that doesn't set its own, so eth0 gets them and eth2 keeps its own. eth1 keeps the DNS servers from DHCP, while eth3's own nameservers replace the ones from DHCP. The search domains are global on Windows.

- **Prior behavior**: the VM-level nameservers only go into `globalIPSettings.dnsServerList`, which Windows ignores, so eth0 gets no DNS servers. eth2 and eth3 are the same as with the new behavior.

### 13. Sysprep, DNS from the network provider

```yaml
spec:
  bootstrap:
    sysprep: {...}
  network:
    interfaces:
    - name: eth0   # its SubnetPort reports 10.1.0.53 / vpc.example
      addresses: ["192.168.10.10/24"]
      gateway4: 192.168.10.1
    - name: eth1   # its SubnetPort reports 10.2.0.53
      dhcp4: true
```

New behavior:

```yaml
globalIPSettings:
  dnsSuffixList: ["vpc.example"]
nicSettingMap:
- adapter:   # eth0
    ip: 192.168.10.10
    subnetMask: 255.255.255.0
    gateway: ["192.168.10.1"]
    dnsServerList: ["10.1.0.53"]
- adapter:   # eth1
    ip: dhcp
    dnsServerList: ["10.2.0.53"]
```

The provider's nameservers go on their adapters like interface-level nameservers. On eth1 they replace the DNS servers from DHCP, and since eth0 has its own, it doesn't get the defaults. The provider's search domains go into the global suffix list. On a domain-joined VM, that list has the same effect on short-name resolution that keeps the default search domains out of Sysprep; see the open questions. Setting `nameservers` on an interface overrides the provider's nameservers, but nothing in the spec removes its search domains.

- **Prior behavior**: provider DNS is ignored. `globalIPSettings.dnsServerList` is `10.0.0.53, fd00::53`, which Windows ignores, so eth0 gets no DNS servers.

### 14. Linux VM with no bootstrap method (implicit LinuxPrep)

```yaml
spec:
  # No spec.bootstrap. The VM's guest OS ID is in the Linux family and the
  # VM has no CD-ROM, so VM Operator customizes it with LinuxPrep anyway.
  network:
    interfaces:
    - name: eth0
      addresses: ["192.168.10.10/24"]
      gateway4: 192.168.10.1
```

New behavior:

```yaml
globalIPSettings:
  dnsServerList: ["10.0.0.53"]
  # No dnsSuffixList.
nicSettingMap:
- adapter:   # eth0
    ip: 192.168.10.10
    subnetMask: 255.255.255.0
    gateway: ["192.168.10.1"]
```

Nothing in the VM Spec says this VM uses LinuxPrep. VM Operator has customized such VMs with LinuxPrep since v1alpha1, when the VM's guest OS ID, from its image or `spec.guestID`, is in the Linux family and the VM has no CD-ROM. The new behavior treats it exactly like explicit LinuxPrep. Its nameservers change the same way as in example 9, and like any LinuxPrep VM it gets no default search domains.

- **Prior behavior**: `dnsServerList` is `10.0.0.53, fd00::53`, and `dnsSuffixList` is `corp.local`. The prior behavior decides whether a VM uses GOSC only from `spec.bootstrap`, so it handles this VM like one that isn't customized with GOSC and fills the global search suffixes with the defaults. LinuxPrep then writes them to the guest. An explicit LinuxPrep VM with the same interface doesn't get `corp.local`. This dates from a 2024 refactor; before it, implicit LinuxPrep VMs didn't get the default search domains either.

What changes for this VM specifically is that it loses `corp.local`, which restores the rule from before the refactor. Existing VMs keep `corp.local`, since they stay on the prior behavior. The webhook rejects VM-level DNS for a VM with no bootstrap method, so to get search domains on a new VM, add `spec.bootstrap.linuxPrep` and set `spec.network.searchDomains`.

## Compatibility and rollout

- Because the prior behavior's output is unchanged, VMs pinned to it keep their bootstrap hashes, and VM Operator doesn't reconfigure or re-customize them.
- Multi-NIC VKS clusters will have a mix of DNS layouts during rollout: existing nodes keep the prior behavior and new nodes get the new one, until every node has been replaced.
- Deactivating the capability returns every VM to the prior behavior, including VMs annotated `scoped`. Their bootstrap changes, so VM Operator rewrites the guestinfo of Cloud-Init VMs, and re-customizes LinuxPrep and Sysprep VMs that don't set `customizeAtNextPowerOn` at their next power-on. The annotation is kept, so each VM goes back to its behavior if the capability is reactivated.
- The release note should call out:
  - Only the first interface gets the Supervisor defaults, and only when it has a static IP address and a gateway. The default nameservers are filtered to the IP families it has a gateway for.
  - Cloud-Init no longer copies `spec.network.nameservers` or `spec.network.searchDomains` to DHCP or NoIPAM interfaces. Set them on the interface instead.
  - Sysprep now applies `spec.network.nameservers` to each static adapter, filtered to the adapter's IP families.

## Risks and open questions

Behavior that may surprise users:

- A Cloud-Init VM whose interfaces all use DHCP gets no VM-level DNS (example 6), even though the webhook accepts it. If VKS or CAPV sets VM-level nameservers or search domains on nodes whose interfaces use DHCP, new nodes lose them. This needs to be confirmed with VKS. An admission warning when every interface explicitly uses DHCP may help.
- New Linux VMs with no bootstrap method don't get the default search suffixes (example 14), the same as explicit LinuxPrep VMs. The prior behavior has only applied them since a 2024 refactor. To set search domains, users add `spec.bootstrap.linuxPrep`, since the webhook rejects VM-level DNS without a bootstrap method.
- An interface whose resolvers are on its own subnet, or reachable through `routes`, gets no defaults if it has no gateway. Users set VM-level or interface-level DNS.
- Users can set static addresses on a NoIPAM interface, and Cloud-Init configures them, but the interface still isn't static, so it gets neither VM-level DNS nor the defaults. The prior behavior applied both.
- Family filtering can leave an interface with none of the VM-level nameservers, for example IPv6-only `spec.network.nameservers` on an IPv4-only interface. The defaults aren't used instead, since the VM Spec set nameservers. Users set interface-level nameservers on that interface.
- A VM whose first interface uses DHCP, is on a NoIPAM network, or has no gateway gets no defaults even when a later interface is static (example 4). A VM with a NoIPAM first interface and a static second one gets no DNS at all. Users set VM-level DNS, which implicit LinuxPrep and vAppConfig-only VMs can't do without adding `spec.bootstrap.linuxPrep`.

DNS from the network provider:

- On Sysprep, provider search domains go into the global suffix list (example 13). On a domain-joined Windows VM, a suffix search list replaces the primary and connection-specific suffixes, which is why the default search domains aren't applied to Sysprep. Provider search domains reintroduce that risk without the user asking for it, and the webhook rejects `interfaces[].searchDomains` for Sysprep, so the user can't remove them. Options include not rolling up provider search domains for Sysprep, or only rolling them up when the VM sets `spec.network.searchDomains`.
- On LinuxPrep, users can't override or remove provider DNS, since the webhook rejects interface-level DNS for LinuxPrep. Provider nameservers from any interface, including a DHCP interface, go into the global list and override DHCP on every interface (example 10).
- Templates don't see provider DNS. `.Net.Nameservers` only includes it for LinuxPrep, and the per-interface template data has no DNS fields, so a vAppConfig or Sysprep template that renders `.Net.Nameservers` gets the VM-level nameservers or the defaults, never the provider's.
- Once SubnetPorts report DNS, the bootstrap of existing VMs that use the new behavior changes. VM Operator would rewrite the guestinfo of Cloud-Init VMs, and re-customize GOSC VMs that don't set `customizeAtNextPowerOn`. This needs a plan before it lands, for example shipping it together with the capability.

Other risks:

- The annotations used to detect an existing guest may miss some VMs, such as VMs jump-upgraded from builds that predate the bootstrap hash annotations, or VMs powered on outside VM Operator. One candidate extra signal is that the VM is already powered on, which would select `legacy`.
- The ReplicaSet controller copies its template's annotations onto VMs as a privileged account, so a DevOps user can set `dns-defaults` that way. This affects other privileged annotations too, and is tracked separately.
- The capability key name (owner: WCP capabilities) and the epic ticket haven't been assigned yet.
- Windows E2E coverage of per-adapter nameservers is waiting for a testbed with a second workload network and Windows images.

## Testing

- **Unit tests**: `vmlifecycle` and `network` unit tests cover each bootstrap method with both behaviors, primary interface selection, family filtering, provider DNS, a VKS node with a primary and a secondary network, the annotation pinning, and status.
- **Webhook**: unit tests cover the privileged-only annotation.
- **E2E**: a Context in `vm_guestcustomization.go` that runs when the capability is activated covers Cloud-Init and LinuxPrep.
