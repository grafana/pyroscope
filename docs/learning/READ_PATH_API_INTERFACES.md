# Pyroscope Read Path: API Interfaces & Component Communication

This document maps all the interfaces, APIs, and communication patterns between read path components.

## Architecture Overview

```
Client
  ↓ (HTTP/gRPC)
Query Frontend
  ↓ (gRPC)
Query Backend / Querier
  ↓ (gRPC streaming)
┌──────────────┬──────────────┐
Ingesters      Store Gateways
```

---

## 1. Public Client APIs

### QuerierService (Primary API)

**Protobuf:** `api/querier/v1/querier.proto`

**Service Definition:**
```protobuf
service QuerierService {
  // Profile queries
  rpc SelectMergeStacktraces(SelectMergeStacktracesRequest)
    returns (SelectMergeStacktracesResponse);
  rpc SelectMergeProfile(SelectMergeProfileRequest)
    returns (google.v1.Profile);
  rpc SelectMergeSpanProfile(SelectMergeSpanProfileRequest)
    returns (SelectMergeSpanProfileResponse);

  // Time series queries
  rpc SelectSeries(SelectSeriesRequest)
    returns (SelectSeriesResponse);
  rpc SelectHeatmap(SelectHeatmapRequest)
    returns (SelectHeatmapResponse);

  // Metadata queries
  rpc ProfileTypes(ProfileTypesRequest)
    returns (ProfileTypesResponse);
  rpc LabelValues(types.v1.LabelValuesRequest)
    returns (types.v1.LabelValuesResponse);
  rpc LabelNames(types.v1.LabelNamesRequest)
    returns (types.v1.LabelNamesResponse);
  rpc Series(SeriesRequest)
    returns (SeriesResponse);

  // Utilities
  rpc Diff(DiffRequest)
    returns (DiffResponse);
  rpc AnalyzeQuery(AnalyzeQueryRequest)
    returns (AnalyzeQueryResponse);
  rpc GetProfileStats(types.v1.GetProfileStatsRequest)
    returns (types.v1.GetProfileStatsResponse);
}
```

**HTTP Endpoints (Connect):**
- `POST /querier.v1.QuerierService/SelectMergeStacktraces`
- `POST /querier.v1.QuerierService/SelectSeries`
- etc.

**Legacy HTTP Endpoints:**
- `GET /pyroscope/render` → `SelectMergeStacktraces`
- `GET /pyroscope/render-diff` → `Diff`
- `GET /pyroscope/label-values` → `LabelValues`

---

## 2. V2 Query APIs (New Architecture)

### QueryFrontendService

**Protobuf:** `api/query/v1/query.proto`

```protobuf
service QueryFrontendService {
  rpc Query(QueryRequest) returns (QueryResponse);
}

message QueryRequest {
  int64 start_time = 1;
  int64 end_time = 2;
  string label_selector = 3;
  repeated Query query = 4;  // Multiple query types
}

message QueryResponse {
  repeated Report reports = 1;  // Typed results
}
```

### QueryBackendService

```protobuf
service QueryBackendService {
  rpc Invoke(InvokeRequest) returns (InvokeResponse);
}

message InvokeRequest {
  repeated string tenant = 1;
  int64 start_time = 2;
  int64 end_time = 3;
  string label_selector = 4;
  repeated Query query = 5;
  QueryPlan query_plan = 6;  // DAG execution plan
  InvokeOptions options = 7;
}

message InvokeResponse {
  repeated Report reports = 1;
  ExecutionDiagnostics diagnostics = 2;
}
```

### Query and Report Types

**Query Union:**
```protobuf
message Query {
  QueryType query_type = 1;
  oneof query {
    LabelNamesQuery label_names = 2;
    LabelValuesQuery label_values = 3;
    SeriesLabelsQuery series_labels = 4;
    TimeSeriesQuery time_series = 5;
    TreeQuery tree = 6;
    PprofQuery pprof = 7;
    HeatmapQuery heatmap = 8;
  }
}
```

**Report Union:**
```protobuf
message Report {
  ReportType report_type = 1;
  oneof report {
    LabelNamesReport label_names = 2;
    LabelValuesReport label_values = 3;
    SeriesLabelsReport series_labels = 4;
    TimeSeriesReport time_series = 5;
    TreeReport tree = 6;
    PprofReport pprof = 7;
    HeatmapReport heatmap = 8;
  }
}
```

---

## 3. Ingester Service

**Protobuf:** `api/ingester/v1/ingester.proto`

### Streaming Merge Operations

```protobuf
service IngesterService {
  // Bidirectional streaming for profile merging
  rpc MergeProfilesStacktraces(stream MergeProfilesStacktracesRequest)
    returns (stream MergeProfilesStacktracesResponse);

  rpc MergeProfilesLabels(stream MergeProfilesLabelsRequest)
    returns (stream MergeProfilesLabelsResponse);

  rpc MergeProfilesPprof(stream MergeProfilesPprofRequest)
    returns (stream MergeProfilesPprofResponse);

  rpc MergeSpanProfile(stream MergeSpanProfileRequest)
    returns (stream MergeSpanProfileResponse);

  // Metadata queries
  rpc LabelValues(types.v1.LabelValuesRequest)
    returns (types.v1.LabelValuesResponse);
  rpc LabelNames(types.v1.LabelNamesRequest)
    returns (types.v1.LabelNamesResponse);
  rpc Series(SeriesRequest)
    returns (SeriesResponse);

  // Block metadata
  rpc BlockMetadata(BlockMetadataRequest)
    returns (BlockMetadataResponse);
  rpc GetBlockStats(GetBlockStatsRequest)
    returns (GetBlockStatsResponse);
  rpc GetProfileStats(types.v1.GetProfileStatsRequest)
    returns (types.v1.GetProfileStatsResponse);
}
```

### Streaming Protocol

**Request Flow:**
```
Client                          Ingester
  │                                │
  ├─ Send: SelectProfilesRequest ─►│
  │                                │
  │◄─── Stream: ProfileSets ───────┤
  │◄─── Stream: ProfileSets ───────┤
  │                                │
  ├─ Send: KeepRequest ───────────►│  (which profiles to keep)
  │                                │
  │◄─── Stream: MergeResult ───────┤
  │                                │
  └─ Close ────────────────────────┘
```

**Key Messages:**
```protobuf
message MergeProfilesStacktracesRequest {
  oneof request {
    SelectProfilesRequest select = 1;
    KeepProfilesRequest keep = 2;
  }
}

message MergeProfilesStacktracesResponse {
  oneof response {
    ProfileSets selectedProfiles = 1;
    MergeProfilesStacktracesResult result = 2;
  }
}

message ProfileSets {
  repeated string labelsSHA256 = 1;
  repeated types.v1.Labels profiles = 2;
}

message MergeProfilesStacktracesResult {
  bytes tree = 1;  // Flamegraph tree bytes
}
```

---

## 4. Store Gateway Service

**Protobuf:** `api/storegateway/v1/storegateway.proto`

**Service Definition:**
```protobuf
service StoreGatewayService {
  // Reuses ingester message types
  rpc MergeProfilesStacktraces(stream ingester.v1.MergeProfilesStacktracesRequest)
    returns (stream ingester.v1.MergeProfilesStacktracesResponse);

  rpc MergeProfilesLabels(stream ingester.v1.MergeProfilesLabelsRequest)
    returns (stream ingester.v1.MergeProfilesLabelsResponse);

  rpc MergeProfilesPprof(stream ingester.v1.MergeProfilesPprofRequest)
    returns (stream ingester.v1.MergeProfilesPprofResponse);

  rpc MergeSpanProfile(stream ingester.v1.MergeSpanProfileRequest)
    returns (stream ingester.v1.MergeSpanProfileResponse);

  rpc LabelValues(types.v1.LabelValuesRequest)
    returns (types.v1.LabelValuesResponse);
  rpc LabelNames(types.v1.LabelNamesRequest)
    returns (types.v1.LabelNamesResponse);
  rpc Series(ingester.v1.SeriesRequest)
    returns (ingester.v1.SeriesResponse);

  rpc BlockMetadata(ingester.v1.BlockMetadataRequest)
    returns (ingester.v1.BlockMetadataResponse);
  rpc GetBlockStats(ingester.v1.GetBlockStatsRequest)
    returns (ingester.v1.GetBlockStatsResponse);
}
```

**Implementation:** Same streaming protocol as ingesters, but reads from object storage instead of memory.

---

## 5. Go Interfaces (Internal)

### Querier Package

**File:** `pkg/querier/querier.go`

```go
type Querier struct {
    ingesterQuerier     *IngesterQuerier
    storeGatewayQuerier *StoreGatewayQuerier
    cfg                 Config
    limits              Limits
}

// Implements querierv1connect.QuerierServiceHandler
func (q *Querier) SelectMergeStacktraces(
    ctx context.Context,
    req *connect.Request[querierv1.SelectMergeStacktracesRequest],
) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error)
```

### Ingester Querier Interface

**File:** `pkg/querier/ingester_querier.go`

```go
type IngesterQueryClient interface {
    LabelValues(context.Context, *connect.Request[...]) (...)
    LabelNames(context.Context, *connect.Request[...]) (...)
    Series(context.Context, *connect.Request[...]) (...)

    // Streaming methods
    MergeProfilesStacktraces(context.Context) BidiClientMergeProfilesStacktraces
    MergeProfilesLabels(context.Context) BidiClientMergeProfilesLabels
    MergeProfilesPprof(context.Context) BidiClientMergeProfilesPprof
    MergeSpanProfile(context.Context) BidiClientMergeSpanProfile

    BlockMetadata(context.Context, *connect.Request[...]) (...)
    GetProfileStats(context.Context, *connect.Request[...]) (...)
    GetBlockStats(context.Context, *connect.Request[...]) (...)
}

type IngesterQuerier struct {
    ring ring.ReadRing
    pool *ring_client.Pool
}

// Query all ingesters with quorum
func (q *IngesterQuerier) forAllIngesters[T any](
    ctx context.Context,
    f QueryReplicaFn[T, IngesterQueryClient],
) ([]ResponseFromReplica[T], error)

// Query specific ingesters based on block hints
func (q *IngesterQuerier) forAllPlannedIngesters[T any](
    ctx context.Context,
    hints Hints,
    f QueryReplicaWithHintFn[T, IngesterQueryClient],
) ([]ResponseFromReplica[T], error)
```

### Store Gateway Querier Interface

**File:** `pkg/querier/store_gateway_querier.go`

```go
type StoreGatewayQueryClient interface {
    MergeProfilesStacktraces(context.Context) BidiClientMergeProfilesStacktraces
    MergeProfilesLabels(context.Context) BidiClientMergeProfilesLabels
    MergeProfilesPprof(context.Context) BidiClientMergeProfilesPprof
    MergeSpanProfile(context.Context) BidiClientMergeSpanProfile

    ProfileTypes(context.Context, *connect.Request[...]) (...)
    LabelValues(context.Context, *connect.Request[...]) (...)
    LabelNames(context.Context, *connect.Request[...]) (...)
    Series(context.Context, *connect.Request[...]) (...)

    BlockMetadata(context.Context, *connect.Request[...]) (...)
    GetBlockStats(context.Context, *connect.Request[...]) (...)
}

type StoreGatewayQuerier struct {
    ring   ring.ReadRing
    pool   *ring_client.Pool
    limits StoreGatewayLimits
}

// Query store-gateways in replication set
func (q *StoreGatewayQuerier) forAllStoreGateways[T any](
    ctx context.Context,
    tenantID string,
    f QueryReplicaFn[T, StoreGatewayQueryClient],
) ([]ResponseFromReplica[T], error)
```

### Bidirectional Stream Interfaces

**File:** `pkg/clientpool/bidi.go`

```go
type BidiClientMergeProfilesStacktraces interface {
    Send(*ingestv1.MergeProfilesStacktracesRequest) error
    Receive() (*ingestv1.MergeProfilesStacktracesResponse, error)
    CloseRequest() error
    CloseResponse() error
}

// Similar interfaces for:
// - BidiClientMergeProfilesLabels
// - BidiClientMergeProfilesPprof
// - BidiClientMergeSpanProfile
```

### Merge Iterator

**File:** `pkg/querier/select_merge.go`

```go
type MergeIterator interface {
    iter.Iterator[*ProfileWithLabels]
    Keep()  // Mark current profile to keep
}

// Wraps bidirectional stream into iterator
type mergeIterator[R any, Req Request, Res Response] struct {
    ctx     context.Context
    bidi    BidiClientMerge[Req, Res]
    curr    *ingestv1.ProfileSets
    currIdx int
    keep    []bool
}

func (it *mergeIterator) Next() bool {
    // Fetch next batch from stream if needed
    // Return next profile
}

func (it *mergeIterator) Keep() {
    // Mark current profile to keep
    it.keep[it.currIdx] = true
}
```

---

## 6. Query Frontend (V2)

**File:** `pkg/frontend/readpath/queryfrontend/query_frontend.go`

```go
type QueryBackend interface {
    Invoke(ctx context.Context, req *queryv1.InvokeRequest)
        (*queryv1.InvokeResponse, error)
}

type QueryFrontend struct {
    metadataQueryClient metastorev1.MetadataQueryServiceClient
    tenantServiceClient metastorev1.TenantServiceClient
    querybackend        QueryBackend
    symbolizer          Symbolizer
    diagnosticsStore    DiagnosticsStore
}

func (q *QueryFrontend) Query(
    ctx context.Context,
    req *queryv1.QueryRequest,
) (*queryv1.QueryResponse, error) {
    // 1. Query metadata (discover blocks)
    blocks, _ := q.QueryMetadata(ctx, req)

    // 2. Build query plan
    plan := queryplan.Build(blocks)

    // 3. Invoke query backend
    resp, _ := q.querybackend.Invoke(ctx, &queryv1.InvokeRequest{
        QueryPlan: plan,
        Query:     req.Query,
    })

    // 4. Symbolize if needed
    if shouldSymbolize {
        q.processAndSymbolizeProfiles(ctx, resp)
    }

    return &queryv1.QueryResponse{Reports: resp.Reports}, nil
}
```

---

## 7. Client Pool Management

### Ingester Client Pool

**File:** `pkg/clientpool/ingester_client_pool.go`

```go
func NewIngesterPool(
    cfg ClientConfig,
    ring ring.ReadRing,
) *ring_client.Pool {
    factory := func(addr string) (IngesterQueryClient, error) {
        httpClient := util.InstrumentedDefaultHTTPClient()
        return ingesterv1connect.NewIngesterServiceClient(
            httpClient,
            "http://"+addr,
            connect.WithGRPC(),
        ), nil
    }

    return ring_client.NewPool(
        "ingester",
        cfg.PoolConfig,
        ring_client.NewRingServiceDiscovery(ring),
        factory,
        ring_client.PoolAddrFunc(ingesterRingAddrFunc),
    )
}
```

### Store Gateway Client Pool

**File:** `pkg/clientpool/store_gateway_client_pool.go`

```go
func NewStoreGatewayPool(
    cfg ClientConfig,
    ring ring.ReadRing,
) *ring_client.Pool {
    factory := func(addr string) (StoreGatewayQueryClient, error) {
        httpClient := util.InstrumentedDefaultHTTPClient()
        return storegatewayv1connect.NewStoreGatewayServiceClient(
            httpClient,
            "http://"+addr,
            connect.WithGRPC(),
        ), nil
    }

    return ring_client.NewPool(...)
}
```

---

## 8. Replication and Quorum

**File:** `pkg/querier/replication.go`

```go
type ResponseFromReplica[T any] struct {
    addr     string
    response T
}

// Execute query on replication set with quorum
func forGivenReplicationSet[Result, Querier any](
    ctx context.Context,
    clientFactory func(string) (Querier, error),
    replicationSet ring.ReplicationSet,
    f QueryReplicaFn[Result, Querier],
) ([]ResponseFromReplica[Result], error) {
    // Fan-out to all replicas
    // Collect until quorum reached
    // Cancel remaining requests
    // Return results
}
```

**Quorum Calculation:**
```go
quorum := (replicationSet.MaxErrors + 1)
// Need responses from at least 'quorum' replicas
```

---

## 9. API Registration

**File:** `pkg/api/api.go`

```go
func (a *API) RegisterQuerierServiceHandler(
    svc querierv1connect.QuerierServiceHandler,
) {
    path, handler := querierv1connect.NewQuerierServiceHandler(
        svc,
        a.connectInterceptorAuth(),
        a.connectInterceptorLog(),
        a.connectInterceptorDelay(),
        a.connectInterceptorRecovery(),
    )
    a.server.HTTP.Handle(path, handler)
}

func (a *API) RegisterStoreGateway(svc *storegateway.StoreGateway) {
    path, handler := storegatewayv1connect.NewStoreGatewayServiceHandler(svc)
    a.server.HTTP.Handle(path, handler)
}
```

**Connect Interceptors:**
- **Auth:** Tenant extraction and authorization
- **Log:** Structured logging with trace IDs
- **Delay:** Rate limiting with delay injection
- **Recovery:** Panic recovery and error handling

---

## 10. Communication Patterns Summary

### Query Fanout Pattern

```
Querier
  ├─ Query Ingesters (parallel)
  │   ├─ Ingester 1 (via ring)
  │   ├─ Ingester 2
  │   └─ Ingester N
  │   └─ Collect quorum responses
  │
  └─ Query Store Gateways (parallel)
      ├─ Store Gateway 1 (via ring)
      ├─ Store Gateway 2
      └─ Store Gateway N
      └─ Collect quorum responses

  Merge Results → Return to Client
```

### Streaming Merge Pattern

```
Client ──Select──► Ingester
       ◄─Batch1─┤
       ◄─Batch2─┤
       ◄─Batch3─┤
       ──Keep──►    (tell which to keep)
       ◄─Result─┤   (merged result)
       ──Close──►
```

### V2 Query Plan Pattern

```
Query Frontend
  ├─ Query Metadata → Metastore
  ├─ Build Plan → DAG
  └─ Invoke → Query Backend
      ├─ MERGE Node
      │   ├─ Child READ Node 1 (parallel)
      │   ├─ Child READ Node 2
      │   └─ Child MERGE Node
      │       ├─ Child READ Node 3
      │       └─ Child READ Node 4
      └─ Aggregate Results
```

---

## 11. Key Files Reference

| Component | File | Purpose |
|-----------|------|---------|
| **Querier Service** | `api/querier/v1/querier.proto` | Public API definition |
| **Query V2** | `api/query/v1/query.proto` | V2 query/report types |
| **Ingester Service** | `api/ingester/v1/ingester.proto` | Ingester streaming API |
| **Store Gateway** | `api/storegateway/v1/storegateway.proto` | Store gateway API |
| **Querier Impl** | `pkg/querier/querier.go` | Querier coordinator |
| **Ingester Querier** | `pkg/querier/ingester_querier.go` | Ingester client |
| **Store Gateway Querier** | `pkg/querier/store_gateway_querier.go` | Store gateway client |
| **Merge Iterator** | `pkg/querier/select_merge.go` | Streaming merge |
| **Replication** | `pkg/querier/replication.go` | Quorum logic |
| **Client Pools** | `pkg/clientpool/*.go` | Connection pooling |
| **API Registration** | `pkg/api/api.go` | HTTP/gRPC setup |

---

## Next Steps

1. **Read protobuf definitions**: Start with `api/querier/v1/querier.proto`
2. **Trace a query**: Add logging in `querier.go` and watch the flow
3. **Explore streaming**: Look at `select_merge.go` and bidirectional streams
4. **Understand replication**: Read `replication.go` for quorum logic
