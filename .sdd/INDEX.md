# SDD Index — vm-operator

This is the directory index for all Spec-Driven Development artifacts.

> **Rule for agents**: Before working on or reviewing any non-trivial change, check the specs table below. If the feature has a spec, read it — the spec is authoritative for intended behavior, acceptance criteria, and implementation decisions. If code and spec diverge, the spec wins (update one or the other in the same PR).

---

## Memory — repository-wide rules

These apply to all work in this repo. Read them before making or reviewing any change.

| File | What it covers |
|------|----------------|
| [constitution.md](memory/constitution.md) | Non-negotiables: API compatibility, controller/webhook design, testing rules, ticket and wiki masking |
| [architectural-standards.md](memory/architectural-standards.md) | Naming, import grouping, import aliases, error handling, copyright header, context/logger/feature-flag usage |
| [operator-best-practices.md](memory/operator-best-practices.md) | Canonical reconcile loop, finalizer management, VM update reconcile order, error semantics, mapper/watch patterns |
| [testing-standards.md](memory/testing-standards.md) | One test file per package, external `_test` package, Ginkgo labels, no `time.Sleep` |
| [e2e-testing.md](memory/e2e-testing.md) | E2E test structure, labels, helpers, make targets |
| [e2e-sync-with-changes.md](memory/e2e-sync-with-changes.md) | When E2E coverage is mandatory and what must ship together |
| [sdd-standards.md](memory/sdd-standards.md) | SDD workflow: when it applies, required artifacts, task format, ticket tagging |
| [commit-message-standards.md](memory/commit-message-standards.md) | Imperative mood, 50-char subject, 72-char body wrap |
| [pull-request-standards.md](memory/pull-request-standards.md) | Emoji prefix, release notes block, issue links, PR template |

---

## Specs — per-feature artifacts

Each spec lives under `specs/NNN-slug/`. Standard artifacts: `spec.md` (behavior + acceptance criteria), `plan.md` (technical approach), `tasks.md` (ordered task list), `research.md` (investigation log), `model.md` (API surface, if applicable).

| # | Slug | Title | Status | Epic |
|---|------|-------|--------|------|
| 000 | [sdd](specs/000-sdd/) | VM Operator Specification Driven Development | In Progress | vmop-3820 |
| 001 | [class-policy-resize](specs/001-class-policy-resize/) | VM Service Class Policy and Resize — Policy + Environment Browser Pipeline | In Progress | vmop-3331 |

### Finding the right spec

- Working on a feature? Match the affected CRD, controller, or package against the spec titles above and read that spec's `spec.md` and `plan.md` before writing code.
- Reviewing a PR? Check whether the changed files correspond to an in-progress spec. If so, verify the change is consistent with the spec's acceptance criteria and task list.
- Starting new work? If no spec exists yet and the change is non-trivial, create one under `specs/NNN-slug/` before writing code (see [sdd-standards.md](memory/sdd-standards.md)).
