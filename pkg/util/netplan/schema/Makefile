# © Broadcom. All Rights Reserved.
# The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
# SPDX-License-Identifier: Apache-2.0

TOOLS_DIR := ../../../../hack/tools
SCHEMA_IN := schema.json
OUTPUT_GO := netplan.go
QUICK_DIR := ../../../../hack/quicktype
START_TYP := Config

include $(QUICK_DIR)/Makefile

# Images.
RUST_IMAGE           ?= rust@sha256:96d0c6fc967aad12993be9592eb4a76c23268c4f8ff49dbe96b10226c267b712
SCHEMA_IMAGE_NAME    := vmop-netplan-schema
SCHEMA_IMAGE_VERSION := latest
SCHEMA_IMAGE         ?= $(SCHEMA_IMAGE_NAME):$(SCHEMA_IMAGE_VERSION)
SCHEMA_IMAGE_RECEIPT := .receipt-$(SCHEMA_IMAGE_NAME)-$(shell echo $(RUST_IMAGE) | md5sum | awk '{print $$1}')

$(SCHEMA_IMAGE_RECEIPT):
	$(CRI_BIN) build \
	  --build-arg BASE_IMAGE="$(RUST_IMAGE)" \
	  -t $(SCHEMA_IMAGE) \
	  -f Dockerfile \
	  .
	touch $(SCHEMA_IMAGE_RECEIPT)
image-build-schema: $(SCHEMA_IMAGE_RECEIPT)

$(SCHEMA_IN): | $(SCHEMA_IMAGE_RECEIPT)
$(SCHEMA_IN): ## Generate the schema
	$(CRI_BIN) run -it --rm -v $$(pwd):/output $(SCHEMA_IMAGE)
