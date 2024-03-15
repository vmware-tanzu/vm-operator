# VirtualMachineService

A `VirtualMachineService` is a method for exposing a network application that is running as one or more virtual machines (VM).

A key aim of `VirtualMachineService` is to make service discovery for VM workloads as simple as it is for pod workloads via the Kubernetes `Service` API.

!!! note "Kubernetes `Service` and VM Operator `VirtualMachineService`"

    For all intents and purposes, the `VirtualMachineService` API is the same as the Kubernetes `Service` API, with the primary difference being the former selects VM workloads instead of pods.

    This page documents the differences between the `VirtualMachineService` and `Service` APIs, and it is highly recommended to read the Kubernetes [documentation](https://kubernetes.io/docs/concepts/services-networking/service/) for the `Service` API before proceeding.


## `VirtualMachineService` API

The `VirtualMachineService` API, helps expose groups of VMs over a network. Each `VirtualMachineService` object defines a logical set of VM endpoints along with a policy about how to make those VMs accessible. Consider a client that needs to communicate with a web service, whose back-end servers may change. The client should not need to updated just because one of the back-end servers is replaced.

The `VirtualMachineService` abstraction enables this decoupling.


## Defining a VirtualMachineService

A `VirtualMachineService` is an <abbr title="An entity in the Kubernetes system, representing part of the state of the cluster.">object</abbr> (the same way that a `VirtualMachine` or a `ConfigMap` is an object). It is possible to create, view or modify `VirtualMachineService` definitions using the Kubernetes API, either directly or with a tool such as `kubectl`.

For example, suppose there is a set of VMs that each listen on port 9376 and are labelled as `app.kubernetes.io/name=my-app`. The following `VirtualMachineService` may be defined to publish the TCP listener:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha2
kind: VirtualMachineService
metadata:
  name: my-vm-service
spec:
  selector:
    app.kubernetes.io/name: my-app
  ports:
  - protocol: TCP
    port: 80
    targetPort: 9376
```

Applying this manifest creates a new `VirtualMachineService` named "my-vm-service" with the default ClusterIP [service type](#service-type). The `VirtualMachineService` targets TCP port 9376 on any VM with the `app.kubernetes.io/name: my-app` label.

The controller for the `VirtualMachineService` reconciles the resource and creates a [selectorless](https://kubernetes.io/docs/concepts/services-networking/service/#services-without-selectors) `Service` resource and `Endpoints` resource with the same name as the `VirtualMachineService` resource, in the same namespace. Then the controller continuously scans for `VirtualMachine` resources that match the selector, and makes the necessary updates to `Endpoints` resource. 


## Service type

Some parts of applications may need to be exposed via an external IP address, accessible from outside the Kubernetes cluster. There are several different types of services:

Supported
: [ClusterIP](#type-clusterip)
  : Exposes the `VirtualMachineService` on a cluster-internal IP. Choosing this value makes the `VirtualMachineService` only reachable from within the cluster. This is the default if a type is not explicitly specified.You can expose the Service to the public internet using an Ingress or a Gateway.

: [LoadBalancer](#type-loadbalancer)
  : Exposes the `VirtualMachineService` externally using a load balancer.

Unsupported
: [NodePort](#type-nodeport)
  : Exposes a port on each of the cluster's nodes for each of the ports defined as part of a service.

: [ExternalName](#type-externalname)
  : Instead of selecting workloads, maps to a DNS name with the `spec.externalName` parameter.

The `type` field in the `VirtualMachineService` API is designed as nested functionality - each level adds to the previous. For example, a `VirtualMachineService` resource of type `LoadBalancer` also has a ClusterIP.

### Supported

#### `type: ClusterIP`

The default type for a `VirtualMachineService`, an IP address is assigned from a pool on the cluster reserved for that purpose.

Several of the other types of `VirtualMachineService` build on the `ClusterIP` type as a foundation.

If a `VirtualMachineService` has the `.spec.clusterIP` set to "None", then no IP address is assigned. Please see [headless services](https://kubernetes.io/docs/concepts/services-networking/service/#headless-services) for more information.

!!! note "Network topologies and `type: ClusterIP`"

    A `VirtualMachineService` of `type: ClusterIP` is only valid when VMs are running on a Kubernetes cluster whose networking topology allows the control plane nodes to access the workload networks directly, such as the basic networking model for VMware vSphere Supervisor. In this model, pods deployed to the cluster can access the VM workloads via a `VirtualMachineService` via its cluster IP. However, not all networking topologies allow the control plane nodes direct network access to the workload networks to which VMs may be connected.


#### `type: LoadBalancer`

In clusters with support for external load balancers, setting the `type` field to `LoadBalancer` provisions a load balanced IP address for a `VirtualMachineService`. The actual creation of the load balanced IP happens asynchronously, and information about the provisioned IP address is published in the `VirtualMachineService`'s `.status.loadBalancer` field. For example:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha2
kind: VirtualMachineService
metadata:
  name: my-vm-service
spec:
  selector:
    app.kubernetes.io/name: my-app
  ports:
  - protocol: TCP
    port: 80
    targetPort: 9376
  type: LoadBalancer
status:
  loadBalancer:
    ingress:
    - ip: 192.168.0.2
```

!!! note "Explicit load balancer IP address"

    The field `spec.loadBalancerIP` was used to request an explicit IP address from the load balancer. However, this field was deprecated in Kubernetes 1.24. Still, if the field is set in a `VirtualMachineService`, the value will be copied to the underlying `Service` resource.


### Unsupported

The following service types are *not* supported by a `VirtualMachineService`:

#### `type: NodePort`

The Kubernetes service type [`NodePort`](https://kubernetes.io/docs/concepts/services-networking/service/#type-nodeport) is not supported by the `VirtualMachineService` API, nor are node ports allocated for ports exposed via a `VirtualMachineService` resource.

This is because VM workloads do not share the same networking stack as the nodes (ESXi hosts) on which the VMs are scheduled. Therefore, unless an ESXi host is configured to have access to the same network where a VM may be running, the ESXi host is unable to access that VM over the network.


#### `type: ExternalName`

The `VirtualMachineService` API also does not support type [`ExternalName`](https://kubernetes.io/docs/concepts/services-networking/service/#externalname). This type maps a service to the contents of the `externalName` field (for example, to the hostname `api.foo.bar.example`). The mapping configures the cluster's DNS server to return a `CNAME` record with that external hostname value. If this type of service is required, simply create a `Service` resource directly instead of using a `VirtualMachineService`.
