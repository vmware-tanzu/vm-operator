# Implementation Plan: Bring Your Own Network Interface CR

- **Spec**: [spec.md](./spec.md)
- **Epic**: vmop-2530
- **Date**: 2026-10-28
- **Status**: In Progress; Q1–Q3 resolved by the user

## Summary

Add an optional `spec.network.interfaces[].externalRef` identifying a pre-created interface CR, exposed through a dedicated Supervisor capability. Resolve it through typed reads, reuse the existing `Device` and `Bootstrap` pipeline, and enqueue referencing VMs through a field index. Preserve external ownership throughout create, update, same-VM rename, detach, deletion, and API conversion. For VPC, VM Operator sets association metadata, updates the interface name when the same resource moves between interface names on one VM, and clears it once the SubnetPort is unreferenced and detached or the VM is deleted; the VM owner reference remains the discriminator for CR deletion.

## Technical context

| Area | Current checkout |
|---|---|
| Go directives | Root and E2E: 1.26.10; `api`: 1.23.0; `api/test`: 1.24.13. No language-version change planned. |
| Primary libraries | Root uses controller-runtime 0.25.2, Kubernetes APIs 0.37.1, and the pinned govmomi, NetOP, and NSX API dependencies. |
| Active VM API | `v1alpha6`; older versions restore hub-only fields explicitly. The user confirmed it remains in development. |
| Modules affected | Root, `api`, `api/test`, and `test/e2e`. No new external dependency or vendored API edit expected. |
| Existing pipeline | `CreateNetworkDevices` for create; `CreateAndWaitForNetworkInterfaces` for session updates; shared provider CR/device extraction and `BuildBootstraps`. |
| Related work | [NIC unit numbers](../008-nic-unit-numbers/spec.md) and its [plan](../008-nic-unit-numbers/plan.md); [Device/Bootstrap refactor](../004-remove-network-interface-result/plan.md). Preserve their contracts without assuming every planned change has landed. |

## Constitution check

| Rule | Planned compliance |
|---|---|
| API compatibility | The user confirmed v1alpha6 remains in development. Restore references through the v1alpha2–v1alpha5 conversions; v1alpha1 is unchanged. Invalid combinations from older clients are rejected by the validation webhook. |
| Generated artifacts | Regenerate deepcopy, conversions, CRD manifests, and API documentation with the API change. |
| Thin controllers and vSphere boundary | Controllers register indexes and watches only. Reference and mapping helpers live in `pkg/`; all vSphere calls remain under `pkg/providers/vsphere/`. Preserve operation-ID context. |
| Reconcile order | Keep the existing 11-step VM update sequence, deferred patching, context joining, status, and finalizer behavior. |
| Conditions | Communicate VM dependency failures through existing condition/error paths. Do not use VM conditions to skip work; provider CR readiness remains an input to device construction. |
| Ownership | External dependencies are not fan-out children. Do not call `SetNetworkInterfaceOwnerRef` or the managed `CreateOrPatch` callback on them. No exception to child ownership rules is needed. |
| Feature flags | Map a dedicated Supervisor capability to `pkgcfg.FromContext(ctx).Features.BringYourOwnNetworkInterface`. Remove the field and dependent CEL from the installed CRD when disabled, under the existing cleanup policy. Gate admission, external resolution and attachment, and reference watches. Runtime guards protect retained schemas; association release is not gated, so it remains available during VM deletion. |
| Testing | External test packages and Ginkgo labels; no new unit/integration filename split or extra suite bootstrap. E2E ships with the first usable behavior. |
| SDD and references | Include spec, plan, tasks, research, model; map work to existing epic stories; use masked ticket references only. |
| Go conventions | New files carry the required copyright header; exported helpers have godoc and errors include interface/reference context. |

No constitutional exception is proposed.

## Project structure

| Path | Change |
|---|---|
| `api/v1alpha6/virtualmachine_network_types.go` | Optional reference and structural validation; v1alpha6 is approved. |
| `api/v1alpha2` through `api/v1alpha5`, `api/test` | Restore the reference by interface name through conversions. v1alpha1 is unchanged. |
| `pkg/config/config.go`, `pkg/config/capabilities` | Dedicated default-off Supervisor capability mapped to the BYO feature state. |
| `pkg/crd/crd.go`, `pkg/crd/crd_test.go` | Remove the field and dependent CEL when the capability is disabled; test installed schemas with both capability and CRD cleanup settings. |
| `webhooks/virtualmachine/mutation`, `webhooks/virtualmachine/validation` | Skip managed-network defaults for external interfaces; validate type, environment, conflicting fields (`network`, `macAddr`), duplicates, and managed/external transitions. |
| `pkg/util/kube/network_interface.go` (new) | Canonical reference key, index registration, and indexed mapper in one package to avoid the PVC helper's import-cycle workaround. |
| `controllers/virtualmachine/virtualmachine` | With the capability on, replace the provider CR owner handler with a mapper that enqueues owner VMs, indexed referencing VMs, and the VM named by the `vm-name` label. |
| `pkg/providers/vsphere/network` | Typed external lookup (`external.go`); create/update dispatch; device matching; VPC association patching and label-based release. |
| `pkg/providers/vsphere/session`, `pkg/providers/vsphere/vmprovider_vm.go` | Release association metadata before network reconcile on update, and after VM destruction or when the VC VM is already absent. |
| `config/crd/bases`, `config/rbac` | Generated schema and RBAC if markers change. Existing provider CR permissions should suffice; verify them. |
| `test/e2e/vmservice/vmservice/virtualmachine`, `test/e2e/vmservice/consts` | One experimental add/remove/delete test for an external interface; capability prerequisites. |
| `docs/concepts/services-networking/guest-net-config.md` | Reference example, ownership, power-state limitations, and provider prerequisites. |

## API and CRD strategy

Use `ExternalRef *common.LocalObjectRef` with JSON name `externalRef`, `+optional`, and `omitempty`; its required `apiVersion`, `kind`, and `name` identify a resource in the VM's namespace. Unlike `network`, this reference has no defaulted kind or name. The proposed schema, examples, and provider matrix are in [model.md](./model.md).

Apply CEL to the static mutual-exclusion and nonempty-field rules, without changing managed-interface defaulting. Use Go admission for the feature gate, namespace-supported provider set, existing interface-override validation, duplicate references, and update semantics. Use the existing `NetworkSettings` resolution for per-namespace environments, not just the global provider setting. Do not require the referenced CR to exist at admission: existence/readiness are reconciliation concerns, which avoids preventing asynchronous creation and later recovery.

Expose the field only with the Supervisor capability `supports_VM_service_byo_network_interface`, represented by `CapabilityKeyBringYourOwnNetworkInterface` and mapped to `FeatureStates.BringYourOwnNetworkInterface`. In the VM CRD installation callback, use `removeFields` with `specFieldPath("network", "interfaces", "[]", "externalRef")` when disabled. Also use `removeValidationRules` at `specCELPath("network", "interfaces", "[]")` for `externalRef`, and audit any other containing rules that reference the removed field so CEL compilation succeeds. Apply this to every version whose generated schema exposes the field. Keep the field in generated bases; installation controls exposure. Follow the existing `shouldRemoveFields`/`CRDCleanupEnabled` policy: disabled fields are removed on first installation and on updates with cleanup enabled; runtime guards remain necessary when cleanup is disabled. Capability mapping must default to false when the capability is absent or inactive, including after a previously active value is removed.

Extend `restore_v1alpha6_VirtualMachineNetworkInterfaces` in v1alpha2–v1alpha5 to restore `externalRef` by interface name from the conversion annotation. v1alpha1 is unchanged: its existing whole-list restore already carries `externalRef`, and BYO-specific v1alpha1 conversion is not attempted. Restore only surviving interfaces. Conversion never fails; invalid results, such as an older client setting `network` on an external interface or dropping the reference from an existing interface, are rejected by the validation webhook against the old VM. Conversion annotations alone cannot protect clients that discard them.

Q3 is resolved: the user confirmed v1alpha6 remains in development. Use v1alpha6 and preserve references across every older served conversion.

## Admission behavior

1. The shared defaulting helper used by `AddDefaultNetworkInterface` and `SetDefaultNetworkOnUpdate` leaves any entry that carries `externalRef` untouched, including malformed references and gate-off requests through a retained schema so validation can reject them accurately. Interfaces without `externalRef` receive the default network as before. The default-network lookup still always happens; namespace provider validation still applies. Capability-based field removal belongs in CRD installation, not a VM mutation helper. When the API server has already pruned an unknown field from a disabled schema, ordinary default-network behavior applies to the remaining object.
2. Reject `network` plus `externalRef`, `macAddr` plus `externalRef` (CEL rule and webhook), empty fields (CEL), and an `apiVersion`/`kind` that does not match a provider supported in the namespace (`NetworkInterface` for VDS, `VirtualNetworkInterface` for NSX-T, `SubnetPort` for VPC). Named-network mode has no supported kind.
3. Reject duplicate references within the desired VM spec. Duplicates are compared by API group, `kind`, and `name`; the API version is ignored because resolution is by group. A reference may move to a new interface name on the same VM, subject to existing mutable-network rules. An existing interface name may switch between managed and external modes, subject to the same mutable-network rules. A resource that this VM already owns keeps that owner reference after the switch, so it is still deleted with the VM. With the capability disabled, only unchanged stored references are allowed.
4. Preserve existing interface-spec overrides other than `macAddr`, including explicit false DHCP values; apply device/bootstrap overrides without writing external CR specifications. Keep guest-only settings and hardware rules on the common validation path; `unitNumber` retains its existing behavior and BYO adds no unit-number handling.
5. Retain existing `MutableNetworks`, schema-upgrade, and power-state restrictions for changes to the interface list.

## Provider and lifecycle behavior

### Resolve the external resource before managed creation

Introduce a small shared typed resolver in `pkg/providers/vsphere/network/external.go`. After validating the supported GVK, fetch the referenced CR with the VM namespace and the exact supplied name. Do not derive `NetOPCRName`, `NCPCRName`, or `VPCCRName` for BYO entries. Reject terminating resources and resources with any VirtualMachine owner reference; preserve unrelated ownership and metadata. For VPC, establish the required association metadata before checking provider readiness. A same-VM rename updates the interface-name label and annotation rather than being rejected as a conflict. This is not an adoption operation.

Use the resolver in both create and session-update flows, before any managed create/patch or legacy-name fallback. Select the provider group from `externalRef` when present. Feed the fetched CR into the existing `getNetOPNetworkInterfaceDevice`, `getNCPNetworkInterfaceDevice`, or `getVPCSubnetPortDevice`. Preserve the spec-to-device ordering consumed by `BuildBootstraps` and the handling of generated MAC addresses; no MAC override is applied to external interfaces. A missing resource is a dependency failure, never permission to create one.

Reuse current readiness extraction and error propagation. Wrap failures with the VM interface name and external GVK/name. Existing condition paths must expose the failure; no separate readiness controller is planned. Reference watches must be installed before enabling any path that waits with a no-requeue error. Retain normal retries for API failures and a bounded dependency retry where a watch may not observe progress; do not add a new blocking poll loop. Existing update-path polling must use the referenced name when it remains in use.

### Preserve external resources during cleanup

`ListOrphanedNetworkInterfaces` already selects labeled resources and verifies the VM owner UID with `isOwnedBy`. Keep that owned-only deletion list. A BYO CR has no VM owner reference, so it is excluded even if its VM association labels match. This check works after the spec entry has disappeared; cleanup does not need to recover the removed `ExternalRef` to determine whether deletion is allowed.

Release of BYO VPC associations is separate from orphan deletion. `ReleaseExternalSubnetPorts` lists SubnetPorts in the VM namespace labeled with this VM's name (`vmoperator.vmware.com/vm-name`), skips any owned by this VM, and skips any still referenced by a `SubnetPort` `externalRef` in the spec. A remaining SubnetPort is released when its `Status.Attachment.ID` is empty or matches no current ethernet card's `ExternalId`.

| Resource state | Action |
|---|---|
| Owned by this VM | Existing managed-CR orphan deletion. |
| Unowned, labeled with this VM name, not referenced by spec, attachment ID empty or not on a current NIC | Patch away only VM Operator's association metadata; retain the CR. |
| Unowned, still referenced by spec (including under a new interface name) | Not released; attachment updates the interface-name label and annotation. |
| Unowned, still attached to a current NIC | Retained until a later reconcile no longer finds it attached. |
| Not labeled with this VM name | Leave untouched. |

Discovery uses the persisted `vm-name` label, so it works even if the last interface is removed or `spec.network` is nil/disabled, and is retried on subsequent reconciles until the labels are removed. No public VM status journal or volatile `Device.IsExternal` flag is required.

### VPC association metadata: attach, rename, and release

Q1 is resolved: VM Operator must set and clear the VM association for BYO SubnetPorts. The current managed callback sets `VMNameLabel`, `VMInterfaceNameLabel`, and the **annotation** `nsx.vmware.com/attachment_ref = virtualmachine/<vm-name>/<interface-name>`. A narrow metadata-only helper reuses those keys. No additional VM UID label or attachment-ID annotation is used. These are association markers, not lifecycle ownership. The key/value contract is in [model.md](./model.md).

On attach, the fetched SubnetPort is rejected as a conflict if its `vm-name` label names a different VM or its attachment annotation refers to a different VM. Otherwise the labels and annotation are patched with a merge patch, preserving the external spec, status, owner references, and unrelated metadata. An identical association is a no-op. Association is established before waiting for NSX readiness or creating the NIC. The managed `CreateOrPatch` callback, which also adopts the CR and rewrites allocation fields, is never invoked.

A same-VM interface rename only updates the interface-name label and attachment annotation; the `vm-name` label is unchanged and no release, delete/recreate, or intermediate spec is required. There is no special reassociation-detach step: hardware changes go through the normal mutable-network device reconcile and its power-state restrictions. Guest naming follows existing bootstrap semantics; this does not promise a live guest rename. Preserve the global VM reconcile order. This feature adds no hot-plug or cross-VM reservation guarantee.

On update, `ReleaseExternalSubnetPorts` runs before network reconciliation using the ethernet cards from the VM properties fetched at the start of the reconcile. A SubnetPort removed from the spec but still attached to a current NIC is retained and released on a later reconcile after the NIC is gone. Release removes the `vm-name` and `vm-interface-name` labels and the attachment annotation in one merge patch, preserving all unrelated metadata and the CR. Not-found resources are skipped; other patch errors are returned for retry.

On VM deletion, `ReleaseAllExternalSubnetPorts` clears every unowned SubnetPort labeled with the VM name after the destroy operation completes, or when the VC VM is already absent, before `DeleteVirtualMachine` reports success and the finalizer is removed. The same release runs when the VM is deleted with the skip-delete-platform-resource annotation. Release is a no-op when the capability is disabled, and it lists by the cluster's supported provider types rather than the namespace NetworkSettings, so namespace teardown cannot block it. A cleanup error retains the finalizer and retries; it does not require the external CR to be ready. There is no pre-create release.

### Status and guest customization

`devices.go` currently dereferences `interfaceSpec.Network` and reconstructs managed CR names during mutable-network mapping. Add explicit BYO lookup here as well; fixing create alone is insufficient. External interfaces are matched to ethernet cards through the referenced CR's provider identity. Do not misattribute same-name resources of different kinds or positionally shift surviving NICs after removal.

Keep `Device` and `Bootstrap` separate. Source IPAM state and device identity from the external CR; preserve interface-spec overrides through existing device/bootstrap helpers. BYO does nothing special with `unitNumber`; it retains its existing behavior, and unit-number work is tracked separately (spec 008). This feature does not add hot-plug support or reorder the VM update sequence.

## Watches and indexes

Register a multi-value VM field index for external references keyed by API group (parsed from `apiVersion`), kind, and name. Scope mapper queries with `client.InNamespace` and `client.MatchingFields`. De-duplicate references and requests. Nil network specs and invalid references must be safe to index.

With the capability on, each cluster-supported provider CR watch uses a single mapper that enqueues owner VMs, VMs referencing the CR via the field index, and the VM named by the `vm-name` label; managed-resource events are preserved. With it off, the existing owner handler is used. Use typed resource knowledge or the scheme to derive the event object's kind/group, since cached objects may have empty TypeMeta. Handle create, update, delete, and generic events, including status-only changes; a generation-only predicate would lose readiness updates. Feature-gate the additional index/watch wiring and propagate setup failures. Missing provider CRDs must not cause watches for providers the cluster does not support.

The `vm-name` label mapping handles SubnetPort changes after the external reference has left the spec; the reference index alone cannot find that VM then. Owner, desired-reference, and label requests are de-duplicated. Cleanup failures are returned for retry, so eventual release does not depend on another SubnetPort event.

## Test strategy

| Layer | Required evidence |
|---|---|
| API, conversion, and installed CRDs | AC2–3, AC10–11: schema, exclusivity, v1alpha2–v1alpha5 round trips restoring `externalRef` by interface name. Verify the capability mapping for absent/inactive/active values; field and dependent CEL presence/absence across capability and CRD cleanup combinations; real API-server acceptance of the resulting schemas and unknown-field handling. |
| Admission | AC2–3, AC10–11: BYO-only/mixed/default interfaces, invalid references, provider matrix, duplicate desired references, `network`/`macAddr` exclusivity, managed/external transitions, create and update, and disabled runtime guards when the schema is retained. |
| Provider with fake client/vcsim | AC1, AC4–7, AC9, AC13: create/update exact lookup; no create/adopt/spec writes; idempotent VPC association before readiness; same-VM rename label/annotation update; cross-VM conflict rejection; VM-owned rejection; release of unreferenced/detached SubnetPorts and retention of attached ones; release of all on deletion. |
| Watch envtest | AC7–8: no-owner creation, status-only update, deletion, late readiness, namespace/kind collisions, multiple referring VMs, managed-owner path retained, and VPC association mapping after spec removal. Demonstrate that the event causes reconciliation rather than merely exercising the mapper. |
| Real Supervisor E2E | AC1, AC4–5, AC12: one experimental test clones the VM's managed interface spec into an external CR, adds it as `eth1` to a powered-off VM, checks two NICs, no owner reference, and VPC association, removes it, checks one NIC and release, then deletes the VM and checks the CR still exists. The full vmop-2585 three-interface workflow, rename/swap, restart, and dependency-recovery scenarios are not yet covered by E2E. |

Run `make test-api`, focused package tests through the existing test harness, `make lint-go`, `make generate-go`, `make generate-go-conversions`, `make generate-manifests`, `make generate-api-docs`, and `make verify-codegen` as appropriate to the implementation. Review the generated diff for changes to shipped schemas. These are future implementation checks, not checks performed for this documentation change.

Extend the current networking E2E suite. The new scenario carries `experimental` until validated on hardware, then remove it once evidence is recorded. Select a provider and a real image/class/bootstrap combination that can validate guest configuration. For removal, require `MutableNetworks` and a powered-off path that actually emits NIC changes; capability skips do not count as provider validation. Delete externally created CRs in test cleanup only after asserting that VM deletion left them intact. Put the first usable feature slice and its E2E tests in the same PR, even though they map to different Jira stories.

## Rollout and migration

Use `FeatureStates.BringYourOwnNetworkInterface`, default `false`, activated by the dedicated Supervisor capability `supports_VM_service_byo_network_interface`. Define `CapabilityKeyBringYourOwnNetworkInterface` in `pkg/config/capabilities/capabilities.go` and the matching `BringYourOwnNetworkInterfaceCapabilityName` for E2E capability checks. This follows the capability-backed configuration path, with no FSS environment-variable switch. Keep it independent of `MutableNetworks` and `VMNetworkUnitNumbers`. The user approved v1alpha6. No existing VM is backfilled into BYO mode and no existing CR changes ownership.

When disabled, remove the field and dependent CEL from the installed schema under the existing CRD cleanup policy. If the schema is retained, reject new/changed external selections while allowing unchanged stored references on unrelated updates/deletion. Any external reference that reaches the disabled provider produces a clear feature-disabled dependency error before managed creation or attachment. Release of already-recorded VPC associations is not gated, so it remains available, including gate-off VM deletion; it does not enable new attachments. Return metadata-cleanup failures for retry before removing the finalizer.

Drain BYO workloads before disabling the capability, enabling cleanup that removes its field, or downgrading to a binary/schema that lacks reference support. Schema removal can cause unknown-field rejection or pruning; once an external reference is pruned, ordinary network defaulting can apply. Do not promise stored-reference preservation or a BYO-specific admission error after pruning. Conversion annotations cannot make an older binary understand external lifecycle semantics. Document capability discovery, same-namespace scope, allocation restrictions, provider coverage, same-VM rename/reuse, and powered-off hardware-change requirements. Coordinate the final field/attachment contract with the image-fetcher owner; direct-boot and hot-plug work remain independently tracked.

## Delivery sequence

1. Implement the resolved association, same-VM rename, and capability contracts in the design and the resolved v1alpha6/override decisions.
2. Land schema/conversion, capability mapping, CRD field/CEL removal, and default-off runtime protection together.
3. Deliver admission, typed provider resolution, indexed watches, lifecycle protection, and the mixed-interface E2E scenario together as the first usable slice.
4. Validate each provider on real infrastructure, record evidence, publish examples/release notes, and enable only the validated provider combinations.

See [tasks.md](./tasks.md) for file-level tasks and [research.md](./research.md) for the evidence behind these decisions.
