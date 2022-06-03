.PHONY: all
all: lint test build

.PHONY: lint
lint: go/lint

.PHONY: test
test: go/test

.PHONY: go/test
go/test:
	go test -v ./...

.PHONY: build
build: go/bin

.PHONY: go/deps
go/deps:
	go mod tidy

.PHONY: go/bin
go/bin:
	mkdir -p ./bin
	go build -o bin/ ./cmd/fire

.PHONY: go/lint
go/lint:
	golangci-lint run

.PHONY: clean
clean:
	rm -rf bin
