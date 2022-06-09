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
	$(GO) build -o bin/ ./cmd/fire

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
