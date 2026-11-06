# Research: VM Group Power-On and Power-Off Ordering

## Resume log — 2026-11-02

Scope extended to power-on at the user's request. Decisions: completion is observed `status.powerState == PoweredOn` (no guest or VMware Tools readiness), and the first boot-order group's `PowerOnDelay` has no effect, the same as power-off. Source-backed points:

- vCD power-on is already gated on completion: it waits for a stage's power-on tasks, then waits the stage's `startDelay`, then starts the next stage, with no guest readiness wait (see "VMware Cloud Director vApps" below). Power-on now matches this.
- Before this change, power-on stamped every member at once with the request time plus cumulative `PowerOnDelay` values, and closed the request after one error-free pass (`reconcileMembers` in `controllers/virtualmachinegroup/virtualmachinegroup_controller.go`).
- A VM with a `poweron.check.vmoperator.vmware.com/` annotation does not power on until the annotation is removed (`api/v1alpha6/virtualmachine_types.go`, `docs/concepts/workloads/vm.md`). Such a member now blocks later boot-order groups.
- The mutation webhook opens a request by setting `last-updated-power-state-time` when `spec.powerState` changes or on force sync (`ProcessPowerState` in `webhooks/virtualmachinegroup/mutation/virtualmachinegroup_mutator.go`). The controller treats the request as pending while `status.lastUpdatedPowerStateTime` is not after that time (`shouldUpdatePowerState`). Unchanged.


## Resume log — 2026-10-21

Checked the working tree at commit `cb078b25305e0fec12a1801058de899934e36016` and retrieved WIKI page 3125019495 — Boot Order Delay Refactor. The worktree contained only the draft spec directory and its index entry; no power-off implementation changes. This update reconciles the draft with the supplied September/October discussion and records source-backed corrections. Jira ticket bodies could not be retrieved in this session, so ticket scope and epic association are carried from the discussion, not freshly verified.

### Decisions carried forward

- Advance early after observed power-off in every mode, including `hard`.
- Superseded 2026-10-31: the `trySoft` graceful-shutdown budget idea is withdrawn. `PowerOffDelay` stays a wait, counted from completion of the previous boot-order group, and `trySoft` keeps the default five-minute wait.
- `soft` never forces. Individual attempts time out and return an error, allowing retries; the group keeps later stages waiting. This was explicitly settled in the discussion and is no longer an open question.
- The deadline does not prove hard power-off has completed. The old acceptance criterion and plan allowed release at expiry and contradicted strict ordering. They now require observed completion before advancing.
- The user objected to `force-power-off-time`; no replacement was selected. Do not implement that annotation as though it were approved.
- Power-on remains outside this spec. The wiki's broader refactor is comparison material, not implicit scope expansion.

### Precise wiki summary

The page proposes replacing the upfront timestamp schedule with stages: confirm the current stage's power-state convergence, then calculate/apply the next stage's delay. Power-on confirmation means vCenter power-state/task completion, not guest or application readiness; power-off traverses the stages in reverse. It retains the delay and proposes a separate optional `MaxDelayTimeout` that permits advancing despite a member not converging. It proposes `Queued` and `TimedOut` condition reasons and condition transition timestamps as the wait clock. It identifies existing member watches and remaining problems with holding queued VMs, timed wakeups, the one-pass sync latch, missing/unlinked members, nested groups, and nil delays.

This differs from spec 013: spec 013 removes fixed power-off spacing, changes the `trySoft` graceful budget, and does not advance past a still-powered-on VM at timeout. The wiki does not define early exit from `PowerOffDelay` or replacement of the five-minute soft wait. Its proposed use of conditions to drive progression/timing must also be reconciled with `.sdd/memory/operator-best-practices.md`, which reserves conditions for reporting state. The current condition helper does update `LastTransitionTime` for Reason/Message changes, not just Status changes; the objection is architectural and lifecycle-related, not a claim that a reason change cannot update the timestamp.

### Source-backed corrections and implementation gaps

| Finding | Local source |
|---------|--------------|
| Scheduling visits every stage in a single pass, but members already observed in the desired state bypass the power-state patch. “Every VM is always stamped” is an approximation. | `controllers/virtualmachinegroup/virtualmachinegroup_controller.go`, `reconcileMembers`, `reconcileMember` |
| Both power-on and power-off use absolute cumulative timestamps, not predecessor completion. Late or slow reconciliation can allow overlap; expiry makes a VM eligible and does not guarantee its operation starts at that instant. | Same file, delay accumulation and `updateMemberPowerState`; `pkg/providers/vsphere/vmprovider_vm.go`, `reconcilePowerState` |
| `PowerOffDelay` currently delays the start of an operation. The five-minute guest wait happens afterward and is independent. A larger configured delay is not ignored or used as the force-off threshold. | Same scheduling/provider paths; `pkg/util/vsphere/vm/power_state.go`, `DefaultTrySoftTimeout`, `doAndWaitOnSoftPowerOp` |
| `trySoft` falls back on any soft-operation error, including timeout or a failed guest-shutdown request. `soft` returns the error without hard fallback. `hard` waits for its vCenter task. | `pkg/util/vsphere/vm/power_state.go`, `setPowerState`, `doAndWaitOnHardPowerOp` |
| The provider blocks in the current guest wait; a group requeue or member spec patch does not interrupt that in-flight call. | `pkg/providers/vsphere/resources/vm.go`, `SetPowerState` calls `SetAndWaitOnPowerState` |
| The apply-time annotation is deleted before the operation, even before pause/running-task checks. Reading it later is not a reliable way to recover release time. | `pkg/providers/vsphere/vmprovider_vm.go`, `reconcilePowerState` |
| `SoftTimeoutKey` is in a Go `internal` package and documented for testing; production code in the provider cannot import it directly. No exported timeout setter was found. | `pkg/util/vsphere/vm/internal/power_state_context_keys.go`; power utility call sites |
| The guest shutdown request precedes creation of the wait-timeout context. Passing an expired duration alone would still send another shutdown request, and canceling the parent context would also impair hard fallback. | `pkg/util/vsphere/vm/power_state.go`, `doAndWaitOnSoftPowerOp`, `setPowerState` |
| Member VM and child-group watches already enqueue the parent. An explicit timer is still needed for expiry without status changes. | `controllers/virtualmachinegroup/virtualmachinegroup_controller.go`, `AddToManager` |
| Mode-only group edits do not bump the power-sync trigger. Recovery by editing a group's mode needs propagation design, or an explicit force sync; it must not be assumed to work automatically after the existing latch closes. | `webhooks/virtualmachinegroup/mutation/virtualmachinegroup_mutator.go`, `ProcessPowerState`; group `shouldUpdatePowerState` |
| Nested group member entries lack an aggregate power state. Arbitrarily nested completion cannot be inferred from one child's status list without membership/freshness checks. | `api/v1alpha6/virtualmachinegroup_types.go`, `GetPowerState`; group `reconcileMember` |
| Suspended is distinct from powered off: hard/trySoft powers off a suspended VM, while soft leaves it suspended. | `pkg/providers/vsphere/vmprovider_vm.go`, `reconcilePowerState`; suspended cases in `vmprovider_vm_power_test.go` |

### Tests inspected

- `controllers/virtualmachinegroup/virtualmachinegroup_controller_test.go`, “power off the group with delay”: asserts cumulative annotations and completion of the propagation pass; does not prove predecessor shutdown completion.
- `pkg/providers/vsphere/vmprovider_vm_power_test.go`: covers future apply-time requeue, expired annotation cleanup, power modes, and suspended behavior. These remain useful baselines.
- `test/e2e/vmservice/vmservice/virtualmachine/vm_group.go`, “Boot order power-off delay”: uses hard power-off and asserts cumulative elapsed delays. Rewrite for the new semantics and capture next-stage request timing as well as previous-stage completion; ordered completion timestamps alone cannot disprove overlapping operations.
- No tests were run for this documentation-only update. Runtime behavior is unchanged.

### Remaining evidence gaps

Resolved by the 2026-10-31 redesign: budget transport (withdrawn), durable operation identity/timing (request time annotation plus the spec-gated stamp), zero versus omitted delay (both mean no pause), nested budgets (child holds until the parent's stamp), edits during a sequence (continue from the current state), suspended members (complete in `soft`, vCD precedent), missing members (block), and the first group's delay (no effect). Repeated `ShutdownGuest` behavior during shutdown and the exact retry-backoff ceiling have not been established here. Preserve the wiki's warning about timed errors passing through `aggregateOrNoRequeue` as a test requirement; verify `RequeueAfter` at the controller boundary.

## Current behavior

- `controllers/virtualmachinegroup/virtualmachinegroup_controller.go` (`reconcileMembers`) iterates boot-order groups in reverse for power-off and adds `PowerOffDelay` to a running `applyPowerStateTime`. Eligible members are stamped with `vmoperator.vmware.com.protected/apply-power-state-time` in one pass; already-converged VMs skip the power-state patch.
- `pkg/providers/vsphere/vmprovider_vm.go` (`reconcilePowerState`) requeues the VM until that time, then runs the power op.
- `pkg/util/vsphere/vm/power_state.go` (`doAndWaitOnSoftPowerOp`) calls `ShutdownGuest` and waits up to `DefaultTrySoftTimeout` (5 minutes) for `PoweredOff`, or the value in context key `internal.SoftTimeoutKey`. On timeout, `trySoft` falls back to a hard `PowerOff`; `soft` returns a `timed out waiting for power state` error that is retried with backoff.
- The VM Group controller already watches member VMs and child groups, so a member power-state change already enqueues the parent group.

## Prior art

### ESXi autostart — earlier investigation, not reverified

- The earlier discussion reported reverse serial processing, polling until powered off or the stop delay expires, early completion, and no hard fallback on timeout. Source pointers: `tera/bora/vim/hostd/hostsvc/impl/AutoStartPowerOps.cpp` (`AutoPowerOff`, `ProcessShutdownEntry`, `PerformVmGuestShutdown`) and `bora/vim/vmodl/vim/vApp/EntityConfigInfo.java`. This host autostart path alone does not establish vCenter vApp behavior.

### VMware Cloud Director vApps — verified 2026-10-28

Source: VCF Automation monorepo (`vcf/automation`), `main` at commit `0708a55915932e13bdeacb934f18f06f8c4d4df9`. This is from code reading only; nothing was run. Files are given as `<module>: <class>` under `backend/cloud/cell/`. This snapshot is not a classic vCD release, and older releases may differ. The earlier pointers to `core-build-cloud` are replaced by the ones below.

- **There is no desired vApp power state.** vApp status is computed from the VM statuses. If the VMs differ, the vApp shows `MIXED`. Power operations are one-time tasks. (`backend-core: VAppManagerImpl`)
- **Individual VM power operations are allowed and are not reverted.** A VM-level operation locks only that VM. Power changes made in vCenter are accepted: vCD deploys or undeploys the VM to match. (`backend-core: VAppServiceImpl.getBusyEntitiesForVAppPowerOperation`, `ReactToVmPowerStateChangeInVcActivity`)
- **Power-on is gated on completion.** This applies when startup settings are honored (global `settings.honorStartupSettings`, seed value `1`).
  - Stages run in ascending `order`, and the VMs in a stage power on in parallel.
  - vCD waits for the stage's power-on tasks, then waits the stage's largest `startDelay`, then starts the next stage.
  - A failed power-on is retried once, then the vApp subset is rolled back.
  - There is no wait for guest or VMware Tools readiness.
  - (`backend-core: ParallelPowerOnActivity`, `VAppVmUtils`)
- **Power-on details, checked 2026-11-02** at commit `56daef6701fdf87336013351aac77accf73590bd`:
  - A stage is the set of VMs with the same boot `order`. Its delay is the largest `startDelay` of its VMs. The delay is waited after the stage completes, before the next stage starts, and also after the last stage. There is no wait before the first stage. (`VAppVmUtils.computeInputs`, `ParallelPowerOnActivity.AfterGroupPowerOnPhase`)
  - A stage is complete when the `PowerOnMultiVM` task completes and the inventory shows every VM as `VMOn`, within the inventory update timeout. (`vim-proxy: PowerOnVmActivity.WaitForCompletionPhase`, `FinalPhase`)
  - Only VMs that are `VMOff` or `VMSuspended` are powered on. VMs that are already on, VMs whose boot action is not power-on, and replication-mode VMs are skipped. (`ParallelPowerOnActivity.InitialPhase`, `filterEligibleVMsForPowerOn`)
  - **A failed stage stops the sequence.** A timeout fails the operation at once. Any other failure is retried once per VM that is still powered off, with relocation on insufficient-resource faults. If the retry fails, vCD undeploys the stage's VMs that are not deployed and fails the operation. Later stages are not powered on. Earlier stages stay on. A VM that is missing from the database, or a CPU or memory limit violation, also fails the operation. (`ParallelPowerOnActivity.RetryPowerOnVmsPhase`, `AfterRetryPhase`, `RollbackPhase`, `DeployVAppVmSubsetActivity.MarkVAppAsDeployedPhase`)
  - Compared with VM Operator: both stop later stages after a stage that does not power on. vCD fails the task, and the user must start it again. VM Operator keeps the request open and continues when the member powers on, for example after a `poweron.check` annotation is removed. vCD has no equivalent of the `poweron.check` annotation. The vCD stage delay after stage N is the same as VM Operator's `PowerOnDelay` on stage N+1.
- **Direct vApp power-off and shutdown are disabled in this snapshot.** `VAppServiceImpl.powerOff` and `shutdown` throw "Operation is permanently disabled". `SerialGroupOperationActivity` (serial, reverse order) is no longer on a live power-off path. Power-off goes through undeploy.
- **Ordered undeploy is gated on completion.** Ordering applies only with `undeployPowerAction=default`. If the value is not set, it means `powerOff`, with no order and no delay.
  - Stages run highest `order` first, and the VMs in a stage stop in parallel.
  - vCD waits for every VM in the stage to finish before it starts the next stage.
  - Each VM uses its own `stopAction`.
  - (`backend-core: UndeployVirtualMachinesActivity`, `UndeployVirtualMachineActivity`, `VAppDeploymentManagerImpl`)
- **Guest shutdown waits for an observed Off state, up to `VimTimeouts.ShutdownVm`** (seed value 900000 ms, hidden setting). The wait ends early when the VM is off. On timeout, the VM fails with `GUEST_SHUTDOWN_FAILED`. There is no hard power-off fallback. If VMware Tools is not available, the VM fails. (`vim-proxy: ShutdownGuestVmActvity`)
- **`stopDelay` is a fixed wait after the VM's stop action succeeds.** It is not a shutdown budget. (`fabric-foundation: PowerOperationActivity`)
- **Suspended VMs are skipped during ordered undeploy.** In `performUndeploy`, `VMOff` and `VMSuspended` both log "Skipping stop" and create no stop operation, so the stage does not wait for them. This applies to every `stopAction`. vCD does not power off a suspended VM even when the stop action is power-off. Checked 2026-10-31 at a newer commit, `56daef6701fdf87336013351aac77accf73590bd`. (`backend-core: UndeployVirtualMachineActivity.performUndeploy`)
- **Undeploy failures do not block later stages.** They are collected and reported at the end. This applies to undeploy only; a power-on failure stops later stages (see above). A VM that is already off, already undeployed, or busy is skipped and counted as done.
- **The activity framework is in the same monorepo** (`backend/cloud/lib/activity-framework`). Its `waitFor` overload with futures and a timeout moves on when the futures complete. On timeout it raises an event, and `ShutdownGuestVmActvity` handles that event by canceling the wait. Not checked: whether the cell build uses this in-tree copy.

## Conclusions

- Early advance and mode-respecting fallback are the selected VM Operator behavior, independently of product-parity claims.
- The earlier ESXi finding supports early advance as precedent. It does not establish strict completion ordering after timeout.
- vCD parity is not a justification for force-off. vCD fails a timed-out guest shutdown and has no hard fallback.
- vCD is precedent for gating each stage on the completion of the previous stage, for both power-on and power-off. Its delays are fixed waits after completion, not budgets. For undeploy, it does not let a failed member block later stages. For power-on, a failed stage stops later stages and fails the operation.
- Kubernetes StatefulSet `OrderedReady` is precedent for a stateless walk: the controller derives the next step from the spec and observed state, stores no plan, and holds later steps behind an unhealthy earlier one (checked in the StatefulSet docs, "Deployment and Scaling Guarantees", 2026-10-28). It has the same limits: a stuck member blocks later steps, and a step already started is not taken back.
- vCD lets individual members diverge from the vApp and does not revert them. The current VMGroup behavior (`NotSynced`, no revert after the request is propagated) is consistent with this.
