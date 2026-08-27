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
    app: nginx                    # mirror of spec.key/spec.value for selector queries
  ownerReferences:                # owning VMs: reference the label from spec.affinity (carriage not required)
    - apiVersion: vmoperator.vmware.com/v1alpha6
      kind: VirtualMachine
      name: vm-a
      uid: 8a7f2c31-0e14-4a37-9c66-1b1f0d3a55e2
      controller: false
      blockOwnerDeletion: false
spec:
  key: app                        # the label KEY the tag represents
  value: nginx                    # the label VALUE the tag represents
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

| Field | JSON | Type | Marker | Description |
|-------|------|------|--------|-------------|
| `Key` | `key` | `string` | `+required`, `MinLength=1` | The label **key** the tag represents (e.g. `app`). Immutable after create. Mirrored into `metadata.labels` as a key. |
| `Value` | `value` | `string` | `+required` | The label **value** the tag represents (e.g. `nginx`). Immutable after create. Mirrored into `metadata.labels[spec.key]`. An empty value is legal (see "Empty label value" below), so **no** `MinLength`. |
| `Type` | `type,omitempty` | `TagType` | `+optional`, `Enum=System`, `default=System` | Describes the origin/category of this Tag. Currently a single-valued enum (`System`, the default); not consumed by any controller, webhook, or provider code in this feature — reserved for forward-compatibility. |

`spec.key` and `spec.value` together are the resource's identity: they determine the resource name (see "Name derivation"), so a change to either would make the name wrong. Both are enforced immutable by the validating webhook.

### `TagStatus`

| Field | JSON | Type | Marker | Description |
|-------|------|------|--------|-------------|
| `ID` | `id,omitempty` | `string` | `+optional` | Reserved for the resolved vCenter Tag UUID. **Not populated by this feature** (spec NG1) — the `Tag` controller performs no vCenter work, and tag attachment uses name+category rather than the UUID. Remains empty; absence is tolerated by every consumer. Retained on the CRD for forward-compatibility. |
| `ObservedGeneration` | `observedGeneration,omitempty` | `int64` | `+optional` | Last reconciled `metadata.generation` (constitution requirement). |
| `Conditions` | `conditions,omitempty` | `[]metav1.Condition` | `+optional`, `+listType=map`, `+listMapKey=type` | Standard conditions. MUST include `Ready` (`vspherepolv1.ReadyConditionType`, already defined in `common_types.go`). |

### Derived metadata

| Field | Written by | Description |
|-------|-----------|-------------|
| `metadata.name` | VM reconcile path, on create | `"tag-" + XXHash64Hex("<metadata.namespace>:<spec.key>:<spec.value>")`. See "Name derivation". |
| `metadata.namespace` | VM reconcile path, on create | The VM's namespace. Doubles as the vCenter Tag Category. |
| `metadata.labels[<spec.key>]` | VM reconcile path, on create | Mirror of `spec.value`, so an admin can run `kubectl get tags -l app=nginx`. This mirror is **not** what makes the controllers' queries efficient — see "Query surface" below. |
| `metadata.ownerReferences` | VM reconcile path — each VM writes **only its own entry** | One non-controller reference per owning VM (`controller: false`, `blockOwnerDeletion: false`). An owner is a VM whose own `spec.affinity` references the pair — carriage is not required. A VM that merely carries the label without referencing it is tagged in vCenter but is **not** an owner. Because `spec.affinity` is immutable, ownership — once established on a VM's first reconcile — lasts for that VM's lifetime. Ownership is not the same predicate as tag carriage; see the note below. |

`Tag` carries **no finalizer** (D10). Deletion at zero owners is a plain, atomic `Delete` — see invariant 5 below.

### Printer columns

Per D9 in [`research.md`](./research.md), no owner column:

```go
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
name = "tag-" + XXHash64Hex(metadata.namespace + ":" + spec.key + ":" + spec.value)
```

- `XXHash64Hex` (`pkg/util/hash.go`) formats the full 64-bit `xxhash.Sum64` digest as 16 hex characters, so the name is always 20 characters, always DNS-subdomain-safe, and independent of the shape of the namespace, key, and value.
- The hashed string uses `":"` as the separator — including the namespace prefix, which doubles as the vCenter Tag Category — so one canonical string identifies the pair everywhere.
- The derivation is a pure function of the namespace/key/value triple, so any VM in the namespace that needs the `Tag` computes the same name and either finds the existing resource or creates it. No lookup-by-selector is needed on the write path; the label mirror exists for read-side and diagnostic queries.
- **Collision handling**: 16 hex characters is 64 bits. Within one namespace's `Tag` set, collision is not a practical concern, and a collision would be self-consistent rather than silently wrong only if the pair matched — so on create-or-adopt the reconciler MUST verify that an existing resource with the derived name has the expected `spec.key` and `spec.value`, and MUST fail the reconcile with a wrapped error if it does not, rather than adopting a mismatched resource.

### Empty label value

`app=""` is a legal Kubernetes label. It yields `XXHash64Hex("my-namespace-1:app:")`, a `Tag` distinct from every non-empty value for the same key, and a `metadata.labels["app"] = ""` mirror — all legal. Hence no `MinLength` on `spec.value`.

---

## vCenter-side identity

| | |
|---|---|
| **Tag name** | `"<spec.key>:<spec.value>"` — byte-for-byte what `affinity.go` emits today |
| **Tag category** | `<metadata.namespace>` |
| **Emitted as** | `vimtypes.TagSpec{ArrayUpdateSpec{Operation: add\|remove}, Id: TagId{NameId: &TagIdNameId{Tag: name, Category: category}}}` |
| **Created by** | `vpxd`, when named in the VM's reconfigure request; or assumed pre-existing (spec NG1) |
| **Applied set recorded in** | the VM's ExtraConfig, key `vmservice.tags` — **not** on the `Tag` resource (D12) |

Keeping this format identical to today's (D6) is what makes the emitted tags interchangeable with the Compute Policy side, which builds `TagId.NameId` the same way in `buildTagIDsFromTopology`.

Which of these tags a given VM currently carries is recorded on the **VM**, in its own ExtraConfig key, and nowhere on the `Tag` resource. That is deliberate and consistent with "Ownership vs. tag carriage" below: the `Tag` never enumerates the VMs that carry it. The record exists because the attached-tag list vCenter returns is a list of tag URNs that cannot be matched against name+category tags, so it is the only way to know which of this feature's tags were previously applied — the same reason and the same workflow mechanism B uses with `vmservice.policy.tags`. See `research.md` "The attached-tag list is UUIDs and is not on `moVM`" and `plan.md` "`ReconcileTagSpecs` — steps 4-5".

---

## Query surface

The resource is designed so that "does a `Tag` for this label exist?" and "give me the `Tag`s for this label" are both answerable without scanning the namespace. Three mechanisms, in order of preference:

1. **The derived name is the primary key.** `metadata.name` is a pure function of `spec.key` + `spec.value` (see "Name derivation"), so an exact-pair existence check is a `Get` — an O(1) point read against the cache, no list and no selector. This is the check the VM reconcile path runs once per owned pair, and it is by far the most frequent query in the feature.

   One caveat applies to every read here, including this one: the client is the manager's **cached** client, so a `Tag` created earlier in the same reconcile is not guaranteed to be readable later in that reconcile. The VM path therefore carries the objects it created forward in memory rather than re-reading them; see `plan.md` "`ReconcileTagSpecs` — steps 4-5".
2. **Field indexes**, registered by the `VirtualMachine` controller's `AddToManager` — every consumer is on that controller's reconcile path — against the shared manager cache and therefore available to every client built from it:

   | Index name | Indexed value | Answers |
   |------------|---------------|---------|
   | `metadata.ownerReferences.uid` | one entry per owner UID | which `Tag`s does this VM own |

   There is deliberately no index for the exact key/value pair. Validation V5 pins `metadata.name` to `TagResourceName(spec.key, spec.value)` on create, and V3/V4 make both fields immutable on update, so a pair is always resolvable by the keyed `Get` in (1) — the name can never drift from the pair it encodes.

   The reverse direction — "which VMs carry this `Tag`'s label?", which the `Tag` controller asks on every create, ownership change, and delete — is indexed on the **VM** side rather than recorded on the `Tag`: a multi-valued field index `metadata.labels.keyValue` on `vmopv1.VirtualMachine`. See `plan.md` "Field indexes and query patterns". Nothing about the tagged-VM set is persisted on the `Tag` itself, which is what keeps the resource free of a second source of truth that could drift from the VMs' actual labels.

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

1. **Creation**: created by the VM reconcile path, always with at least one owner reference (the VM whose reconcile created it) and always with its label mirror set.
2. **Ownership add**: a VM adds **only its own** owner-reference entry, for every pair its own `spec.affinity` references — carriage is not required.
3. **Ownership remove**: a VM removes **only its own** entry when it is being deleted (before its finalizer is released). Because `spec.affinity` is immutable, this is the only trigger: a live VM's set of referenced pairs cannot shrink, so its ownership cannot be lost by relabeling.
4. **Concurrent ownership writes**: every ownership write is a patch built with `client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})`, skipped entirely when `apiequality.Semantic.DeepEqual` reports no change to `ownerReferences`. A conflict is returned and retried by the next reconcile. A plain merge patch is **not** acceptable: `ownerReferences` is a list field and a merge patch would replace it wholesale, silently dropping a concurrent writer's entry.
5. **Deletion at zero owners**: the `Tag` controller `Delete`s the `Tag` when it observes an empty `ownerReferences` list, preconditioned on the `ResourceVersion` it read. `Tag` carries no finalizer (D10), so this delete is atomic — there is no persisted `deletionTimestamp` state to observe, only the resulting delete event (invariant 6). The precondition is what keeps this safe: if a VM concurrently added an owner reference after this reconcile's `Get`, the `Delete` fails with a conflict instead of destroying the newly-owned object, and the next reconcile re-reads the fresh state and skips deletion. Ordinary owner-reference garbage collection is a backstop that covers only the "all owner VMs deleted" case — it does **not** delete an object whose owner list was emptied while those owners still exist. See [`research.md`](./research.md) "Owner-reference garbage collection", and V6's allow-list above for what that backstop needs from admission.
6. **Deletion fan-out**: every VM in the namespace carrying `spec.key=spec.value` is enqueued by the VM controller's `Tag` watch — via the `metadata.labels.keyValue` index, not a namespace scan — so label-only VMs drop the vCenter tag (spec G5). The single delete event is sufficient: a woken VM's desired-set computation no longer finds a matching `Tag` (`Get` returns `NotFound`), so it emits `TagSpec{remove}`. The `Tag` controller itself enqueues nothing on any path (D16, D19).
7. **Re-create after deletion**: while a `Tag` with the derived name does not exist — because it was just deleted, or has not yet been created — a VM that needs it creates a fresh one rather than silently skipping tagging; the create is retried until it succeeds (spec G12).
8. **Status ownership**: only the `Tag` controller writes `status`, via the status subresource, with the same optimistic-lock-and-skip-if-unchanged discipline.
9. **No vCenter work**: nothing in the `Tag`'s lifecycle calls vCenter. `status.id` stays empty (spec NG1).

---

## Status conditions

| Type | Status | Reason | Meaning |
|------|--------|--------|---------|
| `Ready` | `True` | `Ready` | The `Tag` has been observed at its current generation, its label mirror matches its spec, and its owner set is non-empty. |
| `Ready` | `False` | `NoOwners` | The owner-reference list is empty and the `Tag` is being deleted. Set in-memory after the `Delete` call; because deletion is atomic (no finalizer), the object is usually gone before this is ever persisted, so this condition is rarely observed on a real read — it is best-effort, not a guaranteed transient state. |
| `Ready` | `False` | `DeleteFailed` | The owner-reference list is empty but the `Delete` failed with something other than `NotFound` — typically the `ResourceVersion` precondition rejecting the delete because a VM added an owner reference concurrently. The reconcile returns the error, and the next one re-reads the fresh state. |

`Ready` is required by the constitution ("Controllers must track `status.observedGeneration` and set a `Ready` condition"). The condition type constant is the pre-existing `vspherepolv1.ReadyConditionType` in `common_types.go`; `TagReadyReason` and `TagNoOwnersReason` are new constants declared in `tag_types.go`, and `DeleteFailed` is a private constant in the controller package since nothing outside it can observe that state.

There is no condition for a label-mirror failure. Mirror correction writes `metadata.labels[spec.key]` on the in-memory object and the reconcile's deferred patch persists it, so a failure surfaces as that patch's error rather than as a distinct reason.

---

## Admission rules (validating webhook)

Located at `webhooks/vspherepolicy/tag/validation/tag_validator.go` (D8). Verbs: `CREATE`, `UPDATE`, `DELETE`.

| # | Rule | Operation | Error |
|---|------|-----------|-------|
| V1 | `spec.key` MUST be non-empty and a valid Kubernetes label key | CREATE, UPDATE | `field.Invalid(spec.key, …)` |
| V2 | `spec.value` MUST be a valid Kubernetes label value (empty permitted) | CREATE, UPDATE | `field.Invalid(spec.value, …)` |
| V3 | `spec.key` MUST NOT change | UPDATE | `field.Forbidden(spec.key, "field is immutable")` |
| V4 | `spec.value` MUST NOT change | UPDATE | `field.Forbidden(spec.value, "field is immutable")` |
| V5 | `metadata.name` MUST equal the derived name for `spec.key`/`spec.value` | CREATE | `field.Invalid(metadata.name, …)` |
| V6 | Only privileged accounts, or the system service accounts allow-listed below, may create, update, or delete a `Tag` | CREATE, UPDATE, DELETE | `field.Forbidden(…, "only privileged users may …")` |

V6 uses the existing `pkgctx.WebhookRequestContext.IsPrivilegedAccount` mechanism, which is what admits the VM Operator service account and CSP admins while rejecting DevOps users (spec G9, US4 scenarios 3-4).

#### V6's system-account allow-list

`IsPrivilegedAccount` (`pkg/builder/auth.go:42`) matches the VM Operator service account, `system:masters`, `kubernetes-admin`, and `PRIVILEGED_USERS`. Two Kubernetes control-plane clients match none of them and **must** be allow-listed, checked before the privileged test:

```go
var allowedSystemAccountsForTag = map[string]struct{}{
    "system:serviceaccount:kube-system:generic-garbage-collector": {},
    "system:serviceaccount:kube-system:namespace-controller":      {},
}
```

- `generic-garbage-collector` needs **UPDATE** to prune a dangling owner reference left by a deleted VM, and **DELETE** for the all-owners-deleted backstop. Blocking the UPDATE is the more damaging of the two: a stale owner reference makes the `Tag` look owned forever, so the `Tag` controller's delete-at-zero-owners never fires either, leaving an orphaned `Tag` and a permanently stale vCenter tag on every label-only VM.
- `namespace-controller` needs **DELETE** to tear down namespaced resources. Denying it leaves the namespace in `Terminating` indefinitely.

This mirrors `webhooks/persistentvolumeclaim/validation/persistentvolumeclaim_validator.go:36-43`, the only other webhook in this repository that intercepts DELETE, which keeps an equivalent allow-list containing both of these accounts.

The allow-list does not weaken spec G9 or US4 scenarios 3-4: a DevOps user is still rejected on CREATE, UPDATE and DELETE, and cannot impersonate a `kube-system` service account.

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

`spec.affinity` is immutable (spec NG8), so what changes here is `metadata.labels`. Ownership follows the affinity reference alone, so dropping a label never drops ownership — only the VM's own tag carriage reacts.

**Case 1 — `vm-a` drops a label it still references.** `vm-a` drops label `app: nginx` while its `spec.affinity` still references it, and `vm-b` also references and (still) carries it:

- `vm-a`'s owner reference is **unaffected** — it still references `app: nginx` — so the `Tag` survives regardless of what `vm-b` does.
- `vm-a` no longer carries `app: nginx`, so it **loses** vCenter tag `app:nginx` — `TagSpec{Operation: remove}` is emitted for it.
- `vm-label-only` is unaffected: it still carries the label and the `Tag` still exists.

**Case 2 — `vm-a` was the only VM ever referencing the pair, and later drops the label.** `vm-a` drops label `app: nginx`:

- `vm-a`'s owner reference is **unaffected**, because its `spec.affinity` still references `app: nginx` and that cannot change.
- The `Tag` persists indefinitely, with `vm-a` as its sole owner, even though `vm-a` no longer carries the label.
- The `Tag` is removed only if `vm-a` is later deleted — the `Tag` controller observes the resulting empty `ownerReferences` list and deletes the `Tag` outright (no finalizer, no terminating window) — never by a label change alone.
