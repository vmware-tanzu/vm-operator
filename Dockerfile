ARG BASE_IMAGE=gcr.io/distroless/base-debian12

# Copy the controller-manager into a thin image
FROM ${BASE_IMAGE}


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

LABEL branch="${BUILD_BRANCH}" \
      buildNumber="${BUILD_NUMBER}" \
      commit="${BUILD_COMMIT}" \
      name="VM Operator" \
      vendor="Broadcom" \
      version="${BUILD_VERSION}"


## --------------------------------------
## Copy the binaries from the builder
## --------------------------------------

WORKDIR /
COPY ./bin/manager .
COPY ./bin/web-console-validator .
USER nobody
ENTRYPOINT ["/manager"]
