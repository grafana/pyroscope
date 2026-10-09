---
title: "Compaction"
menuTitle: "Compaction"
description: "Learn how Pyroscope v2 compacts segments into optimized blocks."
weight: 60
keywords:
  - Pyroscope v2
  - compaction
  - blocks
  - segments
---

# Compaction

Compaction is the process of merging multiple small segments into larger, optimized blocks. This is essential for maintaining query performance and controlling metadata index size.

## Why compaction matters

The ingestion pipeline creates many small segments—potentially millions of objects per hour at scale. Without compaction:

- **Read amplification**: Queries must fetch many small objects
- **API costs**: More calls to object storage
- **Metadata bloat**: The metastore index grows unboundedly
- **Performance degradation**: Impacts both read and write paths

## How it works

Compaction in Pyroscope v2 is coordinated by the [metastore](../components/metastore/) and executed by [compaction-workers](../components/compaction-worker/).

{{< mermaid >}}
sequenceDiagram
    participant W as Compaction Worker
    participant M as Metastore
    participant S as Object Storage

    loop Continuous
        W->>M: Poll for jobs
        M->>W: Assign job with source blocks
        W->>S: Download source segments
        W->>W: Merge segments into block
        W->>S: Upload compacted block
        W->>M: Report completion
        M->>M: Update metadata index
    end
{{< /mermaid >}}

## Compaction service

The compaction service runs within the metastore and is responsible for:

- **Job planning**: Creating compaction jobs when enough segments are available
- **Job scheduling**: Assigning jobs to workers based on capacity
- **Job tracking**: Monitoring progress and handling failures
- **Index updates**: Replacing source block entries with compacted block entries

### Raft consistency

The compaction service relies on Raft to guarantee consistency:

1. **Plan preparation**: The leader prepares job state changes (read-only).
1. **Plan proposal**: Changes are committed to the Raft log.
1. **State update**: All replicas apply the changes atomically.

This ensures all replicas maintain consistent views of compaction state.

## Job planner

The job planner maintains a queue of blocks eligible for compaction:

- **Queue structure**: FIFO queue, segmented by tenant, shard, and level
- **Job creation**: Jobs are created when enough blocks are queued
- **Boundaries**: Compaction never crosses tenant, shard, or level boundaries

### Data layout

Profiling data from each service is stored as a separate dataset within a block. During compaction:

- Matching datasets from source blocks are merged
- TSDB indexes are combined
- Symbols and profile tables are merged and rewritten
- Output block contains optimized, non-overlapping datasets

### Maximum compaction level

By default, compaction produces blocks through level 3 (L3). Set
`-metastore.max-compaction-level=4`, or configure:

```yaml
metastore:
  max_compaction_level: 4
```

The setting is the maximum **output** level and must be at least 3. A value of
4 enables L3 to L4; 5 also enables L4 to L5, and higher values add further
steps without another binary change. Blocks at the configured maximum are
terminal for new compaction jobs.

Each additional step groups up to 10 blocks from the same tenant, shard, and
source level. The oldest batch becomes eligible for an incomplete job after
24 hours as the queue advances. Source block IDs may span at most 24 hours;
the data inside those blocks can cover a longer period. All additional levels
reuse this final batching policy; the original levels retain their limits.

Only newly completed blocks below the leader's configured maximum are admitted
to the compaction queue. Raising the maximum does not backfill existing terminal
blocks. Larger jobs need more worker memory, temporary disk, and I/O; monitor
job duration, failures, and queue depth when raising the maximum.

#### Rollout and rollback

Set the desired maximum in the same rollout that installs this feature. No
second configuration rollout, worker pause, or simultaneous restart is needed.
Until the replicated FSM version `ConfigurableCompactionLevels` (version 2)
activates, newly completed L3 blocks remain terminal, even when the local
configuration specifies a higher maximum.

The FSM version activator waits for every Raft member, including non-voters, to
report support before committing the activation through Raft. This ensures all
replicas have higher-level queue restoration and read support before new blocks
are admitted above L2. After activation, the leader includes its configured
maximum in compaction plans. Replicas apply that numeric limit independently of
local settings, so subsequent increases and decreases use ordinary rolling
configuration updates. During a rollout, leadership changes can temporarily
change the effective maximum until settings agree.

Use `-metastore.fsm-version.activation-delay` for a rollback window, or pin
`-metastore.fsm-version.max-version=1` on all replicas to hold activation at the
baseline. While version 2 is inactive, disabling automatic activation or capping
it below 2 keeps the effective compaction maximum at 3. The active FSM version is persisted in snapshots
and reproduced at the same point during log replay; entries prepared before
activation retain the historical admission policy when replayed later.

After activation, lowering `max_compaction_level` to 3 stops planning new
higher-level jobs. Existing jobs can still finish above the new maximum, queued
candidates at or above it remain available for a future increase, and existing
higher-level blocks remain readable. The FSM version itself never decreases:
a version-aware binary that does not support version 2 refuses to restore the
activated state. Binaries predating FSM versioning must not rejoin after
activation. Refer to [metastore FSM versioning](../components/metastore/#upgrades-and-rollbacks)
for the activation controls.

Metadata and label queries select shards by their persisted data time bounds
across all partitions, independently of the configured maximum. They no longer
assume that a block's data lies within 24 hours of its inherited ID timestamp.
This adds a scan of shard summaries across retained partitions, while full
metadata is loaded only for shards whose data overlaps the query.

## Job scheduler

The scheduler uses a **Small Job First** strategy:

1. Lower-level blocks are prioritized (smaller, affect read amplification more).
1. Within a level, unassigned jobs are processed first.
1. Jobs with fewer failures are prioritized.
1. Jobs with earlier lease expiration are considered first.

### Adaptive capacity

Workers specify available capacity when polling for jobs. The scheduler:

- Creates jobs based on reported worker capacity
- Balances queue size with worker utilization
- Adapts to available resources automatically

## Job ownership

Jobs are assigned using a lease-based model:

- **Lease duration**: Workers are granted ownership for a limited time
- **Fencing tokens**: Raft log index serves as a unique token
- **Lease refresh**: Workers must refresh leases before expiration
- **Reassignment**: Expired leases allow job reassignment

### Failure handling

When a worker fails:

1. The job lease expires.
1. The metastore detects the expired lease.
1. The job is reassigned to another worker.
1. Source blocks remain until compaction succeeds.

Jobs that repeatedly fail are deprioritized to prevent blocking the queue.

## Job status lifecycle

{{< mermaid >}}
stateDiagram-v2
    [*] --> Unassigned : Create Job
    Unassigned --> InProgress : Assign Job
    InProgress --> Success : Job Completed
    InProgress --> LeaseExpired: Job Lease Expires
    LeaseExpired: Abandoned Job

    LeaseExpired --> Excluded: Failure Threshold Exceeded
    Excluded: Faulty Job

    Success --> [*] : Remove Job from Schedule
    LeaseExpired --> InProgress : Reassign Job
{{< /mermaid >}}

## Performance characteristics

- **Median time to first compaction**: Less than 15 seconds
- **Continuous operation**: Workers constantly poll for new jobs
- **Horizontal scaling**: Add more workers to handle compaction backlog
- **Priority-based**: Smaller blocks compacted first for fastest impact

## Block deletion

After successful compaction:

1. **Tombstone creation**: Source blocks are marked for deletion.
1. **Delay period**: Blocks are retained to allow in-flight queries to complete.
1. **Hard deletion**: After the delay, source blocks are removed from storage.

This two-phase deletion prevents query failures during compaction.

## Implementation details

For detailed implementation information, including job scheduling algorithms and lease management, refer to the [internal documentation](https://github.com/grafana/pyroscope/blob/main/pkg/metastore/compaction/README.md).
