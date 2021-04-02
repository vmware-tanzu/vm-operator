# Sample Code

Sample code demonstrating how to use the vm-operator-api in your project

## Prerequisites

The sample code sets up a local Kubernetes cluster and populates it with vm-operator-api CRD types.
In order to successfully run the samples, these types need to be generated with `make generate-manifests`
in the project root. This is also done automatically if you run `make all` in the samples directory.

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
bin/list-ctrl
```