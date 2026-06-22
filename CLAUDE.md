# VM Operator — Claude Code Guidelines

This project follows Spec-Driven Development (SDD). The authoritative guidelines live in `.sdd/memory/`. This file is a Claude Code proxy that maps those rules into the contexts where they apply, following the same pattern as `.cursor/rules/*.mdc`.

> **Do not duplicate guidance here.** Update `.sdd/memory/*.md` instead; this file and the `.cursor` rules will stay current automatically via the `@` imports below.

---

## SDD index

Before starting any non-trivial task, read `.sdd/INDEX.md`. If the files you are touching correspond to an in-progress spec, read that spec's `spec.md` and `plan.md` before writing code — the spec is the acceptance criteria.

---

## Always-active rules

@.sdd/memory/constitution.md

@.sdd/memory/commit-message-standards.md

@.sdd/memory/pull-request-standards.md

@.sdd/memory/e2e-sync-with-changes.md

---

## Go source files

Applies when editing any `**/*.go` file that is not a test file.

@.sdd/memory/architectural-standards.md

---

## Controller and provider files

Applies when editing `controllers/**/*.go`, `pkg/providers/**/*.go`, `services/**/*.go`, and related packages.

@.sdd/memory/operator-best-practices.md

---

## Test files (`**/*_test.go`)

@.sdd/memory/testing-standards.md
