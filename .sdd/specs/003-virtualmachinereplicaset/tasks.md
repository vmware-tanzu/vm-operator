# Tasks: `VirtualMachineReplicaSet` Functional Test Coverage

- **TDS**: [`tds.md`](./tds.md) — the acceptance-criteria source for this task list (in place of a `spec.md`; each task cites the TDS scenario number(s) it implements as `[SCn]`)
- **Test plan**: [`test-plan.md`](./test-plan.md)
- **Epic**: vmop-1701

<!--
TODO: fill in per-task [vmop-NNN] story/sub-task tags once filed under the
epic, following the same deferral used in 002-vm-extraconfig-reconcile/tasks.md.
vmop-1701 already tracks "Implement VirtualMachineReplicaSet API"; the one
ticket that already exists (vmop-4017, for the TDS §19 Condition-on-adoption
gap) is tagged below on the tasks it blocks.
-->

This task list decomposes `test-plan.md` into ordered, executable tasks. It is test-only for TDS scenarios that already pass or can be written against a genuine gap; three tasks are expected to produce an initially-failing (red) test that documents a real product gap rather than a test bug — see Phase Final.

## Phase 1 — Setup

- [ ] T001 Wire `unitTests` into the existing `suite.Register(t, "VirtualMachineReplicaSet controller suite", intgTests, nil)` call in `controllers/virtualmachinereplicaset/virtualmachinereplicaset_controller_suite_test.go` (currently `nil`) — blocks every unit-test task below.

## Phase 2 — Foundational

- [ ] T002 [P] Audit `webhooks/virtualmachinereplicaset/validation/virtualmachinereplicaset_validator_unit_test.go` and `_intg_test.go` for existing coverage of TDS scenarios 12 (negative replicas) and 13 (selector/template mismatch) to avoid duplicating specs in Phase 7.
- [ ] T003 [P] Confirm the exact `make`/`ginkgo` invocation for each tier referenced in `test-plan.md`'s Verification section (unit, envtest integration, webhook, real-vCenter E2E) against `Makefile` and `test/e2e/README.md`; update `test-plan.md` if any command differs from what's documented there.

## Phase 3 — Controller unit tests: core reconciliation

- [ ] T004 [SC1,SC2,SC3,SC4,SC5,SC9,SC10] Create `controllers/virtualmachinereplicaset/virtualmachinereplicaset_controller_test.go` (external `_test` package, `Label(testlabels.Controller, testlabels.API)`, `builder.UnitTestContextForController` + `providerfake.VMProvider`, structured after `virtualmachinesnapshot_controller_unit_test.go`). Cover: create-from-scratch produces N owned VMs (SC1); default `spec.replicas=1` (SC2); explicit `spec.replicas=0` creates/leaves none (SC3); idempotent no-op reconcile (SC4); scale-to-zero-then-back-up creates fresh VMs, never reuses names (SC9); rapid successive `spec.replicas` edits converge to the final value without leaking VMs (SC10). Depends on T001.

## Phase 4 — Controller unit tests: status & conditions

- [ ] T005 [SC22,SC23,SC25,SC26,SC27,SC28,SC29] Extend `virtualmachinereplicaset_controller_test.go` (same file as T004, sequential — not `[P]` against T004/T006/T007): `status.replicas` always matches owned-VM count (SC22); `status.fullyLabeledReplicas` diverges from `status.replicas` on label drift (SC23); `VirtualMachinesCreated` reflects create-path failures via `providerfake` injected errors (SC25); `ReplicaFailure` condition on sustained failure to converge (SC26); `VirtualMachinesReady` aggregates readiness — resolve the `[NEEDS CLARIFICATION]` from the TDS by writing the test against the chosen condition state for the `spec.replicas=0` edge case and updating TDS §27 to remove the marker (SC27); `observedGeneration` only advances once the generation's spec is actually processed (SC28); `Resized` condition clears once converged (SC29). Depends on T004.

## Phase 5 — Controller unit tests: template/selector/label semantics

- [ ] T006 [SC14,SC15,SC16,SC30,SC31,SC34] Extend `virtualmachinereplicaset_controller_test.go` (same file, sequential after T005): selector edits don't retroactively relabel existing VMs (SC14); template spec edits don't mutate/recreate existing replicas (SC15); template metadata-label edits — resolve the `[NEEDS CLARIFICATION]` from the TDS by writing the test against the chosen behavior and updating TDS §16 to remove the marker (SC16); `vmoperator.vmware.com/replicaset-name` label correctness (SC30) and its ownership-vs-discoverability distinction (SC31); template `Spec` fields (e.g. `powerState`) pass through verbatim (SC34). Depends on T005.

## Phase 6 — Controller unit tests: non-goal absence checks

- [ ] T007 [SC35,SC36] Extend `virtualmachinereplicaset_controller_test.go` (same file, sequential after T006): assert no `ControllerRevision`-equivalent/rollout-status field exists on the type (SC35); assert `status.replicas` never changes except in response to an external `spec.replicas` edit, i.e. no built-in auto-scaling side effect (SC36). Depends on T006.

## Phase 7 — Validation webhook tests

- [ ] T008 [P] [SC12,SC13] In `webhooks/virtualmachinereplicaset/validation/`: add the negative-`spec.replicas` rejection case (SC12 — expected to fail today, no `+kubebuilder:validation:Minimum=0` or webhook rule exists; write it against the intended contract) to `virtualmachinereplicaset_validator_unit_test.go`; confirm/extend selector-vs-template-label mismatch coverage (SC13) per the T002 audit, adding only what's missing to `_unit_test.go`/`_intg_test.go`. Independent file from Phases 3–6, may run in parallel with them. Depends on T002.

## Phase 8 — Controller integration tests: scaling & scale subresource

- [ ] T009 [SC6,SC7,SC8,SC11,SC37] Extend `controllers/virtualmachinereplicaset/virtualmachinereplicaset_controller_intg_test.go` (envtest, existing `intgFakeVMProvider`): scale-up leaves original replicas untouched and sets `Resized`/`ScalingUp` (SC6); scale-down deletes the right count and sets `Resized`/`ScalingDown` (SC7); `deletePolicy: Random` scale-down invariants — deleted VM was owned, remaining VMs untouched, no assertion on *which* one is picked (SC8); editing the `/scale` subresource has identical effect to editing `spec.replicas` directly (SC11); replicas may legitimately co-locate on the same host/cluster module without error, since anti-affinity is out of scope (SC37). Depends on T001 (suite wiring already exists for intg tests; T001 is unit-only but kept as a dependency marker for ordering within this task list).

## Phase 9 — Controller integration tests: ownership, orphans, GC, cross-namespace

- [ ] T010 [SC17,SC18,SC20,SC21,SC33] [vmop-4017 for SC19] Extend the same `_intg_test.go` (sequential after T009): deleting the `VirtualMachineReplicaSet` cascades to owned VMs, best-effort given envtest can't run kube-controller-manager GC — document the limitation rather than asserting what envtest can't prove (SC17); deleting one owned VM directly triggers exactly one replacement (SC18); two `VirtualMachineReplicaSet`s with overlapping selectors never cross-delete each other's owned VMs (SC20); namespace-scoped selector matching — a same-labeled VM in a different namespace is never touched (SC21); a VM deleted by something other than the controller (competing controller, quota enforcement, manual `kubectl delete`) is treated identically to SC18 (SC33). **SC19** (a standalone VM matching the selector is adopted, and a `Condition` records the adoption): write the adoption-occurs half now; the `Condition` assertion is expected to fail until **vmop-4017** is implemented — do not weaken the assertion to make it pass. Depends on T009.

## Phase 10 — Controller integration tests: readiness & finalizer draining

- [ ] T011 [SC24,SC32,SC38] Extend the same `_intg_test.go` (sequential after T010): `status.readyReplicas` tracks each owned VM's `Ready` condition via `providerfake.SetCreateOrUpdateFunction` — **expected to fail** against the current stub that counts every filtered VM as ready regardless of actual status; write it against the intended contract, do not water it down (SC24); a slow-draining finalizer on a replica being scaled down doesn't cause the controller to overshoot `spec.replicas` with a premature replacement (SC32); a replica recreated after deletion gets a fresh generated name, never reusing the deleted replica's identity (SC38). Depends on T010.

## Phase 11 — Mutation webhook baseline tests

- [ ] T012 [P] Create `webhooks/virtualmachinereplicaset/mutation/virtualmachinereplicaset_mutator_unit_test.go` (new file, currently zero coverage) asserting the mutator's current no-op/pass-through behavior, so the eventual implementation of vmop-1827 has a regression baseline. Independent of all controller-file phases; may run in parallel with Phases 3–10.

## Phase 12 — E2E smoke

- [ ] T013 [P] [SC1,SC6,SC7,SC11,SC17 (thin slice)] Create `test/e2e/vmservice/virtualmachinereplicaset/` following the `test/e2e/vmservice/virtualmachine/` layout, registered from `test/e2e/vmservice/vmservice_test.go`; run against a **real vCenter/WCP cluster** per `test/e2e/README.md` (no vcsim path exists in this tier — see `test-plan.md`'s correction note): create with N replicas, scale up, scale down, delete, and confirm the VMs actually reach `PoweredOn`. Independent new package; may run in parallel with Phases 3–11.

## Phase Final — Polish / triage

- [ ] T014 Run the full suite (`make test` for unit + envtest integration + webhook tiers per T003's confirmed commands; the E2E suite from T013 against a real cluster) and confirm the **only** failures are the three known, intentionally-red assertions: SC19's Condition assertion (blocked on vmop-4017), SC24's readyReplicas assertion, SC12's negative-replicas assertion. Any other failure is a real test bug or a newly-discovered gap — fix or triage it, do not suppress it.
- [ ] T015 File a follow-up `vmop-NNN` story/sub-task under epic vmop-1701 for the SC24 `status.readyReplicas` gap (the `// TODO: Figure out an equivalent of Ready condition...` in `virtualmachinereplicaset_controller.go`), linked via `customfield_10830`.
- [ ] T016 File a follow-up `vmop-NNN` story/sub-task under epic vmop-1701 for the SC12 negative-`spec.replicas` validation gap, linked via `customfield_10830`.
- [ ] T017 Update `test-plan.md` and `tds.md` with any deviations discovered during execution (per `.sdd/memory/sdd-standards.md`'s "spec is source of truth" rule) — including resolving TDS §16's and §27's `[NEEDS CLARIFICATION]` markers once T006 and T005 land, respectively.
