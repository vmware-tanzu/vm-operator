# Go version used to build the binaries.
ARG GO_VERSION=1.21.5

## Docker image used to build the binaries.
FROM golang:${GO_VERSION} as builder


## --------------------------------------
## Multi-platform support
## --------------------------------------

ARG TARGETOS
ARG TARGETARCH


## --------------------------------------
## Environment variables
## --------------------------------------

ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}


## --------------------------------------
## Build information
## --------------------------------------

ARG BUILD_COMMIT
ARG BUILD_NUMBER
ARG BUILD_VERSION

ENV BUILD_COMMIT=${BUILD_COMMIT}
ENV BUILD_NUMBER=${BUILD_NUMBER}
ENV BUILD_VERSION=${BUILD_VERSION}


## --------------------------------------
## Configure the working directory
## --------------------------------------

WORKDIR /workspace


## --------------------------------------
## Copy in local sources
## --------------------------------------

# Copy the sources
COPY ./ ./


## --------------------------------------
## Build the binaries
## --------------------------------------

# Build
RUN make manager-only
RUN make web-console-validator-only


## --------------------------------------
## Create the manager image
## --------------------------------------

# Copy the controller-manager into a thin image
FROM gcr.io/distroless/static-debian11


## --------------------------------------
## Multi-platform support
## --------------------------------------

ARG TARGETOS
ARG TARGETARCH


## --------------------------------------
## Environment variables
## --------------------------------------

ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}


## --------------------------------------
## Build information
## --------------------------------------

ARG BUILD_BRANCH
ARG BUILD_COMMIT
ARG BUILD_NUMBER
ARG BUILD_VERSION


## --------------------------------------
## Image labels
## --------------------------------------

LABEL buildNumber=$BUILD_NUMBER commit=$BUILD_COMMIT branch=$BUILD_BRANCH version=$BUILD_VERSION


## --------------------------------------
## Copy the binaries from the builder
## --------------------------------------

WORKDIR /
COPY --from=builder /workspace/bin/manager .
COPY --from=builder /workspace/bin/web-console-validator .
USER nobody
ENTRYPOINT ["/manager"]
