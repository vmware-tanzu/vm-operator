# Operator coverage during E2E tests

Coverage measures the manager running on the Supervisor: controllers, services, webhooks, internal packages, and VM Operator API code linked into that binary. It does not measure the test harness, external dependencies, or the separate web-console-validator binary.

The standard manager image and E2E runtime are unchanged. Coverage uses a separate manager image built with atomic counters and the `e2e_coverage` tag, plus an E2E runner containing the matching Go reporting executables and source. No Go installation, module download, shell, or tar program is needed inside the manager.

Replacing the manager image is sufficient to enable export: no coverage volume, mount, or `GOCOVERDIR` setting is needed. The coverage-enabled manager serves `GET /snapshot` at `http://127.0.0.1:18081` on both Linux and macOS. It captures counters in memory; the existing test runner opens a short-lived `kubectl port-forward` for each snapshot, downloads the archive, closes the forward, and writes artifacts outside the pod. No export-helper executable is added to the image.

The endpoint is intentionally unauthenticated for dedicated internal coverage deployments. On a host-networked Supervisor, other processes on the same node can read it. It binds only to IPv4 loopback; no Service, ingress, or Deployment port declaration is required. The runner needs Kubernetes permission to create `pods/portforward`. A port conflict fails manager startup clearly. To select another port, set `E2E_COVERAGE_PORT` consistently on the manager and collector; setup does not add that environment variable automatically.

## Build

Build the coverage manager image and runner from the same source checkout. The coverage target builds the regular manager image first automatically:

```sh
GOARCH=amd64 make image-build-coverage CRI_BIN=docker
make e2e-image-build CRI_BIN=docker \
  E2E_IMAGE=vmoperator-e2e-coverage E2E_BUILD_TARGET=runtime-coverage
```

To reuse an existing normal manager image instead of rebuilding it, set `COVERAGE_BASE_IMAGE` explicitly:

```sh
make image-build-coverage GOARCH=amd64 CRI_BIN=docker \
  COVERAGE_BASE_IMAGE=registry.example/vmoperator-controller:build-id
```

The base image must match the target architecture and be compatible with the manager build. `COVERAGE_IMAGE` and `COVERAGE_IMAGE_FILE` select the separate output image/archive. `manager-coverage-only` creates `bin/coverage/` without replacing the release binaries. The reporter tools are built with the manager's Go toolchain, independently of the toolchain used to compile the E2E test binary.

Push/load the two coverage images into a registry reachable by both the Supervisor and the E2E runner. Use an immutable image reference for a single build.

## Pipeline ordering

Run setup once per testbed after provisioning and before any suites. The runner needs its existing Supervisor administrator kubeconfig, including permission to patch the Deployment, list its ReplicaSets/pods, and exec into the manager container.

```sh
export E2E_COVERAGE_IMAGE=registry.example/vmoperator-coverage:build-id
export E2E_COVERAGE_SOURCE=/vm-operator/coverage-source
export E2E_ARTIFACT_FOLDER=/path/to/unique/run-artifacts
./hack/e2e/setup-coverage.sh
```

Setup patches the manager image, pull policy, and coverage revision annotation. It leaves volumes, mounts, environment variables, and security settings unchanged, waits for rollout, and validates a real snapshot from every replica. The original Deployment is recorded in `coverage-setup/deployment-before.json` for inspection. Coverage deployments are intended for disposable testbeds; setup does not automatically restore the old image.

For a manually deployed coverage image, start a forward in one terminal:

```sh
kubectl -n vmware-system-vmop port-forward --address=127.0.0.1 pod/MANAGER_POD 18081:18081
```

Download a snapshot in another terminal, then stop the forward when finished:

```sh
curl --fail --silent --show-error http://127.0.0.1:18081/snapshot -o snapshot.tar
```

The archive contains Go metadata, cumulative counters, and build/process identity. A coverage manager running directly on your laptop uses the same URL without port-forwarding. The Go runtime may log `warning: GOCOVERDIR not set, no coverage data emitted` at startup. This refers to automatic filesystem output; the explicit in-memory snapshot API still works. Leave `GOCOVERDIR` unset for streamed collection.

Each suite then runs with:

```sh
export E2E_COVERAGE=true
./hack/e2e/run-e2e.sh prebuilt
```

The runner samples every 60 seconds, then collects once more after the tests finish. A dedicated coverage pipeline sets `E2E_COVERAGE_REPORT=false`: each suite archives raw snapshots and writes status, while one final job generates the combined report. For a standalone local run, reporting remains enabled by default. The suite callback is sent only after collection, artifact publication (when configured), and optional support-bundle collection. A failed test keeps its exit code; if tests pass but coverage fails, the runner exits with code 2. A separate coverage subtest carries collection/reporting errors. Missing coverage fails preflight rather than producing a successful zero-percent report.

Reserve time between the Ginkgo timeout and the container deadline for final export, publication, and any support-bundle collection. Coverage jobs currently use 500m CPU and 1 GiB memory limits as provisional allowances; runtime overhead has not been measured. Ordinary E2E jobs retain their original limits.

## Dedicated pipeline and aggregation

The upstream change provides the coverage binaries, image targets, scripts, and tests. Build-system publication and pipeline metadata are separate integration changes; this PR alone does not activate a scheduled coverage pipeline. The following describes the intended integrated workflow.

Coverage execution is opt-in. The existing integration build target publishes separate coverage images alongside normal images, adding build time; ordinary E2E jobs continue to use uninstrumented images. Coverage variants join the standard `CODE_COVERAGE` stage with `COV_WL` tags and `-code-cov` names. The stage's `CODE_COVERAGE=enabled` environment setting activates the runner's collection path; `E2E_COVERAGE=true` remains supported for local runs. Neither setting can instrument an already-built normal manager.

Coverage runs all enabled suites, with separate testbeds, one setup per topology, collection in each suite, and one final reporting job. Existing suite quarantines remain effective. The generated coverage jobs are also quarantined during bring-up, so inspect their individual results and the combined completeness status instead of treating a green pipeline summary as proof of successful collection. Precheckin validation explicitly selects all coverage job names without creating a custom pipeline name. Report processing uses saved artifacts and can finish after the testbeds have been torn down.

For local aggregation, give each suite a fresh output directory and run:

```sh
export E2E_COVERAGE_INPUTS='{"core":"/results/core/coverage","extended":"/results/extended/coverage"}'
python3 hack/e2e/coverage.py --output /results/combined aggregate
```

The reporter requires the same `E2E_COVERAGE_SOURCE`, matching report tools, and immutable `E2E_COVERAGE_IMAGE` reference as the collectors. It checks each expected suite's status, revision, and image, then picks the newest snapshot for every process across all suites. Earlier process identities are preserved across restarts. A failed test with successful collection contributes valid coverage. A missing or failed collection produces an incomplete report, records the error in the combined `status.json`, and exits with code 2; available valid evidence is still reported.

Pipelines may provide `E2E_COVERAGE_PUBLISH_URL` and `E2E_COVERAGE_TEST_FK` to publish a test-blob manifest containing the suite's shared artifact directory before its callback. In that case `E2E_COVERAGE_INPUTS` maps suite names to their test-blob URLs. The reporter reads the saved files from the shared log mount, validating that their directories are within `E2E_COVERAGE_LOG_ROOT` (default `/cpbu/logs`). It does not connect to the Supervisor. Artifact directories must remain available until the reporting job finishes.

## Results and interpretation

Each collection-only suite writes raw snapshots, `coverage-raw.tar.gz`, and `status.json` under its artifact directory's `coverage/`. The final reporting job (or a standalone run with reporting enabled) additionally writes:

| Artifact | Contents |
| --- | --- |
| `coverage.html` | Source-annotated statement coverage |
| `coverage-summary.txt` | Function and total statement coverage |
| `coverage.out` | Portable Go coverage profile with module source paths |
| `coverage-raw.tar.gz` | Latest snapshot of each observed manager process |
| `raw/<process-id>/snapshot.json` | Source revision, toolchain, timestamp, process identity |
| `raw/<process-id>/pod.json` | Pod UID, container identity, image digest, restart count |
| `status.json` | Test exit code, coverage result, collection warnings/errors, scope |

Counters are cumulative for the lifetime of each process. Concurrent suites on the same Supervisor share execution: these reports include startup, background reconciliation, and other suites' activity. They cannot attribute statements to an individual test. No collector resets counters. Repeated snapshots replace earlier snapshots from that process so merges do not multiply counts; snapshots of replaced processes are retained.

A process killed between polls can lose its most recent activity. Periodic collection failures appear in `sampling_warnings`; missing replicas or export failures on the final pass fail coverage validation. Raw files remain available when report generation fails. Use a fresh artifact directory per run; previously stored snapshots with a different source revision are rejected.

Use the `aggregate` action to combine suites from the same coverage build. Concatenating profiles or blindly merging repeated snapshots from shared testbeds inflates execution counts.

## Configuration

| Variable | Default |
| --- | --- |
| `E2E_COVERAGE` | `false` |
| `E2E_COVERAGE_IMAGE` | Required for coverage setup/runs |
| `E2E_COVERAGE_NAMESPACE` | `vmware-system-vmop` |
| `E2E_COVERAGE_DEPLOYMENT` | `vmware-system-vmop-controller-manager` |
| `E2E_COVERAGE_CONTAINER` | `manager` |
| `E2E_COVERAGE_PORT` | `18081`; overrides must match on manager and collector |
| `E2E_COVERAGE_INTERVAL` | `60` seconds |
| `E2E_COVERAGE_REPORT` | `true`; dedicated pipeline suites set `false` |
| `CODE_COVERAGE` | `enabled` activates collection in the E2E runner; unset by default |
| `E2E_COVERAGE_INPUTS` | Required JSON map of suite names to directories/manifest URLs for aggregation |
| `E2E_COVERAGE_PUBLISH_URL` | Unset; optional test-blob publication endpoint |
| `E2E_COVERAGE_TEST_FK` | Required when publication is configured |
| `E2E_COVERAGE_LOG_ROOT` | `/cpbu/logs`; permitted root for manifest artifact paths |
| `E2E_COVERAGE_SOURCE` | `/vm-operator/coverage-source` |
| `E2E_COVDATA_TOOL` / `E2E_COVER_TOOL` | `covdata` / `cover` on PATH |

## Validation

```sh
python3 -m unittest discover -s hack/e2e/tests -p test_coverage.py -v
python3 -m unittest discover -s hack/e2e/tests -p test_coverage_linux.py -v
```

These tests build real instrumented Go programs and exercise manager startup, loopback HTTP snapshots, port-forward readiness/failures/cleanup, offline reports, pod ownership, replacement, invalid archives, revision mismatches, test failures, and callback ordering. They need local loopback access. The Linux tests additionally need Docker (or `COVERAGE_TEST_CRI=podman`) and build a disposable Python-based test image. They verify read-only operation without added capabilities, loopback-only binding, intentional node-local unauthenticated access, port conflicts, and distinct ports for managers sharing a network namespace. The Python test image is separate from manager images. The `coverage-tools` GitHub Actions workflow runs both suites for changes to coverage tooling/tests, runner scripts, manager entry points, build configuration, Go module dependencies, or the workflow itself. It retains the usual PR/main-push branch scope and supports manual dispatch. Unrelated check-ins do not start it.

Direct VDS Supervisor validation confirmed image-only setup, live exports without `GOCOVERDIR`, retained snapshots across rollout, a dry-run webhook update, periodic/final collection, and reports after a synthetic test failure. Full pipeline acceptance still requires sandbox packaging and VDS smoke, all-replica collection, and shared artifacts before teardown. The direct test does not establish pipeline registry reachability, FIPS-toolchain compatibility, or instrumentation overhead under a full workload.
