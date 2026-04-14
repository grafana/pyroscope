# Pyroscope Read Path: Key Data Types and Structures

This document catalogs the main data structures used throughout Pyroscope's read path.

## Quick Reference

| Category | Key Types | Location |
|----------|-----------|----------|
| **Labels** | `Labels`, `LabelPair` | `pkg/model/labels.go` |
| **Time Series** | `Series`, `Point` | `pkg/model/timeseries/` |
| **Profiles** | `Profile`, `StacktraceSample` | `pkg/model/profiles.go` |
| **Trees** | `Tree`, `FlameGraph` | `pkg/model/tree.go` |
| **Blocks** | `BlockMeta`, `Dataset` | `pkg/block/` |
| **Queries** | `Query`, `Report` | `api/query/v1/` |

---

## 1. Label Types

### Labels (Model)

**File:** `pkg/model/labels.go`

```go
// Type alias for sorted label pairs
type Labels = []*typesv1.LabelPair

// Methods via extensions
func (l Labels) Hash() uint64
func (l Labels) Get(name string) (string, bool)
func (l Labels) WithLabels(labels ...string) Labels
```

**Protobuf:** `api/types/v1/types.proto`
```protobuf
message LabelPair {
  string name = 1;
  string value = 2;
}

message Labels {
  repeated LabelPair labels = 1;
}
```

### LabelsBuilder

**File:** `pkg/model/labels.go`

```go
type LabelsBuilder struct {
    base Labels
    del  []string
    add  []LabelPair
}

func (b *LabelsBuilder) Set(name, value string)
func (b *LabelsBuilder) Del(name string)
func (b *LabelsBuilder) Labels() Labels
```

**Purpose:** Efficiently modify label sets without repeated allocations

### LabelMerger

**File:** `pkg/model/labels_merger.go`

```go
type LabelMerger struct {
    mu     sync.Mutex
    names  map[string]struct{}
    values map[string]map[string]struct{}
    series map[uint64]*typesv1.Labels
}

func (lm *LabelMerger) MergeLabelNames(names []string)
func (lm *LabelMerger) MergeLabelValues(name string, values []string)
func (lm *LabelMerger) MergeSeries(series []*typesv1.Labels)
```

**Purpose:** Thread-safe merging of labels across query results

---

## 2. Time Series Types

### Series and Point

**Protobuf:** `api/types/v1/types.proto`

```protobuf
message Series {
  repeated LabelPair labels = 1;
  repeated Point points = 2;
}

message Point {
  double value = 1;
  int64 timestamp = 2;
  repeated ProfileAnnotation annotations = 3;
  repeated Exemplar exemplars = 4;
}

message Exemplar {
  int64 timestamp = 1;
  string profile_id = 2;
  string span_id = 3;
  int64 value = 4;
  repeated LabelPair labels = 5;
}
```

### Builder

**File:** `pkg/model/timeseries/timeseries.go`

```go
type Builder struct {
    series          map[string]*typesv1.Series
    labelBuf        Labels
    by              []string
    exemplarBuilders map[string]*ExemplarBuilder
}

func (b *Builder) Add(
    ts int64,
    labels Labels,
    value float64,
)

func (b *Builder) Build() []*typesv1.Series
func (b *Builder) BuildWithExemplars(
    exemplarType typesv1.ExemplarType,
) []*typesv1.Series
```

**Purpose:** Accumulate time series points during query execution

### Merger

**File:** `pkg/model/timeseries/merger.go`

```go
type Merger struct {
    series map[uint64]*typesv1.Series
    sum    bool
}

func (m *Merger) MergeTimeSeries(series []*typesv1.Series)
func (m *Merger) TimeSeries() []*typesv1.Series
func (m *Merger) Top(k int) []*typesv1.Series
```

**Purpose:** K-way merge of time series with optional summation

---

## 3. Profile Types

### Profile Interface

**File:** `pkg/model/profiles.go`

```go
type Profile interface {
    Labels() phlaremodel.Labels
    Timestamp() model.Time
}

type Timestamp interface {
    Timestamp() model.Time
}
```

### StacktraceSample

**Protobuf:** `api/ingester/v1/ingester.proto`

```protobuf
message StacktraceSample {
  repeated int32 function_ids = 1;
  int64 value = 2;
}

message Profile {
  string ID = 1;
  types.v1.ProfileType type = 2;
  repeated types.v1.LabelPair labels = 3;
  int64 timestamp = 4;
  repeated StacktraceSample stacktraces = 5;
}
```

### MergeIterator

**File:** `pkg/model/profiles.go`

```go
type MergeIterator[P Profile] struct {
    tree        *loser.Tree[P, Iterator[P]]
    current     P
    deduplicate bool
}

func NewMergeIterator[P Profile](
    maxTime model.Time,
    deduplicate bool,
    iterators []Iterator[P],
) *MergeIterator[P]

func (it *MergeIterator) Next() bool
func (it *MergeIterator) At() P
func (it *MergeIterator) Close() error
```

**Purpose:** Efficiently merge K sorted profile iterators

---

## 4. Tree and Flame Graph Types

### StacktraceMerger

**File:** `pkg/model/stacktraces.go`

```go
type StacktraceMerger struct {
    s  *StacktraceTree
    r  *functionsRewriter
    mu sync.Mutex
}

func (sm *StacktraceMerger) MergeStackTraces(
    stacktraces []*ingestv1.StacktraceSample,
    functionNames []string,
)

func (sm *StacktraceMerger) TreeBytes(maxNodes int64) []byte
func (sm *StacktraceMerger) Size() uint64
```

### StacktraceTree

**File:** `pkg/model/stacktraces.go`

```go
type StacktraceTree struct {
    Nodes []StacktraceNode
}

type StacktraceNode struct {
    FirstChild int32
    NextSibling int32
    Parent int32
    Location int32  // Function name index
    Value int64     // Self value
    Total int64     // Total value (self + children)
}

func (t *StacktraceTree) Insert(
    locations []int32,
    value int64,
)

func (t *StacktraceTree) Bytes() []byte
func (t *StacktraceTree) MinValue() int64
```

**Purpose:** Compact in-memory representation of flame graph

### Tree

**File:** `pkg/model/tree.go`

```go
type Tree struct {
    root []*node
}

type node struct {
    parent   *node
    children []*node
    self     int64
    total    int64
    name     string
}

func (t *Tree) InsertStack(value int64, stack ...string)
func (t *Tree) Merge(other *Tree)
func (t *Tree) String() string
func (t *Tree) WriteCollapsed(w io.Writer) error
```

### FlameGraph

**Protobuf:** `api/querier/v1/querier.proto`

```protobuf
message FlameGraph {
  repeated string names = 1;
  repeated Level levels = 2;
  int64 total = 3;
  int64 max_self = 4;
}

message Level {
  repeated int64 values = 1;  // [xOffset, self, total, nameIndex, ...]
}
```

**Conversion:**

**File:** `pkg/model/flamegraph.go`

```go
func NewFlameGraph(tree *Tree, maxNodes int64) *querierv1.FlameGraph {
    // Convert tree to level-based encoding
    // Each level entry: [xOffset, self, total, name_index]
}
```

### FlameGraphDiff

**File:** `pkg/model/flamegraph_diff.go`

```go
func NewFlamegraphDiff(
    left *Tree,
    right *Tree,
    maxNodes int64,
) *querierv1.FlameGraphDiff
```

---

## 5. Block and Storage Types

### BlockMeta

**Protobuf:** `api/metastore/v1/types.proto`

```protobuf
message BlockMeta {
  uint32 format_version = 1;
  string id = 2;                     // ULID
  int32 tenant = 3;
  uint32 shard = 4;
  uint32 compaction_level = 5;       // 0=segment, 1+=compacted
  int64 min_time = 6;                // Milliseconds
  int64 max_time = 7;
  repeated Dataset datasets = 10;
  repeated string string_table = 11;
}
```

### Dataset

**Protobuf:** `api/metastore/v1/types.proto`

```protobuf
message Dataset {
  uint32 format = 9;                 // 0=traditional, 1=tenant-wide
  int32 tenant = 1;
  int32 name = 2;
  int64 min_time = 3;
  int64 max_time = 4;
  repeated uint64 table_of_contents = 5;
  uint64 size = 6;
  repeated int32 labels = 8;
}
```

**Go Type:** `pkg/block/dataset.go`

```go
type Dataset struct {
    tenant string
    name   string
    meta   *metastorev1.Dataset
    obj    *Object

    tsdb     *tsdbBuffer
    symbols  *symdb.Reader
    profiles *ParquetFile
}

func (s *Dataset) Open(ctx context.Context, sections ...Section) error
func (s *Dataset) Close() error
```

### Block Object

**File:** `pkg/block/object.go`

```go
type Object struct {
    path    string
    meta    *metastorev1.BlockMeta
    storage objstore.BucketReader
    local   *objstore.ReadOnlyFile
    refs    refctr.Counter
    buf     *bufferpool.Buffer
    memSize int
}

func (o *Object) Open(ctx context.Context) error
func (o *Object) LoadIntoMemory(ctx context.Context) error
func (o *Object) Datasets() []*Dataset
```

---

## 6. Parquet Profile Schema

**File:** `pkg/phlaredb/schemas/v1/profiles.go`

```go
var ProfilesSchema = parquet.NewSchema("Profile", Group{
    NewGroupField("ID", parquet.UUID()),
    NewGroupField("SeriesIndex", parquet.Uint(32)),
    NewGroupField("StacktracePartition", parquet.Uint(64)),
    NewGroupField("TotalValue", parquet.Uint(64)),
    NewGroupField("Samples", parquet.List(sampleField)),
    NewGroupField("TimeNanos", parquet.Timestamp(parquet.Nanosecond)),
    NewGroupField("DurationNanos", parquet.Optional(parquet.Int(64))),
    NewGroupField("Period", parquet.Optional(parquet.Int(64))),
    NewGroupField("DropFrames", parquet.Optional(stringRef)),
    NewGroupField("KeepFrames", parquet.Optional(stringRef)),
    NewGroupField("Comments", parquet.List(stringRef)),
    NewGroupField("Annotations", parquet.List(annotationField)),
})

var sampleField = Group{
    NewGroupField("StacktraceID", parquet.Uint(64)),
    NewGroupField("Value", parquet.Int(64)),
    NewGroupField("Labels", pprofLabels),
    NewGroupField("SpanID", parquet.Optional(parquet.Uint(64))),
}
```

**Key Columns:**
- `ID`: Profile UUID
- `SeriesIndex`: Links to TSDB series
- `Samples[].StacktraceID`: Stack trace references
- `Samples[].Value`: Sample values
- `TimeNanos`: Profile timestamp

---

## 7. Query and Report Types (V2)

### Query Union

**Protobuf:** `api/query/v1/query.proto`

```protobuf
message Query {
  QueryType query_type = 1;
  LabelNamesQuery label_names = 2;
  LabelValuesQuery label_values = 3;
  SeriesLabelsQuery series_labels = 4;
  TimeSeriesQuery time_series = 5;
  TreeQuery tree = 6;
  PprofQuery pprof = 7;
  HeatmapQuery heatmap = 8;
}

enum QueryType {
  QUERY_LABEL_NAMES = 1;
  QUERY_LABEL_VALUES = 2;
  QUERY_TIME_SERIES = 4;
  QUERY_TREE = 5;
  QUERY_PPROF = 6;
  QUERY_HEATMAP = 7;
}
```

### Report Union

```protobuf
message Report {
  ReportType report_type = 1;
  LabelNamesReport label_names = 2;
  LabelValuesReport label_values = 3;
  TimeSeriesReport time_series = 5;
  TreeReport tree = 6;
  PprofReport pprof = 7;
  HeatmapReport heatmap = 8;
}
```

### Query Plan

```protobuf
message QueryPlan {
  QueryNode root = 1;
}

message QueryNode {
  Type type = 1;                     // MERGE or READ
  repeated QueryNode children = 2;   // For MERGE nodes
  repeated metastore.v1.BlockMeta blocks = 3;  // For READ nodes

  enum Type {
    MERGE = 1;
    READ = 2;
  }
}
```

---

## 8. Pprof Types

### Profile (Google Format)

**Protobuf:** `api/google/v1/profile.proto`

```protobuf
message Profile {
  repeated ValueType sample_type = 1;
  repeated Sample sample = 2;
  repeated Mapping mapping = 3;
  repeated Location location = 4;
  repeated Function function = 5;
  repeated string string_table = 6;
  int64 time_nanos = 9;
  int64 duration_nanos = 10;
  ValueType period_type = 11;
  int64 period = 12;
}

message Sample {
  repeated int64 location_id = 1;
  repeated int64 value = 2;
  repeated Label label = 3;
}

message Location {
  int64 id = 1;
  int64 mapping_id = 2;
  int64 address = 3;
  repeated Line line = 4;
}

message Function {
  int64 id = 1;
  int64 name = 2;           // String table index
  int64 filename = 3;
  int64 start_line = 4;
}
```

### ProfileMerge

**File:** `pkg/pprof/merge.go`

```go
type ProfileMerge struct {
    profile      *profilev1.Profile
    stringTable  *RewriteTable
    functionTable *RewriteTable
    mappingTable  *RewriteTable
    locationTable *RewriteTable
    sampleTable   *RewriteTable
    mu           sync.Mutex
}

func (pm *ProfileMerge) Merge(ctx context.Context, p *profilev1.Profile)
func (pm *ProfileMerge) Profile() *profilev1.Profile
```

**Purpose:** Thread-safe merging of pprof profiles with deduplication

---

## 9. Heatmap Types

### HeatmapSeries

**Protobuf:** `api/query/v1/query.proto`

```protobuf
message HeatmapPoint {
  int64 timestamp = 1;
  int64 profile_id = 2;
  uint64 span_id = 3;
  int64 value = 4;
  repeated int64 attribute_refs = 5;  // Index into AttributeTable
}

message HeatmapSeries {
  repeated int64 attribute_refs = 1;
  repeated HeatmapPoint points = 2;
}

message AttributeTable {
  repeated string keys = 1;
  repeated string values = 2;
}
```

### Merger

**File:** `pkg/model/heatmap/merger.go`

```go
type Merger struct {
    atMerger *attributetable.Merger
    sum      bool
    series   map[string]*atHeatmapSeries
    mu       sync.Mutex
}

func (m *Merger) MergeHeatmap(
    heatmap []*queryv1.HeatmapSeries,
    attributeTable *queryv1.AttributeTable,
)

func (m *Merger) Heatmap() (
    []*queryv1.HeatmapSeries,
    *queryv1.AttributeTable,
)
```

---

## 10. Data Flow Through Types

### Query Execution Flow

```
1. Request Types
   SelectMergeStacktracesRequest (client)
   ↓
   QueryRequest (V2 frontend)
   ↓
   InvokeRequest (backend)

2. Internal Processing
   BlockMeta[] (metadata query)
   ↓
   Dataset (open sections)
   ↓
   Profile rows (Parquet read)
   ↓
   StacktraceSample[] (extracted)

3. Aggregation
   StacktraceMerger
   ↓
   StacktraceTree
   ↓
   Tree (higher-level)

4. Response Types
   FlameGraph or TreeBytes (wire format)
   ↓
   Report (V2)
   ↓
   SelectMergeStacktracesResponse (client)
```

### Time Series Flow

```
1. Query
   SelectSeriesRequest
   ↓
   TimeSeriesQuery (V2)

2. Processing
   TSDB Index → SeriesIndex values
   ↓
   Parquet → TimeNanos, TotalValue
   ↓
   Builder.Add(ts, labels, value)

3. Response
   Builder.Build() → Series[]
   ↓
   TimeSeriesReport
   ↓
   SelectSeriesResponse
```

---

## 11. Type Transformations

### Labels → Fingerprint

```go
// pkg/model/labels.go
func (l Labels) Hash() uint64 {
    h := hashNew()
    for _, label := range l {
        h = hashAdd(h, label.Name)
        h = hashAdd(h, label.Value)
    }
    return h
}
```

### Tree → FlameGraph

```go
// pkg/model/flamegraph.go
func NewFlameGraph(tree *Tree, maxNodes int64) *FlameGraph {
    // Walk tree level by level
    // Encode as [xOffset, self, total, nameIndex] per node
    // Group by level
    return &FlameGraph{Names: names, Levels: levels}
}
```

### StacktraceSamples → Tree

```go
// pkg/model/stacktraces.go
func (sm *StacktraceMerger) MergeStackTraces(samples, functions) {
    for _, sample := range samples {
        locations := rewriteFunctions(sample.FunctionIds)
        sm.s.Insert(locations, sample.Value)
    }
}
```

---

## 12. Key Files Reference

| Type Category | File | Key Types |
|---------------|------|-----------|
| **Labels** | `pkg/model/labels.go` | `Labels`, `LabelsBuilder`, `LabelMerger` |
| **Time Series** | `pkg/model/timeseries/` | `Series`, `Point`, `Builder`, `Merger` |
| **Profiles** | `pkg/model/profiles.go` | `Profile`, `MergeIterator` |
| **Trees** | `pkg/model/stacktraces.go` | `StacktraceTree`, `StacktraceMerger` |
| **Trees** | `pkg/model/tree.go` | `Tree`, `TreeMerger` |
| **Flame Graphs** | `pkg/model/flamegraph.go` | `FlameGraph`, `FlameGraphDiff` |
| **Blocks** | `pkg/block/` | `Object`, `Dataset`, `BlockMeta` |
| **Parquet** | `pkg/phlaredb/schemas/v1/` | Schema definitions |
| **Pprof** | `pkg/pprof/` | `Profile`, `ProfileMerge` |
| **Heatmaps** | `pkg/model/heatmap/` | `HeatmapSeries`, `Merger` |
| **Queries (V2)** | `api/query/v1/` | `Query`, `Report`, `QueryPlan` |

---

## Next Steps

1. **Explore type definitions**: Start with `pkg/model/` for core types
2. **Read protobuf files**: See `api/` for wire format definitions
3. **Trace type transformations**: Follow a query through the type changes
4. **Understand merging**: Look at merger implementations for each type
