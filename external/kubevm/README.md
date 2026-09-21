# KubeVM — Generic VM API (bootstrap module)

A generic, provider-agnostic Kubernetes API for virtual machines, served under
the `kube-vm.io` group. A `VirtualMachine` here describes a VM portably and
binds to a specific backend through `spec.infrastructureRef`, which points at a
provider-owned object. The generic layer observes that backend only through a
duck-typed status contract, so nothing in this module imports provider code.

The design rationale is in [`docs/one-pager-kubevm-generic-api.md`](docs/one-pager-kubevm-generic-api.md).

> **⚠ Temporary location.** This module lives in a subdirectory of a VM Operator
> worktree only so the API can be iterated on with the tooling already available
> here. It is expected to move to its own repository once a permanent home is
> approved.
>
> It sits under `external/` alongside the vendored copies of other projects'
> API types. Unlike those, this is first-party code, so the parent Makefile's
> blanket exclusion of `external/` from linting is overridden for this one
> directory — see `GO_MOD_DIRS_TO_LINT` there. Nothing else in the parent
> build references this module.
>
> Consequently its module path is currently
> `github.com/vmware-tanzu/vm-operator/external/kubevm`, which is accurate today
> (the code really is fetchable at that path) but **will change on the move**.
> Nothing outside this module imports it yet, so the rename is a mechanical
> search-and-replace across `go.mod` and the import blocks in `api/v1alpha1/`.
> Keep it that way — avoid adding cross-imports to the parent vm-operator module,
> which would turn the move into a real migration.

## Status

These types implement the API proposed in
[`docs/one-pager-kubevm-generic-api.md`](docs/one-pager-kubevm-generic-api.md).
The generic core controller now lives at [`controller/`](controller/) and
reconciles a `VirtualMachine` end to end against a provider object — see
[`controller/internal/contract/`](controller/internal/contract/) for the
duck-typed status contract it reads. VM Operator is a working first provider,
demonstrated managing real VM lifecycle (creation, address reporting,
power on/off) on a vSphere Supervisor cluster; see the one-pager's *Demos*
section.

The CRD has been verified end to end against a live Kubernetes API server:
it establishes, accepts a valid object, applies its defaults, renders its
printer columns, and rejects invalid input (missing `infrastructureRef`,
out-of-enum `powerState`, duplicate disk names).

[`ROADMAP.md`](ROADMAP.md) covers what comes next.

## Layout

```text
api/v1alpha1/         The API types
  common_types.go       Shared enums and reference types
  virtualmachine_types.go  VirtualMachine, its spec and status
  groupversion_info.go  Scheme registration
  zz_generated.deepcopy.go  Generated — do not edit
config/crd/bases/     Generated CRD manifests
config/samples/       Example manifests
controller/           The generic core controller
  internal/contract/    The duck-typed status contract read off a provider object
docs/                 Design docs for this API
hack/boilerplate/     License header used by code generation
hack/tools/bin/       Tooling installed by "make tools" (gitignored)
```

## Usage

The Makefile follows the parent vm-operator repo's conventions — same target
names, same grouped `make help`, same OS/arch-scoped tooling directory, and the
same podman-locally / docker-in-CI container runtime selection.

```bash
make help              # list targets, grouped
make generate          # generate-go (deepcopy) + generate-manifests (CRDs)
make build             # compile
make lint              # lint-go-full + lint-markdown + typos
make verify            # build + lint-go-full + verify-codegen
make verify-codegen    # fail if generated code is stale
make modules           # go mod tidy
```

Tooling is installed on demand into `hack/tools/bin/<os>_<arch>/`, pinned to the
same `controller-gen` and `golangci-lint` versions the parent repo uses so
generated output and lint results do not differ between trees.

`lint-markdown` and `typos` run from container images and default to this repo's
internal mirrors. Without access to that registry, override them with any
equivalent image — `make lint-markdown MDLINT_IMAGE=...` and
`make typos TYPOS_IMAGE=...`. These are the only targets that need registry
access; `build`, `lint-go-full`, `generate`, and `verify-codegen` do not.

To try the CRD against a cluster:

```bash
kubectl apply -f config/crd/bases/
kubectl apply -f config/samples/virtualmachine.yaml
```

The sample references an image, a network, and a provider object that will not
exist in an empty cluster. Without the corresponding provider object and a
running instance of `controller/`, the generic object will be accepted but
nothing will reconcile it.

## Known issues and open API questions

These are open questions worth settling before the API is proposed more
widely.

- **No accelerator field.** Accelerators are requested through
  `instanceType.name` rather than a dedicated field, because on vSphere and EC2
  an accelerator is inseparable from the class or instance type. A portable
  per-VM accelerator field waits for a Dynamic Resource Allocation shape that
  holds across providers. See the one-pager's non-goals.
- **`infrastructureRef` omits the API version** deliberately: the provider
  advertises which contract version it satisfies via a well-known label, and the
  core resolves the served version at runtime. Whether a label is the right
  mechanism, and what the canonical `providerID` form is, are both still open.
- **The provider contract is now expressed in code, but only for one provider
  so far.** [`controller/internal/contract/contract.go`](controller/internal/contract/contract.go)
  reads a fixed set of paths off a provider object's `status` —
  `providerID`, `powerState`, `addresses[]`, a free-form `providerMetadata`
  map, and the `InfrastructureReady` / `UpToDate` conditions — through
  `unstructured.Unstructured`, so the generic core never imports provider
  types. This settles the spec-vs-status question Cluster API leaves open for
  `providerID`: the contract reads it from **status**, precisely to avoid a
  controller writing its own object's spec and showing up as permanent drift
  in Argo CD and Flux. What is still open: there is only one provider
  (VM Operator) exercising the contract, and no conformance test yet holds a
  second provider to it.
- **Companion catalog types are not defined.** `bootDisk.source.image` and
  `network.interfaces[].network` reference `VirtualMachineImage` and network
  objects that this module does not define. Whether those become KubeVM types or stay
  provider-owned is intentionally deferred to community discussion.
- **Only one version, and it is `v1alpha1`.** No conversion machinery exists yet.
  The types are marked as the storage version so a second version can be added
  without a migration.

## Dependencies

Kubernetes dependencies are pinned deliberately low, matching the sibling
`github.com/vmware-tanzu/vm-operator/api` module. This is an API types module
meant to be imported by provider implementations, and a low floor maximizes the
range of Kubernetes versions a consumer can build against. Do not raise these to
match the root vm-operator module.
