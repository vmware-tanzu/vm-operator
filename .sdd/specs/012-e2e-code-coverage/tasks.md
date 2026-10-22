# Tasks: E2E code coverage

- **Spec**: [spec.md](spec.md)
- **Plan**: [plan.md](plan.md)
- **Epic**: TBD; assign epic/story identifiers before merge.

- [x] T001 Create isolated VMOP and Cayman worktrees without commits.
- [x] T002 Implement test-build runtime snapshots (`main.go`, `main_coverage.go`, `main_no_coverage.go`, `test/coverage/coverage.go`, `cmd/coverage-export/main.go`).
- [x] T003 Add separate instrumented build/image outputs (`Makefile`, `Dockerfile.coverage`, `Dockerfile.e2e`).
- [x] T004 Implement deployment, collection, reporting, and runner lifecycle (`hack/e2e/coverage.py`, `hack/e2e/run-e2e.sh`).
- [x] T005 Publish normal and coverage images from the existing Cayman target and generate the standard coverage graph (`vm-operator/Makefile`, `support/uts/bin/generate_coverage.py`, `support/uts/auto_generated/testsuites/vm-operator-code-coverage.json`).
- [x] T006 Validate runtime exports, collector failures/restarts, runner results, and generated pipeline metadata.
- [x] T007 Document operation (`hack/e2e/README.coverage.md`, Cayman `support/uts/README.md`).
- [ ] T008 Validate full container/gobuild packaging and run VDS smoke against a disposable Supervisor, including failure and replacement scenarios. Requires sandbox images and testbed access; not run locally.
- [x] T009 Use the existing `CODE_COVERAGE` stage, `COV_WL` tag, `-code-cov` names, and canonical release build while retaining ordinary E2E job configuration (acceptance criteria 7 and 9).
- [x] T010 Collect raw suite artifacts, publish shared locations before callbacks, and aggregate once with process deduplication and completeness checks (acceptance criterion 8).
- [ ] T011 Advance the Cayman submodule pointer with build/metadata support after upstream publication. Ensure the standard stage consumes the updated Cayman metadata through Tera's Supervisor pin. Cayman changes are outside the upstream PR and remain uncommitted.
- [ ] T012 Validate the standard stage end to end on both enabled topologies, including access to suite artifacts from the final reporting pod and the desired quarantine behavior.
- [x] T013 Limit coverage-tool CI to relevant file changes using `.github/workflows/coverage-tools.yml`, with manual dispatch available. Remove the unconditional job from the normal CI workflow.
- [x] T014 Make `image-build-coverage` build the regular image automatically unless `COVERAGE_BASE_IMAGE` is supplied, and verify build ordering and architecture selection (`Makefile`, `hack/e2e/README.coverage.md`).

## Local validation (2026-10-22)

- The generated graph has 13 quarantined jobs: two provisioning jobs, two setup jobs, six suites (smoke/core/extended on VDS and VDS-SSV), two teardown jobs, and one combined reporter. It depends on the existing canonical release build. Other suites retain their quarantine settings.
- UTS generation and live metadata validation passed: 51 tests, two policies, zero errors. No new GoBuild target or registration is required. The canonical release build adds `COV_WL` while retaining its ordinary tags and gating behavior.
- All 34 local tests passed: 26 exporter/collector/runner integration tests and eight pipeline/build checks. Runner coverage includes the standard `CODE_COVERAGE=enabled` flag, the local `E2E_COVERAGE=true` alias, and ordinary runs with coverage disabled. Shell/Python syntax and whitespace checks passed; the generator remains compatible with Python 3.5.
- The separate coverage-tool workflow parses as YAML; representative filter checks select 19 relevant paths and exclude six unrelated paths. PR/push branch scope, manual dispatch, and the unchanged test command were checked. The normal `ci.yml` is identical to its pre-change version. GitHub-hosted trigger execution has not been exercised.
- Earlier native manager and Linux/amd64 cross-builds passed, as did Go lint for the coverage-enabled manager/export packages. The real Go exporter/collector tests exercise process replacement, ownership checks, source identity, offline reporting, callback ordering, partial results, and malformed artifacts.
- Build dry-runs verify normal images and separately named coverage images are built/published by the same target. Pipeline checks cover quarantine, explicit selection, gateway dependencies, and cleanup/reporting after failed setup or tests.
- Coverage-target dry-runs verify automatic regular-image builds for amd64 and arm64, matching image names/tags and separate archives, and reuse of an explicit base from either command-line or environment settings. The normal image target remains uninstrumented. Full container builds remain unvalidated.
- No overhead measurement, sandbox image build, Supervisor deployment, or live shared-artifact validation has been performed. The 500m CPU / 1 GiB limits apply only to coverage jobs and remain provisional. Building extra coverage images adds build time; ordinary E2E jobs still use normal images.
