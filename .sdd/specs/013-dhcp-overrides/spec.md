# Feature Specification: CloudInit DHCP Overrides

- **Feature branch**: Current working branch
- **Fork**: vmware-tanzu/vm-operator
- **PR target**: vmware-tanzu/vm-operator
- **Created**: 2026-10-15
- **Status**: Draft
- **Epic**: TBD (required before merge)

## Goals

- With CloudInit, explicit interface nameservers MUST take precedence over DHCP DNS.
- CloudInit MUST ignore DHCP routes for an address family when its interface gateway is `None`, while retaining explicitly configured static routes.
- CloudInit MUST emit overrides only for enabled DHCP families and MUST honor Netplan's requirement for identical overrides when both DHCP families are enabled.
- Validation MUST accept supported overrides and reject requests known to be incompatible with Netplan. Existing validation for other bootstrap providers MUST remain unchanged.
- Support MUST be enabled without a feature flag or capability, as explicitly requested by the feature owner.

## Non-goals

- Changes to VM-level nameserver defaulting or using inherited nameservers to suppress DHCP DNS.
- New VM API fields, DHCP search-domain/NTP/MTU overrides, or router-advertisement overrides.
- Changes to LinuxPrep or Sysprep customization, nameserver precedence, or DHCP behavior.
- Treating a static route list alone as a request to discard DHCP routes.

## User stories / acceptance criteria

### VM author

- Given CloudInit and explicit interface nameservers, the guest MUST receive those nameservers and `use-dns: false` for each enabled DHCP family.
- Given a CloudInit DHCP interface with gateway `None`, that family's DHCP overrides MUST contain `use-routes: false` and explicit static routes MUST remain present.
- Given explicitly enabled dual-stack DHCP and different route suppression between families, admission MUST reject the VM. If the conflicting DHCP modes are learned from the network provider, customization MUST retain DHCP routes for both families rather than emit invalid Netplan or fail the reconcile.
- Given a provider other than CloudInit, existing admission and customization behavior MUST remain unchanged.

## Open questions

- Epic and implementation story tickets must be assigned before merge. No ticket identifier was supplied with the request.
