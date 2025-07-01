# © Broadcom. All Rights Reserved.
# The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
# SPDX-License-Identifier: Apache-2.0

ARG BASE_IMAGE=rust@sha256:96d0c6fc967aad12993be9592eb4a76c23268c4f8ff49dbe96b10226c267b712

FROM ${BASE_IMAGE} AS build
WORKDIR /work
RUN git clone https://github.com/TobiasDeBruijn/netplan-types .
COPY main.rs src/
RUN cargo add serde_json
RUN cargo build --features schemars


FROM ${BASE_IMAGE}
COPY --from=build /work/ /work/
WORKDIR /work
CMD ["/bin/sh", "-c", "cargo run --features schemars >/output/schema.json"]