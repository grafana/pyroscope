---
title: "Grafana Mimir querier"
menuTitle: "Querier"
description: "The querier evaluates PromQL expressions."
weight: 50
---

# Grafana Mimir querier

The querier is a stateless component that evaluates [PromQL](https://prometheus.io/docs/prometheus/latest/querying/basics/)
expressions by fetching time series and labels on the read path.

The querier uses the [store-gateway]({{< relref "store-gateway.md" >}}) component to query the [long-term storage]({{< relref "../about-grafana-mimir-architecture/index.md#long-term-storage" >}}) and the [ingester]({{< relref "ingester.md" >}}) component to query recently written data.

## How it works

To find the correct blocks to look up at query time, the querier requires an almost up-to-date view of the bucket in long-term storage. The querier performs one of the following actions to ensure that the bucket view is updated:

1. Periodically download the [bucket index]({{< relref "../bucket-index/index.md" >}}) (default)
2. Periodically scan the bucket

Queriers do not need any content from blocks except their metadata, which includes the minimum and maximum timestamp of samples within the block.

### Bucket index enabled (default)

Queriers lazily download the bucket index when they receive the first query for a given tenant. The querier caches the bucket index in memory and periodically keeps it up-to-date.

The bucket index contains a list of blocks and block deletion marks of a tenant. The querier later uses the list of blocks and block deletion marks to locate the set of blocks that need to be queried for the given query.

When the querier runs with the bucket index enabled, the querier startup time and the volume of API calls to object storage are reduced.
We recommend that you keep the bucket index enabled.

### Bucket index disabled

When [bucket index]({{< relref "../bucket-index/index.md" >}}) is disabled, queriers iterate over the storage bucket to discover blocks for all tenants and download the `meta.json` of each block. During this initial bucket scanning phase, a querier cannot process incoming queries and its `/ready` readiness probe endpoint will not return the HTTP status code `200`.

When running, queriers periodically iterate over the storage bucket to discover new tenants and recently uploaded blocks.

### Anatomy of a query request

When a querier receives a query range request, the request contains the following parameters:

- `query`: the PromQL query expression (for example, `rate(node_cpu_seconds_total[1m])`)
- `start`: the start time
- `end`: the end time
- `step`: the query resolution (for example, `30` yields one data point every 30 seconds)

For each query, the querier analyzes the `start` and `end` time range to compute a list of all known blocks containing at least one sample within the time range.
For each list of blocks per query, the querier computes a list of store-gateway instances holding the blocks. The querier sends a request to each matching store-gateway instance to fetch all samples for the series matching the `query` within the `start` and `end` time range.

The request sent to each store-gateway contains the list of block IDs that are expected to be queried, and the response sent back by the store-gateway to the querier contains the list of block IDs that were queried.
This list of block IDs might be a subset of the requested blocks, for example, when a recent blocks-resharding event occurs within the last few seconds.

The querier runs a consistency check on responses received from the store-gateways to ensure all expected blocks have been queried.
If the expected blocks have not been queried, the querier retries fetching samples from missing blocks from different store-gateways up to `-store-gateway.sharding-ring.replication-factor` (defaults to 3) times or maximum 3 times, whichever is lower.

If the consistency check fails after all retry attempts, the query execution fails.
Query failure due to the querier not querying all blocks ensures the correctness of query results.

If the query time range overlaps with the `-querier.query-ingesters-within` duration, the querier also sends the request to all ingesters.
The request to the ingesters fetches samples that have not yet been uploaded to the long-term storage or are not yet available for querying through the store-gateway.

After all samples have been fetched from both the store-gateways and the ingesters, the querier runs the PromQL engine to execute the query and sends back the result to the client.

### Connecting to store-gateways

You must configure the queriers with the same `-store-gateway.sharding-ring.*` flags (or their respective YAML configuration parameters) that you use to configure the store-gateways so that the querier can access the store-gateway hash ring and discover the addresses of the store-gateways.

### Connecting to ingesters

You must configure the querier with the same `-ingester.ring.*` flags (or their respective YAML configuration parameters) that you use to configure the ingesters so that the querier can access the ingester hash ring and discover the addresses of the ingesters.

## Caching

The querier supports the following cache:

- [Metadata cache](#metadata-cache)

Caching is optional, but highly recommended in a production environment.

### Metadata cache

[Store-gateways]({{< relref "store-gateway.md" >}}) and queriers can use Memcached to cache the following bucket metadata:

- List of tenants
- List of blocks per tenant
- Block `meta.json` existence and content
- Block `deletion-mark.json` existence and content
- Tenant `bucket-index.json.gz` content

Using the metadata cache reduces the number of API calls to long-term storage and stops the number of the API calls that scale linearly with the number of querier and store-gateway replicas.

To enable the metadata cache, set `-blocks-storage.bucket-store.metadata-cache.backend`.

> **Note**: Currently, only the `memcached` backend is supported. The Memcached client includes additional configuration available via flags that begin with the prefix `-blocks-storage.bucket-store.metadata-cache.memcached.*`.

Additional flags for configuring the metadata cache begin with the prefix `-blocks-storage.bucket-store.metadata-cache.*`. By configuring the TTL to zero or a negative value, caching of given item type is disabled.

> **Note:** The same Memcached backend cluster should be shared between store-gateways and queriers.

## Querier configuration

For details about querier configuration, refer to [querier]({{< relref "../../configure/reference-configuration-parameters/index.md#querier" >}}).
