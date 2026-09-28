# Research: Scoped Guest DNS Defaults

## Where the defaults came from (before this change)

- `pkg/providers/vsphere/config/config.go` `GetDNSInformationFromConfigMap` reads `nameservers` and `searchsuffixes` from the `vmoperator-network-config` ConfigMap. It errors when the value is still the `<worker_dns>` placeholder.
- `pkg/providers/vsphere/network/bootstrap.go` `InterfaceBootstrap` sets per-interface DNS. The source is the interface spec or, for Cloud-Init with `useGlobal*AsDefault` unset or true, `spec.network.*`, copied to every interface, including DHCP and NoIPAM interfaces. This change moves that copy into `vmlifecycle`, unchanged for Legacy mode.
- `pkg/providers/vsphere/vmlifecycle/bootstrap.go` `GetBootstrapArgs` pulls in the ConfigMap when any non-DHCP interface (NoIPAM included) lacks DNS. It then does three things:
  - sets the "global" `DNSServers`, which feeds the GOSC global settings, templates, and status;
  - for Cloud-Init, copies the ConfigMap nameservers onto **every** non-DHCP interface;
  - for TKG VMs, also copies the search domains onto those interfaces.

## GOSC semantics (govmomi `vim25/types`)

- `CustomizationGlobalIPSettings.DnsServerList`: "If this list is empty, then the guest operating system is expected to use a DHCP server to get its DNS server settings." A non-empty list therefore replaces the DNS servers from DHCP. Legacy mode applied the defaults even when an interface used DHCP; Scoped mode only considers the first interface.
- `CustomizationGlobalIPSettings.DnsSuffixList`: global for Linux; listed per adapter on Windows.
- `CustomizationIPSettings.DnsServerList`: "A list of server IP addresses to use for DNS lookup in a Windows guest operating system … the Linux guest customization process ignores this setting and looks for its DNS servers in the globalIPSettings object … If this list is not empty, and if a DHCP IpGenerator is used, then these settings override the DHCP settings." Sysprep defaults therefore go to one static adapter only.
- Windows does not use the global `DnsServerList`; only the global `DnsSuffixList` is global on Windows. Scoped mode therefore applies VM-level nameservers per adapter for Sysprep.
- `CustomizationIPSettings.Primary` (vim 9.1): "If no adapter is explicitly marked as primary, the first adapter will be defaulted to be primary only when it's configured with a static IP address and a static gateway, otherwise no adapter will be treated as the primary." The spec's primary interface uses the same rule: the first interface, only when it has a static IP address and a gateway.

## Bootstrap re-application

- Cloud-Init has no customize latch. `DoBootstrap` reconfigures the VM whenever the hash of the guestinfo configSpec changes, including when the VM is powered on. Changing the defaulting logic would therefore rewrite the metadata of existing VMs. This motivates pinning them to `legacy` (see `plan.md`).
- LinuxPrep and Sysprep customize only when the VM goes from off to on. Without the latch, they re-customize when the customization spec hash changes.

## Templates

`docs/concepts/workloads/guest.md` shows vAppConfig templates using `index .V1alpha6.Net.Nameservers 0`. A template that indexes an empty list fails to render, so the resolved global DNS provided to templates is kept unchanged.

## Pre-existing quirks retained in legacy mode

- `isGOSC` is computed from the raw spec. An implicit-LinuxPrep VM (a Linux VM with no bootstrap spec) is treated as non-GOSC, so it receives the ConfigMap search suffixes in its GOSC global settings. This dates from `6c4fbc485` (2024-03), which moved the DNS logic out of `DoBootstrap` into `GetBootstrapArgs`. Before it, `isGOSC` was computed after a nil bootstrap was defaulted to LinuxPrep (`bb4095fd0`), so these VMs did not receive them. The v1alpha1 provider never set a GOSC suffix list; its comment said GOSC had no means to set the DNS search suffix.
- NoIPAM interfaces count as non-DHCP, so they receive the Cloud-Init defaults.

## Signals that a guest was already configured

- Only `DoBootstrap` sets the bootstrap hash annotations. A VM that reaches the Supervisor through register/restore, import or failover may not carry them.
- `first-boot-done` is set in `vmprovider_vm.go` once the first power-on succeeds. That happens after the VM's first bootstrap, so a new VM already carries `dns-defaults` by the time this annotation appears.
- The pin therefore treats any of these as an existing guest: the two hash annotations, `first-boot-done`, `restored-vm`, `imported-vm` or `failed-over-vm`.

## Primary interface and IP families

- A gateway set to `None` (`gatewayIgnored`) clears `IPConfigs[i].Gateway` in `InterfaceBootstrap`. So does a network provider that reports no gateway. Both are treated as "no gateway".
- `AcceptRA` is set only when IPv6 is dynamic: VPC SLAAC, or mirrored from DHCP6 for NetOP/NCP. It implies a default IPv6 route even without a static IPv6 gateway.
- `resolv.conf` honors at most three nameservers (see the API field docs). Unreachable-family entries both waste those slots and add a per-lookup timeout.

## Webhook constraints on DNS fields

`webhooks/virtualmachine/validation/virtualmachine_validator.go` limits which DNS fields each bootstrap provider may set:

| Field | Cloud-Init | LinuxPrep | Sysprep | vAppConfig only / no bootstrap |
|---|---|---|---|---|
| `spec.network.nameservers` | only when `useGlobalNameserversAsDefault` is unset or true | yes | yes | rejected |
| `spec.network.searchDomains` | only when `useGlobalSearchDomainsAsDefault` is unset or true | yes | yes | rejected |
| `interfaces[].nameservers` | yes | rejected | yes | rejected |
| `interfaces[].searchDomains` | yes | rejected | rejected | rejected |

Consequences:

- With Cloud-Init, `useGlobal*AsDefault: false` can never be combined with VM-level DNS. In Legacy mode the knob therefore had no effect on the guest, and the global defaults were still applied.
- Linux VMs with no bootstrap provider (implicit LinuxPrep) and vAppConfig-only VMs cannot set any DNS, so the global defaults are their only source. Adding `spec.bootstrap.linuxPrep` after creation is allowed.
- Network providers never supply nameservers; interface DNS comes only from the spec.

## Netplan and DHCP

VM Operator does not set `dhcp4-overrides`/`dhcp6-overrides`. Nameservers on a DHCP interface are therefore added to the DNS servers from DHCP rather than replacing them, so copying VM-level nameservers to DHCP interfaces (as Cloud-Init always has) does not break DHCP DNS. Scoped mode nevertheless stops copying them to DHCP and NoIPAM interfaces, so DNS on such an interface is an explicit, per-interface choice. Support for the overrides is planned separately.
