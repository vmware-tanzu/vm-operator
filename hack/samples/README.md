# Sample Code

Sample code demonstrating how to use the vm-operator-api in your project

## Prerequisites

The sample code sets up a local Kubernetes cluster and populates it with vm-operator-api CRD types.
In order to successfully run the samples, these types need to be generated with `make generate-manifests`
in the project root. This is also done automatically if you run `make all` in the samples directory.

## Generated Client Samples

A clientset for the API can be generated using the Kubernetes tools `client-gen`, `informer-gen`
and `list-gen`. The clientset can be built by running `make generate-client` in the project root.
This is also done automatically if you run `make all` in the samples directory.

The compatibility of the generated clients varies between Kubernetes versions, so this is why generated
client code is not version controlled. If you want to use a generated client, you should do it yourself
in your own project based on the version of Kubernetes your project is using. You can use the
`hack/client-gen.sh` script as an example of how to do this

The community is moving away from generated clients in favor of dynamic or controller clients, but
the ability to generate clients is offered here for flexibility and compatibility.

## Controller Client Samples

The client in `sigs.k8s.io/controller-runtime/pkg/client` take a different approach to managing custom
resources. It is both dynamic and opinionated and it's what you will likely use if you build a controller
using kubebuilder. Note that it doesn't provide a Watch function as watching resources is a capability
inherent to a controller.

Note that the scheme for the vm-operator-api needs to be explicitly added to the client

## Build and run

Building and running the samples is really simple

```bash
cd hack/samples
make all
bin/list-gen
bin/list-ctrl
```