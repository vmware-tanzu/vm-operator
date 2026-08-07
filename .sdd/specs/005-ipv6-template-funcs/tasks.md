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

## Phase Final — Polish

- [x] T014 [vmop-1152] Release note in the PR description per `pull-request-standards.md` — drafted; included in the final summary for the PR author to paste in when opening the PR.
- [x] T015 Run `go build ./...`, `go vet ./...`, `make lint-go`, and the package's Ginkgo suite; confirm v1a1–v1a5 cases are unaffected — all green: root + `api` module build/vet clean, `golangci-lint run ./pkg/...` reports 0 issues, full `vmlifecycle` Ginkgo suite passes (all v1alpha1–v1alpha6 specs green).
