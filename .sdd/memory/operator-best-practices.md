# Kubernetes Operator Best Practices

> **Aliases used in the examples below** (`pkgcfg`, `pkgctx`, `pkglog`, `pkgerr`, `vmopv1`, `byokv1`, `ctrl`, `ctrlclient`, etc.) come from the `importas` table in [`.golangci.yml`](../../.golangci.yml) `linters.settings.importas.alias`, which is the source of truth. The depguard rules in the same file (under `linters.settings.depguard`) also forbid importing certain packages (`io/ioutil`, `github.com/pkg/errors`, `k8s.io/utils`, and ginkgo / gomega / `testing` outside test files) — keep that in mind when copying these patterns into a new package.

## Reconcile Loop Structure

Every reconciler in VM Operator follows this canonical pattern:

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
    // 1. Join long-lived controller-manager context
    ctx = pkgcfg.JoinContext(ctx, r.Context)

    // 2. Fetch the resource (NotFound = deleted, stop reconciling)
    obj := &vmopv1.MyResource{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 3. Build typed context
    myCtx := &pkgctx.MyResourceContext{
        Context: ctx,
        Logger:  pkglog.FromContextOrDefault(ctx),
        Obj:     obj,
    }

    // 4. Create patch helper (deferred patch ensures status is always persisted)
    patchHelper, err := patch.NewHelper(obj, r.Client)
    if err != nil {
        return ctrl.Result{}, fmt.Errorf("failed to init patch helper for %s: %w", myCtx, err)
    }
    defer func() {
        if err := patchHelper.Patch(ctx, obj); err != nil {
            if reterr == nil {
                reterr = err
            }
            myCtx.Logger.Error(err, "patch failed")
        }
    }()

    // 5. Handle deletion vs normal reconciliation
    if !obj.ObjectMeta.DeletionTimestamp.IsZero() {
        return ctrl.Result{}, r.ReconcileDelete(myCtx)
    }
    return ctrl.Result{}, r.ReconcileNormal(myCtx)
}
```

Key patterns:
- Named return `reterr error` with deferred patch aggregation
- `pkgcfg.JoinContext` to propagate feature flags from the controller-manager context
- `pkglog.FromContextOrDefault(ctx)` for structured logging
- Typed context from `pkg/context/`

## Level-Triggered Reconciliation (Idempotent)

From the controller-runtime FAQ:

> **Q: How do I have different logic for create vs update vs delete?**
> **A: You should not.** Reconcile functions should be idempotent — read all state, then write updates.

```go
// Good - checks actual state, converges toward desired
func (r *Reconciler) ReconcileNormal(ctx *pkgctx.MyResourceContext) error {
    // Read current state from the cluster/vSphere
    // Compare actual vs desired
    // Make changes to converge
}

// Bad - event-driven / edge-triggered
func (r *Reconciler) Reconcile(...) {
    if isCreateEvent { /* ... */ }
    if isUpdateEvent { /* ... */ }
}
```

Why this matters:
- Controller restarts don't break reconciliation
- Manual changes are automatically corrected
- Handles skipped/coalesced events correctly

## Finalizer Management

Finalizers are added early in reconciliation and removed only after cleanup:

```go
func (r *Reconciler) ReconcileNormal(ctx *pkgctx.MyResourceContext) error {
    // Add finalizer before any work (also handle deprecated finalizer migration)
    if !controllerutil.ContainsFinalizer(ctx.Obj, finalizerName) {
        controllerutil.RemoveFinalizer(ctx.Obj, deprecatedFinalizerName)
        controllerutil.AddFinalizer(ctx.Obj, finalizerName)
        return nil  // Return to patch immediately
    }
    // ... normal reconciliation
}

func (r *Reconciler) ReconcileDelete(ctx *pkgctx.MyResourceContext) error {
    // Perform cleanup
    if err := r.cleanup(ctx); err != nil {
        return err
    }
    // Remove finalizer only after successful cleanup
    controllerutil.RemoveFinalizer(ctx.Obj, finalizerName)
    controllerutil.RemoveFinalizer(ctx.Obj, deprecatedFinalizerName)
    return nil
}
```

Finalizer naming convention: `vmoperator.vmware.com/<resource>`

## Controller Setup (AddToManager)

Each controller package exposes `AddToManager`:

```go
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
    var (
        controlledType     = &vmopv1.MyResource{}
        controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()
    )

    r := NewReconciler(ctx, mgr.GetClient(), ctrl.Log.WithName("controllers").WithName(controlledTypeName), ...)

    return ctrl.NewControllerManagedBy(mgr).
        For(controlledType).
        Watches(&vmopv1.RelatedResource{},
            handler.EnqueueRequestsFromMapFunc(relatedToMyResourceMapper(r.Client))).
        WithLogConstructor(pkglog.ControllerLogConstructor(controlledTypeName, controlledType, mgr.GetScheme())).
        Complete(r)
}
```

### Watches

Feature-gated watches use `pkgcfg.FromContext(ctx)`:

```go
if pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey {
    builder = builder.Watches(
        &byokv1.EncryptionClass{},
        handler.EnqueueRequestsFromMapFunc(encryptionClassMapper(ctx, r.Client)),
    )
}
```

### Mapper Functions

Return `[]reconcile.Request` mapping related resources back to the primary resource. Always use a field indexer with `client.MatchingFields` so the List call is filtered server-side rather than loading every object in the namespace:

```go
// In AddToManager: register a field index so List can filter efficiently.
const myResourceByRelatedField = "spec.relatedResourceName"

if err := mgr.GetFieldIndexer().IndexField(
    ctx,
    &vmopv1.MyResource{},
    myResourceByRelatedField,
    func(rawObj client.Object) []string {
        obj := rawObj.(*vmopv1.MyResource)
        return []string{obj.Spec.RelatedResourceName}
    }); err != nil {
    return err
}

// Mapper used by builder.Watches(...):
func relatedToMyResourceMapper(c client.Client) func(context.Context, client.Object) []reconcile.Request {
    return func(ctx context.Context, o client.Object) []reconcile.Request {
        var list vmopv1.MyResourceList
        if err := c.List(ctx, &list,
            client.InNamespace(o.GetNamespace()),
            client.MatchingFields{myResourceByRelatedField: o.GetName()},
        ); err != nil {
            return nil
        }
        requests := make([]reconcile.Request, len(list.Items))
        for i := range list.Items {
            requests[i] = reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])}
        }
        return requests
    }
}
```

## Fan-out to Child Objects (patch vs. CreateOrUpdate)

`controllerutil.CreateOrPatch` uses a JSON merge patch: map fields merge
key-by-key, but **list fields are replaced wholesale** and there is **no
`resourceVersion` precondition**. Fine for disjoint fields or a shared map
keyed per writer; unsafe for a shared list.

When multiple parents fan-in write to the same *list*-typed field (owner
references, or a status list each parent upserts an entry into), a plain merge
patch can silently drop a concurrent writer's entry with no error to trigger a
retry. Patch with an optimistic lock, and skip the write when nothing changed:

```go
base := obj.DeepCopy()
// mutate obj toward desired ...
if !apiequality.Semantic.DeepEqual(base.Spec, obj.Spec) ||
    !apiequality.Semantic.DeepEqual(base.OwnerReferences, obj.OwnerReferences) {
    // MergeFromWithOptimisticLock sends base's resourceVersion, so a racing
    // writer's patch fails with a conflict instead of overwriting. Reconcile
    // is idempotent, so just return the error and let the requeue retry.
    if err := c.Patch(ctx, obj, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
        return err
    }
}
```

- **Compare with `apiequality.Semantic.DeepEqual`** (`k8s.io/apimachinery/pkg/api/equality`),
  not `reflect.DeepEqual` — plain reflection false-diffs on types like
  `resource.Quantity` that carry an unexported lazy-formatted-string cache, so a
  freshly-built value and its round-tripped-through-the-API-server equivalent
  compare unequal even when identical. That silently defeats the skip-if-unchanged
  guard below.
- **Skip unchanged writes** so an idle reconcile does not bump
  `resourceVersion` and re-trigger watchers. (`CreateOrUpdate`'s `Update` gets
  the same conflict guarantee but writes the whole object; prefer the patch.)
- **Status is a subresource:** persist it separately via `c.Status().Patch(...)`
  (same optimistic lock, same skip guard) after the object exists.
- **In tests**, register the type with `WithStatusSubresource` (all VM Operator
  types already are — `test/builder/fake.go` `KnownObjectTypes`) so the fake
  client enforces the spec/status split and catches a missing status write.

## VM Update Reconcile Order

The `updateVirtualMachine` method in the vSphere provider follows a strict, intentional ordering. Each step depends on the state established by prior steps. Do NOT reorder these without understanding the dependencies:

```
 1. Fetch properties        — get the VM's current state from vCenter
 2. Fetch recent tasks      — check for in-flight vSphere tasks
 3. Fetch attached tags      — get tags for policy evaluation
 4. Fetch volume info        — get disk/volume metadata
 5. Reconcile status         — sync k8s status from vCenter state (MUST be before schema upgrade)
 6. Reconcile schema upgrade — backfill/upgrade spec fields using status data from step 5
 7. Reconcile backup state   — backup ExtraConfig/annotations
 8. Reconcile snapshot revert — revert BEFORE config changes (restores VM to snapshot state first)
 9. Reconcile config         — apply desired config (hardware, crypto, boot options, etc.)
10. Reconcile power state    — power on/off after config is applied
11. Reconcile snapshot create — snapshot AFTER power state (captures final desired state)
```

Key ordering rationale:
- **Status before schema upgrade**: Schema upgrade pushes observed data from status back into spec (e.g., backfilling new fields). Status must be fresh.
- **Snapshot revert before config**: Revert restores the VM to a prior snapshot, then config changes are applied on top. Reversing this would overwrite config changes.
- **Config before power state**: Hardware changes often require the VM to be powered off. Config applies reconfigure; power state then brings it up.
- **Snapshot create last**: Captures the final converged state after all changes.

Each step uses `getReconcileErr` to accumulate errors while continuing through subsequent steps where possible. A `NoRequeueError` from any step short-circuits the remaining steps via `errOrReconcileErr`.

## VC Operation IDs (WithVCOpID)

Any code path that calls into vCenter — govmomi, the property collector, or a
package-level helper in `pkg/providers/vsphere/*.go` that takes a
`*vim25.Client` directly — MUST wrap the context with `pkgctx.WithVCOpID`
before making that call, so the operation can be correlated with VC-side
logs.

**Set it inside the provider method, not in the calling controller.** When a
controller reaches vCenter through the `VMProvider` interface (the normal
case), the wrapping belongs at the top of the concrete
`pkg/providers/vsphere/*.go` method that actually talks to VC — the
controller just calls the interface method with a plain `ctx`:

```go
// pkg/providers/vsphere/vmprovider.go
func (vs *vSphereVMProvider) QueryConfigOptionEx(
    ctx context.Context,
    clusterMoID string,
    hardwareVersion string) (*vimtypes.VirtualMachineConfigOption, error) {

    ctx = pkgctx.WithVCOpID(ctx, nil, "queryConfigOptionEx")

    client, err := vs.getVcClient(ctx)
    ...
}
```

- The object parameter is nil-safe — pass the Kubernetes object being
  reconciled when the provider method has one in scope (e.g.
  `pkgctx.WithVCOpID(ctx, vm, "properties")`), or `nil` when the method only
  takes primitive args (e.g. a `clusterMoID` string) and has no object to
  fold into the op ID.
- `operation` is a short, camelCase label naming the specific VC call/step
  (e.g. `"properties"`, `"createOrUpdateVM"`, `"queryConfigOptionEx"`), not a
  generic term like `"reconcile"`. The resulting string is
  `vmoperator-<objName>-<operation>-<reconcileID>`.
- Usually one op ID per provider method is enough: set it once near the top of
  the `pkg/providers/vsphere/*.go` method that reaches vCenter rather than
  labeling every individual govmomi/property-collector call it makes. This is a
  default to steer toward when nothing else dictates otherwise, not a hard rule
  — if a method does genuinely distinct operations worth telling apart in
  VC-side logs, giving them their own op IDs is fine.

## Requeue and Error Semantics (pkgerr)

VM Operator uses three error types from `pkg/errors` to control requeue behavior from deep in the call stack, without the caller needing to translate errors into `ctrl.Result`:

### RequeueError — "Requeue, but not as an error"

```go
// Returned from anywhere in the reconcile stack.
// ResultFromError translates it to ctrl.Result{Requeue: true} or {RequeueAfter: d}.
// The controller-runtime error rate limiter is NOT triggered.
pkgerr.RequeueError{After: 30 * time.Second}
```

Use when: an operation succeeded but the VM needs another reconcile after a delay (e.g., waiting for a task to complete).

### NoRequeueError — "Stop reconciling, log as terminal error"

```go
// Returned from anywhere. ResultFromError translates it to
// reconcile.TerminalError(err) — logged and counted as error, but NO retry.
pkgerr.NoRequeueError{Message: "encryption class not found"}
```

Use when: a permanent/unrecoverable error occurs and retrying will not help (e.g., invalid configuration that requires user intervention).

### NoRequeueNoErr — "Stop reconciling, not an error"

```go
// A NoRequeueError with DoNotErr=true.
// ResultFromError translates it to ctrl.Result{} with nil error — silent stop.
pkgerr.NoRequeueNoErr("created vm")
pkgerr.NoRequeueNoErr("reverted snapshot")
pkgerr.NoRequeueNoErr("is paused")
pkgerr.NoRequeueNoErr("has outstanding task")
```

Use when: reconciliation should stop, but this is a normal/expected condition, not an error. Common examples:
- VM was just created (wait for the next watch event)
- VM has an outstanding vSphere task (wait for task completion signal)
- VM is paused (nothing to do)
- A snapshot revert was just initiated (wait for completion)

### How They Flow

The `Reconcile` method uses `pkgerr.ResultFromError` to translate these errors:

```go
// In the controller's Reconcile method:
if !vm.DeletionTimestamp.IsZero() {
    return pkgerr.ResultFromError(r.ReconcileDelete(vmCtx))
}
err = r.ReconcileNormal(vmCtx)
if err != nil && !ignoredCreateErr(err) {
    return pkgerr.ResultFromError(err)
}
```

`ResultFromError` priority: `RequeueError` > `NoRequeueError` > regular error. These error types can be returned from ANY depth in the call stack (provider, session, vmconfig reconcilers) and will propagate up correctly because they use `errors.As`.

### Sentinel Errors

The provider defines sentinel errors for well-known outcomes:

```go
ErrCreate          = pkgerr.NoRequeueNoErr("created vm")
ErrUpdate          = pkgerr.NoRequeueNoErr("updated vm")
ErrRestart         = pkgerr.NoRequeueNoErr("restarted vm")
ErrIsPaused        = pkgerr.NoRequeueNoErr("is paused")
ErrHasTask         = pkgerr.NoRequeueNoErr("has outstanding task")
ErrSnapshotRevert  = pkgerr.NoRequeueNoErr("reverted snapshot")
ErrReconfigure     = session.ErrReconfigure
```

## Channel Source (cource)

The `pkg/util/kube/cource` package provides a context-based channel source mechanism for triggering reconciliation from outside the standard watch/informer pipeline. This is used for async signals — e.g., when a background service (vm-watcher) detects a vCenter change and needs to enqueue a reconcile.

### How It Works

1. **Context setup** — The controller-manager context is initialized with a channels map:

```go
ctx = cource.WithContext(ctx)
```

2. **Controller watches the channel** — In `AddToManager`, the controller registers a `WatchesRawSource` on the channel:

```go
if pkgcfg.FromContext(ctx).AsyncSignalEnabled {
    builder = builder.WatchesRawSource(source.Channel(
        cource.FromContextWithBuffer(ctx, "VirtualMachine", 100),
        &handler.EnqueueRequestForObject{}))
}
```

3. **Producers send events** — Background services or other reconcilers push events into the channel:

```go
chanSource := cource.FromContextWithBuffer(ctx, "VirtualMachine", 100)
chanSource <- event.GenericEvent{
    Object: &vmopv1.VirtualMachine{
        ObjectMeta: metav1.ObjectMeta{
            Namespace: ns,
            Name:      name,
        },
    },
}
```

4. **Context joining** — The reconciler joins the cource context so it shares the same channel map as the controller setup:

```go
ctx = cource.JoinContext(ctx, r.Context)
```

### Key Points

- Channel keys are strings (e.g., `"VirtualMachine"`, `"VirtualMachineImageCache"`) identifying which resource type the channel serves.
- Always use `FromContextWithBuffer` with a buffer (typically 100) to prevent blocking producers.
- The same channel is shared between producer and consumer via the context — both sides call `FromContextWithBuffer` with the same key.
- `JoinContext` copies channel references from one context to another (used to propagate from controller-manager context into per-reconcile context).
- This pattern enables cross-controller and cross-service signaling without direct dependencies.

## Conditions

Conditions communicate status to users. They do NOT control reconciliation flow:

```go
// Good - reflect observable state
conditions.MarkTrue(ctx.VM, vmopv1.VirtualMachineConditionCreated)

// Good - surface errors
conditions.MarkFalse(ctx.VM,
    vmopv1.VirtualMachineConditionCreated,
    "CreateError",
    "%v", err)

// Bad - using conditions to skip reconciliation steps
if conditions.IsTrue(obj, "StepDone") { return nil }
```

## RBAC

Document permissions using kubebuilder markers above the Reconcile function:

```go
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get;update;patch
```

## Anti-Patterns

1. **Event-driven logic** - Never branch on create/update/delete event type
2. **Modifying Spec in controllers** - Controllers should only modify Status
3. **Missing owner references** - Set `controllerutil.SetControllerReference` on owned resources
4. **Blocking without context** - Always use `select` with `ctx.Done()` for channels
5. **Returning errors without status updates** - Always set conditions before returning errors
6. **Shared mutable state** - Use `sync.Map` or mutexes for concurrent reconciles
