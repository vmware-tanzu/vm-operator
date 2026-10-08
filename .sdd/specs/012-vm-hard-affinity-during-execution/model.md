# Data Model: VM Hard Affinity During Execution

- **Spec**: [spec.md](./spec.md)

No new API fields. This document specifies the admission contract and the vCenter policy mapping for the existing v1alpha6 affinity API.

## API surface

| Field | Type | Notes |
|-------|------|-------|
| `spec.affinity.vmAffinity.requiredDuringSchedulingRequiredDuringExecution` | `[]VMAffinityTerm`, `+optional`, `+listType=atomic` | Previously always forbidden. |
| `spec.affinity.vmAntiAffinity.requiredDuringSchedulingRequiredDuringExecution` | `[]VMAffinityTerm`, `+optional`, `+listType=atomic` | Previously always forbidden. |
| `spec.groupName` | `string`, `+optional` | Required with `spec.affinity` only when the feature is disabled. |

`spec.affinity` is immutable after create. v1alpha5 has no RequiredDuringExecution fields; `restore_v1alpha6_VirtualMachineAffinityRequiredDuringExecution` restores them on round-trip.

`vsphere.policy.vmware.com/v1alpha1` `RequiredDuringExecutionVMPlacementPolicy` (namespaced, `spec.description` optional) is the entitlement. Presence of any one object in the namespace is sufficient.

## Admission rules (create)

| # | Condition | Result |
|---|-----------|--------|
| A1 | RequiredDuringExecution terms set, feature disabled | `Forbidden`: `requiredDuringSchedulingRequiredDuringExecution is not supported` |
| A2 | RequiredDuringExecution terms set, feature enabled, no entitlement in namespace | `Forbidden`: same message |
| A3 | Entitlement lookup fails | `InternalError` |
| A4 | RequiredDuringExecution term topology key not `kubernetes.io/hostname` or `topology.kubernetes.io/zone` | `NotSupported` on `[i].topologyKey` |
| A5 | `spec.affinity` set, `spec.groupName` empty, feature disabled | `Required`: `when setting affinity` |
| A6 | `spec.affinity` set, `spec.groupName` empty, feature enabled | Admitted |

On update, any change to `spec.affinity` is `Forbidden` (`updating Affinity is not allowed`); A1–A6 are not re-evaluated.

## vCenter policy mapping

Feature enabled (`VMHardAffinityDuringExecution`):

| Term list | Topology | Affinity | Anti-affinity | Strictness |
|-----------|----------|----------|---------------|------------|
| requiredDuringSchedulingRequiredDuringExecution | host | `VmVmAffinity` per tag | one `VmToVmGroupsAntiAffinity` | `RequiredDuringPlacementRequiredDuringExecution` |
| requiredDuringSchedulingRequiredDuringExecution | zone | `VmVmAffinity` per tag | one `VmToVmGroupsAntiAffinity` | `RequiredDuringPlacementRequiredDuringExecution` |
| requiredDuringSchedulingPreferredDuringExecution | host | `VmVmAffinity` per tag | one `VmToVmGroupsAntiAffinity` | `RequiredDuringPlacementPreferredDuringExecution` |
| requiredDuringSchedulingPreferredDuringExecution | zone | `VmVmAffinity` per tag | one `VmToVmGroupsAntiAffinity` | `RequiredDuringPlacementPreferredDuringExecution` |
| preferredDuringSchedulingPreferredDuringExecution | host | `VmVmAffinity` per tag | one `VmToVmGroupsAntiAffinity` | `PreferredDuringPlacementPreferredDuringExecution` |
| preferredDuringSchedulingPreferredDuringExecution | zone | `VmVmAffinity` per tag | one `VmToVmGroupsAntiAffinity` | `PreferredDuringPlacementPreferredDuringExecution` |

Feature disabled: RequiredDuringExecution terms are not admitted; host anti-affinity is `VmVmAntiAffinity` per tag; zone anti-affinity is one `VmToVmGroupsAntiAffinity`.

All rules are omitted for VMs that have a cluster module. Zone rules are persisted at create only when the feature is enabled.

## Examples

### US1 — host RequiredDuringExecution anti-affinity

```yaml
apiVersion: vmoperator.vmware.com/v1alpha6
kind: VirtualMachine
metadata:
  name: vnf-a
  namespace: ns1
  labels:
    app: vnf
spec:
  affinity:
    vmAntiAffinity:
      requiredDuringSchedulingRequiredDuringExecution:
      - labelSelector:
          matchLabels:
            app: vnf
        topologyKey: kubernetes.io/hostname
```

Requires a `RequiredDuringExecutionVMPlacementPolicy` in `ns1`. With the feature enabled, `spec.groupName` may be omitted (US6).

### US6 — affinity without a group

```yaml
apiVersion: vmoperator.vmware.com/v1alpha6
kind: VirtualMachine
metadata:
  name: web-2
  namespace: ns1
  labels:
    tier: web
spec:
  affinity:
    vmAffinity:
      requiredDuringSchedulingPreferredDuringExecution:
      - labelSelector:
          matchLabels:
            tier: web
        topologyKey: topology.kubernetes.io/zone
```
