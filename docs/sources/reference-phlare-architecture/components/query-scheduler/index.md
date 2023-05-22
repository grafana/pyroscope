---
title: "Grafana Phlare query-scheduler"
menuTitle: "Query-scheduler"
description: "The query-scheduler distributes work to queriers."
weight: 120
---

# Grafana Phlare query-scheduler

The query-scheduler is a stateless component that retains a queue of queries to execute, and distributes the workload to available [queriers]({{< relref "../querier.md" >}}).

The query-scheduler is a required component when using the [query-frontend]({{< relref "../query-frontend/index.md" >}}).

![Query-scheduler architecture](query-scheduler-architecture.png)

[//]: # "Diagram source at https://docs.google.com/presentation/d/1bHp8_zcoWCYoNU2AhO2lSagQyuIrghkCncViSqn14cU/edit"

The following flow describes how a query moves through a Grafana Phlare cluster:

1. The [query-frontend]({{< relref "../query-frontend/index.md" >}}) receives queries, and then either splits and shards them, or serves them from the cache.
1. The query-frontend enqueues the queries into a query-scheduler.
1. The query-scheduler stores the queries in an in-memory queue where they wait for a querier to pick them up.
1. Queriers pick up the queries, and executes them.
1. The querier sends results back to query-frontend, which then forwards the results to the client.

## Benefits of using the query-scheduler

Query-scheduler enables the scaling of query-frontends. To learn more, see Mimir [Query Frontend](/docs/mimir/latest/operators-guide/architecture/components/query-frontend/#why-query-frontend-scalability-is-limited) documentation.

## Configuration

To use the query-scheduler, query-frontends and queriers need to discover the addresses of query-scheduler instances.
To advertise itself, the query-scheduler uses Ring-based service discovery which is configured via the [memberlist configuration]({{< relref "../../../configure-server/configuring-memberlist.md" >}}).

## Operational considerations

For high-availability, run two query-scheduler replicas.
