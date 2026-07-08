ARG BASE_IMAGE=mirror.gcr.io/library/photon:5.0

## --------------------------------------
## Build a minimal root filesystem (bottom-up)
## --------------------------------------

FROM ${BASE_IMAGE} AS installer

# Install just the FHS layout and the CA trust bundle by extracting the RPM
# payloads directly (--nodeps --downloadonly + rpm2cpio). A normal `tdnf
# install ca-certificates` pulls in bash/coreutils/glibc via the package's
# postinstall scriptlet; ca-certificates-pki ships the pre-built bundle with
# no scriptlet and no dependencies, which is all the static manager binary
# needs for its system cert pool.
RUN tdnf makecache && \
    tdnf install -y rpm && \
    mkdir /download && \
    tdnf install -y --nodeps --downloadonly --downloaddir=/download \
        --releasever=5.0 filesystem ca-certificates-pki && \
    mkdir /installroot && \
    cd /installroot && \
    for f in /download/*.rpm; do rpm2cpio "$f" | cpio -idmu; done

## --------------------------------------
## Copy the controller-manager into a thin image
## --------------------------------------

FROM scratch

## --------------------------------------
## Environment variables
## --------------------------------------

ARG TARGETOS
ARG TARGETARCH
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
## Minimal root filesystem (CA bundle only, no tdnf/rpm/shell)
## --------------------------------------

COPY --from=installer /installroot /


## --------------------------------------
## Copy the binaries from the builder
## --------------------------------------

WORKDIR /
COPY ./bin/manager .
COPY ./bin/web-console-validator .
# No /etc/passwd on the image
# we need to use UID instead of nobody
USER 65534
ENTRYPOINT ["/manager"]
