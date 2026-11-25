# Plan: Reduce controller manager memory at image scale

Implements [spec.md](./spec.md). Evidence is in [research.md](./research.md).

## Summary

Remove the `VirtualMachineImage` informer, so the large LIST decode and the store are never created. Replace the two cache-dependent lookups with API-server field selectors. Add a soft Go memory limit. Then cut two smaller costs in the content library item path.

## Technical context

- Go 1.27.4, controller-runtime v0.25.2, client-go v0.37.1.
- API version: `v1alpha6` only for the CRD change.
- Modules: root module and `api/`. No sub-module changes.

## Constitution check

- API compatibility: the change is additive (selectable fields on the CRD). It does not remove or rename a field. No conversion webhook is needed.
- Controllers stay thin. The publish request lookup change stays in the controller. Shared logic stays in `pkg/`.
- CRD manifests are regenerated with `make generate-manifests` and checked in.
- The change is observable on a Supervisor, so E2E coverage ships with it (see Test strategy).
- Comments follow the grammar rules. New Go files carry the copyright header.

## Changes required

### 1. Selectable fields on the image CRDs
File: `api/v1alpha6/virtualmachineimage_types.go`
- On `VirtualMachineImage`, add `+kubebuilder:selectablefield:JSONPath=".status.name"` and `+kubebuilder:selectablefield:JSONPath=".status.providerItemID"`.
- On `ClusterVirtualMachineImage`, add `+kubebuilder:selectablefield:JSONPath=".status.name"`.
- Regenerate `config/crd/bases/vmoperator.vmware.com_virtualmachineimages.yaml` and `config/crd/bases/vmoperator.vmware.com_clustervirtualmachineimages.yaml`.

### 2. Stop caching `VirtualMachineImage`
File: `pkg/manager/manager.go`
- Add `&vmopv1.VirtualMachineImage{}` to the client `CacheOptions.DisableFor` list, with a comment that explains why. The list already has ConfigMap, Secret and Deployment.
- `DisableFor` only changes client reads. It does not stop informers that are made by `Watches(...)` or `IndexField(...)`. Changes 3 and 4 remove both.

### 3. Remove the `VirtualMachineImage` field index
File: `webhooks/virtualmachine/mutation/virtualmachine_mutator.go`
- Remove the `IndexField` call for `VirtualMachineImage` and `status.name`. Keep the one for `ClusterVirtualMachineImage`.
- `ResolveImageName` in `pkg/util/vmopv1/vm.go` does not change. Its `MatchingFields{"status.name": …}` list now goes to the API server and uses the selectable field.

### 4. Remove the `VirtualMachineImage` watch from the publish request controller
File: `controllers/virtualmachinepublishrequest/virtualmachinepublishrequest_controller.go`
- Remove the `Watches(&vmopv1.VirtualMachineImage{}, …)` call and its mapper.
- Find the published image with `MatchingFields{"status.providerItemID": <item ID>}`.
- While the image is not yet available, requeue with the existing `requeueResult()` poll (10 seconds).

### 5. Fake client index for tests
File: `test/builder/fake.go`
- Register a `status.providerItemID` index for `VirtualMachineImage` on the shared fake client. The real API server handles this for the selectable field, and the fake client must do the same. Check that the existing `status.name` index is also registered there.

### 6. Soft memory limit
New file: `pkg/mem/limit.go`, in the existing `pkg/mem` package. `main.go` calls it from `initMemStats`, before the manager starts.
- If the `GOMEMLIMIT` environment variable is set, do nothing. The Go runtime already uses it.
- Otherwise, read the container memory limit from the cgroup (`/sys/fs/cgroup/memory.max` on cgroup v2, `/sys/fs/cgroup/memory/memory.limit_in_bytes` on v1). If there is no limit, or the value is "max" or very large, set nothing.
- Otherwise, call `debug.SetMemoryLimit` with 85% of the limit. For an 1800Mi limit this is about 1530Mi.
- Log the value that was set, or why none was set.
- Why in-process and not a static environment variable: the 1800Mi limit is not set in this repository (`config/manager/manager.yaml` has 200Mi, and the Supervisor changes it). A static `GOMEMLIMIT` in `config/default/` would not match the real limit. Reading the cgroup keeps the two values together.
- A soft limit is not a hard limit. The garbage collector runs more often near the limit, and this costs CPU. It cannot reduce live memory. It only reduces garbage that has not been collected yet. This is the reason the other changes are still needed.

### 7. Fetch only `config` in fast-deploy sync
File: `pkg/providers/vsphere/vmprovider.go` (`syncVirtualMachineImageFastDeployVM`)
- Change the property list from `config, guest, layoutEx, summary` to `config`.
- First check every caller of `syncVirtualMachineImageFastDeployVM` and of `contentlibrary.UpdateVmiWithVirtualMachine` in `pkg/providers/vsphere/contentlibrary/content_library_utils.go`, to confirm that nothing reads the other three properties. The heap profile shows 107 MB for this fetch.

### 8. Image metrics cardinality
No change on `main`. The per-image metrics were removed by #1989. Before that change, the content library item controller deleted the series when an item was deleted, so the 50 MB was the live series count and not a leak. TODO: For release branches, #1989 is also useful to reduce the memory footprint.

### 9. Content library item concurrency
Files: `config/default/manager_max_concurrent_reconciles_patch.yaml`, `main.go`
- Set a lower value only for `contentlibraryitem-controller`, with the existing `--controller-max-concurrent-reconciles` flag. The flag takes comma-separated `controller=value` pairs (parser in `main.go`, `parseControllerMaxConcurrentReconciles`).
- Pick the value from the goroutine profile and the API call rate measured on the scale testbed. After change 2, each reconcile also makes a live GET for the image, so too many workers makes API server load, not memory.

## Controller and webhook impact

| Component | Change |
|-----------|--------|
| Manager cache | No `VirtualMachineImage` informer |
| VM mutation webhook | Index removed for `VirtualMachineImage`; lookup uses the API server |
| Publish request controller | Watch removed; poll |
| Content library item controller | Live GET per reconcile; fewer workers; smaller property fetch |
| Policy reconciler, CD-ROM reconciler, storage policy usage controller, VM create paths, quota and validation webhooks | Live GET instead of a cache read. No code change. |

No new RBAC: the manager already has `get;list;watch` on `virtualmachineimages`. No new feature flag.

## Test strategy

- **Unit and envtest** (`testlabels.Controller`, `testlabels.EnvTest`):
  - Publish request suite: image-available state is reached by polling, with no watch.
  - `ResolveImageName`: friendly name and `vmi-…` name, against envtest so the real selectable field is used.
  - A test that a field selector on an undeclared field is rejected, to guard the CRD markers.
  - Fake client tests keep working with the new index.
- **Unit** for the memory limit helper (`pkg/mem/limit_test.go`): `GOMEMLIMIT` set, no cgroup limit, v1 and v2 files, value "max", ratio math. Read the cgroup path through a parameter so that tests can use a temp directory.
- **Unit** for the fast-deploy sync: the image status is the same with the smaller property list (vcsim).
- **E2E** (`test/e2e/`, per [`e2e-sync-with-changes.md`](../../memory/e2e-sync-with-changes.md)): create a VM by friendly image name, and check that a publish request reaches the image-available state.
- **Scale check** (not a CI test): on the scale testbed, take heap profiles before and after each change, and a goroutine profile in the OOM window.

## Rollout and migration

- No feature flag. The cache change is on for all installs.
- Needs Kubernetes 1.30 or newer on the Supervisor. This must be confirmed before merge.
- The CRD with selectable fields must be applied before the manager that depends on it starts. If the old CRD is still installed, `ResolveImageName` fails with a "field label not supported" error. Confirm the CRD and manager ship together.
- Release note: say that image reads now use the API server, that publish request detection can take up to 10 seconds longer, and that Kubernetes 1.30 or newer is needed.

## Risks

| Risk | Mitigation |
|------|------------|
| Live GETs increase API server load, mostly from content library item resyncs | Measure; lower concurrency (change 9) |
| Selectable fields missing (old Kubernetes or old CRD) | Version check before merge; envtest guard |
| Soft memory limit raises CPU use near the limit | Log the value; allow `GOMEMLIMIT` override |

## Complexity tracking

None. No constitutional rule is bent.
