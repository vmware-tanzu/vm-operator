# Implementation Plan: CloudInit DHCP Overrides

- **Spec**: [spec.md](spec.md)
- **Epic**: TBD
- **Date**: 2026-10-15

## Summary

Derive DHCP overrides from existing network fields and carry their intent through network bootstraps to CloudInit. Limit the implementation to CloudInit/Netplan; preserve LinuxPrep and Sysprep behavior.

## Technical context

- Go 1.26.9, existing govmomi and Netplan schema dependencies.
- Active API: v1alpha6; root and test/e2e modules.
- No new dependencies.

## Constitution check

- Business logic remains in the vSphere network provider; controllers and reconcile order are unaffected.
- Existing API fields are reused; no field additions, conversion changes, new RBAC, or CRD schema changes.
- The feature owner explicitly requested no FSS or capability. This instruction overrides the repository default for in-progress features.
- Tests use existing external test packages and E2E helpers. New E2E scenarios remain experimental until run on a Supervisor.
- Spec remains Draft until real epic/story tickets are supplied; ticket identifiers must not be invented.

## Project structure

- `pkg/providers/vsphere/network/`: bootstrap override intent, Netplan output and runtime validation.
- `webhooks/virtualmachine/validation/`: permit gateway `None` with CloudInit DHCP and validate known dual-stack conflicts while retaining existing validation for other providers.
- `test/e2e/vmservice/vmservice/virtualmachine/`: admission and generated CloudInit network metadata coverage.
- `docs/tutorials/deploy-vm/cloudinit.md`: configuration and limitations.

## API / CRD strategy

No new API fields. DNS suppression is derived only from explicit interface nameservers. VM-level nameserver defaulting remains outside this feature; PR [#2005](https://github.com/vmware-tanzu/vm-operator/pull/2005) owns that policy. Leave the VM-level Nameservers field documentation unchanged. Concrete gateways remain mutually exclusive with same-family DHCP. Update v1alpha6 API field descriptions and regenerate CRD manifests to document the changed DHCP semantics.

## Controller / webhook impact

Only the provider's bootstrap construction and admission validation change. Admission validates explicit DHCP modes; provider bootstrap validates final effective modes since admission cannot assume a subnet's allocation mode. Other bootstrap providers retain their existing gateway/DHCP admission restrictions and customization behavior.

## Test strategy

- Unit: DNS precedence and existing behavior without overrides; IPv4-only, IPv6-only, static, and dual-stack overrides; static route retention; invalid effective modes; webhook allow/deny cases and regression coverage for unchanged non-CloudInit admission behavior.
- Integration: focused bootstrap/customization tests using existing package suites.
- E2E: experimental generated CloudInit metadata inspection on private DHCP VPC Subnets and SubnetSets, using a VM with explicit nameservers on each interface, one Subnet-backed and one SubnetSet-backed. VM DHCP fields are omitted so allocation modes come from the network provider. Running requires a WCP testbed.

## Rollout / migration

Enabled without a feature flag or capability by explicit user instruction. No stored-object migration. Before promotion, assign tickets and validate E2E on hardware.
