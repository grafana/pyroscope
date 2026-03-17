---
title: "About the Pyroscope architecture"
menuTitle: "About the architecture"
description: "Learn about the Pyroscope architecture."
weight: 10
aliases:
  - /docs/phlare/latest/operators-guide/architecture/about-grafana-phlare-architecture/
  - /docs/phlare/latest/reference-phlare-architecture/about-grafana-phlare-architecture/
---

# About the Pyroscope architecture

Pyroscope has a microservices-based architecture.
The system has multiple horizontally scalable microservices that can run separately and in parallel.
Pyroscope microservices are called components.

Pyroscope's design compiles the code for all components into a single binary.
The `-target` parameter controls which component(s) that single binary will behave as. For those looking for a simple way to get started, Pyroscope can also be run in [monolithic mode](../deployment-modes/#monolithic-mode), with all components running simultaneously in one process.
For more information, refer to [Deployment modes](../deployment-modes/).

## Pyroscope components

Most components are stateless and do not require any data persisted between process restarts. Some components are stateful and rely on non-volatile storage to prevent data loss between process restarts. For details about each component, see its page in [Components](../components/).

### The write path

{{< mermaid >}}
flowchart TD
    A["Writes"]:::highlight --> B["Distributor"]
    B --> C["Ingester"]
    C --> D[("Object storage")]:::highlight
    D --> E["Compactor"]
    E --> D
    classDef highlight fill:#c084fc,color:#000,stroke:#a855f7
{{< /mermaid >}}

Ingesters receive incoming profiles from the distributors.
Each push request belongs to a tenant, and the ingester appends the received profiles to the specific per-tenant Pyroscope database that is stored on the local disk.

The per-tenant Pyroscope database is lazily created in each ingester as soon as the first profiles are received for that tenant.

The in-memory profiles are periodically flushed to disk and new block is created.

For more information, refer to [Ingester](../components/ingester/).

#### Series sharding and replication

By default, each profile series is replicated to three ingesters, and each ingester writes its own block to the long-term storage. The [Compactor](../components/compactor/) merges blocks from multiple ingesters into a single block, and removes duplicate samples. Blocks compaction significantly reduces storage utilization.


### The read path

{{< mermaid >}}
flowchart TD
    A["Reads"]:::highlight --> B["Query frontend"]
    B --> C["Query scheduler"]
    D["Querier"] --> C
    D --> E["Ingester"]
    D --> F["Store-gateway"]
    F --> G[("Object storage")]:::highlight
    classDef highlight fill:#c084fc,color:#000,stroke:#a855f7
{{< /mermaid >}}

Queries coming into Pyroscope arrive at [query-frontend](../components/query-frontend/) component which is responsible for accelerating queries and dispatching them to the [query-scheduler](../components/query-scheduler/).

The [query-scheduler](../components/query-scheduler/) maintains a queue of queries and ensures that each tenant's queries are fairly executed.

The [queriers](../components/querier/) act as workers, pulling queries from the queue in the query-scheduler. The queriers connect to the ingesters to fetch all the data needed to execute a query. For more information about how the query is executed, refer to [querier](../components/querier/).

Depending on the time window selected, the querier involves [ingesters](../components/ingester/) for recent data and [store-gateways](../components/store-gateway/) for data from long-term storage.


## Long-term storage

The Pyroscope storage format is described in detail on the [block format page](../block-format/).
The Pyroscope storage format stores each tenant's profiles into their own on-disk block. Each on-disk block directory contains an index file, a file containing metadata, and the Parquet tables.

Pyroscope requires any of the following object stores for the block files:

[//]: # "TODO: Verify that's correct"

- [Amazon S3](https://aws.amazon.com/s3)
- [Google Cloud Storage](https://cloud.google.com/storage/)
- [Microsoft Azure Storage](https://azure.microsoft.com/en-us/services/storage/)
- [OpenStack Swift](https://wiki.openstack.org/wiki/Swift)
- Local Filesystem (single node only)

For more information, refer to [configure object storage](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-server/storage/configure-object-storage-backend/) and [configure disk storage](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-server/storage/configure-disk-storage/).
