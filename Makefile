GO ?= go
SHELL := bash
.DELETE_ON_ERROR:
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := all
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules
MAKEFLAGS += --no-print-directory
BIN := $(CURDIR)/.tmp/bin
COPYRIGHT_YEARS := 2021-2022
LICENSE_IGNORE := -e /testdata/
GO_TEST_FLAGS ?= -v -race -cover

IMAGE_PLATFORM = linux/amd64
BUILDX_ARGS =
GOPRIVATE=github.com/grafana/frostdb

# Boiler plate for building Docker containers.
# All this must go at top of file I'm afraid.
IMAGE_PREFIX ?= us.gcr.io/kubernetes-dev/

IMAGE_TAG ?= $(shell ./tools/image-tag)
GIT_REVISION := $(shell git rev-parse --short HEAD)
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD)
GIT_LAST_COMMIT_DATE := $(shell git log -1 --date=iso-strict --format=%cd)

# Build flags
VPREFIX := github.com/grafana/fire/pkg/util/build
GO_LDFLAGS   := -X $(VPREFIX).Branch=$(GIT_BRANCH) -X $(VPREFIX).Version=$(IMAGE_TAG) -X $(VPREFIX).Revision=$(GIT_REVISION) -X $(VPREFIX).BuildDate=$(GIT_LAST_COMMIT_DATE)
GO_FLAGS     := -ldflags "-extldflags \"-static\" -s -w $(GO_LDFLAGS)" -tags netgo -mod=mod

.PHONY: help
help: ## Describe useful make targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "%-30s %s\n", $$1, $$2}'

.PHONY: all
all: lint test build ## Build, test, and lint (default)

.PHONY: lint
lint: go/lint helm/lint buf/lint ## Lint Go, Helm and protobuf

.PHONY: test
test: go/test ## Run unit tests

.PHONY: generate
generate: $(BIN)/buf $(BIN)/protoc-gen-go $(BIN)/protoc-gen-go-vtproto $(BIN)/protoc-gen-openapiv2 $(BIN)/protoc-gen-grpc-gateway $(BIN)/protoc-gen-connect-go $(BIN)/protoc-gen-connect-go-mux $(BIN)/gomodifytags ## Regenerate protobuf
	rm -rf pkg/gen/ pkg/openapiv2/gen
	PATH=$(BIN) $(BIN)/buf generate
	PATH=$(BIN):$(PATH) ./tools/add-parquet-tags.sh

.PHONY: buf/lint
buf/lint: $(BIN)/buf
	$(BIN)/buf lint || true # TODO: Fix linting problems and remove the always true

.PHONY: go/test
go/test:
	$(GO) test $(GO_TEST_FLAGS) ./...

.PHONY: build
build: go/bin plugin/datasource/build ## Build all packages

.PHONY: go/deps
go/deps:
	$(GO) mod tidy

.PHONY: go/bin
go/bin:
	mkdir -p ./bin
	CGO_ENABLED=0 $(GO) build $(GO_FLAGS) -o bin/ ./cmd/fire

.PHONY: go/lint
go/lint: $(BIN)/golangci-lint
	$(BIN)/golangci-lint run
	$(GO) vet ./...

.PHONY: go/mod
go/mod:
	GO111MODULE=on go mod download
	GO111MODULE=on go mod verify
	GO111MODULE=on go mod tidy
	cd ./grafana/fire-datasource/ && GO111MODULE=on go mod download
	cd ./grafana/fire-datasource/ && GO111MODULE=on go mod verify
	cd ./grafana/fire-datasource/ && GO111MODULE=on go mod tidy


.PHONY: plugin/datasource/build
plugin/datasource/build: $(BIN)/mage
	pushd ./grafana/fire-datasource && \
	yarn install && yarn build && \
	$(BIN)/mage -v \

.PHONY: plugin/flamegraph/build
plugin/flamegraph/build:
	pushd ./grafana/flamegraph && \
	yarn install && yarn build

.PHONY: start/grafana
start/grafana: plugin/datasource/build plugin/flamegraph/build
	./tools/grafana-fire

.PHONY: fmt
fmt: $(BIN)/golangci-lint $(BIN)/buf $(BIN)/tk ## Automatically fix some lint errors
	git ls-files '*.go' | grep -v 'vendor/' | xargs gofmt -s -w
	# TODO: Reenable once golangci-lint support go 1.18 properly
	# $(BIN)/golangci-lint run --fix
	$(BIN)/buf format -w .
	$(BIN)/tk fmt ./deploy/jsonnet/ tools/monitoring/

.PHONY: check/unstaged-changes
check/unstaged-changes:
	@git --no-pager diff --exit-code || { echo ">> There are unstaged changes in the working tree"; exit 1; }

.PHONY: check/go/mod
check/go/mod: go/mod
	@git --no-pager diff --exit-code -- go.sum go.mod vendor/ || { echo ">> There are unstaged changes in go vendoring run 'make go/mod'"; exit 1; }


define docker_buildx
	docker buildx build $(1) --ssh default --platform $(IMAGE_PLATFORM) $(BUILDX_ARGS) --build-arg=revision=$(GIT_REVISION) -t $(IMAGE_PREFIX)$(shell basename $(@D)) -t $(IMAGE_PREFIX)$(shell basename $(@D)):$(IMAGE_TAG) -f cmd/$(shell basename $(@D))/Dockerfile .
endef

define docker_buildx_grafana
	docker buildx build $(1) --ssh default --platform $(IMAGE_PLATFORM) $(BUILDX_ARGS) --build-arg=revision=$(GIT_REVISION)  -t $(IMAGE_PREFIX)grafana-fire:$(IMAGE_TAG) -f grafana/Dockerfile .
endef

define deploy
	$(BIN)/kind export kubeconfig --name $(KIND_CLUSTER) || $(BIN)/kind create cluster --name $(KIND_CLUSTER)
	# Load image into nodes
	$(BIN)/kind load docker-image --name $(KIND_CLUSTER) $(IMAGE_PREFIX)fire:$(IMAGE_TAG)
	kubectl get pods
	$(BIN)/helm upgrade --install $(1) ./deploy/helm/fire $(2) \
		--set fire.image.tag=$(IMAGE_TAG) --set fire.service.port_name=http-metrics
endef

.PHONY: docker-image/fire/build
docker-image/fire/build:
	$(call docker_buildx,--load)

.PHONY: docker-image/fire/push
docker-image/fire/push:
	$(call docker_buildx,--push)

.PHONY: docker-image/grafana/build
docker-image/grafana/build:
	$(call docker_buildx_grafana,--load)

.PHONY: docker-image/grafana/push
docker-image/grafana/push:
	$(call docker_buildx_grafana,--push)

define UPDATER_CONFIG_JSON
{
  "repo_name": "deployment_tools",
  "destination_branch": "master",
  "wait_for_ci": true,
  "wait_for_ci_branch_prefix": "automation/fire-dev-deploy",
  "wait_for_ci_timeout": "10m",
  "wait_for_ci_required_status": [
    "continuous-integration/drone/push"
  ],
  "update_jsonnet_attribute_configs": [
    {
      "file_path": "ksonnet/environments/fire/waves/dev.libsonnet",
      "jsonnet_key": "fire",
      "jsonnet_value": "$(IMAGE_PREFIX)fire:$(IMAGE_TAG)"
    },
	{
      "file_path": "ksonnet/environments/fire/waves/dev.libsonnet",
      "jsonnet_key": "grafana",
      "jsonnet_value": "$(IMAGE_PREFIX)grafana-fire:$(IMAGE_TAG)"
    }
  ]
}
endef

.PHONY: docker-image/fire/deploy-dev-001
docker-image/fire/deploy-dev-001: export CONFIG_JSON:=$(call UPDATER_CONFIG_JSON)
docker-image/fire/deploy-dev-001: $(BIN)/updater
	$(BIN)/updater

.PHONY: clean
clean: ## Delete intermediate build artifacts
	@# -X only removes untracked files, -d recurses into directories, -f actually removes files/dirs
	git clean -Xdf

$(BIN)/buf: Makefile
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/bufbuild/buf/cmd/buf@v1.5.0

$(BIN)/golangci-lint: Makefile
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.48.0

$(BIN)/protoc-gen-go: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28.0

$(BIN)/protoc-gen-connect-go: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/bufbuild/connect-go/cmd/protoc-gen-connect-go@v0.1.0

$(BIN)/protoc-gen-connect-go-mux: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/grafana/connect-go-mux/cmd/protoc-gen-connect-go-mux@v0.1.1

$(BIN)/protoc-gen-go-vtproto: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/planetscale/vtprotobuf/cmd/protoc-gen-go-vtproto@v0.3.0

$(BIN)/protoc-gen-openapiv2: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@v2.10.3

$(BIN)/protoc-gen-grpc-gateway: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@v2.10.3

$(BIN)/gomodifytags: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/fatih/gomodifytags@v1.16.0

$(BIN)/kind: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install sigs.k8s.io/kind@v0.14.0

$(BIN)/tk: Makefile go.mod $(BIN)/jb
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/grafana/tanka/cmd/tk@v0.22.1

$(BIN)/jb: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/jsonnet-bundler/jsonnet-bundler/cmd/jb@v0.5.1

$(BIN)/helm: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install helm.sh/helm/v3/cmd/helm@v3.8.0

$(BIN)/kubeval: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/instrumenta/kubeval@v0.16.1

$(BIN)/mage: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/magefile/mage@v1.13.0

$(BIN)/updater: Makefile
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) GOPRIVATE=github.com/grafana/deployment_tools $(GO) install github.com/grafana/deployment_tools/drone/plugins/cmd/updater@d64d509

KIND_CLUSTER = fire-dev

.PHONY: helm/lint
helm/lint: $(BIN)/helm
	$(BIN)/helm lint ./deploy/helm/fire/

.PHONY: helm/check
helm/check: $(BIN)/kubeval $(BIN)/helm
	mkdir -p ./deploy/helm/fire/rendered/
	$(BIN)/helm template fire-dev ./deploy/helm/fire/ \
		| tee ./deploy/helm/fire/rendered/single-binary.yaml \
		| $(BIN)/kubeval --strict
	$(BIN)/helm template fire-dev ./deploy/helm/fire/ --values deploy/helm/fire/values-micro-services.yaml \
		| tee ./deploy/helm/fire/rendered/micro-services.yaml \
		| $(BIN)/kubeval --strict

.PHONY: deploy
deploy: $(BIN)/kind $(BIN)/helm docker-image/fire/build
	$(call deploy,fire-dev)

.PHONY: deploy-micro-services
deploy-micro-services: $(BIN)/kind $(BIN)/helm docker-image/fire/build
	$(call deploy,fire-micro-services,--values=deploy/helm/fire/values-micro-services.yaml)

.PHONY: deploy-monitoring
deploy-monitoring: $(BIN)/tk $(BIN)/kind tools/monitoring/environments/default/spec.json
	kubectl  --context="kind-$(KIND_CLUSTER)" create namespace monitoring --dry-run=client -o yaml | kubectl  --context="kind-$(KIND_CLUSTER)" apply -f -
	$(BIN)/tk apply tools/monitoring/environments/default/main.jsonnet

.PHONY: tools/monitoring/environments/default/spec.json # This is a phony target for now as the cluster might be not already created.
tools/monitoring/environments/default/spec.json: $(BIN)/tk $(BIN)/kind
	$(BIN)/kind export kubeconfig --name $(KIND_CLUSTER) || $(BIN)/kind create cluster --name $(KIND_CLUSTER)
	pushd tools/monitoring/ && rm -Rf vendor/ lib/ environments/default/spec.json  && PATH=$(BIN) $(BIN)/tk init -f
	echo "import 'monitoring.libsonnet'" > tools/monitoring/environments/default/main.jsonnet
	$(BIN)/tk env set tools/monitoring/environments/default --server=$(shell $(BIN)/kind get kubeconfig --name fire-dev | grep server: | sed 's/server://g' | xargs) --namespace=monitoring
