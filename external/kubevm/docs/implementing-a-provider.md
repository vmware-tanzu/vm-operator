# Implementing a KubeVM provider

This is a from-scratch guide to writing a new KubeVM infrastructure
provider: a controller that turns a portable `VirtualMachine`
(`kube-vm.io/v1alpha1`) into a real thing on some platform — a vSphere VM,
an EC2 instance, a container, anything with a start/stop/observe-state
notion of its own.

If you are instead adapting an *existing* controller that already manages
platform objects, so that it also satisfies this contract, that is a
narrower job than what this guide covers; ask in the project's usual
channels for guidance specific to that case. This guide assumes you are
starting a provider that does not exist yet.

The running example throughout is
[`external/kubevm-provider-container`](../../kubevm-provider-container),
a real, minimal provider that runs each `VirtualMachine` as a docker or
podman container. It is deliberately the smallest provider that satisfies
the contract for real — short enough to read start to finish in one sitting,
and a working `go build`/`go test` if you want to run it. Everything below
points at specific files in it.

## The contract

The core never calls your platform's API and never creates your provider's
objects. It only does two things to any object your `VirtualMachine`'s
`spec.infrastructureRef` names:

- **Get** it, and read a fixed set of unstructured JSON paths off its
  `status` — never off its `spec`, and it writes nothing back.
- Otherwise leave it alone.

The paths it reads (see
[`controller/internal/contract/contract.go`](../controller/internal/contract/contract.go)
for the literal implementation) are:

| Path | Meaning |
|---|---|
| `status.addresses[]` (`{interface, type, address}`) | Network addresses the core surfaces on the `VirtualMachine`. |
| `status.powerState` | The platform's observed power state. |
| `status.providerID` | An opaque platform-specific id. |
| `status.providerMetadata` (`map[string]string`) | Free-form platform metadata. |
| `status.conditions[]` | Standard `metav1.Condition`s. Two types the core specifically looks for: `InfrastructureReady` and `UpToDate`. |

That's the entire surface. Your provider is free to define whatever
`spec` shape it wants — the core has no opinion about it, because the core
never reads it.

This has a consequence worth internalizing before you write any code:
**your `spec` is not a request channel the core drives.** Nothing tells your
controller what the user wants by writing to your object's spec, because
nothing but your own controller (and whoever creates the object once, by
hand) ever touches it. What the user wants lives on `VirtualMachine.spec` —
`powerState`, `bootDisk`, `instanceType`, and so on — and it is *your*
controller's job to read that and drive your object toward it. See "The
resolve-and-persist pattern" below.

## The two-sided link

A `VirtualMachine`'s `spec.infrastructureRef` (`{apiGroup, kind, name}`,
immutable after creation — see
[`api/v1alpha1/virtualmachine_types.go`](../api/v1alpha1/virtualmachine_types.go))
names your provider object. That is only half a link. Before your
controller does anything to the platform, it must also confirm that your
provider object itself points back — typically via an annotation your
convention defines, naming the `VirtualMachine`.

This is not incidental. Nothing in the core creates your provider object;
a human (or a higher-level tool) creates it by hand, with an empty spec,
alongside the `VirtualMachine`. A `VirtualMachine`'s `infrastructureRef` on
its own is just a claim, and treating a claim as authorization would let
anyone who can create a `VirtualMachine` point it at an infrastructure
object they don't own and have your controller act on it. Requiring the
provider object's own annotation to name the `VirtualMachine` back means
whoever created *that* object is the one who consented to the link.

`external/kubevm-provider-container`'s version of this check lives in
[`internal/link/link.go`](../../kubevm-provider-container/internal/link/link.go):
it reads the annotation, `Get`s the named `VirtualMachine`, and verifies its
`spec.infrastructureRef` names this object back, returning one of three
sentinel errors (`ErrNotLinked`, `ErrParentMissing`, `ErrNotMutual`) that
the controller turns into a `NotAdopted` condition rather than an error —
this is an expected, common state (the objects were just created and
haven't converged yet), not a failure.

## The resolve-and-persist pattern

Every reconcile, before touching the platform:

1. Read whatever fields are relevant off the parent `VirtualMachine`'s spec
   (image, power state, instance sizing, whatever your provider supports).
2. Write the resolved values into **your own object's spec**, patching only
   if changed.
3. Only then act on the platform, reading the desired state back off your
   own spec (not off the parent again).

Why bother copying values that already exist on the parent? Because your
object's spec is the one place `kubectl get -o yaml <your-object>` shows
what your controller is *actually* doing, right now — without this step,
debugging means cross-referencing two objects, and there is no field to
point to when someone asks "what image is this thing actually running."
`external/kubevm-provider-container`'s version is
`persistResolved` in
[`controllers/containermachine/controller.go`](../../kubevm-provider-container/controllers/containermachine/controller.go):
it copies the boot image (once — an existing container's image is fixed)
and the power state (every reconcile — that's the one thing meant to change
after creation) from the parent, then patches only if either actually
differs from what's already there.

Reading the image reference deserves a callout of its own:
`VirtualMachineSpec.BootDisk.Source.Image` is an `ObjectReference`
(`{apiGroup, kind, name}`), and it looks like it should resolve to some
other Kubernetes object. It doesn't have to. `external/kubevm-provider-aws`
reads `Image.Name` **verbatim** as the AMI id — `apiGroup`/`kind` are
required by the schema but resolve to nothing, because AWS has no
Kubernetes-native image catalog. `external/kubevm-provider-container` does
the same thing for OCI image references (`Image.Name` is read directly as
the argument to `docker create <name>`). Unless your platform genuinely
has a matching Kubernetes-native resource to resolve against, reading the
name directly is the established pattern — you do not need to invent an
image-catalog CRD.

One catch this pattern runs into for OCI images specifically, found by
actually running the container provider end to end rather than trusting
that it would work: `ObjectReference.Name` is validated server-side as a
DNS subdomain
(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`), which
an AMI id (`ami-0123456789abcdef0`) happens to satisfy but a real image
reference with a tag or registry path (`nginx:latest`,
`docker.io/library/nginx`) does not — the colon and the slashes are both
rejected at `kubectl apply` time, before your controller ever runs. "Read
it verbatim like an AMI id" is accurate for what the controller does with
the string; it is not a guarantee that the string you actually want to put
there will get past the API server. See
`external/kubevm-provider-container/docs/findings.md` for how its sample
manifest works around this (a bare, untagged image name), and whether your
own provider needs a different field shape if callers need to pin a tag.

## Fields with no portable equivalent

Not everything your provider needs has a matching field on
`VirtualMachineSpec`, and that's fine. The rule that separates the two
kinds of field:

- **Resolved from the parent, every reconcile** — anything with a portable
  equivalent (image, power state). Your controller owns writing these; a
  user editing them directly on your object gets overwritten on the next
  reconcile, because the parent is the source of truth.
- **Provider-only, set once by whoever creates the object** — anything with
  no portable equivalent. Your controller never touches these.

`external/kubevm-provider-container`'s `ContainerMachineSpec.Runtime`
(`docker` or `podman`) is the second kind: nothing on `VirtualMachineSpec`
means "which container engine," so it's set once at creation time and the
controller leaves it alone. If your provider has a field like this, say so
explicitly in a comment on the field — it's easy for a reader to assume
every spec field on a provider object follows the resolved-from-parent rule,
and the ones that don't are worth a sentence explaining why.

## Run the core inside your own manager

The core does not run as a separate deployment your provider talks to over
the network. It's a Go package
([`controller/controllers/virtualmachine`](../controller/controllers/virtualmachine))
whose `AddToManager` you call on the **same** `ctrl.Manager` your
provider's own controller runs on — one binary, one process, one client
cache, watching both API groups.

Concretely, that means your manager's scheme needs **both** groups
registered: your provider's own, and `kube-vm.io`'s. Skipping the second
one is a mistake that won't show up until runtime, and it shows up in a
confusing place — your controller's first `Get` of the parent
`VirtualMachine` fails with a scheme error that looks like a permissions
problem rather than a missed `AddToScheme` call. See
`external/kubevm-provider-container`'s
[`cmd/manager/main.go`](../../kubevm-provider-container/cmd/manager/main.go)
`init()` for both registrations side by side, and
[`controllers/controllers.go`](../../kubevm-provider-container/controllers/controllers.go)
for calling both `AddToManager`s on one manager.

Your controller also needs its own watch on `VirtualMachine`, mapped to
your object, so that an edit to the portable object (a power-state flip,
say) reaches your controller promptly rather than waiting for your next
sync period. See `machineForVirtualMachine` in
[`controllers/containermachine/controller.go`](../../kubevm-provider-container/controllers/containermachine/controller.go).

## How much of the AWS provider's depth do you need?

`external/kubevm-provider-aws` is the other real example in this repo, and
it is considerably more involved than the container provider: a six-file
controller split (create / observe / power / addresses / delete /
controller), client-token idempotency for `RunInstances`, and a two-minute
propagation-grace window before a `DescribeInstances` miss is trusted as
"really gone."

None of that is contract overhead — it's there because EC2 genuinely has
those problems: `RunInstances` can be retried without knowing whether a
previous call already launched an instance, and `DescribeInstances` is
eventually consistent by tag but not immediately by id. A platform with
different consistency properties doesn't need the same machinery. Docker
and podman don't have either problem — a container's name is a
caller-chosen, engine-enforced-unique key from the first call, and
`create`/`run` returns synchronously with either an id or a refusal — so
`external/kubevm-provider-container` has neither idempotency machinery nor
a grace window, and one file rather than six. See
[`external/kubevm-provider-container/docs/findings.md`](../../kubevm-provider-container/docs/findings.md)
for the specific reasoning.

The takeaway: match the depth of your provider to your platform's actual
consistency and idempotency properties, not to what AWS's provider happens
to do. If your platform's create call is synchronous and its lookup is
strongly consistent by the key you use to create things, you likely need
neither piece of that machinery.

## Dev/test

**No test needs the real platform.** Both example providers put the
platform SDK behind a small interface (`external/kubevm-provider-aws`'s
`ec2.Client`, `external/kubevm-provider-container`'s `container.Runner`)
and inject a fake in tests. Structure your own provider so `go test ./...`
never needs live credentials or a live engine — see
`external/kubevm-provider-container/internal/container/client_test.go`
for the pattern: a `fakeRunner` that answers a scripted sequence of CLI
calls.

**The core can't watch an arbitrary provider GVK.** Since your provider
object's Kind isn't known to the core at compile time, the core polls
rather than watches it while waiting on a transitional state, using a fixed
requeue delay (`pollRequeueDelay` in
[`controller/controllers/virtualmachine`](../controller/controllers/virtualmachine)).
Mirror that same requeue-delay pattern in your own controller for your own
transitional states (see `pollRequeueDelay` in
`external/kubevm-provider-container/controllers/containermachine/controller.go`)
rather than trying to make everything watch-driven — some latency between a
platform state change and your object's status catching up is expected,
and tests that assert on state should poll (`Eventually`, not
`time.Sleep`) rather than assume instant convergence.

**The linkage annotation is the most common thing to forget in a manual
test.** If your provider object never leaves the `NotAdopted` condition
during local testing, check the annotation before checking anything else —
forgetting to set it, or getting the two names swapped, produces exactly
the same symptom as an actual conflict.

**Running against a real or kind cluster:** apply both CRDs (yours and
`kube-vm.io`'s `VirtualMachine`), run your manager binary against your
current kubeconfig context (`go run ./cmd/manager`, no need to containerize
it for local iteration), then create the linked pair of objects by hand and
watch them converge with `kubectl get -o yaml`. See
`external/kubevm-provider-container/README.md`'s "Running it" section for
the exact commands this looks like end to end.

## Walkthrough: the container provider

Reading order, if you want to see all of the above in one small codebase:

1. [`api/v1alpha1/containermachine_types.go`](../../kubevm-provider-container/api/v1alpha1/containermachine_types.go) —
   the `ContainerMachine` type: an empty-by-convention spec
   (`image`, `runtime`, `powerState`), and a status shaped to satisfy the
   contract table above exactly.
2. [`internal/link/link.go`](../../kubevm-provider-container/internal/link/link.go) —
   the two-sided link check, in full, in under 60 lines.
3. [`internal/container/client.go`](../../kubevm-provider-container/internal/container/client.go) —
   the platform seam: a `Runner` interface wrapping `docker`/`podman`
   CLI invocations, with a real `ExecRunner` and a test fake.
4. [`controllers/containermachine/controller.go`](../../kubevm-provider-container/controllers/containermachine/controller.go) —
   `Reconcile`, the link-check dispatch, `persistResolved`, the
   optimistic-locked patch helpers, and delete/finalizer handling.
5. [`controllers/containermachine/observe.go`](../../kubevm-provider-container/controllers/containermachine/observe.go) —
   create-then-observe, applying power, and setting the
   `InfrastructureReady`/`UpToDate` conditions the contract reads.
6. [`controllers/controllers.go`](../../kubevm-provider-container/controllers/controllers.go)
   and [`cmd/manager/main.go`](../../kubevm-provider-container/cmd/manager/main.go) —
   hosting this controller and the KubeVM core on one manager.
7. [`config/samples/virtualmachine.yaml`](../../kubevm-provider-container/config/samples/virtualmachine.yaml) —
   the linked object pair a user actually applies.
8. [`docs/findings.md`](../../kubevm-provider-container/docs/findings.md) —
   why this provider's design differs from AWS's, in the specific places it
   does.

The whole module is a few hundred lines across those files, builds and
`go vet`s clean, and its tests run with no container engine installed.
That's the point: this is what "just enough to satisfy the contract" looks
like, so you have something concrete to diff your own provider's design
against.
