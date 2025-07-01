# © Broadcom. All Rights Reserved.
# The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
# SPDX-License-Identifier: Apache-2.0

ARG BASE_IMAGE_BUILD=node:22
ARG BASE_IMAGE_WORK=gcr.io/distroless/nodejs22-debian12

FROM ${BASE_IMAGE_BUILD} AS build
WORKDIR /quicktype
COPY package.json package-lock.json* ./
RUN npm ci --prefix /quicktype quicktype

FROM ${BASE_IMAGE_WORK}
COPY --from=build /quicktype /quicktype
WORKDIR /output
