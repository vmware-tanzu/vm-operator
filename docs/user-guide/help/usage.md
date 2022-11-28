# Usage

Report status, Initiate awesome...

---

## Overview

This page reviews how to use the Kubernetes CLI to interact with VM Operator:

```shell
kubectl
```

## Getting Help

To print the online help for VM Operator APIs, use the following command:

```shell
kubectl explain <API>.vmoperator.vmware.com
```

## Examples

This section illustrates several, common examples for using the Kubernetes CLI:

### Print the VM Operator version

This example shows how to print VM Operator's version:

`// TODO(akutz)`

### Restart VM Operator

`// TODO(akutz)`

### List all VMs across all namespaces

`// TODO(akutz)`

### Create a VM on Supervisor

Let's deploy a VM with VM Operator on vSphere Supervisor:

!!! note "note"

    The example below assumes:

    * an available `VirtualMachineClass` named `small`
    * an available `VirtualMachineImage` named `ubuntu-22.10`
    * an available `StorageClass` named `iscsi`
    * the user has `Editor` permissions on the namespace `my-namespace`

```shell
cat <<EOF | kubectl apply -f
# TODO(akutz)
EOF
```

### Attach a PersistentVolume

`// TODO(akutz)`

### Detach a PersistentVolume

`// TODO(akutz)`
