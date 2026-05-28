# Spec-Driven Development (SDD) Standards

This document defines **how** to practice Spec-Driven Development in this repository. The constitutional **what** lives in [`constitution.md`](./constitution.md); this file is the operations manual.

For the underlying methodology, see the upstream Spec Kit project: <https://github.com/github/spec-kit> and [`spec-driven.md`](https://github.com/github/spec-kit/blob/main/spec-driven.md).

---

## Philosophy

Specifications are the source of truth. Code is the expression of a specification in Go, YAML, and tests. When intent and code diverge, the spec wins — either the code is wrong, or the spec was wrong and must be updated **before** the code is patched.

In practical terms for `vm-operator`:

- A spec captures **what** users (CSP admin, tenant admin, DevOps user, partner engineer) need and **why**.
- A plan captures **how** the system will satisfy the spec while honoring the constitution.
- Tasks decompose the plan into ordered, executable units of work that can be picked up by a human or an AI assistant.
- Research, data models, and contracts back each of the above with reproducible evidence.

---

## Directory layout

All SDD artifacts live under `.sdd/` at the repository root:

```
.sdd/
├── memory/                              # repository-wide rules (this file + companions)
│   ├── constitution.md                  # the non-negotiables
│   ├── sdd-standards.md                 # this file
│   ├── architectural-standards.md
│   ├── operator-best-practices.md
│   ├── testing-standards.md
│   ├── e2e-testing.md
│   └── e2e-sync-with-changes.md
└── specs/
    └── NNN-slug/                        # one directory per feature
        ├── spec.md                      # REQUIRED — feature specification
        ├── plan.md                      # REQUIRED — implementation plan
        ├── tasks.md                     # REQUIRED — executable task list
        ├── research.md                  # SHOULD   — investigation + prior art
        ├── model.md                     # SHOULD   — data model / API surface
        └── contracts/                   # MAY      — OpenAPI / CRD schemas / golden samples
```

The Cursor proxy rules under `.cursor/rules/*.mdc` are **thin pointers** to these files; they are not an alternate source of truth. Edits to guidance go into `.sdd/memory/*.md`, never into the `.mdc` files.

### Feature directory naming

- Format: `NNN-slug` where `NNN` is a zero-padded, monotonically increasing scalar starting at `000`.
- The scalar expands beyond three digits when needed (`1000-something`).
- The `slug` is lowercase kebab-case, derived from the feature title. Keep it short and self-explanatory (`001-class-policy-resize`, `002-async-quota`).
- Never reuse or renumber an existing directory. Abandoned specs stay in tree and are marked `Status: Abandoned` in `spec.md`.

### Scalar allocation

When opening a new feature, scan `.sdd/specs/` and take `max(NNN) + 1`. Allocate the directory and an empty `spec.md` first, then push to a feature branch so subsequent contributors see the number is taken.

---

## Tickets and wiki links

This repository is mirrored upstream, so **internal Broadcom URLs (JIRA tickets, Confluence wiki pages, internal dashboards) MUST NOT appear in any file under `.sdd/`, `.cursor/`, `docs/`, or any other tracked path**. Use the conventions below instead. See [`constitution.md`](./constitution.md) "Tickets and WIKI links" for the underlying rule.

### Ticket references

- Replace the internal JIRA project prefix with `vmop`. The internal ticket `XXX-123` is written as **`vmop-123`** everywhere in the repository (spec headers, task lines, plan tables, PR descriptions, commit trailers).
- Do **not** embed the full ticket URL. A bare `vmop-NNN` token is enough; tooling on the internal side resolves it.
- When a single artifact references multiple tickets, list them comma-separated and in ascending numeric order: `vmop-101, vmop-204, vmop-997`.

### Wiki references

- Replace any internal wiki URL with the literal string `WIKI page <ID>`, e.g. **`WIKI page 644900152`**. Optionally append the page title in plain text for readability: `WIKI page 644900152 — "VM Operator: Design Docs"`.
- Never paste the wiki URL itself, and never include the wiki space slug or query parameters.

### Spec ↔ epic, task ↔ story / sub-task

The constitution requires SDD work to map onto Broadcom JIRA tickets at two levels:

| SDD artifact | JIRA artifact | Field |
|--------------|---------------|-------|
| `.sdd/specs/NNN-slug/spec.md` | **Epic** | recorded in the spec header as `Epic: vmop-NNN` |
| Each task line in `.sdd/specs/NNN-slug/tasks.md` | **Story** or **Sub-task** under the epic above | recorded as a `[vmop-NNN]` tag on the task line |
| Acceptance criteria within `spec.md` | populated into the epic's `customfield_10100` (Acceptance Criteria) | mirrored, not linked |
| Epic Link on a story / sub-task | `customfield_10830` (Epic Link), set **post-create** via PUT | the create screen does not expose it; set it programmatically |

#### Rules

- A spec **MUST** declare its epic in the header: `**Epic**: vmop-NNN`. If the epic does not yet exist, file it before merging the spec PR and update the header in the same PR.
- A task **SHOULD** carry a `[vmop-NNN]` tag pointing to its story or sub-task. Tasks for trivial setup work (lint config, README touch-ups) **MAY** omit the tag; everything that produces shipping code **MUST** have a ticket.
- A story or sub-task ticket created for an SDD task **MUST** be linked to the spec's epic via `customfield_10830`. Without that link, the work is invisible in the epic's burn-down.
- If a single PR closes more than one task, list every `vmop-NNN` it resolves in the PR description's release-note block (per `pull-request-standards.md`).
- If you copy acceptance criteria from `spec.md` into the JIRA epic's `customfield_10100`, keep them in sync. The spec is the source of truth; the JIRA copy is a mirror that may lag by one PR but never by more.

### Design docs and wiki pointers

- One-pagers, design docs, and TDS documents continue to live in the wiki under `WIKI page 644900152 — "VM Operator: Design Docs"`.
- A spec **MAY** reference these wiki pages from `research.md` using the `WIKI page <ID>` form. Do not embed wiki content verbatim — link to it and summarize.

---

## Required artifacts

### `spec.md` — feature specification

Captures _what_ and _why_. Audience is product, engineering, and partners. Forbidden content: technology choices, package paths, function signatures, Go code.

Minimum sections:

1. **Header**: feature branch, fork, PR target, created date, status (`Draft` | `In Progress` | `Implemented` | `Abandoned`), and **`Epic: vmop-NNN`** (see "Tickets and wiki links" above). The epic ticket is required before merge; while the spec is still in `Draft` the header may carry `Epic: TBD` so reviewers know the ticket has not yet been filed.
2. **Goals**: bullet list of testable outcomes. Use `MUST` / `SHOULD` / `MAY` per RFC 2119. These goals are also what gets copied into the epic's `customfield_10100` (Acceptance Criteria) — keep them mirror-able.
3. **Non-goals**: bullet list of things this spec explicitly _does not_ deliver.
4. **User stories / acceptance criteria**: one per persona where applicable. Each criterion must be observable from outside the system (kubectl, vCenter UI, API response).
5. **Open questions**: mark unresolved items inline as `[NEEDS CLARIFICATION: question]`. A spec with open `[NEEDS CLARIFICATION]` markers may still be merged so long as the affected work is blocked in `tasks.md` until the markers are resolved.

A spec is **ready for planning** when every goal is testable, every non-goal is explicit, every `[NEEDS CLARIFICATION]` either has an answer or is annotated with the unblocking owner, and the header's `Epic: vmop-NNN` references a real ticket (not `TBD`).

### `plan.md` — implementation plan

Translates the spec into a constitutionally-compliant technical approach. Audience is engineering. Allowed (and expected): package paths, CRD versioning strategy, controller/webhook/provider impacts, test strategy, migration plan.

Minimum sections:

1. **Summary**: 1–3 sentences linking back to the spec.
2. **Technical context**: Go version, primary dependencies, target API versions (`v1alpha5` / `v1alpha6`), affected modules.
3. **Constitution check**: enumerate the constitutional rules that apply and confirm compliance. If a rule must be bent, justify it in the **Complexity tracking** section and reference the amendment PR (if any).
4. **Project structure**: list new / modified directories and files at package granularity. Reference [`architectural-standards.md`](./architectural-standards.md).
5. **API / CRD strategy**: additive vs version bump; conversion webhook; CEL vs Go validation. Cross-reference [`constitution.md`](./constitution.md) "API compatibility".
6. **Controller / webhook impact**: which reconcilers, webhooks, providers, services change; new RBAC; new feature flags (`pkgcfg.Features.*`). Cross-reference [`operator-best-practices.md`](./operator-best-practices.md).
7. **Test strategy**: unit (`testlabels.Controller`, etc.), integration (`testlabels.EnvTest` / `testlabels.VCSim`), and E2E. Cross-reference [`testing-standards.md`](./testing-standards.md) and [`e2e-sync-with-changes.md`](./e2e-sync-with-changes.md). E2E coverage is **mandatory** for any cluster-observable behavior change.
8. **Rollout / migration**: feature flag default, backfill / schema upgrade, partner-facing communication, release-note guidance.
9. **Complexity tracking** _(optional)_: table of constitutional deviations with rationale and the simpler alternative that was rejected.

The plan stays **high-level and readable**. Long code samples, golden YAML, or deep algorithm walk-throughs belong in `research.md`, `model.md`, or `contracts/`.

### `tasks.md` — executable task list

Decomposes the plan into ordered, parallelizable tasks. Audience is whoever (human or AI assistant) will execute the work.

Task format: `[T###] [P?] [USx?] [vmop-NNN?] Description (path/to/file)`

- `T###` — monotonically increasing within the feature.
- `[P]` — _optional_ marker indicating the task can run in parallel with other `[P]` tasks in the same phase (different files, no dependencies between them).
- `[USx]` — _optional_ user-story tag from `spec.md` for traceability.
- `[vmop-NNN]` — _optional_ for trivial setup tasks, **required** for any task that produces shipping code. Points at the JIRA story or sub-task that tracks the work; that ticket must be linked to the spec's epic via `customfield_10830` (Epic Link). See "Tickets and wiki links" above.
- Description must include the **exact file paths** the task touches.

Phase structure:

1. **Phase 1 — Setup**: scaffolding, generation, dependency bumps. No business logic.
2. **Phase 2 — Foundational**: shared infrastructure that blocks every user story (e.g. a new typed context, a new feature flag, a new provider method signature).
3. **Phase 3..N — Per user story / feature slice**: each phase is independently testable; the suite from `spec.md`'s acceptance criteria passes at the end of the phase.
4. **Phase Final — Polish**: documentation, release notes, dashboards, removing temporary flags.

Tasks are **completable in one PR or less**. A task that grows beyond a single reviewable change set must be split.

### `research.md` — investigation log _(SHOULD)_

Captures the evidence behind the spec/plan: link to upstream issues, prior art in the repo, vCenter API docs, performance benchmarks, library comparisons. Future engineers (and AI assistants) inherit this context — investing here pays off on the next feature in the same area.

### `model.md` — data model / API surface _(SHOULD when applicable)_

Required when the feature introduces or modifies a CRD field, status condition, webhook contract, or partner-facing API. Contents:

- Field-by-field schema with `+optional` / `+required`, validation rules, immutability.
- Status conditions: type, reason constants, transition semantics.
- Conversion strategy when crossing API versions.
- Example YAML for each canonical user story.

`contracts/` _(optional)_ holds machine-readable artifacts: OpenAPI fragments, generated CRD YAML excerpts, JSONSchema, golden vSphere payloads.

---

## When SDD applies

| Change | Apply SDD |
|--------|-----------|
| New CRD, new field, new condition, new webhook | **Yes** (spec + plan + tasks + model) |
| New controller, new reconciler, new provider method | **Yes** (spec + plan + tasks) |
| Behavior change observable on a Supervisor cluster | **Yes** (spec + plan + tasks + E2E) |
| New feature flag (`pkgcfg.Features.*`) | **Yes** (at minimum spec + plan + tasks describing default, rollout, removal criteria) |
| Refactor that crosses package boundaries | **Yes** (plan + tasks; spec optional if no user-visible behavior change) |
| One-line bug fix, typo, doc tweak | **No** |
| Dependency bump, codegen refresh | **No** |
| Test-only addition that does not change product behavior | **No** spec required; still link to the spec that originally introduced the behavior |

When in doubt, write the spec. A two-paragraph spec that prevents one scope-creep argument is worth more than the time it costs.

---

## Workflow

The day-to-day loop mirrors the upstream Spec Kit `/speckit.specify` → `/speckit.plan` → `/speckit.tasks` flow, adapted to this repository's conventions.

### 1. Open the spec

1. Branch off `main` as `feature/<slug>` (matches the `NNN-slug` directory you are about to create).
2. Allocate the next `NNN` and create `.sdd/specs/NNN-slug/spec.md` from the template in this document.
3. File the JIRA **epic** that this spec implements; capture it in the spec header as `Epic: vmop-NNN`. If the epic does not yet exist when you open the PR, the header may carry `Epic: TBD` while the spec is still `Draft`, but it **MUST** be a real `vmop-NNN` before the spec PR merges.
4. Draft goals, non-goals, and acceptance criteria. Mark every unknown `[NEEDS CLARIFICATION: …]`. Plan to mirror the acceptance criteria into the epic's `customfield_10100` once they stabilize.
5. Push the branch with the spec only (no code) so reviewers can comment on the spec before any code lands.

### 2. Research and model

1. Capture findings in `research.md` as you investigate. Link upstream issues, prior PRs in this repo, vCenter docs.
2. If the feature touches an API surface, draft `model.md` next to the spec.

### 3. Plan

1. Write `plan.md`. Run the constitution check explicitly — every applicable rule in [`constitution.md`](./constitution.md) gets a line.
2. If a rule must be bent, add an entry to **Complexity tracking** with the alternative considered and the reason it was rejected.
3. Update the spec when the plan reveals a hidden requirement.

### 4. Tasks

1. Decompose `plan.md` into `tasks.md`. Each task touches at most a small number of files and finishes in one PR.
2. Mark independent tasks `[P]`; group them in phases that respect their dependencies.
3. For every task that produces shipping code, file a **story** or **sub-task** ticket under the spec's epic and add the `[vmop-NNN]` tag to the task line. Set `customfield_10830` (Epic Link) on the new ticket post-create via PUT — the create screen does not expose it.
4. Re-validate against the spec — every acceptance criterion is covered by at least one task, and every shipping-code task carries a `[vmop-NNN]` tag.

### 5. Execute

1. Pick up tasks in phase order. `[P]` tasks within a phase may proceed in parallel.
2. Update `tasks.md` (check the box, add follow-up tasks when discovered) as part of the implementing PR.
3. The implementing PR description lists every `vmop-NNN` story / sub-task it closes (per `pull-request-standards.md`); newly-discovered follow-up tasks get a new `vmop-NNN` filed under the same epic.
4. When implementation reveals a spec error, update `spec.md` in the same PR. **Do not** carry an out-of-date spec into `main`.
5. Tests ship with the behavior they cover. Cluster-observable behavior ships with E2E coverage in the same PR per [`e2e-sync-with-changes.md`](./e2e-sync-with-changes.md).

### 6. Close out

1. Flip `spec.md` status from `In Progress` to `Implemented` in the final PR.
2. Ensure `tasks.md` has every task checked, every `[NEEDS CLARIFICATION]` resolved, and every follow-up either completed or filed as a separate spec.
3. Release notes follow `pull-request-standards.md` (the PR description format).

---

## Amendments to the constitution

The constitution is the contract; it is _not_ immutable. Amendments are explicit:

1. Open a spec (`.sdd/specs/NNN-constitution-amendment/`) describing the rule being changed and why.
2. The amendment PR modifies `constitution.md` and any affected memory files in the same change set.
3. The PR description calls out the amendment and links to the spec.
4. Existing in-flight specs that relied on the old rule get an update note in their `plan.md` "Complexity tracking" section.

Never silently drift from the constitution in a feature PR. If the rule is wrong, fix it. If the feature needs an exception, justify it in `plan.md`.

---

## Templates

### `spec.md` template

```markdown
# Feature Specification: <Title>

- **Feature branch**: [`feature/<slug>`](<branch-url>)
  - **Fork**: <fork>
  - **PR target**: `vmware-tanzu/vm-operator`
- **Created**: YYYY-MM-DD
- **Status**: Draft | In Progress | Implemented | Abandoned
- **Epic**: vmop-NNN          <!-- required before merge; "TBD" allowed only while Draft -->
- **Design docs**: WIKI page <ID>  <!-- optional; see "Tickets and wiki links" -->

---

## Goals

- ...

## Non-goals

- ...

## User stories / acceptance criteria

### <Persona>

- **Given** ... **When** ... **Then** ...

## Open questions

- [NEEDS CLARIFICATION: ...]
```

### `plan.md` template

```markdown
# Implementation Plan: <Title>

- **Spec**: [`spec.md`](./spec.md)
- **Epic**: vmop-NNN  <!-- mirrors the spec header -->
- **Date**: YYYY-MM-DD

## Summary

<1–3 sentences>

## Technical context

- **Go version**: ...
- **API version(s) touched**: v1alphaX
- **Modules touched**: ...
- **New dependencies**: ...

## Constitution check

| Rule | Status | Notes |
|------|--------|-------|
| API compatibility | OK | additive only, no version bump |
| Thin controllers | OK | business logic in `pkg/<x>` |
| ... | ... | ... |

## Project structure

```
controllers/<x>/...
pkg/<x>/...
api/v1alphaX/...
```

## API / CRD strategy

...

## Controller / webhook impact

...

## Test strategy

- Unit: ...
- Integration: ...
- E2E: ... (mandatory if behavior is cluster-observable)

## Rollout / migration

- Feature flag: `pkgcfg.Features.<Name>` default `false`
- Schema upgrade: ...
- Partner comms: ...

## Complexity tracking

| Violation | Why needed | Simpler alternative rejected because |
|-----------|------------|--------------------------------------|
| ... | ... | ... |
```

### `tasks.md` template

```markdown
# Tasks: <Title>

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Epic**: vmop-NNN  <!-- mirrors the spec header -->

## Phase 1 — Setup

- [ ] T001 Scaffold package skeleton in `pkg/<x>/`
- [ ] T002 [P] Add feature flag `pkgcfg.Features.<Name>` in `pkg/config/features.go`

## Phase 2 — Foundational

- [ ] T003 [vmop-201] Add typed context `<Resource>Context` in `pkg/context/<resource>_context.go`

## Phase 3 — User Story 1 (<persona>)

- [ ] T004 [US1] [vmop-202] Implement reconciler scaffold in `controllers/<x>/<x>_controller.go`
- [ ] T005 [P] [US1] [vmop-203] Unit tests in `controllers/<x>/<x>_controller_test.go`
- [ ] T006 [US1] [vmop-204] E2E coverage in `test/e2e/vmservice/<x>/<x>_test.go`

## Phase Final — Polish

- [ ] T0NN [vmop-NNN] Update `docs/` and release notes
- [ ] T0NN [vmop-NNN] Remove temporary feature flag once GA (tracked in `.sdd/specs/NNN-x-ga/`)
```

Every `vmop-NNN` in `tasks.md` is a **story** or **sub-task** linked to the spec's epic via `customfield_10830`. Trivial setup tasks (lint config, scaffold, README) may omit the tag; everything that produces shipping code requires one.

---

## Anti-patterns

1. **Implementation details in `spec.md`** — package paths, function names, Go code. Move them to `plan.md`.
2. **Silent constitution violations** — never bend a rule without an explicit entry in `plan.md` "Complexity tracking" (or an amendment PR).
3. **Stale specs in `main`** — if the code says one thing and the spec says another after a PR merges, the spec is wrong by definition. Update it in the same PR.
4. **Duplicated guidance between `.cursor/rules/*.mdc` and `.sdd/memory/*.md`** — the `.mdc` file is a pointer, not a copy.
5. **Renumbering or reusing `NNN`** — abandoned specs stay; their status flips to `Abandoned`.
6. **Mixing E2E-only PRs with product changes** — if the change set is _only_ E2E, do not touch `pkg/` or `api/` (see [`e2e-sync-with-changes.md`](./e2e-sync-with-changes.md)).
7. **Treating generated plans as authority over production reality** — incidents that contradict a spec result in a spec update, not a wallpaper-over fix.
8. **Internal Broadcom URLs in tracked files** — never paste an internal JIRA, Confluence, or dashboard URL. Use `vmop-NNN` for tickets and `WIKI page <ID>` for wiki pages (see "Tickets and wiki links").
9. **Wrong-prefix ticket references** — never write the internal JIRA project prefix in this repository. The upstream-safe prefix is always `vmop-`.
10. **Merging `tasks.md` shipping-code tasks without a `[vmop-NNN]` tag** — every task that produces shipping code must be backed by a story or sub-task linked to the spec's epic via `customfield_10830`.
11. **Specs without an epic** — a spec whose header still says `Epic: TBD` at merge time is a constitutional violation; file the epic first.
12. **Epic Link skipped on create** — JIRA's create screen does not expose `customfield_10830`; remember to set it via PUT after the ticket is created, otherwise the work is invisible to the epic's burn-down.
