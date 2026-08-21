# Implementation Plan: NIC Unit Numbers

- **Spec**: [`spec.md`](./spec.md)
- **Epic**: vmop-3982
- **Date**: 2026-06-25 (last updated 2026-08-14)

## Summary

Add an optional `unitNumber` to `spec.network.interfaces[i]` and `status.network.interfaces[i]`. A schema-upgrade backfill records each VM's existing slots into its spec — for brownfield VMs on operator upgrade and for greenfield VMs on their first post-create reconcile. From then on, the mutation webhook assigns a slot to any interface added without one, following the disk and CD-ROM pattern including its `IsObjectUpgraded` gate, and the value is carried onto the NIC device. Uniqueness, range, and powered-on immutability are enforced in the validation webhook, and — once set — the unit number becomes the *exclusive* NIC-to-VC-device match key for that interface during reconcile. All of it is gated behind the `VMNetworkUnitNumbers` feature flag. Two constraints shape the edges of the design: the backfill writes through the same admission path a user does, so it must never record a value that would make the spec inadmissible; and the exact-only match rule makes "no device at the declared slot" a real, long-lived state that status and boot options have to tolerate.

## Technical context

- **Go version**: 1.26.5 (as in go.mod)
- **API versions touched**: `v1alpha6` (additive fields only)
- **Modules touched**: root module (`github.com/vmware-tanzu/vm-operator`), `api/` sub-module
- **New dependencies**: none

## Constitution check

| Rule | Status | Notes |
|------|--------|-------|
| API compatibility — additive only | OK | New `+optional` / `omitempty` fields; no removal or rename; deepcopy regeneration required |
| Thin controllers | OK | All new logic in `pkg/`; reconciler delegates unchanged |
| No direct vSphere calls in controllers | OK | vSphere reads in `pkg/providers/vsphere/` only |
| E2E coverage mandatory | Required | NIC unit number assignment, matching, and status are cluster-observable; E2E ships with the code |
| One test file per package | OK | New test code added to existing `_test.go` files per package; the one new test file (`virtualmachine_validator_network_interfaces_test.go`) follows the package's established per-area test-file convention (`..._hardware_controllers_test.go`, `..._compute_test.go`, etc.) with named `xTests()` functions registered via `suite.Register` |
| Webhook kubebuilder markers | OK | `+kubebuilder:validation:Minimum=7` / `Maximum=16` on the new spec field — the platform's static PCI unit allocation gives ethernet cards exactly units 7–16 (`external/vim/api/v1alpha1/testdata/device_keys.txt`) |
| CEL preferred for simple structural rules | Noted | Uniqueness across the interfaces list is a candidate for `XValidation`; the powered-on and account-scoped rules need Go either way (see Validation webhook) |
| Feature flag before behaviour ships | OK | `VMNetworkUnitNumbers` defaults to `false`; all new behaviour is behind it |
| SDD artifacts ship with code | OK | This spec/plan/tasks/model/research committed to same branch |

## Findings register

A deep review of this spec/plan/tasks against the codebase raised 8 research-program gaps (`R1`–`R8`) and 12 implementation findings (`I1`–`I12`). The IDs are stable handles referenced throughout this plan and `tasks.md`; each is defined and dispositioned here, and the disposition is folded into the sections that follow.

| ID | Finding | Disposition | Reasoning lives in |
|----|---------|-------------|--------------------|
| R1 | Same-transaction Remove+Add at the same unit number untested | Answered and load-bearing; vSphere permits it. T001 item 6 confirms on the target build (vcsim is not authoritative) | Design point 5 |
| R2 | OVF `DeployLibraryItem` create path untested | Accepted; premise corrected — OVF-via-CL is *not* the dominant path (FastDeploy is preferred when enabled). Own T001 experiment | Problem 2 |
| R3 | Explicit `UnitNumber` with `ControllerKey=0` untested | Rejected as a design concern — the platform resolves the controller. Survives only as the vcsim caveat, superseded by I26 | Test strategy |
| R4 | Collision experiment should cover non-NIC PCI occupants | Rejected. Wrong address space (`UnitNumber` ≠ `PciSlotNumber`), and PCI units are allocated statically per device class | Complexity table |
| R5 | SR-IOV experiment prerequisites unstated | Accepted; T001 states the prerequisite and a recorded-skip fallback | T001 item 14 |
| R6 | Single-vCenter run can't answer version stability | Accepted; T001 records VC/ESX build and hardware version per run | T001 methodology |
| R7 | Research methodology gaps (cleanup, requested-vs-observed, fault detail) | Accepted; folded into T001, and the collision fault has a consumer in T018/T029 | T001 methodology |
| R8 | `UnitNumber=-1` semantics | Accepted as a defensive note only, not an experiment | T001 methodology |
| I1 | No convergence path for spec≠device unit number | Resolved by the identity model — no new reconcile mechanism | Design |
| I2 | Type-change Remove+Add ignores the always-`vmxnet3` desired device | Accepted; type is excluded from the comparison and type support ships later | Non-goals, T032 |
| I3 | Powered-on nil→set enables a user-driven device-claim swap | Accepted; scoped to `ctx.IsVMOperatorAccount` | Validation webhook |
| I4 | Remove+Add bookkeeping (DeviceKey/MAC/UpdatedEthCards) unspecified | Accepted; the replace branch must not inherit the removed device's key/MAC | Problem 1, T017 |
| I5 | NIC device changes are powered-off-only today | Accepted; powered-on convergence is a committed follow-on | Non-goals, T032 |
| I6 | set→nil rejection interacts badly with annotation-lossy old clients | Accepted; documented as a loud failure mode | Rollout |
| I7 | Backfill zip mis-assignment is sticky and feeds boot-order selection | Accepted; Event for the zip fact, condition for the rest (see G8, I18) | Schema upgrade, T014/T036 |
| I8 | Backfill pseudocode type error (`**int32`) | Fixed | Schema upgrade |
| I9 | T016's mixed-case fallback inconsistent with T020's two-pass | Accepted; T016 made per-interface | Schema upgrade |
| I10 | Network-disabled stamp-then-enable semantics | Accepted; no longer a lasting state once the mutator assigns | Schema upgrade |
| I11 | Out-of-band NIC via vCenter UI | Accepted, no design change — pre-existing behaviour | Complexity table |
| I12 | Problem 5 "unreachable in normal operation" overstated | Accepted; phrasing fixed | Problem 5 |
| I13 | Backfill can write a spec its own webhook rejects | Accepted — requirement **G7**; skip the write per interface | Schema upgrade |
| I14 | Flag-off rejection blocks unrelated updates, including delete | Accepted — requirement narrowed to **G10**; reject only a new or changed value | Validation webhook |
| I15 | A numbered interface with no device at its slot breaks status and boot options | Accepted — requirement **G13**; callers tolerate the miss (three of them, per I20) | Problem 4, Status |
| I16 | Which reconcile paths compute NIC device changes is unstated | Accepted; documented, dispatch unchanged | Reconcile entry points |
| I17 | `networkextraconfig` is a fourth spec↔device matcher, absent from the design | Accepted — **new task T035**. Positional zip mis-writes after a replacement, and `findOrCreateDeviceEdit` can Edit a device the same ConfigSpec Removes | Problem 6 |
| I18 | The mutator runs on the backfill's own patch, so **G7**'s skip does not survive | Accepted, consequence owned. Disk mechanics copied; **G7** re-scoped to admissibility only. Do **not** add an account guard to the mutator | Consistency with the disk placement model |
| I19 | ~~`MutableNetworks` gates interface add/remove~~ | **Rejected** — `MutableNetworks` is on by default, so `validateImmutableNetwork` returns early | — |
| I20 | T021's status-name fix is circular and misses a consumer | Accepted; two entries were conflated, and `updateInterfaceStatus` is a third name-keyed consumer | Problem 4, Status |
| I21 | The `ControllerSpec` change reaches into the `api/` sub-module | Accepted; use an optional interface asserted in the helper, and keep `MaxSlots` a count | T008, model.md |
| I22 | "No slot available" is unreachable through the API | Accepted as defensive-only (`MaxItems=10` = ten slots + uniqueness ⇒ pigeonhole) | G9, T010 |
| I23 | Two gate styles now exist for NIC mutation | Accepted, choice unchanged (whole-object `IsObjectUpgraded`, for disk-pattern consistency); mutator filename changed to avoid collision | Mutation webhook |
| I24 | Enabling the capability makes every VM "not upgraded" for a window | Accepted; expected operational effect, cluster-wide and shared with the disk mutators | Rollout |
| I25 | T007 points at a loop that does not exist | Accepted; populate `Device.UnitNumber` where `interfaceSpec` is in scope | T007 |
| I26 | vcsim's duplicate-unit check cannot fire for operator-built payloads | Accepted; T015 must not claim collision coverage | T015 |
| I27 | `spec.network.interfaces` is `+listType=map`, so list order is not semantically meaningful | Accepted; every positional zip in the feature rests on an ordering the API does not guarantee | Positional zip caveat |
| I28 | No contingency if T001 answers "explicit `UnitNumber` not honoured" | Accepted — **plan B named** so stories survive either answer | If Q1 fails |
| I29 | Snapshot spec synthesis discards observed unit numbers it already has in hand | Accepted; `synthesizeVMSpecForSnapshot` should set them directly | T031 |

**Note on T027's directory number:** `tasks.md` targets `005-nic-unit-numbers-ga` (`003-` is taken by `003-compute-config-reconcile`).

## Design: `unitNumber` is a device identity, not a slot

**Once set, `unitNumber` *is* the identifier linking a `spec.network.interfaces[i]` entry to a specific VC hardware device** — the same role `Name` plays for provider-CR matching, or `DeviceKey` plays internally. There is no "the same NIC, moved to a new slot" concept; there is only "this spec entry identifies whatever hardware is at slot N" (or "there is nothing there yet, so create it"). This has several concrete, MUST-level consequences (see spec.md **G11**/**G12** for the normative statement; this section is the rationale and the resulting engineering simplification):

1. **Matching for a numbered interface is exact-slot-only, never fallback.** An interface with `unitNumber` set MUST NOT ever be matched via MAC/ExternalID/backing — not as a first choice, not as a fallback on a miss. Doing so would let some other device (identified by backing/MAC) impersonate the identity the spec explicitly declared. Two passes: pass 1 is exact-slot lookup for numbered interfaces only; pass 2 is the existing MAC/ExternalID/backing fallback for un-numbered interfaces against whatever devices pass 1 left unclaimed.
2. **Locating the device is only half the job — the located device must then be compared, and converged by replacement.** Pass 1 answers "is there a device at this interface's declared unit number?" It does *not* answer "is that device what the spec asks for?" Two sub-cases follow:
   - **2a. Miss (no device at the declared number) → plain Add.** The interface currently has no VC hardware. It falls through to the ordinary Add path (Problem 2) carrying the new number. Whatever device the interface *used* to identify (if nothing else claims it in either pass) is now unclaimed and is removed by the **already-implemented** "remove any unmatched existing interfaces" pass — re-verified at `reconcile.go:76-83` (`ReconcileNetworkInterfaces` appends a `VirtualDeviceConfigSpecOperationRemove` for every `dev` still in `currentEthCards` after the results loop). No new removal mechanism and no Edit-based `UnitNumber` change is needed; the Add path builds `r.Device` fresh from spec each reconcile (re-verified: `session_vm_update.go:778-788` calls `CreateDefaultEthCard` per result), so there is no prior device in scope to carry stale state from.
   - **2b. Hit → compare, then leave alone or replace.** Compare the located device against the desired state on **backing**, **MAC only when the desired state specifies one** (`AddressType: Manual`; a `Generated` MAC is not compared), and **ExternalID only when the desired state specifies one** (non-empty) — deliberately the same predicate the existing MAC/ExternalID/backing matcher already encodes, so the natural implementation is to extract that per-device predicate from `FindMatchingEthCard` and reuse it rather than write a second, divergent comparison. Unspecified fields are not compared and must not trigger a change. If everything specified agrees → **no device change** (and the existing, correct adoption of the device's `Key`/MAC into the result applies — this is how a `Generated` MAC is learned). If anything specified disagrees → **Remove that device + Add the desired device at the same unit number, in the same `ReconfigVM_Task`** — vSphere supports freeing and reusing a unit number within one Reconfigure (R1, above), so this converges in a single reconcile with no intermediate state. **Deliberately not an Edit for this MR** — see point 7 below.
   - **I4 applies to 2b only.** That branch matches a device and then removes it, so it must not inherit the removed device's `Key`/`MacAddress`: leave `DeviceKey=0`, set `UpdatedEthCards=true`, and let the post-reconfigure `fixupMacAddressMutableNetworks` pass re-identify the newly-added device by unit number. Copying them would point the fixup at a device that no longer exists and leak a stale MAC into bootstrap args and status.
3. **The declared slot can be occupied by a *different* interface's old device — that's a legitimate third outcome, not an edge case to special-case.** If interface A renumbers to a slot currently held by interface B's device (a straight swap: A 8→9, B 9→8 in one update; or B deleted from the spec while A renumbers onto B's former slot), pass 1's exact-slot lookup for A simply finds the device at 9 (which happens to be B's old device) and applies the ordinary compare-then-leave-or-replace rule from point 2b to it — and likewise for B at slot 8. In the common case where A and B differ in backing/specified-MAC/specified-ExternalID, each side's comparison fails and each side replaces the device it found, so neither interface ends up carrying the other's identity. If they agree on everything specified (e.g. same network, both `Generated` MACs), no device change is emitted at all and they simply trade slots — which is harmless precisely because nothing distinguishing was pinned. **Either way, no swap-detection, collision-avoidance, or sequencing logic is needed anywhere.** The reason that is safe is specific and load-bearing: **the validation webhook already enforces `unitNumber` uniqueness across interfaces, so at most one interface can ever declare any given number** — pass 1 is therefore a contention-free lookup, with no ordering dependency between interfaces to get wrong.
   - **Note this outcome is not rare on densely-populated VMs — it is the *only* possible renumber outcome there.** The interfaces list is capped at 10 (`MaxItems=10`, verified in `api/v1alpha6/virtualmachine_network_types.go`) and the valid range 7–16 is exactly 10 slots, so a VM with 10 interfaces has every slot occupied: any renumber on such a VM necessarily targets a slot another interface's device already holds — i.e. always the compare-then-replace branch (2b), never a plain Add into an empty slot. Do not treat 2b as the exotic branch.
   - **A MAC/`ExternalId` coherence hazard exists in any Edit-based reading of this case, and is resolved by compare-then-replace (point 2b).** Recorded here because it constrains the deferred Edit follow-on: the existing match branch (`reconcile.go:36-38`) unconditionally does `results.Results[idx].MacAddress = matchDev.MacAddress` *and* overwrites the desired device's `MacAddress` with the matched device's, **before** any comparison. Had this MR edited a mismatched device in place, an interface claiming another interface's former card would have written its own `ExternalId` onto a device still carrying the other's MAC — broken-but-healthy-looking for NSX-T / VPC, where the CR pins a MAC to the logical port named by `ExternalId`. Replacing the device instead sidesteps this entirely: the Added device is built fresh from the claiming interface's own desired state, so backing, MAC, and `ExternalId` are coherent by construction. **The deferred Edit work MUST re-solve this** — an in-place Edit of a device claimed from another interface has to emit the claiming interface's own pinned MAC (`AddressType: Manual`) rather than adopt the device's, and may only adopt a `Generated` MAC.
4. **vSphere honouring an explicit `UnitNumber` on Add gates whether the design *terminates*, not just whether placement lands correctly at create time.** There is no Edit-based safety net in this model: if vSphere places the Add at slot 10 instead of the requested 9, the *next* reconcile's exact-slot lookup for that interface still finds nothing at 9, so it Adds *again* (Removing the previous attempt each time) — a non-terminating churn loop, not a one-time miss. T001's "does vSphere honour explicit `UnitNumber` on Add" experiment (spec.md **Q1**) is therefore a hard prerequisite for the feature to converge at all.
5. **Same-transaction Remove+Add at the *same* unit number is a core mechanism here, and vSphere supports it.** Point 2b's replacement emits Remove(dev@N) + Add(desired@N) in one `ReconfigVM_Task`; vSphere permits freeing and reusing a unit number within a single Reconfigure. Note the existing code already orders removes before adds in the returned change list (`return append(removeDeviceChanges, deviceChanges...)`, `reconcile.go:85`), which is exactly the ordering this relies on — but the replacement's own Remove is emitted from the results loop, so **T017 must ensure its Remove lands in the same "removes first" position** rather than after the Add. Renumbering to a *different*, empty slot (point 2a) is the ordinary "delete one, add another" pattern and needs nothing special. vcsim is not authoritative evidence for same-slot reuse (duplicate-unit check is scoped by `ControllerKey` + device type), so T001 confirms it against real VC.
6. **A set value stays mutable while powered off.** The webhook allows `set → different` while the VM is powered off — the operator explicitly accepts that changing an existing interface's `unitNumber` is a NIC-replacement operation (new device `Key`; new MAC, and therefore possibly a new DHCP-assigned IP and a brief connectivity interruption, when `AddressType: Generated`), not a lossless move, and chooses to document that plainly rather than restrict the field to fully immutable. A future alternative (fully immutable, express a "renumber" as delete-interface + add-interface at the API level instead) was considered and explicitly not taken.
7. **Replace-not-Edit is an explicit interim decision, with one accepted regression.** For this MR, *any* disagreement between a numbered interface's desired state and the device at its declared unit number is converged by replacement (point 2b), never by an in-place Edit — including changes an Edit could express losslessly, such as a backing change from a network migration. **Converting these to a device-preserving Edit is deferred to later work** (tracked alongside the other follow-ons — see "Future work" and T032). The accepted consequence worth stating plainly, because it is a narrowing of today's behaviour rather than merely a missing optimization: **a numbered interface no longer benefits from the existing orphaned-CR Edit path** (`findExistingEthCardForOrphanedCR`), which today can absorb a backing change by editing the device in place and thereby preserve the device and its MAC. Once an interface carries a `unitNumber`, that same migration replaces the device (new `Key`; new MAC when `Generated`). This matters most to `MutableNetworks` / telco flows, which are precisely the ones that exercise network re-pointing — call it out in the release note (T025) and docs (T030), and treat it as the motivating driver for the deferred Edit follow-on rather than a nice-to-have.

## Consistency with the disk placement model — and the one deliberate divergence (I18)

Disks and CD-ROMs already have an end-to-end unit-number story in this repo, and **this feature copies its backfill and webhook mechanics exactly.** Stating the full disk contract is worth the space, because the part NICs *cannot* copy is the part that makes the disk version safe.

What disks do, in four pieces:

1. **Backfill skips on disagreement and leaves nils.** `needsPlacementBackfill` (`vm_schema_upgrade.go:669`) touches a volume only when a placement field is missing; `hasPlacementMismatch` (`:676`) then abandons the write for that volume — all three fields — if any already-set field disagrees with the observed hardware. Spec wins; nothing invalid is persisted. CD-ROMs mirror this at `:794-818`.
2. **The mutator later fills the nils.** `AddControllersForVolumes` classifies a volume with a nil unit number as implicit placement (`virtualmachine_mutator_hardware_controllers.go:61-71`) and assigns it a slot on the next update that passes the `IsObjectUpgraded` gate — including the backfill's own patch, which carries the annotation stamp. Nobody treats this as a defect.
3. **Identity is established elsewhere.** A volume is matched to its disk by PVC name → disk UUID in the backfill (`pvcNameToDiskUUID`) and by disk UUID → volume name in the hardware check (`update_status_hardware_validation.go:324`). The unit number is requested placement, never a lookup key.
4. **Divergence is reported, never converged.** `checkVolumes` diffs expected-from-spec against actual-from-hardware and `reconcileHardwareCondition` marks `VirtualMachineHardwareDeviceConfigVerified` false with `VirtualMachineHardwareDeviceConfigMismatchReason` (`:289`, `:196-228`). No disk is moved or recreated because its spec unit number is wrong.

**NICs adopt 1, 2 and 4 and deliberately break 3.** Pieces 1 and 2 are **G7** verbatim, including the consequence that a skipped interface does not stay nil. Piece 4 becomes **G8**'s condition. Piece 3 is what **G11** overturns: the unit number *is* the NIC's identity, so a value the mutator invents for a skipped interface is not inert metadata — the interface resolves against whatever device occupies that slot and, per **G12**, replaces it when it disagrees. Same mechanics, different blast radius: a wrong number on a disk yields a false condition; on a NIC it yields a new device key, a new generated MAC, and a dropped DHCP lease.

Two alternatives were weighed and rejected:

- **Fully disk-like** — keep MAC/ExternalID/backing as the identity and demote `unitNumber` to placement plus a condition. Safe, and it deletes most of Phase 6, but it abandons the stable-identity-across-network-migration property this feature exists to provide.
- **Report instead of converge at G12** — mark the condition false when the device at a declared slot disagrees, and change nothing. Closest in spirit to the disk model, but it leaves a genuine network re-point permanently unconverged, which is a regression against today's behaviour.

**Decision: keep G11's identity model and own the consequence.** The plan states the divergence rather than implying disk parity, **G7** is re-scoped to admissibility only, and **G8**'s condition is the disk-style signal that keeps a mis-assignment visible. Implementers should not "fix" the mutator's assignment of a G7-skipped interface — it is the disk behaviour, and suppressing it for the VM Operator account only defers it to the next user update of any kind.

## Reconcile entry points and flag interactions (I16)

`ReconcileNetworkInterfaces` — where every behaviour in "Design" above lives — is **not** reached on every reconcile of a powered-off VM. `reconcilePoweredOffOrPoweredOnVM` dispatches three ways, and only some of them converge NIC devices:

| Situation | Path | NIC device changes computed? |
|---|---|---|
| Powered on | `poweredOnReconfigure` | **No** — no call to `UpdateEthCardDeviceChanges` anywhere in it (I5) |
| Powered off, staying off, `VMResize` or `VMResizeCPUMemory` on | `resizeVMWhenPoweredStateOff` | **Only when `Features.MutableNetworks` is on** — the call site is inside that flag check |
| Powered off → on, `VMResize` on | `poweredOffReconfigure` → `getResizeConfigSpecForPoweredOffVM` | **No** — this builder emits no ethernet device changes at all |
| Powered off → on, `VMResize` off | `poweredOffReconfigure` → `getConfigSpecForPoweredOffVM` | **Yes**, unconditionally |

Consequences this plan takes as constraints rather than fixing:

- **This feature changes NIC device convergence only where convergence already happens.** In a deployment with `VMResize` on and `MutableNetworks` off, none of Problem 1/2's behaviour (placement on Add, compare-then-replace, renumber) is reachable through the update path at all; `unitNumber` still round-trips, still validates, still backfills, and still appears in status, but no device change is ever emitted from it. That is pre-existing behaviour for *all* NIC device changes, not something this feature introduces — but it must be stated, because every reader of "the powered-off reconcile path" assumes a single path.
- **T022's E2E scenarios that assert a device change (2, 3, 5, 6) only hold under a flag combination that reaches a converging path.** The E2E spec must state the combination it assumes and skip rather than fail when the environment does not provide it. Do not diagnose a missing device change in those scenarios as a unit-number bug before checking which path ran.
- **No change to this dispatch is in scope here.** Widening NIC convergence to the other paths belongs with the powered-on follow-on (T032), which is where the question naturally lands.

## If Q1 fails: the observation-only fallback (I28)

T001 items 1 and 3 gate roughly a third of the task list, and "vSphere does not honour an explicit `UnitNumber` on Add" is a real possible answer. Naming the fallback now means stories can be filed against a known branch instead of a cliff.

**Fallback shape: `unitNumber` becomes an operator-owned observed value, not a request.** Everything that only needs to *observe* a slot survives; everything that tries to *choose* one is dropped.

- **Dropped**: setting `UnitNumber` on the Add payload and on the create ConfigSpec (T018, T029); mutation-webhook assignment (T008, T009, T010); user-pinned values and the renumber story (**G4**, **G5**, most of **G9**'s powered-on rules — the field becomes immutable to users and is rejected outright on create).
- **Changed**: the backfill stops being one-shot. If the operator cannot place a device, the spec value is only true until the device is next replaced, so recording the observed slot has to become a step that runs every reconcile rather than a schema-upgrade step that runs once. The feature-version bit still marks the initial population.
- **Survives, and still delivers the core value**: unit-number-first matching (T017 pass 1, T020, T035), which is what makes an interface's identity stable across a backing change or network migration — the motivating problem. Also T004, T005, T007, T014/T015/T016, T021, T028, T033, T036, and **G13**'s degradation rules.

So a "not honoured" answer costs the pinning and renumbering features and re-shapes the backfill; it does not invalidate the identity model or the matching work. **Do not begin T008/T009/T010/T018/T029 before T001 reports.** T009 in particular is *not* currently listed as T001-gated and should be: assigning a slot the platform will not honour manufactures the exact spec/hardware divergence this feature exists to remove.

## Project structure

New files:
```
webhooks/virtualmachine/mutation/virtualmachine_mutator_network_unit_numbers.go
webhooks/virtualmachine/validation/virtualmachine_validator_network_interfaces.go
hack/vcresearch/nic-unit-numbers/main.go    (govmomi research program — dev tooling, not a product binary)
```

(The mutator filename deliberately avoids `..._network_interfaces.go`, which sits one word from the existing `virtualmachine_mutator_network_interface_type.go` — a different NIC mutation with a different gate. See I23. Note also that `hack/` currently contains no `.go` files outside `hack/tools/`, its own module, so this program is the first thing under `hack/` that `go build ./...`, `go vet`, and `golangci-lint` will pick up in the root module; that is accepted for a short-lived program, and T023 deletes it.)

(See "Mutation webhook" below for why assignment lives there rather than being left to vSphere.)

Modified files:
```
api/v1alpha6/virtualmachine_network_types.go
api/v1alpha6/zz_generated.deepcopy.go       (regenerated via make generate-go)
api/v1alpha2/virtualmachine_conversion.go   (annotation-based restore for the new hub-only field)
api/v1alpha3/virtualmachine_conversion.go   (annotation-based restore for the new hub-only field)
api/v1alpha4/virtualmachine_conversion.go   (annotation-based restore for the new hub-only field)
api/v1alpha5/virtualmachine_conversion.go   (annotation-based restore for the new hub-only field)
config/crd/                                 (regenerated via make generate-manifests)
pkg/config/config.go
pkg/util/vmopv1/features.go
pkg/util/vmopv1/hardware.go                 (first-usable-unit on ControllerSpec; NICBusSpec)
webhooks/virtualmachine/mutation/virtualmachine_mutator.go
pkg/providers/vsphere/network/network.go    (NetworkInterfaceResult struct)
pkg/providers/vsphere/network/reconcile.go
pkg/providers/vsphere/network/devices.go
pkg/providers/vsphere/session/session_vm_update.go
pkg/providers/vsphere/upgrade/virtualmachine/backfill/nic.go
pkg/providers/vsphere/upgrade/virtualmachine/vm_schema_upgrade.go
pkg/providers/vsphere/vmlifecycle/update_status.go
pkg/providers/vsphere/vmprovider_vm.go      (create-path ConfigSpec UnitNumber; status unit-number map)
pkg/vmconfig/bootoptions/bootoptions_reconciler.go  (tolerate an unmapped numbered interface — G13/I15)
pkg/vmconfig/networkextraconfig/nic_matcher.go      (unit-number matcher instead of the positional zip — I17)
pkg/vmconfig/networkextraconfig/nic_fields.go       (findOrCreateDeviceEdit must not Edit a device being Removed — I17)
pkg/vmconfig/networkextraconfig/reconciler.go       (pass the flag/unit numbers into the matcher — I17)
pkg/providers/vsphere/vmlifecycle/update_status_hardware_validation.go  (NIC placement condition — G8/I18)
webhooks/virtualmachine/validation/virtualmachine_validator.go
docs/                                       (user-facing documentation for the new field)
```

## API / CRD strategy

Additive fields on `VirtualMachineNetworkInterfaceSpec` and `VirtualMachineNetworkInterfaceStatus`. No version bump required. Run `make generate-go` then `make generate-manifests` after the API change.

**Conversion changes ARE required — in every older API version.** This repo preserves hub-only spec fields across old-version round-trips via the annotation-based restore mechanism (`utilconversion.UnmarshalData`), not automatically. Without it, a client submitting an UPDATE through an older version wipes `spec.network.interfaces[i].unitNumber` — the k8s#111703 additive-field hazard the constitution warns about. **Each of `v1alpha2`, `v1alpha3`, `v1alpha4`, and `v1alpha5` has its own `restore_v1alpha6_VirtualMachineNetworkInterfaces`** (each restores `VNUMANodeID` today); the new spec field must be added to all four, and each version's conversion fuzz tests must be extended to cover it. `v1alpha1` restores the entire interfaces list wholesale when the down-converted list is non-empty, so it needs no per-field change — but its fuzz tests should assert the field survives the round-trip. The status field needs no per-field handling — status is fully restored via `dst.Status = restored.Status`.

## Controller / webhook impact

### Mutation webhook — assigns unit numbers, following the disk pattern

**Design decision: the mutation webhook assigns `unitNumber` to interfaces that do not specify one, using the same shape as the disk / CD-ROM / controller mutators.** The flow of values is:

1. **Explicit user value** → left untouched; it only reserves its slot in the occupied set.
2. **Unset, VM schema-upgraded** → the mutator assigns the next available unit in 7–16, in list order.
3. **Unset, VM not yet schema-upgraded** → the mutator assigns nothing at all for this VM. The backfill records observed hardware first; assignment begins on the request that carries the backfill's own annotation stamp.
4. **No slot available** → leave the interface unassigned, log, and let the validating webhook report it (mirrors `skippedNoSlotMessage` in the CD-ROM mutator).

**This is an update-path mutation, not a create-path one — the same as disks and CD-ROMs.** The upgrade annotations are written by `ReconcileSchemaUpgrade` during the VM's own reconcile (`vm_schema_upgrade.go:100-112`), never at admission, so a VM being *created* is by definition not upgraded and rule 3 always fires for it. That is why `MutateCdromControllerOnUpdate` bails on `oldVM == nil` and why the volume/controller mutation block sits under `case admissionv1.Update` in `virtualmachine_mutator.go`. Consequently: a VM is created with `unitNumber` exactly as submitted, the backfill supplies values on the first post-create reconcile, and the mutator numbers anything added afterward.

New file `webhooks/virtualmachine/mutation/virtualmachine_mutator_network_unit_numbers.go` with `MutateNICUnitNumbersOnUpdate`, registered on the update path in `virtualmachine_mutator.go` alongside the existing hardware mutators, gated on `Features.VMNetworkUnitNumbers`.

**The ordering hazard this raises is not NIC-specific and is already solved generically** — a webhook guess written before the backfill would be locked in by spec-wins and could make unit-number-first matching claim the wrong device. Two existing pieces handle it (rationale in `research.md`, "Disk unit number prior art"):

- `vmopv1util.IsObjectUpgraded(ctx, vm)` is the gate. The CD-ROM mutator uses it (`virtualmachine_mutator_cdrom.go:50`), the controller mutator uses it, and both hardware validators use it. The comment at `virtualmachine_validator_hardware_controllers.go:180-186` records the subtlety worth copying: **evaluate hot-add/powered-on rules against `oldVM`, and the mutation gate against `vm`** — "the patch on schema upgrade will contain the annotation along with any backfilled controllers", so gating the mutator on the new object makes assignment start immediately after the backfill lands, while gating the powered-on validation on the old object avoids judging a VM by rules its spec has not yet been backfilled to satisfy.
- The existing helper is `vmopv1util.NextAvailableUnitNumber(controller, occupiedSlots)` (`pkg/util/vmopv1/hardware.go:36`), with the volume mutator's two-phase shape to copy verbatim: first insert every explicit value into `occupiedSlots`, then assign the remainder (`virtualmachine_mutator_hardware_controllers.go:200-215`).

**Consequences of following the pattern**, all of which simplify the rest of this plan:

- Every interface on an upgraded VM carries a unit number. The "NIC added after the one-shot backfill stays nil forever" case (I10, Problem 5) disappears, and with it the need to keep the positional zip permanently reachable.
- The backfill (T014/T015) keeps **both** roles it always had — brownfield VMs on upgrade, greenfield VMs on their first post-create reconcile — because admission cannot assign anything for a VM that has not been upgraded yet, and a VM is never upgraded at create time. Its correctness requirements (G7, Events) are unchanged.
- The create path is unchanged in shape: only user-pinned values reach the initial ConfigSpec (Problem 2).
- **Q1 becomes more load-bearing, not less.** Once a VM is upgraded, every interface carries a value, so every subsequent Add requests a specific slot and a "not honoured" answer produces the churn in Design point 4 for ordinary NIC additions, not just pinned ones. This does not argue against the pattern — Q1 already gates the create/add paths — but it raises the cost of a "not honoured" answer.

**One helper change is required.** `NextAvailableUnitNumber` scans `0..MaxSlots()-1` and skips a single `ReservedUnitNumber()`. NICs start at 7, which cannot be expressed as one reserved unit. Add a first-usable-unit notion to `ControllerSpec` — 0 for existing controllers, 7 for the NIC bus — rather than forking a NIC-local copy of the helper; one helper with one set of semantics is the point of following the pattern. A `NICBusSpec` then implements `ControllerSpec` with `MaxSlots`/`MaxCount`/first-unit reflecting the 7–16 band and the single implicit PCI bus.

### Validation webhook

New file `virtualmachine_validator_network_interfaces.go` with function `validateNICUnitNumbers`:

1. If flag `VMNetworkUnitNumbers` is off: reject with `field.Forbidden(path, featureNotEnabled)` — the same pattern `validateNetworkVLANs` uses for `VMVlanSubinterface` — but **only for a value that is new or changed relative to `oldVM`** (spec.md **G10**, I14). On CREATE that means any interface setting `unitNumber`; on UPDATE it means an interface whose `unitNumber` differs from the same-named interface's old value. Do NOT silently ignore a new value: values persisted while the flag is off would later poison the spec-wins backfill.

   **Do not reject on mere presence of an unchanged value.** `ValidateUpdate` (`virtualmachine_validator.go:306`) validates the whole object on every UPDATE and has no deletion or no-change early-out, so a presence-based rejection fails *every* update to an already-backfilled VM while the flag is off — including label/annotation edits, the VM controller's own patches, and **finalizer removal, which makes the VM undeletable**. That is a far larger blast radius than the intended "no new values while off", and it is not what "disabling the feature is not a supported flow" was meant to accept. Cover both directions in tests: unchanged value + flag off → allowed; changed/new value + flag off → forbidden; and a delete of a backfilled VM while the flag is off → succeeds.
2. For each interface with a `unitNumber` set:
   a. Range check: 7 ≤ value ≤ 16.
   b. Uniqueness check: value not already in `occupiedSlots`; insert after checking. (CEL `XValidation` on the interfaces list is an alternative for the uniqueness rule per the constitution's CEL-first preference — evaluate at implementation; the powered-on check below requires Go either way.)
3. On update, if the old VM is powered on and an interface's already-set `unitNumber` has changed to a different value, add a `field.Forbidden` error. **nil → set transitions are allowed only when the request is made by the VM Operator service account** (`ctx.IsVMOperatorAccount`, the same mechanism `validateVolumes` already uses to bypass volume validation for the operator's own writes — `virtualmachine_validator.go:1527`) — this is how the schema-upgrade backfill writes observed values into a running VM's spec; a *user*-originated nil → set on a powered-on VM is rejected like any other change (I3). Without this scoping, a user could set an unclaimed interface's `unitNumber` to a slot already occupied by another interface's device and trigger a device-identity swap on the next reconcile (unit-number-first claims the wrong NIC for one interface, the other interface's fallback match then misses and the reconciler removes/re-adds — crossing MAC/ExternalID/guest customization between the two). A set → nil transition (user clears the value) is treated as a change and is rejected while powered on regardless of account; cover both directions, and both accounts, with tests.

Wired into `validateNetwork` in `virtualmachine_validator.go`.

#### Blocking user updates during the schema-upgrade window (spec.md G16)

Separate from the rules above, and reusing machinery that already exists: `ValidateUpdate` calls `validateSchemaUpgrade`, which — for anyone other than the VM Operator service account or `system:masters` — invokes `validateFieldsDuringSchemaUpgrade` when `IsObjectUpgraded(ctx, oldVM)` reports the object is not yet upgraded (`virtualmachine_validator.go:2437-2443`), "to prevent most users from modifying the VM spec fields, that are backfilled by the schema upgrade and mutable, before the schema upgrade is completed."

`validateNICBackfilledFieldsNotChanged` (`:3604`) inside it already forbids **any** change to `spec.network.interfaces` in that window — exactly the guarantee this feature wants. **The only change needed is its gate:** it is reached only under `Features.TelcoVMServiceAPI` (`:3502-3509`), and `VMNetworkUnitNumbers` is independent of Telco — the same orthogonality argument this plan already makes for keeping the unit-number backfill out of `NICConfigFromMoVM`. Extend the condition so the interfaces guard also applies when `VMNetworkUnitNumbers` is enabled (T034).

The three parts then compose into one ordering, and each is weak without the others:

1. **Users cannot change `spec.network.interfaces`** before the upgrade (this rule) — no user-supplied unit number can precede the backfill.
2. **The mutator assigns nothing** before the upgrade (`IsObjectUpgraded` on the new object, G4.3) — no generated value can precede it either.
3. **The backfill records observed hardware** and stamps the bit; from that request on, 1 and 2 both open up.

VM Operator's own account bypasses (1) — `validateSchemaUpgrade` returns nil for it before any field check — which is what lets the backfill's patch through.

### Schema upgrade — brownfield backfill

**Decision (confirmed): the backfill is a standalone schema-upgrade step with its own feature-version bit and its own flag/capability gate.** It must NOT live inside `NICConfigFromMoVM` under the `TelcoVMServiceAPI` gate: that gate is one-shot, so any VM already stamped with the Telco bit would never be backfilled, and `TelcoVMServiceAPI` may be disabled while `VMNetworkUnitNumbers` is enabled.

New `FeatureVersionNICUnitNumbers = 16` bit in `pkg/util/vmopv1/features.go`, included in `FeatureVersionAll`, in the `FeatureVersions()` slice, and in `ActivatedFeatureVersion` when `VMNetworkUnitNumbers` is enabled.

New function `NICUnitNumbersFromMoVM` in `pkg/providers/vsphere/upgrade/virtualmachine/backfill/nic.go`:

1. Iterate `spec.network.interfaces`.
2. For each interface, prefer the existing hodgepodge matching (MAC, ExternalID, backing — same logic as `FindMatchingEthCard` / `MapEthernetDevicesToSpecIdx`) to identify the corresponding VC ethernet device.
3. If a unique match is found and `iface.UnitNumber` is nil (spec wins), copy the observed value: `iface.UnitNumber = ptr.To(dev.GetVirtualDevice().UnitNumber)` (I8 — `dev.GetVirtualDevice().UnitNumber` is already `*int32`; taking its address again produces a `**int32` type error and, worse, would alias a field inside `moVM`. Copy the pointed-to value with a nil check, don't re-take the address).
4. After the hodgepodge pass, positionally zip the remaining unmatched spec interfaces against the remaining unclaimed VC ethernet devices and backfill from those pairs, so every interface receives its observed unit number. The zip is strictly a last resort — it applies only to interfaces the MAC/ExternalID/backing matching could not uniquely claim (decision confirmed).
5. **Raise a `Warning` Event for the one fact nothing else records (I7, G8.2):** that an interface's value came from the positional zip rather than a unique match (reason `NICUnitNumberBackfillAmbiguous`). V(4) alone is insufficient — a zip mis-assignment is one-shot and permanent, and it feeds unit-number-first reconcile matching, status, **and boot-order device selection** (`MapEthernetDevicesToSpecIdx` is called from `bootoptions_reconciler.go:235`), so it can point network boot at the wrong NIC with no operator-visible trace. Nothing in the resulting spec distinguishes a zipped value from a matched one, which is why this case needs an Event rather than a condition.

   **The other two cases are conditions, not Events (G8.1).** A pre-existing explicit value that disagrees with the observed slot, and an interface the admissibility guard skipped, are both *steady-state observable*: on every subsequent reconcile the interface's declared slot either holds no device or holds one that disagrees. Report them the way disks report volume placement divergence — `checkVolumes` → `reconcileHardwareCondition` → `VirtualMachineHardwareDeviceConfigVerified` (`update_status_hardware_validation.go:289`, `:196-228`) — so the signal is recomputed each reconcile, covers a slot the *mutator* invented as well as one the backfill recorded, and clears when the VM converges. A one-shot Event at backfill time cannot do any of that.

#### Admissibility guard — the backfill must not write a spec its own webhook rejects (I13, spec.md G7)

The backfill writes to `vm.Spec` and that write is persisted by the VM controller's patch helper, so it passes through **the same CRD schema validation and the same validating webhook as a user's write**. A backfilled value that violates either rule does not fail quietly once — it makes every subsequent spec patch on that VM fail admission, including the operator's own, and including the patch that would have stamped the feature-version annotation. The VM stops converging and the failure surfaces only as a webhook denial in the controller log.

Two reachable ways to produce such a value:

- **Duplicate — the live case.** Interface A carries an explicit `unitNumber` that vSphere did not honour (**Q1**), or that collides with a VM class ConfigSpec device; interface B's observed device sits at that same slot. Spec-wins keeps A's value and the backfill would write B's observed value → two interfaces at the same number → uniqueness rejection.
- **Out of range — defensive only.** Ethernet cards own units 7–16 by static platform allocation, so a well-formed observation is always in range. The guard costs one comparison and converts an impossible-by-contract observation into a skipped write and an Event instead of a wedged VM.

**Therefore, per interface, the backfill MUST skip the write** (leaving `unitNumber` nil) when the observed value is already claimed by another interface's spec value or by a value written earlier in this same backfill pass, or — defensively — when it is outside the valid range. This is the same shape as the disk backfill's `hasPlacementMismatch` skip (`vm_schema_upgrade.go:676`) and is deliberately identical to it. The decision *not* to replicate that guard applies only to the "explicit spec value disagrees with the observed slot" case, where spec still wins and nothing invalid is written; it does not extend to writing a value the API would reject.

**What the skip does and does not buy (I18).** It guarantees the spec stays admissible. It does **not** leave the interface un-numbered: the mutating webhook has no privileged-account bypass (`virtualmachine_mutator.go:255`; the package's only account check is `:692`), so it runs on the backfill's own patch — which carries the annotation stamp opening the `IsObjectUpgraded` gate — and on every later update of any kind, and a nil `unitNumber` on an upgraded VM is indistinguishable from a newly-added interface. It therefore receives a free slot, precisely as `AddControllersForVolumes` assigns one to a volume the disk backfill skipped. Under **G11** that invented slot is acted on, so the interface may have its device replaced on the next powered-off reconcile. **This is accepted** — see "Consistency with the disk placement model". Do not add an `IsVMOperatorAccount` guard to the mutator to suppress it: it diverges from the disk pattern and only defers the assignment to the next unrelated user update.

Unit tests must cover both skip cases explicitly, and the vcsim integration test (T015) must assert that a VM constructed to hit them still reconciles and still gets its feature-version bit stamped.

#### Additional backfill behaviours

- **`spec.network.disabled` does not skip the backfill.** The backfill is driven by the spec's interface list, so a VM with no interfaces records nothing and the feature-version bit is still stamped (one-shot semantics preserved). Note the premise is the *spec list*, not the hardware: `s.reconcileNetworkInterfaces` returns early for a disabled network with a `TODO: Remove all interfaces`, so a VM whose network was disabled after creation can still have ethernet devices in VC. **The "stamp disabled, enable later" sequence (I10) is no longer a lasting state:** a VM whose network was disabled at backfill time gets the bit stamped with zero interfaces recorded, and NICs added after the network is later enabled are numbered by the mutation webhook (the VM is upgraded by then, so its gate is open) rather than staying nil forever. The one-shot backfill no longer implies permanently un-numbered interfaces.
- **Spec wins over a disagreeing observation.** When an interface already carries an explicit `unitNumber` that disagrees with the observed device slot, the backfill leaves the spec value in place (unlike the disk backfill's `hasPlacementMismatch`, which skips on disagreement — `vm_schema_upgrade.go:678`) and raises the Event from step 5(b) instead of only logging at V(4). This is orthogonal to the admissibility guard above: spec wins, but an *observation* is never written when writing it would be invalid.

New gate in `ReconcileSchemaUpgrade`:
```go
if features.VMNetworkUnitNumbers {
    if f := vmopv1util.FeatureVersionNICUnitNumbers; !vmFeatureVersion.Has(f) {
        backfill.NICUnitNumbersFromMoVM(ctx, vm, moVM)
        vmFeatureVersion.Set(f)
    }
}
```

After `NICUnitNumbersFromMoVM` completes, also update `NICConfigFromMoVM` (TelcoVMServiceAPI path) to resolve its `TODO(BV)` — **per interface, not all-or-nothing (I9):** for each spec interface with `UnitNumber` non-nil, look it up in a `unitNumber → device-index` map built from the VC devices; for interfaces without a `UnitNumber`, zip positionally against the remaining unclaimed devices. Do not gate the whole function on "every interface has a `UnitNumber`" — a mixed VM (some interfaces numbered, some not) must not fall back to an all-positional zip, which would reintroduce mis-zip risk for the already-numbered interfaces. This matches the per-interface two-pass approach `MapEthernetDevicesToSpecIdx` already uses (Problem 4).

### Reconcile — NIC-to-VC matching

This is the most complex and most important change. There are two distinct matching functions that must be updated independently. See the six-problem analysis below.

Throughout, "ethernet card" means any device for which `pkgutil.IsEthernetCard` returns true — this includes `VirtualSriovEthernetCard`, so SR-IOV interfaces are covered by the same matching, backfill, and status logic with no special-casing.

#### Problem 1: `FindMatchingEthCard` — desired device has no `UnitNumber`

The desired `VirtualEthernetCard` built in `CreateDefaultEthCard` never has a `UnitNumber` set. To add unit-number matching, the unit number must be threaded separately.

**Solution:** Add `UnitNumber *int32` to `NetworkInterfaceResult` (in `network.go`). Populate it in `reconcileNetworkInterfaces` (session_vm_update.go) from `interfaceSpec.UnitNumber` when the feature flag is on. Update `FindMatchingEthCard` to accept a `unitNumber *int32` parameter. When non-nil and the flag is on, first scan `currentEthCards` for a card whose `VirtualDevice.UnitNumber` matches and return it immediately — before the existing MAC/ExternalID/backing checks.

Rationale for unit-number-first: the unit number is a stable, vSphere-assigned hardware slot. Backing can change during network migrations. ExternalID is provider-specific and absent on older VMs. MAC is generated and may not be on the spec. Unit number is the most reliable stable identifier once assigned.

**Two-pass claim order, exact-only for numbered interfaces (see "Design" above).** `ReconcileNetworkInterfaces` (`reconcile.go:24-88`) processes results in a single greedy loop today: on a match it simply claims the device and appends no device-change entry at all (no diff of any kind is currently computed, even for backing/property changes on a matched device). The loop must run in two passes, in order:

1. **Exact unit-number claim (numbered interfaces only).** For every result carrying a `UnitNumber`, scan `currentEthCards` for a device whose observed slot equals it; claim that device immediately if found. **A numbered result that misses here MUST NOT participate in pass 2** — it is not eligible for MAC/ExternalID/backing fallback under any circumstance (that's the identity guarantee: a declared number identifies specific hardware or nothing, never "whatever backing-matches instead"). A miss falls straight through to the ordinary unmatched/Add path (Problem 2).
2. **Fallback claim (un-numbered interfaces only).** For results with no `UnitNumber` at all, run the existing MAC/ExternalID/backing matching against whatever devices pass 1 left unclaimed.

That's the whole algorithm — there is no pass 3, no slot-move, and no swap-collision handling to implement: two independent interfaces exchanging numbers in the same update (A: 8→9, B: 9→8) each find and claim whatever pass 1 finds at their own declared slot (which happens to be the other's old device) with zero coordination between them required, and zero device changes emitted for the exchange itself — see Design point 3 above.

**Second call site.** `FindMatchingEthCard` is also called from `fixupMacAddressMutableNetworks` in `session_vm_update.go` (lines 929-977; the second call site is at line 966 — the post-reconfigure MAC-address fixup for results without a `DeviceKey`). That call site must pass the result's `UnitNumber` too, so the fixup pass matches devices with the same exact-slot-only semantics as the reconcile pass (still no fallback for numbered interfaces here either). **Consequence to be aware of:** if vSphere placed a just-Added device at a slot other than the requested one, this fixup's exact-slot lookup misses as well, so that result never records its `DeviceKey`/`MacAddress` — bootstrap args and status silently lack the MAC for that interface, on top of the churn described in point 4 above. Both symptoms share the single root cause (Add not honoured) and the single gate (T001); an implementer seeing a missing MAC here should not chase it as an independent bug.

**Locating a device at the unit number identifies it — it does not mean "no change".** Today `ReconcileNetworkInterfaces` emits nothing at all on a match, so the compare-and-converge step below is entirely new behavior, not a tweak to existing diffing:

- **Compare** the located device against the desired state on backing, MAC *only when specified* (`AddressType: Manual`), and ExternalID *only when specified* (non-empty). This is the same predicate the existing matcher encodes — extract it from `FindMatchingEthCard` into a reusable per-device check rather than writing a second, divergent comparison. Unspecified fields are not compared and must never trigger a change (that is what prevents false-positive churn).
- **All specified fields agree → no device change.** Adopt the device's `Key`/`MacAddress` into the result exactly as the existing match branch does (`reconcile.go:36-38`) — that adoption is correct here and is how a `Generated` MAC gets learned.
- **Anything specified disagrees → Remove the located device + Add the desired device at the same `UnitNumber`, in the same `ReconfigVM_Task`** (not an Edit — Problem 3 and Design point 7). vSphere supports freeing and reusing a unit number in one Reconfigure. **Two implementation requirements on this branch:** (i) **I4 bookkeeping** — do NOT adopt the removed device's `Key`/`MacAddress`; leave `DeviceKey=0` and set `UpdatedEthCards=true` so `fixupMacAddressMutableNetworks` re-identifies the new device by unit number instead of chasing a removed device's stale key/MAC into bootstrap args and status; and (ii) **ordering** — this Remove must be emitted into the removes-first portion of the returned change list, alongside the trailing unclaimed-device removals, not appended after the Add (see Design point 5).
- **Differing device type is OUT OF SCOPE for this MR (I2) and MUST be excluded from the comparison — type support ships in a future MR.** A located device of a differing type is left alone: no Edit, no replacement. `spec.network.interfaces[i].type` is not consulted here; an empty/unset `type` continues to mean "no preference," so a class-ConfigSpec E1000/SR-IOV device is not churned toward the default `vmxnet3` (`defaultEthernetCardType = "vmxnet3"` at `network.go:108`, used unconditionally by `createDefaultEthCardFromDevice`; comparing against that hardcoded default would churn every non-vmxnet3 class device, which excluding type avoids).

**A numbered-interface miss in pass 1 is handled entirely by Problem 2 (Add) plus the existing unmatched-device removal (`reconcile.go:76-83`).** I4 does not apply to *that* branch — the miss never enters a match branch, so its Add builds the device fresh from spec (T007) with no old `Key`/`MacAddress` in scope. I4 applies specifically to the compare-then-replace branch above.

#### Problem 2: Add config spec must carry `UnitNumber` — in BOTH the reconcile and create paths

When no match is found in `ReconcileNetworkInterfaces`, an Add config spec is emitted using `r.Device` as-is. If `r.UnitNumber` is non-nil and vSphere honours explicit placement (confirmed by T001 research), set `r.Device.GetVirtualDevice().UnitNumber = *r.UnitNumber` before appending the Add config spec.

**The VM create path is separate and must be updated too — and it has TWO branches.** The initial VM `ConfigSpec`'s NIC devices are built in `vmprovider_vm.go`, not via `ReconcileNetworkInterfaces`, and the create path consumes `network.Device` (from `CreateNetworkDevices`), not `NetworkInterfaceResult` — so `Device` also needs a `UnitNumber *int32` field, populated from the spec interface in `CreateNetworkDevices` when the flag is on. The two branches:

1. **Class-ConfigSpec devices**: ethernet devices already present in the VM class ConfigSpec are positionally zipped to `createArgs.NetworkDevices` and mutated via `ApplyNetworkDeviceToVirtualEthCard`. These devices may **already carry a `UnitNumber` from the class ConfigSpec**. When the spec interface carries an explicit `unitNumber`, it overrides the class device's value (spec wins, under the T001 gate). When the spec interface has no value, a class-provided unit number is left as-is (vSphere honours or reassigns it) and the post-create backfill records the observed value. Note the validation webhook cannot see the class ConfigSpec, so a spec-vs-class collision only surfaces at create time — vSphere rejects the ConfigSpec (vcsim returns `InvalidDeviceSpec` for a taken slot); T001 confirms the real fault and the create error path must surface it clearly.
2. **Default devices**: spec interfaces beyond the class device count get devices from `network.CreateDefaultEthCardFromNetworkDevice`. When the spec interface carries an explicit `unitNumber`, set it on the device (T001 gate).

Only user-pinned values exist at create time — a VM being created is never in the upgraded state, so the mutator has not assigned anything (see "Mutation webhook"). Interfaces without a pinned value are created without a spec-driven unit number; vSphere assigns the slot and the backfill records it into the spec on the first post-create reconcile.

This is gated on research: if vSphere does NOT honour the field, omit it and let vSphere auto-assign. **Note — this is a termination concern, not just a placement nicety (see "Design" above):** if an Add ever lands at a different slot than requested, the *next* reconcile's exact-slot lookup (Problem 1, pass 1) still finds nothing at the requested number for that interface, so it Adds again — Removing the mismatched previous attempt each time via the existing unmatched-device cleanup — repeating without ever settling. There is no Edit-based correction step to fall back on. If T001 shows vSphere does not honour the field, spec-driven placement cannot ship as designed here; the open question in spec.md must be revisited before implementation (e.g., is there vCenter/host/hardware-version-dependent behaviour T001 should characterize more precisely, rather than a blanket "not honoured"?).

**Verification note (R2) — correcting the "OVF is dominant" premise.** These two branches populate `createArgs.ConfigSpec`, which is consumed by whichever create path `CreateVirtualMachine` dispatches to (`vmlifecycle/create.go`): `folder.CreateVM` (used for FastDeploy, and for ISO when FastDeploy is off), or the OVF content-library path (`deployOVF`), which marshals the *same* `ConfigSpec` to XML and passes it as `deploymentSpec.VmConfigSpec` to the vAPI `DeployLibraryItem` (`create_contentlibrary.go`). **OVF-via-content-library is not the dominant create path** — `deployFromContentLibrary` prefers FastDeploy when the feature is enabled and only falls back to `deployOVF` for an OVF-type library item when FastDeploy is disabled (re-confirmed against the current `create_contentlibrary.go`: `deployFromContentLibrary` checks `pkgcfg.FromContext(vmCtx).Features.FastDeploy` before choosing `fastDeploy` vs. `deployOVF`/`createVM`; note the current file also carries a now-vestigial `var _ = deployOVF` suppression line, since `deployOVF` is in fact genuinely called). It is nonetheless a real, currently-reachable path (FastDeploy-off deployments), and whether the vAPI's XML-`ConfigSpec` overlay honours a per-device `UnitNumber` — and how it interacts with NIC devices already present in the OVF descriptor itself — is a distinct, untested question from the `folder.CreateVM` path. T001 adds this as its own experiment. If the OVF path behaves differently, T029 needs a per-path gate rather than one blanket "T001 confirms" gate covering both.

#### Problem 3: Edit config spec — no code change, and numbered interfaces bypass the orphaned-CR Edit entirely

**This feature makes NO change to any Edit path.** `unitNumber` is never edited on any device, and — per Design point 7 — this MR does not introduce *any* new Edit: a numbered interface whose device disagrees with the spec is converged by replacement (Problem 1), not by editing. The orphaned-CR path (`findExistingEthCardForOrphanedCR`, `reconcile.go` lines 93-141, only reachable when `Features.MutableNetworks` is on and the VM is powered off — `session_vm_update.go:790-799`) is left exactly as-is for un-numbered interfaces.

**But note what that means for numbered interfaces, because it is a behavioural narrowing rather than a no-op:** a numbered interface never reaches the orphaned-CR path — pass 1 either finds a device at its declared number (→ compare, leave or replace) or misses (→ Add), and routing a numbered interface through a name-based orphaned-CR lookup would let it bind to hardware at some other slot, which the identity rule forbids. So the device-preserving Edit that today can absorb a network migration for a MutableNetworks VM stops applying once its interfaces carry unit numbers; those migrations replace the device instead (new `Key`; new MAC when `Generated`). Accepted for this MR and the motivating reason for the deferred Edit follow-on — see Design point 7, Future work, T025, and T030.

#### Problem 4: `MapEthernetDevicesToSpecIdx` — unit-number-first pass

Add a unit-number-first pass at the top of `MapEthernetDevicesToSpecIdx` (both mutable and immutable paths), applying the same exact-only rule as Problem 1 — a numbered interface must not fall back to CR-based/positional matching here either, since a wrong mapping doesn't just mislabel status, it feeds `bootoptions_reconciler.go:235`'s boot-order device selection and would point network boot at the wrong NIC:

1. If the feature flag is on, build a `UnitNumber → VC device` map from the current ethernet cards.
2. For each spec interface with `unitNumber` set, look up the map. If found, claim that card. **If not found, that interface gets no entry in the resulting map — it does NOT fall through to CR-based/positional matching.**
3. For spec interfaces with **no** `unitNumber` set, fall through to the existing per-provider CR-based matching (mutable) or positional zip (immutable), exactly as today.

This:
- Avoids expensive Kubernetes API calls for interfaces that already have a unit number.
- Correctly handles interfaces added or removed since the last schema upgrade.
- Keeps the existing CR-based path as a robust fallback for un-numbered interfaces.

**The miss in step 2 is not transient, and its callers degrade badly on it (I15, spec.md G13).** A `unitNumber` change is *admitted* on a powered-on VM but not applied until the VM is next powered off (I5) — potentially days — and for that whole window the interface declares a slot no device occupies. What each caller does today with a missing entry:

- **Status** (`updateGuestNetworkStatus`, `update_status.go:1194`): entries are built from guest info regardless, and the interface name comes from this map — a miss yields `ifaceName == ""`, so the NIC appears in `status.network.interfaces` with an **empty `name`**. A user-visible regression on a running VM whose only "fault" was an admitted spec edit.
- **Boot options** (`bootoptions_reconciler.go:235-249`): the reconciler scans the map for the interface's index and, finding nothing, returns `unable to locate network interface matching name %q` — a **hard error that fails the reconcile**, every reconcile, for any VM whose `spec.bootOptions` boot order names an ethernet device.

Neither is acceptable, and neither is fixed by relaxing the exact-only rule — falling back would reintroduce precisely the misattribution G11 forbids. The mapping stays exact-only; the **callers** must tolerate the miss:

- **Status**: "resolve the name as it is resolved today" is circular — *today* is the map that now misses, and its only fallback is the CR/positional matching **G11** forbids. Be precise about which entry is at risk, because two are being conflated: the **renumbered interface** has no status entry at all (entries are built from Tools guest info and it has no device), while the **device it left behind**, if still unclaimed and still reported by Tools, is the entry that would come out with an empty name. Concretely: have `MapEthernetDevicesToSpecIdx` return the authoritative exact-only map that boot options consumes, and give the status path a second, **name-resolution-only** pass that may fall back for leftover devices — it labels a status entry and can never move hardware. If that separation is judged not worth the plumbing, the alternative is to accept the empty name and document it; what is not acceptable is relaxing the single map both callers share.
- **Boot options**: treat "no device for this interface yet" as not-yet-converged rather than an error — skip that boot-order entry and surface it via the **G8** condition instead of failing the reconcile.
- **Per-NIC device fields and status** (third consumer, I17/I20): `updateInterfaceStatus` (`reconciler.go:330-348`) locates its status entry **by interface name**, so an entry that loses its name also silently loses `vnumaNodeID` and `vmxnet3` status. Covered by T035's matcher change plus whichever status-naming rule is chosen above.

Both are new work introduced by this feature, not pre-existing behaviour: today no interface can fail to map for this reason. T020/T021 cover the miss explicitly and T033 covers the boot-options change.

#### Problem 5: Immutable-networks positional zip

The positional zip in `MapEthernetDevicesToSpecIdx` becomes a last-resort fallback: used only when a spec interface has no unit number and `MutableNetworks` is off. **Reachability (I12, revised by I18):** on an upgraded VM every interface is numbered — including one the backfill skipped, which the mutator numbers on the next request — so the zip is reached only while a VM is awaiting schema upgrade. That is a bounded window, not a standing state, but it is not dead code: keep the path and test it.

#### The positional zip rests on an ordering the API does not guarantee (I27)

Worth stating once, because four separate places depend on it: `spec.network.interfaces` is declared `+listType=map` / `+listMapKey=name` (`api/v1alpha6/virtualmachine_network_types.go:376-379`). Under that declaration the list is a **set keyed by name**, and its order carries no meaning to the API server — server-side apply merges by key and is free to produce an order no client submitted.

Every positional zip in this feature therefore rests on a premise the API does not promise: the immutable-networks branch of `MapEthernetDevicesToSpecIdx`, `defaultNICMatcher` (Problem 6), the backfill's last-resort zip (T014), and `NICConfigFromMoVM`'s existing zip (T016). None of this is introduced here — the zips predate the feature — but two consequences belong in the plan:

- **It strengthens the case for unit numbers** and for retiring each zip as its consumers gain a real key.
- **The backfill's zip can record a wrong value for a reason unrelated to ambiguity**, which is an independent argument for the `NICUnitNumberBackfillAmbiguous` Event (T014) and for the divergence condition (T036) — a positional mis-record is not detectable from the resulting spec.

The mutator's "assign in list order" is unaffected in any way that matters: which free slot a new interface receives is arbitrary anyway, and the value is written once and then owned by the interface.

#### Problem 6: `pkg/vmconfig/networkextraconfig` is a fourth matcher, and it edits devices (I17)

`FindMatchingEthCard` and `MapEthernetDevicesToSpecIdx` are not the only places a spec interface is mapped to an ethernet device. `defaultNICMatcher` (`nic_matcher.go:29-44`) is an **unconditional positional zip** — `spec.network.interfaces[i]` to `collectManagedEthernetDevices(...)[i]` — and it is consulted twice per reconcile (`reconciler.go:111,122` for the config path; `:251,256` for the status path). It drives three things:

- the per-NIC ExtraConfig overlay (`reconciler.go:137`),
- **per-device field Edits** — `NumaNode`, `Uptv2Enabled` — via `reconcileNICFields` → `findOrCreateDeviceEdit` (`nic_fields.go:123-125`, `:206-224`),
- **per-interface status** — `vnumaNodeID`, `vmxnet3` — via `updateInterfaceStatus` (`reconciler.go:330-375`).

Two problems, in order of how long they last:

**(a) The zip breaks exactly where this feature changes things.** A **G12** replacement appends the new device to the hardware list, so spec order stops matching device order and the zip writes interface *i*'s desired NUMA/UPTv2 state onto interface *j*'s device — silently, and on every reconcile from then on. This is the misattribution **G11** exists to prevent, in a code path no earlier draft covered. Unit numbers are the fix this matcher has been waiting for; the `NICDeviceMatcher` function type (`nic_matcher.go:20`) is already the injection point.

**(b) `Remove(key X)` + `Edit(key X)` can land in one `ReconfigVM_Task`.** The ConfigSpec is shared: `getConfigSpecForPoweredOffVM` appends the ethernet device changes (`session_vm_update.go:740-744`), `poweredOffReconfigure` passes that same spec to `doReconfigure` (`:473-509`), and `doReconfigure` runs `reconcileNetworkExtraConfig` against it (`:1563-1575`). The resize path has the same shape (`:1023-1027`). `findOrCreateDeviceEdit` scans only for an existing **Edit** entry for that key and never checks for a Remove, so a replacement plus a coincident NUMA/UPTv2 diff produces Remove + Add + Edit of the removed key. Narrower than (a) — it needs the two to coincide — and a weak form pre-exists today for un-numbered interfaces whose backing changed, but this feature makes replacement the normal convergence path.

**Solution (T035):** inject a unit-number-based matcher when `VMNetworkUnitNumbers` is on, applying the same exact-only rule as Problems 1 and 4 — a numbered interface with no device at its slot matches nothing rather than zipping onto a stranger's device — and keep the positional zip for un-numbered interfaces and for the flag-off path. Separately, make `findOrCreateDeviceEdit` skip the device when the ConfigSpec already carries a Remove for that key; a device being replaced has nothing to edit, and the replacement Add is built fresh from the interface's own desired state.

Note this cannot be dismissed as a two-flag corner: the plan's own argument for a standalone feature-version bit is that `TelcoVMServiceAPI` and `VMNetworkUnitNumbers` are orthogonal and independently enableable.

#### Summary table

| Location | Current behaviour | Change (flag on) |
|---|---|---|
| `NetworkInterfaceResult` struct | No `UnitNumber` field | Add `UnitNumber *int32` |
| `reconcileNetworkInterfaces` | Builds results from spec, no unit number | Populate `result.UnitNumber` from `interfaceSpec.UnitNumber` |
| `FindMatchingEthCard` | Matches on MAC/ExternalId/backing | New `unitNumber *int32` param; when non-nil, exact-slot lookup ONLY — no fallback on a miss (pass 1); un-numbered results still use MAC/ExternalId/backing (pass 2) |
| `ReconcileNetworkInterfaces` — device located at the declared unit number | Match means no change (no diff at all is emitted today) | Compare the located device on backing + MAC-if-specified + ExternalID-if-specified (reuse the predicate extracted from `FindMatchingEthCard`; exclude device type per I2). All agree → no device change, adopt its `Key`/MAC as today. Any disagree → **Remove + Add at the same `UnitNumber` in one Reconfigure, not an Edit** (interim; Edit deferred to later work) |
| `ReconcileNetworkInterfaces` — replace branch (compare said "differs") | N/A | Emit Remove(located device) + Add(desired @ same `UnitNumber`). **I4:** leave `DeviceKey=0`, set `UpdatedEthCards=true` — do NOT adopt the removed device's `Key`/`MacAddress`, so `fixupMacAddressMutableNetworks` re-identifies the new device by unit number. **Ordering:** this Remove must land in the removes-first portion of the change list, not after the Add |
| `ReconcileNetworkInterfaces` — Add | Sets no `UnitNumber` on device | Set `r.Device.UnitNumber = *r.UnitNumber` when non-nil (gated on T001 research). **This is also how a renumbered existing interface is handled** — a numbered miss in pass 1 routes straight here; the interface's old device (if now unclaimed) is removed separately by the existing unmatched-device cleanup, not by any new code in this branch |
| `ReconcileNetworkInterfaces` — Edit (orphaned-CR path) | Updates backing/MAC/ExternalId only | **Unchanged code, but numbered interfaces never reach it** — they resolve via compare-then-replace instead, so the device-preserving Edit for network migrations stops applying to them (accepted narrowing; motivates the deferred Edit follow-on — Problem 3, Design point 7) |
| `fixupMacAddressMutableNetworks` (second `FindMatchingEthCard` call site) | Matches on MAC/ExternalId/backing | Pass `r.UnitNumber` for unit-number-first matching |
| `network.Device` struct / `CreateNetworkDevices` | No `UnitNumber` field | Add `UnitNumber *int32`, populated from spec interface (create path consumes `Device`, not `NetworkInterfaceResult`) |
| VM create path — class-ConfigSpec branch (`ApplyNetworkDeviceToVirtualEthCard`) | Class device keeps its class-provided `UnitNumber` (if any) | Explicit spec value overrides the class device value (spec wins, T001 gate); unset spec leaves class value as-is |
| VM create path — default branch (`CreateDefaultEthCardFromNetworkDevice`) | Sets no `UnitNumber` on initial ConfigSpec devices | Set desired `UnitNumber` on new NIC devices (gated on T001 research) |
| `MapEthernetDevicesToSpecIdx` | CR-based per-provider or positional zip | Unit-number-first pass; CR/zip as fallback (affects status AND boot-options callers) |
| `updateGuestNetworkStatus` | Writes Name/DeviceKey/IP/DNS | Also writes `UnitNumber` from matched VC device |
| `defaultNICMatcher` (`networkextraconfig`) | Unconditional positional zip; drives ExtraConfig, `NumaNode`/UPTv2 device Edits, and per-interface status | Unit-number matcher with the same exact-only rule; zip retained for un-numbered interfaces and flag-off (T035, Problem 6) |
| `findOrCreateDeviceEdit` (`networkextraconfig`) | Dedupes only against existing Edit entries | Must not emit an Edit for a device key the same ConfigSpec already Removes (T035) |
| `reconcileHardwareCondition` (`update_status_hardware_validation.go`) | Reports controller/volume/CD-ROM placement divergence | Also reports NIC unit-number divergence (T036, G8) |

### Status

`updateGuestNetworkStatus` (vmlifecycle/update_status.go) currently receives only `*vimtypes.GuestInfo` and the `deviceKey → specIdx` map — it has **no access** to the actual hardware device list, so it cannot read `VirtualDevice.UnitNumber` directly.

The fix requires threading a second map through `ReconcileStatusData`:

1. Add `NetworkDeviceKeysToUnitNumber map[int32]int32` to `ReconcileStatusData` in `update_status.go`.
2. In `vmprovider_vm.go`, in the call site that builds `ReconcileStatusData`, iterate `moVM.Config.Hardware.Device`, filter ethernet cards, and build the `deviceKey → VirtualDevice.UnitNumber` map.
3. Pass the new map into `ReconcileStatusData`.
4. In `updateGuestNetworkStatus`, use the map to set `status.network.interfaces[i].unitNumber` for each interface whose `DeviceKey` is in the map.

**Flag-gated.** Populating `status.network.interfaces[i].unitNumber` is gated on `VMNetworkUnitNumbers`: when the flag is off, the `NetworkDeviceKeysToUnitNumber` map is not built and the status field is not written, so the new API surface does not appear while the feature is disabled.

**VMware Tools dependency.** `updateGuestNetworkStatus` builds interface status entries by iterating `guestInfo.Net`; a VM without Tools running has no `status.network.interfaces` entries at all, even though unit numbers are available from `moVM.Config.Hardware`. The `unitNumber` status field therefore only appears for Tools-reported interfaces — the spec's acceptance criterion is scoped accordingly. Building interface status entries from hardware config for Tools-less VMs would be a broader behaviour change and is out of scope here.

## Test strategy

- **Unit tests**: for all new pkg functions — `validateNICUnitNumbers`, `NICUnitNumbersFromMoVM`, updated `FindMatchingEthCard`, updated `MapEthernetDevicesToSpecIdx`, updated `updateGuestNetworkStatus`, the boot-options miss handling, and the create-path device construction. Added to existing `*_test.go` files per package following the `_test` external package convention and `testlabels` Label patterns.
  - **Four cases are regression guards for the failure modes found in review; do not drop them as redundant:** (1) flag off + *unchanged* backfilled value → update allowed, and a backfilled VM deletes cleanly with the flag off (I14); (2) backfill observes a value that duplicates another interface's explicit value, or falls outside the range → no write, spec stays admissible, and the T036 condition reports the divergence (I13/I18 — note the mutator then numbers that interface on the same patch, which is expected, not a bug); (3) numbered interface with no device at its slot → status entry keeps its name (I15); (4) numbered interface with no device at its slot → boot-options reconcile does not error (I15).
  - Webhook test files use named `func xTests()` functions registered via `suite.Register` — not `var _ = Describe`. The new test file (`virtualmachine_validator_network_interfaces_test.go`) follows the same convention as `virtualmachine_validator_hardware_controllers_test.go`. The new mutation test file (`virtualmachine_mutator_network_unit_numbers_test.go`) follows the mutation package's existing conventions; its load-bearing case is "VM not schema-upgraded → nothing assigned", the regression guard for the brownfield ordering race.
  - Backfill/pkg unit tests use `var _ = Describe(...)` at top level (no `testlabels` on outer `Describe`).
- **Unit/conversion tests**: conversion fuzz tests in `api/v1alpha5` extended to cover round-tripping the new spec field through the annotation restore.
- **Integration tests**: covered by the webhook test suite via `builder.NewTestSuiteFor{Mutating,Validating}WebhookWithContext`; no separate integration test file needed. The **brownfield backfill** is verified here too (vcsim: VM with NICs and no spec unit numbers → schema upgrade runs → spec carries observed values, feature-version bit stamped) because the E2E environment cannot flip the flag/capability mid-run. **vcsim viability confirmed** against the pinned govmomi (`v0.55.0-alpha.0.0.20260622155957-a8a7db14910d`): it assigns NIC unit numbers from 7 and honours an explicit `UnitNumber` on Add, so backfill and placement scenarios are exercisable. It **cannot** evidence collision behaviour or same-slot Remove+Add for operator-shaped payloads (I26) — see T015 for the mechanism and the wording that test must not use.
- **E2E tests** (mandatory — cluster-observable; `test/e2e/vmservice/vmservice/virtualmachine/` — note double `vmservice`). Full scenario detail lives in `tasks.md` T022; summary:
  - **Scenarios that assert a device change only hold under a flag combination that reaches a converging reconcile path** — see "Reconcile entry points and flag interactions" above. State the assumed combination in the spec file and skip rather than fail when the environment does not provide it.
  - New file `vm_nic_unit_numbers.go` with `func VMNICUnitNumbersSpec(...)` following the `vm_networking.go` pattern, registered alongside `VM-NETWORKING` in `vmservice_test.go`.
  - Assignment on create with no explicit values (every interface numbered at admission, in range and unique; devices land at those slots; status matches after Tools reports).
  - Explicit placement on create, and mixed explicit/unset across multiple NICs to exercise the two-pass, exact-only-for-numbered matching order — both gated on T001 (does vSphere honour explicit `UnitNumber` on Add).
  - Add NIC while powered on: assert admission without a device change until the VM is next powered off/on (I5 — `poweredOnReconfigure` computes no NIC device changes today); renumber an existing interface powered off and assert NIC *replacement* (new device `Key`, old `Key` gone — not a slot move, per the Design section); remove a NIC identified by device `Key` (not just count), verifying survivors are unchanged.
  - Steady-state reconcile stability (`Consistently` — no spurious remove+add) and power-cycle stability (unit numbers unchanged across power off/on).
  - Conversion round-trip: set a `unitNumber`, submit an UPDATE via the `v1alpha2` manifest shape, re-read as `v1alpha6`, assert the value survived via the annotation-based restore — this is the only place the real API-server conversion chain (not just the fuzz tests) is exercised.
  - ~~Admission-validation scenarios~~ (duplicate, out-of-range, powered-on change rejection, nil→set allowance) — NOT covered in E2E: pure webhook logic with no cluster dependency, exhaustively covered by the `nicUnitNumberTests()` unit tests in T013 instead.
  - ~~Brownfield backfill scenario~~ — NOT covered in E2E: the deployment-level flag/capability cannot be toggled mid-run on a shared supervisor. Covered by vcsim integration tests instead (see above).
  - ~~Snapshot-revert/failover interplay~~ — NOT covered in E2E for the same reason; covered by vcsim integration tests under T031.

## Rollout / migration

- **Feature flag**: `pkgcfg.Features.VMNetworkUnitNumbers` (in the `FeatureStates` struct), default `false`. Decision needed (T002): `VMSharedDisks` is capability-driven (not an FSS env var) via `CapabilityKeySharedDisks` in `pkg/config/capabilities/capabilities.go`. `TelcoVMServiceAPI` is also capability-driven. Determine whether `VMNetworkUnitNumbers` follows the same capabilities path or if it warrants an FSS env var during development. Document the decision in this section once resolved. Either way the feature has its **own** flag/capability and its **own** schema-upgrade bit — it does not piggyback on `TelcoVMServiceAPI`.
- **Schema upgrade** (decision confirmed): standalone `NICUnitNumbersFromMoVM` step gated on `VMNetworkUnitNumbers` + `FeatureVersionNICUnitNumbers = 16`. Not part of `NICConfigFromMoVM` / the `TelcoVMServiceAPI` gate — that gate is one-shot, so already-stamped VMs would never be backfilled, and the two flags are orthogonal. See "Schema upgrade — brownfield backfill" above.
- **Enabling the capability makes every existing VM "not upgraded" until it next reconciles (I24).** Adding `FeatureVersionNICUnitNumbers` to `ActivatedFeatureVersion` makes `IsObjectUpgraded` fail for every VM whose annotation predates it. For the length of that window, and cluster-wide: T034's guard blocks **all** user edits to `spec.network.interfaces`; the CD-ROM and hardware-controller mutators stop assigning (they share the gate — `virtualmachine_mutator_cdrom.go:49-55`); and the powered-on/hot-add controller validations are skipped. This is inherent to adding any feature-version bit — the Telco bit did the same — but it is an operational effect of *enabling this capability*, not of installing the build, so it should be expected rather than diagnosed. Note the window is at least two reconciles when the operator build also changed: `ReconcileSchemaUpgrade` writes the build/schema annotations and returns `ErrUpgradeSchema` (`vm_schema_upgrade.go:114-119`) **before** any feature backfill runs, so the NIC backfill lands on a later pass.
- **Operator downgrade re-runs the backfill, harmlessly.** `ParseFeatureVersion` returns `FeatureVersionEmpty` for any value carrying a bit the running binary does not know (`features.go`, `IsValid`), so a binary predating this feature reads a VM stamped `17`/`31` as un-upgraded and re-stamps it at its own level; upgrading forward then re-runs `NICUnitNumbersFromMoVM`. That is safe because the backfill is spec-wins and idempotent: every interface already carries a value, so nothing is written and the bit is simply re-stamped.
- **Rollback safety**: the field is `+optional` / `omitempty`. Disabling the flag stops assignment and stops recording new values, but does not erase existing ones. Re-enabling triggers a fresh backfill pass only for VMs not yet processed (their feature-version annotation lacks the bit). **Decision: disabling the feature after the backfill has run is not a supported flow** — while the flag is off, an update that changes or adds a `unitNumber` is rejected, so a VM cannot be renumbered until the flag is re-enabled. Updates that merely carry an unchanged backfilled value are still accepted (**G10**/I14): rejecting those would also block unrelated spec edits, the operator's own patches, and finalizer removal, leaving backfilled VMs undeletable — a blast radius well beyond what "not a supported flow" was meant to accept. Also note the flag does not stop unit-number-based *matching* for VMs already backfilled unless the matching code paths check it too, so the flag check must sit on the matching path, not only on the write paths.
- **Old API versions**: the spec field round-trips through v1alpha5 and earlier via the annotation-based conversion restore (see "API / CRD strategy"); without that, old-version clients would silently wipe assigned values on UPDATE.
- **New failure mode: set→nil rejection × annotation-lossy old clients (I6).** The annotation-based conversion restore only protects a value when the client round-trips the `vm-operator/restore-conversion-data` annotation (or equivalent) it received. A v1alpha2 (or earlier) client that rebuilds the object from scratch — rather than read-modify-write — drops that annotation, so its UPDATE arrives with `unitNumber` absent. Because a set→nil transition is rejected while the VM is powered on (validation webhook, above), that client's otherwise-unrelated spec update now fails with "cannot change unit number while VM is powered on" instead of silently wiping the field. This is the intended trade-off (loud failure beats a silent k8s#111703-style wipe) but is a new, initially-confusing failure mode for annotation-lossy old-version clients that didn't exist before this feature; call it out in the release note (T025) and docs (T030).
- **Ordering constraint vs. the type-support follow-on.** A numbered interface whose device disagrees with the spec is converged by replacement (Design points 2b/7), and the replacement device's type comes from `spec.network.interfaces[i].type`, whose documented contract is "if omitted, VMXNet3 will be used for new network interfaces" (`api/v1alpha6/virtualmachine_network_types.go:200-204`; the enum already includes `SRIOV`, `E1000`, `E1000e`, `VMXNet2`, `PCNet32`). Setting `type` explicitly is therefore the sanctioned way to pin a non-default device type across a replacement — **but this MR's reconcile path does not consult `type` yet** (excluded per I2; honouring it is the deferred type-support work, T032). Consequence: in the window between this MR and that one, a VM that has the flag enabled, has an actual device of a non-default type, and has `spec.type` empty cannot pin that type, and a backing/MAC/ExternalID change replaces the device with a VMXNet3 one. Nothing in the code prevents enabling `VMNetworkUnitNumbers` standalone and hitting this, so treat it as a rollout constraint: **do not enable this feature in an environment that relies on class-ConfigSpec-provided non-default NIC types until the type-support MR has landed** (and factor it into the GA criteria, T027). Two things already reduce the exposure: the flag defaults to `false`, and the Telco `NICConfigFromMoVM` backfill already populates `type` *from the observed device*, so for VMs it has touched the spec data needed to preserve the type is present — only the consumer is missing.
- **Partner comms**: `status.network.interfaces[i].unitNumber` is a new observable field; partners reading network status should treat it as informational. No breaking change.

## Complexity tracking

| Consideration | Resolution |
|---|---|
| NIC PCI bus has no spec object (unlike SCSI controllers) | No new CRD needed: the bus is implicit and always present, so `NICBusSpec` exists only as a `ControllerSpec` implementation for slot computation, with no API surface |
| PCI bus shared with non-NIC devices; NIC slots are 7–16, not 0–9 | CRD markers `Minimum=7` / `Maximum=16`; validation range messages use 7–16. The bound is a platform contract, not an observed convention: PCI units are allocated statically per device class, and non-NIC classes occupy their own bands (`external/vim/api/v1alpha1/testdata/device_keys.txt`) |
| Backfill can write a value the CRD or webhook rejects, wedging every later spec patch (I13) | **New requirement (G7):** skip the write per interface when the observed value is out of range or already claimed in the spec; raise the Event; leave the interface on fallback matching. See "Schema upgrade — admissibility guard" |
| Rejecting the field while the flag is off blocks unrelated updates and finalizer removal (I14) | **Requirement narrowed (G10):** reject only a new or changed value, never an unchanged one. `ValidateUpdate` validates the whole object on every UPDATE and has no delete/no-change early-out |
| A numbered interface with no device at its slot blanks its status name and hard-fails the boot-options reconciler (I15) | **New requirement (G13):** the mapping stays exact-only, but the callers tolerate a miss — status keeps the interface name, boot options treat it as not-yet-converged rather than an error (T033). The state persists as long as a renumbered VM stays powered on |
| NIC device changes are computed on only some powered-off paths, one of them gated on `MutableNetworks` (I16) | Documented in "Reconcile entry points and flag interactions"; no change to the dispatch in scope. E2E must state the flag combination it assumes |
| Desired NIC device in reconcile has no `UnitNumber` | Extend `NetworkInterfaceResult` with `UnitNumber *int32`; populate from spec |
| Brownfield first-pass must use hodgepodge (no unit number yet) | `NICUnitNumbersFromMoVM` prefers MAC/ExternalID/backing match with positional-zip fallback; unit-number match used on subsequent reconciles |
| Webhook could guess unit numbers before the backfill runs | Gated on `IsObjectUpgraded`, exactly as the disk / CD-ROM / controller mutators do — the backfill records observed hardware first, and assignment starts on the request carrying its annotation stamp |
| A user could set a unit number before the backfill observes the hardware | The existing schema-upgrade-window guard already forbids changes to `spec.network.interfaces` until upgrade; widen its gate to include `VMNetworkUnitNumbers` (T034). With the mutator gate, this makes the backfill unconditionally first |
| `NextAvailableUnitNumber` assumes slots start at 0 with one reserved unit; NICs start at 7 | Add a first-usable-unit notion to `ControllerSpec` (0 for existing controllers, 7 for the NIC bus) rather than forking the helper |
| vSphere may not honour explicit `UnitNumber` on Add | **Gates whether this feature's design terminates, not just placement — see Design.** If Add doesn't honour the field, the next reconcile's exact-slot lookup misses again and re-Adds, non-terminating; T001 confirming it is a hard prerequisite |
| Unit-number match with a different device type | **Out of scope for this MR — type support ships in a future MR (I2).** A unit-number/fallback match against a device of a differing type is a no-op; `type` is not consulted when diffing |
| Spec≠device unit number on an existing interface (I1) | **`unitNumber` is a device identity, not a slot.** A renumbered interface is a plain miss → Add(new) at the new number; its old device, if now unclaimed, is Removed by the pre-existing unmatched-device cleanup. No Edit, no swap detection, no sequencing — see the Design section |
| Powered-on nil→set could be exploited by a user for a device-claim swap (I3) | Scoped to `ctx.IsVMOperatorAccount`; user-originated nil→set on a powered-on VM is rejected like any other change |
| An interface claiming a device that belonged to another interface could inherit that device's MAC alongside its own `ExternalId`, breaking MAC-pinned NSX-T/VPC ports while looking healthy | **Resolved by compare-then-replace:** a located device that disagrees on backing/specified-MAC/specified-ExternalID is replaced, and the replacement Add is built fresh from the claiming interface's own desired state, so the three are coherent by construction. Only the no-change branch adopts the device's MAC, which is correct there. **The deferred Edit follow-on must re-solve this** (Design point 3) |
| Converging a numbered interface by replacement rather than Edit costs a new device `Key`/MAC even for changes an Edit could absorb, and takes the orphaned-CR Edit optimization away from numbered interfaces | Accepted interim decision (Design point 7); Edit-instead-of-replace tracked as a follow-on (T032). Surfaced to users in the release note (T025) and docs (T030) since it narrows today's MutableNetworks migration behaviour |
| Same-slot Remove+Add must actually be accepted by vSphere, and the Remove must precede the Add in the change list | vSphere supports freeing and reusing a unit number in one Reconfigure (known-supported; T001 confirms on the target build, vcsim not authoritative). The existing code already returns `append(removeDeviceChanges, deviceChanges...)`, but the replace branch's Remove is emitted from inside the results loop — T017 must place it in the removes-first portion |
| Backfill zip mis-assignment is silent, cannot self-heal once written, and feeds boot-order device selection (I7) | **Escalated:** under identity semantics a numbered interface never falls back to MAC/backing again once numbered — a wrong backfilled value is not a cosmetic status problem, it makes the reconciler treat the *wrong* physical device as that interface's hardware and replace it to match the interface's spec. Kubernetes Event on zip-fallback and on spec/observed disagreement, not just a V(4) log |
| `NICConfigFromMoVM` (TelcoVMServiceAPI) uses positional zip | Per-interface (I9): unit-number lookup for numbered interfaces, positional zip only for the unnumbered remainder — not gated on "all interfaces numbered" |
| Old API versions drop the new hub-only spec field on UPDATE | Annotation-based conversion restore in `restore_v1alpha6_VirtualMachineNetworkInterfaces` + fuzz tests; annotation-lossy old clients now hit a loud set→nil rejection instead of a silent wipe (I6) |
| Two different matching functions with different call sites | Updated independently; `FindMatchingEthCard` gets unit-number param (both call sites: `ReconcileNetworkInterfaces` and `fixupMacAddressMutableNetworks`); `MapEthernetDevicesToSpecIdx` gets unit-number-first pass (status + boot-options callers) |
| Greedy single-pass matching could mis-claim an explicitly-requested slot | `ReconcileNetworkInterfaces` runs two passes: exact unit-number claim for numbered interfaces (no fallback on a miss, ever), then MAC/ExternalID/backing fallback for un-numbered interfaces against whatever's left. No swap-collision logic needed — see Design point 3 |
| VM class ConfigSpec may carry NIC devices with their own unit numbers | Create path: explicit spec value overrides the class device value; unset spec leaves it as-is; collisions surface as a vSphere create fault (T001 documents the fault type) |
| OVF-via-content-library create path untested (R2) | Not the dominant path (FastDeploy is preferred when enabled) but a distinct one — `deployOVF` → vAPI `DeployLibraryItem` with XML `VmConfigSpec`, versus `folder.CreateVM`. Its own T001 experiment |
| Snapshot revert restores spec + feature-version annotation from backup yaml | Investigated under T031; revert to a pre-backfill snapshot drops the bit and re-triggers the backfill |
| Out-of-band NIC added via vCenter UI shifts the immutable positional zip for other unnumbered interfaces (I11) | Pre-existing behavior, unrelated to this feature; no design change |
| Reflecting NIC hardware changes (incl. a renumber's Add/Remove) while the VM is powered on (I5) | Out of scope for this MR — tracked as a follow-on spec; today's powered-off-only reconcile code path is unchanged by this feature |
| E2E cannot toggle the flag/capability mid-run | Brownfield backfill covered by vcsim integration tests instead of E2E |
| A fourth spec↔device matcher exists in `networkextraconfig`, positionally zipped, and it *edits* devices and writes per-interface status (I17) | **New task T035.** Inject a unit-number matcher with the same exact-only rule; keep the zip for un-numbered interfaces and flag-off. Separately, `findOrCreateDeviceEdit` must not add an Edit for a device key the same ConfigSpec already Removes. See Problem 6 |
| The mutator numbers an interface the backfill deliberately skipped, and G11 makes that number load-bearing (I18) | **Accepted, matching disks** (`hasPlacementMismatch` skip → `AddControllersForVolumes` assigns). G7 is re-scoped to admissibility only; the possible NIC replacement is owned and surfaced by G8's condition. Do not add an `IsVMOperatorAccount` guard to the mutator. See "Consistency with the disk placement model" |
| Widening `ControllerSpec` means changing shipped API types in another Go module (I21) | Use an optional `interface{ FirstUnitNumber() int32 }` asserted inside `NextAvailableUnitNumber`, implemented only by `NICBusSpec`; scan `First .. First+MaxSlots-1` so `MaxSlots` keeps meaning "count" |
| "No slot available" cannot be produced through the API (I22) | `MaxItems=10` + ten slots + uniqueness ⇒ pigeonhole. Keep the branch as defensive; note in T010/T011 that it is only reachable by invoking the mutator directly |
| vcsim cannot evidence unit-number collisions for operator-shaped payloads (I26) | Duplicate check bails on differing `ControllerKey`, and operator-built cards leave it unset. T015 must not claim collision coverage; T001 items 6/8 are the only evidence |

## Future work (tracked separately, not part of this MR)

Three items are explicitly out of scope here and ship as follow-on work rather than being folded into this plan:

- **Edit-instead-of-replace for a numbered interface's device (Design point 7).** This MR converges any disagreement between a numbered interface and the device at its declared unit number by Remove+Add at that unit number. Later work should emit a device-preserving Edit for the changes an Edit can express (backing, MAC, ExternalID), so that e.g. a network migration keeps the device, its `Key`, and its MAC. Two constraints that work inherits: (a) it must resolve MAC from the claiming interface's addressing mode rather than adopting the located device's — emit the interface's own pinned MAC when `AddressType: Manual`, adopt only when `Generated` — otherwise it reintroduces the `ExternalId`/MAC incoherence that replacement currently sidesteps (point 3); and (b) it is what restores the orphaned-CR Edit optimization's benefit to numbered interfaces, which this MR gives up.

- **Type-changing convergence (I2).** Constructing a typed desired NIC device from `spec.network.interfaces[i].type` and emitting a Remove+Add when a matched device's type disagrees with it. This spec's matching/convergence logic treats an unset **or mismatched** `type` as "no preference" and takes no hardware action on a type difference. **Type support ships in a future MR.**
- **Powered-on NIC hardware convergence (I5).** Today (and after this feature ships), NIC device changes — including the Add/Remove pair produced by renumbering an existing interface — are computed only in the powered-off reconcile path (`getConfigSpecForPoweredOffVM` / `resizeVMWhenPoweredStateOff`); re-confirmed that `poweredOnReconfigure` emits no NIC device changes. A new interface, or a renumbered existing one, may be *admitted* to a powered-on VM's spec, but neither its Add nor its Remove takes effect until the VM is next powered off and reconciled. **As a follow-on to this work, we will allow these changes to be reflected while the VM is powered on** (tracked as its own future spec — see T032 in `tasks.md`).
