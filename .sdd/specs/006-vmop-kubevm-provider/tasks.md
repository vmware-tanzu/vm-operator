# Tasks: VM Operator as a KubeVM provider

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Epic**: [TICKET NEEDED]

## Phase 1 — Demo environment and namespace pre-flight

- [ ] T001 [TICKET NEEDED] Confirm and record a Supervisor demo environment where a new CRD can be installed, `FSS_WCP_VMSERVICE_KUBEVM_PROVIDER` can be set, and an out-of-cluster controller can reach the API server, naming the cluster and namespace (`hack/demo/kubevm/README.md`)
- [ ] T002 [TICKET NEEDED] Decide whether the core controller runs under a namespace-scoped ServiceAccount or a cluster-admin kubeconfig for the demo, and record the decision and the RBAC it implies, resolving the second spec open question (`hack/demo/kubevm/README.md`)
- [ ] T003 [TICKET NEEDED] Write `preflight.sh` with a check that Gets the named `VirtualMachineClass` in the namespace and fails when it is absent, since class existence is not an admission check (`hack/demo/kubevm/preflight.sh`)
- [ ] T004 [TICKET NEEDED] Add a pre-flight check that resolves the image name to either a namespaced `VirtualMachineImage` or a cluster-scoped `ClusterVirtualMachineImage`, asserts its `Ready` condition is true, and prints the resolved kind (`hack/demo/kubevm/preflight.sh`)
- [ ] T005 [TICKET NEEDED] Add a pre-flight check that the named storage class has a storage policy associated with the namespace, matching what `validateStorageFields` enforces at admission (`hack/demo/kubevm/preflight.sh`)
- [ ] T006 [TICKET NEEDED] Add a pre-flight check that a default network is resolvable in the namespace for the configured network provider, matching what `AddDefaultNetworkInterface` does at admission (`hack/demo/kubevm/preflight.sh`)
- [ ] T007 [TICKET NEEDED] Add a pre-flight step that prints the enabled VM Operator feature gates read from the deployment environment (`hack/demo/kubevm/preflight.sh`)
- [ ] T008 [TICKET NEEDED] Run the pre-flight script against the demo namespace and record the resolved image kind in the environment notes, because the generic `bootDisk.source` is immutable and a wrong kind requires recreating the generic object (`hack/demo/kubevm/README.md`)

## Phase 2 — Contract status fields on `vmoperator.vmware.com/v1alpha6`

- [ ] T009 [TICKET NEEDED] Add a `VirtualMachineAddress` struct with `interface`, `type` and `address` fields and an `InternalIP;ExternalIP;InternalDNS;ExternalDNS` enum marker on `type` (`api/v1alpha6/virtualmachine_types.go`)
- [ ] T010 [TICKET NEEDED] Add optional `Addresses []VirtualMachineAddress`, `ProviderID string` and `ProviderMetadata map[string]string` fields to `VirtualMachineStatus` (`api/v1alpha6/virtualmachine_types.go`)
- [ ] T011 [TICKET NEEDED] Add the `VirtualMachineConditionUpToDate = "UpToDate"` condition-type constant (`api/v1alpha6/virtualmachine_types.go`)
- [ ] T012 [TICKET NEEDED] Run `make generate` and commit the regenerated deepcopy and CRD manifest (`api/v1alpha6/zz_generated.deepcopy.go`, `config/crd/bases/vmoperator.vmware.com_virtualmachines.yaml`)
- [ ] T013 [TICKET NEEDED] Run `make generate-go-conversions` and confirm each spoke's existing hand-written `Convert_v1alpha6_VirtualMachineStatus_To_v1alphaN_VirtualMachineStatus` wrapper still compiles against the regenerated `autoConvert_` function (`api/v1alpha1/virtualmachine_conversion.go`, `api/v1alpha2/virtualmachine_conversion.go`, `api/v1alpha3/virtualmachine_conversion.go`, `api/v1alpha4/virtualmachine_conversion.go`, `api/v1alpha5/virtualmachine_conversion.go`)
- [ ] T014 [TICKET NEEDED] Confirm each spoke's `ConvertTo` restores the three fields through its existing `dst.Status = restored.Status` assignment and record that no per-field restore code is needed (`api/v1alpha1/virtualmachine_conversion.go`, `api/v1alpha2/virtualmachine_conversion.go`, `api/v1alpha3/virtualmachine_conversion.go`, `api/v1alpha4/virtualmachine_conversion.go`, `api/v1alpha5/virtualmachine_conversion.go`)
- [ ] T015 [TICKET NEEDED] Run each spoke's whole-object `SpokeHubSpoke` and `HubSpokeHub` fuzz round-trip and confirm the three new status fields survive, adding a fuzzer override only if one fails (`api/test/v1alpha1/conversion_test.go`, `api/test/v1alpha2/conversion_test.go`, `api/test/v1alpha3/conversion_test.go`, `api/test/v1alpha4/conversion_test.go`, `api/test/v1alpha5/conversion_test.go`)
- [ ] T016 [TICKET NEEDED] Set `status.providerID` from `MoVM.Summary.Config.InstanceUuid` and `status.providerMetadata["uniqueID"]` from `MoVM.Self.Value` in `reconcileStatusPlatform`, ungated (`pkg/providers/vsphere/vmlifecycle/update_status.go`)
- [ ] T017 [TICKET NEEDED] Populate a single `InternalIP` entry in `status.addresses` in `updateGuestNetworkStatus` from the same value that produces `status.network.primaryIP4`, ungated (`pkg/providers/vsphere/vmlifecycle/update_status.go`)
- [ ] T018 [TICKET NEEDED] Add unit assertions for `status.providerID`, `status.providerMetadata` and `status.addresses` to the existing status tests (`pkg/providers/vsphere/vmlifecycle/update_status_test.go`)
- [ ] T019 [TICKET NEEDED] Run `make verify-codegen`, `make lint-go-full` and the `api` and `vmprovider-vmlifecycle` test targets on this phase alone to confirm it merges independently (`Makefile`)

## Phase 3 — Feature gate, scheme registration and module wiring

- [ ] T020 [TICKET NEEDED] Add `KubeVMProvider bool` to `FeatureStates` with no entry in `default.go`, so the zero value is the default (`pkg/config/config.go`)
- [ ] T021 [TICKET NEEDED] Add the `FSS_WCP_VMSERVICE_KUBEVM_PROVIDER` variable name (`pkg/config/env/env.go`)
- [ ] T022 [TICKET NEEDED] Wire the variable to `Features.KubeVMProvider` with `setBool` (`pkg/config/env.go`)
- [ ] T023 [TICKET NEEDED] Add a test asserting the new variable name and its default-off behaviour (`pkg/config/env/env_test.go`)
- [ ] T024 [TICKET NEEDED] Add the `replace` and `require` entries for `github.com/vmware-tanzu/vm-operator/external/kubevm` using the zero-pseudo-version pattern, in the same change as the first import so `go mod tidy` does not drop them (`go.mod`)
- [ ] T025 [TICKET NEEDED] Register `kubevmv1a1.AddToScheme` in `pkg/manager/manager.go` `New()` and add the generic `VirtualMachine` to `client.CacheOptions.DisableFor` under the same condition, because `newClient` resolves a GVK for every `DisableFor` entry and errors on a type absent from the scheme (`pkg/manager/manager.go`)
- [ ] T026 [TICKET NEEDED] Confirm the root integration suite starts the manager with the gate both off and on, exercising the scheme and `DisableFor` coupling through the existing `pkgmgr.New` call (`test/builder/test_suite.go`)
- [ ] T027 [TICKET NEEDED] Add the kubevm group to `NewScheme()` (`test/builder/fake.go`)
- [ ] T028 [TICKET NEEDED] Add the generic `VirtualMachine` to `KnownObjectTypes()` so the fake client enforces the spec/status split on it (`test/builder/fake.go`)
- [ ] T029 [TICKET NEEDED] Add a `generate-external-manifests` stanza with `paths=github.com/vmware-tanzu/vm-operator/external/kubevm/...` (`Makefile`)
- [ ] T030 [TICKET NEEDED] Run the new stanza and commit the generated generic CRD (`config/crd/external-crds/kube-vm.io_virtualmachines.yaml`)
- [ ] T031 [TICKET NEEDED] Add the generic CRD to the "Integration Test CRDs" list (`config/crd/external-crds/README.md`)
- [ ] T032 [TICKET NEEDED] Add RBAC markers for `get` on `kube-vm.io/virtualmachines` for the webhook and `list;watch` for the reconciler, and regenerate the role (`config/rbac/role.yaml`)

## Phase 4 — `pkg/kubevm` helpers

- [ ] T033 [TICKET NEEDED] Add the `kube-vm.io/virtual-machine` annotation key constant (`pkg/kubevm/link.go`)
- [ ] T034 [TICKET NEEDED] Add a mutual-linkage predicate that returns true only when the provider object's annotation names the generic object and the generic object's `spec.infrastructureRef` names the provider object by group, kind and name in the same namespace (`pkg/kubevm/link.go`)
- [ ] T035 [TICKET NEEDED] Add an owner-reference conflict check that reports when the provider object already carries a controller owner reference naming a different generic object (`pkg/kubevm/link.go`)
- [ ] T036 [TICKET NEEDED] Add the down-mapping functions for `instanceType.name` to `className`, `bootDisk.source.image` kind and name to `spec.image`, `bootDisk.storageClassName` to `spec.storageClass`, and `powerState` to `spec.powerState` (`pkg/kubevm/mapping.go`)
- [ ] T037 [TICKET NEEDED] Add the suite bootstrap for the package (`pkg/kubevm/kubevm_suite_test.go`)
- [ ] T038 [TICKET NEEDED] Add unit tests for the linkage predicate, the conflict check and the down-mappings (`pkg/kubevm/kubevm_test.go`)

## Phase 5 — VM Operator side

- [ ] T039 [TICKET NEEDED] Spike: start a manager in envtest with a `Watches` on the generic `VirtualMachine` while the CRD is absent, observe whether the manager blocks on cache sync or starts, and record the result and whether the watch stays gate-conditional (`.sdd/specs/006-vmop-kubevm-provider/research.md`)
- [ ] T040 [TICKET NEEDED] Add `ResolveKubeVMParentOnCreate` that returns immediately unless the gate is on and the annotation is present, Gets the named generic object, refuses when its `spec.infrastructureRef` does not name this VM in this namespace, and otherwise fills `spec.className`, `spec.image.kind`, `spec.image.name`, `spec.storageClass` and `spec.powerState` only where empty (`webhooks/virtualmachine/mutation/virtualmachine_mutator_kubevm.go`)
- [ ] T041 [TICKET NEEDED] Call `ResolveKubeVMParentOnCreate` first in the `admissionv1.Create` branch of `Mutate()`, ahead of `AddDefaultNetworkInterface`, as a direct call rather than a `MutateOnCreateFuncs` entry (`webhooks/virtualmachine/mutation/virtualmachine_mutator.go`)
- [ ] T042 [TICKET NEEDED] Add unit tests for resolution with the gate off, the annotation absent, a one-sided reference, a parent with partly empty fields, and a user-set field that must not be overwritten, in a new file registered into the existing suite (`webhooks/virtualmachine/mutation/virtualmachine_mutator_kubevm_test.go`, `webhooks/virtualmachine/mutation/virtualmachine_mutator_suite_test.go`)
- [ ] T043 [TICKET NEEDED] Add a rule to `validateAnnotation` making `kube-vm.io/virtual-machine` immutable once set for non-privileged callers (`webhooks/virtualmachine/validation/virtualmachine_validator.go`)
- [ ] T044 [TICKET NEEDED] Add unit tests for the annotation immutability rule, including the privileged-account bypass, in a new file registered into the existing suite (`webhooks/virtualmachine/validation/virtualmachine_validator_kubevm_test.go`, `webhooks/virtualmachine/validation/virtualmachine_validator_suite_test.go`)
- [ ] T045 [TICKET NEEDED] Add the reconciler skeleton with `For(&vmopv1.VirtualMachine{})`, `AddToManager`, and a `Watches` on the generic `VirtualMachine` mapping to the provider object named by `spec.infrastructureRef` (`controllers/virtualmachine/kubevmlink/kubevmlink_controller.go`)
- [ ] T046 [TICKET NEEDED] Reassert `spec.powerState` from the generic object on every reconcile once mutual linkage is confirmed, persisted with `client.MergeFrom` (`controllers/virtualmachine/kubevmlink/kubevmlink_controller.go`)
- [ ] T047 [TICKET NEEDED] Set `UpToDate=False` with reason `UnsupportedByProvider` when the generic object's instance type, image reference or storage class no longer match the values persisted in the provider spec, and keep reconciling (`controllers/virtualmachine/kubevmlink/kubevmlink_controller.go`)
- [ ] T048 [TICKET NEEDED] Refuse to engage and report it when the VM has both `spec.groupName` and the kubevm annotation set (`controllers/virtualmachine/kubevmlink/kubevmlink_controller.go`)
- [ ] T049 [TICKET NEEDED] Register the reconciler under `Features.KubeVMProvider` (`controllers/controllers.go`)
- [ ] T050 [TICKET NEEDED] Add the suite bootstrap for the reconciler package (`controllers/virtualmachine/kubevmlink/kubevmlink_controller_suite_test.go`)
- [ ] T051 [TICKET NEEDED] Add unit tests labelled `testlabels.Controller` for gate-off, one-sided linkage, power-state reassertion and the group-member refusal (`controllers/virtualmachine/kubevmlink/kubevmlink_controller_test.go`)
- [ ] T052 [TICKET NEEDED] Add integration specs labelled `testlabels.EnvTest` for power-state reassertion, revert of a hand-patched provider power state, and `UpToDate=False` on an unsupported edit (`controllers/virtualmachine/kubevmlink/kubevmlink_controller_test.go`)

## Phase 6 — Core controller module

- [ ] T053 [TICKET NEEDED] Spike: search `external/kubevm` for a contract-version label key, check what the one-pager specifies, and record whether the core resolves the provider version from a CRD label or from the RESTMapper's preferred version in this iteration (`.sdd/specs/006-vmop-kubevm-provider/research.md`)
- [ ] T054 [TICKET NEEDED] Create the module with a `replace` onto `../` and no dependency on the root module (`external/kubevm/controller/go.mod`, `external/kubevm/controller/go.sum`)
- [ ] T055 [TICKET NEEDED] Add the fixed contract status paths and unstructured readers for `status.addresses`, `status.powerState`, `status.providerID`, `status.providerMetadata` and the `Ready` and `UpToDate` conditions (`external/kubevm/controller/internal/contract/contract.go`)
- [ ] T056 [TICKET NEEDED] Add unit tests for the contract readers against an `unstructured.Unstructured` fixture (`external/kubevm/controller/internal/contract/contract_test.go`)
- [ ] T057 [TICKET NEEDED] Add `main.go` that builds a manager, registers the generic scheme, and starts the reconciler (`external/kubevm/controller/main.go`)
- [ ] T058 [TICKET NEEDED] Add the reconciler with `For` the generic `VirtualMachine` and the `kube-vm.io/virtualmachine` finalizer (`external/kubevm/controller/internal/controller/virtualmachine/virtualmachine_controller.go`)
- [ ] T059 [TICKET NEEDED] Resolve the provider object's group-version-kind from `spec.infrastructureRef` using the decision recorded in T053 and Get it as `unstructured`, never creating it (`external/kubevm/controller/internal/controller/virtualmachine/virtualmachine_controller.go`)
- [ ] T060 [TICKET NEEDED] Confirm mutual linkage and refuse adoption when the provider object already carries a controller owner reference naming a different generic object (`external/kubevm/controller/internal/controller/virtualmachine/virtualmachine_controller.go`)
- [ ] T061 [TICKET NEEDED] Set the controller owner reference with `client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})`, skipped when `apiequality.Semantic.DeepEqual` reports the list unchanged (`external/kubevm/controller/internal/controller/virtualmachine/virtualmachine_controller.go`)
- [ ] T062 [TICKET NEEDED] Write the generic status from the contract paths, including `status.ready`, the `Ready` and `InfrastructureReady` conditions, `UpToDate`, and `observedGeneration` (`external/kubevm/controller/internal/controller/virtualmachine/virtualmachine_controller.go`)
- [ ] T063 [TICKET NEEDED] Add a package constant of 10 seconds for the requeue delay while waiting for an address, with a comment naming `PoweredOnVMHasIPRequeueDelay` as its origin (`external/kubevm/controller/internal/controller/virtualmachine/virtualmachine_controller.go`)
- [ ] T064 [TICKET NEEDED] Delete the provider object on generic-object deletion and drop the finalizer only after the provider object is gone (`external/kubevm/controller/internal/controller/virtualmachine/virtualmachine_controller.go`)
- [ ] T065 [TICKET NEEDED] Add the module's own envtest tooling with a `setup-envtest` target and `KUBEBUILDER_ASSETS` export (`external/kubevm/controller/hack/tools/Makefile`, `external/kubevm/controller/Makefile`)
- [ ] T066 [TICKET NEEDED] Add a stub provider CRD carrying only the contract status paths and the linkage annotation, for the module's envtest environment (`external/kubevm/controller/config/crd/stub-provider.yaml`)
- [ ] T067 [TICKET NEEDED] Add the module's suite bootstrap loading both the generic CRD and the stub provider CRD (`external/kubevm/controller/internal/controller/virtualmachine/virtualmachine_controller_suite_test.go`)
- [ ] T068 [TICKET NEEDED] Add integration specs labelled `testlabels.EnvTest` for adoption on mutual linkage, no write on a one-sided reference, status mirrored from the contract paths, refusal on an existing conflicting controller owner reference, no recreation when the provider object is deleted directly, and ordered deletion (`external/kubevm/controller/internal/controller/virtualmachine/virtualmachine_controller_test.go`)
- [ ] T069 [TICKET NEEDED] Add `./external/kubevm/controller/` to `GO_MOD_DIRS_TO_LINT`, which the `filter-out ./external%` rule otherwise excludes (`Makefile`)
- [ ] T070 [TICKET NEEDED] Add a CI job that builds, lints and tests the new module, since the existing `build-image` and `test` matrices cover only the root module and `test/e2e` (`.github/workflows/ci.yml`)

## Phase 7 — Demo

- [ ] T071 [TICKET NEEDED] Write the generic `VirtualMachine` manifest, applied first, using the image kind recorded in T008 (`hack/demo/kubevm/01-generic-vm.yaml`)
- [ ] T072 [TICKET NEEDED] Write the empty-spec provider `VirtualMachine` manifest carrying the back-reference annotation, applied second (`hack/demo/kubevm/02-provider-vm.yaml`)
- [ ] T073 [TICKET NEEDED] Write the RBAC manifests for the core controller's ServiceAccount as decided in T002 (`hack/demo/kubevm/rbac.yaml`)
- [ ] T074 [TICKET NEEDED] Write the runbook covering the ordered apply, why order matters, and fully qualified resource names throughout because both CRDs claim the `vm` short name (`hack/demo/kubevm/README.md`)
- [ ] T075 [TICKET NEEDED] Record the demo covering create, address reported, power off, power on, hand-edit reverted, unsupported edit reported, and delete (`hack/demo/kubevm/README.md`)

## Phase Final — Housekeeping

- [ ] T076 [TICKET NEEDED] Add `external/kubevm/` and `external/kubevm/controller/` to the sub-modules table (`.sdd/memory/constitution.md`)
- [ ] T077 [TICKET NEEDED] Amend the repository-layout section so `external/` is no longer described only as vendored API definitions (`.sdd/memory/constitution.md`)
- [ ] T078 [TICKET NEEDED] Note that the root module now depends on this module, so the expected move to its own repository becomes a migration rather than a rename (`external/kubevm/README.md`)
- [ ] T079 [TICKET NEEDED] File follow-up specs for E2E coverage, for resolving the duplication between the contract status fields and VM Operator's own identifier fields, and for adoption of pre-existing VM Operator VMs (`.sdd/specs/006-vmop-kubevm-provider/plan.md`)
- [ ] T080 [TICKET NEEDED] Update the module README with what the demo proves and what it does not — no E2E, no adoption of pre-existing VMs, no resize, four spec fields and four status fields only (`external/kubevm/README.md`)
