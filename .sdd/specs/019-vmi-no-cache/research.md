# Research: Reduce controller manager memory at image scale

## Heap profile (909 MB, scale testbed)

| Item | Size | Note |
|------|------|------|
| Informer LIST decode | 384 MB | A short-lived decode of the full LIST. It is not the steady-state store. Which type owns it is not yet measured. `VirtualMachineImage` is the likely largest part. |
| Cache store and indexes | about 60 MB | An LRU can save at most this much, so it is not worth the work. |
| Content library item `ReconcileNormal` | 192 MB | 107 MB is the property fetch in the fast-deploy sync. 50 MB is image metrics labeled by name and namespace. |
| vSphere watcher | 44 MB | |
| Conversion webhook | 17 MB heap | In a second run, 55% of CPU. Out of scope here. |

## Why a high worker count is not the main cause

An idle worker is a parked goroutine and costs almost nothing. Only the content library item controller shows large allocations inside `Reconcile`. Its object count grows as images × namespaces, so a resync keeps all of its workers busy. The 384 MB LIST decode does not depend on concurrency.

## Cache behavior that shaped the plan

- `CacheOptions.DisableFor` stops client `Get` and `List` from using the cache. It does not stop an informer that is created by `Watches(...)` or `IndexField(...)`. Both `VirtualMachineImage` sites must go, or the memory is not saved.
- An uncached `List` with `MatchingFields` sends a field selector to the API server. A CRD supports only `metadata.name` and `metadata.namespace` unless the CRD declares `selectableFields`. Selectable fields are beta (on by default) from Kubernetes 1.30 and GA in 1.32. The CRD declares them per version.
- Checked with controller-gen v0.22.0 (`hack/tools/go.mod`) and a real kube-apiserver v1.34.1 from envtest: an uncached client filters by a declared field, and an undeclared field (`status.type`) returns `field label not supported`.
- A CRD can have at most 8 selectable fields, and each must be a scalar.

## Call sites that read `VirtualMachineImage`

| Site | When it runs | Cost without the cache |
|------|--------------|------------------------|
| `GetVirtualMachineImageSpecAndStatus` (`pkg/providers/vsphere/vmprovider_utils.go`) | VM create | One GET per create |
| Mutation webhook `ResolveImageNameOnCreate` | VM create | One GET for `vmi-…` names. A filtered LIST by `status.name` otherwise. |
| Validation webhook `getVMImageConfigSpec` | VM create | One GET per create |
| Unified storage quota webhook | VM create | One GET per create |
| Policy reconciler `getPolicyEvaluationResults` (`pkg/vmconfig/policy/policy_reconciler_util.go`) | Create, placement, and every VM update reconcile when `VSpherePolicies` is on | **Hot path.** One GET per VM reconcile. Only needs the image name and labels. |
| CD-ROM reconciliation `processImage` (`pkg/providers/vsphere/virtualmachine/cdrom.go`) | Every update reconcile for VMs with image-backed CD-ROMs | Narrow but per reconcile |
| Storage policy usage controller | Per VM per usage reconcile | Scales with VMs per policy per namespace. Measure. |
| Content library controller (`controllers/contentlibrary/utils/controller_builder.go`) | Every item reconcile, including resyncs | One GET per item per reconcile. No watch, so no event amplification. |
| Publish request controller | Watch | Replaced by polling |

These are call counts from reading the code. They are not measured. Possible later reductions: skip the policy image lookup when the VM already has policy tags, and cache the CD-ROM backing file name for the length of one reconcile.

## Fast-deploy property fetch

`syncVirtualMachineImageFastDeployVM` (`pkg/providers/vsphere/vmprovider.go`) fetches `config, guest, layoutEx, summary`. `UpdateVmiWithVirtualMachine` reads only `vm.Config` fields (firmware, guest ID, version, vApp config, hardware devices, extra config). Callers other than this one are not yet checked.

## Memory limit

- `GOMEMLIMIT` is not set anywhere in this repository.
- `config/manager/manager.yaml` sets a 200Mi limit. The 1800Mi limit from the ticket comes from outside this repository.
- Go reads `GOMEMLIMIT` as a byte count with a unit suffix. It does not accept a percentage, so a static value cannot follow a changing container limit. For this reason the plan reads the cgroup limit in-process.

## Options considered and not chosen

- **LRU in front of live GETs.** The steady-state store is about 60 MB, so the saving is small. A custom LRU also adds code, and `k8s.io/utils` is forbidden by `depguard`.
- **Metadata-only cache for images.** The mapper in the publish request controller reads `status.name`, which is not in object metadata.
- **In-memory filter in a namespace LIST for `ResolveImageName`.** Works without a CRD change, but each call lists the whole namespace. Selectable fields are cheaper.
- **Lower concurrency for every controller on the global-20 list.** Only the content library item controller shows large allocations in the profile.

## Open items

- Supervisor Kubernetes version.
- Profiled build, and whether it has the `vmi_name` metric.
- Owner of the 384 MB LIST decode, after the cache change.
