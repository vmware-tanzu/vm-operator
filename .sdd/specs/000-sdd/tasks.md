# Tasks: VM Operator Specification Driven Development

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Epic**: vmop-3820

This task list captures both the work already landed in commit `f5906bc` ("Spec-driven development") and the small amount of follow-up work needed to complete the framework. Boxes that are already true on `main` are checked; the rest remain open.

Story / sub-task mapping (see [`../../memory/sdd-standards.md`](../../memory/sdd-standards.md) "Tickets and wiki links"):

| Phase | Story | Sub-task(s) |
|-------|-------|-------------|
| Phase 1 (Setup) | `vmop-3821` | `vmop-3822` |
| Phase 2 (Foundational) | `vmop-3821` | `vmop-3823` |
| Phase 3 (US1 — Bootstrap) | `vmop-3821` | `vmop-3824` |
| Phase 4 (US2 — Adopt) | `vmop-3825` | `vmop-3826`, `vmop-3827`, `vmop-3828` |
| Phase 5 (Polish) | `vmop-3829` | `vmop-3830`–`vmop-3834` |
| Phase 6 (Gap remediation) | `vmop-3829` | `vmop-3836`–`vmop-3843` |

## Phase 1 — Setup (Shared Infrastructure)

**Purpose**: Stand up the `.sdd/` tree and the proxy rule layout.

- [x] T001 [vmop-3822] Create `.sdd/` at the repository root with `memory/` and `specs/` sub-directories.
- [x] T002 [P] [vmop-3822] Move authoritative architectural guidance into `.sdd/memory/architectural-standards.md`.
- [x] T003 [P] [vmop-3822] Move authoritative operator/controller guidance into `.sdd/memory/operator-best-practices.md`.
- [x] T004 [P] [vmop-3822] Move authoritative unit/integration testing guidance into `.sdd/memory/testing-standards.md`.
- [x] T005 [P] [vmop-3822] Move authoritative E2E testing guidance into `.sdd/memory/e2e-testing.md`.
- [x] T006 [P] [vmop-3822] Move authoritative "E2E sync with changes" guidance into `.sdd/memory/e2e-sync-with-changes.md`.
- [x] T007 [P] [vmop-3822] Rewrite `.cursor/rules/architectural-standards.mdc` as a thin proxy that points to `.sdd/memory/architectural-standards.md`.
- [x] T008 [P] [vmop-3822] Rewrite `.cursor/rules/operator-best-practices.mdc` as a thin proxy that points to `.sdd/memory/operator-best-practices.md`.
- [x] T009 [P] [vmop-3822] Rewrite `.cursor/rules/testing-standards.mdc` as a thin proxy that points to `.sdd/memory/testing-standards.md`.
- [x] T010 [P] [vmop-3822] Rewrite `.cursor/rules/e2e-testing.mdc` as a thin proxy that points to `.sdd/memory/e2e-testing.md`.
- [x] T011 [P] [vmop-3822] Rewrite `.cursor/rules/e2e-sync-with-changes.mdc` as a thin proxy that points to `.sdd/memory/e2e-sync-with-changes.md`.
- [x] T012 [vmop-3822] Rename the existing PR-description rule to `pull-request-standards.md` and keep it in place.

## Phase 2 — Foundational (Blocking Prerequisites)

**Purpose**: Land the constitution and the SDD operations manual so every later spec has a stable reference.

**Checkpoint**: Once this phase is complete, the framework is usable end-to-end; subsequent features create their own `.sdd/specs/NNN-slug/` directories.

- [x] T013 [vmop-3823] Author the first cut of `.sdd/memory/constitution.md` covering project identity, API compatibility, controller design, webhooks, testing, repository layout, ticketing, coding style, and roles.
- [x] T014 [US1] [vmop-3823] Refactor `.sdd/memory/constitution.md` so each section that overlaps with a companion file references that file instead of duplicating it (e.g. the testing section points to `testing-standards.md`).
- [x] T015 [US1] [vmop-3823] Add an "SDD" section to `.sdd/memory/constitution.md` that captures the non-negotiables for SDD adoption and the expected behavior for new features.
- [x] T016 [US1] [vmop-3823] Author `.sdd/memory/sdd-standards.md` with the detailed SDD workflow, artifact contracts, anti-patterns, and templates for `spec.md`, `plan.md`, and `tasks.md`.

## Phase 3 — User Story 1 — Bootstrap the SDD framework (Priority: P1) 🎯 MVP

**Goal**: A contributor (or AI assistant) can read `.sdd/memory/` and produce a constitutionally-compliant spec/plan/tasks set for a new feature.

**Independent Test**: Open `.sdd/memory/constitution.md` and `.sdd/memory/sdd-standards.md` cold; confirm that every linked companion file resolves and that the spec/plan/tasks templates are sufficient to start a new `.sdd/specs/NNN-slug/` directory without referring to upstream Spec Kit docs.

- [x] T017 [US1] [vmop-3824] Create `.sdd/specs/000-sdd/spec.md` describing the SDD adoption feature, its goals, and non-goals.
- [x] T018 [US1] [vmop-3824] Create `.sdd/specs/000-sdd/research.md` linking the upstream Spec Kit repository, the SDD methodology doc, the VM Operator docs index, the E2E guide, and the architectural standards.
- [x] T019 [US1] [vmop-3824] Create `.sdd/specs/000-sdd/plan.md` translating `spec.md` into a constitutionally-compliant implementation plan.
- [x] T020 [US1] [vmop-3824] Create `.sdd/specs/000-sdd/tasks.md` (this file) decomposing `plan.md` into executable tasks.

## Phase 4 — User Story 2 — Adopt SDD for the next real feature (Priority: P2)

**Goal**: The first non-trivial feature merged after `000-sdd` lives in `.sdd/specs/001-<slug>/` and ships `spec.md`, `plan.md`, `tasks.md`, plus `model.md` if the feature touches an API surface.

**Independent Test**: A reviewer can walk from `001-<slug>/spec.md` → `plan.md` → `tasks.md` → the implementing PR and trace every acceptance criterion through to code and tests.

- [ ] T021 [US2] [vmop-3826] Communicate the new workflow on the internal `#vm-operator` channel and at the next architecture sync; link to `.sdd/memory/sdd-standards.md`.
- [ ] T022 [US2] [vmop-3827] When opening the next non-trivial feature, allocate `.sdd/specs/001-<slug>/` and populate `spec.md` before any code lands.
- [ ] T023 [US2] [vmop-3827] Use `001-<slug>/plan.md` to perform the constitution check explicitly; record any complexity-tracking entries.
- [ ] T024 [US2] [vmop-3827] Decompose `001-<slug>/plan.md` into `001-<slug>/tasks.md` with `[P]` markers for safely parallel work.
- [ ] T025 [US2] [vmop-3828] Update `001-<slug>/spec.md` status to `Implemented` and verify every task box is checked in the merging PR.

## Phase 5 — Polish & Cross-Cutting Concerns

**Purpose**: Smooth out the framework once it has at least one real feature behind it.

- [ ] T026 [P] [vmop-3830] Add `hack/new-spec.sh` (or equivalent helper) that allocates the next `NNN`, scaffolds `spec.md` / `plan.md` / `tasks.md` from the templates in `.sdd/memory/sdd-standards.md`, and prints the next feature branch name.
- [ ] T027 [P] [vmop-3831] Add `.sdd/specs/README.md` (or generate it from a script) listing every feature directory and its current status.
- [ ] T028 [P] [vmop-3832] Add a CI check that flags PRs touching `api/`, `controllers/`, `webhooks/`, `pkg/`, or `services/` without a corresponding `.sdd/specs/NNN-slug/` directory (or an explicit "trivial change" label).
- [ ] T029 [P] [vmop-3833] Cross-link `docs/README.md` (user-facing docs) to `.sdd/memory/constitution.md` so external readers can find the engineering rules.
- [ ] T030 [P] [vmop-3834] Backfill specs (opportunistically) for in-flight or recently-shipped features that would benefit from an authoritative `.sdd/specs/` entry. Track each backfill as its own `.sdd/specs/NNN-slug/` with `Status: Implemented` from day one.

## Phase 6 — Gap Remediation (from `report.md`)

**Purpose**: Close every finding from the baseline gap analysis (`report.md`). All tasks in this phase are sub-tasks of the existing `vmop-3829` (Polish & tooling) story and can be worked independently unless a dependency is noted.

- [ ] T031 [vmop-3836] Fix 11 `pkgcnd → pkgcond` import-alias violations via `make fix`; confirm `make lint-go-full` is clean. _(Finding G — Minor)_
- [ ] T032 [vmop-3837] Remove the stale "not yet linter-enforced" aliases sub-section from `architectural-standards.md`. _(Finding K — Minor)_
- [ ] T033 [vmop-3838] Rewrite `testing-standards.md` "Test File Naming" to describe the single-file pattern and Ginkgo `Label()` mechanism; remove all references to the `_unit_test.go` / `_intg_test.go` split. Prerequisite for T035. _(Findings F, N — Major)_
- [ ] T034 [vmop-3839] Update `constitution.md`: add `api-docs/` to the canonical layout tree, clarify the non-`vmoperator.vmware.com` controller and webhook grouping rules, and add the 5 missing sub-modules to the "Sub-modules" table. _(Findings C, D.1, E, L — Minor)_
- [ ] T035 [vmop-3840] Open `.sdd/specs/NNN-test-naming-migration/` and execute the 76-file migration (35 `_unit_test.go` + 41 `_intg_test.go` → `_test.go`) in batched, reviewable PRs. Depends on T033. _(Finding F — Major)_
- [ ] T036 [vmop-3841] Add the canonical `vmoperator.vmware.com/virtualmachinereplicaset` finalizer with a legacy-migration path; decide and document the `-finalizer` suffix policy in `constitution.md`. _(Finding M — Minor)_
- [ ] T037 [vmop-3842] Run `make lint-markdown`, `make lint-shell`, and `make typos` on a Docker-capable workstation; fix any findings; update `report.md` Finding O. _(Finding O — procedural)_
- [ ] T038 [vmop-3843] Run `make modules`, `make generate-go`, and `make generate-manifests` on a clean branch; fix any drift; update `report.md` Finding O. _(Finding O — procedural)_

---

## Dependencies & Execution Order

### Phase dependencies

- **Phase 1 (Setup)** — no dependencies; everything in this phase can be done in any order, but all of it must land before Phase 2's constitutional checkpoint.
- **Phase 2 (Foundational)** — depends on Phase 1; blocks every later spec because subsequent specs link into `constitution.md` and `sdd-standards.md`.
- **Phase 3 (US1 — Bootstrap)** — depends on Phase 2; produces the canonical `.sdd/specs/000-sdd/` directory that all future specs imitate.
- **Phase 4 (US2 — Adopt)** — depends on Phase 3; cannot start until the framework can be pointed at by reviewers as the reference.
- **Phase 5 (Polish)** — depends on Phase 4 having at least one iteration through the real workflow so the helpers reflect lived experience.
- **Phase 6 (Gap remediation)** — independent of Phase 4/5; the gap-remediation tasks are driven by `report.md` findings and may proceed in parallel with the Phase 4/5 workflow adoption. The single intra-phase dependency is T033 before T035 (the doc fix must land before the mass migration begins).

### Within each user story

- Spec before plan; plan before tasks; tasks before implementation.
- Updates to a spec / plan / tasks file ship in the **same PR** as the code that changed the assumption.

### Parallel opportunities

- All Phase 1 tasks marked `[P]` are safe to do in parallel (different files).
- Phase 5 polish tasks are independent of each other and may be done in any order.

---

## Notes

- `[P]` = different files, no dependencies between the tasks.
- `[USx]` = traces the task back to the user story in `spec.md`.
- Commit after each task or logical group; do not bundle the constitution rewrite with the per-feature spec in one commit.
- This `tasks.md` itself follows the rule it defines: when a follow-up task is discovered, add it here (don't open a parallel TODO).
