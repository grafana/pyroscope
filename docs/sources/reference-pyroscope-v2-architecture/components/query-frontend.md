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

- **Receiving and validating queries** through the Query API
- **Executing queries** by using the [metastore](../metastore/) for block discovery and delegating execution to [query-backend](../query-backend/) instances

## Query flow

When a query arrives, the query frontend:

1. Validates the query request.
1. Queries the [metastore](../metastore/) to find all blocks matching the query criteria (time range, tenant, and optionally service name).
1. Builds a physical query plan as a tree: leaf nodes are read operations targeting specific blocks and datasets, while intermediate nodes are merge operations that combine results from their children.
1. Sends the plan root to a [query backend](../query-backend/) instance, which distributes subtrees to other query backend instances for parallel execution and merging. For more details, refer to [Query backend](../query-backend/).

Because the metastore serves block metadata from memory with linearizable reads, query planning is fast and does not require the query frontend to maintain any local state about blocks.

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
