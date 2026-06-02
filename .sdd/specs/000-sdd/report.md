# Gap Analysis Report: VM Operator vs `.sdd/memory` Standards

- **Date**: 2026-05-28
- **Repo HEAD**: `vm-operator` `main` at the audit timestamp
- **Auditor**: AI-assisted manual review, augmented by `make lint-go-full` and direct repo introspection
- **Scope**: every standard documented under [`.sdd/memory/`](../../memory/) compared against the current state of the codebase

---

## Summary

Overall compliance is **strong**. The standards landed by the `000-sdd` bootstrap are mostly aspirational, and the codebase already satisfies the vast majority of what they codify. The handful of real gaps cluster around three themes:

1. **Test-naming migration** (the only true mass migration on the table) — 76 files still use the legacy `_unit_test.go` / `_intg_test.go` split that the constitution explicitly forbids, while `testing-standards.md` itself still describes that legacy split. This is the single largest gap and it's internally inconsistent: the constitution and the operations manual disagree.
2. **A handful of import-alias misses** — `make lint-go-full` against the root module surfaces **11 occurrences in 3 files** of `pkgcnd` where the linter requires `pkgcond`. All five other Go modules linted with 0 issues. Auto-fixable via `make fix`.
3. **Minor documentation drift** — three "not yet linter-enforced" aliases that `architectural-standards.md` claims are aspirational are in fact already in `.golangci.yml`. The constitution's repository-layout sections also miss a top-level directory (`api-docs/`) and five `go.mod` modules.

There are **no Blocker-severity findings**. Everything is either a documentation refresh, an auto-fix, or a tracked migration.

The bulk of the rules that no tool checks — controller thinness, finalizer naming, copyright headers, comment grammar, no-direct-vSphere-from-controllers, soft-wrap Markdown, no-internal-URL-leakage — are **compliant** under spot-check.

---

## Audit methodology

This report is **not** just lint output. The linter is one of three inputs:

| Method | What it checks | Covers |
|--------|----------------|--------|
| **Tool-enforced** (`make lint-go-full` and the sibling sub-module lints) | Everything wired into `.golangci.yml`: `importas` aliases, `depguard` forbidden imports, `goimports.local-prefixes` import grouping, `goheader` copyright, `godot` comment punctuation, `revive`, `gocritic`, ~50 other linters. | About a third of the rules. |
| **Static repo introspection** (`git ls-files`, `Grep`/`rg`, `wc`, directory walks) | Counts and patterns: test-file naming, `.cursor/rules/*.mdc` proxy size, internal-URL leakage, finalizer-name constants, layout vs the constitution's canonical tree, max-line-length sweeps for hard-wrapped Markdown. | About a third more. |
| **AI manual review** (this auditor reading representative files against the standards text) | Rules no tool implements: doc-vs-doc consistency (constitution ↔ companion files), controller / webhook layout intent, presence of canonical reconcile-loop patterns in controllers, whether the `architectural-standards.md` "not yet linter-enforced" table matches reality, whether `controllers/<sub>` groups satisfy the "no non-vmoperator-group controllers directly in `controllers/`" rule. | The remaining third. |

What this report does **not** cover (and the user should run separately when Docker / time permits — none are blocking):

- `make lint-markdown` — Markdown lint (requires the internal mdlint container image; Docker unavailable in this session).
- `make lint-shell` — shellcheck (same; Docker unavailable).
- `make typos` — typos-cli (same; Docker unavailable).
- `make modules` — writes to `go.mod` / `go.sum` if not tidy; **not run** to avoid destructive change against a clean tree.
- `make generate-go` / `make generate-manifests` — would expose generator drift; **not run** for the same reason.

Severity labels used below:

- **Blocker** — must fix before claiming SDD compliance.
- **Major** — meaningful drift; should fix in a planned change.
- **Minor** — small inconsistencies that can ride along with the next normal PR.
- **Info** — neither a violation nor an action item; recorded for context.

---

## Findings by standard

### A. SDD adoption state

**Method**: directory listing + ripgrep over `.sdd/` and `.cursor/`.

| Item | State | Severity |
|------|-------|----------|
| `.sdd/` exists at repo root | ✅ Present, with `memory/` and `specs/` sub-directories. | — |
| `.sdd/specs/` contains feature directories with `NNN-slug` naming | ✅ Only `.sdd/specs/000-sdd/` exists (the bootstrap). | — |
| `000-sdd/` has the required artifacts (`spec.md`, `plan.md`, `tasks.md`) | ✅ All three present. `research.md` also present. `model.md` absent — correct: the bootstrap does not introduce a data model / API surface. | — |
| `.cursor/rules/*.mdc` are thin proxies (per constitution non-negotiable) | ✅ All seven files are 5–6 lines and contain only the `Follow the rules in ../../.sdd/memory/<file>.md` proxy line. | — |
| No internal Broadcom URLs / JIRA project names in `.sdd/` or `.cursor/` | ✅ `rg 'VMSVC' .sdd/ .cursor/` returns no matches. | — |
| Hard-line-wrapping prohibited in Markdown prose | ✅ Spot-check via `awk` max-line-length per file: every `.sdd/memory/*.md` and `.sdd/specs/000-sdd/*.md` has max-line-length 152–654 chars (i.e. soft-wrapped paragraphs, not hard-wrapped). Code fences inside are short, as allowed. | — |

**Severity: Info.** SDD adoption is in the expected state for a freshly-landed bootstrap: framework in, one spec, no other features yet adopting it (that's tracked under the `vmop-3825` story under the SDD adoption epic).

### B. API tree

**Method**: `ls api/` + grep against `.golangci.yml` `importas`.

| Item | State | Severity |
|------|-------|----------|
| `api/` contains `v1alpha1`..`v1alphaN` for the active alpha series | ✅ `api/v1alpha1`, `v1alpha2`, `v1alpha3`, `v1alpha4`, `v1alpha5`, `v1alpha6` all present. | — |
| The active alpha (aliased as `vmopv1` in `.golangci.yml`) is the newest | ✅ `.golangci.yml` lines 154–161 alias `vmopv1` → `api/v1alpha6`. | — |
| `architectural-standards.md` tree comment uses a version-agnostic placeholder | ✅ Recently updated to say `v1alphaN` with a pointer to `.golangci.yml`. | — |

**Severity: Info.** API tree is internally consistent with the linter SoT.

### C. Repository layout vs the constitution's canonical tree

**Method**: `ls -d */` at repo root compared to the tree literal in `constitution.md` "Repository layout and Go modules".

Actual top-level directories: `api-docs/`, `api/`, `cmd/`, `config/`, `controllers/`, `docs/`, `external/`, `hack/`, `pkg/`, `services/`, `test/`, `webhooks/` — plus `.sdd/`.

| Item | State | Severity |
|------|-------|----------|
| Every directory the constitution names is present | ✅ All present. | — |
| Every actual top-level directory appears in the constitution's tree | ❌ **`api-docs/` is missing from the canonical tree** in `constitution.md`. It exists on disk and has its own `go.mod`. | **Minor** |

**Fix**: add an `api-docs/` line to the canonical tree block in `constitution.md` and add `api-docs/` to the "Sub-modules" table (see Finding L).

### D. Controller layout

**Method**: `ls -d controllers/*/` + `Grep` for `controlledType = &<alias>.Type{}` across every `_controller.go` to discover what API group each controller targets.

#### D.1. Constitution rule: "Controllers for API groups other than `vmoperator.vmware.com` should not be placed directly in the `controllers/` directory."

Top-level subdirectories under `controllers/`:

```
controllers/contentlibrary/      controllers/storage/
controllers/infra/               controllers/util/
controllers/virtualmachineclass/ controllers/virtualmachinegroup/
controllers/virtualmachinegrouppublishrequest/
controllers/virtualmachineimagecache/
controllers/virtualmachinepublishrequest/
controllers/virtualmachinereplicaset/
controllers/virtualmachineservice/
controllers/virtualmachinesetresourcepolicy/
controllers/virtualmachinesnapshot/
controllers/virtualmachine/
controllers/virtualmachinewebconsolerequest/
controllers/vspherepolicy/
```

Mapping non-vmoperator-group controllers to their location (sampled from `controlledType = ...`):

| Path | Controlled type | API group | Compliant? |
|------|-----------------|-----------|------------|
| `controllers/storage/storageclass/` | `storagev1.StorageClass` | `storage.k8s.io` | ✅ grouped under `storage/` |
| `controllers/storage/storagepolicy/` | `infrav1.StoragePolicy` | external `infra` | ✅ grouped under `storage/` |
| `controllers/storage/storagepolicyquota/` | `spqv1.StoragePolicyQuota` | external `storage-policy-quota` | ✅ grouped under `storage/` |
| `controllers/storage/volumeattributesclass/` | `storagev1.VolumeAttributesClass` | `storage.k8s.io` | ✅ grouped under `storage/` |
| `controllers/infra/zone/` | `topologyv1.Zone` | external `tanzu-topology` | ✅ grouped under `infra/` |
| `controllers/infra/validatingwebhookconfiguration/` | `admissionv1.ValidatingWebhookConfiguration` | `admissionregistration.k8s.io` | ✅ grouped under `infra/` |
| `controllers/infra/secret/` | `corev1.Secret` | core | ✅ grouped under `infra/` |
| `controllers/infra/node/` | `corev1.Node` | core | ✅ grouped under `infra/` |
| `controllers/vspherepolicy/policyevaluation/` | `vspherepolv1.PolicyEvaluation` | external `vsphere-policy` | ✅ grouped under `vspherepolicy/` |

**Reading**: every non-`vmoperator.vmware.com` controller is one extra directory level deep (grouped under an API-group-named subdirectory) rather than sitting directly under `controllers/`. The constitution rule appears to be **satisfied via grouping convention**, but the rule's wording is ambiguous about that interpretation.

**Severity: Minor.** Reword the constitution rule to make the grouping convention explicit, e.g. "Non-`vmoperator.vmware.com` controllers MUST live under an API-group-named sub-directory of `controllers/` rather than directly under `controllers/`."

#### D.2. Constitution rule: "No controller may call vSphere APIs directly; use the provider abstraction under `pkg/providers/vsphere/`."

**Method**: `rg -l 'github.com/vmware/govmomi' controllers/` — returned **zero matches**.

**Severity: Info.** Compliant.

#### D.3. Canonical reconcile-loop pattern

`operator-best-practices.md` spells out a specific reconcile-loop shape (deferred patch helper, named `reterr`, `pkgcfg.JoinContext`, typed context, `ReconcileNormal`/`ReconcileDelete` split, `AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager)`).

**Method**: spot-checked four representative controllers — `virtualmachine`, `virtualmachineservice`, `virtualmachinegroup`, `infra/zone`. All four follow the canonical pattern.

**Severity: Info.** Compliant under spot-check; a deeper audit could confirm every controller.

### E. Webhook layout

**Method**: `ls -d webhooks/*/`.

```
webhooks/common/             webhooks/conversion/
webhooks/persistentvolumeclaim/ webhooks/unifiedstoragequota/
webhooks/virtualmachine/         webhooks/virtualmachineclass/
webhooks/virtualmachinegroup/    webhooks/virtualmachinegrouppublishrequest/
webhooks/virtualmachinepublishrequest/
webhooks/virtualmachinereplicaset/
webhooks/virtualmachineservice/
webhooks/virtualmachinesetresourcepolicy/
webhooks/virtualmachinesnapshot/
webhooks/virtualmachinewebconsolerequest/
```

| Item | State | Severity |
|------|-------|----------|
| Every webhook lives under `webhooks/` | ✅ Yes. | — |
| Non-`vmoperator.vmware.com` webhooks are grouped under an API-group subdirectory | ⚠️ `webhooks/persistentvolumeclaim/` and `webhooks/unifiedstoragequota/` are direct children of `webhooks/` but their controlled types are not in `vmoperator.vmware.com`. Same ambiguous constitution wording as D.1. | **Minor** |
| Validators are unexported types and `+kubebuilder:webhook` markers drive registration | ✅ Spot-checked `webhooks/virtualmachine/validation/virtualmachine_validator.go`-style files — convention holds. | — |

**Severity: Minor.** Same remedy as D.1 — clarify the constitution about whether direct children must be `vmoperator.vmware.com`, or whether grouping under an API-group subdirectory is the requirement.

### F. Test naming and structure

**This is the largest gap.**

**Method**: `git ls-files | grep -c` against each naming pattern.

```
*_unit_test.go : 35    ← forbidden by constitution
*_intg_test.go : 41    ← forbidden by constitution
*_suite_test.go: 126   ← allowed (suite bootstrap)
other _test.go : 266   ← canonical
```

The constitution (companion files reference removed for brevity) says:

> - **One test file** per package: `<package>_test.go` (external `_test` package).
> - **One suite bootstrap** per package: `<package>_suite_test.go` containing only the `TestXxx(t *testing.T)` entry-point that calls `RunSpecs`.
> - Do **not** use the old `_unit_test.go` / `_intg_test.go` split — all tests live in `_test.go` and are differentiated **only** by Ginkgo `Label()` decorators.

**76 test files in the repo violate this rule** (35 + 41).

But there is a second-order problem: **`testing-standards.md` itself still documents the old split as the recommended layout**:

> ## Test File Naming
> | Suffix | Purpose | Example |
> |--------|---------|---------|
> | `_unit_test.go` | Unit tests (mocked dependencies, fake client) | … |
> | `_intg_test.go` | Integration tests (envtest with real API server) | … |
> | `_suite_test.go` | Ginkgo suite setup and registration | … |

So the constitution forbids what the companion file recommends. This is **internal doc-vs-doc drift** introduced when the constitutional non-negotiable was written without updating the companion.

**Severity: Major** (largest single source of standards drift; affects 76 files).

**Recommended remedy** (two-PR plan, suitable for a follow-up spec):

1. **Doc PR**: rewrite `testing-standards.md` "Test File Naming" and adjacent sections to describe the single-file pattern (`<package>_test.go` + `<package>_suite_test.go`) and the `testlabels.Controller` / `testlabels.EnvTest` / `testlabels.VCSim` mechanism for distinguishing unit vs integration vs vcsim tests.
2. **Migration PR(s)**: collapse `_unit_test.go` and `_intg_test.go` into `_test.go` per package, preserving the Ginkgo `Label(...)` decoration so the existing label-filter workflows continue to work. This is mechanical but touches many files; should be split into multiple reviewable PRs.

### G. Import aliases

**Method**: `make lint-go-full` against the root module + independent runs of `golangci-lint --fast-only=false` against every other Go module in the linted-module set (`api/`, `api/test/`, `test/e2e/`, `pkg/backup/api/`, `pkg/constants/testlabels/`).

| Module | Net issues after exclusions | Notes |
|--------|------------------------------|-------|
| **`./` (root)** | **3 visible + 8 hidden = 11 occurrences** of `importas: pkgcond` violation | All other linters clean. |
| `./api` | 0 | |
| `./api/test` | 0 | |
| `./test/e2e` | 0 | `importas` and `depguard` are explicitly disabled here by `.golangci.yml` `linters.exclusions.rules`. |
| `./pkg/backup/api` | 0 | |
| `./pkg/constants/testlabels` | 0 | |

The 11 root-module hits all flag the same convention:

> `import "github.com/vmware-tanzu/vm-operator/pkg/conditions" imported as "pkgcnd" but must be "pkgcond" according to config (importas)`

Files (3 visible; the remaining 8 are additional uses, see `--max-same-issues`):

- `controllers/contentlibrary/utils/controller_builder.go:26`
- `pkg/errors/vmicache_not_ready_error.go:14`
- `pkg/util/vmopv1/image.go:25`

**Severity: Minor.** All auto-fixable with `make fix` (which invokes `golangci-lint --fix`). The fix is mechanical alias substitution; no logic changes.

**Side observation worth noting** ➔ see Finding K: `architectural-standards.md` lists `pkgcond` among "aliases not yet in `.golangci.yml`", but in fact `.golangci.yml` lines 183–184 already register it. The 11 violations confirm the linter is enforcing it.

### H. Copyright header

**Method**: `goheader` is enabled in `.golangci.yml` and produced **zero violations** across the root module and all sub-modules.

**Severity: Info.** Compliant.

### I. Comment grammar (sentence terminators, exported-symbol comments)

**Method**: `godot` is enabled in `.golangci.yml` and **zero violations** were reported. `revive` exclusion rules suppress two specific patterns (`should have (a package )?comment` and `exported: comment on exported const`) — those are tolerated by project convention.

**Severity: Info.** Compliant within the enforcement scope. Whether _grammar_ (proper sentences, no jargon, etc.) is fully observed is not measurable by any tool; recommend human review during PR review.

### J. Markdown wrapping in `.sdd/`

**Method**: `awk` max-line-length per file across `.sdd/memory/*.md` and `.sdd/specs/000-sdd/*.md`.

```
.sdd/memory/architectural-standards.md      : max 654
.sdd/memory/commit-message-standards.md     : max 287
.sdd/memory/constitution.md                 : max 483
.sdd/memory/e2e-sync-with-changes.md        : max 447
.sdd/memory/e2e-testing.md                  : max 495
.sdd/memory/operator-best-practices.md      : max 555
.sdd/memory/pull-request-standards.md       : max 246
.sdd/memory/sdd-standards.md                : max 351
.sdd/memory/testing-standards.md            : max 496

.sdd/specs/000-sdd/plan.md                  : max 400
.sdd/specs/000-sdd/research.md              : max 152
.sdd/specs/000-sdd/spec.md                  : max 241
.sdd/specs/000-sdd/tasks.md                 : max 294
```

Every file's prose is soft-wrapped (one paragraph per logical line), well above the 72/80-char threshold a hard-wrap would produce. Code fences inside the docs are kept short (≤ ~80 chars) per the constitution's exception clause.

**Severity: Info.** Compliant.

### K. Linter-config drift in `architectural-standards.md`

**Method**: cross-reference of `architectural-standards.md` "Package Alias Conventions" against the `.golangci.yml` `linters.settings.importas.alias` table.

`architectural-standards.md` claims these three aliases are "not yet in `.golangci.yml`":

| Alias | Package | Actually in `.golangci.yml`? |
|-------|---------|------------------------------|
| `pkgcond` | `pkg/conditions` | ✅ lines 183–184 |
| `pkgconst` | `pkg/constants` | ✅ lines 185–186 |
| `kubeutil` | `pkg/util/kube` | ✅ lines 207–208 |

All three are **already enforced**. The 11 `pkgcnd` lint hits surfaced in Finding G are direct proof — `pkgcond` is enforced and the codebase isn't fully aligned yet.

**Severity: Minor.** Remove the stale "not yet linter-enforced" subsection of `architectural-standards.md` (or invert it into a short note that the linter is now the source of truth for the full list).

### L. Module count

**Method**: `find . -name go.mod` (excluding `hack/tools/bin/` and vendored caches) vs the constitution's "Sub-modules" table.

20 `go.mod` files on disk (root + 19 sub-modules). The constitution lists 14 sub-modules. The **5 missing entries**:

| Path on disk | In sub-modules table? |
|--------------|------------------------|
| `./test/e2e/` | ❌ missing |
| `./external/image-registry-operator/` | ❌ missing |
| `./external/nsx-operator/` | ❌ missing |
| `./external/mobility-operator/` | ❌ missing |
| `./api-docs/` | ❌ missing (and the directory itself is also missing from the canonical tree — Finding C) |

**Severity: Minor.** Update the "Sub-modules" table in `constitution.md` to include these five. Verify each has the correct module path before listing.

### M. Finalizer naming

**Method**: `Grep` for `finalizer\s*=\s*"` (and `FinalizerName` / `Finalizer` constants) across `controllers/`, `pkg/`, `test/`.

Constitution rule: `vmoperator.vmware.com/<resource>`.

| File | Constant value | Compliant? |
|------|----------------|------------|
| `controllers/virtualmachine/virtualmachine/virtualmachine_controller.go:58` | `vmoperator.vmware.com/virtualmachine` (deprecated pair: `virtualmachine.vmoperator.vmware.com`) | ✅ |
| `controllers/virtualmachineservice/virtualmachineservice_controller.go:40` | `vmoperator.vmware.com/virtualmachineservice` (deprecated pair: `virtualmachineservice.vmoperator.vmware.com`) | ✅ |
| `controllers/virtualmachinesetresourcepolicy/.../...:32` | `vmoperator.vmware.com/virtualmachinesetresourcepolicy` (deprecated pair: `virtualmachinesetresourcepolicy.vmoperator.vmware.com`) | ✅ |
| `controllers/virtualmachinepublishrequest/.../...:54` | `vmoperator.vmware.com/virtualmachinepublishrequest` (deprecated pair: `virtualmachinepublishrequest.vmoperator.vmware.com`) | ✅ |
| `controllers/virtualmachinegrouppublishrequest/.../...:39` | `vmoperator.vmware.com/virtualmachinegrouppublishrequest` | ✅ |
| `controllers/virtualmachinegroup/virtualmachinegroup_controller.go:43` | `vmoperator.vmware.com/virtualmachinegroup` | ✅ |
| `controllers/virtualmachinereplicaset/virtualmachinereplicaset_controller.go:71` | `virtualmachinereplicaset.vmoperator.vmware.com` | ❌ **legacy form only**, no migration to the canonical form |
| `controllers/vspherepolicy/policyevaluation/policyevaluation_controller.go:93` | `vmoperator.vmware.com/policy-evaluation-finalizer` | ⚠️ non-standard `-finalizer` suffix |
| `controllers/infra/zone/zone_controller.go:78` | `vmoperator.vmware.com/zone-finalizer` | ⚠️ non-standard `-finalizer` suffix |

**Severity: Minor.**

- `virtualmachinereplicaset` should add a `finalizerName` constant in the canonical form, treat the existing value as the deprecated-migration target (same pattern other controllers use), and have `ReconcileDelete` remove both during cleanup.
- The two `-finalizer`-suffixed names (`policy-evaluation-finalizer`, `zone-finalizer`) are not flagged by any linter and don't break anything, but they drift from the literal `<resource>` form the constitution prescribes. Either tighten the constitution to allow the `-finalizer` suffix, or rename the constants on the next opportunity.

### N. Internal doc-vs-doc consistency

**Method**: AI manual review of `.sdd/memory/*.md` files for statements that contradict each other or the linter.

| Pair | State | Severity |
|------|-------|----------|
| `constitution.md` "Testing" (one test file per package) vs `testing-standards.md` "Test File Naming" (still lists `_unit_test.go` / `_intg_test.go`) | ❌ Direct contradiction. The constitution forbids what the companion describes. | **Major** (same root cause as Finding F) |
| `architectural-standards.md` "not yet linter-enforced" table vs `.golangci.yml` importas section | ❌ Stale (Finding K). | Minor |
| `constitution.md` "Repository layout" canonical tree vs actual top-level dirs | ❌ Missing `api-docs/` (Finding C). | Minor |
| `constitution.md` "Sub-modules" table vs actual `go.mod` count | ❌ Missing 5 modules (Finding L). | Minor |

All four of these doc-vs-reality drifts can be fixed as a small documentation PR; they don't touch product code.

### O. Audit tooling not yet run

The following audits are **available** in the `Makefile` but were **not** run as part of this report — either because they require Docker (not available in this session) or because they write to tracked files and would distort the gap analysis:

| Target | Reason not run | Recommendation |
|--------|----------------|----------------|
| `make lint-markdown` | Requires internal mdlint container image; Docker unavailable. | Run when Docker is available. Useful for confirming Finding J at byte level. |
| `make lint-shell` | Same; Docker unavailable. | Run when Docker is available. |
| `make typos` | Same; Docker unavailable. | Run when Docker is available. |
| `make modules` | Invokes `go mod tidy` in every module; would write to `go.mod`/`go.sum` if any module is not tidy. Destructive in the sense that the diff itself becomes the audit signal. | Run on a clean branch; any non-empty diff is the finding. |
| `make generate-go` / `make generate-manifests` | Regenerates code and CRDs; any non-empty diff is the finding (drift between hand-edits and generators). | Same — run on a clean branch and inspect the resulting diff. |

---

## Severity counts

- **Blockers**: 0
- **Major**: 1 (Finding F / N — the test-naming migration + the testing-standards.md inconsistency)
- **Minor**: 6 (C `api-docs/` in tree, D.1 constitution wording on non-vmop controllers, E webhooks wording, G the 11 `pkgcnd` aliases, K stale alias table, L missing sub-modules, M finalizer drift)
- **Info**: rest

---

## Recommended follow-ups (in priority order)

1. **Land the 11 `pkgcnd → pkgcond` alias fixes** via `make fix` followed by `make lint-go-full` to confirm zero remaining issues. _(Tracked under the SDD adoption epic as a candidate first feature for `vmop-3827` — the "first non-trivial feature merged after the bootstrap" — since it's tiny and gives the team an end-to-end SDD dry-run.)_
2. **Refresh `architectural-standards.md`** to drop the stale "not yet linter-enforced" subsection of "Package Alias Conventions" — those three aliases are already enforced.
3. **Refresh `testing-standards.md`** to align with the constitution's one-`_test.go`-per-package rule and remove the obsolete `_unit_test.go` / `_intg_test.go` table. The constitution and the companion file MUST agree before any large migration starts.
4. **Refresh `constitution.md`**:
   - Add `api-docs/` to the canonical repository-layout tree.
   - Add the 5 missing modules (`test/e2e/`, `external/image-registry-operator/`, `external/nsx-operator/`, `external/mobility-operator/`, `api-docs/`) to the "Sub-modules" table.
   - Clarify the "Controllers for API groups other than `vmoperator.vmware.com` should not be placed directly in the `controllers/` directory" rule to make it explicit that grouping under an API-group-named subdirectory satisfies the rule. Apply the same clarification to the webhooks section.
5. **Plan a test-naming migration spec** (`.sdd/specs/NNN-test-naming-migration/`) covering the 76 affected files. Migrate in reviewable batches by top-level subdirectory (e.g. one PR per `controllers/<resource>/`), preserving Ginkgo `Label(...)` decorators.
6. **Reconcile finalizer-name drift**:
   - Add the canonical `vmoperator.vmware.com/virtualmachinereplicaset` finalizer to `controllers/virtualmachinereplicaset/` with a migration path off the legacy name.
   - Decide whether to allow or disallow the `-finalizer` suffix used in `controllers/infra/zone/` and `controllers/vspherepolicy/policyevaluation/`; update the constitution accordingly.
7. **Run the Docker-gated audits** (`make lint-markdown`, `make lint-shell`, `make typos`) on a workstation where Docker is available; the results are highly likely to be clean given what spot-checks already show, but they are part of the canonical compliance gate.
8. **Run the destructive audits on a clean branch**: `make modules` followed by `git status` (any diff = a module is not tidy); `make generate-go` then `make generate-manifests` then `git status` (any diff = generator drift).

---

## Appendix: data sources

- `make lint-go-full` against `./` — log at `/tmp/vmop-lint-go-full.log` (preserved for the duration of this audit session).
- Per-sub-module `golangci-lint run --fast-only=false` — logs under `/tmp/vmop-submod-lint/*.log`.
- `git ls-files | grep -c` for each test-naming pattern.
- `wc -l .cursor/rules/*.mdc` for proxy-size sanity.
- `awk 'length>max{max=length}'` for max-line-length sweeps in `.sdd/`.
- `Grep` tool for `controlledType =`, `finalizerName =`, and `github.com/vmware/govmomi` against the relevant trees.
- Direct reads of `.sdd/memory/*.md`, `.golangci.yml`, and the `Makefile` to map standards to audit targets.
