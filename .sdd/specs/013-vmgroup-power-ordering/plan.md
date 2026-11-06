# Implementation Plan: VM Group Power-On and Power-Off Ordering

## Summary

Replace the precomputed cumulative power-off schedule with a stateless, level-triggered walk. On each reconcile, the group controller finds the first boot-order group (in reverse order) that is not complete and releases it: it sets the target on the members whose spec differs and stamps the existing `apply-power-state-time` annotation with `now + PowerOffDelay`. The VM provider is not changed. See [spec.md](spec.md) and [research.md](research.md).

**Extended 2026-11-02 to power-on.** The same walk runs in forward order for power-on, with `PowerOnDelay` and the power-on completion check (observed `PoweredOn`). Design items 1 to 8 apply to both directions unless they say otherwise.

**Draft, redesigned 2026-10-31.** An earlier plan stored operation state and a `trySoft` budget. Both are withdrawn. `PowerOffDelay` keeps its meaning as a wait. It now starts when the previous boot-order group is complete. WIKI page 3125019495 proposes a broader power-on/off refactor and a new timeout field; those are not adopted here.

## Technical context

- Go, controller-runtime. API `v1alpha6` only for doc text; older versions carry the same field through conversion and need comment-only updates if their docs are generated.
- Modules: root and API documentation. No Go type, conversion, provider or webhook change. No new field, no new annotation, no status field.

## Constitution check

- Controllers stay thin: the stage walk and completion math go in `pkg/util/vmopv1`, called from `reconcileMembers`.
- No new vSphere calls in the controller. The provider keeps the power op and the existing apply-time gate.
- Conditions communicate state; they do not gate progression. The walk uses `spec.powerState` (release check) and observed `status.powerState` (completion check). The new `Queued` and blocked-member reasons are reporting only.
- VC op IDs: no new VC call sites.

## Design

The reconcile is level-triggered and restart-safe because it keeps no plan. Inputs: the group spec, and the member spec and observed power state.

1. **Walk (power-on and power-off).** Iterate boot-order groups in reverse for power-off, and in order for power-on. A boot-order group is complete when every member is complete (item 5). Find the first incomplete one. Release it. Do not touch later boot-order groups; their members keep their old spec and get `PowerStateSynced=False`, reason `Queued`. Suspend keeps the existing single-pass behavior, with no stamp.
2. **Release check on the spec.** For each member of the released group, compare `spec.powerState` with the target:
   - Differs: set power state, `powerOffMode` and `suspendMode`, and set `apply-power-state-time` to `now + PowerOffDelay`. Set no stamp, and remove any stale one, when the delay is omitted or zero, or when the boot-order group is the first in walk order (the last list entry for power-off, the first for power-on); that group has no previous group to wait for. Power-on uses `PowerOnDelay`.
   - Equal: do not touch the stamp. Update the modes only if they differ. This keeps the pause from moving on later reconciles, and from restarting after the provider has deleted the stamp and while the guest shutdown still runs.
3. **Operation identity.** The existing `last-updated-power-state-time` annotation. A power-state change or a force sync sets it and starts a new request. No new ID.
4. **Latch.** `shouldUpdatePowerState` stays as is. While the request is pending, each reconcile re-runs the walk. Close the request (set `Status.LastUpdatedPowerStateTime`) only when every boot-order group is complete and there are no member errors. Today the latch closes after one pass; now it stays open for the whole sequence. After the close, members can diverge (`NotSynced`), as today.
5. **Done.** For power-on, a member VM is complete when observed `status.powerState` is `PoweredOn`; `Suspended` is not complete. For power-off, a member VM is complete when observed `status.powerState` is `PoweredOff`, or `Suspended` when the group's `powerOffMode` is `soft` (the provider does not act on a suspended VM in `soft`; vCD skips suspended VMs too). A child group is complete when every VM in its current membership, at any depth, is complete by the same rule. The parent reads the child's spec membership and the VMs. It does not use the child's status list or `Ready`; `VirtualMachineGroup.GetPowerState()` returns empty and member entries carry no aggregate state. A missing member (deleted or not created) and an unlinked member count as not complete, and the member status says why (`NotFound`, `NotMember`). The group does not create members.
6. **Nested groups.** The parent releases a child group like a member: it sets the spec and stamps the child group's `apply-power-state-time` with `now + PowerOffDelay` of the boot-order group that holds it. The child group does not release its first boot-order group while that stamp is in the future, and requeues for the time left. The child then runs its own walk, with its own delays. During a power-off the parent sets `PowerStateSynced` on the child group's member status: `Queued`, `Pending` with the blocking descendant in the message, or `True`; outside a power-off the condition is removed, as a group has no observed power state. If a released child group is not powered off but its own request is closed (for example a user powered on one of its VMs afterwards), the parent sets a new `last-updated-power-state-time` on the child, which reopens the child's request so it powers its members off again. A privileged account may set that annotation (`validatePowerState`).
7. **Wakeups.** Existing VM and child-group watches (`MemberToGroupMapperFn`) enqueue the parent when a member changes. The pause after completion needs no group timer for VM members: the provider already requeues the VM until the stamp, and the VM's power-off then enqueues the group. A child group that holds for its parent's stamp has no such event, so it returns `RequeueError{After: ...}` for the time left. The `RequeueError` is returned after `aggregateOrNoRequeue`, not inside it, because k8s `aggregate` implements `Is` but not `As` or `Unwrap`, and `pkgerr.ResultFromError` uses `errors.As`. A regular member error takes precedence, since it is retried with a backoff. The mapper only enqueues for members whose `GroupLinked` condition is true; an unlinked member does not wake the group, so recovery needs a group edit or force sync. A parent requeue cannot interrupt a VM reconcile that waits for guest shutdown.
8. **Provider.** No change. It keeps the apply-time gate, the `hard`, `trySoft` (5 minute default) and `soft` behavior.
9. **Power-on** uses the same walk in forward order (added 2026-11-02). `FirstIncompleteBootOrder` and `IsGroupMemberInPowerState` in `pkg/util/vmopv1` take the target from the group spec.

### Deviations from the current implementation

| Area | Today | New |
|------|-------|-----|
| Stamping | One pass stamps every boot-order group with a cumulative absolute time (`reconcileMembers`). | Only the first incomplete boot-order group is released. Later groups are not patched. |
| Stamp value | Request time + cumulative delays. Same value on every pass. | `now + PowerOffDelay` at release, written once per member (gated on `spec.powerState`). |
| Re-push gate | Skips a member when observed `status.powerState` equals the target (`reconcileMember`). A member that is shutting down is pushed again. | Gate on `spec.powerState` for the stamp. The observed state is used only for completion. |
| `PowerOffDelay` | A wait before the group, counted from the request, cumulative. | A wait before the group, counted from completion of the previous group. Not cumulative. |
| `PowerOnDelay` | A wait before the group, counted from the request, cumulative. | A wait before the group, counted from completion of the previous group. Not cumulative. |
| First group in power-on order | Waits its `PowerOnDelay` from the request. | Released at once. Its delay has no effect. |
| First group in power-off order | Waits its `PowerOffDelay` from the request. | Released at once. Its delay has no effect. |
| Suspended member, `soft` | The provider leaves it suspended. Nothing waits for it. | Same provider behavior. Counts as complete, so nothing waits for it. |
| Latch | Closes after one error-free pass. | Closes when all boot-order groups are complete and there are no member errors. |
| Queued members | Spec already changed, held by the apply time. | Spec unchanged until released. New `Queued` reason on `PowerStateSynced`. |
| Child group | Receives the target and a cumulative time. It uses the time as the base of its own delays. | Receives the target when released, with a stamp it must wait for. Parent checks descendant VMs for completion. |
| Mode edit during a request | Re-applied in the one pending pass only. | Re-applied on every reconcile until the request closes. Works as a way out of a stuck request. |
| Missing member | `NoRequeueError` keeps the request open; other members are not held. | Blocks the walk at its group, and keeps the request open. |
| Timers | The provider requeues until the stamp. | Same, plus the group requeues until the next release time. |
| Tests/E2E | Assert cumulative annotations and elapsed delays. | Rewrite. See the test strategy. |

### Decisions required before implementation

- None. The spec decisions of 2026-10-31 settle suspended members, missing members and the first group's delay.
- No `model.md` is needed: no field, status or webhook change.

### Logging and diagnosis

Add structured logs for group release, the blocking member, early completion and request replacement. Include group identity, boot-order index, member identity where relevant, mode, and the stamp time. Repeated waiting belongs at a verbose level; log transitions normally and failures with context. Any new Events or condition reasons need explicit deduplication and tests.

## API / CRD strategy

No new user-facing field. Update the `PowerOnDelay`, `PowerOffDelay` and `BootOrder` comments in `api/v1alpha6/virtualmachinegroup_types.go`. They say that delays are cumulative across groups for power-off; this changes. The new text: the delay is a wait before the members of the boot-order group power off, counted from completion of the previous boot-order group; a boot-order group is released only after the previous one is observed off; the delay of the last list entry (the first to power off) has no effect. Regenerate CRDs with `make generate-manifests`. The `PowerOnDelay` text changes in the same way, in forward order. Do not add the wiki's `MaxDelayTimeout`.

## Controller / webhook impact

- `controllers/virtualmachinegroup`: staged walk for power-on and power-off, spec-gated stamping, timed requeue.
- `pkg/util/vmopv1`: stage completion helper, including descendant completion.
- `pkg/providers/vsphere`, `pkg/util/vsphere/vm`: no change.
- Webhooks: no change expected. `ProcessPowerState` already removes the apply-time annotation on a direct group update.
- No new RBAC or feature flag; behavior stays behind the existing `VMGroups` feature.

## Test strategy

- Unit (`testlabels.Controller`): 1, 2 and 3 boot-order groups; all modes; omitted and zero delay; the first group in power-off order released at once with no stamp; already-off members; suspended members in `soft` (complete) and in `hard`/`trySoft` (wait for `PoweredOff`); a missing member that blocks, then is created and set Off; nested groups; every row of the "behavior when things change" tables in the spec (mode change, member added/removed, reorder, off → on, force sync, user powers a released member on, deleted and unlinked members); restart recovery (reconcile from a mid-sequence state gives the same step); the stamp does not move on repeated reconciles; the stamp is not rewritten after the provider deleted it while the VM is still on; the close of the request; the requeue value at the controller boundary.
- Power-on unit tests: forward walk, `Suspended` not complete, first group released at once, delay counted from completion, a missing member that blocks, a child group that is powered on again after a descendant is powered off. Rewrite the old controller test "power on the group with delay", which asserts cumulative annotations.
- Rewrite the existing controller test "power off the group with delay". It asserts cumulative annotations and does not prove predecessor completion.
- E2E in `test/e2e/vmservice/vmservice/virtualmachine/vm_group.go`: rewrite "Boot order power-off delay" (it uses hard power-off and asserts cumulative elapsed delays). Assert that the next group is not released before the previous one is off (guest that blocks shutdown, `trySoft`), and that the delay counts from completion (`hard`). Capture the next-group release time, not only completion times. Add a mid-sequence change case (user powers on a released member, group sets Off again). Add "Boot order power-on delay": the first group ignores its delay, and each gap between `runtime.bootTime` values is at least the next group's `PowerOnDelay`.

## Rollout

Keep new paths behind `pkgcfg.FromContext(ctx).Features.VMGroups`. Release note: `PowerOffDelay` is now a wait that starts when the previous boot-order group is observed off, and it is no longer cumulative from the request; the delay on the first group to power off has no effect; a member that is missing from the namespace stops the power-off at its group; the request stays open for the whole sequence, so a change to a released member is reverted until the sequence ends. `PowerOnDelay` changes in the same way: it is a wait that starts when the previous boot-order group is observed on, it is no longer cumulative, the delay on the first group has no effect, and a member that does not power on stops the power-on at its group. The boot order E2E cases are not labeled `experimental`, as they cover existing VM group behavior, not a new feature (decided 2026-11-02).
