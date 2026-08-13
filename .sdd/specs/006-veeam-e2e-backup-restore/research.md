# Research: Use Veeam for Backup/Restore E2E Tests

- **Spec**: [`spec.md`](./spec.md)
- **Epic**: vmop-4013

## Current mimicry in the E2E suite

All three restore workflows below are currently exercised in `test/e2e/vmservice/vmservice/viadmin/registervm.go`, backed by shared helpers in `test/e2e/vmservice/vmservice/util.go`. None of this is product code; it is E2E-only tooling that fakes what a VADP backup vendor would otherwise do to the vSphere VM.

### Restore to new

- `Context("RegisterVM Alarm", ...)` in `registervm.go` (and the general "Register VM with pre-existing VM CR" contexts) call `vmservice.DeleteVMResource(...)`, which:
  1. Waits for VM Operator's own backup-state recording to catch up (`vmservice.WaitForBackupToComplete` / `waitForBackupToComplete`, `util.go:818`).
  2. Powers off the VM and adds the `vmopv1.PauseAnnotation` annotation.
  3. Calls `vmservice.UnregisterPVCVolumes` (`util.go:928`), which creates `CnsUnregisterVolume` objects per PVC to detach the Kubernetes/CNS side while leaving vSphere-side disks attached.
  4. Deletes the Kubernetes `VirtualMachine` object and strips both `VMFinalizerName` and `VMFinalizerNameDeprecated` (`util.go:1004-1086`) so deletion completes without the controller re-adding the finalizer.
- Net effect: the vSphere VM and its `vmservice.*` ExtraConfig (written continuously by VM Operator) are untouched; only the Kubernetes object is gone. This fakes "the VM was restored to a Supervisor that has no record of it."
- `vmservice.InvokeRegisterVM` (`util.go:1090`) and `vmservice.VerifyPostRegisterVM` (`util.go:1112`) are then used unchanged to drive and verify registration — these are the parts that stay.

### Restore to existing (in-place)

- The two "Incremental Restore - Register VM..." contexts in `registervm.go` (lines ~224-348) do this in-line:
  1. Create and power on a VM, wait for backup completion.
  2. Power off, pause, and unregister PVCs (same helpers as above), but **do not delete the K8s object**.
  3. Directly call `vmObj.Reconfigure(ctx, vmSpec)` on the vSphere VM with an `ExtraConfig` payload that sets `backupapi.VMResourceYAMLExtraConfigKey`, `backupapi.BackupVersionExtraConfigKey`, and (when PVCs are involved) `backupapi.PVCDiskDataExtraConfigKey` back to an **earlier, saved value** — i.e., the test hand-crafts "an older backup was restored" by overwriting ExtraConfig itself, rather than restoring anything.
- `pkg/backup/api` (module `github.com/vmware-tanzu/vm-operator/pkg/backup/api`) defines the ExtraConfig key constants used here; this is the same contract a real VADP-restored VMX would need to preserve.

### Disk-only restore

- `Context("Restore disk only", ...)` in `registervm.go` (lines ~792-1068) is the most involved mimicry:
  1. Creates a VM with a PVC-backed disk, waits for backup, powers off, pauses.
  2. Resolves the disk's `VirtualDisk`/backing/datastore path via `govmomi` device introspection and the PVC → PV → CSI `VolumeHandle` chain.
  3. Calls `vmObj.RemoveDevice(ctx, true, disk)` to detach the disk (`keepFiles=true`, so the VMDK stays on disk) — fakes "Veeam removed the disk from the VM to restore it."
  4. Uses the datastore `FileManager` to `Copy` the VMDK to a new path under the VM's home directory, then `Delete`s the original — fakes "Veeam wrote the restored disk to a new location."
  5. Calls `vmObj.AddDeviceWithProfile(ctx, profile, disk)` to reattach the disk from the new path.
  6. Calls `vslm.NewObjectManager(vCenterClient).ReconcileDatastoreInventory(ctx, ds.Reference())` — this is the step that actually strips the FCD/CNS identity from the old disk, and is explicitly commented in the test as "emulating what Veeam's restore flow does."
- `vmservice.InvokeRegisterVM` + `vmservice.VerifyPostRegisterVM` are again reused unchanged to drive and verify the resulting registration, expecting exactly one new `restored-*` PVC.

### Shared verification/skip conventions worth preserving

- FSS-style skips: `utils.IsFssEnabled(...)` gates `vmServiceBackupRestoreEnabled` / `incrementalRestoreEnabled` in `registervm.go`'s top-level `BeforeEach`. A Veeam-availability skip should follow the same shape (skip with a descriptive message, not fail).
- `skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)` gates the whole spec on running against WCP infra; a Veeam server dependency would plausibly gate on a new infra/testbed capability in the same style.
- Admin-privileged operations use `clusterProxy.NewAdminClusterProxy(ctx)` / `adminProxy.GetAdminClient()` because the regular supervisor-admin kubeconfig lacks RBAC on some API groups (e.g. `cns.vmware.com`). Any new Veeam-driven helper that needs elevated K8s access should follow the same pattern rather than widening RBAC for the default test user.

## Spike: enable and confirm the VBR REST API on the existing appliance

**Status: open, blocking.** This must be resolved before `plan.md` can commit to a concrete client design — every other Veeam-specific investigation item in this document assumes the REST API is reachable, and right now it is not confirmed to be.

- Connectivity check performed 2026-08-10/11 against the existing VBR 12 appliance (internal test-infra host; address intentionally omitted from this repo — see the internal spike ticket `vmop-4030` for connection details):
  - ICMP ping succeeds — the host is reachable on the network.
  - TCP 3389 (RDP) — **open**. This is the access pattern currently used to manage the appliance (RDP in, open the VBR console).
  - TCP 6160 (Veeam Installer Service) — **open**.
  - TCP 9419 (VBR REST API default port) — **connection refused**.
  - TCP 443, 9392, 9401, 10005 — all **connection refused**.
- Refused (not timed-out/filtered) strongly suggests the `Veeam.Backup.RestAPI` Windows service is not running on the box, rather than a network firewall dropping the packets silently — a silent drop would more likely time out. This needs confirming on the box itself, not just inferring from the outside.
- Whoever picks up implementation should, on the VBR appliance (via the existing RDP access — no SSH or other new access method is needed):
  1. Open Windows **Services** and check whether `Veeam RESTful API Service` (exact name may vary by build) is present and running; start it if stopped, and set it to auto-start if this appliance is meant to stay available for E2E runs going forward.
  2. Check **Windows Defender Firewall with Advanced Security** for an inbound allow rule for TCP `9419`; add one scoped to the CI runner's network if missing.
  3. Once the service responds, re-run the connectivity check (`curl -k https://<vbr-host>:9419/api/swagger/ui/index.html`) and capture which `x-api-version` value(s) VBR 12 actually advertises via the Swagger UI, plus whether that endpoint is reachable from outside `localhost` (see the "403 for non-localhost" community report noted below) or needs the client to fall back to the try-newest/step-down strategy.
- This spike is tracked as `vmop-4030` (VMSVC-4030), linked to the epic (`vmop-4013` / VMSVC-4013) via Epic Link, per [`sdd-standards.md`](../../memory/sdd-standards.md) "Tickets and wiki links."

## Testbed and connectivity decisions

- A Veeam Backup & Replication appliance already runs today, on a Windows Server VM, for manual/ad hoc use. It is not currently wired into any CI pipeline.
- The pipeline will be extended to accept the Veeam appliance's address and credentials as parameters, each defaulting to the known existing instance's values when not supplied. This is deliberately low-friction: the appliance is treated as a test-infra dependency with a lower security bar than production credentials, so no Kubernetes Secret plumbing is required for this feature.
- The appliance currently runs **VBR 12**. The REST API link referenced above (`vbr/13/rest/1.3-rev2`) is for VBR 13, which may not be what VBR 12 exposes. The test tooling must not hardcode a single API version; see "Version autodetection" below.

## Version autodetection

- Goal (per `spec.md`): the connecting client detects, at connect time, which REST API version the target Veeam server actually supports, and uses the corresponding request shapes — avoiding a hard version coupling between the E2E pipeline and whatever VBR release happens to be running.
- Confirmed mechanism (cross-referenced from Veeam's own REST API docs, VBR community/forum posts, and third-party write-ups; not yet verified against our own live VBR 12 appliance):
  - The REST API is served over HTTPS on port `9419` (e.g. `https://<vbr-host>:9419/api/...`).
  - Every request — including the token request itself — must carry an `x-api-version` header whose value is a `<major>.<minor>-rev<N>` string (e.g. `1.2-rev1`, `1.3-rev2`). Omitting it or naming an unsupported version produces a documented "Unsupported RESTAPI version" error.
  - The server exposes a Swagger UI at `https://<vbr-host>:9419/api/swagger/ui/index.html` with a "Select a definition" dropdown that enumerates exactly which API version(s) that specific server instance supports. This is reachable pre-authentication and is the concrete answer to "how does a client discover what a given appliance supports without hardcoding a version" — the client can either parse this (or the underlying per-version Swagger/OpenAPI JSON document it references) to pick the newest version the server advertises, or fall back to a try-newest-then-step-down strategy against the "unsupported version" error if parsing the Swagger index turns out to be impractical.
- Once the supported version is known, the client should select the newest mutually-supported version and pin the `x-api-version` header to it for the duration of a single client session (i.e., no re-detection mid-test-run).
- Still to verify directly against the appliance before `plan.md` locks in a client design: the exact Swagger/OpenAPI document path(s) VBR 12 serves, the precise list of `x-api-version` values VBR 12 accepts (VBR 13's docs reference `1.1-rev0` through `1.3-rev2`; VBR 12 almost certainly supports an earlier, non-overlapping subset), and whether VBR 12's Swagger endpoint is reachable unauthenticated by default or requires a config change (one community forum thread reports a VBR instance returning 403 for the Swagger endpoint on anything but `localhost`, which would affect a remote E2E runner).

## Restore-flow decisions

- **Restore to new**: use Veeam's standard "Restore VM" action, targeting a new VM identity in the Supervisor namespace folder. Rejected alternative: "Instant VM Recovery" (boots directly from the backup repository) — its identity/registration implications diverge from what the current acceptance criteria (and the existing mimicked test) assume.
- **Disk-only restore**: use Veeam's dedicated "virtual disk restore" action — https://helpcenter.veeam.com/docs/vbr/userguide/virtual_drive_recovery.html?ver=13 — which restores a single virtual disk directly. Rejected alternative: a full VM restore with everything except the target disk discarded, which would produce extra restore artifacts to track and clean up for no benefit.

## Veeam Backup & Replication REST API

- API reference (VBR 13, REST API 1.3-rev2): job creation endpoint — https://helpcenter.veeam.com/references/vbr/13/rest/1.3-rev2/tag/Jobs#operation/CreateJob
- Login/token reference: https://helpcenter.veeam.com/references/vbr/13/rest/1.3-rev2/tag/Login
- REST API reference index for all documented revisions: https://helpcenter.veeam.com/references/vbr/13/rest/1.3-rev2/ (this page's navigation lists sibling revisions `1.3-rev1`, `1.2-rev1`, `1.2-rev0`, `1.1-rev2`, `1.1-rev1`, `1.1-rev0` — the actual API reference content renders client-side, so a plain fetch only returns the page shell; the concrete auth/versioning facts below came from Veeam's other static doc pages, VBR community forum threads, and third-party write-ups, cross-referenced with each other).
- Confirmed authentication flow: `POST https://<vbr-host>:9419/api/oauth2/token`, `Content-Type: application/x-www-form-urlencoded`, body `grant_type=password&username=<user>&password=<pass>` (domain credentials need the backslash URL-encoded), plus the mandatory `x-api-version` header (see "Version autodetection" above). Response contains `access_token` and `refresh_token`; subsequent requests carry `Authorization: Bearer <access_token>` and must still repeat the `x-api-version` header. Access tokens are short-lived; a refresh flow exists ("Using Refresh Token" in the docs nav) and should be used rather than re-authenticating with the password grant on every call.
- Web UI walkthrough for vSphere backup job creation (useful for confirming REST payload shape by comparison with the UI flow): https://helpcenter.veeam.com/docs/vbr/userguide/backup_job_web.html?ver=13
- Virtual disk restore (used for the disk-only scenario): https://helpcenter.veeam.com/docs/vbr/userguide/virtual_drive_recovery.html?ver=13 — this is a UI-guide link; the equivalent REST endpoint(s) still need to be located for whichever VBR version ends up in scope (see "Not yet investigated" below).
- User-supplied high-level flow for this feature:
  1. Create a temporary backup job via the REST API, named so it can be traced back to the CI pipeline run and target VM.
  2. Take a backup via that job.
  3. Change the VM's configuration (or delete it, for restore-to-new; or remove a disk, for disk-only restore) to set up the "needs restoring" state.
  4. Restore the VM via the REST API.
  5. Invoke the platform's manual VM registration API (`vmservice.InvokeRegisterVM` today).
  6. Verify the result with existing verification helpers (`vmservice.VerifyPostRegisterVM` today).
- Not yet investigated (candidates for follow-up spikes before `plan.md`):
  - The exact `x-api-version` value(s) and Swagger endpoint path VBR 12 (the version actually running today) exposes, confirmed against the live appliance rather than inferred from VBR 13's docs.
  - Whether the Swagger/discovery endpoint is reachable from the E2E runner's network path or only from `localhost` on the VBR appliance itself (see the 403-for-non-localhost forum report noted above) — if it's `localhost`-only by default, the version-autodetection goal needs a fallback (try-newest-then-step-down against the "unsupported version" error) rather than relying on discovery.
  - Exact REST endpoints/payloads for triggering a backup run of a job once created, for triggering a "Restore VM" operation, for triggering a "virtual disk restore" operation, and for job/restore-point deletion during cleanup, for whichever API version(s) end up in scope.
  - Whether the target VBR version's REST API supports scoping a job to a single already-existing VM by MoID/name directly, or whether it requires resolving the VM through VBR's own inventory browsing endpoints first.
  - How VBR's REST API reports job/task failure detail (to satisfy the "surface Veeam's own error" goal).

## Prior art referenced

- `docs/guides/backup-restore/README.md` — the in-repo backup/restore guide already documents the intended production workflow (Sections 3–9) that this feature is meant to make the E2E suite actually exercise, including the restore-type detection logic (Section 4.3) and the hard limitations (Section 7) that any Veeam-driven restore must still respect (e.g., duplicate BIOS UUID, missing ExtraConfig, wrong namespace folder).
