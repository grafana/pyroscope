---
title: "About the Grafana Phlare architecture"
menuTitle: "About the architecture"
description: "Learn about the Grafana Phlare architecture."
weight: 10
---

# About the Grafana Phlare architecture

Grafana Phlare has a microservices-based architecture.
The system has multiple horizontally scalable microservices that can run separately and in parallel.
Grafana Phlare microservices are called components.

Grafana Phlare's design compiles the code for all components into a single binary.
The `-target` parameter controls which component(s) that single binary will behave as. For those looking for a simple way to get started, Grafana Phlare can also be run in [monolithic mode]({{< relref "../deployment-modes/index.md#monolithic-mode" >}}), with all components running simultaneously in one process.
For more information, refer to [Deployment modes]({{< relref "../deployment-modes/index.md" >}}).

## Grafana Phlare components

Most components are stateless and do not require any data persisted between process restarts. Some components are stateful and rely on non-volatile storage to prevent data loss between process restarts. For details about each component, see its page in [Components]({{< relref "../components/_index.md" >}}).

### The write path

[//]: # "To edit open with https://mermaid.live/edit#pako{...}"

<p align="center">
  <img alt="Architecture of Grafana Phlare's write path" width="200px" src="https://mermaid.ink/svg/pako:eNqNUc9PwyAU_lcavGzJtrqi3cbBg9GzB008rDtQeLQoLQ08nMvS_11onHr09vH9gvc4E2ElEEaUsUfRcofZy33VZ1nw4Gb7V6cR_GGeLZd3mdQena4DWpccf46TrPsGPMKkXfAkWJ8o62f7p_oNBGY-RuAwT6zHk4HpskxpY9iV2qlF7LXvwK4opd94edQSW1YMn78h6_8dIQvSgeu4lnHSc6qoCLbQQUVYhBIUDwYrUvVjtIZBcoRHqeMzCVPceFgQHud8PvWCMHQBLqYHzRvHux-XsVxCDJ0Jnoa01iYuKVYK2yvdJD44E-kWcfAsz5O8ajS2oV4J2-Vey_QH7ceuzMui3PKCQrmh_JZSKer1bquKm7WSm-t1wck4jl9KVZdq" />
  </a>
</p>

Ingesters receive incoming profiles from the distributors.
Each push request belongs to a tenant, and the ingester appends the received profiles to the specific per-tenant PhlareDB that is stored on the local disk.

The per-tenant PhlareDB is lazily created in each ingester as soon as the first profiles are received for that tenant.

The in-memory profiles are periodically flushed to disk and new block is created.

For more information, refer to [Ingester]({{< relref "../components/ingester.md" >}}).

#### Series sharding and replication

By default, each profile series is replicated to three ingesters, and each ingester writes its own block to the long-term storage.

### The read path

[//]: # "To edit open with https://mermaid.live/edit#pako{...}"

<p align="center">
  <img alt="Architecture of Grafana Phlare's read path" width="400px" src="https://mermaid.ink/svg/pako:eNqNkU1PwzAMQP9KlF02aV1ZC92WAwcEZyTgtu6QNU4bSJOSOIxq6n8nnfg-7eY8P1uOfaSVFUAZldoeqoY7JE83pSEkeHDT7QNw4XczkiTX5DWA6xPprEEwYnT-kl-SrxoQQYP7shS4c9LK1ODxPyeVbWOGeGvNSbN-FKyfbu_3z1Ah8Wgd7GYj9dhrOE1PpNKaTeRGzj06-wJskuf5Z5wclMCGZd37T5H1Z5fQOW3BtVyJuLrj2KKk2EALJWUxFCB50FjS0gxR5QHtY28qytAFmNPQCY5wq3jteEuZ5Np_0zuh4me-obZcQHweKfbdeKdaeYwtK2ukqkcenI64Qew8S9MxvagVNmG_iGtLvRLjUZu3TZEWWbHmWQ7FKudXeS6q_XKzltnlUorVxTLjdBiGD-Nas38" />
  </a>
</p>

Queries coming into Grafana Phlare arrive at [query-frontend]({{< relref "../components/query-frontend" >}}) component which is responsible for accelerating queries and dispatching them to the [query-scheduler]({{< relref "../components/query-scheduler" >}}).

The [query-scheduler]({{< relref "../components/query-scheduler" >}}) maintains a queue of queries and ensures that each tenant's queries are fairly executed.

The [querier]({{< relref "../components/querier" >}}) act as workers, pulling queries from the queue in the query-scheduler. The queriers connect to the ingesters to fetch all the data needed to execute a query. For more information about how the query is executed, refer to [querier]({{< relref "../components/querier.md" >}}).

## Long-term storage

The Grafana Phlare storage format is described in detail in on the [block format page]({{< relref "..//block-format/" >}}).
The Grafana Phlare storage format stores each tenant's profiles into their own on-disk block. Each on-disk block directory contains an index file, a file containing metadata, and the Parquet tables.

Grafana Phlare requires any of the following object stores for the block files:

[//]: # "TODO: Verify that's correct"

- [Amazon S3](https://aws.amazon.com/s3)
- [Google Cloud Storage](https://cloud.google.com/storage/)
- [Microsoft Azure Storage](https://azure.microsoft.com/en-us/services/storage/)
- [OpenStack Swift](https://wiki.openstack.org/wiki/Swift)
- Local Filesystem (single node only)

For more information, refer to [configure object storage]({{< relref "../../configure/configure-object-storage-backend.md" >}}) and [configure disk storage]({{< relref "../../configure/configure-disk-storage.md" >}}).
