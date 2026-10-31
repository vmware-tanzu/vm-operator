# Tasks: VM Group Power-Off Ordering

Redesigned 2026-10-31 as a stateless, level-triggered walk with `PowerOffDelay` as a wait after completion of the previous group. The budget transport and the provider tasks are removed. Implementation started 2026-10-31.

## Phase 1 — Setup

- [T001] Done 2026-10-31: open questions resolved (suspended in `soft` is complete, missing member blocks, first group is released at once) and recorded in the spec and plan (.sdd/specs/013-vmgroup-power-off-ordering/plan.md, .sdd/specs/013-vmgroup-power-off-ordering/spec.md)

## Phase 2 — Foundational

- [T002] Done: stage completion helper `FirstIncompletePowerOffBootOrder`; `PoweredOff` is complete, `Suspended` is complete only in `soft`; missing and unlinked members are not complete (pkg/util/vmopv1/group_power_state.go)
- [T003] Done: descendant completion for nested groups from current spec membership, with loop detection (`IsGroupMemberPoweredOff`) (pkg/util/vmopv1/group_power_state.go)
- [T004] Done: unit tests for T002 and T003 (pkg/util/vmopv1/vmgroup_test.go)

## Phase 3 — US1: ordering with slow guests (vmop-4046)

- [T005] Done: in `reconcileMembers`, power-off releases only the first incomplete boot-order group, gates push and stamp on `spec.powerState`, stamps `now + PowerOffDelay` (no stamp, and removes a stale one, if the delay is omitted or zero or the group is the first in power-off order), marks later members `Queued`, and closes the request only when all groups are complete and there are no member errors; power-on and suspend are unchanged (controllers/virtualmachinegroup/virtualmachinegroup_controller.go)
- [T006] Done: a child group holds its first release until the parent's stamp passes and returns `RequeueError{After}` outside `aggregateOrNoRequeue` (controllers/virtualmachinegroup/virtualmachinegroup_controller.go)
- [T007] Done: controller tests for the walk, the spec gate, a stamp that does not move, re-release after a user powers a member on, and the close of the request; the old "power off the group with delay" and "try soft" tests are replaced (controllers/virtualmachinegroup/virtualmachinegroup_controller_test.go)

## Phase 4 — US2: delay counts from completion (vmop-4047)

- [T008] Done: tests that the delay starts at completion of the previous group and that the first group in power-off order ignores its delay (controllers/virtualmachinegroup/virtualmachinegroup_controller_test.go)

## Phase 5 — US3: nested groups

- [T009] Done: descendant completion is used by the walk; controller test for a child group holding for its parent's stamp (controllers/virtualmachinegroup/virtualmachinegroup_controller.go, controllers/virtualmachinegroup/virtualmachinegroup_controller_test.go)

## Phase Final — Polish

- [T010] Done: `PowerOffDelay` and `BootOrder` docs updated, CRDs regenerated, and the user docs describe power-off ordering (api/v1alpha6/virtualmachinegroup_types.go, config/crd/, docs/concepts/workloads/vm.md, docs/concepts/workloads/vm-group.md). `docs/ref/api/*.md` is not regenerated: it is stale on main (it predates `PowerOffDelay`), and regenerating it pulls in unrelated changes.
- [T011] [vmop-4046, vmop-4047] Written, not yet run on a cluster (all `experimental`): the "Boot order power-off delay" case asserts each delay counts from the previous group's power-off and that the first group's delay is ignored; the new "Boot order power-off ordering" context covers a user powering on a released member mid-sequence (`hard`), `soft` holding the next group until the mode is changed to `hard`, and `trySoft` powering off hard after the soft timeout before the next group is released. The blocked guest masks `poweroff.target` and `halt.target` through cloud-init; whether this stops a VMware Tools guest shutdown on the default image must be confirmed on a cluster (test/e2e/vmservice/vmservice/virtualmachine/vm_group.go)
- [T012] Done: row 013 exists in the spec table (.sdd/INDEX.md)
- [T013] Done: release and close logs, verbose logs while waiting, and the `Queued` reason on `PowerStateSynced` (controllers/virtualmachinegroup/virtualmachinegroup_controller.go)
