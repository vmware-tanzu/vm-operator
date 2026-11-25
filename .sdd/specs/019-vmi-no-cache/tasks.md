# Tasks: Reduce controller manager memory at image scale

Format: `[T###] [P?] [vmop-NNN?] Description (paths)`. Task tickets are not yet assigned (see the open question in `spec.md`). Tasks that ship code need a `[vmop-NNN]` tag before merge.

## Phase 1: Setup and gates

- [ ] T001 Confirm the Supervisor Kubernetes version is 1.30 or newer. Blocks T004 to T008.
- [ ] T002 Find which build the heap profile came from, and take a baseline heap and goroutine profile on the scale testbed (`hack/scale-repro/`). Do not delete `scale-ns-*` or `scale-lib`.
- [ ] T003 [P] Get a decision on the image metric labels (spec open question). Blocks T016.

## Phase 2: Stop caching VirtualMachineImage (user story: cache removal)

- [ ] T004 Add selectable field markers for `.status.name` and `.status.providerItemID` on `VirtualMachineImage`, and `.status.name` on `ClusterVirtualMachineImage`. Run `make generate-manifests`. (`api/v1alpha6/virtualmachineimage_types.go`, `config/crd/bases/vmoperator.vmware.com_virtualmachineimages.yaml`, `config/crd/bases/vmoperator.vmware.com_clustervirtualmachineimages.yaml`)
- [ ] T005 Add `&vmopv1.VirtualMachineImage{}` to `DisableFor`, with an explanatory comment. (`pkg/manager/manager.go`)
- [ ] T006 [P] Remove the `VirtualMachineImage` `status.name` field index. Keep the `ClusterVirtualMachineImage` one. (`webhooks/virtualmachine/mutation/virtualmachine_mutator.go`)
- [ ] T007 [P] Remove the `VirtualMachineImage` watch and mapper from the publish request controller. Look up the image by `status.providerItemID`. Requeue while the image is not available. (`controllers/virtualmachinepublishrequest/virtualmachinepublishrequest_controller.go`)
- [ ] T008 [P] Register the `status.providerItemID` index (and check `status.name`) for `VirtualMachineImage` on the fake client. (`test/builder/fake.go`)
- [ ] T009 Update unit and envtest tests: publish request suite, `ResolveImageName` with a real API server, and a test that an undeclared field selector is rejected. (`controllers/virtualmachinepublishrequest/`, `pkg/util/vmopv1/`, `webhooks/virtualmachine/mutation/`)
- [ ] T010 Add or update E2E: VM create with a friendly image name, and publish request reaches the image-available state. (`test/e2e/vmservice/`)
- [ ] T011 Run `make lint-go` and fix the findings.

## Phase 3: Soft memory limit

- [ ] T012 Add a helper that sets the Go soft memory limit to 85% of the cgroup limit, and does nothing when `GOMEMLIMIT` is set or no limit exists. Log the result. (`pkg/util/memlimit/memlimit.go`)
- [ ] T013 Call the helper from `main.go` before the manager starts. (`main.go`)
- [ ] T014 Unit tests for the helper: env override, cgroup v1 and v2, "max", no limit. (`pkg/util/memlimit/memlimit_test.go`)

## Phase 4: Content library item path

- [ ] T015 Check all callers of `syncVirtualMachineImageFastDeployVM` and `UpdateVmiWithVirtualMachine`. Then fetch only `config`. Add a vcsim test that the image status is unchanged. (`pkg/providers/vsphere/vmprovider.go`, `pkg/providers/vsphere/contentlibrary/content_library_utils.go`)
- [ ] T016 Remove or bound the `vmi_name` and `vmi_namespace` metric labels. Blocked on T003. (metrics registration; find with `git grep vmi_name`)
- [ ] T017 Set a lower concurrency for `contentlibraryitem-controller` only. Check the flag syntax first. Use the numbers from T002 and T019. (`config/default/manager_max_concurrent_reconciles_patch.yaml`, `main.go`)

## Phase Final: Verify and polish

- [ ] T018 Take heap profiles on the scale testbed after T005 to T008, after T013, and after T015 to T017. Record the peak in `research.md`.
- [ ] T019 Take a goroutine profile in the OOM window to see which controllers have busy workers. Record the result in `research.md`.
- [ ] T020 Measure API server request rate from the content library item controller before and after T005.
- [ ] T021 Write the release note: API server reads, up to 10 seconds more publish delay, Kubernetes 1.30 or newer.
- [ ] T022 Set the spec `Status` to `Implemented` and fill in the epic ticket.
