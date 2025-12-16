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

1. **Plan reception**: The query-backend receives an execution plan from the query-frontend.
1. **Block reading**: Required blocks are read from object storage.
1. **Parallel processing**: Query execution is parallelized across multiple blocks.
1. **Result aggregation**: Results from sub-queries are combined.
1. **Response**: The final result is returned to the query-frontend.

## Graph-based query execution

Query execution is represented as a graph where:

- Nodes represent operations (read, merge, aggregate)
- Edges represent data flow between operations
- Sub-queries can execute in parallel
- Results are combined and optimized at merge points

This approach:
- Minimizes network overhead
- Enables horizontal scalability
- Reduces memory requirements through streaming
- Optimizes data transfer between components

## Direct object storage access

Unlike v1 where queries may need to access ingesters for recent data, the v2 query-backend reads directly from object storage:

- No coordination with write-path components needed
- Simplified query execution
- Better isolation between read and write paths
- Easier horizontal scaling

## Supported operations

The query-backend supports various query operations:

- **Profile queries**: Reading and merging profile data
- **Label queries**: Extracting label names and values
- **Time series queries**: Aggregating data over time
- **Tree queries**: Building flame graph trees
- **pprof queries**: Generating pprof-format output

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
- **Streaming execution**: Results streamed as they're computed
- **Memory efficient**: Graph-based execution minimizes memory requirements
- **Network optimized**: Results combined close to the data source

## Future: serverless execution

Future versions will include a serverless query-backend option, making querying even more cost-effective by only paying for actual query execution time.
