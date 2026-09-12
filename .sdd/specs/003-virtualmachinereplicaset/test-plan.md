# Plan: Implement `VirtualMachineReplicaSet` functional tests via AI, driven by the TDS

## Context

[`tds.md`](./tds.md) defines 38 Given/When/Then scenarios that specify the intended, implementation-independent behavior of `VirtualMachineReplicaSet`. The goal is to turn each scenario into a real, passing (or intentionally-flagged-failing) test, so that "all tests green" is a trustworthy signal that this feature is safe to switch on for users.

Existing coverage today (verified by reading the code, not assumed):

- `controllers/virtualmachinereplicaset/`: controller (`virtualmachinereplicaset_controller.go`), delete-policy helper (`virtualmachinereplicaset_delete_policy.go`), a suite file, and **only an integration (`envtest`) test file** (`_intg_test.go`) with 4 thin `It`s (create, scale up, scale down, delete/finalizer). **No unit test file exists.**
- `webhooks/virtualmachinereplicaset/validation/`: validator + unit + intg tests already exist, covering selector/template label-match validation.
- `webhooks/virtualmachinereplicaset/mutation/`: mutator exists but is a no-op stub (`// TODO` referencing vmop-1827) with **no test file at all**.
- `test/e2e/`: **no coverage** of `VirtualMachineReplicaSet`.
- `test/builder/dummies.go` already has `DummyVirtualMachineReplicaSet()` to reuse as a base fixture.

**Important — real discrepancies found between the TDS and the current implementation, discovered by reading the controller code (not the TDS)**. These aren't test-writing details, they change what "correct" means:

1. **Orphan adoption (TDS §19) — RESOLVED, code is correct.** The controller (`adoptOrphan`, `ReconcileNormal`) adopts any standalone VM with no controller owner ref that matches the selector. This is the intended, correct behavior, not a bug: adoption never relocates/reconfigures the VM (so it doesn't run into zone/placement-mobility limits), it matches the well-established Kubernetes `ReplicaSet` adoption pattern, and it keeps the controller self-healing instead of getting permanently stuck on a pre-existing matching VM. The one gap is **visibility**: today adoption only emits a transient `Event` (`SuccessfulAdopt`), with no persistent `Condition`, so there's no durable, `kubectl describe`-visible record that a given replica was adopted rather than created. Filed as **vmop-4017** under epic vmop-1701 to add a Condition on adoption. TDS §19 has been updated to assert adoption happens *and* that a Condition is set recording it — the test should currently fail on the Condition assertion until vmop-4017 lands, which is expected and desired (the test should stay red until the ticket is fixed, not be watered down). The competing-ReplicaSets case (TDS §20) is unaffected — that one still must surface a conflict rather than silently double-adopt.
2. **`status.readyReplicas` (TDS §24)**: the code has a literal `// TODO: Figure out an equivalent of Ready condition...` and currently counts every filtered VM as ready regardless of actual `VirtualMachine.Status` readiness. TDS scenario 24 will fail against current code. This is a known, intentional gap — the test should be written to assert the *intended* contract and will fail until the TODO is implemented (or the test is labeled `Pending`/skipped with a tracking ticket, per team preference).
3. **Negative replica validation (TDS §12)**: no CRD marker (`+kubebuilder:validation:Minimum=0`) or webhook rule rejects `spec.replicas = -1` today. This test is also expected to fail/expose a gap.

These three are called out explicitly in the task list below rather than hidden — the point of this exercise is to surface exactly this kind of drift.

## Approach

Map the 38 TDS scenarios onto 4 concrete test surfaces, matching existing repo conventions (`.sdd/memory/testing-standards.md`), and implement them in ordered batches using subagents (one agent per batch, sequential batches so later batches can build on earlier fixtures/helpers). Each batch ends with running the real test suite so failures are triaged immediately (bug vs. TDS update vs. test bug), not batched up at the end.

### Test surface mapping

| Surface | File(s) | TDS scenarios | Why |
|---|---|---|---|
| Controller **unit** tests (new) | `controllers/virtualmachinereplicaset/virtualmachinereplicaset_controller_test.go` (new, following `virtualmachinesnapshot_controller_unit_test.go` as the template) | 1–5, 9, 10, 12(partial), 14, 15, 16, 22, 23, 25, 26, 27, 28, 29, 30, 31, 34, 35, 36 | Fast, exhaustive coverage of status/condition/label logic against a fake client — no envtest needed. |
| Controller **integration** tests (extend existing) | `controllers/virtualmachinereplicaset/virtualmachinereplicaset_controller_intg_test.go` | 6, 7, 8, 11, 13(cross-check), 17, 18, 19 (resolved — see Context), 20, 21, 24, 32, 33, 37, 38 | Needs real API-server GC, real owner refs, real scale subresource, real cross-object interaction — exactly what the existing 4 tests already do a little of. |
| Validation **webhook** tests (extend existing) | `webhooks/virtualmachinereplicaset/validation/virtualmachinereplicaset_validator_unit_test.go` / `_intg_test.go` | 12, 13 | Webhook already has a suite; add the negative-replicas case (currently likely absent — confirm and add) and confirm selector/template mismatch is fully covered (scenario 13 may already be covered — audit before adding duplicates). |
| Mutation **webhook** tests (new) | `webhooks/virtualmachinereplicaset/mutation/virtualmachinereplicaset_mutator_unit_test.go` (new) | none directly from the 38, but needed for completeness since the mutator has zero tests today | Not in the original TDS scope, but flagged as a coverage gap while in the neighborhood; keep minimal (assert current no-op passthrough behavior) so it doesn't block the main effort. |
| **E2E** smoke test (new) | `test/e2e/vmservice/virtualmachinereplicaset/` (new package, following `test/e2e/vmservice/virtualmachine/` layout) | thin slice of 1, 6, 7, 11, 17 | `test/e2e/` runs only against a **real vCenter/WCP cluster** (confirmed in `test/e2e/README.md` — no vcsim option in this tier). One end-to-end create/scale/delete pass, labeled per the existing `smoke`/`core-functional` conventions, per `.sdd/memory/e2e-sync-with-changes.md` — required because this is cluster-observable behavior. |

Scenarios not explicitly listed above (e.g. 3, 4 defaulting) are folded into the unit-test batch's fixture setup rather than getting a dedicated bullet.

**Correction — no new vcsim-tier test is being added.** vcsim-backed tests in this repo are a distinct, real-vSphere-simulator integration tier (`test/builder`'s `TestContextForVCSim`, `testlabels.VCSim`) that lives at the **provider** level (`pkg/providers/vsphere/**`, e.g. `vmprovider_vm_power_test.go`, `session_vm_update_test.go`) — that's where vSphere-isms (power state transitions, disks, networking) actually need a simulated vCenter. `VirtualMachineReplicaSet`'s controller (`controllers/virtualmachinereplicaset/virtualmachinereplicaset_controller.go`) never imports or calls `pkg/providers/vsphere` — it only creates/updates/deletes `VirtualMachine` CRs and counts/labels them; any vSphere-isms of an owned replica are already exercised by the existing VM-level vcsim tests. So the "Controller integration tests" row above uses plain `envtest` + the fake `VMProvider` (`testlabels.EnvTest`, no `testlabels.VCSim`), and the "E2E" row is the only tier that touches real vSphere behavior for this feature.

### Batches (sequential; each is one subagent turn + a test run + triage)

1. **Unit test scaffolding + core reconciliation** — create `virtualmachinereplicaset_controller_test.go` with suite wiring for unit tests (add `unitTests` to the existing `suite.Register` call in `virtualmachinereplicaset_controller_suite_test.go`, currently passing `nil`), using `builder.UnitTestContextForController` + `providerfake.VMProvider` per `testing-standards.md`. Implement scenarios 1–5, 9, 10.
2. **Status & conditions** — scenarios 22, 23, 25, 26, 27, 28, 29 against the fake client, asserting `status.replicas`, `fullyLabeledReplicas`, conditions (`VirtualMachinesCreated`, `Resized`/`ScalingUp`/`ScalingDown`, `VirtualMachinesReady`, `ReplicaFailure`), `observedGeneration`.
3. **Template/selector/label semantics** — scenarios 14, 15, 16, 30, 31, 34.
4. **Negative/validation-adjacent unit cases** — scenario 12 unit-side assertion (reconciler should never act on a spec that shouldn't have passed validation) + audit/extend the validation webhook tests (scenarios 12, 13) in `webhooks/virtualmachinereplicaset/validation/`.
5. **Non-goal absence checks** — scenarios 35, 36 (unit-level: no revision/rollout fields, no auto-scale side effects).
6. **Integration: scaling & scale subresource** — extend `_intg_test.go` with scenarios 8, 11, 37, and the "flapping" scenario 10 at the envtest level if scenario 10's unit version doesn't fully cover real reconcile timing.
7. **Integration: ownership, orphans, GC, cross-namespace** — scenarios 17 (best-effort — the existing suite already notes envtest can't run real GC; document this limitation rather than asserting something envtest can't prove), 18, 20, 21, 33, and **19 (resolved: assert adoption occurs — owner reference is set, VM is not duplicated — and assert a `Condition` records the adoption; the Condition assertion is expected to fail until vmop-4017 is implemented, which is intentional).**
8. **Integration: readiness & finalizer draining** — scenarios 24 (expected to fail — write it against the intended contract, mark clearly, do not water it down to match the stub), 32, 38.
9. **Mutation webhook baseline tests** — new `_unit_test.go` for the mutator, asserting current pass-through/no-op behavior (so future implementation of vmop-1827 has a regression baseline).
10. **E2E smoke** — new `test/e2e/vmservice/virtualmachinereplicaset/` suite, run against a real vCenter/WCP cluster per `test/e2e/README.md` (no vcsim path exists here): create with N replicas, scale up, scale down, delete, confirm VMs actually reach `PoweredOn`.
11. **Triage pass** — run the full suite, confirm the only failures are the three known, intentionally-red scenarios (§19's Condition assertion → vmop-4017, §24 readyReplicas stub, §12 negative-replicas), file follow-up tickets for §24 and §12 (vmop-4017 already filed for §19), and do not silently adjust any test to match broken behavior just to turn it green.

### Key reused patterns (don't reinvent)

- `test/builder.DummyVirtualMachineReplicaSet()` as the base fixture for unit tests; the existing `_intg_test.go`'s inline `rs := &vmopv1.VirtualMachineReplicaSet{...}` literal as the pattern for integration tests (or migrate it to the dummy builder for consistency — small win, call it out but don't force it if it causes churn).
- `providerfake.SetCreateOrUpdateFunction` (already used in `_intg_test.go`) to control what happens when the fake provider creates/updates a `VirtualMachine`, e.g. to simulate a VM becoming `Ready` for scenario 24.
- `virtualmachinesnapshot_controller_unit_test.go` as the structural template for the new unit test file (external `_test` package, `Describe`/`Context`/`When`/`It`, `Label(testlabels.Controller, testlabels.API)`, `BeforeEach`/`JustBeforeEach`/`AfterEach` with `ctx.AfterEach()`).
- `pkg/constants/testlabels` labels: `Controller`, `EnvTest`, `API`, `Validation`, `Webhook` — use the existing combination on `Describe` blocks per file type.

### Verification

- Unit: `go test ./controllers/virtualmachinereplicaset/... -run TestVirtualMachine` (or the repo's `make test` / `ginkgo` target used elsewhere — confirm exact command from `Makefile`/README during batch 1).
- Integration: same binary, but requires `envtest` binaries set up (`make test` should already handle this — verify).
- Webhook: `go test ./webhooks/virtualmachinereplicaset/...`.
- E2E: per `test/e2e/README.md`, run against a real vCenter/WCP cluster (`make test-e2e` / the `smoke`/`core-functional` Ginkgo-labeled targets — confirm exact target during batch 10; there is no vcsim-backed e2e target).
- After each batch, run only that batch's package tests before moving on; run the full `make test` after batch 11 to catch cross-package regressions (e.g. the mapper function `VMToReplicaSets` touching other controllers' watch behavior is unlikely but worth a full run once).

### Deliverable shape

One PR (or a small stack) per batch or logical group of batches, each following `.sdd/memory/commit-message-standards.md` and `.sdd/memory/pull-request-standards.md`, calling out in the PR description which TDS scenario numbers are covered and which discovered gaps are being deliberately left red with a linked follow-up.
