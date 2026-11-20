# Integration and E2E Test Plan: Flex VM Classes

- **Status**: Draft; implementation and execution remain outstanding.
- **Design**: [tds.md](tds.md), scenarios F01–F52 (52 IDs, all active; F07 covers T122 and is pending #2153).
- **Acceptance criteria**: [spec.md](spec.md); delivery tickets and dependencies: [tasks.md](tasks.md).
- **Epic**: vmop-4034; Milestone 1 implementation: vmop-4038/4039/4201/4040/4041; Milestone 2: vmop-4312.

## 1. Integration suite structure

Extend the owning package's `_test.go` and `_suite_test.go`; use external `_test` packages, Ginkgo/Gomega and `pkg/constants/testlabels`. Do not introduce `_unit_test.go`/`_intg_test.go` splits from older examples. Use `test/builder` suite/context builders and cleanup hooks. Use `testlabels.EnvTest` for API-server integration and `testlabels.VCSim` only when a simulator actually backs the test, alongside the existing API/controller/webhook labels.

| Surface | Planned coverage | Fixture and assertion boundary |
|---|---|---|
| `api/v1alpha6`, older conversion packages, `api/utilconversion` | F01–F05, F07, F08, F26, F43, F45, F47, F50–F51 | Generated CRDs in envtest, real conversion webhook, all served old versions and storage read-back. Unit tables cover value shapes; envtest must exercise every CEL rule. Generated schema/field-mapping checks run in API tests; fuzz and annotation-size round trips (F51) run in `api/test`. |
| `webhooks/virtualmachineclass/{mutation,validation}` | F01–F06, F25–F28, F35–F36, F38–F40, F41, F43, F48 | Real API Create/Update through both webhooks, seeded namespace-local classes, zones and provider profile objects; verify stored defaults and no mutation after rejected writes. |
| `webhooks/virtualmachine/{mutation,validation}` | F07, F09–F15, F18, F21, F25, F27, F29–F30, F38–F41, F43–F44, F46, F48 | Install both webhooks in one admission path so validation sees effective defaults; vary caller identity and subresource. Seed image configuration explicitly. |
| `controllers/virtualmachineclass`, `controllers/infra/zone` | F07, F19, F21–F22, F24, F42, F45 | Start managers and drive changes through API writes to prove watches, status convergence and conflict retries. Environment-browser objects are fixtures, not real hardware evidence. |
| `pkg/providers/vsphere`, `controllers/virtualmachine` | F07, F09, F13–F14, F17–F20, F30–F31, F44, F46–F47, F49, F52 | Envtest plus vcsim for VM config/power observations; instrument power calls and inject conflicts. An unsupported simulator operation needs a controlled provider test plus real E2E evidence, not a false simulator assertion. |
| `pkg/providers/vsphere/placement` | F16 | Record candidate zones and group intersection at the provider boundary. Pin the actual chosen zone only where the eligible set is a singleton. |
| `controllers/vim`, `controllers/virtualmachineconfigoptions` | F21–F24, F34, F42, F45 | Existing environment-browser fixtures with controlled cluster changes and backend call counts; include co-owned options and conservative cluster selection. |
| `pkg/config/capabilities`, `pkg/config`, manager/admission integration | F33–F34 | Shared gate off/on/off in the same process; schema presence and behavior are separate assertions. Verify removal in installed manifests and controller behavior. |
| New governance package under `pkg/` | F02, F06, F11, F15–F18, F22, F35–F36, F38–F41, F43–F44, F46, F48 | Table-driven unit boundary tests plus integration through each public consumer (admission, reconcile, placement). Agree package path with T200; do not create a second governance implementation in test helpers. |

Use namespace-scoped, independently created fixtures per test. For controllers, wait for the relevant generation/status before changing a dependency, and then observe the changed result without explicitly calling Reconcile: this proves watch wiring. Direct reconcile tests supplement manager-driven integration for deterministic injected errors. Restore feature config in cleanup and do not run cases that mutate process-wide configuration concurrently. API conflict tests must introduce a real resourceVersion mismatch or controlled conflict and verify the final unrelated user field is preserved.

## 2. E2E layout, prerequisites and fixtures

Follow `test/e2e/README.md`, the `Spec`/`SpecInput` pattern in `test/e2e/vmservice/vmservice/configpolicy/configpolicy.go`, and registration in `test/e2e/vmservice/vmservice_test.go`. All Flex VM Classes specs live in one feature package, `test/e2e/vmservice/vmservice/flexvmclasses/`, registered once as `FLEX-VM-CLASSES` with a shared `SpecInput`; each story adds its own file (for example `vm_api.go`, `class_resources.go`, `upgrade.go`) instead of a story-specific context. Extend existing `virtualmachine/` lifecycle tests only to keep them passing on the new schema. The staged upgrade spec (`upgrade.go`, `FLEX_UPGRADE_PHASE=prepare|verify`) is shared by the whole initiative: each story adds a legacy fixture case to its table rather than a new upgrade test. Preserve environment-browser coverage from `configpolicy/` while removing obsolete policy assertions. Reuse `lib/vmoperator`, `manifestbuilders` and `infrastructure/vsphere`; add typed v1alpha6 fixtures without changing the API version emitted by existing builders.

Every spec that adds new behavior is paired with a check that the existing behavior still works: legacy classes created through the VI admin API, older served API versions as readers and writers, objects created before an operator upgrade, and a vCenter that has not been upgraded writing through its old client. Neither half alone counts as coverage for a story.

Use the `flex-vm-classes` feature label, document it in the E2E README, and add the suite's tier label. All new/significantly changed tests start with `experimental`; remove that label only after real WCP validation. Capability names belong in `consts/consts.go`, with `skipper.SkipUnlessSupervisorCapabilityEnabled` for positive cases using the shared capability from vmop-4313. The negative/off and toggle tests must not use an enabled-only skip guard. Quarantine is a pipeline property distinct from `experimental`: the dedicated pipeline must explicitly select experimental Flex tests during validation rather than silently excluding the whole suite.

| Lane | Required environment | Scenarios / evidence |
|---|---|---|
| Single-zone core | Flex-capable Supervisor, image with known CPU/memory metadata, storage/network, admin and namespace identities, two selectable classes and one governor | F01/F02 representative admission, F05–F06, F09–F14, F17–F19, F22–F23, F39–F41 and F43–F44 representatives, F46 (custom shares visible in vCenter), F48–F49 representatives. Store admitted specs and compare powered VM CPU/memory. |
| Multi-zone extended | At least two usable zones; fixtures for same, distinct and missing governors; known compatibility differences | F04, F15–F16, F21–F22, F42, F45 zone maximum. Assert namespace/placement eligibility and actual VM zone. |
| Devices extended | Available vGPU and DirectPath devices, zone-specific profile inventory, supported observed fields | F20–F21, F25, F38, F52 (blocked on T020). Compare real device configuration and reservation, not just admission success. |
| Reserved/provider extended | Provider profile publication, real reserved allocations, v2/legacy class APIs and namespace association support | F27–F32. Preserve real copies and backend profile ID evidence; do not substitute directly written namespace CRs for provider-copy cases. |
| Upgrade extended | Supported pre-upgrade Supervisor with legacy fixed/instance-storage/reserved classes and live VMs, then upgrade to candidate build | F03–F04, F31, F34, F47 (existing High or Custom shares survive), F51 conversion slice. Capture before/after IDs/specs/power state and exercise post-upgrade lifecycle; creating an old-version CR on the new release is only a conversion test. |
| Capability extended | Dedicated testbed with authorized capability control | F33 off/on/off without pod restart. Check pod UID/restart count and fresh behavior after each transition. Restore original capability even after failure. |
| Tenant extended, M2 only | Both capabilities, provider parent-reference support and approved family design | F35–F37. Keep separate from Milestone 1 pass-rate and enablement. |

E variants that need a class with real min/max ranges (F01, F03, F05, F09, F11, F43, F44, F45 and F51) are blocked on T912 (vmop-4327): no supported path authors range classes until the v2 class API. Legacy VI admin fixtures, which convert to fixed ranges, are not blocked. Integration coverage is unchanged.

Create a unique namespace and class/profile fixture names where the provider permits. Never narrow or delete a preexisting shared governor. Capability changes and any unavoidable shared hardware configuration use isolated/serialized lanes with snapshot-and-restore cleanup. Register cleanup immediately after each successful allocation. Delete test VMs and wait for backend deletion before removing class associations, profiles/reservations and namespace; let the owning provider release capacity. Keep diagnostics on failure before cleanup. A cleanup failure must be reported, not hidden by an otherwise passing assertion.

## 3. Delivery order and ownership

These batches refine existing tasks, not new completion claims or replacement tickets. Each behavior story includes its integration and user-visible E2E coverage in the same deliverable. Do not defer all E2E implementation to the final enablement story.

| Batch | Existing owner / task range | Scenario families | Exit evidence |
|---|---|---|---|
| 1. Harness, shared gate, schema and conversion | vmop-4313–4319, vmop-4479 (shares), vmop-4481 (VM CPU in cores), vmop-4428 (vmx-23 CPU fields, blocked on a vSphere/govmomi release); T100–T121 | F01–F05, F24, F26, F33–F34, F45–F47, F49–F52 | Real schema/CEL/conversion tests, including shares (T120, T121); pipeline demonstrably selects new specs; upgrade and off/on test fixtures prepared. F49 and the T022 variants wait for vmop-4481; the vmx-23 variant of F45 waits for vmop-4428; F52 VM-side variants wait for T020. |
| 2. Governance and class status | vmop-4320–4323; T200–T205a (T205 superseded by T122, pending #2153) | F02, F07, F11, F15–F16, F21–F24, F42, F45 | Engine boundaries plus manager/watch integration; per-zone E2E status checks. Consumer E2E follows its implementing batch below. |
| 3. Class admission and schema mapping | vmop-4324–4326; T300–T308 | F01–F06, F08, F25, F28, F38–F41, F48 | Combined mutation/validation API tests and class E2E; mapping/schema negative checks. |
| 4. VM admission, resize and reservation | vmop-4330–4334; T400–T411 | F09–F15, F25, F27–F31, F38–F41, F43, F46 | Admission, vcsim configuration and corresponding real VM flows. Reserved cases depend on T910, not fabricated publication. |
| 5. Compliance, placement and M1 enablement | vmop-4335–4338; T500–T506 | F16–F20, F33, F44 plus all M1 regression lanes | Power-race protection, real placement, complete M1 sweep and evidence reviewed before capability enablement. |
| Provider coordination alongside batches 1–5 | T910–T916, vmop-4327–4329, vmop-4392–4393 | F04–F05, F08, F28, F32 | Provider-owned API/copy/generation checks plus real cross-component E2E. Publication/backfill and capability advertisement block enablement. |
| 6. Tenant classes after M1 | vmop-4339–4343; T600–T607 | F35–F37 | Parent/family integration and E2E under dependent capability; separate tenant enablement sweep. |

## 4. Running and recording validation

For implementation PRs, run the affected packages first, with envtest assets available (the repository `make test` target provisions its test dependencies). Examples once tests exist:

```bash
go test ./webhooks/virtualmachineclass/... ./webhooks/virtualmachine/...
go test ./controllers/virtualmachineclass/... ./controllers/infra/zone/...
go test ./pkg/providers/vsphere/...
make test
```

Include API/conversion and other touched package tests from the surface table. Run affected nested modules with their own module context; the root sweep alone does not certify them. Follow the repository toolchain and test setup rather than assuming locally installed envtest binaries match the CRDs.

Configure a real testbed per the E2E README, then select the proposed feature label after implementation:

```bash
source ./hack/e2e/setup-testbed-env.sh <testbedInfo.json-or-URL> --e2e
make test-e2e LABEL_FILTER="flex-vm-classes && experimental"
make test-e2e LABEL_FILTER="flex-vm-classes"
```

Run lane-specific selections on the appropriate testbeds; capability-off tests need their dedicated lane. The second selection must include all required enabled M1 cases when used for enablement. A successful exit with zero selected tests or capability/hardware skips is not coverage.

For each F-ID and parameter variant record implementation PR/commit, package or E2E name, lane, versions/capabilities, pass/fail/blocked/skipped status, and artifact location. On failure retain Ginkgo/JUnit output, object spec/status/generation snapshots, relevant events, operator logs and backend config/power/zone observations; include testbed topology and provider version. Capture credentials only through existing redacted diagnostics. Triage whether the failure is a product defect, test defect, environment prerequisite or unresolved contract; never weaken an assertion to match a bug.

## 5. Completion criteria

- Every applicable Milestone 1 row (F01–F34 and F38–F52) has traceable implemented variants and passing integration checks; required E2E lanes have actual execution evidence, including negative tests and cleanup.
- The schema and shared gate are tested independently; capability off/on/off, upgrade and both resize feature flags are covered.
- Hardware-dependent and provider-dependent results are backed by their real lanes. Remaining blocked spec variants have an explicit decision/owner and disposition before enabling affected behavior; skips and retries are reported separately from passes.
- Production-facing tests ship with their stories. Real WCP validation precedes removing experimental labels; vmop-4337 removes quarantine only after the full M1 sweep and provider prerequisites are satisfied.
- Milestone 2 F35–F37 and unresolved branch behavior are tracked separately under vmop-4312, with their own capability and enablement gate.

This documentation PR validates the design's links, scenario/task references and diff only. It does not implement tests or claim the feature meets these criteria.
