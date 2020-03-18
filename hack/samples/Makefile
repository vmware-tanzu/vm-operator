# If you update this file, please follow
# https://suva.sh/posts/well-documented-makefiles

# Ensure Make is run with bash shell as some syntax below is bash-specific
SHELL := /usr/bin/env bash

.DEFAULT_GOAL := help

# Active module mode, as we use go modules to manage dependencies
export GO111MODULE := on

# Directories.
BIN_DIR := bin

HOST_OS=$(shell go env GOOS)
HOST_ARCH=$(shell go env GOARCH)

# Binaries.
LIST_GEN  := $(BIN_DIR)/list-gen
LIST_CTRL  := $(BIN_DIR)/list-ctrl

.PHONY: all
all: prereqs list-gen list-ctrl ## Build all the samples

prereqs:
	cd ../../; $(MAKE) generate-manifests

## --------------------------------------
##@ Help
## --------------------------------------

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

## --------------------------------------
##@ Binaries
## --------------------------------------

.PHONY: $(LIST_GEN)
list-gen: prereqs $(LIST_GEN) ## Build list samnple with generated client
$(LIST_GEN): go.mod
	cd ../../; $(MAKE) generate-client
	go build -o $@ github.com/vmware-tanzu/vm-operator-api/hack/samples/generated

.PHONY: $(LIST_CTRL)
list-ctrl: prereqs $(LIST_CTRL) ## Build list sample with controller client
$(LIST_CTRL): go.mod
	go build -o $@ github.com/vmware-tanzu/vm-operator-api/hack/samples/controller

## --------------------------------------
##@ Cleanup
## --------------------------------------

.PHONY: clean
clean: ## Run all the clean targets
	$(MAKE) clean-bin

.PHONY: clean-bin
clean-bin: ## Remove all generated binaries
	rm -rf bin
