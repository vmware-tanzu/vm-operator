# Tasks: Use Veeam for Backup/Restore E2E Tests

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Epic**: vmop-4013

## Spike

- [x] T001 [vmop-4030] Confirm the VBR REST API is reachable. Resolved 2026-09-25: VBR 13.1 serves REST on `9419`, reachable from the E2E runner when HTTP(S) proxies are bypassed. See `research.md`.
- [x] T002 [vmop-4030] Capture the API version and confirm the disk-restore and cleanup surface. Resolved: API `1.3-rev2`; no virtual-disk restore operation exists; backups are deleted with `DELETE /api/v1/backups/{id}?fromDB=false`. The `spec.md` open questions were updated.
- [x] T002a [vmop-4125] Author `plan.md`.

## Phase 1 — Setup

- [x] T003 [P] [vmop-4126] Scaffold the Veeam client package at `test/e2e/infrastructure/veeam/`. It is not under `test/e2e/vmservice/lib/`, because Ginkgo-mode `-e2e.*` flags break plain `go test` packages under `vmservice/...`.
- [x] T004 [P] [vmop-4127] Add `VeeamConfig` (`server`, `username`, `password`, `repository`, `runID`) to `test/e2e/vmservice/config/config.go`. Add the `veeamConfig` block to `wcp.yaml` with `${VEEAM_SERVER:-}` and the other variables, plus the `wait-veeam-session` and `wait-veeam-session-start` intervals. The defaults are empty here; the appliance values live in the internal CI suite definition (T022).

## Phase 2 — Veeam REST client

- [x] T005 [vmop-4128] OAuth2 password grant with `x-api-version`, token reuse for 10 minutes, and re-login plus retry on `401` (`client.go`).
- [x] T006 [vmop-4129] API version detection via the per-version Swagger document, newest first, pinned per client (`client.go`).
- [x] T007 [vmop-4130] `*ConnectError` kinds (`NotConfigured`, `Unreachable`, `UnsupportedVersion`, `AuthFailure`) and the skip/fail decision in `backuprestore/veeam.go` `connectVeeam`.
- [x] T008 [vmop-4131] Job naming (`JobName`), `FindVM` (matched by moref), `RepositoryID`, and `CreateJob` with the version-specific job type (`operations.go`).
- [x] T009 [vmop-4132] `StartJob`, `Backup`, and `WaitForSession`, with distinct "did not start", "did not finish", and "failed" errors carrying the session id, result, and log.
- [x] T010 [vmop-4133] `LatestRestorePoint`. Restoring an older of two points is not needed after the US2 redesign (see T017).
- [x] T011 [vmop-4134] `DeleteJob` deletes the job's backups with their files, then the job, tolerating `404`. It is registered with `DeferCleanup` right after job creation.
- [x] T012 [P] [vmop-4135] `infrastructure/veeam/veeam_test.go`: plain `go test` against an `httptest` fake VBR.

## Phase 3 — US1: restore to new

- [x] T013 [US1] [vmop-4136] `RestoreVM(..., overwrite=false, ...)` with the version-specific restore path.
- [x] T014 [US1] [vmop-4137] New spec "Should restore a lost VM as a new VM and register it" in `test/e2e/vmservice/vmservice/backuprestore/veeam.go`, labelled `experimental`. It lives in a new suite rather than rewriting `registervm.go` in place, so the simulated tests keep running until the new ones are proven.
- [x] T015 [US1] [vmop-4138] Restore failures fail the test with the `SessionError` (session id, result, and log).

## Phase 4 — US2: restore to existing

- [x] T016 [US2] [vmop-4139] `RestoreVM(..., overwrite=true, ...)`.
- [x] T017 [US2] [vmop-4140] New spec "Should restore an existing VM in place and register it", labelled `experimental`. It uses one backup plus in-guest and annotation divergence instead of two backups, and verifies seeded disk data in the guest. See `plan.md` "Scenarios".
- [x] T018 [US2] [vmop-4141] Registration runs only after the restore session reaches a terminal state.

## Phase 4a — RegisterVM alarm on a restored VM

- [x] T018a New spec "Should raise the RegisterVM alarm on failure and clear it on success", labelled `experimental`. It reuses the restore-to-new steps (`restoreLostVM`), and ports the alarm and event checks from the simulated "RegisterVM Alarm" context into `backuprestore/alarm.go`.

## Phase 5 — US3: disk-only restore

- [x] T019 [US3] [vmop-4142] Re-scoped: VBR exposes no REST virtual-disk restore. See `plan.md` "Scope changes".
- [x] T020 [US3] [vmop-4143] Re-scoped: the "Restore disk only" context in `registervm.go` stays simulated, with a comment saying why.
- [x] T021 [US3] [vmop-4144] `spec.md` updated to record the re-scope.

## Phase 6 — Suite selection and CI

- [x] T022a [vmop-4145] Register the specs under `Context("BACKUP-RESTORE", Label("backup-restore"))` in `vmservice_test.go`. Add `make e2e-backup-restore`, and exclude `backup-restore` from `e2e-smoke`, `e2e-core`, and `e2e-extended`.
- [x] T022 [vmop-4145] Internal CI: add a dedicated, quarantined backup/restore suite (`LABEL_FILTER: "backup-restore"`) that sets the `VEEAM_*` and `E2E_RUN_ID` env values, and add `&& !backup-restore` to the existing suites' filters, including `experimental`. The testbed teardown and support-bundle jobs wait for the new suite, and it is added to the pipeline policy. CI pods are assumed to reach the appliance; if they cannot, the suite fails as `Unreachable` (T007).
- [ ] T022b Register the testbed vCenter on the appliance when it is not already registered. CI provisions a new vCenter for each run, and `FindVM` fails when the appliance does not know the vCenter. Add it as a managed server by PNID, and on cleanup remove only a managed server (and credentials) that the suite added itself. Never touch a registration the suite did not create. See `research.md` for the registration calls.
- [x] T023 [vmop-4146] Skip gate in the suite's `BeforeEach` (T007).

## Phase 7 — Rollout and deprecation

- [x] T026a Mark the replaced simulated contexts in `registervm.go` as `Deprecated:` ("Incremental Restore - Register VM with pre-existing VM CR", "... and PVCs", "RegisterVM - Restore to new", "RegisterVM Alarm").
- [ ] T026b Run `make e2e-backup-restore` on a real testbed, and fix what it finds.
- [ ] T026c After T026b, T022b, and a green run of the CI suite (T022): remove `experimental` from the three specs, delete the deprecated contexts and any helpers used only by them from `registervm.go`, and drop the `&& !backup-restore` exclusion from the quarantined `experimental` CI suite if it is no longer needed.

## Phase Final — Polish

- [x] T024 [vmop-4147] `test/e2e/README.md`: the Backup/Restore category, the `backup-restore` label, the `VEEAM_*` / `E2E_RUN_ID` variables, and the skip/fail and proxy behavior. `.sdd/memory/e2e-testing.md` Makefile table updated.
- [ ] T025 [vmop-4148] Update `docs/guides/backup-restore/README.md` if the real-Veeam runs surface a gap from the documented restore-type detection. None found in the manual runs so far.
- [ ] T027 Flip `spec.md` to `Implemented` and complete its checklist once T026c is done.

---

## Traceability

| Task(s) | Spec section |
|---|---|
| T001, T002 | Open questions |
| T005, T006, T007, T023 | Goals: version autodetection, skip-not-fail; Edge case: auth vs. unreachable |
| T009, T015 | Goals: surface Veeam's job/session id and error |
| T008 | Goals: traceable job naming; Edge case: concurrent-run collision |
| T009 | Edge case: backup run never starts |
| T011 | Goals/Edge case: cleanup on pass and fail |
| T013-T015 | US1: restore to new |
| T016-T018 | US2: restore to existing |
| T019-T021 | US3: disk-only restore (re-scoped) |
| T004, T022 | Goals: parameterized server address and credentials |
| T026a-T026c | Summary: replace the simulated backup/restore steps |
