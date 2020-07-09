# Ensure Make is run with bash shell as some syntax below is bash-specific
SHELL := /usr/bin/env bash

.DEFAULT_GOAL := help

INFRASTRUCTURE_COMPONENTS := infrastructure-components.yaml

# The way the images appear in the kustomize files.
# We only want the actual sha1 to identify the product version the way the
# container is tagged on the control plane VM This must be the same name (and version)
# that is being applied in cayman-photon file:
# - support/scripts/dockerfiles/vmop/build.sh
DEPLOYED_IMAGE_VERSION = $(BUILD_VERSION)
DEPLOYED_IMAGE_VMOPERATOR_CONTROLLER_NAME ?= vmware/vmop

ARTIFACTS_DIR := ../../artifacts
ARTIFACTS_DIR_TMP := $(ARTIFACTS_DIR)/tmp

KUSTOMIZE_TYPE ?= "wcp"

## --------------------------------------
## Help
## --------------------------------------

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

# We need to copy this kustomization file _and_ the config files above ("default" directory)
# as those files are referenced in the kustomization file
.PHONY: kustomize-copy
kustomize-copy:
	mkdir -p $(ARTIFACTS_DIR_TMP)
	rsync -ah ../ $(ARTIFACTS_DIR_TMP) --delete

# This is to set the new image name based on the current commit/version
# Seems like best practice from https://kubectl.docs.kubernetes.io/pages/app_management/container_images.html
.PHONY: kustomize-set-version
kustomize-set-version: kustomize-copy ## Update kustomization.yaml to set the current version
	cd $(ARTIFACTS_DIR_TMP)/wcp/vmoperator; kustomize edit set image $(DEPLOYED_IMAGE_VMOPERATOR_CONTROLLER_NAME):$(DEPLOYED_IMAGE_VERSION)

.PHONY: $(INFRASTRUCTURE_COMPONENTS)
infrastructure-components: $(INFRASTRUCTURE_COMPONENTS)
infrastructure-components: ## Build the WCP infrastructure components
$(INFRASTRUCTURE_COMPONENTS): kustomize-set-version
	kustomize build $(ARTIFACTS_DIR_TMP)/$(KUSTOMIZE_TYPE) --load_restrictor none >$@
