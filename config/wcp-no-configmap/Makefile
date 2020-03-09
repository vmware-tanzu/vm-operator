# Ensure Make is run with bash shell as some syntax below is bash-specific
SHELL := /usr/bin/env bash

.DEFAULT_GOAL := help

INFRASTRUCTURE_COMPONENTS := infrastructure-components.yaml

WCP_DIR := ../wcp/

export KUSTOMIZE_TYPE="wcp-no-configmap"

## --------------------------------------
## Help
## --------------------------------------

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: $(INFRASTRUCTURE_COMPONENTS)
infrastructure-components: ## Build the WCP infrastructure components
infrastructure-components:
	$(MAKE) -C $(WCP_DIR) infrastructure-components
	cp $(WCP_DIR)/$(INFRASTRUCTURE_COMPONENTS) .
