# Implementation Plan: VirtualMachineInfraMigration Status on VirtualMachine

- **Spec**: [`spec.md`](./spec.md)
- **Model**: [`model.md`](./model.md)

## Summary

Add a controller that watches `VirtualMachineInfraMigration` resources from the mobility operator and mirrors every one of their conditions that describes the migration's current generation onto the VirtualMachine with the same namespace and name, as `VirtualMachineInfraMigration<Type>` conditions. When the migration is ready, it also sets a new `status.volumeAttributesClassName` field on the VirtualMachine. The controller does not act when the migration is deleted. The controller only starts when the `StoragePolicyMutability` feature is enabled. The migration only carries the home directory's storage policy ID, so the controller looks up the VolumeAttributesClass for that policy and records it only if it is also the one in the VirtualMachine's spec.

## Technical context

- **Go**: as pinned by the root `go.mod`.
- **API versions**: adds condition constants and the `status.volumeAttributesClassName` field to `v1alpha6`. Consumes `mobility-operator.vmware.com/v1alpha4` (`VirtualMachineInfraMigration`).
- **Modules**: the root module now requires `external/mobility-operator` (with a `replace`), because a root package imports its API. `external/mobility-operator` remains its own Go module, and its `go.mod` and `go.sum` are unchanged because `v1alpha4` depends only on `k8s.io/apimachinery`.
- **Feature flag**: `pkgcfg.Features.StoragePolicyMutability`, driven by the existing storage policy mutability capability. No new flag.

## Constitution check

- **Controllers are thin; logic in `pkg/`**: the logic is a condition mirror of under 100 lines, kept in the reconciler, matching the `VolumeAttributesClass` controller which this controller is modeled on. If it grows, it moves to `pkg/`.
- **No direct vSphere calls**: none are made.
- **`status.observedGeneration` and a `Ready` condition**: not applicable. The controller does not own a resource. It writes the mirrored conditions on a VirtualMachine whose status is owned by the VirtualMachine controller, and it never writes the migration resource.
- **Shared list-typed field**: `status.conditions` on the VirtualMachine is written by several controllers. The controller uses `pkg/patch.Helper`, which re-reads the latest VirtualMachine, applies only the changed conditions, and patches with an optimistic lock, retrying on conflict. The mirrored conditions are all the conditions whose type starts with `VirtualMachineInfraMigration`, which no other controller writes, so the controller declares them as owned (`patch.WithOwnedConditions`) and the helper writes or removes them without treating a concurrent change as a conflict. This satisfies the fan-in rule in the constitution. `status.volumeAttributesClassName` is a scalar that only this controller writes, so it needs no optimistic lock: the helper's merge patch sends only that field and cannot drop another writer's change. If the spec changes between the read and the write, the VirtualMachine event it causes reconciles the migration again and corrects the field, or emits the mismatch event.
- **Controllers for other API groups are not placed directly in `controllers/`**: the controller lives under `controllers/mobility/`.
- **Tests**: one `_test.go` and one `_suite_test.go` per package, labels from `pkg/constants/testlabels`.
- **E2E**: scoped as a follow-up, see Test strategy. This is the explicit exception allowed by `e2e-sync-with-changes.md` and is called out in the PR description.

## Project structure

```
api/v1alpha6/virtualmachine_types.go          MODIFY — VirtualMachineConditionInfraMigrationPrefix, VirtualMachineConditionInfraMigrationReady, status.volumeAttributesClassName
api/v1alpha{1..5}/zz_generated.conversion.go  MODIFY — generated: one warning line each for the new status field
config/crd/bases/vmoperator.vmware.com_virtualmachines.yaml  MODIFY — generated: status.volumeAttributesClassName
controllers/controllers.go                    MODIFY — register mobility.AddToManager
controllers/mobility/controllers.go           NEW    — gates the controllers on StoragePolicyMutability
pkg/util/kube/storage.go                      MODIFY — GetVACNamesForStoragePolicyID (and tests in storage_test.go)
pkg/conditions/{conditions,getter}.go          MODIFY — IsReady, the WithGeneration option, and GetObservedAt (and tests in getter_test.go)
controllers/mobility/virtualmachineinframigration/
  virtualmachineinframigration_controller.go            NEW — reconciler, watches, RBAC markers
  virtualmachineinframigration_controller_suite_test.go NEW
  virtualmachineinframigration_controller_test.go       NEW — unit and envtest specs
pkg/manager/manager.go                        MODIFY — register the mobility v1alpha4 scheme when the feature is on
test/builder/fake.go                          MODIFY — register the scheme and the status subresource for the fake client
config/rbac/role.yaml                         MODIFY — generated: get/list/watch virtualmachineinframigrations
config/crd/external-crds/mobility-operator.vmware.com_virtualmachineinframigrations.yaml  NEW — generated, test-only CRD for envtest
external/mobility-operator/api/v1alpha4/      NEW    — VirtualMachineInfraMigration API only, plus generated deepcopy
go.mod                                        MODIFY — replace and require external/mobility-operator
.golangci.yml                                 MODIFY — importas alias mobilityv1a4
Makefile                                      MODIFY — generate-go and generate-external-manifests cover mobility-operator
```

## API / CRD strategy

Additive only, in `v1alpha6`: condition type constants, which need no schema change because conditions are opaque strings, and one optional string field, `status.volumeAttributesClassName`, which changes the `VirtualMachine` CRD schema and is regenerated with `make generate-manifests`. No version bump is needed. Each older version already has a manual `VirtualMachineStatus` converter, and its `ConvertTo` restores the whole hub `status` from the conversion annotation, so the new field round-trips without a restore function. `make generate-go-conversions` only adds a `does not exist in peer-type` warning line to each older version's generated file. The conversion round-trip suites in `api/test` pass with the field.

## Controller impact

- **Watches**: `VirtualMachineInfraMigration` (primary) and `VirtualMachine`. A VirtualMachine event is mapped to the migration with the same namespace and name. It is dropped when no such migration exists, so most VirtualMachine events cost one cache read. The VirtualMachine watch covers the case where the migration's conditions were set before the VirtualMachine existed.
- **VolumeAttributesClass lookup**: for a ready migration with `spec.targetStorage.homeStoragePolicyID`, the controller lists the cluster's VolumeAttributesClasses through the manager's cache and keeps those whose `parameters["storagePolicyID"]` matches (`kubeutil.GetVACNamesForStoragePolicyID`). If the VirtualMachine's `spec.volumeAttributesClassName` is one of them, it is written to `status.volumeAttributesClassName`. Otherwise nothing is written and a `VolumeAttributesClassMismatch` warning event is emitted on the VirtualMachine, which is deduplicated by the event recorder. A failure to list is returned so the reconcile is retried. The VirtualMachine's conditions are still mirrored in the mismatch cases. The controller does not watch VolumeAttributesClasses, so a VolumeAttributesClass that appears later is picked up on the next VirtualMachine or migration event.
- **Reconcile**: every condition of the migration that describes its current generation is mirrored onto the VirtualMachine, as `VirtualMachineInfraMigration` followed by the condition's type, with its status, reason (or the status when it has none), and message followed by a `@ VirtualMachineInfraMigration/<name>` reference (the reference alone when it has no message). A condition describes the current generation when its own `observedGeneration` is at least the migration's `metadata.generation` (`conditions.GetObservedAt(obj, obj.Generation)`), so one condition can be held back while another is mirrored, and a condition without an `observedGeneration` is never mirrored. A condition that is behind describes the previous spec, so it is not mirrored and the mirrored condition of that type is removed, as is one the migration no longer reports. When the migration's spec changes, the VirtualMachine therefore drops the outcomes of the previous spec until the migration reports them for the new generation, instead of briefly reporting a stale `True`. The `observedGeneration` is not copied. A migration is considered ready only when `conditions.IsReady(obj, conditions.WithGeneration(obj.Generation))` is true, i.e. its `Ready` condition is `True` and its `observedGeneration` is at least its `metadata.generation`, and only then is `status.volumeAttributesClassName` resolved and recorded (and the mismatch event emitted). A missing or deleting VirtualMachine is skipped without error. A migration that does not exist, or that is being deleted, is not treated specially: nothing is written for a missing migration, so the VirtualMachine keeps its mirrored conditions and `status.volumeAttributesClassName`. `status.volumeAttributesClassName` is never cleared.
- **No pause and no validation**: the controller does not pause the reconciliation of a VirtualMachine while a migration for it exists and is not ready, and no webhook rejects a change to `spec.volumeAttributesClassName` while the migration is running. The VirtualMachine reconcile already skips a VirtualMachine's spec while a task is running on it in vSphere (`ErrHasTask` in `pkg/providers/vsphere/vmprovider_vm.go`), which is enough to avoid concurrent operations, so this change adds nothing for it.
- **RBAC**: `get;list;watch` on `virtualmachineinframigrations` in `mobility-operator.vmware.com`, `get;list;watch` on `volumeattributesclasses` in `storage.k8s.io` (already granted for the `VolumeAttributesClass` controller), and `get;list;watch` on `virtualmachines` plus `get;update;patch` on `virtualmachines/status` in `vmoperator.vmware.com`. The VirtualMachine rules already exist, so only the migration rule is new in `config/rbac/role.yaml`.
- **Scheme**: the mobility v1alpha4 scheme is registered in the manager only when the feature is on, like the other feature-gated external APIs.
- **Known risk**: a controller `For()` a kind whose CRD is absent fails to start, which would stop VM Operator. The feature gate is the only protection, and it is enough because the spec guarantees that the CRD is installed whenever the capability is enabled.

## Build and generation

- `make generate-go` generates deepcopy for `external/mobility-operator/...` using the same form as every other external module. This is possible because the root module now imports the package, so `go mod tidy` keeps the `require`. This also updates the v1alpha2 deepcopy file's license header to the repository boilerplate.
- `make generate-external-manifests` generates the test-only CRD for `mobility-operator/api/v1alpha4` only. v1alpha2 holds `ImportOperation`, which only the E2E tests use, and generating over both versions would also emit an `importoperations` CRD that no envtest suite needs.

## The `v1alpha4` API package

`external/mobility-operator/api/v1alpha4` contains only what this controller consumes: the `VirtualMachineInfraMigration` types, the group-version registration, and the generated deepcopy. Like the other `external/*` modules, it is not a full copy of the owning operator's API. The mobility operator also defines `ImportOperation`, `StoragePolicyChange`, and related types in `v1alpha4`, which are deliberately not copied because nothing in this repository uses them. Adding one later means adding its types here and regenerating. The package carries the repository license header, and has no `Hub()` conversion stubs or `SetConditions` because nothing in this repository converts or writes these objects.

## Test strategy

- **Unit** (`testlabels.Controller`, `testlabels.API`): fake client. Covers: migration not found, with and without a VirtualMachine, and a VirtualMachine with mirrored conditions left unchanged; no conditions (which removes stale mirrored conditions), a different condition only, several conditions mirrored with their status, reason, and message, no `observedGeneration` copied, a changed condition updated and a condition the migration no longer reports removed, `Ready` False and `Ready` Unknown (each mirrored but without recording a VolumeAttributesClass or emitting the mismatch event), a condition that describes a previous generation (not mirrored, a copy mirrored earlier removed, and no VolumeAttributesClass recorded and no event), a mix of current and previous generations judged condition by condition, a condition without an `observedGeneration` (not mirrored), conditions that caught up with a newer generation (mirrored, and the VolumeAttributesClass recorded), no VirtualMachine, VirtualMachine deleting, the happy path (status, reason, message), empty reason for every status, empty message, the message reference, preservation of other conditions, an unchanged last transition time on re-reconcile and a moved one when the condition changed, and the VolumeAttributesClass cases: spec agrees, a stale previous value replaced, several VolumeAttributesClasses for one policy, spec naming another policy's class, spec unset, no class for the policy, no `homeStoragePolicyID`, and a failing list. The helper has its own unit tests in `pkg/util/kube`.
- **Integration** (`testlabels.Controller`, `testlabels.EnvTest`): VirtualMachine exists first and the migration's conditions change over time (no conditions, `Ready` False with another condition, `Ready` True, and a condition the migration stops reporting), with the VirtualMachine following each change; the migration's spec changing, which advances its generation and removes the mirrored conditions until they are reported for the new generation; the migration ready before the VirtualMachine is created; and, with real VolumeAttributesClass objects, the status field being recorded only when the migration is ready and the VirtualMachine's spec agrees, and the status field and the conditions surviving the migration's deletion.
- **Conversion** (`api/test`): the existing round-trip suites for `v1alpha1` to `v1alpha5` cover the new status field.
- **E2E**: follow-up. It needs a Supervisor with the mobility operator and the `StoragePolicyMutability` capability, so it cannot be validated from a unit or envtest environment. Tracked as a task in `tasks.md`.

## Rollout

- Off unless the existing `StoragePolicyMutability` capability is enabled. No new flag or default.
- Release note: the VirtualMachine now mirrors the conditions of its `VirtualMachineInfraMigration` as `VirtualMachineInfraMigration<Type>` conditions (for example `VirtualMachineInfraMigrationReady`), and has a new `status.volumeAttributesClassName` field.
