# Feature Specification: VM Operator Specification Driven Development

- **Feature branch**: [`feature/sdd`](https://github.com/akutz/vm-operator/tree/feature/sdd/)
  - **Fork**: `akutz/vm-operator`
  - **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-05-27
- **Status**: In Progress (Architecture phase; code not yet started)
- **Epic**: vmop-3820

---

# Overview

This feature describes how to use specification driven development (SDD) for all new features in this repository going forward.

# Goals

* All specification driven development (SDD) files should be located under a directory at the root of the project named `.sdd`.
* There should be a markdown-formatted constitution that describes the rules for working in the repository located at `.sdd/memory/constitution.md`.
* All specifications **MUST** be located in distinct directories located under `.sdd/specs`.
* All specification directories **MUST** adhere to the format `XYZ-FEATURE_NAME` where `XYZ` is an auto-incrementing scalar value that starts at `000` and increases with each new feature, ex. `.sdd/specs/000-sdd`.
* All specifications **MUST** include the following files:
    * `spec.md` -- The feature specification.
    * `plan.md` -- A high-level plan for implementing the feature.
    * `tasks.md` -- The plan broken down into executable tasks.
* All specifications **SHOULD** include the following files:
    * `model.md` -- Any data model(s) / API(s) introduced as part of the feature.
    * `research.md` -- The research performed while designing the feature that provides context for other engineers.
* All AI-model specific files, such as `.cursor/rules/*.mdc` should serve only as proxies to files located in the generic path. For example, `.cursor/rules/architectural-standards.mdc` should link to `.sdd/memory/architectural-standards.md`.

# Non-goals

* **Replacing** `docs/`, design docs in Google Docs, or the public RTD site—Spec Kit artifacts are **engineering-local** unless the team promotes them.
* **Mandating** SDD for every one-line fix or dependency bump.
* **Treating generated plans as authority** over production incidents: operational truth still wins; specs should be **updated** after incidents when they encode wrong assumptions.

# Gap remediation (Phase 6)

After the framework bootstrap landed, a baseline gap analysis was run and documented in [`report.md`](./report.md). The findings were triaged into eight follow-up tasks (T031–T038, `vmop-3836`–`vmop-3843`) under the existing `vmop-3829` (Polish & tooling) story. Closing those tasks is in-scope for the overall `vmop-3820` epic but out-of-scope for the Phase 1–3 bootstrap PR. See [`tasks.md`](./tasks.md) Phase 6 for details.
