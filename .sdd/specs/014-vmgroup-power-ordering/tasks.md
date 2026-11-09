# Tasks: VM Group Power-On and Power-Off Ordering

Redesigned 2026-10-31 as a stateless, level-triggered walk with `PowerOffDelay` as a wait after completion of the previous group. The budget transport and the provider tasks are removed. Implementation started 2026-10-31. Extended to power-on 2026-11-02 (Phase 6).

## Phase 1 — Setup

- [T001] Done 2026-10-31: open questions resolved (suspended in `soft` is complete, missing member blocks, first group is released at once) and recorded in the spec and plan (.sdd/specs/014-vmgroup-power-ordering/plan.md, .sdd/specs/014-vmgroup-power-ordering/spec.md)

## Phase 2 — Foundational

- [T002] Done: stage completion helper `FirstIncompletePowerOffBootOrder` (renamed to `FirstIncompleteBootOrder` in T014); `PoweredOff` is complete, `Suspended` is complete only in `soft`; missing and unlinked members are not complete (pkg/util/vmopv1/group_power_state.go)
- [T003] Done: descendant completion for nested groups from current spec membership, with loop detection (`IsGroupMemberPoweredOff`, renamed to `IsGroupMemberInPowerState` in T014) (pkg/util/vmopv1/group_power_state.go)
- [T004] Done: unit tests for T002 and T003 (pkg/util/vmopv1/vmgroup_test.go)

## Phase 3 — US1: ordering with slow guests (vmop-4046)

- [T005] Done: in `reconcileMembers`, power-off releases only the first incomplete boot-order group, gates push and stamp on `spec.powerState`, stamps `now + PowerOffDelay` (no stamp, and removes a stale one, if the delay is omitted or zero or the group is the first in power-off order), marks later members `Queued`, and closes the request only when all groups are complete and there are no member errors; power-on and suspend are unchanged (controllers/virtualmachinegroup/virtualmachinegroup_controller.go)
- [T006] Done: a child group holds its first release until the parent's stamp passes and returns `RequeueError{After}` outside `aggregateOrNoRequeue` (controllers/virtualmachinegroup/virtualmachinegroup_controller.go)
- [T007] Done: controller tests for the walk, the spec gate, a stamp that does not move, re-release after a user powers a member on, and the close of the request; the old "power off the group with delay" and "try soft" tests are replaced (controllers/virtualmachinegroup/virtualmachinegroup_controller_test.go)

## Phase 4 — US2: delay counts from completion (vmop-4047)

- [T008] Done: tests that the delay starts at completion of the previous group and that the first group in power-off order ignores its delay (controllers/virtualmachinegroup/virtualmachinegroup_controller_test.go)

## Phase 5 — US3: nested groups

- [T009] Done: descendant completion is used by the walk; controller test for a child group holding for its parent's stamp (controllers/virtualmachinegroup/virtualmachinegroup_controller.go, controllers/virtualmachinegroup/virtualmachinegroup_controller_test.go)

## Phase 6 — US4: power-on follows completion (vmop-4046)

- [T014] Done 2026-11-02: completion helpers take the target from the group spec; power-on walks forward and is complete at observed `PoweredOn`, `Suspended` is not complete (pkg/util/vmopv1/group_power_state.go, pkg/util/vmopv1/vmgroup_test.go)
- [T015] Done 2026-11-02: `reconcileMembers` runs the walk for power-on too, with `PowerOnDelay` counted from completion and no delay on the first group; the power-on request closes only when every boot-order group is observed on; suspend keeps the single pass without a stamp (controllers/virtualmachinegroup/virtualmachinegroup_controller.go)
- [T016] Done 2026-11-02: controller tests for the forward walk, the delay, a stamp that does not move, a missing member that blocks, and a child group powered on again after a descendant is powered off; the old cumulative "power on the group with delay" test is replaced (controllers/virtualmachinegroup/virtualmachinegroup_controller_test.go)
- [T017] Done 2026-11-02: `PowerOnDelay` and `BootOrder` docs, CRDs, and user docs; examples no longer put a delay on the first boot group (api/v1alpha6/virtualmachinegroup_types.go, config/crd/, docs/concepts/workloads/vm.md, docs/concepts/workloads/vm-group.md)
- [T018] Written, not yet run on a cluster: "Boot order power-on delay" asserts the first group ignores its delay and each `runtime.bootTime` gap is at least the next group's `PowerOnDelay` (test/e2e/vmservice/vmservice/virtualmachine/vm_group.go)

## Phase Final — Polish

- [T010] Done: `PowerOffDelay` and `BootOrder` docs updated, CRDs regenerated, and the user docs describe power-off ordering (api/v1alpha6/virtualmachinegroup_types.go, config/crd/, docs/concepts/workloads/vm.md, docs/concepts/workloads/vm-group.md). `docs/ref/api/*.md` is not regenerated: it is stale on main (it predates `PowerOffDelay`), and regenerating it pulls in unrelated changes.
- [T011] [vmop-4046, vmop-4047] Written, not yet run on a cluster: the "Boot order power-off delay" case asserts each delay counts from the previous group's power-off and that the first group's delay is ignored; the new "Boot order power-off ordering" context covers a user powering on a released member mid-sequence (`hard`), `soft` holding the next group until the mode is changed to `hard`, and `trySoft` powering off hard after the soft timeout before the next group is released. The blocked guest masks `poweroff.target` and `halt.target` through cloud-init; whether this stops a VMware Tools guest shutdown on the default image must be confirmed on a cluster. The boot order cases are not labeled `experimental`: they cover existing VM group behavior, not a new feature (decided 2026-11-02) (test/e2e/vmservice/vmservice/virtualmachine/vm_group.go)
- [T019] [vmop-4046, vmop-4047] Written, not yet run on a cluster: core cases that need no Telco capability (`verifyReleaseOrder` checks the order from spec and status, not from vCenter times): power-off walk with zero delays and a closed request, blocked walk (missing member, unlinked member, missing member that keeps the request open, removal from `bootOrder`), group deleted while blocked, power-on gate (power-on check annotation, suspended member, unlinked member, missing member) with `PowerOnDelay` counted from completion, suspended members across suspend, soft power-off, power-on and hard power-off, and nested groups (root, child, grandchild) in both directions, including the parent's `Queued` and `Pending` status for the child, the child's own first-group `PowerOnDelay` being ignored, and a descendant powered on while the root request is pending being powered off again. The blocked-walk test also unlinks a member from the VM side (clears `spec.groupName`) and deletes and recreates a member. The `soft` test also reorders `bootOrder` while held. The power-on gate test also appends a boot order group with no delay while the walk is held, and edits `PowerOnDelay` while the group is held and checks a member powered off directly after the request closes is not reverted. With Telco: stamp is absent for the first group and a zero delay, and does not move; a child group's delay is carried by the parent's stamp and the delay of the child's first group is ignored; a mode change, a delay edit and an added member made while a delay is pending. Extended: `soft` also covers force sync and a `powerState` flip while held. The `soft` and `trySoft` cases are labeled `extended-functional`, and the Telco skip is only on tests that set `PowerOffDelay` (test/e2e/vmservice/vmservice/virtualmachine/vm_group.go)
- [T012] Done: row 014 exists in the spec table; renamed to `vmgroup-power-ordering` on 2026-11-02 (.sdd/INDEX.md)
- [T013] Done: release and close logs, verbose logs while waiting, and the `Queued` reason on `PowerStateSynced` (controllers/virtualmachinegroup/virtualmachinegroup_controller.go)
