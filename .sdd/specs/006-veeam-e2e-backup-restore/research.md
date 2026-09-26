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

## Validated walkthrough: restore to new (manual run, 2026-09-25)

**Status: validated end to end against a live appliance.** This section supersedes the doc/OSS-derived claims further down this file wherever they conflict (notably the VBR 12 assumption, the `vmware/` restore path, the `"Backup"` job type, and "no REST call to delete a backup").

### Environment

- Appliance: VBR **13.1.1.18** (not VBR 12 as assumed below), REST on `https://<vbr-host>:9419`.
- Supported `x-api-version` values: `1.1-rev0`, `1.1-rev2`, `1.2-rev0`, `1.2-rev1`, `1.3-rev0`, `1.3-rev1`, `1.3-rev2`. The walkthrough used `1.3-rev2`.
- Supervisor namespace VM (`v1alpha6`, Photon 5 image, `best-effort-small`) with one user PVC (1Gi) plus the backfilled boot-disk PVC (all-disks-are-PVCs), powered on, `VirtualMachineBackupUpToDate=True`.

### Version detection (no auth needed)

- `GET /swagger/v{ver}/swagger.json` is unauthenticated and returns `200` for a supported version and `404` otherwise — a cheap, deterministic probe. Walk a newest-first candidate list and take the first `200`.
- Token endpoint with an unsupported version returns `400` with error code `NotImplemented`; bad credentials return `401` `AccessDenied`; any API call without a token returns `401`.
- Request shapes differ by version and must be selected from the detected version:

| Item | `1.1-rev0` | `1.2-rev0`+ |
|------|------------|-------------|
| Entire VM restore | `POST /api/v1/restore/vmRestore/vmware/` | `POST /api/v1/restore/vmRestore/vSphere` |
| Backup job `type` | `Backup` | `VSphereBackup` |
| `quickBackup/vSphere` | absent | present |

### Auth

- `POST /api/oauth2/token`, form-encoded `grant_type=password&username=..&password=..`, with the `x-api-version` header. `expires_in` is `900` seconds, so a client must refresh (or re-login) during long waits; the backup + restore + register sequence below alone takes ~5 minutes.
- Every subsequent call carries `Authorization: Bearer <token>` and the same `x-api-version`.

### One-time: make the vCenter known to Veeam

Done once per vCenter (the appliance is shared; tests must never remove it). Idempotent check first: `GET /api/v1/backupInfrastructure/managedServers` and match on `name`.

1. `POST /api/v1/credentials` — `{"type":"Standard","username":..,"password":..,"description":..}` → credential `id`.
2. `POST /api/v1/connectionCertificate` — `{"serverName":"<vc-pnid>","type":"ViHost","credentialsId":..,"port":443}` → `certificate.thumbprint` (uppercase SHA-1, no colons). A hand-computed colon-separated SHA-1 or a SHA-256 thumbprint is rejected (`PartialChain`).
3. `POST /api/v1/backupInfrastructure/managedServers` — `{"type":"ViHost","name":"<vc-pnid>","credentialsId":..,"port":443,"certificateThumbprint":..,"description":..}` → async session; poll it (see below). The VC **PNID** is required: registering by IP fails with "Remote certificate name mismatch".

### Step 1 — create a backup job

- Resolve the VM: `GET /api/v1/inventory/vmware/hosts/<vc-pnid>?nameFilter=<vm-name>` → `objectId` (the moref, e.g. `vm-126`) and `hostName`.
- Resolve the repository: `GET /api/v1/backupInfrastructure/repositories` (the walkthrough used "Default Backup Repository").
- `POST /api/v1/jobs` → `201` with the job `id`. Minimal body accepted by `1.3-rev2`:

```json
{
  "type": "VSphereBackup",
  "name": "vmop-e2e-<run-id>-<vm-name>",
  "description": "vmop e2e",
  "isHighPriority": false,
  "virtualMachines": {"includes": [{
    "type": "VirtualMachine", "name": "<vm-name>",
    "objectId": "<moref>", "hostName": "<vc-pnid>",
    "platform": "VSphere"}]},
  "storage": {
    "backupRepositoryId": "<repo-id>",
    "backupProxies": {"autoSelectEnabled": true},
    "retentionPolicy": {"type": "RestorePoints", "quantity": 7}},
  "guestProcessing": {
    "appAwareProcessing": {"isEnabled": false},
    "guestFSIndexing": {"isEnabled": false}},
  "schedule": {"runAutomatically": false}
}
```

- The job is bound to the VM **moref**, not its name or UUID (see Step 5 for why that matters).

### Step 2 — run the backup

- `POST /api/v1/jobs/{id}/start` with `{"performActiveFull":false}` → `201` session.
- Poll `GET /api/v1/sessions/{id}`: `state` goes `Starting` → `Working` → `Postprocessing` → `Stopped`; the outcome is `result.result`. Logs: `GET /api/v1/sessions/{id}/logs` → `records[].{status,title}`.
- Took ~2.5 minutes for a 17 GB (859 MB used) VM. CBT was enabled automatically.
- **Result was `Warning`, not `Success`**: "Your vCenter Server API version is higher than the version this product build has been tested against … KB2443". The client must treat `Success` and `Warning` as success and only `Failed` as failure, and should log the warning message.

### Step 3 — find the restore point

- `GET /api/v1/backups?jobIdFilter=<job-id>` → backup `id`.
- `GET /api/v1/restorePoints?backupIdFilter=<backup-id>` → restore point(s) in one call (no need for the `backups → objects → restorePoints` chain). Pick the newest by `creationTime`.
- Note: `GET /api/v1/backups/{id}/objects` reports a *different* (child) `backupId` than the parent backup — do not key lookups off it.
- The restore point's `allowedOperations` were `StartEntireVmRestore`, `StartFCDInstantRecovery`, `StartViVMInstantRecovery`, `StartHvVMInstantRecovery`, `StartFlrRestore` — confirming there is **no virtual-disk restore** operation via REST on 13.1.
- `GET /api/v1/restorePoints/{id}/disks` listed both disks: the boot VMDK and the PVC's FCD (named by its FCD file name).

### Step 4 — lose the VM

- `kubectl delete vm <name>` then `kubectl delete pvc <user-pvc>` (the boot-disk PVC is owned by the VM and is garbage-collected). This is a real disaster, not mimicry: the vSphere VM, its VM home directory, and the FCD were all gone from the datastore afterwards. No pause annotation, finalizer surgery, or `CnsUnregisterVolume` is needed for this scenario.

### Step 5 — restore with Veeam

- `POST /api/v1/restore/vmRestore/vSphere` with:

```json
{
  "type": "OriginalLocation",
  "restorePointId": "<rp-id>",
  "powerUp": false,
  "overwrite": false,
  "reason": "vmop e2e restore to new"
}
```

- → `201` session; ~2 minutes; result `Success`. Log: "Registering restored VM on host …, pool: <ns>, folder: <ns>, storage: <datastore>" — i.e. back in the namespace resource pool and namespace VM folder, which RegisterVM requires (guide §7).
- Observed state of the restored VM:

| Property | Result |
|----------|--------|
| moref | **New** (`vm-129` vs. original `vm-126`) |
| BIOS UUID / instance UUID | Preserved |
| `vmservice.*` ExtraConfig | Preserved (`backupVersion` = value at snapshot time) |
| VM home path / datastore | Same as original |
| Former FCD disk | Plain VMDK in the VM home directory, new disk UUID |
| Power state | Powered off |

- Consequences for the test design: (a) the moref must be looked up after restore (by name in the namespace folder), not reused from before; (b) the backup job still targets the old moref, so a later backup of the restored VM needs a new job (or a job edit) — per-test jobs keep this simple.

### Step 6 — register and verify

- RegisterVM on the new moref (the suite's `wcpClient.RegisterVM` → dcli `namespaces instances registervm --namespace <ns> --vm <moref>`; `govc namespace.registervm -vm <path> <ns>` is equivalent for manual runs) → task succeeded in ~20 seconds.
- Result: `VirtualMachine` CR recreated with `status.uniqueID` = new moref, annotation `vmoperator.vmware.com/restored-vm`, two `restored-*` PVCs (boot disk + user disk), all conditions `True` (including `UnmanagedVolumesRegistered`/`Backfilled`), powered off. Powering on produced an IPv4 address.
- So `vmservice.VerifyPostRegisterVM(..., expectedRestoredPVCCount = <number of disks>, ...)` applies as-is.

### Cleanup (endpoints confirmed present in `1.3-rev2`)

- `DELETE /api/v1/backups/{id}?fromDB=false&includeGFS=true` removes the backup and its files (`fromDB=true` would only forget it and keep the files); returns `201` with a session.
- `DELETE /api/v1/jobs/{id}` → `204`.
- Order: delete the backup first, then the job. Not yet exercised on the appliance — verify during implementation.

## Validated walkthrough: restore to existing (manual run, 2026-09-25)

**Status: validated end to end against a live appliance.** Same appliance, API version, auth, and Steps 1–3 (job, backup, restore point) as the restore-to-new walkthrough above; only the differences are recorded here.

### Setup

- Ubuntu image, `best-effort-small`, `v1alpha6`, powered on, with a 2Gi user PVC in addition to the backfilled boot-disk PVC.
- A cloud-init `rawCloudConfig` Secret writes a seed script via `write_files` and runs it from `runcmd`. The script formats the non-root disk as ext4 (label `vmopdata`) only when it is blank, mounts it at `/mnt/data` via an fstab entry, writes an 8 MiB random file and a small text file to the boot disk and to `/mnt/data`, records their `sha256sum` in the guest, then `sync`s and touches a done marker.
- A user annotation `e2e.vmoperator.vmware.com/restore-marker=before-backup` was set on the CR and allowed to propagate to `vmservice.virtualmachine.resource.yaml` (`VirtualMachineBackupUpToDate=True`) before the backup ran.
- Guest access: vCenter guest operations (`guest.run`) are denied on Supervisor VMs even for `administrator@vsphere.local`, so seeding verification and file deletion used SSH through the testbed gateway — the same path as the suite's `VerifyLoginAndRunCmdsInVDSSetup`.

### Step 4 — diverge from the backup

- Delete the four seeded files (`rm` + `sync`) and change the annotation to `after-backup`. VM Operator bumps the `backup-version` annotation and ExtraConfig on every change, so the live ExtraConfig version is now newer than the one inside the restore point.
- Power off (`spec.powerState: PoweredOff`), then add `vmoperator.vmware.com/paused=true` **before** starting the restore. The pause is load-bearing: while unpaused, VM Operator rewrites ExtraConfig from the CR, which would erase both the older backed-up `backupVersion` (the restore-to-existing trigger) and the backed-up annotations. The task list confirmed no VM Operator `ReconfigVM` ran after the pause.
- Not needed: `Removable=true` on volumes, and `CnsUnregisterVolume`. The mimic test needs both only because its fake restore leaves the FCDs registered. Running `CnsUnregisterVolume` with `RetainFCD: false` here would be actively harmful, since the old FCD backing path is reused by the restored disk (see below).

### Step 5 — restore with Veeam

- Same call as restore to new, with `"overwrite": true`:

```json
{
  "type": "OriginalLocation",
  "restorePointId": "<rp-id>",
  "powerUp": false,
  "overwrite": true,
  "reason": "vmop e2e restore to existing"
}
```

- → `201` session; ~2 minutes; result `Success`. Veeam works on the **same VM object**: `Reload`, several `ReconfigVM`, `CreateSnapshot`, `RevertToSnapshot`, `RemoveSnapshot`, all initiated by the Veeam service account.
- Observed state of the restored VM:

| Property | Result |
|----------|--------|
| moref | **Preserved** (same `vm-NNN`) |
| BIOS UUID / instance UUID / vmx path | Preserved |
| `vmservice.*` ExtraConfig | Restored to backup time; `backupVersion` < current CR annotation |
| FCD registrations (boot + user PVC) | **Destroyed**: both FCD IDs gone from the datastore catalog |
| Disks | Plain VMDKs, new disk UUIDs, no `vDiskId`. User disk keeps its old `fcd/<id>.vmdk` path; boot disk moved to `fcd/<vm>_N.vmdk`. Device labels swap (unit 0 becomes "Hard disk 2") — match disks by unit number or capacity, never by label |
| PVCs / `CnsNodeVmBatchAttachment` | Still `Bound` / `Ready`, pointing at the destroyed volumes (stale until RegisterVM) |
| Power state | Powered off |

- No RegisterVM task ran automatically, despite the wording of guide §9.2 step 4. The test must invoke RegisterVM itself.

### Step 6 — register and verify

- RegisterVM on the (unchanged) moref, with no additional preparation → task succeeded in ~11 seconds.
- CR after registration:
  - `restore-marker` back to `before-backup` (user annotations from the backup take precedence, guide §8.1.3).
  - `vmoperator.vmware.com/restored-vm` added.
  - The pause annotation was **removed by RegisterVM**.
  - Both volumes now reference new `restored-*` PVCs backed by newly registered FCDs; volume names and `removable` values are kept from the backup.
  - Powered off; all conditions `True` after power-on.
- Old PVCs were marked for deletion but stayed `Terminating` with the `cns.vmware.com/pvc-protection` finalizer — the same PV/PVC cleanup gap noted by the TODO in `vmservice.UnregisterPVCVolumes`. The test should assert that they have a `deletionTimestamp` rather than wait for them to disappear, and cleanup must tolerate them.
- Power on → same IPv4 address. After `mount -a`, the `sha256sum` of all four seeded files matched the pre-backup values exactly. File mtimes equal the original seed time, which confirms the data came from the restore and not from a cloud-init re-run.

### Consequences for the test design

- Order is: power off → pause → Veeam restore (`overwrite: true`) → RegisterVM → verify. Pause must precede the restore.
- `VerifyPostRegisterVM(..., expectedRestoredPVCCount = <number of disks>, ...)` applies. The moref is stable, so the existing `status.uniqueID` can be reused for RegisterVM.
- Data verification needs guest SSH through the gateway and a deterministic seed (hashes recorded in-guest before the backup and compared after the restore). A `restore-marker`-style user annotation is a cheap check that the backed-up CR was applied.
- Because the moref is stable, the same Veeam job can back up the VM again after an in-place restore; this is unlike restore to new.

## Spike: enable and confirm the VBR REST API on the existing appliance

**Status: resolved (2026-09-25).** The REST API is reachable on port `9419`; see "Validated walkthrough: restore to new" above. The notes below are kept for history.

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
  4. Authenticate with `x-api-version: 1.1-rev0` (the oldest VBR-12-era value we have direct spec evidence for — see "VeeamHub OSS evidence" below) and call `GET /api/v1/serverInfo` to capture the appliance's exact `buildVersion`. Then fetch that version's own Swagger document from the appliance and diff it against the vendored `1.1-rev0` spec in `veeamhub/veeam-vbr-sdk-go` to confirm whether disk-level restore and backup/restore-point deletion endpoints exist on our specific build (see "Open risk: disk-only restore may need a design change" below) — this determines whether `spec.md`'s disk-only restore and cleanup acceptance criteria need to change.
- This spike is tracked as `vmop-4030` (VMSVC-4030), linked to the epic (`vmop-4013` / VMSVC-4013) via Epic Link, per [`sdd-standards.md`](../../memory/sdd-standards.md) "Tickets and wiki links."

## Testbed and connectivity decisions

- A Veeam Backup & Replication appliance already runs today, on a Windows Server VM, for manual/ad hoc use. It is not currently wired into any CI pipeline.
- The pipeline will be extended to accept the Veeam appliance's address and credentials as parameters, each defaulting to the known existing instance's values when not supplied. This is deliberately low-friction: the appliance is treated as a test-infra dependency with a lower security bar than production credentials, so no Kubernetes Secret plumbing is required for this feature.
- The appliance currently runs **VBR 12**. The REST API link referenced above (`vbr/13/rest/1.3-rev2`) is for VBR 13, which may not be what VBR 12 exposes. The test tooling must not hardcode a single API version; see "Version autodetection" below.

## VeeamHub OSS evidence (concrete API surface, not just docs)

A survey of `github.com/veeamhub` (2026-08-12) turned up a primary source that beats every public doc page consulted so far: **`veeamhub/veeam-vbr-sdk-go`** vendors Veeam's own `swagger.json` with `info.version: "1.1-rev0"` and `x-veeam-prev-version: "1.0-rev2"` — i.e. this is the actual VBR **12.0** REST spec, plus a generated Go client and runnable examples (`pkg/client/example_test.go`). `veeamhub/veeam-postman` independently corroborates the `1.1-rev0` ⇒ VBR 12.0 mapping and the on-appliance Swagger URLs. `veeamhub/powershell` (`BR-VBR13-PreUpgradeCheck/VBR13-PreUpgradeCheck.ps1`, `VSPC-HostedUsage/Set-HostedVbrJobAssignment.ps1`) demonstrates working auth/header code against real appliances. Dead ends: `veeam-python` (AWS/Azure assessment, unrelated), `veeam-terraform` (unrelated `x-api-version` hit), `veeam-vscan-security` (only references the Swagger UI URL), `moov` (docs-only; confirms disk streaming uses the **Data Integration API over iSCSI**, not REST).

**Base URL confirmed as `https://<vbr-host>:9419`** (`example_test.go`) — the SDK's own README claims `:9398`, which is the legacy Enterprise Manager port and is wrong; do not follow that page.

Concrete, code/spec-backed answers this surfaced (supersedes the doc-only version further down this file where they conflict):

- **Create a job**: `POST /api/v1/jobs`, body is a full `JobSpec`/`BackupJobSpec` (`name`, `description`, `type:"Backup"`, `virtualMachines.includes[]`, `storage.backupRepositoryId` + `retentionPolicy`, `guestProcessing`, `schedule`). There is **no ad-hoc/"quick backup" endpoint** in 1.1-rev0 — set `schedule.runAutomatically:false` to get a job that only runs when explicitly started. Resolve the target VM's `objectId` via `GET /api/v1/inventory/vmware/hosts/{vcName}?nameFilter=<vmName>` and the repository id via `GET /api/v1/backupInfrastructure/repositories`.
- **Trigger a backup run**: `POST /api/v1/jobs/{id}/start` with `JobStartSpec{performActiveFull, startChainedJobs}` → 201 `SessionModel{id, state, progressPercent, result}`. Poll `GET /api/v1/sessions/{id}`; logs at `GET /api/v1/sessions/{id}/logs`; `POST /api/v1/sessions/{id}/stop` to abort.
- **Restore VM (restore-to-new)**: `POST /api/v1/restore/vmRestore/vmware/` (trailing slash matters), body `EntireViVMRestoreSpec` with `type: "Customized"` to rename — the new name goes in `folder.vmName`, and `folder.folder` must still be a full `VmwareObjectModel` (`name`/`type`/`hostName` all required) even when keeping the original vSphere folder. Restore point discovery chain: `GET /api/v1/backups` → `/api/v1/backups/{id}/objects` → `GET /api/v1/backupObjects/{id}/restorePoints`.
- **Virtual disk restore — likely NOT available via REST on our appliance.** Enumerating all 97 paths in the 1.1-rev0 (VBR 12.0) spec, the only `/restore/*` paths are `instantRecovery/vmware/vm{...}`, `instantRecovery/vmware/fcd{...}`, and `vmRestore/vmware/`. **There is no disk-level restore endpoint.** `GET /api/v1/objectRestorePoints/{id}/disks` only lists disks, it does not restore one. This directly contradicts the "Restore-flow decisions" entry below, which was based on VBR 13's UI-guide docs. See "Open risk" callout right after this section.
- **Cleanup is partial via REST**: `DELETE /api/v1/jobs/{id}` removes the job definition, but `/api/v1/backups`, `/api/v1/backups/{id}`, `/api/v1/backupObjects/*`, and `/api/v1/objectRestorePoints/*` are **GET-only** in 1.1-rev0 — there is no REST call to delete a backup or a restore point. Whether deleting the job also deletes its backup files (vs. orphaning them on the repository) is unverified and must be checked directly against the appliance.
- **Auth mechanics, precisely**: `POST /api/oauth2/token` requires the `x-api-version` header even though the endpoint itself is unauthenticated (`security: []` in the spec) — this is a common integration mistake worth calling out explicitly in whatever client we write. Body: `grant_type=password&username=<u>&password=<p>` (form-encoded) → `{access_token, token_type, refresh_token, expires_in}`. **`expires_in` is 900 seconds (15 minutes)** — a VM restore can plausibly take longer than that, so the client MUST implement the refresh-token flow (`grant_type=refresh_token`) rather than assuming one token lasts a whole test. `POST /api/oauth2/logout` ends the session.
- **Version discovery is circular, and community version-mapping tables disagree.** The on-appliance Swagger document itself lives at a version-scoped path (`/api/swagger/v1.1-rev0/swagger.json` style) — you cannot fetch "the" spec without already knowing (or guessing) the version segment; the practical use is the human-facing Swagger UI page, which lists what the appliance actually supports. `GET /api/v1/serverInfo` returns `buildVersion`, which can be mapped to a revision — but two VeeamHub sources disagree on that exact mapping (the SDK's own spec says VBR 12.0 = `1.1-rev0`; a PowerShell script in the same org's `powershell` repo comments VBR 12 = `1.2-rev0`, likely reflecting a later 12.x patch). **Do not trust a hardcoded version-to-build table; query the appliance's own `serverInfo`/Swagger UI at connect time.** There is also no version-specific error code in the `Error` schema — version negotiation by trial-and-error has to key off HTTP status/message text, not a structured error code.
- **VBR 12 minor versions may add REST surface.** The 1.1-rev0 spec's job `type` enum is `["Backup"]` only, yet a VeeamHub PowerShell script pins `1.1-rev1` and queries a `CloudDirectorBackup` job type — meaning the REST surface changed within the VBR 12 line. Everything above about "not available" is proven only for 1.1-rev0/VBR 12.0; it must be re-verified against whatever `buildVersion` the actual appliance reports once the spike ticket (`vmop-4030`) gets the REST API reachable.

### Open risk: disk-only restore may need a design change

The "Restore-flow decisions" section below picked Veeam's "virtual disk restore" UI feature for the disk-only scenario, based on VBR 13's user guide. The VeeamHub evidence above says that feature has no REST endpoint in VBR 12.0's own spec. Until `vmop-4030` confirms our appliance's actual `buildVersion` and re-derives its Swagger spec, treat the disk-only restore acceptance criteria in `spec.md` as provisional. Fallback options if REST genuinely can't do disk-level restore on our appliance: (a) use REST's FCD instant-recovery endpoints (`POST /api/v1/restore/instantRecovery/vmware/fcd`, then `/migrate` to relocate it to a datastore) as the closest REST-native approximation, or (b) do a full VM restore via REST to a throwaway VM and then move/detach the one disk in vSphere — which reintroduces some of the manual-disk-manipulation mimicry this feature is meant to eliminate, so it should be a last resort, not the default plan.

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
- **Disk-only restore**: use Veeam's dedicated "virtual disk restore" action — https://helpcenter.veeam.com/docs/vbr/userguide/virtual_drive_recovery.html?ver=13 — which restores a single virtual disk directly. Rejected alternative: a full VM restore with everything except the target disk discarded, which would produce extra restore artifacts to track and clean up for no benefit. **Reopened**: this decision assumed VBR 13's documented UI feature has a REST equivalent; VeeamHub evidence for VBR 12.0's actual REST spec suggests it may not (see "Open risk" above). Treat as provisional until spike `vmop-4030` confirms against the real appliance.

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
- Still not yet investigated against our actual appliance (candidates for follow-up spikes before `plan.md`; several of these are now answered *in general* by the VeeamHub evidence above but need confirming against our specific `buildVersion`):
  - Our appliance's exact `x-api-version` and `buildVersion` (via `GET /api/v1/serverInfo` once reachable), and whether that build's REST surface matches the 1.1-rev0 spec vendored in `veeam-vbr-sdk-go` or a later 12.x revision with more endpoints.
  - Whether our appliance's REST API genuinely has no disk-level restore endpoint (per the 1.1-rev0 evidence) — if confirmed, `spec.md`'s disk-only restore acceptance criteria need to pick one of the fallback approaches noted above instead of "virtual disk restore."
  - Whether `DELETE /api/v1/jobs/{id}` also removes the job's backups/restore points on our appliance, or orphans them — determines whether the cleanup goal in `spec.md` is achievable via REST alone or needs a documented manual/PowerShell fallback.
  - Whether the Swagger/discovery endpoint is reachable from the E2E runner's network path or only from `localhost` on the VBR appliance itself (see the 403-for-non-localhost forum report noted above) — if it's `localhost`-only by default, the version-autodetection goal needs the try-newest/bootstrap-with-`serverInfo` strategy above rather than parsing the Swagger UI remotely.
  - How VBR's REST API reports job/task failure detail in practice (session `result`/`state` fields plus `GET /api/v1/sessions/{id}/logs`) — confirm this is enough to satisfy the "surface Veeam's own error" goal without needing to scrape the Veeam console.

## Prior art referenced

- `docs/guides/backup-restore/README.md` — the in-repo backup/restore guide already documents the intended production workflow (Sections 3–9) that this feature is meant to make the E2E suite actually exercise, including the restore-type detection logic (Section 4.3) and the hard limitations (Section 7) that any Veeam-driven restore must still respect (e.g., duplicate BIOS UUID, missing ExtraConfig, wrong namespace folder).
