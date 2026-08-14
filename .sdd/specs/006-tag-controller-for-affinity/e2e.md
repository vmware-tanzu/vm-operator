# E2E Test Plan: Tag CRD + Tag Controller for Affinity

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Tasks**: [`tasks.md`](./tasks.md) (T031, T032, T033)

This document specifies the end-to-end suite for this feature: the scenarios the implementation must satisfy against a real Supervisor, organized by the user story in `spec.md` each one validates.

## Suite

- `VMAffinityTagSpec`, in `test/e2e/vmservice/vmservice/virtualmachine/vm_affinity.go`, registered from `test/e2e/vmservice/vmservice_test.go` under a `Context("VM-AFFINITY-TAG", ...)` block. `TEST_FOCUS="VM-AFFINITY-TAG"` selects it.
- Every scenario carries `"core-functional"`, except those that need direct `govmomi` access to read a VM's attached vCenter tags, which carry `"extended-functional"` and skip when that access is unavailable.
- Every scenario also carries `"experimental"` per `e2e-testing.md`, dropped once the suite has been validated on hardware (tasks.md T037).
- New config keys in `test/e2e/vmservice/config/wcp.yaml`: `default/wait-vm-affinity-tag-applied` (60s/10s) and `default/wait-tag-cr-deleted` (3m/10s).
- The suite reads `Tag` resources through the `vsphere.policy.vmware.com/v1alpha1` scheme. No change is needed there: `test/e2e/vmservice/common/scheme.go` calls the module's `AddToScheme`, and `Tag` registers itself via its `init()`.
- VM shapes are built with the existing affinity helpers' pattern from `test/e2e/vmservice/vmservice/virtualmachine/vm_group.go` (`createVMWithAffinityAndAntiAffinityFunc`); label-only VMs use the plain `manifestbuilders` path with labels and no `spec.affinity`.

## Gating

The feature is gated on `Features.TaggingAPI`, which in this change set is a plain feature flag with **no** Supervisor capability backing it (spec NG6), so there is no capability for the suite to query. Two proxies are used together. The suite:

- Skips when the FSS variable `FSS_WCP_VMSERVICE_TAGGING_API` is not enabled on the VM Operator deployment — `skipper.SkipUnlessTaggingAPIFSSEnabled`, reading the `EnvFSSTaggingAPI` config variable (tasks.md T024a). Note the flag defaults to **on**, so this catches an explicit opt-out, not an absent variable.
- Skips entirely when no `Tag` CRD is registered on the target Supervisor — a `NoKindMatchError` on the first `Tag` list. This is a distinct condition from the FSS check above, which is why both are kept: because the default is on, a Supervisor that never installed the CRD reports the FSS as enabled while the API is absent.
- Relies on T002b's `case "Tag":` in `pkgcrd.Install` for that to mean anything: without it the kind falls through to `Install`'s `default:` branch and is installed unconditionally, on every Supervisor, feature or no feature.
- Gets a **one-way** signal even with T002b, and should be read that way. Absence is conclusive — the flag has never been on. Presence is not: CRD deletion on flag-off is additionally gated on `pkgcfg.CRDCleanupEnabled`, which defaults to `false`, so a Supervisor that once had the flag on keeps the CRD after it is turned off. On such a cluster the suite will run and fail rather than skip. That is acceptable for CI, which builds fresh, and is the reason every scenario keeps `"experimental"` until the capability lookup lands.
- Should switch to a capability lookup with a named constant in `test/e2e/vmservice/consts/consts.go` — alongside the existing `VMAffinityDuringExecutionCapabilityName` — once the flag is wired to one (`research.md` "Open follow-ups"). That closes the one-way gap; T037 should not drop `"experimental"` before it does.

Verifying the vCenter-side tag is what actually proves the behavior; asserting only on `Tag` resources would pass even if no tag ever reached a VM. Scenarios that assert the vCenter side read the VM's attached tags directly via `govmomi` and therefore carry `"extended-functional"`.

## Scenarios

### US1 — label referenced by a new VM's affinity becomes a vCenter tag

| Scenario | Verifies |
|----------|----------|
| creates a VM whose affinity references its own label and observes the Tag CR and the vCenter tag | US1.1 — `Tag` exists in the namespace with the derived name, `spec.key`/`spec.value`, the label mirror, and one owner reference to the VM; vCenter tag `<key>:<value>` in category `<namespace>` is attached |
| adds a second VM with the same label and affinity and observes two owners on one Tag CR | US1.2 — exactly one `Tag` resource, two owner references, both VMs tagged in vCenter |
| creates a VM with two labels but affinity referencing only one | US1.3 — a `Tag` exists for the referenced label only; the unreferenced label produces no `Tag` and no vCenter tag (spec G3) |
| creates a VM whose affinity (a `VmToVmGroupsAntiAffinity`-style term) references a label the VM does not itself carry | US1.4 — a `Tag` is created with the VM as sole owner even though it does not carry the pair; the VM itself is **not** given the vCenter tag; a second VM created afterward carrying that label **is** tagged once the `Tag` exists (US2 mechanics) |
| deletes one of two owning VMs and observes the Tag CR survives | US1.5 — owner count drops to one, `Tag` is not deleted, the surviving VM keeps its vCenter tag |
| deletes the only owning VM and observes the Tag CR is removed | US1.6 — `Tag` disappears from the namespace within `wait-tag-cr-deleted` |

### US2 — pre-existing labeled VM is tagged when a later VM references the label

| Scenario | Verifies |
|----------|----------|
| tags a pre-existing label-only VM once a later VM references the label | US2.1 — the label-only VM is untagged before the referencing VM exists, and is tagged after; the `Tag` records **only** the referencing VM as an owner |
| untags the label-only VM when the referencing VM is deleted | US2.2 — the vCenter tag is removed from the label-only VM and the `Tag` is deleted, and the untagging is driven by the VM controller's `Tag` watch reacting to that delete event |
| does not tag a same-labeled VM in a different namespace | US2.3 — namespace isolation: a `Tag` appears only in the referencing VM's namespace, and the other namespace's VM stays untagged (spec G6) |
| tags a label-only VM created after the Tag CR already exists | US2.4 — a newly-created label-only VM picks up the vCenter tag on its first reconcile and is not added as an owner |

### US3 — label participation changes and tag carriage converges

`spec.affinity` is immutable (spec NG9), so these scenarios change a running VM's **labels**. Ownership follows the affinity reference alone, so dropping a label never drops ownership — the VM's `spec.affinity` still references the pair and cannot change — only that VM's own vCenter tag carriage reacts.

| Scenario | Verifies |
|----------|----------|
| drops the label on the sole owner and observes the Tag CR survive while only the vCenter tag is removed | US3.2, SC-005 — the VM's vCenter tag is removed, but its owner reference is **unaffected** and the `Tag` is **not** deleted (its `spec.affinity` still references the pair), with no power cycle or re-create |
| swaps a referenced label for another referenced label in one update and observes the vCenter tag follow | US3.4 and US3.5 — the VM references two pairs and carries one; after the swap it carries the gained pair's vCenter tag and not the dropped one, it still owns **both** `Tag`s, and a peer VM that carries the dropped pair keeps its own tag. The label-gain direction is what neither scenario above covers |
| drops the label on one of two owners and observes both owners and the label-only VM keep their Tag-CR standing | US3.1 and US3.3 — the relabeled VM's owner reference is **unaffected** and both owners remain; the relabeled VM loses its vCenter tag because it no longer carries the label, the surviving owner keeps its own vCenter tag, and a third label-only VM **keeps** its tag. Assert the surviving owner and the label-only VM with `Consistently`, so a regression that untags either fails rather than racing to a pass |

### Edge cases from `spec.md`

| Scenario | Edge case covered |
|----------|--------------------|
| re-creates the Tag CR when a VM needs it right after the previous one was deleted | Tag deletion racing a re-create (spec G12) — delete the sole owner and immediately create a new VM with the same label and affinity; the `Tag` must end up present with the new VM as owner, and the new VM must end up tagged |
| converges two VMs created at once on a single Tag CR | Tag create racing a create (spec G12) — create two VMs carrying and referencing the same label pair in one go; exactly one `Tag` must exist for the pair, with both VMs as owners and both tagged in vCenter |
| keeps a same-key different-value label untagged | A `Tag` represents key **and** value; a VM carrying the key with another value is unrelated |
| applies the tag independently of VirtualMachineGroup membership | SC-004 — two VMs in **different** `VirtualMachineGroup`s sharing the same affinity label both carry the same vCenter tag |
| leaves tags in place when the VM is re-created | A delete/re-create cycle of a participating VM converges back to the same `Tag` and the same vCenter tag |

## Scope boundaries (not E2E-tested by design)

- Flag-off behavior (spec SC-006) is unit-tested only. Toggling `Features.TaggingAPI` requires restarting the controller manager on the Supervisor, which is outside what this suite does; the flag-off path's equivalence to today's behavior is asserted in `pkg/providers/vsphere/virtualmachine/` unit tests (tasks.md T010).
- The concurrent-owner-reference race (two VMs patching the same `Tag` simultaneously) is covered by the envtest integration test (tasks.md T027), not E2E — reliably interleaving two reconciles against a real Supervisor is not achievable, while envtest can drive both writes deterministically.
- `status.id` remaining empty (spec NG1) needs no scenario: it is an absence, already asserted at the unit level, with no cluster-observable consequence.
- Admission rule V5 (resource name must equal the derived name) is unit-tested only; it can only be triggered by hand-authoring a `Tag`, which no supported workflow does.
- Unsupported label-selector operators being ignored rather than fatal is unit-tested only — it has no cluster-observable effect beyond the absence of a tag.
- The two accepted limitations of diffing against the ExtraConfig record rather than the live attached-tag list (spec "Edge cases": a tag detached out of band is not re-applied, and a remove may be emitted for a tag already gone) are not E2E-tested. The first would require detaching a tag directly in vCenter to assert that VM Operator does *not* react, which is a test that passes for the wrong reasons; the second has no observable effect. Both are covered at the unit level (tasks.md T014a).
