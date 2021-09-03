# If you update this file, please follow
# https://suva.sh/posts/well-documented-makefiles

# Ensure Make is run with bash shell as some syntax below is bash-specific
SHELL := /usr/bin/env bash

.DEFAULT_GOAL := help

# Active module mode, as we use go modules to manage dependencies
export GO111MODULE := on

# Directories.
BIN_DIR := bin

# Versions.
KUBEBUILDER_VERSION=2.3.1
KUBECTL_VERSION=1.15.3
KIND_VERSION=0.7.0
GOLANGCI_LINT_VERSION=v1.40.0

HOST_OS=$(shell go env GOOS)
HOST_ARCH=$(shell go env GOARCH)

# Binaries.
CONTROLLER_GEN    := $(BIN_DIR)/controller-gen
CLIENT_GEN        := $(BIN_DIR)/client-gen
GOLANGCI_LINT     := $(BIN_DIR)/golangci-lint
KUSTOMIZE         := $(BIN_DIR)/kustomize
GO_JUNIT_REPORT   := $(BIN_DIR)/go-junit-report
GOCOVMERGE        := $(BIN_DIR)/gocovmerge
GOCOVER_COBERTURA := $(BIN_DIR)/gocover-cobertura
GINKGO            := $(BIN_DIR)/ginkgo
KUBE_APISERVER    := $(BIN_DIR)/kube-apiserver
KUBEBUILDER       := $(BIN_DIR)/kubebuilder
KUBECTL           := $(BIN_DIR)/kubectl
ETCD              := $(BIN_DIR)/etcd
KIND              := $(BIN_DIR)/kind

## --------------------------------------
## Help
## --------------------------------------

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

## --------------------------------------
## Binaries
## --------------------------------------

.PHONY: $(CONTROLLER_GEN)
controller-gen: $(CONTROLLER_GEN) ## Install controller-gen
$(CONTROLLER_GEN): go.mod
	go build -tags=tools -o $@ sigs.k8s.io/controller-tools/cmd/controller-gen

.PHONY: $(CLIENT_GEN)
client-gen: $(CLIENT_GEN) ## Install client-gen
$(CLIENT_GEN): go.mod
	go build -tags=tools -o $@ k8s.io/code-generator/cmd/client-gen

.PHONY: $(GOLANGCI_LINT)
golangci-lint: $(GOLANGCI_LINT) ## Install golangci-lint
$(GOLANGCI_LINT):
	[ -f $(BIN_DIR)/golangci-lint ] || curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh| sh -s -- -b $(BIN_DIR) $(GOLANGCI_LINT_VERSION)

.PHONY: $(KUSTOMIZE)
kustomize: $(KUSTOMIZE) ## Install kustomize
$(KUSTOMIZE):
	../ensure-kustomize.sh

.PHONY: $(GO_JUNIT_REPORT)
go-junit-report: $(GO_JUNIT_REPORT) ## Install go-junit-report
$(GO_JUNIT_REPORT): go.mod
	go build -tags=tools -o $@ github.com/jstemmer/go-junit-report

.PHONY: $(GINKGO)
ginkgo: $(GINKGO) ## Install ginkgo
$(GINKGO): go.mod
	go build -tags=tools -o $@ github.com/onsi/ginkgo/ginkgo

.PHONY: $(GOCOVMERGE)
gocovmerge: $(GOCOVMERGE) ## Install gocovmerge
$(GOCOVMERGE): go.mod
	go build -tags=tools -o $@ github.com/wadey/gocovmerge

.PHONY: $(GOCOVER_COBERTURA)
gocover-cobertura: $(GOCOVER_COBERTURA) ## Install gocover-cobertura
$(GOCOVER_COBERTURA): go.mod
	go build -tags=tools -o $@ github.com/t-yuki/gocover-cobertura

etcd: $(ETCD) ## Install etcd
kube-apiserver: $(KUBE_APISERVER) ## Install kube-apiserver
kubebuilder: $(KUBEBUILDER) ## Install kubebuilder
kubectl: $(KUBECTL) ## Install kubectl
$(ETCD) $(KUBE_APISERVER) $(KUBEBUILDER):
	@mkdir -p $(@D)
	TEMP_DL_DIR=$$(mktemp -d) && echo $${TEMP_DL_DIR} && \
	curl -sL https://github.com/kubernetes-sigs/kubebuilder/releases/download/v$(KUBEBUILDER_VERSION)/kubebuilder_$(KUBEBUILDER_VERSION)_$(HOST_OS)_$(HOST_ARCH).tar.gz | \
	tar -xz -C "$${TEMP_DL_DIR}" --exclude=kubectl && \
	mv "$${TEMP_DL_DIR}/"*/bin/* $(@D)
$(KUBECTL):
	curl -sL https://storage.googleapis.com/kubernetes-release/release/v$(KUBECTL_VERSION)/bin/$(HOST_OS)/$(HOST_ARCH)/kubectl -o $(@) && \
	chmod a+x $(@)

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
