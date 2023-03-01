# If you update this file, please follow
# https://suva.sh/posts/well-documented-makefiles

# Ensure Make is run with bash shell as some syntax below is bash-specific
SHELL := /usr/bin/env bash

.DEFAULT_GOAL := help

# Active module mode, as we use go modules to manage dependencies
export GO111MODULE := on

# Versions.
KUBEBUILDER_VERSION=3.3.0
K8S_VERSION=1.24.1
KIND_VERSION=0.11.1

HOST_OS=$(shell go env GOOS)
HOST_ARCH=$(shell go env GOARCH)

# Directories.
BIN_DIR := bin

# The SETUP_ENVTEST download directory. setup-envtest will install binaries here,
# but it is easiest if we copy them to our BIN_DIR.
ENVTEST_DOWNLOAD_DIR = $(shell $(SETUP_ENVTEST) use -p path $(K8S_VERSION))

# Binaries.
CRD_REF_DOCS      := $(BIN_DIR)/crd-ref-docs
CONTROLLER_GEN    := $(BIN_DIR)/controller-gen
SETUP_ENVTEST     := $(BIN_DIR)/setup-envtest
GOLANGCI_LINT     := $(BIN_DIR)/golangci-lint
KUSTOMIZE         := $(BIN_DIR)/kustomize
GINKGO            := $(BIN_DIR)/ginkgo
GOCOVMERGE        := $(BIN_DIR)/gocovmerge
KUBEBUILDER       := $(BIN_DIR)/kubebuilder
ETCD              := $(BIN_DIR)/etcd
KUBE_APISERVER    := $(BIN_DIR)/kube-apiserver
KUBECTL           := $(BIN_DIR)/kubectl
KIND              := $(BIN_DIR)/kind
GOCOV             := $(BIN_DIR)/gocov
GOCOV_XML         := $(BIN_DIR)/gocov-xml

## --------------------------------------
## Help
## --------------------------------------

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

## --------------------------------------
## Binaries
## --------------------------------------

.PHONY: $(CRD_REF_DOCS)
crd-ref-docs: $(CRD_REF_DOCS) ## Install crd-ref-docs
$(CRD_REF_DOCS):
	go build -tags=vmop_tools -o $@ github.com/elastic/crd-ref-docs

.PHONY: $(CONTROLLER_GEN)
controller-gen: $(CONTROLLER_GEN) ## Install controller-gen
$(CONTROLLER_GEN): go.mod
	go build -tags=vmop_tools -o $@ sigs.k8s.io/controller-tools/cmd/controller-gen

.PHONY:$(SETUP_ENVTEST)
setup-envtest: $(SETUP_ENVTEST) ## Install setup-envtest
$(SETUP_ENVTEST): go.mod
	go build -tags=vmop_tools -o $@ sigs.k8s.io/controller-runtime/tools/setup-envtest

.PHONY: $(GOLANGCI_LINT)
golangci-lint: $(GOLANGCI_LINT) ## Install golangci-lint
$(GOLANGCI_LINT):
	go build -tags=vmop_tools -o $@ github.com/golangci/golangci-lint/cmd/golangci-lint

.PHONY: $(KUSTOMIZE)
kustomize: $(KUSTOMIZE) ## Install kustomize
$(KUSTOMIZE):
	../ensure-kustomize.sh

.PHONY: $(GINKGO)
ginkgo: $(GINKGO) ## Install ginkgo
$(GINKGO): go.mod
	go build -tags=vmop_tools -o $@ github.com/onsi/ginkgo/ginkgo

.PHONY: $(GOCOVMERGE)
gocovmerge: $(GOCOVMERGE) ## Install gocovmerge
$(GOCOVMERGE): go.mod
	go build -tags=vmop_tools -o $@ github.com/wadey/gocovmerge

.PHONY: $(GOCOV)
gocov: $(GOCOV) ## Install gocov
$(GOCOV): go.mod
	go build -tags=vmop_tools -o $@ github.com/axw/gocov/gocov

.PHONY: $(GOCOV_XML)
gocov-xml: $(GOCOV_XML) ## Install gocov-xml
$(GOCOV_XML): go.mod
	go build -tags=vmop_tools -o $@ github.com/AlekSi/gocov-xml

.PHONY: $(KUBEBUILDER)
kubebuilder: $(KUBEBUILDER) ## Install kubebuilder
	curl -sL https://github.com/kubernetes-sigs/kubebuilder/releases/download/v$(KUBEBUILDER_VERSION)/kubebuilder_$(HOST_OS)_$(HOST_ARCH) --output $(KUBEBUILDER) && chmod a+x $(KUBEBUILDER)

.PHONY: k8s-envtest
k8s-envtest: $(SETUP_ENVTEST) ## Download envtest binaries
	$(SETUP_ENVTEST) use --os $(HOST_OS) --arch $(HOST_ARCH) $(K8S_VERSION)

etcd: $(ETCD) ## Install etcd
kube-apiserver: $(KUBE_APISERVER) ## Install kube-apiserver
kubectl: $(KUBECTL) ## Install kubectl
$(ETCD) $(KUBE_APISERVER) $(KUBECTL): k8s-envtest  ## Install envtest related binaries
	@cp -f "$(ENVTEST_DOWNLOAD_DIR)/$(@F)" $(BIN_DIR)

.PHONY: $(KIND)
kind: $(KIND) ## Install kind
$(KIND):
	@mkdir -p $(@D)
	curl -sL https://github.com/kubernetes-sigs/kind/releases/download/v$(KIND_VERSION)/kind-$(HOST_OS)-$(HOST_ARCH) -o $(@) && \
	chmod a+x $(@)

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
