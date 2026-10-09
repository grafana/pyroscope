---
title: "Pyroscope v2 metastore"
menuTitle: "Metastore"
description: "The metastore maintains the metadata index and coordinates compaction."
weight: 30
keywords:
  - Pyroscope v2
  - metastore
  - Raft
  - metadata
---

# Pyroscope v2 metastore

The metastore is the only stateful component in the Pyroscope v2 architecture. It maintains the metadata index for all data objects stored in object storage and coordinates the compaction process.

## Responsibilities

The metastore service is responsible for:

- **Metadata index**: Maintaining an index of all blocks and segments in object storage
- **Compaction coordination**: Scheduling and coordinating compaction jobs for [compaction-workers](../compaction-worker/)
- **Query planning**: Providing metadata to [query-frontend](../query-frontend/) for locating data objects
- **Data placement**: Managing placement rules for the data distribution algorithm
- **Retention enforcement**: Applying time-based retention policies and generating tombstones for expired data

## Raft consensus

The metastore uses the Raft protocol for consensus and replication, ensuring:

- **Consistency**: All replicas maintain the same view of the metadata
- **High availability**: The cluster can continue operating if some nodes fail
- **Fault tolerance**: Data is replicated across multiple nodes

### Fault tolerance

| Cluster size | Tolerated failures |
|--------------|-------------------|
| 3 nodes      | 1 node            |
| 5 nodes      | 2 nodes           |

## Storage requirements

Even at large scale, the metastore only needs a few gigabytes of disk space for the metadata index. The index is implemented using BoltDB as the underlying key-value store.

For better performance, the index database can be stored on an in-memory volume, as it's recovered from the Raft log and snapshot on startup. Durable storage is not required for the index itself—only for the Raft log.

## Metadata index

The metadata index stores information about data objects (blocks and segments) including:

- Block identifiers (ULID)
- Tenant and shard assignments
- Time ranges
- Dataset information (service names, profile types)

The index is partitioned by time, with each partition covering a 6-hour window. Within each partition, data is organized by tenant and shard.

For detailed information about the metadata index structure, refer to [Metadata index](../../metadata-index/).

## Compaction coordination

The metastore coordinates the compaction process by:

1. **Job planning**: Creates compaction jobs when enough segments are available.
1. **Job scheduling**: Assigns jobs to available compaction-workers.
1. **Job tracking**: Monitors job progress and handles failures.
1. **Index updates**: Updates the metadata index when compaction completes.

The compaction service uses a lease-based ownership model with fencing tokens to prevent conflicts when workers fail or become unresponsive.

For detailed information about the compaction process, refer to [Compaction](../../compaction/).

## Dead letter queue

If the metastore is temporarily unavailable, [segment writers](../segment-writer/) fall back to writing metadata to a dead letter queue (DLQ) directory in object storage. The metastore recovers these entries in the background once it becomes available again.

## Retention

The metastore enforces time-based retention policies on a per-tenant basis. Retention operates at the partition level: entire partitions are removed when they exceed the configured retention period, rather than evaluating individual blocks. When partitions are deleted, tombstones are created for the underlying data objects, which are eventually cleaned up by compaction workers.

## Query support

The metastore provides linearizable reads for query operations, ensuring that:

- Queries observe the most recent committed state
- Previous writes are visible to read operations
- Both leader and follower replicas can serve queries

## Leader election

One metastore instance is elected as the leader through Raft consensus. The leader is responsible for:

- Processing write requests
- Coordinating compaction scheduling
- Enforcing retention policies
- Running cleanup operations
- Recovering metadata entries from the dead letter queue
- Activating new FSM versions

Follower replicas can serve read requests, distributing the query load across the cluster.

## Upgrades and rollbacks

Some releases change how metastore replicas apply the Raft log. To keep replicas consistent while a rolling update is in progress, such changes stay inactive until every metastore replica runs a version that supports them. The leader checks this periodically and then activates the change for the whole cluster through the Raft log. This is called the FSM version.

You can roll back a metastore update as long as the new FSM version hasn't been activated. After activation, replicas running an older version that doesn't support the active FSM version refuse to start and log `unsupported FSM version`. In that case, roll forward instead.

To keep the option to roll back for a while after an update, use these flags:

- `-metastore.fsm-version.activation-delay`: how long every replica must report support for a new FSM version before the leader activates it. The default is `0s`, which activates the version as soon as all replicas support it.
- `-metastore.fsm-version.max-version`: the highest FSM version the leader activates. The default, `0`, means no limit.

Each replica reports its supported and active FSM versions in the `pyroscope_metastore_fsm_version_supported` and `pyroscope_metastore_fsm_version_active` metrics.
