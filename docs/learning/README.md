# Pyroscope Read Path Learning Guide

This directory contains comprehensive documentation about Pyroscope's read path, created to help you master the query lifecycle, data storage, APIs, and key data structures.

## Documents Overview

### 1. [Query Lifecycle](READ_PATH_QUERY_LIFECYCLE.md)
**Complete trace from HTTP request to response**

- Entry points (HTTP/gRPC handlers)
- Query Frontend orchestration
- Metadata discovery and query planning
- Query Backend execution (DAG-based)
- Block reading and data access
- Query type handlers (time series, trees, pprof, etc.)
- Result aggregation and merging
- Response transformation and symbolization
- V1 vs V2 architecture comparison
- Complete flow diagrams

**Start here** to understand how queries flow through the system.

### 2. [Data Storage](READ_PATH_DATA_STORAGE.md)
**How profiling data is stored and read**

- Block structure and organization
- Dataset structure (TSDB, Parquet, Symbols)
- Parquet profiles table schema
- TSDB index format and postings
- Symbol database format (V3)
- Object storage directory structure
- Store Gateway read flow
- Memory management and optimization
- Performance optimizations
- Concrete examples

**Read this** to understand how data is organized on disk and in object storage.

### 3. [API Interfaces](READ_PATH_API_INTERFACES.md)
**Component communication and APIs**

- Public client APIs (QuerierService)
- V2 query APIs (QueryFrontend, QueryBackend)
- Ingester Service (streaming merges)
- Store Gateway Service
- Go interfaces (internal)
- Bidirectional streaming protocol
- Client pool management
- Replication and quorum
- Communication patterns
- API registration

**Use this** to understand how components talk to each other.

### 4. [Data Types](READ_PATH_DATA_TYPES.md)
**Key data structures throughout the read path**

- Label types (Labels, LabelsBuilder, LabelMerger)
- Time series types (Series, Point, Builder, Merger)
- Profile types (Profile, StacktraceSample, MergeIterator)
- Tree and flame graph types
- Block and storage types
- Parquet schema
- Query and report types (V2)
- Pprof types
- Heatmap types
- Type transformations

**Reference this** when you need to understand specific data structures.

---

## Suggested Learning Path

### Week 1: High-Level Understanding
1. Read [Query Lifecycle](READ_PATH_QUERY_LIFECYCLE.md) sections 1-3 (Entry to Frontend)
2. Skim [API Interfaces](READ_PATH_API_INTERFACES.md) section 1 (Public APIs)
3. Run Pyroscope locally: `go run ./cmd/pyroscope --target all`
4. Send a test query and watch the logs

**Goal:** Understand the major components and their roles

### Week 2: Deep Dive - Components
1. Read [Query Lifecycle](READ_PATH_QUERY_LIFECYCLE.md) sections 4-7 (Backend to Handlers)
2. Read [Data Storage](READ_PATH_DATA_STORAGE.md) sections 1-5 (Block to Symbols)
3. Read actual code: Start with `pkg/frontend/readpath/queryfrontend/query_frontend.go`
4. Trace through one query type handler (e.g., `query_time_series.go`)

**Goal:** Understand how queries execute and access data

### Week 3: Data and Storage
1. Read [Data Storage](READ_PATH_DATA_STORAGE.md) completely
2. Read [Data Types](READ_PATH_DATA_TYPES.md) sections 1-6
3. Use `profilecli` to inspect a local block
4. Read Parquet files with `parquet-tools` or similar

**Goal:** Understand data formats and storage layout

### Week 4: Hands-On
1. Add logging to trace a query through the system
2. Modify a query handler to add a new feature
3. Run tests: `make go/test`
4. Profile a query: `go test -bench=. -cpuprofile=cpu.prof`

**Goal:** Solidify understanding through practice

---

## Quick Reference

### Key Components

| Component | Location | Purpose |
|-----------|----------|---------|
| **Query Frontend** | `pkg/frontend/readpath/queryfrontend/` | Entry point, query planning |
| **Query Backend** | `pkg/querybackend/` | DAG execution, block reading |
| **Querier** | `pkg/querier/` | V1 querier, ingester/store coordination |
| **Store Gateway** | `pkg/storegateway/` | Serves blocks from object storage |
| **Block** | `pkg/block/` | Block object and dataset management |

### Key Files to Read First

1. `pkg/frontend/readpath/queryfrontend/query_frontend.go` - Main query entry
2. `pkg/querybackend/backend.go` - Query plan execution
3. `pkg/querybackend/query.go` - Query context and handlers
4. `pkg/querier/querier.go` - V1 querier implementation
5. `pkg/storegateway/bucket.go` - Block syncing and serving

### Useful Commands

```bash
# Run Pyroscope locally
go run ./cmd/pyroscope --target all

# Run tests
make go/test

# Profile a benchmark
go test -bench=. -cpuprofile=cpu.prof ./pkg/querybackend/
go tool pprof cpu.prof

# Inspect a block (if you have profilecli)
profilecli block inspect /path/to/block

# Generate code after protobuf changes
make generate
```

---

## Additional Resources

### Pyroscope Documentation
- Component docs: `docs/sources/reference-pyroscope-architecture/components/`
- Contributing guide: `docs/internal/contributing/README.md`
- Main README: `README.md`

### Related Projects
- **Prometheus TSDB**: Similar index structure
- **Parquet**: Column storage format
- **Cortex/Mimir**: Similar distributed architecture
- **Loki**: Similar query patterns

### Protobuf Definitions
- `api/querier/v1/querier.proto` - QuerierService
- `api/query/v1/query.proto` - V2 Query/Report types
- `api/ingester/v1/ingester.proto` - IngesterService
- `api/storegateway/v1/storegateway.proto` - StoreGatewayService
- `api/metastore/v1/types.proto` - BlockMeta, Dataset

---

## How These Docs Were Created

These documents were generated by Claude Code exploration agents that:
1. Traced query flow through the codebase
2. Explored storage formats and data structures
3. Mapped API interfaces and communication
4. Cataloged key data types

The agents read actual code, followed imports, and built a comprehensive understanding of the read path. The results are in these markdown files for your reference.

---

## Questions?

As you work through this material, keep notes on:
- Concepts that are unclear
- Code patterns you see repeatedly
- Areas you want to dive deeper into
- Questions for the team

Good luck mastering the Pyroscope read path! 🔥
