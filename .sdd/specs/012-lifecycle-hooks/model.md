# Data Model: Blocking Lifecycle Hooks

- **Spec**: [`spec.md`](./spec.md)
- **Research**: [`research.md`](./research.md)

This feature is **consumer-only**: the `lifecycle.vcfa.vmware.com` CRDs are owned by the Lifecycle Operator. VM Operator vendors client types for them (mirroring `external/byok`), authors the `LifecycleStages` instance for `VirtualMachine`, creates each VM's `LifecycleState` when absent and finalizes it, and adds one `VirtualMachine` condition.

## Vendored external types (`lifecycle.vcfa.vmware.com/v1alpha1`)

### `LifecycleStages` (cluster-scoped)

One instance for the `VirtualMachine` CRD, `vmoperator-stages`. VM Operator authors it as `config/lifecycle/vmoperator-stages.yaml`; it ships with and is applied by the Lifecycle Operator's package, not by VM Operator.

| Field | Value |
|---|---|
| `spec.target.group` / `spec.target.kind` | `vmoperator.vmware.com` / `VirtualMachine`. |
| `spec.stages[].name` | `Create`, `PowerStateChange`, `Delete`, `ResourceDelete`. |
| `spec.stages[].type` / `.blocking` | `Reentrant` / `true` for all four; see "Stage `type`/`blocking` defaults". |
| `spec.stages[].eventName` | The `VirtualMachine` Event emitted after the stage's action: `CreateSuccess`, `UpdateSuccess`, `DeleteSuccess`; unset for `ResourceDelete`. |
| `spec.stages[].conditionName` | `LifecycleHookReady` for all four. |
| `spec.stages[].description` | Human-readable summary of what the stage pauses before; informational. |

### `LifecycleHook` (namespaced)

A consumer's registration to pause a `VirtualMachine` stage. VM Operator never reads it; it sees hooks only through `LifecycleSubscribedStages` and `LifecycleState`.

### `LifecycleSubscribedStages` (namespaced)

One per namespace for the `VirtualMachine` CRD, listing in `spec.stages[]` the `VirtualMachine` stages that have at least one hook there. VM Operator gets it by name (`lss-1b52874a51e98527`, from the vendored `SubscribedStagesName` helper) only when a checkpoint finds no `LifecycleState`, to create one. `NotFound`, or a stage missing from `spec.stages[]`, means no hook for that stage (spec G5).

### `LifecycleState` (namespaced, one per VM)

The coordination surface VM Operator reads and writes at each stage checkpoint.

| Field | Written by | VM Operator's use |
|---|---|---|
| `metadata.name` | creator | `virtualmachine-<VM UID>`, from the vendored `LifecycleStateName` helper; VM Operator gets it by name. The UID is the `VirtualMachine`'s `metadata.uid`. |
| `metadata.ownerReferences` | creator | The owning `VirtualMachine`, `controller: true`. Deletes the State with the VM, and maps the State watch back to the VM. |
| `metadata.labels[target-group]`, `[target-kind]` | creator | With the owner reference, the binding VM Operator verifies after every lookup; a mismatch is retried, never treated as "no hook". |
| `metadata.finalizers` | **VM Operator** | Added by get-or-create, including to adopted `LifecycleState`s; removed once `ResourceDelete` resolves. See "Namespace-deletion protection". |
| `spec.stages[].name` | creator; Lifecycle Operator for hooks added later | Entry present = the stage has a hook; missing = proceed. |
| `spec.stages[].workflowPaused` / `.workflowResumed` | **VM Operator** | Both `false`: never held. **Pause** sets `workflowPaused=true`, `workflowResumed=false`: the stage is held. **Release** sets `workflowPaused=false`, `workflowResumed=true`: the hold is over and the action may run. No other transition is admitted. |
| `status.stages[]` | Lifecycle Operator | Created on pause, removed after release. Its presence before a pause means the previous hold has not been cleared. |
| `status.stages[].conditions[HooksReady]` | Lifecycle Operator | `True` is the resume signal. Status and reason drive `LifecycleHookReady`. |

## Stage checkpoint

`GetOrCreateLifecycleState` and `ReconcileStage`.

1. **Get or create.**
   - Get the `LifecycleState` by name and verify the binding.
   - If it is absent and `LifecycleSubscribedStages` lists stages, create it (owner reference, finalizer) with every listed stage as never held (both switches `false`). If none are listed, create nothing.
   - On `AlreadyExists`, another creator won; re-read.
   - Add VM Operator's finalizer if it is missing (adoption). 
2. **Decide.** Check the rows top to bottom; the first row that matches decides what VM Operator does.

| What's happening | `LifecycleState` shows | VM Operator                                           |
|---|---|-------------------------------------------------------|
| No hook for this stage | No `LifecycleState`, or no `spec.stages[]` entry for the stage | Proceed.                                              |
| Previous hold still being cleared | `workflowPaused=false`, the stage's `status.stages[]` entry still present | Wait. Pausing now would reuse the previous hold's results. |
| Ready to hold | `workflowPaused=false`, no `status.stages[]` entry | Pause the stage, then wait.                           |
| Hooks running | `workflowPaused=true`, `HooksReady` absent or not `True` | Wait.                                                 |
| Hooks finished | `workflowPaused=true`, `HooksReady=True` | Release the stage, proceed.                           |

   Definition of a hold:  pause → hooks run → `HooksReady=True` → release → the Lifecycle Operator clears the status entry → the next reach can pause again.

   **Abandon**: when the caller will no longer run the stage's action, VM Operator releases the paused stage without running the action, ignoring whatever `HooksReady` says. This ends the hold, so hook owners stop waiting and abort in-flight work. For example: `spec.powerState` is reverted while a power change is held, the VM is deleted while `Create` or a power change is held, or the vSphere VM is already gone while `Delete` is held.

3. **Watch.** The `LifecycleState` watch re-triggers the VM on status changes and when the Lifecycle Operator creates a State.
4. **Finish.** Once `ResourceDelete` resolves, VM Operator removes its finalizer. It never deletes the `LifecycleState`; GC or the namespace controller does.

Notes:

- **Stale copies.** Pause and release are optimistic-locked writes, so a stale copy can delay a stage but never let it run early. Only "absent / no entry" proceeds without a write; a stale "no entry" for a hook registered after the read is an accepted race.
- **Wakeups.** Waits rely on the `LifecycleState` status watch, with no timed requeue; the VM's periodic resync is the backstop.
- **Lifecycle Operator unavailable.** A stage with an entry stays blocked until it returns; stages without an entry are unaffected. Turning the capability off unblocks every stage.

### Releasing at `ResourceDelete`

VM Operator removes its `LifecycleState` finalizer in the same write that releases `ResourceDelete`. A separate patch would conflict with the Lifecycle Operator's status cleanup, which follows the release immediately, and block the VM's own finalizer removal.

### Namespace-deletion protection

Namespace deletion removes every lifecycle object directly, bypassing owner references. VM Operator's finalizer keeps the `LifecycleState` until `ResourceDelete` resolves, so `Delete` and `ResourceDelete` hooks are still honored (spec SC-007); `Create` and `PowerStateChange` don't need this. Creating the State with every listed stage declares `Delete`/`ResourceDelete` before any deletion. Accepted gaps: a hook registered as the namespace starts terminating; a Lifecycle-Operator-created `LifecycleState` deleted before VM Operator adopts it; and a namespace deleted before either operator creates the `LifecycleState`, where get-or-create's create is rejected and VM Operator proceeds.

## New `VirtualMachine` API surface (this repo, `api/v1alpha6`)

One new condition type, `VirtualMachineConditionLifecycleHookReady` (`"LifecycleHookReady"`), reflecting the last evaluated stage that has hooks. Additive to `api/v1alpha6` (see `api/v1alpha6/condition_consts.go`); no version bump or conversion.

| Field | Value |
|---|---|
| `reason` | The stage: `CreateStage`, `PowerStateChangeStage`, `DeleteStage`, `ResourceDeleteStage`. |
| `status` | `True` when the stage's hooks finished without failure, or the hold was abandoned. `False` while blocked, or when a hook failed. `Unknown` when VM Operator could not evaluate the stage's hooks. |
| `message` | The stage's `HooksReady` message, prefixed by VM Operator (below). |
| `lastTransitionTime` | Set by `conditions.Set`: updated when status, reason or message changes. |

| Observation | Action | `status` | `message` |
|---|---|---|---|
| No `LifecycleState`, or no entry for the stage | proceeds | not written; an earlier stage's condition is left unchanged | — |
| Paused, `HooksReady` absent; or previous hold still being cleared | held | `False` | `Waiting for <Stage> hooks` |
| `HooksReady=False/HooksPending` | held | `False` | `Waiting for <Stage> hooks: <msg>` |
| `HooksReady=False/HookFailed` | held | `False` | `<Stage> hook failed, waiting for remaining hooks: <msg>` |
| `HooksReady=True/AllHooksSucceeded` or `NoHooksRegistered` | released | `True` | `<msg>` |
| `HooksReady=True/HookFailed` | released, action runs | `False` | `<Stage> hook failed, <Stage> proceeded: <msg>` |
| Abandon | released | `True` | — |
| The stage's hooks cannot be evaluated (e.g. reading the `LifecycleState` fails) | held, retried | `Unknown` | `Could not evaluate <Stage> hooks: <error>` |

The prefix comes from `HooksReady`'s status and reason. `False` does not by itself mean blocked; the message says which. The condition is absent until a stage with hooks is reached, and never written for a VM in a namespace with no hooks; consumers treat absent as `True`. At most one stage is blocked at a time (`Delete` resolves before `ResourceDelete`; `Create`, `PowerStateChange` and `Delete` are mutually exclusive by VM phase).

### Hooks-not-ready handling

VM Operator reads only `HooksReady`, never per-hook results. A wait is not terminal: VM Operator has no timeout, retry limit or escalation of its own. It re-checks when the `LifecycleState` changes, and the condition follows `HooksReady` (see `research.md` "Terminal failures").

### Stage `type`/`blocking` defaults

| Stage | `type` | `blocking` | Rationale |
|---|---|---|---|
| `Create` | `Reentrant` | `true` | Before the vSphere create. |
| `PowerStateChange` | `Reentrant` | `true` | Before every power operation VM Operator applies (power on/off, suspend, resume, restart). |
| `Delete` | `Reentrant` | `true` | Before the vSphere delete. |
| `ResourceDelete` | `Reentrant` | `true` | Before the VM finalizer is removed. |

VM Operator never branches on the stage: every reach of a checkpoint after a release is a new hold, including a retry of a failed action, so hooks must be idempotent.

## Capability gating

`supports_vm_service_lifecycle_hooks` sets `pkgcfg.Features.LifecycleHooks`. It depends on a capability the Lifecycle Framework enables when its package is installed, which also installs the `lifecycle.vcfa.vmware.com` CRDs. So `Features.LifecycleHooks` set means the CRDs are present, and VM Operator never installs them. When disabled, every checkpoint is a no-op: no `LifecycleState` access, no watch, no pause (spec G6, SC-005). Exception: deleting a VM still removes VM Operator's finalizer from its `LifecycleState`, so disabling the capability never blocks VM or namespace deletion.

## Examples

A namespace with a `Create` hook and a `Delete` hook on `VirtualMachine`. The Lifecycle Operator's CRDs are the source of truth if these diverge.

### `LifecycleStages`

```yaml
apiVersion: lifecycle.vcfa.vmware.com/v1alpha1
kind: LifecycleStages
metadata:
  name: vmoperator-stages
spec:
  target:
    group: vmoperator.vmware.com
    kind: VirtualMachine
  stages:
  - name: Create
    description: Before the VM is created in vSphere.
    type: Reentrant
    blocking: true
    eventName: CreateSuccess
    conditionName: LifecycleHookReady
  - name: PowerStateChange
    description: Before a power operation (on, off, suspend, restart) is applied.
    type: Reentrant
    blocking: true
    eventName: UpdateSuccess
    conditionName: LifecycleHookReady
  - name: Delete
    description: Before the VM is deleted from vSphere.
    type: Reentrant
    blocking: true
    eventName: DeleteSuccess
    conditionName: LifecycleHookReady
  - name: ResourceDelete
    description: Before the VM's Kubernetes finalizer is removed.
    type: Reentrant
    blocking: true
    conditionName: LifecycleHookReady
```

### `LifecycleHook`

```yaml
apiVersion: lifecycle.vcfa.vmware.com/v1alpha1
kind: LifecycleHook
metadata:
  name: domain-join-hook
  namespace: workload-ns
spec:
  target:
    group: vmoperator.vmware.com
    kind: VirtualMachine
  stage: Create
---
apiVersion: lifecycle.vcfa.vmware.com/v1alpha1
kind: LifecycleHook
metadata:
  name: cmdb-cleanup-hook
  namespace: workload-ns
spec:
  target:
    group: vmoperator.vmware.com
    kind: VirtualMachine
  stage: Delete
```

### `LifecycleSubscribedStages`

The `VirtualMachine` instance for `workload-ns`, from the two hooks above:

```yaml
apiVersion: lifecycle.vcfa.vmware.com/v1alpha1
kind: LifecycleSubscribedStages
metadata:
  name: lss-1b52874a51e98527
  namespace: workload-ns
spec:
  stages:
  - Create
  - Delete
```

### `LifecycleState`

**Created by VM Operator:**

```yaml
apiVersion: lifecycle.vcfa.vmware.com/v1alpha1
kind: LifecycleState
metadata:
  name: virtualmachine-3f1c2b9e-0000-0000-0000-000000000001
  namespace: workload-ns
  labels:
    lifecycle.vcfa.vmware.com/target-group: vmoperator.vmware.com
    lifecycle.vcfa.vmware.com/target-kind: VirtualMachine
  finalizers:
  - lifecycle.vcfa.vmware.com/vm-operator-state
  ownerReferences:
  - apiVersion: vmoperator.vmware.com/v1alpha6
    kind: VirtualMachine
    name: windows-abc123
    uid: 3f1c2b9e-0000-0000-0000-000000000001
    controller: true
    blockOwnerDeletion: false
spec:
  stages:
  - name: Create
    workflowPaused: false
    workflowResumed: false
  - name: Delete
    workflowPaused: false
    workflowResumed: false
```

**`Create` blocked.** VM Operator paused `Create`; the hook is still running. `LifecycleHookReady` is `status=False`, `reason=CreateStage`, `message: "Waiting for Create hooks: 0 of 1 hooks completed"`.

```yaml
spec:
  stages:
  - name: Create
    workflowPaused: true
    workflowResumed: false
  - name: Delete
    workflowPaused: false
    workflowResumed: false
status:
  stages:
  - name: Create
    conditions:
    - type: HooksReady
      status: "False"
      reason: HooksPending
      message: 0 of 1 hooks completed
```

**`Create` ready to resume, with a failed hook.** VM Operator releases the stage, sets `LifecycleHookReady` to `status=False`, `reason=CreateStage`, `message: "Create hook failed, Create proceeded: domain-join-hook: timed out connecting to domain controller"`, then creates the VM.

```yaml
status:
  stages:
  - name: Create
    conditions:
    - type: HooksReady
      status: "True"
      reason: HookFailed
      message: >-
        domain-join-hook: timed out connecting to domain controller
```
