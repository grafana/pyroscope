# Symbol Bloom Index for Pyroscope

## Summary

Build a compact per-tenant, per-block Bloom index during L1+ compaction that maps exact `Function.Name` membership to candidate `service_name` datasets.

The Bloom index is not the source of truth. It is a pruning structure. Query execution uses it to find candidate services quickly, then opens existing `symbols.symdb` and profile data for exact verification or metric computation.

This supports two primary use cases:

- Querying or backfilling metrics from profiles for stacktraces containing a function.
- Searching for services affected by a CVE or critical flaw by exact function symbol name.

## Goals

- Query by exact `Function.Name`.
- Return services that contain the symbol.
- Optionally compute profile-derived metrics for matching stacktraces.
- Keep index storage small.
- Preserve existing sharding and block-local query execution.
- Make lookup horizontally parallel across blocks, shards, tenants, and services.

## Non-Goals

- No prefix, regex, package, or module search in the first version.
- No filename, line number, or `Function.SystemName` matching.
- No L0 fallback; results are available after L1 compaction.
- No exact inverted posting list in the first version.
- No stacktrace refs stored in the index.

## Terminology And Existing Metadata

The implementation should reuse existing block metadata conventions where possible:

```text
metadata.LabelNameTenantDataset = "__tenant_dataset__"
metadata.LabelValueDatasetTSDBIndex = "dataset_tsdb_index"
```

The new pseudo-dataset should add a sibling tenant dataset value:

```text
metadata.LabelValueSymbolBloomIndex = "symbol_bloom_index"
```

Profile type filtering should use the existing `__profile_type__` label during exact verification. The Bloom row does not store profile type; profile types in API responses are derived from verified profile rows.

`dataset_index` should refer to the position of the real service/profile dataset in the containing block metadata. It must not refer to the tenant-wide `dataset_tsdb_index` pseudo-dataset.

## Use Cases

### Backfilling Or Querying Metrics From Profiles

Users may want to compute a metric from historical profiles for all stacktraces containing a function, for example CPU samples attributed to `runtime.mallocgc` grouped by service.

The Bloom index should prune the search space to candidate datasets. The query backend then verifies exact symbol presence and scans only candidate profile samples to compute the result.

### CVE Or Critical Flaw Search

Users may want to know which services have ever contained a vulnerable function symbol over a time range.

The Bloom index should quickly identify candidate services and profile types. The query backend should verify exact presence through `symbols.symdb` before returning security-sensitive results by default.

## Data Model

Add a new tenant pseudo-dataset to compacted blocks:

```text
__tenant_dataset__ = "symbol_bloom_index"
```

Each symbol Bloom index payload contains entries like:

```text
service_name
dataset_index
min_time
max_time
bloom_filter(Function.Name values)
```

The containing block metadata already provides:

```text
tenant
shard
block_id
compaction_level
block time range
```

The index remains block-local. No global symbol index is required.

## Compaction Behavior

During L1+ compaction:

1. Compact real service datasets as today.
2. While rewriting symbols, observe `Function.Name` values used by stacktraces.
3. Group observed function names by `service_name + dataset_index`.
4. Build one Bloom filter per service dataset entry.
5. Write the symbol Bloom index payload into the block.
6. Add a pseudo-dataset metadata entry labeled `__tenant_dataset__="symbol_bloom_index"`.

This index should be rebuilt for every compacted output block at `CompactionLevel >= 1`, including L2+ compactions, so lookup remains available after older blocks are compacted away.

The index is intentionally absent from L0 blocks. A query over a range that still has un-compacted L0 data cannot claim complete coverage unless the query path either excludes that time span or reports partial index coverage.

## Query Flow: Service And CVE Lookup

The public service lookup API is `querier.v1.SymbolLookup`. The query frontend handles it as two sequential phases. This does not pipeline yet: validation starts after the Bloom phase has returned all candidates.

### Phase 1: Bloom Candidate Lookup

1. User queries one or more exact `Function.Name` values via `querier.v1.SymbolLookup`.
2. Query frontend starts span `SymbolLookup.bloom`.
3. Query frontend asks metastore for `symbol_bloom_index` pseudo-datasets in the requested time range.
4. Query frontend invokes the query backend with internal query type `QUERY_SYMBOL_BLOOM_CANDIDATES`.
5. Query backend reads symbol Bloom index payloads from matching blocks.
6. For each row, the backend checks Bloom membership for the requested symbols.
7. Bloom misses are skipped.
8. Bloom hits become `SymbolBloomCandidate` rows containing block ID, dataset index, service name, matching requested symbols, time bounds, and symbol count estimate.

### Phase 2: Exact Validation

1. Query frontend starts span `SymbolLookup.validate`.
2. Query frontend starts sub-span `SymbolLookup.validationMetadata`.
3. Query frontend builds a `service_name` selector from Bloom candidates and performs a second metadata query.
4. Metadata results are filtered to candidate block IDs.
5. Query frontend invokes the query backend with internal query type `QUERY_SYMBOL_SERVICES`, passing the candidate list on `SymbolServicesQuery`.
6. Query backend groups candidates by `(block_id, dataset_index)`.
7. Query backend opens only candidate datasets' profile, TSDB, and `symbols.symdb` sections.
8. Query backend verifies that the requested `Function.Name` exists in the dataset before returning a service.
9. Query frontend returns deduped services and verified profile types.

Responses should be exact for indexed data. Bloom candidates must be verified against `symbols.symdb` before they are returned. If the requested time range includes L0-only data or blocks where index writing was disabled, the response should indicate incomplete index coverage instead of silently presenting the result as complete.

### Current Trace Findings

Trace `f456cf87d90b7cf2927a8123b80a941f` shows the two-phase path is active:

- `SymbolLookup`: about 6.2s.
- `SymbolLookup.bloom`: about 783ms.
- `SymbolLookup.validationMetadata`: about 61ms.
- `SymbolLookup.validate`: about 5.4s.
- Bloom phase: 187 blocks, 1,932 candidates.
- Validation phase: 187 blocks, 1,932 candidates, 829 services.
- Object storage reads: about 833 MiB requested, about 781 MiB unique ranges.
- Repeated range amplification is low, around 1.07x.

The original repeated-read problem is mostly resolved. The current bottleneck is validation scan volume: the trace scanned about 1.43M profiles across candidates.

## Query Flow: Metrics From Profiles

1. User queries exact `Function.Name` with time range, profile type, selector, step, and group-by labels.
2. Query frontend finds `symbol_bloom_index` blocks.
3. Query backend checks Bloom filters to prune candidate service/profile datasets.
4. For candidate datasets, open `symbols.symdb` and verify matching functions.
5. Resolve matching stacktraces from the existing symbol tree.
6. Scan profile samples only for candidate datasets.
7. Aggregate samples whose stacktrace contains the symbol.
8. Return metric-like time series.

The Bloom index only prunes. It does not store metric values.

## API Sketch

Public service lookup lives on `querier.v1`:

```proto
rpc SymbolLookup(SymbolLookupRequest) returns (SymbolLookupResponse) {}

message SymbolLookupRequest {
  int64 start = 1;
  int64 end = 2;
  string label_selector = 3;
  repeated string symbol_names = 4;
}

message SymbolLookupResponse {
  repeated SymbolLookupResult results = 1;
  bool complete = 2;
}

message SymbolLookupResult {
  string symbol_name = 1;
  repeated SymbolLookupService services = 2;
}

message SymbolLookupService {
  string service_name = 1;
  repeated string profile_types = 2;
}
```

Internal query backend APIs split discovery from validation:

```proto
enum QueryType {
  QUERY_SYMBOL_BLOOM_CANDIDATES = 10;
  QUERY_SYMBOL_SERVICES = 9;
}

message SymbolBloomCandidatesQuery {
  repeated string symbol_names = 1;
}

message SymbolBloomCandidatesReport {
  SymbolBloomCandidatesQuery query = 1;
  repeated SymbolBloomCandidate candidates = 2;
  bool complete = 3;
}

message SymbolBloomCandidate {
  string block_id = 1;
  uint32 dataset_index = 2;
  string service_name = 3;
  repeated string symbol_names = 4;
  int64 min_time = 5;
  int64 max_time = 6;
  uint32 symbol_count_estimate = 7;
}

message SymbolServicesQuery {
  repeated string symbol_names = 1;
  repeated SymbolBloomCandidate candidates = 2;
}
```

`complete=false` means the response is verified for scanned indexed blocks but may miss data from unindexed blocks, such as L0 blocks or blocks created before rollout.

Future metrics endpoint:

```proto
rpc SymbolMetrics(SymbolMetricsRequest) returns (SymbolMetricsResponse) {}
```

The metrics API can mirror existing time series query concepts:

```text
symbol_name
matchers
start/end
step
profile_type
group_by
aggregation
```

## Profile CLI Plan

Add `profilecli` support as a first-class client for the symbol lookup API. This is useful for engineering, incident response, and security workflows where users want a scriptable answer without opening the UI.

Initial command:

```text
profilecli query symbols \
  --symbol 'github.com/example/vulnerable.Function' \
  --from now-30d \
  --to now \
  --query '{service_name=~"pyroscope/.*"}' \
  --output table
```

The command should call the public `querier.v1.SymbolLookup` API and support:

```text
--symbol       Exact Function.Name to search for. Required. Repeatable.
--query        Existing label selector syntax. Optional.
--from         Start time. Required or defaulted consistently with other query commands.
--to           End time. Required or defaulted consistently with other query commands.
--output       table or json.
```

Default output should be verified and human-readable:

```text
SERVICE_NAME                 PROFILE_TYPES
pyroscope/distributor        process_cpu,memory,mutex
pyroscope/query-frontend     process_cpu
```

If the API returns `complete=false`, table output should print a warning before the table and JSON output should include the top-level `complete` value.

JSON output should preserve enough structure for automation:

```json
{
  "symbols": ["github.com/example/vulnerable.Function"],
  "from": "2026-06-18T00:00:00Z",
  "to": "2026-06-19T00:00:00Z",
  "complete": true,
  "results": [
    {
      "symbol_name": "github.com/example/vulnerable.Function",
      "services": [
        {
          "service_name": "pyroscope/distributor",
          "profile_types": ["process_cpu", "memory", "mutex"]
        }
      ]
    }
  ]
}
```

Future command for metrics-from-profiles:

```text
profilecli query symbol-metrics \
  --symbol 'runtime.mallocgc' \
  --profile-type process_cpu:cpu:nanoseconds:cpu:nanoseconds \
  --from now-7d \
  --to now \
  --step 1h \
  --group-by service_name \
  --output json
```

The `symbol-metrics` command should wait until the backend `SymbolMetrics` API is implemented. The first CLI change should only add `query symbols` for service/CVE lookup.

## Bloom Filter Sizing

Recommended starting point:

```text
false positive rate: 0.1% to 1%
bits per symbol: ~10 to 14
bytes per unique function name: ~1.25 to 1.75
```

Example:

```text
10k unique function names in a service/profile dataset
1% FP filter: ~12.5 KB
0.1% FP filter: ~17.5 KB
```

For ops-scale planning, this likely lands in:

```text
tens to hundreds of MB per hour across the cell
```

This is expected to be significantly smaller than an exact inverted index.

## Correctness

Bloom filters can produce false positives but not false negatives, assuming the filter is built correctly.

- Bloom miss means the dataset can be skipped.
- Bloom hit means the dataset must be verified for exact results.
- Security/CVE workflows should use exact verification by default.
- Completeness depends on index coverage. L0-only blocks and blocks created while index writing is disabled are unindexed unless a later compaction rebuilds the index.

## Operational Guardrails

The first implementation should include hard limits to prevent one tenant, service, or malformed symbol payload from creating unbounded compaction or query cost:

- Maximum accepted `symbol_name` length for lookup requests.
- Maximum unique function names per Bloom row before the row is skipped or marked truncated.
- Maximum Bloom bytes per row and per output block.
- Maximum candidate datasets per query before returning a resource-exhausted error.
- Query timeout and cancellation checks inside morsel workers.
- Metrics for skipped or truncated Bloom rows.

If truncation is allowed, the API must treat affected blocks as incomplete. A simpler first implementation can omit the symbol Bloom index for the affected output block and rely on rollout limits until sizing data is available.

## Sharding

Sharding continues to work because the index is block-local.

Each Bloom index belongs to:

```text
tenant + shard + block + compaction level
```

Query planning already fans out across matching blocks and shards. Results are unioned and deduped by `service_name` and `profile_type`.

## Storage Format

Store the symbol Bloom index as a Parquet table in the compacted block.

The block metadata still represents the table as a tenant pseudo-dataset:

```text
__tenant_dataset__ = "symbol_bloom_index"
```

The pseudo-dataset points to a `symbol_bloom.parquet` payload. Each row represents one searchable service dataset within the containing block. This likely requires a new dataset format or section identifier; it should not overload `dataset_tsdb_index` format semantics.

Proposed Parquet schema:

```text
service_name: string
dataset_index: uint32
min_time: int64
max_time: int64
bloom_bits: binary
bloom_hash_count: uint32
bloom_bit_count: uint32
symbol_count_estimate: uint32
format_version: uint32
```

The Bloom filter remains opaque to Parquet. Parquet stores and compresses the table; query code reads `bloom_bits` and tests membership in Go.

The Bloom hash function, seed, bit ordering, and serialization must be part of `format_version` compatibility. Readers should reject unknown versions instead of guessing.

Parquet advantages:

- Easier inspection and debugging with existing tools.
- Schema evolution without a custom binary compatibility layer.
- Compression for repeated `service_name` values.
- Predicate pushdown for `service_name` and time range filters.
- Alignment with the existing block format, which already stores profiles in Parquet.

Parquet tradeoffs:

- More footer/read overhead than a minimal custom binary format.
- Bloom membership checks cannot be pushed down into Parquet.
- Many very small Bloom tables may be less efficient than a custom packed format.

If Parquet overhead proves too high, a later format version can replace the payload with a custom binary encoding behind the same pseudo-dataset label.

## Columnar Query Execution

The symbol Bloom index query path should use a columnar morsel execution model instead of the current row-oriented query style.

The initial implementation should use four workers per query backend read task. Each worker processes independent morsels from the `symbol_bloom.parquet` table. A morsel should be a bounded unit of Parquet work, such as one row group or a fixed row range within a row group, chosen so workers can make progress independently without excessive scheduling overhead.

Projected columns for service lookup:

```text
service_name
dataset_index
min_time
max_time
bloom_bits
bloom_hash_count
bloom_bit_count
format_version
```

The reader should avoid materializing full rows. It should read only the projected columns, apply cheap column-level filters first, and test Bloom filters only for rows that survive those filters.

Execution flow:

1. Open the `symbol_bloom.parquet` section for a block.
2. Plan morsels from row groups or fixed row ranges.
3. Start four workers for the block read task.
4. Workers read projected columns for their morsels.
5. Workers apply time and service filters before reading or testing Bloom bytes where possible.
6. Workers test `bloom_bits` for `Function.Name` membership.
7. Workers emit candidate `dataset_index` values.
8. The query backend dedupes candidates and performs exact verification with `symbols.symdb`.

This keeps the Bloom index scan horizontally parallel across query plan blocks and locally parallel within each block. The four-worker limit is a starting point, not a global concurrency target. It should be configurable or easy to tune if production measurements show a different value is better.

The exact verification step can remain dataset-oriented initially. The important requirement for the first implementation is that scanning the Bloom Parquet table is columnar and morsel-based.

## Implementation Steps

1. Add metadata constants, including `LabelValueSymbolBloomIndex = "symbol_bloom_index"`.
2. Add a new dataset section or format for symbol Bloom index payloads without overloading `dataset_tsdb_index`.
3. Implement `SymbolBloomIndexWriter` and `SymbolBloomIndexReader`.
4. Extend compaction symbol observation to collect `Function.Name` only.
5. Group observed symbols by `service_name + dataset_index`.
6. Write the symbol Bloom index pseudo-dataset during L1+ compaction.
7. Update metastore/query frontend to request `__tenant_dataset__="symbol_bloom_index"` for symbol Bloom candidate queries.
8. Add a four-worker columnar morsel reader for `symbol_bloom.parquet`.
9. Add query backend handlers for `QUERY_SYMBOL_BLOOM_CANDIDATES` and candidate-driven `QUERY_SYMBOL_SERVICES`.
10. Implement exact verification by reading existing `symbols.symdb`, keeping symbol partitions open for the duration of candidate dataset validation.
11. Add query and compaction metrics.
12. Add operational guardrails for symbol length, Bloom bytes, candidate counts, and cancellation.
13. Add tests.
14. Run `make generate` after proto changes.

## Implementation Phases

### Phase 0: Measurement And Validation

Add or validate metrics needed to size the feature before enabling it widely:

- Symbol payload bytes and profile counts by cell, tenant, service, and profile type where available.
- Estimated unique `Function.Name` counts during compaction in dry-run or debug mode.
- Candidate Bloom filter sizes at target false-positive rates.

Deliverable: sizing data from at least one high-volume cell, such as `profiles-ops-002`, and a selected default false-positive target.

### Phase 1: Block-Local Bloom Format

Implement the storage primitive without wiring it into production queries:

- Add metadata constants and dataset format or section identifiers.
- Implement `SymbolBloomIndexWriter` and `SymbolBloomIndexReader`.
- Implement the Parquet schema, format version column, and reader compatibility checks.
- Add unit tests for roundtrip, misses, hits, corrupt payloads, and version mismatch.

Deliverable: a tested local reader/writer that can encode service/profile Bloom entries and read them back.

### Phase 2: Compaction Index Build

Build the Bloom index during L1+ compaction:

- Observe rewritten `Function.Name` values during symbol rewrite.
- Group observed names by `service_name + dataset_index`.
- Write a `symbol_bloom_index` pseudo-dataset into compacted blocks.
- Rebuild the index for L2+ outputs so the index survives further compaction.
- Add compaction metrics for entries, symbol count, bytes, and build duration.

Deliverable: compacted blocks at `CompactionLevel >= 1` contain a symbol Bloom index, but no public query path depends on it yet.

### Phase 3: Internal Query Path And Exact Verification

Add the backend lookup path behind internal query types:

- Query metastore for `__tenant_dataset__="symbol_bloom_index"` in the Bloom phase.
- Read block-local Bloom payloads with the four-worker columnar morsel reader.
- Produce `SymbolBloomCandidate` rows from Bloom hits.
- Query metastore a second time for candidate services and filter to candidate block IDs.
- Verify exact `Function.Name` matches using existing `symbols.symdb` in candidate datasets.
- Return exact service/profile results by default.
- Track false positives by comparing Bloom hits to exact verification misses.

Deliverable: internal Bloom candidate discovery and candidate-driven exact verification work end-to-end with query metrics.

### Phase 4: Public API And Profile CLI

Expose the lookup to users and automation:

- Add the public `querier.v1.SymbolLookup` API.
- Add `profilecli query symbols` using that API.
- Support `table` and `json` output.
- Keep exact verification enabled by default.

Deliverable: users can run exact CVE/service lookup from `profilecli` and API clients.

### Phase 5: Rollout And Operational Tuning

Roll out cautiously:

- Gate index writing and querying behind a feature flag or tenant/cell config.
- Enable in one low-risk cell, then in `profiles-ops-002`, then more broadly.
- Monitor index bytes, compaction latency, query latency, candidate counts, and false positives.
- Tune Bloom false-positive rate and maximum candidate limits if needed.
- Track indexed block coverage and expose incomplete query responses while rollout is partial.

Deliverable: production rollout with dashboards and alerting for build cost, query cost, and false-positive behavior.

### Phase 6: Metrics From Profiles

Add metric backfill/query support after service lookup is proven:

- Add `SymbolMetrics` API.
- Use Bloom filters to prune candidate datasets.
- Verify symbols and resolve matching stacktraces from existing `symbols.symdb`.
- Scan profile samples only in candidate datasets.
- Add `profilecli query symbol-metrics` after the API is stable.

Deliverable: users can compute metric-like time series from profiles for stacktraces containing an exact `Function.Name`.

### Phase 7: Future Search Modes

Consider only after exact `Function.Name` lookup is operational:

- Prefix or package search.
- Module-aware CVE matching.
- Additional Bloom tokenization or a separate package/module index.
- Exact inverted postings if verification reads or false positives are too expensive.

Deliverable: informed follow-up design based on production query metrics, not assumptions.

## Metrics To Add

Compaction metrics:

```text
pyroscope_symbol_bloom_index_entries
pyroscope_symbol_bloom_index_symbols
pyroscope_symbol_bloom_index_bytes
pyroscope_symbol_bloom_index_false_positive_rate_estimate
pyroscope_symbol_bloom_index_build_duration_seconds
pyroscope_symbol_bloom_index_skipped_rows
pyroscope_symbol_bloom_index_truncated_rows
```

Query metrics:

```text
pyroscope_symbol_query_bloom_blocks_checked
pyroscope_symbol_query_bloom_candidates
pyroscope_symbol_query_candidate_datasets
pyroscope_symbol_query_exact_verified
pyroscope_symbol_query_profiles_scanned
pyroscope_symbol_query_profiles_matched
pyroscope_symbol_query_false_positives
pyroscope_symbol_query_duration_seconds
pyroscope_symbol_query_morsels_processed
pyroscope_symbol_query_columnar_bytes_read
pyroscope_symbol_query_incomplete_responses
pyroscope_symbol_query_resource_exhausted
```

## Tests

- Bloom index writer/reader roundtrip.
- Bloom miss skips datasets.
- Bloom hit triggers exact verification.
- `SymbolLookup` runs Bloom candidate lookup before exact validation.
- `SymbolLookup` validation metadata filters by candidate services and candidate block IDs.
- `QUERY_SYMBOL_BLOOM_CANDIDATES` returns correct candidate payloads.
- Candidate-driven `QUERY_SYMBOL_SERVICES` validates only provided candidate datasets.
- Exact `Function.Name` match only.
- No `Function.SystemName` or filename matching.
- L1 compaction writes symbol Bloom index.
- L2+ compaction preserves/rebuilds symbol Bloom index.
- Query returns empty results for L0-only data.
- Query marks responses incomplete for L0-only or partially indexed ranges.
- Query dedupes services across shards/blocks.
- Query enforces candidate limits and returns resource-exhausted errors.
- Query workers respect context cancellation.
- Reader rejects unknown Bloom format versions.
- Metrics query scans only candidate datasets.
- Symbol Bloom Parquet scans project only required columns.
- Symbol Bloom Parquet scans split work into morsels and process them with four workers.
- Symbol Bloom Parquet scans apply service, profile type, and time filters before Bloom membership tests where possible.

## Tradeoffs

Advantages:

- Small storage footprint.
- Highly parallel.
- Simple compaction output.
- Reuses existing symbol/profile data for exactness.
- Suitable for CVE and profile-derived metric workflows.

Disadvantages:

- Cannot answer exact service lookup from the index alone.
- Requires exact verification reads for final results.
- False positives increase query work.
- Does not support prefix/package queries without additional indexing.

## Recommendation

Start with the Bloom-filter-first design.

Use it as a pruning layer for both CVE/service lookup and profile-derived metrics. Keep exact verification in the query path by default. The current implementation splits `SymbolLookup` into Bloom candidate discovery and exact validation in the query frontend. Avoid exact inverted indexes and stacktrace refs until real query metrics show Bloom false positives or validation scan cost are too high.

The next optimization target is reducing validation scan volume. Prioritize early exit, candidate-specific service filtering during exact verification, and explicit validation scan limits before adding frontend/backend pipelining.
