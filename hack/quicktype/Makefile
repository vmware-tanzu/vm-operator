# Copyright (c) 2024 VMware, Inc. All Rights Reserved.
# SPDX-License-Identifier: Apache-2.0

# Required vars
SCHEMA_IN ?=
OUTPUT_GO ?=
TOOLS_DIR ?=
QUICK_DIR ?=
START_TYP ?=

# If you update this file, please follow
# https://suva.sh/posts/well-documented-makefiles

# Ensure Make is run with bash shell as some syntax below is bash-specific
SHELL := /usr/bin/env bash

.DEFAULT_GOAL := help

# Get the information about the platform on which the tools are built/run.
GOHOSTOS := $(shell go env GOHOSTOS)
GOHOSTARCH := $(shell go env GOHOSTARCH)
GOHOSTOSARCH := $(GOHOSTOS)_$(GOHOSTARCH)

# Directories.
TOOLS_BIN_DIR := $(TOOLS_DIR)/bin/$(GOHOSTOSARCH)
export PATH := $(abspath $(TOOLS_BIN_DIR)):$(PATH)

# Binaries.
QUICKTYPE   := $(QUICK_DIR)/node_modules/.bin/quicktype
GOIMPORTS   := $(TOOLS_BIN_DIR)/goimports

# Images.
BASE_IMAGE_BUILD        ?= node:22
BASE_IMAGE_WORK         ?= gcr.io/distroless/nodejs22-debian12
QUICKTYPE_IMAGE_NAME    := vmop-quicktype
QUICKTYPE_IMAGE_VERSION := latest
QUICKTYPE_IMAGE         ?= $(QUICKTYPE_IMAGE_NAME):$(QUICKTYPE_IMAGE_VERSION)
QUICKTYPE_IMAGE_RECEIPT := $(abspath $(QUICK_DIR)/.receipt-$(QUICKTYPE_IMAGE_NAME)-$(shell echo '$(BASE_IMAGE_BUILD)-$(BASE_IMAGE_WORK)' | md5sum | awk '{print $$1}'))

# CRI_BIN is the path to the container runtime binary.
ifeq (,$(strip $(GITHUB_RUN_ID)))
# Prefer podman locally.
CRI_BIN := $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null)
else
# Prefer docker in GitHub actions.
CRI_BIN := $(shell command -v docker 2>/dev/null || command -v podman 2>/dev/null)
endif
export CRI_BIN

# Select how to run quicktype.
QUICKTYPE_METHOD ?= local
ifeq (local,$(QUICKTYPE_METHOD))
ifeq (,$(shell command -v npm))
QUICKTYPE_METHOD ?= container
endif
endif

# Tooling binaries
GOIMPORTS := $(TOOLS_BIN_DIR)/goimports


## --------------------------------------
## Help
## --------------------------------------

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)


## --------------------------------------
## Tooling Binaries
## --------------------------------------

TOOLING_BINARIES := $(GOIMPORTS)
tools: $(TOOLING_BINARIES) ## Build tooling binaries
$(TOOLING_BINARIES):
	make -C $(TOOLS_DIR) $(@F)


## --------------------------------------
## Image
## --------------------------------------

$(QUICKTYPE_IMAGE_RECEIPT):
	$(CRI_BIN) build \
	  --build-arg "BASE_IMAGE_BUILD=$(BASE_IMAGE_BUILD)" \
	  --build-arg "BASE_IMAGE_WORK=$(BASE_IMAGE_WORK)" \
	  -t $(QUICKTYPE_IMAGE) \
	  -f $(abspath $(QUICK_DIR)/Dockerfile) \
	  $(abspath $(QUICK_DIR))
	  touch $(QUICKTYPE_IMAGE_RECEIPT)
image-build-quicktype: $(QUICKTYPE_IMAGE_RECEIPT)


## --------------------------------------
## Binaries
## --------------------------------------

quicktype: $(QUICKTYPE)
$(QUICKTYPE): $(QUICK_DIR)/package.json
	cd $(QUICK_DIR) && npm ci --user quicktype


## --------------------------------------
## Generate
## --------------------------------------

generate-schema: $(SCHEMA_IN)
generate-schema: ## Generate the schema

$(OUTPUT_GO): $(SCHEMA_IN) | $(GOIMPORTS)
ifeq (local,$(QUICKTYPE_METHOD))
$(OUTPUT_GO): | $(QUICKTYPE)
	$(QUICKTYPE) \
	  --src $(SCHEMA_IN) --src-lang schema \
	  --out $@ --lang go --package schema \
	  --top-level $(START_TYP)
	$(GOIMPORTS) -w $@
else
$(OUTPUT_GO): | $(QUICKTYPE_IMAGE_RECEIPT)
	$(CRI_BIN) run -it --rm \
	  -v $$(pwd):/output \
	  -v $(abspath $(SCHEMA_IN)):/schema.json \
	  $(QUICKTYPE_IMAGE) \
	  /quicktype/node_modules/quicktype/dist/index.js \
	  --src /schema.json \
	  --src-lang schema \
	  --out /output/schema.go \
	  --lang go \
	  --package schema \
	  --top-level $(START_TYP)
	mv -f schema.go $@
	$(GOIMPORTS) -w $@
endif

generate-go: $(OUTPUT_GO)
generate-go: ## Generate the go source code from the schema

## --------------------------------------
## Cleanup
## --------------------------------------

.PHONY: clean
clean: ## Run all the clean targets
	rm -f $(SCHEMA_IN) $(OUTPUT_GO) $(QUICKTYPE_IMAGE_RECEIPT)
