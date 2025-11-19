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

# Get the information about the platform on which the tools are built/run.
GOHOSTOS := $(shell go env GOHOSTOS)
GOHOSTARCH := $(shell go env GOHOSTARCH)
GOHOSTOSARCH := $(GOHOSTOS)_$(GOHOSTARCH)

# Default the GOOS and GOARCH values to be the same as the platform on which
# this Makefile is being executed.
export GOOS ?= $(GOHOSTOS)
export GOARCH ?= $(GOHOSTARCH)

# Default GOTOOLCHAIN to the one on the system instead of downloading whatever
# the go.mod file(s) specify. This fixes an issue of when the build happens
# behind a firewall or when GOSUMDB=off and the toolchain cannot be verified,
# and thus a potential download fails.
export GOTOOLCHAIN ?= local

# The directory in which this Makefile is located. Please note this will not
# behave correctly if the path to any Makefile in the list contains any spaces.
ROOT_DIR ?= $(dir $(realpath $(lastword $(MAKEFILE_LIST))))

# Get the GOPATH, but do not export it. This is used to determine if the project
# is in the GOPATH and if not, to use the container runtime for the
# generate-go-conversions target.
GOPATH ?= $(shell go env GOPATH)
PROJECT_SLUG := github.com/vmware-tanzu/vm-operator

# ROOT_DIR_IN_GOPATH is non-empty if ROOT_DIR is in the GOPATH.
ROOT_DIR_IN_GOPATH := $(findstring $(GOPATH)/src/$(PROJECT_SLUG),$(ROOT_DIR))

# R_WILDCARD recursively searches for files matching pattern $2 starting from
# directory $1.
R_WILDCARD=$(wildcard $(1)$(2)) $(foreach DIR,$(wildcard $(1)*),$(call R_WILDCARD,$(DIR)/,$(2)))

# Find all the go.mod files and directories from the root and subdirectories.
GO_MOD_FILES := $(strip $(call R_WILDCARD,./,go.mod))
GO_MOD_DIRS := $(dir $(GO_MOD_FILES))

# CONVERSION_GEN_FALLBACK_MODE determines how to run the conversion-gen tool if
# this project is not in the GOPATH at the expected location. Possible values
# include "symlink" and "docker|podman".
CONVERSION_GEN_FALLBACK_MODE ?= symlink

endif # ifneq (,$(filter-out $(NON_GO_GOALS),$(MAKECMDGOALS)))
endif # ifeq (,$(strip $(shell command -v go 2>/dev/null || true)))

# Directories
BIN_DIR       := bin
TOOLS_DIR     := hack/tools
TOOLS_BIN_DIR := $(TOOLS_DIR)/bin/$(GOHOSTOSARCH)
UPGRADE_DIR   := upgrade
export PATH := $(abspath $(BIN_DIR)):$(abspath $(TOOLS_BIN_DIR)):$(PATH)
export KUBEBUILDER_ASSETS := $(abspath $(TOOLS_BIN_DIR))

# Binaries
MANAGER                := $(BIN_DIR)/manager
WEB_CONSOLE_VALIDATOR  := $(BIN_DIR)/web-console-validator
VMCLASS                := $(BIN_DIR)/vmclass

# Tooling binaries
CRD_REF_DOCS       := $(TOOLS_BIN_DIR)/crd-ref-docs
CONTROLLER_GEN     := $(TOOLS_BIN_DIR)/controller-gen
CONVERSION_GEN     := $(TOOLS_BIN_DIR)/conversion-gen
GOLANGCI_LINT      := $(TOOLS_BIN_DIR)/golangci-lint
KUSTOMIZE          := $(TOOLS_BIN_DIR)/kustomize
GOCOV              := $(TOOLS_BIN_DIR)/gocov
GOCOV_XML          := $(TOOLS_BIN_DIR)/gocov-xml
GINKGO             := $(TOOLS_BIN_DIR)/ginkgo
KUBE_APISERVER     := $(TOOLS_BIN_DIR)/kube-apiserver
KUBEBUILDER        := $(TOOLS_BIN_DIR)/kubebuilder
KUBECTL            := $(TOOLS_BIN_DIR)/kubectl
ETCD               := $(TOOLS_BIN_DIR)/etcd
GOVULNCHECK        := $(TOOLS_BIN_DIR)/govulncheck
KIND               := $(TOOLS_BIN_DIR)/kind

# Allow overriding manifest generation destination directory
MANIFEST_ROOT     ?= config
CRD_ROOT          ?= $(MANIFEST_ROOT)/crd/bases
EXTERNAL_CRD_ROOT ?= $(MANIFEST_ROOT)/crd/external-crds
WEBHOOK_ROOT      ?= $(MANIFEST_ROOT)/webhook
RBAC_ROOT         ?= $(MANIFEST_ROOT)/rbac

# Image URL to use all building/pushing image targets
BASE_IMAGE ?= gcr.io/distroless/base-debian12
IMAGE ?= vmoperator-controller
IMAGE_TAG ?= latest
IMG ?= ${IMAGE}:${IMAGE_TAG}

# Code coverage files
COVERAGE_FILE ?= cover.out

# Gather a set of root packages that have at least one file that matches
# the pattern *_test.go as a child or descendent in that directory.
# However, given this is not a cheap operation, only gather these packages if
# the test-nocover target is one of the currenty active goals.
ifeq (,$(filter-out test-nocover,$(MAKECMDGOALS)))
COVERED_PKGS ?= $(shell find . -name '*_test.go' -not -path './api/*' -print | awk -F'/' '{print "./"$$2}' | sort -u)
endif

# CRI_BIN is the path to the container runtime binary.
ifeq (,$(strip $(GITHUB_RUN_ID)))
# Prefer podman locally.
CRI_BIN := $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null)
else
# Prefer docker in GitHub actions.
CRI_BIN := $(shell command -v docker 2>/dev/null || command -v podman 2>/dev/null)
endif
export CRI_BIN

KIND_CMD := $(KIND)
ifeq (podman,$(notdir $(CRI_BIN)))
KIND_CMD := KIND_EXPERIMENTAL_PROVIDER=podman $(KIND)
endif

# KIND_IMAGE may be overridden to use an upstream image.
KIND_IMAGE ?= dockerhub.packages.vcfd.broadcom.net/kindest/node:v1.31.0

ifneq (,$(strip $(KIND_IMAGE)))
KIND_IMAGE_FLAG := --image $(KIND_IMAGE)
endif

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
NET_OP_API_SLUG := github.com/vmware-tanzu/net-operator-api

BUILD_TYPE ?= dev
BUILD_NUMBER ?= 00000000

BUILD_BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD)
BUILD_COMMIT ?= $(shell git rev-parse --short HEAD)

# ex. 1.2.3+abcdefg+4.5.6+hijklmn
ifeq (,$(strip $(PRDCT_VERSION)))
PRDCT_VERSION := $(shell git describe --always --match 'v*' 2>/dev/null | awk -F- '{gsub(/^v/,"",$$1);gsub(/^g/,"+",$$3);print $$1$$3}')
endif

ifeq (,$(strip $(BUILD_VERSION)))

# ex. 1.2.3+abcdefg+4.5.6+hijklmn
BUILD_VERSION := $(PRDCT_VERSION)

ifneq (, $(strip,$(BUILD_NUMBER)))
# ex. 1.2.3+abcdefg+4.5.6+hijklmn+7891011
BUILD_VERSION := $(BUILD_VERSION)+$(BUILD_NUMBER)
endif

endif

# ex. 1.2.3-abcdefg-4.5.6-hijklmn-7891011
IMAGE_VERSION ?= $(subst +,-,$(BUILD_VERSION))

IMAGE_FILE ?= $(abspath $(ARTIFACTS_DIR))/$(IMAGE)-$(GOOS)_$(GOARCH).tar

export BUILD_BRANCH
export BUILD_COMMIT
export BUILD_VERSION

CGO_ENABLED ?= 0

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
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

## --------------------------------------
## Testing
## --------------------------------------

.PHONY: test-api
test-api: | $(GINKGO)
test-api: ## Run API tests
	COVERAGE_FILE="" hack/test.sh ./api/test

.PHONY: test-nocover
test-nocover: | $(GINKGO)
test-nocover: ## Run tests sans coverage
	hack/test.sh $(COVERED_PKGS)

.PHONY: test
test: | $(GINKGO) $(ETCD) $(KUBE_APISERVER)
test: ## Run tests
	@rm -f "$(COVERAGE_FILE)"
	COVERAGE_FILE="$(COVERAGE_FILE)" $(MAKE) test-nocover

.PHONY: coverage-xml
coverage-xml: $(GOCOV) $(GOCOV_XML)
coverage-xml:
	gocov convert "$(COVERAGE_FILE)" | gocov-xml >"$(COVERAGE_FILE:.out=.xml)"

.PHONY: coverage
coverage: ## Show test coverage in browser
	go tool cover -html="$(COVERAGE_FILE)"


## --------------------------------------
## Binaries
## --------------------------------------

.PHONY: $(MANAGER) manager-only
manager-only: $(MANAGER) ## Build manager binary only
$(MANAGER):
	GOOS="$(GOOS)" GOARCH="$(GOARCH)" CGO_ENABLED=$(CGO_ENABLED) go build -o $@ -ldflags $(BUILDINFO_LDFLAGS) .

.PHONY: manager
manager: prereqs generate lint-go manager-only ## Build manager binary

.PHONY: $(WEB_CONSOLE_VALIDATOR) web-console-validator-only
web-console-validator-only: $(WEB_CONSOLE_VALIDATOR) ## Build web-console-validator binary only
$(WEB_CONSOLE_VALIDATOR):
	GOOS="$(GOOS)" GOARCH="$(GOARCH)" CGO_ENABLED=$(CGO_ENABLED) go build -o $@ -ldflags $(BUILDINFO_LDFLAGS) cmd/web-console-validator/main.go

.PHONY: web-console-validator
web-console-validator: prereqs generate lint-go web-console-validator-only ## Build web-console-validator binary

vmclass: $(VMCLASS) ## Build vmclass binary
$(VMCLASS): cmd/vmclass/main.go
	GOOS="$(GOOS)" GOARCH="$(GOARCH)" CGO_ENABLED=$(CGO_ENABLED) go build -o $@ -ldflags $(BUILDINFO_LDFLAGS) cmd/vmclass/main.go


## --------------------------------------
## Tooling Binaries
## --------------------------------------

TOOLING_BINARIES := $(CRD_REF_DOCS) $(CONTROLLER_GEN) $(CONVERSION_GEN) \
                    $(GOLANGCI_LINT) $(KUSTOMIZE) \
                    $(KUBE_APISERVER) $(KUBEBUILDER) $(KUBECTL) $(ETCD) \
                    $(GINKGO) $(GOCOV) $(GOCOV_XML) $(GOVULNCHECK) $(KIND)
tools: $(TOOLING_BINARIES) ## Build tooling binaries
$(TOOLING_BINARIES):
	make -C $(TOOLS_DIR) $(@F)

ifneq (,$(strip $(wildcard $(GOPATH))))
.PHONY: tools-install
tools-install: $(TOOLING_BINARIES)
tools-install: ## Install the tooling binaries to $GOPATH/bin
ifeq (,$(strip $(wildcard $(GOPATH)/bin)))
	mkdir -p "$(GOPATH)/bin"
endif # (,$(strip $(wildcard $(GOPATH)/bin)))
	cp -f $(TOOLS_BIN_DIR)/* "$(GOPATH)/bin"
endif # (,$(strip $(wildcard $(GOPATH))))

## --------------------------------------
## Linting and fixing linter errors
## --------------------------------------

.PHONY: lint
lint: ## Run all the lint targets
	$(MAKE) lint-go-full
	$(MAKE) lint-markdown
	$(MAKE) lint-shell

GOLANGCI_LINT_FLAGS ?= --fast-only=true
GOLANGCI_LINT_ABS_PATH := $(abspath $(GOLANGCI_LINT))

GO_MOD_DIRS_TO_LINT := $(GO_MOD_DIRS)
GO_MOD_DIRS_TO_LINT := $(filter-out ./external%,$(GO_MOD_DIRS_TO_LINT))
GO_MOD_DIRS_TO_LINT := $(filter-out ./hack/tools%,$(GO_MOD_DIRS_TO_LINT))
GO_LINT_DIR_TARGETS := $(addprefix lint-,$(GO_MOD_DIRS_TO_LINT))

.PHONY: $(GO_LINT_DIR_TARGETS)
$(GO_LINT_DIR_TARGETS): | $(GOLANGCI_LINT)
	@echo
	@echo "####################################################################"
	@echo "## Linting $(subst lint-,,$@)"
	@echo "####################################################################"
	@echo
	cd $(subst lint-,,$@) && $(GOLANGCI_LINT_ABS_PATH) run -v $(GOLANGCI_LINT_FLAGS)

.PHONY: lint-go
lint-go: $(GO_LINT_DIR_TARGETS)
lint-go: ## Lint codebase

.PHONY: lint-go-full
lint-go-full: GOLANGCI_LINT_FLAGS = --fast-only=false
lint-go-full: lint-go ## Run slower linters to detect possible issues

.PHONY: lint-markdown
lint-markdown: ## Lint the project's markdown
	$(CRI_BIN) run --rm -v "$$(pwd)":/build gcr.io/cluster-api-provider-vsphere/extra/mdlint:0.17.0

.PHONY: lint-shell
lint-shell: ## Lint the project's shell scripts
	$(CRI_BIN) run --rm -t -v "$$(pwd)":/build:ro gcr.io/cluster-api-provider-vsphere/extra/shellcheck

.PHONY: fix
fix: GOLANGCI_LINT_FLAGS = --fast-only=false --fix
fix: lint-go ## Tries to fix errors reported by lint-go-full target

.PHONY: unfocus
unfocus: | $(GINKGO) ## Unfocuses all tests
	$(GINKGO) unfocus .

## --------------------------------------
## Generate
## --------------------------------------

reverse = $(if $(1),$(call reverse,$(wordlist 2,$(words $(1)),$(1)))) $(firstword $(1))
GO_MOD_FILES := $(call reverse,$(shell find . -name go.mod))
GO_MOD_OP := tidy

.PHONY: $(GO_MOD_FILES)
$(GO_MOD_FILES):
	go -C $(@D) mod $(GO_MOD_OP)

.PHONY: modules
modules: $(GO_MOD_FILES)
modules: ## Validates the modules

.PHONY: modules-vendor
modules-vendor: GO_MOD_OP=vendor
modules-vendor: $(GO_MOD_FILES)
modules-vendor: ## Vendors the modules

.PHONY: modules-download
modules-download: GO_MOD_OP=download
modules-download: $(GO_MOD_FILES)
modules-download: ## Downloads and caches the modules

.PHONY: generate
generate: ## Generate code
	$(MAKE) generate-go
	$(MAKE) generate-manifests
	$(MAKE) generate-external-manifests
# Disable for now until we figure out why the check fails on GH actions
#	$(MAKE) generate-api-docs

.PHONY: generate-go
generate-go: $(CONTROLLER_GEN)
generate-go: ## Generate golang sources
	go -C ./api generate ./...
	$(CONTROLLER_GEN) \
		paths=github.com/vmware-tanzu/vm-operator/api/... \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt
	$(CONTROLLER_GEN) \
		paths=github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/... \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt
	$(CONTROLLER_GEN) \
		paths=github.com/vmware-tanzu/vm-operator/external/tanzu-topology/... \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt
	$(CONTROLLER_GEN) \
		paths=github.com/vmware-tanzu/vm-operator/external/byok/... \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt
	$(CONTROLLER_GEN) \
		paths=github.com/vmware-tanzu/vm-operator/external/capabilities/... \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt
	$(CONTROLLER_GEN) \
		paths=github.com/vmware-tanzu/vm-operator/external/appplatform/... \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt
	$(CONTROLLER_GEN) \
		paths=github.com/vmware-tanzu/vm-operator/external/vsphere-policy/... \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt
	$(CONTROLLER_GEN) \
		paths=github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/... \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt
	$(MAKE) -C ./pkg/util/cloudinit/schema $@
	$(MAKE) -C ./pkg/util/netplan/schema $@

.PHONY: generate-manifests
generate-manifests: $(CONTROLLER_GEN)
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
generate-external-manifests: $(CONTROLLER_GEN)
generate-external-manifests: ## Generate manifests for the external types for testing
	API_MOD_DIR=$(shell go mod download -json $(IMG_REGISTRY_OP_API_SLUG) | grep '"Dir":' | awk '{print $$2}' | tr -d '",') && \
	$(CONTROLLER_GEN) \
		paths={$${API_MOD_DIR}/api/v1alpha1/...,$${API_MOD_DIR}/api/v1alpha2/...} \
		crd:crdVersions=v1 \
		output:crd:dir=$(EXTERNAL_CRD_ROOT) \
		output:none
	API_MOD_DIR=$(shell go mod download -json $(NET_OP_API_SLUG) | grep '"Dir":' | awk '{print $$2}' | tr -d '",') && \
	$(CONTROLLER_GEN) \
		paths=$${API_MOD_DIR}/api/v1alpha1/... \
		crd:crdVersions=v1 \
		output:crd:dir=$(EXTERNAL_CRD_ROOT) \
		output:none
	$(CONTROLLER_GEN) \
		paths=github.com/vmware-tanzu/vm-operator/external/tanzu-topology/... \
		crd:crdVersions=v1 \
		output:crd:dir=$(EXTERNAL_CRD_ROOT) \
		output:none
	$(CONTROLLER_GEN) \
		paths=github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/... \
		crd:crdVersions=v1 \
		output:crd:dir=$(EXTERNAL_CRD_ROOT) \
		output:none
	$(CONTROLLER_GEN) \
		paths=github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/... \
		crd:crdVersions=v1 \
		output:crd:dir=$(EXTERNAL_CRD_ROOT) \
		output:none
	$(CONTROLLER_GEN) \
		paths=github.com/vmware-tanzu/vm-operator/external/byok/... \
		crd:crdVersions=v1 \
		output:crd:dir=$(EXTERNAL_CRD_ROOT) \
		output:none
	$(CONTROLLER_GEN) \
		paths=github.com/vmware-tanzu/vm-operator/external/capabilities/... \
		crd:crdVersions=v1 \
		output:crd:dir=$(EXTERNAL_CRD_ROOT) \
		output:none
	$(CONTROLLER_GEN) \
		paths=github.com/vmware-tanzu/vm-operator/external/appplatform/... \
		crd:crdVersions=v1 \
		output:crd:dir=$(EXTERNAL_CRD_ROOT) \
		output:none
	$(CONTROLLER_GEN) \
		paths=github.com/vmware-tanzu/vm-operator/external/vsphere-policy/... \
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
generate-go-conversions: $(CONVERSION_GEN)
endif

EXTRA_PEER_DIRS := ./v1alpha2/sysprep/conversion/v1alpha2
EXTRA_PEER_DIRS := $(EXTRA_PEER_DIRS),./v1alpha2/sysprep/conversion/v1alpha5
EXTRA_PEER_DIRS := $(EXTRA_PEER_DIRS),./v1alpha3/sysprep/conversion/v1alpha3
EXTRA_PEER_DIRS := $(EXTRA_PEER_DIRS),./v1alpha3/sysprep/conversion/v1alpha5
EXTRA_PEER_DIRS := $(EXTRA_PEER_DIRS),./v1alpha4/sysprep/conversion/v1alpha4
EXTRA_PEER_DIRS := $(EXTRA_PEER_DIRS),./v1alpha4/sysprep/conversion/v1alpha5
EXTRA_PEER_DIRS := $(EXTRA_PEER_DIRS),./v1alpha3/common/conversion/v1alpha3
EXTRA_PEER_DIRS := $(EXTRA_PEER_DIRS),./v1alpha3/common/conversion/v1alpha5
EXTRA_PEER_DIRS := $(EXTRA_PEER_DIRS),./v1alpha4/common/conversion/v1alpha4
EXTRA_PEER_DIRS := $(EXTRA_PEER_DIRS),./v1alpha4/common/conversion/v1alpha5

generate-go-conversions:
	cd api && \
	$(abspath $(CONVERSION_GEN)) \
		-v 10 \
		--output-file=zz_generated.conversion.go \
		--go-header-file=$(abspath hack/boilerplate/boilerplate.generatego.txt) \
		--extra-peer-dirs='$(EXTRA_PEER_DIRS)' \
		./v1alpha1 ./v1alpha2 ./v1alpha3 ./v1alpha4

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

else ifeq ($(notdir $(CRI_BIN)),$(CONVERSION_GEN_FALLBACK_MODE))

ifeq (,$(CRI_BIN))
$(error Container runtime is required for generate-go-conversions and not detected in path!)
endif

# The generate-go-conversions target will use a container runtime. Step-by-step,
# the target:
#
# 1. GOLANG_IMAGE is set to golang:YOUR_LOCAL_GO_VERSION and is the image used
#    to run make generate-go-conversions.
#
# 2. If using an arm host, the GOLANG_IMAGE is prefixed with arm64v8, which is
#    the prefix for Golang's container images for arm systems.
#
# 3. A new, temporary directory is created and its path is stored in
#    TOOLS_BIN_DIR. More on this later.
#
# 4. The flag --rm ensures that the container will be removed upon success or
#    failure, preventing orphaned containers from hanging around.
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
	  $(CRI_BIN) run -it --rm \
	  -v "$(ROOT_DIR)":/go/src/$(PROJECT_SLUG) \
	  -v "$${TOOLS_BIN_DIR}":/go/src/$(PROJECT_SLUG)/hack/tools/bin \
	  -w /go/src/$(PROJECT_SLUG) \
	  "$${GOLANG_IMAGE}" \
	  make generate-go-conversions
endif

.PHONY: generate-api-docs
generate-api-docs: $(CRD_REF_DOCS)
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
	$(CRD_REF_DOCS) \
	  --renderer=markdown \
	  --source-path=./api/v1alpha3 \
	  --config=./.crd-ref-docs/config.yaml \
	  --templates-dir=./.crd-ref-docs/template \
	  --output-path=./docs/ref/api/
	mv ./docs/ref/api/out.md ./docs/ref/api/v1alpha3.md
	$(CRD_REF_DOCS) \
	  --renderer=markdown \
	  --source-path=./api/v1alpha4 \
	  --config=./.crd-ref-docs/config.yaml \
	  --templates-dir=./.crd-ref-docs/template \
	  --output-path=./docs/ref/api/
	mv ./docs/ref/api/out.md ./docs/ref/api/v1alpha4.md
	$(CRD_REF_DOCS) \
	  --renderer=markdown \
	  --source-path=./api/v1alpha5 \
	  --config=./.crd-ref-docs/config.yaml \
	  --templates-dir=./.crd-ref-docs/template \
	  --output-path=./docs/ref/api/
	mv ./docs/ref/api/out.md ./docs/ref/api/v1alpha5.md


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
kustomize-local: prereqs generate-manifests $(KUSTOMIZE)
kustomize-local: kustomize-x ## Kustomize for local cluster

.PHONY: kustomize-local-vcsim
kustomize-local-vcsim: CONFIG_TYPE=local-vcsim
kustomize-local-vcsim: YAML_OUT=$(LOCAL_YAML)
kustomize-local-vcsim: prereqs generate-manifests $(KUSTOMIZE)
kustomize-local-vcsim: kustomize-x ## Kustomize for local-vcsim cluster

.PHONY: kustomize-wcp
kustomize-wcp: CONFIG_TYPE=wcp
kustomize-wcp: YAML_OUT=$(LOCAL_YAML)
kustomize-wcp: prereqs generate-manifests $(KUSTOMIZE)
kustomize-wcp: kustomize-x ## Kustomize for wcp cluster

.PHONY: kustomize-wcp-no-configmap
kustomize-wcp-no-configmap: CONFIG_TYPE=wcp-no-configmap
kustomize-wcp-no-configmap: YAML_OUT=$(LOCAL_YAML)
kustomize-wcp-no-configmap: prereqs generate-manifests $(KUSTOMIZE)
kustomize-wcp-no-configmap: kustomize-x ## Kustomize for wcp cluster sans configmap


## --------------------------------------
## Development - kind
## --------------------------------------

.PHONY: kind-cluster-info
kind-cluster-info: | $(KIND)
kind-cluster-info:
	@$(KIND_CMD) get kubeconfig --name "$(KIND_CLUSTER_NAME)" >/dev/null 2>&1
	@printf "kind cluster name:   %s\nkind cluster config: %s\n" "$(KIND_CLUSTER_NAME)" "$(KUBECONFIG)"
	@printf "KUBECONFIG=%s\n" "$(KUBECONFIG)" >local.envvars

.PHONY: kind-info
kind-info: kind-cluster-info
kind-info: ## Print kind cluster info

.PHONY: kind-cluster-info-dump
kind-cluster-info-dump: | $(KUBECTL)
	@KUBECONFIG=$(KUBECONFIG) $(KUBECTL) cluster-info dump --all-namespaces --output-directory $(KIND_CLUSTER_INFO_DUMP_DIR) 1>/dev/null
	# Collect any logs from previously failed container invocations
	@KUBECONFIG=$(KUBECONFIG) $(KUBECTL) -n vmop-system logs --previous vmoperator-controller-manager-0 >$(KIND_CLUSTER_INFO_DUMP_DIR)/manager-0-prev-logs.txt 2>&1 || true
	@KUBECONFIG=$(KUBECONFIG) $(KUBECTL) -n vmop-system logs --previous vmoperator-controller-manager-1 >$(KIND_CLUSTER_INFO_DUMP_DIR)/manager-1-prev-logs.txt 2>&1 || true
	@KUBECONFIG=$(KUBECONFIG) $(KUBECTL) -n vmop-system logs --previous vmoperator-controller-manager-2 >$(KIND_CLUSTER_INFO_DUMP_DIR)/manager-2-prev-logs.txt 2>&1 || true
	@printf "kind cluster dump:   %s\n" "./$(KIND_CLUSTER_INFO_DUMP_DIR)"

.PHONY: kind-info-dump
kind-info-dump: kind-cluster-info-dump
kind-info-dump: ## Collect diagnostic info from kind cluster

.PHONY: kind-cluster
kind-cluster: | $(KIND)
kind-cluster:
	@$(MAKE) --no-print-directory kind-cluster-info 2>/dev/null || \
	$(KIND_CMD) create cluster --name "$(KIND_CLUSTER_NAME)" $(KIND_IMAGE_FLAG)

.PHONY: kind-up
kind-up: kind-cluster
kind-up: ## Create kind cluster

.PHONY: delete-kind-cluster
delete-kind-cluster: | $(KIND)
delete-kind-cluster:
	@{ $(MAKE) --no-print-directory kind-cluster-info >/dev/null 2>&1 && \
	$(KIND_CMD) delete cluster --name "$(KIND_CLUSTER_NAME)"; } || true

.PHONY: kind-down
kind-down: delete-kind-cluster
kind-down: ## Delete kind cluster

.PHONY: deploy-local-kind
deploy-local-kind: image-build load-kind

.PHONY: kind-deploy
kind-deploy: deploy-local-kind
kind-deploy: ## Deploy controller into kind cluster

.PHONY: load-kind
load-kind: | $(KIND)
load-kind:
	$(KIND_CMD) load docker-image $(IMG) --name $(KIND_CLUSTER_NAME) -v 10

.PHONY: kind-load
kind-load: load-kind
kind-load: ## Load image into kind cluster

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
deploy-local: prereqs kustomize-local | $(KUBECTL)
deploy-local: ## Deploy local YAML into kind cluster
	KUBECONFIG=$(KUBECONFIG) hack/deploy-local.sh $(LOCAL_YAML) $(DEFAULT_VMCLASSES_YAML)

.PHONY: undeploy-local
undeploy-local: | $(KUBECTL)
undeploy-local: ## Undeploy local YAML from kind cluster
	KUBECONFIG=$(KUBECONFIG) kubectl delete -f $(DEFAULT_VMCLASSES_YAML)
	KUBECONFIG=$(KUBECONFIG) kubectl delete -f $(LOCAL_YAML)

## --------------------------------------
## Development - gce2e
## --------------------------------------

.PHONY: deploy-local-vcsim
deploy-local-vcsim: prereqs kustomize-local-vcsim | $(KUBECTL)
deploy-local-vcsim: ## Deploy local-vcsim YAML into kind cluster
	KUBECONFIG=$(KUBECONFIG) hack/deploy-local.sh $(LOCAL_YAML)

## --------------------------------------
## Development - wcp
## --------------------------------------

.PHONY: deploy-wcp
deploy-wcp: prereqs kustomize-wcp | $(KUBECTL)
deploy-wcp: prereqs kustomize-wcp  ## Deploy wcp YAML into local kind cluster
	KUBECONFIG=$(KUBECONFIG) hack/deploy-local.sh $(LOCAL_YAML) $(DEFAULT_VMCLASSES_YAML)


## --------------------------------------
## Documentation
## --------------------------------------

MKDOCS_VENV := .venv/mkdocs

ifeq (,$(strip $(GITHUB_RUN_ID)))
MKDOCS := $(MKDOCS_VENV)/bin/mkdocs
else
MKDOCS := mkdocs
endif

.PHONY: docs-build-python
docs-build-python: ## Build docs w python
ifeq (,$(strip $(GITHUB_RUN_ID)))
	@[ -d "$(MKDOCS_VENV)" ] || python3 -m venv $(MKDOCS_VENV)
	$(MKDOCS_VENV)/bin/pip3 install -r ./docs/requirements.txt
endif
	$(MKDOCS) build --clean --config-file mkdocs.yml

.PHONY: docs-build-docker
docs-build-docker: ## Build docs w container
	$(CRI_BIN) build -f Dockerfile.docs -t $(IMAGE)-docs:$(IMAGE_TAG) .
	$(CRI_BIN) run -it --rm -v "$$(pwd)":/docs --entrypoint /usr/bin/mkdocs \
	  $(IMAGE)-docs:$(IMAGE_TAG) build --clean --config-file mkdocs.yml

.PHONY: docs-serve-python
docs-serve-python: ## Serve docs w python
ifeq (,$(strip $(GITHUB_RUN_ID)))
	@[ -d "$(MKDOCS_VENV)" ] || python3 -m venv $(MKDOCS_VENV)
	$(MKDOCS_VENV)/bin/pip3 install -r ./docs/requirements.txt
endif
	$(MKDOCS) serve

.PHONY: docs-serve-docker
docs-serve-docker: ## Serve docs w container
	$(CRI_BIN) build -f Dockerfile.docs -t $(IMAGE)-docs:$(IMAGE_TAG) .
	$(CRI_BIN) run -it --rm -p 8000:8000 -v "$$(pwd)":/docs $(IMAGE)-docs:$(IMAGE_TAG)

## --------------------------------------
## Docker
## --------------------------------------

.PHONY: image-build
image-build: GOOS=linux
image-build: manager-only web-console-validator-only
image-build: ## Build container image
	GOOS="$(GOOS)" GOARCH="$(GOARCH)" hack/build-container.sh \
	  -i "$(IMAGE)" \
	  -t "$(IMAGE_TAG)" \
	  -v "$(BUILD_VERSION)" \
	  -V "$(IMAGE_VERSION)" \
	  -n "$(BUILD_NUMBER)" \
	  -B "$(BASE_IMAGE)" \
	  -o "$(IMAGE_FILE)"

.PHONY: image-build-amd64
image-build-amd64: GOARCH=amd64
image-build-amd64: image-build
image-build-amd64: ## Build amd64 container image

.PHONY: image-build-arm64
image-build-arm64: GOARCH=arm64
image-build-arm64: image-build
image-build-arm64: ## Build arm64 container image

.PHONY: image-push
image-push: ## Push container image
	$(CRI_BIN) push ${IMG}

.PHONY: image-remove
image-remove: ## Remove container image
	@if [[ "`$(CRI_BIN) images -q ${IMG} 2>/dev/null`" != "" ]]; then \
		echo "Remove container ${IMG}"; \
		$(CRI_BIN) rmi ${IMG}; \
	fi

# The following are for backwards compatibility.
docker-build: image-build
docker-push: image-push
docker-remove: image-remove

## --------------------------------------
## Vulnerability Checks
## --------------------------------------

.PHONY: vulncheck-go
vulncheck-go: $(GOVULNCHECK)
	$(GOVULNCHECK) ./...


## --------------------------------------
## Clean and verify
## --------------------------------------

.PHONY: clean
clean: image-remove ## Remove all generated files
	rm -rf bin $(ARTIFACTS_DIR)
	rm -f covmeta.* covcounters.* cover.* coverage*
	find . -name '*.test' -delete
	find . -name '*.report' -delete
	find . -name '*.out' -delete

.PHONY: verify
verify: prereqs ## Run static code analysis
	hack/lint.sh

.PHONY: verify-codegen
verify-codegen: ## Verify generated code
	hack/verify-codegen.sh

.PHONY: verify-unfocus
verify-unfocus: ## Verify no tests have focus
	hack/verify-unfocus.sh

.PHONY: verify-filenames
verify-filenames: ## Verify webhook files follow naming convention
	hack/verify-filenames.sh

.PHONY: verify-local-manifests
verify-local-manifests: ## Verify the local manifests
	VERIFY_MANIFESTS=true $(MAKE) deploy-local

.PHONY: verify-wcp-manifests
verify-wcp-manifests: ## Verify the WCP manifests
	VERIFY_MANIFESTS=true $(MAKE) deploy-wcp
