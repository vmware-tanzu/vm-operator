# Data Model: Bring Your Own Network Interface CR

- **Epic**: vmop-2530
- **Status**: In Progress; API version and override contracts resolved by the user

## Reference schema

Propose the following field on `VirtualMachineNetworkInterfaceSpec` in v1alpha6:

```go
// +optional
// ExternalRef identifies an existing network interface in this VM's
// namespace. VM Operator does not manage its lifecycle.
ExternalRef *vmopv1common.LocalObjectRef `json:"externalRef,omitempty"`
```

| Field | Contract |
|---|---|
| `name` | Existing required VM interface name, distinct from the referenced CR name and hardware slot. With mutable networks enabled, changing this name can retain the same external resource; NIC changes follow the normal device reconcile. |
| `externalRef` | Optional; mutually exclusive with `network` and `macAddr`. No default. Exposed in the installed schema only when the BYO capability is enabled, subject to the existing CRD cleanup policy. |
| `externalRef.apiVersion` | Required, nonempty supported version for the selected provider kind. |
| `externalRef.kind` | Required supported interface CR kind, not the network/subnet kind. |
| `externalRef.name` | Required valid resource name in the VM's namespace. |
| Namespace / UID | No public fields added. Namespace is implicit. A name reference is not an immutable UID claim. |

Use the existing `common.LocalObjectRef`; do not weaken that shared type's validation for unrelated consumers. Add field-specific CEL/Go validation at the containing interface as needed. Within a VM, duplicate detection compares `(apiVersion, kind, name)`; the watch index keys on `(API group, kind, name)`. Each supported kind has a single allowed `apiVersion`.

## Supported kinds

| Provider | `apiVersion` | `kind` |
|---|---|---|
| VDS / NetOP | `netoperator.vmware.com/v1alpha1` | `NetworkInterface` |
| NSX-T / NCP | `vmware.com/v1alpha1` | `VirtualNetworkInterface` |
| VPC / NSX Operator | `crd.nsx.vmware.com/v1alpha1` | `SubnetPort` |

The namespace-supported provider set further restricts this table. Named-network mode and arbitrary GVKs are unsupported. If dependencies add a served version, extend the allowlist intentionally rather than accepting any version string. Read the resource through its typed client and reuse existing provider status interpretation.

## Field combinations and updates

| Combination | Proposed behavior |
|---|---|
| Neither `network` nor `externalRef` | Existing default-network behavior. |
| `network` only | Existing managed-interface behavior. |
| `externalRef` only | Resolve an externally managed interface. |
| Both selections | Reject. |
| Same external resource twice on one VM | Reject. |
| External resource owned by a different VM | Reconciliation rejects; do not remove ownership or promise preservation of a resource another VM owns. |
| External resource owned by this VM | Accept and keep the owner reference; the resource is deleted with the VM. |
| External resource owned by another controller | Preserve that owner; attachment must satisfy the provider contract. |
| `macAddr` together with `externalRef` | Reject (CEL rule and validation webhook). |
| VPC SubnetPort labeled with a different VM name, or with an attachment annotation for a different VM | Reconciliation rejects as a conflict; metadata is not overwritten. |
| Managed ↔ external for the same interface name | Allow, subject to the mutable-network rules. A resource this VM owns keeps its owner reference. |
| Same external resource moved to another interface name on the same VM | Preserve the resource; for VPC, update the interface-name label and attachment annotation. NIC changes follow the normal mutable-network device reconcile; no intermediate remove-and-add request is required. |
| External reference changed to another external resource | Subject to existing mutable-network and power-state rules; preserve the old external CR. |
| Missing or recreated resource at the same name | No automatic CR creation. Re-evaluate actual provider readiness/attachment; do not reuse stale data from the deleted UID. |

Continue allowing `addresses`, `dhcp4`, `dhcp6`, `gateway4`, `gateway6`, and `ipamModes` under existing validation and override semantics. Explicit false DHCP pointers override provider defaults. Apply device/bootstrap overrides without mutating the external CR specification. `macAddr` cannot be set with `externalRef`, and no MAC override is applied to external interfaces. IP-family requests do not trigger allocation on an external CR; the external owner remains responsible for its allocation.

Retain `guestDeviceName`, `mtu`, `nameservers`, `searchDomains`, `routes`, `type`, `unitNumber`, `vnumaNodeID`, `vmxnet3`, and `advancedProperties` under their current bootstrap/hardware validation and feature gates; BYO adds no special `unitNumber` handling. Allowing a field does not add hardware functionality that the current provider does not implement.

## Ownership and attachment

Do not add VM owner references or finalizers, or write spec/status on an external interface. Under the resolved Q1 contract, VM Operator manages the following association metadata on BYO VPC SubnetPorts, preserving unrelated creator-owned metadata:

| Location and key | Attached value | Purpose |
|---|---|---|
| Label `vmoperator.vmware.com/vm-name` (`VMNameLabel`) | VM name | Existing association convention; used to discover release candidates and to map SubnetPort events to the VM. |
| Label `vmoperator.vmware.com/vm-interface-name` (`VMInterfaceNameLabel`) | VM spec interface name | Identifies the attachment within the VM. |
| Annotation `nsx.vmware.com/attachment_ref` (`VPCAttachmentRef`) | `virtualmachine/<vm-name>/<interface-name>` | Existing NSX attachment metadata. |

On attachment, set these fields in one merge patch before waiting for provider readiness; an unchanged association causes no write. An association with the same VM name but a different interface name is a same-VM rename: update the interface-name label and attachment annotation; the `vm-name` label is unchanged. Reject a `vm-name` label for a different VM or an attachment annotation for a different VM. No managed create/adopt callback is used. VDS/NCP BYO resources do not receive these VPC association markers; this feature does not add atomic cross-VM reservation.

If the resource leaves the VM's desired references entirely, the fields persist while its `Status.Attachment.ID` matches a current NIC's `ExternalId`. On each update reconcile, before network reconciliation, unowned SubnetPorts labeled with the VM name that are not referenced by the spec and whose attachment ID is empty or matches no current NIC are released. A resource still referenced under another interface name is excluded from release. On VM deletion (VC VM already absent or after destroy), all such SubnetPorts are released. Release removes the three fields in one merge patch, preserving the CR. Not-found resources are skipped; other patch failures are returned for retry. A VM-owned resource follows owned-resource cleanup instead. Association labels alone never authorize deletion.

No new public status field or condition is proposed. Preserve the existing interface status and VM condition/error reporting. The referenced CR supplies device backing, MAC/external identity where available, and provider addressing data. No NIC external ID is derived from the CR UID. Diagnostics include the VM interface name and referenced object identity. Do not set success before readiness, and do not use a stored VM condition as a reconciliation gate.

## Capability and schema exposure

Use the dedicated Supervisor capability `supports_vm_service_byo_network_interface`, mapped to `FeatureStates.BringYourOwnNetworkInterface`, default `false`. Keep it independent of `MutableNetworks` and `VMNetworkUnitNumbers`. Generated CRD bases contain the field; installation removes `spec.network.interfaces[].externalRef` and containing CEL rules that reference it when the capability is disabled. Follow the existing `CRDCleanupEnabled` policy for updates to installed CRDs; first installation always removes disabled fields. Retained schemas still require runtime admission and provider guards.

When the field is absent from the installed schema, ordinary API-server unknown-field handling applies. Strict requests can be rejected; a pruned reference is absent and ordinary network defaulting can apply. A capability-disabled webhook cannot promise a BYO-specific error for a value already pruned by the API server. Users must check capability availability before submitting BYO manifests and drain active BYO workloads before disabling the capability or removing the schema field. This replaces the earlier promise to preserve stored references across capability disablement; no mutation helper proactively clears references from existing VM objects.

## Conversion contract

Older versions do not expose `externalRef`. v1alpha2–v1alpha5 restore it by surviving interface name from the existing conversion annotation. v1alpha1 conversion is unchanged; its existing whole-list restore already carries `externalRef`, and BYO-specific v1alpha1 conversion is not attempted. Removed interfaces remain removed. Conversion never fails: a legacy edit that sets `network` on an external interface or loses the reference from an existing interface is rejected by the validation webhook against the old VM instead of falling through to managed creation.

## Examples

These are network fragments for the proposed v1alpha6 shape, not complete VM manifests. The target CRs already exist and are configured by their external owner. The user confirmed v1alpha6 is still in development.

Mixed NetOP interfaces, matching the vmop-2585 lifecycle scenario:

```yaml
spec:
  network:
    interfaces:
    - name: eth0
      externalRef:
        apiVersion: netoperator.vmware.com/v1alpha1
        kind: NetworkInterface
        name: image-fetcher-primary
    - name: eth1
      externalRef:
        apiVersion: netoperator.vmware.com/v1alpha1
        kind: NetworkInterface
        name: image-fetcher-secondary
    - name: eth2
      network:
        apiVersion: netoperator.vmware.com/v1alpha1
        kind: Network
        name: workload-network
```

VPC reference; VM Operator sets the association metadata described above:

```yaml
spec:
  network:
    interfaces:
    - name: eth0
      externalRef:
        apiVersion: crd.nsx.vmware.com/v1alpha1
        kind: SubnetPort
        name: image-fetcher-port
```

For NCP, the corresponding reference uses `vmware.com/v1alpha1`, kind `VirtualNetworkInterface`, and the existing interface resource's name. Detach removes the selected list entry under the existing mutable-network rules; the externally owned object remains present.

For a name-only change on the same VM, keep the external reference:

| Before | After | Expected result |
|---|---|---|
| `name: eth0`, `externalRef.name: image-fetcher-port` | `name: eth1`, same reference | Same CR; VPC interface label and attachment annotation use `eth1`. NIC changes, if any, follow the normal mutable-network device reconcile. |

`unitNumber` retains its existing behavior and is not required to enable BYO. Guest device naming follows `guestDeviceName` and the existing bootstrap behavior rather than promising an immediate rename inside a running guest.
