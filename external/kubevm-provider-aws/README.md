# kubevm-provider-aws

An AWS EC2 provider for [KubeVM](../kubevm/README.md), the vendor-neutral `kube-vm.io` Kubernetes API for virtual machine lifecycle.

## What this is

KubeVM splits VM lifecycle the way Cluster API splits cluster lifecycle: a portable core that knows about virtual machines in general, and providers that know about one platform each. A user writes a `kube-vm.io` `VirtualMachine`, and a provider turns it into a real machine.

The core is proven on vSphere, with VM Operator as the first provider. **This is the second provider**, the work [ROADMAP](../kubevm/ROADMAP.md) milestone 2 describes, and its purpose is as much evidence as software:

> Satisfy the published KubeVM contract unchanged, and produce evidence of where that contract does and does not generalise beyond vSphere.

Where EC2 cannot express something the contract asks for, that is **recorded as a finding against the core, not worked around**. See [`docs/findings.md`](docs/findings.md).

## Shape

One CRD, `AWSMachine`, one controller of its own, and no webhooks. A user writes two objects:

```yaml
apiVersion: kube-vm.io/v1alpha1
kind: VirtualMachine
metadata:
  name: web-01
  namespace: team-a
spec:
  powerState: PoweredOn
  instanceType:
    name: t3.micro
  bootDisk:
    source:
      image:
        apiGroup: infrastructure.kube-vm.io
        kind: AWSImage
        name: ami-0123456789abcdef0
  infrastructureRef:
    apiGroup: infrastructure.kube-vm.io
    kind: AWSMachine
    name: web-01
---
apiVersion: infrastructure.kube-vm.io/v1alpha1
kind: AWSMachine
metadata:
  name: web-01
  namespace: team-a
  annotations:
    kube-vm.io/virtual-machine: web-01
spec: {}
```

- **Both objects have to name each other, and the user writes both halves.** The `VirtualMachine` names the provider object in `spec.infrastructureRef`; the `AWSMachine` names it back with the annotation `kube-vm.io/virtual-machine: <name of the VirtualMachine>`. Name only one side and nothing happens: the core does not adopt, this provider does not launch, and no EC2 call is made at all — the object says which half is missing. The rule exists because a machine holds state: if one side were enough, anyone who can create a `VirtualMachine` could point it at a machine already running and take it over. The vSphere provider works the same way.
- **Deletion does not apply that rule, and that is the core's behaviour, not this provider's.** Deleting a `VirtualMachine` deletes whatever its `infrastructureRef` names, including an object it was never linked to — so a mistyped reference, which reads honestly as `NotAdopted`, becomes destructive as soon as somebody deletes the mistaken object to tidy up. See [`docs/findings.md`](docs/findings.md), F1.
- **The provider object's spec is empty on submission and never empty afterwards.** A create carrying any spec field is rejected by the CRD. The controller then writes what it resolved from the portable object: image, instance type, subnet, public-address preference and power state. Until the launch is requested, it writes the first three on every pass, so a value written in by hand reverts to what the portable object asks for; from then on a CRD rule fixes them. The public address and power state follow the portable object throughout and are applied to the running instance. So `kubectl get awsmachine -o yaml` shows what the machine will actually do, and the launch is built from that spec.
- **Status lands at the contract's fixed paths**: `addresses`, `powerState`, `providerID` (`aws:///<zone>/<instance-id>`), `providerMetadata` (the availability zone, and nothing else — see the note on the zone in `docs/findings.md`), and the `InfrastructureReady` and `UpToDate` conditions.

`RunInstances` has 44 parameters. A launch sends six, plus a network interface when a subnet or a public-address preference is asked for, and the user writes two of them.

## How it runs

One Deployment runs one manager, which hosts two controllers: this provider's `AWSMachine` controller, and the KubeVM core controller that adopts the `AWSMachine` and mirrors its status onto the `VirtualMachine`. The core runs in-process, the same way VM Operator hosts it on vSphere, so there is no second Deployment to install.

Two consequences follow:

- **The manager's `--sync-period` (default 10 minutes) bounds how stale the portable object can be, at about two periods.** EC2 sends no events, and the core watches only `VirtualMachine`, so a change made outside Kubernetes takes one resync to reach the `AWSMachine` and about one more for the core to copy it to the `VirtualMachine`. See finding F10, which also records the two ways to remove most of it: a runtime watch in the core, and, on this side, consuming EC2 state-change events rather than waiting for a resync.
- **One provider per cluster.** A second provider hosting its own copy of the core would run two cores against the same `VirtualMachine`s.

## Portability, stated honestly

Moving a manifest to another cloud changes **three** fields, not one: `infrastructureRef.kind`, `instanceType.name` and `bootDisk.source.image.name`. The manifest's *shape* travels; its *content* does not.

This is deliberate, and stands until the community agrees how to approach value portability. The API declines to decide: an image reference "may be a portable image type **or one the provider owns**". A second provider that invented its own naming layer would prove that layer works for AWS, not that one design serves both clouds.

## Credentials

Only ever through the AWS SDK's own credential chain: a Secret locally, IAM Roles for Service Accounts on EKS. **No API field names a credential**, and nothing resembling a key belongs in this module or in an image.

The Deployment reads an optional Secret named `aws-credentials`, so locally:

```shell
kubectl -n kubevm-provider-aws-system create secret generic aws-credentials \
  --from-literal=AWS_ACCESS_KEY_ID=… --from-literal=AWS_SECRET_ACCESS_KEY=…
```

It is optional because on EKS there is no Secret at all: delete the `envFrom` block and annotate the ServiceAccount with an IRSA role instead. Without either, the manager starts and reports healthy — the SDK does not check credentials until something calls EC2 — and every launch then fails with a condition saying so. The region comes from `AWS_REGION` in the Deployment, which takes precedence over anything in the Secret.

The controller calls six EC2 APIs and needs seven IAM actions: `ec2:RunInstances`, `ec2:DescribeInstances`, `ec2:TerminateInstances`, `ec2:StartInstances`, `ec2:StopInstances`, `ec2:ModifyNetworkInterfaceAttribute` (to add or remove a running instance's public address), and `ec2:CreateTags`. The last is needed because every launch tags the new instance with the object that owns it, and AWS checks `ec2:CreateTags` for tags applied at launch. It can be limited to launches with the condition `ec2:CreateAction` = `RunInstances`. A policy of exactly these seven actions has been run on its own against a live account, through a full lifecycle, with no call refused (`docs/ec2-notes.md` §19).

## Running it beside other things

Two names collide, and both matter on a shared cluster:

- **`AWSMachine`** is also the kind Cluster API's AWS provider ships, in `infrastructure.cluster.x-k8s.io`. On a cluster running both, `kubectl get awsmachines` resolves to whichever group discovery answers with first. Use `kubectl get awsmachines.infrastructure.kube-vm.io` in anything that has to be unambiguous.
- **`vm`** is claimed by the KubeVM `VirtualMachine`, by VM Operator's, and by others. The porting guide says the same: use fully qualified names in scripts and documentation.

The CRD needs a Kubernetes API server at **1.30 or newer**, or 1.28 with the `CRDValidationRatcheting` feature gate on. The rule that refuses a spec at create relies on `optionalOldSelf`, which an older server silently drops from the schema — the CRD still installs, and the guard is simply not there.

## Building and testing

`go.mod` resolves the KubeVM API and core controller modules from `../kubevm`, so this module builds inside this repository and the image builds with the repository root as its context.

```shell
make build          # compile
make test           # every test, against an in-memory EC2; never reaches AWS
make lint           # golangci-lint, markdown and typos
make image-build    # the manager image
make install deploy # both CRDs, then the manager, into KUBECONFIG's cluster
```

No automated test reaches AWS, and that is enforced by construction rather than by policy: tests run against an in-memory EC2 fake, and `internal/enforce` proves that no test binary links the code that builds a real client.

## Documentation

- [`docs/findings.md`](docs/findings.md): where the contract could not be satisfied, or says one thing and does another.
- [`docs/ec2-notes.md`](docs/ec2-notes.md): EC2 behaviour measured against a live account, not read from documentation.
- [`.sdd/specs/007-kubevm-provider-aws`](../../.sdd/specs/007-kubevm-provider-aws/): the spec, plan, tasks, data model and the research behind every design decision.
