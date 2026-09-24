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
can delay it indefinitely. Application lifecycle ordering is outside the recorder,
which cannot prevent another bucket holder from closing shared storage early.
