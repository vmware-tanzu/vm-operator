# Build Reference

// TODO ([github.com/vmware-tanzu/vm-operator#118](https://github.com/vmware-tanzu/vm-operator/issues/118))

## Build with Docker

By far the simplest method for building VM Operator is with Docker.

### Docker Build Requirements

This project has very few build requirements, but there are still one or two items of which to be aware. Also, please note these are the requirements to *build* VM Operator, not run it.

Requirement | Version
------------|--------
Operating System | Linux, macOS
[Docker](https://www.docker.com/) | >=21.0

### Build the Container Image

The following one-line command is the quickest, simplest, and most deterministic approach to building the VM Operator container image:

```bash
make docker-build
```

## Build with Go

The other option is to build the VM Operator binaries directly with Golang.

### Go Build Requirements

Building VM Operator with Go has the following requirements:

Requirement | Version
------------|--------
Operating System | Linux, macOS
[Go](https://golang.org/) | >=1.19
[Git](https://git-scm.com/) | >= 2.0

### Build the Manager

The primary artifact for VM Operator is the `manager` binary:

```
make manager
```

### Build the Web Console Validator

The other artifact is the `web-console-validator` binary that is used on vSphere Supervisors to enable the web console feature via `kubectl`:

```
make web-console-validator
```
