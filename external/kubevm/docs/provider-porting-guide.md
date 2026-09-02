# KubeVM Provider Porting Guide

This describes what a provider has to implement to serve KubeVM `VirtualMachine` objects, and then walks through the vSphere port as a concrete example.

Part 1 is the provider contract and applies to every provider. Part 2 is specific to VM Operator on vSphere; skip it if you are not working on that port.

The API types are in [`api/v1alpha1`](../api/v1alpha1). The design rationale is in [`one-pager-kubevm-generic-api.md`](one-pager-kubevm-generic-api.md).

## Part 1: the provider contract

### Object model

A portable `kube-vm.io/v1alpha1` `VirtualMachine` holds configuration that means the same thing on every platform. It binds to one backend through `spec.infrastructureRef`, which names a provider-owned object by `apiGroup`, `kind`, and `name` in the same namespace. There is no version in the reference: the group and kind identify the resource, and the served version is resolved at runtime, so a provider can roll its API forward without rewriting every object that points at it.

The provider object carries platform-specific configuration only. Anything portable belongs on the generic object.

You do not have to create a new CRD. The provider object can be an existing type you already ship. That is the case for the vSphere port, which uses VM Operator's own `VirtualMachine`.

### Write direction

This is the rule that everything else follows from.

- The generic core never writes the provider object's spec. It does a `Get`, reads status at contract paths, sets an owner reference, and manages finalizers and deletion ordering. This matches Cluster API, whose core `Machine` controller gets the infrastructure object and errors if it is absent rather than creating it.
- The core reads provider **status** at contract-defined paths. It never reads or interprets provider **spec**, because doing so would require knowing platform-specific field names.
- The provider resolves its configuration by reading the owning generic object. Providers importing the core API is normal; Cluster API ships `util.GetOwnerMachine` for exactly this.
- The core never imports provider code. The dependency runs one way: provider module depends on the core API module.

### Linkage requires both sides to agree

Adoption happens only when the generic object and the provider object each name the other.

Generic side: `spec.infrastructureRef` names the provider object. It is immutable, enforced by CEL (`self == oldSelf`) in [`virtualmachine_types.go`](../api/v1alpha1/virtualmachine_types.go).

Provider side: the annotation `kube-vm.io/virtual-machine: <name>` on the provider object, naming the generic object in the same namespace. It should be immutable once set.

A one-sided reference must never cause adoption. On mismatch the core writes nothing to the provider object and reports a not-adopted condition on the generic object.

The owner reference follows from confirmed linkage. The core sets it after both sides check out, for garbage collection. It is not the adoption signal; treating it as one would be self-certifying, since the core is the thing that writes it.

If the provider object already carries a controller owner reference from a different generic object, refuse and report it. Never take it over.

The reason for two-sided declaration is that VMs hold state. With a one-sided reference, anyone who can create a generic `VirtualMachine` could point it at a running VM and reconfigure it. Deployments adopt Pods by label selector, which is safe because Pods are disposable and a wrongly adopted Pod is replaced rather than mutated in place.

There is precedent in VM Operator. A `VirtualMachineGroup` only takes ownership of a VM when the VM sets `spec.groupName` to the group's name **and** the group lists the VM in one of its boot order groups, at `spec.bootOrder[].members`. See the `GroupName` field doc in `api/v1alpha6/virtualmachine_types.go` (lines 1193-1204) and `virtualmachinegroup_types.go` lines 14 and 50. The controller walks the group's member list and checks `obj.GetGroupName() != ctx.VMGroup.Name` before setting the owner reference (`controllers/virtualmachinegroup/virtualmachinegroup_controller.go:406`).

An annotation rather than a spec field, because the provider object may be a CRD whose schema the provider does not control, and requiring a spec field would rule that case out. The contract already uses metadata normatively: the contract version a provider satisfies is advertised as a label on its CRD. A spec field is more discoverable, and promoting the annotation to a field later is a non-breaking migration, so this can change.

### Minimal object pair

```yaml
apiVersion: kube-vm.io/v1alpha1
kind: VirtualMachine
metadata:
  name: web-01
  namespace: team-a
spec:
  powerState: PoweredOn
  instanceType:
    name: small
  bootDisk:
    source:
      image:
        apiGroup: infrastructure.example.com
        kind: ExampleImage
        name: ubuntu-2204
    storageClassName: fast
  infrastructureRef:
    apiGroup: infrastructure.example.com
    kind: ExampleVirtualMachine
    name: web-01
```

```yaml
apiVersion: infrastructure.example.com/v1alpha1
kind: ExampleVirtualMachine
metadata:
  name: web-01
  namespace: team-a
  annotations:
    kube-vm.io/virtual-machine: web-01
spec: {}
```

The provider object's spec is empty on submission. The provider's mutating webhook fills in the fields it needs from the parent.

### Field handling depends on whether the provider's field is mutable

**Immutable on the provider.** Resolve once, at create, in the provider's mutating webhook, by reading the parent and writing the value into the provider object's spec.

**Mutable on the provider.** The provider's own controller copies the parent's value into the provider spec on every reconcile. VM Operator already does this shape for group members: `updateMemberPowerState` assigns `obj.Spec.PowerState = group.Spec.PowerState` (`virtualmachinegroup_controller.go:842-850`), persisted with the patch at line 426.

Either way, **persist the value into the provider object's spec.** Do not resolve it in memory at reconcile time. `kubectl get -o yaml` on the provider object has to show what the VM will actually do. Other controllers read that object, and GitOps diffing, backup and restore, and audit all assume the spec is truthful.

This deviates from the usual guidance that a controller should only write status. The trade-off is deliberate and the VM Operator group controller already accepted it for the same shape.

### Precedence

The generic object wins for the fields it owns. That is enforced by the provider controller reasserting the value on every reconcile, not by a policy. A user who hand-patches a delegated field on the provider object sees it revert on the next reconcile, the same as editing a Pod owned by a Deployment.

There is no conflict-resolution policy, no condition vocabulary for conflicts, and nothing that can deadlock.

### Status contract

The core reads a fixed set of paths off the provider object as `unstructured`. It knows nothing about the provider's native field names. Every provider is obliged to surface these paths and populate them from whatever its platform gives it.

| Contract path on provider object | Generic status field |
|---|---|
| `status.addresses[]`, entries typed `InternalIP` / `ExternalIP` | `status.addresses[]` |
| `status.powerState` | `status.powerState` |
| `status.providerID` | `status.providerID` |
| `status.providerMetadata` | `status.providerMetadata` |
| `Ready` condition in `status.conditions` | `Ready` condition |

Readiness travels through the `Ready` condition. The core mirrors it onto the generic object and also sets the generic `status.ready` boolean from it. That boolean is a plain field on `VirtualMachineStatus`, separate from conditions, and the core has to set it; tools that read only `status.ready` are common. Providers do not need a boolean of their own.

`status.providerMetadata` is a `map[string]string` for observed facts with no portable equivalent. The core copies it wholesale and never reads a value back into a reconcile decision, so a provider can add keys without negotiating a schema change.

For `providerID`, pick a stable, globally unique identifier the platform already gives you. Do not invent a URI scheme. A managed-object reference or similar handle that is unique only within one management endpoint is not good enough, since a cluster can face more than one.

Sketch of the adapter, on the core side:

```go
var infra unstructured.Unstructured
infra.SetGroupVersionKind(gvk) // resolved from apiGroup + kind at runtime
if err := c.Get(ctx, key, &infra); err != nil {
    return err // never create it
}

providerID, _, _ := unstructured.NestedString(infra.Object, "status", "providerID")
powerState, _, _ := unstructured.NestedString(infra.Object, "status", "powerState")
addrs, _, _ := unstructured.NestedSlice(infra.Object, "status", "addresses")
meta, _, _ := unstructured.NestedStringMap(infra.Object, "status", "providerMetadata")
```

### Feature gating

Put the provider's KubeVM behaviour behind a feature gate, default off. Nothing changes for existing users until the gate is on, and the gate plus the annotation together keep the new code path to a single conditional branch.

### Corner cases

**Both sides default the same field.** The generic API defaults `spec.powerState` to `PoweredOn` (`+kubebuilder:default=PoweredOn`). Most providers default their equivalent too. So "field is empty" cannot mean "delegated to the parent", and you cannot tell a user-set value from a defaulted one by inspection. Do not build a conflict rule on presence or absence. "Empty means delegated" is also not portable, since a provider whose field is required cannot express it.

**Post-create edits the provider cannot apply.** The generic API may accept an edit that is immutable on your platform. Report it with `UpToDate=False`, reason `UnsupportedByProvider`, and keep reconciling everything else. Never stop reconciling a live VM over one unappliable field. Do not make the generic field immutable because your platform's is: EC2 can change instance type while the instance is stopped, and pinning the portable API to the strictest platform would be wrong.

**Adopting a VM that already exists.** Supported and intended. An operator adds the annotation to an existing provider object and creates a generic object whose immutable fields match the provider object's current values. An import flow can back-fill the generic spec from the provider object at create time so the two agree by construction. Adoption cannot change immutable fields, so a mismatch has to be rejected, not reconciled.

**Someone deletes the provider object.** The core does `Get` only and never recreates it. It reports a not-adopted condition and waits. This is intended behaviour, not a bug.

**Short name collision.** The generic CRD declares `shortName=vm;vms` and VM Operator's declares `shortName=vm`, so `kubectl get vm` is ambiguous once both are installed. Use fully qualified resource names in scripts and documentation: `virtualmachines.kube-vm.io` and `virtualmachines.vmoperator.vmware.com`.

**Module dependency direction.** Your module depends on the core API module; the core never depends on yours. The core API module keeps its Kubernetes dependency floor deliberately low because providers import it, so do not expect it to track your own dependency versions. If provider and core live in one repository during development, wire them with a `replace` directive. The root `go.mod` in this repository already does that for the other `external/*` modules (lines 5-16). Note there is no `replace` for `external/kubevm` today, because nothing in the parent build imports it yet.

## Part 2: the vSphere port (VM Operator)

The provider object is VM Operator's existing `vmoperator.vmware.com` `VirtualMachine`. No new CRD.

### Feature gate

Add a gate to `FeatureStates` in `pkg/config/config.go` (the struct is at line 191), default off. The resolution path engages only when the gate is on and the annotation is present.

### The object pair, vSphere flavour

```yaml
apiVersion: kube-vm.io/v1alpha1
kind: VirtualMachine
metadata:
  name: web-01
  namespace: team-a
spec:
  powerState: PoweredOn
  instanceType:
    name: best-effort-small
  bootDisk:
    source:
      image:
        apiGroup: vmoperator.vmware.com
        kind: VirtualMachineImage
        name: vmi-0a0044d7c690bcbea
    storageClassName: wcpglobal-storage-profile
  infrastructureRef:
    apiGroup: vmoperator.vmware.com
    kind: VirtualMachine
    name: web-01
```

```yaml
apiVersion: vmoperator.vmware.com/v1alpha6
kind: VirtualMachine
metadata:
  name: web-01
  namespace: team-a
  annotations:
    kube-vm.io/virtual-machine: web-01
spec: {}
```

The sample in `config/samples/virtualmachine.yaml` in this module instead shows a dedicated `VSphereVirtualMachine` type in its own group. That is a hypothetical shape for a provider that ships a new CRD. The port uses VM Operator's existing type.

### Fields that move down

| Generic | vSphere provider | Mutability |
|---|---|---|
| `spec.powerState` | `spec.powerState` | mutable, reasserted each reconcile |
| `spec.instanceType.name` | `spec.className` | resolved at create; immutable afterwards unless a resize feature gate is enabled |
| `spec.bootDisk.source.image.kind` | `spec.image.kind` | immutable, resolved at create |
| `spec.bootDisk.source.image.name` | `spec.image.name` | immutable, resolved at create |
| `spec.bootDisk.storageClassName` | `spec.storageClass` | immutable, resolved at create |

Power state values are identical on both sides. The generic `PowerState` enum spells them `PoweredOn`, `PoweredOff`, `Suspended` to match `VirtualMachinePowerState`, so there is no value mapping.

Field names verified in `api/v1alpha6/virtualmachine_types.go`: `spec.className` (881), `spec.image` (827, a `VirtualMachineImageRef` with `kind` and `name`), `spec.storageClass` (922), `spec.powerState` (963).

The field set is deliberately small. The detailed fields are expected to change as more providers engage, so the first version moves five values and no more.

### Status fields to add

VM Operator's `VirtualMachineStatus` today has `powerState` (1436), `network.primaryIP4` (`virtualmachine_network_types.go:866`), `uniqueID` (1460), `instanceUUID` (1473), and `conditions` (1440). It has no `addresses`, no `providerID`, and no `providerMetadata`. The port has to add those three contract paths and populate them from the values it already has.

| Contract path to add | Populated from |
|---|---|
| `status.addresses[]`, one entry typed `InternalIP` | `status.network.primaryIP4` |
| `status.providerID` | `status.instanceUUID` |
| `status.providerMetadata["uniqueID"]` | `status.uniqueID` |

`status.powerState` and the `Ready` condition (`api/v1alpha6/condition_consts.go:9`) already sit at the contract paths and need no work.

`providerID` comes from `status.instanceUUID`, not `status.uniqueID`. `uniqueID` is set to `vmCtx.MoVM.Self.Value` (`pkg/providers/vsphere/vmlifecycle/update_status.go:438`), a vSphere managed-object reference that is unique within one vCenter but not globally, so it is not usable as `providerID`. It is still worth surfacing, which is what the `providerMetadata` entry is for. `instanceUUID` is set from `Summary.Config.InstanceUuid` two lines below and is the stable UUID.

### Resolution belongs in the mutating webhook

The validating webhook requires `spec.image` and `spec.className` to be non-empty on create: see `validateImageOnCreate` (`webhooks/virtualmachine/validation/virtualmachine_validator.go:607`, `field.Required` when `vm.Spec.Image == nil`) and `validateClassOnCreate` (line 652, `field.Required` on `spec.className` for a classless VM unless the import gate is on). A provider object submitted with an empty spec would be rejected before any controller ran.

Mutating webhooks run before validating webhooks, so resolving in the mutator means the object that reaches validation is an ordinary VM Operator `VirtualMachine` and no validation changes are needed.

The empty spec is schema-valid. The v1alpha6 schema in `config/crd/bases/vmoperator.vmware.com_virtualmachines.yaml` has no `required` list at spec level, so `spec: {}` is accepted by the API server.

### Ordering the mutations

Resolution has to be an explicit call in `mutator.Mutate`, placed before VM Operator's own defaulting and resolution steps: `SetDefaultPowerState`, `SetDefaultCdromImgKindOnCreate`, `ResolveImageNameOnCreate`, and `ResolveClassAndClassName` (`webhooks/virtualmachine/mutation/virtualmachine_mutator.go:272-295`).

Do not register it through `MutateOnCreateFuncs`. That is a `sync.Map` (line 112) iterated with `Range` at line 305, and `sync.Map` iteration order is unspecified, so registration there cannot guarantee resolution runs first.

`SetDefaultPowerState` (line 672) sets `spec.powerState` to `VirtualMachinePowerStateOn` when it is empty. That is the concrete instance of both sides defaulting the same field: once it runs you cannot distinguish a value from the parent, a value from the user, and a value from this defaulter.

### Reading the parent from inside the webhook

```go
// The parent's name comes from the annotation on the provider object.
parentName := vm.Annotations["kube-vm.io/virtual-machine"]
if parentName == "" {
    return false, nil // not a KubeVM-managed VM
}

var parent kubevmv1a1.VirtualMachine
key := ctrlclient.ObjectKey{Namespace: vm.Namespace, Name: parentName}
if err := apiReader.Get(ctx, key, &parent); err != nil {
    return false, err
}

// Mutual declaration: the parent must point back at this object.
ref := parent.Spec.InfrastructureRef
if ref.APIGroup != vmopv1.GroupName || ref.Kind != "VirtualMachine" || ref.Name != vm.Name {
    return false, fmt.Errorf("%s does not reference this object", parentName)
}

vm.Spec.ClassName = parent.Spec.InstanceType.Name
// ... remaining create-time fields
```

Use the uncached reader, `mgr.GetAPIReader()`. A manager's default client is cache-backed, and the first `Get` of a type it has never seen lazily starts an informer. Inside a live admission request with `failurePolicy: Fail`, that request blocks waiting for cache sync, and it never succeeds if the CRD is not installed at all. Three VM Operator controllers already take the uncached reader for their own reasons, including `controllers/virtualmachinegroup/virtualmachinegroup_controller.go:62`.

The alternative is to exclude the type from the cache via `client.CacheOptions.DisableFor` in `pkg/manager/manager.go` (lines 108-109), which already lists `ConfigMap`, `Secret`, and `Deployment`.

There is also an RBAC consequence. A cache-backed client needs `list` and `watch` on the type, not just `get`, over the cache's configured scope. `GetNamespaceCacheConfigs` (`pkg/manager/cache.go:107`) returns `nil` and therefore a cluster-wide cache when no watch namespace is set. The uncached reader needs only `get`.

### Patching owner references

Owner references are a shared list, and a plain merge patch replaces a list wholesale with no `resourceVersion` precondition, so it can silently drop a concurrent writer's entry with no error to trigger a retry. There is a second writer to that list on VM Operator VMs: the group controller calls `controllerutil.SetOwnerReference` on member VMs (`virtualmachinegroup_controller.go:428`).

Patch with an optimistic lock and skip the write when nothing changed:

```go
base := obj.DeepCopy()
// set the owner reference on obj ...
if !apiequality.Semantic.DeepEqual(base.OwnerReferences, obj.OwnerReferences) {
    if err := c.Patch(ctx, obj,
        client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
        return err
    }
}
```

Compare with `apiequality.Semantic.DeepEqual` from `k8s.io/apimachinery/pkg/api/equality`, not `reflect.DeepEqual`, which false-diffs on types like `resource.Quantity` and would defeat the skip guard.

### Waiting for an address

Provisioning and guest boot take time, and an address only appears once VM Tools reports one. Requeue with a delay instead of hot-looping. VM Operator already has a constant for this case: `PoweredOnVMHasIPRequeueDelay`, 10 seconds (`pkg/config/default.go:47`), used at `controllers/virtualmachine/virtualmachine/virtualmachine_controller.go:503`.

### Scheme registration

Register the core API types in three places:

- `pkg/manager/manager.go`, alongside the other `AddToScheme` calls at lines 64-96, gated on the feature gate as `imgregv1` and `vimv1` are.
- `test/builder/fake.go`, in the `AddToScheme` block at lines 110-118, for unit tests using the fake client. Add the type to `KnownObjectTypes` (line 63) so the fake client enforces the spec/status split.
- For envtest integration tests, drop the generic CRD into `config/crd/external-crds/`, which `test/builder/test_suite.go:310` loads as `CRDDirectoryPaths`.

## Open items

- The contract version label key is described in the API doc comments but is not yet defined as a constant in this module.
- There is no conformance test suite holding a provider to the status contract.
- The core controller does not exist in this module yet. This module is API types and generation only.
