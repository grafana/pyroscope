---
title: "Pyroscope v2 query-backend"
menuTitle: "Query-backend"
description: "The query-backend executes queries with high parallelism."
weight: 60
keywords:
  - Pyroscope v2
  - query-backend
  - queries
  - parallel execution
---

# Pyroscope v2 query-backend

The query-backend is a stateless component that executes queries with high parallelism. It reads data directly from object storage and processes it according to the query plan received from the [query-frontend](../query-frontend/).

## How it works

The [query frontend](../query-frontend/) builds a physical query plan as a tree structure:

- **Read nodes** (leaves) fetch and process data from specific blocks in object storage.
- **Merge nodes** (intermediate) combine results from their child nodes.

The query frontend sends the plan root to a query backend instance. That instance distributes subtrees to other query backend instances for parallel execution, collects their results, and merges them. The final merged result is returned to the query frontend, which forwards it to the client.

This tree-based execution allows queries to fan out across many query backend instances in parallel, with merging happening at each level of the tree rather than in a single aggregation point.

## Direct object storage access

Unlike v1 where queries may need to access ingesters for recent data, the v2 query-backend reads directly from object storage:

- No coordination with write-path components needed
- Simplified query execution
- Better isolation between read and write paths
- Easier horizontal scaling

## Stateless design

The query-backend is completely stateless:

- Requires no persistent storage
- Needs no caching layer (reads directly from object storage)
- Scales horizontally to hundreds of instances
- Allows instances to be added or removed without coordination

## Scalability

The query-backend enables horizontal scaling of the read path:

- Handles heavier query workloads by adding more instances
- Scales independently of the write path
- Shares no state between instances
- Supports auto-scaling based on query load

## Performance characteristics

- **High parallelism**: Multiple blocks processed concurrently
- **Memory efficient**: Tree-based execution minimizes memory requirements
- **Network optimized**: Results combined close to the data source
