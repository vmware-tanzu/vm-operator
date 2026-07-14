# Tasks: Automated Deployment from ISO Image

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Epic**: vmop-TBD

> Ticket filing is out of scope for this coding pass (no JIRA access from
> this environment). Every shipping-code task below is tagged `[vmop-TBD]`
> as a placeholder; before merge, file a story/sub-task per task under the
> spec's epic and replace the tag, per `sdd-standards.md`.

This decomposition covers `plan.md`'s framework phases 1-9 plus the Ubuntu
(US2) end-to-end walkthrough only. RHEL (US4) and Windows (US3) end-to-end
work is out of scope for this pass — the framework phases are OS-agnostic by
design, so no throwaway work is expected when those land later.

## Phase 1 — Setup

- [x] T001 Scaffold `.sdd/specs/003-auto-iso/tasks.md` (this file)

## Phase 2 — Foundational (API + feature flag)

- [x] T002 [US1] [vmop-TBD] Add `VirtualMachineBootstrapISOSpec` /
  `VirtualMachineBootstrapISOAsset` types and `ISO` field on
  `VirtualMachineBootstrapSpec` (`api/v1alpha6/virtualmachine_bootstrap_types.go`)
- [x] T003 [US1] [vmop-TBD] Add `VirtualMachineBootstrapISOSynced` condition
  constant and ISO-specific reason constants
  (`api/v1alpha6/virtualmachine_types.go`)
- [x] T004 [US1] [vmop-TBD] Add v1alpha5 (and any other spoke version with a
  `restore_v1alpha6_VirtualMachineBootstrapDisabled`-style function) restore
  function for `Spec.Bootstrap.ISO` (`api/v1alpha5/virtualmachine_conversion.go`,
  siblings)
- [x] T005 [vmop-TBD] Run `make generate-go` (deepcopy/conversion) and
  `make generate-manifests` (CRD YAML); hand-author the diffs if the
  generator cannot run in this environment
- [x] T006 [P] [vmop-TBD] Add `AutoISO` feature flag
  (`pkg/config/config.go`, `pkg/config/env/env.go`, `pkg/config/env.go`)

## Phase 3 — USB keyboard driver (framework)

- [x] T007 [US1] [vmop-TBD] HID scan-code table (`pkg/util/vsphere/keyboard/hid.go`)
- [x] T008 [US1] [vmop-TBD] Boot-command tokenizer + `TotalWait`
  (`pkg/util/vsphere/keyboard/command.go`)
- [x] T009 [US1] [vmop-TBD] `ScanCodeSender`/`SendCommands`
  (`pkg/util/vsphere/keyboard/send.go`)
- [x] T010 [P] [US1] [vmop-TBD] Unit tests for hid/command/send + one
  vcsim-backed test (`pkg/util/vsphere/keyboard/*_test.go`)

## Phase 4 — Ephemeral HTTP server (framework)

- [x] T011 [US1] [vmop-TBD] `isohttp.Manager` (`EnsureReady`/`Teardown`, Pod +
  Service `CreateOrPatch`, always `type: LoadBalancer`)
  (`pkg/util/kube/isohttp/manager.go`)
- [x] T012 [P] [US1] [vmop-TBD] Unit tests with fake `ctrlclient`
  (`pkg/util/kube/isohttp/manager_test.go`)
- [x] T013 [US1] [vmop-TBD] `iso-httpserver` server package + binary
  (`pkg/isohttpserver/server.go`, `cmd/iso-httpserver/main.go`)
- [x] T014 [P] [vmop-TBD] `cmd/iso-httpserver/Dockerfile` + `Makefile` targets
- [x] T015 [P] [vmop-TBD] Configurable image name
  (`pkg/config/config.go`/`default.go`, `VSPHERE_ISO_HTTP_SERVER_IMAGE`)

## Phase 5 — Template variable (framework)

- [x] T016 [US1] [vmop-TBD] `V1alpha6BootstrapService` constant
  (`pkg/providers/vsphere/constants/constants.go`)
- [x] T017 [US1] [vmop-TBD] `withBootstrapServiceFunc` helper, scoped to the
  ISO render path only (`pkg/providers/vsphere/vmlifecycle/bootstrap_templatedata.go`)

## Phase 6 — Bootstrap integration (framework)

- [x] T018 [US1] [vmop-TBD] `BootstrapISO` control flow + `ErrBootstrapISO`
  sentinel (`pkg/providers/vsphere/vmlifecycle/bootstrap_iso.go`)
- [x] T019 [US1] [vmop-TBD] `DoBootstrap` dispatch case + AutoISO flag gate
  (`pkg/providers/vsphere/vmlifecycle/bootstrap.go`)
- [x] T020 [US1] [vmop-TBD] Secrets RBAC marker
  (`controllers/virtualmachine/virtualmachine/virtualmachine_controller.go`)
- [x] T021 [P] [US1] [vmop-TBD] Unit tests for `BootstrapISO` control flow
  (fake `ScanCodeSender`/`ctrlclient`)
- [x] T022 [P] [US1] [vmop-TBD] vcsim/envtest integration test for
  `DoBootstrap` dispatch + hash idempotency

## Phase 7 — Webhook validation (framework)

- [x] T023 [US1] [vmop-TBD] Mutual exclusivity with CloudInit/LinuxPrep/Sysprep/VAppConfig
  (`webhooks/virtualmachine/validation/virtualmachine_validator.go`)
- [x] T024 [US1] [vmop-TBD] ISO-typed cdrom image requirement
  (`webhooks/virtualmachine/validation/virtualmachine_validator_iso.go`, new)
- [x] T025 [US1] [vmop-TBD] Asset name DNS-1123 validation (same file)
- [x] T026 [US1] [vmop-TBD] Commands-immutable-while-powered-on rule (same file)
- [x] T027 [P] [US1] [vmop-TBD] Webhook validation tests

## Phase 8 — Conditions & status (framework)

- Covered by T003 (constants) and T018 (Mark calls) — no separate tasks.

## Phase 9 — Ubuntu end-to-end (US2)

- [x] T028 [US2] [vmop-TBD] `iso_bootstrap_test.go` Ubuntu scenario
  (`test/e2e/vmservice/vmservice/virtualmachine/iso_bootstrap_test.go`)
- [x] T029 [US2] [vmop-TBD] FSS gating wiring
  (`test/e2e/vmservice/config/wcp.yaml`, `test/e2e/vmservice/consts/consts.go`)
- [x] T030 [US2] [vmop-TBD] Wire scenario into `test/e2e/vmservice/vmservice_test.go`
- [x] T031 [P] [vmop-TBD] `test/e2e/README.md` prerequisites note (ISO content
  library item, bootstrap Secret shape)

## Phase Final — Polish

- [x] T032 [vmop-TBD] Fill in `spec.md`'s Reconciliation pipeline, US1/US2
  Given/When/Then scenarios, Edge cases; flip Status to `In Progress`
- [x] T033 [P] [vmop-TBD] Docs page for `spec.bootstrap.iso`
- [x] T034 [vmop-TBD] Full build/vet/test/lint verification pass
