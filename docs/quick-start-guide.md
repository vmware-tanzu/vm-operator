# VM Operator Quick Start Guide

## Building the project

Building VM Operator is relatively straightforward. The typical workflow comprises:

- Check you have all the necessary dependencies in your environment (or use a container)
- Build the tools for the platform you're developing on
- Build the VM operator manager binary
- Run the unit tests against the binary
- Run the integration tests against the binary

### Development Environment

You can choose to develop locally or in an existing golang container.

#### Locally

Please ensure that you have the following installed to your system:

* Golang 1.15+
* Bash 4+
* The md5 and shasum hashing tools (ex. `md5sum`, `sha1sum`, `sha256sum`, etc.)

#### Container

You can build and test VM Operator using the standard golang container from DockerHub.
You can do this by bind-mounting your workspace into the container and either running it
with an interactive shell or running commands inside a paused container using `docker exec`

```
docker run --name vmop-build --rm -d -v <mypath>/vm-operator:/go/src/github.com/vmware-tanzu/vm-operator -w /go/src/github.com/vmware-tanzu/vm-operator golang:1.15 sleep infinity
docker exec vm-op build make manager
```
### Build the tools

VM Operator build depends on a few tools which are managed in `/hack/tools`. You will need
to build the tools for the platform you're developing on the first time you clone VM Operator.
This is a one-time operation, after which your tools will be in `hack/tools/bin`. You can build
the tools by running:

`make tools`

### Build the manager

VM Operator is designed to run as a single binary in a container deployed as a pod in a Kubernetes
cluster. You can build this binary by running:

`make manager`

### Run the unit tests

VM Operator code is divided amongst core code and controller code, all of which has unit tests.
The unit tests run by default with code coverage enabled. You can run the unit tests with the command:

`make test`

After running the command, the output of the test run will be in `unit-tests.out` and the coverage
data is in `cover.out`. If you don't want the code coverage, you can elect to run `make test-nocover`

### Run the integration tests

The integration tests can be run in a similar way to the unit tests. The integration tests included
in this repository currently don't build a container and try to deploy it to a Kubernetes cluster,
but this is an improvement we wish to make soon.

`make test-integration`

### Test coverage

If you're interested in a graphical interactive representation of the code coverage, you can run:

`make coverage` to run the unit tests or 

`make coverage-full` to run the unit and integration tests.

Both of these commands will bring up a browser that shows all of the lines covered by the tests.