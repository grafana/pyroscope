# Query-backend tree-result cache

The query backend can cache a serialized tree report for a query against one
immutable block dataset. A hit avoids reopening that dataset, scanning its
profiles, resolving symbols, and rebuilding the tree. Format1 dataset-index
lookup still occurs before the cache lookup. This cache is process-local, so
repeat requests must reach the same backend pod to hit.

The cache is disabled by default. Set
`-query-backend.tree-result-cache-max-bytes=<bytes>` on query-backend pods to
enable it; setting the flag to `0` disables it again. The configured budget
covers serialized report payloads, not map/LRU overhead or in-flight decoded
reports. Size the budget with pod RSS and the cache metrics in mind. Enabling
the cache does not change object storage contents or any write-path component.

Each key identifies the immutable object and dataset offset/size, plus the
tenant list, label selector, complete tree query, and request options. The
request time range is clipped to the dataset's recorded bounds. Windows that
fully cover the same dataset therefore share an entry, while partial windows
remain distinct. Because the recorded maximum is in milliseconds and profile
timestamps can have finer precision, a query must end *after* that maximum
millisecond to count as fully covering the dataset. Missing or invalid bounds
retain the exact request range. A hit deserializes a fresh report before
aggregation; callers cannot mutate the cached bytes. Concurrent misses may
perform duplicate work, but cannot share mutable output.

The backend exports these per-pod metrics:

- `pyroscope_query_backend_tree_result_cache_hits_total`
- `pyroscope_query_backend_tree_result_cache_misses_total`
- `pyroscope_query_backend_tree_result_cache_evictions_total`
- `pyroscope_query_backend_tree_result_cache_oversize_total`
- `pyroscope_query_backend_tree_result_cache_size_bytes`
- `pyroscope_query_backend_tree_result_cache_entries`

The block-backed `BlockReader.Invoke` benchmark exercises Format1
dataset-index lookup as well as the tree query. On its small fixture, an
uncached invocation took 13–15 ms/op and a warm hit took 0.18–0.23 ms/op
(three five-operation trials on Apple M4 Pro), with equal reports. This is
a local functional benchmark, not a prediction of end-to-end latency on
larger or moving-window workloads. In production, compare fixed and moving
windows, request latency, hit rate, evictions, fetched bytes, and pod memory
before choosing a budget.
