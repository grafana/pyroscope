---
title: "Grafana Phlare query-frontend"
menuTitle: "Query-frontend"
description: "The query-frontend accelerates queries."
weight: 60
---

# Grafana Phlare query-frontend

The query-frontend is a stateless component that provides the same API as the [querier]({{< relref "../querier.md" >}}) and can be used to accelerate the read path and ensure fair scheduling between tenants using the [query-scheduler]({{< relref "../query-scheduler/index.md" >}}).

In this situation, queriers act as workers that pull jobs from the queue, execute them, and return the results to the query-frontend for aggregation.

We recommend that you run at least two query-frontend replicas for high-availability reasons.

> Because the [query-scheduler]({{< relref "../query-scheduler" >}}) is a mandatory component when using the query-frontend, you must run at least one query-scheduler replica.

The following steps describe how a query moves through the query-frontend.

1. A query-frontend receives a query.
1. The query-frontend places the query in an queue by communicating with the query-scheduler, where it waits to be picked up by a querier.
1. A querier picks up the query from the queue and executes it.
1. A querier or queriers return the result to query-frontend, which then aggregates and forwards the results to the client.
