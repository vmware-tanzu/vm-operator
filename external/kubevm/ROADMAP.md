# KubeVM Roadmap

These milestones are expected to change substantially once provider
implementations begin — the ordering reflects what the design has run into so
far, not a fixed plan.

## Today

The `kube-vm.io/v1alpha1` types generate CRDs and install and validate against a
live API server. There is no controller yet, the provider contract is prose
rather than code, and the companion catalog types are undefined.
[`README.md`](README.md) tracks the gaps.

## 1. A working generic core

- Generic controller: adopt the provider object, own finalizers and deletion
  ordering, read the duck-typed status back, maintain conditions.
- Provider contract expressed in code, including a precise definition of
  `status.ready`.
- Companion catalog types, starting with `VirtualMachineImage`, which the API
  already references.
- Settle the open API questions: who owns the provider object's spec,
  cross-namespace references, and per-instance accelerators.

## 2. A second provider

- A provider for Kubevirt, EC2 or GCE, following the shape `cluster-api-provider-aws`
  and `cluster-api-provider-gcp` etc. already use.
- A conformance suite defining what "supports KubeVM" means. Gated on a second
  provider existing.
- A provider authoring guide.

## 3. Orchestration above the machine

- `VirtualMachineTemplate` and `VirtualMachineSet`.
- `VirtualMachineService`, generalizing the one VM Operator already ships.
- `VirtualMachineDeployment`, which also lets an immutable field be applied by
  replacing the machine instead of failing the edit.
- A bootstrap provider contract, so cloud-init is not the only path — Windows
  guests on some platforms need another.

## 4. API and project maturity

- Capability tiers, plus provider feature discovery.
- `v1alpha2` and conversion machinery.
- Independent release cadence for the generic API and each provider, gated on an
  advertised contract version.
- Governance, maintainers, contribution guide, security disclosure, public
  backlog.

## Not on the roadmap

- Re-implementing live migration, high availability, or host placement. These
  stay with the provider that already has them.
- Cross-provider migration of a running machine or its disks.
- Being a hypervisor, or replacing KubeVirt — the VM-as-Pod model remains right
  where Kubernetes is the only infrastructure layer.
- A workload model spanning VMs and Pods: a direction worth exploring, not yet
  concrete enough to commit to.
