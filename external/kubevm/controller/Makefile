# If you update this file, please follow
# https://suva.sh/posts/well-documented-programs

SHELL := /usr/bin/env bash

.DEFAULT_GOAL := help

GOHOSTOS := $(shell go env GOHOSTOS)
GOHOSTARCH := $(shell go env GOHOSTARCH)
GOHOSTOSARCH := $(GOHOSTOS)_$(GOHOSTARCH)

TOOLS_DIR     := hack/tools
TOOLS_BIN_DIR := $(TOOLS_DIR)/bin/$(GOHOSTOSARCH)

export PATH := $(abspath $(TOOLS_BIN_DIR)):$(PATH)
export KUBEBUILDER_ASSETS := $(abspath $(TOOLS_BIN_DIR))

GINKGO := $(TOOLS_BIN_DIR)/ginkgo

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } ' $(MAKEFILE_LIST)

.PHONY: manager
manager: ## Build the controller binary
	go build -o bin/manager .

.PHONY: test
test: $(GINKGO) ## Run tests
	$(MAKE) -C $(TOOLS_DIR) etcd kube-apiserver
	$(GINKGO) -v --keep-going -r ./...

.PHONY: lint
lint: ## Lint this module
	go vet ./...

$(GINKGO):
	$(MAKE) -C $(TOOLS_DIR) ginkgo
