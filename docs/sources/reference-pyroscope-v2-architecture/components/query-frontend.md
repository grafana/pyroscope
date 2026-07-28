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

## Symbolization of native profiles

Profiles collected from native code (for example, eBPF-based profiling) can contain stack frames that only carry a build ID and an address, without a resolved function name. Pyroscope resolves these frames using [debuginfod](https://sourceware.org/elfutils/Debuginfod.html): it fetches debug information for the build ID, extracts function names from it, and caches the result in object storage for reuse.

Symbolization is disabled by default. `-symbolizer.enabled=true` turns it on — the flag sets the default for all tenants and can be overridden per tenant — and `-symbolizer.debuginfod-url` selects the debuginfod server to fetch debug information from (default `https://debuginfod.elfutils.org`).

With symbolization enabled, the per-tenant flag `symbolizer.symbol-ref-trees-enabled` (default `false`) makes the query backend emit tree-query results with unresolved native frames carried in the tree itself, and has the query frontend resolve them once after merging results from all query backends. Resolution of a single binary's addresses is bounded by `symbolizer.resolve-timeout` (default `20s`).
