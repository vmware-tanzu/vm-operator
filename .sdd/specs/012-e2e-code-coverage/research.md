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

## Validation limits

Local integration tests exercise the real Go exporter and reporting tools with a simulated Kubernetes CLI. They do not establish Supervisor permissions, shared UTS log accessibility, image packaging, rollout behavior, scheduler selection of quarantined jobs, or runtime overhead. Those remain live acceptance checks. Coverage resources are provisional and isolated from ordinary E2E jobs.
