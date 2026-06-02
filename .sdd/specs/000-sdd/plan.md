# Implementation Plan: VM Operator Specification Driven Development

- **Spec**: [`spec.md`](./spec.md)
- **Epic**: vmop-3820
- **Date**: 2026-05-27
- **Status**: In Progress

## Summary

Introduce Spec-Driven Development (SDD) to `vm-operator` by rooting all engineering-local rules and per-feature artifacts under a new `.sdd/` directory, with `.cursor/rules/*.mdc` files acting as thin proxies into `.sdd/memory/*.md`. The constitution stays small and declarative; companion files own the operational detail; per-feature `.sdd/specs/NNN-slug/` directories hold the actual feature work.

## Technical context

- **Go version**: N/A (this feature is documentation + tooling only; no production code changes).
- **API versions touched**: none.
- **Modules touched**: root module only — and only the `.sdd/` and `.cursor/` trees.
- **New dependencies**: none.
- **Tooling impact**: optional helper script under `hack/` (out of scope for this PR; tracked as a follow-up).

## Constitution check

This spec _establishes_ the constitution, so the check is largely self-referential. The rules below are the ones this PR must satisfy on the day it merges.

| Rule | Status | Notes |
|------|--------|-------|
| All SDD artifacts under `.sdd/` | OK | This PR creates the tree. |
| `spec.md` / `plan.md` / `tasks.md` required per feature | OK | This feature ships all three. |
| `NNN-slug` directory naming | OK | `000-sdd`. |
| `.cursor/rules/*.mdc` are thin proxies into `.sdd/memory/*.md` | OK | This PR converts every rule file. |
| No duplicated guidance between `.mdc` and `.md` | OK | Verified by inspection. |
| API compatibility | N/A | No API changes. |
| Thin controllers / no direct vSphere calls | N/A | No controller changes. |
| E2E sync with product changes | N/A | No product behavior change. |
| Markdown line wrapping | OK | Files use IDE soft-wrap; no hard wrapping. |

No complexity-tracking entries are required: this PR adopts the constitution it documents.

## Project structure

New / modified paths (relative to repository root):

```
.cursor/rules/
  architectural-standards.mdc           # proxy → .sdd/memory/architectural-standards.md
  e2e-sync-with-changes.mdc             # proxy → .sdd/memory/e2e-sync-with-changes.md
  e2e-testing.mdc                       # proxy → .sdd/memory/e2e-testing.md
  operator-best-practices.mdc           # proxy → .sdd/memory/operator-best-practices.md
  pull-request-standards.mdc                     # (kept; PR description format)
  testing-standards.mdc                 # proxy → .sdd/memory/testing-standards.md

.sdd/
  memory/
    constitution.md                     # NEW — non-negotiables + SDD overview
    sdd-standards.md                    # NEW — detailed SDD workflow + templates
    architectural-standards.md          # MOVED from .cursor/rules/*.mdc body
    operator-best-practices.md          # MOVED from .cursor/rules/*.mdc body
    testing-standards.md                # MOVED from .cursor/rules/*.mdc body
    e2e-testing.md                      # MOVED from .cursor/rules/*.mdc body
    e2e-sync-with-changes.md            # MOVED from .cursor/rules/*.mdc body
  specs/
    000-sdd/
      spec.md                           # NEW — the spec for this very feature
      research.md                       # NEW — upstream Spec Kit links
      plan.md                           # NEW — this file
      tasks.md                          # NEW — executable task list
```

No production code (`api/`, `controllers/`, `webhooks/`, `pkg/`, `cmd/`, `services/`, `test/`) is touched.

## API / CRD strategy

Not applicable.

## Controller / webhook impact

Not applicable.

## Test strategy

This change set is documentation-only. There is no Go code to test.

- **Unit / integration / E2E**: none.
- **Verification**: editor preview of every Markdown file; manual confirmation that each `.mdc` proxy resolves to an existing `.sdd/memory/*.md` file; spot-check that every internal link in `constitution.md`, `sdd-standards.md`, and the spec/plan/tasks resolves.
- **Lint**: rely on the repository's existing Markdown tooling (if any); otherwise, visually inspect for broken links and stale section headers.

## Rollout / migration

- **Phase 1 — Land the framework (this PR)**: introduce `.sdd/`, move authoritative content out of `.cursor/rules/*.mdc` bodies, leave `.mdc` files as proxies, ship the `000-sdd` spec/plan/tasks/research.
- **Phase 2 — Adopt for the next feature**: the next non-trivial feature merged after this PR uses the SDD workflow end-to-end (`001-...`). Reviewers reject the PR if `.sdd/specs/NNN-slug/` is missing required artifacts.
- **Phase 3 — Backfill (optional)**: high-value existing features may get retroactive `.sdd/specs/` directories so future contributors can find the canonical spec. This is opportunistic, not mandatory.
- **Phase 6 — Gap remediation**: close the findings from the `report.md` baseline gap analysis. Eight sub-tasks under `vmop-3829` address: the 11 `pkgcnd` alias violations (auto-fixable), two stale documentation inconsistencies, four `constitution.md` omissions, the 76-file test-naming migration (own spec + batched PRs), one finalizer naming gap, and two procedural audits that require Docker / a clean branch.
- **Partner communication**: announce on the internal `#vm-operator` channel and at the next architecture sync that all new features follow SDD. Link to `sdd-standards.md`.
- **Release notes**: this is an engineering-internal change; release note is `NONE`.

## Complexity tracking

None. This PR introduces, rather than deviates from, the constitution.

## Out of scope (deferred follow-ups)

The items below are tracked as open tasks in `tasks.md` Phase 5 and Phase 6:

- A `hack/new-spec.sh` helper that allocates the next `NNN` and scaffolds `spec.md` / `plan.md` / `tasks.md` from the templates in `sdd-standards.md`. _(T026 — vmop-3830)_
- A CI check that rejects PRs which touch `api/`, `controllers/`, `webhooks/`, or `pkg/` without a corresponding `.sdd/specs/NNN-slug/` directory. _(T028 — vmop-3832)_
- A `.sdd/specs/README.md` index that lists every feature directory and its current status. _(T027 — vmop-3831)_
- Backfilling specs for shipped features (case-by-case). _(T030 — vmop-3834)_
- Gap-remediation work identified by the `report.md` baseline audit: `pkgcnd` alias fixes, doc consistency updates, the 76-file test-naming migration, finalizer drift, and Docker/destructive audits. _(T031–T038 — vmop-3836–vmop-3843; Phase 6 in tasks.md)_
