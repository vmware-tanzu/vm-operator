# Constitution: vm-operator Spec-Driven Development

- **Repository**: [`github.com/vmware-tanzu/vm-operator`](https://github.com/vmware-tanzu/vm-operator)
- **Last updated**: 2026-05-27
- **Applies to**: All feature specs under `.sdd/specs/`

---

## Project identity

`vm-operator` (`github.com/vmware-tanzu/vm-operator`) implements VM Operator on vSphere Supervisor. It is a **multi-module** repository: the root `go.mod` is the main binary module and several sub-directories each have their own `go.mod`. All modules share the same git history. See [`architectural-standards.md`](./architectural-standards.md) for the full project structure and module listing.

---

## How to use this constitution

This document defines the **non-negotiables** that govern every change in this repository. Detailed guidance on _how_ to satisfy these non-negotiables lives in companion memory files:

| Topic | Companion file | Cursor rule |
|-------|----------------|-------------|
| Project layout, naming, imports, Go style | [`architectural-standards.md`](./architectural-standards.md) | `.cursor/rules/architectural-standards.mdc` |
| Controller patterns, reconciler structure, error semantics | [`operator-best-practices.md`](./operator-best-practices.md) | `.cursor/rules/operator-best-practices.mdc` |
| Ginkgo/Gomega unit and integration test conventions | [`testing-standards.md`](./testing-standards.md) | `.cursor/rules/testing-standards.mdc` |
| Running and writing E2E tests | [`e2e-testing.md`](./e2e-testing.md) | `.cursor/rules/e2e-testing.mdc` |
| Keeping E2E coverage aligned with product changes | [`e2e-sync-with-changes.md`](./e2e-sync-with-changes.md) | `.cursor/rules/e2e-sync-with-changes.mdc` |
| Spec-Driven Development (SDD) workflow and artifacts | [`sdd-standards.md`](./sdd-standards.md) | _(see below)_ |

When a section below references one of these files, treat the linked file as authoritative for the details. The constitution states **what is required**; the companion files state **how to do it**.

`.cursor/rules/*.mdc` files are thin Cursor proxies — each `.mdc` points at the matching `.md` here so that humans and AI assistants read the same source of truth.

---

## Tickets and WIKI links

Due to Broadcom rules, internal links are disallowed in upstream repositories. To that end, any internal tickets or WIKI links should not be used in this repository:

* Instead, the prefix `vmop` is used in place of our internal JIRA project. Therefore, an internal ticket `XXX-123` becomes `vmop-123`.
* In place of a direct WIKI URL, just reference the page ID with the prefix `WIKI page`, ex. `WIKI page 123456`.

---

## Spec-Driven Development (SDD)

All non-trivial work in this repository follows a Spec-Driven Development workflow rooted at `.sdd/`. The intent is that **specifications drive code**, not the other way around. See [`sdd-standards.md`](./sdd-standards.md) for the full workflow, templates, and artifact contracts.

### Non-negotiables for SDD

- All SDD artifacts live under `.sdd/` at the repository root. There is no other "specs" or "memory" tree.
- Repository-wide rules live in `.sdd/memory/*.md`. Per-feature artifacts live in `.sdd/specs/NNN-slug/`.
- Each feature directory **MUST** contain `spec.md`, `plan.md`, and `tasks.md`. It **SHOULD** contain `research.md`; it **SHOULD** contain `model.md` when the feature introduces or changes a data model / API.
- Feature directory names follow `NNN-slug`, where `NNN` is a zero-padded auto-incrementing scalar starting at `000`. The scalar expands beyond three digits when needed (`1000-...`).
- AI-assistant-specific rule files (e.g. `.cursor/rules/*.mdc`) **MUST** be proxies that link back into `.sdd/memory/*.md`. Do not duplicate guidance across the two trees.
- New features that materially affect product behavior **MUST** ship with their spec, plan, tasks, and (if applicable) model artifacts in the same change set as the code. Code-only PRs that bypass `.sdd/specs/` are reserved for trivial fixes (see [`sdd-standards.md`](./sdd-standards.md) for the exemption list).
- The constitution is **immutable per change**: a PR that needs to bend a constitutional rule must amend the constitution in the same PR with a documented rationale, and call that amendment out in the PR description.
- An SDD spec should be related to an epic ticket.
- All SDD tasks should be related to story or sub-task tickets.

### Expected behavior for new features

1. **Start with the spec.** Create `.sdd/specs/NNN-slug/spec.md` describing user-visible behavior, goals, non-goals, and acceptance criteria. Mark every unknown with `[NEEDS CLARIFICATION: …]` rather than guessing.
2. **Research as needed.** Capture investigation, prior art, and external references in `research.md` so future engineers (and AI assistants) inherit the context.
3. **Plan against the constitution.** `plan.md` translates the spec into a constitutionally-compliant technical approach: project structure, module impacts, API/CRD versioning strategy, test strategy, and any constitution-check exceptions with justification.
4. **Decompose into tasks.** `tasks.md` breaks the plan into ordered, parallelizable, executable tasks. Each task identifies the files it touches and the user story / acceptance criterion it serves.
5. **Implement with tests.** Follow [`testing-standards.md`](./testing-standards.md) and [`e2e-sync-with-changes.md`](./e2e-sync-with-changes.md). E2E coverage ships with the behavior, not after it.
6. **Keep specs alive.** When implementation reveals a wrong assumption, update the spec/plan/tasks in the same PR. Production incidents that invalidate a spec result in a follow-up PR that updates the spec.

See [`sdd-standards.md`](./sdd-standards.md) for templates, the file contracts for each artifact, and the workflow for amendments.

---

## API compatibility

- The default API group for this repository is `vmoperator.vmware.com`. These types are consumed by controllers, webhooks, and external partners, all within this same repo. Breaking changes (field removal, rename, type change) require a version bump and a conversion webhook. Additive changes are *not* safe once an API has shipped due to the way Kubernetes handles `UPDATE` operations. See <https://github.com/kubernetes/kubernetes/issues/111703> for more information on this topic.
- Every new CRD must include `+kubebuilder:object:root=true`, a `+groupversion` doc.go entry, a `// +groupName:` marker, and deepcopy generated via `make generate-go`.
- CRD manifests are generated under `config/crd/`; they are **checked in** (not generated at deploy time) and must be regenerated with `make generate-manifests`.
- There are also some external APIs maintained in this repository:
  - By default these live under the `external/` directory.
  - Each of them belong to their own Go module.
  - Their manifests may be generated with `make generate-external-manifests`.

## Controller design

The non-negotiables below are constitutional; for the canonical reconcile-loop template, watch setup, error semantics, and channel-source patterns see [`operator-best-practices.md`](./operator-best-practices.md).

- Controllers are **thin**: reconcile loops live in `controllers/`; all business logic lives in `pkg/`.
- No controller may call vSphere APIs directly; use the provider abstraction under `pkg/providers/vsphere/`.
- Controllers must track `status.observedGeneration` and set a `Ready` condition.
- Fan-out to child objects uses `controllerutil.CreateOrPatch`; ownership is set via `controllerutil.SetControllerReference` unless the object is cluster-scoped, in which case use labels for ownership tracing.
- Controllers for API groups other than `vmoperator.vmware.com` should not be placed directly in the `controllers/` directory.

## Webhooks

- Admission webhooks live in `webhooks/`; share validation logic with unit tests via unexported validator types (not embedded in the webhook handler).
- `+kubebuilder:webhook` markers drive code generation; registration lives in `webhooks/suite_test.go` and `main.go`.
- CEL validation is preferred for simple structural rules; Go validation for complex, cross-field, or vSphere-data-dependent rules.
- Webhooks for API groups other than `vmoperator.vmware.com` should not be placed directly in the `webhooks/` directory.

## Testing

The non-negotiables below are constitutional; for unit and integration patterns see [`testing-standards.md`](./testing-standards.md), and for the E2E suite see [`e2e-testing.md`](./e2e-testing.md) and [`e2e-sync-with-changes.md`](./e2e-sync-with-changes.md).

- **One test file** per package: `<package>_test.go` (external `_test` package).
- **One suite bootstrap** per package: `<package>_suite_test.go` containing only the `TestXxx(t *testing.T)` entry-point that calls `RunSpecs`.
- Do **not** use the old `_unit_test.go` / `_intg_test.go` split — all tests live in `_test.go` and are differentiated **only** by Ginkgo `Label()` decorators.
- Labels come from `pkg/constants/testlabels` and are applied to top-level `Describe` (or `Context`) blocks so they propagate to every spec inside. A pure unit test carries only the category label (e.g. `testlabels.Controller`) and no infrastructure label; tests requiring `vcsim` add `testlabels.VCSim` and tests requiring `envtest` add `testlabels.EnvTest`.
- Any change observable on a Supervisor cluster **must** have E2E coverage in the same PR per [`e2e-sync-with-changes.md`](./e2e-sync-with-changes.md).

## Markdown

- Markdown files should not use hard line wrapping. Do not wrap lines; allow IDEs to do it for the user if that is their wish.
  - Exception: fenced code blocks (` ``` ` … ` ``` `). Lines inside a code fence should still be kept to ≤ 80 characters where practical.

## Repository layout and Go modules

`vm-operator` is a **multi-module** repository. The root `go.mod` is the main binary module; every `go.mod` nested below it is an independent Go module that is imported by the root (or by tooling) via `replace` directives in the workspace. For the package-level organization principles, import aliases, and naming conventions, see [`architectural-standards.md`](./architectural-standards.md).

### Root module — `github.com/vmware-tanzu/vm-operator`

```
go.mod / go.sum
main.go
api/                   — vmoperator.vmware.com API types (v1alpha1..v1alpha6)
cmd/                   — additional binaries
config/                — Kubernetes manifests, RBAC, Kustomize bases
controllers/           — reconcile loops (thin); one sub-package per controller
docs/                  — user-facing documentation
external/              — vendored API definitions (each sub-directory is its own module — see below)
hack/                  — build scripts (hack/tools/ is its own module — see below)
pkg/                   — business logic, providers, config, conditions
services/              — long-running manager runnables (e.g. vm-watcher)
test/                  — test builders, envtest helpers, E2E suites
webhooks/              — admission webhooks
.sdd/                  — Spec-Driven Development root
  memory/              — repository-wide rules (this file + companions)
  specs/               — per-feature artifacts
    NNN-slug/          — one directory per feature (spec.md, plan.md, tasks.md, ...)
```

### Sub-modules

| Directory | Module |
|-----------|--------|
| `api/` | `github.com/vmware-tanzu/vm-operator/api` |
| `api/test/` | `github.com/vmware-tanzu/vm-operator/api/test` |
| `external/appplatform/` | `github.com/vmware-tanzu/vm-operator/external/appplatform` |
| `external/byok/` | `github.com/vmware-tanzu/vm-operator/external/byok` |
| `external/capabilities/` | `github.com/vmware-tanzu/vm-operator/external/capabilities` |
| `external/infra/` | `github.com/vmware-tanzu/vm-operator/external/infra` |
| `external/ncp/` | `github.com/vmware-tanzu/vm-operator/external/ncp` |
| `external/storage-policy-quota/` | `github.com/vmware-tanzu/vm-operator/external/storage-policy-quota` |
| `external/tanzu-topology/` | `github.com/vmware-tanzu/vm-operator/external/tanzu-topology` |
| `external/vsphere-csi-driver/` | `github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver` |
| `external/vsphere-policy/` | `github.com/vmware-tanzu/vm-operator/external/vsphere-policy` |
| `hack/tools/` | `github.com/vmware-tanzu/vm-operator/hack/tools` |
| `pkg/backup/api/` | `github.com/vmware-tanzu/vm-operator/pkg/backup/api` |
| `pkg/constants/testlabels/` | `github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels` |

## Ticket / wiki conventions

- All code Stories belong to an Epic via `customfield_10830` (Epic Link), set post-create via PUT (ticket creation screen does not expose it).
- Acceptance Criteria in `customfield_10100`.
- Architecture docs (One Pager, Design, TDS) live in the wiki under `VM Operator: Design Docs` (wiki page ID: `644900152`, space `WCP`).

## Coding style

The non-negotiables below are constitutional; for import aliases, import group ordering, error wrapping, context usage, copyright headers, and comment grammar see [`architectural-standards.md`](./architectural-standards.md). Wherever a rule is also enforced by `golangci-lint`, [`.golangci.yml`](../../.golangci.yml) is the **source of truth** — this constitution and `architectural-standards.md` document the rule, but the linter is what enforces it.

- Follow the import alias table in [`.golangci.yml`](../../.golangci.yml) `linters.settings.importas.alias` (enforced by the `importas` linter). The `corev1` / `metav1` / `ctrl` / `vmopv1` / `pkgcfg` / `pkgctx` / … aliases are not optional.
- Follow the import-grouping rules enforced by `goimports` via `.golangci.yml` `formatters.settings.goimports.local-prefixes` (`github.com/vmware`, `github.com/vmware-tanzu`, `github.com/vmware-tanzu/vm-operator`).
- Do not import packages forbidden by `.golangci.yml` `linters.settings.depguard` (`io/ioutil`, `github.com/pkg/errors`, `k8s.io/utils`, and `testing` / `github.com/onsi/ginkgo` / `github.com/onsi/gomega` outside `_test.go`).
- `+optional` / `+required` markers on every struct field; `omitempty` JSON tags on optional fields.
- Resource names must be DNS-subdomain safe (`^[a-z][a-z0-9-]{0,61}[a-z0-9]?$`).
- Managed object IDs (`spec.id`) are immutable after create; enforce via webhook.

## Roles referenced in specs

| Role | Persona |
|------|---------|
| **CSP admin** | Cloud Service Provider admin with vCenter and Supervisor admin access |
| **Tenant admin** | Namespace-level admin managing VM Classes and policies within a tenancy boundary |
| **DevOps user** | Namespace member creating and operating VMs |
| **Platform engineer** | Builds controllers/webhooks in this repo (`controllers/`, `webhooks/`, `pkg/`) |
| **Partner engineer** | CCI/VCFA UI, VKS, supportability — consumes the API from outside `vmop` |
