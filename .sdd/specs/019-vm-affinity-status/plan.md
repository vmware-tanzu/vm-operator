# Implementation Plan: VirtualMachine Affinity Status

- **Spec**: [`spec.md`](./spec.md)
- **Model**: [`model.md`](./model.md)
- **Research**: [`research.md`](./research.md)

---

## Summary

Populate `Tag.status.id` and `Tag.status.categoryID` from the `Tag` controller through a new provider method that uses the tagging list APIs with name filters. In the VM update path, read the new `computePolicies` property. Map each reported policy ID to a `Tag` with a cached field-index lookup on `Tag.status.policyID`. While a related policy `Tag` is still unmapped, perform a one-time reverse lookup through the VM compute-policy query API and record `status.policyID` on the `Tag`. Rebuild `VirtualMachine.status.affinity` from the matched `Tag`s' `spec.policy`. Everything is gated on the existing `VMHardAffinityDuringExecution` capability (`Features.TaggingAPI && Features.VMHardAffinityDuringExecution`).

## Technical context

- Go modules touched: root, `api/`, and `external/vsphere-policy/`.
- vCenter: `computePolicies` and `vm/compute/policies?action=query` (vSphere 9.1+), and `/api/vcenter/tagging/*`. The capability implies a vCenter that supports them.
- govmomi: the tagging, VM compute-policy, and `ComputePolicies` changes are checked in as a govmomi bump (task T001).

## Constitution check

| Rule | Compliance |
|------|------------|
| Thin controllers / no direct vSphere calls | The `Tag` controller calls `VMProvider.ResolveTagIDs`. The policy query lives in `pkg/providers/vsphere` |
| `WithVCOpID` | `resolveTagIDs` in the provider. `queryComputePolicies` in the status step |
| `observedGeneration` + `Ready` | The `Tag` controller already sets both. The new condition is additive |
| Fan-in to a shared field | `Tag.status.policyID` is written by many VMs → `MergeFromWithOptimisticLock` plus skip-if-unchanged |
| Mapper / index pattern | `status.policyID` field index, `MatchingFields` lookups |
| API compatibility | Status-only additive fields. CRDs are regenerated and checked in |
| `+optional` / `omitempty` | Yes |
| Feature gating | `TaggingAPI && VMHardAffinityDuringExecution` at every entry point |
| E2E in the same change | Yes, in Phase 5 |

No constitution amendment.

## Project structure

```
api/v1alpha6/
  virtualmachine_affinity_status_types.go    (new)
  virtualmachine_types.go                    (Status.Affinity)
external/vsphere-policy/api/v1alpha1/tag_types.go
pkg/config/{config.go,default.go,env.go}     (AffinityStatusPendingRequeueDelay)
pkg/config/env/env.go
pkg/providers/vm_provider_interface.go       (ResolveTagIDs)
pkg/providers/fake/fake_vm_provider.go
pkg/providers/vsphere/vmprovider.go          (category ID cache)
pkg/providers/vsphere/vmprovider_tags.go     (new: ResolveTagIDs)
pkg/providers/vsphere/vmprovider_vm.go       (selector, status wiring)
pkg/providers/vsphere/vmlifecycle/
  update_status.go                           (querier in status data)
  update_status_affinity.go                  (new)
pkg/providers/vsphere/virtualmachine/
  affinity_status.go                         (new: related Tags, gate)
pkg/util/vsphere/watcher/watcher.go          (computePolicies path)
services/vm-watcher/vm_watcher_service.go
pkg/util/kube/vmtags.go                      (policyID index, predicate)
controllers/vspherepolicy/tag/tag_controller.go
controllers/virtualmachine/virtualmachine/virtualmachine_controller.go
test/e2e/vmservice/vmservice/virtualmachine/vm_group.go (status.affinity assertions in the existing hard-affinity specs)
```

## Design

### 0. Gate

The helper `virtualmachine.AffinityStatusEnabled(ctx)` returns `pkgcfg.FromContext(ctx).Features.TaggingAPI && …VMHardAffinityDuringExecution`. No new flag and no capability change are needed, because `CapabilityKeyVMHardAffinityDuringExecution` already sets both.

### 1. Tag ID resolution (`Tag` controller)

`reconcileVCenterTag` runs at the end of `ReconcileNormal`, after the owner check, the label mirror, and `Ready` are set, so resolution never affects `Ready`. It does nothing when the gate is off.

1. If `status.id != ""`, return (US2 scenario 4).
2. Call `r.VMProvider.ResolveTagIDs(ctx, tag.Namespace, [virtualmachine.AddAffinityTagPrefix(spec.name)])`.
   - The provider calls `tagging.ListCategories(ctx, namespace)` only when the category ID is not cached, then calls `tagging.ListTags(ctx, names...)` and keeps the items whose `Info.Category == categoryID`. Both calls are name-filtered. A missing category is not an error: it returns an empty category ID and no tags.
   - The provider caches category IDs by category name (`vSphereVMProvider.tagCategoryIDs`, a `sync.Map`), which removes the category call for every `Tag` after the first in a namespace. When a cached category ID finds none of the names, the entry is dropped and the lookup is retried once with a fresh category lookup, which handles a recreated category.
3. Found → set `status.categoryID`, `status.id`, and `VCenterTagResolved=True`.
4. Not found → set `VCenterTagResolved=False/NotFound` and return `ctrl.Result{RequeueAfter: d}` with a nil error. `d` is the time since the condition became `False/NotFound`, clamped to [5s, 5m], so the delay roughly doubles on each attempt and the reconciler needs no in-memory state.
5. Error → set `VCenterTagResolved=False/ResolveFailed` and return the wrapped error (rate-limited requeue).

`AddToManager` passes `ctx.VMProvider`, and `NewReconciler` takes the provider as its last parameter.

### 2. Index, watch, and fan-out

- Register the `status.policyID` field index on `Tag` (`kubeutil.TagPolicyIDIndexKey`, `kubeutil.TagPolicyIDIndexerFunc`) with `kubeutil.RegisterTagPolicyIDIndex` in the VM controller's `AddToManager` when `AffinityStatusEnabled` is true. It is registered separately from `RegisterVMTagsIndexes`, which stays gated on `TaggingAPI` alone.
- Reuse the VM controller's existing `Tag` watch. `TagToVirtualMachineMapper` enqueues the `Tag`'s owners and the VMs carrying it: owners plus the VMs with the source label for `SourceLabel` membership, and owners for `PolicyRequesters` membership (requesters are the owners).
- When `AffinityStatusEnabled(ctx)` is true, the mapper also enqueues, for a `GroupedAntiAffinitySelf` `Tag`, the VMs that carry any of its `spec.policy.targets` labels, with one `VMLabelKeyValueIndexKey` lookup per target. Those peers are not related to the self `Tag`, so they cannot record its `status.policyID` themselves. They map the policy that vCenter reports for them only through the index, after an owner records the ID. Without this fan-out, a peer that reconciled before that write would wait for the resync period. A failed lookup for one target is logged, and the other targets and the owners are still enqueued.
- `TagFanOutPredicate(ctx)` gains a gated `Update` branch. When `AffinityStatusEnabled(ctx)` is true, it admits an update of an `AffinityTarget` or `GroupedAntiAffinitySelf` `Tag` whose `status.id` or `status.policyID` changed. Every other update is still dropped, including owner-reference, label-mirror, and condition writes, so spec 006's fan-out contract is unchanged. `pkg/util/kube` reads the two feature flags through `pkgcfg` directly, because `pkg/providers/vsphere/virtualmachine` imports `pkg/util/kube`.
- This fills `Pending` entries on peer VMs, and the entries of anti-affinity peers, as soon as `status.id` is set or any one VM resolves the mapping.

### 3. Property plumbing

- `watcher.ComputePoliciesPropertyPath` (`"computePolicies"`) is not part of `DefaultWatchedPropertyPaths()`. The vm-watcher service passes `vmwatcher.WatchedPropertyPaths(ctx)` to `watcher.Start`, which appends it when the gate is on.
- `updateVirtualMachine` fetches `vsphere.VMUpdatePropertiesSelectorFor(ctx)`, which appends it to `VMUpdatePropertiesSelector` under the same gate. vcsim accepts the property and returns it empty.

### 4. Status reconcile (VM update step 5)

New `vmlifecycle.ReconcileAffinityStatus(vmCtx, k8sClient, querier)`, called from `ReconcileStatus` with the querier from a new `ReconcileStatusData.ComputePolicyQuerier` field that the provider sets. It has no return value. It runs after the attached-tags fetch and before schema upgrade, so the operator-best-practices update order is preserved.

```
if !gate: vm.Status.Affinity = nil; return
reported := moVM.ComputePolicies              // policyID -> compliance
byPolicy := {}                                 // policyID -> Tag
for id in reported:
    List Tags InNamespace, MatchingFields{status.policyID: id}  // cache
    if found: byPolicy[id] = tag
related := policy-defining Tags the VM owns or carries
    // owned: DesiredAffinityTags identities, read from the cache by
    //   ConcreteTagResourceName (AffinityTarget, GroupedAntiAffinitySelf);
    //   a Tag not in the cache yet is used as its identity (Pending)
    // carried: ListLabelDerivedTags(ctx, c, vm, owned), filtered to
    //   AffinityTarget (it never returns self Tags)
needMap := related with status.id != "" and status.policyID == ""
unclaimed := reported IDs not in byPolicy
if needMap and unclaimed:                      // query trigger
    res := querier.Query(VMs=[moref], Filter{Policies: unclaimed})
    for p in res.Items[vm].Policies:
        if t := needMap[tagID == p.Tag]:
            patch t.status.policyID = p.Policy  // lock, skip if same
            byPolicy[p.Policy] = t
owned := names of the VM's own DesiredAffinityTags policy Tags
entries := for id, t in byPolicy: entry(t.spec.policy, reported[id])
        + for t in related not in byPolicy values: entry(t, Pending)
        // origin = Self if t.name in owned, else Peer
vm.Status.Affinity = sorted(entries) or nil
if any error above: MarkAffinityStatusRetry(vmCtx)  // see "Requeue" below
```

**Query trigger.** The query runs only when a related policy `Tag` is resolved but unmapped **and** the VM reports at least one ID that no `Tag` claims. Unrelated policies alone never trigger a query, and a mapped VM never queries (G6, G7). No per-VM cache is kept, because it would not scale with VM count.

**Requeue when nothing matches.** If the query matches no unmapped related `Tag`, vCenter has not associated the policy with the VM yet. That is an expected transient state, not an error, so, when async signal is off, the VM is requeued after `AffinityStatusPendingRequeueDelay` (default 30s) with a nil error. With async signal on there is no requeue; the property collector wakes the VM when vCenter associates the policy.

The requeue is **level-triggered from status**, not returned as an error. `requeueDelay(vmCtx, err)` in `controllers/virtualmachine/virtualmachine/virtualmachine_controller.go` returns the delay when the gate is on, async signal is off, and any `vm.Status.Affinity.Policies` entry is `Pending`. This check goes after the `AsyncSignalEnabled` early return, because with async signal on the property collector reports `computePolicies` changes and the `Tag` watch covers the Kubernetes side. When the powered-on-without-IP delay also applies, the shorter delay wins, so IP discovery is not slowed. Do **not** return `pkgerr.RequeueError` from `ReconcileAffinityStatus`. Two reasons, both verified in the code:

1. `vmlifecycle.ReconcileStatus` combines step errors with `apierrorsutil.NewAggregate`, which implements `Is` but not `Unwrap() []error`. `errors.As` in `pkgerr.ResultFromError` therefore cannot find a `RequeueError` inside it, and the requeue would be logged and rate-limited as an ordinary error.
2. Even if it were surfaced, `ResultFromError` gives `RequeueError` priority over regular errors. `updateVirtualMachine` joins step errors through `getReconcileErr` (`%w, %w`), so a pending-affinity requeue would turn a real failure in a later step (config, power state) into a silent nil-error requeue.

With async signal off, requeuing on any `Pending` entry also covers the other transient states (`Tag.status.id` not resolved yet, or the policy not associated yet). That costs nothing in vCenter calls: the query still runs only under the query trigger above. The property collector (`computePolicies` changes) and the `Tag` status watch (a peer maps the `Tag`, or `status.id` is set) usually wake the VM before the delay expires, and the requeues stop once no entry is `Pending`.

**Errors.** A `Query` failure, a per-VM `Errors` entry, a failed `Tag` patch, or a failed `Tag` read is logged; a `Tag` patch conflict is logged at V(4), because VMs that share a policy race to record it. Entries keep their previous `policyID` from the old `vm.Status.Affinity` where the policy is still reported, with the compliance that `computePolicies` currently reports for it, and are otherwise `Pending`. Previous entries are matched by policy identity (type, strictness, topology key, and targets). When the related `Tag`s cannot be read, previous entries whose policy is still reported, and previous `Pending` entries, are kept. None of these fail the status reconcile. `ReconcileAffinityStatus` returns nothing to aggregate, so it never adds an error to `ReconcileStatus`.

**Requeue after an error.** Any of these errors, including a patch conflict, calls `virtualmachine.MarkAffinityStatusRetry(vmCtx)`. The VM controller adds the flag to every reconcile context with `virtualmachine.WithAffinityStatusRetryContext`, the same way it adds `ctxop`. `requeueDelay` reads it with `virtualmachine.IsAffinityStatusRetry` and returns `AffinityStatusPendingRequeueDelay`, with async signal on or off. No watch event is guaranteed to follow such an error: a failed query or read changes nothing, and a conflicting write may not change the `Tag`'s IDs, which is all the predicate admits. `requeueDelay` runs only when the reconcile returned no error, so a real error still goes through the error rate limiter, and the flag never masks it. The retry is not counted as an error and emits no event. Mark and read are no-ops on a context without the flag, so callers and tests that do not add it are unaffected.

**Querier.** `vmlifecycle.NewComputePolicyQuerier(restClient)` returns a `vmlifecycle.ComputePolicyQuerier` that wraps `policies.NewManager(restClient).Query` with `WithVCOpID(ctx, vm, "queryComputePolicies")`. The query names only this VM (`moVM.Self`), so it is a single page. Tests inject a fake, because vcsim has no support for it.

## API / CRD strategy

- Additive, status-only. Regenerate with `make generate-go`, `make generate-go-conversions`, `make generate-manifests`, and `make generate-external-manifests`.
- RBAC: the manager already has `tags/status` `get;update;patch`. Verify `config/rbac/role.yaml` after `make generate-manifests`.

## Controller / webhook impact

- **`Tag` controller**: provider dependency and the resolution step.
- **VM controller**: a gated `status.policyID` index, a gated `Update` branch in the existing `Tag` watch's predicate, a gated peer fan-out for `GroupedAntiAffinitySelf` `Tag`s in its mapper, the affinity status retry context, and the `Pending` and retry checks in `requeueDelay`.
- **Webhooks**: no change.

## Test strategy

No vcsim support exists for `computePolicies` or the query API (spec NG9). Tests set `moVM.ComputePolicies` directly and use a fake `computePolicyQuerier` that counts calls. Tagging resolution uses the vcsim tagging simulator that ships with the govmomi bump.

| Layer | What | Labels |
|-------|------|--------|
| Unit | `ResolveTagIDs`: found, missing category, empty category name, missing tag, same name in another category, category cache hit | `testlabels.VCSim` |
| Unit / envtest | `Tag` controller: resolve, `NotFound` backoff, error, no call once resolved, gate off, `Ready` unaffected | `testlabels.Controller` (+ `EnvTest`) |
| Unit | Related-`Tag` computation and status projection per kind, strictness, and topology; stable sort | pkg unit |
| Unit | `ReconcileAffinityStatus` with a fake querier and fake client: steady state makes 0 queries; first mapping makes 1 query and 1 `Tag` patch; peer VM makes 0 queries; label-only VM reports a reported policy; unrelated policies with mapped `Tag`s make 0 queries; query error keeps old values; conflict on the `Tag` patch; `Pending` cases; gate off clears status; compliance mapping; the retry context is marked on every error and only then | pkg unit |
| Unit | `status.policyID` index function; `TagFanOutPredicate` with the gate on (ID changes on `AffinityTarget`/`GroupedAntiAffinitySelf` admitted; other kinds and other updates dropped) and off (every update dropped); `TagToVirtualMachineMapper` fans a `GroupedAntiAffinitySelf` `Tag` out to the carriers of any target label only with the gate on | pkg unit |
| Unit | `requeueDelay`: a marked retry gives the delay with async signal on and off, and a provider error is returned with no delay | `testlabels.Controller` |
| Unit | Watcher paths and update selector with the gate on and off | pkg unit |
| E2E | Host anti-affinity between two VMs, plus an affinity requester and a label-only peer: all report `Compliant` entries with matching `policyID`s; the `Tag`s have `status.id` and `status.policyID`; removing affinity clears status | `test/e2e` affinity labels |

## Dependencies

1. A govmomi bump containing `vapi/tagging` (name-filtered list APIs and the simulator), `vapi/vcenter/vm/compute/policies`, and `mo.VirtualMachine.ComputePolicies` (task T001).

## Rollout / migration

- The capability is already the gate for spec 006. When it is on, `Tag`s resolve on their next reconcile (a manager restart reconciles every `Tag`), and VMs fill `status.affinity` on their next reconcile.
- When it is off, `status.affinity` is cleared. `Tag.status` IDs are left in place: they are harmless, and they are reused on re-enable.

## Complexity tracking

| Item | Justification |
|------|---------------|
| Two writers of `Tag.status` | Disjoint fields. The mapping is discovered per VM but owned by the shared `Tag`, so it is resolved once per policy |
| Requeue instead of a negative cache | Avoids per-VM memory that grows with VM count. The requeue comes from `requeueDelay` reading `Pending` entries, not from an error, and stops when no entry is `Pending` |
