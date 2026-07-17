ARG BASE_IMAGE=gcr.io/distroless/static-debian12:nonroot

# Copy the statically-linked iso-httpserver binary into a minimal image.
# The binary is built on the host (CGO_ENABLED=0) by `make iso-httpserver-only`
# before this image is built -- see the iso-httpserver-image-build target.
FROM ${BASE_IMAGE}

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
      name="VM Operator ISO Bootstrap HTTP Server" \
      vendor="Broadcom" \
      version="${BUILD_VERSION}"

## --------------------------------------
## Copy the binary from the build context
## --------------------------------------

WORKDIR /
COPY ./bin/iso-httpserver .
USER nonroot:nonroot
ENTRYPOINT ["/iso-httpserver"]
