# Feature Specification: Differentiate IPv4/IPv6 vApp/Sysprep template functions

- **Feature branch**: [`ipv6-template-funcs`](../../..)
  - **Fork**: N/A
  - **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-08-06
- **Status**: In Progress
- **Epic**: vmop-787
- **Design docs**: N/A

---

## Goals

- MUST provide V1alpha6 vApp/Sysprep template functions that let an author request an IPv4 address, an IPv6 address, or "whichever family is present" explicitly and predictably, instead of the current behavior where every existing function is silently IPv4-only.
- MUST NOT change the observed output of any existing V1alpha1–V1alpha6 template function for a VM whose network device already has an IPv4 address (backward compatibility for the common case today).
- MUST let a VM whose device has only IPv6 addresses still get a useful answer from the existing, generic function names (`FirstIP`, `FirstIPFromNIC`, `IPsFromNIC`) rather than an error, so existing templates do not need conditional (family-branching) logic to keep working once IPv6-only devices exist.
- MUST provide new, explicitly-named functions that always return one specific family and never fall back, for authors who need deterministic, non-degrading behavior (e.g. a network-function/router VM with an intentionally link-local-only interface).
- MUST let an author explicitly filter out addresses that are not usable off the local link (unspecified, loopback, link-local unicast/multicast) wherever they choose to, without VM Operator silently doing this filtering on their behalf in places they didn't ask for it.
- MUST provide a dual-stack-safe way to obtain a CIDR's prefix length (`/24`, `/64`, ...), since the existing `SubnetMask` function only supports IPv4's dotted-decimal notation and has no IPv6 equivalent.
- MUST cause a clear, immediate error (not silently wrong output) when an existing IPv4-only helper function is given IPv6 input.

## Non-goals

- Does not add any of these functions to V1alpha1 through V1alpha5. Per `docs/concepts/workloads/guest.md`, those versions are documented as legacy, with users already encouraged to prefer V1alpha6. New capability is added only to V1alpha6.
- Does not change how `IPConfigs`/`NetworkInterfaceIPConfig` are produced upstream (`pkg/providers/vsphere/network/bootstrap.go`) — in particular, does not add link-local filtering to the existing, unfiltered IPv4 `IPAddresses` population. That is a separate, pre-existing gap, unrelated to this feature, and is called out here only as a known, documented limitation.
- Does not change `VirtualMachineNetworkStatus`, `PrimaryIP4`/`PrimaryIP6`, or any other CRD-serialized status field. `VirtualMachineTemplate` / `NetworkDeviceStatus` (the type this feature extends) is a Go-only rendering helper with no `+kubebuilder` markers — it is not part of the CRD schema.
- Does not change `FormatNameservers`, `FormatIP`, or `FirstNicMacAddr` — these are already either family-agnostic or out of scope.
- Does not gate the new functions behind the `WorkloadIPv6` capability — they are always registered; a VM without any IPv6 address simply gets an error from the strict `*IPv6*` functions, same as today's `FirstIP` errors when there's no IPv4 address at all.

## User stories / acceptance criteria

### DevOps user (vApp/Sysprep template author)

- **Given** a VM whose first NIC has both an IPv4 and an IPv6 address, **when** the template renders `{{ V1alpha6_FirstIP }}`, **then** the result is the IPv4 address, identical to today's behavior.
- **Given** a VM whose first NIC has both an IPv4 and an IPv6 address, **when** the template renders `{{ V1alpha6_FirstIPv6 }}`, **then** the result is the NIC's first IPv6 address.
- **Given** a VM whose first NIC has only IPv6 addresses (no IPv4 at all), **when** the template renders `{{ V1alpha6_FirstIP }}`, **then** the result is the NIC's first IPv6 address (preferring one that is not link-local/loopback/unspecified if such an address exists) rather than an error.
- **Given** a VM whose interface is intentionally link-local-only (e.g. an unnumbered router link) and has no IPv4 address, **when** the template renders `{{ V1alpha6_FirstIPv6 }}` or `{{ V1alpha6_FirstIPv6FromNIC 0 }}`, **then** the result includes the link-local address exactly as assigned — it is never silently dropped from these explicit, raw functions.
- **Given** the same link-local-only VM, **when** the template renders `{{ V1alpha6_FirstIP }}` (the generic, non-family-specific function), **then** the result still degrades gracefully to the link-local address rather than erroring, since it is the only address the device has.
- **Given** an address string, **when** the template calls `{{ V1alpha6_IsUsableIP $addr }}`, **then** it returns `false` for unspecified, loopback, or link-local addresses (either family) and `true` otherwise, so an author can filter explicitly wherever they choose.
- **Given** an IPv4 or IPv6 CIDR string, **when** the template calls `{{ V1alpha6_PrefixLength $cidr }}`, **then** it returns the numeric prefix length (e.g. `24` or `64`) regardless of family.
- **Given** an IPv6 CIDR or address string, **when** the template calls `{{ V1alpha6_SubnetMask $cidr }}` or `{{ V1alpha6_IP $addr }}` (both IPv4-only by design), **then** the template render fails with a clear error instead of producing garbage or an incorrect result.

## Open questions

- ~~E2E dual-stack coverage — does the E2E harness used in CI support provisioning a VM with a genuine dual-stack (IPv4 + IPv6) NIC under the `WorkloadIPv6` capability today?~~ **Resolved: yes, given a `WorkloadIPv6`-capable testbed (e.g. an NSX dual-stack testbed).** The V1alpha6 vApp-property E2E test in `vm_guestcustomization.go` creates a VM with `spec.network.interfaces[0].ipamModes: [IPv6, IPv4]`, gated on the `supports_workload_ipv6` Supervisor capability, and covers all 14 registered `V1alpha6_*` template functions (up from 6) across three VM-creation "rounds". `V1alpha6_FirstIPv6`/`FirstIPv6FromNIC` remain asserted leniently (valid IPv6 CIDR, or gracefully-unrendered) since a capability-gated test still can't guarantee the underlying network/backend hands out an address in time, but with dual-stack IPAM actually requested this should render in practice on a capable testbed. The `WorkloadIPv6`-incapable/standard CI testbed path is unaffected — the test simply skips there. See `plan.md` Test strategy for the full design and the still-open multi-NIC (`*FromNIC` beyond index 0) follow-up.
