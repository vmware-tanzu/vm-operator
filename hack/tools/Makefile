# If you update this file, please follow
# https://suva.sh/posts/well-documented-makefiles

# Ensure Make is run with bash shell as some syntax below is bash-specific
SHELL := /usr/bin/env bash

.DEFAULT_GOAL := help

# Active module mode, as we use go modules to manage dependencies
export GO111MODULE := on

# Versions.
K8S_VERSION=1.24.1

# Get the information about the platform on which the tools are built/run.
GOHOSTOS := $(shell go env GOHOSTOS)
GOHOSTARCH := $(shell go env GOHOSTARCH)
GOHOSTOSARCH := $(GOHOSTOS)_$(GOHOSTARCH)

# Default the GOOS and GOARCH values to be the same as the platform on which
# this Makefile is being executed.
export GOOS := $(GOHOSTOS)
export GOARCH := $(GOHOSTARCH)

# Directories.
BIN_DIR := bin

# Binaries.
CRD_REF_DOCS      := $(BIN_DIR)/$(GOHOSTOSARCH)/crd-ref-docs
CONTROLLER_GEN    := $(BIN_DIR)/$(GOHOSTOSARCH)/controller-gen
CONVERSION_GEN    := $(BIN_DIR)/$(GOHOSTOSARCH)/conversion-gen
SETUP_ENVTEST     := $(BIN_DIR)/$(GOHOSTOSARCH)/setup-envtest
GOLANGCI_LINT     := $(BIN_DIR)/$(GOHOSTOSARCH)/golangci-lint
KUSTOMIZE         := $(BIN_DIR)/$(GOHOSTOSARCH)/kustomize
GINKGO            := $(BIN_DIR)/$(GOHOSTOSARCH)/ginkgo
GOCOVMERGE        := $(BIN_DIR)/$(GOHOSTOSARCH)/gocovmerge
KUBEBUILDER       := $(BIN_DIR)/$(GOHOSTOSARCH)/kubebuilder
ETCD              := $(BIN_DIR)/$(GOHOSTOSARCH)/etcd
KUBE_APISERVER    := $(BIN_DIR)/$(GOHOSTOSARCH)/kube-apiserver
KUBECTL           := $(BIN_DIR)/$(GOHOSTOSARCH)/kubectl
KIND              := $(BIN_DIR)/$(GOHOSTOSARCH)/kind
GOCOV             := $(BIN_DIR)/$(GOHOSTOSARCH)/gocov
GOCOV_XML         := $(BIN_DIR)/$(GOHOSTOSARCH)/gocov-xml
GOVULNCHECK       := $(BIN_DIR)/$(GOHOSTOSARCH)/govulncheck

## --------------------------------------
## Help
## --------------------------------------

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

## --------------------------------------
## Binaries
## --------------------------------------

crd-ref-docs: $(CRD_REF_DOCS) ## Install crd-ref-docs
$(CRD_REF_DOCS): go.mod
	GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) \
	go build -tags=vmop_tools -o $(@) github.com/elastic/crd-ref-docs

controller-gen: $(CONTROLLER_GEN) ## Install controller-gen
$(CONTROLLER_GEN): go.mod
	GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) \
	go build -tags=vmop_tools -o $(@) sigs.k8s.io/controller-tools/cmd/controller-gen

conversion-gen: $(CONVERSION_GEN) ## Install conversion-gen
$(CONVERSION_GEN): go.mod
	GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) \
	go build -tags=vmop_tools -o $(@) k8s.io/code-generator/cmd/conversion-gen

setup-envtest: $(SETUP_ENVTEST) ## Install setup-envtest
$(SETUP_ENVTEST): go.mod
	GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) \
	go build -tags=vmop_tools -o $(@) sigs.k8s.io/controller-runtime/tools/setup-envtest

golangci-lint: $(GOLANGCI_LINT) ## Install golangci-lint
$(GOLANGCI_LINT): go.mod
	GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) \
	go build -tags=vmop_tools -o $(@) github.com/golangci/golangci-lint/cmd/golangci-lint

kustomize: $(KUSTOMIZE) ## Install kustomize
$(KUSTOMIZE): go.mod
	GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) \
	go build -tags=vmop_tools -o $(@) sigs.k8s.io/kustomize/kustomize/v5

ginkgo: $(GINKGO) ## Install ginkgo
$(GINKGO): go.mod
	GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) \
	go build -tags=vmop_tools -o $(@) github.com/onsi/ginkgo/ginkgo

gocovmerge: $(GOCOVMERGE) ## Install gocovmerge
$(GOCOVMERGE): go.mod
	GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) \
	go build -tags=vmop_tools -o $(@) github.com/wadey/gocovmerge

gocov: $(GOCOV) ## Install gocov
$(GOCOV): go.mod
	GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) \
	go build -tags=vmop_tools -o $(@) github.com/axw/gocov/gocov

gocov-xml: $(GOCOV_XML) ## Install gocov-xml
$(GOCOV_XML): go.mod
	GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) \
	go build -tags=vmop_tools -o $(@) github.com/AlekSi/gocov-xml

kubebuilder: $(KUBEBUILDER) ## Install kubebuilder
$(KUBEBUILDER): go.mod
	GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) \
	go build -tags=vmop_tools -o $(@) sigs.k8s.io/kubebuilder/v3/cmd

kind: $(KIND) ## Install kind
$(KIND): go.mod
	GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) \
	go build -tags=vmop_tools -o $(@) sigs.k8s.io/kind

govulncheck: $(GOVULNCHECK)  ## Install govulncheck
$(GOVULNCHECK): go.mod
	GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) \
	go build -tags=vmop_tools -o $(@) golang.org/x/vuln/cmd/govulncheck

.PHONY: k8s-envtest
k8s-envtest: $(SETUP_ENVTEST) ## Download envtest binaries
	$(SETUP_ENVTEST) use --os $(GOHOSTOS) --arch $(GOHOSTARCH) $(K8S_VERSION)

etcd: $(ETCD) ## Install etcd
kube-apiserver: $(KUBE_APISERVER) ## Install kube-apiserver
kubectl: $(KUBECTL) ## Install kubectl
$(ETCD) $(KUBE_APISERVER) $(KUBECTL): k8s-envtest  ## Install envtest related binaries
	@mkdir -p $(@D)
	@cp -f "$$($(SETUP_ENVTEST) use -p path $(K8S_VERSION))/$(@F)" $(@)


## --------------------------------------
## Generate
## --------------------------------------

.PHONY: modules
modules: ## Runs go mod tidy to validate modules
	go mod tidy -v

.PHONY: modules-download
modules-download: ## Downloads and caches the modules
	go mod download

## --------------------------------------
## Cleanup / Verification
## --------------------------------------

.PHONY: clean
clean: ## Run all the clean targets
	$(MAKE) clean-bin

.PHONY: clean-bin
clean-bin: ## Remove all generated binaries
	rm -rf bin
