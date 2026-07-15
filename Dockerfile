ARG BASE_IMAGE=mirror.gcr.io/library/photon:5.0

## --------------------------------------
## Build a minimal root filesystem (bottom-up)
## --------------------------------------

FROM ${BASE_IMAGE} AS installer

RUN mkdir -p /installroot

# For distroless, we just need the files from filesystem,
# ca-certificates-pki, glibc, and glibc-libs, which are
# pre-installed on most curated base images including
# internal+external Photon builds. 
# glibc/glibc-libs are required because manager is 
# built internally with CGO_ENABLED=1 and
# GOEXPERIMENT=boringcrypto for FIPS compliance
# so it is dynamically linked against libc, not static.
# Copy their files straight out of the live root
# filesystem using tdnf's own package queries, which
# needs no extra package and no network access.

RUN set -eo pipefail; \
    for pkg in filesystem ca-certificates-pki glibc glibc-libs; do \
            tdnf --disablerepo="*" list installed "$pkg" >/dev/null 2>&1 \
                # if any missing package, fail the build
                || { echo "missing package: $pkg" >&2; exit 1; }; \
            tdnf --disablerepo="*" repoquery --installed --list "$pkg" \
                | while read -r f; do if [ -e "$f" ]; then echo "$f"; fi; done \
                | cpio -pdm /installroot; \
    done

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
## Minimal root filesystem (CA bundle + glibc, no tdnf/rpm/shell)
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
USER 65534:65534
ENTRYPOINT ["/manager"]
