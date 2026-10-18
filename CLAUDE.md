# VM Operator — Claude Code Guidelines

This project follows Spec-Driven Development (SDD). The authoritative guidelines live in `.sdd/memory/`. This file is a proxy — **do not duplicate guidance here**; update `.sdd/memory/*.md` instead.

<!--
Loading model (verified with Claude Code 2.1.302; de-duplication verified with 2.1.303):
- @imports below load at launch. Keep this list to rules that matter in every session.
- Nested CLAUDE.md files load on demand, together with their @imports, the first time Claude touches a file in that directory.
- An @import already loaded in the session is not expanded again, so several nested CLAUDE.md files can safely import the same guide.
- .claude/rules/*.md with `paths:` frontmatter load on demand, but an @import inside such a rule expands at launch anyway. Use plain-text "Read <file>" pointers there instead.
-->

Before starting any non-trivial task, read `.sdd/INDEX.md` and follow its "When to read" triggers for task-specific guidance. If the files you are touching correspond to an in-progress spec, read that spec's `spec.md` and `plan.md` before writing code — the spec is the acceptance criteria.

## Always-active rules

@.sdd/memory/constitution.md

@.sdd/memory/architectural-standards.md

@.sdd/memory/e2e-sync-with-changes.md

@.sdd/memory/commit-message-standards.md

@.sdd/memory/pull-request-standards.md

## Loaded on demand

Opening a matching file loads the guide or tells you to read it. When a guide applies but no matching file is opened — planning, reviewing from `git diff` or `gh pr diff` output, or editing shared test helpers — read it directly per the index's "When to read" column.

- `.sdd/memory/operator-best-practices.md` — `controllers/`, `pkg/providers/`, `pkg/vmconfig/`, `pkg/errors/`, `pkg/util/kube/cource/`, `services/` (reconcile loop, pkgerr requeue semantics, VM update order, `WithVCOpID`, cource).
- `.sdd/memory/testing-standards.md` — `**/*_test.go`.
- `.sdd/memory/e2e-testing.md` — `test/e2e/`.
- `.sdd/memory/sdd-standards.md` — no automatic trigger; read it when planning or reviewing non-trivial work, or when creating or editing anything under `.sdd/specs/`.
