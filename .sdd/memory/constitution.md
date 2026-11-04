# Constitution: vm-operator Spec-Driven Development

- **Repository**: [`github.com/vmware-tanzu/vm-operator`](https://github.com/vmware-tanzu/vm-operator)
- **Last updated**: 2026-10-18
- **Applies to**: All changes in this repository, including all feature specs under `.sdd/specs/`

This document defines the **non-negotiables** that govern every change. Companion files in `.sdd/memory/` (indexed in [`.sdd/INDEX.md`](../INDEX.md)) state _how_ to satisfy them. Wherever a rule is also enforced by `golangci-lint`, [`.golangci.yml`](../../.golangci.yml) is the source of truth.

<!--
Maintainers: AI-assistant proxies (CLAUDE.md, nested CLAUDE.md files, .claude/rules/, .cursor/rules/*.mdc) point at these files. Edit guidance here, never in the proxies. Block-level HTML comments like this one are stripped before Claude Code loads the file, so they cost no context.
-->

---

## Tickets and WIKI links

Due to Broadcom rules, internal links are disallowed in upstream repositories. To that end, any internal tickets or WIKI links should not be used in this repository:

* Instead, the prefix `vmop` is used in place of our internal JIRA project. Therefore, an internal ticket `XXX-123` becomes `vmop-123`.
* In place of a direct WIKI URL, just reference the page ID with the prefix `WIKI page`, ex. `WIKI page 123456`.

---

## Spec-Driven Development (SDD)

All non-trivial work follows the SDD workflow in [`sdd-standards.md`](./sdd-standards.md) — templates, artifact contracts, when SDD applies, ticket tagging, and the amendment process. Read it before creating or editing anything under `.sdd/specs/`.

- All SDD artifacts live under `.sdd/`: repository-wide rules in `.sdd/memory/*.md`, per-feature artifacts in `.sdd/specs/NNN-slug/`, where `NNN` is a zero-padded auto-incrementing scalar starting at `000` that expands beyond three digits when needed (`1000-...`).
- Each feature directory **MUST** contain `spec.md`, `plan.md`, and `tasks.md`. It **SHOULD** contain `research.md`, and **SHOULD** contain `model.md` when the feature introduces or changes a data model / API.
- An SDD spec should be related to an epic ticket; all SDD tasks should be related to story or sub-task tickets.
- New features that materially affect product behavior **MUST** ship with their spec, plan, tasks, and (if applicable) model artifacts in the same change set as the code. Code-only PRs are reserved for the trivial fixes exempted in `sdd-standards.md`.
- Mark every unknown in a spec with `[NEEDS CLARIFICATION: …]` rather than guessing. When implementation reveals a wrong assumption, update the spec/plan/tasks in the same PR.
- AI-assistant-specific files (`CLAUDE.md`, `.claude/rules/`, `.cursor/rules/*.mdc`) **MUST** be proxies that link back into `.sdd/memory/*.md`. Do not duplicate guidance across the trees.
- The constitution is **immutable per change**: a PR that needs to bend a constitutional rule must amend the constitution in the same PR with a documented rationale, and call that amendment out in the PR description.

---

## API compatibility

- The default API group for this repository is `vmoperator.vmware.com`. These types are consumed by controllers, webhooks, and external partners, all within this same repo. Breaking changes (field removal, rename, type change) require a version bump and a conversion webhook. Additive changes are *not* safe once an API has shipped due to the way Kubernetes handles `UPDATE` operations. See <https://github.com/kubernetes/kubernetes/issues/111703> for more information on this topic.
- Every new CRD must include `+kubebuilder:object:root=true`, a `+groupversion` doc.go entry, a `// +groupName:` marker, and deepcopy generated via `make generate-go`.
- CRD manifests are generated under `config/crd/`; they are **checked in** (not generated at deploy time) and must be regenerated with `make generate-manifests`.
- External APIs maintained in this repository live under `external/`, each in its own Go module; regenerate their manifests with `make generate-external-manifests`.

## Controller design

Details (reconcile-loop template, watches, error semantics, channel sources) are in [`operator-best-practices.md`](./operator-best-practices.md).

- Controllers are **thin**: reconcile loops live in `controllers/`; all business logic lives in `pkg/`.
- No controller may call vSphere APIs directly; use the provider abstraction under `pkg/providers/vsphere/`.
- Controllers must track `status.observedGeneration` and set a `Ready` condition.
- Fan-out to child objects uses `controllerutil.CreateOrPatch` by default; ownership is set via `controllerutil.SetControllerReference` unless the object is cluster-scoped, in which case use labels for ownership tracing. When multiple parents fan-in write to the same *list-typed* field on a shared child (owner references, a status list each parent upserts into), patch with `client.MergeFromWithOptimisticLock` instead and skip the write when nothing changed — a plain merge patch replaces lists wholesale and can silently drop a concurrent writer's entry.
- Controllers for API groups other than `vmoperator.vmware.com` should not be placed directly in the `controllers/` directory.

## Webhooks

- Admission webhooks live in `webhooks/`; share validation logic with unit tests via unexported validator types (not embedded in the webhook handler).
- `+kubebuilder:webhook` markers drive code generation; registration lives in `webhooks/suite_test.go` and `main.go`.
- CEL validation is preferred for simple structural rules; Go validation for complex, cross-field, or vSphere-data-dependent rules.
- Webhooks for API groups other than `vmoperator.vmware.com` should not be placed directly in the `webhooks/` directory.

## Testing

Unit/integration patterns are in [`testing-standards.md`](./testing-standards.md); E2E in [`e2e-testing.md`](./e2e-testing.md) and [`e2e-sync-with-changes.md`](./e2e-sync-with-changes.md).

- **One test file** per package: `<package>_test.go` (external `_test` package), plus **one suite bootstrap** `<package>_suite_test.go` containing only the `TestXxx(t *testing.T)` entry-point that calls `RunSpecs`.
- Do **not** use the old `_unit_test.go` / `_intg_test.go` split — many older packages still have it; do not copy it. Tests are differentiated **only** by Ginkgo `Label()` decorators.
- Labels come from `pkg/constants/testlabels` and are applied to top-level `Describe` (or `Context`) blocks. A pure unit test carries only the category label (e.g. `testlabels.Controller`); tests requiring `vcsim` add `testlabels.VCSim` and tests requiring `envtest` add `testlabels.EnvTest`.
- Any change observable on a Supervisor cluster **must** have E2E coverage in the same PR.

## Markdown

- Markdown files should not use hard line wrapping. Do not wrap lines; allow IDEs to do it for the user if that is their wish.
  - Exception: fenced code blocks (` ``` ` … ` ``` `). Lines inside a code fence should still be kept to ≤ 80 characters where practical.

## Repository layout and Go modules

```
api/          vmoperator.vmware.com API types (v1alpha1..v1alpha6)
cmd/          additional binaries
config/       Kubernetes manifests, RBAC, Kustomize bases
controllers/  reconcile loops (thin); one sub-package per controller
docs/         user-facing documentation
external/     vendored external API types (byok, ncp, capabilities, ...)
hack/         build and development scripts
pkg/          business logic: providers/, vmconfig/, conditions/,
              config/ (pkgcfg), context/ (pkgctx), errors/ (pkgerr),
              log/ (pkglog), patch/, record/, util/,
              constants/ (incl. testlabels)
services/     long-running manager runnables (e.g. vm-watcher)
test/         builders (test/builder), envtest helpers, E2E (test/e2e)
webhooks/     admission webhooks (validation/ and mutation/ subdirs)
.sdd/         Spec-Driven Development root (memory/, specs/)
```

This is a **multi-module** repository sharing one git history. Besides the root module (`github.com/vmware-tanzu/vm-operator`), these directories have their own `go.mod`: `api/`, `api/test/`, `api-docs/`, each vendored API under `external/` (`external/<name>/`, except `external/vim/api/`), `hack/tools/`, `hack/tools/kube-api-linter/`, `pkg/backup/api/`, `pkg/constants/testlabels/`, and `test/e2e/`. There is no `go.work`, so `go test ./...` from the root does not cover them.

## Coding style

Details (naming, error wrapping, context usage, copyright header, comment grammar) are in [`architectural-standards.md`](./architectural-standards.md).

- Run `make lint-go`. It enforces the import aliases (`importas`), import grouping (`goimports` local-prefixes), and forbidden imports (`depguard`: `io/ioutil`, `github.com/pkg/errors`, `k8s.io/utils`, and `testing` / ginkgo / gomega outside `_test.go`).
- `+optional` / `+required` markers on every struct field; `omitempty` JSON tags on optional fields.
- Resource names must be DNS-subdomain safe (`^[a-z][a-z0-9-]{0,61}[a-z0-9]?$`).
- Managed object IDs (`spec.id`) are immutable after create. Use a plain CEL `self == oldSelf` transition rule for the unconditional case, and a webhook only when the immutability rule is conditional or cross-field.
