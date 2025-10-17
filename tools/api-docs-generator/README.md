# API Documentation Generator

A Go tool that generates unified API documentation from OpenAPI v3 YAML files for Pyroscope's Connect API.

## Usage

```bash
go run . [flags]
```

### Flags

- `-input string`: Directory containing OpenAPI YAML files (default: `api/connect-openapi/gen`)
- `-template string`: Template file used to generate markdown (default: `docs/sources/reference-server-api/index.template`)
- `-output string`: Output file for generated markdown (default: `docs/sources/reference-server-api/index.md`)
- `-help`: Show help information

### Example

```bash
# Generate documentation with default paths
go run .

# Generate documentation with custom paths
go run . -input ./openapi-specs -template ./custom.template -output ./api-docs.md
```

## Build

```bash
go build -o api-docs-generator .
```