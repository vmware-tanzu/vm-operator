# Research: VirtualMachine Affinity Status

- **Spec**: [`spec.md`](./spec.md)

---

## R1 — Why VM Operator cannot map policy IDs today

- Policies are generated in `pkg/providers/vsphere/virtualmachine/affinity.go` (`generateAffinityPolicies`) as `vimtypes.BaseVmPlacementPolicy` values on the reconfigure `ConfigSpec.VmPlacementPolicies`. Tags are referenced **by name** (`TagIdNameId{Tag, Category: namespace}`), never by UUID.
- vpxd creates the vCenter tags (when they are named in the reconfigure) and the compute policies. Neither UUID is returned to VM Operator on the reconfigure task.
- Spec 006 identifies a `Tag` by name only. Nothing in the reconfigure path records the vCenter tag UUID; this feature resolves it into `Tag.status.id` (G5).
- vSphere 9.1 adds the VM property `computePolicies` (`[]VirtualMachineComputePolicyInfo{policyId, complianceStatus}`). It is cheap to watch, but it carries only the policy ID.

So a join key is needed: policy ID → tag UUID → tag name → `Tag` → `spec.affinity` term.

## R2 — Why one tag maps to one policy under hashed tags

`NewAffinityPolicyHash` (`affinity_policy_hash.go`) hashes `(type, strictness, topology, sorted targets)` with no VM or namespace input:

- `VmVmAffinity` → `AffinedVmsTag` is the hashed `af-<strict>-<topo>-<hash>-<key>:<value>` tag (`Tag` kind `AffinityTarget`). This is true outside placement. At placement, the target is the original tag, which is out of scope (spec NG3).
- `VmToVmGroupsAntiAffinity` → `SelfTag` is the hashed `aaf-<strict>-<topo>-<hash>` tag (`Tag` kind `GroupedAntiAffinitySelf`). `AntiAffinedVmGroupTags` are the derived `GroupedAntiAffinityTarget` tags (or the originals, without derived targets).

A policy is therefore uniquely determined by its defining tag, and two VMs with identical policy inputs share both the tag and (as vpxd dedups by its inputs) the policy. Under the legacy non-hashed path, the original `key:value` tag is the target of several policies, so the mapping is not one-to-one (spec NG2).

Hashes are one-way, so VM Operator cannot recover a policy's inputs from a tag name. It regenerates the expected names forward from `spec.affinity` instead. `DesiredAffinityTags` already does this when it computes `Tag` identities, and each policy `Tag`'s immutable `spec.policy` holds the full canonical inputs.

## R3 — govmomi APIs (fork branch `aniketd/dev-branch`)

The fork branch merges `aniketd/tag-category-name-filters` and `aniketd/policy-status-apis`.

| Package | API | Notes |
|---------|-----|-------|
| `vapi/tagging` | `Manager.ListCategories(ctx, names...)` → `[]CategoryListItem{CategoryID, Info}` | `GET /api/vcenter/tagging/categories?names=…`, vSphere 9.0+, pages via marker |
| `vapi/tagging` | `Manager.ListTags(ctx, names...)` → `[]TagListItem{Tag, Info{Name, Category}}` | `GET /api/vcenter/tagging/tags?names=…`; no category filter, so callers filter by category ID |
| `vapi/tagging` | `Manager.ListTagsForCategory(ctx, category, names...)` | Convenience: one category lookup plus one tag lookup. Two REST calls per invocation, so prefer caching the category ID |
| `vapi/vcenter/vm/compute/policies` | `Manager.Query(ctx, QuerySpec{VMs, Filter, Iterate})` / `QueryAll` | `POST /api/vcenter/vm/compute/policies?action=query`; `PolicyInfo{Status, Policy, Name, Capability, Strictness, Topology, Tag}`; per-VM `Errors` map; stored status only, does not evaluate |
| `vapi/vcenter/compute/policies` | `Manager.ListInfos` / `ListAllInfos` | Policy catalog with `Names` / `Policies` filters. Not required by this design; it is an alternative if `PolicyInfo.Tag` proves insufficient |
| `vim25/types` / `vim25/mo` | `VirtualMachineComputePolicyInfo{PolicyId, ComplianceStatus}`, `mo.VirtualMachine.ComputePolicies` | `minAPIVersionForType` is `9.1.0.0` |
| `vapi/simulator` | `tagging.go` | vcsim handlers for the new tagging list APIs exist |

Gaps:

- No vcsim support for the `computePolicies` property or `vm/compute/policies?action=query`, and none is planned for this feature (spec NG9). Tests set `moVM.ComputePolicies` directly and use a fake querier.
- `PolicyInfo.Tag` is the policy's single defining tag: `AffinedVmsTag` for `VmVmAffinity`, and the `SelfTag` for `VmToVmGroupsAntiAffinity` (confirmed). The anti-affined group tags are not returned, and they are not needed to report status.
- The compliance values of `VirtualMachineComputePolicyInfo.ComplianceStatus` differ from the REST `ObjectCompliance` values. The property reports PascalCase values (`Compliant`, `NonCompliant`, observed on a live vCenter), while the REST query reports `COMPLIANT`, `NOT_COMPLIANT`, `NOT_APPLICABLE`, and `UNKNOWN`. Only the property values are mapped, because status compliance comes only from `computePolicies`. `NotApplicable` and `Unknown` follow the same PascalCase pattern but have not been observed yet.

## R4 — Existing plumbing that this feature reuses

- **Property collector**: `pkg/util/vsphere/watcher/watcher.go` `DefaultWatchedPropertyPaths()`. Adding a path makes vm-watcher enqueue the VM on change. Requesting a property that vCenter does not know (pre-9.1) faults the whole filter, so the path must be added only when the `VMHardAffinityDuringExecution` capability gate is on. The capability implies a vCenter that supports the property.
- **VM update fetch**: `pkg/providers/vsphere/vmprovider_vm.go` `VMUpdatePropertiesSelector` (`config`, `guest`, `summary`, …). `computePolicies` is top-level, so it must be added explicitly, and only when the gate is on.
- **Attached tags**: `getTags` already calls `tags.Manager.ListAttachedTags` on every update (step 3) and stores the tag IDs through `pkgctx.WithVMTags`. This confirms that a REST client is available in the update path. The attached IDs are not names, so they cannot replace R3 resolution.
- **Desired identities**: `DesiredAffinityTags(ctx, vm, …)` in `affinity_tags.go` returns the prefix-free `TagSpec`s that a VM requests. `ConcreteTagResourceName(namespace, name)` gives the `Tag` resource name, so a VM can `Get` its owned policy `Tag`s from the cache by name. `ListLabelDerivedTags` returns the `Tag`s a VM carries through its labels. Mapping a reported policy ID back to a `Tag` uses a new field index on `status.policyID`.
- **`Tag` → VM fan-out**: `kubeutil.TagToVirtualMachineMapper` (`pkg/util/kube/vmtags.go`) already enqueues a `Tag`'s owners and the VMs carrying it, which is the set that needs to see a `status.id` or `status.policyID` transition. `TagFanOutPredicate` drops every `Update` event, so it gets a gated `Update` branch for those transitions, and the existing watch is reused.
- **`Tag` controller**: `controllers/vspherepolicy/tag/tag_controller.go` gets the VM provider from `pkgctx.ControllerManagerContext.VMProvider` in `AddToManager`.
- **Flag wiring**: `pkg/config/capabilities/capabilities.go` maps `CapabilityKeyVMHardAffinityDuringExecution` to both `TaggingAPI` and `VMHardAffinityDuringExecution`.
- **Existing status precedent**: `status.policies []PolicyStatus` reports vSphere Policy (`ComputePolicy` CR) application. It is a different source and stays separate.

## R5 — Alternatives considered

| Alternative | Rejected because |
|-------------|------------------|
| Query the compute-policy API on every reconcile and skip caching | One REST call per VM per reconcile. vm-watcher drives frequent reconciles, which violates G7 |
| List all compute policies (`compute/policies.ListAllInfos`) and match by name | Scans the whole vCenter. Policy naming by vpxd is not a documented contract |
| Cache the mapping in VM annotations or ExtraConfig | Duplicates the mapping per VM. A shared policy would be resolved N times instead of once (G6) |
| Resolve tag UUIDs in the VM controller instead of the `Tag` controller | Repeats per VM. The `Tag` is the natural single owner of its vCenter identity |
| Resolve tags with `vapi/tags` `GetTagForCategory` / `GetTags` | Deprecated in the fork and O(all tags) without name filters |
| Store `policyID` on `Tag.spec` | `spec` is the immutable identity (spec 006 G9). The policy ID is observed state |
| Trigger the reverse lookup on any unknown policy ID, with a per-VM in-memory negative cache of non-affinity policy IDs | Rejected in review: per-VM memory grows with VM count. The trigger is instead driven by related `Tag`s that lack `status.policyID`, so unrelated policies alone never cause a query |
| Build `status.affinity` only from the VM's own `spec.affinity` | The status is driven by VM state. Label-only VMs that vCenter associates with a policy must report it too, and the `Tag`'s `spec.policy` supplies the entry's identity |

## R6 — Review decisions (2026-11-24)

| # | Question | Decision |
|---|----------|----------|
| 1 | Gate | Reuse the `VMHardAffinityDuringExecution` capability (`TaggingAPI && VMHardAffinityDuringExecution`). No new flag |
| 2 | Label-only VMs | Report policies too, because the status is driven by what vCenter reports for the VM. The entry identity comes from `Tag.spec.policy` |
| 3 | Aggregate condition | No. Status per policy is enough |
| 4 | Surfacing failures | Logs only |
| 5 | Out-of-band tag or policy recreation | Out of scope |
| 6 | Group-level roll-up | No |
| 7 | Epic | vmop-4283 |
| 8 | `PolicyInfo.tag` for grouped anti-affinity | The `SelfTag`. Anti-affined tags are not returned or needed |
| 9 | Compliance enums | The SOAP and REST values match |
| 10 | govmomi | The required changes are checked in as a govmomi bump |
| 11 | vcsim | No vcsim support for compute policies now. Tests use fakes |
| — | Writer of `Tag.status.policyID` | The VM update path |
| — | Missing vCenter tag | The `Tag` controller retries with backoff, 5s up to 5m |
| — | Repeated lookups | Use `Tag.status.policyID` index lookups. No per-VM negative cache, and no query on every reconcile |
| — | Query matches nothing (policy not yet associated) | Not an error. When async signal is off, requeue the VM after 30s; when on, the property collector wakes it. It is implemented through `requeueDelay` on `Pending` entries, because a `RequeueError` is hidden by `NewAggregate` and would mask real errors in `ResultFromError` |
