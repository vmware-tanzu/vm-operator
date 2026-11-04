# Data Model: Tag CRD + Tag Controller for Affinity

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Epic**: vmop-3882

This document is the API contract for the `Tag` resource introduced by this feature: its schema, its derived metadata, its lifecycle invariants, its admission rules, and the RBAC it needs.

---

## Group, version, kind

| | |
|---|---|
| **Group** | `vsphere.policy.vmware.com` |
| **Version** | `v1alpha1` |
| **Kind** | `Tag` (list kind `TagList`) |
| **Scope** | Namespaced |
| **Go module** | `github.com/vmware-tanzu/vm-operator/external/vsphere-policy` |
| **Go package** | `external/vsphere-policy/api/v1alpha1`, alias `vspherepolv1` |
| **Source file** | `external/vsphere-policy/api/v1alpha1/tag_types.go` |
| **Status subresource** | yes |
| **Storage version** | yes (only version) |
| **Generated CRD** | `config/crd/external-crds/vsphere.policy.vmware.com_tags.yaml` (via `make generate-external-manifests`) |
| **Conversion** | none — single version, no conversion webhook (see "Conversion strategy") |

`Tag` and `TagList` append themselves to the package's `objectTypes` slice from an `init()`, exactly as `TagPolicy` does, so `AddToScheme` picks them up with no change to `groupversion_info.go`.

---

## Example

```yaml
apiVersion: vsphere.policy.vmware.com/v1alpha1
kind: Tag
metadata:
  namespace: my-namespace-1
  name: tag-41dd6b09ef06426a     # "tag-" + XXHash64Hex("my-namespace-1:app:nginx")
  labels:
    app: nginx                    # mirror of spec.key/spec.value for selector queries (SourceLabel membership only)
  ownerReferences:                # owning VMs: reference the label from spec.affinity (carriage not required)
    - apiVersion: vmoperator.vmware.com/v1alpha6
      kind: VirtualMachine
      name: vm-a
      uid: 8a7f2c31-0e14-4a37-9c66-1b1f0d3a55e2
spec:
  name: app:nginx                 # the exact, prefix-free backend tag name
  kind: Original                  # the vc tag's role
  membership: SourceLabel         # VMs carrying the source label carry the tag
  key: app                        # the source label KEY
  value: nginx                    # the source label VALUE
status:
  id: ""                          # reserved for the resolved vCenter tag UUID; NOT populated by this feature
  observedGeneration: 1
  conditions:
    - type: Ready
      status: "True"
      reason: Ready
      lastTransitionTime: "2026-08-04T10:11:12Z"
```

---

## Schema

### `TagSpec`

`TagSpec` is the complete, immutable identity of one concrete backend tag, plus the rule that decides which VMs carry it. The whole of `spec` is immutable after create (a type-level CEL transition rule, `self == oldSelf`). A second CEL rule requires `key` for every kind except `GroupedAntiAffinitySelf` and forbids `key` and `value` for it.

| Field | JSON | Type | Marker | Description |
|-------|------|------|--------|-------------|
| `Name` | `name` | `string` | `+required`, `MinLength=1` | The exact backend tag name, **without** the `vmoperator:` prefix. The namespace supplies the vCenter tag category. |
| `Kind` | `kind` | `TagKind` | `+required`, `Enum=Original;AffinityTarget;GroupedAntiAffinityTarget;GroupedAntiAffinitySelf` | The role of the vc tag (see "Tag kinds"). |
| `Membership` | `membership` | `TagMembership` | `+required`, `Enum=SourceLabel;PolicyRequesters` | Which VMs carry the tag, independently of ownership. `SourceLabel` selects VMs carrying the source label. `PolicyRequesters` selects VMs currently requesting the policy. |
| `Key` | `key,omitempty` | `string` | `+optional` | The source label key. Absent for `GroupedAntiAffinitySelf`. |
| `Value` | `value,omitempty` | `string` | `+optional` | The source label value. Omission means the empty label value (see "Empty label value"). Absent for `GroupedAntiAffinitySelf`. |
| `Policy` | `policy,omitempty` | `*TagPolicyIdentity` | `+optional` | The canonical policy inputs of a derived tag. Absent for `Original`, required for every other kind. |
| `Type` | `type,omitempty` | `TagType` | `+optional`, `Enum=System`, `default=System` | Describes the origin/category of this Tag. Currently a single-valued enum (`System`, the default); not consumed by any controller, webhook, or provider code in this feature — reserved for forward-compatibility. |

`TagPolicyIdentity` holds the inputs of a derived tag's policy hash:

| Field | JSON | Type | Marker | Description |
|-------|------|------|--------|-------------|
| `Type` | `type` | `string` | `+required`, `Enum=VmVmAffinity;VmToVmGroupsAntiAffinity` | The backend affinity policy type. |
| `Strictness` | `strictness` | `string` | `+required`, `Enum=PreferredDuringPlacementPreferredDuringExecution;RequiredDuringPlacementPreferredDuringExecution;RequiredDuringPlacementRequiredDuringExecution` | Placement and execution enforcement. |
| `Topology` | `topology` | `string` | `+required`, `Enum=Host;VSphereZone` | The policy's scope. |
| `Targets` | `targets` | `[]TagLabelPair` | `+required`, `MinItems=1`, `+listType=atomic` | The target label pairs, sorted by `key:value` with no duplicates. Each `TagLabelPair` has a required `key` (`MinLength=1`) and a required `value` that may be empty. |

#### Tag kinds

| Kind | Membership | `key`/`value` | `policy` | `spec.name` |
|------|------------|---------------|----------|-------------|
| `Original` | `SourceLabel` | the source label | absent | `<key>:<value>` |
| `AffinityTarget` | `SourceLabel` | the affinity policy's single target label | required, type `VmVmAffinity` | the policy's hashed tag name |
| `GroupedAntiAffinityTarget` | `SourceLabel` | one target label of the policy | required, type `VmToVmGroupsAntiAffinity` | the policy's hashed self-tag name, then `-<key>:<value>` |
| `GroupedAntiAffinitySelf` | `PolicyRequesters` | absent | required, type `VmToVmGroupsAntiAffinity` | the policy's hashed tag name |

The prefix-free names are produced by the helpers in `pkg/providers/vsphere/virtualmachine/affinity_tags.go`. The `vmoperator:` prefix exists only at the vCenter boundary: `spec.name`, `spec.key`, `spec.value` and the resource name are all prefix-free.

`spec` is the identity: it determines the resource name (see "Name derivation"), so a change to any of it would make the name wrong.

### `TagStatus`

| Field | JSON | Type | Marker | Description |
|-------|------|------|--------|-------------|
| `ID` | `id,omitempty` | `string` | `+optional` | Reserved for the resolved vCenter Tag UUID. **Not populated by this feature** (spec NG1) — the `Tag` controller performs no vCenter work, and tag attachment uses name+category rather than the UUID. Remains empty; absence is tolerated by every consumer. Retained on the CRD for forward-compatibility. |
| `ObservedGeneration` | `observedGeneration,omitempty` | `int64` | `+optional` | Last reconciled `metadata.generation` (constitution requirement). |
| `Conditions` | `conditions,omitempty` | `[]metav1.Condition` | `+optional`, `+listType=map`, `+listMapKey=type` | Standard conditions. MUST include `Ready` (`vspherepolv1.ReadyConditionType`, already defined in `common_types.go`). |

### Derived metadata

| Field | Written by | Description |
|-------|-----------|-------------|
| `metadata.name` | VM reconcile path, on create | `"tag-" + XXHash64Hex("<metadata.namespace>:<spec.name>")`. See "Name derivation". |
| `metadata.namespace` | VM reconcile path, on create | The VM's namespace. Doubles as the vCenter Tag Category. |
| `metadata.labels[<spec.key>]` | VM reconcile path, on create; corrected by the `Tag` controller | Present only when `spec.membership` is `SourceLabel`. Mirror of `spec.value`, so an admin can run `kubectl get tags -l app=nginx`. This mirror is **not** what makes the controllers' queries efficient — see "Query surface" below. |
| `metadata.ownerReferences` | VM reconcile path — each VM writes **only its own entry** | One non-controller reference per owning VM (`controller: false`, `blockOwnerDeletion: false`). An owner is a VM whose own `spec.affinity` references the pair — carriage is not required. A VM that merely carries the label without referencing it is tagged in vCenter but is **not** an owner. Ownership is added when `spec.affinity` starts referencing the pair and removed when it stops, independently of labels. Ownership is not the same predicate as tag carriage; see the note below. |

`Tag` carries **no finalizer** (D10). Deletion at zero owners is a plain, atomic `Delete` — see invariant 5 below.

### Printer columns

Per D9 in [`research.md`](./research.md), no owner column:

```go
// +kubebuilder:printcolumn:name="Tag",type="string",JSONPath=".spec.name"
// +kubebuilder:printcolumn:name="Kind",type="string",JSONPath=".spec.kind"
// +kubebuilder:printcolumn:name="Key",type="string",JSONPath=".spec.key"
// +kubebuilder:printcolumn:name="Value",type="string",JSONPath=".spec.value"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
```

This satisfies spec G10 and US4 scenario 1. Owner detail stays available via `kubectl get tag <name> -o yaml` (US4 scenario 2).

### Type markers

```go
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:storageversion:true
// +kubebuilder:subresource:status
```

Matching `TagPolicy`'s marker set, plus the printer columns above.

---

## Name derivation

```
name = "tag-" + XXHash64Hex(metadata.namespace + ":" + spec.name)
```

- `XXHash64Hex` (`pkg/util/hash.go`) formats the full 64-bit `xxhash.Sum64` digest as 16 hex characters, so the name is always 20 characters, always DNS-subdomain-safe, and independent of the shape of the namespace and the backend tag name.
- The hashed string uses `":"` as the separator — including the namespace prefix, which doubles as the vCenter Tag Category — so one canonical string identifies the backend tag everywhere. `ConcreteTagResourceName` in `pkg/providers/vsphere/virtualmachine/affinity_tags.go` is the single implementation.
- For an `Original` tag, `spec.name` is `<key>:<value>`, so the name is a pure function of the namespace and the label pair.
- `kind`, `membership` and `policy` deliberately do not contribute to the name. Two different interpretations of one backend tag name therefore meet at one resource, where the identity check rejects adoption.
- The derivation is a pure function of the namespace and backend tag name, so any VM in the namespace that needs the `Tag` computes the same name and either finds the existing resource or creates it. No lookup-by-selector is needed on the write path; the label mirror exists for read-side and diagnostic queries.
- **Collision handling**: 16 hex characters is 64 bits. Within one namespace's `Tag` set, collision is not a practical concern. On create-or-adopt the reconciler MUST verify that an existing resource with the derived name has the expected complete identity (`ValidateTagIdentity` and an exact `spec` match), and MUST fail the reconcile with a wrapped error if it does not, rather than adopting a mismatched resource.

### Empty label value

`app=""` is a legal Kubernetes label. It yields an `Original` tag with `spec.name: "app:"`, resource name `tag-` + `XXHash64Hex("my-namespace-1:app:")`, no `spec.value` in the serialized object, and a `metadata.labels["app"] = ""` mirror — all legal. The tag is distinct from every non-empty value for the same key. Hence `spec.value` is optional and has no `MinLength`.

---

## vCenter-side identity

| | |
|---|---|
| **Tag name** | `"vmoperator:<spec.name>"` — the shared affinity tag-name helper adds the prefix to the prefix-free `spec.name` |
| **Tag category** | `<metadata.namespace>` |
| **Emitted as** | `vimtypes.TagSpec{ArrayUpdateSpec{Operation: add\|remove}, Id: TagId{NameId: &TagIdNameId{Tag: name, Category: category}}}` |
| **Created by** | `vpxd`, when named in the VM's reconfigure request; or assumed pre-existing (spec NG1) |
| **Applied set recorded in** | the VM's ExtraConfig, key `vmservice.placementpolicies.tags` — **not** on the `Tag` resource (D12) |

The Compute Policy side and VM attachments use the same generated tag name. The `vmoperator:` prefix is a vCenter-boundary concern only; it never appears in a `Tag` resource and does not affect the resource-name hash. For reverse lookup of an original tag, remove the shared prefix before splitting the remaining `key:value` pair on its colon.

Which of these tags a given VM currently carries is recorded on the **VM**, in its own ExtraConfig key, and nowhere on the `Tag` resource. That is deliberate and consistent with "Ownership vs. tag carriage" below: the `Tag` never enumerates the VMs that carry it. The record exists because the attached-tag list vCenter returns is a list of tag URNs that cannot be matched against name+category tags, so it is the only way to know which of this feature's tags were previously applied — the same reason and the same workflow mechanism B uses with `vmservice.policy.tags`. See `research.md` "The attached-tag list is UUIDs and is not on `moVM`" and `plan.md` "`ReconcileTagSpecs` — steps 4-5".

---

## Query surface

The resource is designed so that "does a `Tag` for this label exist?" and "give me the `Tag`s for this label" are both answerable without scanning the namespace. Three mechanisms, in order of preference:

1. **The derived name is the primary key.** `metadata.name` is a pure function of the namespace and `spec.name` (see "Name derivation"), so an exact-tag existence check is a `Get` — an O(1) point read against the cache, no list and no selector. This is the check the VM reconcile path runs once per owned pair, and it is by far the most frequent query in the feature.

   One caveat applies to every read here, including this one: the client is the manager's **cached** client, so a `Tag` created earlier in the same reconcile is not guaranteed to be readable later in that reconcile. The VM path therefore carries the objects it created forward in memory rather than re-reading them: `ReconcileTagCRs` returns the Tags it owns, and `ListLabelDerivedTags` merges them with what the cache returns.
2. **Field indexes**, registered by the `VirtualMachine` controller's `AddToManager` — every consumer is on that controller's reconcile path — against the shared manager cache and therefore available to every client built from it:

   | Index name | Indexed value | Answers |
   |------------|---------------|---------|
   | `metadata.ownerReferences.uid` | one entry per owner UID | which `Tag`s does this VM own |
   | `spec.sourceLabel` | one `"<key>:<value>"` entry for each `SourceLabel` Tag with a key | which label-derived `Tag`s match this VM's labels |

   An exact tag is still resolvable by the keyed `Get` in (1): validation V5 pins `metadata.name` to `ConcreteTagResourceName(namespace, spec.name)` on create, and the CEL immutability rule makes `spec` immutable on update, so the name can never drift from the identity it encodes. The `spec.sourceLabel` index exists because one source label backs several `Tag`s (the original tag and every derived target tag for that pair), so the carriage lookup in `ListLabelDerivedTags` lists by source pair instead of computing every possible name.

   The reverse direction — "which VMs carry this `Tag`'s label?", which the VM controller's `Tag` watch mapper asks on every `Tag` create and delete (D16) — is indexed on the **VM** side rather than recorded on the `Tag`: a multi-valued field index `metadata.labels.keyValue` on `vmopv1.VirtualMachine`. See `plan.md` "Field indexes and query patterns". Nothing about the tagged-VM set is persisted on the `Tag` itself, which is what keeps the resource free of a second source of truth that could drift from the VMs' actual labels.

3. **The `metadata.labels` mirror** for human and `kubectl` use (`kubectl get tags -l app=nginx`, `-l app`). It is deliberately **not** used by the controllers: controller-runtime's cache maintains field indexes only, so `client.MatchingLabels` is an in-memory filter applied after the cache has returned every object in the namespace. The mirror is a convenience surface, not an index.

The mirror is still worth maintaining even though nothing in the control plane queries by it: it is the only way an admin can select `Tag`s by label from `kubectl`, and it makes a `Tag`'s meaning legible in a raw `get -o yaml` without decoding the hashed name.

## Ownership vs. tag carriage

Two different predicates, both defined in `spec.md` "Ownership vs. tag carriage", restated here because the schema above touches both:

| | Predicate | Recorded in |
|---|-----------|-------------|
| **Ownership** | VM's own `spec.affinity` references the pair — carriage not required | `metadata.ownerReferences` |
| **Tag carriage** | VM carries the label **AND** a `Tag` for the pair exists in the namespace | vCenter only — nothing on the `Tag` records which VMs carry it |

Neither predicate is a subset or superset of the other in general: a VM can own a `Tag` without carrying the vCenter tag (a `VmToVmGroupsAntiAffinity` reference to a pair it does not carry), and a VM can carry the vCenter tag without being an owner (a label-only VM, US2). Nothing on the `Tag` enumerates the tagged VMs: that set is recomputed from live state on each VM's own reconcile, which is what keeps the resource small and free of a second source of truth.

## Lifecycle invariants

1. **Creation**: created by the VM reconcile path, always with at least one owner reference (the VM whose reconcile created it). A `SourceLabel` Tag is created with its label mirror set; a `GroupedAntiAffinitySelf` Tag has no source label and no mirror.
2. **Ownership add**: a VM adds **only its own** owner-reference entry, for every pair its own `spec.affinity` references — carriage is not required.
3. **Ownership remove**: a live VM removes **only its own** entry when its `spec.affinity` no longer references the `Tag`. Relabeling never removes ownership. A deleted VM removes nothing itself: the garbage collector prunes its entry, or deletes a `Tag` it solely owned, once the VM object is actually removed. That happens only after vCenter deleted the VM, so its policies never outlive the `Tag`s they target (`plan.md`, D18).
4. **Concurrent ownership writes**: every ownership write is a patch built with `client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})`, skipped entirely when `apiequality.Semantic.DeepEqual` reports no change to `ownerReferences`. A conflict is returned and retried by the next reconcile. A plain merge patch is **not** acceptable: `ownerReferences` is a list field and a merge patch would replace it wholesale, silently dropping a concurrent writer's entry.
5. **Deletion at zero owners**: the `Tag` controller `Delete`s the `Tag` when it observes an empty `ownerReferences` list, preconditioned on the `ResourceVersion` it read. `Tag` carries no finalizer (D10), so this delete is atomic — there is no persisted `deletionTimestamp` state to observe, only the resulting delete event (invariant 6). The precondition is what keeps this safe: if a VM concurrently added an owner reference after this reconcile's `Get`, the `Delete` fails with a conflict instead of destroying the newly-owned object, and the next reconcile re-reads the fresh state and skips deletion. Ordinary owner-reference garbage collection covers only the "all owner VMs deleted" case — it does **not** delete an object whose owner list was emptied while those owners still exist, which is why the `Tag` controller deletes explicitly. See [`research.md`](./research.md) "Owner-reference garbage collection", and V6's garbage-collector admission above for what garbage collection needs from admission.
6. **Deletion fan-out**: every VM in the namespace carrying the source label `spec.key=spec.value` is enqueued by the VM controller's `Tag` watch — via the `metadata.labels.keyValue` index, not a namespace scan — so label-only VMs drop the vCenter tag (spec G5). The single delete event is sufficient: a woken VM's desired-set computation no longer finds a matching `Tag` in the `spec.sourceLabel` index, so it emits `TagSpec{remove}`. The `Tag` controller itself enqueues nothing on any path (D16, D19).
7. **Re-create after deletion**: while a `Tag` with the derived name does not exist — because it was just deleted, or has not yet been created — a VM that needs it creates a fresh one rather than silently skipping tagging; the create is retried until it succeeds (spec G12).
8. **Status ownership**: only the `Tag` controller writes `status`, via the status subresource, with the same optimistic-lock-and-skip-if-unchanged discipline.
9. **No vCenter work**: nothing in the `Tag`'s lifecycle calls vCenter. `status.id` stays empty (spec NG1).

---

## Status conditions

| Type | Status | Reason | Meaning |
|------|--------|--------|---------|
| `Ready` | `True` | `Ready` | The `Tag` has been observed at its current generation, its label mirror matches its spec, and its owner set is non-empty. |
| `Ready` | `False` | `DeleteFailed` | The owner-reference list is empty but the `Delete` failed with something other than `NotFound` — typically the `ResourceVersion` precondition rejecting the delete because a VM added an owner reference concurrently. The reconcile returns the error, and the next one re-reads the fresh state. |

`Ready` is required by the constitution ("Controllers must track `status.observedGeneration` and set a `Ready` condition"). The condition type constant is the pre-existing `vspherepolv1.ReadyConditionType` in `common_types.go`; the `True` status sets no reason of its own (`conditions.MarkTrue` supplies the default), and `TagDeleteFailedReason` is declared in `tag_types.go` alongside the other API constants. A `Tag` with no owners is deleted outright and the reconcile returns without mutating it, so no condition is written for that case: deletion is atomic (no finalizer) and a patch would target an object that no longer exists.

There is no condition for a label-mirror failure. Mirror correction writes `metadata.labels[spec.key]` on the in-memory object and the reconcile's deferred patch persists it, so a failure surfaces as that patch's error rather than as a distinct reason.

---

## Admission rules (validating webhook)

Located at `webhooks/vspherepolicy/tag/validation/tag_validator.go` (D8). Verbs: `CREATE`, `UPDATE`. The webhook is not registered for `DELETE`, so deletion is governed by RBAC alone (D15).

The identity rules (V1, V2, V5) are implemented once, by `ValidateTagIdentity` in `pkg/providers/vsphere/virtualmachine/affinity_tags.go`, which the webhook and the VM path's adopt-existing check share.

| # | Rule | Operation | Error |
|---|------|-----------|-------|
| V1 | For `Original`, `AffinityTarget` and `GroupedAntiAffinityTarget` tags, `spec.membership` MUST be `SourceLabel` and `spec.key`/`spec.value` MUST be a valid, non-reserved Kubernetes label key and value (empty value permitted). For `GroupedAntiAffinitySelf` tags, `spec.membership` MUST be `PolicyRequesters` and `spec.key`/`spec.value` MUST be absent. | CREATE, UPDATE | `field.Invalid(spec, …)` |
| V2 | `spec.policy` MUST be absent for `Original` tags and present for every other kind, with a `type` that matches the kind, canonical (sorted, unique) `targets`, and a source label that agrees with those targets. `spec.name` MUST equal the name derived from the kind and the policy inputs (for `Original`, `<key>:<value>`). | CREATE, UPDATE | `field.Invalid(spec, …)` |
| V3/V4 | `spec` MUST NOT change after create. Enforced by a type-level CEL transition rule on the CRD; the webhook does not repeat the check. | UPDATE | CEL: "tag identity is immutable" |
| V5 | `metadata.name` MUST equal `ConcreteTagResourceName(metadata.namespace, spec.name)` | CREATE, UPDATE | `field.Invalid(spec, …)` |
| V6 | Only privileged accounts may create or update a `Tag`; on UPDATE the Kubernetes garbage collector (`generic-garbage-collector`) is also admitted | CREATE, UPDATE | `field.Forbidden(metadata, "only privileged users may …")` |

V6 uses the existing `pkgctx.WebhookRequestContext.IsPrivilegedAccount` mechanism, which is what admits the VM Operator service account and CSP admins while rejecting DevOps users (spec G9, US4 scenarios 3-4).

#### V6's garbage-collector admission

`IsPrivilegedAccount` (`pkg/builder/auth.go:42`) matches the VM Operator service account, `system:masters`, `kubernetes-admin`, and `PRIVILEGED_USERS`. The Kubernetes garbage collector matches none of them, so the webhook admits it explicitly, and only on UPDATE:

```go
const garbageCollectorUsername = "system:serviceaccount:kube-system:generic-garbage-collector"
```

`generic-garbage-collector` needs **UPDATE** to prune a dangling owner reference left by a deleted VM when other owners remain. Blocking it is damaging: a stale owner reference makes the `Tag` look owned forever, so neither the `Tag` controller's delete-at-zero-owners nor garbage collection ever removes it, leaving an orphaned `Tag` and a permanently stale vCenter tag on every label-only VM.

The garbage collector's DELETE of a solely-owned `Tag`, and the `namespace-controller`'s DELETE during namespace teardown, need no admission allowance because the webhook does not intercept DELETE.

Because DELETE is not intercepted, a DevOps user is kept from deleting a `Tag` by RBAC alone: no role in `config/` grants namespace users any access to `Tag`. A DevOps user is still rejected on CREATE and UPDATE by V6, and cannot impersonate a `kube-system` service account.

Note on V3/V4: the fan-out predicate depends on them. Because `spec` cannot change and the mapper's `List` key is derived from `spec` alone, no UPDATE can change the answer — `Tag` carries no finalizer (D10), so deletion is a plain, atomic `Delete` rather than a `deletionTimestamp` transition, and the predicate accordingly admits only create and delete, filtering out the `Tag` controller's own status/label-mirror writes and the VM path's owner-reference patches (`plan.md` "Fan-out — the VM controller's `Tag` watch"). If V3/V4 were ever relaxed, that predicate must be revisited in the same change.

Note on V5: it is a consistency check, not a security control — the name derivation is deterministic, so a hand-written `Tag` with a mismatched name would be invisible to the VM reconcile path (which looks the resource up by derived name) and would linger as garbage.

---

## RBAC

The VM controller's role needs write access to `Tag` (it creates them and patches ownership) **and** `watch` on it (the fan-out watch); the `Tag` controller needs the standard controller set plus status:

```go
// On both controllers/virtualmachine/virtualmachine/virtualmachine_controller.go
// and controllers/vspherepolicy/tag/tag_controller.go:
// +kubebuilder:rbac:groups=vsphere.policy.vmware.com,resources=tags,verbs=get;list;watch;create;update;patch;delete

// On controllers/vspherepolicy/tag/tag_controller.go only:
// +kubebuilder:rbac:groups=vsphere.policy.vmware.com,resources=tags/status,verbs=get;update;patch
```

No new `virtualmachines` marker is needed. The fan-out `List` runs in the VM controller's mapper, and that controller already holds `virtualmachines` read; the `Tag` controller reads only its own object. `make generate-manifests` regenerates `config/rbac/role.yaml`.

DevOps-user-facing RBAC is deliberately **not** extended: no role in `config/` grants `Tag` access to namespace users, and admission rule V6 backs that up.

---

## Conversion strategy

None. `v1alpha1` is the only version of the type, so there is no conversion webhook and no `utilconversion.MarshalData` annotation round-trip to worry about. If a future version is added, the storage version marker (`+kubebuilder:storageversion:true`) already sits on `v1alpha1` and a hub/spoke conversion would be introduced then, following the pattern in `api/`.

Because the `Tag` resource is internal bookkeeping (spec NG6), it carries no compatibility obligation toward DevOps users; the compatibility obligation is toward the vSphere policy module's other consumers, which is why the type lives in that module rather than in `api/` (D1).

---

## Canonical examples per user story

### US1 — VM references its own label

VM:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha6
kind: VirtualMachine
metadata:
  namespace: ns-1
  name: vm-a
  labels:
    app: nginx
spec:
  affinity:
    vmAffinity:
      requiredDuringSchedulingPreferredDuringExecution:
        - labelSelector:
            matchLabels:
              app: nginx
          topologyKey: kubernetes.io/hostname
```

Resulting `Tag` (`ns-1/tag-<hash>`): `spec.key: app`, `spec.value: nginx`, `metadata.labels: {app: nginx}`, one owner reference to `vm-a`. Resulting vCenter tag on `vm-a`: name `app:nginx`, category `ns-1`.

### US2 — label-only VM

```yaml
apiVersion: vmoperator.vmware.com/v1alpha6
kind: VirtualMachine
metadata:
  namespace: ns-1
  name: vm-label-only
  labels:
    app: nginx
spec: {}   # no affinity
```

The `Tag` above is **unchanged** — `vm-label-only` does not become an owner — but `vm-label-only` receives vCenter tag `app:nginx` in category `ns-1` on its next reconcile, triggered by the VM controller's `Tag` watch.

### US1 (reference without carriage) — `VmToVmGroupsAntiAffinity` against a label the VM does not carry

```yaml
apiVersion: vmoperator.vmware.com/v1alpha6
kind: VirtualMachine
metadata:
  namespace: ns-1
  name: vm-a
  labels:
    tier: web        # vm-a's own label — unrelated to the pair below
spec:
  affinity:
    vmAntiAffinity:
      requiredDuringSchedulingPreferredDuringExecution:
        - labelSelector:
            matchLabels:
              tier: db   # the anti-affinity target group's label, not vm-a's own
          topologyKey: kubernetes.io/hostname
```

Resulting `Tag` (`ns-1/tag-<hash-of-tier:db>`): one owner reference to `vm-a`, even though `vm-a` carries `tier: web`, not `tier: db`. `vm-a` itself is **not** given vCenter tag `tier:db` — its own tag carriage is driven only by the labels it carries. Any VM in `ns-1` carrying `tier: db` is tagged once this `Tag` exists, via the ordinary fan-out (US2), which is what lets `vm-a`'s anti-affinity term actually have a target to enforce against.

### US3 — label participation dropped

What changes here is `metadata.labels`, not `spec.affinity`. Ownership follows the affinity reference alone, so dropping a label never drops ownership — only the VM's own tag carriage reacts.

**Case 1 — `vm-a` drops a label it still references.** `vm-a` drops label `app: nginx` while its `spec.affinity` still references it, and `vm-b` also references and (still) carries it:

- `vm-a`'s owner reference is **unaffected** — it still references `app: nginx` — so the `Tag` survives regardless of what `vm-b` does.
- `vm-a` no longer carries `app: nginx`, so it **loses** vCenter tag `app:nginx` — `TagSpec{Operation: remove}` is emitted for it.
- `vm-label-only` is unaffected: it still carries the label and the `Tag` still exists.

**Case 2 — `vm-a` was the only VM ever referencing the pair, and later drops the label.** `vm-a` drops label `app: nginx`:

- `vm-a`'s owner reference is **unaffected**, because its `spec.affinity` still references `app: nginx`.
- The `Tag` persists indefinitely, with `vm-a` as its sole owner, even though `vm-a` no longer carries the label.
- The `Tag` is removed only if `vm-a` stops referencing the pair in its `spec.affinity` (the `Tag` controller then deletes it at zero owners) or `vm-a` is deleted (once the `vm-a` object is removed, the garbage collector deletes the `Tag` it solely owned; no finalizer, no terminating window) — never by a label change alone.
