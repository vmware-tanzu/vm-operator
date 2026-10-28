# Research: Bring Your Own Network Interface CR

- **Epic**: vmop-2530
- **Investigated**: 2026-10-28

## Ticket evidence

Read the epic and its five children through the configured Jira CLI. The epic is a proposed epic; the children are untriaged stories. This planning change does not update tickets or their acceptance criteria.

| Ticket | Requirement |
|---|---|
| vmop-2530 | Reference a pre-created interface CR from the VM interface spec; VM Operator does not own its lifecycle or add a VM owner reference. |
| vmop-2531 | Add the reference field to `VirtualMachineNetworkInterfaceSpec`. |
| vmop-2532 | Suppress default-network mutation; prohibit network plus external reference; validate the group/kind against the environment. IP/MAC exclusions are suggested, not settled. |
| vmop-2533 | Resolve the supplied CR instead of using `CreateOrPatch`; preserve it on interface removal. VPC attachment metadata is called out as a potential exception needing investigation. |
| vmop-2585 | Create two external CRs, create a VM with those plus one managed interface, verify configuration/customization, remove one external interface, delete the VM, and verify both external CRs survive. |
| vmop-3636 | Watch referenced CRs without owner mapping, following the existing PVC reference-watch precedent. |

The epic also blocks the image-fetcher task vmop-3609 and is a dependency of vmop-4223. The latter points to WIKI page 3003301695, "PodVM v2 Image Service Requirements". That wiki's contents were not read and are not treated as acceptance criteria here. Direct boot is a separate dependency, not part of this plan.

## Repository findings

| Evidence | Consequence for the plan |
|---|---|
| `api/v1alpha6/common/types.go` offers `LocalObjectRef` and `PartialObjectRef`. | Prefer the complete local reference: external kind/version/name must be explicit. |
| `pkg/config/capabilities/capabilities.go` maps Supervisor capability activation to `FeatureStates`; `pkg/crd/crd.go` removes disabled VM schema fields and dependent CEL. | Add a dedicated BYO capability and follow the existing field/CEL removal path. Test absent capability values as well as explicit deactivation. |
| `pkg/crd/crd.go:updateOrDeleteUnstructured` always removes disabled fields on first installation, but uses `CRDCleanupEnabled` for updates. | Document both installed-schema outcomes and retain runtime guards when cleanup is disabled; do not claim schema removal preserves existing references. |
| `network.go` dispatches through `getNetworkInterfaceAPIGroup`, which currently reads only `interfaceSpec.Network`. | Extend provider selection before calling any create/patch helper. |
| `CreateNetworkDevices` and `CreateAndWaitForNetworkInterfaces` have separate entry paths. | Both must use the external CR; create-only support would regress on the next VM update. |
| Managed provider helpers call `SetNetworkInterfaceOwnerRef` and stamp VM/interface labels in their `CreateOrPatch` callbacks. | BYO must bypass these callbacks entirely; disabling only creation still adopts/mutates existing CRs. |
| `createVPCNetworkInterface` sets `VMNameLabel`, `VMInterfaceNameLabel`, and `pkgconst.VPCAttachmentRef`, and rewrites `AddressBindings`. | Reuse the labels and attachment annotation for BYO through a metadata-only patch; never reuse the full adopt/configure callback. |
| `ListOrphanedNetworkInterfaces` filters by provider/name and `isOwnedBy`, which checks the VM UID. | Preserve that deletion discriminator. A separate label-based list of SubnetPorts with this VM's `vm-name` label discovers BYO metadata-release candidates after spec removal; no deleted spec entry or transient BYO bit is needed. |
| `session_vm_update.go:reconcileNetworkAndGuestCustomizationState` deletes returned orphan candidates. | Audit the producer/consumer boundary; never supply a list of all referenced CRs as deletion candidates. |
| `reconcilePoweredOffOrPoweredOnVM` returns before cleanup on reconfigure errors/sentinels; some hardware paths do not emit NIC changes. | BYO release runs before network reconciliation, using the ethernet cards fetched at the start of the reconcile: a SubnetPort is released only if unreferenced and its attachment ID is empty or matches no current NIC, so a NIC not yet removed is released on a later reconcile. |
| `DeleteVirtualMachine` returns early when the VC VM is absent; the controller removes its finalizer after provider success. | Perform association release on both successful destruction and already-absent paths, propagating patch errors so finalization waits for cleanup. |
| `devices.go` uses generated names and dereferences `interfaceSpec.Network` during mutable-network status matching. | Add exact external lookup and nil-safe dispatch beyond the creation path. |
| `BuildBootstraps` uses `Device.InterfaceObj` and spec/device slice alignment. | Keep the existing typed `Device` + `Bootstrap` model; do not introduce a replacement aggregate. |
| `getNetOPNetworkInterfaceDevice` documents no requested-MAC spec field on NetOP. | A blanket instruction to put MAC requests into every provider CR would be inaccurate. Per the user's later direction, `macAddr` cannot be combined with `externalRef`. |
| `AddDefaultNetworkInterface` and `SetDefaultNetworkOnUpdate` resolve the default before iterating entries. | The default-network lookup still always happens; only the assignment is skipped for interfaces with `externalRef`. |
| Existing network watches use `EnqueueRequestForOwner`. | Add a VM reference index and event mapping while retaining owner handling. |
| PVC precedent: `pkg/util/kube/pvc.go` plus `PVCToVirtualMachineVolumeClaimNameMapper`. | Reuse indexed namespace-scoped lookup, but place the new index/key/mapper together to avoid copying the existing import-cycle workaround. |
| v1alpha2–v1alpha5 each have `restore_v1alpha6_VirtualMachineNetworkInterfaces`. | All restore `externalRef` by interface name. v1alpha1's whole-list restore already carries `externalRef` and is unchanged. |
| Spec 008 defines unit-number identity; portions of this checkout still use older mapping paths. | BYO does nothing special with `unitNumber`; that work is under separate development. |
| An unindexed `.sdd/specs/004-remove-network-interface-result/plan.md` describes the `Device`/`Bootstrap` refactor. | Account for the refactor already reflected in current types; leave unrelated index cleanup outside this change. |

## Proposed decisions and alternatives

- **Use `externalRef`, not an overloaded `network`.** The latter identifies a network/subnet from which VM Operator creates an interface, so overloading it obscures lifecycle ownership and defaulting.
- **Resolve dependencies without adopting them.** A special `CreateOrPatch` mode would be harder to audit than an explicit typed lookup before the managed branch.
- **Keep deletion eligibility based on actual VM ownership.** Current filtering already survives removal from spec. A volatile `Device.IsExternal` flag alone would not protect restart/removal cleanup.
- **Use indexed watches for desired external references.** They work before VM Operator writes any association metadata and apply to all providers. Also map SubnetPort events to the VM named by the `vm-name` label so release is triggered after spec removal.
- **VM Operator manages VPC association metadata; Q1 resolved by the user on 2026-10-28.** Set the VM labels and NSX attachment annotation on BYO attach, and clear them once the SubnetPort is unreferenced and not attached to a current NIC, or the VM is deleted. Persist the association on the SubnetPort using the existing `vm-name`/`vm-interface-name` labels and `attachment_ref` annotation; no VM UID label or attachment-ID annotation is used. This replaces the earlier producer-owned proposal and avoids a new public VM status journal. The CR remains externally owned throughout.
- **Same-VM reassociation is supported; resolved by the user on 2026-10-28.** Moving a reference from `eth0` to `eth1` preserves the CR and updates the interface-name label and annotation. A blanket rejection of changed attachment metadata would conflict with keeping still-desired resources out of release cleanup. A SubnetPort labeled with a different VM name, or whose attachment annotation refers to a different VM, is rejected. NIC changes go through the normal device reconcile with existing power-state restrictions; hot-plug is separate work.
- **Cross-VM attachment coordination remains external.** The user clarified that VM Operator cannot provide an atomic cross-VM reservation for this feature. Keep simultaneous attachment unsupported and retain provider-specific association conflict checks without introducing a claim service.
- **Capability-backed schema exposure; resolved by the user on 2026-10-28.** Use `supports_VM_service_byo_network_interface` to drive `FeatureStates.BringYourOwnNetworkInterface`, and remove `externalRef` plus dependent CEL from the installed VM CRD under the existing cleanup policy when disabled. This replaces the open FSS-versus-capability choice and the earlier promise to preserve references across disablement. Drain BYO workloads before disabling/removing the schema; retained schemas still require admission and provider gates.
- **Keep a complete VM-side field policy; Q2 resolved.** IP, DHCP, gateways, and IP-family choices can change effective configuration even when no external spec is patched; `macAddr` is excluded with `externalRef`. Guest naming/DNS/routes and VM hardware settings serve different purposes and can keep their existing rules.
- **No automatic adoption of managed interfaces into BYO.** Removing an owner reference to rescue an existing managed CR changes ownership and is outside the epic's pre-created-resource contract.

## Remaining evidence needed

Q1's ownership decision and same-VM rename are settled; real VPC tests still need to demonstrate metadata establishment, NSX behavior after an attachment-annotation rename, detach-before-release ordering, and preservation of the external CR. The current E2E covers only a single add/remove/delete flow; rename, swap, restart, dependency-recovery, and the full vmop-2585 three-interface workflow are not covered. Capability-backed activation and schema removal are settled; implementation tests must cover field/CEL removal and both CRD cleanup settings. Q2 is resolved, with `macAddr` excluded. Q3 requires confirmation of active API release status only. No runtime experiment, API generation, or product test was performed as part of this documentation-only planning change.

## Implementation decisions and local validation — 2026-10-28

The user confirmed v1alpha6 remains in development and approved adding `externalRef` there. The user also directed that interface-spec overrides remain allowed. The implementation therefore preserves existing device/bootstrap overrides, including explicit false DHCP pointers, without modifying externally owned resource specifications. IP-family requests do not allocate addresses on externally managed CRs.

The implementation was subsequently simplified at the user's direction: no VM UID label, no attachment-ID annotation, no NetOP external ID derived from the CR UID, no pre-create or failed-create release, no special reassociation-detach step, no BYO-specific unit-number handling, `macAddr` excluded with `externalRef`, and v1alpha1 conversion unchanged. Earlier review findings and local results tied to the removed behavior no longer apply.

Local environment: macOS arm64, Go 1.27.3, envtest Kubernetes 1.34.1. The results below were recorded before the simplification and have not been revalidated against the simplified implementation. The full `pkg/util/kube` and VM controller suites passed, including actual controller reconciliation from unowned interface creation, status changes, and deletion.

`make generate-manifests`, conversion generation, and `make generate-api-docs` succeeded. API documentation regeneration exposed unrelated stale output; only the new reference row was retained. Deepcopy generation completed, but the encompassing `make generate-go` and `make verify-codegen` targets cannot complete because quicktype's npm dependency installation receives HTTP 403 from the configured registry. `make lint-go` passed root/API checks then failed loading the E2E module because its checked-in dependency metadata needs updates. Focused lint on changed root packages reported zero issues.

No real Supervisor or NSX hardware validation was performed. The single new E2E scenario remains `experimental`; AC12 and T017 remain incomplete, and no provider is certified for release enablement by these local results.

Final local checks: the full mutation and validation webhook suites passed; capability tests passed; the full CRD suite ran 310 cases with one failure caused by an erroneously added spec-only field in a status expectation table. That test-table entry was removed and the corrected BYO schema/cleanup matrix was rerun. The full controller and mapper suites passed. The complete network suite and the session network-device test passed. Session/provider packages compile. `make lint-go` passed after adding the two indirect E2E dependencies required by its networking helper imports.

The corrected BYO CRD matrix passed (`go test ./pkg/crd -ginkgo.focus='external interface|BYO'`). Normal E2E compile with vet also passed (`cd test/e2e && go test ./vmservice -run '^$'`); the earlier generated-import vet issue was eliminated by removing unrelated generator churn. These commands compiled E2E tests but did not run them against infrastructure.
