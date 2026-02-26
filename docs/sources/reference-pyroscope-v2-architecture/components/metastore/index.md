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

Follower replicas can serve read requests, distributing the query load across the cluster.
