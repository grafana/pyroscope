# Per-block metadata result cache

## Scope

Cache label names, label values, and series labels in object storage only.
Caching is opt-in per tenant with `result_cache_enabled`; the default is false.
Only blocks whose `compaction_level` is exactly 2 are eligible. The request
must cover the block's complete inclusive time range (`start <= min_time` and
`end >= max_time`). Lower-level and partially covered blocks execute uncached.
Diagnostic requests and multi-tenant requests bypass caching.

## Identity

Entries use the versioned `results-cache/blocks/v1/` prefix. Keys include the
tenant, tenant invalidation generation, and a deterministic SHA-256 digest of
an identity containing:

- The canonical label selector and normalized metadata query parameters.
- The block's complete time range, rather than the incoming request's bounds.
- The block ID and a digest of the selected block metadata, including datasets.

The selected dataset identity matters because plans can select different
services or tenant datasets within the same physical block. Changing the
query, dataset selection, block ID, tenant, generation, or format version
selects a different entry. Entries contain the identity alongside the reports;
lookups verify it before accepting a hit. Object-store lifecycle policies may
remove old entries; no Redis/Valkey service or TTL configuration is required.

## Execution

MERGE nodes forward the existing query plan without checking cache eligibility
or looking up entries. READ leaves check eligibility inside the block reader's
existing block-execution loop, after tenant dataset filtering. The cache does
not flatten, rebuild, or modify the query plan. For each eligible block:

1. Start an object-store lookup immediately.
2. Start normal block execution after the configured delay (15 ms by default),
   unless already canceled.
3. If a valid cache hit wins, return its reports and cancel execution.
4. If the lookup misses, fails, or contains an invalid entry, let execution
   continue. A slow lookup does not delay a completed execution.
5. If execution wins successfully, return its result and enqueue an upload.

The race uses separate buffered result channels and a child cancellation
context. Caller cancellation cancels both operations. Each block execution uses
a private aggregator, so a canceled execution cannot contribute reports after a
cache hit wins. Canceled execution is joined before the leaf samples its shared
byte and query-weight counters. Results from all blocks are merged through the
normal metadata aggregator. No bypass flag is needed.

## Asynchronous writes

Successful computed results are cloned into a bounded, nonblocking queue with
128 slots. Two workers marshal and upload entries using a 30-second
per-upload timeout. A full queue drops the write rather than delaying a query.
Write failures do not affect the returned query result. On shutdown, all workers
share one overall 30-second budget for in-flight uploads and draining the remaining
queue. When the budget expires, in-flight uploads are canceled and queued writes
are dropped before the service stops.

## Configuration

Configure the bucket under `query_backend.result_cache.storage` using the standard
bucket configuration (`backend`, `s3`, `gcs`, etc.). For example:

```yaml
query_backend:
  result_cache:
    execution_delay: 15ms
    storage:
      backend: s3
      s3:
        bucket_name: pyroscope-result-cache
```

The corresponding storage flags use the `-query-backend.result-cache.storage.`
prefix. Enable the cache globally with `-query-backend.result-cache.enabled=true`,
or per tenant:

```yaml
overrides:
  tenant-a:
    result_cache_enabled: true
    result_cache_generation: 1
```

The experimental tuning flag
`-query-backend.result-cache.execution-delay` controls the delay before block
execution starts (YAML: `query_backend.result_cache.execution_delay`). It defaults
to `15ms`; `0s` starts execution immediately. Negative durations are rejected.

Increment `result_cache_generation` to invalidate a tenant's existing entries.
Time-fragment, minimum-age, and service-name minimum-query-duration settings
are no longer used or exposed.

## Metrics

`pyroscope_query_backend_result_cache_lookups_total` and
`pyroscope_query_backend_result_cache_writes_total` use `query_type`,
`block_level` (`L2`), and `outcome` labels. Lookups distinguish hits, misses,
cancellations, errors, and identity collisions; writes distinguish success,
error, and drops. Cancellations occur when block execution wins the race
against a still-running lookup and are not errors.
