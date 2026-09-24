# Capture retention

Run an **admin target** against the same customer bucket and `storage.prefix` as
capturing distributors. Distributor-only deployments do not run cleanup. The
cleaner starts with the admin service and borrows its storage client.

Configure retention and pass limits under `profile_dump`. The provisional defaults
are `retention: 168h`, `sweep_interval: 1h`, `sweep_timeout: 1m` and
`max_entries: 10000`. All must be positive. See the
[configuration reference](../../docs/sources/configure-server/reference-configuration-parameters/index.md)
for flags and descriptions.

## Deletion and progress

Cleanup visits only `profile-debug-dumps/`, relative to `storage.prefix`. It deletes
valid capture keys strictly older than the retention cutoff, using server capture
time rather than client profile timestamps. Malformed entries and unrelated
objects are preserved. A real V1 tenant named `profile-debug-dumps` remains
available through its normal `phlaredb/` storage.

The first pass starts immediately. Partial passes resume after `sweep_interval`,
retaining their listing position and initial cutoff. Clock rollback can lower that
cutoff. Failed objects and subtrees wait for the next traversal, allowing later
tenants to progress. Restarts lose the cursor, and objects uploaded behind it may
wait for another traversal.

Retention makes objects eligible for deletion, without guaranteeing a deletion
deadline. Cleanup continues after policy removal or expiry. Those policy changes
stop new admissions while already admitted captures drain normally. Multiple admin
replicas may safely race, with duplicate listing work.

## Resource limits and shutdown

The cleaner retains at most five partition iterators and performs one deletion at
a time. `max_entries` bounds processed listing entries per pass. Provider pages,
prefetch and cancellation draining are outside that budget. The filesystem
provider reads whole directories, so these limits do not bound total process memory.

Timeouts require provider cooperation. In particular, Swift ignores cancellation
for listing and deletion, so requests can exceed pass budgets and delay shutdown
indefinitely. Storage stays open until cleanup and recorder uploads release it.

## Monitoring

Metrics use the `pyroscope_profile_dump_cleanup_` prefix. Watch
`last_success_timestamp_seconds`, `last_success_cutoff_timestamp_seconds` and
`errors_total` for retention lag. Success gauges update only after a full traversal
without storage errors. Partial passes never count as successful coverage, and
the cutoff cannot account for uploads that appeared behind the cursor.

`passes_total` distinguishes complete, partial, error and canceled passes.
`deleted_total` and `missing_total` distinguish successful deletes from recognized
not-found results. `malformed_total` counts preserved invalid entries and may count
them again on later traversals.

See [recorder contracts](RECORDER.md) for admission and upload behavior, and the
[CLI reference](../../cmd/profilecli/PROFILE_DUMP.md) for retrieval. Extracted
profiles remain unsanitized raw customer data.
