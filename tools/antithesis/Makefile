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

# Antithesis runs x86-64 Linux. Use override so a host environment or command
# line cannot accidentally turn these into native arm64 images.
override ANTITHESIS_PLATFORM := linux/amd64

IMAGE_TAG ?= $(shell $(ROOT)/tools/image-tag)
PYROSCOPE_IMAGE := $(ANTITHESIS_REGISTRY)/pyroscope:$(IMAGE_TAG)
TEST_CLIENT_IMAGE := $(ANTITHESIS_REGISTRY)/pyroscope-test-client:$(IMAGE_TAG)
CONFIG_IMAGE := $(ANTITHESIS_REGISTRY)/pyroscope-config:$(IMAGE_TAG)

HELM := $(ROOT)/.tmp/bin/helm

.PHONY: help
help: ## Describe useful make targets
	@grep -hE '^[a-zA-Z0-9_/-]+:.*##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*##"}; {printf "%-16s %s\n", $$1, $$2}'

.PHONY: images
images: pyroscope-image test-client-image config-image ## Build all images used by the Antithesis environment

.PHONY: manifests
manifests: $(HELM) ## Render the Kubernetes manifests included in the config image
	$(HELM) template -n default --kube-version "1.23.0" pyroscope $(ROOT)/operations/pyroscope/helm/pyroscope \
		--values values.yaml \
		--set pyroscope.image.repository=$(ANTITHESIS_REGISTRY)/pyroscope \
		--set pyroscope.image.tag="$(IMAGE_TAG)" \
		--set-string antithesis.testClientImage="$(TEST_CLIENT_IMAGE)" \
		> manifests/pyroscope.yaml

# Verify the loaded image itself, not merely the build command. This also
# prevents `make push` from uploading a same-tag arm64 image built elsewhere.
define verify_image_platform
	@platform="$$(docker image inspect --format '{{.Os}}/{{.Architecture}}' "$(1)")"; \
	if [ "$$platform" != "$(ANTITHESIS_PLATFORM)" ]; then \
		echo "refusing image $(1): expected $(ANTITHESIS_PLATFORM), got $${platform:-unknown}" >&2; \
		exit 1; \
	fi; \
	echo "verified $(1): $$platform"
endef

.PHONY: pyroscope-image
# Built inside Docker (rather than delegating to the repository's
# docker-image/pyroscope/build) because the Antithesis build runs the Go
# instrumentor and needs a linux/amd64 cgo toolchain, which host builds on
# other platforms cannot provide. The builder stage runs `make build/antithesis`.
pyroscope-image: ## Build the Antithesis-instrumented Pyroscope image from the working tree
	docker buildx build -f Dockerfile.pyroscope --load --platform $(ANTITHESIS_PLATFORM) --provenance=false --sbom=false \
		--build-arg IMAGE_TAG="$(IMAGE_TAG)" \
		--build-arg GIT_REVISION="$(shell git rev-parse --short HEAD)" \
		--build-arg GIT_BRANCH="$(shell git rev-parse --abbrev-ref HEAD)" \
		-t $(PYROSCOPE_IMAGE) $(ROOT)
	$(call verify_image_platform,$(PYROSCOPE_IMAGE))

.PHONY: test-client-image
# Test commands live in a dedicated client image so they are not copied into
# every Pyroscope component. The Dockerfile independently rejects any target
# platform other than linux/amd64.
test-client-image: ## Build the long-running Test Composer client image
	docker buildx build -f Dockerfile.test-client --load --platform $(ANTITHESIS_PLATFORM) --provenance=false --sbom=false \
		-t $(TEST_CLIENT_IMAGE) $(ROOT)
	$(call verify_image_platform,$(TEST_CLIENT_IMAGE))

.PHONY: config-image
# The Antithesis environment is linux/amd64; a host-platform build (e.g. arm64
# on Apple Silicon) has no linux/amd64 variant, so its contents never reach the
# environment and setup fails at the kapp step. Attestations are disabled to
# push a plain single-manifest image.
config-image: manifests ## Build the config image holding the rendered manifests
	docker buildx build -f Dockerfile.config --load --platform $(ANTITHESIS_PLATFORM) --provenance=false --sbom=false -t $(CONFIG_IMAGE) .
	$(call verify_image_platform,$(CONFIG_IMAGE))

.PHONY: verify-images
verify-images: ## Refuse unless every local image is linux/amd64
	$(call verify_image_platform,$(PYROSCOPE_IMAGE))
	$(call verify_image_platform,$(TEST_CLIENT_IMAGE))
	$(call verify_image_platform,$(CONFIG_IMAGE))

.PHONY: push
push: verify-images ## Push all images to the Antithesis registry after verifying their platform
	docker push $(PYROSCOPE_IMAGE)
	docker push $(TEST_CLIENT_IMAGE)
	docker push $(CONFIG_IMAGE)

.PHONY: docker-login
docker-login: ## Log docker in to the Antithesis registry, requires ANTITHESIS_KEY_FILE
	@test -n "$(ANTITHESIS_KEY_FILE)" || { echo "ANTITHESIS_KEY_FILE must point to the JSON key provided by Antithesis" >&2; exit 1; }
	cat "$(ANTITHESIS_KEY_FILE)" | docker login -u _json_key https://us-central1-docker.pkg.dev --password-stdin

$(HELM):
	$(MAKE) -C $(ROOT) $(HELM)
