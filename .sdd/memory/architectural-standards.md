# VM Operator Architectural Standards

> **Linter-enforced rules.** Package aliases, import grouping, and forbidden imports are enforced by `golangci-lint` via [`.golangci.yml`](../../.golangci.yml), which is the source of truth for them. Run `make lint-go` (or `golangci-lint fmt`) to apply. The project layout and Go modules are described in the constitution.

## Package Organization

- Controllers handle only the reconcile loop, patch helpers, finalizers, and watches; they MUST delegate business logic to `pkg/` and MUST NOT contain complex validation, vSphere API calls, or state computation. Example: `controllers/virtualmachine/` delegates to `pkg/providers/vsphere/` and `pkg/vmconfig/`.
- Keep `pkg/` independent of `controllers/` — no circular imports.

## Package Alias Conventions

Canonical import aliases (`corev1`, `metav1`, `apierrors`, `ctrl`, `ctrlclient`, `vimtypes`, every `vmopv1aN` and the current `vmopv1`, `pkgcfg`, `pkgctx`, `pkgerr`, `pkglog`, …) are defined in `.golangci.yml` `linters.settings.importas.alias`. `vmopv1` always points at whichever alpha version that table declares as current. If you introduce a new widely-imported package, add its alias there in the same change set.

## Naming Conventions

- Files: `<resource>_controller.go`, `<resource>_validator.go`, `<resource>_mutator.go`, typed contexts in `pkg/context/<resource>_context.go`. Test files follow the constitution: `<package>_test.go` plus `<package>_suite_test.go` (not the legacy `_unit_test.go` / `_intg_test.go` split).
- Types: `type Reconciler struct` for reconcilers, `type validator struct` for webhook validators, `<Resource>Context` for typed contexts (e.g. `VirtualMachineContext`).
- Receivers: single letter — `func (r *Reconciler)`, `func (v *validator)`.
- Finalizers: `vmoperator.vmware.com/<resource>` (e.g. `vmoperator.vmware.com/virtualmachine`). Label keys: `<domain>/<key>`.

## Import Organization

Imports are organized into groups separated by a blank line, in this order:

1. Standard library.
2. Third-party packages, including govmomi, `k8s.io/...`, and `sigs.k8s.io/...`, in a single group.
3. Other `github.com/vmware-tanzu/...` modules this repo consumes (e.g. `nsx-operator`, `net-operator-api`).
4. Internal `github.com/vmware-tanzu/vm-operator/...` packages.

`goimports` produces this layout from `.golangci.yml` `formatters.settings.goimports.local-prefixes`, so `make lint-go` / `golangci-lint fmt` reorders imports to match. In test files the Ginkgo/Gomega dot imports may form their own group after the standard library.

## Error Handling

- Always wrap errors with context: `fmt.Errorf("failed to get VM properties for %s: %w", vmID, err)`, never a bare `return err`.
- Record failure reasons on conditions using constants from the active API version, not raw strings:

```go
conditions.MarkFalse(ctx.VM,
    vmopv1.VirtualMachineConditionCreated,
    "CreateError",
    "%v", err)
```

## Context Usage

- Always pass `context.Context` as the first parameter.
- Use typed context structs from `pkg/context/` for domain-specific data.
- Never store `context.Context` in structs (except the long-lived controller-manager context).
- Access the logger via `ctx.Logger`.
- Read feature flags via `pkgcfg.FromContext(ctx)`, and check `pkgcfg.FromContext(ctx).Features.*` before any feature-gated logic.

## Copyright Header

All Go files MUST have this header:

```go
// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0
```

## Code Comments and Documentation

- All comments, including inline ones, are complete sentences with proper grammar and a terminating period.
- Public functions and methods MUST have godoc comments that start with the identifier name and explain what the function does (not how). Include examples for complex functions.

```go
// GetVirtualMachine retrieves a VirtualMachine resource from the Kubernetes
// API. It returns an error if the resource is not found or the call fails.
func GetVirtualMachine(ctx context.Context, name string) (*vmopv1.VirtualMachine, error)
```

## Anti-Patterns to Avoid

1. **Using `interface{}` without type assertion** — prefer generics or typed interfaces.
2. **Ignoring feature flags** — new behavior for an in-progress spec is gated on `pkgcfg.FromContext(ctx).Features.*`.
