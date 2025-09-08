# Makefile for API Documentation Generation

# The api-docs image referenced here is being sourced from
# https://github-vcf.devops.broadcom.net/vcf/wcp-ci/tree/master/api-doc-tool
#
# WARNING: The Go version baked into this image must be compatible with the
# go directive in go.mod. If either is updated, verify the other still matches
# or the vendor/docs-generation step will fail with a version mismatch.
API_DOCS_IMAGE ?= wcp-docker-local.packages.vcfd.broadcom.net/tools/api-docs:699a6a4
DOCKER_RUN := docker run --platform=linux/amd64 \
  -v $(CURDIR):$(CURDIR) -w $(CURDIR) --rm $(API_DOCS_IMAGE)

# Prefer native binaries when present on PATH (faster for local dev).
# CI hosts that only ship the Docker image fall back to the container.
CRD_REF_DOCS_NATIVE := $(shell command -v crd-ref-docs 2>/dev/null)
ASCIIDOCTOR_NATIVE  := $(shell command -v asciidoctor 2>/dev/null)

ifneq ($(CRD_REF_DOCS_NATIVE),)
CRD_REF_DOCS := $(CRD_REF_DOCS_NATIVE)
else
CRD_REF_DOCS := $(DOCKER_RUN) crd-ref-docs
endif

ifneq ($(ASCIIDOCTOR_NATIVE),)
ASCIIDOCTOR := $(ASCIIDOCTOR_NATIVE)
else
ASCIIDOCTOR := $(DOCKER_RUN) asciidoctor
endif

.PHONY: help setup vendor docs docs-grouped docs-html docs-html-grouped \
        clean all package

help: ## Show this help message
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

setup: ## Verify crd-ref-docs tool availability
	@echo "Verifying crd-ref-docs..."
	$(CRD_REF_DOCS) --version

vendor: ## Vendor API dependencies for documentation generation
	@echo "Tidying go modules..."
	go mod tidy
	@echo "Vendoring dependencies..."
	go mod vendor

docs: vendor ## Generate API documentation in Asciidoc format
	@echo "Generating API documentation..."
	mkdir -p generated/single
	$(CRD_REF_DOCS) --config crd-ref-docs.config.yaml \
		--source-path vendor \
		--output-path generated/single/api-docs.asciidoc

docs-grouped: vendor ## Generate API documentation in Asciidoc format, one file per group
	@echo "Generating API documentation (grouped)..."
	mkdir -p generated/grouped
	$(CRD_REF_DOCS) --config crd-ref-docs.config.yaml \
		--source-path vendor \
		--output-path generated/grouped/ \
		--output-mode=group

docs-html: docs ## Generate API documentation in HTML format
	@echo "Generating HTML (single)..."
	$(ASCIIDOCTOR) generated/single/api-docs.asciidoc \
		-a stylesheet=$(CURDIR)/boot-yeti.css \
		-o generated/single/api-docs.html

docs-html-grouped: docs-grouped ## Generate HTML documentation, one file per group
	@echo "Generating HTML (grouped)..."
	$(ASCIIDOCTOR) generated/grouped/* \
		-a stylesheet=$(CURDIR)/boot-yeti.css

clean: ## Clean generated files
	@echo "Cleaning generated files..."
	rm -rf vendor/ generated/ go.sum

all: ## Run complete documentation generation pipeline with HTML output
	$(MAKE) clean
	$(MAKE) setup
	$(MAKE) docs-html docs-html-grouped

package: all ## Generate docs and assemble into publish/
	cp generated/single/api-docs.html publish/api-docs.html
