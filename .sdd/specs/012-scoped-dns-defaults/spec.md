# Feature Specification: Scoped Guest DNS Defaults

- **Feature branch**: `bryanv/network-dns-bootstrap-changes`
  - **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-09-26
- **Status**: Draft
- **Epic**: TBD <!-- [NEEDS CLARIFICATION: epic ticket] -->

---

## Summary

The Supervisor defines default, "global" DNS nameservers and search domains in the `vmoperator-network-config` ConfigMap. VM Operator falls back to those defaults when bootstrapping a VM's guest networking. Today it applies them, and the VM-level DNS, too broadly:

- **Cloud-Init**: the default nameservers are written to **every** non-DHCP interface in the generated netplan, including interfaces without IP management (NoIPAM). The VM-level DNS is copied to every interface, including DHCP and NoIPAM interfaces. Netplan's DNS is per interface, so the same resolvers end up configured on links that may not be able to reach them, including resolvers of an IP family the link does not have.
- **LinuxPrep**: the default nameservers are written to the customization's global DNS server list whenever the VM sets no VM-level nameservers and has any non-DHCP interface, even a NoIPAM one, and even when its first interface uses DHCP. Guest OS Customization (GOSC) treats a non-empty global list as an override of DNS learned from DHCP.
- **Sysprep**: the default nameservers, and `spec.network.nameservers`, are written only to the global list. Windows DNS servers are configured per adapter, and GOSC documents the per-adapter list as the one Windows uses.
- **VMs with no bootstrap provider**: Linux VMs without a bootstrap provider are customized with LinuxPrep. Since `6c4fbc485` (2024-03), which moved the DNS logic out of `DoBootstrap` so it could be reported in status, the operator decides whether search domains apply from the raw `spec.bootstrap`, as if these VMs used no GOSC engine at all. As a result, they receive the default search domains even though explicit LinuxPrep VMs never do. Before that change, implicit LinuxPrep counted as GOSC and did not receive them, and the v1alpha1 provider never set a GOSC suffix list at all.

This feature adds a second mode, **Scoped**. In Scoped mode, DNS in the VM spec always wins, VM-level DNS is applied only to interfaces with a static IP address, and a global default is applied only to the VM's first interface, and only when that interface has a static IP address with a gateway and nothing in the VM spec provides its DNS. Nameservers are only applied to an interface when they match its IP families, and the default nameservers only when the interface has a gateway for their family, since they are often resolvers on other networks, such as `1.1.1.1`. Per-interface DNS from the network provider is treated like interface-level DNS. The existing behavior remains available as **Legacy** mode.

## Terminology

| Term | Meaning |
|---|---|
| **Global defaults** | `nameservers` / `searchsuffixes` from the `vmoperator-network-config` ConfigMap in the VM Operator namespace. |
| **VM-level DNS** | `spec.network.nameservers` / `spec.network.searchDomains`. |
| **Interface-level DNS** | `spec.network.interfaces[i].nameservers` / `.searchDomains`. |
| **Provider DNS** | Nameservers / search domains for an interface reported by its network provider, such as a VPC SubnetPort. |
| **Interface DNS** | An interface's interface-level DNS, or else its provider DNS. Nameservers and search domains are resolved independently. |
| **Static interface** | An interface that has at least one static IP address, is not DHCPv4, is not DHCPv6, and whose network does not report NoIPAM. |
| **Interface families** | The IP families of an interface's static IP addresses, plus IPv6 when the interface accepts Router Advertisements. |
| **Gateway families** | IPv4 when the interface has a static IPv4 address with a gateway. IPv6 when it has a static IPv6 address with a gateway, or accepts Router Advertisements, which provide the default route. A gateway set to `None` does not count. |
| **Family filter** | Keeping only the nameservers of the given families, in order. A nameserver that is not an IP address is kept. |
| **Primary interface** | The VM's first interface in `spec.network.interfaces` order, when it is a static interface with at least one gateway family. Otherwise, for example when the first interface is DHCP or NoIPAM, or has no gateway, the VM has none. Later interfaces are never considered. This matches the vSphere GOSC primary adapter: the first adapter, when it has a static IP address and a static gateway. |
| **TKG VM** | A VM carrying Cluster API labels (a VKS node). TKG VMs are always Linux, always use Cloud-Init, and their first interface is the node's primary interface. |
| **Resolved global DNS** | The global DNS (VM-level DNS, followed for LinuxPrep by the rolled-up provider DNS), falling back to the global defaults. Exposed to bootstrap templates as `.Net.Nameservers`. |

## Goals

### Precedence

- **G0 (MUST)** — In Scoped mode, DNS from the VM spec always wins over the global defaults. For each interface, the order is: interface DNS (interface-level, then provider), then VM-level DNS, then the global defaults. Each source is used only when no earlier source provides that value. Nameservers and search domains each fall back independently.
  - Interface DNS is applied to its interface however the interface is addressed, including DHCP and NoIPAM interfaces.
  - VM-level DNS is applied only to static interfaces. VM-level nameservers are filtered to each interface's families.
  - A global default is applied only to the primary interface, and only when the matching VM-level value is empty. The default nameservers are filtered to its gateway families. It is never applied to a DHCP or NoIPAM interface, or to any interface other than the primary one.
- **G0a (MUST)** — In Scoped mode, provider DNS for an interface MUST be treated as that interface's interface-level DNS when the interface spec does not set its own. Where the bootstrap engine supports a value per interface, it is applied to that interface. Where the engine supports a value only globally, the interfaces' DNS MUST be rolled up into the global list: the VM-level values first, followed by each interface's DNS in interface order, without duplicates (G9, G11). Legacy mode MUST ignore provider DNS, so its output does not change when a provider starts to report it. [NEEDS CLARIFICATION: the SubnetPort API fields are not available yet; only the plumbing is done.]

### Mode selection

- **G1 (MUST)** — Scoped mode MUST be gated by a Supervisor capability. [NEEDS CLARIFICATION: capability key name; the placeholder `supports_vm_service_scoped_dns_defaults` is used until WCP assigns one.] When the capability is not activated, every VM MUST use Legacy mode, whether or not it has the mode annotation, and the operator MUST NOT add or change the annotation (G3). Deactivating the capability therefore returns `scoped` VMs to Legacy mode, which re-applies Cloud-Init guestinfo, and re-customizes LinuxPrep and Sysprep VMs without the latch at their next power-on.
- **G2 (MUST)** — Legacy mode MUST produce exactly the same customization data as today, for every bootstrap provider.
- **G3 (MUST)** — The internal annotation `vmoperator.vmware.com/dns-defaults` MUST select the mode for a VM while the capability is activated:
  - `legacy` selects Legacy mode. Any unrecognized non-empty value also selects Legacy mode.
  - `scoped` selects Scoped mode.
  - If the annotation is absent, the operator MUST set it on the VM's next reconcile. It sets `legacy` when the VM's guest may already have been configured, and `scoped` otherwise. A guest may already have been configured if the VM carries any of these annotations: a bootstrap hash annotation, `first-boot-done`, `restored-vm`, `imported-vm`, or `failed-over-vm`. Existing VMs therefore keep the DNS configuration they were deployed with, and only new VMs receive Scoped behavior.
- **G4 (MUST)** — Only privileged users MAY add, modify, or remove `vmoperator.vmware.com/dns-defaults`.

### Scoped mode: Cloud-Init

- **G5 (MUST)** — Interface DNS MUST be applied to its interface. When `useGlobalNameserversAsDefault` is unset or true, the VM-level nameservers MUST be applied to each static interface that has no interface nameservers, filtered to that interface's families. `useGlobalSearchDomainsAsDefault` works the same way for search domains, which are not filtered. DHCP and NoIPAM interfaces MUST NOT receive VM-level DNS; to configure DNS on them, users set interface-level DNS. This differs from Legacy mode, which copies VM-level DNS to every interface.
- **G6 (MUST)** — The global default nameservers MUST be written to the primary interface only, filtered to its gateway families, and only when `spec.network.nameservers` is empty and the primary interface has no interface nameservers. `useGlobalNameserversAsDefault` does not affect them. Interface DNS on other interfaces does not prevent this.
- **G7 (MUST)** — As in Legacy mode, the global default search domains MUST only be applied to TKG VMs with Cloud-Init. They MUST be written to the primary interface only, and only when `spec.network.searchDomains` is empty and the primary interface has no interface search domains. `useGlobalSearchDomainsAsDefault` does not affect them. They are not filtered.
- **G8 (MUST)** — DHCP and NoIPAM interfaces MUST receive only their interface DNS: neither VM-level DNS nor global defaults.

### Scoped mode: LinuxPrep (explicit or implicit)

LinuxPrep supports DNS only globally, and a non-empty GOSC global DNS server list overrides the DNS servers from DHCP on every interface. The webhook rejects interface-level DNS with LinuxPrep, so the only interface DNS is provider DNS.

- **G9 (MUST)** — The GOSC global DNS server list MUST be the VM-level nameservers followed by the interfaces' nameservers, rolled up per G0a. It is not family filtered. When it is empty, it MUST be the global default nameservers, filtered to the primary interface's gateway families, when the VM has a primary interface. Only the first interface is considered: another interface that uses DHCP does not prevent the defaults, and receives them in place of its DHCP-provided DNS servers, as in Legacy mode. The GOSC global suffix list MUST be the VM-level search domains followed by the interfaces' search domains, rolled up per G0a. The global default search domains are not applied, as with explicit LinuxPrep in Legacy mode, and with implicit LinuxPrep before `6c4fbc485`.

### Scoped mode: Sysprep

Windows configures DNS servers per adapter, and does not use the GOSC global DNS server list. Its DNS suffix search list is global. A non-empty per-adapter DNS server list overrides the DNS servers from DHCP on that adapter. The webhook rejects interface-level search domains with Sysprep, so the only interface search domains are provider search domains.

- **G10 (MUST)** — The GOSC global DNS server list MUST be empty. Each adapter's interface nameservers MUST be applied to it, including on an adapter that uses DHCP, where they override DHCP. The VM-level nameservers MUST be applied to each static adapter that has no interface nameservers, filtered to that adapter's families. DHCP and NoIPAM adapters MUST NOT receive the VM-level nameservers, so a DHCP adapter keeps its DHCP-provided DNS servers unless it has interface nameservers.
- **G11 (MUST)** — The primary adapter MUST get the global default nameservers, filtered to its gateway families, when `spec.network.nameservers` is empty and it has no interface nameservers. The GOSC global suffix list MUST be the VM-level search domains followed by the adapters' search domains, rolled up per G0a. The global default search domains are not applied, as in Legacy mode.

### Scoped mode: status and templates

- **G12 (MUST)** — The resolved nameservers provided to vAppConfig and Sysprep templates MUST be the resolved global DNS: the VM-level nameservers (followed for LinuxPrep by the rolled-up interface nameservers), falling back to the global defaults even when every interface has nameservers of its own, so existing templates that index `.Net.Nameservers` keep rendering.
- **G12a (MUST)** — Status MUST report only DNS that was applied:
  - For Cloud-Init, `status.network.config.dns` MUST report no nameservers or search domains, since netplan has no global DNS. For LinuxPrep, it MUST report the GOSC global lists that are applied. For Sysprep, it MUST report no nameservers, since the global DNS server list is not used, and the GOSC global suffix list that is applied.
  - When no bootstrap engine configures the guest network (vAppConfig only, no bootstrap provider, or bootstrap disabled), it MUST report the resolved global DNS, for users who configure the guest by hand.
  - `status.network.config.interfaces[].dns` MUST reflect the DNS applied to each interface, so it changes along with G5–G8, G10 and G11.

  Legacy mode keeps reporting the resolved global DNS, as it does today.
- **G13 (MUST)** — The global-defaults ConfigMap SHOULD be read only when a default or the resolved global DNS is actually needed.

## Non-goals

- Adding new API fields, or changing which DNS fields the webhook accepts for each bootstrap provider.
- Per-adapter search domains (`dnsDomain`) for Sysprep.
- Changing DNS for VMs that use only vAppConfig (the operator does not configure DNS for them; templates do).

## User stories / acceptance criteria

All stories below assume the capability is activated and a new VM, so Scoped mode is selected, unless they say otherwise.

### DevOps user

Cloud-Init:

- **Given** a Cloud-Init VM has two static interfaces and no DNS in its spec, **when** it is deployed, **then** the netplan contains the global default nameservers on the first interface only, filtered to its gateway families, and no default search domains. For a TKG VM, the first interface also carries the default search domains.
- **Given** a Cloud-Init VM's first interface is IPv4 only, **when** it is deployed, **then** it carries only the IPv4 global default nameservers.
- **Given** a Cloud-Init VM's first interface is dual-stack with `gateway6: None`, **when** it is deployed, **then** it carries only the IPv4 global default nameservers.
- **Given** a Cloud-Init VM has a DHCP or NoIPAM interface, or a static interface with `gateway4: None`, followed by a static interface with a gateway, **when** it is deployed, **then** no interface carries the global defaults.
- **Given** a Cloud-Init VM's second interface sets interface-level nameservers, **when** it is deployed, **then** the first interface still carries the global default nameservers.
- **Given** a Cloud-Init VM sets `spec.network.nameservers` with an IPv4 and an IPv6 address, and has an IPv4-only static interface, a dual-stack static interface, and a DHCP interface, **when** it is deployed, **then** the IPv4-only interface carries only the IPv4 nameserver, the dual-stack interface carries both, the DHCP interface carries none, and no interface carries the global default nameservers.
- **Given** a Cloud-Init VM's DHCP interface sets interface-level nameservers, **when** it is deployed, **then** that interface carries them.
- **Given** a Cloud-Init VM sets `useGlobalNameserversAsDefault: false` and its first interface is static with a gateway, **when** it is deployed, **then** the first interface carries the filtered global default nameservers.
- **Given** a Cloud-Init VM has only DHCP or NoIPAM interfaces, **when** it is deployed, **then** no interface carries the global defaults or the VM-level DNS.
- **Given** a Cloud-Init VM's first interface has provider nameservers and the VM sets `spec.network.nameservers`, **when** it is deployed, **then** the first interface carries the provider nameservers, and every other static interface without interface nameservers carries the VM-level nameservers.

LinuxPrep:

- **Given** a LinuxPrep VM's first interface is static with a gateway, a later interface uses DHCP, and the VM has no VM-level or provider nameservers, **when** it is customized, **then** the GOSC global DNS server list contains the global default nameservers of the first interface's gateway families.
- **Given** a LinuxPrep VM's first interface uses DHCP and the VM has no VM-level or provider nameservers, **when** it is customized, **then** the GOSC global DNS server list is empty.
- **Given** a LinuxPrep VM sets `spec.network.nameservers` and its interfaces have provider nameservers, **when** it is customized, **then** the GOSC global DNS server list contains the VM-level nameservers followed by each interface's provider nameservers in interface order, without duplicates.

Sysprep:

- **Given** a Sysprep VM has a static first adapter with a gateway and no DNS in its spec, **when** it is customized, **then** the first adapter carries the global default nameservers of its gateway families, and the global DNS server list and the global suffix list are empty.
- **Given** a Sysprep VM sets `spec.network.nameservers`, **when** it is customized, **then** every static adapter without interface nameservers carries them, filtered to its families, DHCP and NoIPAM adapters do not, and the global DNS server list is empty.
- **Given** a Sysprep VM has a DHCP adapter with interface-level nameservers, **when** it is customized, **then** that adapter carries those nameservers, overriding the DNS servers from DHCP.
- **Given** a Sysprep VM sets `spec.network.searchDomains`, **when** it is customized, **then** the global suffix list contains them.

Status:

- **Given** Legacy mode, **when** a VM is reconciled, **then** `status.network.config.dns` reports the same resolved global DNS as it does today.
- **Given** a LinuxPrep VM's first interface uses DHCP and it has no VM-level nameservers, **when** it is reconciled, **then** `status.network.config.dns` reports no nameservers.
- **Given** a Cloud-Init VM has two static interfaces and no DNS in its spec, **when** it is reconciled, **then** `status.network.config.interfaces[]` reports nameservers for the first interface only.

### CSP admin

- **Given** the capability is activated and a VM needs the prior behavior, **when** an admin sets `vmoperator.vmware.com/dns-defaults: legacy` on it, **then** subsequent bootstraps use Legacy mode.
- **Given** the capability becomes activated on a Supervisor with existing VMs, **when** those VMs are reconciled, **then** each already-bootstrapped VM is annotated `legacy`, and its customization data does not change.
- **Given** a DevOps user, **when** they try to add, change, or remove `vmoperator.vmware.com/dns-defaults`, **then** the request is denied.

## Open questions

- [NEEDS CLARIFICATION: capability key name (owner: WCP capabilities).]
- [NEEDS CLARIFICATION: epic ticket.]
- [NEEDS CLARIFICATION: VM-level DNS is no longer applied to DHCP or NoIPAM interfaces with Cloud-Init (G5). A Cloud-Init VM whose interfaces all use DHCP, and which sets `spec.network.nameservers` or `spec.network.searchDomains`, therefore gets no VM-level DNS, while the webhook still accepts it. Confirm that VKS / CAPV does not set VM-level nameservers or search domains on nodes whose interfaces use DHCP, since new nodes would lose them. Consider an admission warning when VM-level DNS is set and every interface explicitly uses DHCP.]
- [NEEDS CLARIFICATION: on NoIPAM networks, users can still specify static addresses. Cloud-Init configures those addresses, but such an interface is not static, so it receives neither VM-level DNS nor the defaults (Legacy mode applied both). GOSC disables IPv4 on NoIPAM adapters. Decide whether NoIPAM interfaces with user-specified addresses count as static for Cloud-Init.]
- [NEEDS CLARIFICATION: the G3 annotations may not identify every existing guest. The bootstrap-hash annotations date from 2025-09, and `first-boot-done` is set only when VM Operator itself powers the VM on. A VM that is powered on but carries none of them, for example after a jump upgrade from an older build or when powered on outside VM Operator, would be classified as `scoped`, and a running Cloud-Init VM would have its guestinfo metadata rewritten. A candidate additional signal: a VM without the annotation that is already powered on is `legacy`. This is safe for new VMs because the mode is decided before the first power-on. Undecided.]
- Resolved: the global defaults apply only to the first interface, for TKG and non-TKG VMs alike. A user who wants DNS on other interfaces can add it with VM-level or interface-level DNS, whereas a default applied to every static interface could not be removed from an interface that should not have it.
- Resolved: the first interface must have a gateway, and the default nameservers are filtered to its gateway families. The defaults are often resolvers on other networks, such as `1.1.1.1`, which an interface without a gateway cannot reach. This also lets users keep the defaults off the first interface, per family, with `gateway4: None` / `gateway6: None`. An interface whose resolvers are on its own subnet, or reachable through `interfaces[].routes`, needs no gateway but still does not get the defaults; users set VM-level or interface-level DNS instead.
- Resolved: accepting Router Advertisements counts as IPv6 for the interface's families and its gateway families, since it provides both an IPv6 address and the default route.
- Resolved: the global default search domains are never applied by LinuxPrep or Sysprep, as in Legacy mode for explicit LinuxPrep and Sysprep. On Windows, a configured DNS suffix search list replaces appending the primary and connection-specific DNS suffixes (per the Windows DNS client documentation), which could break short-name resolution in a domain-joined VM's AD domain. LinuxPrep has no such reason: it keeps the v1alpha1 rule, whose code comment said GOSC had no means to set the DNS search suffix. Applying the defaults to LinuxPrep would be a separate behavior change.
- Resolved: implicit LinuxPrep VMs (Linux VMs with no bootstrap provider) do not receive the global default search domains in Scoped mode (G9), the same as explicit LinuxPrep. They receive them in Legacy mode only because `6c4fbc485` (2024-03) started computing `isGOSC` from the raw `spec.bootstrap` when it moved the DNS logic out of `DoBootstrap`. Before then, implicit LinuxPrep counted as GOSC and did not receive them, and the v1alpha1 provider never set a GOSC suffix list. Existing VMs keep them, since they are pinned to Legacy mode (G3). New VMs that need search domains add `spec.bootstrap.linuxPrep` and set `spec.network.searchDomains`.
- Resolved: a first interface that is DHCP or NoIPAM, or has no gateway, gets no defaults, and later interfaces are not considered. A DHCP first interface usually gets DNS from DHCP. Otherwise, users set VM-level DNS, except for VMs with no bootstrap provider or with vAppConfig only, which the webhook does not allow to set VM-level DNS. They can add `spec.bootstrap.linuxPrep`, which the webhook allows after creation.
- Resolved: the global defaults are not used when the matching VM-level value is set, even if family filtering leaves an interface without nameservers. The user chose the nameservers; to configure another family on an interface, they set interface-level nameservers.
- Resolved: VM-level DNS is applied only to static interfaces, for Cloud-Init and Sysprep. To configure DNS on a DHCP or NoIPAM interface, users set interface-level DNS, which Cloud-Init and Sysprep (nameservers only) support.
- Resolved: for LinuxPrep, only the first interface determines whether the defaults are applied, even though the global list also overrides DHCP on later DHCP interfaces, as it does in Legacy mode.
- Resolved: provider DNS is treated like interface-level DNS, including on DHCP interfaces. For the global-only lists, it is rolled up after the VM-level values.
- Resolved: `useGlobal*AsDefault: false` does not opt out of the global defaults. The knobs only control whether VM-level DNS is copied to interfaces, and the webhook already rejects VM-level DNS when the knob is false.
- Resolved: deactivating the capability returns every VM to Legacy mode, including annotated VMs (G1). The annotation is kept, so a VM resumes its mode if the capability is reactivated. To return a `scoped` VM to Legacy mode while the capability is activated, an admin sets the annotation to `legacy`.
- Resolved: Sysprep does not apply VM-level nameservers to DHCP adapters, since that would override DHCP. Interface-level nameservers on a DHCP adapter are applied, so users can still override DHCP explicitly.
- Once SubnetPorts report DNS, the bootstrap of existing scoped VMs changes, which re-applies Cloud-Init guestinfo and re-customizes GOSC VMs without the latch. This needs a plan before it lands (T009b).
- During the rollout, multi-NIC VKS clusters will mix DNS layouts: existing nodes stay `legacy` and new nodes are `scoped`, until every node is replaced.
- Follow-up: the VirtualMachineReplicaSet controller copies its template's annotations onto the VMs it creates as a privileged account, so a DevOps user can set `vmoperator.vmware.com/dns-defaults` through a ReplicaSet template, bypassing G4. The same applies to other privileged annotations such as `first-boot-done`, so it is tracked separately.
