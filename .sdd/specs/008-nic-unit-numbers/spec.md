# Feature Specification: NIC Unit Numbers

- **Feature branch**: [`feature/vm-operator-net-unit-numbers`](https://github.com/vmware-tanzu/vm-operator/tree/feature/vm-operator-net-unit-numbers)
  - **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-06-25
- **Last updated**: 2026-08-14
- **Status**: In Progress
- **Epic**: vmop-3982

---

## Summary

A VM's network interfaces are ethernet cards on the VM's single virtual PCI bus, and each card occupies a numbered slot on that bus. Today that slot is invisible in the VM Operator API and is never used to identify a NIC: a spec interface is matched to its vSphere device by MAC address, external ID, and backing — values that legitimately change over an interface's lifetime, most notably during a network migration.

This feature surfaces the slot as `spec.network.interfaces[i].unitNumber` and `status.network.interfaces[i].unitNumber`, lets a user pin an interface to a specific slot, and makes that value the interface's **identifier** for its vSphere device. A VM's existing slots are recorded into the spec by a schema-upgrade backfill; from then on, an interface added without a unit number is assigned one at admission, the same way disk and CD-ROM unit numbers are assigned.

All behaviour described here is gated behind the `VMNetworkUnitNumbers` feature flag, which defaults to off.

## Terminology

| Term | Meaning |
|---|---|
| **Unit number** | The ethernet device's slot on the VM's virtual PCI bus. Distinct from the device's PCI slot number, which this feature neither reads nor writes. |
| **Numbered interface** | A `spec.network.interfaces[i]` entry whose `unitNumber` is set — whether set by a user or recorded by the backfill. |
| **Backfill** | The one-shot schema-upgrade step that records observed unit numbers into the spec. It serves brownfield VMs on operator upgrade **and** greenfield VMs on their first post-create reconcile — a VM has no upgrade annotations when it is created, so admission cannot assign anything for it yet. |
| **Fallback matching** | The existing MAC address / external ID / backing matching, which remains the only matching used for un-numbered interfaces. |

---

## Goals

Goals carry stable identifiers (`G1`…`G16`); `plan.md` and `tasks.md` reference them by ID. Never renumber one — retire it in place if it is dropped.

### The API surface

- **G1 (MUST)** — `spec.network.interfaces[i].unitNumber` MUST identify the interface's slot on the virtual PCI bus. The field is optional. Valid values are **7–16**: the platform allocates PCI units statically per device class, and ethernet cards own exactly that band (every other occupant — video, audio, storage HBAs, VMCI, passthrough — has its own reserved band). Ten units for at most ten interfaces, matching the existing cap on the interfaces list.

- **G2 (MUST)** — `status.network.interfaces[i].unitNumber` MUST reflect the slot observed in vSphere after each reconcile, for every interface that appears in the interface status. Status population MUST be gated on `VMNetworkUnitNumbers`: with the flag off the field is not written. Interface status entries are built from VMware Tools guest info, so this applies only to interfaces Tools reports; interfaces Tools does not report have no status entry at all today and this feature does not change that.

- **G3 (MUST)** — Older API versions (v1alpha5 and earlier) MUST round-trip `unitNumber` so that an UPDATE submitted by an old-version client does not silently wipe the value.

### How values get set

- **G4 (MUST)** — **The mutation webhook MUST assign a `unitNumber` to any interface that does not specify one, following the same pattern as disk and CD-ROM unit numbers.** Specifically:

  1. An explicit user value is never overwritten; it only reserves its slot.
  2. Remaining interfaces receive the next available slot in the valid range, in list order.
  3. Assignment MUST be skipped entirely — leaving values as submitted — until the VM has been schema-upgraded, i.e. until its build, schema, and feature versions are all current. This is what prevents the webhook from guessing slots ahead of the backfill (**G6**), where a guess would be locked in by spec-wins and could make matching claim the wrong device.
  4. When no slot is available, the mutator MUST leave the interface unassigned and let validation report it, rather than erroring in the mutating path.

  **A VM being created is never in the upgraded state** — the upgrade annotations are written by the VM's own reconcile, not at admission — so assignment is an update-path behaviour, exactly as it is for disks and CD-ROMs. A VM is therefore created with `unitNumber` exactly as the user submitted it, receives values from the backfill (**G6**), and from that point on any interface added without one is assigned a slot at admission.

- **G5 (MUST)** — An interface's `unitNumber` — whether the user pinned it or the webhook assigned it (**G4**) — MUST be carried onto the ethernet device, both on the VM create path and when a NIC is added to an existing VM. On the create path this MUST hold for NIC devices originating in the VM class ConfigSpec (the spec value overrides a class-provided one — spec wins) as well as for devices built for spec interfaces that have no class counterpart. At create time only user-pinned values exist (**G4**); interfaces without one are created without a spec-driven slot and vSphere assigns it.

  **This goal is gated on Q1, and G4 raises the stakes on that answer.** Once a VM is upgraded, every interface carries a value, so every subsequent Add asks for a specific slot — a "not honoured" answer then produces the churn described in **Q1** for ordinary NIC additions, not just for explicitly-pinned ones.

- **G6 (MUST)** — The backfill MUST record observed unit numbers into `spec.network.interfaces[i].unitNumber` — for brownfield VMs on operator upgrade, and for greenfield VMs on their first post-create reconcile, since admission cannot assign anything until the VM is upgraded (**G4**) — matching spec interfaces to devices with the existing fallback matching and using a positional zip only for interfaces that matching cannot uniquely claim. It MUST be a standalone schema-upgrade step with its own feature-version bit — not folded into the existing Telco-driven NIC backfill, whose gate is one-shot (already-stamped VMs would never be backfilled) and whose feature may be disabled while this one is enabled. An explicit spec value always wins over the observed value.

  The backfill records *observed* hardware, so it MUST run before the webhook is allowed to assign anything for that VM (**G4.3**). Interfaces added after a VM is upgraded are numbered by the webhook, not by the backfill — the backfill's one-shot nature is therefore not a source of permanently-unnumbered interfaces.

- **G7 (MUST)** — **Neither the backfill nor the mutation webhook may write a spec that validation would reject.** For the mutator this falls out of assigning only genuinely free slots and skipping when none remain (**G4**). For the backfill, which copies observed values rather than choosing them, it is an explicit rule: **the backfill MUST NOT write a spec that its own admission rules would reject.** For each interface it MUST skip the write, leaving the field unset, when the observed value is already claimed by another interface in the same spec — reachable when an explicit value was not honoured, or when a VM class ConfigSpec device occupies the slot — or, defensively, when it falls outside the valid range. Writing such a value would make every subsequent spec patch on that VM — including the operator's own — fail admission, wedging the VM. A skipped interface MUST be reported per **G8**.

  **This is the disk and CD-ROM behaviour, adopted deliberately** — `hasPlacementMismatch` skips a volume's whole placement write on any spec/observed disagreement and leaves the fields nil (`vm_schema_upgrade.go:676`). What that leaves behind is also the same: **a skipped interface does not stay unnumbered.** The mutating webhook has no privileged-account bypass (`virtualmachine_mutator.go:255`, and the sole account check in the package is `:692`), so it runs on the backfill's own patch — which carries the annotation stamp that opens the **G4.3** gate — and on every later update of any kind. A nil `unitNumber` on an upgraded VM is indistinguishable from a newly-added interface, so the mutator assigns it the next free slot, exactly as `AddControllersForVolumes` does for a skipped volume.

  **Where NICs diverge from disks, and the accepted consequence.** For a disk that assignment is harmless: a volume's identity is its PVC name / disk UUID, the unit number is placement metadata only, and a spec-vs-hardware divergence is *reported* — `VirtualMachineHardwareDeviceConfigVerified` — never converged. Under **G11** a NIC's unit number *is* its identity, so a slot the mutator invents for a skipped interface is acted on: the interface resolves against whatever device sits there and replaces it if it disagrees. **This spec accepts that**: the guarantee G7 provides is admissibility, not placement correctness. A VM that reaches the skip case can lose and re-create NIC hardware on its next powered-off reconcile, with new device keys and new generated MACs, and **G8**'s signal is what makes that traceable.

- **G8 (MUST)** — A NIC unit-number mis-assignment MUST be operator-visible, not merely a log line. It feeds device matching, status, and network boot-order selection, and under **G7** it can cost the VM its NIC hardware, so it needs a trace an operator can alert on. Two mechanisms, split by whether the fact is observable after the fact:

  1. **A steady-state condition, following the disk and CD-ROM precedent.** Each reconcile MUST compare every numbered interface's declared slot against the observed ethernet-device slots and report divergence — an interface whose declared slot holds no device, or holds one that disagrees with the interface's desired state — the way `checkVolumes` / `reconcileHardwareCondition` report volume placement divergence (`update_status_hardware_validation.go:289`, `:196`), rather than as a one-shot Event. This is the primary signal: it catches a bad backfill value, a slot the mutator invented for a **G7**-skipped interface, and out-of-band drift, with one mechanism, and it clears when the VM converges.
  2. **A Kubernetes Event for the one fact that is not observable later**: that an interface's value came from the backfill's positional zip rather than a unique match. Nothing in the resulting spec records this, so it MUST be raised as a `Warning` Event at backfill time.

  A pre-existing explicit value that disagrees with the observed slot is covered by (1) — the reconciler sees the same divergence on every subsequent reconcile — and needs no separate Event.

- **G9 (MUST)** — The validation webhook MUST reject: two interfaces sharing a `unitNumber`; a value outside the valid range; an interface left without a slot because none was available (**defensive only** — with `MaxItems=10` on the interfaces list, exactly ten valid slots, and uniqueness enforced, pigeonhole guarantees a free slot; the eleventh interface is rejected by the CRD schema first, so this case is not reachable through the API); and, while the VM is powered on, changing an already-set value or clearing one.

  The powered-on rules apply to **interfaces that already existed on the VM**, not to a newly-added interface, whose value the mutator supplies in the same request. For an interface that existed with no value — reachable only for a VM whose backfill skipped it (**G7**) — a nil → set transition on a powered-on VM MUST be allowed only for requests made by the VM Operator service account, so a user cannot claim a slot held by another interface's device and trigger a device-identity swap on the next reconcile. Like the disk rules, the powered-on checks MUST be evaluated against the *old* object's upgrade state, so a VM being schema-upgraded in this very request is not judged by rules its spec has not yet been backfilled to satisfy.

- **G10 (MUST)** — While `VMNetworkUnitNumbers` is disabled, the validation webhook MUST reject a request that **sets a new value or changes an existing one**, rather than silently ignoring it, so values cannot be persisted while the feature is off and later poison the spec-wins backfill. It MUST NOT reject a request that merely carries an unchanged, previously-backfilled value: validation runs against the whole object on every UPDATE, so rejecting on presence would block unrelated updates, the operator's own writes, and finalizer removal — i.e. it would make an already-backfilled VM undeletable while the flag is off.

- **G16 (MUST)** — **User updates to `spec.network.interfaces` MUST be rejected until the VM's schema upgrade has completed**, as they already are for the other spec fields the schema upgrade backfills. VM Operator's own writes are exempt, which is what lets the backfill record observed values. Together with **G4.3** this gives one unambiguous ordering: no user-supplied value and no webhook-assigned value can precede the backfill's observation of what the VM's hardware actually uses. The existing rule already covers this field set; it is currently reached only when the Telco feature is enabled, and MUST also apply when `VMNetworkUnitNumbers` is enabled, since the two features are independent.

### What a unit number means

- **G11 (MUST)** — **Once set, an interface's `unitNumber` is that interface's exclusive identifier for its vSphere device — not a label on a slot a device can be moved between.** This applies at **every** point that maps a spec interface to an ethernet device — there are four, and all four MUST use it: device reconciliation (`FindMatchingEthCard`), the post-reconfigure MAC-address fixup, the device-to-spec-interface mapping used for status and boot options (`MapEthernetDevicesToSpecIdx`), and the per-NIC matcher that drives ExtraConfig, per-device field edits (`NumaNode`, UPTv2) and per-interface status in `pkg/vmconfig/networkextraconfig` (`defaultNICMatcher`, today an unconditional positional zip). At each of them:

  1. A numbered interface is matched **only** by an exact lookup against the VM's current ethernet-device slots. It is never matched by fallback matching — not as a first choice and not as a recovery on a miss. Matching it any other way would let a device the spec did not name impersonate the identity the spec declared.
  2. Un-numbered interfaces are then matched by fallback matching against whatever devices step 1 left unclaimed.

  Uniqueness validation (**G9**) guarantees at most one interface can declare any given slot, so step 1 is contention-free: two interfaces exchanging declared numbers in a single update need no swap detection, collision check, or cross-reconcile sequencing. Each simply resolves against whatever it finds at its own declared slot.

- **G12 (MUST)** — Locating a device at an interface's declared slot answers "which device is this interface", not "is that device correct". Having located it, the reconciler MUST compare it against the interface's desired state and converge:

  - **Compare** the device's backing; its MAC address **only when the desired state pins one**; and its external ID **only when the desired state specifies one**. Fields the desired state leaves unspecified MUST NOT be compared and MUST NOT trigger a change — this is the same predicate the existing matcher already applies, so a correct device produces no device change at all. Device **type** is excluded from the comparison (see Non-goals).
  - **Everything specified agrees** → no device change; the interface adopts the device's key and MAC, which is how a generated MAC is learned.
  - **Anything specified disagrees** → **replace the device**: remove the device at that slot and add the desired device carrying the same unit number, in one reconfigure. The replacement MUST NOT inherit the removed device's key or MAC; the post-reconfigure fixup re-identifies the new device by its unit number instead of chasing a removed device's stale identity into bootstrap arguments and status.
  - **No device at the declared slot** → the interface has no vSphere hardware: it follows the ordinary add path carrying the desired unit number. It MUST NOT be routed through the orphaned-CR edit path, which identifies a device by interface name and could therefore bind the interface to hardware at another slot. Its former device, if no other interface claims it, is removed by the existing unmatched-device cleanup.

  **Replacement, not in-place edit, is a deliberate interim decision for this change set** (see Non-goals) — including for changes an in-place edit could express losslessly.

- **G13 (MUST)** — A numbered interface with no device at its declared slot MUST degrade cleanly. It has no device-to-spec mapping by construction (**G11**), and the reconciler MUST NOT, on that account, fail the reconcile, drop the interface's name from status, or silently retarget network boot order at a different NIC. This state is reachable and persistent, not transient: a `unitNumber` change is admitted on a powered-on VM but not applied until the VM is next powered off (see Non-goals).

  **Three consumers must tolerate the miss, not two.** Besides the interface name in `status.network.interfaces` and boot-order device selection, `pkg/vmconfig/networkextraconfig` locates its status entry **by interface name** (`reconciler.go:337`), so an entry that loses its name also silently loses its `vnumaNodeID` and `vmxnet3` status. A miss MUST NOT be resolved by falling back to CR-based or positional matching, which would reintroduce exactly the misattribution this goal exists to prevent; the callers absorb it instead.

  Distinguish two distinct entries when specifying the fix: the **renumbered interface** has no status entry at all (status entries are built from Tools guest info, and it has no device), while the **device it left behind**, if unclaimed, still reports through Tools and is the entry at risk of an empty name.

### Delivery

- **G14 (MUST)** — All new behaviour — matching, placement, backfill, and status population — MUST be gated behind the `VMNetworkUnitNumbers` feature flag.

- **G15 (SHOULD)** — A govmomi research program SHOULD be written and run against a real vCenter to answer the gating questions below before the matching and placement logic is finalized.

---

## Non-goals

- **No new API object for the bus.** NICs remain a flat list on the single implicit virtual PCI bus; no NIC-controller CRD is introduced and the bus is not represented in `spec.hardware`.
- **No change to network provider CRs.** The unit number is a vSphere-layer identifier; NetOP / NCP / VPC CRs are untouched.
- **No SR-IOV passthrough placement.** Physical/virtual function placement is out of scope. SR-IOV NICs in `spec.network.interfaces` are still ethernet cards on the virtual PCI bus and *are* covered by this feature's placement, validation, backfill, and matching like any other interface type. This rests on the platform's static per-device-class PCI unit allocation (`external/vim/api/v1alpha1/testdata/device_keys.txt`), not on a live observation — **Q3** attempted to confirm it directly but the test environment had no SR-IOV hardware; see `research.md`.
- **Fallback matching is not removed.** It is retained unchanged as the matching path for interfaces that carry no unit number, and is never consulted for an interface that does carry one. After this feature the un-numbered state is **transient in every case**: a VM awaiting schema upgrade is numbered by the backfill, and an interface the backfill had to skip (**G7**) is numbered by the mutator on the very next admission request. Fallback matching therefore covers a narrow window rather than a standing population — but the window is real, so the path stays and stays tested.
- **The immutable-networks positional zip is not replaced.** It remains the mapping of last resort for the un-numbered interfaces described above.
- **No device `type` convergence.** When a device's type disagrees with `spec.network.interfaces[i].type`, the device is left alone — type is excluded from **G12**'s comparison, and an unset or mismatched `type` means "no preference", so a class ConfigSpec device (E1000, SR-IOV) is not churned toward the reconciler's default. **Type support ships in a later change set.** Note the corollary: because the desired device is built without consulting `type`, a replacement under **G12** materializes the default type, which matches that field's documented contract ("if omitted, VMXNet3 will be used for new network interfaces"). Pinning a non-default type across a replacement becomes possible when the deferred type work honours `type` in the desired-device builder; until then it is not available (see the rollout ordering constraint in `plan.md`).
- **No in-place edit to converge a numbered interface's device.** Even for changes an edit could express — a backing change from a network migration — this change set replaces the device (**G12**). Two consequences are accepted for now: a change an edit could have absorbed losslessly instead yields a new device key and, for a generated MAC, a new MAC and DHCP lease; and **a numbered interface no longer benefits from the existing orphaned-CR edit optimization**, so a migration that today can be absorbed by editing the backing in place will replace the device instead. This is the primary motivation for the deferred edit follow-on and is called out in the release note and docs.
- **No `unitNumber` relocation.** There is no "slot move": changing an existing interface's `unitNumber` always nets out to removing the old device (if unclaimed) and adding a new one — NIC replacement, never in-place relocation of the same device.
- **No powered-on NIC hardware changes.** Device changes are computed only in the powered-off reconcile path, matching today's behaviour for every other NIC device change. A new interface, or a changed `unitNumber` on an existing one, may be *admitted* while the VM is powered on, but neither the resulting add nor the resulting remove is applied until the VM is next powered off and reconciled. **Reflecting these changes on a powered-on VM is a committed follow-on**, tracked as its own spec.
- **No support for disabling the feature after rollout.** Once the backfill has populated values, spec updates that change or add a value fail while the flag is off (**G10**), and no new values are recorded. Rolling the feature off after rollout is not an expected operational flow.

---

## User stories / acceptance criteria

### DevOps user

1. **Given** a new VM whose interfaces set no `unitNumber`, **When** the VM is created and first reconciled, **Then** the spec is unchanged at admission — a VM being created is not yet upgraded, so nothing is assigned — vSphere assigns the slots when the devices are created, and the backfill records the observed values into `spec.network.interfaces[i].unitNumber`. *(G4, G6)*
2. **Given** a new VM where one interface sets `unitNumber: 9`, **When** the VM is created, **Then** that NIC is created at slot 9, the remaining interfaces receive vSphere-assigned slots, and the backfill records those into the spec. *(G5; gated on Q1)*
3. **Given** two interfaces with the same `unitNumber`, or a value outside the valid range, **When** the VM is submitted, **Then** admission is rejected with a clear, field-specific error. *(G9)*
4. **Given** a powered-on VM, **When** a user changes an interface's already-set `unitNumber`, clears it, or sets one on an interface that has none, **Then** admission is rejected. *(G9)*
5. **Given** a VM after reconcile, **When** the user inspects `status.network.interfaces`, **Then** each Tools-reported entry carries the `unitNumber` observed in vSphere; **and** with the feature flag off, no entry carries the field at all. *(G2)*
6. **Given** a powered-off VM whose interface is renumbered to an unoccupied slot, **When** the reconciler runs, **Then** the interface's previous device is removed (if no other interface claims it), a new device is added at the new slot with a new key — and, for a generated MAC, a new MAC — and spec and status reflect the new device. **This is a NIC replacement, not a slot move**: expect the transient interruption and DHCP/IP churn that come with replacing a NIC. *(G11, G12)*
7. **Given** a powered-off VM whose interface keeps its `unitNumber` but is re-pointed at a different network, **When** the reconciler runs, **Then** the device at that slot is replaced — removed and re-added at the same slot in one reconfigure — and afterwards exactly one device occupies the slot, with a new key. *(G12; interim behaviour, see Non-goals)*
8. **Given** a powered-off VM where two interfaces exchange `unitNumber` values in one update and differ in backing, MAC, or external ID, **When** the reconciler runs, **Then** each interface resolves against the device now at its own declared slot, finds a mismatch, and replaces it — so each slot ends up holding a device carrying **that interface's own** backing, MAC, and external ID, with neither inheriting the other's identity. *(G11, G12)*
9. **Given** a VM with a `unitNumber` set, **When** an UPDATE is submitted through an older API version that omits the field, **Then** the value survives. *(G3)*

### Platform engineer

10. **Given** a brownfield VM with no `unitNumber` on any interface, **When** VM Operator is upgraded and the VM is next reconciled with the flag enabled, **Then** the schema upgrade records each interface's observed slot into the spec, matching by MAC / external ID / backing with a positional zip for otherwise-unmatched devices. *(G6)*
11. **Given** a VM whose backfill has already run, **When** a NIC is later added without an explicit `unitNumber`, **Then** admission assigns it the next free slot — the backfill's one-shot nature does not leave it unnumbered — and it is matched by unit number like every other interface. *(G4, G11)*
12. **Given** a VM with backfilled values and the flag enabled, **When** interfaces are reconciled against vSphere, **Then** each numbered interface resolves by exact slot, and a steady-state reconcile emits no device changes — no spurious remove/add cycles. *(G11, G12)*
13. **Given** a VM where a NIC is removed from the spec, **When** the reconciler runs, **Then** the device identified by that interface's unit number is the one removed, and the surviving NICs keep their device keys and unit numbers in both spec and status. *(G11)*
14. **Given** a VM that has not yet completed its schema upgrade, **When** a user submits any change to `spec.network.interfaces`, **Then** the request is rejected as not-yet-upgraded; **and When** VM Operator submits the backfill's own write, **Then** it is accepted. *(G16)*
15. **Given** a VM where the backfill cannot uniquely match an interface, **When** the schema upgrade runs, **Then** a `Warning` Event on the VM records that the value came from a positional zip; **and Given** any numbered interface whose declared slot holds no device or holds one that disagrees with its desired state, **When** the VM reconciles, **Then** a hardware-configuration condition reports the divergence and clears once the VM converges. *(G8)*
16. **Given** a VM whose observed slot is already claimed by another interface's explicit value (or is otherwise outside the valid range), **When** the backfill runs, **Then** no value is written for that interface, the VM's spec remains admissible, and subsequent reconciles — including the operator's own patches — continue to succeed. **And** because a nil value on an upgraded VM is indistinguishable from a newly-added interface, the mutating webhook assigns that interface a free slot on the backfill's own patch — matching the disk behaviour — so the interface may subsequently have its device replaced; this is accepted, and the condition from AC 15 is what surfaces it. *(G7)*
17. **Given** a powered-on VM whose interface `unitNumber` has been changed (admitted but not yet applied), **When** the VM reconciles, **Then** reconciliation continues to succeed, the interface's status entry keeps its name, and network boot order is not silently retargeted at a different NIC. *(G13)*

---

## Decisions

Design decisions that are settled; each was an open question at some point in this spec's life.

- **NIC unit numbers are 7–16, by static allocation.** The platform assigns PCI units per device class, with ethernet cards owning units 7–16 and the lower units held by the video card, audio, and storage HBAs. This is a documented platform contract rather than an observed convention, so the CRD range markers can be fixed on it directly; the research program confirmed the 7–16 band in practice for ordinary ethernet cards (T001 E01–E15) but could not exercise the SR-IOV or non-NIC-PCI-occupant corners of it (Q3, and the E10 experiment) for lack of matching hardware in the test environment.
- **The mutation webhook assigns unit numbers, following the disk pattern.** The `IsObjectUpgraded` gate — not a NIC-specific workaround, but the mechanism the disk, CD-ROM and controller webhooks already use for exactly this hazard — is what keeps the webhook from guessing a slot ahead of the backfill. Following the pattern also means every interface carries a value from creation onward, removing the class of permanently-unnumbered interfaces a no-assignment design would leave behind on every post-backfill NIC addition.

- **The shared next-available-slot helper needs a starting-offset concept.** Disk controllers number from 0 and reserve at most one unit; NICs occupy 7–16. The helper should gain a first-usable-unit notion (0 for existing controllers, 7 for the NIC bus) rather than growing a NIC-specific copy — one helper, one set of semantics. Expressing the offset through the existing single reserved-unit field cannot work, since it would have to reserve seven distinct units.
- **`unitNumber` is a device identity, not a relocatable slot label** (**G11**). This is the single decision the rest of the design follows from. There is no "convergence via edit": a spec value with no matching device is resolved by the existing add-device and remove-unmatched-device mechanisms, and no new reconcile mechanism is introduced.

- **The backfill and webhook mechanics copy disks exactly; the reconcile semantics deliberately do not.** Disks and CD-ROMs skip the placement write on a spec/observed disagreement and leave the fields nil, and the mutator later assigns a slot to whatever was left nil. NICs do the same (**G7**). What disks *also* have is a safety net NICs give up: a volume's identity is its PVC name / disk UUID, so a wrong unit number is metadata that never moves hardware, and the divergence is reported through `VirtualMachineHardwareDeviceConfigVerified` rather than converged. **G11** makes the NIC unit number the identity itself, so the same wrong value causes a NIC replacement. Both alternatives were considered and rejected: making the unit number placement-only (fully disk-like) would abandon the stable-identity-across-migration motivation the feature exists for, and reporting-instead-of-converging at **G12** would leave a genuine network re-point permanently unconverged. The divergence from the disk model is accepted, and **G8**'s condition is the disk-style signal retained to make it visible.
- **A set value stays mutable while powered off.** Making it fully immutable was considered and rejected. The operator accepts that a change is a NIC replacement (new key, possibly new MAC and IP), and documents it as such rather than restricting the field further.
- **Removing a device at a slot and adding a device at that same slot within one reconfigure is supported platform behaviour**, and is what lets **G12**'s replacement resolve in a single reconcile with no intermediate state. **Confirmed** on vCenter 9.2.0 build 25689988 / ESX 9.2.0 build 25690016 (`research.md` E06) — the simulator is not authoritative for it, since its duplicate-unit-number check is scoped to devices sharing a controller key and device type. The same run also surfaced a build-specific quirk worth carrying into the implementation: on this build a same-slot Remove+Add reproduces the *same* device `Key` (Key is derived from `UnitNumber`, not creation order), so `Key` equality must not be read as "no replacement happened" — MAC address is the reliable signal instead.
- **The unit number is `VirtualDevice.UnitNumber`, not the PCI slot number.** They are different address spaces; the PCI slot number is out of scope entirely. Non-NIC PCI-bus occupants cannot land in the 7–16 unit range: the platform's PCI unit allocation is static per device class and reserves separate bands for them (see `research.md`), so no cross-device-class collision testing is needed for this feature.
- **Operator-built ethernet cards do not set a controller key explicitly**; the platform resolves the device to the correct PCI controller. No controller-key computation is needed.
- **The backfill runs regardless of `spec.network.disabled`.** It is driven by the spec's interface list, so a VM with no interfaces records nothing and still stamps the feature-version bit, preserving one-shot semantics. A VM whose network is disabled at backfill time and enabled later keeps nil values on any interfaces added afterwards — accepted semantics, kept safe by **G11**'s claim order and **G9**'s account scoping.
- **Powered-on NIC hardware changes are out of scope here** and are a committed follow-on rather than a research question.

---

## Open questions

**Unblocking owner:** the govmomi research program (`tasks.md` T001) for Q1–Q7; the snapshot-revert investigation (`tasks.md` T031) for Q8. Work blocked on a gating question is marked as such in `tasks.md`.

Q1, Q2, Q4–Q7 were answered by T001's research run against a real vCenter (9.2.0 build 25689988 / ESX 9.2.0 build 25690016) on 2026-08-23; full detail lives in `research.md`, "govmomi research program (T001) — results". Q3 was attempted but the test environment had no SR-IOV hardware, so it remains open pending a re-run on a testbed that has it. Q8 remains open, owned by T031.

### Gating — answered

- **Q1 — ANSWERED: HONOURED.** vSphere honours an explicit `VirtualDevice.UnitNumber` on every Add path this feature depends on — the create ConfigSpec (`folder.CreateVM`), a post-create `ReconfigVM_Task` Add on a powered-off VM, and a powered-on hot-add. Confirmed on vCenter 9.2.0 build 25689988 / ESX 9.2.0 build 25690016; see `research.md` E01, E03, E15. **This releases the T001 gate on T008–T010, T017, T018, T029, and T022 scenarios 2/3/5** — implementation proceeds on the identity-model design as written; the "If Q1 fails" fallback in `plan.md` was not taken.
- **Q2 — ANSWERED: HONOURED, plus an interaction finding.** The OVF content-library deploy path honours explicit unit numbers identically to `folder.CreateVM`. Confirmed by two runs (`research.md` E02): when `deploymentSpec.VmConfigSpec` carries ethernet-card Add entries, **the OVF descriptor's own NIC declaration is suppressed outright, not merged with the ConfigSpec's** — a discriminator rerun with explicit units excluding the descriptor's own auto-assigned slot produced exactly the ConfigSpec's NICs and none from the descriptor. No per-path collision guard is needed in T029's OVF branch.
- **Q3 — NOT YET ANSWERED (environment gap, not a design gap); corroborated but not closed.** T001's SR-IOV experiment (E14) was run and recorded an explicit skip: this test environment (nested/nimbus ESXi) has no SR-IOV-capable physical NIC or SR-IOV-enabled network — confirmed directly via each host's `HostPciPassthruSystem` (no PCI device anywhere reports passthrough- or SR-IOV-capable), and a follow-up attempt to fake SR-IOV hardware the same way `test/e2e/vmservice/vmservice/util.go`'s `EnsureVGPUConfiguration` fakes a vGPU (installing the matching `test-vmx.vib`) confirmed that vib mocks vGPU only, not SR-IOV. Separately, `EnvironmentBrowser.QueryConfigOption` confirms `VirtualSriovEthernetCardOption` is a declared ethernet-card device option at this environment's hardware version — live corroboration that the device class exists as an ethernet card, though not of the specific 7–16 sub-band claim (its `ControllerType` doesn't discriminate that — `VirtualPCIPassthroughOption`, which does not share the 7–16 band, reports the same `ControllerType`) and not a substitute for a real Add. The claim that SR-IOV cards share the 7–16 unit-number space still primarily rests on the documented platform contract (`external/vim/api/v1alpha1/testdata/device_keys.txt` — `VirtualSriovEthernetCard` is a `VirtualEthernetCard` and draws from the same 4000-series device keys). Re-run E14 on a testbed with real SR-IOV hardware before treating this as fully closed; nothing in the implementation is blocked on it in the meantime, since the Non-goals claim was always sourced from the platform contract rather than from this experiment. See `research.md`'s Q3 answer for the full write-up.
- **Q4 — ANSWERED: `InvalidDeviceSpec`, `Property: "unitNumber"`, `DeviceIndex` populated.** Confirmed identically for a colliding `CreateVM` and a colliding `ReconfigVM_Task` Add (`research.md` E08). Matches vcsim's simulated fault type (confirms I26's disposition). T018/T029 should treat this specific fault (type + property) as the permanent-error signal (`pkgerr.NoRequeueError`), not retryable.

### To characterize — answered, not blocking

- **Q5 — ANSWERED: reused.** vSphere's auto-assignment reuses the lowest freed unit rather than continuing past the previous high-water mark (`research.md` E11).
- **Q6 — ANSWERED: stable.** Unit numbers were unchanged across a full power-off → on → off cycle (`research.md` E12).
- **Q7 — ANSWERED: existing NICs unshifted; the new NIC takes the next free slot.** An out-of-band add — via the "out-of-band API call" form the question itself allows — left pre-existing NICs' unit numbers unchanged (`research.md` E13). No design change follows (I11).
- **Q8** — [NEEDS CLARIFICATION] Snapshot revert interplay: the snapshot's backup data restores the VM spec and annotations, so unit numbers and the feature-version bit travel with the snapshot, while the imported-VM fallback synthesizes interfaces without unit numbers. Reverting to a pre-backfill snapshot drops the bit and re-triggers the backfill — confirm it converges. Owned by T031, not T001.
