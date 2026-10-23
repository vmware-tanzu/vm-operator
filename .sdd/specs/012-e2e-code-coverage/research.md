# Research: E2E code coverage

## Existing coverage stage and build target

The user chose the existing `CODE_COVERAGE` stage and `cayman_vm-operator` build target. The target publishes separately named coverage images alongside normal images. This adds build time without enabling instrumentation in normal E2E executions. Coverage jobs are quarantined during bring-up, independently of their stage membership.

Tera's `bora/support/uts/bin/CODE_COVERAGE_PIPELINE.md` documents `COV_WL`, the `-code-cov` suffix, and optional target swapping. Its `codecoverage_framework_handlers.py` sets `CODE_COVERAGE=enabled` for the `k8spod` framework. The runner accepts that standard flag and retains `E2E_COVERAGE=true` as a local alias. Neither flag changes the instrumentation of an already-built binary.

The canonical build record must be named `build-cayman_vm-operator-release`; UTS rejects a second suffixed build record pointing to that same target/build type. Gateway validation also requires its `COV_WL` tag. Add that tag to the original template while retaining the ordinary workload tags and gating behavior.

## Component metadata integration

Tera's `bora/support/gobuild/specs/vcf_uts_meta.py` already includes `cayman_vm-operator-uts-metadata` using the Supervisor component pin. Its `extract_gobuild_aliases_vcf.py` copies component-generated metadata before applying central coverage transforms. VM Operator therefore publishes its coordinated graph in a component-specific JSON filename and uses the standard stage tags. It does not need another transform that would clone the same jobs again.

A component generator is needed because the graph has one installation per shared Supervisor, coverage collection in concurrent suites, failure cleanup, and a single final reporter. A generic per-suite flag injection alone does not establish these dependencies. The local launcher explicitly names all quarantined jobs and the canonical build in a normal precheckin; it does not create or rename a pipeline.

## Collection and aggregation

WIKI page 3126842702 describes dedicated coverage workloads and post-run aggregation against matching build artifacts in other projects. VM Operator uses Go runtime coverage and one report across all currently enabled E2E suites.

Snapshots are cumulative within a manager process. Concurrent suites on one Supervisor overlap: aggregation selects the newest snapshot for each process identity instead of adding repeated cumulative snapshots. Different process identities survive restarts and remain separate merge inputs. Failing tests still contribute valid evidence; missing or failed collection makes the final report explicitly incomplete.

## Streaming without a coverage volume

The user chose periodic collection from the existing runner instead of staging files in the manager pod. Go's `runtime/coverage.WriteMeta` and `WriteCounters` accept writers, so metadata can be cached in memory and counters captured per request, then streamed as tar entries. Go coverage format version 1 embeds the metadata hash at bytes 24–39; matching toolchain integration tests verify that the resulting names and binary contents remain consumable by `covdata` and `cover`.

The user selected loopback TCP to simplify the internal coverage build and use the same server on Linux and macOS. The manager listens on `127.0.0.1:18081` and serves an unauthenticated `/snapshot` endpoint. On host-networked Supervisors this is deliberately node-local access, including other users and PID namespaces on that node. Normal builds contain no listener. An optional matching port setting handles conflicts when extra managers share a network namespace.

The collector uses Kubernetes port-forward authorization and opens a short-lived forward for each snapshot, with an atomically allocated local port, bounded startup/HTTP waits, continuous output draining, bounded download size, and unconditional process cleanup. This removes the export-helper binary and platform-specific socket code. The live coverage image's `/tmp` was owned by root with mode 1755 and was not writable by UID 65534; TCP requires no filesystem entry or permission change.

Leaving `GOCOVERDIR` unset produces Go's startup warning about automatic file output. It does not disable explicit in-memory snapshots. Collection remains every 60 seconds plus a final pass, with only the runner persisting artifacts. Abrupt termination can still lose execution since the last successful export.

## Validation limits

Local integration tests exercise the real Go HTTP server and reporting tools with a simulated Kubernetes CLI. Linux container tests additionally establish read-only operation, loopback-only binding, and the intentional node-local access boundary. Direct TCP validation on a VDS Supervisor established manager image packaging, real Kubernetes port-forwarding and cleanup, image-only setup, rollout, process retention, dry-run webhook activity, and reporting after a synthetic test failure; evidence is recorded in tasks.md. Shared UTS log accessibility, full GoBuild/runner packaging, complete E2E suites, scheduler selection of quarantined jobs, and instrumentation overhead remain pipeline acceptance checks. Coverage resources are provisional and isolated from ordinary E2E jobs.
