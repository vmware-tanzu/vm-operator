# Implementation Plan: Remove `NetworkInterfaceResult`, use `Device` + `Bootstrap` directly

- **Branch**: [`bryanv/remove-network-interface-results`](https://github.com/bryanv/vm-operator/tree/bryanv/remove-network-interface-results)
  - **Fork**: `bryanv/vm-operator`
  - **PR target**: `vmware-tanzu/vm-operator`
- **Date**: 2026-07-29
- **Spec**: none — refactor with no user-visible behavior change (see "When SDD applies" in `sdd-standards.md`; spec.md is optional for this class of change)
- **Story**: vmop-3999 (tracks every shipping-code task in `tasks.md`)
- **Epic**: vmop-3707 — vmop-3999's Epic Link (`customfield_10830`) must be set to this once the story exists in JIRA

---

## Summary

`pkg/providers/vsphere/network` currently has two overlapping representations of a VM's network interface: the old `NetworkInterfaceResult` (a flat struct mixing CR-identity, vSphere-backing, and guest-customization fields) and the newer `Device` (vSphere backing/CR info) + `Bootstrap` (guest-customization data) pair used by the VM-create path. A bridge function, `devAndBootstrapToNetworkInterfaceResult`, merges `Device`+`Bootstrap` back into a `NetworkInterfaceResult` solely so the VM-update path's callers didn't have to change when `Device`/`Bootstrap` were introduced — that bridge is the only remaining reason `NetworkInterfaceResult` exists. This plan removes it and updates every caller to consume `Device` and `Bootstrap` fields directly, per-need, instead of through a merged struct. No CRD, webhook, controller behavior, or cluster-observable output changes; MAC-address casing behavior in particular must be preserved exactly.

---

## Technical context

| Field | Value |
|-------|-------|
| **Language** | Go (root module) |
| **Primary dependencies** | `govmomi`/`vim25` (`vimtypes`), `pkg/util/nil` (`pkgnil`) — no new dependencies added |
| **Modules touched** | root module only: `pkg/providers/vsphere/network`, `pkg/providers/vsphere/session`, `pkg/providers/vsphere/vmlifecycle` |
| **API version(s) touched** | none — no `api/` changes |
| **Testing** | Ginkgo v2 + Gomega; existing unit/integration coverage in the three packages above, extended per "Test strategy" |

---

## Constitution check

| Rule | Status | Notes |
|------|--------|-------|
| Thin controllers / business logic in `pkg/` | OK | Change is entirely within `pkg/providers/vsphere/{network,session,vmlifecycle}`; no `controllers/` package touched. |
| No controller calls vSphere directly | OK | Unaffected — provider abstraction boundary is unchanged. |
| API compatibility | OK | No `api/` types touched; no CRD/webhook change. |
| Error wrapping / context usage | OK | No new error paths introduced; existing `pkgerr` semantics in `session_vm_update.go` are untouched. |
| Testing standards (`_test.go` + `Label()`) | OK | All touched test files already follow the single-file + `Label()` convention; no `_unit_test.go`/`_intg_test.go` split introduced. |
| E2E sync with changes | OK — N/A | No cluster-observable behavior change, so no E2E addition/update is required (see "Test strategy"). |

No constitutional rule is bent by this change; **Complexity tracking** is empty.

---

## Design

### Key finding 1: no new "pair" type needed

Tracing every consumer of the old merged struct shows most need only one side of `Device`/`Bootstrap`, not both:

- **Pure `Bootstrap`** (guest-customization-facing): `GuestOSCustomization`, `NetPlanCustomization`, the six `toTemplateNetworkStatusV1A*` functions, `UpdateNetworkStatusConfig`, `GetBootstrapArgs`'s two DNS-default loops.
- **Pure `Device`** (vSphere-identity-facing): `ListOrphanedNetworkInterfaces`, `VPCPostRestoreBackingFixup`, `ReconcileNetworkInterfaces`, `fixupMacAddresses`, `fixupMacAddressMutableNetworks` — these all turn out to need only `Device` once `Name` moves there too (see "`Device` gains three fields" below).

So the merged struct disappears and each side lives where it is consumed. `NetworkInterfaceResults` (the plural container — keep its name) carries only the `Device` slice plus the two reconcile-state fields. The `Bootstrap` slice is **never stored on it**: `Bootstrap`s are built at the last moment (see key finding 2) and carried on `vmlifecycle.BootstrapArgs`, the struct every guest-customization consumer already receives. This avoids a phase-dependent field that would be invalid for most of the container's lifetime, and avoids index-aligned parallel slices in one struct — which was one of the original goals of the `Device`/`Bootstrap` split.

```go
type NetworkInterfaceResults struct {
	Devices                   []Device
	UpdatedEthCards           bool
	OrphanedNetworkInterfaces []ctrlclient.Object
}
```

Functions that only need one side take that slice type directly (`[]Device` or `[]Bootstrap`). `ReconcileNetworkInterfaces` and the two `fixupMacAddress*` functions keep taking `*NetworkInterfaceResults` since they also read/write `UpdatedEthCards` and `OrphanedNetworkInterfaces`.

`NetworkInterfaceIPConfig` and `NetworkInterfaceRoute` are unaffected — they already belong to `Bootstrap` (`Bootstrap.IPConfigs`, `Bootstrap.Routes`) and are not part of the merged struct being removed.

### Key finding 2: `Bootstrap` isn't needed until the very end — defer building it

Tracing *when* `Bootstrap` data is actually consumed: nothing before `reconcileNetworkAndGuestCustomizationState` (the last step of `session.Session.Update`) ever reads a `Bootstrap` field. `ReconcileNetworkInterfaces`, `VPCPostRestoreBackingFixup`, and the MAC-fixup functions all run earlier and, once `Device` carries a `Name` field, need nothing from `Bootstrap` at all. There's already direct evidence this is the right call: `createAndWaitNamedNetworkInterface` today returns `(Device, Bootstrap, error)`, and its VM-*create*-path caller (`createNetworkDevicesForNamedNetwork`) already discards the `Bootstrap` return with `_` — it was never needed there in the first place.

So instead of building `Bootstrap` eagerly, alongside `Device`, during `CreateAndWaitForNetworkInterfaces` (as the old bridge function did), build it once, lazily, right where it's first needed:

- `CreateAndWaitForNetworkInterfaces` and every `createAndWait*` helper (`createAndWaitNetOPNetworkInterface`, `createAndWaitNCPNetworkInterface`, `createAndWaitVPCNetworkInterface`, `createAndWaitNamedNetworkInterface`) drop their `Bootstrap` return entirely: `(Device, Bootstrap, error)` → `(Device, error)`. `CreateAndWaitForNetworkInterfaces` itself returns `([]Device, error)` instead of `(NetworkInterfaceResults, error)` — the other two `NetworkInterfaceResults` fields were never populated at that call site anyway (only ever set later, by `ReconcileNetworkInterfaces`/`ListOrphanedNetworkInterfaces`).
- A new `network.BuildBootstraps(ctx, vm *vmopv1.VirtualMachine, devices []Device) ([]Bootstrap, error)` (in `bootstrap.go`) builds one `Bootstrap` per `Device`, dispatching on `dev.InterfaceObj`'s concrete type (nil-safe via `pkgnil.IsNil`, mirroring the concrete-CR-type dispatch `CreateNetworkDevices` does today) to call `NetOPInterfaceBootstrap`/`NCPInterfaceBootstrap`/`VPCInterfaceBootstrap`, or `InterfaceBootstrap` directly for the named-network (`InterfaceObj == nil`) case. It zips `devices[i]` with `vm.Spec.Network.Interfaces[i]` — safe because `CreateAndWaitForNetworkInterfaces` builds `devices` in that exact order/length, and nothing reorders or filters it afterward. As cheap insurance against future drift, `BuildBootstraps` starts with an explicit guard: when `len(devices) == 0` return `nil, nil` (this also covers `vm.Spec.Network == nil`/disabled without touching `Interfaces`); otherwise error if `len(devices) != len(vm.Spec.Network.Interfaces)`.
- The one and only call to `BuildBootstraps` is added in `reconcileNetworkAndGuestCustomizationState`, *after* `s.fixupMacAddresses` has finished backfilling `Device.MacAddress` (see below). The returned `[]Bootstrap` is handed straight to `vmlifecycle.GetBootstrapArgs(...)` — the single choke point every guest-customization consumer passes through — and travels on `BootstrapArgs.Bootstraps` from there. It is never written back onto `NetworkInterfaceResults`.

This has a nice side effect: because `Bootstrap` is now built *after* every MAC-address backfill instead of *before*, `Bootstrap.MacAddress` is correct by construction from `Device.MacAddress` — no separate sync step is needed at all. `ReconcileNetworkInterfaces`, `fixupMacAddresses`, and `fixupMacAddressMutableNetworks` become 100% `Device`-only: they read/write `Device.EthCard`/`Device.EthCardKey`/`Device.MacAddress`/`Device.InterfaceName` exclusively, never touching `Bootstrap`.

### MAC-address case handling (behavior preservation)

Today the update path's merged `NetworkInterfaceResult.MacAddress` is the *Bootstrap* MAC, which `InterfaceBootstrap` lowercases (`strings.ToLower`, `bootstrap.go`). `CreateDefaultEthCard` therefore creates the eth card with a **lowercased** MAC. `Device.MacAddress` is never lowercased anywhere, so naively repointing the caller at `CreateDefaultEthCardFromNetworkDevice` would write a raw-case MAC to the created device whenever the user supplies an uppercase `MACAddr` (the unadvertised VDS override, and named networks) — a cluster-observable change. To preserve behavior exactly:

- `CreateAndWaitForNetworkInterfaces` (and `createAndWaitNamedNetworkInterfaces`) lowercase `dev.MacAddress` with `strings.ToLower` before appending each `Device`. These are the update-path-only entry points, and this reproduces exactly the value the old merged struct carried into `CreateDefaultEthCard`, `ReconcileNetworkInterfaces`, and `VPCPostRestoreBackingFixup`.
- `CreateNetworkDevices` (the VM-*create* path) is intentionally untouched: it passes the raw-case MAC into `ApplyNetworkDeviceToVirtualEthCard`/`CreateDefaultEthCardFromNetworkDevice` today and continues to. The create-vs-update case divergence predates this change; harmonizing it is a possible follow-up, not part of a behavior-preserving refactor.
- One nuance to accept: MACs backfilled from VC by the fixups now flow through `BuildBootstraps` → `InterfaceBootstrap`, which applies `ToLower`, whereas today the post-fixup merged MAC reached GOSC/templatedata raw. VC-assigned MACs are lowercase by convention, so this is a no-op in practice — called out here so it is a decision, not an accident.
- Test coverage: add a unit test with an uppercase `spec.network.interfaces[].macAddr` asserting the created eth card's `MacAddress` is lowercase on the update path (matching today).

### `Device` gains three fields, and one derived-name helper

`Device` currently lacks: a home for the actual `vimtypes.BaseVirtualDevice` and its VC device key (on `NetworkInterfaceResult` these were the `Device` and `DeviceKey` fields, populated after the fact by `CreateDefaultEthCard` and mutated by `ReconcileNetworkInterfaces`/`fixupMacAddressMutableNetworks`); and the interface's spec name (needed by `ReconcileNetworkInterfaces` for orphan-CR matching, previously read via `Bootstrap.Name` — now available before `Bootstrap` exists). Add all three (naming the device-object field `EthCard`/`EthCardKey` to avoid a `Device.Device` stutter; naming the spec name `InterfaceName` to avoid ambiguity with `ObjectName()` below):

```go
type Device struct {
	ProviderType pkgcfg.NetworkProviderType
	InterfaceObj ctrlclient.Object

	// InterfaceName is the interface name from the InterfaceSpec.
	InterfaceName string

	Backing    object.NetworkReference
	NetworkID  string
	MacAddress string
	ExternalID string

	// EthCard is the actual virtual ethernet card device for this interface,
	// set once it is created (update path) or matched to one of the VM's
	// existing devices (reconcile).
	EthCard vimtypes.BaseVirtualDevice
	// EthCardKey is EthCard's device key once it exists on the VM.
	EthCardKey int32
}

// ObjectName returns the name of the network interface CR backing this
// Device, or "" for named networks which have no CR.
func (d Device) ObjectName() string {
	if pkgnil.IsNil(d.InterfaceObj) {
		return ""
	}
	return d.InterfaceObj.GetName()
}
```

`InterfaceName` is set from `interfaceSpec.Name` at construction time in `getNetOPNetworkInterfaceDevice`, `getNCPNetworkInterfaceDevice`, `getVPCSubnetPortDevice`, and `createAndWaitNamedNetworkInterface` — all four already receive `interfaceSpec` as a parameter today except `getNCPNetworkInterfaceDevice`/`getVPCSubnetPortDevice`, which need `interfaceSpec` added as a new parameter (their callers already have it in scope, so this is a mechanical addition).

`ObjectName()` replaces the old `NetworkInterfaceResult.ObjectName`/`ObjectProviderType` fields (pure derivations of `d.InterfaceObj.GetName()` and `d.ProviderType`) — callers use `dev.ObjectName()` / `dev.ProviderType` directly instead of separate copied fields. This centralizes the nil-safe CR-name derivation that currently lives inline in `devAndBootstrapToNetworkInterfaceResult` (being deleted).

---

## Project structure

No new directories or files. Changes are confined to existing files in three packages:

```
pkg/providers/vsphere/network/
├── network.go              — Device fields, NetworkInterfaceResults shape, CreateAndWaitForNetworkInterfaces
├── bootstrap.go            — delete bridge fn, add BuildBootstraps
├── gosc.go                 — GuestOSCustomization signature
├── netplan.go              — NetPlanCustomization signature
├── list_interfaces.go      — ListOrphanedNetworkInterfaces signature
├── nsxt.go                 — VPCPostRestoreBackingFixup signature
└── reconcile.go            — ReconcileNetworkInterfaces body, Device-only

pkg/providers/vsphere/session/
└── session_vm_update.go    — reconcileNetworkInterfaces, fixup*, reconcileNetworkAndGuestCustomizationState

pkg/providers/vsphere/vmlifecycle/
├── bootstrap.go             — BootstrapArgs, GetBootstrapArgs
├── bootstrap_cloudinit.go   — call site update
├── bootstrap_linuxprep.go   — call site update
├── bootstrap_sysprep.go     — call site update
├── bootstrap_templatedata.go — toTemplateNetworkStatusV1A* functions
└── update_status.go         — UpdateNetworkStatusConfig
```

### File-by-file changes

**`pkg/providers/vsphere/network/network.go`**
- Delete `NetworkInterfaceResult` struct.
- Change `NetworkInterfaceResults` to the `Devices`-only shape above.
- Add `InterfaceName`/`EthCard`/`EthCardKey` fields and `ObjectName()` method to `Device`; add the `pkgnil` import (moves here from `bootstrap.go`).
- `CreateAndWaitForNetworkInterfaces`: return type becomes `([]Device, error)`. Drop the `Bootstrap` half of each `createAndWait*` call; lowercase `dev.MacAddress` (`strings.ToLower`) and append only the `Device` to the result slice (see "MAC-address case handling"; same in `createAndWaitNamedNetworkInterfaces`).
- `createAndWaitNetOPNetworkInterface`, `createAndWaitNCPNetworkInterface`, `createAndWaitVPCNetworkInterface`, `createAndWaitNamedNetworkInterface`, `createAndWaitNamedNetworkInterfaces`: drop `Bootstrap` from their return signatures entirely (no more `NetOPInterfaceBootstrap`/`NCPInterfaceBootstrap`/`VPCInterfaceBootstrap`/`InterfaceBootstrap` calls inside them — that logic moves to `BuildBootstraps`).
- `createNetworkDevicesForNamedNetwork` (VM-create path): drop the now-unnecessary `_` discard (`dev, _, err := ...` → `dev, err := ...`).
- `getNCPNetworkInterfaceDevice`, `getVPCSubnetPortDevice`: add an `interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec` parameter (to set `Device.InterfaceName`); update their two call sites each (`CreateNetworkDevices`'s loop and the corresponding `createAndWait*` helper — both already have `interfaceSpec` in scope).
- Delete `CreateDefaultEthCard` and `ApplyInterfaceResultToVirtualEthCard` (both `*NetworkInterfaceResult`-based). `ApplyInterfaceResultToVirtualEthCard` has zero callers anywhere in the repo — pure removal. `CreateDefaultEthCard`'s one caller (in `session_vm_update.go`) already has a `Device`-native twin, `CreateDefaultEthCardFromNetworkDevice` — repoint the caller at it.

**`pkg/providers/vsphere/network/bootstrap.go`**
- Delete `devAndBootstrapToNetworkInterfaceResult` and the now-unused `pkgnil` import (moves to `network.go`).
- Add `BuildBootstraps(ctx, vm, devices []Device) ([]Bootstrap, error)` (see "Key finding 2" above for its dispatch logic).
- `Bootstrap`, `InterfaceBootstrap`, `NetOPInterfaceBootstrap`, `NCPInterfaceBootstrap`, `VPCInterfaceBootstrap` are otherwise unchanged.

**`pkg/providers/vsphere/network/gosc.go`**
- `GuestOSCustomization(results NetworkInterfaceResults)` → `GuestOSCustomization(bootstraps []Bootstrap)`. Loop body: `results.Results` → `bootstraps`.

**`pkg/providers/vsphere/network/netplan.go`**
- `NetPlanCustomization(result NetworkInterfaceResults, vlans ...)` → `NetPlanCustomization(bootstraps []Bootstrap, vlans ...)`. Loop body: `result.Results` → `bootstraps`.

**`pkg/providers/vsphere/network/list_interfaces.go`**
- `ListOrphanedNetworkInterfaces(vmCtx, client, results *NetworkInterfaceResults) error` → `ListOrphanedNetworkInterfaces(vmCtx, client, devices []Device) ([]ctrlclient.Object, error)`. Body uses `d.ProviderType`/`d.ObjectName()` instead of `r.ObjectProviderType`/`r.ObjectName`; caller now assigns the returned slice to `results.OrphanedNetworkInterfaces` itself.

**`pkg/providers/vsphere/network/nsxt.go`**
- `VPCPostRestoreBackingFixup(vmCtx, currentEthCards, networkResults NetworkInterfaceResults)` → `VPCPostRestoreBackingFixup(vmCtx, currentEthCards, devices []Device)`. Loop over `devices`; `result.MacAddress`/`.ExternalID`/`.Device` → `dev.MacAddress`/`.ExternalID`/`.EthCard`; the log's `"name", result.Name` → `"name", dev.InterfaceName` (same interface-spec-name semantics as today — do **not** switch it to `dev.ObjectName()`, which is the CR name).

**`pkg/providers/vsphere/network/reconcile.go`**
- `ReconcileNetworkInterfaces` keeps its `*NetworkInterfaceResults` signature (needs `UpdatedEthCards` and `OrphanedNetworkInterfaces` too), but the loop becomes 100% `Device`-only: index `results.Devices[idx]` in place of `results.Results[idx]` — `r.Device` → `dev.EthCard`, `r.Name` → `dev.InterfaceName`, writes to `.DeviceKey`/`.MacAddress` → `dev.EthCardKey`/`dev.MacAddress`. No `Bootstrap` reference anywhere in this file.

**`pkg/providers/vsphere/session/session_vm_update.go`**
- `reconcileNetworkInterfaces`: `network.CreateAndWaitForNetworkInterfaces` now returns `[]Device` directly; assign it to a local `devices` var, run the existing `CreateDefaultEthCard` loop against `network.CreateDefaultEthCardFromNetworkDevice(vmCtx, &devices[idx])` (assigning to `devices[idx].EthCard`), then build `results := network.NetworkInterfaceResults{Devices: devices}` and, when gated by `Features.MutableNetworks` + powered-off, call the new `network.ListOrphanedNetworkInterfaces(vmCtx, s.K8sClient, devices)` and assign its return to `results.OrphanedNetworkInterfaces`.
- `handleRestoredVPCInterfaces`: pass `networkResults.Devices` to `VPCPostRestoreBackingFixup` instead of `networkResults`.
- `fixupMacAddresses` (both branches) and `fixupMacAddressMutableNetworks`: keep taking `*network.NetworkInterfaceResults` (for the `UpdatedEthCards` gate), but every per-interface read/write (`.MacAddress`, `.DeviceKey`, `.Device`) moves to `.Devices[i]` — no `Bootstrap` in sight.
- `reconcileNetworkAndGuestCustomizationState`: immediately after `s.fixupMacAddresses(vmCtx, vcVM, &networkResults)` returns, build the bootstraps and hand them (plus the `UpdatedEthCards` flag) straight to `GetBootstrapArgs`:
  ```go
  bootstraps, err := network.BuildBootstraps(vmCtx, vmCtx.VM, networkResults.Devices)
  if err != nil {
  	return err
  }

  bootstrapArgs, err := vmlifecycle.GetBootstrapArgs(
  	vmCtx,
  	s.K8sClient,
  	bootstraps,
  	networkResults.UpdatedEthCards,
  	bootstrapData)
  ```
- `UpdateEthCardDeviceChanges` is unaffected (thin wrapper). `VMUpdateArgs`/`VMResizeArgs.NetworkResults` stay typed `network.NetworkInterfaceResults` — they're consumed only for `.Devices`/`.OrphanedNetworkInterfaces`/`.UpdatedEthCards` within the power-state branch, which was already true before this change.
- **Preserve the slice-aliasing behavior exactly — do not "fix" it in passing.** `updateArgs.NetworkResults = networkResults` and `resizeArgs.NetworkResults = networkResults` are struct *copies* of the local `networkResults` in `reconcilePoweredOffOrPoweredOnVM`, and `reconcileNetworkAndGuestCustomizationState` receives the original local, not the copy. Two consequences:
  - `UpdatedEthCards` set by `ReconcileNetworkInterfaces` lands on the copy and never reaches `reconcileNetworkAndGuestCustomizationState` — the gates in `fixupMacAddressMutableNetworks` and `bootstrap_cloudinit.go` apparently always see `false` on this path today. That looks like a pre-existing bug; it is **out of scope** here (file/track it separately) and this refactor must not change it as a side effect — in particular, do not "simplify" by passing `updateArgs.NetworkResults` into `reconcileNetworkAndGuestCustomizationState`.
  - The per-element writes that *do* propagate (`.EthCardKey`, `.MacAddress`, mutations to `.EthCard`) do so purely because both copies share the `Devices` slice's backing array. So `reconcileNetworkInterfaces` must build the `Devices` slice **once** — no append/reallocation after the copies into `updateArgs`/`resizeArgs` are taken — or the MAC/key backfill silently stops reaching `BuildBootstraps`.

**`pkg/providers/vsphere/vmlifecycle/bootstrap.go`**
- `BootstrapArgs`: drop the `NetworkResults network.NetworkInterfaceResults` field; add `Bootstraps []network.Bootstrap` and `UpdatedEthCards bool` in its place. Every guest-customization consumer only ever needed those two pieces, so `vmlifecycle` no longer sees `NetworkInterfaceResults` at all.
- `GetBootstrapArgs`: the `networkResults network.NetworkInterfaceResults` parameter becomes `bootstraps []network.Bootstrap, updatedEthCards bool`; `bsa` is initialized with `Bootstraps: bootstraps, UpdatedEthCards: updatedEthCards`.
- `GetBootstrapArgs` has **two** loops over the old `networkResults.Results`, not one — both move to `bootstraps`:
  - the read loop that decides `getDNSInformationFromConfigMap` (reads `DHCP4`/`DHCP6`/`Nameservers`/`SearchDomains`);
  - the cloud-init loop that **mutates** `r.Nameservers`/`r.SearchDomains` in place to backfill global DNS defaults. Those writes are visible in `bsa.Bootstraps` only because `bsa` holds the same slice header the loop iterates — keep it that way (mutate `bootstraps[i]` by index or pointer; do not deep-copy the slice into `bsa`).

**`pkg/providers/vsphere/vmlifecycle/bootstrap_cloudinit.go`, `bootstrap_linuxprep.go`, `bootstrap_sysprep.go`**
- Update the three `network.GuestOSCustomization(...)`/`network.NetPlanCustomization(...)` call sites to pass `bsArgs.Bootstraps` instead of `bsArgs.NetworkResults`. The `UpdatedEthCards` check in `bootstrap_cloudinit.go` becomes `bsArgs.UpdatedEthCards`.

**`pkg/providers/vsphere/vmlifecycle/bootstrap_templatedata.go`**
- All six `toTemplateNetworkStatusV1A*` functions: `bsArgs.NetworkResults.Results` → `bsArgs.Bootstraps`.

**`pkg/providers/vsphere/vmlifecycle/update_status.go`**
- `UpdateNetworkStatusConfig`: `args.NetworkResults.Results` → `args.Bootstraps`.

---

## API / CRD strategy

Not applicable — no `api/` types, CRD manifests, or webhooks are touched by this change.

---

## Controller / webhook impact

No `controllers/` or `webhooks/` package is touched. The change is entirely internal to the vSphere provider (`pkg/providers/vsphere/network`, `pkg/providers/vsphere/session`, `pkg/providers/vsphere/vmlifecycle`), which controllers already consume only through the `VMProvider` interface. No new RBAC and no new feature flag.

---

## Test strategy

- **Unit** (`testlabels.Controller` where applicable, package-local elsewhere): mechanical rewrites, split by which struct the fixture's fields belong to:
  - `gosc_test.go`, `netplan_test.go`, `bootstrap_linuxprep_test.go`, `bootstrap_sysprep_test.go`, `bootstrap_templatedata_test.go`, `update_status_test.go`: fixtures only ever set Bootstrap-shaped fields (`MacAddress`, `IPConfigs`, `DHCP4`, `Nameservers`, `GuestDeviceName`, `Name`, `Routes`, ...) — rewrite `network.NetworkInterfaceResult{...}` → `network.Bootstrap{...}`, passed as a plain `[]network.Bootstrap` or assigned to `bsArgs.Bootstraps` where the fixture builds a `BootstrapArgs`. Fixtures that set `NetworkResults.UpdatedEthCards` move to `bsArgs.UpdatedEthCards`.
  - `list_interfaces_test.go`: switch to calling `network.ListOrphanedNetworkInterfaces(vmCtx, client, []network.Device{...})` and asserting on the returned slice, instead of mutating a `*NetworkInterfaceResults`.
  - `nsxt_test.go`: `initNetworkResult` helper becomes a `network.Device{...}` builder (set `EthCard`, `ExternalID`, `MacAddress`); calls to `VPCPostRestoreBackingFixup` pass `[]network.Device{...}` directly.
  - `reconcile_test.go`: build `network.NetworkInterfaceResults{Devices: [...]}` (with `InterfaceName`/`EthCard` set on each `Device`) instead of `Results: []network.NetworkInterfaceResult{...}` — this file needs no `Bootstrap` fixtures at all.
  - `network_test.go`: update the lone `network.NetworkInterfaceResults{}` declaration/usage, and any test exercising `CreateAndWaitForNetworkInterfaces`/`createAndWait*` helpers, to the new `[]Device`-returning (no `Bootstrap`) signatures.
  - `bootstrap_test.go`: add coverage for the new `BuildBootstraps` dispatch function (nil `InterfaceObj` → named-network path; each CR type → the matching `*InterfaceBootstrap` call; unsupported type → error; empty `devices` → `nil, nil` even when `vm.Spec.Network` is nil; device/interface length mismatch → error).
  - `session_vm_update_test.go`: the `XContext`-disabled "Ethernet Card Changes" block still needs to compile — update its `network.NetworkInterfaceResult{Device: ...}` fixtures to the `Devices` shape even though the block is currently skipped.
  - New: a unit test with an uppercase `spec.network.interfaces[].macAddr` asserting the created eth card's `MacAddress` is lowercase on the update path (see "MAC-address case handling").
- **Integration** (`testlabels.EnvTest`/`testlabels.VCSim`): none added — no behavior change to exercise beyond the existing suites in the three touched packages, which must continue to pass unchanged.
- **E2E**: none required — no cluster-observable behavior change, per `e2e-sync-with-changes.md`.
- **Manual verification** (in addition to automated tests):
  - `go build ./...` and `go vet ./...` across the root module.
  - `go test ./pkg/providers/vsphere/network/... ./pkg/providers/vsphere/session/... ./pkg/providers/vsphere/vmlifecycle/...` — all existing unit/integration tests must pass unchanged in behavior (only fixture construction changes).
  - `make lint-go` for import grouping/alias conventions on the new/moved `pkgnil` import.
  - Manually re-read the final `ReconcileNetworkInterfaces`, `fixupMacAddresses`, `fixupMacAddressMutableNetworks`, `BuildBootstraps`, and the new call site in `reconcileNetworkAndGuestCustomizationState` side-by-side with the original to confirm every MAC-address backfill still ends up reflected in `Device.MacAddress` before `BuildBootstraps` runs — this is the highest-risk spot since it's the one place behavior could silently regress (e.g. a VM whose interface gets a VC-generated MAC would otherwise end up with an empty MAC in cloud-init/GOSC/sysprep/status). While there, confirm the `Devices` slice built in `reconcileNetworkInterfaces` is never reallocated afterward (the slice-aliasing requirement above).
  - Verify the MAC-address case behavior per "MAC-address case handling": the new uppercase-`MACAddr` unit test passes, and the create path (`CreateNetworkDevices` consumers) is diff-free.

---

## Rollout / migration

Not applicable — internal refactor with no feature flag, no schema/backfill change, and no partner-facing surface. Ships as a single PR with no rollout sequencing.

**--**-

## Complexity tracking

None — no constitutional rule is bent by this change.
