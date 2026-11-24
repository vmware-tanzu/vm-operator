# Data Model: VirtualMachine Affinity Status

- **Spec**: [`spec.md`](./spec.md)
- **APIs touched**: `vmoperator.vmware.com/v1alpha6` `VirtualMachine` (status only), `vsphere.policy.vmware.com/v1alpha1` `Tag` (status only)

---

## 1. `VirtualMachine.status.affinity` (v1alpha6)

New file `api/v1alpha6/virtualmachine_affinity_status_types.go`.

```go
// VirtualMachineAffinityPolicyType is the kind of a placement policy.
// +kubebuilder:validation:Enum=Affinity;AntiAffinity
type VirtualMachineAffinityPolicyType string

const (
    VirtualMachineAffinityPolicyTypeAffinity     VirtualMachineAffinityPolicyType = "Affinity"
    VirtualMachineAffinityPolicyTypeAntiAffinity VirtualMachineAffinityPolicyType = "AntiAffinity"
)

// VirtualMachineAffinityPolicyStrictness uses the spec.affinity list names.
// +kubebuilder:validation:Enum=RequiredDuringSchedulingRequiredDuringExecution;RequiredDuringSchedulingPreferredDuringExecution;PreferredDuringSchedulingPreferredDuringExecution
type VirtualMachineAffinityPolicyStrictness string

// VirtualMachineAffinityCompliance is the VM's compliance with a policy.
// +kubebuilder:validation:Enum=Pending;Compliant;NotCompliant;NotApplicable;Unknown
type VirtualMachineAffinityCompliance string

// VirtualMachineAffinityPolicyOrigin describes why a policy applies to the VM.
// +kubebuilder:validation:Enum=Self;Peer
type VirtualMachineAffinityPolicyOrigin string

const (
    // The VM's own spec.affinity requests the policy.
    VirtualMachineAffinityPolicyOriginSelf VirtualMachineAffinityPolicyOrigin = "Self"
    // Another VM's spec.affinity requests the policy, and it applies to this
    // VM because this VM carries the policy's target labels.
    VirtualMachineAffinityPolicyOriginPeer VirtualMachineAffinityPolicyOrigin = "Peer"
)

// VirtualMachineAffinityLabel is a resolved target label pair.
type VirtualMachineAffinityLabel struct {
    // +required
    Key string `json:"key"`
    // +required
    Value string `json:"value"`
}

// VirtualMachineAffinityPolicyStatus describes one VM Operator affinity
// policy that applies to the VM.
type VirtualMachineAffinityPolicyStatus struct {
    // +required
    Type VirtualMachineAffinityPolicyType `json:"type"`

    // +required
    Strictness VirtualMachineAffinityPolicyStrictness `json:"strictness"`

    // TopologyKey is kubernetes.io/hostname or topology.kubernetes.io/zone.
    // +required
    TopologyKey string `json:"topologyKey"`

    // Targets are sorted by key, then value, with no duplicates.
    // +required
    // +listType=atomic
    Targets []VirtualMachineAffinityLabel `json:"targets"`

    // PolicyID is the vCenter compute policy ID. Empty while Pending.
    // +optional
    PolicyID string `json:"policyID,omitempty"`

    // Compliance is the most recent compliance vCenter recorded.
    // +required
    Compliance VirtualMachineAffinityCompliance `json:"compliance"`

    // Origin is Self when the VM's own spec.affinity requests the policy,
    // or Peer when another VM's does and this VM carries the target labels.
    // +required
    Origin VirtualMachineAffinityPolicyOrigin `json:"origin"`
}

// VirtualMachineAffinityStatus reports the VM Operator affinity policies
// that apply to the VM.
type VirtualMachineAffinityStatus struct {
    // +optional
    // +listType=atomic
    Policies []VirtualMachineAffinityPolicyStatus `json:"policies,omitempty"`
}
```

Added to `VirtualMachineStatus`:

```go
    // +optional

    // Affinity describes the VM Operator affinity policies that apply to
    // this VM and the VM's compliance with each.
    Affinity *VirtualMachineAffinityStatus `json:"affinity,omitempty"`
```

### Entry set

The entry set is the union, keyed by `Tag`, of:

1. **Reported**: each policy ID in `moVM.ComputePolicies` that the `Tag` index (`status.policyID`) maps to a policy-defining `Tag` in the VM's namespace. The entry gets the policy ID and the mapped compliance.
2. **Pending**: each related policy-defining `Tag` (owned or carried by the VM, spec "Related `Tag`s") that is not in (1). The entry gets `compliance: Pending` and no policy ID.

Each entry is built from the `Tag`'s `spec.policy`:

| `Tag` field | Status field |
|-------------|--------------|
| `spec.kind` `AffinityTarget` / `GroupedAntiAffinitySelf` | `type` `Affinity` / `AntiAffinity` |
| `spec.policy.strictness` `PreferredDuringPlacementPreferredDuringExecution` (and the other two) | `strictness` `PreferredDuringSchedulingPreferredDuringExecution` (and the other two) |
| `spec.policy.topology` `Host` / `VSphereZone` | `topologyKey` `kubernetes.io/hostname` / `topology.kubernetes.io/zone` |
| `spec.policy.targets` | `targets` |
| `metadata.name` in the VM's own `DesiredAffinityTags` (from `spec.affinity`) / otherwise | `origin` `Self` / `Peer` |

`origin` is computed from the VM's `spec.affinity`, not from the `Tag`'s owner references, so it is correct before the VM's owner reference is written. A VM that both requests a policy and carries its target labels (for example self anti-affinity across replicas) is `Self`. When the owned `Tag`s cannot be computed in a pass, an entry keeps the previous `origin` of the same policy, defaulting to `Peer`.

| `computePolicies` `complianceStatus` | `compliance` |
|--------------------------------------|--------------|
| `Compliant` | `Compliant` |
| `NonCompliant` | `NotCompliant` |
| `NotApplicable` | `NotApplicable` |
| `Unknown`, or any unrecognized value | `Unknown` |

### Semantics

- **Single writer.** Only the VM controller writes the field, through the existing deferred `patch.Helper` status patch. The list is `atomic` and is rebuilt on every reconcile, so `WithStatusOptimisticLock` is not needed.
- **Ordering.** Entries are sorted by `(topologyKey, strictness, type, targets)`, so an idle reconcile produces no diff.
- **Empty.** When the entry set is empty, `status.affinity` is `nil`.

### Conversion

v1alpha6 is the hub. Down-conversion to v1alpha1 through v1alpha5 drops the field, and it is regenerated on the next reconcile. Regenerate with `make generate-go-conversions`.

### Example

```yaml
status:
  affinity:
    policies:
    - type: AntiAffinity
      strictness: RequiredDuringSchedulingPreferredDuringExecution
      topologyKey: kubernetes.io/hostname
      targets:
      - key: app
        value: db
      policyID: 8f1c2d0e-1b2a-4c3d-9e8f-0a1b2c3d4e5f
      compliance: Compliant
      origin: Self
    - type: Affinity
      strictness: PreferredDuringSchedulingPreferredDuringExecution
      topologyKey: topology.kubernetes.io/zone
      targets:
      - key: tier
        value: web
      compliance: Pending
      origin: Peer
```

---

## 2. `Tag.status` (vsphere.policy v1alpha1)

`external/vsphere-policy/api/v1alpha1/tag_types.go`:

```go
type TagStatus struct {
    // ID is the vCenter tag UUID, resolved by name in the namespace
    // category.
    // +optional
    ID string `json:"id,omitempty"`

    // CategoryID is the vCenter tag category UUID.
    // +optional
    CategoryID string `json:"categoryID,omitempty"`

    // PolicyID is the vCenter compute policy this tag defines. Set only
    // for AffinityTarget and GroupedAntiAffinitySelf kinds.
    // +optional
    PolicyID string `json:"policyID,omitempty"`

    // ObservedGeneration, Conditions: unchanged.
}
```

CEL on `TagStatus`:

```go
// +kubebuilder:validation:XValidation:rule="!has(self.policyID) || has(self.id)",message="policyID requires id"
```

New print columns: `TagID` (`.status.id`, `priority=1`) and `PolicyID` (`.status.policyID`, `priority=1`).

### Field index

`kubeutil.TagPolicyIDIndexKey = "status.policyID"`, with `kubeutil.TagPolicyIDIndexerFunc` (`pkg/util/kube/vmtags.go`), registered on `vspherepolv1.Tag` by `kubeutil.RegisterTagPolicyIDIndex` from the VM controller's `AddToManager` when `AffinityStatusEnabled` is true. The fake client in `test/builder/fake.go` registers it unconditionally. It is queried with `client.InNamespace(vm.Namespace)` and `client.MatchingFields{kubeutil.TagPolicyIDIndexKey: id}`. Empty values are not indexed.

### Condition

| Type | Status | Reason | When |
|------|--------|--------|------|
| `VCenterTagResolved` | `True` | — | `status.id` and `status.categoryID` are set |
| `VCenterTagResolved` | `False` | `NotFound` | The category or the tag `vmoperator:` + `spec.name` does not exist yet |
| `VCenterTagResolved` | `False` | `ResolveFailed` | The tagging API call failed |

Constants: `TagConditionVCenterTagResolved`, `TagVCenterTagNotFoundReason`, `TagVCenterTagResolveFailedReason`. `Ready` keeps its spec 006 meaning, and resolution does not gate it.

### Writers

| Field | Writer | Patch |
|-------|--------|-------|
| `id`, `categoryID`, `VCenterTagResolved` | `Tag` controller | Existing deferred `patch.Helper` |
| `policyID` | VM update path (`pkg/providers/vsphere/vmlifecycle`) | `Status().Patch` with `MergeFromWithOptimisticLock`, skipped when unchanged. A conflict is logged, the entry keeps its previous policy ID, with the compliance currently reported for it, if the policy is still reported (otherwise `Pending`) for this pass, and the next pass retries |

The fields are disjoint scalars, so the two writers do not interfere.

---

## 3. Provider interface additions

`pkg/providers/vm_provider_interface.go`:

```go
// ResolveTagIDs returns the ID of the vCenter tag category with the given
// name and the IDs of the named tags in that category. Names that do not
// exist are absent from the returned map. The category name is required. A
// missing category is not an error: the returned category ID is empty and
// the map has no entries.
ResolveTagIDs(
    ctx context.Context,
    category string,
    names []string) (string, map[string]string, error)
```

The compute-policy query is internal to the vSphere provider's VM update path. It sits behind the `vmlifecycle.ComputePolicyQuerier` interface, whose `Query(ctx, vm, spec)` method takes the VM the query is made for. `vmlifecycle.NewComputePolicyQuerier(restClient)` returns the unexported `computePolicyQuerier` implementation, which the provider passes through `ReconcileStatusData.ComputePolicyQuerier` only when the gate is on. Tests substitute a fake, because vcsim does not support the endpoint (spec NG9). A nil querier skips the query.
