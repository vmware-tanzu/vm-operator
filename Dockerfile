ARG BASE_IMAGE=mirror.gcr.io/library/photon:5.0

# Copy the controller-manager into a thin image
FROM ${BASE_IMAGE}

## --------------------------------------
## Cleanup (Distroless image)
## --------------------------------------

RUN tdnf makecache && tdnf -y update && \
    rm /etc/tdnf/protected.d/tdnf.conf && tdnf -y autoremove photon-repos tdnf && \
    rm -rf /var/cache/tdnf /usr/lib/{rpm,tdnf} /usr/lib/sysimage/{rpm,tdnf} /etc/{rpm,tdnf}

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
