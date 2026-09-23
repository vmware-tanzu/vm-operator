# Implementation Plan: VM Operator as a KubeVM provider

- **Spec**: [`spec.md`](./spec.md)
- **Epic**: TBD
- **Date**: 2026-08-31

## Summary

Add a generic-side core controller for `kube-vm.io/v1alpha1` `VirtualMachine` that adopts a VM Operator `VirtualMachine` as its infrastructure object and drives the generic object's status from fixed contract paths on it; add additive contract status fields to `vmoperator.vmware.com/v1alpha6` so those paths exist; and add the VM Operator side that resolves its own configuration from the owning generic object — a mutating-webhook step at create, and a small reconciler that keeps the desired power state in step. The delegation behaviour is behind a new `KubeVMProvider` feature gate, default off.

## Technical context

- **Go version**: root module `go 1.26.7`; `external/kubevm` deliberately floors at `go 1.23.0`.
- **API version(s) touched**: `vmoperator.vmware.com/v1alpha6` gains three optional status fields and one condition-type constant; `v1alpha1`–`v1alpha5` gain conversion-restore handling for those fields. `kube-vm.io/v1alpha1` is unchanged.
- **Modules touched**: root (`api/`, `webhooks/`, `controllers/`, `pkg/`, `config/`, `test/builder/`, `Makefile`, `go.mod`), and one new module for the core controller. `external/kubevm` itself gains no code.
- **New dependencies**: root gains `github.com/vmware-tanzu/vm-operator/external/kubevm` via the existing `replace`-plus-zero-pseudo-version pattern used by the eleven other `external/*` and `pkg/*` sub-modules. Root is at `k8s.io/apimachinery v0.36.1` / `controller-runtime v0.24.0` against kubevm's floor of `v0.31.0` / `v0.19.0`; module resolution takes the higher, so the floor costs root nothing.
- Every field path, function name and file reference below was checked against the repo. Items that could not be confirmed carry an `[UNVERIFIED]` marker.

## Constitution check

| Rule | Status | Notes |
|------|--------|-------|
| SDD artifacts under `.sdd/specs/NNN-slug/` | OK | `spec.md`, `plan.md`, `tasks.md` in this directory. |
| Spec declares an epic | Deviation | Header carries `Epic: TBD` while `Status: Draft`, which `sdd-standards.md` permits. The epic must be filed before this spec merges. |
| API compatibility | **Deviation** | Three optional status fields are added to a shipped CRD. See API / CRD strategy and Complexity tracking. |
| New CRD markers, `make generate-manifests` | OK | No new CRD. Regeneration of the existing manifests and of `zz_generated.*` is a task. |
| Thin controllers | OK | Mapping and linkage logic in `pkg/kubevm/`; the reconciler holds the loop, patch helper and watches only. |
| Controllers only modify Status | **Deviation** | The VM Operator reconciler writes `spec.powerState` on the provider object. See Complexity tracking. |
| Controllers must track `observedGeneration` and set `Ready` | OK | The core sets both on the generic object. |
| Controllers for other API groups not directly in `controllers/` | OK | The generic-group controller lives in its own module. The VM Operator-side reconciler is `For(&vmopv1.VirtualMachine{})`, so `controllers/` is its correct home. |
| Webhooks for other API groups not directly in `webhooks/` | OK | No webhook is added for the `kube-vm.io` group; the changes are inside the existing `vmoperator.vmware.com` mutating and validating webhooks. |
| CEL preferred for simple structural rules | Deviation | Annotation immutability is enforced in Go, because CEL expresses "immutable once non-empty on one key of a map" poorly and `validateAnnotation` already owns annotation rules. |
| Fan-in list-typed writes use an optimistic lock | OK | Owner references are patched with `client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})` and a skip-if-unchanged guard, because `controllers/virtualmachinegroup/virtualmachinegroup_controller.go:428` is a second writer of the same list via `controllerutil.SetOwnerReference`. |
| Feature flag gets spec + plan + tasks | OK | This is that spec. |
| One test file and one suite bootstrap per package | Partial | Honoured for all new packages. The existing `webhooks/virtualmachine/mutation` package still carries `virtualmachine_mutator_unit_test.go` and `virtualmachine_mutator_intg_test.go`; new coverage there goes into a new `virtualmachine_mutator_kubevm_test.go` registered into the existing `virtualmachine_mutator_suite_test.go`, rather than extending the legacy files. |
| E2E in the same PR for Supervisor-observable changes | **Deviation** | See Complexity tracking. |
| Sub-modules table in `constitution.md` | **Needs amendment** | `external/kubevm` is missing from the table, and the repo-layout section calls `external/` "vendored external API types", which is wrong for first-party code — the root `Makefile` already says so at lines 322–325 when it adds `./external/kubevm/` back into `GO_MOD_DIRS_TO_LINT`. Both need amending in the implementing PR, along with the new controller module. Not done here; this change set is limited to `spec.md` and `plan.md`. |
| No internal ticket or wiki URLs | OK | `vmop-NNN` form only. |

## Key decisions

**1. The status contract is fixed paths the provider surfaces, not paths the core translates.** Cluster API works this way: `sigs.k8s.io/cluster-api@v1.11.0/internal/contract/` defines paths such as `{"status","ready"}` and `{"status","failureReason"}`, and CAPI core reads those exact paths on any infrastructure object with no per-provider adapter anywhere. The generic core does the same. Consequence: VM Operator has to publish at the contract paths rather than the core learning VM Operator's names. The one-pager's phrase "a thin status adapter" is what this decision moves — the adapter lives on the provider side, as fields, not in the core as code.

**2. VM Operator therefore gains three additive status fields.** `status.addresses`, `status.providerID` and `status.providerMetadata` on `v1alpha6.VirtualMachineStatus`, all `+optional`. They duplicate information already reported under VM Operator's own names, which is a real cost, and they are written unconditionally rather than behind the gate — a contract is not something a provider satisfies only sometimes. This is the scope change against an earlier reading of this work as "no new fields", and it is recorded in the spec as such.

**3. This limitation goes on the record.** Requiring contract status paths assumes the provider owns the provider object's type. A provider that reuses a third-party CRD can put the linkage annotation on it but cannot meaningfully write its status, so such a provider needs its own thin object to stand in front of the borrowed one. VM Operator owns its CRD, so this does not block the port.

**4. Linkage requires mutual declaration.** The generic object's `spec.infrastructureRef` names the provider object (`+required`, and immutable via a `self == oldSelf` CEL rule). The provider object carries annotation `kube-vm.io/virtual-machine: <generic object name>`, same namespace. Adoption happens only when both agree. A one-sided reference produces a status message and no write of any kind. This costs one line of YAML and buys the guarantee that no generic object can conscript a provider object whose owner did not opt in.

**5. The owner reference is a consequence of adoption, not its signal.** The core sets `controllerutil.SetControllerReference` after confirming mutual linkage, for garbage collection, and refuses to adopt when the provider object already carries a controller owner reference naming a different generic object. The precedent is `controllers/virtualmachinegroup/virtualmachinegroup_controller.go`: the member VM must carry `spec.groupName` matching the group (guard at lines 406–423, returning `NoRequeueError` with condition reason `NotMember`) *and* be reachable from the group's own membership before line 428 sets an owner reference. Two corrections to the design as briefed: the group-side list is `spec.bootOrder[].members`, not `spec.members`, and the call is `controllerutil.SetOwnerReference` — a plain owner reference, because a VM may be in a group and owned by something else. Our case wants the controller slot; the two coexist, since an object may have one controller reference and any number of plain ones.

**6. Immutable-on-update fields are resolved once, in VM Operator's mutating webhook.** `validateImmutableFields` (`webhooks/virtualmachine/validation/virtualmachine_validator.go:2486`) makes `spec.storageClass` unconditionally immutable, `spec.image`/`spec.imageName` immutable via `validateImageOnUpdate` (:268), and `spec.className` immutable via `validateClassOnUpdate` (:722). Resolving these post-create is therefore impossible, so they are resolved at create. The mutating webhook is the right place because mutating webhooks run first, so `validateImageOnCreate` (:607) and `validateClassOnCreate` (:652) see a fully populated spec and no validation change is needed. One nuance the design as briefed does not capture: `validateClassOnUpdate` only enforces className immutability when neither `VMResize` nor `VMResizeCPUMemory` is enabled (:726–730), so class is not universally immutable. The plan still resolves it at create, because the demo cannot depend on a resize gate being on.

**7. The mutable field is reasserted by the controller, every reconcile.** `spec.powerState` is copied from the generic object into the provider object on each reconcile, exactly as `updateMemberPowerState` (`virtualmachinegroup_controller.go:840`, assignment at :850) copies a group's power state onto its members, persisted with `client.MergeFrom` at :426. This is what makes precedence real: the generic object wins because the controller keeps saying so, and a hand-patch of the provider object's power state reverts on the next reconcile.

**8. Values are persisted in the provider spec, not resolved in memory.** `kubectl get virtualmachines.vmoperator.vmware.com -o yaml` has to be truthful about what the VM was built from, and other tooling reads the object rather than recomputing it.

**9. Ordering inside `Mutate()` is explicit, not a registered hook.** `MutateOnCreateFuncs` is a `sync.Map` (`virtualmachine_mutator.go:112`) iterated with `Range` at :306, and `sync.Map.Range` order is unspecified. Resolution must run before `SetDefaultPowerState` (:275, which defaults an empty power state to `PoweredOn`) and before `ResolveImageNameOnCreate` (:283, which resolves `spec.imageName` into `spec.image` and would otherwise deny a spec that has neither). So it is a direct call placed first in the `admissionv1.Create` branch, not a `MutateOnCreateFuncs.Store` entry.

**10. Cross-group reads in the webhook bypass the cache by excluding the type.** `pkg/manager/manager.go` lines 107–127 already carry a `client.CacheOptions.DisableFor` list (`corev1.ConfigMap`, `corev1.Secret`, `appsv1.Deployment`); the generic `VirtualMachine` is added to it. Without that, the first admission request would lazily start an informer during a live call whose webhook has `failurePolicy: Fail`, and that informer never syncs if the CRD is absent. `mgr.GetAPIReader()` is the alternative and is used in three controllers (no webhooks today), but it would mean threading a second reader through `NewMutator(client)` and every test that constructs it. `DisableFor` also narrows the webhook's RBAC to `get`, where a cached read would need `list` and `watch`.

**11. Provider objects are read as `unstructured`.** The core resolves the group and kind from `spec.infrastructureRef` and reads the contract paths out of an `unstructured.Unstructured`, importing no provider types. Version resolution uses the RESTMapper's preferred version for the group-kind. `[UNVERIFIED: the one-pager describes a contract version advertised through a well-known label on the provider CRD (line 319), and no label key constant exists anywhere in `external/kubevm`. Preferred-version resolution is a placeholder for this iteration and must be revisited when the contract version is defined.]`

**12. The core controller lives in a new module nested under `external/kubevm/`.** `external/kubevm/go.mod` has exactly two direct dependencies (`k8s.io/apimachinery v0.31.0`, `sigs.k8s.io/controller-runtime v0.19.0`) and a header comment stating the low floor is deliberate so that providers importing the API types are not forced up a Kubernetes version. Adding a manager means `k8s.io/api` and `k8s.io/client-go`, neither of which is present today even transitively. Nesting the controller as `external/kubevm/controller/` rather than beside it keeps it moving with the API when the module relocates to its own repository, which its README says is expected. The core module must never depend on the root module.

**13. Adoption of pre-existing VM Operator VMs is out of scope**, recorded as a non-goal with the intended future mechanism, not as an open question.

## Project structure

```
api/v1alpha6/virtualmachine_types.go                — status.addresses, status.providerID, status.providerMetadata;
                                                      VirtualMachineConditionUpToDate constant
api/v1alpha1..v1alpha5/virtualmachine_conversion.go — restore-annotation handling for the three new fields
api/v1alpha1..v1alpha5/zz_generated.conversion.go   — regenerated
api/v1alpha6/zz_generated.deepcopy.go               — regenerated
config/crd/bases/vmoperator.vmware.com_virtualmachines.yaml — regenerated
pkg/providers/vsphere/vmlifecycle/update_status.go  — publish the contract fields
pkg/config/config.go                                — FeatureStates.KubeVMProvider
pkg/config/env/env.go, pkg/config/env.go            — FSS_WCP_VMSERVICE_KUBEVM_PROVIDER wiring
pkg/kubevm/                                         — NEW: annotation constant, mutual-linkage predicate,
  link.go, mapping.go, *_test.go, kubevm_suite_test.go  owner-reference conflict check, the down-mappings
controllers/virtualmachine/kubevmlink/              — NEW: power-state reassertion, UpToDate condition
controllers/controllers.go                          — gate-conditional AddToManager
webhooks/virtualmachine/mutation/
  virtualmachine_mutator.go                           first call in the admissionv1.Create branch
  virtualmachine_mutator_kubevm.go                    NEW: ResolveKubeVMParentOnCreate
  virtualmachine_mutator_kubevm_test.go               NEW, registered into the existing suite file
webhooks/virtualmachine/validation/
  virtualmachine_validator.go                         annotation immutability inside validateAnnotation (:2636)
pkg/manager/manager.go                              — scheme registration in New(); CacheOptions.DisableFor entry
test/builder/fake.go                                — NewScheme() (:108) gains the kubevm group
config/crd/external-crds/kube-vm.io_virtualmachines.yaml + README.md — CRD for root integration tests
config/rbac/                                        — kube-vm.io/virtualmachines and .../status
Makefile, go.mod                                    — generate-external-manifests stanza, lint dirs, replace + require
hack/demo/kubevm/                                   — namespace pre-flight script, ordered demo manifests, runbook

external/kubevm/controller/                         — NEW module (.../external/kubevm/controller)
  go.mod, go.sum, main.go                             replace onto ../ ; never onto the root module
  internal/controller/virtualmachine/*.go             adoption, linkage, owner ref, finalizer, deletion ordering
  internal/contract/contract.go                       the fixed status paths, read from unstructured
  config/crd/, hack/tools/                            stub provider CRD and this module's envtest tooling
```

## API / CRD strategy

**Additive status fields on a shipped CRD.** `constitution.md` states that additive changes are not safe once an API has shipped, citing `kubernetes/kubernetes#111703` — a client that round-trips an object through an older schema drops fields it does not know. Three things reduce that exposure here, and none of them eliminates it, which is why this is recorded as a deviation rather than waved through. The fields live on the status subresource, whose only writer is VM Operator itself, always compiled against the current schema. They are `+optional` with `omitempty`, so an object that has never been through the new code is indistinguishable from one whose VM has no address yet. And the repo already has the machinery for the remaining case: `api/v1alpha5/virtualmachine_conversion.go` uses the Cluster API `utilconversion.MarshalData` / `UnmarshalData` restore-annotation pattern (`:213`, `:243`) precisely so that v1alpha6 fields with no v1alpha5 peer survive a round trip through an older served version. The three new fields are added to that restore path in each spoke. Expect `make generate-go-conversions` to need attention before it passes — `.sdd/specs/005-ipv6-template-funcs/plan.md` documents the same class of failure, where conversion-gen emits the partial `autoConvert_` function but withholds the public `Convert_` wrapper that a containing type's generated conversion still references by name.

Fields added to `v1alpha6.VirtualMachineStatus`:

| Field | Type | Source today |
|---|---|---|
| `addresses` | list of `{interface, type, address}`, `type` one of `InternalIP`/`ExternalIP`/`InternalDNS`/`ExternalDNS` | the same data that feeds `status.network.primaryIP4` in `updateGuestNetworkStatus` (`pkg/providers/vsphere/vmlifecycle/update_status.go:1201`, assignment at :1341) |
| `providerID` | `string` | `status.instanceUUID`, set in `reconcileStatusPlatform` (:431) from `MoVM.Summary.Config.InstanceUuid` |
| `providerMetadata` | `map[string]string` | `uniqueID` key from `status.uniqueID`, set in the same function from `MoVM.Self.Value` |

`status.addresses` is a new VM Operator-side struct that is shape-compatible with the generic API's own address type, not a reuse of it. `api/` is its own module with its own deliberately low dependency floor, and making it depend on `external/kubevm` would be a larger change than the three fields; the contract is a wire shape, not a shared Go type.

`status.powerState` already exists on VM Operator with the same field name and the same three values, so it is a contract path already satisfied and needs no work.

Note the tension with an existing plan in the codebase: `reconcileStatusPlatform` carries `// TODO(v1alpha6): Deprecate and migrate these fields to the Provider status`, and `status.provider` (`VirtualMachineProviderStatus`, `api/v1alpha6/virtualmachine_types.go:1408`) already exists as a home for provider-reported facts. The contract requires a fixed path that is identical across providers, so the new fields go at the top level of status, not under `status.provider`. Reconciling the two is carried as an open question in the spec.

One condition-type constant is added, `VirtualMachineConditionUpToDate = "UpToDate"`. Condition types are strings, not schema, so this is not a CRD change.

Down-mapping. The generic `ObjectReference` requires all three of `apiGroup`, `kind` and `name`, so a boot-disk image reference must name its group explicitly.

| Generic path | Provider path | Note |
|---|---|---|
| `spec.powerState` | `spec.powerState` | Identical enum values. No translation. |
| `spec.instanceType.name` | `spec.className` | `spec.instanceType.resources` is not mapped. |
| `spec.bootDisk.source.image.kind` | `spec.image.kind` | Must be `VirtualMachineImage` or `ClusterVirtualMachineImage`, else `validateImageOnCreate` (:644) rejects. |
| `spec.bootDisk.source.image.name` | `spec.image.name` | |
| `spec.bootDisk.storageClassName` | `spec.storageClass` | |

Contract paths the core reads on the provider object, and where each lands on the generic object:

| Contract path | Generic path |
|---|---|
| `status.addresses` | `status.addresses` |
| `status.powerState` | `status.powerState` |
| `status.providerID` | `status.providerID` |
| `status.providerMetadata` | `status.providerMetadata` |
| `status.conditions[type=Ready]` | `Ready` and `InfrastructureReady` conditions, plus the `status.ready` boolean |
| `status.conditions[type=UpToDate]` | `UpToDate` condition |

The core is the sole writer of the generic object's status.

## Controller / webhook impact

**Mutating webhook.** `ResolveKubeVMParentOnCreate(ctx, client, vm)` returns immediately unless the gate is on and the annotation is present. It Gets the named generic object, refuses if that object's `spec.infrastructureRef` does not name this VM in this namespace, then fills `spec.className`, `spec.image.kind`, `spec.image.name`, `spec.storageClass` and `spec.powerState` from the parent, only where the field is empty, so a user who set a value explicitly is not silently overwritten. It is called first in the `Create` branch of `Mutate()` (`virtualmachine_mutator.go:268`), ahead of the four existing create-time mutators.

Because this is a synchronous admission check with no retry, **the generic object must exist before the provider object is created.** A refusal is an `admission.Denied`, not a requeue. The demo therefore applies two manifests in order, and the runbook says so.

**Validating webhook.** `validateAnnotation` (`virtualmachine_validator.go:2636`) gains a rule making `kube-vm.io/virtual-machine` immutable once set. That function returns early for a privileged account (:2650), so a privileged caller can still change it; that matches the rest of the annotation rules and is not worked around.

**Status publishing.** `reconcileStatusPlatform` (`update_status.go:431`) sets `status.providerID` and `status.providerMetadata["uniqueID"]` from the values it already reads; `updateGuestNetworkStatus` (:1201) appends the `InternalIP` entry to `status.addresses` from the same source that produces `primaryIP4` at :1341. Neither is gated.

**New VM Operator reconciler.** `controllers/virtualmachine/kubevmlink`, `For(&vmopv1.VirtualMachine{})` with a `Watches` on the generic `VirtualMachine` mapping back to the provider object named by `spec.infrastructureRef`. Per reconcile: confirm the gate, confirm mutual linkage, reassert `spec.powerState`, and set the `UpToDate` condition false with reason `UnsupportedByProvider` when the parent's instance type, image reference or storage class no longer match what was persisted. It registers from `controllers/controllers.go` under the gate, following the existing `if pkgcfg.FromContext(ctx).Features.VMGroups` pattern at :81. Registering the watch only under the gate matters: `[UNVERIFIED: a Watches on a CRD that is not installed is expected to leave the informer unsynced and block manager start. Confirm empirically before relying on it — if it turns out to be tolerated, gating the watch is still the right default.]`

**Core controller.** `For` the generic `VirtualMachine`. Per reconcile: add finalizer `kube-vm.io/virtualmachine`, confirm mutual linkage, set the controller owner reference with an optimistic-locked patch skipped when unchanged, read the contract paths off the provider object, write the generic status, and requeue while waiting for an address. On delete: delete the provider object, wait until it is gone, then drop the finalizer. It never writes the provider object's spec and never reads it.

**Feature gate.** `FeatureStates.KubeVMProvider` in `pkg/config/config.go:191`, read from `FSS_WCP_VMSERVICE_KUBEVM_PROVIDER` via `setBool` in `pkg/config/env.go`. No entry in `pkg/config/default.go`, which only sets the five gates needing a non-zero default; `false` is the zero value and the wanted default.

**Requeue delay while waiting for an address.** VM Operator's existing value is `PoweredOnVMHasIPRequeueDelay`, defaulted to `10 * time.Second` at `pkg/config/default.go:47`. It is a `Config` field, not a `const`, and lives in the root module's `pkgcfg`, which the core module cannot import. The core declares its own constant with the same value and a comment naming the origin. That duplication is the price of the core not depending on the root module.

**RBAC.** Root manager: `get` on `kube-vm.io/virtualmachines` for the webhook, plus `list` and `watch` for the reconciler's cache-backed `Watches`. Core controller: full verbs on `kube-vm.io/virtualmachines` and its status, plus `get;list;watch;update;patch;delete` on the provider group, granted for the demo through a namespace-scoped ServiceAccount and Role.

## Test strategy

- **Unit** (`testlabels.Webhook` / `testlabels.Controller`, no infrastructure label): `pkg/kubevm` covers the down-mappings, the mutual-linkage predicate, and the owner-reference conflict check. `virtualmachine_mutator_kubevm_test.go` covers resolution with the gate off, with the annotation absent, with a one-sided reference, with a parent whose fields are partly empty, and with a user-set field that must not be overwritten. `update_status.go`'s existing tests gain assertions for the three contract fields. New coverage in the mutation package goes into a `_test.go` file registered into the existing `virtualmachine_mutator_suite_test.go`, not into its legacy `_unit_test.go` / `_intg_test.go` pair.
- **Conversion**: round-trip tests for the three new status fields through each spoke version, extending the existing per-version `conversion_test.go` files in the `api/test` module, which drive `fuzztests.FuzzTestFuncInput` from `api/test/utilconversion/fuzztests`.
- **Integration** (`testlabels.EnvTest`): the new reconciler's suite exercises power-state reassertion, revert-on-hand-patch, and `UpToDate=False` on an unsupported edit. Root integration tests load `config/crd/bases` as parsed objects and `config/crd/external-crds` as a directory (`test/builder/test_suite.go:302` and :311), so the generic CRD must be present in the latter for any of this to run.
- **Integration, core module**: net-new harness. There is no `setup-envtest` target in the root Makefile; envtest binaries come from `hack/tools/Makefile` and `KUBEBUILDER_ASSETS` is exported once at `Makefile:74` pointing at `hack/tools/bin/$(GOHOSTOSARCH)`. The root harness is in the root module and is not importable from `external/kubevm/controller`, so that module needs its own tools directory, its own suite bootstrap, and a stub provider CRD — the core is duck-typed, so the stub only needs the contract status paths and an annotation.
- **E2E**: none in this change set. Explicitly scoped as a follow-up and stated in the PR summary, which `e2e-sync-with-changes.md` permits. See Complexity tracking.
- **Demo evidence**: a recorded run of the acceptance criteria in `spec.md`, driven by scripts under `hack/demo/kubevm/`. Because both CRDs claim short name `vm` — `api/v1alpha6/virtualmachine_types.go:1561` uses `shortName=vm`, `external/kubevm/api/v1alpha1/virtualmachine_types.go:660` uses `shortName=vm;vms` — every demo command uses fully qualified resource names.

## Rollout / migration

- **Feature flag**: `pkgcfg.Features.KubeVMProvider`, default `false`. With the gate off, the resolution step returns immediately and the reconciler is not registered, so an empty provider spec is rejected by `validateImageOnCreate` / `validateClassOnCreate` exactly as today. The three contract status fields are populated regardless of the gate; that is the one behaviour change that ships enabled.
- **Removal criteria**: the gate stays until the generic API has a decided permanent home, adoption of pre-existing VMs works, and the duplication between the contract fields and VM Operator's own names is resolved. A follow-up spec covers each.
- **Schema upgrade / backfill**: none needed. The new status fields populate on the next reconcile of each VM; a VM that has not reconciled since the upgrade simply has them absent, which is indistinguishable from a VM with no address yet.
- **Partner comms**: the release note calls out the three new status fields, since anything reading VM Operator status sees them appear. The generic API itself is a strawman with no external consumers. `external/kubevm/README.md` records that its module path will change when it moves, and root taking a dependency on it makes that move a migration rather than a mechanical rename; that trade-off should be noted in the README in the implementing PR.

## Risks

| # | Risk | Handling |
|---|---|---|
| 1 | Both APIs default `powerState` to `PoweredOn` — the generic side via `+kubebuilder:default=PoweredOn`, VM Operator via `SetDefaultPowerState` (`virtualmachine_mutator.go:672`). Absence cannot signal delegation, and a defaulted value is indistinguishable from a user-set one. | Delegation is signalled by the annotation and the mutual reference, never by a field being empty. The controller reasserts unconditionally once linkage is confirmed, so which side originated the value stops mattering after the first reconcile. |
| 2 | Post-create edits to instance type, image or storage class cannot be applied. | Report `UpToDate=False` with reason `UnsupportedByProvider` and keep reconciling. Never halt, never make the generic fields immutable to compensate. |
| 3 | Conversion of the three new status fields to older served versions loses them, or breaks `make generate-go-conversions`. | Add them to the `utilconversion` restore path in each spoke's `virtualmachine_conversion.go` — all five of `api/v1alpha1`–`api/v1alpha5` have that file and already call both `MarshalData` and `UnmarshalData` — with round-trip tests. Budget for the codegen failure described in the 005 plan. |
| 4 | A cross-group Get inside an admission webhook with a cache-backed client starts an informer during a live request under `failurePolicy: Fail`. | `client.CacheOptions.DisableFor` in `pkg/manager/manager.go`, with RBAC derived from that choice. |
| 5 | `sync.Map.Range` ordering would make a registered mutation hook run at an unpredictable point relative to the four existing create-time mutators. | Explicit call, first in the `Create` branch. |
| 6 | Owner references are a shared list and the VMGroup controller is a second writer (`virtualmachinegroup_controller.go:428`); a plain merge patch replaces list fields wholesale with no conflict detection. | `client.MergeFromWithOptimisticLock` plus a skip-if-unchanged guard using `apiequality.Semantic.DeepEqual`. |
| 7 | A VM that is both a group member and kubevm-owned would have two controllers writing `spec.powerState`. | Out of scope. The reconciler refuses to engage and reports it when both `spec.groupName` and the kubevm annotation are set. |
| 8 | The provider object is deleted directly. | The core never recreates it; it reports not-adopted and waits. Intended behaviour, and an acceptance criterion. |
| 9 | Applying both manifests at once fails, because resolution is a synchronous admission check. | Two ordered manifests, generic object first, stated in the runbook. |
| 10 | Scheme registration missed. | Two places need it: `pkg/manager/manager.go` `New()` (:60–97) and `test/builder/fake.go` `NewScheme()` (:108). `test/builder/test_suite.go` needs no change — it builds a bare `runtime.NewScheme()` at :405 and hands it to `pkgmgr.New`, which does the registration. |
| 11 | The core module's envtest harness is net-new and easy to under-build. | Own `hack/tools`, own `setup-envtest`, own suite bootstrap, stub provider CRD. Its own task. |
| 12 | The generic CRD is generated into `external/kubevm/config/crd/bases/` by the sub-module's own Makefile and is absent from `config/crd/external-crds/`, so root integration tests cannot see it. | New stanza in root `generate-external-manifests`, plus an entry in `config/crd/external-crds/README.md` under "Integration Test CRDs". The root stanzas use `paths=github.com/vmware-tanzu/vm-operator/external/<x>/...`, which only resolves once the module is in root `go.mod`. |
| 13 | CI does not cover the new module. `GO_MOD_DIRS_TO_LINT` already adds `./external/kubevm/` (Makefile:322–325), but no GitHub workflow iterates sub-modules — `ci.yml`'s `build-image` job matrices over `.` and `test/e2e` only, and its `test` job matrices over packages within the root module. | Add the controller module to `GO_MOD_DIRS_TO_LINT` and add a `ci.yml` job that builds and tests it. Without the second, the module is unverified in CI. |
| 14 | `kubectl get vm` is ambiguous once both CRDs are installed. | Fully qualified resource names in every demo command and script. |
| 15 | **Demo environment. The largest risk, and a gate on the work.** Needs a Supervisor where a CRD can be installed, the gate can be enabled, and the core controller can run out-of-cluster against a namespace that already has a working VM class, ready image and associated storage class. | Confirm the environment before any demo-recording task starts; the spec carries this as a `[NEEDS CLARIFICATION]`. `vcsim` is not a fallback: it is root-module in-process Ginkgo, cannot be driven with kubectl, and its VMs run no guest, so no address is ever reported. |
| 16 | A namespace that looks usable but is not, since most of these failures are asynchronous rather than at admission. | A pre-flight script. Verified: storage-class-to-namespace association *is* an admission check (`validateStorageFields`, :756, via `spqutil.IsStoragePolicyInNamespace`); image-name resolvability *is* checked, in the mutating webhook (`ResolveImageNameOnCreate`, :751, via `vmopv1util.ResolveImageName`, which resolves both namespaced `VirtualMachineImage` and cluster-scoped `ClusterVirtualMachineImage` and records the kind). But VM class existence is *not* — it surfaces later as `VirtualMachineClassReady=False`. Image readiness likewise surfaces as `VirtualMachineImageReady=False`, and default-network resolution happens in `AddDefaultNetworkInterface` (:567). The pre-flight must Get the class and check image readiness itself. |
| 17 | Running the core controller with a cluster-admin kubeconfig makes it a privileged account. Twelve validator call sites branch on `ctx.IsPrivilegedAccount`, several of which *remove* checks a DevOps user would hit. | Prefer a namespace-scoped ServiceAccount; otherwise state the caveat on the recording. Carried as a `[NEEDS CLARIFICATION]` in the spec. |

## Sequencing

An ordered list a later agent can turn into `tasks.md`. Phase boundaries are where a reviewer can stop and see something work.

**Phase 1 — Setup.** Confirm the demo environment (blocks Phase 5 only). Add `replace` and `require` for `external/kubevm` to the root `go.mod`. Register the generic scheme in `pkg/manager/manager.go` `New()` and in `test/builder/fake.go` `NewScheme()`, and add the type to `client.CacheOptions.DisableFor`. Add a kubevm stanza to root `generate-external-manifests`, copy the CRD into `config/crd/external-crds/`, and document it in that directory's README. Delete the orphan `+kubebuilder:validation:Enum=Pending;Provisioning;...` marker at `external/kubevm/api/v1alpha1/common_types.go:82`, which is attached to no type and generates nothing.

**Phase 2 — Contract status fields.** Add `status.addresses`, `status.providerID` and `status.providerMetadata` to `v1alpha6.VirtualMachineStatus` plus the `VirtualMachineConditionUpToDate` constant; regenerate deepcopy, conversions and CRD manifests; add the three fields to each spoke's conversion restore path with round-trip tests; populate them in `reconcileStatusPlatform` and `updateGuestNetworkStatus` with unit-test assertions. This phase stands alone and can merge before anything else in this spec.

**Phase 3 — Foundational.** Add `FeatureStates.KubeVMProvider`, its `env.VarName`, and its `setBool` wiring. Create `pkg/kubevm` with the annotation constant, the mutual-linkage predicate, the owner-reference conflict check, and the down-mappings, with unit tests. Add RBAC markers.

**Phase 4 — VM Operator side.** Add `ResolveKubeVMParentOnCreate` and call it first in the `Create` branch of `Mutate()`, with tests in a new file registered into the existing mutation suite. Add annotation immutability to `validateAnnotation`, with tests. Add the `controllers/virtualmachine/kubevmlink` reconciler — power-state reassertion, `UpToDate` reporting, group-member refusal — register it under the gate in `controllers/controllers.go`, and cover it with unit and envtest specs. At the end of this phase, a hand-created generic object plus an empty-spec provider object produces a working VM, with nothing on the generic side yet doing adoption.

**Phase 5 — Core controller.** Create the `external/kubevm/controller` module: `go.mod` with a `replace` onto the API module, `main.go`, `internal/contract` with the fixed paths, and the reconciler (finalizer, mutual-linkage check, owner reference, status from contract paths, requeue-for-address, ordered deletion). Build its envtest harness. Add the module to `GO_MOD_DIRS_TO_LINT` and to `ci.yml`. At the end of this phase the full lifecycle works end to end.

**Phase 6 — Demo.** Write the namespace pre-flight script (class present; image present, `Ready`, and its kind recorded; storage class associated with the namespace; default network resolvable; relevant gates on). Write the ordered demo manifests and the runbook, using fully qualified resource names. Record the demo: create, address, power cycle, delete.

**Phase Final — Polish.** Amend `constitution.md`: add `external/kubevm` and `external/kubevm/controller` to the sub-modules table, and correct the repo-layout description of `external/` so it no longer claims the directory holds only vendored API definitions. Note the module-path move in `external/kubevm/README.md`. File follow-up specs for E2E coverage, for resolving the duplication between the contract status fields and VM Operator's own names, and for adoption of pre-existing VMs.

## Complexity tracking

| Violation | Why needed | Simpler alternative rejected because |
|-----------|------------|--------------------------------------|
| `constitution.md` "API compatibility": three optional status fields are added to a shipped CRD, where the rule is that additive changes are not safe once an API has shipped. | The generic core must read one fixed set of paths for every provider, or it stops being generic. Cluster API's `internal/contract` package works exactly this way, with no per-provider adapter in core. VM Operator reports the same facts today under its own names, so the only way to satisfy the contract without putting VM Operator knowledge into the core is to publish at the contract paths. | Teaching the core to read `status.network.primaryIP4`, `status.instanceUUID` and `status.uniqueID` was rejected: it puts one provider's field names into the component whose whole purpose is not to have them, and every future provider would add another branch. Having VM Operator write the generic object's status directly was rejected because it creates a second writer on a subresource the core also owns, and it moves the adapter into a controller instead of into fields. A new API version was rejected as disproportionate for three optional, controller-written status fields on a version that is still the development hub, and the repo's existing `utilconversion` restore-annotation pattern already covers the round-trip case the rule is protecting against. |
| `operator-best-practices.md` anti-pattern 2, "controllers should only modify Status": the VM Operator reconciler writes `spec.powerState` on the provider object, and the mutating webhook writes four more spec fields. | The delegated values have to be visible on the provider object; `kubectl get -o yaml` must be truthful about what the VM was built from, and other tooling reads the object rather than recomputing it. Three of the five fields are immutable after create in VM Operator, so there is no post-create window in which to apply them. The precedent already exists and was accepted for the same reason: `updateMemberPowerState` (`virtualmachinegroup_controller.go:840`) assigns `obj.Spec.PowerState = group.Spec.PowerState` at :850 and persists it with `client.MergeFrom` at :426. | Resolving the values in memory on the provider's read path was rejected: the object on the API server would then disagree with the VM that was actually built, which is the failure mode that makes a delegated API hard to debug. Putting the values only on the generic object and teaching every VM Operator read path to consult the parent was rejected as a far larger blast radius than a webhook plus one reconciler. |
| `constitution.md` "Testing": E2E coverage does not ship in the same PR. | The delegation behaviour is gated off, so the only thing observable on a Supervisor at merge is three newly populated status fields, whose correctness is covered by unit and conversion tests. `e2e-sync-with-changes.md` permits deferral when the team explicitly scopes E2E as follow-up and says so in the PR summary, which this plan does. E2E for the gated behaviour also needs a testbed with the generic CRD installed and the gate on, which no CI testbed has today. | Writing E2E specs that skip unconditionally on every current testbed was rejected: an always-skipped spec is not coverage and would be rewritten once a capable testbed exists. Holding the whole change set until a testbed exists was rejected because it blocks the demo that is the point of the spec. |
| `constitution.md` "Webhooks": annotation immutability is enforced in Go rather than CEL. | The rule is "immutable once non-empty" on one key inside `metadata.annotations`, which CEL expresses awkwardly, and `validateAnnotation` (:2636) already owns annotation rules. | A CEL transition rule on the annotation map was rejected because it would also have to permit every other annotation to change freely, and splitting annotation validation across CEL and Go makes the next annotation rule harder to place. |
