# Findings

Places where satisfying the KubeVM contract took a real design decision, and
why this provider decided the way it did. Recorded here rather than only in
code comments because `external/kubevm-provider-aws` established the
convention: a provider's own `docs/findings.md` is where the next provider
author looks first to see what already tripped someone up.

## No client-token idempotency, no propagation-grace window

`external/kubevm-provider-aws` generates a client token before calling
`RunInstances` and, after a create, waits out a `propagationGrace` window
(two minutes) before trusting a `DescribeInstances` miss as "really gone"
rather than "EC2 hasn't caught up yet." Both exist because EC2's
`RunInstances` can be retried without knowing whether the previous call
already launched an instance, and because `DescribeInstances` is eventually
consistent by tag (the identifying property AWS's create path has at hand
before an instance id exists) even though it is consistent by id.

Docker and podman have neither problem. The container name this provider
picks (`containerName`, `kubevm-<namespace>-<name>`) is the identifying key
from the very first call, the engine enforces its uniqueness itself, and
`docker create`/`run` returns synchronously with either an id or a refusal —
there is no window where the call has returned but the container's existence
is still unknown. `internal/container.Client.Create` calls the engine once,
by name, and an immediate `Inspect` by that same name is authoritative. See
`controllers/containermachine/observe.go`'s `reconcileContainer` and
`vanished` for where this shows up: a missing container is treated as
terminal on the spot, with no grace window to wait out.

## `Runtime` has no portable equivalent

`ContainerMachineSpec.Image` and `.PowerState` are resolved from the parent
`VirtualMachine` every reconcile by `persistResolved`, the same as AWS
resolves `AWSMachineSpec.AMI` and `.InstanceType` from fields on
`VirtualMachineSpec`. `Runtime` (docker vs. podman) has nothing to resolve:
`VirtualMachineSpec` has no field that means "which container engine." It is
set once, by whoever creates the `ContainerMachine`, and the controller never
touches it. This is the one field on this provider's spec that is genuinely
provider-only rather than a mirror of something portable — worth calling out
because it is easy to assume every provider-object field follows the
resolved-from-parent pattern, and this one deliberately doesn't.

## A single create-and-observe pass, not separate steps

AWS splits `create.go` and `observe.go` into separate reconcile steps because
`RunInstances` returning success does not mean the instance is visible yet —
the state has to be independently observed afterward, possibly on a later
reconcile. `reconcileContainer` in this provider does both in one call: after
`docker create`/`run` returns, the container's state is already knowable by
inspecting the same name, so there is no reason to defer that observation to
a following reconcile.

## A Create that succeeds can still look like a failure to the next reconcile

The claim above — "no window where the call has returned but the
container's existence is still unknown" — is true of the engine call
itself, but it is not the whole story, and live-cluster testing found the
gap: the engine call and the Kubernetes status write that records its
result are two separate operations, and only one of them is guaranteed by
the container name being a strongly consistent key.

Concretely: reconcile A calls `docker/podman run -d`, gets back a real
container id, and then fails to persist it — the status patch loses an
optimistic-lock race against, say, the core's `ensureOwnerReference` patch
landing on the same object first. Reconcile A returns that error. Reconcile
B retries from `machine.Status.ContainerID == ""`, calls Create again
against the same deterministic name, and the engine refuses: the name is
already in use, by the container reconcile A already made. Treating that
refusal as terminal (the original version of `reconcileContainer` did)
leaves the object stuck retrying a Create that can never succeed again.

The fix, and the thing `internal/container.Client.Create`'s own doc comment
already promised but `reconcileContainer` never implemented: a name
conflict is exactly the recoverable case, distinguishable from every other
Create failure (bad image, engine unreachable) by the engine's own error
text (`container.IsNameConflict`). On that specific error, `Inspect` the
same name instead of giving up — the container it finds is the one the
earlier, half-recorded Create call actually made.

This does mean any name conflict is treated as recoverable, including one
against a container a human created by hand under the same name — the
recovery path adopts it rather than reporting a hijack the way
`internal/link`'s two-sided check would for two Kubernetes objects. In
practice this is unlikely by construction: the name is
`kubevm-<namespace>-<name>`, a string nothing outside this provider has a
reason to pick. It is a real, if narrow, gap in the hijack protection this
provider otherwise takes seriously, not a case this fix specifically
considered and dismissed.

Confirmed by a regression test, not only by the one live run that found the
bug: `controllers/containermachine/observe_test.go`'s
`TestReconcileContainerRecoversFromNameConflict` scripts a Create refusal
followed by a successful Inspect and asserts `InfrastructureReady` still
lands `True`; `TestReconcileContainerCreateFailureIsTerminal` asserts a
non-conflict Create failure is still reported as `RuntimeError`, so the
recovery path cannot silently swallow every failure.

## An image name is not always a valid `ObjectReference.Name`

The guide's claim that this provider "reads the image reference
verbatim, the same way AWS reads an AMI id" is true of the controller, but
incomplete: `spec.bootDisk.source.image.name` is typed as an
`ObjectReference`, and the core's CRD validates that field's `name` as
DNS-subdomain-safe
(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`) before
this controller — or any controller — ever sees it. An AMI id
(`ami-0123456789abcdef0`) happens to satisfy that pattern. A real OCI image
reference usually does not: a tag (`nginx:latest`) has a colon, a registry
path (`docker.io/library/nginx`) has slashes, both rejected outright by the
API server at apply time. `config/samples/virtualmachine.yaml` uses a bare
`nginx` for exactly this reason — this provider cannot pin a tag or name a
registry through this field at all, which is a real limitation to call out
rather than paper over with a same-as-AMI comparison that only holds for
image references that happen to already look like a Kubernetes name.
