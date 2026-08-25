# PYROEP-001: Metadata result cache

Status: Draft

## Summary

Add a full-result cache for V2 `LabelNames`, `LabelValues`, and `Series` queries.
The first query-backend receiving a request coordinates caching: it partitions
the request into aligned, per-tenant duration tiers, reads matching results from
a dedicated object-storage bucket, executes cache misses through the existing
query-backend DAG, merges reports, and asynchronously schedules cache writes.

The query frontend continues to validate the request, query the metastore, and
build the full-range plan. It does not read or write cache entries, merge
cached reports, or publish cache metrics.

Caching is opt-in per tenant. Cache identities include the exact fragment query
and sorted block IDs, so late ingestion, compaction, and retention naturally
select different entries. The smallest configured fragment duration is also
the minimum cache age.

## Motivation

Metadata queries repeatedly scan TSDB sections from object storage. Caching
results for an exact query and block set reduces object-storage reads and
query-backend work while allowing recent data to participate safely.

The cache must preserve current query semantics, fail open when cache storage
is unavailable, and keep its storage and lifecycle independent from profile
blocks.

## Goals

- Cache complete metadata-query reports for aligned duration tiers.
- Coordinate cache lookup, execution, merging, and writes in the first
  query-backend.
- Store cache entries in a dedicated object-storage bucket.
- Enable caching per tenant and invalidate entries through tenant generations.
- Invalidate entries naturally when the fragment's block set changes.
- Keep cache writes outside the query result delivery path.
- Provide global, low-cardinality cache metrics.
- Keep the most recent smallest-duration window uncached.

## Non-goals

- Caching federated requests or diagnostic requests.
- Distributed locking or cache-stampede prevention.
- Application-managed expiration or immediate deletion of old generations.
- Returning diagnostics from cached execution.

## Architecture

The query frontend keeps its existing responsibilities:

1. Validate and sanitize the query range.
2. Query the metastore.
3. Build a full-range query plan.
4. Send one `InvokeRequest` to the query-backend.

The first query-backend receiving an `InvokeRequest` becomes the cache
coordinator:

1. Determine whether the request is eligible for result caching.
2. Split the request into aligned tiered fragments.
3. Look up eligible fragments in the result-cache bucket.
4. Create fragment-specific plans for misses and uncached fragments.
5. Execute those fragments through the normal query-backend DAG.
6. Merge cached and executed reports.
7. Enqueue successful eligible misses for asynchronous cache persistence.
8. Return a normal `InvokeResponse`.

The coordinator runs only on the first backend. Add an internal field to
`queryv1.InvokeOptions`:

```protobuf
bool bypass_result_cache = 3;
```

The frontend leaves the field unset. Before executing or dispatching every
fragment, the coordinator clones the request and sets it to `true`, preventing
child query-backends from coordinating the cache a second time.

Requests that are disabled or ineligible still pass through the coordinator
once. It marks them as bypassed and executes their original plan normally.

## Tenant configuration

Add generic runtime overrides:

```yaml
overrides:
  tenant-a:
    result_cache_enabled: true
    result_cache_generation: 1
    result_cache_fragment_durations: [24h, 2h, 15m]
    result_cache_metadata_service_name_min_query_duration: 7d
```

Defaults:

```yaml
result_cache_enabled: false
result_cache_generation: 1
result_cache_fragment_durations: [24h, 2h, 15m]
result_cache_metadata_service_name_min_query_duration: 7d
```

Expose these through:

```go
ResultCacheEnabled(tenantID string) bool
ResultCacheGeneration(tenantID string) uint32
ResultCacheFragmentDurations(tenantID string) []time.Duration
```

The enable flag controls participation. Generation is only an invalidation
namespace and must not double as an enable switch. Fragment durations are
positive, unique, evenly divisible tiers. Durations must be multiples of 15
minutes, and at most eight tiers may be configured per tenant. The minimum
duration and tier-count limit bound request fan-out.

## Dedicated cache bucket

Provision a dedicated result-cache bucket through deployment infrastructure or
IaC. Add a top-level `result_cache` configuration section with a separate
object-store client configuration; do not reuse `storage`.

The query-backend owns this client. Query frontends do not need credentials for
the result-cache bucket.

The separate bucket provides independent lifecycle policies, IAM permissions,
cost accounting, and retention. Cache storage errors are fail-open: the query
executes normally if the bucket is unconfigured or unavailable.

The backend requires only read and create-or-replace permissions. Lifecycle
management owns deletion.

## Tiered partitioning and recent data

Split the sanitized time range into epoch-aligned, inclusive-millisecond
fragments. At each boundary, choose the largest configured duration that fits,
then fall back through smaller durations. With the default tiers, this produces
`24h`, `2h`, and `15m` fragments. Unaligned edge remainders execute normally
and are not cached.

The smallest configured duration is also the minimum cache age. With the
default `15m` tier, fragments ending within the latest 15 minutes execute
normally without a cache lookup or write. This replaces the ingestion-window
stability delay.

## Fragment plans

The initial query plan already includes full-range block metadata. The
coordinator creates fragment plans without another metastore request:

1. Flatten and deduplicate the blocks in the original plan.
2. Clone blocks overlapping the fragment.
3. Remove datasets that do not overlap the fragment.
4. Remove blocks with no remaining datasets.
5. Sort blocks by ID.
6. Build the fragment plan using `queryplan.Build`.

Dataset filtering is required because `LabelNames` does not independently
timestamp-filter series within a selected dataset.

Fragments with no matching blocks return empty reports and may be cached when
otherwise eligible. Cache lookups and fragment execution use a fixed maximum
concurrency of 64 to avoid unbounded fan-out for long permitted query ranges.
Block IDs identify immutable block contents and execution-relevant metadata.

## Cache key and format

Cache objects use this key layout:

```text
results-cache/$fragment-duration/$tenant-id/%04d-$cache-hash
```

For example:

```text
results-cache/24h/tenant-a/0001-d5f3...c02a
```

`$cache-hash` is the full lowercase, 64-character SHA-256 digest of a
deterministic protobuf serialization of `ResultCacheKey`. It includes the exact
fragment boundaries, selector, query parameters, and sorted block IDs. Tenant,
generation, and fragment duration are excluded because they occur in the path.

Add this protobuf message to `api/query/v1/query.proto`:

```protobuf
message ResultCacheKey {
  QueryRequest query = 1;
  repeated string block_ids = 2;
}

message ResultCacheEntry {
  ResultCacheKey key = 1;
  repeated Report reports = 2;
}
```

Reports are stored before public client-capability filtering;
the query frontend continues to apply UTF-8 label-name filtering after the
merged result is returned.

Empty report lists are valid cache entries.

## Lookup, validation, and collisions

An entry is served only when it unmarshals successfully and its stored cache
identity exactly matches the expected query and sorted block list.

- Object-not-found is a normal cache miss.
- A read failure is an `error` outcome. Execute normally and do not enqueue a
  write because the current object state is unknown.
- A corrupt protobuf is an `error` outcome. Execute normally; a successful
  result may replace the corrupt object.
- A decoded entry whose identity does not match is a `collision` outcome.
  Execute normally and do not overwrite that key, preventing collision
  ping-pong.

Concurrent writers for the same validated request are not collisions and may
safely replace one another with equivalent complete results.

## Asynchronous cache writes

Cache persistence must not delay or alter query result delivery. After a
successful eligible cache miss, the coordinator non-blockingly enqueues an
immutable write job and returns the query result independently.

A job contains the object key, cache identity, fragment duration, reports, and
query type. A query-backend-owned worker
pool then:

1. Builds and marshals `ResultCacheEntry`.
2. Uses a service-scoped context with a write timeout, never the request
   context.
3. Uploads the object to the result-cache bucket.
4. Records the write outcome.

The queue is bounded. If it is full, the backend drops the write immediately;
it never blocks query result delivery. A subsequent request can repopulate the
entry.

Do not enqueue after cache collisions, unknown-state read errors, query errors,
request cancellation, or for ineligible fragments. Corrupt entries may be
replaced after a successful execution.

The writer belongs to the query-backend service lifecycle. On startup it
creates the queue and starts its workers. On shutdown it stops accepting new
jobs, drains queued jobs best-effort within the normal shutdown deadline, then
cancels active uploads and discards remaining jobs.

## Report merging and diagnostics

Use the existing query-backend report aggregation framework to merge cached and
executed fragment responses. `LabelNames` aggregation unions, deduplicates, and
sorts names.

Cache hits contribute zero object-storage bytes. The coordinator sums bytes
from executed fragments so existing query-frontend statistics remain accurate.

Requests with `CollectDiagnostics` enabled bypass result caching to avoid
returning incomplete or misleading execution trees.

Federated requests containing more than one tenant also bypass caching. The
key format intentionally has a single tenant path component.

## Failure handling

The cache is fail-open. The backend executes normally for disabled tenants,
unsupported query shapes, federated requests, diagnostic requests, unaligned
or recent fragments, cache misses, and cache failures.

Only errors from normal query execution are returned to callers. Cache lookup,
queueing, marshaling, and upload errors cannot fail a query.

## Metrics and logging

Emit global, low-cardinality query-backend counters:

```text
pyroscope_query_backend_result_cache_lookups_total{
  query_type="label_names",
  fragment_duration="24h",
  outcome="hit|miss|error|collision"
}
```

```text
pyroscope_query_backend_result_cache_writes_total{
  query_type="label_names",
  fragment_duration="24h",
  outcome="success|error|dropped"
}
```

Metrics count eligible aligned fragments. Bypassed fragments do not count as
misses. Fragment duration values are canonicalized from the validated tier
list. Do not add tenant IDs, cache keys, dates, generations, or selectors as
labels.

Logs may include the operation, query type, UTC date, generation, and error
classification. They must not include selectors, report contents, or sensitive
tenant identifiers.

## Cleanup

Pyroscope does not delete cache entries. Configure lifecycle expiration on the
entire dedicated bucket, including version cleanup where the provider supports
it. AWS S3, Google Cloud Storage, and Azure Blob Storage all support this.

Expired entries become cache misses and are recomputed. Old generations expire
through the same lifecycle policy.

## Rollout

1. Provision dedicated cache buckets and lifecycle policies.
2. Deploy the implementation with `result_cache_enabled: false` by default.
3. Opt in selected tenants through runtime overrides.
4. Monitor global lookup and write outcomes, storage cost, and backend load.
5. Expand opt-in gradually.
6. Increment a tenant generation to invalidate its entries.

Rollback consists of setting `result_cache_enabled: false`; existing objects
expire through the bucket lifecycle policy.

## Testing

Unit tests must cover:

- Tiered splitting, alignment, and inclusive millisecond boundaries.
- Unaligned edge and minimum-cache-age handling.
- Diagnostic and federated bypasses.
- Tenant enablement, generation, and duration-tier selection.
- Plan flattening and block/dataset overlap filtering.
- Exact key formatting, generation padding, and deterministic hashing.
- Hash sensitivity to exact time fields, duration, and block-set changes.
- Protobuf round trips with real fragment boundaries and sorted block IDs.
- Hits, misses, corrupt entries, read errors, and collisions.
- Empty result caching and cached/executed report merging.
- Non-blocking queueing, dropped writes, worker upload failures, and
  best-effort shutdown draining.
- Metric outcomes, fragment-duration labels, and the absence of tenant labels.

Integration tests must verify that hits avoid downstream block reads and return
