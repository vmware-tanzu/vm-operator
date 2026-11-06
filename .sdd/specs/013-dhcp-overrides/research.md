# DHCP Override Research

## Netplan

[DHCP overrides](https://netplan.readthedocs.io/en/stable/netplan-yaml/#dhcp-overrides), consulted 2026-10-15 and verified against the implementation on 2026-10-18:

- Overrides affect only the enabled DHCP family.
- With networkd, both enabled families must have identical override keys and values. VM Operator does not choose NetworkManager, so it must honor networkd's constraints.
- `use-dns: false` prevents DHCP DNS from taking precedence over explicit nameservers; this setting affects networkd only.
- `use-routes: false` discards DHCP routes. Explicit static routes still apply. Supported by networkd and NetworkManager.
- RA configuration is independent and is outside this request.

## Existing implementation

Derive DNS override intent only from `spec.network.interfaces[].nameservers`, independently of VM-level or Supervisor DNS defaulting. User-provided route lists augment DHCP routes unless the matching gateway is `None`. Effective DHCP modes may be supplied by NetOP, NCP, or VPC, so render-time validation is required in addition to admission checks.

## Metadata verification

The unit table in `pkg/providers/vsphere/vmlifecycle/bootstrap_cloudinit_test.go` verifies 12 scenarios through both GuestInfo and CloudInitPrep. Assertions inspect decoded, serialized metadata rather than compressed byte fixtures. Coverage includes DNS precedence and global opt-out, nameservers of either address family, enabled and disabled DHCP families, explicit `false` overrides, static default-route and metric retention, unchanged router-advertisement handling, and provider-derived dual-stack conflicts that retain DHCP routes for both families.

## Scope relative to PR #2005

[PR #2005](https://github.com/vmware-tanzu/vm-operator/pull/2005), reviewed 2026-10-29, moves VM-level DNS defaulting into the bootstrap engine and limits it to static interfaces in scoped mode, preserving legacy defaulting for existing VMs. This feature suppresses DHCP DNS only for explicit interface nameservers, so it does not depend on where or how VM-level defaults are applied. Keep global-only nameserver cases as regression coverage that DHCP DNS is retained. The tutorial and API additions describe only the per-interface override; #2005 owns documentation of global DNS policy.
