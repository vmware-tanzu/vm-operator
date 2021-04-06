# Ensure Make is run with bash shell as some syntax below is bash-specific
SHELL := /usr/bin/env bash

.DEFAULT_GOAL := help

# For more information please see https://golang.org/doc/go1.13#modules.
# Detect the Go version for now to be able to run gce2e before is resolved
GO_VERSION := $(shell go version)

GITHUB_PATH := github.com/vmware-tanzu/vm-operator

# Active module mode, as we use go modules to manage dependencies
export GO111MODULE := on

# Directories
BIN_DIR       := bin
TOOLS_DIR     := hack/tools
TOOLS_BIN_DIR := $(TOOLS_DIR)/bin
export PATH := $(abspath $(BIN_DIR)):$(abspath $(TOOLS_BIN_DIR)):$(PATH)
export KUBEBUILDER_ASSETS := $(abspath $(TOOLS_BIN_DIR))

# Binaries
MANAGER       := $(BIN_DIR)/manager

# Tooling binaries
CONTROLLER_GEN     := $(TOOLS_BIN_DIR)/controller-gen
CLIENT_GEN         := $(TOOLS_BIN_DIR)/client-gen
GOLANGCI_LINT      := $(TOOLS_BIN_DIR)/golangci-lint
KUSTOMIZE          := $(TOOLS_BIN_DIR)/kustomize
GO_JUNIT_REPORT    := $(TOOLS_BIN_DIR)/go-junit-report
GOCOVMERGE         := $(TOOLS_BIN_DIR)/gocovmerge
GOCOVER_COBERTURA  := $(TOOLS_BIN_DIR)/gocover-cobertura
GINKGO             := $(TOOLS_BIN_DIR)/ginkgo
KUBE_APISERVER     := $(TOOLS_BIN_DIR)/kube-apiserver
KUBEBUILDER        := $(TOOLS_BIN_DIR)/kubebuilder
KUBECTL            := $(TOOLS_BIN_DIR)/kubectl
ETCD               := $(TOOLS_BIN_DIR)/etcd
MOCKGEN            := $(TOOLS_BIN_DIR)/mockgen

# Allow overriding manifest generation destination directory
MANIFEST_ROOT ?= config
CRD_ROOT      ?= $(MANIFEST_ROOT)/crd/bases
WEBHOOK_ROOT  ?= $(MANIFEST_ROOT)/webhook
RBAC_ROOT     ?= $(MANIFEST_ROOT)/rbac

# Code coverage files
COVERAGE_FILE = cover.out
INT_COV_FILE  = integration-cover.out
FULL_COV_FILE = merged-cover.out

# Generated YAML output locations
ARTIFACTS_DIR := artifacts
LOCAL_YAML = $(ARTIFACTS_DIR)/local-deployment.yaml
DEFAULT_VMCLASSES_YAML = $(ARTIFACTS_DIR)/default-vmclasses.yaml

BUILDINFO_LDFLAGS = "-extldflags -static -w -s "

.PHONY: all
all: prereqs test manager ## Tests and builds the manager

prereqs:
	@mkdir -p bin $(ARTIFACTS_DIR)

## --------------------------------------
## Help
## --------------------------------------

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

## --------------------------------------
## Testing
## --------------------------------------

.PHONY: test-nocover
test-nocover: prereqs generate lint-go ## Run Tests (without code coverage)
	hack/test-unit.sh

.PHONY: test
test: prereqs generate lint-go ## Run unit tests
	@rm -f $(COVERAGE_FILE)
	hack/test-unit.sh $(COVERAGE_FILE) 2>&1 | tee unit-tests.out | go-junit-report > unit-tests-report.xml

.PHONY: test-integration
test-integration: prereqs generate lint-go ## Run integration tests
	hack/test-integration.sh $(INT_COV_FILE) 2>&1 | tee integration-tests.out | go-junit-report > integration-tests-report.xml

.PHONY: coverage
coverage: test ## Show unit test code coverage (opens a browser)
	go tool cover -html=$(COVERAGE_FILE)

.PHONY: coverage-full
coverage-full: test test-integration | $(GOCOVMERGE) ## Show combined code coverage for unit and integration tests (opens a browser)
	$(GOCOVMERGE) $(COVERAGE_FILE) $(INT_COV_FILE) >$(FULL_COV_FILE)
	go tool cover -html=$(FULL_COV_FILE)

## --------------------------------------
## Binaries
## --------------------------------------

.PHONY: manager-only
manager-only: $(MANAGER) ## Build manager binary only
$(MANAGER): go.mod prereqs generate
	go build -o $@ -ldflags $(BUILDINFO_LDFLAGS) .

.PHONY: manager
manager: prereqs generate lint-go manager-only ## Build manager binary

## --------------------------------------
## Docker Build Workflow
## --------------------------------------

# The necessary tools are built into the container at /tools/bin
# They need to be copied into the tools dir becasue this is bind-mounted into the container
# This will overwrite any locally built tools in the bin dir
IMAGE_TOOLS_BIN := /tools/bin
COPY_TOOLS_CMD := cp -rf $(IMAGE_TOOLS_BIN) $(TOOLS_DIR)

DOCKER_BUILD_IMAGE_NAME := vmop-build:latest
DOCKERFILE_NAME := Dockerfile.build

.PHONY: docker-image
docker-image: ## Builds a Docker image that includes the tools and modules necessary for building the manager
	rm -fr $(TOOLS_BIN_DIR)
	docker build -f $(DOCKERFILE_NAME) --build-arg TOOLS_BIN=$(IMAGE_TOOLS_BIN) -t $(DOCKER_BUILD_IMAGE_NAME) .

.PHONY: manager-docker
manager-docker: ## Build manager binary using a Docker build image
	docker run --rm -v $$(pwd):/go/src/$(GITHUB_PATH) -w /go/src/$(GITHUB_PATH) $(DOCKER_BUILD_IMAGE_NAME) /bin/sh -c "$(COPY_TOOLS_CMD) && make manager"

.PHONY: test-docker
test-docker: ## Unit test manager binary using a Docker build image
	docker run --rm -v $$(pwd):/go/src/$(GITHUB_PATH) -w /go/src/$(GITHUB_PATH) $(DOCKER_BUILD_IMAGE_NAME) /bin/sh -c "$(COPY_TOOLS_CMD) && make test"

.PHONY: test-integration-docker
test-integration-docker: ## Integration test manager binary using a Docker build image
	docker run --rm -v $$(pwd):/go/src/$(GITHUB_PATH) -w /go/src/$(GITHUB_PATH) $(DOCKER_BUILD_IMAGE_NAME) /bin/sh -c "$(COPY_TOOLS_CMD) && make test-integration"

.PHONY: clean-docker
clean-docker: ## Clean up the Docker image from the local image cache
	docker image rm $(DOCKER_BUILD_IMAGE_NAME)

## --------------------------------------
## Tooling Binaries
## --------------------------------------

TOOLING_BINARIES := $(CONTROLLER_GEN) $(CLIENT_GEN) $(GOLANGCI_LINT) $(KUSTOMIZE) \
                    $(KUBE_APISERVER) $(KUBEBUILDER) $(KUBECTL) \
                    $(ETCD) $(GINKGO) $(GO_JUNIT_REPORT) \
                    $(GOCOVMERGE) $(GOCOVER_COBERTURA) $(MOCKGEN)
tools: $(TOOLING_BINARIES) ## Build tooling binaries
.PHONY: $(TOOLING_BINARIES)
$(TOOLING_BINARIES):
	make -C $(TOOLS_DIR) $(@F)

## --------------------------------------
## Linting and fixing linter errors
## --------------------------------------

.PHONY: lint
lint: ## Run all the lint targets
	$(MAKE) lint-go-full
	$(MAKE) lint-markdown
	$(MAKE) lint-shell

GOLANGCI_LINT_FLAGS ?= --fast=true
.PHONY: lint-go
lint-go: $(GOLANGCI_LINT) ## Lint codebase
	$(GOLANGCI_LINT) run -v $(GOLANGCI_LINT_FLAGS)

.PHONY: lint-go-full
lint-go-full: GOLANGCI_LINT_FLAGS = --fast=false
lint-go-full: lint-go ## Run slower linters to detect possible issues

.PHONY: lint-markdown
lint-markdown: ## Lint the project's markdown
	docker run --rm -v "$$(pwd)":/build gcr.io/cluster-api-provider-vsphere/extra/mdlint:0.17.0

.PHONY: lint-shell
lint-shell: ## Lint the project's shell scripts
	docker run --rm -t -v "$$(pwd)":/build:ro gcr.io/cluster-api-provider-vsphere/extra/shellcheck

.PHONY: fix
fix: GOLANGCI_LINT_FLAGS = --fast=false --fix
fix: lint-go ## Tries to fix errors reported by lint-go-full target

## --------------------------------------
## Generate
## --------------------------------------

.PHONY: modules
modules: ## Validates the modules
	go mod tidy

.PHONY: modules-vendor
modules-vendor: ## Vendors the modules
	go mod vendor

.PHONY: modules-download
modules-download: ## Downloads and caches the modules
	go mod download

.PHONY: generate
generate: ## Generate code
	$(MAKE) generate-go
	$(MAKE) generate-manifests

.PHONY: generate-go
generate-go: ## Runs Go related generate targets
ifneq (0,$(GENERATE_CODE))
	go generate ./...
endif

.PHONY: generate-manifests
generate-manifests: $(CONTROLLER_GEN) ## Generate manifests e.g. CRD, RBAC etc.
	$(CONTROLLER_GEN) \
		paths=github.com/vmware-tanzu/vm-operator-api/api/... \
		crd:trivialVersions=true \
		crd:crdVersions=v1 \
		crd:preserveUnknownFields=false \
		output:crd:dir=$(CRD_ROOT) \
		output:none
	$(CONTROLLER_GEN) \
		paths=./webhooks/... \
		output:webhook:dir=$(WEBHOOK_ROOT) \
		webhook
	$(CONTROLLER_GEN) \
		paths=./controllers/... \
		paths=./pkg/... \
		paths=./webhooks/... \
		output:rbac:dir=$(RBAC_ROOT) \
		rbac:roleName=manager-role

.PHONY: generate-client
generate-client: $(CLIENT_GEN) ## Generates client for vm-operator-api
	hack/client-gen.sh

## --------------------------------------
## Kustomize
## --------------------------------------

.PHONY: kustomize-x
kustomize-x: prereqs generate-manifests | $(KUSTOMIZE)
	$(MAKE) -C config/$(CONFIG_TYPE) infrastructure-components
	@cp -f config/$(CONFIG_TYPE)/infrastructure-components.yaml $(YAML_OUT)
	$(MAKE) -C config/virtualmachineclasses default-vmclasses
	@cp -f config/virtualmachineclasses/default-vmclasses.yaml $(DEFAULT_VMCLASSES_YAML)

.PHONY: kustomize-local
kustomize-local: CONFIG_TYPE=local
kustomize-local: YAML_OUT=$(LOCAL_YAML)
kustomize-local: kustomize-x ## Kustomize for local cluster

## --------------------------------------
## Clean and verify
## --------------------------------------

.PHONY: clean
clean: 
	rm -rf bin *.out $(ARTIFACTS_DIR)

.PHONY: verify
verify: prereqs ## Run static code analysis
	hack/lint.sh

.PHONY: verify-codegen
verify-codegen: ## Verify generated code
	hack/verify-codegen.sh
