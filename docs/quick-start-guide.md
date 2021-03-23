# VM Operator Quick Start Guide

## Building the project

Building VM Operator is relatively straightforward. The typical workflow comprises:

- Check you have all the necessary dependencies in your environment (or use the Docker workflow)
- Build the tools for the platform you're developing
- Build the VM operator manager binary
- Run the unit tests against the binary
- Run the integration tests against the binary

### Development Environment

You can choose to develop locally or using Docker. The advantage of using Docker is that the
dependencies are all kept inside of Docker containers/images and are easier to clean up

#### Locally

Please ensure that you have the following installed to your system:

* Golang 1.15+
* Bash 4+
* The md5 and shasum hashing tools (ex. `md5sum`, `sha1sum`, `sha256sum`, etc.)

VM Operator build depends on a few tools which are managed in `/hack/tools`. You will need
to build the tools for the platform you're developing on the first time you clone VM Operator.
This is a one-time operation, after which your tools will be in `hack/tools/bin`. You can build
the tools by running:

`make tools`

#### Container

You can use Docker to make and test VM Operator. All of the necessary tools and modules will be
downloaded and built inside of a container image as a one-time operation.

`make docker-image`

This will create a Docker image in your local image cache which can be removed using

`make clean-docker`

Build and test commands are run using ephemeral containers with the local work directory
mounted inside of the container. This means that the output of the build and test commands
will be written locally and the build container is automatically cleaned up each time.

Note that if you're developing on a Mac, the binaries produced by the container build process
both in `/bin` and `/hack/tools/bin` will need to be regenerated if you want to switch to
a local build process.

### Build the manager

VM Operator is designed to run as a single binary in a container deployed as a pod in a Kubernetes
cluster. You can build this binary by running:

`make manager`

or if you're using the container workflow, you can run

`make manager-docker`

### Run the unit tests

VM Operator code is divided amongst core code and controller code, all of which has unit tests.
The unit tests run by default with code coverage enabled. You can run the unit tests with the command:

`make test`

or if you're using the container workflow, you can run

`make test-docker`

After running the command, the output of the test run will be in `unit-tests.out` and the coverage
data is in `cover.out`. If you don't want the code coverage, you can elect to run `make test-nocover`

### Run the integration tests

The integration tests can be run in a similar way to the unit tests. The integration tests included
in this repository currently don't build a container and try to deploy it to a Kubernetes cluster,
but this is an improvement we wish to make soon.

`make test-integration`

or if you're using the container workflow, you can run

`make test-integration-docker`

### Test coverage

If you're interested in a graphical interactive representation of the code coverage, you can run:

`make coverage` to run the unit tests or 

`make coverage-full` to run the unit and integration tests.

Both of these commands will bring up a browser that shows all of the lines covered by the tests.