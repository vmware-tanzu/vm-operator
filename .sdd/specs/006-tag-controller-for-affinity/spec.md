# Feature Specification: Tag CRD + Tag Controller for Affinity

- **Feature branch**: [`aniketd/tag-controller-for-affinity`](https://github.com/aniket-deole/vm-operator/tree/aniketd/tag-controller-for-affinity)
  - **Fork**: `aniket-deole/vm-operator`
  - **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-08-04
- **Status**: Draft
- **Epic**: vmop-3882

---

## Summary

The Kubernetes platform lets users schedule workloads with affinity or anti-affinity to other workloads using label selectors, including affinity against all current _and_ future workloads matching a set of criteria. VM Service delivered the equivalent for VM-based workloads in VCF 9.1, but only for members of a `VirtualMachineGroup`, and only for labels referenced by the VM's own `spec.affinity` at create time — a VM carrying a participating label without declaring affinity of its own was never tagged, so it could not be a target of anyone else's affinity rule.

VMs participate in affinity/anti-affinity placement rules by carrying Kubernetes labels and by referencing those labels from `spec.affinity`. When a workload is reconciled:

- Labels participating in placement are converted to vCenter Tags.
- `spec.affinity` policies are converted into vCenter Compute Policies.
- vCenter manages the lifecycle of the Tags and Compute Policies used for placement.

This feature:

- Introduces a Supervisor `Tag` API that records, per namespace, which label key/value pairs currently participate in an affinity relationship and which VMs own that participation.
- Uses that record to tag **every** VM in the namespace carrying a participating label — including VMs that carry the label but declare no `spec.affinity` of their own — so that both initial and execution-time placement can be satisfied.

The `Tag` API is **invisible to DevOps users**: they continue to express intent solely through VM labels and `spec.affinity`, and are not permitted to create, modify, or delete `Tag` resources. The `Tag` resource is an internal bookkeeping surface for VM Operator, visible to a CSP admin or platform engineer for diagnosis.

---

## Goals

- **G1**: A VM whose `spec.affinity` references one of its own labels **MUST** have the corresponding vCenter Tag applied, and a `Tag` resource for that label key/value **MUST** exist in the VM's namespace with that VM recorded as an owner.
- **G2**: Every VM in the namespace carrying a label that some VM's `spec.affinity` references **MUST** be tagged with the corresponding vCenter Tag, regardless of what that VM's own `spec.affinity` says (including declaring none at all), and regardless of the order in which the VMs were created. See "Ownership vs. tag carriage" below.
- **G3**: A label that no VM's `spec.affinity` references **MUST NOT** produce a `Tag` resource and **MUST NOT** produce a vCenter Tag on any VM.
- **G4**: A `Tag` resource **MUST** persist for exactly as long as at least one VM owns it — that is, references it from its own `spec.affinity`, whether or not that VM carries the label — and **MUST** be removed once ownership reaches zero. Because `spec.affinity` is immutable, ownership reaches zero only when every owning VM has been deleted; relabeling a live VM never changes its ownership.
- **G5**: When a `Tag` resource is removed, the vCenter Tag **MUST** be removed from every VM in that namespace that carried it, including label-only VMs.
- **G6**: Tag participation **MUST** be scoped to a single namespace. The same label key/value in two namespaces **MUST** produce two distinct `Tag` resources, and a VM **MUST NOT** be tagged on account of a `Tag` resource in another namespace.
- **G7**: A change to a VM's label participation — a label it carries being added or removed while `spec.affinity` references it — **MUST** converge both that VM's own tags and the tags of every other VM in the namespace affected by the change. Convergence **MUST** be re-evaluated on every reconcile, not only at create time. This convergence only checks "did the VM's labels change?" — it does not check vCenter to see if the tag is still actually attached. If someone manually removes a tag directly in vCenter, VM Operator won't notice or re-add it until that VM's labels change again (see the "vCenter Tag detached out of band" edge case).
- **G8**: Tag application and removal **MUST** be independent of `VirtualMachineGroup` membership.
- **G9**: DevOps users **MUST NOT** be able to create, update, or delete a `Tag` resource; only privileged accounts may. `spec.key` and `spec.value` **MUST** be immutable after create.
- **G10**: A CSP admin listing `Tag` resources **MUST** be able to see, from the default `kubectl get` output, which label key and value each `Tag` represents.
- **G11**: The entire behavior **MUST** be gated behind a feature flag, and with the flag off, tagging behavior **MUST** be exactly what it is today.
- **G12**: A `Tag` resource that has been deleted and is concurrently needed again **MUST** be re-created rather than silently skipped, and two VMs needing the same `Tag` at the same time **MUST** converge on a single resource carrying both owners. Either request **MUST** be retried until it succeeds rather than dropped. Deletion is atomic — a `Tag` carries no finalizer — so there is no "pending deletion" state for a reconcile to observe: the resource either exists or does not.

## Non-goals

- **NG1**: No vCenter-side work is performed on behalf of a `Tag` resource. The `Tag` resource is bookkeeping only; the vCenter Tag itself is created by `vpxd` when named in a VM's reconfigure request, or is assumed to pre-exist. The `Tag` resource's reserved identity field for a resolved vCenter Tag UUID is **not** populated by this feature.
- **NG2**: Multi-vCenter. A `Tag` records a target vCenter identifier for forward-compatibility only; a single vCenter is assumed.
- **NG3**: Removing the `VirtualMachineGroup` dependency from affinity as a whole. This feature is the first incremental step; the group-based path continues to exist.
- **NG4**: Retiring the pre-existing create-time label-to-tag mechanism. That mechanism continues to serve the flag-off path in this feature; its removal is deferred to a follow-up spec.
- **NG5**: Compute Policy generation. The vCenter Compute Policies that consume these tags are generated by the existing affinity path and are unchanged by this feature.
- **NG6**: Tying the feature flag to a Supervisor capability. The flag is introduced standalone and is enabled by default on the development branch, with an FSS environment variable as its off switch; capability wiring is deferred.
- **NG7**: Exposing the `Tag` API to DevOps users, or documenting it as a user-facing API.
- **NG8**: Support for label selector operators beyond what the existing affinity path supports (`matchLabels`, and `matchExpressions` with the `In` operator).
- **NG9**: Making `spec.affinity` mutable. It remains immutable after create, exactly as it is today. The `Tag`-driven path converges on every reconcile, so it *would* handle an affinity change correctly for a label the VM already carried at placement time — but a change that references a label pair the VM has never carried has no way to make vCenter provision the tag (see `research.md` D7), so admitting affinity updates at all is deferred to a follow-up. Label participation is mutable and is what US3 covers.

---

## Key entities

### Tag

A namespace-scoped resource representing a single vCenter Tag, corresponding to exactly one label **key + value** pair (e.g. `app=nginx`). A distinct key+value pair yields a distinct `Tag`. Its lifecycle is driven by how many VMs currently participate in the associated label/affinity relationship.

vCenter Tags are identified by name **and** category, and are unique across that combination. Every vCenter Tag backing this feature uses the **namespace name** as its Tag Category, derived from the `Tag`'s own namespace — the category is not a separate user-settable field.

Attributes:

- The label **key** and label **value** the `Tag` represents, both required, and both also mirrored into the `Tag`'s own metadata labels, so an admin can select `Tag`s by label. (The control plane answers "does a `Tag` already exist for this key/value?" from the resource's derived name, not from that mirror — see `model.md` "Query surface".)
- The target vCenter identifier, for forward-compatibility (see NG2).
- A reserved identity field for the resolved vCenter Tag UUID, left empty by this feature (see NG1).
- The set of owning VMs, recorded as owner references. An owner is a VM whose own `spec.affinity` references the label pair — regardless of whether the VM itself currently carries the label. A VM can own a `Tag` for a pair it never carries: this is what lets it establish a `VmToVmGroupsAntiAffinity` relationship against other VMs that carry the pair without carrying it itself. Because `spec.affinity` is immutable after create, a VM's ownership — once established on its first reconcile — lasts for that VM's entire lifetime; it is not affected by later relabeling. A VM that merely carries the label without referencing it is **not** an owner, even though it does get the vCenter Tag.
- The type of tag: currently limited to "System". The `type` field is added for future modularity and usage of the Tag CR by other features.

### VirtualMachine

The workload. The attributes relevant here are its labels and its `spec.affinity` rules. A VM becomes an owner of zero or more `Tag` resources and carries the corresponding vCenter Tags.

### vCenter Tag

The vCenter-side object (name + category) attached to a VM and consumed by Compute Policies for placement. It is created by `vpxd` when the VM's reconfigure request names it, or is assumed to pre-exist. It is not lifecycle-managed by VM Operator (NG1).

### Compute Policy

The vCenter mechanism that enforces affinity/anti-affinity using vCenter Tags. It consumes the tags this feature applies and is otherwise unchanged (NG5).

### Ownership vs. tag carriage

Two distinct rules govern this feature, and conflating them is the single easiest way to misread it:

1. **Ownership** — which VMs own a `Tag`, and therefore whether it exists at all:

   > A VM owns the `Tag` for a label pair **iff** its own `spec.affinity` references it — whether or not the VM itself carries the label.

2. **Tag carriage** — which VMs carry the vCenter Tag:

   > A VM carries the vCenter Tag for a label pair **iff** it carries that label **and** a `Tag` for that pair exists in its namespace.

Neither rule makes ownership a precondition of carriage or vice versa. Consequences worth stating explicitly, because each one is a scenario below:

- A VM carrying the label with no `spec.affinity` at all is tagged, but is not an owner (US2).
- A VM that carries the label without owning the `Tag` **keeps the vCenter Tag** for as long as some other VM still owns it. Whether it never referenced the pair or once did is not distinguishable and does not matter, which is exactly right: `spec.affinity` expresses which relationships a VM *asks for*, while its labels express which relationships it *is available for*.
- A VM's own vCenter tag reacts to its labels: it loses the tag when it drops the label (US3 scenario 1), or when the `Tag` itself is deleted because every owning VM has been deleted (US1 scenarios 5-6).
- A VM referencing a pair it does not carry still produces (and owns) a `Tag` — this is what lets a VM establish a `VmToVmGroupsAntiAffinity` relationship against other VMs that carry the pair without carrying it itself (US1 scenario 4). The referencing VM does not itself get the vCenter tag unless it separately carries the pair.
- Because `spec.affinity` is immutable, ownership — once established — is not reversible by relabeling. The only way a VM stops owning a `Tag` is by being deleted itself (US3).

### Namespace isolation

Interaction between `Tag` resources and workloads is confined to a single namespace. Workloads and `Tag` resources never reference objects across namespaces. Because the vCenter Tag Category is the namespace name, the same label value in two namespaces yields two distinct vCenter Tags and two distinct `Tag` resources (G6).

---

## Relationship to the existing tagging mechanisms

Two tag-configuring mechanisms exist today:

1. A UUID-based path driven by the vSphere Policy reconciler, which consumes tag UUIDs from evaluated policies.
2. A create-time path that converts a VM's own affinity-referenced labels directly into tag specifications on the VM's reconfigure request.

This feature **supersedes** mechanism (2) functionally and **coexists** with mechanism (1). Concretely: when the feature flag is on, the `Tag`-driven path is what decides which vCenter Tags a VM carries; when the flag is off, mechanism (2) behaves exactly as it does today (G11, NG4).

Mechanism (2) is limited to labels referenced by the VM's *own* `spec.affinity`, and only at create time. The `Tag`-driven path removes both limits: participation is namespace-wide (G2) and is re-evaluated on every reconcile rather than only at create time (G7).

---

## User scenarios & testing *(mandatory)*

### User Story 1 — Label referenced by a new VM's affinity becomes a vCenter Tag (Priority: P0)

A DevOps user creates a VM that carries a label and references that label in `spec.affinity`. A matching vCenter Tag is applied to the VM and a `Tag` resource exists in the VM's namespace, with the VM recorded as an owner.

**Why this priority**: This is the core capability — without translating affinity-referenced labels into vCenter Tags, no placement policy can be enforced. It delivers a demonstrable MVP on its own.

**Independent test**: Create a single VM with label `LBL-1` and `spec.affinity` matching `LBL-1`, reconcile it, and verify the vCenter Tag is applied, the `Tag` resource is created in the namespace, and the VM is listed as an owner.

**Acceptance scenarios**:

1. **Given** VM-A with label `LBL-1` and `spec.affinity` matching `LBL-1`, **When** VM-A is reconciled, **Then** vCenter Tag `LBL-1` is applied to VM-A, a `Tag` resource for `LBL-1` is created in VM-A's namespace, and VM-A is added as an owner of that `Tag`.
2. **Given** VM-A with label `LBL-1` and `spec.affinity` matching `LBL-1` and an existing `Tag` for `LBL-1` owned by VM-A, **When** VM-B with label `LBL-1` and `spec.affinity` matching `LBL-1` is created and reconciled, **Then** vCenter Tag `LBL-1` is applied to VM-B, VM-B is added as an owner of the `Tag`, and the `Tag` has two owners.
3. **Given** VM-A with labels `LBL-1` and `LBL-2` and `spec.affinity` matching only `LBL-2`, **When** VM-A is reconciled on create, **Then** vCenter Tag `LBL-2` (only) is applied to VM-A, a `Tag` for `LBL-2` is created in the namespace, and it records VM-A as an owner. No `Tag` is created for `LBL-1`.
4. **Given** VM-A carries label `LBL-1` (not `LBL-2`) and its `spec.affinity` — a `VmToVmGroupsAntiAffinity` term naming the target group's label — references `LBL-2`, **When** VM-A is reconciled, **Then** a `Tag` for `LBL-2` is created in VM-A's namespace with VM-A recorded as its owner, and VM-A itself is **not** given vCenter Tag `LBL-2` because it does not carry that label. Any VM in the namespace that does carry `LBL-2` is tagged once the `Tag` exists, via the ordinary fan-out (US2).
5. **Given** VM-A and VM-B both with label `LBL-1` and `spec.affinity` matching `LBL-1`, **When** VM-A is deleted, **Then** VM-A is removed as an owner of the `Tag`, the `Tag` is **not** deleted, and VM-B keeps the vCenter Tag.
6. **Given** VM-A is the only owner of the `Tag` for `LBL-1`, **When** VM-A is deleted, **Then** the `Tag` is deleted from the namespace.

---

### User Story 2 — Existing labeled VM is tagged when a later VM references that label (Priority: P1)

A VM already exists carrying a label but with no `spec.affinity`. When another VM is later created with that same label and a `spec.affinity` that references it, the pre-existing VM is proactively reconciled so that it, too, receives the vCenter Tag and participates in the affinity relationship. Removing the referencing VM reverses the tagging on the pre-existing VM.

**Why this priority**: A VM must be able to establish affinity with VMs that already exist. Without proactive reconciliation of pre-existing labeled VMs, affinity policies would silently miss participants.

**Independent test**: Create VM-A with label `LBL-1` and no affinity; confirm it is untagged. Then create VM-B with label `LBL-1` and `spec.affinity` matching `LBL-1`; verify VM-A is proactively tagged and a `Tag` resource is created.

**Acceptance scenarios**:

1. **Given** VM-A exists with label `LBL-1` but no `spec.affinity`, **When** VM-B is created with label `LBL-1` and `spec.affinity` matching `LBL-1`, **Then** vCenter Tag `LBL-1` is applied to both VM-A and VM-B, and a `Tag` for `LBL-1` is created in the namespace with VM-B — and only VM-B — as an owner.
2. **Given** VM-A exists with label `LBL-1` and no `spec.affinity`, and VM-B exists in the same namespace with label `LBL-1` and `spec.affinity` matching `LBL-1`, **When** VM-B is deleted, **Then** vCenter Tag `LBL-1` is removed from VM-A, and the `Tag` is deleted because no VM owns it any longer.
3. **Given** VM-A exists with label `LBL-1` and no `spec.affinity`, **When** VM-B is created in a **different** namespace with label `LBL-1` and `spec.affinity` matching `LBL-1`, **Then** vCenter Tag `LBL-1` is applied to VM-B and a `Tag` is created in VM-B's namespace, and vCenter Tag `LBL-1` is **not** applied to VM-A (namespace isolation).
4. **Given** a `Tag` for `LBL-1` exists in the namespace, **When** a new VM-C carrying label `LBL-1` and no `spec.affinity` is created, **Then** vCenter Tag `LBL-1` is applied to VM-C on its first reconcile, and VM-C is **not** added as an owner of the `Tag`.

---

### User Story 3 — VM's label participation changes and tag carriage converges (Priority: P1)

A DevOps user changes the labels on an existing VM. The VM's own vCenter tag carriage converges to match its new labels; its `Tag` ownership — driven solely by its immutable `spec.affinity` — is unaffected by any label change and persists until the VM itself is deleted.

**Why this priority**: This is an enhancement over US1 and US2. `metadata.labels` is mutable where `spec.affinity` is not (NG9), so a live VM's *labels* change even though what it *owns* cannot. Without this, a relabeled VM's vCenter tags would drift from what its current labels actually justify, even though the ownership record itself stays correct by construction.

**Independent test**: Create a single VM with label `LBL-1` and `spec.affinity` matching `LBL-1` and verify the vCenter Tag, the `Tag` resource, and the ownership. Then remove the label and verify the vCenter tag is removed while the `Tag` resource and the VM's ownership are unaffected.

**Acceptance scenarios**:

1. **Given** VM-A and VM-B both with label `LBL-1` and `spec.affinity` matching `LBL-1`, **When** `LBL-1` is removed from VM-A's labels, **Then** VM-A **remains** an owner of the `Tag` (its `spec.affinity` still references `LBL-1`), vCenter Tag `LBL-1` is removed from VM-A because it no longer carries the label, and the `Tag` is **not** deleted.
2. **Given** VM-A is the sole owner of the `Tag` for `LBL-1` and drops the `LBL-1` label, **When** VM-A is reconciled again with no further label changes, **Then** the `Tag` persists indefinitely — VM-A's ownership is untouched by the label drop, and the `Tag` is removed only if VM-A itself is later deleted (US1 scenario 6), never by relabeling alone.
3. **Given** VM-A exists with label `LBL-1` and no `spec.affinity`, and VM-B exists in the same namespace with label `LBL-1` and `spec.affinity` referencing `LBL-1`, **When** `LBL-1` is removed from VM-B's labels, **Then** VM-B **remains** an owner and the `Tag` is **not** deleted, VM-B loses vCenter Tag `LBL-1` because it no longer carries the label, and VM-A is unaffected — it keeps the vCenter tag because it still carries the label and the `Tag` still exists.
4. **Given** VM-A with labels `LBL-1` and `LBL-2` and `spec.affinity` matching both — so VM-A owns `Tag`s for both pairs from its first reconcile, independent of which it carries at any given time — and only `LBL-1` currently carried by any VM, **When** VM-A gains the `LBL-2` label, **Then** vCenter Tag `LBL-2` is applied to VM-A because it now carries a pair its `Tag` already exists for; VM-A's ownership of both `Tag`s is unchanged, and its participation in `LBL-1` is unaffected.
5. **Given** VM-A carries label `LBL-1` and references both `LBL-1` and `LBL-2` from `spec.affinity`, and a second VM-B also carries and references `LBL-1`, **When** VM-A drops `LBL-1` and gains `LBL-2` in a single update, **Then** VM-A ends up carrying vCenter Tag `LBL-2` and not `LBL-1` — only its own tag carriage swaps, while VM-A remains an owner of **both** `Tag`s regardless, and VM-B keeps carrying vCenter Tag `LBL-1` because it still carries the label and the `Tag` still exists.

---

### User Story 4 — CSP admin diagnoses affinity tagging (Priority: P2)

A CSP admin inspects which label relationships are currently driving affinity in a namespace, and which VMs are responsible for each.

**Why this priority**: Diagnosability. Without it, an admin explaining why a VM carries a given vCenter Tag has to reconstruct the relationship from every VM's `spec.affinity` by hand.

**Independent test**: With two VMs participating in two different label relationships in a namespace, listing `Tag` resources shows both, each identifying its label key and value; and the owner references on each `Tag` identify the responsible VMs.

**Acceptance scenarios**:

1. **Given** `Tag` resources exist in a namespace, **When** a CSP admin lists them, **Then** the default output identifies, for each `Tag`, the label key and the label value it represents.
2. **Given** a `Tag` exists, **When** a CSP admin inspects it, **Then** its owner references name every VM whose `spec.affinity` references the label — whether or not that VM carries it — and its `Ready` condition and last-observed generation reflect the current state.
3. **Given** a DevOps user (non-privileged account), **When** they attempt to create, modify, or delete a `Tag`, **Then** the request is rejected by admission.
4. **Given** a privileged account, **When** they attempt to change a `Tag`'s label key or value after create, **Then** the request is rejected as immutable.

---

## Edge cases

- **Tag deletion racing a re-create**: Because a `Tag` carries no finalizer, its deletion is atomic — a reconcile either finds the resource or finds nothing, never a copy marked for deletion. Two races survive that, and both MUST converge (G12): (a) a VM reconciled just after the `Tag` was deleted MUST create a fresh one, rather than proceeding as though it existed or silently skipping tagging; (b) two VMs that need the same pair concurrently MUST end up sharing one `Tag` with both recorded as owners — the VM that loses the create race adopts the winner's resource instead of failing its reconcile. A stale read that adopts a `Tag` already deleted on the server fails its ownership write and is retried on the next reconcile, which is the general case of (a).
- **Ownership dropping to zero while the owning VM still (briefly) exists**: A VM being deleted has its owner reference explicitly released *before* its finalizer is removed — while the VM object still exists with a `deletionTimestamp`, not yet actually gone. If that VM was the last owner, the `Tag` must still be removed (G4, US1 scenario 6). This case is not covered by ordinary owner-reference garbage collection, which acts only once the owner object is actually deleted; it requires an explicit delete. Relabeling never produces this case: ownership does not react to labels at all.
- **Concurrent owners writing the same `Tag`**: Several VMs in a namespace add and remove their own owner reference on the same `Tag`. The owner-reference list must not lose a concurrent writer's entry.
- **Ownership, once established, is permanent for the VM's lifetime**: because `spec.affinity` is immutable, the set of pairs a VM owns is fixed on its first reconcile and cannot shrink by relabeling. A VM that drops a label it once carried keeps owning that pair's `Tag` — only its own vCenter tag carriage changes. The `Tag` itself is removed only once every owning VM has been deleted.
- **A VM carries a label but the `Tag`'s key/value match is only partial**: A `Tag` represents a key **and** a value. A VM carrying the same key with a different value is unrelated to that `Tag` and is not tagged from it.
- **Empty label value**: A label with an empty value is a legal Kubernetes label and yields a `Tag` distinct from any non-empty value for the same key.
- **A label key or value that cannot appear verbatim in a resource name**: Label keys may be prefixed (`example.com/tier`) and values may be long. The `Tag`'s resource name is therefore derived rather than composed literally, so it is always a valid resource name (see `model.md`).
- **Unsupported label selector operators**: Selector expressions the existing affinity path does not support are ignored for tag derivation exactly as they are today (NG8) — they do not fail the VM's reconcile.
- **A VM that carries a participating label and is a VKS node VM, or has an explicit zone**: Whether host-topology and zone-topology affinity terms are considered at all continues to follow the existing constraint rules; this feature does not change which terms are eligible, only what is done with the labels those terms reference.
- **Flag turned off after tags were applied**: With the flag off, the `Tag`-driven path stops running. Already-applied vCenter Tags are not proactively cleaned up, and existing `Tag` resources are left in place; the pre-existing mechanism resumes deciding tags at every point it does today — placement, create, update and resize — from the VM's own `spec.affinity`, add-only.
- **The first VM to reference a label pair is placed without its tag**: Placement happens before the VM exists, so it can only be told about tags whose `Tag` resource already exists. The VM that *establishes* a relationship therefore has no tag at placement time — the resource is created on its own first reconcile, and the tag is applied then. A VM joining an already-established relationship is placed with the tag. Execution-time affinity is unaffected in both cases; only the initial host choice for the very first participant is made without the relationship in view.
- **A vCenter Tag detached out of band**: VM Operator tracks the tags it has applied to a VM by recording them on the VM itself, not by reading back the tags vCenter reports as attached — the attached list is not resolvable to the tag names this feature uses (see `research.md`). If an administrator detaches one of these tags directly in vCenter, VM Operator does **not** re-attach it; the VM's tags converge again the next time the set of participating labels for that VM actually changes. This is accepted: the tags are VM-Operator-managed and detaching one by hand is outside the supported workflow.
- **A removal emitted for a tag that is already gone**: For the same reason, a remove may be emitted for a tag that is no longer attached. This is harmless — the reconfigure ignores it — and is the same behavior the pre-existing UUID-based path has today.

---

## Success criteria *(mandatory)*

### Measurable outcomes

- **SC-001**: A VM created with a label referenced by its `spec.affinity` has the corresponding vCenter Tag applied and a `Tag` resource present with the VM as owner, with no DevOps user action beyond setting the label and the affinity.
- **SC-002**: VMs sharing an affinity relationship in a namespace carry the matching vCenter Tag regardless of creation order — a pre-existing labeled VM is tagged once a later VM references the label, and is untagged once the last referencing VM is deleted (referencing, once established, is permanent for that VM's lifetime — see "Ownership vs. tag carriage").
- **SC-003**: A `Tag` resource persists for exactly as long as at least one VM owns it and is removed once every owning VM has been deleted — verifiable across VM-create / VM-delete sequences with no orphaned `Tag` resources and no orphaned vCenter Tags. Ownership itself is unaffected by label-add / label-remove sequences on a live VM; those converge only the VM's own vCenter tag carriage (SC-005).
- **SC-004**: The tagging mechanism applies vCenter Tags to participating VMs based solely on their labels and `spec.affinity`, independent of any `VirtualMachineGroup` membership. A consequence is that VMs in different `VirtualMachineGroup`s sharing the same affinity label share affinity via the same vCenter Tag — more efficiently and more broadly (including label-only VMs) than the pre-existing create-time behavior. This is intentional, and is the first incremental step toward removing the `VirtualMachineGroup` dependency.
- **SC-005**: A VM's labels can be changed on an existing, running VM, and its resulting vCenter tag carriage converges without a VM restart or re-create; the `Tag` resource and the VM's ownership of it are unaffected by the label change.
- **SC-006**: With the feature flag off, tagging behaves exactly as it does today, verifiable by the pre-existing test suites passing unchanged.

## Open questions

None outstanding. Decisions previously open and now settled — the API's group and version, the division of labor between the VM reconcile path and the `Tag` controller, the feature-flag strategy, the fate of the pre-existing create-time mechanism, resource-name derivation, the vCenter tag name format, leaving `spec.affinity` immutable (NG9), admission validation, the split between ownership and tag carriage, and how each `Tag` query is served — are recorded in [`research.md`](./research.md) "Decision log".

## Review & acceptance checklist

- [x] All user stories have at least two Given/When/Then scenarios.
- [x] Each scenario is independently testable.
- [x] Out-of-scope items are listed (see "Non-goals").
- [x] Namespace isolation is specified.
- [x] `Tag` lifecycle at zero owners is specified for both the VM-deleted and the label-dropped paths.
- [x] Ownership and tag carriage are specified as separate rules, and every scenario is consistent with both.
- [x] Feature-flag-off behavior is specified.
- [x] DevOps-user invisibility (admission) is specified.
