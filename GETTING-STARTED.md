# Get Started with vm-operator-api

This guide will walk you through the process of integrating vm-operator-api into your project

## Prerequisites

The vm-operator-api project currently requires a ESXi cluster in vSphere 7 with Kubernetes.

What this means in functional terms is that you can manage workloads in a given Workload Namespace using a Kubernetes client connected directly to the an embedded Kubernetes API Server running in the vSphere cluster. With the correct privileges, you can create CRD objects in your vSphere cluster which are reconciled into vSphere objects - everything from a single VM (`VirtualMachine`) to a full virtualized Kubernetes Cluster (`TanzuKubernetesCluster`)

The vm-operator-api APIs currently allow you to monitor VirtualMachine objects that exist in the target namespace and before long, it will also allow you to create and manage them.

## Step 1: Verify client access

Check your client access by viewing VirtualMachines in the target cluster

```bash
kubectl get VirtualMachines --all
```

## Step 2: Build and Test the sample code

There are examples using different clients. The generated client, that's created by the root makefile and the controller-runtime client that doesn't use generated client libraries.

```bash
cd hack/samples
make all
bin/list-gen
bin/list-ctrl
```

