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

## Configuration

The segment-writer flush behavior can be configured to balance between:

- **Latency**: How quickly data becomes queryable
- **Cost**: Number of write operations to object storage
- **Memory usage**: Amount of data held in memory before flush
