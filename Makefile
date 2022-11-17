# Ensure Make is run with bash shell as some syntax below is bash-specific
SHELL := /usr/bin/env bash

.DEFAULT_GOAL := help

# Active module mode, as we use go modules to manage dependencies
export GO111MODULE := on

# Default the GOOS and GOARCH values to be the same as the platform on which
# this Makefile is being executed.
export GOOS ?= $(shell go env GOHOSTOS)
export GOARCH ?= $(shell go env GOHOSTARCH)

# Directories
BIN_DIR       := bin
TOOLS_DIR     := hack/tools
TOOLS_BIN_DIR := $(TOOLS_DIR)/bin
UPGRADE_DIR   := upgrade
export PATH := $(abspath $(BIN_DIR)):$(abspath $(TOOLS_BIN_DIR)):$(PATH)
export KUBEBUILDER_ASSETS := $(abspath $(TOOLS_BIN_DIR))

# Binaries
MANAGER                  := $(BIN_DIR)/manager
WEB_CONSOLE_VALIDATOR     := $(BIN_DIR)/web-console-validator

# Tooling binaries
CONTROLLER_GEN     := $(TOOLS_BIN_DIR)/controller-gen
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

# Allow overriding manifest generation destination directory
MANIFEST_ROOT ?= config
CRD_ROOT      ?= $(MANIFEST_ROOT)/crd/bases
WEBHOOK_ROOT  ?= $(MANIFEST_ROOT)/webhook
RBAC_ROOT     ?= $(MANIFEST_ROOT)/rbac

# Image URL to use all building/pushing image targets
IMAGE ?= vmoperator-controller
IMAGE_TAG ?= latest
IMG ?= ${IMAGE}:${IMAGE_TAG}

# Code coverage files
COVERAGE_FILE = cover.out
INT_COV_FILE  = integration-cover.out
FULL_COV_FILE = merged-cover.out

# Kind cluster name used in integration tests. Please note this name must
# match the value in the Groovy CI script or else the CI process won't
# discover the KubeConfig file for the cluster name.
KIND_CLUSTER_NAME ?= kind-it

# The path to the kubeconfig file used to access the bootstrap cluster.
KUBECONFIG ?= $(HOME)/.kube/config

# The directory to which information about the kind cluster is dumped.
KIND_CLUSTER_INFO_DUMP_DIR ?= kind-cluster-info-dump

ARTIFACTS_DIR := artifacts
LOCAL_YAML = $(ARTIFACTS_DIR)/local-deployment.yaml
DEFAULT_VMCLASSES_YAML = $(ARTIFACTS_DIR)/default-vmclasses.yaml

BUILD_TYPE ?= dev
BUILD_NUMBER ?= 00000000
BUILD_COMMIT ?= $(shell git rev-parse --short HEAD)
export BUILD_VERSION ?= $(shell git describe --always --match "v*" | sed 's/v//')

VMOP_PREFIX = github.com/vmware-tanzu/vm-operator
BUILDINFO_LDFLAGS = "\
-X $(VMOP_PREFIX)/pkg.BuildVersion=$(BUILD_VERSION) \
-X $(VMOP_PREFIX)/pkg.BuildNumber=$(BUILD_NUMBER) \
-X $(VMOP_PREFIX)/pkg.BuildCommit=$(BUILD_COMMIT) \
-X $(VMOP_PREFIX)/pkg.BuildType=$(BUILD_TYPE) \
-extldflags -static -w -s "

.PHONY: all
all: prereqs test manager web-console-validator ## Tests and builds the manager and web-console-validator binaries.

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
test: prereqs generate lint-go ## Run tests
	@rm -f $(COVERAGE_FILE)
	build/stage-unit-tests.sh $(COVERAGE_FILE)

.PHONY: test-integration
test-integration: prereqs generate lint-go ## Run integration tests
	KUBECONFIG=$(KUBECONFIG) build/stage-integration-tests.sh $(INT_COV_FILE)

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

.PHONY: web-console-validator-only
web-console-validator-only: $(WEB_CONSOLE_VALIDATOR) ## Build web-console-validator binary only
$(WEB_CONSOLE_VALIDATOR): go.mod prereqs generate
	go build -o $@ -ldflags $(BUILDINFO_LDFLAGS) cmd/web-console-validator/main.go

.PHONY: web-console-validator
web-console-validator: prereqs generate lint-go web-console-validator-only ## Build web-console-validator binary

## --------------------------------------
## Tooling Binaries
## --------------------------------------

TOOLING_BINARIES := $(CONTROLLER_GEN) $(GOLANGCI_LINT) $(KUSTOMIZE) \
                    $(KUBE_APISERVER) $(KUBEBUILDER) $(KUBECTL) \
                    $(ETCD) $(GINKGO) $(GO_JUNIT_REPORT) \
                    $(GOCOVMERGE) $(GOCOVER_COBERTURA)
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
		paths=github.com/vmware-tanzu/vm-operator/api/... \
		crd:crdVersions=v1 \
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

## --------------------------------------
## Kustomize
## --------------------------------------

.PHONY: kustomize-x
kustomize-x:
	$(MAKE) -C config/$(CONFIG_TYPE) infrastructure-components
	@cp -f config/$(CONFIG_TYPE)/infrastructure-components.yaml $(YAML_OUT)
	$(MAKE) -C config/virtualmachineclasses default-vmclasses
	@cp -f config/virtualmachineclasses/default-vmclasses.yaml $(DEFAULT_VMCLASSES_YAML)

.PHONY: kustomize-local
kustomize-local: CONFIG_TYPE=local
kustomize-local: YAML_OUT=$(LOCAL_YAML)
kustomize-local: prereqs generate-manifests | $(KUSTOMIZE)
kustomize-local: kustomize-x ## Kustomize for local cluster

.PHONY: kustomize-local-vcsim
kustomize-local-vcsim: CONFIG_TYPE=local-vcsim
kustomize-local-vcsim: YAML_OUT=$(LOCAL_YAML)
kustomize-local-vcsim: prereqs generate-manifests | $(KUSTOMIZE)
kustomize-local-vcsim: kustomize-x ## Kustomize for local-vcsim cluster


## --------------------------------------
## Development - kind
## --------------------------------------

.PHONY: kind-cluster-info
kind-cluster-info: ## Print the name of the Kind cluster and its kubeconfig
	@kind get kubeconfig --name "$(KIND_CLUSTER_NAME)" >/dev/null 2>&1
	@printf "kind cluster name:   %s\nkind cluster config: %s\n" "$(KIND_CLUSTER_NAME)" "$(KUBECONFIG)"
	@printf "KUBECONFIG=%s\n" "$(KUBECONFIG)" >local.envvars

.PHONY: kind-cluster-info-dump
kind-cluster-info-dump: ## Collect diagnostic information from the kind cluster.
	@KUBECONFIG=$(KUBECONFIG) kubectl cluster-info dump --all-namespaces --output-directory $(KIND_CLUSTER_INFO_DUMP_DIR) 1>/dev/null
	# Collect any logs from previously failed container invocations
	@KUBECONFIG=$(KUBECONFIG) kubectl -n vmop-system logs --previous vmoperator-controller-manager-0 >$(KIND_CLUSTER_INFO_DUMP_DIR)/manager-0-prev-logs.txt 2>&1 || true
	@KUBECONFIG=$(KUBECONFIG) kubectl -n vmop-system logs --previous vmoperator-controller-manager-1 >$(KIND_CLUSTER_INFO_DUMP_DIR)/manager-1-prev-logs.txt 2>&1 || true
	@KUBECONFIG=$(KUBECONFIG) kubectl -n vmop-system logs --previous vmoperator-controller-manager-2 >$(KIND_CLUSTER_INFO_DUMP_DIR)/manager-2-prev-logs.txt 2>&1 || true
	@printf "kind cluster dump:   %s\n" "./$(KIND_CLUSTER_INFO_DUMP_DIR)"

.PHONY: kind-cluster
kind-cluster: ## Create a kind cluster of name $(KIND_CLUSTER_NAME) for integration (if it does not exist yet)
	@$(MAKE) --no-print-directory kind-cluster-info 2>/dev/null || \
	kind create cluster --name "$(KIND_CLUSTER_NAME)" --image harbor-repo.vmware.com/dockerhub-proxy-cache/kindest/node:v1.22.4

.PHONY: delete-kind-cluster
delete-kind-cluster: ## Delete the kind cluster created for integration tests
	@{ $(MAKE) --no-print-directory kind-cluster-info >/dev/null 2>&1 && \
	kind delete cluster --name "$(KIND_CLUSTER_NAME)"; } || true

.PHONY: deploy-local-kind
deploy-local-kind: docker-build load-kind ## Deploy controller in the kind cluster used for integration tests

.PHONY: load-kind
load-kind: ## Load the image into the kind cluster
	kind load docker-image $(IMG) --name $(KIND_CLUSTER_NAME) --loglevel debug

## --------------------------------------
## Development - run
## --------------------------------------

.PHONY: run
run: prereqs generate lint-go
run: ## Run against the configured Kubernetes cluster in $(HOME)/.kube/config
	go run main.go

## --------------------------------------
## Development - local
## --------------------------------------

.PHONY: deploy-local
deploy-local: prereqs kustomize-local  ## Deploy controller in the configured Kubernetes cluster in ~/.kube/config
	KUBECONFIG=$(KUBECONFIG) hack/deploy-local.sh $(LOCAL_YAML) $(DEFAULT_VMCLASSES_YAML)

.PHONY: undeploy-local
undeploy-local:  ## Un-Deploy controller in the configured Kubernetes cluster in ~/.kube/config
	KUBECONFIG=$(KUBECONFIG) kubectl delete -f $(DEFAULT_VMCLASSES_YAML)
	KUBECONFIG=$(KUBECONFIG) kubectl delete -f $(LOCAL_YAML)

## --------------------------------------
## Development - gce2e
## --------------------------------------

.PHONY: deploy-local-vcsim
deploy-local-vcsim: prereqs kustomize-local-vcsim  ## Deploy controller in the configured Kubernetes cluster in ~/.kube/config
	KUBECONFIG=$(KUBECONFIG) hack/deploy-local.sh $(LOCAL_YAML)


## --------------------------------------
## Docker
## --------------------------------------

.PHONY: docker-build
docker-build: ## Build the docker image
	hack/build-container.sh -i $(IMAGE) -t $(IMAGE_TAG) -v $(BUILD_VERSION) -n $(BUILD_NUMBER)

.PHONY: docker-push
docker-push: prereqs  ## Push the docker image
	docker push ${IMG}

.PHONY: docker-remove
docker-remove: ## Remove the docker image
	@if [[ "`docker images -q ${IMG} 2>/dev/null`" != "" ]]; then \
		echo "Remove docker container ${IMG}"; \
		docker rmi ${IMG}; \
	fi


## --------------------------------------
## Clean and verify
## --------------------------------------

.PHONY: clean
clean: docker-remove ## Remove all generated files
	rm -rf bin *.out $(ARTIFACTS_DIR)

.PHONY: verify
verify: prereqs ## Run static code analysis
	hack/lint.sh

.PHONY: verify-codegen
verify-codegen: ## Verify generated code
	hack/verify-codegen.sh
