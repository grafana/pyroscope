# Capture retention

Run an **admin target** against the same bucket and `storage.prefix` as capturing
distributors. Distributor-only deployments do not run cleanup. The cleaner uses
`NewBorrowedBucket`, leaving storage ownership with the application.

Set positive `profile_dump.retention` or `-profile-dump.retention`. The default is
seven days (`168h`), with no maximum. See the
[configuration reference](../../docs/sources/configure-server/reference-configuration-parameters/index.md).

## Cleanup

A sequential sweep starts with the service, fixes its cutoff at `now - retention`,
and visits `profile-debug-dumps/native/` under the configured storage prefix.
Only fully expired hours are eligible. Hourly granularity adds less than an hour
before eligibility, followed by scheduling, traversal, or outage delays.

The next sweep starts **one hour after completion**. This interval is independent
of retention. Deletion has no exact deadline and continues after policy expiry.
Each sweep starts from the namespace root, allowing later sweeps to find late arrivals.

Payloads and sidecars are deleted independently, including orphans, without reading
contents. Every key must match the native namespace and its ULID/minute partition.
Unexpected keys and unrelated data remain untouched. Cleanup never deletes broad
prefixes. Missing objects are harmless. Other storage errors are recorded while
reachable siblings continue.

## Shutdown

Deletes run one at a time with a ten-second cooperative timeout. Cancellation stops
new deletes and allows listing producers to drain. Service completion waits for
provider calls, so providers that ignore cancellation can delay shutdown indefinitely.
Shared storage stays open until recorder and cleaner termination. Provider buffers
are outside cleaner bounds. The separate CLI listing teardown limitation remains deferred.

## Monitoring

Metrics use the `pyroscope_profile_dump_cleanup_` prefix:

- `deleted_total`: successful deletes, excluding recognized missing responses.
- `errors_total{operation="list"|"delete"}`: storage failures, excluding shutdown cancellation.
- `last_success_timestamp_seconds`: completion of a sweep without storage errors.

Skipped unrecognized keys do not prevent success. Listings are not snapshots.

See [recorder contracts](RECORDER.md), [native format](FORMAT.md), and the
[CLI reference](../../cmd/profilecli/PROFILE_DUMP.md) for activation and retrieval.
