# Data Model: Blocking Lifecycle Hooks

- **Spec**: [`spec.md`](./spec.md)
- **Research**: [`research.md`](./research.md)

This feature is **consumer-only**: VM Operator does not own the `LifecycleStages`, `LifecycleHook`, or `LifecycleState` CRDs. It vendors read/write client types for them (mirroring `external/byok`) and adds new `VirtualMachine` status conditions plus a static `LifecycleStages` manifest declaring the stages it exposes.

## Vendored external types (`lifecycle.vcfa.vmware.com/v1alpha1`, owned by the Lifecycle Operator)

### `LifecycleStages` (cluster-scoped)

Declares which stages a target operator exposes for hooking. VM Operator **writes** one instance of this (`vmoperator-stages`, see below) via its helm chart; it does not reconcile this type.

| Field | Type | Notes |
|---|---|---|
| `spec.objects[].group` | string | `vmoperator.vmware.com` for all VM Operator entries. |
| `spec.objects[].kind` | string | `VirtualMachine`. |
| `spec.objects[].stages[].name` | string | Stage name, referenced by `LifecycleHook.spec.stage`. |
| `spec.objects[].stages[].type` | enum `Reentrant`\|`Single` | `Reentrant` pauses every reconcile cycle it's reached; `Single` pauses at most once per object lifetime. |
| `spec.objects[].stages[].blocking` | bool | Whether VM Operator waits for hooks before proceeding. |
| `spec.objects[].stages[].eventName` | string | K8s event reason emitted on stage transition. |
| `spec.objects[].stages[].conditionName` | string | Condition type set on the `VirtualMachine`. |

### `LifecycleHook` (namespaced, owned by consumers via the Lifecycle Operator)

A consumer's registration of interest in one stage for one `(group, kind)`. VM Operator only **reads** `LifecycleHook` existence indirectly, through `LifecycleState.status.hooks[]` — it does not `Get`/`List` `LifecycleHook` directly in the reconcile path (the Lifecycle Operator is responsible for fan-out into `LifecycleState`).

| Field | Type | Notes |
|---|---|---|
| `spec.target.group` / `spec.target.kind` | string | e.g. `vmoperator.vmware.com` / `VirtualMachine`. |
| `spec.stage` | string, immutable | Must match a stage name in a `LifecycleStages` resource. |

### `LifecycleState` (namespaced, owned by the target `VirtualMachine` via `ownerReferences`)

The coordination surface VM Operator reads and writes at each stage checkpoint.

| Field | Type | Written by | Notes |
|---|---|---|---|
| `spec.target.{apiVersion,kind,name,namespace,uid}` | object | VM Operator (on create) | Identifies the `VirtualMachine` this state tracks. |
| `spec.stages[].name` | string | VM Operator / Lifecycle Operator | Stage name. |
| `spec.stages[].workflowPaused` | bool | **VM Operator** | Set `true` when VM Operator reaches this stage and pauses. |
| `spec.stages[].workflowResumed` | bool | **VM Operator** | Set `true` after VM Operator observes `HooksReady=True` and is proceeding — signals the Lifecycle Operator to clear/reset the stage entry for future `Reentrant` passes. |
| `status.stages[].conditions[type=WorkflowPaused]` | condition | Lifecycle Operator | Mirrors `spec.stages[].workflowPaused`. |
| `status.stages[].conditions[type=HooksReady]` | condition | Lifecycle Operator | `True` = every registered hook for this stage has completed — **VM Operator's sole resume signal**. VM Operator treats any non-`True` value identically (waiting); it does not branch on `reason` (`HooksPending`, `HookFailed`, timed-out, etc.) — see "HooksReady handling" below. |
| `status.stages[].hooks[].lifecycleHookRef` / `.state` / `.message` | object | Lifecycle Operator | Per-hook progress (`Pending`\|`InProgress`\|`Succeeded`\|`Failed`), including timeout handling. Entirely internal to the Lifecycle Operator's bookkeeping — VM Operator does not read this field. |

**VM Operator's read/write contract per stage checkpoint**:
1. `Get` (or `Create` if absent) the `LifecycleState` for the VM.
2. If `spec.stages[stage]` is absent or `workflowPaused=false`: patch it to `workflowPaused=true`, set the VM's stage condition to the "paused" reason, emit the stage's `eventName`, exit the reconcile step without error/requeue (rely on the watch below to re-trigger).
3. If `workflowPaused=true`: check `status.stages[stage].conditions[HooksReady]`. If `True`: patch `workflowResumed=true`, set the VM's stage condition to `True`, proceed with the reconcile step. If not `True`: exit without error/requeue.
4. VM Operator's controller watches `LifecycleState` updates (mapped back to the owning `VirtualMachine`) so a `HooksReady` flip promptly re-triggers reconciliation instead of waiting for the next poll.

## New `VirtualMachine` API surface (this repo, `api/v1alphaN`)

One condition type per stage — exact names are `[NEEDS CLARIFICATION]` in `spec.md` pending review, proposed as:

| Condition type | Set during | `True` means | `False` reason |
|---|---|---|---|
| `VirtualMachineConditionLifecycleCreateReady` | before vSphere VM create | no blocking hook registered, or all hooks ready | `HooksPending` |
| `VirtualMachineConditionLifecyclePowerStateReady` | before a power-state change is applied | — | `HooksPending` |
| `VirtualMachineConditionLifecycleDeleteReady` | before vSphere VM delete | — | `HooksPending` |
| `VirtualMachineConditionLifecycleResourceDeleteReady` | before CR finalizer removal | — | `HooksPending` |

These conditions are additive to `api/v1alphaN`'s existing condition set (see `api/v1alpha4/condition_consts.go` for the current pattern) — no field removal, no version bump required for the condition types themselves.

### `HooksReady` handling (single reason, no failure-detail parsing)

VM Operator's stage condition has exactly one `False` reason (`HooksPending`) regardless of *why* `LifecycleState.status.stages[stage].conditions[HooksReady]` isn't `True` yet — a hook still running, a hook that errored, or a hook that timed out are all the Lifecycle Operator's concern, tracked in its own `HooksReady` reason/`status.stages[].hooks[]` bookkeeping, which VM Operator does not parse. This is **not** a terminal state on VM Operator's side either way — VM Operator is a level-triggered controller with no concept of a permanent failure (see `research.md` "Terminal failures"): it keeps reconciling at the normal cadence and the condition self-heals to `True` the moment `HooksReady` does, with no VM Operator-side retry limit or escalation.

### Stage `type`/`blocking` defaults (resolved)

| Stage | `type` | `blocking` | Rationale |
|---|---|---|---|
| `Create` | `Single` | `true` | Fires at most once per VM lifecycle. |
| `PowerStateChange` | `Reentrant` | `true` | Fires on every power-on **and** power-off transition, independently. |
| `Delete` | `Single` | `true` | Fires once, at vSphere-side deletion. |
| `ResourceDelete` | `Single` | `true` | Fires once, as the terminal step of the same delete flow as `Delete`. |

## Capability gating

A new Supervisor capability (name TBD, e.g. `supports_vm_service_lifecycle_hooks`) gates the entire feature, following the same mechanism `supports_telco_vm_service_api` uses today: `pkg/config/capabilities/capabilities.go` reads the `Capability` CR's `Activated` status and sets `pkgcfg.Features.LifecycleHooks` accordingly (see `research.md`'s BYOK/`BringYourOwnEncryptionKey` cross-reference — BYOK is also capability-drivable via this same code path). When the capability is disabled, `Features.LifecycleHooks` is `false` and every stage checkpoint is a pure no-op: no `LifecycleState` `Get`/`Create`, no watch, no pause — behavior is identical to the feature not existing (per `spec.md` US4).

## Static `LifecycleStages` instance

VM Operator's helm chart ships one cluster-scoped `LifecycleStages` CR (`vmoperator-stages`) declaring the four stages above for `(vmoperator.vmware.com, VirtualMachine)`. This is data, not a CRD definition — the CRD itself is installed by the Lifecycle Operator's own chart. Exact YAML lands in `plan.md`'s project structure once `type`/`blocking` are finalized per stage.

## Conversion strategy

Not applicable — no existing field is being changed or removed. The new conditions are additive and version-agnostic (conditions are not versioned per-`apiVersion` the way `spec`/`status` typed fields are).
