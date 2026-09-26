# Plan: Move Conversion to a Separate Module

## Goal

Remove `sigs.k8s.io/controller-runtime` from `api/go.mod` and `api/test/go.mod` by moving all conversion method implementations out of the `api/` sub-tree and into the root module's existing `webhooks/conversion/` packages. No new Go module is required.

## Background

The `api/` module currently imports `sigs.k8s.io/controller-runtime v0.19.0` solely for `pkg/conversion`, which provides two interfaces:

- `conversion.Hub` — a marker interface with a `Hub()` method
- `conversion.Convertible` — requires `ConvertTo(Hub) error` and `ConvertFrom(Hub) error`

Every spoke type (v1alpha1–v1alpha5) implements `Convertible` via methods in `*_conversion.go` files, and hub types (v1alpha6) implement `Hub` via trivial marker methods. This forces controller-runtime — and its large transitive closure — into the dependency graph of anyone who imports `api/`.

Controller-runtime v0.24.0, already used by the root module, shipped the APIs from upstream PR [kubernetes-sigs/controller-runtime#3335](https://github.com/kubernetes-sigs/controller-runtime/pull/3335): `NewHubSpokeConverter`, `NewSpokeConverter`, and `WithConverter` in `pkg/webhook/conversion`. These allow conversion logic to live anywhere — the types themselves no longer need to implement `Convertible`. This is the mechanism we exploit.

Reference implementation: [kubernetes-sigs/cluster-api#13762](https://github.com/kubernetes-sigs/cluster-api/pull/13762), which moves conversion into the root module's `webhooks/conversion/` packages — no new sub-module inside `api/`.

---

## Atomicity Constraint

Tasks 1, 2, 3a, 3b, and 3c are **not independent** and must land in a single compilable changeset:

- **Task 3a cannot precede 3b.** Deleting `webhooks/conversion/v1alpha1/`–`v1alpha5/` while `webhooks/conversion/webhooks.go` still imports them produces a compile error. 3a and 3b are atomic.
- **Task 1 cannot precede 3c.** After Task 1 removes `ConvertTo`/`ConvertFrom` from spoke types, controller-runtime's `IsConvertible()` check during `Complete()` finds hub types without their convertible spokes and returns a `PartialImplementationError`. Task 3c (adding `WithConverter`) must land in the same commit as Task 1.
- **Task 2 must complete before 3c.** The `WithConverter(<var>)` calls reference package-level variables defined in Task 2.

---

## Target Architecture

```
api/                         — type definitions only
  go.mod: k8s.io/api, k8s.io/apimachinery  (no controller-runtime)
  v1alphaN/*_conversion.go:  Convert_xxx_To_yyy() functions remain;
                              ConvertTo/ConvertFrom methods removed;
                              Hub()/List Hub() markers removed from v1alpha6;
                              empty files deleted after removal

webhooks/conversion/         — conversion logic moves here (root module)
  v1alpha6/webhooks.go:      registers all conversion webhooks with
                              .WithConverter(<var>).Complete()
  v1alpha6/convert_xxx.go:   package-level converter var + version-suffixed
                              or shared converter functions per spoke version
  v1alpha6/convert_xxx_test.go: conversion round-trip tests (moved from api/test/)
  v1alpha1..v1alpha5/:        deleted (no longer needed)

api/test/                    — fuzz harness redesigned without Convertible
  go.mod: controller-runtime removed after harness redesign
  v1alphaN/*_conversion_test.go: ConvertTo/ConvertFrom call sites replaced
                              with direct Convert_xxx_To_yyy calls; tests
                              that verify the full round-trip pipeline move
                              to webhooks/conversion/v1alpha6/
```

---

## Spoke Version Matrix

`NewHubSpokeConverter` validates at `Complete()` time that every version registered in the scheme for a given `GroupKind` has a spoke converter. Getting this wrong causes the manager to fail to start. The following matrix, derived from the current `*_conversion.go` files, is the authoritative reference for Task 3.

| Kind | v1alpha1 | v1alpha2 | v1alpha3 | v1alpha4 | v1alpha5 |
|---|:---:|:---:|:---:|:---:|:---:|
| VirtualMachine | ✓ | ✓ | ✓ | ✓ | ✓ |
| VirtualMachineClass | ✓ | ✓ | ✓ | ✓ | ✓ |
| VirtualMachineImage | ✓ | ✓ | ✓ | ✓ | ✓ |
| ClusterVirtualMachineImage | ✓ | ✓ | ✓ | ✓ | ✓ |
| VirtualMachinePublishRequest | ✓ | ✓ | ✓ | ✓ | ✓ |
| VirtualMachineService | ✓ | ✓ | ✓ | ✓ | ✓ |
| VirtualMachineSetResourcePolicy | ✓ | ✓ | ✓ | ✓ | ✓ |
| VirtualMachineGroup | ✗ | ✓ | ✓ | ✓ | ✓ |
| VirtualMachineWebConsoleRequest | ✗ | ✓ | ✓ | ✓ | ✓ |
| VirtualMachineImageCache | ✗ | ✗ | ✓ | ✓ | ✓ |
| VirtualMachineReplicaSet | ✗ | ✗ | ✓ | ✓ | ✓ |
| VirtualMachineClassInstance | ✗ | ✗ | ✗ | ✓ | ✓ |
| VirtualMachineSnapshot | ✗ | ✗ | ✗ | ✗ | ✓ |
| VirtualMachineGroupPublishRequest | ✗ | ✗ | ✗ | ✗ | ✓ |

---

## Task 1 — Strip controller-runtime from `api/`

### 1a. Spoke versions (v1alpha1–v1alpha5)

For every file matching `api/v1alphaN/*_conversion.go`:

1. Remove the `ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"` import.
2. Delete all `ConvertTo(dstRaw ctrlconversion.Hub) error` method implementations.
3. Delete all `ConvertFrom(srcRaw ctrlconversion.Hub) error` method implementations.
4. Move the `restore_*` helper functions used by ConvertTo/ConvertFrom to `webhooks/conversion/v1alpha6/` (Task 2), applying the naming rules in that task.
5. Remove the `"slices"` import from `api/v1alpha1/`, `api/v1alpha2/`, `api/v1alpha3/`, and `api/v1alpha4/` `virtualmachine_conversion.go`. In every case `"slices"` is used only inside `restore_v1alpha6_VirtualMachinePolicies`, which moves to the root module in step 4. `api/v1alpha5/virtualmachine_conversion.go` does not import `"slices"` and is unaffected.

### 1b. Hub version (v1alpha6)

For every file matching `api/v1alpha6/*_conversion.go`, delete **all** `Hub()` marker methods. Note that each file declares `Hub()` on both the singular type **and** the corresponding List type (e.g., `func (*VirtualMachine) Hub() {}` and `func (*VirtualMachineList) Hub() {}`). Both must be removed.

After deletion, any file that contains only a copyright header and `package v1alpha6` declaration must be **deleted entirely** rather than left as an empty shell.

### 1c. Update `api/go.mod`

Remove `sigs.k8s.io/controller-runtime` and all indirect dependencies it was pulling in that are no longer reachable. Run `go mod tidy` inside `api/` to verify and clean `go.sum`.

---

## Task 2 — Implement converter functions in the root module

### 2a. Add import alias to `.golangci.yml`

The new converter files import `sigs.k8s.io/controller-runtime/pkg/webhook/conversion`. This package has no alias in `.golangci.yml` `linters.settings.importas.alias` yet. Add the entry before writing any code, as the `importas` linter enforces it on all source files:

```yaml
- pkg: sigs.k8s.io/controller-runtime/pkg/webhook/conversion
  alias: whconversion
```

### 2b. Naming convention for moved restore helpers

There are **24 distinct `restore_` function names** that appear in two or more spoke versions and will collide if moved into the same package unchanged. Derive the full list before starting by running:

```
grep -rh "^func restore_" api/v1alpha*/ --include="*.go" | sed 's/(.*$//' | sort | uniq -c | sort -rn
```

For each colliding name, compare the function bodies across versions:

- **Identical body across all versions** (e.g., `restore_v1alpha6_VirtualMachineResources` is `dst.Spec.Resources = src.Spec.Resources` in all five versions) → use a **single shared helper** with no version suffix in `webhooks/conversion/v1alpha6/`. This avoids needless code explosion.
- **Divergent body** across versions (e.g., `restore_v1alpha6_VirtualMachineAdvanced` has a different set of fields restored in v1alpha2 vs v1alpha3 vs v1alpha4 vs v1alpha5) → rename each with a **version suffix** when moved: `restoreV1Alpha2VirtualMachineAdvanced`, `restoreV1Alpha3VirtualMachineAdvanced`, etc.

### 2c. Converter file pattern

Create one file per resource kind inside `webhooks/conversion/v1alpha6/`, named `convert_<kind>.go`. Each file defines:

1. A **package-level variable** of type `func(*runtime.Scheme) (whconversion.Converter, error)` — the return type of `NewHubSpokeConverter` — assembling all spoke converters for that resource kind per the matrix above.
2. The **typed converter functions** for every applicable spoke version (one hub-to-spoke and one spoke-to-hub per version), calling the existing `Convert_xxx_To_yyy` functions from `api/`.

**Simple type (no marshal/restore logic):**

```go
// webhooks/conversion/v1alpha6/convert_virtualmachineclass.go

var VirtualMachineClass = whconversion.NewHubSpokeConverter(
    &vmopv1.VirtualMachineClass{},
    whconversion.NewSpokeConverter(&vmopv1a1.VirtualMachineClass{}, convertVMClassHubToV1Alpha1, convertVMClassV1Alpha1ToHub),
    whconversion.NewSpokeConverter(&vmopv1a2.VirtualMachineClass{}, convertVMClassHubToV1Alpha2, convertVMClassV1Alpha2ToHub),
    whconversion.NewSpokeConverter(&vmopv1a3.VirtualMachineClass{}, convertVMClassHubToV1Alpha3, convertVMClassV1Alpha3ToHub),
    whconversion.NewSpokeConverter(&vmopv1a4.VirtualMachineClass{}, convertVMClassHubToV1Alpha4, convertVMClassV1Alpha4ToHub),
    whconversion.NewSpokeConverter(&vmopv1a5.VirtualMachineClass{}, convertVMClassHubToV1Alpha5, convertVMClassV1Alpha5ToHub),
)

func convertVMClassHubToV1Alpha2(_ context.Context, hub *vmopv1.VirtualMachineClass, spoke *vmopv1a2.VirtualMachineClass) error {
    return vmopv1a2.Convert_v1alpha6_VirtualMachineClass_To_v1alpha2_VirtualMachineClass(hub, spoke, nil)
}

func convertVMClassV1Alpha2ToHub(_ context.Context, spoke *vmopv1a2.VirtualMachineClass, hub *vmopv1.VirtualMachineClass) error {
    return vmopv1a2.Convert_v1alpha2_VirtualMachineClass_To_v1alpha6_VirtualMachineClass(spoke, hub, nil)
}
// ... repeat for other versions
```

**Complex type (with marshal/restore logic):**

The spoke-to-hub direction handles `UnmarshalData` and restore (equivalent to the existing `ConvertTo`). The hub-to-spoke direction handles `MarshalData` (equivalent to `ConvertFrom`). `dst.Status = restored.Status` at the end of spoke-to-hub is taken verbatim from the existing `ConvertTo` implementations (confirmed at `api/v1alpha2/virtualmachine_conversion.go:591` and `api/v1alpha3/virtualmachine_conversion.go:529`).

```go
// webhooks/conversion/v1alpha6/convert_virtualmachine.go

var VirtualMachine = whconversion.NewHubSpokeConverter(
    &vmopv1.VirtualMachine{},
    whconversion.NewSpokeConverter(&vmopv1a1.VirtualMachine{}, convertVMHubToV1Alpha1, convertVMV1Alpha1ToHub),
    whconversion.NewSpokeConverter(&vmopv1a2.VirtualMachine{}, convertVMHubToV1Alpha2, convertVMV1Alpha2ToHub),
    // ... v1alpha3, v1alpha4, v1alpha5
)

// convertVMHubToV1Alpha2 is the ConvertFrom equivalent: hub → spoke.
func convertVMHubToV1Alpha2(_ context.Context, hub *vmopv1.VirtualMachine, spoke *vmopv1a2.VirtualMachine) error {
    if err := vmopv1a2.Convert_v1alpha6_VirtualMachine_To_v1alpha2_VirtualMachine(hub, spoke, nil); err != nil {
        return err
    }
    return utilconversion.MarshalData(hub, spoke)
}

// convertVMV1Alpha2ToHub is the ConvertTo equivalent: spoke → hub.
func convertVMV1Alpha2ToHub(_ context.Context, spoke *vmopv1a2.VirtualMachine, hub *vmopv1.VirtualMachine) error {
    if err := vmopv1a2.Convert_v1alpha2_VirtualMachine_To_v1alpha6_VirtualMachine(spoke, hub, nil); err != nil {
        return err
    }
    restored := &vmopv1.VirtualMachine{}
    if ok, err := utilconversion.UnmarshalData(spoke, restored); err != nil || !ok {
        return err
    }
    restoreV1Alpha2VirtualMachineImage(hub, restored)
    // ... all restore calls, version-suffixed or shared per naming rules
    hub.Status = restored.Status  // taken verbatim from existing ConvertTo
    return nil
}
```

### 2d. Note on list types

`NewHubSpokeConverter` uses `scheme.ObjectKinds()` and filters by `GroupKind()`. A `VirtualMachineList` has `Kind = "VirtualMachineList"`, giving it a different `GroupKind` from `VirtualMachine`, so list types are not enumerated and require no separate spoke converters.

---

## Task 3 — Consolidate webhook conversion registrations

### 3a+3b. Delete per-spoke webhook directories and update the top-level aggregator (atomic)

Delete `webhooks/conversion/v1alpha1/` through `webhooks/conversion/v1alpha5/` **in the same change** as removing their `AddToManager` calls from `webhooks/conversion/webhooks.go`. These two steps must be done together to avoid a compile error from dangling imports.

### 3c. Update `webhooks/conversion/v1alpha6/webhooks.go`

For each resource kind, switch from `.Complete()` to `.WithConverter(<var>).Complete()`, referencing the package-level variable from Task 2. The spoke version coverage is encoded in the variable definition, so the registration file needs no per-kind version logic.

**Before:**

```go
ctrl.NewWebhookManagedBy(mgr, &vmopv1.VirtualMachine{}).Complete()
```

**After:**

```go
ctrl.NewWebhookManagedBy(mgr, &vmopv1.VirtualMachine{}).
    WithConverter(VirtualMachine).
    Complete()
```

**Feature-gated type — `VirtualMachineClassInstance`:** The existing `pkgcfg.FromContext(ctx).Features.ImmutableClasses` guard around the `VirtualMachineClassInstance` registration **must be preserved**. The `.WithConverter(VirtualMachineClassInstance).Complete()` call must remain inside that guard, matching the behavior of the v1alpha4 and v1alpha5 per-spoke webhooks it replaces. `VirtualMachineClassInstance` is unconditionally registered in the scheme, so omitting the gate would silently enable a conversion handler for a feature-gated type.

---

## Task 4 — Migrate `api/test/` conversion tests

### 4a. Affected files

The following 17 test files in `api/test/` import `sigs.k8s.io/controller-runtime/pkg/conversion` and call `.ConvertTo()`/`.ConvertFrom()` directly on typed spoke objects (108 call sites total):

```
api/test/v1alpha1/virtualmachine_conversion_test.go
api/test/v1alpha1/virtualmachineclass_conversion_test.go
api/test/v1alpha1/virtualmachineimage_conversion_test.go
api/test/v1alpha1/virtualmachinepublishrequest_conversion_test.go
api/test/v1alpha1/virtualmachineservice_conversion_test.go
api/test/v1alpha2/virtualmachine_conversion_test.go
api/test/v1alpha2/virtualmachineclass_conversion_test.go
api/test/v1alpha2/virtualmachinepublishrequest_conversion_test.go
api/test/v1alpha2/virtualmachineservice_conversion_test.go
api/test/v1alpha3/virtualmachine_conversion_test.go
api/test/v1alpha3/virtualmachinepublishrequest_conversion_test.go
api/test/v1alpha3/virtualmachineservice_conversion_test.go
api/test/v1alpha4/virtualmachine_conversion_test.go
api/test/v1alpha4/virtualmachinepublishrequest_conversion_test.go
api/test/v1alpha4/virtualmachineservice_conversion_test.go
api/test/v1alpha5/virtualmachine_conversion_test.go
api/test/v1alpha5/virtualmachineservice_conversion_test.go
```

### 4b. Migration approach per test type

**Tests that verify the full round-trip pipeline** (calling `spoke.ConvertTo(hub)` / `spoke.ConvertFrom(hub)` to exercise the MarshalData/UnmarshalData/restore connector layer) → **move to the root module** as `webhooks/conversion/v1alpha6/convert_<kind>_test.go`. In the root module, these tests call the new typed wrapper functions (`convertVMV1Alpha2ToHub`, `convertVMHubToV1Alpha2`, etc.) directly. This migration also satisfies Finding 7 — it is the mechanism for testing the connector layer.

**Tests that verify only field-level conversion** (calling `spoke.ConvertTo(hub)` only to exercise a `Convert_xxx_To_yyy` function, with no assertion on MarshalData behavior) → **adapt in-place** inside `api/test/`. Replace `spoke.ConvertTo(&hub)` with the direct call `vmopv1aN.Convert_v1alphaN_Type_To_v1alpha6_Type(spoke, hub, nil)`, and `spoke.ConvertFrom(hub)` with `vmopv1aN.Convert_v1alpha6_Type_To_v1alphaN_Type(hub, spoke, nil)`. Remove the `ctrlconversion` import from those files after replacement.

An implementer should classify each call site by inspection before making changes. The majority of the `virtualmachine_conversion_test.go` sites are round-trip tests and belong in the root module; the `virtualmachineclass_conversion_test.go` and similar simpler-type tests are likely field-level only.

### 4c. Redesign the fuzz harness

`api/test/utilconversion/fuzztests/conversion_fuzz.go` is built entirely around `ctrlconversion.Convertible`. Replace the interface-parameterized `FuzzTestFuncInput` with a typed function-pair struct:

```go
type FuzzTestFuncInput[Hub, Spoke any] struct {
    Hub        Hub
    Spoke      Spoke
    SpokeToHub func(spoke Spoke, hub Hub) error
    HubToSpoke func(hub Hub, spoke Spoke) error
    ...
}
```

Each per-spoke fuzz caller passes the corresponding `Convert_xxx_To_yyy` pair from `api/v1alphaN/` directly. The `conversion_test.go` suite files in `api/test/v1alphaN/` are updated to match.

### 4d. Clean up `api/test/go.mod`

After steps 4a–4c, `sigs.k8s.io/controller-runtime` is no longer imported anywhere in `api/test/`. Run `go mod tidy` inside `api/test/` to remove it and clean `go.sum`. This completes the goal of removing controller-runtime from the entire `api/` sub-tree.

---

## What Does NOT Change

| Artifact | Reason |
|---|---|
| `Convert_vNalphaX_Type_To_vMalphaY_Type()` functions in `api/` | Only depend on `k8s.io/apimachinery`; stay exactly as-is |
| `zz_generated.conversion.go` files in `api/` | Auto-generated, no controller-runtime imports |
| `api/utilconversion/` package | No controller-runtime dependency; stays in `api/` |
| Scheme registration (`RegisterConversions`) | Stays in `api/`; only uses `k8s.io/apimachinery/pkg/runtime` |
| Root `go.mod` | No new dependencies; root module already imports both `api/` and `controller-runtime v0.24.0` |
| Webhook TLS, certificate management, routing | Unchanged |
| E2E tests | Conversion is an internal implementation detail; observable behavior is identical |
