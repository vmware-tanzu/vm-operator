# Tasks: Move Conversion to a Separate Module

Reference: [`plan.md`](./plan.md) — read it before implementing any story here. The plan contains the full converter matrix, naming rules for colliding restore helpers, code patterns for simple vs. complex types, and the atomicity constraint that governs Change 2.

## Story ordering

```
Change 1 (additive)  →  Change 2 (atomic removal)  →  Change 3 (test cleanup)
```

Each change is a single compilable, reviewable PR. Changes 2 and 3 have hard compile-time dependencies on the previous change and must not be merged out of order.

---

## Change 1 — Implement converter functions in the root module

**Summary:** Create the new converter infrastructure inside the root module without touching `api/`. This change is purely additive: the root module compiles before and after; the `api/` sub-module is completely unchanged.

### Why this change can land independently

After this change, `api/v1alpha1`–`v1alpha5` spoke types still implement `ctrlconversion.Convertible` (their `ConvertTo`/`ConvertFrom` methods are still present). The new converter variables in `webhooks/conversion/v1alpha6/` are defined and tested but not yet wired into the webhook registrar. The binary behaviour is identical to `main`.

### Sub-tasks

#### 1.1 — Add `whconversion` import alias to `.golangci.yml`

**Files touched:** `.golangci.yml`

Add the following entry under `linters.settings.importas.alias` before writing any code that imports the package, so that the `importas` linter enforces the alias from the first commit:

```yaml
- pkg: sigs.k8s.io/controller-runtime/pkg/webhook/conversion
  alias: whconversion
```

**Acceptance criteria:**
- The alias entry is present in `.golangci.yml`.
- `make lint-go` passes on a no-change tree (verifies the entry is syntactically valid).

#### 1.2 — Create `convert_<kind>.go` files in `webhooks/conversion/v1alpha6/`

**Files touched (new):**
```
webhooks/conversion/v1alpha6/convert_virtualmachine.go
webhooks/conversion/v1alpha6/convert_virtualmachineclass.go
webhooks/conversion/v1alpha6/convert_virtualmachineimage.go
webhooks/conversion/v1alpha6/convert_clustervirtualmachineimage.go
webhooks/conversion/v1alpha6/convert_virtualmachinepublishrequest.go
webhooks/conversion/v1alpha6/convert_virtualmachineservice.go
webhooks/conversion/v1alpha6/convert_virtualmachinesetresourcepolicy.go
webhooks/conversion/v1alpha6/convert_virtualmachinegroup.go
webhooks/conversion/v1alpha6/convert_virtualmachinewebconsolerequest.go
webhooks/conversion/v1alpha6/convert_virtualmachineimagecache.go
webhooks/conversion/v1alpha6/convert_virtualmachinereplicaset.go
webhooks/conversion/v1alpha6/convert_virtualmachineclassinstance.go
webhooks/conversion/v1alpha6/convert_virtualmachinesnapshot.go
webhooks/conversion/v1alpha6/convert_virtualmachinegrouppublishrequest.go
```

For each file, follow the patterns in `plan.md` § "Task 2 — Implement converter functions":

- Declare a **package-level exported variable** whose type satisfies `whconversion.Converter`. Use `whconversion.NewHubSpokeConverter` with one `whconversion.NewSpokeConverter` per version row in the matrix (see `plan.md` § "Spoke Version Matrix").
- Declare **typed converter functions** (one hub-to-spoke and one spoke-to-hub per applicable version). For simple types (no MarshalData), call the `Convert_xxx_To_yyy` function from the respective `api/v1alphaN/` package directly. For complex types (`VirtualMachine`, and any other type that currently uses `MarshalData`/`UnmarshalData`), follow the `convertVMHubToV1AlphaN` / `convertVMV1AlphaNToHub` pattern shown in `plan.md`.
- Move all `restore_*` helpers from `api/v1alphaN/virtualmachine_conversion.go` into the appropriate file here, applying the naming rules from `plan.md` § "Task 2b": identical-body helpers across all versions become a single shared helper; helpers with divergent bodies are renamed with a version suffix (e.g., `restoreV1Alpha2VirtualMachineAdvanced`). Derive the full set of 24 colliding names by running `grep -rh "^func restore_" api/v1alpha*/ --include="*.go" | sed 's/(.*$//' | sort | uniq -c | sort -rn` before starting.

**Acceptance criteria:**
- All 14 converter files exist.
- Each exported converter variable references the correct set of spoke versions per the matrix in `plan.md`.
- All `restore_*` helpers have been moved and renamed per the collision rules; no two functions in the package share the same name.
- `go build ./webhooks/...` passes.
- `make lint-go` passes (no import alias violations, no unused imports).
- The `api/` sub-module is **not modified**.

#### 1.3 — Create converter tests in `webhooks/conversion/v1alpha6/`

**Files touched (new):**
```
webhooks/conversion/v1alpha6/convert_virtualmachine_test.go
webhooks/conversion/v1alpha6/convert_virtualmachineclass_test.go
```
(At minimum these two; add more for other complex kinds as appropriate.)

These test files live in the root module and can import both `api/v1alphaN` types and the typed converter functions from the same package (`webhooks/conversion/v1alpha6`). They replace the full round-trip tests that currently live in `api/test/v1alphaN/` and will be removed in Change 3.

Each test file should include:
- A spoke-to-hub round-trip test: build a spoke object with all fields populated, call `convertVM<Kind>V1AlphaNToHub`, verify hub fields match expectations, call `convertVM<Kind>HubToV1AlphaN`, verify the spoke round-trips correctly.
- A test that verifies `MarshalData`/`UnmarshalData` preserves hub-only fields that have no counterpart in the spoke schema.

Follow the Ginkgo/Gomega testing conventions in `.sdd/memory/testing-standards.md` (external `_test` package, `Label()` decorators from `pkg/constants/testlabels`).

**Acceptance criteria:**
- Tests exist for at least `VirtualMachine` (complex, has restore logic) and `VirtualMachineClass` (simple, no restore logic).
- `go test ./webhooks/conversion/v1alpha6/...` passes.
- Tests use `testlabels.API` label (no `testlabels.EnvTest` — these are pure unit tests).

---

## Change 2 — Remove conversion interfaces from `api/` (atomic)

**Summary:** Strip `ConvertTo`/`ConvertFrom` methods and `Hub()` markers from `api/`, delete the now-redundant per-spoke webhook packages, wire up `WithConverter`, and remove controller-runtime from `api/go.mod`.

**Prerequisite:** Change 1 must be merged. The converter variables and typed functions defined in Change 1 are what make `webhooks/conversion/v1alpha6/webhooks.go` compilable after the old per-spoke webhooks are deleted.

### Why this is atomic

After the `ConvertTo`/`ConvertFrom` methods are removed from spoke types, controller-runtime's `IsConvertible()` check at `Complete()` time returns `PartialImplementationError` for any hub type that does not have `WithConverter` registered. All sub-tasks below must therefore land in a single commit (or squashed PR) to avoid a startup-time panic in any intermediate state.

### Sub-tasks

#### 2.1 — Remove `ConvertTo`/`ConvertFrom` methods from spoke types (v1alpha1–v1alpha5)

**Files touched:**
All `api/v1alphaN/*_conversion.go` files for N ∈ {1, 2, 3, 4, 5}.

Steps per file:
1. Delete the `ConvertTo(dstRaw ctrlconversion.Hub) error` method body.
2. Delete the `ConvertFrom(srcRaw ctrlconversion.Hub) error` method body.
3. Remove the `ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"` import.
4. The `restore_*` helpers were moved to `webhooks/conversion/v1alpha6/` in Change 1 sub-task 1.2; delete the originals from `api/v1alphaN/` now.
5. Remove the `"slices"` import from `api/v1alpha1/`, `api/v1alpha2/`, `api/v1alpha3/`, and `api/v1alpha4/` `virtualmachine_conversion.go`. (`api/v1alpha5/virtualmachine_conversion.go` does not import `"slices"` and is unaffected.)

**Acceptance criteria:**
- No `ConvertTo`/`ConvertFrom` methods remain in any file under `api/v1alpha1/` through `api/v1alpha5/`.
- No `ctrlconversion` import remains in any `api/v1alphaN/*_conversion.go` file.
- No `"slices"` import remains in `api/v1alpha1–4/virtualmachine_conversion.go`.
- No `restore_*` function remains under `api/` (all moved to `webhooks/conversion/v1alpha6/` in Change 1).

#### 2.2 — Remove `Hub()` markers from v1alpha6 and delete empty files

**Files touched:**
All `api/v1alpha6/*_conversion.go` files.

Steps:
1. Delete every `func (*TypeName) Hub() {}` and `func (*TypeNameList) Hub() {}` declaration from each file. Both the singular type and the corresponding List type have a `Hub()` method — remove both.
2. After deletion, any file that contains only the copyright header and a bare `package v1alpha6` declaration **must be deleted entirely**.

**Acceptance criteria:**
- No `Hub()` method remains under `api/v1alpha6/`.
- No file under `api/v1alpha6/` contains only a copyright header and package declaration.

#### 2.3 — Update `api/go.mod`

**Files touched:** `api/go.mod`, `api/go.sum`

Remove `sigs.k8s.io/controller-runtime` from `api/go.mod`. Run `go mod tidy` inside `api/` to remove the entry and clean up `go.sum`.

**Acceptance criteria:**
- `api/go.mod` contains no reference to `sigs.k8s.io/controller-runtime`.
- `go build ./...` inside `api/` passes.
- `go mod tidy` inside `api/` produces no diff.

#### 2.4 — Delete per-spoke webhook packages (v1alpha1–v1alpha5) and update the aggregator (atomic with 2.4)

**Files touched:**
```
webhooks/conversion/v1alpha1/   — delete entire directory
webhooks/conversion/v1alpha2/   — delete entire directory
webhooks/conversion/v1alpha3/   — delete entire directory
webhooks/conversion/v1alpha4/   — delete entire directory
webhooks/conversion/v1alpha5/   — delete entire directory
webhooks/conversion/webhooks.go — remove AddToManager calls for v1alpha1–5
```

Delete the five directories and remove their `AddToManager` invocations from `webhooks/conversion/webhooks.go` in the same operation. Leaving dangling imports in `webhooks.go` while the directories are gone produces a compile error.

**Acceptance criteria:**
- Directories `webhooks/conversion/v1alpha1/` through `webhooks/conversion/v1alpha5/` no longer exist.
- `webhooks/conversion/webhooks.go` no longer imports or calls any per-spoke `AddToManager`.
- `go build ./webhooks/...` passes after this sub-task.

#### 2.5 — Wire `WithConverter` into `webhooks/conversion/v1alpha6/webhooks.go`

**Files touched:** `webhooks/conversion/v1alpha6/webhooks.go`

For each resource kind registration in the file, switch from `.Complete()` to `.WithConverter(<var>).Complete()`, where `<var>` is the package-level converter variable defined in Change 1 sub-task 1.2.

**Special case — `VirtualMachineClassInstance`:** The existing `pkgcfg.FromContext(ctx).Features.ImmutableClasses` guard around the `VirtualMachineClassInstance` registration **must be preserved**. The new form is:

```go
if pkgcfg.FromContext(ctx).Features.ImmutableClasses {
    if err := ctrl.NewWebhookManagedBy(mgr, &vmopv1.VirtualMachineClassInstance{}).
        WithConverter(VirtualMachineClassInstance).
        Complete(); err != nil {
        return err
    }
}
```

**Acceptance criteria:**
- Every `ctrl.NewWebhookManagedBy(mgr, &vmopv1.<Kind>{}).Complete()` call in the file is replaced with `.WithConverter(<var>).Complete()`.
- The `ImmutableClasses` feature gate wraps `VirtualMachineClassInstance` registration exactly as it did before.
- `go build ./...` passes for the root module.
- `make lint-go` passes.

### Change 2 overall acceptance criteria

- `api/go.mod` has no dependency on `sigs.k8s.io/controller-runtime`.
- `go build ./...` passes for the root module.
- `go build ./...` passes inside `api/`.
- `make lint-go` passes for the root module.
- `make test` (unit tests, no vcsim/envtest required) passes for the root module.
- The manager binary starts without errors when conversion webhooks are enabled.

---

## Change 3 — Migrate `api/test/` conversion tests

**Summary:** Fix the 17 `api/test/` files that call `.ConvertTo()`/`.ConvertFrom()` (now removed), redesign the fuzz harness, and remove controller-runtime from `api/test/go.mod`.

**Prerequisite:** Change 2 must be merged. The `ConvertTo`/`ConvertFrom` methods no longer exist on spoke types; any `api/test/` file that calls them fails to compile.

### Affected files

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

### Sub-tasks

#### 3.1 — Migrate full round-trip tests to the root module

**Files touched (in root module, new or extended):**
```
webhooks/conversion/v1alpha6/convert_virtualmachine_test.go   (extend, or create if not done in Change 1)
webhooks/conversion/v1alpha6/convert_virtualmachinepublishrequest_test.go
webhooks/conversion/v1alpha6/convert_virtualmachineservice_test.go
```
(and any other kind whose `api/test/` file exercises `MarshalData`/`UnmarshalData` or restore logic)

**Files touched (in api/test/, deleted or gutted):**
The corresponding `*_conversion_test.go` files that exercised the full pipeline are deleted after their coverage is replicated in the root module tests above.

**Decision rule:** A test exercises the *full round-trip pipeline* if it calls `spoke.ConvertTo(hub)` or `spoke.ConvertFrom(hub)` and then makes assertions about fields that are preserved only via `MarshalData`/`UnmarshalData` (i.e., hub-only fields with no spoke counterpart). Such tests cannot be adapted in-place after `ConvertTo`/`ConvertFrom` are removed; they must move to the root module where the typed wrapper functions are available.

**Acceptance criteria:**
- Every test that previously relied on `MarshalData`/`UnmarshalData` round-trip semantics now exists in `webhooks/conversion/v1alpha6/` and passes.
- No round-trip assertion is silently dropped.

#### 3.2 — Adapt field-level tests in-place

**Files touched (in api/test/, modified):**
Any `*_conversion_test.go` file whose tests only verify that specific fields map correctly between spoke and hub (i.e., they call `spoke.ConvertTo(&hub)` purely to exercise a `Convert_xxx_To_yyy` function, with no assertion dependent on `MarshalData`).

For each such call site, replace:
- `spoke.ConvertTo(&hub)` → `vmopv1aN.Convert_v1alphaN_Type_To_v1alpha6_Type(spoke, &hub, nil)`
- `spoke.ConvertFrom(hub)` → `vmopv1aN.Convert_v1alpha6_Type_To_v1alphaN_Type(hub, spoke, nil)`

Remove the `ctrlconversion` import from any file where all call sites have been replaced.

**Acceptance criteria:**
- No `ctrlconversion` import remains in any `api/test/v1alphaN/` file.
- No `.ConvertTo(` or `.ConvertFrom(` call remains in any file under `api/test/`.
- `go build ./...` inside `api/test/` passes.

#### 3.3 — Redesign the fuzz harness

**Files touched:**
```
api/test/utilconversion/fuzztests/conversion_fuzz.go
api/test/v1alphaN/conversion_test.go  (per-spoke callers of the fuzz harness)
```

Replace the `ctrlconversion.Convertible`-parameterized `FuzzTestFuncInput` struct with a generic function-pair struct:

```go
type FuzzTestFuncInput[Hub, Spoke any] struct {
    Hub        Hub
    Spoke      Spoke
    SpokeToHub func(spoke Spoke, hub Hub) error
    HubToSpoke func(hub Hub, spoke Spoke) error
    // ... existing fields (SkipImmutableFieldValidation etc.) unchanged
}
```

Update `SpokeHubSpoke` and `HubSpokeHub` to call `input.SpokeToHub` / `input.HubToSpoke` instead of `.ConvertTo()` / `.ConvertFrom()`.

Update every per-spoke `conversion_test.go` file in `api/test/v1alphaN/` to pass the appropriate `Convert_xxx_To_yyy` pair from `api/v1alphaN/` as the function arguments.

**Acceptance criteria:**
- `FuzzTestFuncInput` uses no types from `sigs.k8s.io/controller-runtime`.
- `SpokeHubSpoke` and `HubSpokeHub` use the typed function pair, not interface method calls.
- All per-spoke fuzz test callers compile and pass.

#### 3.4 — Remove controller-runtime from `api/test/go.mod`

**Files touched:** `api/test/go.mod`, `api/test/go.sum`

Run `go mod tidy` inside `api/test/`. After sub-tasks 3.1–3.3, `sigs.k8s.io/controller-runtime` should no longer be reachable from any import in the module.

**Acceptance criteria:**
- `api/test/go.mod` contains no reference to `sigs.k8s.io/controller-runtime`.
- `go mod tidy` inside `api/test/` produces no diff.
- `go test ./...` inside `api/test/` passes.

### Change 3 overall acceptance criteria

- No file under `api/test/` imports `sigs.k8s.io/controller-runtime`.
- `api/test/go.mod` has no dependency on `sigs.k8s.io/controller-runtime`.
- `go test ./...` inside `api/test/` passes.
- `go test ./webhooks/conversion/v1alpha6/...` passes in the root module (round-trip coverage moved here in sub-task 3.1).
- `make lint-go` passes for the root module.
