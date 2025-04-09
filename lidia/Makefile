GO ?= go
BIN := $(CURDIR)/.tmp/bin

.PHONY: all
all: lint test

.PHONY: test
test: go/test ## Run all tests

.PHONY: go/test
go/test: ## Run Go tests
	$(GO) test -v -race -cover ./...

.PHONY: lint
lint: go/lint ## Run all linters

.PHONY: go/lint
go/lint: $(BIN)/golangci-lint ## Lint Go code
	$(BIN)/golangci-lint run
	$(GO) vet ./...

$(BIN)/golangci-lint: Makefile
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.61.0

.PHONY: help
help: ## Describe useful make targets
	@grep -E '^[a-zA-Z_/]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
