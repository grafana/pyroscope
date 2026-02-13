.DEFAULT_GOAL: help

# Adapted from https://www.thapaliya.com/en/writings/well-documented-makefiles/
.PHONY: help
help: ## Display this help.
help:
	@awk 'BEGIN {FS = ": ##"; printf "Usage:\n  make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_\.\-\/%]+: ##/ { printf "  %-45s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

include docs.mk

.PHONY: test
test:
	docker run -v $(CURDIR)/sources:/hugo/content/docs/pyroscope/latest -e HUGO_REFLINKSERRORLEVEL=ERROR --rm grafana/docs-base:latest /bin/bash -c 'exec make hugo'

# Find all mermaid files and derive SVG targets
MERMAID_FILES := $(shell find $(CURDIR)/sources -name '*.mermaid' 2>/dev/null)
SVG_FILES := $(MERMAID_FILES:.mermaid=.svg)

.PHONY: mermaid
mermaid: ## Generate SVG files from mermaid diagrams.
mermaid: $(SVG_FILES)

%.svg: %.mermaid
	npx @mermaid-js/mermaid-cli@11.12.0 -i $< -o $@
