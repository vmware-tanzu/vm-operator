# Implementation Plan: Use Veeam for Backup/Restore E2E Tests

- **Spec**: [`spec.md`](./spec.md)
- **Research**: [`research.md`](./research.md)
- **Epic**: vmop-4013
- **Date**: 2026-09-25
- **Status**: In Progress

## Summary

Add a dedicated, label-selected E2E suite (`backup-restore`) that backs up a VM Service VM with a real Veeam B&R job, restores it through the VBR REST API, and then runs the existing manual-registration flow (`InvokeRegisterVM` / `VerifyPostRegisterVM`). Two restore scenarios are covered, restore to new and restore to existing (in place), plus the RegisterVM alarm on a VM restored as new. The simulated contexts in `viadmin/registervm.go` that these replace are marked deprecated and are removed once the new suite leaves quarantine. Disk-only restore stays simulated, because VBR has no REST operation for it (see "Scope changes" below).

## Technical context

- **Go version**: the `test/e2e` module's toolchain; no new module.
- **API versions touched**: none. The suite creates `v1alpha2` VMs through the existing manifest builders and reads `v1alpha6` via `vmopv1`.
- **Modules touched**: `test/e2e` only, plus the root `Makefile` and docs.
- **New dependencies**: none. The Veeam client uses only `net/http` and `encoding/json`. The vendored `veeamhub/veeam-vbr-sdk-go` client was rejected: it pins `1.1-rev0`, while the appliance serves `1.3-rev2` and renames the job type and restore path.
- **Product impact**: none. No changes under `pkg/`, `api/`, `controllers/`, or `webhooks/`.

## Constitution check

| Rule | Status | Notes |
|------|--------|-------|
| spec / plan / tasks present | OK | This file retires the "plan.md does not exist yet" note in `tasks.md`. |
| E2E-only change does not touch `pkg/` / `api/` | OK | See [`e2e-sync-with-changes.md`](../../memory/e2e-sync-with-changes.md). |
| New E2E tests carry `Label("experimental")` | OK | Both specs. See [`e2e-testing.md`](../../memory/e2e-testing.md). |
| New labels / Makefile targets documented | OK | `test/e2e/README.md` and `e2e-testing.md`. |
| No internal links, hosts, or credentials in the repo | OK | Server and credentials come from env vars; docs use `<vbr-host>` placeholders. |
| Import aliases, copyright header, lint | OK | `golangci-lint` is clean on the touched packages. |
| One test file per package, external `_test` package | OK | `infrastructure/veeam/veeam_test.go` is a plain `go test` file with no Ginkgo, so no suite file is needed. |

## Design

### Package layout

```
test/e2e/
  infrastructure/veeam/          test-only VBR REST client
    client.go                    connect, version detection, auth, do()
    operations.go                inventory, job, session, restore, cleanup
    veeam_test.go                go test against an httptest fake VBR
  vmservice/vmservice/
    backuprestore/veeam.go       the two Ginkgo specs + helpers
  vmservice/config/              VeeamConfig + wcp.yaml variables
  manifestbuilders/secret.go     seed-data cloud-config secret
  fixtures/yaml/vmoperator/secret/
    secretCloudConfigSeedData.yaml.in
```

The client lives under `infrastructure/` and not under `vmservice/lib/`. In Ginkgo mode, the `-e2e.*` flags are passed to every package under `vmservice/...`, so a plain `go test` package there fails flag parsing.

### Veeam client

- **Version detection**: probe the unauthenticated `GET /swagger/v{ver}/swagger.json` newest first, using `SupportedAPIVersions` (`1.3-rev2` down to `1.1-rev0`). The first `200` wins and is pinned for the life of the client.
- **Auth**: `POST /api/oauth2/token` password grant with `x-api-version`. The token is reused for 10 minutes (VBR expires it after 900 s), and any `401` triggers one re-login and retry. A password re-login was chosen over the refresh-token flow because it is simpler and has the same effect for a test client.
- **Version-specific shapes**: `1.1-*` uses job type `Backup` and `/restore/vmRestore/vmware/`. `1.2+` uses `VSphereBackup` and `/restore/vmRestore/vSphere`.
- **Errors**: connection-time failures are `*ConnectError` with a kind: `NotConfigured`, `Unreachable`, `UnsupportedVersion`, or `AuthFailure`. Session failures are `*SessionError`, carrying the session id, state, result, and the session log.
- **Proxy**: the transport sets `Proxy: nil`. The E2E environment points `HTTPS_PROXY` at the testbed gateway, which cannot reach the appliance.
- **Cleanup**: `DeleteJob` deletes every backup of the job with `DELETE /api/v1/backups/{id}?fromDB=false&includeGFS=true`, waits for the delete session, and polls the backup until it returns `404`. Only then does it delete the job: once the job is gone its backups lose their `jobId` and can no longer be traced back to the run. If a backup survives, the job is kept and an error is returned. A backup without a `jobId` never matches a job, since it may belong to any run. A `404` is treated as already gone.
- **Job naming**: `vmop-e2e-<runID>-<vmName>`. `runID` comes from `E2E_RUN_ID`; if it is unset, a random 6-character value is used.

### Suite selection

- The specs register under `Context("BACKUP-RESTORE", Label("backup-restore"), ...)` in `vmservice_test.go`.
- `make e2e-backup-restore` runs `LABEL_FILTER="backup-restore"`.
- `e2e-smoke`, `e2e-core`, and `e2e-extended` add `&& !backup-restore`, so the Veeam specs never run in the general suites even after `experimental` is dropped.
- The internal CI suite definitions (outside this repo) get a dedicated backup/restore suite and add `&& !backup-restore` to their existing filters, including the quarantined `experimental` suite. That suite carries the appliance address and credentials as env values, which is how the spec's "default to the existing appliance" goals are met without putting internal details in this public repo.

### Skip vs. fail

- Skip when the infra is not WCP, when either backup/restore FSS is disabled, or when no Veeam server is configured (`NotConfigured`). An unconfigured vendor is a missing capability, so a suite running several vendors' tests runs only the ones it has settings for.
- Fail when a configured server is `Unreachable`, `UnsupportedVersion`, or rejects the credentials (`AuthFailure`). Each is a configuration problem that should be debugged, not hidden as a skip; the failure names the `ConnectError` kind and server.
- Fail on any session failure or timeout, with the `SessionError` text: session id, result, and log.

### Scenarios

**Restore to new**, validated manually in `research.md`:

1. Create a VM with a data PVC and the seed-data cloud-config, wait for the seed, record the VM's extension compatibility constraint count, and wait for the backup to be up to date.
2. Create the job, back up, and pick the latest restore point.
3. Delete the VM and its data PVC, and wait until no vSphere VM with that name remains.
4. Restore with `overwrite: false`. From here on, a cleanup destroys any vSphere VM with that name that no VM resource manages, so a restore or RegisterVM failure does not leak the VM and its disks.
5. Assert exactly one VM with that name and a new moref.
6. Run `InvokeRegisterVM`, then `VerifyPostRegisterVM(diskCount)`.
7. Assert the guest hashes verify, and that protection is restored (see below).

**Restore to existing**, validated manually in `research.md`:

1. Create the VM with the seed-data cloud-config, which writes known data to the boot and data disks and records their hashes in the guest.
2. Set the marker annotation to `before-backup`, and wait until it shows up in the backup ExtraConfig.
3. Back up.
4. Diverge: delete the seed files in the guest and set the marker to `after-backup`.
5. Power off and pause, and wait for the `vmoperator.vmware.com/paused` label, which VM Operator sets once a reconcile has seen the pause. Then clear the VM's extension compatibility constraints, recording how many there were, and restore with `overwrite: true`.
   - The constraints' DEVICE invariant makes vCenter reject Veeam's reconfigure, and Veeam cannot skip the check. Clearing them is a VI-admin step: a reconfigure with an empty constraint set and `skipExtensionCompatibilityChecks`.
   - Waiting for the label first ensures no reconcile in flight puts the constraints back.
   - The client must speak vCenter's newest API version (`UseServiceVersion`), as VM Operator does. A development vCenter answers `InvalidProperty` below it.
   - From the restore on, a cleanup deletes the VM and the old PVCs, waits for the VM to be gone, and removes the `cns.vmware.com/pvc-protection` finalizer from the old PVCs, whose volumes the restore destroyed. This runs before the VM's own cleanups, so the PVCs do not stay `Terminating` even when the spec fails early.
6. Run `InvokeRegisterVM` on the same moref.
7. Assert:
   - the `restored-vm` annotation is present and the marker is back to `before-backup`;
   - the pause annotation is removed;
   - the VM no longer references any old PVC;
   - `VerifyPostRegisterVM` passes;
   - the guest hashes verify;
   - protection is restored (see below);
   - the old PVCs are gone or marked for deletion. A failure describes the PVC and the VM's volumes.

**Protection restored**, after either restore:

- VM Operator sets the same number of extension compatibility constraints on the restored VM as the original had.
- VM Operator writes backup data again: the backup ExtraConfig matches the VM's current PVCs, and a new marker value (`after-restore`) shows up in it, so the next backup of the restored VM is not stale.

**RegisterVM alarm**:

1. Skip unless the vCenter defines `WCPRegisterVMFailedAlarm`.
2. Run the restore-to-new steps 1-5 above (shared helper `restoreLostVM`).
3. Save the restored VM resource from ExtraConfig and replace it with invalid data.
4. `InvokeRegisterVM` must fail, post a `com.vmware.wcp.RegisterVM.failure` event, and raise the alarm (yellow).
5. Put the saved resource back. `InvokeRegisterVM` must succeed, `VerifyPostRegisterVM` must pass, and the success event must clear the alarm.

The spec's "two backup runs, restore the older point" is replaced by one backup plus divergence. This proves the same thing, that state after the restore equals the backed-up state and not the current state, with half the backup time.

### Scope changes against `spec.md`

- **Disk-only restore stays simulated.** On VBR 13.1 with API `1.3-rev2`, a restore point's `allowedOperations` include no virtual-disk restore, and the Swagger surface has no disk-restore path. The only REST-native alternative is FCD instant recovery plus migrate. Its result differs from what a VADP disk restore produces, so it would not be a more faithful test than the current mimicry. The simulated context stays and is annotated accordingly.
- **The RegisterVM alarm test moves to the new suite as its own test.** It is kept separate from restore to new so an alarm failure is not reported as a restore failure, and so a vCenter without the alarm skips it before paying for a backup and restore. Its failure injection (an invalid VM resource in ExtraConfig) is a deliberate RegisterVM failure, not a simulated restore, so it stays.

### Deprecation path

1. This change: the new suite lands as `experimental`, and the replaced contexts in `registervm.go` (both incremental-restore contexts, "RegisterVM - Restore to new", and "RegisterVM Alarm") get a `Deprecated:` comment naming their replacement.
2. After the new suite passes on real testbeds, remove `experimental` from both specs.
3. In the same change as step 2, delete the deprecated contexts, and any helpers they alone used, from `registervm.go`.

## Test strategy

- `go test ./infrastructure/veeam/...` covers version selection, connect-error kinds, re-login on `401`, moref matching, repository lookup, the version-specific job type and restore path, the `WaitForSession` start/finish/fail timeouts, and `DeleteJob`, including the `404` case, waiting for each backup to be gone, keeping the job while a backup survives, and never deleting a backup without a job.
- `ginkgo --dry-run` confirms all three specs are selected by `backup-restore` and by none of the smoke/core/extended filters.
- Both restore scenarios were validated manually against the appliance (see `research.md`); the alarm test reuses the restore-to-new steps. The automated suite runs against a live testbed with `make e2e-backup-restore` before `experimental` is removed.

## Risks

- **Shared appliance**: concurrent runs share one VBR server. Job names include the run ID, and cleanup always runs through `DeferCleanup`. A vCenter managed-server registration that already exists on the appliance is shared, and the suite never modifies it.
- **vCenter registration**: the appliance must know the testbed vCenter as a managed server before `FindVM` can see its VMs. CI provisions a new vCenter for each run, so the specs run in an `Ordered` `Veeam` container whose `BeforeAll` registers the vCenter by PNID when the appliance does not manage it yet (T022b), and whose `DeferCleanup` removes that registration and its credentials after the last spec. Only a registration the suite added is removed. A registration failure fails the specs, since it means the appliance cannot reach the vCenter on 443 or its hosts on 902, which code cannot fix.
- **Network reachability from CI pods** to the appliance on `9419` is assumed. If it is not reachable, the tests fail as `Unreachable`, which is intended: the suite is configured with a server, so an unreachable one is a bug to fix.
- **Duration**: a full backup plus restore takes several minutes per scenario. Session waits are bounded by `wait-veeam-session` (20m) and `wait-veeam-session-start` (5m) in `wcp.yaml`.
