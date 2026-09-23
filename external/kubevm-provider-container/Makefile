# © Broadcom. All Rights Reserved.
# The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
# SPDX-License-Identifier: Apache-2.0

# This Makefile follows the conventions of the KubeVM modules beside it: the
# same target names (generate / generate-go / generate-manifests, lint,
# modules, verify), the same help mechanism, and the same OS/arch-scoped
# tooling directory, with tools installed by pinned version rather than
# through a hack/tools sub-module.
#
# `make test` never reaches a real docker or podman binary. internal/container
# and controllers/containermachine are both driven through a fake Runner, so
# ordinary `go test` covers this module with no container engine installed.

SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c

.DEFAULT_GOAL := help

# Directories.
ROOT_DIR      := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
REPO_ROOT     := $(abspath $(ROOT_DIR)/../..)
GOHOSTOS      := $(shell go env GOHOSTOS)
GOHOSTARCH    := $(shell go env GOHOSTARCH)
GOHOSTOSARCH  := $(GOHOSTOS)_$(GOHOSTARCH)
TOOLS_BIN_DIR := $(ROOT_DIR)/hack/tools/bin/$(GOHOSTOSARCH)

# Tooling binaries.
CONTROLLER_GEN := $(TOOLS_BIN_DIR)/controller-gen
GOLANGCI_LINT  := $(TOOLS_BIN_DIR)/golangci-lint

# Tool versions. Both match external/kubevm-provider-aws's, so the two
# providers regenerate and lint identically.
CONTROLLER_TOOLS_VERSION := v0.21.0
GOLANGCI_LINT_VERSION    := v2.1.6

# The license header applied to generated Go sources.
BOILERPLATE := ../kubevm/hack/boilerplate/boilerplate.generatego.txt

# The manager image, named as config/manager/manager.yaml names it. The build
# context is the repository root, because go.mod resolves the KubeVM modules
# from ../kubevm.
IMG ?= localhost/kubevm-provider-container:latest

# CRI_BIN is the path to the container runtime binary, used both to build the
# manager image and, at runtime, by internal/container to run the sample
# workload itself.
ifeq (,$(strip $(GITHUB_RUN_ID)))
CRI_BIN := $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null)
else
CRI_BIN := $(shell command -v docker 2>/dev/null || command -v podman 2>/dev/null)
endif
export CRI_BIN

## --------------------------------------
## Help
## --------------------------------------

##@ Help

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

## --------------------------------------
## Tooling binaries
## --------------------------------------

##@ Tooling

$(CONTROLLER_GEN):
	GOBIN=$(TOOLS_BIN_DIR) \
	go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

$(GOLANGCI_LINT):
	GOBIN=$(TOOLS_BIN_DIR) \
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

.PHONY: tools
tools: $(CONTROLLER_GEN) $(GOLANGCI_LINT) ## Build tooling binaries

## --------------------------------------
## Generate
## --------------------------------------

##@ Generate

.PHONY: generate
generate: ## Generate code
	$(MAKE) generate-go
	$(MAKE) generate-manifests

.PHONY: generate-go
generate-go: $(CONTROLLER_GEN)
generate-go: ## Generate golang sources
	$(CONTROLLER_GEN) \
		object:headerFile=$(BOILERPLATE) \
		paths=./api/...

.PHONY: generate-manifests
generate-manifests: $(CONTROLLER_GEN)
generate-manifests: ## Generate manifests e.g. CRD and RBAC
	$(CONTROLLER_GEN) \
		paths=./... \
		crd:crdVersions=v1 \
		rbac:roleName=manager-role \
		output:crd:dir=config/crd \
		output:rbac:dir=config/rbac

## --------------------------------------
## Modules
## --------------------------------------

##@ Modules

GO_MOD_OP ?= tidy

.PHONY: modules
modules: ## Validates the modules
	go mod $(GO_MOD_OP)

.PHONY: modules-download
modules-download: GO_MOD_OP=download
modules-download: modules ## Downloads and caches the modules

## --------------------------------------
## Build and test
## --------------------------------------

##@ Build and test

.PHONY: build
build: ## Compile the module
	go build ./...

.PHONY: manager
manager: ## Build the manager binary
	go build -o bin/manager ./cmd/manager

.PHONY: test
test: ## Run tests. Never reaches a real docker/podman binary
	go test -v ./...

## --------------------------------------
## Linting
## --------------------------------------

##@ Linting

GOLANGCI_LINT_FLAGS ?= --fast-only=false

.PHONY: lint
lint: lint-go ## Run all the lint targets

.PHONY: lint-go
lint-go: $(GOLANGCI_LINT)
lint-go: ## Lint codebase
	$(GOLANGCI_LINT) run -v $(GOLANGCI_LINT_FLAGS)

.PHONY: fix
fix: GOLANGCI_LINT_FLAGS = --fast-only=false --fix
fix: lint-go ## Tries to fix errors reported by lint-go

## --------------------------------------
## Verification
## --------------------------------------

##@ Verification

.PHONY: verify
verify: build lint-go verify-codegen ## Run static analysis and verify generated code

.PHONY: verify-codegen
verify-codegen: ## Verify generated code
	@# Snapshot, regenerate, and compare, so files git does not yet track are
	@# compared too.
	@tmp=$$(mktemp -d) && trap 'rm -rf "$$tmp"' EXIT && \
	cp -R api "$$tmp/api" && cp -R config "$$tmp/config" && \
	$(MAKE) --no-print-directory generate >/dev/null && \
	if ! diff -rq "$$tmp/api" api >/dev/null 2>&1 || ! diff -rq "$$tmp/config" config >/dev/null 2>&1; then \
		echo ""; \
		echo "ERROR: generated code is out of date. Run 'make generate' and commit the result."; \
		diff -ru "$$tmp/api" api || true; \
		diff -ru "$$tmp/config" config || true; \
		exit 1; \
	fi

## --------------------------------------
## Image and deployment
## --------------------------------------

##@ Image and deployment

.PHONY: image-build
image-build: ## Build the manager image
	$(CRI_BIN) build -t $(IMG) -f $(ROOT_DIR)/Dockerfile $(REPO_ROOT)

# Both CRDs: this provider's ContainerMachine, and the kube-vm.io
# VirtualMachine that the KubeVM core hosted in this manager reconciles.
.PHONY: install
install: ## Install the CRDs into the cluster named by KUBECONFIG
	kubectl apply -f ../kubevm/config/crd/bases/
	kubectl apply -f config/crd/

.PHONY: deploy
deploy: ## Deploy the manager into the cluster named by KUBECONFIG
	kubectl apply -f config/manager/namespace.yaml
	kubectl apply -f config/rbac
	kubectl apply -f config/manager/manager.yaml

.PHONY: undeploy
undeploy: ## Remove the manager
	kubectl delete --ignore-not-found -f config/manager/manager.yaml
	kubectl delete --ignore-not-found -f config/rbac
