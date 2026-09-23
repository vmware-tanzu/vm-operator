# kubevm-provider-container

A KubeVM infrastructure provider that runs each `VirtualMachine` as a
docker or podman container. It exists to be the smallest complete example of
implementing the KubeVM contract — see
`external/kubevm/docs/implementing-a-provider.md` for the full walkthrough,
and [`docs/findings.md`](docs/findings.md) for the specific places where
this provider's design diverges from `external/kubevm-provider-aws`'s, and
why.

This is a teaching example, not a production provider: a container is not a
virtual machine, and this provider does not pretend it can do everything one
can. It satisfies the real KubeVM contract end to end with the smallest
amount of code that does, so a new provider author has something short to
read before tackling a real platform's SDK.

## How it works

Two objects, both created by hand, linked in both directions:

- A `VirtualMachine` (`kube-vm.io/v1alpha1`), naming this provider's object
  in `spec.infrastructureRef`.
- A `ContainerMachine` (`infrastructure.kube-vm.io/v1alpha1`), created with
  an empty spec and a `kube-vm.io/virtual-machine: <name>` annotation naming
  the `VirtualMachine` back.

See [`config/samples/virtualmachine.yaml`](config/samples/virtualmachine.yaml)
for a full pair. Both sides of the link are required before this provider
will create anything — see `internal/link`'s package comment for why a
one-sided reference is treated as unclaimed rather than adopted.

Once linked, the controller:

1. Reads the image reference and desired power state from the parent
   `VirtualMachine` and writes them into `ContainerMachine.spec` (so
   `kubectl get -o yaml` on the `ContainerMachine` shows what is actually
   running, not just what was asked for).
2. Creates the container by a deterministic name (`kubevm-<namespace>-<name>`)
   if it does not exist yet.
3. Starts or stops it to match the requested power state.
4. Reports `InfrastructureReady` and `UpToDate` conditions, the container's
   id as `status.providerID`, and its IP as an `InternalIP` address — the
   fields `external/kubevm/controller/internal/contract` reads back.
5. On delete, stops and removes the container before releasing its
   finalizer.

`ContainerMachineSpec.Runtime` (`docker` or `podman`) is the one field with
no counterpart on `VirtualMachineSpec`; it is set once by whoever creates the
object and the controller never overwrites it — see `docs/findings.md`.

## Running it

You need three things before applying the sample, in this order:

1. **A cluster.** Any cluster the KubeVM CRDs can be installed on works; this
   was tested against a throwaway [kind](https://kind.sigs.k8s.io/) cluster:

   ```sh
   kind create cluster --name kubevm-provider-test
   kubectl config use-context kind-kubevm-provider-test
   ```

2. **`docker` or `podman` on the machine the manager runs on** —
   `internal/container.ExecRunner` shells out to whichever binary
   `ContainerMachineSpec.Runtime` names (see "Why the manager runs as a
   local binary, not a Pod" below for why that machine is your workstation,
   not a node in the cluster).

3. **The image already pulled**, or a network path to pull it that actually
   works. `docker create`/`run` and `podman create`/`run` both pull an
   image they do not have locally, but that pull has no timeout of its own
   — it inherits whatever timeout the reconcile's context carries, which
   for a plain `go run ./cmd/manager` is none. On one test machine, `docker`
   (via Docker Desktop) hung indefinitely on this implicit pull, stuck
   inside a `docker-credential-desktop get` call that never returned —
   with no error, no timeout, and no log line, which silently wedges the
   controller's single worker forever (see `docs/findings.md`). Pre-pull
   explicitly and you sidestep the question entirely:

   ```sh
   docker pull nginx   # or: podman pull nginx
   ```

The sample's `VirtualMachine` and `ContainerMachine` also both live in a
`team-a` namespace that nothing here creates for you:

```sh
kubectl create namespace team-a

# Install the CRDs (this provider's, and KubeVM core's).
kubectl apply -f config/crd/
kubectl apply -f ../kubevm/config/crd/bases/

# Run the manager locally against your current kubeconfig context.
go run ./cmd/manager

# In another shell, create a linked pair.
kubectl apply -f config/samples/virtualmachine.yaml

# Watch it converge.
kubectl get containermachine web-01 -n team-a -o yaml
kubectl get virtualmachine web-01 -n team-a -o yaml
podman ps --filter name=kubevm-team-a-web-01   # or: docker ps --filter ...

# Delete both, and the container goes with them.
kubectl delete -f config/samples/virtualmachine.yaml
podman ps -a --filter name=kubevm-team-a-web-01   # gone, no leftover
```

A converged `VirtualMachine` shows `status.ready: true` and a real
`status.providerID`; the `ContainerMachine` shows the same plus the
container's `InternalIP` address. Deleting both removes the container and
releases both finalizers with no manual intervention — this whole sequence,
create through delete, was run against a real `kind` cluster to write this
section, not assumed from reading the code.

There is no webhook and no admission-time validation: an invalid or missing
image is reported on the `InfrastructureReady` condition after the fact, the
same as `external/kubevm-provider-aws`. One exception the CRD does enforce
up front: `spec.bootDisk.source.image.name` is validated as DNS-subdomain-safe
by the core's own schema (see the constitution's "Resource names must be DNS
subdomain safe"), which accepts a bare name like `nginx` but rejects a tag
(`nginx:latest`) or a registry path (`docker.io/library/nginx`) outright —
the apply fails before this controller ever sees it. See
[`config/samples/virtualmachine.yaml`](config/samples/virtualmachine.yaml)
and `docs/findings.md`.

### Why the manager runs as a local binary, not a Pod

It would be more realistic to deploy `config/manager/manager.yaml` and let
the cluster run this controller like any other. The obstacle is
`internal/container.ExecRunner`: it shells out to a `docker`/`podman` binary
on whatever machine the process is running on, and a kind node's container
runtime is containerd, not docker or podman — there is no engine to shell
out to from inside a Pod scheduled on that node without deliberately
mounting the host's docker socket into it (`-v /var/run/docker.sock:...`),
which means running that Pod privileged or with a hostPath mount most
clusters' PodSecurity policy already forbids, purely to reconstruct the
docker daemon this whole approach exists to talk to directly. That is a
real, well-known tradeoff (the "Docker-in-Docker" / sibling-containers
problem), not one this provider has solved — `config/manager/manager.yaml`
is here as the shape a real in-cluster Deployment would take, not as
something that works out of the box. Running the manager as a local binary
against the cluster's API server sidesteps the problem for a teaching
example by making "the machine with the engine" and "the machine running
the reconciler" the same machine, on purpose.

A provider driving a platform through a real SDK (`external/kubevm-provider-aws`,
talking to the EC2 API over the network) has no such problem and runs
in-cluster normally; this is specific to shelling out to a local CLI.

## Testing

```sh
go build ./...
go vet ./...
go test ./...
```

`internal/container` and `controllers/containermachine` are both driven
through the `Runner`/`Engine` seams (`internal/container.Runner`,
`Reconciler.Engine`), so none of `go test ./...` needs a real docker or
podman binary installed.

## Layout

```
api/v1alpha1/           ContainerMachine CRD types
internal/container/      docker/podman CLI client (Runner seam for tests)
internal/link/           two-sided VirtualMachine <-> ContainerMachine linkage check
controllers/containermachine/  the reconciler
controllers/controllers.go     hosts this controller and KubeVM core on one manager
cmd/manager/              the binary
config/                    CRD, RBAC, sample manifests, Deployment
docs/findings.md            design decisions and why they differ from AWS's
```
