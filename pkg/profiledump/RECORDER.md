# Recorder contracts

`Capture` returns enqueue or drop status. Upload failures are recorded separately.
Each admission checks the current policy's deadline, selector, probability and
local tenant/process rates. Accepted captures drain even after policy removal or
expiry. Rate tokens are not refunded on later rejection, and limits provide no
fleet quota or successful-capture fairness.

## Memory and ownership

Candidate metadata, labels and payload are borrowed until Capture returns.
Bounded metadata is marshaled once and reused for sizing and encoding:

```text
object bytes = HeaderSize + len(metadataJSON) + len(payload)
reserved bytes = object bytes + cap(metadataJSON) + 4096 bytes of item overhead
```

The reservation covers simultaneous metadata and final-object buffers, plus
bounded key, tenant and item bookkeeping. It precedes final-object allocation and
the direct payload copy. Successful enqueue transfers the reservation to the
queue. Rejected enqueue releases it in the producer. Workers and shutdown drains
release their reservations after the object is no longer in use.

The full reservation stays charged through upload or terminal drop, including the
metadata allowance after encoding. This is a capture-buffer budget, not an RSS
bound. The bounded initial metadata marshal occurs before byte admission. Queue
and worker structures, capped limiter state, metrics, allocator/GC behavior and
provider-internal buffers have separate lifetimes.

## Uploads and shutdown

Workers retain owned object bytes and trace span context, without retaining the
request context or borrowed candidate buffers. Upload contexts derive from the
recorder lifecycle with a per-upload timeout. Request cancellation does not cancel
accepted work. `BucketUpload` applies current tenant SSE settings to the complete
capture key and leaves the shared bucket open. The recorder does not retry uploads.

Shutdown closes admission and the queue under the enqueue lock, then permits a
graceful drain. When the caller or drain deadline expires, it cancels uploads and
discards queued items. Active upload reservations remain charged until those calls
return. `Done` closes after all preparations, workers and reservations are gone.

A return from `Shutdown` alone does not establish that storage is no longer in use.
Shared-storage teardown depends on `Done`. A provider that ignores cancellation
can delay it indefinitely. The application owns the shared bucket and gives components borrowed views whose
`Close` does not close storage. Its recorder service waits for `Done` before
terminating. The storage service stops after its dependents, waits for recorder
`Done` again, and closes the underlying bucket once. `Run` repeats this cleanup on
partial initialization failure, including constructors that fail before services
start. A provider ignoring cancellation can therefore delay process shutdown
indefinitely. This preserves worker ownership of storage and retained buffers.

One recorder is constructed for each distributor process with customer storage.
The module depends on storage and runtime overrides, and the distributor depends
on it. A legacy monolith without customer storage has no recorder. Recorder
configuration shares `profile_dump` and the existing application registry. The admin target constructs a cleaner that borrows the same storage client.
Storage teardown waits for its listing and deletion calls as well as recorder
`Done`. See [Cleaner contracts](CLEANER.md).
