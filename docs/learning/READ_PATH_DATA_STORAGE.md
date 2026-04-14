# Pyroscope Read Path: Data Storage Formats

This document explains how profiling data is stored and read in Pyroscope.

## Quick Overview

```
Object Storage
└── blocks/{shard}/{tenant}/{block_id}/
    └── block.bin
        ├── Dataset 1 (e.g., service="api", profile_type="cpu")
        │   ├── Profiles (Parquet table)
        │   ├── TSDB Index (inverted index)
        │   └── Symbols (symbol database)
        ├── Dataset 2 (e.g., service="web", profile_type="memory")
        └── Block Metadata (protobuf)
```

---

## 1. Block Structure

### File Locations

**Compacted Blocks (Level 1+):**
```
blocks/{shard}/{tenant}/{block_id}/block.bin
```

**Segments (Level 0):**
```
segments/{shard}/anonymous/{segment_id}/block.bin
```

### Block Metadata

**File:** `pkg/block/metadata/metadata.go`

**Format:**
```
[Dataset 1 data]
[Dataset 2 data]
[...]
[Block Metadata (protobuf)]
[be_uint32: metadata size]
[be_uint32: CRC32 checksum]
```

**Metadata Structure:**
```protobuf
message BlockMeta {
  uint32 format_version = 1;      // Currently 3
  string id = 2;                  // ULID
  int32 tenant = 3;               // Index into string_table
  uint32 shard = 4;               // Consistent hash shard (0-N)
  uint32 compaction_level = 5;    // 0=segment, 1+=compacted
  int64 min_time = 6;             // Milliseconds
  int64 max_time = 7;             // Milliseconds
  repeated Dataset datasets = 10;
  repeated string string_table = 11;  // String deduplication
}
```

---

## 2. Dataset Structure

### What is a Dataset?

A dataset represents profiles for a specific combination of:
- Service name (e.g., "my-api")
- Profile type (e.g., "cpu", "memory")
- Additional labels

Each dataset contains three main sections:

### Dataset Metadata

```protobuf
message Dataset {
  uint32 format = 9;              // 0=traditional, 1=tenant-wide index
  int32 tenant = 1;
  int32 name = 2;                 // Service name
  int64 min_time = 3;
  int64 max_time = 4;
  repeated uint64 table_of_contents = 5;  // Section offsets
  uint64 size = 6;
  repeated int32 labels = 8;      // Label key-value pairs
}
```

### Table of Contents

Maps section types to byte offsets:

```
Format 0 (Traditional):
  [0] → Profiles.parquet
  [1] → TSDB Index
  [2] → Symbols.symdb

Format 1 (Tenant-wide):
  [0] → Dataset Index TSDB
```

---

## 3. Parquet Profiles Table

**Schema File:** `pkg/phlaredb/schemas/v1/profiles.go`

### Column Structure

```
Profile
├── ID (UUID)                           - Unique profile identifier
├── SeriesIndex (uint32)                - TSDB series reference
├── StacktracePartition (uint64)        - Partition hint
├── TotalValue (uint64)                 - Sum of all sample values
├── Samples (list)                      - Stack trace samples
│   └── element
│       ├── StacktraceID (uint64)       - Stack trace reference
│       ├── Value (int64)               - Sample value
│       ├── Labels (list)               - pprof labels (optional)
│       └── SpanID (uint64, optional)   - Tracing span ID
├── TimeNanos (timestamp)               - Profile timestamp
├── DurationNanos (int64, optional)     - Profile duration
├── Period (int64, optional)            - Sampling period
├── DropFrames (string ref, optional)   - Regex for frames to drop
├── KeepFrames (string ref, optional)   - Regex for frames to keep
└── Annotations (list, optional)        - Key-value metadata
    └── element
        ├── Key (string ref)
        └── Value (string ref)
```

### Encoding Schemes

- **Delta Binary Packed:** For integer columns (efficient compression)
- **RLE Dictionary:** For categorical data (SpanID)
- **String References:** Delta-encoded int64 pointers to string table

### Example Profile Row

```
ID: 550e8400-e29b-41d4-a716-446655440000
SeriesIndex: 12345
TotalValue: 1000000 (nanoseconds)
TimeNanos: 2024-01-01T00:00:45Z
Samples:
  [0]: {StacktraceID: 100, Value: 500000, SpanID: 555}
  [1]: {StacktraceID: 101, Value: 300000, SpanID: 555}
  [2]: {StacktraceID: 102, Value: 200000, SpanID: null}
Annotations:
  [0]: {Key: "version", Value: "1.2.3"}
```

### Reading Profiles

```go
// Open parquet file
file := block.Dataset.Profiles()

// Create iterator
iter := file.NewIterator(ctx)

// Read rows
for iter.Next() {
    profile := iter.At()
    fmt.Printf("Profile ID: %s, Time: %d\n",
        profile.ID, profile.TimeNanos)
}
```

---

## 4. TSDB Index

**Location:** `pkg/phlaredb/tsdb/index/`

### Purpose

Fast lookups of series by label matchers using an inverted index.

### Index File Structure

```
┌──────────────┐
│ Magic Number │  0xBAAAD700
├──────────────┤
│ Format Ver   │  2
├──────────────┤
│ Symbols      │  String table
├──────────────┤
│ Series       │  Series metadata + labels
├──────────────┤
│ Label Index  │  Name → Values mapping
├──────────────┤
│ Postings     │  (label,value) → [SeriesIDs]
├──────────────┤
│ TOC          │  Table of contents
└──────────────┘
```

### Table of Contents (TOC)

```go
type TOC struct {
    Symbols            uint64  // Offset to symbols section
    Series             uint64  // Offset to series section
    LabelIndices       uint64  // Offset to label index
    LabelIndicesTable  uint64  // Label index lookup table
    Postings           uint64  // Offset to postings
    PostingsTable      uint64  // Postings lookup table
    FingerprintOffsets uint64  // Fingerprint → offset mapping
    Metadata           Metadata
}

type Metadata struct {
    From, Through int64   // Time range
    Checksum      uint32  // CRC32
}
```

### Series Entry

```
Labels: {service_name="api", profile_type="cpu", job="ingester"}
Fingerprint: 0x1234567890abcdef
SeriesRef: 0x00000001
```

### Postings Lists

Map label pairs to series IDs:

```
Posting("service_name", "api") → [1, 5, 12, 45, ...]
Posting("profile_type", "cpu") → [1, 2, 3, 4, 5, ...]

Intersection → Series matching both labels
```

### Query Pattern

```go
// 1. Parse label matchers
matchers := parseSelector("{service_name='api', profile_type='cpu'}")

// 2. Get postings for each matcher
postings := index.Postings(matchers...)

// 3. Intersect postings
seriesIDs := intersectPostings(postings)

// 4. Get series metadata
for _, id := range seriesIDs {
    series := index.Series(id)
    // series.Labels, series.Chunks, etc.
}
```

---

## 5. Symbol Database

**Location:** `pkg/phlaredb/symdb/`

### Purpose

Stores resolved symbols (function names, file locations, etc.) for stack traces.

### Format V3 (Current)

**Single File Structure:**
```
┌──────────────────────┐
│ Partition Data       │  Variable size
├──────────────────────┤
│ Index Section        │  Partition headers + TOC
├──────────────────────┤
│ Footer (24 bytes)    │  Magic + Version + Offsets
└──────────────────────┘
```

### Footer (24 bytes)

```
[0-3]   Magic: 'sym1'
[4-7]   Version: 3
[8-15]  IndexOffset (uint64)
[16-19] Reserved
[20-23] CRC32 Checksum
```

### Index Structure

```
IndexHeader (16B)
TOC (Table of Contents)
PartitionHeaders[]
  ├── Partition ID
  ├── NumStacktraces
  ├── StacktraceBlockHeaders[]
  └── SymbolsBlockReferences
      ├── Locations
      ├── Mappings
      ├── Functions
      └── Strings
```

### Symbol Resolution

```
StacktraceID → Locations[] → Functions[] → Strings[]
                    ↓
             Resolved Stack Trace:
             [
               "main.handler() at main.go:42",
               "http.ServeHTTP() at server.go:123",
               ...
             ]
```

---

## 6. Object Storage Organization

### Directory Structure

```
blocks/
├── 0/                               # Shard 0
│   ├── tenant-a/
│   │   ├── 01J2VJR31PJTT89RKM1ZG8BSB9/
│   │   │   └── block.bin
│   │   └── 01J2VJR42QKUU9ASLN2AH9CTC0/
│   │       └── block.bin
│   └── tenant-b/
│       └── ...
├── 1/                               # Shard 1
│   └── ...
└── N/                               # Shard N

segments/
├── 0/
│   └── anonymous/
│       ├── 01J2VJQTJ3PGF7KB39ARR1BX3Y/
│       │   └── block.bin
│       └── ...
└── ...
```

### Path Construction

**File:** `pkg/block/object.go`

```go
func BuildObjectPath(meta *BlockMeta) string {
    dir := "blocks"
    if meta.CompactionLevel == 0 {
        dir = "segments"
    }
    tenant := meta.StringTable[meta.Tenant]
    return fmt.Sprintf("%s/%d/%s/%s/block.bin",
        dir, meta.Shard, tenant, meta.ID)
}
```

---

## 7. Reading Data: Store Gateway Flow

### High-Level Flow

```
Query Request
    ↓
1. Get tenant's BucketStore
    ↓
2. Sync block metadata from object storage
    ↓
3. Filter blocks by time range
    ↓
4. Open relevant datasets
    ├─ Load into memory if size < 1MB
    └─ OR read on-demand from storage
    ↓
5. Execute queries
    ├─ TSDB: Series lookup
    ├─ Parquet: Profile data
    └─ Symbols: Stack resolution
    ↓
6. Merge results
    ↓
Response
```

### Block Loading

**File:** `pkg/storegateway/bucket.go`

```go
func (bs *BucketStore) addBlock(meta *BlockMeta) error {
    // 1. Create Block object
    block := block.NewObject(storage, meta)

    // 2. Add to block set
    bs.blocks.Add(block)

    // 3. Open if recent
    if isRecent(meta) {
        block.Open(ctx)
    }
}
```

### Dataset Opening

**File:** `pkg/block/dataset.go`

```go
func (d *Dataset) Open(ctx context.Context, sections ...Section) error {
    // Load entire dataset into memory if small
    if d.meta.Size < memSizeThreshold {
        d.obj.LoadIntoMemory(ctx)
    }

    // Initialize requested sections
    for _, sec := range sections {
        switch sec {
        case SectionProfiles:
            d.profiles = openParquet(...)
        case SectionTSDB:
            d.tsdb = openTSDB(...)
        case SectionSymbols:
            d.symbols = openSymDB(...)
        }
    }
}
```

---

## 8. Memory Management

### Size Thresholds

```go
defaultObjectSizeLoadInMemory = 1 MB
defaultTenantDatasetSizeLoadInMemory = 1 MB
```

### Buffer Pooling

**File:** `pkg/util/bufferpool/`

```go
// Pre-allocated buffers for reading
readBufferPool := sync.Pool{
    New: func() interface{} {
        return make([]byte, 64KB)
    },
}

// Reuse across queries
buf := readBufferPool.Get().([]byte)
defer readBufferPool.Put(buf)
```

### Lazy Loading

- Metadata: Loaded first, always in memory
- TSDB Index: Loaded on first query
- Profiles: Streamed or loaded based on size
- Symbols: Loaded on-demand during symbolization

---

## 9. Data Flow Example

### Query: Time Series for `{service="api", profile_type="cpu"}`

```
1. TSDB Index Lookup
   Input: Label matchers
   Output: SeriesIndex values [12345, 12346, ...]

2. Parquet Scan
   Filter: WHERE SeriesIndex IN (12345, 12346, ...)
   Read columns: TimeNanos, TotalValue

3. Group & Aggregate
   Group by: Time buckets (step interval)
   Aggregate: SUM(TotalValue) per bucket

4. Build Response
   Output: [{timestamp: t1, value: v1}, ...]
```

### Query: Flame Graph for Same Selector

```
1. TSDB Index Lookup (same as above)

2. Parquet Scan
   Filter: WHERE SeriesIndex IN (...)
   Read columns: Samples[].StacktraceID, Samples[].Value

3. Symbol Resolution
   Input: StacktraceID values
   Lookup: Symbol database
   Output: Resolved stack traces

4. Build Tree
   Insert each stack trace into tree
   Accumulate values

5. Generate Flame Graph
   Convert tree to level-based format
   Output: FlameGraph protobuf
```

---

## 10. Performance Optimizations

### Compression

- **Parquet:** Delta encoding for integers, dictionary encoding for strings
- **TSDB:** Compact postings lists with bitmap encoding
- **Symbols:** Partitioned storage, reference deduplication

### Indexing

- **Fingerprint Offsets:** Every 1024th series cached for fast lookup
- **Label Indices:** Pre-built label → values mapping
- **Postings:** Inverted index for O(1) label matcher lookup

### Caching

- **Block Metadata:** Cached in memory per tenant
- **Index Headers:** Loaded once, reused for all queries
- **Recently Accessed Blocks:** Kept open for subsequent queries

### Streaming

- Large Parquet files: Row group streaming (10K rows/group)
- Symbol resolution: On-demand, only for requested stacks
- Profile iteration: Lazy evaluation, yield profiles one at a time

---

## 11. Key Files Reference

| Component | File | Purpose |
|-----------|------|---------|
| **Block Object** | `pkg/block/object.go` | Block loading and lifecycle |
| **Dataset** | `pkg/block/dataset.go` | Dataset sections and opening |
| **Metadata** | `pkg/block/metadata/metadata.go` | Encode/decode block metadata |
| **Parquet Schema** | `pkg/phlaredb/schemas/v1/profiles.go` | Profile table definition |
| **TSDB Index** | `pkg/phlaredb/tsdb/index/index.go` | Index structure and TOC |
| **Symbol DB** | `pkg/phlaredb/symdb/format.go` | Symbol format and partitions |
| **Store Gateway** | `pkg/storegateway/bucket.go` | Block syncing and serving |

---

## Next Steps

1. **Explore block files**: Use `profilecli` to inspect blocks locally
2. **Read Parquet schemas**: See `pkg/phlaredb/schemas/v1/`
3. **Understand TSDB**: Read Prometheus TSDB docs for context
4. **Trace queries**: Add logging in `block_reader.go` to see data access
