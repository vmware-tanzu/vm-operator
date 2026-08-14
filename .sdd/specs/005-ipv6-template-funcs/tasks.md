# Tasks: Differentiate IPv4/IPv6 vApp/Sysprep template functions

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Epic**: vmop-787

## Phase 1 — Foundational

- [x] T001 [vmop-1152] Add `Gateway6 string` and `IPv6Addresses []string` fields to `NetworkDeviceStatus` (`api/v1alpha6/virtualmachinetempl_types.go`)
- [x] T002 [vmop-1152] Add `V1alpha6FirstIPv4`, `V1alpha6FirstIPv6`, `V1alpha6FirstIPv4FromNIC`, `V1alpha6FirstIPv6FromNIC`, `V1alpha6IsUsableIP`, `V1alpha6PrefixLength` constants (`pkg/providers/vsphere/constants/constants.go`)

## Phase 2 — Data population

- [x] T003 [vmop-1152] Update `toTemplateNetworkStatusV1A6` to populate `Gateway6`/`IPv6Addresses` unfiltered in the `else` branch of the `IsIPv4` check (`pkg/providers/vsphere/vmlifecycle/bootstrap_templatedata.go`)

## Phase 3 — New functions

- [x] T004 [P] [vmop-1152] Add strict `FirstIPv4`/`FirstIPv6`/`FirstIPv4FromNIC`/`FirstIPv6FromNIC` functions + `FuncMap` entries to `v1a6TemplateFunctions` (`pkg/providers/vsphere/vmlifecycle/bootstrap_templatedata.go`). Implemented as standalone package-level functions (not closures) to keep `v1a6TemplateFunctions`'s cyclomatic complexity under the `gocyclo` threshold. `IPv4sFromNIC`/`IPv6sFromNIC` were considered and dropped — `(index .V1alpha6.Net.Devices N).IPAddresses`/`IPv6Addresses` already exposes the same raw, unfiltered list via direct field access on the template's root data object (documented in `docs/concepts/workloads/guest.md` "Input Object"), so a dedicated wrapper function added no capability, only a redundant name.
- [x] T005 [P] [vmop-1152] Add `IsUsableIP` function + `FuncMap` entry to `v1a6TemplateFunctions` (`pkg/providers/vsphere/vmlifecycle/bootstrap_templatedata.go`)
- [x] T006 [P] [vmop-1152] Add `PrefixLength` function + `FuncMap` entry to `v1a6TemplateFunctions` (`pkg/providers/vsphere/vmlifecycle/bootstrap_templatedata.go`)

## Phase 4 — Fallback and guards on existing functions

- [x] T007 [vmop-1152] Add cascading IPv4→usable-IPv6→any-IPv6 fallback to `v1a6FirstIP`/`v1a6FirstIPFromNIC`; IPv4→raw-IPv6 fallback to `v1a6IPsFromNIC`; fixes the latent index-out-of-range panic on a device with zero IPv4 addresses (`pkg/providers/vsphere/vmlifecycle/bootstrap_templatedata.go`)
- [x] T008 [vmop-1152] Add explicit IPv6-input error guard to `v1a6SubnetMask` and `v1a6IP` (`pkg/providers/vsphere/vmlifecycle/bootstrap_templatedata.go`)

## Phase 5 — Tests

- [x] T009 [vmop-1152] Add dual-stack, IPv6-only, link-local-only, and mixed (usable + link-local) fixtures to `bootstrap_templatedata_test.go`
- [x] T010 [vmop-1152] Add the missing `"v1alpha6 template functions"` / `"v1alpha6 constant names"` `DescribeTable`s covering existing + new functions, `IsUsableIP`, `PrefixLength`, and `SubnetMask`/`IP` IPv6-error cases (`bootstrap_templatedata_test.go`)
- [x] T011 [P] [vmop-1152] Add regression fixture asserting IPv4-only cases are byte-for-byte unchanged (`bootstrap_templatedata_test.go`) — satisfied by the backfilled v1alpha6 tables asserting the same expected values as v1alpha1–v1alpha5 against the shared IPv4-only fixture, plus an explicit dual-stack `It` confirming `FirstIP`/`FirstIPFromNIC`/`IPsFromNIC` still return IPv4 when present.

## Phase 6 — Docs and E2E

- [x] T012 [P] [vmop-1152] Add new function rows + fallback/degrade note to the V1alpha6 table in `docs/concepts/workloads/guest.md`
- [x] T013 [vmop-1152] Investigate dual-stack E2E harness support; add a V1alpha6 scenario to `test/e2e/vmservice/vmservice/virtualmachine/vm_guestcustomization.go`, or document the gap per the spec's open question — investigated three levels deep (no dual-stack subnet/builder support; zero active E2E coverage of template-function rendering at all before this change, for any version; `RawProperties`-driven vApp properties are silently dropped unless pre-defined on the deployed OVF image). Resolved by reusing `photon-ovf-vapp-properties`'s already-defined properties (from "Multiple vAppConfigs specified") with V1alpha6 template-function values instead of literals — one property per function under test — added `When("Property values use V1alpha6 template functions", ...)` + `verifyV1alpha6TemplateFunctionProperties`, `Label("experimental")`. Genuine dual-stack NIC provisioning and multi-NIC (`*FromNIC` beyond index 0) coverage remain tracked follow-ups in `plan.md` Test strategy.

## Phase 7 — Fix conversion-gen build break

- [x] T016 [vmop-1152] Fix `make generate-go-conversions`/`make manager` failure (`undefined: Convert_v1alpha6_NetworkDeviceStatus_To_v1alphaN_NetworkDeviceStatus`) caused by T001's new fields having no peer in v1alpha1–v5. Confirmed no call site anywhere invokes the generated `NetworkDeviceStatus`/`NetworkStatus`/`VirtualMachineTemplate` conversions, and `VirtualMachineTemplate` isn't a `runtime.Object` (no `DeepCopyObject()`), so they're unreachable via the real conversion-webhook path too. Added `// +k8s:conversion-gen=false` to all three types in all six version packages (`api/v1alpha1`–`api/v1alpha6/virtualmachinetempl_types.go`) and reran `make generate-go-conversions`. Empirically ruled out two narrower alternatives first (marking only `NetworkDeviceStatus`, and marking all three but only in `v1alpha6`) — both reproduce the same undefined-symbol error; all three types in all six packages is the only configuration that works.

## Phase 8 — Fix scaffolding script for future versions

- [x] T017 [vmop-1152] Fix `hack/new-schema-version.py` so scaffolding the version after V1alpha6 (e.g. V1alpha7) works with V1alpha6's new standalone (non-closure) template functions — a review comment ("Have AI update hack/new-schema-version.py too") led to finding the script's hardcoded `_make_v1aN_template_functions_block` generator still emitted the old inline-closure style (missing every new IPv4/IPv6/`IsUsableIP`/`PrefixLength` function), and its anchor-based insertion into the shared `bootstrap_templatedata.go` no longer matched V1alpha6's restructured FuncMap — confirmed by actually running `python3 hack/new-schema-version.py v1alpha7 --verbose` against an isolated, `.git`-detached copy of the repo and observing the real compiler errors, not by inspection. Split V1alpha6's template-function code out of `bootstrap_templatedata.go` into a new dedicated `bootstrap_templatedata_v1alpha6.go` (V1alpha1-V1alpha5 stay bundled in the shared file forever, frozen legacy); rewrote `step_update_template_constants` (extract-and-rename the previous hub's constant block instead of a hardcoded template) and `step_update_bootstrap_templatedata`'s hub-add/demote logic (whole-file copy + token rename of the previous hub's dedicated file, instead of anchor-based insertion); deleted the now-dead `_make_v1aN_template_functions_block` generator outright. Re-verified end-to-end by re-scaffolding V1alpha7 against a fresh isolated copy: all 24 script steps succeeded, including `make generate`, `make generate-go-conversions`, `make manager-only`, and `make lint-go-full`. One additional bug found only by that live re-run (not by inspection): `step_update_imports`, a pre-existing step earlier in the pipeline, already bulk-renames the previous hub's dedicated file's import path before the hub-add/demote step runs; the first pass of the new logic wrongly assumed the pre-migration import state, which was fixed to build both the new hub file and the demoted spoke file from the already-bumped state instead.

## Phase 9 — Genuine dual-stack E2E coverage

- [x] T018 [vmop-1152] Add `WorkloadIPv6CapabilityName = "supports_workload_ipv6"` to `test/e2e/vmservice/consts/consts.go`.
- [x] T019 [vmop-1152] Rework the V1alpha6 vApp-property test in `vm_guestcustomization.go` into a `Context` with 3 `It` rounds, covering all 14 registered `V1alpha6_*` template functions (up from 6). Each round builds a `*vmopv1.VirtualMachine` (Go struct) and creates it directly via `svClusterClient.Create`, gates dual-stack rounds on the `WorkloadIPv6` Supervisor capability, and requests dual-stack IPAM via `spec.network.interfaces[0].ipamModes = [IPv6, IPv4]` where needed. New helpers: `vAppProp`, `buildVAppConfigVM`, `createAndVerifyVAppConfigVM`. The `Context` manages its own VM lifecycle (`skipCleanup = true` plus a local `AfterEach` that deletes via the client).
  - Round A (dual-stack): `FirstIPv4`/`FirstIPv6`/`FirstIPv4FromNIC`/`FirstIPv6FromNIC`/`IsUsableIP`/`PrefixLength`.
  - Round B (dual-stack): `FirstIP`/`FirstIPFromNIC`/`FirstNicMacAddr`/`IPsFromNIC` plus two more `IsUsableIP`/`PrefixLength` edge cases (link-local input, IPv6 CIDR input).
  - Round C (single-stack): `FormatNameservers` (leniently — reads real VM nameservers), `SubnetMask`/`IP`/`FormatIP` (fixed literal inputs, asserted exactly).
  - `Label("experimental")` retained on all three `It`s until validated on real dual-stack hardware.

## Phase Final — Polish

- [x] T014 [vmop-1152] Release note in the PR description per `pull-request-standards.md` — drafted; included in the final summary for the PR author to paste in when opening the PR.
- [x] T015 Run `go build ./...`, `go vet ./...`, `make lint-go`, `make manager`, and the package's Ginkgo suite; confirm v1a1–v1a5 cases are unaffected — all green: root + `api` + `test/e2e` module build/vet clean, `make manager` succeeds, `golangci-lint` reports 0 issues across `pkg/...` and `api/...`, full `vmlifecycle` Ginkgo suite passes (all v1alpha1–v1alpha6 specs green).
