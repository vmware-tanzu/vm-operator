# Implementation Plan: E2E code coverage

- **Spec**: [spec.md](spec.md)
- **Epic**: TBD
- **Date**: 2026-10-20

## Summary

Build a separate instrumented manager image and a coverage-capable E2E runner alongside the normal images in the existing Cayman build target. Quarantined workloads in the standard `CODE_COVERAGE` stage deploy once per Supervisor, run the existing suites with raw collection only, and run one offline aggregation job after collection finishes. Ordinary E2E jobs retain their original configuration.

## Technical context and structure

- Root Go module and existing Go toolchain; standard-library-only runtime exporter under `test/coverage` and `cmd/coverage-export`.
- `main_coverage.go` is included only with the `e2e_coverage` build tag. The hook runs at the start of main after Go initializes coverage metadata; `main_no_coverage.go` supplies a production no-op. A Unix socket in the writable coverage volume serves snapshots from the actual manager process. An uninstrumented helper streams archives through `kubectl exec`; no shell, network listener, or tar utility is added to the manager image.
- `hack/e2e/coverage.py` deploys the image, collects all Deployment-owned replicas, retains only the newest cumulative snapshot per process, validates revision and completeness, and generates reports.
- Separate build outputs preserve `bin/manager`, the normal release image, and its archive. The coverage runner bundles source and the matching `covdata` and `cover` executables from the manager build toolchain, avoiding runtime downloads.
- `image-build-coverage` builds the normal manager image first by default and layers coverage binaries onto it. An explicitly supplied `COVERAGE_BASE_IMAGE` reuses an existing base without rebuilding it, including when the integration build has already produced the normal image. Both images use the selected `GOARCH` and Linux target OS.
- The existing Cayman target publishes both normal and coverage images. Component-owned metadata derives the complete VM Operator coverage graph from existing enabled suites, using the standard `COV_WL` tag and `-code-cov` suffix. Real clones record `original_test_suite`; synthetic setup/report jobs do not pretend to be original suites. Every generated coverage job is quarantined. UTS requires the canonical `build-cayman_vm-operator-release` record, so its template adds `COV_WL` while retaining the ordinary tags and gating behavior. The launcher names that build and all coverage jobs explicitly in a normal precheckin, without overriding the pipeline name.
- `CODE_COVERAGE=enabled` activates the runner's existing collection path; `E2E_COVERAGE=true` remains a local compatibility alias. Coverage images and report tools must still be present; a stage flag cannot instrument an ordinary binary.
- Tera already consumes the Cayman metadata artifact and copies its generated jobs before applying its own coverage transforms. Use a component-specific JSON filename to avoid collisions with Tera's `coverage_tests.json`. Retain the existing `wcp`, `vsphere`, and `vcf` groups and add a local `COV_WL` branch configuration. No new Tera transform call is required for these already-generated variants; cloning them again would duplicate the graph.
- No new target or component alias is introduced. Publish the upstream implementation, advance the Cayman submodule pointer with the build/UTS changes, and validate a sandbox from the existing target before live execution.
- The final job reads suite artifacts from their UTS log paths on the shared log mount. It validates every expected suite status, source revision, image reference, process identity, and snapshot timestamp. For repeated snapshots of one process it selects the latest, retaining earlier process identities across restarts. It reports partial coverage with an explicit failed completeness status when evidence is missing.
- Suites set `E2E_COVERAGE_REPORT=false` to archive raw data before callbacks. The final job owns merge, HTML, and summary generation. It uses the same coverage runner image so source and report tools match the instrumented build; it needs no Supervisor connection. The Cayman submodule pointer and pipeline changes will be published separately from this upstream implementation.

## Constitution check and API strategy

No CRDs, API versions, controller flow, vSphere calls, or production feature flags change. Instrumentation is test infrastructure compiled out of production. The build tag and explicit pipeline environment opt-in provide the gate; this does not require a product capability. Internal service URLs belong only in Cayman. Epic/story assignment is required before the draft PR is merged.

## Test strategy

Exercise a real coverage-instrumented fixture through the socket/export helper, process restart, repeated snapshots, corrupt exports, and report generation. Test runner pass/fail/callback ordering and deployment selection with a simulated Kubernetes CLI. Syntax-check shell, lint Go, generate/validate UTS metadata, and verify build commands. A VDS smoke run with a deliberate failing test is the live acceptance check and must be recorded separately from local tests.

Run the local coverage-tool tests in a separate GitHub Actions workflow filtered to changes in coverage tooling/tests, runner scripts, manager entry points, build configuration, Go module dependencies, or the workflow itself. Preserve the existing PR and main-branch push scope and allow manual dispatch. Unrelated check-ins do not start the coverage-tool workflow; the standard CI workflow keeps its existing jobs.

## Rollout and limitations

Validate the coverage workloads explicitly by name before publication; published `COV_WL` metadata belongs to the standard `CODE_COVERAGE` stage. Do not add coverage jobs to ordinary Smoke/CF/EF tags or change-selection policies. Quarantine excludes coverage jobs from gating, so inspect their individual results and the final completeness artifact instead of treating a green pipeline summary as proof of complete coverage. The existing build target still builds both image sets; coverage image build failures can therefore fail ordinary builds too. Report cumulative testbed coverage, including setup and concurrent activity; do not clear counters. Every successful export preserves evidence outside the pod; a force-killed process may lose execution since its last snapshot. Missing exports or reporter failures must never appear as successful zero-percent results. Normal release artifacts remain uninstrumented; their build now also produces the separate coverage artifacts.
