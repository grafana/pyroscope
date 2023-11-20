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
The `-target` parameter controls which component(s) that single binary will behave as. For those looking for a simple way to get started, Pyroscope can also be run in [monolithic mode]({{< relref "../deployment-modes/index.md#monolithic-mode" >}}), with all components running simultaneously in one process.
For more information, refer to [Deployment modes]({{< relref "../deployment-modes/index.md" >}}).

## Pyroscope components

Most components are stateless and do not require any data persisted between process restarts. Some components are stateful and rely on non-volatile storage to prevent data loss between process restarts. For details about each component, see its page in [Components]({{< relref "../components/_index.md" >}}).

### The write path

[//]: # "To edit open with https://mermaid.live/edit#pako{...}"

<p align="center">
  <img alt="Architecture of Pyroscope's write path" width="200px" src="https://mermaid.ink/svg/pako:eNqNkT1PwzAQhv9K5C6t1DY0hrT1wIBgZgCJoeng2OfE4MSRfaFUVf47dkoLI9vd896H7_WJCCuBMKKMPYiaO0xeH4o2SXoPbrp7cxrB72fJYnGfSO3R6bJH62LFn3SUdVuBRxi1SzwK1kckbNNxcSk-M-vH5Cqd2XT3XL6DwMQHBPtZpB6PBsZHJUobwyZqq-Zhv_0ANqGU_sSLg5ZYs6z7-m0KS_7bQuakAddwLYMjpziiIFhDAwVhIZSgeG-wIEU7hFIeTn85toIwdD3MSd9JjvCoeeV4Q5jixl_pk9ThmCs0lksI6YngsYv2V8HMMFLYVukq8t6ZgGvEzrM0jfKy0lj35TK4lXot41_Vn9s8zbN8wzMK-ZryO0qlKFfbjcpuV0qub1YZJ8MwfAN3mqSC" />
  </a>
</p>

Ingesters receive incoming profiles from the distributors.
Each push request belongs to a tenant, and the ingester appends the received profiles to the specific per-tenant Pyroscope database that is stored on the local disk.

The per-tenant Pyroscope database is lazily created in each ingester as soon as the first profiles are received for that tenant.

The in-memory profiles are periodically flushed to disk and new block is created.

For more information, refer to [Ingester]({{< relref "../components/ingester.md" >}}).

#### Series sharding and replication

By default, each profile series is replicated to three ingesters, and each ingester writes its own block to the long-term storage. The [Compactor]({{< relref "../components/compactor" >}}) merges blocks from multiple ingesters into a single block, and removes duplicate samples. Blocks compaction significantly reduces storage utilization.


### The read path

[//]: # "To edit open with https://mermaid.live/edit#pako{...}"

<p align="center">
  <img alt="Architecture of Pyroscope's read path" width="400px" src="https://mermaid.ink/svg/pako:eNqNkT1PwzAQhv9K5C6t1DQ0gX54YEAwIwFb08G1z4nBiYN9pkRV_jt2gQJi6XZ-7rmTXt-BcCOAUCK12fOaWUyebso2SbwDO948ABNuO0nS9Dp59WD7VFrTIrQiOn_JL8nxGoTXYL8tBfactmorcPifOzQW0ooh7Fl_JMZFx7jx5n73DBw_le0kUoe9hmOARCqt6Uiu5dShNS9AR0VRfNXpXgmsad69_wwZd_YImZIGbMOUCL93iCtKgjU0UBIaSgGSeY0lKdshqMyjeexbTihaD1PiOxHS3CpWWdYQKpl2J3onVAhzgtowAeF5INh38VSVchhWctNKVUXurQ64RuwczbLYnlUKa7-bcdNkTol41_ptvcgW-WLF8gIWy4JdFYXgu_l6JfPLuRTLi3nOyDAMHzzctK0" />
  </a>
</p>

Queries coming into Pyroscope arrive at [query-frontend]({{< relref "../components/query-frontend" >}}) component which is responsible for accelerating queries and dispatching them to the [query-scheduler]({{< relref "../components/query-scheduler" >}}).

The [query-scheduler]({{< relref "../components/query-scheduler" >}}) maintains a queue of queries and ensures that each tenant's queries are fairly executed.

The [queriers]({{< relref "../components/querier" >}}) act as workers, pulling queries from the queue in the query-scheduler. The queriers connect to the ingesters to fetch all the data needed to execute a query. For more information about how the query is executed, refer to [querier]({{< relref "../components/querier.md" >}}).

Depending on the time window selected, the querier involves [ingesters]({{< relref "../components/ingester" >}}) for recent data and [store-gateways]({{< relref "../components/store-gateway" >}}) for data from long-term storage.


## Long-term storage

The Pyroscope storage format is described in detail in on the [block format page]({{< relref "../block-format" >}}).
The Pyroscope storage format stores each tenant's profiles into their own on-disk block. Each on-disk block directory contains an index file, a file containing metadata, and the Parquet tables.

Pyroscope requires any of the following object stores for the block files:

[//]: # "TODO: Verify that's correct"

- [Amazon S3](https://aws.amazon.com/s3)
- [Google Cloud Storage](https://cloud.google.com/storage/)
- [Microsoft Azure Storage](https://azure.microsoft.com/en-us/services/storage/)
- [OpenStack Swift](https://wiki.openstack.org/wiki/Swift)
- Local Filesystem (single node only)

For more information, refer to [configure object storage]({{< relref "../../configure-server/configure-object-storage-backend.md" >}}) and [configure disk storage]({{< relref "../../configure-server/configure-disk-storage.md" >}}).
