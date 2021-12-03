# Ensure Make is run with bash shell as some syntax below is bash-specific
SHELL := /usr/bin/env bash

.DEFAULT_GOAL := help

INFRASTRUCTURE_COMPONENTS := infrastructure-components.yaml

## --------------------------------------
## Help
## --------------------------------------

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)	

.PHONY: $(INFRASTRUCTURE_COMPONENTS)
infrastructure-components: $(INFRASTRUCTURE_COMPONENTS)
infrastructure-components: ## Build the local infrastructure components
$(INFRASTRUCTURE_COMPONENTS):
	kustomize build --load-restrictor LoadRestrictionsNone >$@
