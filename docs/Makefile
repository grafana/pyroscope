.DEFAULT_GOAL: help

# Adapted from https://www.thapaliya.com/en/writings/well-documented-makefiles/
.PHONY: help
help: ## Display this help.
help:
	@awk 'BEGIN {FS = ": ##"; printf "Usage:\n  make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_\.\-\/%]+: ##/ { printf "  %-45s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

include docs.mk

.PHONY: test
test:

.PHONY: test
test:
	docker run -v $(CURDIR)/sources:/hugo/content/docs/phlare/latest -e HUGO_REFLINKSERRORLEVEL=ERROR --rm grafana/docs-base:latest /bin/bash -c 'exec make hugo'

.PHONY: validate
validate:
	docker run -v $(CURDIR)/sources:/docs/sources --rm grafana/doc-validator:v1.5.0 /docs/sources
