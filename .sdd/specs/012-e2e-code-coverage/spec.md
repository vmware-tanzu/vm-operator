# Feature Specification: E2E code coverage

- **Feature branch**: `e2e-code-coverage`
- **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-10-20
- **Status**: Draft
- **Epic**: TBD (required before merge)

## Goals

- The standard `CODE_COVERAGE` stage MUST run the applicable E2E coverage variants and report the operator statements exercised on a Supervisor, including controller and webhook execution on every replica. Local precheckin validation MUST explicitly select these variants.
- The existing build target MUST publish separate coverage images alongside normal images. Ordinary E2E pipelines MUST retain their existing images, resource limits, test selection, and dependencies. Building the additional images adds build time; coverage execution remains opt-in.
- Coverage MUST be explicitly enabled for test deployments and MUST NOT change normal release images.
- Reports MUST identify their source revision and distinguish missing coverage from zero coverage.
- Collection MUST run for passing and failing suites, preserve the test result, and finish before reporting suite completion. One final reporting job MUST combine available suite artifacts and disclose missing or failed collections.
- Concurrent suites sharing a Supervisor MUST NOT reset each other's counters or redeploy the operator independently.
- Periodic snapshots SHOULD retain evidence across operator replacement. Reports MUST disclose incomplete collection; a hard kill between snapshots can lose the most recent activity.
- Coverage collection MUST stream snapshots over loopback HTTP without a coverage volume, filesystem writes, or `GOCOVERDIR`. The existing E2E runner owns periodic collection and artifact storage and reaches each manager through Kubernetes port-forwarding. The dedicated coverage build deliberately exposes an unauthenticated node-local endpoint on host-networked Supervisors; production builds MUST NOT include this listener.

## Non-goals

- Measuring test harness execution, third-party dependencies, or unrelated binaries.
- Attributing shared-Supervisor execution to an individual test or concurrent suite.
- Introducing a coverage percentage gate or changing production reconciliation behavior.

## Acceptance criteria

1. An explicitly enabled test deployment produces nonempty raw coverage, a statement summary, and a source-annotated HTML report for its exact build.
2. A failing test still produces coverage and retains its failure exit code.
3. Repeated snapshots from one process are replaced rather than counted repeatedly; snapshots from replaced processes remain available.
4. A missing replica, wrong source revision, failed export, or failed report generation is visible in a machine-readable status artifact and fails coverage validation.
5. An ordinary operator build contains no coverage export listener or helper executable.
6. Pipeline metadata orders one setup operation per testbed before suites and lets collection finish before teardown.
7. Coverage uses isolated provision/setup/suite/teardown jobs with the standard `COV_WL` tag, `-code-cov` names, and `CODE_COVERAGE=enabled` runner setting. They remain quarantined during bring-up so their results do not gate regular recommendations. Coverage jobs are excluded from ordinary workload tags and change-selection policies; existing suite quarantine choices are preserved.
8. Suites publish raw snapshots and status without generating individual HTML reports. The final reporting job selects the newest cumulative snapshot for each process across suites before merging, validates source and image identity, and writes a consolidated report even when some suites fail. Missing evidence fails completeness validation.
9. Coverage jobs reuse the existing release build target and canonical build record; no new target registration, component alias, or custom coverage stage is required. The build record adds `COV_WL` while retaining its ordinary tags and gating behavior. Normal and coverage images have distinct names and outputs. Coverage images and report tools are pinned to the same source revision.
10. Setup changes the manager image and coverage revision annotation without adding volumes, mounts, environment variables, or security settings. The server works with a read-only Linux root filesystem, listens only on IPv4 loopback, and preserves valid Go reports after process replacement. Port-forwards MUST close after successful and failed collection; startup errors and timeouts MUST fail collection visibly.

## Open questions

- Epic and story identifiers must be assigned before merging. The upstream PR contains VM Operator implementation, tests, and documentation; Cayman build and pipeline integration will be published separately.
- Live Supervisor validation requires a disposable testbed and published sandbox images.
