# Pyroscope Read Path: Complete Query Lifecycle

This document provides a comprehensive trace of how queries flow through Pyroscope's read path, from HTTP request to response.

## Quick Overview

**Entry Point** → **Query Frontend** → **Query Backend** → **Block Reader** → **Query Handlers** → **Result Aggregation** → **Response**

---

## 1. Entry Points

### HTTP/gRPC Handlers
**File:** `/pkg/api/api.go`

Queries arrive via Connect RPC protocol:
- `POST /querier.v1.QuerierService/SelectMergeStacktraces` - Flame graphs
- `POST /querier.v1.QuerierService/SelectSeries` - Time series
- `POST /querier.v1.QuerierService/LabelValues` - Label discovery

Legacy HTTP endpoints:
- `GET /pyroscope/render` - Flame graph rendering
- `GET /pyroscope/label-values` - Label values

---

## 2. Query Frontend (Entry & Orchestration)

**File:** `/pkg/frontend/readpath/queryfrontend/query_frontend.go`

### Main Flow in `Query()` method:

```
1. Extract tenant from context
2. Query metadata (discover relevant blocks)
3. Build query plan (DAG of READ/MERGE nodes)
4. Invoke query backend
5. Optional: Symbolize profiles
6. Return response
```

### 2.1 Metadata Discovery

**Method:** `QueryMetadata()` (line 181)

Queries the Metastore to find blocks matching:
- Time range (start/end)
- Label selector (e.g., `{service="api"}`)
- Profile type

**Returns:** List of `BlockMeta` objects

### 2.2 Query Plan Building

**File:** `/pkg/querybackend/queryplan/query_plan.go`

**Algorithm:**
1. Create **READ nodes** - batch up to 4 blocks per node
2. Create **MERGE nodes** - hierarchical tree structure
   - Each MERGE can have up to 20 children
   - Reduces final merge complexity

**Example with 20 blocks:**
```
        MERGE (root)
       /      \
    MERGE    MERGE
   /  |  \   /  |  \
  R   R   R  R   R   R   (R = READ node with ~4 blocks each)
```

---

## 3. Query Backend (Execution Engine)

**File:** `/pkg/querybackend/backend.go`

### 3.1 Invoke() - Entry Point

Executes the query plan recursively:

**MERGE Node Processing:**
```go
func (q *QueryBackend) merge(children []*QueryNode) {
    // For each child:
    //   - Clone request
    //   - Invoke recursively (parallel)
    //   - Collect responses
    // Aggregate all results
    // Return merged response
}
```

**READ Node Processing:**
```go
func (q *QueryBackend) read(blocks []*BlockMeta) {
    return q.blockReader.Invoke(request)
}
```

### 3.2 Concurrency Model

Multiple levels of parallelism:
1. **Query Plan Level:** All child nodes execute in parallel
2. **Block Level:** All blocks in READ node execute concurrently
3. **Dataset Level:** Datasets within blocks execute concurrently
4. **Query Level:** Multiple queries per dataset execute concurrently

Uses `golang.org/x/sync/errgroup` for synchronization

---

## 4. Block Reader (Data Access Layer)

**File:** `/pkg/querybackend/block_reader.go`

### Main Flow in `Invoke()`:

```
For each block in query plan:
  1. Create Block object from object storage
  2. (Optional) Open dataset index for filtering
  3. For each dataset:
     - Open dataset sections (TSDB, Profiles, Symbols)
     - Execute queries
     - Aggregate results
  4. Return aggregated reports
```

### Dataset Structure

```
Block
└── Dataset (e.g., "my-service", profile_type="cpu")
    ├── TSDB Index - Series metadata and postings
    ├── Profiles - Parquet table with profile data
    └── Symbols - Symbol database for stack resolution
```

---

## 5. Query Execution (Per Dataset)

**File:** `/pkg/querybackend/query.go`

### Query Context Execution

```go
func (q *queryContext) execute(query *Query) {
    // 1. Get handler for query type
    handle := getQueryHandler(query.QueryType)

    // 2. Open dataset sections
    q.ds.Open(ctx, TSDB, Profiles, Symbols, ...)

    // 3. Execute handler
    report := handle(q, query)

    // 4. Aggregate report
    q.agg.aggregateReport(report)
}
```

### Query Type Handlers

| Query Type | Handler File | Output |
|------------|-------------|--------|
| TIME_SERIES | `query_time_series.go` | Time series with points |
| TREE | `query_tree.go` | Flame graph tree |
| PPROF | `query_pprof.go` | pprof format profile |
| LABEL_NAMES | `query_label_names.go` | List of label names |
| LABEL_VALUES | `query_label_values.go` | List of label values |
| HEATMAP | `query_heatmap.go` | Heatmap data points |

---

## 6. Query Handler Example: Time Series

**File:** `/pkg/querybackend/query_time_series.go`

### Flow:

```
1. Open TSDB index
2. Lookup series matching label selector
3. Create profile iterator
4. For each profile:
   - Read from Parquet (SeriesIndex, TimeNanos, TotalValue)
   - Group by labels
   - Aggregate values into time buckets
5. Build time series report
6. Return Report{TimeSeries: result}
```

### Data Access Pattern:

```
TSDB Index → Series IDs
    ↓
Parquet Table → Filter by SeriesIndex
    ↓
Extract: TimeNanos, TotalValue, Labels
    ↓
Group & Aggregate → Time Series Points
```

---

## 7. Result Aggregation

**File:** `/pkg/querybackend/report_aggregator.go`

### Aggregation Strategy

```go
type reportAggregator struct {
    staged      map[ReportType]*Report   // First report of each type
    aggregators map[ReportType]aggregator // Active aggregators
}
```

**Process:**
1. **First report of type:** Cache in `staged`
2. **Second report of type:**
   - Trigger aggregation
   - Aggregate first + second report
3. **Subsequent reports:** Aggregate with existing aggregator
4. **Build response:** Collect staged + aggregated reports

### Aggregator Types

Each report type has a specialized aggregator:
- **TimeSeriesAggregator:** Merges time series points
- **TreeAggregator:** Merges flame graph trees
- **PprofAggregator:** Merges pprof profiles
- **LabelNamesAggregator:** Deduplicates label names
- etc.

---

## 8. Response Transformation

**File:** `/pkg/frontend/readpath/queryfrontend/query_frontend.go`

### Post-Processing (lines 161-178)

1. **Symbolization** (if enabled):
   ```go
   if shouldSymbolize {
       processAndSymbolizeProfiles(resp)
   }
   ```
   - Resolves function IDs to function names
   - Uses symbol service or local symbol cache

2. **Diagnostics** (if requested):
   - Attach execution plan
   - Add timing information
   - Include block statistics

3. **Response Wrapping:**
   ```go
   return &QueryResponse{Reports: resp.Reports}
   ```

---

## 9. V1 vs V2 Architecture

### V1 (Legacy)
- Direct queries to ingesters + store gateways
- Query Frontend → Query Scheduler → Querier
- Limited parallelism control
- Separate code paths for different backends

### V2 (Current)
- Unified query plan execution
- Query Frontend → Query Backend → Parallel Execution
- DAG-based query plans
- Hierarchical merge reduces complexity
- Segment-based storage with metastore

### Compatibility Layer

**File:** `/pkg/frontend/readpath/router.go`

Routes queries to V1 or V2 based on:
- Time range (V2 enabled from specific date)
- Feature flags
- Query type support

---

## 10. Complete Flow Diagram

```
┌─────────────────┐
│  HTTP Request   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   API Router    │  pkg/api/api.go
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Query Frontend  │  pkg/frontend/readpath/queryfrontend/
│                 │
│ 1. QueryMetadata│───► Metastore (get blocks)
│ 2. Build Plan   │
│ 3. Invoke       │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Query Backend   │  pkg/querybackend/backend.go
│                 │
│ Execute DAG:    │
│  ┌─────────┐   │
│  │  MERGE  │   │
│  └─┬───┬─┬─┘   │
│    │   │ │     │
│  ┌─▼┐ │ ├─┐   │
│  │ R│ │R│R│   │  (R = READ node)
│  └──┘ └─┘└─┘   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Block Reader   │  pkg/querybackend/block_reader.go
│                 │
│ For each block: │
│  1. Open        │
│  2. Query       │
│  3. Aggregate   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Query Handlers  │  pkg/querybackend/query_*.go
│                 │
│ - Time Series   │
│ - Tree          │
│ - Pprof         │
│ - Labels        │
│ - Heatmap       │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   Aggregator    │  pkg/querybackend/report_aggregator.go
│                 │
│ Merge results   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   Response      │
│  (Reports[])    │
└─────────────────┘
```

---

## 11. Key Files Reference

| Component | File | Key Functions |
|-----------|------|---------------|
| **Frontend** | `pkg/frontend/readpath/queryfrontend/query_frontend.go` | `Query()`, `QueryMetadata()` |
| **Query Plan** | `pkg/querybackend/queryplan/query_plan.go` | `Build()` |
| **Backend** | `pkg/querybackend/backend.go` | `Invoke()`, `merge()`, `read()` |
| **Block Reader** | `pkg/querybackend/block_reader.go` | `Invoke()` |
| **Query Execution** | `pkg/querybackend/query.go` | `execute()` |
| **Handlers** | `pkg/querybackend/query_*.go` | Type-specific handlers |
| **Aggregation** | `pkg/querybackend/report_aggregator.go` | `aggregate()`, `response()` |
| **API** | `pkg/api/api.go` | Handler registration |

---

## 12. Debugging Tips

### Enable Query Logging

Add log statements in:
- `QueryFrontend.Query()` - See incoming requests
- `QueryBackend.Invoke()` - See query plan execution
- `blockContext.execute()` - See block-level operations

### Inspect Query Plans

```go
// In query_frontend.go after Build():
level.Info(logger).Log(
    "msg", "query plan built",
    "blocks", len(blocks),
    "plan_nodes", countNodes(p.Root),
)
```

### Profile Query Performance

```go
// Use pprof to profile query handlers
go test -bench=. -cpuprofile=cpu.prof
go tool pprof cpu.prof
```

---

## Next Steps

1. **Read component docs**: `docs/sources/reference-pyroscope-architecture/components/`
2. **Trace a query locally**: Add logging and run with `-target all`
3. **Explore query handlers**: Start with `query_time_series.go`
4. **Understand storage**: See `READ_PATH_DATA_STORAGE.md`
