---
title: "Pyroscope v2 query-frontend"
menuTitle: "Query-frontend"
description: "The query-frontend handles query planning and routes requests to query-backends."
weight: 50
keywords:
  - Pyroscope v2
  - query-frontend
  - queries
---

# Pyroscope v2 query-frontend

The query-frontend is a stateless component that serves as the entry point for the query path. It handles query planning and routes requests to [query-backend](../query-backend/) instances for execution.

## Responsibilities

The query-frontend is responsible for:

- **Query reception**: Receiving queries through the Query API
- **Metadata lookup**: Finding relevant data blocks via the [metastore](../metastore/)
- **Query planning**: Creating an execution plan for the query
- **Request routing**: Dispatching query execution to query-backend instances
- **Result handling**: Collecting and returning results to clients

## Query flow

1. **Query received**: A query arrives at the query-frontend.
1. **Metadata query**: The frontend queries the metastore to locate relevant blocks.
1. **Plan creation**: A query execution plan is created based on block locations.
1. **Backend invocation**: The plan is sent to query-backend instances for execution.
1. **Result return**: Results are collected and returned to the client.

## Block discovery

The query-frontend uses the metastore's metadata index to find blocks containing data for a query. The index provides:

- Block identifiers matching the query's time range
- Tenant and shard information
- Dataset labels for filtering

This allows the frontend to identify exactly which blocks need to be read, minimizing unnecessary object storage access.

## Supported query types

The query-frontend supports various query types:

- **Flame graph queries**: Profile data for visualization
- **Label queries**: Label names and values for filtering
- **Time series queries**: Profile data over time
- **Series label queries**: Prometheus-style series labels

## Stateless design

The query-frontend is completely stateless:

- Requires no persistent storage
- Scales horizontally to hundreds of instances
- Allows instances to be added or removed without coordination
- Supports auto-scaling based on query load

## Scalability

The query-frontend can scale independently of the write path:

- Heavy query workloads don't impact ingestion performance
- Handles increased query volume by adding more instances
- Works with any number of query-backend instances

## Load balancing

Query-frontends can be load balanced using standard HTTP load balancers. Each instance can handle any query, making round-robin load balancing effective.
