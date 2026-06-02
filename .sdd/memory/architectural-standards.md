# VM Operator Architectural Standards

> **Linter-enforced rules.** A number of standards in this document — package
> aliases, import grouping, and forbidden imports — are also enforced by
> `golangci-lint` via [`.golangci.yml`](../../.golangci.yml). When a section
> below points at a specific `.golangci.yml` key, treat that key as the
> source of truth and update this document if the linter changes. Run
> `make lint-go` (or `golangci-lint run` / `golangci-lint fmt`) to apply.

## Project Structure

```
vm-operator/
├── api/              # CRD type definitions (one directory per alpha version).
│   └── v1alphaN/     # The active alpha is whichever directory is aliased
│                     # as `vmopv1` in `.golangci.yml` importas (see below).
├── controllers/      # Reconciliation logic for each CRD
├── webhooks/         # Admission webhooks (validation/ and mutation/ subdirs)
├── pkg/              # Reusable packages (business logic, utilities)
│   ├── conditions/   # Kubernetes condition management (Getter/Setter)
│   ├── config/       # Feature flags and configuration (pkgcfg)
│   ├── context/      # Typed context structs per resource
│   ├── constants/    # Shared constants and test labels
│   ├── log/          # Structured logging helpers
│   ├── patch/        # Strategic merge patch helpers for status updates
│   ├── providers/    # Provider interface + vsphere implementation
│   ├── record/       # Event recording helpers
│   ├── util/         # General-purpose utilities (kube, ptr, vmopv1)
│   └── vmconfig/     # VM configuration reconcilers (crypto, bootoptions, diskpromo)
├── external/         # Vendored external API types (byok, ncp, capabilities, etc.)
├── test/             # Test utilities
│   └── builder/      # Test context builders (unit, intg, vcsim)
├── config/           # Kubernetes manifests and RBAC
└── hack/             # Build and development scripts
```

## Package Organization Principles

### Keep Controllers Thin

- Controllers MUST delegate business logic to `pkg/` packages
- Controllers handle: reconcile loop, patch helpers, finalizers, watches
- Controllers MUST NOT contain complex validation, vSphere API calls, or state computation
- Example: `controllers/virtualmachine/` delegates to `pkg/providers/vsphere/`, `pkg/vmconfig/`

### Package Alias Conventions

The full list of canonical import aliases is **enforced by `golangci-lint`** via the `importas` linter. The authoritative table lives in [`.golangci.yml`](../../.golangci.yml) under `linters.settings.importas.alias` — it covers `corev1`, `metav1`, `apierrors`, `ctrl`, `ctrlclient`, `ctrlmgr`, `vim25`, `vimtypes`, `pbmtypes`, every `vmopv1aN` (and the current `vmopv1`), `vmopv1util`, the external CRD APIs (`byokv1`, `capv1`, `appv1a1`, `vspherepolv1`, `infrav1`), and the `pkg*` family (`pkgcfg`, `pkgctx`, `pkgerr`, `pkglog`, `pkgmgr`, `pkgutil`, `pkgexit`, `pkgcrd`, `pkgnil`, `ctxop`, `proberctx`, `clsutil`, `proxyaddr`, `backupapi`, `vmopapi`).

If you introduce a new widely-imported package, add its alias to `.golangci.yml` `linters.settings.importas.alias` in the same change set so the convention is enforced from day one rather than relying on this document to track it.

## Naming Conventions

### Files

- Controller files: `<resource>_controller.go`
- Test files: `<resource>_controller_unit_test.go`, `<resource>_controller_intg_test.go`
- Suite files: `<resource>_controller_suite_test.go`
- Webhook validators: `<resource>_validator.go`
- Webhook mutators: `<resource>_mutator.go`
- Typed contexts: `<resource>_context.go` in `pkg/context/`

### Types

- Reconciler structs: `type Reconciler struct`
- Context types: `<Resource>Context` (e.g., `VirtualMachineContext`, `VirtualMachineSetResourcePolicyContext`)
- Webhook validators: `type validator struct`

### Receiver Names

- Use single-letter receivers: `func (r *Reconciler)`, `func (v *validator)`

### Constants

- Finalizer names: `vmoperator.vmware.com/<resource>` (e.g., `vmoperator.vmware.com/virtualmachine`)
- Label keys: `<domain>/<key>` format

## Import Organization

Imports MUST be organized into groups separated by a blank line, in this order:

1. Standard library.
2. Third-party packages (e.g. `github.com/go-logr/logr`, `github.com/vmware/govmomi/...`).
3. Kubernetes packages (`k8s.io/...`, `sigs.k8s.io/controller-runtime/...`).
4. External vendored APIs that this repo consumes (e.g. `github.com/vmware-tanzu/nsx-operator/...`).
5. Internal packages — `github.com/vmware-tanzu/vm-operator/api/...` first, then `pkg/...`, `controllers/...`, `webhooks/...`, etc.

The split between third-party and VMware/internal imports is **enforced by `goimports`** through [`.golangci.yml`](../../.golangci.yml) `formatters.settings.goimports.local-prefixes`, which pins `github.com/vmware`, `github.com/vmware-tanzu`, and `github.com/vmware-tanzu/vm-operator` to the local-prefix group. Running `make lint-go` (or `golangci-lint fmt`) reorders imports to match.

The aliases used inside each group come from `.golangci.yml` `linters.settings.importas.alias`; see "Package Alias Conventions" above.

Example shape (aliases shown are illustrative — `vmopv1` always points at whichever alpha version `importas` declares as current):

```go
import (
    "context"
    "fmt"

    "github.com/go-logr/logr"
    vimtypes "github.com/vmware/govmomi/vim25/types"

    corev1 "k8s.io/api/core/v1"
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    ctrl "sigs.k8s.io/controller-runtime"
    ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

    vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"

    vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alphaN"
    pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
    pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)
```

## Error Handling

### Error Wrapping

Always wrap errors with context using `fmt.Errorf`:

```go
// Good
return fmt.Errorf("failed to get VM properties for %s: %w", vmID, err)

// Bad - no context
return err
```

### Condition-Based Errors

Use `conditions.MarkFalse()` to record failure reasons with constants from the API package:

```go
if err != nil {
    conditions.MarkFalse(ctx.VM,
        vmopv1.VirtualMachineConditionCreated,
        "CreateError",
        "%v", err)
    return err
}
```

## Context Usage

- Always pass `context.Context` as the first parameter
- Use typed context structs from `pkg/context/` for domain-specific data
- Never store `context.Context` in structs (except the long-lived controller-manager context)
- Access logger via `ctx.Logger`
- Feature flags via `pkgcfg.FromContext(ctx)`

## Copyright Header

All Go files MUST have this header:

```go
// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0
```

## Code Comments and Documentation

### Comment Grammar

All comments MUST follow proper grammar rules:

```go
// Good - proper sentence structure with period
// ErrorMessageFromTaskInfo extracts a comprehensive error message from a TaskInfo.
// It combines the localized message with all fault messages to provide complete
// error details.

// Bad - missing periods
// ErrorMessageFromTaskInfo extracts a comprehensive error message from a TaskInfo
// It combines the localized message with all fault messages to provide complete
// error details

// Good - inline comments end with period
// No fault messages, return the localized message if available.
if taskErr.LocalizedMessage != "" {
    return taskErr.LocalizedMessage
}

// Bad - inline comments missing period
// No fault messages, return the localized message if available
if taskErr.LocalizedMessage != "" {
    return taskErr.LocalizedMessage
}
```

### Function Documentation

Public functions and methods MUST have godoc comments that:
- Start with the function name
- Use complete sentences with proper punctuation
- Explain what the function does, not how
- Include examples for complex functions

```go
// Good
// GetVirtualMachine retrieves a VirtualMachine resource from the Kubernetes API.
// It returns an error if the resource is not found or if the API call fails.
func GetVirtualMachine(ctx context.Context, name string) (*vmopv1.VirtualMachine, error)

// Bad - no period, unclear
// GetVirtualMachine gets a vm
func GetVirtualMachine(ctx context.Context, name string) (*vmopv1.VirtualMachine, error)
```

## Anti-Patterns to Avoid

1. **Leaking business logic into controllers** - Controllers orchestrate; `pkg/` computes
2. **Using raw strings for condition types/reasons** - Use constants from the active API version (whatever `vmopv1` points at in `.golangci.yml` `linters.settings.importas.alias`)
3. **Creating circular imports** - Keep `pkg/` independent of `controllers/`
4. **Skipping error wrapping** - Always add context to errors
5. **Using `interface{}` without type assertion** - Prefer generics or typed interfaces
6. **Ignoring feature flags** - Check `pkgcfg.FromContext(ctx).Features.*` before feature-gated logic
