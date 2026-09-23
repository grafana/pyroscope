# Shared recorder

`Recorder` captures native inputs using injected policies and uploads. Application
wiring, runtime overrides, ingestion hooks, and CLI integration follow later.
See [FORMAT.md](FORMAT.md) for envelope and representation rules.

## Admission and ownership

`Capture` checks policy expiry, selectors, probability, and local rates before
sizing and reserving memory. Connect and ingest evaluate selectors. OTLP bypasses
them. Enqueue never waits, and the returned outcome does not establish persistence
or determine ingestion success. `PolicyActive` is an allocation-free hint without
metrics. `Capture` always rechecks the policy.

Keep borrowed metadata, labels, and payload stable until `Capture` returns.
`Payload.Size` must count payload and all serialization scratch without cloning.
`Write` must honor cancellation, use only supplied buffers, report the actual byte
count, and neither retain buffers, mutate input, nor launch goroutines. Errors,
size mismatches, and payload/span-start panics drop capture. Span-finish panics
leave the outcome unchanged. Adapter allocation and cancellation behavior still
require testing.

Reservations cover the complete envelope, declared scratch, and 260 KiB of
encoding/control overhead, plus 128 KiB for HTTP metadata. The entire reservation
stays charged through upload. These limits cover recorder buffers, excluding
process RSS and storage/exporter internals. Workers retain only owned bytes,
tenant, key, source, and trace context.

Rate tokens are not refunded after later rejection, including a process-rate
rejection after tenant admission. Policy updates preserve token history. Removed
or expired tenant limiters are pruned periodically and at capacity. Expiry stops
new admissions while accepted captures continue.

## Provisional defaults

These controls apply per recorder, with no fleet or cumulative-volume quota.
Later wiring will register `profile_dump.recorder` and `-profile-dump.*` flags.

| Control | Default |
| --- | --- |
| Complete object / reserved bytes | 16 MiB / 64 MiB |
| Queue items / workers | 16 / 2 |
| Tenant rate / burst / rate ceiling | 1/s / 1 / 10/s |
| Process rate / burst | 10/s / 2 |
| Upload timeout / shutdown drain | 10s / 15s |
| Tenant limiters / prune interval | 1,024 / 1m |

## Upload and shutdown

`UploadFunc` receives the tenant and full capture key. Wiring must apply tenant
SSE without adding another tenant prefix or closing borrowed storage. Uploads
use recorder cancellation and timeouts, independent of request cancellation.

`Shutdown` stops admission and drains until its caller or drain deadline, then
cancels work and discards queued items. Active callbacks retain their reservations
until they return. Wait for `Done()` before closing storage. Uncooperative callbacks
can delay final release beyond `Shutdown`.

## Observability

`Profile.Capture` spans cover preparation through enqueue/drop after selection and
rate admission. They do not force sampling or measure uploads. Metrics use bounded
source/result/reason labels. Reserved bytes include preparation and uploads,
while queue gauges count waiting objects. Enqueued, dropped, and uploaded byte
counters overlap. Shutdown discards count as drops, and upload timeouts may still
leave persisted objects. See [CLEANER.md](CLEANER.md) for cleanup telemetry.
