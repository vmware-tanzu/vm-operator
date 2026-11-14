# Feature Specification: VM Group Power-On and Power-Off Ordering

- **Feature branch**: `power-off-ordering`
  - **Fork**: `vmware-tanzu/vm-operator`
  - **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-09-30
- **Status**: Draft
- **Epic**: vmop-4060
- **Stories**: vmop-4046 (power-off and power-on), vmop-4047
- **Last reconciled**: 2026-11-02. Extended to power-on: the same walk applies in forward order, and `PowerOnDelay` counts from completion of the previous boot-order group. 2026-10-31: redesigned as a stateless, level-triggered sequence. `PowerOffDelay` keeps its meaning as a wait, counted from completion of the previous boot-order group (VMware Cloud Director precedent). Earlier input: WIKI page 3125019495 — Boot Order Delay Refactor

The earlier proposal to turn `PowerOffDelay` into a `trySoft` graceful-shutdown budget is withdrawn. It needed a budget transport to the provider and it broke the meaning of the field for existing users. Power-on follows the wiki's stage model (confirm the observed power state of a stage, then wait the next stage's delay), but the wiki's `MaxDelayTimeout` field is not adopted.

---

## Background

A `VirtualMachineGroup` powers its boot-order groups off in reverse order. Today the group controller computes an absolute apply time per boot-order group as the cumulative sum of `PowerOffDelay` values and stamps it on all members at once. The schedule ignores how long a group takes to shut down, so ordering is not guaranteed:

- A guest shutdown can block for up to 5 minutes. If the delay is shorter than the time a group needs, the next group starts while the previous group is still shutting down (vmop-4046).
- The delay does not follow the actual progress of the previous group (vmop-4047). This spec resolves it the way VMware Cloud Director does: a group is gated on completion of the previous group, and the delay is a fixed wait after that completion.

Power-on has the same problem in forward order. The controller stamps every member at once with the request time plus the cumulative `PowerOnDelay` values. If a VM takes longer to power on than the delay (placement, a slow power-on task, a `poweron.check` annotation), the next boot-order group powers on before it. Power-on uses the same walk as power-off, in forward order.

## Design principle

The group stores no plan. On each reconcile, the controller works out the next step from two inputs only: the current group spec and the member state. Every change in the middle of a sequence then has the same answer: **the controller continues as if the request started now, from the current state.**

Kubernetes StatefulSet (`OrderedReady`) works this way. Its documentation says: "If web-0 were to fail after web-2 has been terminated and is completely shutdown, but prior to web-1's termination, web-1 would not be terminated until web-0 is Running and Ready."

Two different checks are used. Do not mix them:

- **Release check** uses the member's `spec.powerState`. It answers "did the group already release this member?" A member whose spec already equals the target is not pushed or stamped again.
- **Completion check** uses the member's observed `status.powerState`. It answers "has this boot-order group really reached the target, so the next one can start?" A member that does not exist, or that is not linked to the group, is not complete.
  - Power-off: a member is complete when it is observed `PoweredOff`, or observed `Suspended` when the group's `powerOffMode` is `soft`. With `hard` and `trySoft` the provider powers a suspended VM off, so the walk waits for `PoweredOff`.
  - Power-on: a member is complete when it is observed `PoweredOn`. A `Suspended` VM is not complete; the provider resumes it. The walk does not wait for the guest OS or VMware Tools to be ready (vCD and WIKI page 3125019495 do the same).

## Goals

The goals below are written for power-off. Unless a goal says otherwise, they apply to power-on in the same way, with forward order, `PowerOnDelay`, and the power-on completion check.

- While a power-off request is pending, the controller MUST find the first boot-order group (in reverse order) that is not complete and release it. A boot-order group is complete when every member is complete (see the completion check above). The controller MUST NOT release later boot-order groups.
- To release a member, the controller MUST set the group's power state and modes on the member, and stamp `apply-power-state-time` with `now + PowerOffDelay` of the boot-order group. It MUST do this only when the member's `spec.powerState` differs from the target, so a later reconcile does not move the stamp. If there is no delay to apply, the member is released with no stamp, and any stale stamp is removed.
- `PowerOffDelay` MUST be a fixed wait before the members of its boot-order group power off. It starts when the previous boot-order group is complete. It MUST NOT overlap with the shutdown of the previous group.
- The first boot-order group in power-off order (the last list entry) MUST be released at once. It has no previous group to wait for, so its `PowerOffDelay` has no effect. Today it waits its delay from the request time; this changes.
- The `powerOffMode` behavior in the VM provider MUST NOT change: `hard` powers off at once, `trySoft` waits up to the default 5 minutes for a guest shutdown and then powers off hard, `soft` never forces and retries on timeout. In all modes the next boot-order group MUST wait for observed `PoweredOff`.
- When every boot-order group is complete, the request MUST be closed. After that, members MAY diverge from the group. The group reports `NotSynced` and does not revert them (same as today).
- A group that is blocked MUST say which member blocks it and why, in the member status.
- The group MUST NOT store a plan: no stage index, no stage start time. Operation identity is the existing request time (`last-updated-power-state-time` annotation).
- While a power-on request is pending, the controller MUST release boot-order groups in forward order, one at a time, with the same release and completion rules. `PowerOnDelay` MUST be a fixed wait that starts when the previous boot-order group is observed `PoweredOn`. It MUST NOT be cumulative.
- The first boot-order group in power-on order (the first list entry) MUST be released at once. Its `PowerOnDelay` has no effect. Today it waits its delay from the request time; this changes.
- Suspend behavior MUST NOT change: all members are set at once, with no order and no delay.

## Non-goals

- Waiting for guest OS, VMware Tools, or application readiness during power-on.
- Adding an API field or a new annotation contract. The existing `apply-power-state-time` annotation is reused.
- Turning `PowerOffDelay` into a shutdown time limit. `trySoft` keeps its 5 minute default.
- Changing suspend behavior.
- Freezing a snapshot of the spec when the request is made. This needs stored state, still needs rules for deleted members, and stops a user from fixing a stuck sequence by editing the group.
- Interrupting an operation that already runs on vSphere.

## User stories and acceptance criteria

### US1 — DevOps user powers off a group with slow guests (vmop-4046)

- Given groups A (order 0) and B (order 1), `powerOffMode: trySoft`, and a guest in B that blocks shutdown, when the parent group is set to `PoweredOff`, then B's members start guest shutdown first. A's members are not released until B's members are observed off, including while the soft wait runs, while hard power-off is pending, and when hard power-off has failed.
- Given the same setup with `powerOffMode: soft`, then A's members are not released until B's members are `PoweredOff`, however long that takes.
- Given `powerOffMode: hard`, then hard power-off is requested as each boot-order group is released; after its members are observed off, the next boot-order group is released after its `PowerOffDelay` and not earlier.
- Members of A keep their old `spec.powerState` until A is released. The member status shows `Pending` with reason `Queued`.

### US2 — Delay counts from completion (vmop-4047)

- Given B with 10s of shutdown time and A with `PowerOffDelay: 30s`, when the parent group is set to `PoweredOff`, then A's members power off 30s after B's members are observed off, not 30s after the request. The delay does not overlap with B's shutdown time. This matches VMware Cloud Director, where the next stage starts after the stage completes and then waits the configured delay.
- Given `PowerOffDelay` omitted or `0s` on A, then A's members are released within a reconcile of the last B member reaching `PoweredOff`.
- Given `PowerOffDelay: 60s` on B (the first group in power-off order), then B's members are released at once when the request starts, with no stamp.

### US3 — Nested groups

- Given a boot-order group whose member is a child `VirtualMachineGroup`, the child group counts as powered off only when all of its own members, at any depth, are observed powered off. The parent works this out from the current descendant membership, not from the child's status list.
- The parent's `PowerOffDelay` for the boot-order group that holds the child group is carried by the stamp on the child group. The child group does not release its first boot-order group until that stamp has passed.
- If a descendant VM is powered on after the child group finished its power-off, and the parent's request is still pending, the parent starts a new request on the child group, which powers the VM off again.
- During a power-off, the parent's member status for the child group shows `PowerStateSynced` with reason `Queued` or `Pending`; the `Pending` message names the descendant member that is not powered off.

### US4 — Power-on follows completion (vmop-4046)

- Given groups A (order 0) and B (order 1), when the parent group is set to `PoweredOn`, then A's members are released at once with no stamp, even if A has a `PowerOnDelay`. B's members are not released until every A member is observed `PoweredOn`. B's members keep their old `spec.powerState` and show `Queued`.
- Given B with `PowerOnDelay: 30s`, then B's members power on 30s after the last A member is observed `PoweredOn`, not 30s (or the sum of the delays) after the request.
- Given an A member that does not power on (missing, unlinked, a failed power-on, or a `poweron.check.vmoperator.vmware.com/` annotation), then B is not released and the request stays open.
- Given a child group in A, then the child counts as powered on only when all of its VMs, at any depth, are observed `PoweredOn`. The child does not release its first boot-order group until the parent's stamp has passed.
- When every boot-order group is observed `PoweredOn`, the request is closed.

## Behavior when things change during a sequence

The tables below use a power-off example. A power-on sequence behaves the same way in forward order: for example, a user who powers off a released member while the power-on runs gets it powered on again, and later boot-order groups wait for it.

Example: power-off with boot-order groups web → api → db. web is off, api is shutting down, db has not been released.

### The group changes

| Change | Result |
|--------|--------|
| `powerState` Off → On | New request, in power-on order. db is still on, so it is complete. api gets On; api VMs that are shutting down go off first, then come back on. web waits until every api member is observed `PoweredOn`. |
| `powerOffMode` `trySoft` → `hard` | api members whose spec differs in mode get `hard` on the next reconcile (mode is updated without a new stamp). db gets `hard` when released. An operation already in the soft wait is not interrupted. |
| `PowerOffDelay` changes | Applies to members that have not been stamped. A stamp already written is kept. |
| Member added to web | web is not complete again. The new member is released, and db waits. |
| Member removed from api | It is not counted. If it was the last api member still on, api is complete and db is released. |
| Boot orders reordered | The new order applies from the next reconcile. |
| Force sync (`nextForcePowerStateSyncTime: now`) | New request with the same target. Complete boot-order groups pass at once. Members whose spec already equals the target are not stamped again. |

### A VM changes

| Change | Result |
|--------|--------|
| User sets web-1 to On | web-1 powers on, so web is not complete. The group sets Off again (the spec differs) with a new stamp, and db waits until web-1 is off. api continues. |
| Someone powers on web-1 in vCenter | The VM controller sets it back to its spec (Off). db waits until then. |
| User powers off db-1 early | Allowed. db-1 is already complete when db is released. |
| User suspends web-1, mode `soft` | `soft` does not power off a suspended VM. web-1 counts as complete, because a suspended guest runs nothing. web-1 stays suspended and its member status shows it is not synced. This matches vCD. |
| User suspends web-1, mode `hard` or `trySoft` | The provider powers web-1 off. db waits until it is observed `PoweredOff`. |
| api-1 never powers off (`soft`, no VMware Tools) | db is never released. Ways out: change the group's mode, flip `powerState`, or remove api-1 from `bootOrder`. Changing api-1's own mode does not help, because the group pushes the group's mode again. |
| api-1 deleted, or not created yet | Counts as not complete. db is not released, and the request stays open. The member status shows `NotFound`. If the walk moved on and the VM came back powered on, the order would be broken. The group does not create members. When someone creates api-1 with the same name and `groupName`, the group sets it Off. The way out is to remove api-1 from `bootOrder`. |
| api-1 unlinked (`groupName` changed) | The group cannot push to it. Counts as not complete. The member status says why. |
| Controller restarts | Nothing is lost. The next reconcile works out the same step. The stamp and the spec are on the objects. |

### Limits that stay

- An operation already running on vSphere is not interrupted. A guest shutdown in progress finishes or times out first. A change takes effect at the next step, not at once. When the group goes Off → On, api VMs that are shutting down go off first, then come back on.
- The group does not take back a boot-order group it already released. If a user powers on web-1 while api shuts down, api keeps shutting down. The order is guaranteed only for when each boot-order group is released. StatefulSet has the same limit.
- A stuck member blocks later boot-order groups until someone acts. StatefulSet has the same problem. For power-off, vCD undeploy does not block later stages on failure; this is a deliberate difference. For power-on, vCD also stops later stages after a failed stage, but it fails the operation, while VM Operator waits and continues when the member reaches the target (see [research.md](research.md)).
- A re-released member (the user turned it on and the group turned it off) gets a new pause.
- A member that is deleted while it is still in `bootOrder` stops the power-off at its group until someone updates `bootOrder` or creates the VM again.

## Decisions (2026-11-02)

- Power-on uses the same walk as power-off, in forward order. This spec covers both directions.
- A member is complete for power-on when it is observed `PoweredOn`. No guest or VMware Tools readiness wait.
- The first boot-order group in power-on order is released at once. Its `PowerOnDelay` has no effect.
- Power-off keeps `powerOffMode`, which differs from vCD on purpose. vCD fails a guest shutdown that times out and has no hard fallback. VM Operator powers off hard after the soft timeout in `trySoft`, powers off at once in `hard`, and never forces in `soft`, where the walk waits for the member.

## Decisions (2026-10-31)

- A `Suspended` member is complete when `powerOffMode` is `soft`. With `hard` and `trySoft` the walk waits for `PoweredOff`. vCD skips suspended VMs during ordered undeploy (see [research.md](research.md)).
- A missing member (deleted or not created) blocks the walk and the close of the request. Today it already blocks the close.
- The first boot-order group in power-off order is released at once. Its `PowerOffDelay` has no effect.

## Open questions

None. Power-on is tracked under vmop-4046 (decided 2026-11-02).
