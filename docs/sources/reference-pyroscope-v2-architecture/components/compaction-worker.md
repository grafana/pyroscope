---
title: "Pyroscope v2 compaction-worker"
menuTitle: "Compaction-worker"
description: "The compaction-worker merges small segments into larger blocks."
weight: 40
keywords:
  - Pyroscope v2
  - compaction-worker
  - compaction
  - blocks
---

# Pyroscope v2 compaction-worker

The compaction-worker is a stateless component responsible for merging small segments into larger blocks. This improves query performance by reducing the number of objects that need to be read from object storage.

## Why compaction is needed

The ingestion pipeline creates many small segments—potentially millions of objects per hour at scale. Without compaction, this leads to:

- **Read amplification**: Queries must fetch many small objects
- **Increased costs**: More API calls to object storage
- **Metadata bloat**: The metastore index grows unboundedly
- **Performance degradation**: Both read and write paths slow down

## How it works

1. **Job polling**: Workers poll the [metastore](../metastore/) for available compaction jobs.
1. **Segment download**: Workers download source segments from object storage.
1. **Merge operation**: Matching datasets from different segments are merged.
1. **Block upload**: The compacted block is uploaded to object storage.
1. **Status report**: Workers report job completion to the metastore.

## Compaction speed

Compaction workers compact data as soon as possible after it's written to object storage:

- **Median time to first compaction**: Less than 15 seconds
- **Continuous operation**: Workers constantly poll for new jobs

This ensures that query performance remains optimal even during high ingestion rates.

## Job scheduling

Compaction jobs are coordinated by the metastore, which:

- Creates jobs when enough segments are available for compaction
- Assigns jobs to workers based on available capacity
- Tracks job progress and handles failures
- Uses a "Small Job First" strategy to prioritize smaller blocks

Workers specify their available capacity when polling for jobs, allowing the system to adapt to the available resources.

## Data layout

Profiling data from each service (identified by the `service_name` label) is stored as a separate dataset within a block. During compaction:

- Matching datasets from different blocks are merged
- TSDB indexes are combined
- Symbols and profile tables are merged and rewritten

The output block contains non-overlapping, independent datasets optimized for efficient reading.

## Stateless design

Compaction workers are completely stateless:

- Require no persistent local storage
- Scale horizontally by adding more instances
- Allow instances to be added or removed at any time
- Use default concurrency based on available CPU cores

## Fault tolerance

If a compaction worker fails:

- The job lease expires
- The metastore reassigns the job to another worker
- Source segments remain in object storage until compaction succeeds

Jobs that repeatedly fail are deprioritized to prevent blocking the compaction queue.

For detailed information about the compaction process, refer to [Compaction](../../compaction/).
