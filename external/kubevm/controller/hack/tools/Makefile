# If you update this file, please follow
# https://suva.sh/posts/well-documented-makefiles

SHELL := /usr/bin/env bash

.DEFAULT_GOAL := help

export GO111MODULE := on

K8S_VERSION=1.34.1

GOHOSTOS := $(shell go env GOHOSTOS)
GOHOSTARCH := $(shell go env GOHOSTARCH)
GOHOSTOSARCH := $(GOHOSTOS)_$(GOHOSTARCH)

export GOOS := $(GOHOSTOS)
export GOARCH := $(GOHOSTARCH)

BIN_DIR := bin

GINKGO         := $(BIN_DIR)/$(GOHOSTOSARCH)/ginkgo
SETUP_ENVTEST  := $(BIN_DIR)/$(GOHOSTOSARCH)/setup-envtest
ETCD           := $(BIN_DIR)/$(GOHOSTOSARCH)/etcd
KUBE_APISERVER := $(BIN_DIR)/$(GOHOSTOSARCH)/kube-apiserver
KUBECTL        := $(BIN_DIR)/$(GOHOSTOSARCH)/kubectl

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } ' $(MAKEFILE_LIST)

ginkgo: $(GINKGO) ## Install ginkgo
$(GINKGO): go.mod
	GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) \
	go build -tags=vmop_tools -o $(@) github.com/onsi/ginkgo/v2/ginkgo

setup-envtest: $(SETUP_ENVTEST) ## Install setup-envtest
$(SETUP_ENVTEST): go.mod
	GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) \
	go build -tags=vmop_tools -o $(@) sigs.k8s.io/controller-runtime/tools/setup-envtest

.PHONY: k8s-envtest
k8s-envtest: $(SETUP_ENVTEST) ## Download envtest binaries
	$(SETUP_ENVTEST) use --os $(GOHOSTOS) --arch $(GOHOSTARCH) $(K8S_VERSION)

etcd: $(ETCD) ## Install etcd
kube-apiserver: $(KUBE_APISERVER) ## Install kube-apiserver
kubectl: $(KUBECTL) ## Install kubectl
$(ETCD) $(KUBE_APISERVER) $(KUBECTL): k8s-envtest
	@mkdir -p $(@D)
	@cp -f "$$($(SETUP_ENVTEST) use -p path $(K8S_VERSION))/$(@F)" $(@)
