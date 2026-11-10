# Feature Specification: Bring Your Own Network Interface CR

- **Feature branch**: `bryanv/byo-network-interface`
- **Fork**: `bryanv/vm-operator`
- **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-10-28
- **Status**: In Progress
- **Epic**: vmop-2530
- **Stories**: vmop-2531, vmop-2532, vmop-2533, vmop-2585, vmop-3636

## Summary

A user or another controller can create a network interface resource and reference it from a VM. VM Operator uses the referenced interface to configure the VM while the creator retains responsibility for the interface's lifecycle. This supports the image-fetcher consumer tracked by vmop-3609 and vmop-4223.

The epic requires an external reference, no VM owner reference, and no lifecycle management of the supplied resource. Its stories also require admission changes, watches independent of ownership, and a mixed-interface lifecycle test. The user clarified that VM Operator must set the VM association metadata required by NSX on BYO VPC interfaces and remove it when detached. Managing this association does not confer ownership of the resource. Remaining proposed design decisions and unresolved contracts are identified below.

## Goals

- **G1 (MUST)**: A VM can use an existing interface resource in its own namespace. One VM can mix externally supplied and automatically created interfaces.
- **G2 (MUST)**: VM Operator neither creates a replacement for a missing external resource nor adds a VM owner reference to it. It does not rewrite the resource's network or addressing configuration.
- **G3 (MUST)**: Removing an external interface from a VM or deleting the VM does not delete that resource. Existing automatically created interfaces retain their current lifecycle behavior.
- **G4 (MUST)**: Default-network selection does not replace or supplement an explicit external reference. Invalid combinations and unsupported resource types are rejected with a field-specific admission error.
- **G5 (MUST)**: Changes to referenced interface resources trigger VM reconciliation even without a VM owner reference. Missing or unready resources produce actionable VM diagnostics and recover when the dependency becomes usable.
- **G6 (MUST)**: Device backing, identity, guest customization, and network status use the selected resource consistently. Existing VM power-state restrictions on device changes continue to apply.
- **G7 (MUST)**: Older API clients cannot silently convert an externally supplied interface into an automatically managed interface during an update.
- **G8 (MUST)**: A dedicated Supervisor capability enables the feature, which is disabled by default. When the capability is disabled, the external-reference field is removed from the installed VM API schema under the existing CRD cleanup policy. Existing VMs using automatically created interfaces keep their current behavior.
- **G9 (MUST)**: VM Operator sets the VM association metadata required by NSX on BYO VPC interfaces. After an interface is removed from the VM spec and its NIC is no longer attached to the VM, or the VM is deleted, VM Operator removes that association metadata while preserving the interface resource. Release is level-triggered from the persisted `vm-name` label, so it is retried on later reconciles.
- **G10 (MUST)**: With mutable networks enabled, a user can move a BYO resource between interface names within the same VM. VM Operator preserves the external resource and, for VPC, updates the interface-name label and attachment annotation without requiring an intermediate remove-and-add request. Any NIC change goes through the normal mutable-network device reconcile. Existing power-state and guest-customization restrictions continue to apply.

## Scope and non-goals

The proposed scope covers the three existing resource-backed network providers, subject to the providers enabled for the VM's namespace. References are local to that namespace and explicitly identify a supported resource type and name. Reusing one resource for two interfaces in the same desired VM spec is invalid; moving a reference between names within one VM is supported. Simultaneous attachment to multiple VMs is not a supported use case, and this feature does not provide an atomic reservation across VMs. The caller or external resource manager coordinates exclusive use; provider-specific association conflict checks remain in place.

This feature does not add cross-namespace references, arbitrary third-party network providers, a claim/allocation service, conversion of an already VM-owned interface into an external one, or new support for changing NICs while a VM is powered on. It does not implement direct boot or change the image-fetcher controller. The external owner remains responsible for retaining and eventually deleting its resource.

## User stories and acceptance criteria

### US1 — Consume an existing interface

- **AC1**: Given two external interfaces, when a user creates a VM with those interfaces and one automatically managed interface, VM Operator establishes the required VPC association metadata, waits for provider readiness, and configures the expected three NICs, provider identities, and guest network configuration. The external resources retain their UIDs, specifications, and owner references.
- **AC2**: Given a valid external reference, creation and later unrelated VM updates leave its automatically selected network unset. An interface without either selection continues to receive the existing default.
- **AC3**: Admission rejects a reference combined with an explicit network, a reference combined with `macAddr`, an empty or unsupported reference, a provider unavailable in the namespace, and a duplicate reference within the VM, while retaining the other existing interface-spec overrides and their validation. Guest naming and supported hardware settings remain available.

### US2 — Keep lifecycle ownership outside VM Operator

- **AC4**: With mutable networks enabled and a supported powered-off device-change path, removing one of the two external interfaces removes the corresponding VM NIC while preserving both external resources and the remaining NIC identities. For VPC, VM Operator removes its VM association labels and attachment annotation once the SubnetPort is no longer referenced by the spec and its attachment ID is empty or matches none of the VM's current NICs; while the NIC is still attached, they are retained. Unrelated metadata remains intact.
- **AC5**: After normal VM deletion destroys the underlying VM, both external resources still exist with their original UIDs and owner references, and VM Operator's VPC association metadata is removed. The automatically managed resource follows its ordinary cleanup behavior. Missing external resources do not prevent VM deletion; a metadata-cleanup failure is returned so finalization is retried.
- **AC6**: Repeated reconciliation neither adopts an external resource nor rewrites unchanged metadata. Because release is discovered from the persisted `vm-name` label, a later reconcile still completes cleanup without deleting the resource. A resource owned by a different VirtualMachine is rejected as an external dependency rather than silently adopted or stripped of ownership. A SubnetPort labeled with a different VM name, or with an attachment annotation for a different VM, is rejected as a conflict and not overwritten; a different interface name on the same VM is reconciled under AC13.

### US3 — Recover from dependency changes

- **AC7**: A syntactically valid reference to a missing, unready, or terminating resource does not cause VM Operator to create that resource or attach an incomplete NIC. The VM reports the dependency and recovers after the external owner makes it usable, without a VM spec edit.
- **AC8**: Creation, status-only updates, and deletion of an unowned referenced resource enqueue the correct VM. Resources with the same name in another namespace or of another kind do not enqueue unrelated VMs. Automatically managed interface events still enqueue their owning VMs.
- **AC9**: An external resource disappearing from a running VM produces a dependency failure without deleting other interfaces or replacing the selection with the default network. VM deletion can still finish when that resource is absent.

### US4 — Preserve compatibility and rollout safety

- **AC10**: A read-modify-write through v1alpha2–v1alpha5 preserves the external reference and the absence of a managed-network selection; references are restored by interface name from the conversion annotation. Reordering or removing interfaces does not restore a reference onto the wrong interface. Conversion never fails; an older-version edit that produces an invalid combination (for example, setting `network` on an external interface, or dropping the reference from an existing interface) is rejected by the validation webhook rather than silently changing lifecycle ownership. v1alpha1 conversion is unchanged; BYO round-tripping through v1alpha1 is not attempted beyond its existing whole-list restore.
- **AC11**: With the capability disabled, the installed VM API schema omits the external-reference field and validations that depend on it, subject to the existing CRD cleanup policy. With it enabled, the field is available and BYO admission, reconciliation, and watches are enabled independently of mutable networking and NIC unit numbers. Through a retained schema, a disabled runtime gate rejects new or changed external selections while allowing unchanged stored references on unrelated updates/deletion; it does not attach or adopt them. Ordinary managed networking remains unchanged. Active BYO workloads must be drained before disabling the capability or removing the schema field; preservation of references across that removal is not promised.
- **AC12**: The mixed-interface lifecycle scenario from vmop-2585 runs on real Supervisor infrastructure for each supported provider before that provider's feature support is enabled for release. Provider-specific attachment requirements are documented and validated.

### US5 — Move an interface within the same VM

- **AC13**: Given a powered-off VM with mutable networks enabled, changing an interface name from `eth0` to `eth1` while retaining its external reference preserves the external resource's UID, specification, owner references, and unrelated metadata. For VPC, the interface-name label and attachment annotation change to the new interface name while the `vm-name` label is unchanged. NIC changes, if any, follow the normal mutable-network device reconcile and its power-state restrictions. Later reconciles converge toward the latest desired mapping without requiring the user to remove and re-add the reference.

## Resolved decisions

- **Q1 — Resolved by the user, 2026-10-28**: VM Operator owns the VPC VM association metadata even for BYO interfaces. It must set the metadata for NSX and remove it after the interface is detached from the VM. The interface CR remains externally owned and must not be deleted. The implementation uses the existing `vm-name` and `vm-interface-name` labels plus the `nsx.vmware.com/attachment_ref` annotation; the `vm-name` label is used for discovery after spec removal. No VM owner reference is added.
- **Same-VM reassociation — Resolved by the user, 2026-10-28**: Moving a BYO resource from one interface name to another on the same VM is supported. Preserve the resource and update its interface-name association; NIC changes follow the normal device reconcile. Hot-plug remains separate work.
- **Capability gating — Resolved by the user, 2026-10-28**: Use a dedicated Supervisor capability and remove the external-reference field from the installed API schema when that capability is disabled, following the repository's CRD cleanup policy. Q3 now concerns only the target API version's release eligibility.

## Resolved API and override decisions

- **Q2 — Resolved by the user, 2026-10-28**: Continue allowing interface-spec overrides. Preserve existing guest customization and hardware override semantics, including explicit false DHCP pointers, addresses, gateways, and IP-family settings. Per the user's later direction, `macAddr` is excluded: it cannot be set together with `externalRef` (CEL rule and validation webhook), and no MAC override is applied to external interfaces. External resource specifications remain creator-owned: VM Operator applies overrides to its device/bootstrap pipeline and does not rewrite the external CR's allocation settings. An IP-family request does not allocate addresses on an external resource; its owner supplies the allocation.
- **Q3 — Resolved by the user, 2026-10-28**: v1alpha6 is still in development and is eligible for the new field. Add the field there and restore it by interface name through the v1alpha2–v1alpha5 conversions; v1alpha1 conversion is unchanged.

There are no remaining blocking API contract questions. Real-provider validation remains required before release enablement.
