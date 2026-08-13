# Tasks: Use Veeam for Backup/Restore E2E Tests

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: plan.md does not exist yet — this feature is E2E-tooling-only (no CRD/controller/webhook
  surface), and the client design in Phase 2 is itself blocked on spike `vmop-4030`. See T002a below,
  which authors `plan.md` once that spike answers the REST-surface questions; until then this task
  list carries the technical decisions that would otherwise live there.
- **Epic**: vmop-4013

## Blocking spike (must resolve before Phase 2 starts)

- [ ] T001 [vmop-4030] Spike: enable/confirm the VBR REST API on the existing appliance — start the
      `Veeam RESTful API Service`, open firewall TCP 9419, confirm `GET /api/v1/serverInfo` and the
      Swagger UI respond from the CI runner's network path (not just `localhost`). Record findings in
      `research.md`. Blocks Phase 2 (T005-T010, T012).
- [ ] T002 [vmop-4030] Spike: capture the appliance's exact `buildVersion` and `x-api-version`, diff its
      live Swagger document against the vendored `veeamhub/veeam-vbr-sdk-go` `1.1-rev0` spec, and confirm
      (a) whether a disk-level restore endpoint exists on this build, and (b) whether
      `DELETE /api/v1/jobs/{id}` cascades to backups/restore points or orphans them. Update `research.md`
      "Open risk: disk-only restore may need a design change" and resolve the two
      `[NEEDS CLARIFICATION]` items in `spec.md` (or explicitly re-scope them) based on the answer.
      Blocks T011 (cleanup fallback shape) and Phase 5 (T019-T021, disk-only restore).
- [ ] T002a [vmop-4125] Author `plan.md` capturing the client design (package layout below), test
      strategy, and confirmation that this feature has no `pkg/`/`api/` impact, now that T001/T002 have
      resolved the open questions that were blocking it. This retires the note at the top of this file.

## Phase 1 — Setup

- [ ] T003 [P] [vmop-4126] Scaffold the Veeam client package `test/e2e/vmservice/lib/veeam/` (mirrors the existing
      `test/e2e/vmservice/lib/vmoperator/` and `lib/csi/` shared-helper packages).
- [ ] T004 [P] [vmop-4127] Add `VeeamConfig` (server address, credentials, defaults) to
      `test/e2e/vmservice/config/config.go`, wired the same way `InfraConfig` is today, and add the
      corresponding `veeamServerAddress`/`veeamUsername`/`veeamPassword` variable defaults (pointing at
      the existing manual-test appliance) to `test/e2e/vmservice/config/wcp.yaml`, using the same
      `${VAR:-default}` `expandEnv` convention already used for `STORAGE_CLASS` etc.

## Phase 2 — Foundational (Veeam REST client)

*Blocked on T001. (T011 additionally blocked on T002.)*

- [ ] T005 [vmop-4128] Implement OAuth2 token client in `test/e2e/vmservice/lib/veeam/auth.go`:
      `POST /api/oauth2/token` password grant with the mandatory `x-api-version` header, storing
      `access_token`/`refresh_token`/`expires_in`, and a refresh-token flow invoked before the ~900s
      access token would expire (a restore can outlast one token's lifetime per `research.md`).
- [ ] T006 [vmop-4129] Implement API version autodetection in
      `test/e2e/vmservice/lib/veeam/version.go`: query `GET /api/v1/serverInfo` (and/or the Swagger
      index) at connect time, select the newest mutually-supported `x-api-version`, and pin it for the
      client session. Return a typed "unsupported/unreachable" error the caller can turn into a Ginkgo
      skip.
- [ ] T007 [vmop-4130] Implement a Ginkgo skip helper in
      `test/e2e/vmservice/vmservice/viadmin/registervm.go` (or a new
      `test/e2e/vmservice/lib/veeam/skip.go`) that distinguishes "Veeam unreachable", "Veeam auth
      failure", and "Veeam API version unsupported" per spec's edge case on triage clarity, following
      the existing `utils.IsFssEnabled` / `skipper.SkipUnlessInfraIs` skip-not-fail convention noted in
      `research.md`.
- [ ] T008 [vmop-4131] Implement job naming and creation in `test/e2e/vmservice/lib/veeam/job.go`:
      derive a job name from the CI pipeline run ID and target VM name (unique per concurrent run per
      spec's edge case), resolve the VM's `objectId` via
      `GET /api/v1/inventory/vmware/hosts/{vcName}?nameFilter=<vmName>`, resolve the repository id via
      `GET /api/v1/backupInfrastructure/repositories`, and `POST /api/v1/jobs` with
      `schedule.runAutomatically:false`.
- [ ] T009 [vmop-4132] Implement backup run trigger/wait in `test/e2e/vmservice/lib/veeam/backup.go`:
      `POST /api/v1/jobs/{id}/start`, poll `GET /api/v1/sessions/{id}` to a successful terminal state,
      and on failure/timeout surface the session id, `state`, `result`, and
      `GET /api/v1/sessions/{id}/logs` in the returned error (spec: "surface Veeam's own job/task
      identifier and reported error/status"). Fail fast with a distinct "backup run did not start"
      error if the session never leaves its initial state, rather than waiting out the suite's outer
      timeout (spec edge case).
- [ ] T010 [vmop-4133] Implement restore-point discovery in `test/e2e/vmservice/lib/veeam/restorepoint.go`:
      `GET /api/v1/backups` → `/api/v1/backups/{id}/objects` → `GET /api/v1/backupObjects/{id}/restorePoints`,
      exposing a way to pick the newest and the Nth-oldest restore point (needed by the in-place-restore
      user story, which restores an older of two points).
- [ ] T011 [vmop-4134] (Blocked on T002.) Implement job (and, per T002's findings, restore point/backup) cleanup in
      `test/e2e/vmservice/lib/veeam/cleanup.go`, callable unconditionally from `DeferCleanup`/`AfterEach`
      so it runs on both pass and fail (spec: "Cleanup ... must be attempted even when an assertion
      earlier in the test fails").
- [ ] T012 [P] [vmop-4135] Unit-style tests for the client package (name/version negotiation, error
      surfacing, cleanup-always-runs contract) in `test/e2e/vmservice/lib/veeam/veeam_test.go`, using a
      local `httptest` fake VBR server rather than a live appliance, plus the companion
      `test/e2e/vmservice/lib/veeam/veeam_suite_test.go` holding just the `TestXxx(t *testing.T)`
      entry point per `testing-standards.md`.

## Phase 3 — User Story 1: restore-to-new via real Veeam backup/restore

- [ ] T013 [US1] [vmop-4136] Implement `RestoreToNew` in `test/e2e/vmservice/lib/veeam/restore.go`:
      `POST /api/v1/restore/vmRestore/vmware/` with `type: "Customized"`, targeting a new VM identity
      in the Supervisor namespace folder, and wait for the restore session to reach a successful
      terminal state (same session-polling/error-surfacing shape as T009).
- [ ] T014 [US1] [vmop-4137] Replace the `vmservice.DeleteVMResource` mimicry in the "RegisterVM Alarm"
      / "Register VM with pre-existing VM CR" contexts of
      `test/e2e/vmservice/vmservice/viadmin/registervm.go` with: create job (T008) → run backup (T009)
      → `RestoreToNew` (T013) → existing `vmservice.InvokeRegisterVM` / `vmservice.VerifyPostRegisterVM`
      (unchanged, per spec's "reused as-is" requirement) → cleanup (T011).
- [ ] T015 [US1] [vmop-4138] Add/extend the failing-restore assertion so a `RestoreToNew` failure or
      timeout produces a test failure message containing Veeam's job/task id and reported failure
      reason, per this user story's second acceptance criterion.

## Phase 4 — User Story 2: restore-to-existing (in-place) via real Veeam backup/restore

- [ ] T016 [US2] [vmop-4139] Implement `RestoreInPlace` in `test/e2e/vmservice/lib/veeam/restore.go`:
      trigger Veeam's in-place VM restore against a specified (older) restore point id, and wait for
      the restore session to reach a successful terminal state before returning.
- [ ] T017 [US2] [vmop-4140] Replace the hand-crafted `vmObj.Reconfigure(...)` ExtraConfig overwrite in
      the "Incremental Restore - Register VM..." contexts of `registervm.go` with: two backup runs via
      the job from T008/T009 (older + newer restore point) → `RestoreInPlace` (T016) against the older
      point → existing `vmservice.InvokeRegisterVM` / `vmservice.VerifyPostRegisterVM` → cleanup (T011).
- [ ] T018 [US2] [vmop-4141] Ensure the test waits for the Veeam in-place restore's terminal state
      before invoking manual registration (preferred path in the spec), and if registration is ever
      invoked while Veeam is still restoring, capture and report that as a registration failure rather
      than masking it as test-infrastructure flakiness.

## Phase 5 — User Story 3: disk-only restore via real Veeam backup/restore

*Blocked on T002's answer for which REST operation is available on the real appliance.*

- [ ] T019 [US3] [vmop-4142] Once T002 resolves the approach, implement the chosen disk-only restore
      operation in `test/e2e/vmservice/lib/veeam/restore.go` — either a virtual-disk-restore call, or
      the FCD instant-recovery-then-migrate REST sequence
      (`POST /api/v1/restore/instantRecovery/vmware/fcd` + its `/migrate` follow-up) per `research.md`'s
      documented fallback — scoped to a single disk, not the whole VM.
- [ ] T020 [US3] [vmop-4143] Replace the manual disk relocate/`ReconcileDatastoreInventory` mimicry in
      the "Restore disk only" context of `registervm.go` with: job/backup (T008/T009) → the disk-only
      restore from T019 → existing `vmservice.InvokeRegisterVM` / `vmservice.VerifyPostRegisterVM`,
      asserting exactly one new `restored-*` PVC as today → cleanup (T011).
- [ ] T021 [US3] [vmop-4144] If T002 determines no REST-native disk-level restore exists and the
      full-VM-restore-then-manual-detach fallback (option (b) in `research.md`) must be used instead,
      update `spec.md`'s disk-only restore acceptance criteria to match before merging this task, per
      the SDD rule that a revealed wrong assumption updates the spec in the same PR.

## Phase 6 — Pipeline parameterization and skip conventions

- [ ] T022 [P] [vmop-4145] Wire the CI pipeline definition(s) to accept Veeam server address and
      credentials as parameters, defaulting to the existing appliance's values, feeding
      `VeeamConfig` (T004) via the same env-var-expansion mechanism as `E2E_KUBECONFIG_PATH` etc.
- [ ] T023 [vmop-4146] Add a top-level `BeforeEach`/`skipper` gate in `registervm.go` that attempts
      Veeam connect + version negotiation (T005/T006) once per spec run and skips (not fails) every
      Veeam-backed context with a message naming the missing configuration when the server is
      unreachable, unauthenticated, or on an unsupported API version, per the "QA / CI maintainer"
      acceptance criteria.

## Phase Final — Polish

- [ ] T024 [vmop-4147] Update `test/e2e/README.md` with the new Veeam server/credential
      parameters, the skip behavior, and how to point a local run at a different VBR instance.
- [ ] T025 [vmop-4148] Update `docs/guides/backup-restore/README.md` if the real-Veeam E2E flow
      surfaces any gap between documented production restore-type detection (Section 4.3) and what the
      real appliance actually produces.
- [ ] T027 Flip `spec.md` status from `Draft` to `Implemented` and check off every item in its
      "Review & acceptance checklist" once all tasks above are complete and both
      `[NEEDS CLARIFICATION]` markers are resolved.

---

## Traceability

| Task(s) | Spec section |
|---|---|
| T001, T002 | Open questions (both `[NEEDS CLARIFICATION]` items) |
| T005, T006, T023 | Goals: version autodetection, skip-not-fail |
| T007, T009, T015 | Goals: surfacing Veeam's own job/task id and error on failure |
| T008 | Goals: traceable job naming; Edge case: concurrent-run name collision |
| T009 | Edge case: backup run never starts |
| T011 | Goals/Edge case: cleanup on pass and fail, no orphaned artifacts |
| T013-T015 | US1: restore-to-new |
| T016-T018 | US2: restore-to-existing (in-place) |
| T019-T021 | US3: disk-only restore |
| T022 | Goals: pipeline-parameterized server address/credentials |

All tasks above have been filed as stories under epic `vmop-4013` (`VMSVC-4013`), with
`customfield_10830` set to the epic on each. See `vmop-4125` through `vmop-4148` (`VMSVC-4125`
through `VMSVC-4148`).
