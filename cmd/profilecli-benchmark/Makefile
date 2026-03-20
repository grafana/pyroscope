.PHONY: build clean run run-minimal init help

# Build the benchmark tool
build:
	@echo "Building profilecli-benchmark..."
	GOWORK=off go build -o profilecli-benchmark .

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f profilecli-benchmark

# Run with minimal config (no Google Sheets upload)
run-minimal: build
	@echo "Running minimal benchmark..."
	./profilecli-benchmark --config benchmark-minimal.yaml

# Run with default config
run: build
	@echo "Running benchmark with default config..."
	./profilecli-benchmark

# Initialize go modules
init:
	@echo "Initializing Go modules..."
	go mod download
	go mod tidy

# Help target
help:
	@echo "Available targets:"
	@echo "  build         - Build the benchmark tool"
	@echo "  clean         - Remove build artifacts"
	@echo "  run-minimal   - Run with minimal config (no Google Sheets)"
	@echo "  run           - Run with default config (benchmark.yaml)"
	@echo "  init          - Download and tidy Go modules"
	@echo "  help          - Show this help message"
	@echo ""
	@echo "Examples:"
	@echo "  make build"
	@echo "  make run-minimal"
	@echo "  make run"
	@echo "  ./profilecli-benchmark --config my-config.yaml"
