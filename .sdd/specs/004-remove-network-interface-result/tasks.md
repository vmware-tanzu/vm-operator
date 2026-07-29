# Tasks: Remove `NetworkInterfaceResult`, use `Device` + `Bootstrap` directly

- **Plan**: [`plan.md`](./plan.md)
- **Epic**: vmop-3707
- **Story**: vmop-3999 — needs its Epic Link (`customfield_10830`) set to vmop-3707

<!--
TODO: set vmop-3999's Epic Link (customfield_10830) to vmop-3707.
Check off each task as it lands, and add any task discovered mid-implementation
that isn't listed yet.
-->

## Phase 1 — Foundational (`network` package core types)

- [ ] T001 [vmop-3999] Delete `NetworkInterfaceResult`; change `NetworkInterfaceResults` to the `Devices`-only shape; add `InterfaceName`/`EthCard`/`EthCardKey` fields and `ObjectName()` to `Device`; move the `pkgnil` import in from `bootstrap.go` (`pkg/providers/vsphere/network/network.go`)
- [ ] T002 [vmop-3999] Change `CreateAndWaitForNetworkInterfaces` and every `createAndWait*` helper to drop their `Bootstrap` return (`(Device, Bootstrap, error)` → `(Device, error)`; container return becomes `([]Device, error)`); lowercase `dev.MacAddress` before appending in the update-path entry points per "MAC-address case handling" in `plan.md` (`pkg/providers/vsphere/network/network.go`)
- [ ] T003 [P] [vmop-3999] Drop the now-unnecessary `_` discard in `createNetworkDevicesForNamedNetwork`; add the `interfaceSpec` parameter to `getNCPNetworkInterfaceDevice`/`getVPCSubnetPortDevice` and update their two call sites each (`pkg/providers/vsphere/network/network.go`)
- [ ] T004 [P] [vmop-3999] Delete `CreateDefaultEthCard` and `ApplyInterfaceResultToVirtualEthCard` (`pkg/providers/vsphere/network/network.go`)
- [ ] T005 [vmop-3999] Delete `devAndBootstrapToNetworkInterfaceResult` and its now-unused `pkgnil` import; add `BuildBootstraps(ctx, vm, devices []Device) ([]Bootstrap, error)` per "Key finding 2" in `plan.md` (`pkg/providers/vsphere/network/bootstrap.go`)

## Phase 2 — `network` package consumers (depend on Phase 1)

- [ ] T006 [P] [vmop-3999] `GuestOSCustomization(results NetworkInterfaceResults)` → `GuestOSCustomization(bootstraps []Bootstrap)` (`pkg/providers/vsphere/network/gosc.go`)
- [ ] T007 [P] [vmop-3999] `NetPlanCustomization(result NetworkInterfaceResults, ...)` → `NetPlanCustomization(bootstraps []Bootstrap, ...)` (`pkg/providers/vsphere/network/netplan.go`)
- [ ] T008 [P] [vmop-3999] `ListOrphanedNetworkInterfaces(vmCtx, client, results *NetworkInterfaceResults) error` → `ListOrphanedNetworkInterfaces(vmCtx, client, devices []Device) ([]ctrlclient.Object, error)` (`pkg/providers/vsphere/network/list_interfaces.go`)
- [ ] T009 [P] [vmop-3999] `VPCPostRestoreBackingFixup(vmCtx, currentEthCards, networkResults NetworkInterfaceResults)` → `VPCPostRestoreBackingFixup(vmCtx, currentEthCards, devices []Device)`; keep the log's `"name"` value as `dev.InterfaceName`, not `dev.ObjectName()` (`pkg/providers/vsphere/network/nsxt.go`)
- [ ] T010 [P] [vmop-3999] Make `ReconcileNetworkInterfaces`'s loop 100% `Device`-only (`results.Devices[idx]`, `dev.EthCard`/`.EthCardKey`/`.InterfaceName`/`.MacAddress`) (`pkg/providers/vsphere/network/reconcile.go`)

## Phase 3 — `vmlifecycle` bootstrap plumbing (depends on Phase 1)

- [ ] T011 [vmop-3999] `BootstrapArgs`: drop `NetworkResults network.NetworkInterfaceResults`, add `Bootstraps []network.Bootstrap` and `UpdatedEthCards bool`; update `GetBootstrapArgs`'s signature and its two loops (DNS-default read loop and the mutating cloud-init loop — keep the mutation visible via shared slice header, not a deep copy) (`pkg/providers/vsphere/vmlifecycle/bootstrap.go`)

## Phase 4 — `session` wiring (depends on Phases 1–3)

- [ ] T012 [vmop-3999] `reconcileNetworkInterfaces`: consume `[]Device` from `CreateAndWaitForNetworkInterfaces`, repoint the eth-card loop at `network.CreateDefaultEthCardFromNetworkDevice`, build `results.Devices` once (no later append/reallocation — see slice-aliasing note in `plan.md`), wire the new `ListOrphanedNetworkInterfaces` return into `results.OrphanedNetworkInterfaces`; update `handleRestoredVPCInterfaces` to pass `networkResults.Devices`; move `fixupMacAddresses`/`fixupMacAddressMutableNetworks` per-interface reads/writes onto `.Devices[i]` (`pkg/providers/vsphere/session/session_vm_update.go`)
- [ ] T013 [vmop-3999] `reconcileNetworkAndGuestCustomizationState`: after `s.fixupMacAddresses` returns, call `network.BuildBootstraps` and pass its result plus `networkResults.UpdatedEthCards` into `vmlifecycle.GetBootstrapArgs`; do **not** change the existing `updateArgs.NetworkResults`/`resizeArgs.NetworkResults` copy semantics (`pkg/providers/vsphere/session/session_vm_update.go`)

## Phase 5 — `vmlifecycle` consumers (depend on Phase 3)

- [ ] T014 [P] [vmop-3999] Update the `network.GuestOSCustomization`/`network.NetPlanCustomization` call site and the `UpdatedEthCards` check to `bsArgs.Bootstraps`/`bsArgs.UpdatedEthCards` (`pkg/providers/vsphere/vmlifecycle/bootstrap_cloudinit.go`)
- [ ] T015 [P] [vmop-3999] Update the `network.GuestOSCustomization`/`network.NetPlanCustomization` call site to `bsArgs.Bootstraps` (`pkg/providers/vsphere/vmlifecycle/bootstrap_linuxprep.go`)
- [ ] T016 [P] [vmop-3999] Update the `network.GuestOSCustomization`/`network.NetPlanCustomization` call site to `bsArgs.Bootstraps` (`pkg/providers/vsphere/vmlifecycle/bootstrap_sysprep.go`)
- [ ] T017 [P] [vmop-3999] All six `toTemplateNetworkStatusV1A*` functions: `bsArgs.NetworkResults.Results` → `bsArgs.Bootstraps` (`pkg/providers/vsphere/vmlifecycle/bootstrap_templatedata.go`)
- [ ] T018 [P] [vmop-3999] `UpdateNetworkStatusConfig`: `args.NetworkResults.Results` → `args.Bootstraps` (`pkg/providers/vsphere/vmlifecycle/update_status.go`)

## Phase 6 — Tests (depend on the corresponding source-file tasks above)

- [ ] T019 [P] [vmop-3999] Rewrite `network.NetworkInterfaceResult{...}` fixtures to `network.Bootstrap{...}` / `[]network.Bootstrap` / `bsArgs.Bootstraps`; move `NetworkResults.UpdatedEthCards` fixtures to `bsArgs.UpdatedEthCards` (`pkg/providers/vsphere/network/gosc_test.go`, `pkg/providers/vsphere/network/netplan_test.go`, `pkg/providers/vsphere/vmlifecycle/bootstrap_linuxprep_test.go`, `pkg/providers/vsphere/vmlifecycle/bootstrap_sysprep_test.go`, `pkg/providers/vsphere/vmlifecycle/bootstrap_templatedata_test.go`, `pkg/providers/vsphere/vmlifecycle/update_status_test.go`)
- [ ] T020 [P] [vmop-3999] Switch to `network.ListOrphanedNetworkInterfaces(vmCtx, client, []network.Device{...})` and assert on the returned slice (`pkg/providers/vsphere/network/list_interfaces_test.go`)
- [ ] T021 [P] [vmop-3999] Turn the `initNetworkResult` helper into a `network.Device{...}` builder; update `VPCPostRestoreBackingFixup` calls to pass `[]network.Device{...}` (`pkg/providers/vsphere/network/nsxt_test.go`)
- [ ] T022 [P] [vmop-3999] Build `network.NetworkInterfaceResults{Devices: [...]}` fixtures (`InterfaceName`/`EthCard` set per `Device`) instead of `Results: []network.NetworkInterfaceResult{...}` (`pkg/providers/vsphere/network/reconcile_test.go`)
- [ ] T023 [P] [vmop-3999] Update the `network.NetworkInterfaceResults{}` usage and any `CreateAndWaitForNetworkInterfaces`/`createAndWait*` test to the new `[]Device`-returning signatures (`pkg/providers/vsphere/network/network_test.go`)
- [ ] T024 [P] [vmop-3999] Add `BuildBootstraps` dispatch coverage: nil `InterfaceObj` → named-network path; each CR type → matching `*InterfaceBootstrap` call; unsupported type → error; empty `devices` → `nil, nil` even when `vm.Spec.Network` is nil; length mismatch → error (`pkg/providers/vsphere/network/bootstrap_test.go`)
- [ ] T025 [P] [vmop-3999] Add a unit test with an uppercase `spec.network.interfaces[].macAddr` asserting the created eth card's `MacAddress` is lowercase on the update path (`pkg/providers/vsphere/network/network_test.go` or `pkg/providers/vsphere/session/session_vm_update_test.go`, per "MAC-address case handling" in `plan.md`)
- [ ] T026 [vmop-3999] Update the `XContext`-disabled "Ethernet Card Changes" block's `network.NetworkInterfaceResult{Device: ...}` fixtures to the `Devices` shape so the file keeps compiling (`pkg/providers/vsphere/session/session_vm_update_test.go`)

## Phase Final — Verification

- [ ] T027 [vmop-3999] `go build ./...`, `go vet ./...`, `make lint-go`; `go test ./pkg/providers/vsphere/network/... ./pkg/providers/vsphere/session/... ./pkg/providers/vsphere/vmlifecycle/...`; manually re-read `ReconcileNetworkInterfaces`, `fixupMacAddresses`, `fixupMacAddressMutableNetworks`, `BuildBootstraps`, and the new call site in `reconcileNetworkAndGuestCustomizationState` side-by-side with the original per the "Test strategy" checklist in `plan.md`
