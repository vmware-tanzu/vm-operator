# Ensure Make is run with bash shell as some syntax below is bash-specific
SHELL := /usr/bin/env bash

.DEFAULT_GOAL := help

DEFAULT_VMCLASSES := default-vmclasses.yaml

## --------------------------------------
## Help
## --------------------------------------

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)	

.PHONY: $(DEFAULT_VMCLASSES)
default-vmclasses: $(DEFAULT_VMCLASSES)
default-vmclasses: ## Generate the default VirtualMachineClasses yaml
$(DEFAULT_VMCLASSES):
	kustomize build >$@
