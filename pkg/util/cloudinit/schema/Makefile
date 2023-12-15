# Copyright (c) 2023 VMware, Inc. All Rights Reserved.
# SPDX-License-Identifier: Apache-2.0

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
BIN_DIR       := bin
TOOLS_DIR     := ../../../../hack/tools
TOOLS_BIN_DIR := $(TOOLS_DIR)/bin/$(GOHOSTOSARCH)
export PATH := $(abspath $(BIN_DIR)):$(abspath $(TOOLS_BIN_DIR)):$(PATH)

# Binaries.
QUICKTYPE   := node_modules/.bin/quicktype
GOIMPORTS   := $(TOOLS_BIN_DIR)/goimports

# Schemas.
SCHEMA_CLOUD_CONFIG := schema-cloud-config-v1.json

# Output.
CLOUD_CONFIG_GO := cloudconfig.go

# Images.
QUICKTYPE_IMAGE_NAME    := vm-op-quicktype
QUICKTYPE_IMAGE_VERSION := latest
QUICKTYPE_IMAGE         ?= $(QUICKTYPE_IMAGE_NAME):$(QUICKTYPE_IMAGE_VERSION)

# Select how to run quicktype.
ifeq (,$(shell command -v npm))
QUICKTYPE_METHOD ?= docker
endif

# Binaries
MANAGER                := $(BIN_DIR)/manager
WEB_CONSOLE_VALIDATOR  := $(BIN_DIR)/web-console-validator

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

.PHONY: build-images-quicktype
build-images-quicktype:
	docker build -t $(QUICKTYPE_IMAGE) -f Dockerfile.quicktype .

.PHONY: build-images
build-images: ## Build the docker images
	$(MAKE) build-images-quicktype


## --------------------------------------
## Binaries
## --------------------------------------

quicktype: $(QUICKTYPE) ## Install quicktype
$(QUICKTYPE): package.json
	npm install --user quicktype


## --------------------------------------
## Generate
## --------------------------------------

$(CLOUD_CONFIG_GO): $(SCHEMA_CLOUD_CONFIG) | $(GOIMPORTS)
ifeq (docker,$(QUICKTYPE_METHOD))
$(CLOUD_CONFIG_GO): build-images-quicktype
	docker run -it --rm \
	  -v $$(pwd):/output \
	  -v $$(pwd)/$(SCHEMA_CLOUD_CONFIG):/schema.json \
	  $(QUICKTYPE_IMAGE)
	$(GOIMPORTS) -w $@
else
$(CLOUD_CONFIG_GO): | $(QUICKTYPE)
	$(QUICKTYPE) \
	  --src $(SCHEMA_CLOUD_CONFIG) --src-lang schema \
	  --out $@ --lang go --package schema
	$(GOIMPORTS) -w $@
endif

generate-go: ## Generate the go source code from the schemas
	$(MAKE) $(CLOUD_CONFIG_GO)


## --------------------------------------
## Cleanup
## --------------------------------------

.PHONY: clean
clean: ## Run all the clean targets
	rm -f cloudconfig.go

.PHONY: clobber
clobber: ## Remove all of the tooling as well
	$(MAKE) clean
	rm -fr $(BIN_DIR)
