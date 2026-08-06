---
title: "Pyroscope v2 segment-writer"
menuTitle: "Segment-writer"
description: "The segment-writer accumulates profiles and writes them to object storage."
weight: 20
keywords:
  - Pyroscope v2
  - segment-writer
  - ingestion
  - object storage
---

# Pyroscope v2 segment-writer

The segment-writer is a stateless component that accumulates incoming profiles in memory and periodically writes them to object storage as segments. This is a new component in v2 that replaces the ingester's role in the write path.

## How it works

1. **Profile accumulation**: The segment-writer receives profiles from [distributors](../distributor/) and accumulates them in memory.
1. **Segment creation**: Profiles are batched into small blocks called segments.
1. **Object storage write**: Segments are written directly to object storage.
1. **Metadata update**: The segment-writer updates the [metastore](../metastore/) with metadata about newly created segments.

## Key features

### Single object per shard

Each segment-writer produces a single object per shard containing data from all tenant services assigned to that shard. This approach minimizes the number of write operations to object storage, significantly reducing costs compared to writing individual objects for each tenant or service.

### Synchronous ingestion

Ingestion clients are blocked until data is durably stored in object storage and an entry for the object is created in the metadata index. This guarantees data durability without requiring local disk persistence.

By default, ingestion is synchronous with median latency expected to be less than 500ms using default settings and popular object storage providers such as Amazon S3, Google Cloud Storage, and Azure Blob Storage.

### In-memory accumulation

Profiles are accumulated in an in-memory database before being flushed to object storage. The in-memory structure includes:

- **Profile index**: Efficient indexing for accumulated profiles
- **Inverted index**: For label-based lookups during segment creation

### Data co-location

Profiles from the same application (identified by the `service_name` label) are co-located in the same segments. This co-location is maintained by the distributor's routing algorithm and is crucial for:

- Improves query performance
- Increases compaction efficiency
- Optimizes storage usage

## Stateless design

Unlike the v1 ingester which required local disk storage, the segment-writer is completely stateless:

- Requires no persistent local storage
- Writes all data directly to object storage
- Scales horizontally by adding more instances
- Allows instances to be added or removed without data migration
- Recovers immediately after failure (no WAL replay needed)

## Segment lifecycle

1. **Creation**: Profiles are accumulated in memory.
1. **Flush**: When conditions are met (time or size thresholds), a segment is written to object storage.
1. **Registration**: Segment metadata is registered in the metastore.
1. **Compaction**: Small segments are later merged into larger blocks by [compaction-workers](../compaction-worker/).

## Failure handling

The segment writer relies on at-least-once delivery semantics. If a write fails after the segment has been uploaded to object storage but before the metastore acknowledges the metadata, the client retries the request. This can result in the same profile appearing in multiple segments, which is resolved during compaction.

### Dead letter queue

If the segment writer cannot register metadata with the metastore (for example, during metastore unavailability), the metadata is written to a dead letter queue (DLQ) directory in object storage. The metastore recovers these entries in the background, ensuring that data is eventually made visible to queries.

## Deployment

- The segment writer runs as a StatefulSet without persistent volumes.
- It participates in a hash ring used by the [distributor](../distributor/) to route profiles.
- Multiple segment writers can write to the same shard during topology changes (for example, scaling events or rollouts). This is expected and handled by compaction.

## Configuration

The segment-writer flush behavior can be configured to balance between:

- **Latency**: How quickly data becomes queryable
- **Cost**: Number of write operations to object storage
- **Memory usage**: Amount of data held in memory before flush

### Reduce object storage costs

Because each flush results in write operations to object storage (and each subsequent query results in read operations), the flush frequency and the number of objects produced per flush directly affect your object storage bill. Use the following levers to reduce `PUT` and `GET` costs:

- **Increase the segment duration**: `-segment-writer.segment-duration` (default `500ms`) controls how long profiles accumulate in memory before a segment is flushed. Increasing it produces fewer, larger segments and therefore fewer write operations.

  {{< admonition type="warning" >}}
  With the default synchronous ingestion, the segment duration must stay within the ingestion client's request timeout. Clients block until the segment is flushed, so a segment duration close to or exceeding the client timeout causes rejected profiles with errors such as `context deadline exceeded` and `context canceled`, and `422` responses on `/ingest`. Raise the segment duration gradually and keep it comfortably below the client timeout.
  {{< /admonition >}}

- **Reduce shards per segment-writer**: `-segment-writer.num-tokens` (default `4`) controls how many shards each segment-writer owns. Each shard produces its own object per flush, so lowering this value (down to `1`) reduces the number of objects written per flush cycle, and consequently the number of read operations at query time.

- **Account for replicas**: The number of segment-writer replicas scales the volume of requests to object storage roughly linearly. More replicas means more objects and more requests.

[Compaction-workers](../compaction-worker/) further reduce costs over time by merging small segments into larger blocks, which lowers the object count and read amplification during queries.

### Trade consistency for headroom

By default, ingestion is synchronous: clients wait until profiles are durably stored and indexed, which provides read-after-write consistency but bounds how large `-segment-writer.segment-duration` can be.

If read-after-write consistency and synchronous durability confirmation are not required, set `-async-ingest=true`. The write path then responds without waiting for the segment-writer to flush and index the profile, which allows a larger segment duration (and therefore lower object storage costs) without triggering client timeouts. The trade-off is that a successful ingestion response no longer guarantees the profile is durable and immediately queryable.
