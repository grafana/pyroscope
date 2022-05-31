.PHONY: build
build: go/bin

.PHONY: clean
clean:
	rm -rf bin

.PHONY: go/deps
go/deps:
	go mod tidy

.PHONY: go/bin
go/bin: go/deps
	mkdir -p ./bin
	go build -o bin/ ./cmd/fire

.PHONY: go/lint
go/lint:
	golangci-lint run
