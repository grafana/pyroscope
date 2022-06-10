GO ?= go
SHELL := bash
.DELETE_ON_ERROR:
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := all
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules
MAKEFLAGS += --no-print-directory
BIN := .tmp/bin
COPYRIGHT_YEARS := 2021-2022
LICENSE_IGNORE := -e /testdata/
GO_TEST_FLAGS ?= -v -race -cover

IMAGE_PLATFORM = linux/amd64

# Boiler plate for building Docker containers.
# All this must go at top of file I'm afraid.
IMAGE_PREFIX ?= us.gcr.io/kubernetes-dev/

IMAGE_TAG ?= $(shell ./tools/image-tag)
GIT_REVISION := $(shell git rev-parse --short HEAD)
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD)

GO_LDFLAGS   := -X $(VPREFIX).Branch=$(GIT_BRANCH) -X $(VPREFIX).Version=$(IMAGE_TAG) -X $(VPREFIX).Revision=$(GIT_REVISION) -X $(VPREFIX).BuildUser=$(shell whoami)@$(shell hostname) -X $(VPREFIX).BuildDate=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GO_FLAGS     := -ldflags "-extldflags \"-static\" -s -w $(GO_LDFLAGS)" -tags netgo -mod=mod

.PHONY: help
help: ## Describe useful make targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "%-30s %s\n", $$1, $$2}'

.PHONY: all
all: lint test build ## Build, test, and lint (default)

.PHONY: lint
lint: go/lint ## Lint Go and protobuf

.PHONY: test
test: go/test ## Run unit tests

.PHONY: generate
generate: $(BIN)/buf $(BIN)/protoc-gen-go $(BIN)/protoc-gen-connect-go ## Regenerate protobuf
	rm -rf pkg/gen/
	PATH=$(BIN) $(BIN)/buf generate

.PHONY: go/test
go/test:
	$(GO) test $(GO_TEST_FLAGS) ./...

.PHONY: build
build: go/bin ## Build all packages

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
	GO111MODULE=on go mod vendor

.PHONY: fmt
fmt: $(BIN)/golangci-lint $(BIN)/buf ## Automatically fix some lint errors
	git ls-files '*.go' | grep -v 'vendor/' | xargs gofmt -s -w
	# TODO: Reenable once golangci-lint support go 1.18 properly
	# $(BIN)/golangci-lint run --fix
	$(BIN)/buf format -w .

.PHONY: check/unstaged-changes
check/unstaged-changes:
	@git --no-pager diff --exit-code || { echo ">> There are unstaged changes in the working tree"; exit 1; }

.PHONY: check/go/mod
check/go/mod: go/mod
	@git --no-pager diff --exit-code -- go.sum go.mod vendor/ || { echo ">> There are unstaged changes in go vendoring run 'make go/mod'"; exit 1; }


define docker_buildx
	docker buildx build $(1) --platform $(IMAGE_PLATFORM) --build-arg=revision=$(GIT_REVISION) -t $(IMAGE_PREFIX)$(shell basename $(@D)) -t $(IMAGE_PREFIX)$(shell basename $(@D)):$(IMAGE_TAG) -f cmd/$(shell basename $(@D))/Dockerfile .
endef

.PHONY: docker-image/fire/build
docker-image/fire/build:
	$(call docker_buildx,--load)

.PHONY: docker-image/fire/push
docker-image/fire/push:
	$(call docker_buildx,--push)

.PHONY: clean
clean: ## Delete intermediate build artifacts
	@# -X only removes untracked files, -d recurses into directories, -f actually removes files/dirs
	git clean -Xdf

$(BIN)/buf: Makefile
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/bufbuild/buf/cmd/buf@v1.5.0

$(BIN)/golangci-lint: Makefile
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.46.2

$(BIN)/protoc-gen-go: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28.0

$(BIN)/protoc-gen-connect-go: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/bufbuild/connect-go/cmd/protoc-gen-connect-go@v0.1.0

$(BIN)/kind: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install sigs.k8s.io/kind@v0.14.0

$(BIN)/helm: Makefile go.mod
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install helm.sh/helm/v3/cmd/helm@v3.8.0

KIND_CLUSTER = fire-dev

.PHONY: deploy
deploy: $(BIN)/kind $(BIN)/helm docker-image/fire/build
	$(BIN)/kind export kubeconfig --name $(KIND_CLUSTER) || $(BIN)/kind create cluster --name $(KIND_CLUSTER)
	# Load image into nodes
	$(BIN)/kind load docker-image --name $(KIND_CLUSTER) $(IMAGE_PREFIX)fire:$(IMAGE_TAG)
	kubectl get pods
	$(BIN)/helm upgrade --install fire-dev ./deploy/helm/fire \
		--set image.tag=$(IMAGE_TAG)
