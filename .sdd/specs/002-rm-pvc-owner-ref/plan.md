# Implementation Plan: Remove PVC OwnerRef on Detach

- **Spec**: [`spec.md`](./spec.md)
- **Ticket**: bz-3734442
- **Date**: 2026-07-08

## Summary

Before the `VirtualMachine` controller removes a VM's finalizer, it will list every `PersistentVolumeClaim` in the VM's namespace that carries an `OwnerReference` back to that VM, and — for any such PVC whose name is no longer present in `spec.volumes[].persistentVolumeClaim.claimName` — patch the PVC to strip that one `OwnerReference` entry, unless the PVC carries the `vmoperator.vmware.com/keep-owner-ref` annotation. This prevents Kubernetes' garbage collector from cascade-deleting a PVC that was detached from the VM before the VM itself was deleted, while still letting a user opt a specific PVC back into the old cascade-delete behavior.

## Technical context

- **Go version**: 1.26.4 (root `go.mod`)
- **API version touched**: `v1alpha6` (current `vmopv1` alias per `.golangci.yml` `importas`) — read-only (`VirtualMachine.Spec.Volumes`); no CRD schema changes.
- **Modules touched**: root module only (`github.com/vmware-tanzu/vm-operator`).
- **Packages touched**:
  - `pkg/constants/constants.go` — new `KeepOwnerRefAnnotationKey = "vmoperator.vmware.com/keep-owner-ref"` constant, following the existing `XxxAnnotationKey` convention (e.g. `NoUnmanagedVolumesRegisterAnnotationKey`).
  - `pkg/util/kube/` — new file with the listing + patch logic (business logic, per constitution "thin controllers").
  - `controllers/virtualmachine/virtualmachine/virtualmachine_controller.go` — call site in `ReconcileDelete`, new field-index registration in `AddToManager`, new RBAC marker.
- **New dependencies**: none. `k8s.io/client-go/util/retry` is already an indirect dependency of `client-go` (already in `go.sum`) but is not currently imported anywhere in this repo; this plan introduces its first use for a standard `RetryOnConflict` patch loop. No `go.mod` change is required.

## Constitution check

| Rule | Status | Notes |
|------|--------|-------|
| API compatibility | OK | No API/CRD field changes; purely reads existing `spec.volumes`. |
| Thin controllers | OK | Listing/filtering/patching logic lives in `pkg/util/kube/pvc.go`; the controller only calls one function from `ReconcileDelete`. |
| No direct vSphere calls from controllers | OK | This is a pure Kubernetes-object operation; no provider/vSphere involvement. |
| Field indexer for filtered List calls | OK | A new field index on `PersistentVolumeClaim` (owner-ref VM UID) is added so the list is server-side filtered, matching the existing pattern in `volumebatch_controller.go` (`spec.nodeuuid`, `spec.volumes.persistentVolumeClaim.claimName`). |
| RBAC markers documented above Reconcile | OK | New `persistentvolumeclaims` marker added to `virtualmachine_controller.go`; `config/rbac` regenerated via `make generate-manifests`. |
| Level-triggered / idempotent reconciliation | OK | The cleanup re-derives "detached" from current state (`spec.volumes` vs. live `OwnerReferences`) on every call; patching is a no-op if the OwnerRef is already gone. |
| Testing standards | OK | Unit coverage in `pkg/util/kube/pvc_test.go` (external `_test` package, Ginkgo/Gomega, `testlabels`) and in `virtualmachine_controller_test.go`'s existing `ReconcileDelete` `Describe`. |
| E2E sync with changes | Applies | This is cluster-observable VM-deletion behavior; E2E coverage required in the same PR — see "Test strategy." |

No constitutional rule needs to be bent; no Complexity tracking entry is required.

## Project structure

```
pkg/constants/
  constants.go                        # MODIFIED: new KeepOwnerRefAnnotationKey constant

pkg/util/kube/
  pvc.go                              # NEW: field-index key, index registration, RemoveStaleVMOwnerRefFromPVCs
  pvc_test.go                         # NEW: unit tests (fake client, external _test package)

controllers/virtualmachine/virtualmachine/
  virtualmachine_controller.go        # MODIFIED: AddToManager registers the new field index;
                                       #           ReconcileDelete calls kubeutil.RemoveStaleVMOwnerRefFromPVCs
                                       #           before RemoveFinalizer; new RBAC marker added
  virtualmachine_controller_test.go   # MODIFIED: new ReconcileDelete scenarios (see Test strategy)

config/rbac/                          # REGENERATED via `make generate-manifests`

test/e2e/vmservice/virtualmachine/     # MODIFIED: new scenario for detach-then-delete (see Test strategy)
```

## API / CRD strategy

Not applicable — no CRD, field, or status-condition changes. `VirtualMachineVolume` / `PersistentVolumeClaimVolumeSource` (`api/v1alpha6/virtualmachine_storage_types.go:71-245`) are read-only inputs to this feature.

## Controller / webhook impact

### `pkg/util/kube/pvc.go` (new)

Two exported functions, following the existing filter-and-`SetOwnerReferences` pattern already used in `pkg/util/vmopv1/vmgroup.go:365-399` (`RemoveStaleGroupOwnerRef`) and the field-indexer pattern in `controllers/virtualmachine/volumebatch/volumebatch_controller.go:73-100`:

- `const PVCOwnerRefVMUIDIndexKey = "metadata.ownerReferences.vmUID"` — the field-index name, exported so the controller's `AddToManager` and this package's `List` call agree on it.
- `IndexPVCByVMOwnerRef(ctx context.Context, mgr ctrlmgr.Manager) error` — registers a field index on `corev1.PersistentVolumeClaim`, extracting the `UID` of any `OwnerReference` whose `Kind == "VirtualMachine"` and `APIVersion == vmopv1.GroupVersion.String()`. Called once from the `VirtualMachine` controller's `AddToManager`.
- `RemoveStaleVMOwnerRefFromPVCs(ctx context.Context, c ctrlclient.Client, vm *vmopv1.VirtualMachine) error`:
  1. `client.List` PVCs with `ctrlclient.InNamespace(vm.Namespace)` + `ctrlclient.MatchingFields{PVCOwnerRefVMUIDIndexKey: string(vm.UID)}`.
  2. Returns immediately if the list is empty (no PVCs owned by this VM — the common case for a VM with no unmanaged disks).
  3. Builds a `sets.Set[string]` of currently attached claim names from `vm.Spec.Volumes[].PersistentVolumeClaim.ClaimName`.
  4. For every listed PVC **not** in that set, calls an unexported `removeVMOwnerRef` helper that:
     - Re-`Get`s the PVC (avoids acting on a possibly-stale list entry).
     - Checks `pkgconst.KeepOwnerRefAnnotationKey` via `_, ok := pvc.Annotations[pkgconst.KeepOwnerRefAnnotationKey]` — presence-only, value never inspected (US4). If present, returns `nil` immediately without patching.
     - Filters `OwnerReferences` to drop the entry matching `vm.UID` (and `Kind == "VirtualMachine"`), leaving every other entry untouched (US3).
     - No-ops (returns `nil` without a network call) if nothing changed.
     - Otherwise patches via `ctrlclient.MergeFrom` inside `retry.RetryOnConflict` (`k8s.io/client-go/util/retry`), and treats `apierrors.IsNotFound` as success (PVC already gone).
  5. Aggregates any real errors with `k8s.io/apimachinery/pkg/util/errors.NewAggregate` and returns them; one failing PVC does not stop the others from being processed.

The annotation check happens on the freshly-`Get`'d PVC (not the stale list entry), so a user adding/removing the annotation concurrently with the delete reconcile is resolved consistently with the rest of the level-triggered design (edge case in `spec.md`).

### `controllers/virtualmachine/virtualmachine/virtualmachine_controller.go`

- **`AddToManager`**: add a call to `kubeutil.IndexPVCByVMOwnerRef(ctx, mgr)` alongside any existing index registrations for this controller.
- **RBAC**: add `// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;patch` near the existing markers (line ~302-314). The controller currently has **no** PVC permissions at all — confirmed via `grep kubebuilder:rbac` — unlike `controllers/virtualmachine/volume/volume_controller.go:227` and `volumebatch_controller.go`, which already have broader PVC verbs for a different purpose. Run `make generate-manifests` to regenerate `config/rbac/role.yaml`.
- **`ReconcileDelete`** (currently lines 488-548): insert a call to `kubeutil.RemoveStaleVMOwnerRefFromPVCs(ctx, r.Client, ctx.VM)` immediately before the two `controllerutil.RemoveFinalizer` calls at lines 538-539, **outside/after** the `if pkgconst.SkipDeletePlatformResourceKey ... else ...` branch (lines 510-536) — i.e. it runs unconditionally whenever the finalizer is about to be removed, whether the vCenter VM was actually deleted or merely unregistered/skipped. This matches the spec's edge case that the Kubernetes-side cascade-delete risk is independent of what happened on the vSphere side. If this call returns an error, propagate it and do **not** remove the finalizer — the existing deferred-patch-and-return pattern in `Reconcile` already handles that correctly, and retrying on the next reconcile is safe (idempotent).

No other controller, webhook, or provider changes. `controllers/virtualmachine/volume/volume_controller.go` and `controllers/virtualmachine/volumebatch/volumebatch_controller.go` are unaffected — their `ReconcileDelete` no-ops (relying on GC for `CnsNodeVmAttachment`/`CnsNodeVMBatchAttachment`) are correct as-is and out of scope.

## Test strategy

- **Unit — `pkg/util/kube/pvc_test.go`** (`Label(testlabels.Controller)`, fake `ctrlclient.Client`, external `_test` package):
  - PVC with matching OwnerRef, name absent from `spec.volumes` → OwnerRef removed, PVC otherwise unchanged.
  - PVC with matching OwnerRef, name present in `spec.volumes` → no patch issued.
  - PVC with two OwnerReferences (VM + unrelated) → only the VM's entry removed (US3).
  - No PVCs indexed for this VM → function returns `nil` without listing errors.
  - PVC deleted between `List` and `Get` → tolerated, no error returned.
  - Patch conflict on first attempt, succeeds on retry → end state correct, `RetryOnConflict` exercised.
  - Detached PVC carries `pkgconst.KeepOwnerRefAnnotationKey` (US4) → OwnerRef left intact, no patch issued.
  - Detached PVC carries the annotation with an empty-string value → still opted out (value is never inspected).
- **Unit — `virtualmachine_controller_test.go`** (existing `ReconcileDelete` `Describe`, `testlabels.Controller` + `testlabels.EnvTest` for the integration variant):
  - Extend with a fake-provider scenario: VM with an attached and a detached unmanaged-disk PVC; after `ReconcileDelete`, assert the detached PVC's `OwnerReferences` no longer contains the VM and the attached PVC's does.
  - Assert the `SkipDeletePlatformResourceKey` (unregister) path also triggers OwnerRef cleanup.
  - Extend with a third PVC that is detached but carries `pkgconst.KeepOwnerRefAnnotationKey`; assert its OwnerRef survives `ReconcileDelete` alongside the attached PVC's.
- **Integration (envtest)**: extend the controller's existing `intgTestsReconcile` to cover the same detach/attach scenario against a real API server, confirming the patch persists, plus a detached-but-annotated PVC that keeps its OwnerRef. Note: `envtest` does not run the upstream garbage-collector controller by default, so this layer verifies the `OwnerReferences` mutation itself, not the resulting cascade-delete (or lack thereof) — that is proven at the E2E layer.
- **E2E (mandatory per `e2e-sync-with-changes.md`)** — extend `test/e2e/vmservice/virtualmachine/`: deploy a VM Service VM from a multi-disk VMI, wait for both disks to register as PVCs with the VM's OwnerRef, remove the second disk's entry from `spec.volumes`, delete the VM, and assert: the vSphere VM is gone, the first (still-attached) PVC is garbage-collected, and the second (detached) PVC still exists with no OwnerRef to the deleted VM. Where practical, cover the opt-out annotation in the same or a sibling scenario: annotate a detached PVC with `vmoperator.vmware.com/keep-owner-ref` and assert it is garbage-collected along with the VM instead of surviving.

## Rollout / migration

- **No feature flag.** This is a bug fix that only ever *prevents* an erroneous deletion; it never causes a PVC to be deleted that wouldn't have been deleted before. There is no rollback scenario where reverting to the old (always-delete) behavior is preferable, so gating this behind `pkgcfg.Features.*` would add operational complexity without a corresponding safety benefit.
- **No schema/backfill migration.** The fix only affects the OwnerRef state evaluated at the moment a VM is deleted going forward; PVCs already lost to the pre-fix cascade-delete are not recoverable and are explicitly out of scope (see spec).
- **Partner comms**: release note should mention the corrected behavior, since a partner who was relying on "detach + delete VM" to fully clean up a disk will see a behavior change (the PVC now survives and must be deleted explicitly). The release note should also document the `vmoperator.vmware.com/keep-owner-ref` annotation as the escape hatch back to the old per-PVC behavior.

## Complexity tracking

Not applicable — no constitutional rule is bent by this change.
