# © Broadcom. All Rights Reserved.
# The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
# SPDX-License-Identifier: Apache-2.0

# Build context is the repository root, not this directory: go.mod resolves
# the KubeVM API and core controller modules from ../kubevm, which a build
# from here cannot see. `make image-build` passes the right context.
# --platform pins the builder to the machine doing the building, while the
# binary is built for the target below. Without it, a build on an arm64
# workstation produces an arm64 image that an amd64 cluster cannot run.
FROM --platform=$BUILDPLATFORM golang:1.24 AS builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /workspace

# The KubeVM modules first, because of the replace directives in go.mod.
COPY external/kubevm/ external/kubevm/
COPY external/kubevm-provider-aws/go.mod external/kubevm-provider-aws/go.sum \
     external/kubevm-provider-aws/

WORKDIR /workspace/external/kubevm-provider-aws
RUN go mod download

# Only what the manager is built from. Listed rather than `COPY . .`, so the
# image cannot pick up anything else from the repository.
COPY external/kubevm-provider-aws/api/ api/
COPY external/kubevm-provider-aws/cmd/ cmd/
COPY external/kubevm-provider-aws/controllers/ controllers/
COPY external/kubevm-provider-aws/internal/ internal/

# -trimpath keeps the building machine's paths out of the binary. No -a: with
# CGO disabled the standard library is already rebuilt as needed, and forcing
# it only discards the build cache.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o manager ./cmd/manager

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/external/kubevm-provider-aws/manager .
USER 65532:65532
ENTRYPOINT ["/manager"]
