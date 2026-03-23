---
title: "Design motivation"
menuTitle: "Design motivation"
description: "Learn about the v1 limitations that motivated the Pyroscope v2 architecture redesign."
weight: 5
keywords:
  - Pyroscope v2
  - v1 limitations
  - architecture
---

# Design motivation

The v2 architecture addresses fundamental scalability and resilience limitations in v1 that cannot be resolved incrementally.

## Write path limitations in v1

- **No write-ahead log (WAL)**: Ingesters accumulate profiles in memory and periodically flush them to disk, but there is no WAL to durably record writes on arrival. If an ingester crashes between flushes, the in-memory profiles are lost. Replication mitigates this but cannot fully prevent data loss when multiple ingesters fail.
- **Deduplication overhead**: In v1, each profile series is replicated to N ingesters, and each ingester writes its own block. At query time, these duplicates have to be merged and deduplicated. This becomes increasingly expensive as the number of ingesters grows.
- **Weak read/write isolation**: Ingestion latency spikes can cause distributor out-of-memory (OOM) errors. Expensive queries can increase ingestion latency due to broad locks, and can themselves cause ingester OOM.
- **Suboptimal data distribution**: The label-hash-based sharding distributes profiles of the same service across all ingesters, causing excessive duplication of symbolic information and reducing query selectivity.
- **Slow rollouts**: Ingester rollouts can take hours in large deployments due to the need to flush in-memory data before shutdown.

## Read path limitations in v1

- **Store-gateway instability**: Heavy queries can cause store-gateway OOM. The block index overhead grows with the number of blocks, putting memory pressure on store-gateways.
- **Limited elasticity**: The querier and store-gateway services are difficult to scale dynamically, as store-gateways need to load block indexes before serving queries.
- **Slow rollouts**: Like ingesters, store-gateway rollouts can be slow due to the need to load block indexes on startup.

## Compaction limitations in v1

- **Scalability**: The v1 compactor can struggle to keep up with large tenants as data is replicated during ingestion. Delays in compaction place pressure on the read path, as queries have to process and deduplicate more uncompacted blocks.

## Extensibility

- Adding a new data access method (for example, a new API endpoint for heatmaps) in v1 requires changes across many components. The tight coupling between ingesters, store-gateways, and queriers makes the codebase harder to maintain and extend.

## Comparison with v1

| Aspect          | v1                                                                                   | v2                                                                  |
|-----------------|--------------------------------------------------------------------------------------|---------------------------------------------------------------------|
| **Write path**  | Distributor &rarr; Ingester &rarr; Object Storage                                    | Distributor &rarr; Segment writer &rarr; Object storage + Metastore |
| **Metadata**    | Per-tenant bucket index in object storage                                            | Metastore (Raft-based, in-memory index)                             |
| **Read path**   | Query frontend &rarr; Query scheduler &rarr; Querier &rarr; Ingester / Store-gateway | Query frontend &rarr; Metastore + Query backend                     |
| **Compaction**  | Compactor (hash-ring sharded, per-tenant)                                            | Compaction worker orchestrated by metastore                         |
| **Replication** | Write replication to N ingesters                                                     | No write replication; durability via object storage                 |

v2 addresses these issues by eliminating write replication in favor of object storage durability, centralizing metadata in the metastore for fast query planning and enabling stateless query backends that access object storage directly, and decoupling compaction into a more scalable job-based system orchestrated by the metastore.

For details on how the v2 architecture works, refer to [About the Pyroscope v2 architecture](../about-pyroscope-v2-architecture/).
