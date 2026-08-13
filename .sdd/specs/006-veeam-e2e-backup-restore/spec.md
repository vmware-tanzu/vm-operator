# Feature Specification: Use Veeam for Backup/Restore E2E Tests

- **Feature branch**: [`features/use-veeam-for-backup-restore-tests`](../../../)
  - **Fork**: `vmware-tanzu/vm-operator`
  - **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-08-07
- **Status**: Draft
- **Epic**: vmop-4013
- **Spike (open)**: vmop-4030 — confirm the existing VBR appliance's REST API is enabled/reachable before implementation begins; see [`research.md`](./research.md) "Spike: enable and confirm the VBR REST API on the existing appliance."

---

## Summary

The E2E suite that exercises VM Service backup/restore and manual VM registration does not exercise a real VADP backup vendor today. Each restore workflow it claims to cover is instead hand-mimicked directly against vSphere/CNS:

- **Restore to new**: the Kubernetes representation of the VM is deleted while the underlying vSphere VM and its recorded state are left behind, to fake "the VM's Kubernetes object is gone but the VM was restored on vSphere."
- **Restore to existing (in-place)**: the VM is paused and the state VM Operator itself recorded on the vSphere VM is directly overwritten to an older value, to fake "an older backup was restored over the current VM."
- **Disk-only restore**: a virtual disk is relocated on the datastore and its cloud-native storage identity is manually stripped, to fake "a backup vendor restored just this one disk."

Because none of these steps involve an actual backup/restore product, the tests have occasionally been shaped around the mechanics of the mimicry (specific device add/remove ordering, manually re-deriving storage inventory, manually keeping recorded state in sync) rather than around what a real VADP vendor actually produces on restore. Those mechanics do not occur, and would not need workarounds, in production.

This feature replaces the mimicked backup/restore steps with real operations performed through Veeam Backup & Replication (VBR), driven by its REST API. Everything the suite already does to validate VM Operator's own behavior — waiting for backup state to be recorded, invoking the VM registration API, and verifying the resulting Kubernetes state after registration — is unaffected and continues to be used as-is. Only the steps that fabricate "a backup was taken" or "a restore happened" are replaced with the genuine equivalent.

Background on the current mimicry, the VBR REST API, and prior art referenced while drafting this spec is recorded in [`research.md`](./research.md).

---

## Goals

- E2E MUST create a real, dedicated Veeam backup job scoped to the specific test VM, rather than relying solely on VM Operator's own continuous state recording to simulate "a backup exists."
- The backup job's name MUST be derivable from the CI pipeline run and the target VM name, so a job found in Veeam can be traced back to the E2E run and VM that created it.
- E2E MUST trigger a real backup run of that job and wait for the backup to reach a successful terminal state before proceeding, replacing the current practice of treating VM Operator's own recorded state as the sole signal that "a backup exists to restore from."
- For the **restore-to-new** scenario, E2E MUST use a real Veeam restore operation that produces a VM in the Supervisor namespace folder with no matching Kubernetes object, in place of the current mimicry of deleting the Kubernetes object while leaving the vSphere VM and its recorded state untouched.
- For the **restore-to-existing (in-place)** scenario, E2E MUST use a real Veeam in-place VM restore in place of the current mimicry of directly overwriting the VM's recorded backup state to an older value.
- For the **disk-only restore** scenario, E2E MUST use a real Veeam operation that restores a single virtual disk without restoring the whole VM, in place of the current mimicry of relocating a virtual disk and manually stripping its cloud-native storage identity. Which specific Veeam operation satisfies this (virtual disk restore vs. an FCD instant-recovery-and-migrate sequence) is not yet confirmed to be available via REST on the target appliance — see the open question below.
- After each real Veeam restore, E2E MUST invoke the existing manual VM registration workflow against the restored VM, exactly as production operators are expected to when automatic registration is not used.
- After registration completes, E2E MUST verify the resulting Kubernetes state using the suite's existing verification behavior (VM existence and power state, PVC/volume counts and naming, no errors surfaced) — this verification behavior is reused unchanged; only the mechanism that produces "a restored VM to register" is in scope for this feature.
- E2E MUST clean up the temporary Veeam backup job it created — and any restore points or artifacts that job produced — whether the test passes or fails, so repeated CI runs do not accumulate orphaned Veeam jobs. If deleting the job does not, by itself, remove its backups/restore points on the target appliance, cleanup MUST take whatever additional step is needed to remove them rather than leaving orphaned backup data behind.
- The pipeline MUST accept the target Veeam server's address as a parameter, defaulting to the pre-existing Veeam appliance used for manual testing today when the parameter is not supplied, so a run against a different Veeam instance does not require a code change.
- The pipeline MUST accept Veeam server credentials as a parameter, defaulting to the pre-existing appliance's known credentials when not supplied.
- E2E test tooling MUST detect, at connection time, which Veeam REST API version the configured server actually supports and use the corresponding request shapes, rather than assuming a single hardcoded API version — so the test suite keeps working across a Veeam appliance upgrade without a code change, and does not create a hard version coupling between the test pipeline and the Veeam appliance.
- Tests MUST skip (not fail) with a clear message when the configured Veeam server is unreachable or does not expose a REST API version the tooling knows how to speak, mirroring the suite's existing skip-on-missing-capability conventions.
- Failures returned by Veeam (job creation, backup run, restore run) MUST surface Veeam's own job/task identifier and reported error/status in the test failure message, so a failing E2E run is diagnosable from CI logs without needing interactive access to the Veeam server.

## Non-goals

- Testing Veeam Backup & Replication itself (its scheduling, retention, replication, or SureBackup features).
- Provisioning, installing, or upgrading the Veeam server itself. A VBR appliance already runs (on a Windows Server VM) for manual use today; this feature only wires the existing pipeline to reach it (as a parameterized address/credentials pair) and does not change how that appliance is deployed or maintained.
- Automatic registration (the platform's own background detection of a restored VM). This feature only changes how a backup/restore is *produced*; the existing tests that invoke manual registration continue to do so. Automatic-registration E2E coverage is a separate concern.
- Cross-vCenter failover restore coverage.
- Application-consistent / guest-quiesced backups, VSS integration, or pre/post backup scripts.
- Support for any VADP vendor other than Veeam.
- Changes to VM Operator's product behavior. This is an E2E-only change; if the work below uncovers an actual product defect, that defect is fixed in the same change set and called out explicitly rather than worked around in the test.

---

## User stories / acceptance criteria

### Platform engineer — restore-to-new via real Veeam backup/restore

- **Given** a VM Service VM with a real Veeam backup job and at least one successful backup run, **when** the E2E suite triggers a real Veeam restore that produces a VM in the Supervisor namespace folder without a matching Kubernetes object present, **then** invoking manual VM registration against that VM succeeds and produces the same post-registration state that the current mimicked test asserts.
- **Given** the Veeam restore operation fails or times out, **when** the E2E test observes this, **then** the test fails with an error message that includes Veeam's job/task identifier and Veeam's reported failure reason, not just a generic timeout.

### Platform engineer — restore-to-existing (in-place) via real Veeam backup/restore

- **Given** a VM Service VM with an existing Kubernetes object and two successful Veeam backup runs (an older and a newer restore point), **when** the E2E suite triggers a real Veeam in-place restore to the older restore point, **then** the VM's recorded backup state on vSphere reflects the older restore point as a side effect of Veeam's own restore (not a hand-edited overwrite), and invoking manual VM registration against the existing VM succeeds, producing the same post-registration state the current mimicked test asserts (dangling volumes cleaned up, new restored volumes present).
- **Given** the in-place restore is still in progress on Veeam's side, **when** manual VM registration is invoked too early, **then** the test either waits for Veeam's restore operation to reach a terminal state first (preferred) or captures and reports the resulting registration failure without masking it as a test infrastructure error.

### Platform engineer — disk-only restore via real Veeam backup/restore

- **Given** a VM Service VM with a persistent-volume-backed disk captured in a Veeam backup, **when** the E2E suite triggers a real Veeam operation scoped to that single disk (not the whole VM), **then** the disk's cloud-native storage identity is removed as an authentic side effect of Veeam's restore (not a manual workaround step), and invoking manual VM registration re-registers exactly that disk as a new restored volume, matching the count and shape asserted by the current mimicked test.

### QA / CI maintainer — traceable, self-cleaning Veeam artifacts

- **Given** an E2E pipeline run creates a Veeam backup job for a specific test VM, **when** the job is inspected in the Veeam console or via its API, **then** its name unambiguously identifies the CI pipeline run and the VM it targets.
- **Given** an E2E test using Veeam completes (pass or fail), **when** the test's cleanup runs, **then** the Veeam backup job it created is deleted, and no restore point or job artifact is left behind for that run.
- **Given** the configured testbed has no Veeam server available, **when** a Veeam-backed E2E test runs, **then** the test is skipped with a message naming the missing configuration, not failed.

---

## Edge cases

- Two E2E runs targeting the same testbed concurrently must not collide on Veeam job names; job names must incorporate a value unique per run (e.g., pipeline run ID) in addition to the VM name.
- A Veeam restore-to-new that targets a VM name already present in vSphere (but with no matching Kubernetes object) must not be confused with a duplicate-identity conflict; the platform's existing hard limitation on duplicate VM identity within a namespace still applies and is not something this feature works around.
- If Veeam backup job creation succeeds but the backup run itself never starts (e.g., backup proxy exhaustion), the test must fail with a clear "backup run did not start" signal rather than hanging until the outer test-framework timeout.
- Cleanup of the Veeam backup job must be attempted even when an assertion earlier in the test fails.
- If the Veeam server is reachable but authentication fails (expired credentials, revoked API key), tests must skip or fail with a message that distinguishes "Veeam auth failure" from "Veeam unreachable" from "restore logic failure," so CI triage isn't spent chasing the wrong layer.

---

## Out of scope

- Any change to VM Operator's product code or the platform's VM registration implementation.
- Veeam job scheduling, retention policies, backup copy jobs, or scale-out repository configuration.
- A general-purpose, reusable Veeam API client as a product/library deliverable — any client tooling produced here is test-only.
- Load/scale testing of Veeam itself (number of concurrent jobs, throughput).
- Windows-guest-specific VSS/application-aware backup validation.

---

## Open questions

- [NEEDS CLARIFICATION: A connectivity check against the existing VBR appliance found port 9419 — VBR's default REST API port — refused, along with several other candidate ports; only RDP and the Veeam Installer Service port are open. This means the REST API is not currently reachable on that appliance. Tracked as spike vmop-4030; see `research.md` for the full connectivity finding and what needs to be checked/enabled on the VM. This blocks every other Veeam-specific investigation item below and must be resolved before `plan.md` can commit to a concrete client design.]
- ~~Is a Veeam server already provisioned...~~ **Resolved**: a VBR appliance already runs on a Windows Server VM and is used manually today. The pipeline will be extended to accept its address as a parameter (defaulting to that known instance) rather than provisioning a new one.
- ~~Which VBR release and REST API version should be targeted...~~ **Resolved**: the appliance in use today runs VBR 12, but the version MUST NOT be hardcoded — see the new version-autodetection goal above. `research.md` needs a follow-up investigation into what VBR 12's REST API actually exposes (VBR 12 predates the `1.3-rev2` surface documented for VBR 13) and how to detect API version/capability at connect time.
- ~~How are Veeam service-account credentials distributed...~~ **Resolved**: credentials are a pipeline parameter, defaulting to the existing appliance's known credentials. No Kubernetes Secret plumbing is required for this feature; this is a lower-security-bar internal test appliance.
- ~~Does "restore to new" mean Veeam's "restore VM" flow or an "instant recovery" flow?~~ **Resolved**: use Veeam's standard "Restore VM" action, targeting a new VM identity in the Supervisor namespace folder. This matches the acceptance criteria already written above.
- [NEEDS CLARIFICATION: Should disk-only restore use Veeam's "virtual disk restore" UI feature, or an FCD instant-recovery-and-migrate REST sequence? Previously marked resolved in favor of "virtual disk restore," but a survey of Veeam's own vendored VBR 12.0 REST spec (`github.com/veeamhub/veeam-vbr-sdk-go`) found no disk-level restore endpoint in that spec — only `instantRecovery/vmware/{vm,fcd}` and `vmRestore/vmware/`. This may be specific to VBR 12.0 (12.x point releases have added REST surface before); it must be re-confirmed against our actual appliance's `buildVersion` under spike `vmop-4030` before `plan.md` commits to an approach. See `research.md` "Open risk: disk-only restore may need a design change."]
- [NEEDS CLARIFICATION: Does `DELETE /api/v1/jobs/{id}` remove a job's backups and restore points on our appliance, or only the job definition (orphaning backup data)? Veeam's vendored VBR 12.0 REST spec exposes no delete operation for backups or restore points. If job deletion doesn't cascade, the cleanup goal above needs a concrete fallback mechanism, to be confirmed under spike `vmop-4030`.]

---

## Review & acceptance checklist

- [ ] Every mimicked step currently in the E2E suite (deleting the Kubernetes VM object while leaving the vSphere VM behind, hand-overwriting recorded backup state, relocating and re-identifying a virtual disk) has a corresponding real-Veeam replacement described above.
- [ ] Verification behavior that already validates VM Operator's own behavior (waiting for backup state, invoking registration, verifying post-registration state) is explicitly called out as reused, not replaced.
- [ ] Job naming/traceability requirement is testable (a human can map a Veeam job back to a CI run and VM).
- [ ] Cleanup behavior is specified for both pass and fail outcomes.
- [ ] Skip-vs-fail behavior is specified for missing/unreachable/unauthenticated Veeam server.
- [ ] All open `[NEEDS CLARIFICATION]` items are either resolved or explicitly blocking `tasks.md` until answered, per [`sdd-standards.md`](../../memory/sdd-standards.md).
- [ ] Out-of-scope items are listed, including the explicit "no product code changes" boundary.
