# Reduce controller manager memory at image scale

- **Feature branch**: `vmi-no-cache`
- **Created**: 2026-11-25
- **Status**: Draft
- **Epic**: TBD (tracking ticket: vmop-4090)

## Problem

On a Supervisor with many content library items and many namespaces, the VM Operator controller manager is OOMKilled at its 1800Mi memory limit. The reported scale is 11 content libraries, 633 content library items, about 104 namespaces and about 52,000 `VirtualMachineImage` objects. The number of namespaced image objects grows as images × namespaces, so memory grows with it.

A heap profile of 909 MB at this scale shows these main costs:

| Cost | Size |
|------|------|
| Informer LIST decode | 384 MB |
| Cache store and indexes | about 60 MB |
| Content library item reconcile | 192 MB (107 MB vSphere property fetch, 50 MB image metrics labeled by name and namespace) |
| vSphere watcher | 44 MB |

The goal is an interim 2x–3x reduction in peak memory. A permanent design is out of scope.

## Goals

- The manager MUST NOT keep namespaced `VirtualMachineImage` objects in its informer cache. `ClusterVirtualMachineImage` objects MAY stay cached.
- Every lookup of a `VirtualMachineImage` by `status.name` or by `status.providerItemID` MUST return the same result as before, without a cache index.
- A VM publish request MUST still reach the image-available state without a `VirtualMachineImage` watch. The delay MAY be up to 10 seconds longer.
- The manager SHOULD limit Go heap growth to a fraction of its container memory limit, so that the garbage collector works harder before the kernel kills the process. An operator MUST be able to override the value with the `GOMEMLIMIT` environment variable.
- The content library item sync for fast-deploy images SHOULD fetch only the vSphere properties that it uses.
- The content library item controller SHOULD use fewer concurrent workers than the global default.
- Peak heap on the scale testbed SHOULD be at most half of the baseline measured on `main` at the same scale. The 909 MB profile came from an older build that still had the per-image metrics.

## Non-goals

- No LRU or other bounded cache for images.
- No change to `ClusterVirtualMachineImage` caching.
- No change to the `VirtualMachineImage` schema other than selectable fields.
- No change to the memory limit set by the Supervisor.
- No change to other controllers' concurrency.
- No release branch work. This spec covers `main` only.
- No metrics change. The per-image metrics (`vmi_name`, `vmi_namespace` labels) were already removed from `main` by #1989.

## Acceptance criteria

- With 52,000 `VirtualMachineImage` objects in the cluster, the manager heap profile has no informer for `VirtualMachineImage`, and the peak heap is at most half of the baseline measured on `main` at the same scale.
- Creating a VM with a friendly image name (not `vmi-…`) still resolves the image, and the VM is created.
- Creating a VM with a `vmi-…` image name still works.
- A `VirtualMachinePublishRequest` still reaches the image-available state.
- Starting the manager with no `GOMEMLIMIT` and a container memory limit sets a Go soft memory limit below that container limit. Setting `GOMEMLIMIT` overrides it. Starting with no container limit sets no soft limit.
- Fast-deploy image sync still fills the image status fields from the VM configuration.
- A field selector on `status.name` or `status.providerItemID` of `VirtualMachineImage`, and on `status.name` of `ClusterVirtualMachineImage`, is accepted by the API server.
- If the API server does not support the selectable fields, the manager logs a warning at startup and keeps working. The `status.name` and `status.providerItemID` lookups then use a namespace list filtered in memory, and return the same results.

## Behavior differences

- Each read of a `VirtualMachineImage` is now a call to the API server. Create-time paths make one call per VM create. Update-path callers make one call per reconcile: the policy reconciler when `VSpherePolicies` is on, the CD-ROM reconciler for image-backed CD-ROMs, the storage policy usage controller, and every content library item reconcile.
- Image-available detection in a publish request can take up to 10 seconds longer.
- The change needs selectable fields on the CRD. These need Kubernetes 1.31 or newer (beta, on by default), or 1.30 with the `CustomResourceFieldSelectors` feature gate turned on.
- Field selectors work only on API version `v1alpha6`.
- Without selectable field support, each `status.name` or `status.providerItemID` lookup lists all images in the namespace. This costs more per call, but the results are the same.

## Open questions

- [NEEDS CLARIFICATION: Which Kubernetes version does Supervisor ship? Selectable fields need 1.31 or newer. Owner: platform team. This blocks the cache change.]
- [NEEDS CLARIFICATION: The heap profile shows the per-image metrics, but `main` has no such metrics. Which build was profiled? Owner: spec author.]
- TODO: For release branches, #1989 (remove the per-image metrics) also reduces the memory footprint. Consider it for each release branch that still has the per-image metrics.
- [NEEDS CLARIFICATION: What fraction of the container limit is right for the soft memory limit? The plan proposes 85%.]
- [NEEDS CLARIFICATION: Assign the epic and the task tickets before merge.]
