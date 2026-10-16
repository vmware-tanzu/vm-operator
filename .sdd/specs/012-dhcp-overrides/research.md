# DHCP Override Research

## Netplan

[DHCP overrides](https://netplan.readthedocs.io/en/stable/netplan-yaml/#dhcp-overrides), consulted 2026-10-15 and verified against the implementation on 2026-10-18:

- Overrides affect only the enabled DHCP family.
- With networkd, both enabled families must have identical override keys and values. VM Operator does not choose NetworkManager, so it must honor networkd's constraints.
- `use-dns: false` prevents DHCP DNS from taking precedence over explicit nameservers; this setting affects networkd only.
- `use-routes: false` discards DHCP routes. Explicit static routes still apply. Supported by networkd and NetworkManager.
- RA configuration is independent and is outside this request.

## Existing implementation

CloudInit inherits global nameservers only when `UseGlobalNameserversAsDefault` is true or unset. Preserve that decision, deriving DNS override intent before Supervisor ConfigMap DNS backfill. User-provided route lists augment DHCP routes unless the matching gateway is `None`. Effective DHCP modes may be supplied by NetOP, NCP, or VPC, so render-time validation is required in addition to admission checks.

## Metadata verification

The audit found no production-code changes necessary. The unit table in `pkg/providers/vsphere/vmlifecycle/bootstrap_cloudinit_test.go` verifies 11 scenarios through both GuestInfo and CloudInitPrep. Assertions inspect decoded, serialized metadata rather than compressed byte fixtures. Coverage includes DNS precedence and global opt-out, nameservers of either address family, enabled and disabled DHCP families, explicit `false` overrides, static default-route and metric retention, unchanged router-advertisement handling, and provider-derived dual-stack conflicts rejected before a customization payload is returned.
