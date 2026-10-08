# Research: VM Hard Affinity During Execution

- **Spec**: [spec.md](./spec.md) · **Plan**: [plan.md](./plan.md)

## govmomi

The required types are not in a govmomi release. `go.mod` pins `github.com/vmware/govmomi v0.57.0-alpha.0.0.20260930024634-9b65cf3964d3` (ToT); the latest release, v0.56, lacks them. Expected in v0.57. Types used:

- `vimtypes.VmToVmGroupsAntiAffinity` (`AntiAffinedVmGroupTags`)
- `vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementRequiredDuringExecution`
- `PlaceVmsXCluster` `VmPlacementSpecs[].CandidateVsphereZone`

## Prior art in this repository

### Capability and features

`pkg/config/capabilities/capabilities.go` maps `CapabilityKeyVMHardAffinityDuringExecution` to both `Features.TaggingAPI` and `Features.VMHardAffinityDuringExecution`. `Features.VMPlacementPolicies` (zone rules) and `Features.VMAffinityDuringExecution` (host rules) are prerequisites that `CalculateAffinityConstraints` (`pkg/providers/vsphere/virtualmachine/configspec.go`) already consults.

### Affinity constraints

`CalculateAffinityConstraints` produces `ConfigureZoneRules` / `ConfigureHostRules`:

- Zone rules off on create (`isCreateVM`) because DRS only evaluated zonal policies at placement. G4 changes this when `VMHardAffinityDuringExecution` is enabled.
- VMs with a cluster module get no placement policies; VKS node VMs without one get placement policies like any other VM.
- Zone rules off for VKS node VMs with a zone label, and for zone-labeled VMs when `VMAffinityDuringExecution` is on but `VMHardAffinityDuringExecution` is off.

### Policy generation

`pkg/providers/vsphere/virtualmachine/affinity.go`: `processVMAffinity` emits `VmVmAffinity` per tag; `processVMAntiAffinity` emits a grouped `VmToVmGroupsAntiAffinity` for zones and, before PR 2012, `VmVmAntiAffinity` per tag for hosts.

### Admission

`webhooks/virtualmachine/validation/virtualmachine_validator.go`:

- `validateVMAffinity` runs on create only. Before PR 2012 it always forbade `requiredDuringSchedulingRequiredDuringExecution` and always required `spec.groupName`.
- `validateImmutableVMAffinity` forbids any update to `spec.affinity` (`updating Affinity is not allowed`). This is why the entitlement only needs to be evaluated at create.

### Placement

`pkg/providers/vsphere/vmprovider_vm.go` places via the group (`vmCreateDoPlacementByGroup`) only when `Features.VMGroups` is on and `spec.groupName != ""`. Group-less VMs (G5) use the regular placement path.

### PR 2000 — candidate zones

`placement/cluster_placement.go` sets `CandidateVsphereZone` per VM; `placement/group_placement.go` uses per-VM zones and fails the whole group if any VM's zone is unplaceable. Both gated on `VMHardAffinityDuringExecution`.

### PR 2012 — RequiredDuringExecution and host grouping

Branch `aniket-deole:aniketd/configure-required-host-affinity`:

- Includes RequiredDuringExecution terms in tag extraction and policy generation (host only).
- `addAntiAffinityPolicies(grouped)` with `grouped = VMHardAffinityDuringExecution` for host anti-affinity.
- Webhook `validateRequiredDuringExecutionTerms` + `hasRequiredDuringExecutionPolicy` (`List`, `Limit(1)`); host key only.
- RBAC `get;list;watch` on `requiredduringexecutionvmplacementpolicies`.
- E2E in `vm_group.go`: rejection without entitlement; required-required host AF / AAF with compliance checks via `WCPClient.GetVMPolicyCompliance`. The E2E provisions the entitlement by assigning an InfraPolicy of kind `infrapolicy.PolicyKindRequiredDuringExecutionVMPlacementPolicy` to the test namespace.

### E2E prior art

`test/e2e/vmservice/vmservice/virtualmachine/vm_group.go` contexts "Group placement with affinity and anti-affinity" (zonal, stretched Supervisor) and "...at host topology" (`SkipUnlessSupervisorHasAtleastOneZoneWithHostCount`) verify placement, policy compliance, and tags. The `computepolicies` suite (`vmevictionpolicy.go`, `controlledrebalancingpolicy.go`) shows infra/compute policy E2E patterns.
