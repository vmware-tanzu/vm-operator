# Implementation Plan: Differentiate IPv4/IPv6 vApp/Sysprep template functions

- **Spec**: [`spec.md`](./spec.md)
- **Epic**: vmop-787
- **Date**: 2026-08-06

## Summary

Extend V1alpha6's Go-template function map (`bootstrap_templatedata.go`) with explicit, non-degrading IPv4/IPv6 functions, a family-agnostic usability predicate, and a family-agnostic prefix-length extractor; give the existing generic functions (`FirstIP`, `FirstIPFromNIC`, `IPsFromNIC`) a cascading IPv4→IPv6 fallback so they keep working (rather than erroring) for devices that only have IPv6 addresses. V1alpha1–V1alpha5 are untouched.

## Technical context

- **Go version**: per repo `go.mod` (root module).
- **API version(s) touched**: `v1alpha6` only (`api/v1alpha6`).
- **Modules touched**: root module only — `api/v1alpha6`, `pkg/providers/vsphere/constants`, `pkg/providers/vsphere/vmlifecycle`, `docs/`, `test/e2e/`.
- **New dependencies**: none (uses `net` stdlib only).

## Constitution check

| Rule | Status | Notes |
|------|--------|-------|
| API compatibility | OK | `NetworkDeviceStatus`/`NetworkStatus`/`VirtualMachineTemplate` (api/v1alpha6/virtualmachinetempl_types.go) carry no `+kubebuilder` markers and are not part of the CRD schema — they're a Go-only template-rendering helper. Adding fields is a purely additive Go API change, not a CRD/etcd compatibility concern. |
| Thin controllers | OK | No controller changes. All logic lives in `pkg/providers/vsphere/vmlifecycle` (existing home for this code) and `pkg/providers/vsphere/constants`. |
| Error wrapping | OK | New/changed functions preserve the existing `errors.New(...)` convention already used by sibling functions in this file. |
| Testing standards | OK | Tests added to the existing `bootstrap_templatedata_test.go` (external `_test` package, Ginkgo `DescribeTable`, no new suite file needed). |
| E2E sync with changes | OK | Cluster-observable (Sysprep/vAppConfig rendering) — added a real E2E scenario (`vm_guestcustomization.go`, `Label("experimental")`) exercising `V1alpha6_FirstIPv4`/`FirstIPv4FromNIC`/`FirstIPv6`/`FirstIPv6FromNIC`/`IsUsableIP`/`PrefixLength` against a live cluster. Genuine dual-stack NIC provisioning and multi-NIC coverage remain infra-gated follow-ups; see Test strategy. |
| Ticket/wiki masking | OK | This document and all others under `.sdd/` use `vmop-787`/`vmop-1152`, never the internal `VMSVC-*` keys. |

## Project structure

```
api/v1alpha6/virtualmachinetempl_types.go          — new Gateway6/IPv6Addresses fields
pkg/providers/vsphere/constants/constants.go       — new V1alpha6* function-name constants
pkg/providers/vsphere/vmlifecycle/bootstrap_templatedata.go       — toTemplateNetworkStatusV1A6, v1a6TemplateFunctions
pkg/providers/vsphere/vmlifecycle/bootstrap_templatedata_test.go  — new/backfilled v1alpha6 test coverage
docs/concepts/workloads/guest.md                   — V1alpha6 function table + note
test/e2e/vmservice/vmservice/virtualmachine/vm_guestcustomization.go — new experimental V1alpha6 template-function scenario
```

Genuine dual-stack NIC provisioning and multi-NIC coverage remain infra-gated follow-ups — see Test strategy.

## API / CRD strategy

Additive only; no version bump, no conversion webhook impact. The changed type (`NetworkDeviceStatus`) is not a CRD-serialized type (see Constitution check above), so the "additive changes are unsafe once shipped" rule for CRD types (constitution.md "API compatibility") does not apply here — this is a plain Go struct used only inside `GetTemplateRenderFunc`'s in-memory template data, never round-tripped through the API server.

## Controller / webhook impact

None. `GetTemplateRenderFunc` (`bootstrap_templatedata.go`) is called from `pkg/providers/vsphere/vmlifecycle/bootstrap.go`, itself invoked by the Sysprep/vAppConfig/LinuxPrep bootstrap paths in the same package — no controller, webhook, or RBAC changes. No new feature flag: the new functions are unconditionally registered in V1alpha6's `FuncMap` (see spec's non-goals — not gated behind `WorkloadIPv6`).

## Test strategy

- **Unit** (`testlabels.Controller`, no infra label — pure function tests): `bootstrap_templatedata_test.go` gains dual-stack, IPv6-only, link-local-only, and mixed (usable + link-local) fixtures, plus the missing v1alpha6 `DescribeTable`s for both existing and new functions. A regression fixture confirms IPv4-only cases are byte-for-byte unchanged.
- **Integration**: not applicable — this code path has no vCenter/envtest dependency; existing tests already run without `testlabels.EnvTest`/`testlabels.VCSim`.
- **E2E** (cluster-observable — Sysprep/vAppConfig rendering; investigated per `e2e-sync-with-changes.md`): investigation went through three rounds before landing a working approach:
  1. No dual-stack infra: no IP-family knob on `test/e2e/manifestbuilders`' network/subnet builders, no IPv6 CIDR fixture under `test/e2e/fixtures/yaml/vmoperator/subnet/`, and a CI testbed (`vds_standard_medium`) that is IPv4-only.
  2. Go-template-function rendering had **zero active E2E coverage before this change, for any version** — `vAppStringData.yaml`/`sysprepStringData.yaml` (the fixtures with `{{ V1alpha1_FirstIP }}`-style template strings) were only referenced by `test/e2e/manifestbuilders/secret.go`; no `It`/spec in the whole `test/e2e/` tree ever invoked that code path. Pre-existing gap, unrelated to this change.
  3. The `VAppConfig.RawProperties` mechanism (naming a Secret whose keys become vApp properties) only applies properties that are `userConfigurable` **and already defined by the deployed OVF image** — see `GetMergedvAppConfigSpec`'s doc comment in `pkg/providers/vsphere/vmlifecycle/bootstrap_vappconfig.go`. A *new* property key invented for a test would be silently dropped.

  **Resolution**: `VAppConfig.Properties` (the literal, non-Secret path the existing "Multiple vAppConfigs specified" test already uses) hits the same `userConfigurable`/pre-existing constraint — but that test already proves the `photon-ovf-vapp-properties` OVF image defines several userConfigurable properties across string/bool/int types. Reusing those exact, already-working keys and setting their *values* to V1alpha6 template-function calls sidesteps the property-schema problem entirely — no new OVF image or infra needed. Added a new `When("Property values use V1alpha6 template functions", ...)` sibling to "Multiple vAppConfigs specified" in `vm_guestcustomization.go`, `Label("experimental")` matching that sibling's own convention (excluded from default CI per `test/e2e/README.md`) — one property per function under test, nothing else set:
  - `string-valid` → `{{ V1alpha6_FirstIPv4 }}`, `string-padding-user-configurable` → `{{ V1alpha6_FirstIPv4FromNIC 0 }}` — asserted **strictly** as valid IPv4 CIDRs (`LinuxPrep` in this test already guarantees the VM gets an IPv4 address).
  - `string-trimmed` → `{{ V1alpha6_FirstIPv6 }}`, `string-empty` → `{{ V1alpha6_FirstIPv6FromNIC 0 }}` — asserted **leniently**: either a valid IPv6 CIDR, or the literal unrendered `"{{ ... }}"` text (which is exactly what `renderTemplate` falls back to on error) — accepted either way rather than failing the whole spec over an environment/infra property (does this VM's subnet hand out IPv6) this test doesn't control.
  - `bool-user-configurable-1` → `{{ V1alpha6_IsUsableIP "192.168.1.10" }}`, `int-user-configurable-1` → `{{ V1alpha6_PrefixLength "10.0.0.0/24" }}` — fixed, literal inputs unrelated to the VM's network, asserted **exactly** (`"True"` / `"24"`).
  - Single-NIC only (index `0`); genuine multi-NIC coverage of `*FromNIC`/`*Devices N*` was scoped out — `NetworkA2`/`GetVirtualMachineWithMultiNetworkYamlA2` is a real, working pattern (`vm_vpcnetworking.go`), but it's wired to network resources that topology's namespace setup provisions, unconfirmed to be available in `vm_guestcustomization.go`'s default topology. **Follow-up**: extend to a real second NIC once that's confirmed; also fix `vmoperator.GetVirtualMachineIP` in `test/e2e/vmservice/lib/vmoperator/vmoperator.go`, which only reads `PrimaryIP4` today, never `PrimaryIP6`.

  The new V1alpha6 IPv4-preferred behavior is provably unchanged for the common case by construction (see Design: `v1a6FirstIP`/`v1a6FirstIPFromNIC`/`v1a6IPsFromNIC` return `IPAddresses[0]`/`IPAddresses` unchanged whenever `IPAddresses` is non-empty) and is covered by unit tests.

## Rollout / migration

- No feature flag — additive Go template functions, always registered.
- No schema upgrade / backfill — no CRD field changes.
- Partner comms: `docs/concepts/workloads/guest.md` documents the new functions and the fallback/degrade semantics of the existing generic ones; release note calls out the new functions and the (rare, backward-compatible) behavior change for IPv6-only devices that previously errored on `FirstIP`/`FirstIPFromNIC`/`IPsFromNIC`.

## Complexity tracking

| Violation | Why needed | Simpler alternative rejected because |
|-----------|------------|---------------------------------------|
| None | — | — |
