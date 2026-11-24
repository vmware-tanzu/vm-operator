# Tasks: VirtualMachine Affinity Status

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Epic**: vmop-4283
- **Story**: vmop-4283 (split into stories per phase if needed)

<!--
User stories: US1 "DevOps user: see whether my affinity rules are satisfied",
US2 "Platform engineer: diagnose the tag and policy mapping",
US3 "CSP admin: rollout safety and vCenter load".
-->

## Phase 1 — Setup

- [x] T001 [vmop-4283] Bump govmomi to the revision with `vapi/tagging` (name-filtered `ListCategories`/`ListTags` plus the simulator), `vapi/vcenter/vm/compute/policies`, and `mo.VirtualMachine.ComputePolicies` (`go.mod`, `go.sum`, `test/e2e/go.mod`, `test/e2e/go.sum`)

## Phase 2 — Foundational

- [x] T002 [P] [US1] [vmop-4283] Add `VirtualMachineAffinityStatus`, its policy, label, and enum types, and `Status.Affinity`. Regenerate with `make generate-go`, `make generate-go-conversions`, and `make generate-manifests` (`api/v1alpha6/virtualmachine_affinity_status_types.go`, `api/v1alpha6/virtualmachine_types.go`, `api/v1alpha6/zz_generated.deepcopy.go`, `api/v1alpha{1..5}/zz_generated.conversion.go`, `config/crd/bases/vmoperator.vmware.com_virtualmachines.yaml`)
- [x] T003 [P] [US2] [vmop-4283] Add `TagStatus.CategoryID` and `TagStatus.PolicyID`, document `ID`, and add the CEL rule, the print columns, and the `VCenterTagResolved` condition and reason constants. Regenerate (`external/vsphere-policy/api/v1alpha1/tag_types.go`, `external/vsphere-policy/api/v1alpha1/zz_generated.deepcopy.go`, `config/crd/external-crds/vsphere.policy.vmware.com_tags.yaml`)
- [x] T004 [P] [US3] [vmop-4283] Add the `AffinityStatusEnabled(ctx)` gate (`TaggingAPI && VMHardAffinityDuringExecution`) with unit tests (`pkg/providers/vsphere/virtualmachine/affinity_status.go`, `pkg/providers/vsphere/virtualmachine/affinity_status_test.go`)
- [x] T005 [US2] [vmop-4283] Add `ResolveTagIDs` to the provider interface and the fake. Implement it with name-filtered `tagging.ListCategories`/`ListTags`, the per-namespace category ID cache, and `WithVCOpID(…, "resolveTagIDs")`, with VCSim tests (`pkg/providers/vm_provider_interface.go`, `pkg/providers/fake/fake_vm_provider.go`, `pkg/providers/vsphere/vmprovider_tags.go`, `pkg/providers/vsphere/vmprovider_tags_test.go`)

## Phase 3 — User Story 2 (tag and policy identifiers on `Tag`)

- [x] T006 [US2] [vmop-4283] Resolve `status.id` and `status.categoryID` in the `Tag` controller when gated on and `status.id` is empty. Set `VCenterTagResolved`, and requeue on `NotFound` with `ctrl.Result{RequeueAfter}` and a 5s→5m backoff derived from the condition's `lastTransitionTime`. Inject `ctx.VMProvider` through `AddToManager` and `NewReconciler` (`controllers/vspherepolicy/tag/tag_controller.go`). Covers US2 scenarios 1, 2, and 4
- [x] T007 [US2] [vmop-4283] Unit and envtest specs for T006: found, not found then found, backoff growth and cap, error, already resolved (no provider call), gate off, and `Ready` unaffected (`controllers/vspherepolicy/tag/tag_controller_test.go`)
- [x] T008 [P] [US1] [vmop-4283] Add `TagPolicyIDIndexKey`, `TagPolicyIDIndexerFunc`, and `RegisterTagPolicyIDIndex`, register the index on the fake client (`test/builder/fake.go`), and change `TagFanOutPredicate()` to `TagFanOutPredicate(ctx)` with a gated `Update` branch that admits `status.id` or `status.policyID` changes on `AffinityTarget` and `GroupedAntiAffinitySelf` `Tag`s. When gated on, make `TagToVirtualMachineMapper` also enqueue, for a `GroupedAntiAffinitySelf` `Tag`, the VMs carrying any of its `spec.policy.targets` labels, deduplicated with the owners, and tolerating a failed lookup for one target. Add unit tests for the index function, for the predicate with the gate on and off, and for the self `Tag` peer fan-out with the gate on and off, with no policy identity, and with a failed lookup (`pkg/util/kube/vmtags.go`, `pkg/util/kube/vmtags_test.go`). In the VM controller, register the index when `AffinityStatusEnabled` is true and pass `ctx` to the predicate on the existing `Tag` watch (`controllers/virtualmachine/virtualmachine/virtualmachine_controller.go`). Update spec 006's predicate table and `TagFanOutPredicate` test contract to match (`.sdd/specs/006-tag-controller-for-affinity/plan.md`)

## Phase 4 — User Story 1 (`status.affinity`)

- [x] T009 [P] [US1] [vmop-4283] Add `RelatedAffinityPolicyTags(ctx, client, vm)`, which returns the related policy-defining `Tag`s sorted by name: the owned `AffinityTarget`/`GroupedAntiAffinitySelf` `Tag`s (`DesiredAffinityTags` identities, read from the cache by `ConcreteTagResourceName`, falling back to the status-free identity when the `Tag` is not in the cache yet so it still reports `Pending`) and the carried `AffinityTarget` `Tag`s (`ListLabelDerivedTags`, which never returns self `Tag`s). Add `AffinityPolicyStatusFromTag(tag, compliance)` with the kind, strictness, topology, and target mappings (it returns `false` for a `Tag` with no policy mapping and leaves `policyID` to the caller), `AffinityComplianceFromVCenter` for the compliance mapping, and `SortAffinityPolicyStatuses` for the entry order, plus unit tests (`pkg/providers/vsphere/virtualmachine/affinity_status.go`, `pkg/providers/vsphere/virtualmachine/affinity_status_test.go`)
- [x] T010 [P] [US3] [vmop-4283] Add `watcher.ComputePoliciesPropertyPath` (not in `DefaultWatchedPropertyPaths`). Add `vmwatcher.WatchedPropertyPaths(ctx)`, which appends it when gated on and is passed to `watcher.Start`, and `vsphere.VMUpdatePropertiesSelectorFor(ctx)`, which appends it to `VMUpdatePropertiesSelector` when gated on and is used by `updateVirtualMachine`. Test both gate states (`pkg/util/vsphere/watcher/watcher.go`, `services/vm-watcher/vm_watcher_service.go`, `pkg/providers/vsphere/vmprovider_vm.go`, and the matching `_test.go` files)
- [x] T011 [US3] [vmop-4283] Add the `AffinityStatusPendingRequeueDelay` config field (default 30s) with its `AFFINITY_STATUS_PENDING_REQUEUE_DELAY` env var (`pkg/config/config.go`, `pkg/config/default.go`, `pkg/config/env.go`, `pkg/config/env/env.go`, `pkg/config/config_test.go`, `pkg/config/env_test.go`). Make `requeueDelay` return it when the gate is on and any `status.affinity` entry is `Pending` (`hasPendingAffinityPolicy`), placed after the create-delay check and the `AsyncSignalEnabled` early return, so it applies only when async signal is off. When the powered-on-without-IP delay also applies, the shorter delay wins. Add `WithAffinityStatusRetryContext`, `MarkAffinityStatusRetry`, and `IsAffinityStatusRetry` (`pkg/providers/vsphere/virtualmachine/affinity_status.go`). The VM controller adds the context to every reconcile, and `requeueDelay` returns the delay when the retry is marked, with async signal on or off. Unit tests: a marked retry gives the delay with async signal on and off, a marked retry with a provider error returns the error with no delay, Pending gives the delay with a nil error with async signal off and no requeue with async signal on, the shorter of the IP and pending delays wins, no Pending gives the existing behavior, the gate off gives the existing behavior, and a real reconcile error is still returned as an error (`controllers/virtualmachine/virtualmachine/virtualmachine_controller.go`, `controllers/virtualmachine/virtualmachine/virtualmachine_controller_unit_test.go`). Covers US3 scenario 4
- [x] T012 [US1] [vmop-4283] Implement `ReconcileAffinityStatus` and the `computePolicyQuerier` wrapper with `WithVCOpID(…, "queryComputePolicies")`. The function maps reported IDs through the index and queries only under the plan's query trigger, filtered to the VM and the unclaimed IDs. It patches `Tag.status.policyID` with `MergeFromWithOptimisticLock` and skips unchanged values, builds the reported and pending entries, never returns an error or a `RequeueError` (see plan "Requeue when nothing matches"), logs and tolerates errors while keeping prior values, marks the retry context on any error (see plan "Requeue after an error"), sorts entries, and clears status when gated off. Wire it into `ReconcileStatus` through a new `ReconcileStatusData.ComputePolicyQuerier` field set by the provider (`pkg/providers/vsphere/vmlifecycle/update_status_affinity.go`, `pkg/providers/vsphere/vmlifecycle/update_status.go`, `pkg/providers/vsphere/vmprovider_vm.go`). Update spec 006's `Tag` status-ownership invariant to name the VM update path as the `status.policyID` writer (`.sdd/specs/006-tag-controller-for-affinity/model.md`). Covers US1 scenarios 1–7, US2 scenarios 3 and 5, and US3 scenarios 2–5
- [x] T018 [US1] [vmop-4283] Add `VirtualMachineAffinityPolicyOrigin` (`Self`, `Peer`) and the required `origin` field on each `status.affinity` entry, and regenerate (`api/v1alpha6/virtualmachine_affinity_status_types.go`, `config/crd/bases/vmoperator.vmware.com_virtualmachines.yaml`). Add `OwnedAffinityPolicyTagNames(ctx, vm)` with unit tests (`pkg/providers/vsphere/virtualmachine/affinity_status.go`, `pkg/providers/vsphere/virtualmachine/affinity_status_test.go`), and set `origin` in `ReconcileAffinityStatus`, keeping the previous origin when the owned `Tag`s cannot be computed (`pkg/providers/vsphere/vmlifecycle/update_status_affinity.go`). Covers G2a and US1 scenarios 3 and 8
- [x] T013 [US1] [vmop-4283] Unit specs for T012 with a call-counting fake querier and the fake client, setting `moVM.ComputePolicies` directly: steady state makes 0 queries; the first mapping makes 1 query and 1 patch; a peer VM makes 0 queries; a label-only VM reports the policy; unrelated policies with every related `Tag` mapped make 0 queries; an unrelated policy with an unmapped related `Tag` is queried and matches nothing, leaving the entry `Pending`; query and per-VM errors keep the prior `policyID` with the currently reported compliance; a patch conflict; a failed patch; a failed `Tag` list; the retry is marked on every error and not on success or an empty match; `Pending` before `status.id` and before association; removed affinity; gate off; `origin` `Self` for a requester and `Peer` for a label-only VM and for an anti-affinity target VM; and the compliance mapping (`pkg/providers/vsphere/vmlifecycle/update_status_test.go`)

## Phase 5 — E2E

- [x] T014 [US1] [vmop-4283] E2E on a Supervisor with `VMHardAffinityDuringExecution`: two VMs with host anti-affinity reach `Compliant` and share one `GroupedAntiAffinitySelf` `Tag` with `status.id` and `status.policyID` set. An affinity requester and a label-only peer both report the same `Affinity` `policyID`, with `origin: Self` and `origin: Peer` respectively. Removing affinity clears `status.affinity`. Use the existing helpers and labels (`test/e2e/vmservice/vmservice/virtualmachine/vm_group.go`, `test/e2e/vmservice/vmservice/tag/tag.go`, `test/e2e/vmservice/lib/vmoperator/`)

## Phase Final — Polish

- [x] T015 [P] Document `status.affinity` for DevOps users in the VM affinity docs under `docs/`
- [x] T016 [P] Update spec 006 for the `Tag` controller's vCenter ID resolution (`status.id`, `status.categoryID`) (`.sdd/specs/006-tag-controller-for-affinity/spec.md`, `.sdd/specs/006-tag-controller-for-affinity/model.md`, `.sdd/specs/006-tag-controller-for-affinity/research.md`, `.sdd/specs/006-tag-controller-for-affinity/e2e.md`)
- [x] T017 Update the `.sdd/INDEX.md` status as phases land

---

## Handoff notes for implementing agents

Read `spec.md` (acceptance criteria), `model.md` (exact types), and `plan.md` "Design" before you start a task. The spec wins if code and spec disagree. Update the spec in the same PR if an assumption proves wrong.

### Dependency order

| Task | Depends on |
|------|------------|
| T001 govmomi bump | — |
| T002 VM status types | — |
| T003 `Tag` status types | — |
| T004 gate helper | — |
| T005 `ResolveTagIDs` | T001 |
| T006 `Tag` controller resolution | T003, T005 |
| T007 `Tag` controller tests | T006 |
| T008 index and predicate | T003, T004 |
| T009 related `Tag`s and projection | T002, T003, T004 |
| T010 `computePolicies` plumbing | T001, T004 |
| T011 `requeueDelay` on `Pending` | T002, T004 |
| T012 `ReconcileAffinityStatus` | T001, T002, T003, T008, T009, T010 |
| T018 `origin` field | T002, T009, T012 |
| T013 `ReconcileAffinityStatus` tests | T012, T018 |
| T014 E2E | T006, T008, T011, T012, T018 |
| T015, T017 polish | T014 |
| T016 spec 006 update | T006 |

- **Parallel batch A:** T001, T002, T003, T004.
- **Parallel batch B (after A):** T005, T008, T009, T010, T011.
- **Then:** T006 → T007 alongside T012 → T013, then T014.

### Verified facts and gotchas

- `vmlifecycle.ReconcileStatus` returns `apierrorsutil.NewAggregate(errs)`. That aggregate has no `Unwrap() []error`, so `errors.As` cannot see a typed error inside it. `ReconcileAffinityStatus` must not return errors, and the requeue is done by T011 in `requeueDelay`, from `Pending` entries or the retry context.
- `pkgcfg.FromContext` panics on a context without a config. `TagToVirtualMachineMapper` reads the feature flags from its construction context, so tests must build it with `pkgcfg.NewContextWithDefaultConfig()`, not `context.Background()`.
- `pkgerr.ResultFromError` gives `RequeueError` priority over every other error. Never use `RequeueError` in the VM update path for this feature.
- `kubeutil.TagFanOutPredicate` drops every `Update` event when the gate is off, and spec 006 depends on that. T008's `Update` branch must admit only `status.id` or `status.policyID` changes on `AffinityTarget` and `GroupedAntiAffinitySelf` `Tag`s, and only when gated on. There is one `Tag` watch on the VM controller; do not add a second one.
- `pkg/util/kube` cannot import `pkg/providers/vsphere/virtualmachine` (that package imports `pkg/util/kube`), so the predicate reads `Features.TaggingAPI` and `Features.VMHardAffinityDuringExecution` through `pkgcfg` directly.
- Tag backend names in vCenter carry the `vmoperator:` prefix (`virtualmachine.AddAffinityTagPrefix(spec.name)`). `Tag.spec.name` is prefix-free. Keep the colon-safe naming from spec 006.
- `Tag` resource name = `virtualmachine.ConcreteTagResourceName(namespace, spec.name)`.
- Owned `Tag` identities come from `virtualmachine.DesiredAffinityTags`, which returns identities with no status, so read each `Tag` from the cache by `ConcreteTagResourceName`. Carried ones come from `virtualmachine.ListLabelDerivedTags(ctx, client, vm, ownedTags)`, which needs a cache-backed client with the source-pair index and never returns `GroupedAntiAffinitySelf` `Tag`s.
- `vmlifecycle.ReconcileStatus(vmCtx, k8sClient, vcVM, data)` is the only status entry point, called once from `vmprovider_vm.go`. Pass the querier through `ReconcileStatusData`.
- `getTags` in `pkg/providers/vsphere/vmprovider_vm.go` shows how to get a REST client in the update path (`vcClient.RestClient()`).
- The `Tag` controller's `NewReconciler(ctx, client, logger, vmProvider)` takes the VM provider, and `AddToManager` passes `ctx.VMProvider`. Not-found requeues use `ctrl.Result{RequeueAfter}`, not `pkgerr.RequeueError`.
- vcsim has **no** support for `computePolicies` or the compute-policy query. Set `moVM.ComputePolicies` directly and use the fake `computePolicyQuerier`. The tagging list APIs **are** simulated (`vapi/simulator/tagging.go` in the bumped govmomi).
- Request `computePolicies` only when the gate is on. An unknown property faults the whole vm-watcher filter on older vCenter.

### Definition of done (every task)

- Copyright header, `importas` aliases, and import grouping (`make lint-go` is clean).
- Tests follow `testing-standards.md`: one `_test.go` per package, external `_test` package, `Label(...)` from `pkg/constants/testlabels`, no `time.Sleep`.
- Generated files are regenerated and checked in where types change (`make generate-go`, `make generate-go-conversions`, `make generate-manifests`, `make generate-external-manifests`).
- Commit messages follow `commit-message-standards.md`.
- Check off the task here and update `.sdd/INDEX.md` status when a phase lands.
