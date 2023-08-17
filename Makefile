# Ensure Make is run with bash shell as some syntax below is bash-specific
SHELL := /usr/bin/env bash

.DEFAULT_GOAL := help

# The list of goals that do not require Golang.
NON_GO_GOALS := lint-markdown lint-shell

# If one of the goals that require golang is present and the
# Go binary is not in the path, then print an error message
# and exit.
ifneq (,$(filter-out $(NON_GO_GOALS),$(MAKECMDGOALS)))
ifeq (,$(strip $(shell command -v go 2>/dev/null || true)))

$(error Golang binary not detected in path)

else # (,$(strip $(shell command -v go 2>/dev/null || true)))

# Active module mode, as we use go modules to manage dependencies.
export GO111MODULE := on

# Default the GOOS and GOARCH values to be the same as the platform on which
# this Makefile is being executed.
export GOOS ?= $(shell go env GOHOSTOS)
export GOARCH ?= $(shell go env GOHOSTARCH)

# The directory in which this Makefile is located. Please note this will not
# behave correctly if the path to any Makefile in the list contains any spaces.
ROOT_DIR ?= $(dir $(realpath $(lastword $(MAKEFILE_LIST))))

# Get the GOPATH, but do not export it. This is used to determine if the project
# is in the GOPATH and if not, to use Docker for the generate-go-conversions
# target.
GOPATH ?= $(shell go env GOPATH)
PROJECT_SLUG := github.com/vmware-tanzu/vm-operator

# ROOT_DIR_IN_GOPATH is non-empty if ROOT_DIR is in the GOPATH.
ROOT_DIR_IN_GOPATH := $(findstring $(GOPATH)/src/$(PROJECT_SLUG),$(ROOT_DIR))

# CONVERSION_GEN_FALLBACK_MODE determines how to run the conversion-gen tool if
# this project is not in the GOPATH at the expected location. Possible values
# include "symlink" and "docker."
CONVERSION_GEN_FALLBACK_MODE ?= symlink

endif # ifneq (,$(filter-out $(NON_GO_GOALS),$(MAKECMDGOALS)))
endif # ifeq (,$(strip $(shell command -v go 2>/dev/null || true)))

# Directories
BIN_DIR       := bin
TOOLS_DIR     := hack/tools
TOOLS_BIN_DIR := $(TOOLS_DIR)/bin
UPGRADE_DIR   := upgrade
export PATH := $(abspath $(BIN_DIR)):$(abspath $(TOOLS_BIN_DIR)):$(PATH)
export KUBEBUILDER_ASSETS := $(abspath $(TOOLS_BIN_DIR))

# Binaries
MANAGER                := $(BIN_DIR)/manager
WEB_CONSOLE_VALIDATOR  := $(BIN_DIR)/web-console-validator

# Tooling binaries
CRD_REF_DOCS       := $(TOOLS_BIN_DIR)/crd-ref-docs
CONTROLLER_GEN     := $(TOOLS_BIN_DIR)/controller-gen
CONVERSION_GEN     := $(TOOLS_BIN_DIR)/conversion-gen
GOLANGCI_LINT      := $(TOOLS_BIN_DIR)/golangci-lint
KUSTOMIZE          := $(TOOLS_BIN_DIR)/kustomize
GOCOVMERGE         := $(TOOLS_BIN_DIR)/gocovmerge
GOCOV              := $(TOOLS_BIN_DIR)/gocov
GOCOV_XML          := $(TOOLS_BIN_DIR)/gocov-xml
GINKGO             := $(TOOLS_BIN_DIR)/ginkgo
KUBE_APISERVER     := $(TOOLS_BIN_DIR)/kube-apiserver
KUBEBUILDER        := $(TOOLS_BIN_DIR)/kubebuilder
KUBECTL            := $(TOOLS_BIN_DIR)/kubectl
ETCD               := $(TOOLS_BIN_DIR)/etcd

# Allow overriding manifest generation destination directory
MANIFEST_ROOT     ?= config
CRD_ROOT          ?= $(MANIFEST_ROOT)/crd/bases
EXTERNAL_CRD_ROOT ?= $(MANIFEST_ROOT)/crd/external-crds
WEBHOOK_ROOT      ?= $(MANIFEST_ROOT)/webhook
RBAC_ROOT         ?= $(MANIFEST_ROOT)/rbac

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

IMG_REGISTRY_OP_API_SLUG := github.com/vmware-tanzu/image-registry-operator-api

BUILD_TYPE ?= dev
BUILD_NUMBER ?= 00000000
BUILD_COMMIT ?= $(shell git rev-parse --short HEAD)
export BUILD_VERSION ?= $(shell git describe --always --match "v*" | sed 's/v//')

BUILDINFO_LDFLAGS = "\
-X $(PROJECT_SLUG)/pkg.BuildVersion=$(BUILD_VERSION) \
-X $(PROJECT_SLUG)/pkg.BuildNumber=$(BUILD_NUMBER) \
-X $(PROJECT_SLUG)/pkg.BuildCommit=$(BUILD_COMMIT) \
-X $(PROJECT_SLUG)/pkg.BuildType=$(BUILD_TYPE) \
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
test-nocover: ## Run Tests (without code coverage)
	hack/test-unit.sh

.PHONY: test
test: | $(GOCOVMERGE)
test: ## Run tests
	@rm -f $(COVERAGE_FILE)
	hack/test-unit.sh $(COVERAGE_FILE)

.PHONY: test-integration
test-integration: | $(ETCD) $(KUBE_APISERVER)
test-integration: ## Run integration tests
	KUBECONFIG=$(KUBECONFIG) hack/test-integration.sh $(INT_COV_FILE)

.PHONY: coverage
coverage-merge: | $(GOCOVMERGE) $(GOCOV) $(GOCOV_XML)
coverage-merge: ## Merge the coverage from unit and integration tests
	$(GOCOVMERGE) $(COVERAGE_FILE) $(INT_COV_FILE) >$(FULL_COV_FILE)
	gocov convert "$(FULL_COV_FILE)" | gocov-xml >"$(FULL_COV_FILE:.out=.xml)"

.PHONY: coverage
coverage: test ## Show unit test code coverage (opens a browser)
	go tool cover -html=$(COVERAGE_FILE)

.PHONY: coverage-full
coverage-full: test test-integration
coverage-full: ## Show combined code coverage for unit and integration tests (opens a browser)
	$(MAKE) coverage-merge
	go tool cover -html=$(FULL_COV_FILE)

## --------------------------------------
## Binaries
## --------------------------------------

.PHONY: $(MANAGER) manager-only
manager-only: $(MANAGER) ## Build manager binary only
$(MANAGER):
	CGO_ENABLED=0 go build -o $@ -ldflags $(BUILDINFO_LDFLAGS) .

.PHONY: manager
manager: prereqs generate lint-go manager-only ## Build manager binary

.PHONY: $(WEB_CONSOLE_VALIDATOR) web-console-validator-only
web-console-validator-only: $(WEB_CONSOLE_VALIDATOR) ## Build web-console-validator binary only
$(WEB_CONSOLE_VALIDATOR):
	CGO_ENABLED=0 go build -o $@ -ldflags $(BUILDINFO_LDFLAGS) cmd/web-console-validator/main.go

.PHONY: web-console-validator
web-console-validator: prereqs generate lint-go web-console-validator-only ## Build web-console-validator binary

## --------------------------------------
## Tooling Binaries
## --------------------------------------

TOOLING_BINARIES := $(CRD_REF_DOCS) $(CONTROLLER_GEN) $(CONVERSION_GEN) \
                    $(GOLANGCI_LINT) $(KUSTOMIZE) \
                    $(KUBE_APISERVER) $(KUBEBUILDER) $(KUBECTL) $(ETCD) \
                    $(GINKGO) $(GOCOVMERGE) $(GOCOV) $(GOCOV_XML)
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
lint-go: | $(GOLANGCI_LINT)
lint-go: ## Lint codebase
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
	cd hack/tools && go mod tidy
	cd api && go mod tidy

.PHONY: modules-vendor
modules-vendor: ## Vendors the modules
	go mod vendor
	cd api && go mod tidy

.PHONY: modules-download
modules-download: ## Downloads and caches the modules
	go mod download
	cd hack/tools && go mod download
	cd api && go mod download

.PHONY: generate
generate: ## Generate code
	$(MAKE) generate-go
	$(MAKE) generate-manifests
	$(MAKE) generate-external-manifests
# Disable for now until we figure out why the check fails on GH actions
#	$(MAKE) generate-api-docs

.PHONY: generate-go
generate-go: | $(CONTROLLER_GEN)
generate-go: ## Generate deepcopy
	$(CONTROLLER_GEN) \
		paths=github.com/vmware-tanzu/vm-operator/api/... \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt

.PHONY: generate-manifests
generate-manifests: | $(CONTROLLER_GEN)
generate-manifests: ## Generate manifests e.g. CRD, RBAC etc.
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

.PHONY: generate-external-manifests
generate-external-manifests: | $(CONTROLLER_GEN)
generate-external-manifests: ## Generate manifests for the external types for testing
	API_MOD_DIR=$(shell go mod download -json $(IMG_REGISTRY_OP_API_SLUG) | grep '"Dir":' | awk '{print $$2}' | tr -d '",') && \
	$(CONTROLLER_GEN) \
		paths=$${API_MOD_DIR}/api/v1alpha1/... \
		crd:crdVersions=v1 \
		output:crd:dir=$(EXTERNAL_CRD_ROOT) \
		output:none

.PHONY: generate-go-conversions
generate-go-conversions: ## Generate conversions go code

ifneq (,$(ROOT_DIR_IN_GOPATH))

# If the project is not cloned in the correct location in the GOPATH then the
# conversion-gen tool does not work. If ROOT_DIR_IN_GOPATH is non-empty, then
# the project is in the correct location for conversion-gen to work. Otherwise,
# there are two fallback modes controlled by CONVERSION_GEN_FALLBACK_MODE.

# When the CONVERSION_GEN_FALLBACK_MODE is symlink, the conversion-gen binary
# is rebuilt every time due to GNU Make, MTIME values, and symlinks. This ifeq
# statement ensures that there is not an order-only dependency on CONVERSION_GEN
# if it already exists.
ifeq (,$(strip $(wildcard $(CONVERSION_GEN))))
generate-go-conversions: | $(CONVERSION_GEN)
endif

generate-go-conversions:
	$(CONVERSION_GEN) \
		--input-dirs=./api/v1alpha1 \
		--output-file-base=zz_generated.conversion \
		--go-header-file=./hack/boilerplate/boilerplate.generatego.txt

else ifeq (symlink,$(CONVERSION_GEN_FALLBACK_MODE))

# The generate-go-conversions target uses a symlink. Step-by-step, the target:
#
# 1. Creates a temporary directory to act as a GOPATH location and stores it
#    in NEW_GOPATH.
#
# 2. Determines the path to this project under the NEW_GOPATH and stores it in
#    NEW_ROOT_DIR.
#
# 3. Creates all of the path components for NEW_ROOT_DIR.
#
# 4. Removes the last path component in NEW_ROOT_DIR so it can be recreated as
#    a symlink in the next step.
#
# 5. Creates a symlink from this project to its new location under NEW_GOPATH.
#
# 6. Changes directories into NEW_ROOT_DIR.
#
# 7. Invokes "make generate-go-conversions" from NEW_ROOT_DIR while sending in
#    the values of GOPATH and ROOT_DIR to make this Makefile think it is in the
#    NEW_GOPATH.
#
# Because make runs targets in a separate shell, it is not necessary to change
# back to the original directory.
generate-go-conversions:
	NEW_GOPATH="$$(mktemp -d)" && \
	NEW_ROOT_DIR="$${NEW_GOPATH}/src/$(PROJECT_SLUG)" && \
	mkdir -p "$${NEW_ROOT_DIR}" && \
	rm -fr "$${NEW_ROOT_DIR}" && \
	ln -s "$(ROOT_DIR)" "$${NEW_ROOT_DIR}" && \
	cd "$${NEW_ROOT_DIR}" && \
	GOPATH="$${NEW_GOPATH}" ROOT_DIR="$${NEW_ROOT_DIR}" make $@

else ifeq (docker,$(CONVERSION_GEN_FALLBACK_MODE))

ifeq (,$(strip $(shell command -v docker 2>/dev/null || true)))
$(error Docker is required for generate-go-conversions and not detected in path!)
endif

# The generate-go-conversions target will use Docker. Step-by-step, the target:
#
# 1. GOLANG_IMAGE is set to golang:YOUR_LOCAL_GO_VERSION and is the image used
#    to run make generate-go-conversions.
#
# 2. If using an arm host, the GOLANG_IMAGE is prefixed with arm64v8, which is
#    the prefix for Golang's Docker images for arm systems.
#
# 3. A new, temporary directory is created and its path is stored in
#    TOOLS_BIN_DIR. More on this later.
#
# 4. The docker flag --rm ensures that the container will be removed upon
#    success or failure, preventing orphaned containers from hanging around.
#
# 5. The first -v flag is used to bind mount the project's root directory to
#    the path /go/src/github.com/vmware-tanzu/vm-operator inside of the
#    container. This is required for the conversion-gen tool to work correctly.
#
# 6. The second -v flag is used to bind mount the temporary directory stored
#    TOOLS_BIN_DIR to /go/src/github.com/vmware-tanzu/vm-operator/hack/tools/bin
#    inside the container. This ensures the local host's binaries are not
#    overwritten case the local host is not Linux. Otherwise the container would
#    fail to run the binaries because they are the wrong architecture or replace
#    the binaries with Linux's elf architecture when the localhost uses
#    something else (ex. macOS is Darwin and uses mach).
#
# 7. The -w flag sets the container's working directory to where the project's
#    sources are bind mounted, /go/src/github.com/vmware-tanzu/vm-operator.
#
# 8. The image calculated earlier, GOLANG_IMAGE, is specified.
#
# 9. Finally, the command "make generate-go-conversions" is specified as what
#    the container will run.
#
# Once this target completes, it will be as if the generate-go-conversions
# target was executed locally. Any necessary updates to the generated conversion
# sources will be found on the local filesystem. Use "git status" to verify the
# changes.
generate-go-conversions:
	GOLANG_IMAGE="golang:$$(go env GOVERSION | cut -c3-)"; \
	[ "$$(go env GOHOSTARCH)" = "arm64" ] && GOLANG_IMAGE="arm64v8/$${GOLANG_IMAGE}"; \
	TOOLS_BIN_DIR="$$(mktemp -d)"; \
	  docker run -it --rm \
	  -v "$(ROOT_DIR)":/go/src/$(PROJECT_SLUG) \
	  -v "$${TOOLS_BIN_DIR}":/go/src/$(PROJECT_SLUG)/hack/tools/bin \
	  -w /go/src/$(PROJECT_SLUG) \
	  "$${GOLANG_IMAGE}" \
	  make generate-go-conversions
endif

.PHONY: generate-api-docs
generate-api-docs: | $(CRD_REF_DOCS)
generate-api-docs: ## Generate API documentation
	$(CRD_REF_DOCS) \
	  --renderer=markdown \
	  --source-path=./api/v1alpha1 \
	  --config=./.crd-ref-docs/config.yaml \
	  --templates-dir=./.crd-ref-docs/template \
	  --output-path=./docs/ref/api/
	mv ./docs/ref/api/out.md ./docs/ref/api/v1alpha1.md
	$(CRD_REF_DOCS) \
	  --renderer=markdown \
	  --source-path=./api/v1alpha2 \
	  --config=./.crd-ref-docs/config.yaml \
	  --templates-dir=./.crd-ref-docs/template \
	  --output-path=./docs/ref/api/
	mv ./docs/ref/api/out.md ./docs/ref/api/v1alpha2.md


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
	kind create cluster --name "$(KIND_CLUSTER_NAME)" --image harbor-repo.vmware.com/dockerhub-proxy-cache/kindest/node:v1.24.12

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
## Documentation
## --------------------------------------

docs-serve-python: ## Serve docs w python
	pip3 install --user -r ./docs/requirements.txt
	$$(python3 -m site --user-base)/bin/mkdocs serve

docs-serve-docker: ## Serve docs w docker
	docker build -f Dockerfile.docs -t $(IMAGE)-docs:$(IMAGE_TAG) .
	docker run -it --rm -p 8000:8000 -v "$$(pwd)":/docs $(IMAGE)-docs:$(IMAGE_TAG)

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
