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
GO_MOD_VERSION := 1.23.0

GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
IMAGE_PLATFORM = linux/amd64
BUILDX_ARGS =
GOPRIVATE=github.com/grafana/frostdb

# Boiler plate for building Docker containers.
# All this must go at top of file I'm afraid.
IMAGE_PREFIX ?= grafana/

IMAGE_TAG ?= $(shell ./tools/image-tag)
GIT_REVISION := $(shell git rev-parse --short HEAD)
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD)
GIT_LAST_COMMIT_DATE := $(shell git log -1 --date=iso-strict --format=%cd)
EMBEDASSETS ?= embedassets

UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
	NPROC := $(shell sysctl -n hw.physicalcpu)
else
	NPROC := $(shell nproc)
endif

# Build flags
VPREFIX := github.com/grafana/pyroscope/pkg/util/build
GO_LDFLAGS   := -X $(VPREFIX).Branch=$(GIT_BRANCH) -X $(VPREFIX).Version=$(IMAGE_TAG) -X $(VPREFIX).Revision=$(GIT_REVISION) -X $(VPREFIX).BuildDate=$(GIT_LAST_COMMIT_DATE)
GO_GCFLAGS_DEBUG := all="-N -l"

# Folders with go.mod file
GO_MOD_PATHS := api/ ebpf/ lidia/ examples/language-sdk-instrumentation/golang-push/rideshare examples/language-sdk-instrumentation/golang-push/rideshare-alloy examples/language-sdk-instrumentation/golang-push/rideshare-k6 examples/language-sdk-instrumentation/golang-push/simple/ examples/tracing/golang-push/ examples/golang-pgo/

# Add extra arguments to helm commands
HELM_ARGS =

# Local deployment params
KIND_CLUSTER = pyroscope-dev

.PHONY: help
help: ## Describe useful make targets
	@grep -E '^[a-zA-Z_/-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ": .*?## "}; {printf "%-50s %s\n", $$1, $$2}'

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
	PATH="$(BIN):$(PATH)" ./tools/add-parquet-tags.sh
	go run ./tools/doc-generator/ ./docs/sources/configure-server/reference-configuration-parameters/index.template > docs/sources/configure-server/reference-configuration-parameters/index.md

.PHONY: buf/lint
buf/lint: $(BIN)/buf
	cd api/ && $(BIN)/buf lint || true # TODO: Fix linting problems and remove the always true
	cd pkg && $(BIN)/buf lint || true # TODO: Fix linting problems and remove the always true

EBPF_TESTS='^TestEBPF.*'

.PHONY: go/test
go/test: $(BIN)/gotestsum
ifeq ($(GOOS),darwin)
	$(BIN)/gotestsum --rerun-fails=2 --packages './... ./lidia/...' -- $(GO_TEST_FLAGS)
else
	$(BIN)/gotestsum --rerun-fails=2 --packages './... ./ebpf/... ./lidia/...' -- $(GO_TEST_FLAGS) -skip $(EBPF_TESTS)
endif

# Run test on examples
# This can also be used to run it on a subset of tests
# $ make examples/test RUN=TestDockerComposeBuildRun/tracing/java
.PHONY: examples/test
examples/test: RUN := .*
examples/test: $(BIN)/gotestsum
	$(BIN)/gotestsum --format testname --rerun-fails=2 --packages ./examples -- --count 1 --parallel 2 --timeout 1h --tags examples -run "$(RUN)"

.PHONY: build
build: frontend/build go/bin ## Do a production build (requiring the frontend build to be present)

.PHONY: build-dev
build-dev: ## Do a dev build (without requiring the frontend)
	$(MAKE) EMBEDASSETS="" go/bin

.PHONY: frontend/build
frontend/build:
	docker build -f cmd/pyroscope/frontend.Dockerfile --output=public/build .

.PHONY: frontend/shell
frontend/shell:
	docker build -f cmd/pyroscope/frontend.Dockerfile --iidfile .docker-image-digest-frontend --target builder .
	docker run -t -i $$(cat .docker-image-digest-frontend) /bin/bash

.PHONY: profilecli/build
profilecli/build: go/bin-profilecli ## Build the profilecli binary

.PHONY: pyroscope/build
pyroscope/build: go/bin-pyroscope ## Build just the pyroscope binary

.PHONY: release
release/prereq: $(BIN)/goreleaser ## Ensure release pre requesites are met
	# remove local git tags coming from helm chart release
	git tag -d $(shell git tag -l "phlare-*" "api/*" "ebpf/*" "@pyroscope*")
	# ensure there is a docker cli command
	@which docker || { apt-get update && apt-get install -y docker.io; }
	@docker info > /dev/null

.PHONY: release
release: release/prereq ## Create a release
	$(BIN)/goreleaser release -p=$(NPROC) --clean

.PHONY: release/prepare
release/prepare: release/prereq ## Prepare a release
	$(BIN)/goreleaser release -p=$(NPROC) --clean --snapshot

.PHONY: release/build/all
release/build/all: release/prereq ## Build all release binaries
	$(BIN)/goreleaser build -p=$(NPROC) --clean --snapshot

.PHONY: release/build
release/build: release/prereq ## Build current platform release binaries
	$(BIN)/goreleaser build -p=$(NPROC) --clean --snapshot --single-target

.PHONY: go/deps
go/deps:
	$(GO) mod tidy

define go_build_pyroscope
	GOOS=$(GOOS) GOARCH=$(GOARCH) GOAMD64=v2 CGO_ENABLED=0 $(GO) build -tags "netgo $(EMBEDASSETS)" -ldflags "-extldflags \"-static\" $(1)" -gcflags=$(2) ./cmd/pyroscope
endef

define go_build_profilecli
	GOOS=$(GOOS) GOARCH=$(GOARCH) GOAMD64=v2 CGO_ENABLED=0 $(GO) build -ldflags "-extldflags \"-static\" $(1)" -gcflags=$(2) ./cmd/profilecli
endef

.PHONY: go/bin-debug
go/bin-debug:
	$(call go_build_pyroscope,$(GO_LDFLAGS),$(GO_GCFLAGS_DEBUG))
	$(call go_build_profilecli,$(GO_LDFLAGS),$(GO_GCFLAGS_DEBUG))

.PHONY: go/bin
go/bin:
	$(call go_build_pyroscope,-s -w $(GO_LDFLAGS),)
	$(call go_build_profilecli,-s -w $(GO_LDFLAGS),)

.PHONY: go/bin-pyroscope-debug
go/bin-pyroscope-debug:
	$(call go_build_pyroscope,$(GO_LDFLAGS),$(GO_GCFLAGS_DEBUG))

.PHONY: go/bin-profilecli-debug
go/bin-profilecli-debug:
	$(call go_build_profilecli,$(GO_LDFLAGS),$(GO_GCFLAGS_DEBUG))

.PHONY: go/lint
go/lint: $(BIN)/golangci-lint
	$(BIN)/golangci-lint run ./... ./lidia/...
	$(GO) vet ./... ./lidia/...

.PHONY: update-contributors
update-contributors: ## Update the contributors in README.md
	go run ./tools/update-contributors

.PHONY: go/mod
go/mod: $(foreach P,$(GO_MOD_PATHS),go/mod_tidy/$P)

.PHONY: go/mod_tidy_root
go/mod_tidy_root:
	GO111MODULE=on go mod download
	# doesn't work for go workspace
	# GO111MODULE=on go mod verify
	go work sync
	GO111MODULE=on go mod tidy -go $(GO_MOD_VERSION)

.PHONY: go/mod_tidy/%
go/mod_tidy/%: go/mod_tidy_root
	cd "$*" && GO111MODULE=on go mod download
	cd "$*" && GO111MODULE=on go mod tidy -go $(GO_MOD_VERSION)

.PHONY: fmt
fmt: $(BIN)/golangci-lint $(BIN)/buf $(BIN)/tk ## Automatically fix some lint errors
	git ls-files '*.go' | grep -v 'vendor/' | grep -v 'og/' | xargs gofmt -s -w
	$(BIN)/golangci-lint run --fix
	cd api/ && $(BIN)/buf format -w .
	cd pkg && $(BIN)/buf format -w .
	$(BIN)/tk fmt ./operations/pyroscope/jsonnet/ tools/monitoring/

.PHONY: check/unstaged-changes
check/unstaged-changes:
	@git --no-pager diff --exit-code || { echo ">> There are unstaged changes in the working tree"; exit 1; }

.PHONY: check/go/mod
check/go/mod: go/mod
	@git --no-pager diff --exit-code -- go.sum go.mod vendor/ || { echo ">> There are unstaged changes in go vendoring run 'make go/mod'"; exit 1; }


define docker_buildx
	docker buildx build $(1) --platform $(IMAGE_PLATFORM) $(BUILDX_ARGS) --build-arg=revision=$(GIT_REVISION) -t $(IMAGE_PREFIX)$(shell basename $(@D)):$(2)$(IMAGE_TAG) -f cmd/$(shell basename $(@D))/$(2)Dockerfile .
endef

define deploy
	$(BIN)/kind export kubeconfig --name $(KIND_CLUSTER) || $(BIN)/kind create cluster --name $(KIND_CLUSTER)
	# Load image into nodes
	$(BIN)/kind load docker-image --name $(KIND_CLUSTER) $(IMAGE_PREFIX)pyroscope:$(IMAGE_TAG)
	kubectl get pods
	$(BIN)/helm upgrade --install $(1) ./operations/pyroscope/helm/pyroscope $(2) $(HELM_ARGS) \
		--set pyroscope.image.tag=$(IMAGE_TAG) \
		--set pyroscope.image.repository=$(IMAGE_PREFIX)pyroscope \
		--set pyroscope.podAnnotations.image-digest=$(shell cat .docker-image-digest-pyroscope) \
		--set pyroscope.service.port_name=http-metrics \
		--set pyroscope.podAnnotations."profiles\.grafana\.com\/memory\.port_name"=http-metrics \
		--set pyroscope.podAnnotations."profiles\.grafana\.com\/cpu\.port_name"=http-metrics \
		--set pyroscope.podAnnotations."profiles\.grafana\.com\/goroutine\.port_name"=http-metrics \
		--set pyroscope.extraEnvVars.JAEGER_AGENT_HOST=jaeger.monitoring.svc.cluster.local. \
		--set pyroscope.extraArgs."pyroscopedb\.max-block-duration"=5m
endef

# Function to handle multiarch image build. Depending on the
# debug_build and push_image args, we run one of:
#  - docker-image/pyroscope/build
#  - docker-image/pyroscope/build-debug
#  - docker-image/pyroscope/push
#  - docker-image/pyroscope/push-debug
define multiarch_build
	$(eval push_image=$(1))
	$(eval debug_build=$(2))
	$(eval build_cmd=docker-image/pyroscope/$(if $(push_image),push,build)$(if $(debug_build),-debug))
	$(eval image_name=$(IMAGE_PREFIX)$(shell basename $(@D)):$(if $(debug_build),debug.)$(IMAGE_TAG))

	GOOS=linux GOARCH=arm64 IMAGE_TAG="$(IMAGE_TAG)-arm64" $(MAKE) $(build_cmd) IMAGE_PLATFORM=linux/arm64
	GOOS=linux GOARCH=amd64 IMAGE_TAG="$(IMAGE_TAG)-amd64" $(MAKE) $(build_cmd) IMAGE_PLATFORM=linux/amd64

	$(if $(push_image), docker buildx imagetools create --tag "$(image_name)" "$(image_name)-amd64" "$(image_name)-arm64")
	$(if $(push_image), docker buildx imagetools inspect "$(image_name)" --format "{{json .Manifest.Digest}}" | tr -d '"' > .docker-image-digest-pyroscope)
	$(if $(push_image), echo "$(image_name)" > .docker-image-name-pyroscope)
endef

.PHONY: docker-image/pyroscope/build-multiarch
docker-image/pyroscope/build-multiarch:
	$(call multiarch_build,,)

.PHONY: docker-image/pyroscope/build-multiarch-debug
docker-image/pyroscope/build-multiarch-debug:
	$(call multiarch_build,,debug)

.PHONY: docker-image/pyroscope/push-multiarch
docker-image/pyroscope/push-multiarch:
	$(call multiarch_build,push,)

.PHONY: docker-image/pyroscope/push-multiarch-debug
docker-image/pyroscope/push-multiarch-debug:
	$(call multiarch_build,push,debug)

.PHONY: docker-image/pyroscope/build-debug
docker-image/pyroscope/build-debug: frontend/build go/bin-debug docker-image/pyroscope/dlv
	$(call docker_buildx,--load,debug.)

.PHONY: docker-image/pyroscope/push-debug
docker-image/pyroscope/push-debug: frontend/build go/bin-debug docker-image/pyroscope/dlv
	$(call docker_buildx,--push,debug.)

.PHONY: docker-image/pyroscope/build
docker-image/pyroscope/build: frontend/build go/bin
	$(call docker_buildx,--load --iidfile .docker-image-digest-pyroscope,)

.PHONY: docker-image/pyroscope/push
docker-image/pyroscope/push: frontend/build go/bin
	$(call docker_buildx,--push,)

.PHONY: docker-image/pyroscope/dlv
docker-image/pyroscope/dlv:
	# dlv is not intended for local use and is to be installed in the
	# platform-specific docker image together with the main Pyroscope binary.
	@mkdir -p $(@D)
	GOPATH=$(CURDIR)/.tmp GOAMD64=v2 CGO_ENABLED=0 $(GO) install -ldflags "-s -w -extldflags '-static'" github.com/go-delve/delve/cmd/dlv@v1.23.0
	mv $(CURDIR)/.tmp/bin/$(GOOS)_$(GOARCH)/dlv $(CURDIR)/.tmp/bin/dlv

.PHONY: clean
clean: ## Delete intermediate build artifacts
	@# -X only removes untracked files, -d recurses into directories, -f actually removes files/dirs
	git clean -Xdf

.PHONY: reference-help
reference-help: ## Generates the reference help documentation.
reference-help: go/bin
	@(./pyroscope -h || true) > cmd/pyroscope/help.txt.tmpl
	@(./pyroscope -help-all || true) > cmd/pyroscope/help-all.txt.tmpl

$(BIN)/buf: Makefile
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/bufbuild/buf/cmd/buf@v1.31.0

$(BIN)/golangci-lint: Makefile
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.2.2

$(BIN)/protoc-gen-go: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.5

$(BIN)/protoc-gen-connect-go: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install connectrpc.com/connect/cmd/protoc-gen-connect-go@v1.18.1

$(BIN)/protoc-gen-connect-go-mux: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/grafana/connect-go-mux/cmd/protoc-gen-connect-go-mux@v0.2.0

$(BIN)/protoc-gen-go-vtproto: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/planetscale/vtprotobuf/cmd/protoc-gen-go-vtproto@v0.6.0

$(BIN)/protoc-gen-openapiv2: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@v2.25.1

$(BIN)/protoc-gen-grpc-gateway: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@v2.25.1

$(BIN)/gomodifytags: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/fatih/gomodifytags@v1.16.0

$(BIN)/kind: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install sigs.k8s.io/kind@v0.25.0

$(BIN)/tk: Makefile go.mod $(BIN)/jb
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/grafana/tanka/cmd/tk@v0.24.0

$(BIN)/jb: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/jsonnet-bundler/jsonnet-bundler/cmd/jb@v0.5.1

$(BIN)/helm: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install helm.sh/helm/v3/cmd/helm@v3.14.3

$(BIN)/kubeconform: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/yannh/kubeconform/cmd/kubeconform@v0.6.4

$(BIN)/mage: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/magefile/mage@v1.13.0

$(BIN)/mockery: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/vektra/mockery/v2@v2.45.0

# Note: When updating the goreleaser version also update .github/workflow/release.yml and .git/workflow/weekly-release.yaml
$(BIN)/goreleaser: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/goreleaser/goreleaser/v2@v2.7.0

$(BIN)/gotestsum: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install gotest.tools/gotestsum@v1.12.0

.PHONY: cve/check
cve/check:
	docker run -t -i --rm --volume "$(CURDIR)/:/repo" -u "$(shell id -u)" aquasec/trivy:0.45.1 filesystem --cache-dir /repo/.cache/trivy --scanners vuln --skip-dirs .tmp/ --skip-dirs node_modules/ --skip-dirs tools/monitoring/vendor/ /repo

.PHONY: helm/lint
helm/lint: $(BIN)/helm
	$(BIN)/helm lint ./operations/pyroscope/helm/pyroscope/

.PHONY: helm/docs
helm/docs: $(BIN)/helm
	docker run --rm --volume "$(CURDIR)/operations/pyroscope/helm:/helm-docs" -u "$(shell id -u)" jnorwood/helm-docs:v1.8.1

.PHONY: goreleaser/lint
goreleaser/lint: $(BIN)/goreleaser
	$(BIN)/goreleaser check

.PHONY: helm/check
helm/check: $(BIN)/kubeconform $(BIN)/helm
	$(BIN)/helm repo add --force-update minio https://charts.min.io/
	$(BIN)/helm repo add --force-update grafana https://grafana.github.io/helm-charts
	$(BIN)/helm dependency update ./operations/pyroscope/helm/pyroscope/
	$(BIN)/helm dependency build ./operations/pyroscope/helm/pyroscope/
	mkdir -p ./operations/pyroscope/helm/pyroscope/rendered/
	$(BIN)/helm template -n default --kube-version "1.23.0" pyroscope-dev ./operations/pyroscope/helm/pyroscope/ \
		| tee ./operations/pyroscope/helm/pyroscope/rendered/single-binary.yaml \
		| $(BIN)/kubeconform --summary --strict --kubernetes-version 1.23.0
	$(BIN)/helm template -n default --kube-version "1.23.0" pyroscope-dev ./operations/pyroscope/helm/pyroscope/ --values operations/pyroscope/helm/pyroscope/values-micro-services.yaml \
		| tee ./operations/pyroscope/helm/pyroscope/rendered/micro-services.yaml \
		| $(BIN)/kubeconform --summary --strict --kubernetes-version 1.23.0
	$(BIN)/helm template -n default --kube-version "1.23.0" pyroscope-dev ./operations/pyroscope/helm/pyroscope/ --values operations/pyroscope/helm/pyroscope/values-micro-services-hpa.yaml \
		| tee ./operations/pyroscope/helm/pyroscope/rendered/micro-services-hpa.yaml \
		| $(BIN)/kubeconform --summary --strict --kubernetes-version 1.23.0
	cat operations/pyroscope/helm/pyroscope/values-micro-services.yaml \
		| go run ./tools/yaml-to-json \
		> ./operations/pyroscope/jsonnet/values-micro-services.json
		cat operations/pyroscope/helm/pyroscope/values-micro-services-hpa.yaml \
		| go run ./tools/yaml-to-json \
		> ./operations/pyroscope/jsonnet/values-micro-services-hpa.json
	cat operations/pyroscope/helm/pyroscope/values.yaml \
		| go run ./tools/yaml-to-json \
		> ./operations/pyroscope/jsonnet/values.json

.PHONY: deploy
deploy: $(BIN)/kind $(BIN)/helm docker-image/pyroscope/build
	$(call deploy,pyroscope-dev,)
	# Create a service to provide the same endpoint as micro-services
	echo '{"kind":"Service","apiVersion":"v1","metadata":{"name":"pyroscope-micro-services-query-frontend"},"spec":{"ports":[{"name":"pyroscope","port":4040,"targetPort":4040}],"selector":{"app.kubernetes.io/component":"all","app.kubernetes.io/instance":"pyroscope-dev"},"type":"ClusterIP"}}' | kubectl apply -f -

.PHONY: deploy-micro-services
deploy-micro-services: $(BIN)/kind $(BIN)/helm docker-image/pyroscope/build
	# Ensure to delete existing service, that has been created manually by the deploy target
	kubectl delete svc --field-selector metadata.name=pyroscope-micro-services-query-frontend -l app.kubernetes.io/managed-by!=Helm || true
	$(call deploy,pyroscope-micro-services,--values=operations/pyroscope/helm/pyroscope/values-micro-services.yaml --set pyroscope.components.querier.resources=null --set pyroscope.components.distributor.resources=null --set pyroscope.components.ingester.resources=null --set pyroscope.components.store-gateway.resources=null --set pyroscope.components.compactor.resources=null)

.PHONY: deploy-monitoring
deploy-monitoring: $(BIN)/tk $(BIN)/kind tools/monitoring/environments/default/spec.json
	kubectl  --context="kind-$(KIND_CLUSTER)" create namespace monitoring --dry-run=client -o yaml | kubectl  --context="kind-$(KIND_CLUSTER)" apply -f -
	$(BIN)/tk apply tools/monitoring/environments/default/main.jsonnet

.PHONY: tools/monitoring/environments/default/spec.json # This is a phony target for now as the cluster might be not already created.
tools/monitoring/environments/default/spec.json: $(BIN)/tk $(BIN)/kind
	$(BIN)/kind export kubeconfig --name $(KIND_CLUSTER) || $(BIN)/kind create cluster --name $(KIND_CLUSTER)
	pushd tools/monitoring/ && rm -Rf vendor/ lib/ environments/default/spec.json  && PATH=$(BIN):$(PATH) $(BIN)/tk init -f
	echo "import 'monitoring.libsonnet'" > tools/monitoring/environments/default/main.jsonnet
	$(BIN)/tk env set tools/monitoring/environments/default --server=$(shell $(BIN)/kind get kubeconfig --name pyroscope-dev | grep server: | sed 's/server://g' | xargs) --namespace=monitoring

include Makefile.examples

.PHONY: docs/%
docs/%:
	$(MAKE) -C docs $*

.PHONY: run
run: ## Run the pyroscope binary (pass parameters with 'make run PARAMS=-myparam')
	./pyroscope $(PARAMS)

.PHONY: mockery
mockery: $(BIN)/mockery
	$(BIN)/mockery
