# Pyroscope - AI Agent Development Guide

This document provides context and guidance for AI coding assistants (Claude, Cursor, GitHub Copilot, etc.) working on the Pyroscope codebase.

## What is Pyroscope?

Pyroscope is a horizontally scalable, highly available, multi-tenant continuous profiling aggregation system. 
It's designed to store and query profiling data at scale, similar to how Prometheus works for metrics and Loki for logs.

**Key Characteristics:**
- Written in **Go**
- Microservices-based architecture inspired by Cortex/Mimir/Loki
- Stores profiling data in object storage (S3, GCS, Azure, etc.)
- Multi-tenant by design

## Architecture Overview

Pyroscope uses a **microservices architecture** where a single binary can run different components based on the `-target` parameter.

### V1 Components

**Write Path:**
- **Distributor**: Receives profile ingestion requests, validates, and forwards to ingesters
- **Ingester**: Stores profiles in memory, periodically flushes to disk as blocks, periodically uploads blocks to long-term object storage
- **Compactor**: Merges blocks and removes duplicates

**Read Path:**
- **Query Frontend**: Entry point for queries, handles query splitting and caching
- **Query Scheduler**: Manages query queue and ensures fair execution across tenants
- **Querier**: Executes queries by fetching data from ingesters and store-gateways
- **Store Gateway**: Indexes and serves blocks from long-term object storage

### V2 Components

**Write Path:**
- **Distributor**: Receives profile ingestion requests, validates, and forwards to segment writers
- **Segment Writer**: Writes block segments to long-term object storage and the block metadata to metastore
- **Metastore**: Maintains an index for the block metadata and coordinates the block compaction process
- **Compaction Worker**: Merges small segments into larger blocks

**Read Path:**
- **Query Frontend**: Entry point for queries, creates the query plan and executes it against query backends
- **Query Backend**: Executes queries and merges query responses

### Storage

- **Block Format**: Profiles stored in Parquet tables, series data in a TSDB index, symbols in a custom format
- **Multi-tenant**: Each tenant has isolated storage
- **Object Storage**: Primary storage backend (S3, GCS, Azure, local filesystem)

## Repository Structure

```
.
├── cmd/
│   ├── pyroscope/           # Main server binary
│   └── profilecli/          # CLI tool for profile operations
├── pkg/                     # Core Go packages
│   ├── distributor/         # Distributor component
│   ├── ingester/            # Ingester component
│   ├── querier/             # Querier component
│   ├── frontend/            # Query frontend component
│   ├── compactor/           # Compactor component
│   ├── metastore/           # Metadata component
│   ├── phlaredb/            # V1 database storage engine
│   ├── model/               # Data models and types
│   ├── objstore/            # Object storage abstraction
│   ├── api/                 # API definitions and handlers
│   └── og/                  # Legacy code (original Pyroscope)
├── public/app/              # React/TypeScript frontend
├── api/                     # API definitions (protobuf, OpenAPI)
├── docs/                    # Documentation
├── operations/              # Deployment configs (jsonnet, helm)
├── examples/                # Example applications and SDKs
└── tools/                   # Development and build tools
```

## Tech Stack

### Backend
- **Language**: Go 1.24+
- **RPC**: gRPC with Connect protocol
- **Storage**: Parquet, TSDB
- **Hash Ring**: Consistent hashing with memberlist (gossip protocol)
- **Observability**: Prometheus metrics, Structured logs, Distributed traces, pprof profiles

### Frontend
- **Language**: TypeScript
- **Framework**: React
- **Build**: Webpack
- **Styling**: Emotion (CSS-in-JS)
- **State**: React hooks, Context API
- **UI Library**: Grafana UI components

### Testing
- **Go**: Standard `testing` package, testify for assertions
- **Frontend**: Jest, React Testing Library, Cypress (e2e)

## Development Workflow

### Setup & Build

```bash
# Install dependencies (Go 1.24+, Docker, Node v18, Yarn v1.22)
# All other tools auto-download to .tmp/bin/

# Build backend
make go/bin

# Run tests
make go/test

# Build frontend
yarn install
yarn dev          # Dev server on :4041

# Run backend for frontend development
yarn backend:dev  # Runs Pyroscope server

# Docker image
make GOOS=linux GOARCH=amd64 docker-image/pyroscope/build
```

### Code Generation

**IMPORTANT**: After changing protobuf, configs, or flags:
```bash
make generate
```
Commit the generated files with your changes.

### Running Locally

```bash
# Run all components in monolithic mode with embedded Grafana
go run ./cmd/pyroscope --target all,embedded-grafana
# Pyroscope: http://localhost:4040
# Grafana: http://localhost:4041
```

## Code Style & Conventions

### Go Code

1. **Imports**: Three groups separated by blank lines:
   ```go
   import (
       // Standard library
       "context"
       "fmt"

       // Third-party packages
       "github.com/prometheus/client_golang/prometheus"
       "go.uber.org/atomic"

       // Internal packages
       "github.com/grafana/pyroscope/pkg/model"
       "github.com/grafana/pyroscope/pkg/objstore"
   )
   ```

2. **Formatting**: Use `golangci-lint` (run via `make lint`)
   - gofmt for formatting
   - goimports with `-local github.com/grafana/pyroscope`

3. **Linting**:
   - Enabled: depguard, goconst, misspell, revive, unconvert, unparam
   - Use `github.com/go-kit/log` (NOT `github.com/go-kit/kit/log`)

4. **Error Handling**:
   - Always check errors explicitly
   - Wrap errors with context: `fmt.Errorf("failed to query: %w", err)`
   - Use structured logging: `level.Error(logger).Log("msg", "failed to process", "err", err)`

5. **Context**:
   - Always pass `context.Context` as the first parameter
   - Respect context cancellation in loops and long operations

6. **Testing**:
   - File naming: `*_test.go`
   - Test function naming: `TestFunctionName` or `TestComponentName_Method`
   - Use table-driven tests for multiple cases
   - Prefer `t.Run()` for subtests
   - Use `require` for fatal assertions, `assert` for non-fatal

### TypeScript/React Code

1. **File Extensions**: `.tsx` for components, `.ts` for utilities
2. **Components**: Use functional components with hooks
3. **Styling**: Use Emotion CSS-in-JS with Grafana UI theme
4. **Props**: Define explicit TypeScript interfaces for all component props
5. **Formatting**: Use Prettier (run via `yarn lint`)

## Common Patterns

### Multi-tenancy

All requests must include a tenant ID in the `X-Scope-OrgID` header:

```go
import "github.com/grafana/pyroscope/pkg/tenant"

// Extract tenant ID from context
tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
if err != nil {
    return err
}
```

### Consistent Hashing

Components use a hash ring for sharding:

```go
// Get ingester for a given label set
replicationSet, err := ring.Get(key, op, bufDescs, bufHosts, bufZones)
```

### Object Storage

Abstract object storage operations:

```go
import "github.com/grafana/pyroscope/pkg/objstore"

// Use the Bucket interface
bucket := objstore.NewBucket(cfg)
reader, err := bucket.Get(ctx, "path/to/object")
```

### Configuration

Use `github.com/grafana/dskit` for configuration:

```go
type Config struct {
    ListenPort int `yaml:"listen_port"`
    // Use RegisterFlags pattern
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
    f.IntVar(&cfg.ListenPort, "server.http-listen-port", 4040, "HTTP listen port")
}
```

Run `make generate` after changing config definitions, to regenerate docs.

## Testing Best Practices

1. **Unit Tests**: Test individual functions/methods in isolation
2. **Integration Tests**: Use build tags: `//go:build integration`
3. **Mocking**: Use `mockery` for generating mocks from interfaces
4. **Fixtures**: Store test data in `testdata/` directories
5. **Parallel Tests**: Use `t.Parallel()` when tests are independent
6. **Cleanup**: Always use `t.Cleanup()` for resource cleanup

Example test:
```go
func TestDistributor_Push(t *testing.T) {
    t.Parallel()

    tests := []struct {
        name    string
        input   *pushv1.PushRequest
        wantErr bool
    }{
        {name: "valid request", input: validRequest(), wantErr: false},
        {name: "invalid tenant", input: invalidRequest(), wantErr: true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            d := setupDistributor(t)
            err := d.Push(context.Background(), tt.input)
            if tt.wantErr {
                require.Error(t, err)
            } else {
                require.NoError(t, err)
            }
        })
    }
}
```

## Common Pitfalls & Things to Avoid

1. **Don't** introduce dependencies on `pkg/og/` - this is legacy code being phased out
2. **Don't** use `github.com/go-kit/kit/log` - use `github.com/go-kit/log`
3. **Don't** forget to run `make generate` after changing protobuf/config definitions
4. **Don't** hardcode tenant IDs – always extract from context
5. **Don't** create unbounded goroutines – use worker pools or semaphores
6. **Don't** ignore context cancellation in loops
7. **Don't** log PII or sensitive data
8. **Don't** use `fmt.Println` for logging - use structured logging
9. **Don't** add imports within the three import groups (keep them separate)
10. **Don't** commit changes to `node_modules/` or generated code without source changes

## Security Considerations

1. **Input Validation**: Always validate and sanitize user input
2. **Path Traversal**: Validate object keys before storage operations
3. **Rate Limiting**: Distributor implements per-tenant rate limiting
4. **Authentication**: Multi-tenancy via `X-Scope-OrgID` header (authentication delegated to gateway)

## Performance Considerations

1. **Profiling**: This is a profiling system – profile your own changes!
   ```bash
   go test -cpuprofile=cpu.prof -memprofile=mem.prof -bench=.
   go tool pprof cpu.prof
   ```

2. **Allocations**: Minimize allocations in hot paths
   - Reuse buffers with `sync.Pool`
   - Avoid string concatenation in loops
   - Use `strings.Builder` for string building

3. **Concurrency**:
   - Use worker pools for bounded concurrency
   - Prefer channels for coordination over mutexes when possible
   - Always consider the scalability implications

## Documentation

- **User Docs**: `docs/sources/` - Published to grafana.com
- **Contributing**: `docs/internal/contributing/README.md`
- **Component Docs**: In `docs/sources/reference-pyroscope-architecture/components/`

## Useful Make Targets

```bash
make help              # Show all available targets
make lint              # Run linters
make go/test           # Run Go unit tests
make go/bin            # Build binaries
make go/mod            # Tidy go modules
make generate          # Generate code (protobuf, mocks, etc.)
make docker-image/pyroscope/build  # Build Docker image
```

## Key Dependencies

- **dskit**: Grafana's distributed systems toolkit (ring, services, middleware)
- **connect**: RPC framework (gRPC-compatible)
- **parquet-go**: Parquet file format implementation
- **go-kit/log**: Structured logging
- **prometheus/client_golang**: Metrics instrumentation
- **opentelemetry**: Distributed tracing

## When Working on Features

1. **Read Component Docs**: Check `docs/sources/reference-pyroscope-architecture/components/` for the component you're modifying
2. **Understand the Ring**: If working on write/read path, understand consistent hashing
3. **Multi-tenancy First**: Always consider multi-tenant implications
4. **Check for Similar Code**: Pyroscope is inspired by Cortex/Mimir - similar patterns apply
5. **Test Multi-tenancy**: Test with multiple tenants to catch isolation issues
6. **Profile Your Changes**: Use `go test -bench` and verify performance impact
7. **Update Documentation**: If changing user-facing behavior, update docs

## Getting Help

- **Contributing Guide**: `docs/internal/contributing/README.md`
- **Code Comments**: The codebase has extensive comments – read them
- **Git History**: Use `git blame` and `git log` to understand design decisions

## Commit Guidelines

- **Atomic Commits**: Each commit should be a logical unit
- **Commit Messages**: Focus on "why" not just "what"
- **Generated Code**: Include generated files in the same commit as source changes
- **Format**: Follow existing commit message style (see `git log --oneline -20`)

## Additional Notes for AI Agents

- **Favor Simplicity**: Pyroscope values simple, maintainable code over clever abstractions
- **Performance Matters**: This system handles high-throughput profiling data
- **Multi-tenancy is Critical**: Tenant isolation bugs are severe – test thoroughly
- **Consistency with Grafana Labs Style**: Follow patterns from dskit, Mimir, Loki
- **Ask Before Large Refactors**: Propose significant architectural changes before implementing

---

For detailed setup and contributing instructions, see:
- `docs/internal/contributing/README.md` - Development setup and workflow
- `docs/sources/reference-pyroscope-architecture/` - System architecture deep dive
