# Feature Specification: VM Hard Affinity During Execution

- **Feature branch**: [`aniketd/affinity-spec`](https://github.com/aniket-deole/vm-operator/tree/aniketd/affinity-spec)
  - **Fork**: `aniket-deole/vm-operator`
  - **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-10-07
- **Status**: In Progress
- **Epic**: vmop-3332

---

## Summary

DRS has added several placement capabilities that VM Service exposes to VM Service VMs in 9.2:

- A `RequiredDuringExecution` compute policy strictness, i.e. hard affinity / anti-affinity that DRS keeps enforcing after the VM is placed.
- `VmToVmGroupsAntiAffinity` at host topology, which supersedes `VmVmAntiAffinity`.
- A per-VM `candidateVSphereZone` in `PlaceVmsXCluster`, allowing VMs in one placement group to request different zones.
- `PlaceVmsXCluster` considering existing workloads when evaluating affinity, so a VM no longer has to be a member of a `VirtualMachineGroup` to use affinity.

All of this is gated by a single Supervisor capability, `supports_vm_service_vm_hard_affinity_during_execution`, which itself depends on the vCenter FSS `PlacementPoliciesForVmSvcVmsV3`. The capability enables two VM Service features:

| Feature | Purpose |
|---------|---------|
| `VMHardAffinityDuringExecution` | All placement behavior in this spec. |
| `TaggingAPI` | The `Tag` CRD and controller defined in [006-tag-controller-for-affinity](../006-tag-controller-for-affinity/spec.md). |

When the capability is not activated, VM Service behaves as in 9.1.x.

---

## Key entities

| Entity | Description |
|--------|-------------|
| `VirtualMachine.spec.affinity` | v1alpha6 VM affinity / anti-affinity terms. Each term is a label selector plus a topology key (`kubernetes.io/hostname` or `topology.kubernetes.io/zone`), in one of three strictness lists: `requiredDuringSchedulingRequiredDuringExecution`, `requiredDuringSchedulingPreferredDuringExecution`, `preferredDuringSchedulingPreferredDuringExecution`. Immutable after create. |
| `RequiredDuringExecutionVMPlacementPolicy` | Namespaced `vsphere.policy.vmware.com/v1alpha1` resource. Its presence in a namespace is the entitlement for RequiredDuringExecution terms. Created when a CSP admin assigns the matching InfraPolicy to the namespace. Has no behavior beyond presence. |
| vCenter Tag | Created from a VM label referenced by an affinity term; identifies the VMs a placement policy targets. See spec 006. |
| vCenter VM placement policy | Configured on the VM: `VmVmAffinity`, `VmVmAntiAffinity`, or `VmToVmGroupsAntiAffinity`, with a strictness (`RequiredDuringPlacementRequiredDuringExecution`, `RequiredDuringPlacementPreferredDuringExecution`, `PreferredDuringPlacementPreferredDuringExecution`) and topology (host or vSphere zone). Cannot be removed from a VM once configured. |
| `PlaceVmsXCluster` | DRS placement API for one or more VMs. Accepts a per-VM `candidateVSphereZone` and considers existing workloads' policies. |
| Supervisor capability `supports_vm_service_vm_hard_affinity_during_execution` | Gates `VMHardAffinityDuringExecution` and `TaggingAPI`. Depends on vCenter FSS `PlacementPoliciesForVmSvcVmsV3`. |

---

## Relationship to existing affinity behavior

| 9.1.x behavior | With this feature |
|----------------|-------------------|
| Two strictness lists: `requiredDuringSchedulingPreferredDuringExecution`, `preferredDuringSchedulingPreferredDuringExecution`. | Adds `requiredDuringSchedulingRequiredDuringExecution`, entitlement-gated. |
| Host anti-affinity is one `VmVmAntiAffinity` per tag (9.1.1). | One `VmToVmGroupsAntiAffinity` per strictness for new VMs; existing VMs keep `VmVmAntiAffinity`. |
| Zone policies are used only at placement and not persisted on create. | Zone policies are persisted on the VM at create. |
| Affinity requires `spec.groupName`; VMs in a group are placed together without per-VM zones. | `spec.groupName` optional; per-VM `candidateVSphereZone` in group placement. |
| Only labels referenced by a VM's own `spec.affinity` at create are tagged. | Labels referenced by any VM are tagged on every carrying VM (spec 006). |
| VKS node host anti-affinity via cluster modules. | VKS configures either cluster modules or placement policies per VM; VM Service never configures both. New NodePools may use placement policies; existing NodePools keep cluster modules. |

## Policy selection flow

```
VM create
  |
  +-- capability off? -- yes --> 9.1.x: RDE terms denied, groupName
  |                              required, VmVmAntiAffinity per host
  |                              tag, zone policies placement-only
  no
  |
  +-- RDE terms set?
  |     +-- no entitlement in namespace --> deny
  |     +-- topology not host/zone ------> deny
  |
  +-- VM has cluster module? -- yes --> no placement policies
  |
  +-- per term (strictness x topology):
  |     affinity       --> VmVmAffinity per tag
  |     anti-affinity  --> one VmToVmGroupsAntiAffinity
  |
  +-- zone policies persisted in create ConfigSpec
  |
  +-- PlaceVmsXCluster (group or single VM),
        candidateVSphereZone per VM
```

---

## Goals

### G1 — RequiredDuringExecution affinity and anti-affinity

- G1.1 A v1alpha6 VM **MUST** be able to specify `spec.affinity.vmAffinity.requiredDuringSchedulingRequiredDuringExecution` and `spec.affinity.vmAntiAffinity.requiredDuringSchedulingRequiredDuringExecution`, in addition to the existing `requiredDuringSchedulingPreferredDuringExecution` and `preferredDuringSchedulingPreferredDuringExecution`.
- G1.2 RequiredDuringExecution terms **MUST** be accepted with the host topology key (`kubernetes.io/hostname`) and the zone topology key (`topology.kubernetes.io/zone`). Zone RequiredDuringExecution is accepted for API symmetry; it is behaviorally equivalent to RequiredDuringPlacement because a VM cannot move across zones after it is created.
- G1.3 Because a host RequiredDuringExecution policy can prevent a host from entering maintenance mode, its use **MUST** be an explicit namespace entitlement: the namespace **MUST** contain at least one `RequiredDuringExecutionVMPlacementPolicy` (any name). It is created in the namespace when the CSP admin assigns an InfraPolicy of kind `RequiredDuringExecutionVMPlacementPolicy` to the namespace.
- G1.4 The webhook **MUST** reject a create with RequiredDuringExecution terms when `VMHardAffinityDuringExecution` is disabled or the namespace has no `RequiredDuringExecutionVMPlacementPolicy`. `spec.affinity` is immutable after create, so the entitlement is only evaluated at create.
- G1.5 When the entitlement is removed from a namespace, existing VMs **MUST** continue to run with their RequiredDuringExecution policies (vCenter placement policies cannot be removed from a VM). Other updates to those VMs **MUST** be admitted. New VMs **MUST NOT** be admitted with RequiredDuringExecution terms.
- G1.6 VM Service **MUST** configure whichever mechanism a VKS node VM specifies: cluster modules or placement policies (`spec.affinity`), never both. A VM that has a cluster module **MUST NOT** be configured with any placement policies. A VKS node VM without a cluster module is treated like any other VM, including RequiredDuringExecution terms.
- G1.7 VKS is expected to use placement policies only for new NodePools. Operations on existing NodePools continue to use cluster modules, so no NodePool mixes cluster modules and placement policies (avoiding a split-brain between the two mechanisms).

### G2 — VmToVmGroupsAntiAffinity at host topology

- G2.1 When `VMHardAffinityDuringExecution` is enabled, host-topology anti-affinity **MUST** be configured as a single `VmToVmGroupsAntiAffinity` policy per strictness, covering all anti-affined tags.
- G2.2 When `VMHardAffinityDuringExecution` is disabled, host-topology anti-affinity **MUST** be configured as one `VmVmAntiAffinity` policy per tag (9.1.1 behavior).
- G2.3 Zone-topology anti-affinity continues to use `VmToVmGroupsAntiAffinity` regardless of the feature.

### G3 — candidateVSphereZone in PlaceVmsXCluster

- G3.1 When `VMHardAffinityDuringExecution` is enabled, each VM's zone **MUST** be passed as its `candidateVSphereZone` in `PlaceVmsXCluster`, so VMs that request different zones can be placed in a single group.
- G3.2 When disabled, `candidateVSphereZone` **MUST NOT** be sent (9.1.x behavior), and VMs without zone pinning in a group may land in different zones.

### G4 — Persist zonal placement policies at create

- G4.1 When `VMHardAffinityDuringExecution` is enabled, zone-topology placement policies **MUST** be persisted on the VM at create time, not only used during placement. Today they are omitted from the create `ConfigSpec`.
- G4.2 When disabled, zone-topology policies **MUST** continue to be used only at placement time.

### G5 — Affinity for VMs that are not in a VirtualMachineGroup

- G5.1 When `VMHardAffinityDuringExecution` is enabled, `spec.groupName` **MUST** be optional for every affinity type (affinity and anti-affinity; all three strictness lists; host and zone topology). DRS honors the affinity policies configured on existing VMs when placing the new VM.
- G5.2 When disabled, the webhook **MUST** continue to require `spec.groupName` whenever `spec.affinity` is set.

### G6 — Tagging VMs without placement policies

- G6.1 VMs that carry labels but declare no `spec.affinity` **MUST** have those labels promoted to vCenter tags if, and only if, another VM in the namespace references them in an affinity term. Labels that are not referenced **MUST NOT** be promoted. Behavior is defined in [006-tag-controller-for-affinity](../006-tag-controller-for-affinity/spec.md) and gated by `TaggingAPI`.

---

## Non-goals

- Deprecating or removing cluster modules. Cluster modules are still used by VKS control plane nodes (see "Interaction with cluster modules").
- Removing placement policies from existing VMs when the RequiredDuringExecution entitlement is revoked.
- Migrating existing VKS NodePools from cluster modules to placement policies.
- Exposing a new API field: `requiredDuringSchedulingRequiredDuringExecution` already exists in v1alpha6. It is restored on round-trip through v1alpha5.

---

## User stories and acceptance criteria

### US1 — CSP admin entitles a namespace for RequiredDuringExecution (G1) (Priority: P0)

As a **CSP admin**, I assign an InfraPolicy of kind `RequiredDuringExecutionVMPlacementPolicy` to a namespace so DevOps users there can use hard execution-time affinity.

- **AC1.1** **Given** the capability is activated and the namespace has a `RequiredDuringExecutionVMPlacementPolicy`, **When** a DevOps user creates a VM with a `requiredDuringSchedulingRequiredDuringExecution` affinity or anti-affinity term with topology `kubernetes.io/hostname`, **Then** the VM is admitted and vCenter shows a `VmVmAffinity` (affinity) or `VmToVmGroupsAntiAffinity` (anti-affinity) policy with strictness `RequiredDuringPlacementRequiredDuringExecution` and host topology, and the VM is compliant.
- **AC1.2** **Given** the same namespace, **When** the term uses topology `topology.kubernetes.io/zone`, **Then** the VM is admitted and vCenter shows the zone policy with strictness `RequiredDuringPlacementRequiredDuringExecution`.
- **AC1.3** **Given** the capability is activated and the namespace has no `RequiredDuringExecutionVMPlacementPolicy`, **When** a DevOps user creates a VM with a RequiredDuringExecution term, **Then** the request is denied with `requiredDuringSchedulingRequiredDuringExecution is not supported`.
- **AC1.4** **Given** the capability is not activated, **When** a DevOps user creates a VM with a RequiredDuringExecution term, **Then** the request is denied with the same message, regardless of entitlement.
- **AC1.5** **Given** an entitled namespace, **When** a RequiredDuringExecution term uses any other topology key, **Then** the request is denied with `Unsupported value`.
- **AC1.6** **Given** an entitled namespace, **When** a VKS node VM that has a cluster module is created with `spec.affinity` terms, **Then** no placement policies are configured on it and its host anti-affinity uses the cluster module.
- **AC1.7** **Given** an entitled namespace, **When** a VKS node VM without a cluster module is created with host RequiredDuringExecution terms, **Then** the placement policies are configured on it as for any other VM.

### US2 — CSP admin revokes the entitlement (G1.5) (Priority: P0)

As a **CSP admin**, I unassign the namespace's `RequiredDuringExecutionVMPlacementPolicy` InfraPolicy.

- **AC2.1** **Given** VMs with RequiredDuringExecution terms exist in the namespace, **When** the entitlement is removed, **Then** the VMs keep running and their vCenter policies are unchanged.
- **AC2.2** **Given** the entitlement has been removed, **When** a DevOps user updates such a VM outside `spec.affinity` (e.g. a label or power state), **Then** the update is admitted.
- **AC2.3** **Given** the entitlement has been removed, **When** a DevOps user creates a new VM with RequiredDuringExecution terms, **Then** the request is denied.

### US3 — DevOps user anti-affines groups of VMs on hosts (G2) (Priority: P0)

As a **DevOps user**, I specify host anti-affinity against one or more label selectors.

- **AC3.1** **Given** the capability is activated, **When** a VM is created with host anti-affinity against one or more labels, **Then** vCenter shows a single `VmToVmGroupsAntiAffinity` policy with host topology per strictness, listing every anti-affined tag.
- **AC3.2** **Given** the capability is not activated, **When** the same VM is created, **Then** vCenter shows one `VmVmAntiAffinity` policy per tag.
- **AC3.3** **Given** VMs created with `VmVmAntiAffinity` host policies before the capability was activated, **When** the capability is activated, **Then** those VMs keep their `VmVmAntiAffinity` policies.

### US4 — DevOps user places a group whose VMs request different zones (G3) (Priority: P0)

As a **DevOps user**, I create a `VirtualMachineGroup` whose members pin to different zones.

- **AC4.1** **Given** the capability is activated, **When** a `VirtualMachineGroup` whose members pin to different zones is placed, **Then** placement happens in one `PlaceVmsXCluster` call and every VM lands in its requested zone.
- **AC4.2** **Given** the capability is not activated, **When** the same group is placed, **Then** placement behaves as in 9.1.x.

### US5 — DevOps user's zonal policies persist after create (G4) (Priority: P0)

As a **DevOps user**, I create a VM with zone-topology affinity or anti-affinity.

- **AC5.1** **Given** the capability is activated, **When** the VM is created, **Then** vCenter shows the zone placement policies on the VM after create.
- **AC5.2** **Given** the capability is not activated, **When** the VM is created, **Then** the zone policies are used for placement only and are not on the VM after create.

### US6 — DevOps user uses affinity without a VirtualMachineGroup (G5) (Priority: P0)

As a **DevOps user**, I set `spec.affinity` on a VM without putting it in a `VirtualMachineGroup`.

- **AC6.1** **Given** the capability is activated and an existing VM carries label `tier: web`, **When** a VM with no `spec.groupName` is created with zone affinity to `tier: web`, **Then** it is admitted and lands in the same zone as the existing VM.
- **AC6.2** **Given** the capability is not activated, **When** the same VM is created, **Then** the request is denied with `spec.groupName: Required value: when setting affinity`.
- **AC6.3** **Given** the capability is activated and existing VMs carry label `app: vnf`, **When** a VM with no `spec.groupName` is created with host anti-affinity to `app: vnf`, **Then** it is admitted and lands on a host without those VMs.

### US7 — DevOps user targets VMs that declare no affinity (G6) (Priority: P0)

As a **DevOps user**, I reference a label carried by VMs that declare no `spec.affinity`.

- **AC7.1** **Given** a VM carries label `db: primary` and no `spec.affinity`, **When** another VM is created with anti-affinity to `db: primary`, **Then** the label is promoted to a vCenter tag on the first VM and the policy targets it.
- **AC7.2** **Given** a VM carries a label no other VM references, **When** it is reconciled, **Then** the label is not promoted to a vCenter tag. Full behavior: [006-tag-controller-for-affinity](../006-tag-controller-for-affinity/spec.md).

### US8 — Placement status on VMs with affinity policies (Priority: P2)

As a **DevOps user**, I can see on a VM whether its affinity / anti-affinity policies are satisfied.

- [NEEDS CLARIFICATION: Which status surface (condition vs. status field), which vCenter source of compliance, and which states are reported? All US8 tasks are blocked until resolved.]

---

## Edge cases

| # | Scenario | Expected behavior |
|---|----------|-------------------|
| E1 | Namespace has multiple `RequiredDuringExecutionVMPlacementPolicy` objects; one is deleted. | Entitlement still holds; admission unchanged. |
| E2 | User tries to add, change, or remove RequiredDuringExecution terms on an existing VM. | Denied: `spec.affinity` is immutable after create (`updating Affinity is not allowed`), regardless of entitlement. |
| E3 | Capability is deactivated after VMs were created with RequiredDuringExecution terms. | Existing VMs keep running and other updates are admitted; new VMs with RequiredDuringExecution terms are denied. |
| E4 | Cluster is upgraded and the capability is activated; existing VMs have 9.1.1 `VmVmAntiAffinity` host policies. | Existing VMs keep `VmVmAntiAffinity`. Only newly created VMs get `VmToVmGroupsAntiAffinity`. |
| E5 | The webhook cannot list `RequiredDuringExecutionVMPlacementPolicy` (API error). | Request denied with `InternalError`; no silent admit. |
| E6 | VM specifies both a cluster module and `spec.affinity` terms. | Only the cluster module is configured; no placement policies are configured (G1.6). |
| E7 | VM specifies both host and zone RequiredDuringExecution terms. | Both are admitted (with entitlement), and a policy is emitted for each topology. |
| E8 | Host RequiredDuringExecution affinity cannot be satisfied at placement (e.g. no host has capacity alongside the affined VMs). | Placement fails and the VM is not created; the placement error is shown in the VM's status. |
| E9 | Host RequiredDuringExecution anti-affinity blocks a host from entering maintenance mode. | Expected DRS behavior; this is why the entitlement exists. Not handled by VM Service. |
| E10 | Group-less VM with affinity whose selector matches no existing VM. | VM is admitted and placed; the policy is configured and applies to future matching VMs. |
| E11 | VMs in one `VirtualMachineGroup` pin to different zones and the feature is disabled. | 9.1.x behavior: `candidateVSphereZone` is not sent; VMs without zone pinning may land in different zones. |
| E12 | A VM with RequiredDuringExecution terms is read and updated through a v1alpha5 client. | Terms are preserved across the v1alpha5 round-trip by the conversion restore. |
| E13 | Zone RequiredDuringExecution on an already-created VM. | Accepted; no behavioral difference from RequiredDuringPlacement because VMs do not move across zones. |
| E14 | Label referenced by a RequiredDuringExecution term exists only on VMs that declare no affinity. | Label is promoted to a tag via the Tag controller (G6), so the policy targets those VMs. |

---

## Success criteria

The combinations of strictness, affinity/anti-affinity, topology, tag sets, and cluster layout are too many for exhaustive E2E coverage, and each E2E scenario is slow. Coverage is therefore layered:

- **SC-001**: Each user story US1–US7 has E2E coverage for its representative combinations on a stretched (multi-zone) Supervisor, verifying admission, placement, vCenter policy compliance, and VM tags (see [e2e.md](./e2e.md)).
- **SC-002**: Unit tests cover every combination of feature state × strictness × affinity/anti-affinity × topology for the generated placement policies, and every webhook admit/deny path.
- **SC-003**: With the capability not activated, the generated placement policies and placement requests are identical to 9.1.x, verifiable by the pre-existing unit and E2E suites passing unchanged.
- **SC-004**: [Todo] vcsim-based integration coverage for the broader combination matrix (cluster layouts, multiple tags per policy, mixed strictness).

---

## Interaction with cluster modules

Cluster modules cannot be deprecated because VKS control plane nodes still use them. VKS imperatively chooses, per VM, either cluster modules or placement policies, and VM Service configures exactly what is specified. VM Service never configures both: a VM that has a cluster module gets no placement policies. VKS configures placement policies only for new NodePools; operations on existing NodePools (scale, rolling update) continue to use cluster modules so a NodePool never has members split between the two mechanisms.

---

## Best practices for mixing placement policies and tags

[Todo]

---

## Open questions

- [NEEDS CLARIFICATION: US8 placement status design.]

---

## Review & acceptance checklist

- [x] Every goal is testable and uses RFC 2119 keywords.
- [x] Non-goals are explicit.
- [x] Every user story has observable acceptance criteria and a priority.
- [x] Every user story US1–US7 has at least two Given/When/Then scenarios.
- [x] Capability-off behavior is specified for every goal.
- [x] Entitlement presence, absence, and revocation are specified.
- [x] Immutability of `spec.affinity` is reflected in admission rules and edge cases.
- [x] VKS node VM behavior is specified.
- [ ] US8 placement status is specified.
- [ ] Best practices for mixing placement policies and tags are written.
