# Recorder contracts

`Capture` returns enqueue or drop status. Upload failures are recorded separately.
Each admission checks the current policy's deadline, selector, probability and
local tenant/process rates. Each sample uses ordinary `Capture` with its series
labels, evaluating selection independently. Selection cost is not bounded for
arbitrary labels or selectors. Accepted captures drain even after policy removal or
expiry. Rate tokens are not refunded on later rejection, and limits provide no
fleet quota or successful-capture fairness.

## Activation

Use the existing per-tenant runtime overrides file. Process limits alone do not
activate capture, and `profile_debug_dump` cannot be set in global `limits`.

```yaml
overrides:
  "3648":
    profile_debug_dump:
      active_until: "2026-09-25T18:30:00Z"
      selector: '{service_name="checkout"}'
      probability: 0.01
      max_captures_per_second: 1
```

Replace the illustrative timestamp with an explicit absolute deadline. An absent or
null block disables capture. A valid expired policy is inactive. A present block
requires `active_until` and a finite `probability` in `(0, 1]`. The selector defaults
to `{}` and matches external Connect series labels before relabeling or profile parsing.
Selectors choose whole captures and do not redact their contents.

Strict configuration validation is unchanged. Invalid semantic or structural
settings reject the entire candidate runtime load. A failed reload keeps the last
valid snapshot, whose absolute deadline still expires. Initial load failures retain
the existing runtime manager startup behavior. Reloads and restarts do not renew
deadlines. Removing or expiring a policy stops new admissions after local propagation
without revoking queued captures or deleting stored files.

## Process controls and internal bounds

The public YAML settings under `profile_dump` are:

| Setting | Default | Meaning |
| --- | --- | --- |
| `max_object_bytes` | `16777216` (16 MiB) | Combined payload and serialized JSON size |
| `max_retained_bytes` | `67108864` (64 MiB) | Admitted capture-buffer budget |
| `process_captures_per_second` | `10` | Aggregate admission rate per distributor |
| `upload_timeout` | `10s` | One cooperative deadline for both uploads |
| `retention` | `168h` (seven days) | Configurable retention for the admin cleaner |

Flags use `profile-dump.` followed by the hyphenated setting name. Byte limits must
be positive, the maximum capture must fit an `int`, and retained bytes must cover
that maximum plus 4096 bytes of item overhead. Rates must be finite and positive.
Durations must be positive. See the ordinary generated
[configuration reference](../../docs/sources/configure-server/reference-configuration-parameters/index.md).

An omitted tenant `max_captures_per_second` defaults to
`min(1, process_captures_per_second)`. An explicit value must be finite and positive.
It may exceed the process rate, while actual admissions remain constrained by
both the tenant limiter and the independent process limiter. Rates are local to
each distributor.

Internal bounds are a queue of 16, two workers, tenant burst 1, process burst 2,
1024 tenant limiters, and a 15-second shutdown drain. Metadata has a 64 KiB bound
and field limits described in [FORMAT.md](FORMAT.md). These are implementation
bounds, not additional flags or YAML fields.

## Memory and ownership

Candidate metadata, labels and payload are borrowed until Capture returns.
Capture preparation validates metadata once against its constructed payload key
before allocating JSON, then marshals it and checks the serialized 64 KiB bound.
Invalid metadata drops as `invalid`, while serialized or combined capture size
failures drop as `too_large`. The sidecar name comes from the constructed payload
key. Public metadata helpers retain validation for their callers. Bounded JSON
is reused for sizing and upload:

```text
capture bytes = len(payload) + len(metadataJSON)
reserved bytes = len(payload) + cap(metadataJSON) + 4096 bytes of item overhead
```

The reservation covers owned payload and JSON buffers, plus
bounded key, tenant and item bookkeeping. It precedes payload allocation and
the direct payload copy. Successful enqueue transfers the reservation to the
queue and rejected enqueue releases it in the producer. Workers release their
reservations after both buffers are no longer in use.

The full reservation stays charged through upload or terminal drop. This is a
capture-buffer budget, not an RSS bound. The bounded initial metadata marshal occurs before byte admission. Queue
and worker structures, capped limiter state, metrics, allocator/GC behavior and
provider-internal buffers have separate lifetimes.

## Uploads and shutdown

Workers retain owned payload and JSON bytes and trace span context, without
retaining the request context or borrowed candidate buffers. Upload contexts derive from the
recorder lifecycle with one timeout covering both sequential uploads. Request
cancellation does not cancel accepted work. `BucketUpload` applies current tenant SSE settings to the complete
capture key and leaves the shared bucket open. The recorder uploads the unchanged
`.pprof` payload first, then the `.json` sidecar using the native key helpers. Payload failure or cancellation prevents a
sidecar attempt. The recorder does not retry uploads or roll back incomplete pairs.

`uploads_total{result="success"}` counts captures only after both calls succeed
within the capture deadline. `bytes_total{result="uploaded"}` counts the combined
payload and JSON lengths of those complete captures. Partial or uncertain uploads
contribute zero uploaded bytes even if storage accepted one or both objects, so
this counter does not measure network traffic or all bytes left in storage.
Enqueued, dropped, and queued byte measurements also use the combined lengths.
Reservation measurements include JSON capacity and item overhead.

Constructors only validate and allocate. Start the recorder as a managed service
before capturing. Capture before start or after service cancellation drops.
Stopping closes the queue under the enqueue lock and allows an internal 15-second
graceful drain before canceling uploads. Workers are the only queue consumers.
Service termination waits for all preparations and workers to release their
reservations. A provider that ignores cancellation can retain bounded queued
buffers and delay completion until it returns.

The application owns the shared bucket and gives components borrowed views. It
closes storage only after recorder and cleaner termination, including after
partial initialization failure. A timeout while awaiting termination does not
establish that storage is no longer in use.

The capped tenant limiter map preserves active token history and prunes inactive
entries only when a new tenant needs capacity. There is no periodic maintenance.
Shutdown clears the map.

A recorder runs in each distributor process with customer storage. An admin target
must run retention cleanup against the same bucket and storage prefix.
See [Cleaner contracts](CLEANER.md).

Process crashes or OOM can lose pending captures. There is no capture recovery or
delivery guarantee. Retrieval uses `profilecli admin profile-dumps` as described in
the [CLI reference](../../cmd/profilecli/PROFILE_DUMP.md).

## Monitoring

The overrides exporter exposes `pyroscope_limits_overrides` with
`limit_name="profile_debug_dump_active_until_timestamp_seconds"` and
`limit_name="profile_debug_dump_active"` for configured tenant overrides. Active
status is evaluated at scrape time, so expiry needs no reload. An expired policy
keeps its configured deadline in the exporter. Absent or null policies export zero
for both values. A rejected reload
leaves the last valid deadline in effect. These values have no global-default series.

Recorder metrics under `pyroscope_profile_dump_` are process aggregates without
tenant labels. Monitor `dropped_total` for local drops and `uploads_total` by
`result` for complete success, error, timeout, and cancellation. An enqueued
`Profile.Capture` span's `capture.object_key` identifies intended storage, not
proof that the pair has persisted. Payloads and metadata remain raw customer data.
