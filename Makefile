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

GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
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
EMBEDASSETS ?= embedassets

# Build flags
VPREFIX := github.com/grafana/phlare/pkg/util/build
GO_LDFLAGS   := -X $(VPREFIX).Branch=$(GIT_BRANCH) -X $(VPREFIX).Version=$(IMAGE_TAG) -X $(VPREFIX).Revision=$(GIT_REVISION) -X $(VPREFIX).BuildDate=$(GIT_LAST_COMMIT_DATE)

.PHONY: help
help: ## Describe useful make targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "%-30s %s\n", $$1, $$2}'

.PHONY: all
all: lint test build ## Build, test, and lint (default)

.PHONY: lint
lint: go/lint helm/lint buf/lint goreleaser/lint ## Lint Go, Helm and protobuf

.PHONY: test
test: go/test ## Run unit tests

.PHONY: generate
generate: $(BIN)/buf $(BIN)/protoc-gen-go $(BIN)/protoc-gen-go-vtproto $(BIN)/protoc-gen-openapiv2 $(BIN)/protoc-gen-grpc-gateway $(BIN)/protoc-gen-connect-go $(BIN)/protoc-gen-connect-go-mux $(BIN)/gomodifytags ## Regenerate protobuf
	rm -Rf api/openapiv2/gen/ api/gen
	find pkg/ \( -name \*.pb.go -o -name \*.connect\*.go \) -delete
	cd api/ && PATH=$(BIN) $(BIN)/buf generate
	cd pkg && PATH=$(BIN) $(BIN)/buf generate
	PATH=$(BIN):$(PATH) ./tools/add-parquet-tags.sh
	go run ./tools/doc-generator/ ./docs/sources/operators-guide/configure/reference-configuration-parameters/index.template > docs/sources/operators-guide/configure/reference-configuration-parameters/index.md

.PHONY: buf/lint
buf/lint: $(BIN)/buf
	cd api/ && $(BIN)/buf lint || true # TODO: Fix linting problems and remove the always true
	cd pkg && $(BIN)/buf lint || true # TODO: Fix linting problems and remove the always true

.PHONY: go/test
go/test: $(BIN)/gotestsum
	$(BIN)/gotestsum -- $(GO_TEST_FLAGS) ./...

.PHONY: build
build: frontend/build go/bin ## Do a production build (requiring the frontend build to be present)

.PHONY: build-dev
build-dev: ## Do a dev build (without requiring the frontend)
	$(MAKE) EMBEDASSETS="" go/bin

.PHONY: frontend/build
frontend/build: frontend/deps ## Do a production build for the frontend
	yarn build

.PHONY: frontend/deps
frontend/deps:
	yarn --frozen-lockfile

.PHONY: release
release/prereq: $(BIN)/goreleaser ## Ensure release pre requesites are met
	# remove local git tags coming from helm chart release
	git tag -d $(shell git tag -l "phlare-*" "api/*")
	# ensure there is a docker cli command
	@which docker || { apt-get update && apt-get install -y docker.io; }
	@docker info > /dev/null

.PHONY: release
release: release/prereq ## Create a release
	$(BIN)/goreleaser release -p=$(shell nproc) --rm-dist

.PHONY: release/prepare
release/prepare: release/prereq ## Prepare a release
	$(BIN)/goreleaser release -p=$(shell nproc) --rm-dist --snapshot

.PHONY: release/build/all
release/build/all: release/prereq ## Build all release binaries
	$(BIN)/goreleaser build -p=$(shell nproc) --rm-dist --snapshot

.PHONY: release/build
release/build: release/prereq ## Build current platform release binaries
	$(BIN)/goreleaser build -p=$(shell nproc) --rm-dist --snapshot --single-target

.PHONY: go/deps
go/deps:
	$(GO) mod tidy

define go_build
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 $(GO) build -tags "netgo $(EMBEDASSETS)" -ldflags "-extldflags \"-static\" $(1)" ./cmd/phlare
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 $(GO) build -ldflags "-extldflags \"-static\" $(1)" ./cmd/profilecli
endef

.PHONY: go/bin-debug
go/bin-debug:
	$(call go_build,$(GO_LDFLAGS))

.PHONY: go/bin
go/bin:
	$(call go_build,-s -w $(GO_LDFLAGS))

.PHONY: go/lint
go/lint: $(BIN)/golangci-lint
	$(BIN)/golangci-lint run
	$(GO) vet ./...

.PHONY: go/mod
go/mod:
	GO111MODULE=on go mod download
	# doesn't work for go workspace
	# GO111MODULE=on go mod verify
	go work sync
	GO111MODULE=on go mod tidy
	cd api/ && GO111MODULE=on go mod download
	cd api/ && GO111MODULE=on go mod tidy

.PHONY: fmt
fmt: $(BIN)/golangci-lint $(BIN)/buf $(BIN)/tk ## Automatically fix some lint errors
	git ls-files '*.go' | grep -v 'vendor/' | xargs gofmt -s -w
	$(BIN)/golangci-lint run --fix
	cd api/ && $(BIN)/buf format -w .
	cd pkg && $(BIN)/buf format -w .
	$(BIN)/tk fmt ./operations/phlare/jsonnet/ tools/monitoring/

.PHONY: check/unstaged-changes
check/unstaged-changes:
	@git --no-pager diff --exit-code || { echo ">> There are unstaged changes in the working tree"; exit 1; }

.PHONY: check/go/mod
check/go/mod: go/mod
	@git --no-pager diff --exit-code -- go.sum go.mod vendor/ || { echo ">> There are unstaged changes in go vendoring run 'make go/mod'"; exit 1; }


define docker_buildx
	docker buildx build $(1) --platform $(IMAGE_PLATFORM) $(BUILDX_ARGS) --build-arg=revision=$(GIT_REVISION) -t $(IMAGE_PREFIX)$(shell basename $(@D)) -t $(IMAGE_PREFIX)$(shell basename $(@D)):$(IMAGE_TAG) -f cmd/$(shell basename $(@D))/$(2)Dockerfile .
endef

define deploy
	$(BIN)/kind export kubeconfig --name $(KIND_CLUSTER) || $(BIN)/kind create cluster --name $(KIND_CLUSTER)
	# Load image into nodes
	$(BIN)/kind load docker-image --name $(KIND_CLUSTER) $(IMAGE_PREFIX)phlare:$(IMAGE_TAG)
	kubectl get pods
	$(BIN)/helm upgrade --install $(1) ./operations/phlare/helm/phlare $(2) \
		--set phlare.image.tag=$(IMAGE_TAG) \
		--set phlare.image.repository=$(IMAGE_PREFIX)phlare \
		--set phlare.podAnnotations.image-id=$(shell cat .docker-image-id-phlare) \
		--set phlare.service.port_name=http-metrics \
		--set phlare.podAnnotations."profiles\.grafana\.com\/memory\.port_name"=http-metrics \
		--set phlare.podAnnotations."profiles\.grafana\.com\/cpu\.port_name"=http-metrics \
		--set phlare.podAnnotations."profiles\.grafana\.com\/goroutine\.port_name"=http-metrics \
		--set phlare.components.querier.resources=null --set phlare.components.distributor.resources=null --set phlare.components.ingester.resources=null
endef

.PHONY: docker-image/phlare/build-debug
docker-image/phlare/build-debug: GOOS=linux GOARCH=amd64
docker-image/phlare/build-debug: frontend/build go/bin-debug $(BIN)/dlv
	$(call docker_buildx,--load,debug.)

.PHONY: docker-image/phlare/build
docker-image/phlare/build: GOOS=linux GOARCH=amd64
docker-image/phlare/build: frontend/build go/bin
	$(call docker_buildx,--load --iidfile .docker-image-id-phlare)

.PHONY: docker-image/phlare/push
docker-image/phlare/push: frontend/build go/bin
	$(call docker_buildx,--push)

define UPDATER_CONFIG_JSON
{
  "repo_name": "deployment_tools",
  "destination_branch": "master",
  "wait_for_ci": true,
  "wait_for_ci_branch_prefix": "automation/phlare-dev-deploy",
  "wait_for_ci_timeout": "10m",
  "wait_for_ci_required_status": [
    "continuous-integration/drone/push"
  ],
  "update_jsonnet_attribute_configs": [
    {
      "file_path": "ksonnet/environments/phlare/waves/dev.libsonnet",
      "jsonnet_key": "phlare",
      "jsonnet_value": "$(IMAGE_PREFIX)phlare:$(IMAGE_TAG)"
    }
  ]
}
endef

.PHONY: docker-image/phlare/deploy-dev-001
docker-image/phlare/deploy-dev-001: export CONFIG_JSON:=$(call UPDATER_CONFIG_JSON)
docker-image/phlare/deploy-dev-001: $(BIN)/updater
	$(BIN)/updater

.PHONY: clean
clean: ## Delete intermediate build artifacts
	@# -X only removes untracked files, -d recurses into directories, -f actually removes files/dirs
	git clean -Xdf

.PHONY: reference-help
reference-help: ## Generates the reference help documentation.
reference-help: build
	@(./phlare -h || true) > cmd/phlare/help.txt.tmpl
	@(./phlare -help-all || true) > cmd/phlare/help-all.txt.tmpl

$(BIN)/buf: Makefile
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/bufbuild/buf/cmd/buf@v1.5.0

$(BIN)/golangci-lint: Makefile
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.52.2

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
	GOBIN=$(abspath $(@D)) $(GO) install sigs.k8s.io/kind@v0.17.0

$(BIN)/tk: Makefile go.mod $(BIN)/jb
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/grafana/tanka/cmd/tk@v0.24.0

$(BIN)/jb: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/jsonnet-bundler/jsonnet-bundler/cmd/jb@v0.5.1

$(BIN)/helm: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install helm.sh/helm/v3/cmd/helm@v3.8.0

$(BIN)/kubeconform: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/yannh/kubeconform/cmd/kubeconform@v0.5.0

$(BIN)/mage: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/magefile/mage@v1.13.0

$(BIN)/updater: Makefile
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) GOPRIVATE=github.com/grafana/deployment_tools $(GO) install github.com/grafana/deployment_tools/drone/plugins/cmd/updater@d64d509

$(BIN)/goreleaser: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/goreleaser/goreleaser@v1.14.1

$(BIN)/gotestsum: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install gotest.tools/gotestsum@v1.9.0

$(BIN)/dlv: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) CGO_ENABLED=0 $(GO) install -ldflags "-s -w -extldflags '-static'" github.com/go-delve/delve/cmd/dlv@v1.20.1

$(BIN)/trunk: Makefile
	@mkdir -p $(@D)
	curl -L https://trunk.io/releases/trunk -o $(@D)/trunk
	chmod +x $(@D)/trunk

KIND_CLUSTER = phlare-dev

.PHONY: helm/lint
helm/lint: $(BIN)/helm
	$(BIN)/helm lint ./operations/phlare/helm/phlare/

helm/docs: $(BIN)/helm
	docker run --rm --volume "$(CURDIR)/operations/phlare/helm:/helm-docs" -u "$(shell id -u)" jnorwood/helm-docs:v1.8.1

.PHONY: goreleaser/lint
goreleaser/lint: $(BIN)/goreleaser
	$(BIN)/goreleaser check

.PHONY: trunk/lint
trunk/lint: $(BIN)/trunk
	$(BIN)/trunk check

.PHONY: trunk/fmt
trunk/fmt: $(BIN)/trunk
	$(BIN)/trunk fmt

.PHONY: helm/check
helm/check: $(BIN)/kubeconform $(BIN)/helm
	$(BIN)/helm repo add --force-update minio https://charts.min.io/
	$(BIN)/helm dependency build ./operations/phlare/helm/phlare/
	mkdir -p ./operations/phlare/helm/phlare/rendered/
	$(BIN)/helm template phlare-dev ./operations/phlare/helm/phlare/ \
		| tee ./operations/phlare/helm/phlare/rendered/single-binary.yaml \
		| $(BIN)/kubeconform --summary --strict --kubernetes-version 1.21.0
	$(BIN)/helm template phlare-dev ./operations/phlare/helm/phlare/ --values operations/phlare/helm/phlare/values-micro-services.yaml \
		| tee ./operations/phlare/helm/phlare/rendered/micro-services.yaml \
		| $(BIN)/kubeconform --summary --strict --kubernetes-version 1.21.0
	cat operations/phlare/helm/phlare/values-micro-services.yaml \
		| go run ./tools/yaml-to-json \
		> ./operations/phlare/jsonnet/values-micro-services.json
	cat operations/phlare/helm/phlare/values.yaml \
		| go run ./tools/yaml-to-json \
		> ./operations/phlare/jsonnet/values.json

.PHONY: deploy
deploy: $(BIN)/kind $(BIN)/helm docker-image/phlare/build
	$(call deploy,phlare-dev,--set=phlare.extraEnvVars.JAEGER_AGENT_HOST=jaeger.monitoring.svc.cluster.local.)

.PHONY: deploy-micro-services
deploy-micro-services: $(BIN)/kind $(BIN)/helm docker-image/phlare/build
	$(call deploy,phlare-micro-services,--values=operations/phlare/helm/phlare/values-micro-services.yaml --set=phlare.extraEnvVars.JAEGER_AGENT_HOST=jaeger.monitoring.svc.cluster.local.)

.PHONY: deploy-monitoring
deploy-monitoring: $(BIN)/tk $(BIN)/kind tools/monitoring/environments/default/spec.json
	kubectl  --context="kind-$(KIND_CLUSTER)" create namespace monitoring --dry-run=client -o yaml | kubectl  --context="kind-$(KIND_CLUSTER)" apply -f -
	$(BIN)/tk apply tools/monitoring/environments/default/main.jsonnet

.PHONY: tools/monitoring/environments/default/spec.json # This is a phony target for now as the cluster might be not already created.
tools/monitoring/environments/default/spec.json: $(BIN)/tk $(BIN)/kind
	$(BIN)/kind export kubeconfig --name $(KIND_CLUSTER) || $(BIN)/kind create cluster --name $(KIND_CLUSTER)
	pushd tools/monitoring/ && rm -Rf vendor/ lib/ environments/default/spec.json  && PATH=$(BIN):$(PATH) $(BIN)/tk init -f
	echo "import 'monitoring.libsonnet'" > tools/monitoring/environments/default/main.jsonnet
	$(BIN)/tk env set tools/monitoring/environments/default --server=$(shell $(BIN)/kind get kubeconfig --name phlare-dev | grep server: | sed 's/server://g' | xargs) --namespace=monitoring

.PHONY: deploy-demo
deploy-demo: $(BIN)/kind
	docker build -t cp-java-simple:0.1.0 ./tools/docker-compose/java/simple
	$(BIN)/kind load docker-image --name $(KIND_CLUSTER) cp-java-simple:0.1.0
	kubectl  --context="kind-$(KIND_CLUSTER)" apply -f ./tools/kubernetes/java-simple-deployment.yaml

.PHONY: docs/%
docs/%:
	$(MAKE) -C docs $*
