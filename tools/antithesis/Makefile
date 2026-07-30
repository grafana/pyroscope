# Minimal integration with the Antithesis testing platform (https://antithesis.com).
# See README.md for usage.

ROOT := $(abspath ../..)

.DELETE_ON_ERROR:

# Tenant name assigned by Antithesis; determines the container registry
# repository. Launching runs and inspecting results goes through the REST API
# instead, see scripts/.
ANTITHESIS_TENANT ?= grafana
ANTITHESIS_REGISTRY := us-central1-docker.pkg.dev/molten-verve-216720/$(ANTITHESIS_TENANT)-repository
ANTITHESIS_KEY_FILE ?=

IMAGE_TAG ?= $(shell $(ROOT)/tools/image-tag)
PYROSCOPE_IMAGE := $(ANTITHESIS_REGISTRY)/pyroscope:$(IMAGE_TAG)
CONFIG_IMAGE := $(ANTITHESIS_REGISTRY)/pyroscope-config:$(IMAGE_TAG)

HELM := $(ROOT)/.tmp/bin/helm

.PHONY: help
help: ## Describe useful make targets
	@grep -hE '^[a-zA-Z0-9_/-]+:.*##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*##"}; {printf "%-16s %s\n", $$1, $$2}'

.PHONY: images
images: pyroscope-image config-image ## Build the Pyroscope image and the config image

.PHONY: manifests
manifests: $(HELM) ## Render the Kubernetes manifests included in the config image
	$(HELM) template -n default --kube-version "1.23.0" pyroscope $(ROOT)/operations/pyroscope/helm/pyroscope \
		--values values.yaml \
		--set pyroscope.image.repository=$(ANTITHESIS_REGISTRY)/pyroscope \
		--set pyroscope.image.tag="$(IMAGE_TAG)" \
		> manifests/pyroscope.yaml

.PHONY: pyroscope-image
# Built inside Docker (rather than delegating to the repository's
# docker-image/pyroscope/build) because the Antithesis build runs the Go
# instrumentor and needs a linux/amd64 cgo toolchain, which host builds on
# other platforms cannot provide. The builder stage runs `make build/antithesis`.
pyroscope-image: ## Build the Antithesis-instrumented Pyroscope image from the working tree
	docker buildx build -f Dockerfile.pyroscope --load --platform linux/amd64 --provenance=false --sbom=false \
		--build-arg IMAGE_TAG="$(IMAGE_TAG)" \
		--build-arg GIT_REVISION="$(shell git rev-parse --short HEAD)" \
		--build-arg GIT_BRANCH="$(shell git rev-parse --abbrev-ref HEAD)" \
		-t $(PYROSCOPE_IMAGE) $(ROOT)

.PHONY: config-image
# The Antithesis environment is linux/amd64; a host-platform build (e.g. arm64
# on Apple Silicon) has no linux/amd64 variant, so its contents never reach the
# environment and setup fails at the kapp step. Attestations are disabled to
# push a plain single-manifest image.
config-image: manifests ## Build the config image holding the rendered manifests
	docker buildx build -f Dockerfile.config --load --platform linux/amd64 --provenance=false --sbom=false -t $(CONFIG_IMAGE) .

.PHONY: push
push: ## Push both images to the Antithesis registry
	docker push $(PYROSCOPE_IMAGE)
	docker push $(CONFIG_IMAGE)

.PHONY: docker-login
docker-login: ## Log docker in to the Antithesis registry, requires ANTITHESIS_KEY_FILE
	@test -n "$(ANTITHESIS_KEY_FILE)" || { echo "ANTITHESIS_KEY_FILE must point to the JSON key provided by Antithesis" >&2; exit 1; }
	cat "$(ANTITHESIS_KEY_FILE)" | docker login -u _json_key https://us-central1-docker.pkg.dev --password-stdin

$(HELM):
	$(MAKE) -C $(ROOT) $(HELM)
