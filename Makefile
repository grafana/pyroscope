.PHONY: build clean go/deps go/bin go/deps go/lint

build: go/bin

clean:
	rm -rf bin

go/deps:
	go mod tidy

go/bin:
	mkdir -p ./bin
	go build -o bin/ ./cmd/fire

go/lint:
	golangci-lint run
