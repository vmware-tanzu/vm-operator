# Feature Specification: VM Operator as a KubeVM provider

- **Feature branch**: [`vmop-kubevm-provider`](../../..)
  - **Fork**: N/A
  - **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-08-31
- **Status**: Draft
- **Epic**: TBD
- **Design docs**: `external/kubevm/docs/one-pager-kubevm-generic-api.md`

---

A user drives a VM's whole lifecycle — create, get an address, power off, power on, delete — through the generic `kube-vm.io/v1alpha1` `VirtualMachine` object, with VM Operator's own `VirtualMachine` acting as the provider object behind it. A minimal field set only. The acceptance evidence is a short recorded demo on a Supervisor.

## Goals

- MUST let a DevOps user create a generic `VirtualMachine` that names a VM Operator `VirtualMachine` as its infrastructure object, and have the VM come up without the user setting a VM class, image, or storage class on the provider object.
- MUST make the generic object the single place the user edits for the delegated fields. A change to the generic object's desired power state MUST reach the backend VM; a hand-edit of the same field on the provider object MUST be reverted.
- MUST report the backend VM's observed state on the generic object: its address, its power state, its platform identifier, and whether it is ready.
- MUST have VM Operator publish that observed state on its own object at the fixed field paths the generic status contract defines, so the generic core reads the same paths for every provider and contains no VM Operator-specific code.
- MUST require both objects to name each other before any of this engages. A generic object that points at a provider object which does not point back MUST NOT cause adoption and MUST NOT cause any write to the provider object.
- MUST persist the delegated values in the provider object's spec, so that reading the provider object shows what the VM was actually built from.
- MUST accept a post-create edit to a generic field that the provider cannot apply, report that it was not applied with reason `UnsupportedByProvider`, and keep reconciling everything else.
- MUST be off by default. The delegation behaviour engages only when a feature gate is enabled and mutual linkage is confirmed.
- MUST leave every existing VM Operator behaviour unchanged when the gate is off or when a VM Operator `VirtualMachine` has no generic owner.

## Non-goals

- Does not support the full generic API. Only the four spec values and four status values listed under "Delegated fields" are mapped. Every other generic field is ignored in this iteration; nothing is silently approximated.
- Does not support adoption of a pre-existing VM Operator `VirtualMachine` that was created before the generic object. Both objects are created together in this iteration. The intended future mechanism is an explicit annotation gesture on the existing provider object plus an import step that back-fills the generic object's spec at create time, so the generic spec is truthful about a VM it did not provision.
- Does not apply post-create edits to the generic fields that map onto VM Operator fields which are immutable after create — sizing, image, and storage class. Such an edit is accepted by the API and reported as not applied. The generic fields are deliberately **not** made immutable to compensate, because a future VM Operator that can resize should not require an API break to allow the edit.
- Does not recreate a provider object that the user deletes directly. The generic object reports that it has no adopted infrastructure and waits.
- Does not add data disks, additional network interfaces, static addressing, cloud-init bootstrap, SSH keys, tags, failure domains, or spot scheduling.
- Does not add a second reconciler for suspend. `Suspended` is a legal value in both APIs and passes through, but is not part of the demo or the acceptance criteria.
- Does not introduce a provider-specific API group (for example `infrastructure.vsphere.kube-vm.io`). VM Operator's existing `vmoperator.vmware.com` `VirtualMachine` is the provider object.
- Does not remove or rename any existing VM Operator status field. The contract fields are added alongside the ones that carry the same information under VM Operator's own names today.
- Does not ship E2E coverage in the same change set. E2E is explicitly scoped as a follow-up and is stated as such in the PR summary.

## Delegated fields

The generic object owns four values. The user sets them there and nowhere else.

| Generic spec path | Applied |
|---|---|
| `spec.powerState` | On every reconcile, for as long as the objects are linked |
| `spec.instanceType.name` | Once, when the VM is created |
| `spec.bootDisk.source.image` (its kind and name) | Once, when the VM is created |
| `spec.bootDisk.storageClassName` | Once, when the VM is created |

Power state values are spelled identically in both APIs (`PoweredOn`, `PoweredOff`, `Suspended`), so a user sees the same word wherever they look.

The generic object reports four values back, plus readiness: the guest's address as an entry of type `InternalIP` in `status.addresses`, the observed power state in `status.powerState`, the platform's identifier for the VM in `status.providerID`, any further provider-reported facts in `status.providerMetadata`, and readiness as both the `Ready` condition and the `status.ready` boolean.

## Scope note: this adds fields to a published VM Operator API

The generic core must not contain provider-specific code, so the observed values above are read from fixed contract paths that every provider surfaces on its own object. VM Operator reports the same information today under its own names, so it gains new, optional, additive status fields at the contract paths: `status.addresses`, `status.providerID`, and `status.providerMetadata`. These are written by VM Operator's normal status reconciliation, are not gated, and duplicate information already present elsewhere in the same status. This is an API change to a shipped CRD and is treated as one — see `plan.md`.

## User stories / acceptance criteria

### DevOps user

- **Given** a namespace with a working VM class, a ready image, and an associated storage class, **when** the user applies a generic `VirtualMachine` carrying only a power state, an instance type name, a boot disk image reference and a storage class name, followed by a VM Operator `VirtualMachine` in the same namespace whose spec is empty and which carries the back-reference annotation, **then** the VM is created and both objects report ready.
- **Given** that VM, **when** the user reads the provider object, **then** its spec shows the class, image and storage class the VM was built from — not an empty spec.
- **Given** that VM, **when** the user reads the generic object's status, **then** it carries the VM's address as an `InternalIP` entry, its observed power state, its platform identifier, and `ready: true`.
- **Given** that VM, **when** the user sets the generic object's desired power state to `PoweredOff`, **then** the backend VM powers off and both objects report the new observed power state; **and when** the user sets it back to `PoweredOn`, **then** the VM powers on again.
- **Given** that VM, **when** the user hand-edits the desired power state on the provider object only, **then** the value is restored to the generic object's value on the next reconcile.
- **Given** that VM, **when** the user changes the instance type name on the generic object, **then** the request is accepted, the backend VM is unchanged, and the generic object reports `UpToDate` false with reason `UnsupportedByProvider` — reconciliation of everything else continues.
- **Given** that VM, **when** the user deletes the generic object, **then** the provider object and the backend VM are deleted, and the generic object goes away only after the provider object has gone.
- **Given** that VM, **when** the user deletes the provider object directly, **then** the generic object reports that it has no adopted infrastructure and waits; nothing is recreated.
- **Given** a generic object that names a provider object which does not carry the back-reference annotation, **when** the user applies it, **then** nothing is adopted, no field on the provider object is written, and the generic object says why it is waiting.
- **Given** a provider object that carries the back-reference annotation, **when** the user tries to change that annotation to name a different generic object, **then** the change is rejected.
- **Given** a provider object already owned by one generic object, **when** a second generic object names it, **then** the second generic object refuses to adopt it and says so.

### Platform engineer

- **Given** the feature gate is off, **when** a generic `VirtualMachine` and a matching provider object are applied, **then** no field on the provider object's spec is written by anyone and the existing VM Operator behaviour is unchanged — an empty provider spec fails VM Operator's own create-time validation exactly as it does today.
- **Given** the feature gate is off, **when** any VM Operator `VirtualMachine` is created and operated normally, **then** its behaviour is unchanged in every respect except that its status also reports the address, platform identifier and provider metadata at the contract paths.
- **Given** a VM Operator `VirtualMachine` read through an older API version, **when** the contract status fields are populated, **then** they survive a round trip back to the current version rather than being silently lost.

## Open questions

- [NEEDS CLARIFICATION: Demo environment. This work needs a Supervisor where a new CRD can be installed, the feature gate can be turned on, and the generic controller can be run out-of-cluster against a namespace that already has a working VM class, ready image, and associated storage class. Until an environment meeting all of those is confirmed, the demo-recording work cannot start. There is no acceptable fallback: the in-process vSphere simulator harness cannot be driven with kubectl and its VMs run no guest, so no address is ever reported.]
- [NEEDS CLARIFICATION: Whether the generic controller should run under a namespace-scoped ServiceAccount for the demo. Running it with a cluster-admin kubeconfig makes it a privileged account, which bypasses validations a normal DevOps user would hit, so the demo would not demonstrate the user's actual experience. A namespace-scoped ServiceAccount is preferred; if the environment cannot provide one, the caveat has to be stated on the recording.]
- [NEEDS CLARIFICATION: Whether the contract status fields should eventually replace VM Operator's existing equivalents rather than sit beside them. There is already a `TODO(v1alpha6)` in the status reconciler proposing that the existing identifier fields migrate into a provider sub-struct, which pulls in the opposite direction. Deciding this is out of scope here but should not be left open indefinitely, because three fields reporting the same fact is a maintenance cost.]
- [NEEDS CLARIFICATION: Whether the generic object's image reference kind — namespaced `VirtualMachineImage` versus cluster-scoped `ClusterVirtualMachineImage` — should be validated at admission on the generic side. The generic boot disk source is immutable, so a user who names the wrong kind has to delete and recreate the generic object. Deferred, because the generic API server cannot resolve a provider-owned kind without importing provider code.]
