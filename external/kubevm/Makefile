# © Broadcom. All Rights Reserved.
# The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
# SPDX-License-Identifier: Apache-2.0

# This Makefile follows the conventions of the parent vm-operator repo's
# Makefile: the same target names (generate / generate-go / generate-manifests,
# lint / lint-go-full, modules, verify-codegen), the same help mechanism, the
# same OS/arch-scoped tooling directory, and the same podman-locally /
# docker-in-CI container-runtime selection.
#
# It deliberately does NOT replicate the parent's hack/tools sub-module. This
# module needs two tools, so they are installed by pinned version directly. Tool
# versions below are kept in sync with the parent's hack/tools/go.mod so the
# generated output does not churn depending on which tree you run this from.

SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c
.DEFAULT_GOAL := help

# Directories.
ROOT_DIR      := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
GOHOSTOS      := $(shell go env GOHOSTOS)
GOHOSTARCH    := $(shell go env GOHOSTARCH)
GOHOSTOSARCH  := $(GOHOSTOS)_$(GOHOSTARCH)
TOOLS_DIR     := hack/tools
TOOLS_BIN_DIR := $(TOOLS_DIR)/bin/$(GOHOSTOSARCH)
CRD_ROOT      := $(ROOT_DIR)/config/crd/bases

# Tooling binaries.
CONTROLLER_GEN := $(ROOT_DIR)/$(TOOLS_BIN_DIR)/controller-gen
GOLANGCI_LINT  := $(ROOT_DIR)/$(TOOLS_BIN_DIR)/golangci-lint

# Tool versions, kept in sync with the parent repo's hack/tools/go.mod.
CONTROLLER_TOOLS_VERSION := v0.21.0
GOLANGCI_LINT_VERSION    := v2.1.6

# The license header applied to generated Go sources.
BOILERPLATE := ./hack/boilerplate/boilerplate.generatego.txt

# CRI_BIN is the path to the container runtime binary, used by the
# container-based linters. Same selection logic as the parent repo.
ifeq (,$(strip $(GITHUB_RUN_ID)))
# Prefer podman locally.
CRI_BIN := $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null)
else
# Prefer docker in GitHub actions.
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
	GOBIN=$(ROOT_DIR)/$(TOOLS_BIN_DIR) \
	go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

$(GOLANGCI_LINT):
	GOBIN=$(ROOT_DIR)/$(TOOLS_BIN_DIR) \
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

TOOLING_BINARIES := $(CONTROLLER_GEN) $(GOLANGCI_LINT)

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
generate-manifests: ## Generate manifests e.g. CRD
	$(CONTROLLER_GEN) \
		paths=./api/... \
		crd:crdVersions=v1 \
		output:crd:dir=$(CRD_ROOT) \
		output:none

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

.PHONY: test
test: ## Run tests
	go test ./...

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

# The container-based linters default to the internal mirrors this repo's CI
# uses. Contributors without access to that registry can override either image
# with any equivalent, e.g. the upstream markdownlint-cli2 and crate-ci/typos
# images at a tag of their choosing:
#
#   make lint-markdown MDLINT_IMAGE=<markdownlint image:tag>
#   make typos TYPOS_IMAGE=<typos image:tag>
#
# These two targets are the only ones that need registry access at all; build,
# lint-go-full, generate, and verify-codegen do not.
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
	@# Snapshot, regenerate, and compare. Deliberately not implemented with
	@# "git diff", which reports nothing for files git does not yet track and so
	@# would silently pass before this module is first committed.
	@tmp=$$(mktemp -d) && trap 'rm -rf "$$tmp"' EXIT && \
	cp -R api "$$tmp/api" && cp -R config "$$tmp/config" && \
	$(MAKE) --no-print-directory generate >/dev/null && \
	if ! diff -rq "$$tmp/api" api >/dev/null 2>&1 || ! diff -rq "$$tmp/config" config >/dev/null 2>&1; then \
		echo ""; \
		echo "ERROR: generated code is out of date. Run 'make generate' and commit the result."; \
		diff -ru "$$tmp/api" api || true; \
		diff -ru "$$tmp/config" config || true; \
		exit 1; \
	fi; \
	echo "generated code is up to date"

## --------------------------------------
## Cleanup
## --------------------------------------
##@ Cleanup

.PHONY: clean
clean: ## Remove all generated tooling
	@rm -rf $(TOOLS_DIR)/bin
