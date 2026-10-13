# Implementation Plan: Scoped Guest DNS Defaults

- **Spec**: [`spec.md`](./spec.md)
- **Epic**: TBD
- **Date**: 2026-09-26

## Summary

Split the DNS defaulting in `vmlifecycle.GetBootstrapArgs` into two paths: an unchanged legacy path, and a scoped path. The scoped path applies VM-level DNS only to static interfaces, filtering nameservers to each interface's IP families, and applies the Supervisor's default DNS only to the VM's first interface, and only when that interface is static with a gateway and nothing in the VM spec provides its DNS. A Supervisor capability gates the scoped path. A user-editable annotation pins each VM to one mode, so turning on the capability never changes the DNS of an already-bootstrapped VM.

## Technical context

- **API versions touched**: v1alpha6 field documentation only; no schema changes. CRDs regenerated.
- **Modules touched**: root module; `test/e2e` module (E2E only).
- **New dependencies**: none.

## Constitution check

| Rule | Status | Notes |
|------|--------|-------|
| API compatibility | OK | Field documentation only; no schema changes. The annotation is internal (`pkg/constants`). |
| Thin controllers | OK | All logic lives in `pkg/providers/vsphere/{network,vmlifecycle}`. |
| Feature flag before behavior ships | OK | `pkgcfg.Features.ScopedDNSDefaults`, from capability `supports_vm_service_scoped_dns_defaults`. |
| One test file per package | OK | Tests are added to the existing `bootstrap_test.go`, `capabilities_test.go` and `virtualmachine_validator_unit_test.go`. |
| E2E coverage | OK | A new Context in `vm_guestcustomization.go`, gated on the capability. |

## Design

### Mode selection

`useScopedDNSDefaults` (in `vmlifecycle/bootstrap.go`) works as follows:

- If the capability is off, the result is legacy and the VM is not touched, even when it carries the annotation.
- Otherwise, if `vmoperator.vmware.com/dns-defaults` is set, it selects the mode.
- Otherwise, the function sets the annotation:
  - to `legacy` if the VM is already powered on in vSphere or carries any annotation in `existingGuestAnnotationKeys`: `bootstrap-hash-configspec`, `bootstrap-hash-customspec`, `first-boot-done`, `restored-vm`, `imported-vm` or `failed-over-vm`;
  - to `scoped` otherwise.

  The annotation is persisted by the controller's deferred patch, like the bootstrap hash annotations. `first-boot-done` is only set when a VM is first powered on, and that happens after the VM's first bootstrap. A new VM is therefore already annotated `scoped` before it acquires `first-boot-done`.
- The result is scoped only when the annotation's value is `scoped`.

Pinning existing VMs to legacy matters for two reasons:

- For Cloud-Init, `DoBootstrap` re-applies the guestinfo metadata whenever its hash changes, including on powered-on VMs.
- For GOSC without the latch, a changed customization spec hash triggers re-customization on the next power-on.

Without pinning, activating the capability would rewrite the network metadata or customization of every existing VM.

### Interface DNS and the VM-level copy

`network.InterfaceBootstrap` sets an interface's `Nameservers` / `SearchDomains` only from its interface spec. It no longer copies the VM-level DNS to interfaces for Cloud-Init. That copy moves into `vmlifecycle`, so each mode owns all of its fallback logic:

- `applyLegacyDNSDefaults` performs the copy exactly as `InterfaceBootstrap` did: for Cloud-Init, to every interface without its own, when the matching `useGlobal*AsDefault` knob is unset or true. Legacy output is unchanged.
- `applyScopedDNSDefaults` applies VM-level DNS only to static interfaces (see below).

The only consumers of the bootstraps' DNS are reached through `BootstrapArgs.NetBootstraps` after `GetBootstrapArgs`: netplan, GOSC, templates, and `UpdateNetworkStatusConfig`.

### Static interfaces, IP families, and the primary interface

- `network.Bootstrap.IsStatic()` is true when the interface is not NoIPAM, not DHCP4, not DHCP6, and has at least one IPConfig.
- `Bootstrap.AddressFamilies()` reports IPv4 when the interface has a static IPv4 address, and IPv6 when it has a static IPv6 address or `AcceptRA` is set, since Router Advertisements provide an address.
- `Bootstrap.GatewayFamilies()` reports IPv4 when a static IPv4 address has a gateway. It reports IPv6 when a static IPv6 address has a gateway, or when `AcceptRA` is set, since Router Advertisements provide the default route. A gateway set to `None` has already been cleared by `InterfaceBootstrap`.
- `network.PrimaryInterface` returns the first interface when it `IsStatic` and has a gateway family, otherwise nil. Only the first interface is considered, matching the vSphere GOSC primary adapter.
- `network.FilterNameserversByFamily` keeps the nameservers of the given families. VM-level nameservers are filtered to each interface's `AddressFamilies`. The ConfigMap nameservers are filtered to the primary interface's `GatewayFamilies`. Search domains are not family-specific.

Rationale:

- One interface and one rule are easy to reason about and document. A user who wants DNS on other interfaces can add it with VM-level or interface-level DNS, whereas a default applied to every static interface could not be removed from an interface that should not have it.
- The ConfigMap's resolvers are often on another network, for example `1.1.1.1`. An interface without a gateway of their family can't reach them, and `gateway: None` is an explicit signal that the interface isn't the VM's route out for that family. It also lets users keep the defaults off the first interface per family.
- Resolvers of an unusable family take up `resolv.conf` slots and add a timeout to each lookup.

### VM-level DNS

In Scoped mode, VM-level DNS is applied only to static interfaces:

- **Cloud-Init**: copied to each static interface without its own when the matching `useGlobal*AsDefault` knob is unset or true. Nameservers are filtered to the interface's families. DHCP and NoIPAM interfaces don't get it; users set interface-level DNS on them instead.
- **LinuxPrep**: applied to the GOSC global lists, unfiltered.
- **Sysprep**: Windows does not use the GOSC global DNS server list, so the VM-level nameservers are copied to each static adapter without its own, filtered to its families, and the global list is left empty. A per-adapter list overrides DHCP, so DHCP adapters keep their DHCP DNS servers; interface-level nameservers on a DHCP adapter are still applied, which lets users override DHCP explicitly. VM-level search domains go to the global suffix list, since Windows search suffixes are global.

### Network provider nameservers

`network.Bootstrap.ProviderNameservers` holds only resolver addresses. The SubnetPort integration will populate it from `status.dnsResolverIPs` once that field is available. There is no provider search-domain field.

`InterfaceBootstrap` clears provider nameservers when the interface spec sets its own nameservers. Setting interface search domains leaves provider nameservers intact. Scoped defaulting copies the provider nameservers to the interface before the VM-level and Supervisor fallbacks; Legacy mode ignores them.

Cloud-Init and Sysprep retain provider nameservers per interface, including DHCP interfaces, without family filtering. LinuxPrep appends them to the global nameserver list after VM-level nameservers, in interface order and without duplicates, then clears the per-interface DNS that Linux GOSC cannot apply. LinuxPrep and Sysprep global search suffixes come only from `spec.network.searchDomains`. Provider nameservers do not suppress VM-level or VKS Supervisor search domains.

The merged LinuxPrep global nameserver list suppresses Supervisor nameserver defaults when non-empty and supplies template nameservers, falling back to the ConfigMap when empty. Search-domain resolution is independent.

### Global defaults

The global defaults are applied only when the VM has a primary interface, and the default nameservers only of its gateway families:

- **Cloud-Init**: the primary interface gets the filtered default nameservers only when `spec.network.nameservers` is empty and the interface has none of its own. `useGlobalNameserversAsDefault` keeps its documented meaning: it only controls whether the VM-level nameservers are copied to interfaces. For TKG VMs only, the default search domains follow the same rule: only when `spec.network.searchDomains` is empty and the interface has none of its own, regardless of `useGlobalSearchDomainsAsDefault`. Interface DNS on other interfaces does not prevent the default.
- **LinuxPrep**: the global list gets the filtered default nameservers only when the merged global value (VM-level plus interfaces', see above) is empty. Only the first interface is considered: a later DHCP interface does not prevent the defaults, and GOSC uses them in place of its DHCP DNS servers, as in Legacy mode. The default search domains are not applied.
- **Sysprep**: like Cloud-Init, the primary adapter gets the filtered default nameservers only when `spec.network.nameservers` is empty and the adapter has none of its own. The default search domains are not applied, as in Legacy mode; on Windows, a suffix search list replaces appending the primary and connection-specific DNS suffixes, which could break a domain-joined VM.

As in Legacy mode, the default search domains apply only to TKG VMs, which use Cloud-Init, and never through GOSC. Unlike Legacy mode, they go only on the primary interface rather than every non-DHCP interface.

### Applied vs. resolved DNS

`BootstrapArgs.DNSServers`/`SearchSuffixes` previously did three jobs: GOSC global settings, template data, and status. Template data is now split out:

- **`TemplateDNSServers`** holds the resolved nameservers used by templates. They are the global nameservers (VM-level, merged with the interfaces' for LinuxPrep), falling back to the ConfigMap. Unlike Legacy mode, the ConfigMap is read even when every interface has nameservers, so templates are not left without any.
- **`DNSServers`/`SearchSuffixes`** hold the global DNS configuration that is applied to the GOSC global IP settings and reported in status. In scoped mode:
  - for Cloud-Init, nothing: netplan has no global DNS, and Cloud-Init does not read these fields, so all DNS is reported per interface;
  - for LinuxPrep, the GOSC global lists, as described above;
  - for Sysprep, no nameservers, since Windows does not use the global DNS server list, and the global suffix list. Nameservers applied to an adapter are reported on that interface;
  - when no engine configures the guest network, the resolved values.

The engines are mutually exclusive: the webhook rejects CloudInit with any other provider, and LinuxPrep with Sysprep.

In legacy mode, both sets equal the resolved values, so legacy output is byte-for-byte unchanged. `BootstrapSysPrep` still sets the global DNS server list from `DNSServers`, which keeps the legacy customization spec, and so its hash, unchanged.

### Implicit LinuxPrep

A Linux VM without a bootstrap spec is bootstrapped with LinuxPrep. `getEffectiveBootstrapSpec` factors that decision out of `DoBootstrap` so that the scoped path also sees such a VM as GOSC. The legacy path keeps computing `isGOSC` from the raw spec, as it has since `6c4fbc485` (2024-03) moved the DNS logic out of `DoBootstrap`, where `isGOSC` had been computed after the LinuxPrep default. As a result, legacy still puts the default search suffixes into GOSC for these VMs, which is a known quirk kept for compatibility. Scoped mode restores the behavior from before `6c4fbc485`. The webhook does not allow these VMs to set VM-level DNS, so the global defaults are their only source.

### ConfigMap reads

The scoped path reads the ConfigMap only when a global default is applied, or when templates or status need the resolved DNS under the same conditions as the legacy path.

### Status

In scoped mode, `status.network.config.dns` reports the global DNS described above, which for Cloud-Init has no nameservers or search domains. The per-interface `status.network.config.interfaces[].dns` is built from `NetBootstraps`, so it shows the defaults applied to the primary interface and the VM-level nameservers copied to each static interface.

## Controller / webhook impact

- The VM validation webhook allows users to add, change, or remove `vmoperator.vmware.com/dns-defaults` without privileged-account status. Removing or clearing it causes normal mode selection to run again on the next reconcile, using the vSphere power state and existing-guest markers.
- There are no RBAC or watch changes.

## Test strategy

- Unit tests (`network/bootstrap_test.go`): `InterfaceBootstrap` no longer copies VM-level DNS; `PrimaryInterface`, `AddressFamilies`, `GatewayFamilies`, `FilterNameserversByFamily`.
- Unit tests (`vmlifecycle/bootstrap_test.go`, `GetBootstrapArgs`):
  - legacy with the capability off, including the unfiltered VM-level copy to every interface and the knobs;
  - pinning for each existing-guest annotation and a running VM without markers, including cleared annotations, explicit overrides, and a disabled capability;
  - `legacy` and unknown annotation values;
  - scoped Cloud-Init: multiple static interfaces, static then DHCP, a first interface that is DHCP, NoIPAM, NoIPAM with addresses, or static without a gateway (no defaults anywhere, ConfigMap not read), IPv6-only, dual-stack, dual-stack with `gateway6: None`, IPv4 with Router Advertisements, DHCP-only VM does not read the ConfigMap, interface-level nameservers on another interface (default still applied) and on the first interface, VM-level DNS on static interfaces only, interface-level nameservers on a DHCP interface, VM-level nameservers filtered per interface family, VM-level nameservers of another family (neither they nor the defaults applied), each knob false, VM-level search domains, TKG (defaults on the first interface only; not when it is DHCP or the VM sets search domains), non-TKG (no default search domains), status versus template values;
  - scoped LinuxPrep, explicit and implicit: DHCP first, static then DHCP (defaults applied), first interface without a gateway, VM-level DNS;
  - scoped vAppConfig: status reports the resolved values;
  - scoped Sysprep: static first, DHCP first, VM-level nameservers on every static adapter, with an adapter's own nameservers kept, interface-level nameservers on a DHCP adapter applied, NoIPAM and other-family adapters (not applied), all-DHCP adapters (not applied or reported), VM-level search domains, interface-level nameservers on another adapter and on the first adapter;
  - provider nameservers: Cloud-Init (instead of default/VM nameservers, while VM-level and VKS default search domains remain independent), LinuxPrep (merged in interface order without duplicates, including DHCP; only VM-level search suffixes), Sysprep (per adapter, including DHCP; only VM-level search suffixes), vAppConfig templates.
- Capability tests extended for the new key.
- Webhook annotation tests verify non-privileged create with either mode, addition, changes in both directions, and removal. Other admin-only annotations remain covered by their rejection tests.

### E2E coverage

All E2E cases are capability-gated and `experimental` until validated on a real Supervisor. The non-admin mode-edit/removal test retains automatic legacy reselection.

The cases in `vm_scopeddns.go` assert on what VM Operator renders: the Cloud-Init metadata in guestinfo, the VM's network status, the `dns-defaults` annotation, and the bootstrap hash. They make no guest-side assertions and need no fixture.

The Sysprep case creates its own Windows VM inside the spec instead of using the VMs the suite precreates, and omits `consts.WindowsSysprepLabel` so the suite does not precreate more. It waits for the Windows image and for the vSphere VM to exist, then asserts on VM status, which reflects the applied configuration without waiting for the guest to boot. It carries the `windows` label so it can be filtered out, and skips when no Windows image is configured.

Most cases run on every network provider. Their static interfaces use the namespace's default network, with two interfaces on it for multi-NIC cases, so IPAM supplies static addresses and a gateway. On VPC each static interface gets its own CIDR Subnet instead, created with the existing `createSubnetOrSubnetSet` helper. Because address families vary by provider and testbed, the cases read each interface's addresses and gateways from VM status and compute the expected family filtering from them. A case skips when the network doesn't provide the layout it needs, such as a static first interface with a gateway. Cases that depend on Supervisor defaults read `vmoperator-network-config` and skip when it has no IPv4 nameserver. VM-level and interface-level DNS use fixed documentation addresses.

| Scenario | Providers | Setup and essential assertions |
|---|---|---|
| Legacy and scoped VMs coexist | All | Cloud-Init VMs with two static interfaces: one annotated `legacy`, one unannotated. The legacy VM gets the unfiltered defaults on both interfaces. The unannotated VM selects `scoped`, and gets defaults filtered to its first interface's gateway families, plus the Supervisor search suffixes, on that interface only. After a metadata-neutral reconcile, each VM keeps its mode, bootstrap hash, and metadata. |
| VM-level DNS on static interfaces | All | Two static interfaces with mixed-family VM-level nameservers and search domains. Each interface gets the nameservers filtered to its address families and the search domains. No Supervisor defaults appear, even if filtering empties an interface's list. |
| Interface-level DNS precedence | All | Interface-level nameservers and search domains on the first of two static interfaces. It keeps them unfiltered. The second interface falls back to the filtered VM-level nameservers and the VM-level search domains. |
| VM-level DNS with a DHCP interface | VPC | One static and one DHCP interface with VM-level DNS. Only the static interface gets DNS, and the DHCP interface gets none from VM Operator. |
| LinuxPrep global DNS | All, except as noted | Implicit LinuxPrep with a static first interface has the gateway-family-filtered defaults in the global status. Explicit LinuxPrep with VM-level nameservers has exactly those, unfiltered, and no defaults. Neither has search domains. Explicit LinuxPrep with a DHCP first interface has an empty global list (VPC only). |
| Sysprep per-adapter DNS | All, if a Windows image is configured | A Windows VM with two static adapters, VM-level nameservers and search domains, and explicit nameservers on the second adapter. The first adapter gets the VM-level nameservers filtered by family. The second keeps its own, unfiltered. The global nameserver list is empty, the only search domains are the global VM-level ones, and no adapter has search domains. |

Out of scope for E2E:

- **Sysprep defaults and guest state.** The Windows case does not cover Supervisor defaults on the first adapter, which are unit coverage, and it does not wait for the guest to boot or finish customization.
- **DHCP and NoIPAM on VDS and NSX-T.** Only VPC lets the test control a Subnet's DHCP behavior.
- **Guest resolver state.** Lease contents and resolver files depend on the image and the DHCP server, which these tests do not control. Guest verification belongs to upgrade or manual testing.
- **Existing-guest selection and the full precedence, family, RA, NoIPAM, and marker combinations.** These are unit coverage. Capability activation against existing VMs can be exercised in upgrade testing.

When SubnetPort ingestion lands, add a provider-path E2E proving that real `status.dnsResolverIPs` values reach a scoped Cloud-Init VM's metadata while VM-level search domains remain intact.

## Rollout / migration

- The capability is off by default, and behavior is unchanged until WCP activates it.
- When it is activated, the first reconcile of every VM patches the `dns-defaults` annotation onto it; this initializes each VM's mode. Existing VMs are pinned to `legacy`. If a user removes or clears the annotation later, the next bootstrap reconcile selects and writes the mode again.
- Once enabled, the capability remains enabled. Capability deactivation is not a supported rollout operation. VMs created before activation use legacy defaulting; annotation-based pinning governs VMs after activation.
- To restore legacy behavior on a new VM, a user sets the annotation to `legacy`.
- Release note: describe the scoped behavior per bootstrap provider. Call out that only the first interface gets the Supervisor's default DNS, and only when it is static with a gateway; that Cloud-Init no longer copies VM-level DNS to DHCP or NoIPAM interfaces; and that Sysprep applies `spec.network.nameservers` per adapter.

- SubnetPort ingestion must land with a migration plan for already-scoped VMs and automated per-node VKS prerequisite checks of required DNS resolution through the selected VPC resolver (WIKI page 2485688854, comment 3126531699). Neither ingestion nor these checks is implemented by this branch.
