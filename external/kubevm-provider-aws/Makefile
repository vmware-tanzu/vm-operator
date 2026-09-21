# © Broadcom. All Rights Reserved.
# The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
# SPDX-License-Identifier: Apache-2.0

# This Makefile follows the conventions of the KubeVM modules beside it: the
# same target names (generate / generate-go / generate-manifests, lint /
# lint-go-full, modules, verify-codegen), the same help mechanism, and the same
# OS/arch-scoped tooling directory, with tools installed by pinned version
# rather than through a hack/tools sub-module.
#
# `make test` never reaches AWS. Every test runs against an in-memory EC2
# fake, and internal/enforce proves by construction that no test binary can
# build a real client.

SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c

.DEFAULT_GOAL := help

# Directories.
ROOT_DIR      := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
REPO_ROOT     := $(abspath $(ROOT_DIR)/../..)
GOHOSTOS      := $(shell go env GOHOSTOS)
GOHOSTARCH    := $(shell go env GOHOSTARCH)
GOHOSTOSARCH  := $(GOHOSTOS)_$(GOHOSTARCH)
TOOLS_DIR     := hack/tools
TOOLS_BIN_DIR := $(ROOT_DIR)/$(TOOLS_DIR)/bin/$(GOHOSTOSARCH)

# Tooling binaries.
CONTROLLER_GEN := $(TOOLS_BIN_DIR)/controller-gen
GOLANGCI_LINT  := $(TOOLS_BIN_DIR)/golangci-lint
GINKGO         := $(TOOLS_BIN_DIR)/ginkgo
SETUP_ENVTEST  := $(TOOLS_BIN_DIR)/setup-envtest

# Tool versions. controller-gen and golangci-lint match the KubeVM API module;
# ginkgo matches go.mod; the envtest API server matches the KubeVM core.
CONTROLLER_TOOLS_VERSION := v0.21.0
GOLANGCI_LINT_VERSION    := v2.1.6
GINKGO_VERSION           := v2.19.0
SETUP_ENVTEST_VERSION    := release-0.19
ENVTEST_K8S_VERSION      := 1.34.1

# The license header applied to generated Go sources.
BOILERPLATE := ./hack/boilerplate.go.txt

# The manager image, named as config/manager/manager.yaml names it. The build
# context is the repository root, because go.mod resolves the KubeVM modules
# from ../kubevm.
IMG ?= localhost/kubevm-provider-aws:latest

# CRI_BIN is the path to the container runtime binary. Same selection logic as
# the parent repo: podman locally, docker in GitHub actions.
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

$(GINKGO):
	GOBIN=$(TOOLS_BIN_DIR) \
	go install github.com/onsi/ginkgo/v2/ginkgo@$(GINKGO_VERSION)

$(SETUP_ENVTEST):
	GOBIN=$(TOOLS_BIN_DIR) \
	go install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(SETUP_ENVTEST_VERSION)

TOOLING_BINARIES := $(CONTROLLER_GEN) $(GOLANGCI_LINT) $(GINKGO) $(SETUP_ENVTEST)

.PHONY: tools
tools: $(TOOLING_BINARIES) ## Build tooling binaries

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
		rbac:roleName=kubevm-provider-aws-manager-role \
		output:crd:dir=config/crd/bases \
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
# Never reaches AWS. It does reach GitHub the first time: setup-envtest
# downloads the API server and etcd binaries into hack/tools/bin, then uses
# the local copy on every later run.
test: $(GINKGO) $(SETUP_ENVTEST) ## Run tests. Never reaches AWS
	KUBEBUILDER_ASSETS="$$($(SETUP_ENVTEST) use -p path \
		--bin-dir $(TOOLS_BIN_DIR) $(ENVTEST_K8S_VERSION))" \
	$(GINKGO) -v --keep-going -r ./...

## --------------------------------------
## Linting
## --------------------------------------

##@ Linting

GOLANGCI_LINT_FLAGS ?= --fast-only=true

.PHONY: lint
lint: ## Run all the lint targets
	$(MAKE) lint-go-full
	$(MAKE) lint-markdown
	$(MAKE) typos

.PHONY: lint-go
lint-go: $(GOLANGCI_LINT)
lint-go: ## Lint codebase
	$(GOLANGCI_LINT) run -v $(GOLANGCI_LINT_FLAGS)

.PHONY: lint-go-full
lint-go-full: GOLANGCI_LINT_FLAGS = --fast-only=false
lint-go-full: lint-go ## Run slower linters to detect possible issues

.PHONY: fix
fix: GOLANGCI_LINT_FLAGS = --fast-only=false --fix
fix: lint-go ## Tries to fix errors reported by lint-go-full target

# The same container images, and the same overrides, as the KubeVM API
# module's lint-markdown and typos targets.
MDLINT_IMAGE ?= lifeguard-docker-local.packages.vcfd.broadcom.net/github-runner/mdlint-alpine:0.44.0
TYPOS_IMAGE  ?= lifeguard-docker-local.packages.vcfd.broadcom.net/github-runner/typos-cli:1.40.0

.PHONY: lint-markdown
lint-markdown: ## Lint the project's markdown
	$(CRI_BIN) run --rm -v "$$(pwd)":/build $(MDLINT_IMAGE)

.PHONY: typos
typos: ## Detect typos
	$(CRI_BIN) run --rm -t -v "$$(pwd)":/$$(pwd):ro $(TYPOS_IMAGE) /$$(pwd)

## --------------------------------------
## Verification
## --------------------------------------

##@ Verification

.PHONY: verify
verify: build lint-go-full verify-codegen ## Run static analysis and verify generated code

.PHONY: verify-codegen
verify-codegen: ## Verify generated code
	@# Snapshot, regenerate, and compare, as the KubeVM API module does, so
	@# files git does not yet track are compared too.
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

# Both CRDs: this provider's AWSMachine, and the kube-vm.io VirtualMachine
# that the KubeVM core hosted in this manager reconciles.
.PHONY: install
install: ## Install the CRDs into the cluster named by KUBECONFIG
	kubectl apply -f ../kubevm/config/crd/bases/kube-vm.io_virtualmachines.yaml
	kubectl apply -k config/crd/bases

.PHONY: deploy
deploy: ## Deploy the manager into the cluster named by KUBECONFIG
	kubectl apply -f config/manager/namespace.yaml
	kubectl apply -f config/rbac
	kubectl apply -f config/manager/manager.yaml

.PHONY: undeploy
undeploy: ## Remove the manager
	kubectl delete --ignore-not-found -f config/manager/manager.yaml
	kubectl delete --ignore-not-found -f config/rbac
