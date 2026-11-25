# Tasks: Reduce controller manager memory at image scale

Format: `[T###] [P?] [vmop-NNN?] Description (paths)`. Task tickets are not yet assigned (see the open question in `spec.md`). Tasks that ship code need a `[vmop-NNN]` tag before merge.

## Phase 1: Setup and gates

- [ ] T001 Confirm the Supervisor Kubernetes version is 1.31 or newer. Blocks T003 to T008.
- [ ] T002 Find which build the heap profile came from. Take a baseline heap profile on `main` at the same scale, and a goroutine profile in the OOM window to see which controllers have busy workers. Record both in `research.md`. Use the scale testbed (`hack/scale-repro/`). Do not delete `scale-ns-*` or `scale-lib`.

## Phase 2: Stop caching VirtualMachineImage (user story: cache removal)

- [ ] T003 Add selectable field markers for `.status.name` and `.status.providerItemID` on `VirtualMachineImage`, and `.status.name` on `ClusterVirtualMachineImage`. Run `make generate-manifests`. (`api/v1alpha6/virtualmachineimage_types.go`, `config/crd/bases/vmoperator.vmware.com_virtualmachineimages.yaml`, `config/crd/bases/vmoperator.vmware.com_clustervirtualmachineimages.yaml`)
- [ ] T004 After `initCRDs`, check that the image CRDs have the selectable fields. On failure, log a warning and use the in-memory filter fallback (pending the spec open question). (`main.go`, `pkg/crd/crd.go`, `pkg/util/vmopv1/vm.go`, `controllers/virtualmachinepublishrequest/virtualmachinepublishrequest_controller.go`)
- [ ] T005 Add `&vmopv1.VirtualMachineImage{}` to `DisableFor`, with an explanatory comment. (`pkg/manager/manager.go`)
- [ ] T006 [P] Remove the `VirtualMachineImage` `status.name` field index. Keep the `ClusterVirtualMachineImage` one. (`webhooks/virtualmachine/mutation/virtualmachine_mutator.go`)
- [ ] T007 [P] Remove the `VirtualMachineImage` watch and mapper from the publish request controller. Look up the image by `status.providerItemID`. The existing 10 second requeue stays as is. (`controllers/virtualmachinepublishrequest/virtualmachinepublishrequest_controller.go`)
- [ ] T008 [P] Register the `status.providerItemID` index for `VirtualMachineImage` on the fake client. (`test/builder/fake.go`)
- [ ] T009 Update unit and envtest tests: publish request suite, `ResolveImageName` with a real API server, a test that a declared field selector filters and an undeclared one is rejected, and the startup check and fallback. (`controllers/virtualmachinepublishrequest/`, `pkg/util/vmopv1/`, `webhooks/virtualmachine/mutation/`)
- [ ] T010 Add or update E2E: VM create with a friendly image name, and publish request reaches the image-available state. (`test/e2e/vmservice/`)
- [ ] T011 Run `make lint-go` and fix the findings.

## Phase 3: Soft memory limit

- [ ] T012 Add a helper that sets the Go soft memory limit to 85% of the cgroup limit, and does nothing when `GOMEMLIMIT` is set or no limit exists. Log the result. (`pkg/mem/limit.go`)
- [ ] T013 Call the helper from `initMemStats` in `main.go`, before the manager starts. (`main.go`)
- [ ] T014 Unit tests for the helper: env override, cgroup v1 and v2, "max", no limit. (`pkg/mem/limit_test.go`)

## Phase 4: Content library item path

- [ ] T015 Check all callers of `syncVirtualMachineImageFastDeployVM` and `UpdateVmiWithVirtualMachine`. Then fetch only `config`. Add a vcsim test that the image status is unchanged. (`pkg/providers/vsphere/vmprovider.go`, `pkg/providers/vsphere/contentlibrary/content_library_utils.go`)
- [ ] T016 Set a lower concurrency for `contentlibraryitem-controller` only. Use the `controller=value` form. Use the numbers from T002 and T018. (`config/default/manager_max_concurrent_reconciles_patch.yaml`, `main.go`)

## Phase Final: Verify and polish

- [ ] T017 Take heap profiles on the scale testbed after T005 to T008, after T013, and after T015 to T016. Record the peak in `research.md`.
- [ ] T018 Measure API server request rate from the content library item controller before and after T005.
- [ ] T019 Write the release note: API server reads, up to 10 seconds more publish delay, Kubernetes 1.31 or newer.
- [ ] T020 Set the spec `Status` to `Implemented` and fill in the epic ticket.
