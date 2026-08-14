# Implementation Plan: Differentiate IPv4/IPv6 vApp/Sysprep template functions

- **Spec**: [`spec.md`](./spec.md)
- **Epic**: vmop-787
- **Date**: 2026-08-06

## Summary

Extend V1alpha6's Go-template function map (`bootstrap_templatedata.go`) with explicit, non-degrading IPv4/IPv6 functions, a family-agnostic usability predicate, and a family-agnostic prefix-length extractor; give the existing generic functions (`FirstIP`, `FirstIPFromNIC`, `IPsFromNIC`) a cascading IPv4→IPv6 fallback so they keep working (rather than erroring) for devices that only have IPv6 addresses. V1alpha1–V1alpha5 are untouched.

## Technical context

- **Go version**: per repo `go.mod` (root module).
- **API version(s) touched**: new functions/fields only in `v1alpha6` (`api/v1alpha6`). `api/v1alpha1`–`api/v1alpha5` are touched only for the conversion-gen fix below (no behavior change).
- **Modules touched**: root module only — `api/v1alpha1`–`api/v1alpha6`, `pkg/providers/vsphere/constants`, `pkg/providers/vsphere/vmlifecycle`, `docs/`, `test/e2e/`.
- **New dependencies**: none (uses `net` stdlib only).

## Constitution check

| Rule | Status | Notes |
|------|--------|-------|
| API compatibility | OK | `NetworkDeviceStatus`/`NetworkStatus`/`VirtualMachineTemplate` (api/v1alpha6/virtualmachinetempl_types.go) carry no `+kubebuilder` markers and are not part of the CRD schema — they're a Go-only template-rendering helper. Adding fields is a purely additive Go API change, not a CRD/etcd compatibility concern. Adding those fields did break `make generate-go-conversions`/`make manager` (conversion-gen could no longer auto-generate a lossy hub→spoke conversion for the changed type) — fixed by opting all three types out of conversion-gen entirely (`+k8s:conversion-gen=false`) in all six version packages; see API / CRD strategy below. |
| Thin controllers | OK | No controller changes. All logic lives in `pkg/providers/vsphere/vmlifecycle` (existing home for this code) and `pkg/providers/vsphere/constants`. |
| Error wrapping | OK | New/changed functions preserve the existing `errors.New(...)` convention already used by sibling functions in this file. |
| Testing standards | OK | Tests added to the existing `bootstrap_templatedata_test.go` (external `_test` package, Ginkgo `DescribeTable`, no new suite file needed). |
| E2E sync with changes | OK | Cluster-observable (Sysprep/vAppConfig rendering) — added a real E2E scenario (`vm_guestcustomization.go`, `Label("experimental")`) exercising `V1alpha6_FirstIPv4`/`FirstIPv4FromNIC`/`FirstIPv6`/`FirstIPv6FromNIC`/`IsUsableIP`/`PrefixLength` against a live cluster. Genuine dual-stack NIC provisioning and multi-NIC coverage remain infra-gated follow-ups; see Test strategy. |
| Ticket/wiki masking | OK | This document and all others under `.sdd/` use `vmop-787`/`vmop-1152`, never the internal `VMSVC-*` keys. |

## Project structure

```
api/v1alpha6/virtualmachinetempl_types.go          — new Gateway6/IPv6Addresses fields
api/v1alpha1..v1alpha6/virtualmachinetempl_types.go — +k8s:conversion-gen=false on NetworkDeviceStatus/NetworkStatus/VirtualMachineTemplate
api/v1alpha1..v1alpha5/zz_generated.conversion.go  — regenerated (make generate-go-conversions)
pkg/providers/vsphere/constants/constants.go       — new V1alpha6* function-name constants
pkg/providers/vsphere/vmlifecycle/bootstrap_templatedata.go       — GetTemplateRenderFunc wiring for all versions; v1alpha1-v1alpha5 template functions stay here (frozen legacy)
pkg/providers/vsphere/vmlifecycle/bootstrap_templatedata_v1alpha6.go — new: toTemplateNetworkStatusV1A6, v1a6TemplateFunctions, and all other V1alpha6-specific template-function code, split out of bootstrap_templatedata.go (see "Scaffolding tooling" below for why)
pkg/providers/vsphere/vmlifecycle/bootstrap_templatedata_test.go  — new/backfilled v1alpha6 test coverage
docs/concepts/workloads/guest.md                   — V1alpha6 function table + note
test/e2e/vmservice/vmservice/virtualmachine/vm_guestcustomization.go — new experimental V1alpha6 template-function scenario
hack/new-schema-version.py                          — updated for the new per-version-file model (see "Scaffolding tooling" below)
```

Genuine dual-stack NIC provisioning and multi-NIC coverage remain infra-gated follow-ups — see Test strategy.

## Scaffolding tooling (hack/new-schema-version.py)

Adding the six new strict `*IPv4*`/`*IPv6*` functions plus `IsUsableIP`/`PrefixLength` pushed `v1a6TemplateFunctions`'s cyclomatic complexity over the `gocyclo` lint threshold, so those functions were refactored from inline closures (the style every prior version still uses) into standalone package-level functions. That refactor broke `hack/new-schema-version.py`'s scaffolding for the *next* version in two independent ways, discovered via a review comment ("Have AI update hack/new-schema-version.py too") and confirmed empirically by actually running the script end-to-end against an isolated copy of this repo (`python3 hack/new-schema-version.py v1alpha7 --root <copy> --verbose`), not by inspection alone: (1) `_make_v1aN_template_functions_block`, a hardcoded Python string generator, still emitted the old inline-closure style and was missing every new function entirely; (2) the anchor-based text insertion the script used to splice a new version's functions into the shared `bootstrap_templatedata.go` matched against `v1a6TemplateFunctions`'s old FuncMap-ending shape, which no longer existed after the refactor, so the insertion silently landed in the wrong place or not at all.

The fix, proposed and scoped by the PR author: starting with V1alpha6, every hub version's template-function code (`toTemplateNetworkStatusV1A{N}`, `v1a{N}TemplateFunctions`, and everything they depend on) lives in its own dedicated `bootstrap_templatedata_v1alpha{N}.go` file, instead of sharing the single, ever-growing `bootstrap_templatedata.go` that V1alpha1-V1alpha5 still use (those five stay bundled in the shared file forever — they are frozen legacy and never gain new functions, so there's nothing to make scaffolding-safe there). This turns "scaffold the next version's template functions" into a whole-file copy plus a mechanical token rename (`v1a6`→`v1a7`, `V1A6`→`V1A7`, `V1alpha6`→`V1alpha7`) rather than fragile anchor-based surgery on a file whose internal structure keeps changing — because each dedicated file belongs to exactly one version, nothing else in it can collide with the blanket rename, no matter how that version's own function set grows later.

`hack/new-schema-version.py` was updated accordingly: `step_update_template_constants` now extracts the previous hub's whole constant block from `constants.go` by anchor-to-anchor slicing and renames it, rather than emitting a hardcoded, now-stale 8-constant template; `step_update_bootstrap_templatedata`'s hub-add/demote logic (previously steps 7-10, hardcoded insertion) now copies the previous hub's dedicated file whole and renames tokens to produce the new hub's file, then demotes the previous hub's dedicated file in place (versioned import alias, `vmopv1.`→`{alias}.`) now that it is a spoke; the dead `_make_v1aN_template_functions_block` generator was deleted outright rather than left as unreachable code. One subtlety only surfaced by actually running the scaffold rather than reading the diff: `step_update_imports`, a pre-existing, version-agnostic step earlier in the pipeline, already rglob-replaces every non-excluded `.go` file's import path from the old to the new API version — including the previous hub's now-dedicated file, since it has no special knowledge of the per-version split. By the time the hub-add/demote step runs, that file's import already points at the new version (under the unchanged `vmopv1` alias); the hub-add/demote logic was written to build both the new hub file and the demoted spoke file from that already-bumped state, not from the pre-migration state a first pass assumed. Verified by scaffolding V1alpha7 end-to-end against an isolated, `.git`-detached copy of this repository: all 24 steps succeeded, including `make generate`, `make generate-go-conversions`, `make manager-only`, and `make lint-go-full`, and the resulting `bootstrap_templatedata_v1alpha7.go` contained every function (legacy and new) with correct imports, and `bootstrap_templatedata_v1alpha6.go` was correctly demoted to a spoke.

## API / CRD strategy

Additive only; no version bump, no conversion webhook impact. The changed type (`NetworkDeviceStatus`) is not a CRD-serialized type (see Constitution check above), so the "additive changes are unsafe once shipped" rule for CRD types (constitution.md "API compatibility") does not apply here — this is a plain Go struct used only inside `GetTemplateRenderFunc`'s in-memory template data, never round-tripped through the API server.

**Conversion-gen fix**: adding `Gateway6`/`IPv6Addresses` to `v1alpha6.NetworkDeviceStatus` broke `make generate-go-conversions` (and therefore `make manager`) with `undefined: Convert_v1alpha6_NetworkDeviceStatus_To_v1alphaN_NetworkDeviceStatus` for every spoke version — `conversion-gen` generates the partial `autoConvert_...` function with a `// WARNING: requires manual conversion` comment for fields with no peer, but withholds the public `Convert_...` wrapper it needs, and `NetworkStatus`'s own generated conversion (for its `Devices []NetworkDeviceStatus` field) still references that withheld wrapper by name. Investigated whether these three types (`NetworkDeviceStatus`, `NetworkStatus`, `VirtualMachineTemplate`) need conversion generation at all, and confirmed they don't: no call site anywhere in the codebase invokes their generated `Convert_...` functions (grepped the whole repo), and `VirtualMachineTemplate` has no `DeepCopyObject()` — it isn't a `runtime.Object`, so it can never be reached by the real CRD conversion-webhook machinery either. Fix: `// +k8s:conversion-gen=false` on all three types, in all six version packages, then `make generate-go-conversions`. Empirically verified three narrower alternatives do **not** work — marking only `NetworkDeviceStatus` (not `NetworkStatus`/`VirtualMachineTemplate`) reproduces the same undefined-symbol error, and marking all three but only in `v1alpha6` (leaving v1alpha1–v5 unmarked) also reproduces it, because each spoke package's own `doc.go` `+k8s:conversion-gen=` tag independently starts a generation task against its own local type declarations regardless of what's marked on the hub side. All three types, in all six packages, is the only configuration that leaves zero references to any of the three types in any generated `zz_generated.conversion.go` file.

## Controller / webhook impact

None. `GetTemplateRenderFunc` (`bootstrap_templatedata.go`) is called from `pkg/providers/vsphere/vmlifecycle/bootstrap.go`, itself invoked by the Sysprep/vAppConfig/LinuxPrep bootstrap paths in the same package — no controller, webhook, or RBAC changes. No new feature flag: the new functions are unconditionally registered in V1alpha6's `FuncMap` (see spec's non-goals — not gated behind `WorkloadIPv6`).

## Test strategy

- **Unit** (`testlabels.Controller`, no infra label — pure function tests): `bootstrap_templatedata_test.go` gains dual-stack, IPv6-only, link-local-only, and mixed (usable + link-local) fixtures, plus the missing v1alpha6 `DescribeTable`s for both existing and new functions. A regression fixture confirms IPv4-only cases are byte-for-byte unchanged.
- **Integration**: not applicable — this code path has no vCenter/envtest dependency; existing tests already run without `testlabels.EnvTest`/`testlabels.VCSim`.
- **E2E** (cluster-observable — Sysprep/vAppConfig rendering; per `e2e-sync-with-changes.md`): `vm_guestcustomization.go` has a `Context("Property values use V1alpha6 template functions", ...)` with 3 `It` rounds. Each round builds a `*vmopv1.VirtualMachine` directly and creates it via `svClusterClient.Create`. All three reuse the vApp property keys already defined on the `photon-ovf-vapp-properties` OVF image (a new property key would be silently dropped — `VAppConfig.Properties` only applies values for keys that are `userConfigurable` and pre-existing on the deployed image), setting each key's value to a V1alpha6 template-function call instead of a literal. The image exposes 4 usable string-typed properties plus 1 bool and 1 int slot, so covering all 14 registered `V1alpha6_*` functions takes 3 rounds:
  - **Round A**: `string-valid`/`string-trimmed`/`string-padding-user-configurable`/`string-empty` → `FirstIPv4`/`FirstIPv6`/`FirstIPv4FromNIC`/`FirstIPv6FromNIC`; `bool-user-configurable-1`/`int-user-configurable-1` → `IsUsableIP`/`PrefixLength` with fixed inputs. VM requests dual-stack IPAM (`spec.network.interfaces[0].ipamModes: [IPv6, IPv4]`), gated on the `WorkloadIPv6` Supervisor capability.
  - **Round B**: same 4 string slots → `FirstIP`/`FirstIPFromNIC`/`FirstNicMacAddr`/`IPsFromNIC`; the bool/int slots → two more `IsUsableIP`/`PrefixLength` edge cases (link-local input, IPv6 CIDR input). Also dual-stack.
  - **Round C**: the 4 string slots → `FormatNameservers`/`SubnetMask`/`IP`/`FormatIP`, all with fixed literal inputs except `FormatNameservers` (reads the VM's real nameservers). Single-stack — none of these functions need dual-stack IPAM.
  - IPv4-returning functions are asserted strictly as valid IPv4 CIDRs (`LinuxPrep` guarantees an IPv4 address). IPv6-returning and `FormatNameservers` values are asserted leniently: either a valid rendered value, or the literal unrendered `"{{ ... }}"` text (`renderTemplate`'s fallback on error) — accepted either way, since even with dual-stack IPAM requested this test can't guarantee the address/nameservers show up in time. Fixed-input functions (`IsUsableIP`, `PrefixLength`, `SubnetMask`, `IP`, `FormatIP`) are asserted exactly.
  - Single-NIC only (index `0`) in all rounds. **Follow-up**: extend to a real second NIC once multi-NIC network resources are confirmed available in this test's default topology; also fix `vmoperator.GetVirtualMachineIP` in `test/e2e/vmservice/lib/vmoperator/vmoperator.go`, which only reads `PrimaryIP4` today, never `PrimaryIP6`.

  The new V1alpha6 IPv4-preferred behavior is provably unchanged for the common case by construction (see Design: `v1a6FirstIP`/`v1a6FirstIPFromNIC`/`v1a6IPsFromNIC` return `IPAddresses[0]`/`IPAddresses` unchanged whenever `IPAddresses` is non-empty) and is covered by unit tests.

## Rollout / migration

- No feature flag — additive Go template functions, always registered.
- No schema upgrade / backfill — no CRD field changes.
- Partner comms: `docs/concepts/workloads/guest.md` documents the new functions and the fallback/degrade semantics of the existing generic ones; release note calls out the new functions and the (rare, backward-compatible) behavior change for IPv6-only devices that previously errored on `FirstIP`/`FirstIPFromNIC`/`IPsFromNIC`.

## Complexity tracking

| Violation | Why needed | Simpler alternative rejected because |
|-----------|------------|---------------------------------------|
| None | — | — |
