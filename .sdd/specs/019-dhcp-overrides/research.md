# DHCP Override Research

## Netplan

[DHCP overrides](https://netplan.readthedocs.io/en/stable/netplan-yaml/#dhcp-overrides), consulted 2026-10-15 and verified against the implementation on 2026-10-18:

- Overrides affect only the enabled DHCP family.
- With networkd, both enabled families must have identical override keys and values. VM Operator does not choose NetworkManager, so it must honor networkd's constraints.
- `use-dns` is not emitted. With networkd, static `nameservers` are always written as `DNS=`; without `use-dns: false`, DHCP-provided servers are merged into the same per-link list, and systemd-resolved may use either. Suppressing DHCP DNS is deferred to a follow-up spec with an explicit API field.
- `use-routes: false` discards DHCP routes. Explicit static routes still apply. Supported by networkd and NetworkManager.
- RA configuration is independent and is outside this request.

## Existing implementation

User-provided route lists augment DHCP routes unless the matching gateway is `None`. Effective DHCP modes may be supplied by NetOP, NCP, or VPC, so render-time validation is required in addition to admission checks.

## Metadata verification

The `DHCP overrides` context under `CloudInit Bootstrap` in `pkg/providers/vsphere/vmlifecycle/bootstrap_cloudinit_test.go` verifies 11 scenarios through both GuestInfo and CloudInitPrep. Assertions inspect decoded, serialized metadata rather than compressed byte fixtures. Coverage includes interface nameservers without DHCP DNS overrides, enabled and disabled DHCP families, explicit `false` overrides, static default-route and metric retention, unchanged router-advertisement handling, and provider-derived dual-stack conflicts that retain DHCP routes for both families.

## Scope relative to PR #2005

[PR #2005](https://github.com/vmware-tanzu/vm-operator/pull/2005), reviewed 2026-10-29, moves VM-level DNS defaulting into the bootstrap engine and limits it to static interfaces in scoped mode, preserving legacy defaulting for existing VMs. This feature does not change DNS behavior, so it does not depend on where or how VM-level defaults are applied; #2005 owns documentation of global DNS policy.

## DHCP DNS suppression removed (2026-11-23)

An earlier revision derived `use-dns: false` from non-empty interface nameservers. It was dropped because the motivating issue (a secondary interface's DHCP default route selecting the VM's primary IP) concerns routes only, and inferring DNS suppression changes the behavior of existing, valid specs that rely on static and DHCP DNS being merged. A follow-up spec should add an explicit, opt-in DNS override to the interface API and consider router-advertisement DNS (`ra-overrides`).
