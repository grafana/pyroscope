# Capture retention

`Cleaner` is a managed service that borrows storage and never closes it.
Construction starts no work. Later application wiring will run it on the admin
target and stop it before closing storage. Cleanup is independent of capture policy.

## Provisional defaults

All values must be positive. Later wiring will expose them under
`profile_dump.cleaner` with `-profile-dump.*` flags.

| Field | Flag suffix | Default |
| --- | --- | --- |
| `retention` | `retention` | 168h |
| `sweep_interval` | `sweep-interval` | 1h |
| `sweep_timeout` | `sweep-timeout` | 1m |
| `max_entries` | `cleanup-max-entries` | 10,000 |

## Deletion and progress

The first pass starts with the service. Later passes wait `sweep_interval` after
completion. Deletion requires a strict `ParseObjectKey` match in the listed day
and ULID time strictly before `now - retention`. Equality and malformed entries
are retained. The cleaner never reads payloads, normalizes paths, or deletes
outside `profile-debug-dumps/`. See [FORMAT.md](FORMAT.md) for key rules.

Traversal is sequential, with at most five frames and iterator goroutines for
namespace, tenant, year, month, and day. Entry/time budgets pause traversal while
keeping provider iterators open for progress without replay. The initial expiry
horizon stays fixed across passes, except for clock rollback. A watchdog times
provider fetches only, excluding parked callbacks. New objects behind an iterator
wait for the next round.

Listing failures skip the subtree, and failed deletes advance. Both retry next
round. Concurrent deletion and provider-recognized not-found responses are harmless.
Retention is eventual, without a deletion SLA. Provider lifecycle rules are optional.

## Provider limits

Shutdown cancels and joins the cleaner's iterators. The pinned S3 client can still
retain a listing producer after cancellation during a partial pass. Swift ignores
listing/deletion cancellation, potentially exceeding budgets and delaying shutdown.
SDK paging and allocations remain provider-owned. The filesystem provider buffers
whole directories. These limitations remain deferred.

## Metrics

Metrics use the `pyroscope_profile_dump_cleanup_` prefix without tenant or key labels.

| Suffix | Meaning |
| --- | --- |
| `deleted_total` | Successful deletes, excluding recognized not-found errors. Not a unique-object count across replicas. |
| `errors_total{operation}` | List/delete failures and timeouts. Timeouts also count under their operation. Excludes shutdown cancellation and budget exhaustion. |
| `malformed_total` | Invalid entries encountered, including repeat visits. Pruned descendants are not counted. |
| `passes_total{result}` | Complete, partial, error, or canceled. Earlier traversal errors persist across its passes. |
| `last_success_timestamp_seconds` | Last full traversal without storage errors. Partial passes do not advance it. Zero means none completed. |

Provider iteration duration includes parked time, so it is not request latency.
