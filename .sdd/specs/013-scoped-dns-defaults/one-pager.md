# One-Pager: VM Service Updates to Network Bootstrapping

| Authors | Shepherd | ETA | JIRA |
|---|---|---|---|
| TBD | TBD | TBD | vmop-4255 |

**Status:** WIP. Rollout decisions, provider integration, and additional E2E coverage remain open.

**Draft PR:** [Implementation PR #2005](https://github.com/vmware-tanzu/vm-operator/pull/2005)

## Business Problem

A Supervisor can be configured with cluster wide default DNS nameservers and search domains, stored in the `vmoperator-network-config` ConfigMap. When VM Operator bootstraps a VM's network and the VM Spec doesn't provide DNS, it falls back to these defaults. The rules that decide where the defaults, and the DNS from the VM Spec, end up were written when VMs typically had a single interface, and they apply DNS far more broadly than they should. Resolvers are configured on interfaces that can't reach them, DNS servers from DHCP are overridden, multi-NIC VKS nodes repeat default nameservers across links, and Windows VMs get their nameservers in a setting the guest ignores.

| Bootstrap method | What happens today | Why it is a problem |
|---|---|---|
| Cloud-Init | The default nameservers go on **every** interface that doesn't use DHCP, including NoIPAM interfaces. VM-level DNS is copied to **every** interface, including DHCP and NoIPAM interfaces. | Netplan DNS is per interface, so resolvers are configured on links that can't reach them, and lookups sent over those links can time out. IPv6 resolvers are added to IPv4-only links. |
| Cloud-Init, multi-NIC VKS nodes | Each default nameserver is written to the primary and every static secondary interface. | Repeated per-interface entries can consume the three nameserver slots available to a glibc resolver when the guest combines them into resolv.conf. The exact result depends on the guest resolver and network backend. vmop-4124 |
| LinuxPrep | The default nameservers go into the GOSC **global** DNS server list whenever any interface doesn't use DHCP, even when the first interface does. | GOSC treats a non-empty global list as an override of DHCP. A VM whose primary network uses DHCP loses the DNS servers its DHCP server provides. |
| Sysprep | The default nameservers, **and `spec.network.nameservers`**, go only into the GOSC global DNS server list. | Windows doesn't use the global DNS server list, only the per-adapter lists, so the VM gets no DNS servers from either the defaults or the VM Spec. |
| Implicit LinuxPrep (Linux VM with no bootstrap method) | Supervisor search domains are applied. | Explicit LinuxPrep VMs do not get them, so DNS depends on whether the bootstrap method was specified. |

## Glossary

| Term | Meaning |
|---|---|
| Supervisor defaults | `nameservers` and `searchsuffixes` in `vmoperator-network-config`, in VM Operator's namespace. |
| VM-level DNS | `spec.network.nameservers` and `spec.network.searchDomains`. |
| Interface-level DNS | `spec.network.interfaces[].nameservers` and `.searchDomains`. |
| Provider DNS | Nameserver addresses reported for an interface by its network provider via the network interface CR; SubnetPort API changes are still pending. Providers do not supply search domains. |
| Interface DNS | Interface-level nameservers, falling back to provider nameservers. Search domains resolve separately from the VM Spec and Supervisor defaults. |
| NoIPAM | Network with no IP address management; addressing is left to the guest. |
| Static interface | At least one static address, no DHCPv4 or DHCPv6, and not NoIPAM. |
| Address families | Static IPv4/IPv6 address families, plus IPv6 when Router Advertisements (RA) are accepted. |
| Gateway families | Families with a static address and a non-empty gateway, plus IPv6 when RA is accepted. `gateway6: None` clears the static gateway; it does not turn off RA. |
| Primary interface | The first interface in VM Spec order, only when static and with at least one gateway family. This is the same first-adapter convention used by GOSC. |
| GOSC | vSphere Guest OS Customization, used by LinuxPrep and Sysprep. |
| VKS / TKG VM | A Kubernetes cluster node identified by the CAPV or CAPW cluster-role label. Default search domains apply only to these VMs using Cloud-Init. |
| Legacy / scoped | The unchanged existing DNS behavior / the new behavior selected by the capability and VM annotation. |

## Goals

- Configure Supervisor DNS defaults on the intended primary link, instead of repeating them on secondary or unmanaged links.
- For Cloud-Init and Sysprep, apply VM-level nameservers only to static interfaces, filtered to their address families; apply Supervisor nameservers only to the primary interface, filtered to its gateway families.
- Preserve explicit interface DNS and VM-level DNS ahead of Supervisor defaults. For per-interface nameservers, provider nameservers sit between interface-level and VM-level nameservers. LinuxPrep merges VM-level and provider nameservers in that order without duplicates. Providers do not supply search domains.
- Set VM-level and default nameservers to Windows through the per-adapter GOSC fields it honors.
- Keep already-configured VMs detected by the existing-guest markers on legacy behavior when the capability is activated, with an explicit per-VM override.

## Non Goals

- New VM API fields, schema changes, or changes to which DNS fields admission accepts.
- DHCP override support.
- Per-adapter Sysprep search domains, or applying Supervisor search domains to LinuxPrep, Sysprep, or ordinary non-cluster Cloud-Init VMs.
- DNS auto-records, PodVM proxies, ImageFetcher, and DNS conflict reporting.

## Big Picture

The network provider supplies the initial addresses, gateways, and DHCP settings; VM Spec interface settings overlay them. The Supervisor capability and the VM's pinned mode select legacy or scoped DNS behavior. The bootstrap method renders the resulting netplan or GOSC customization spec.

| DNS value | Cloud-Init | LinuxPrep | Sysprep |
|---|---|---|---|
| Interface/provider nameservers | Per interface, unfiltered | Global, unfiltered: VM-level first, then provider nameservers in interface order, without duplicates | Per adapter, unfiltered |
| VM-level nameservers | Each static interface lacking its own; address-family filter | Global, unfiltered | Each static adapter lacking its own; address-family filter |
| Supervisor nameservers | Primary interface only when VM-level and interface/provider nameservers are empty; gateway-family filter | Global if merged list empty and primary exists; gateway-family filter from primary | Primary adapter only when VM-level and interface/provider nameservers are empty; gateway-family filter |
| VM search domains | Per-interface precedence; VM-level copy only to static interfaces | VM-level only | VM-level only |
| Supervisor search domains | For VKS VMs only: primary interface when VM-level and interface search domains are empty | Not applied | Not applied |

Nameservers and search domains fall back independently. A non-empty VM-level nameserver list prevents Supervisor nameserver fallback even when family filtering removes every entry.

## User Workflows

### DevOps persona

For a new Cloud-Init VM, place nameservers and search domains at VM level to use them on static interfaces that lack their own DNS. Interface DNS takes precedence independently for nameservers and search domains. Examples 5, 6, 15, and 16 show the resulting configurations.

For a Windows VM, use VM-level nameservers for static adapters or interface-level nameservers for an explicit adapter override. Set search domains at VM level; Windows has one global suffix search list. Examples 11 and 12 show the GOSC fields.

For LinuxPrep, set DNS at VM level. Linux customization only applies global lists, so any non-empty nameserver list also overrides DHCP on other interfaces. Example 9 shows this remaining limitation. A Linux VM with no bootstrap spec can add `spec.bootstrap.linuxPrep` to make VM-level DNS fields available to admission.

With the scoped DNS capability enabled, VM Operator automatically pins each VM to a mode:

| VM state | Selected behavior | Annotation action |
|---|---|---|
| no mode, existing-guest marker present | Legacy | Set `legacy` |
| no mode, no existing-guest marker | Scoped | Set `scoped` |
| mode `legacy` | Legacy | Retain it |
| mode `scoped` | Scoped | Retain it |
| unknown non-empty mode | Legacy | Retain it |

Users who can edit a VM can add, change, or remove `vmoperator.vmware.com/dns-defaults` without privileged-account status. Set `legacy` to select the prior behavior or `scoped` to select scoped behavior. Removing or clearing the annotation resets automatic selection: the next bootstrap reconcile writes `legacy` when existing-guest markers are present and `scoped` otherwise. Changing a VM's pinned mode can change its bootstrap hash and guest configuration.

## Architecture Areas

### How each bootstrap method applies DNS

Cloud-Init writes netplan to `guestinfo.metadata`. LinuxPrep and Sysprep use vSphere Guest OS Customization (GOSC). The table shows the guest settings each method honors and the DNS fields accepted by admission.

|  | Cloud-Init | LinuxPrep | Sysprep | vAppConfig only, or no bootstrap |
|---|---|---|---|---|
| **Guest DNS servers** | Per interface (netplan) | Global only; Linux GOSC ignores the per-adapter list | Per adapter only; Windows ignores the global list | Not configured by VM Operator; templates may use `.Net.Nameservers` |
| **Guest search domains** | Per interface (netplan) | Global only | Global only | Not configured by VM Operator |
| **Interaction with DHCP** | Netplan adds the configured nameservers to DHCP's | A non-empty global list overrides DHCP on every interface | A non-empty adapter list overrides DHCP on that adapter | — |
| `spec.network.nameservers` | Allowed when `useGlobalNameserversAsDefault` is unset or true | Allowed | Allowed | Rejected |
| `spec.network.searchDomains` | Allowed when `useGlobalSearchDomainsAsDefault` is unset or true | Allowed | Allowed | Rejected |
| `interfaces[].nameservers` | Allowed | Rejected | Allowed | Rejected |
| `interfaces[].searchDomains` | Allowed | Rejected | Rejected | Rejected |

A Linux VM without a bootstrap spec and without a CD-ROM is implicitly customized with LinuxPrep (historical VM Operator behavior), but the webhook treats it as having no bootstrap method, so none of its DNS fields can be set. Adding `spec.bootstrap.linuxPrep`, which the webhook allows after creation, makes them available.

### Bootstrap exceptions

- Explicit interface DNS applies regardless of addressing mode. For Cloud-Init, `useGlobal*AsDefault` controls copying the matching VM-level values; it does not affect interface DNS, provider nameservers, or Supervisor defaults (example 17).
- A gateway is required for Supervisor defaults even when a resolver is reachable on the local subnet or through `routes`. Set VM-level or interface-level DNS for those links.

### Network provider DNS

SubnetPort `status.dnsResolverIPs` and its ingestion by `bootstrapFromVPC` are pending. Examples 7, 10, 13, and 18 use synthetic provider input.

For Cloud-Init and Sysprep, interface-level nameservers replace provider nameservers, which take precedence over VM-level and Supervisor nameservers. Provider nameservers apply regardless of addressing mode and are unfiltered. LinuxPrep merges them globally, so users cannot preserve per-link resolver scope or remove them through interface fields, which admission rejects. Legacy mode ignores provider nameservers.

### Status and templates

| Effective network bootstrap engine | Global `status.network.config.dns` | Interface DNS status |
|---|---|---|
| Cloud-Init | No nameservers or search domains | Netplan DNS applied per interface |
| LinuxPrep, explicit or implicit | Applied global nameservers and search domains | Rolled-up DNS cleared from interfaces |
| Sysprep | Applied global search domains; no nameservers | Per-adapter nameservers; no per-adapter search domains |
| No network bootstrap engine | Resolved global DNS for users/templates configuring the guest | Available interface/provider DNS on interfaces with static or DHCP IP configuration; not proof it was configured in the guest |

No network bootstrap engine means vAppConfig only, bootstrap disabled, or no effective bootstrap method. It excludes a Linux VM without a bootstrap spec that qualifies for implicit LinuxPrep. These status fields describe VM Operator's configuration, not DNS learned by the guest through DHCP. A global DNS block can still contain `hostName` or `domainName` when its nameservers and search domains are empty.

Bootstrap templates continue to use `.Net.Nameservers`. It is the VM-level list, with rolled-up interface/provider nameservers for LinuxPrep, falling back to the unfiltered Supervisor list when needed. Sysprep and vAppConfig template lists do not incorporate provider DNS or the per-adapter family filter. Scoped Sysprep and vAppConfig resolution can now read the ConfigMap even when every adapter has DNS, whereas legacy could leave the template list empty. Legacy template output remains unchanged. Example 19 contrasts status and template values.

### Capability and mode selection

The Supervisor capability is `supports_vm_service_scoped_dns_defaults`. Once enabled, it remains enabled. When bootstrap arguments are next resolved, automatic mode selection uses the bootstrap hash, `first-boot-done`, `restored-vm`, `imported-vm`, or `failed-over-vm` annotations as existing-guest markers.

Pinning prevents capability activation from rewriting Cloud-Init guestinfo or re-customizing LinuxPrep and Sysprep VMs at their next power-on when `customizeAtNextPowerOn` is unset.

### Compatibility and rollout

- Because the prior behavior's output is unchanged, VMs pinned to it do not acquire a changed bootstrap hash merely from capability activation. Subsequent user or network changes can still cause normal bootstrap updates.
- Multi-NIC VKS clusters will have a mix of DNS layouts during rollout: existing nodes keep the prior behavior and new nodes get the new one, until every node has been replaced.

The release note should call out:

- Only the first interface gets the Supervisor defaults, and only when it has a static IP address and a gateway. The default nameservers are filtered to the IP families it has a gateway for.
- Cloud-Init no longer copies `spec.network.nameservers` or `spec.network.searchDomains` to DHCP or NoIPAM interfaces. Set them on the interface instead.
- Sysprep now applies `spec.network.nameservers` to each static adapter, filtered to the adapter's IP families.

### Risks and open questions

- Decide whether admission should warn when Cloud-Init VM-level DNS is accepted but all interfaces use DHCP (example 6).
- Existing-guest detection may miss VMs upgraded from builds predating bootstrap hash annotations or powered on outside VM Operator. Consider whether a powered-on VM should also select `legacy`.
- Interface status currently omits provider DNS when the interface has neither static nor DHCP IP configuration. Provider integration needs coverage for this case, especially NoIPAM.

### Supervisor DNS integration requirements

The Supervisor DNS proposal (WIKI page 2485688854) requires the following work beyond the current bootstrap plumbing:

| Area | Remaining requirement |
|---|---|
| Migration (WIKI page 2485688854, comment 2512616389) | Define migration for already-scoped VMs before SubnetPorts supply DNS. New provider values change their bootstrap and can trigger Cloud-Init guestinfo updates or GOSC re-customization. |
| Resolver validation (WIKI page 2485688854, comment 3126531699) | Before each node switches to the VPC resolver, automated checks must verify required workload-name resolution, including forwarding to Supervisor workload DNS where needed. These checks remain VKS/NSX integration work. |
| Precedence and DHCP (WIKI page 2485688854, comment 2517809043) | Agree on the contract with the upstream VM CR > DHCP > VPC resolver > Supervisor proposal. Current Cloud-Init/Sysprep behavior places provider nameservers ahead of VM-level nameservers. Cloud-Init adds them to DHCP DNS; Sysprep overrides DHCP per adapter; LinuxPrep overrides it globally. DHCP override support remains separate. |
| Provider integration (WIKI page 2485688854, comment 2912673119) | NSX Operator must publish resolver IPs for the VPC associated with each SubnetPort, and VM Operator must ingest them. Resolver reachability requires validation outside this bootstrap change. |

### Testing

- Network unit tests cover interface/provider precedence, static and primary selection, address/gateway families, and ordered family filtering. Non-IP nameserver entries are preserved by the filter.
- Bootstrap argument unit tests cover legacy/scoped modes, every existing-guest marker, unknown annotations, each bootstrap engine, DHCP/NoIPAM, VM-level DNS, cluster labels, provider-DNS rollup, and status/template separation.
- Capability tests cover the feature mapping; webhook unit tests verify that non-privileged users can create a VM with either mode, add the annotation, change it in both directions, and remove it. Bootstrap tests cover automatic reselection after removal or clearing and explicit scoped selection on an existing guest.

Current E2E coverage is capability-gated and `experimental`: Cloud-Init metadata/global status, LinuxPrep global status, and non-admin mode edits/removal with automatic legacy reselection. The following five scenarios extend coverage to fixed multi-NIC layouts and guest DNS, with the capability already enabled:

| Scenario | Setup and essential assertions |
|---|---|
| Legacy and scoped VMs coexist | Create a Cloud-Init VM with `vmoperator.vmware.com/dns-defaults: legacy` and an equivalent VM without the annotation. Verify legacy output for the first VM and automatic `scoped` selection with scoped output for the second. Subsequent unchanged reconciles preserve each VM's mode, metadata, bootstrap hash, and guest DNS. |
| Cloud-Init defaults on a multi-NIC VKS node | Two static interfaces, the first with an IPv4 gateway, no VM/interface/provider DNS, and mixed IPv4/IPv6 Supervisor nameservers. Only the first interface receives Supervisor nameservers and search domains, and only IPv4 nameservers survive filtering. |
| Cloud-Init VM-level DNS preserves DHCP DNS | One IPv4 static interface and one DHCP interface; VM-level IPv4/IPv6 nameservers and search domains supplied. Only the static interface receives VM-level DNS, with nameservers filtered to IPv4. The DHCP interface retains its leased DNS, and neither interface receives Supervisor defaults. |
| LinuxPrep respects the first interface | With DHCP first and static second, explicit LinuxPrep supplies no Supervisor nameservers and preserves DHCP DNS. With static first and DHCP second, implicit LinuxPrep applies Supervisor nameservers filtered to the first interface's gateway families globally, overriding DHCP DNS as expected. Neither variant receives Supervisor search domains. Both omit VM-level and provider DNS. |
| Windows receives VM-level DNS | Sysprep with an IPv4 static adapter, a DHCP adapter, VM-level IPv4/IPv6 nameservers, and VM-level search domains. Windows uses only the IPv4 VM-level nameservers on the static adapter; the DHCP adapter retains its leased DNS. The GOSC global nameserver list is empty, and the global suffix list contains only the VM-level search domains. |

Check guest resolver settings and resolution of a known name alongside generated netplan/GOSC configuration and corresponding global/interface DNS status. Use distinct, non-empty Supervisor, VM-level, and DHCP DNS inputs; IPv4 networking with mixed-family DNS inputs covers family filtering. The multi-NIC cases need a second workload network; Windows needs a suitable image and guest access.

Retain the non-admin annotation test. Automatic pinning of existing VMs and the full precedence, family, RA, NoIPAM, and existing-guest marker combinations remain unit coverage. Capability activation against existing VMs can be exercised separately in upgrade testing.

When SubnetPort ingestion lands, add an E2E proving that real `status.dnsResolverIPs` values reach a scoped Cloud-Init guest while VM-level search domains remain intact.

## References

Supervisor DNS proposal and VM/VKS integration discussion (WIKI page 2485688854).

- [Draft implementation PR #2005](https://github.com/vmware-tanzu/vm-operator/pull/2005).
- Feature spec and implementation plan: [`spec.md`](./spec.md), [`plan.md`](./plan.md); pending integration and test work: [`tasks.md`](./tasks.md).
- [Netplan DNS and DHCP configuration](https://netplan.readthedocs.io/en/stable/netplan-yaml/).
- [Linux resolver nameserver limit](https://www.man7.org/linux/man-pages/man5/resolv.conf.5.html).
- [Windows suffix search behavior](https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/unattend/microsoft-windows-dns-client-dnssuffixsearchorder).

## Notes

### Examples

Examples show focused VM Spec fragments, the DNS-related configuration generated in scoped mode, and the difference from legacy mode. They are not complete deployable manifests: image, class, network references, bootstrap content, and unrelated guest settings are omitted. They all assume the Supervisor's ConfigMap has:

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

Unless stated otherwise, the capability is activated and the VM has selected `scoped`. Interfaces shown with static addresses are on networks with IPAM, with DHCPv4, DHCPv6, and Router Advertisements disabled unless an example says otherwise. An address in the VM Spec alone does not establish this: provider addressing mode still matters. Provider-DNS examples 7, 10, 13, and 18 use synthetic provider input; reading those values from SubnetPorts is a follow-up.

How to read the output:

- For Cloud-Init, the output is the netplan VM Operator writes to `guestinfo.metadata`. The excerpts leave out `match`, `set-name`, `dhcp6`, and `accept-ra`. An interface without DNS has an empty `nameservers` block.
- For LinuxPrep and Sysprep, the output is the relevant part of the vSphere customization spec: `globalIPSettings`, and each adapter in `nicSettingMap` in interface order. An adapter's `ip` is shortened to its fixed address, or `dhcp`.
- Addresses are set in the fragments to show their IP families. Addresses from a static IP pool behave the same. Sysprep fragments omit the required identity and credentials; GOSC output is explanatory YAML, not a Kubernetes resource.
- `nameservers: {}` means VM Operator supplies no DNS on that interface. It does not show DNS learned by the guest from DHCP or other sources.
- Nameserver entries retain source order after filtering. Explicit interface DNS and provider DNS are never family filtered; only copied VM-level DNS and Supervisor defaults are filtered.

#### Changes at a glance

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
| DNS from the network provider (future input) | Ignored | Treated as interface-level DNS; global rollup where required | 7, 10, 13, 18 |
| Cloud-Init or Sysprep, VM nameservers of another family | Cloud-Init copies unfiltered; Sysprep writes unused global list | No matching nameservers and no fallback to Supervisor defaults | 15 |
| Cloud-Init, NoIPAM with explicit addresses | DNS copied/defaulted as on other non-DHCP links | No VM-level or Supervisor DNS; explicit interface DNS still works | 16 |
| Cloud-Init, useGlobal flags false | Suppress VM-level copy only | Still do not suppress Supervisor defaults | 17 |
| Status versus templates | Resolved global DNS reused for both | Applied DNS reported by engine; template resolution kept separate | 19 |

#### 1. Cloud-Init, VKS node with a primary and a secondary network

```yaml
metadata:
  labels:
    capv.vmware.com/cluster.role: worker
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

The defaults go only on the primary network. eth0 only has an IPv4 gateway, so it only gets the IPv4 default nameserver. VM Operator writes one default nameserver, however many secondary networks it has; explicit or provider DNS on secondary links can still add other resolvers.

- **Prior behavior**: both interfaces get `10.0.0.53, fd00::53` and `corp.local`. This produces four configured entries. If the guest merges them without deduplication into resolv.conf, they exceed the three-server glibc limit.
- **Non-VKS VM**: eth0 gets the same nameservers, without the search domains. With the prior behavior, both interfaces get the nameservers, and neither gets search domains.

#### 2. Cloud-Init, dual-stack first interface with `gateway6: None`

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

With Router Advertisements disabled, eth0 has an IPv6 address but no IPv6 gateway, so it doesn't get the IPv6 default nameserver. With a `gateway6`, it would get both. If `gateway4` were also `None`, it would get no defaults at all.

- **Prior behavior**: eth0 gets `10.0.0.53, fd00::53`.

#### 3. Cloud-Init, static interface and a NoIPAM interface

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

#### 4. Cloud-Init, DHCP first interface

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

The first interface isn't static, so the VM has no primary interface and gets no defaults; later interfaces aren't considered. The guest still gets DNS from DHCP on eth0. A NoIPAM first interface, or an IPv4-only first interface with `gateway4: None` and no IPv6 Router Advertisements, has the same result. To configure DNS anyway, set `spec.network.nameservers`, which goes to every static interface.

- **Prior behavior**: eth1 gets `10.0.0.53, fd00::53`. So does eth0 if it is NoIPAM or static instead of DHCP.

#### 5. Cloud-Init, DNS in the VM Spec

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

#### 6. Cloud-Init, DNS in the VM Spec with only DHCP interfaces

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

The webhook still accepts the VM-level DNS, but the VM has no static interface to apply it to, so the guest only uses the DNS from DHCP. To supply this DNS on DHCP links, set it on each interface that needs it:

```yaml
    interfaces:
    - name: eth0
      dhcp4: true
      nameservers: ["192.168.1.53"]
      searchDomains: ["app.example"]
```

- **Prior behavior**: both interfaces get `192.168.1.53` and `app.example`, which netplan adds to the DNS from DHCP.

#### 7. Cloud-Init, DNS from the network provider (future provider input)

```yaml
spec:
  bootstrap:
    cloudInit: {}
  network:
    nameservers: ["192.168.1.53"]
    interfaces:
    - name: eth0   # future provider input supplies nameservers 10.1.0.53
      addresses: ["192.168.10.10/24"]
      gateway4: 192.168.10.1
    - name: eth1
      addresses: ["192.168.20.10/24"]
      gateway4: 192.168.20.1

# Synthetic provider input (future SubnetPort API; not part of the VM Spec).
---
# SubnetPort status fragment for eth0
status:
  dnsResolverIPs: ["10.1.0.53"]
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

#### 8. LinuxPrep, DHCP first interface

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

#### 9. LinuxPrep, static first interface

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

#### 10. LinuxPrep, DNS in the VM Spec and from the network provider (future provider input)

```yaml
spec:
  bootstrap:
    linuxPrep: {}
  network:
    nameservers: ["192.168.1.53"]
    searchDomains: ["app.example"]
    interfaces:
    - name: eth0   # future provider input supplies 10.1.0.53
      addresses: ["192.168.10.10/24"]
      gateway4: 192.168.10.1
    - name: eth1   # future provider input supplies 10.2.0.53, 10.1.0.53
      dhcp4: true

# Synthetic provider input (future SubnetPort API; not part of the VM Spec).
---
# SubnetPort status fragment for eth0
status:
  dnsResolverIPs: ["10.1.0.53"]
---
# SubnetPort status fragment for eth1
status:
  dnsResolverIPs: ["10.2.0.53", "10.1.0.53"]
```

New behavior:

```yaml
globalIPSettings:
  dnsServerList: ["192.168.1.53", "10.1.0.53", "10.2.0.53"]
  dnsSuffixList: ["app.example"]
nicSettingMap:
- adapter:   # eth0
    ip: 192.168.10.10
    subnetMask: 255.255.255.0
    gateway: ["192.168.10.1"]
- adapter:   # eth1
    ip: dhcp
```

LinuxPrep's global nameserver list gets the VM-level nameservers first, then each interface's provider nameservers in interface order without duplicates. eth1's provider nameservers are included even though it uses DHCP, and the global list overrides DHCP on eth1. The suffix list contains only the VM-level `app.example`; providers supply no search domains.

- **Prior behavior**: provider DNS is ignored, so `dnsServerList` is `192.168.1.53` and `dnsSuffixList` is `app.example`.

#### 11. Sysprep, no DNS in the spec

```yaml
spec:
  bootstrap:
    sysprep: {}  # Identity and credentials omitted from this fragment.
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

Windows only uses the per-adapter DNS server lists, so the defaults go on the first adapter. The default search domains are never applied to Sysprep, so VM Operator does not introduce a Supervisor suffix search list that could disrupt short-name resolution in a domain-joined VM.

- **Prior behavior**: `globalIPSettings.dnsServerList` is `10.0.0.53, fd00::53`. Windows ignores it, so the guest gets no DNS servers.

#### 12. Sysprep, DNS in the VM Spec

```yaml
spec:
  bootstrap:
    sysprep: {}  # Identity and credentials omitted from this fragment.
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

#### 13. Sysprep, DNS from the network provider (future provider input)

```yaml
spec:
  bootstrap:
    sysprep: {}  # Identity and credentials omitted from this fragment.
  network:
    interfaces:
    - name: eth0   # future provider input supplies 10.1.0.53
      addresses: ["192.168.10.10/24"]
      gateway4: 192.168.10.1
    - name: eth1   # future provider input supplies 10.2.0.53
      dhcp4: true

# Synthetic provider input (future SubnetPort API; not part of the VM Spec).
---
# SubnetPort status fragment for eth0
status:
  dnsResolverIPs: ["10.1.0.53"]
---
# SubnetPort status fragment for eth1
status:
  dnsResolverIPs: ["10.2.0.53"]
```

New behavior:

```yaml
globalIPSettings: {}
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

The provider's nameservers go on their adapters like interface-level nameservers. On eth1 they replace DHCP DNS, and eth0 does not get Supervisor nameservers because the provider supplies its own. The global suffix list stays empty: providers do not supply search domains, and this VM specifies none. Setting `nameservers` on an interface overrides the provider's nameservers.

- **Prior behavior**: provider DNS is ignored. `globalIPSettings.dnsServerList` is `10.0.0.53, fd00::53`, which Windows ignores, so eth0 gets no DNS servers.

#### 14. Linux VM with no bootstrap method (implicit LinuxPrep)

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

This Linux VM uses implicit LinuxPrep. Scoped mode gives it the same DNS as explicit LinuxPrep (example 9), with no Supervisor search domains. Legacy mode keeps `dnsServerList: ["10.0.0.53", "fd00::53"]` and `dnsSuffixList: ["corp.local"]`; the implicit-only suffix defaulting dates from a 2024 refactor. Existing VMs pinned to legacy retain it. To set search domains explicitly, add `spec.bootstrap.linuxPrep` and `spec.network.searchDomains`, since admission rejects VM-level DNS without a bootstrap method.

#### 15. Cloud-Init, family filtering does not fall back to defaults

```yaml
spec:
  bootstrap:
    cloudInit: {}
  network:
    nameservers: ["fd00:1::53"]
    interfaces:
    - name: eth0
      addresses: ["192.168.10.10/24"]
      gateway4: 192.168.10.1
```

Scoped output:

```yaml
network:
  ethernets:
    eth0:
      dhcp4: false
      addresses: ["192.168.10.10/24"]
      gateway4: 192.168.10.1
      nameservers: {}
```

The IPv6 VM-level nameserver is filtered out, but the non-empty VM-level list still suppresses Supervisor nameservers. Set an IPv4 interface nameserver to supply DNS on this link. With Sysprep, the same input leaves the adapter without operator-supplied DNS; LinuxPrep instead keeps the IPv6 VM-level nameserver in its unfiltered global list.

**Legacy:** Cloud-Init copies `fd00:1::53` onto eth0. Sysprep writes it only to the unused global server list.

Additional primary-interface cases, with no explicit or provider DNS:

| First interface after provider/spec resolution | Scoped default nameservers | Reason |
|---|---|---|
| IPv6-only static address and IPv6 gateway | `fd00::53` | Only an IPv6 gateway family |
| Dual-stack static addresses and both gateways | `10.0.0.53`, `fd00::53` | Both gateway families |
| Static IPv4 address, no IPv4 gateway, IPv6 RA accepted, no DHCP | `fd00::53` | RA contributes IPv6 address and gateway families; the static IPv4 address satisfies the static-interface test |
| Dual-stack static addresses, `gateway6: None`, IPv6 RA still accepted | `10.0.0.53`, `fd00::53` when an IPv4 gateway exists | `None` clears the static IPv6 gateway, not RA acceptance |
| RA-only interface with no static address | None | RA alone does not satisfy the static-interface test |
| Static IPv4 plus DHCPv6 | None | Either DHCP family excludes the entire interface from static defaulting |
| Static first interface without any gateway; static second interface with gateway | None | A later interface is never promoted to primary |

These rows describe resolved network inputs, including RA reported by a provider, rather than additional VM API fields. Under legacy Cloud-Init, every non-DHCP interface lacking its own DNS gets both default families regardless of gateways; a later eligible interface can also get defaults.

#### 16. Cloud-Init, NoIPAM with explicit addresses

The provider reports NoIPAM for eth0, even though the user specifies an address and a gateway:

```yaml
spec:
  bootstrap:
    cloudInit: {}
  network:
    nameservers: ["192.168.1.53"]
    interfaces:
    - name: eth0  # NoIPAM network
      addresses: ["192.168.10.10/24"]
      gateway4: 192.168.10.1
```

Scoped output:

```yaml
network:
  ethernets:
    eth0:
      dhcp4: false
      addresses: ["192.168.10.10/24"]
      gateway4: 192.168.10.1
      nameservers: {}
```

Cloud-Init configures the explicit address, but NoIPAM remains true, so eth0 receives neither the VM-level nameserver nor Supervisor defaults. Setting `interfaces[0].nameservers: ["192.168.1.53"]` applies that DNS. Without VM-level DNS, the result remains empty rather than falling back to the ConfigMap.

**Legacy:** the VM-level nameserver is copied to eth0; without it, both ConfigMap nameservers are defaulted.

#### 17. Cloud-Init, the useGlobal flags do not disable Supervisor defaults

```yaml
metadata:
  labels:
    capv.vmware.com/cluster.role: worker
spec:
  bootstrap:
    cloudInit:
      useGlobalNameserversAsDefault: false
      useGlobalSearchDomainsAsDefault: false
  network:
    interfaces:
    - name: eth0
      addresses: ["192.168.10.10/24"]
      gateway4: 192.168.10.1
    - name: eth1
      addresses: ["192.168.20.10/24"]
      gateway4: 192.168.20.1
```

Scoped DNS output:

```yaml
network:
  ethernets:
    eth0:
      nameservers:
        addresses: ["10.0.0.53"]
        search: ["corp.local"]
    eth1:
      nameservers: {}
```

These flags control copying matching VM-level values only. The webhook rejects a non-empty VM-level value when its flag is false, so the example omits both values. Explicit interface DNS and Supervisor defaults are unaffected by the flags.

**Legacy:** both non-DHCP interfaces get both default nameservers and `corp.local`, despite the false flags.

#### 18. Cloud-Init, nameservers and search domains resolve independently (future provider input)

Synthetic provider input supplies eth0 with nameservers `10.1.0.53` only:

```yaml
spec:
  bootstrap:
    cloudInit: {}
  network:
    nameservers: ["192.168.1.53"]
    searchDomains: ["app.example"]
    interfaces:
    - name: eth0
      addresses: ["192.168.10.10/24"]
      gateway4: 192.168.10.1
      nameservers: ["172.16.0.53"]
    - name: eth1
      addresses: ["192.168.20.10/24"]
      gateway4: 192.168.20.1

# Synthetic provider input (future SubnetPort API; not part of the VM Spec).
---
# SubnetPort status fragment for eth0
status:
  dnsResolverIPs: ["10.1.0.53"]
```

Scoped DNS output:

```yaml
network:
  ethernets:
    eth0:
      nameservers:
        addresses: ["172.16.0.53"]
        search: ["app.example"]
    eth1:
      nameservers:
        addresses: ["192.168.1.53"]
        search: ["app.example"]
```

The interface nameservers replace the provider nameservers, while eth0 independently receives the VM-level `app.example` search domain. Setting only interface search domains would leave provider nameservers intact; removing eth0's nameservers would select the provider's `10.1.0.53` and keep `app.example`. Neither interface nor provider nameservers are family filtered. DNS on eth1 alone does not suppress defaults on eth0 when the matching VM-level field and eth0 DNS are empty.

**Legacy:** provider DNS is ignored; eth0 uses its explicit nameservers and the VM-level `app.example` search domain. Eth1 is unchanged.

#### 19. Applied DNS in status versus nameservers in templates

For the VM in example 1, named `example-vm`, the scoped Cloud-Init status excerpt is:

```yaml
status:
  network:
    config:
      interfaces:
      - name: eth0
        dns:
          nameservers: ["10.0.0.53"]
          searchDomains: ["corp.local"]
      - name: eth1
        # No DNS block; other interface status fields omitted.
      dns:
        hostName: example-vm
        # No global nameservers or searchDomains.
```

The absence of global nameservers and search domains describes the engine's configuration scope, not a guest DNS failure. The hostname is still reported globally. Legacy mode also reports resolved global DNS, so consumers must read interface DNS for scoped Cloud-Init.

For example 11, scoped Sysprep reports `10.0.0.53` only on the first interface and no global nameservers. Template `.Net.Nameservers` nevertheless contains the unfiltered resolved ConfigMap list, `["10.0.0.53", "fd00::53"]`. Setting VM-level nameservers changes the template list to that VM-level list even when no adapter can use its families. This separates template input from applied configuration.

For vAppConfig-only VMs, VM Operator does not configure the network: global status and `.Net.Nameservers` expose resolved nameservers for guest configuration by the template. In a fixture where both interfaces have provider DNS, scoped global status and template nameservers still contain the ConfigMap list; the provider DNS is reported per interface and is not added to template network data. A Linux VM with no bootstrap spec and no CD-ROM instead uses implicit LinuxPrep, as in example 14.
