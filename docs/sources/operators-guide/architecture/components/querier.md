---
title: "Grafana Phlare querier"
menuTitle: "Querier"
description: "The querier evaluates PromQL expressions."
weight: 50
---

# Grafana Phlare querier

The querier is a stateless component that evaluates queries  expressions by fetching profiles series and labels on the read path.

The querier uses the [ingester]({{< relref "ingester.md" >}}) component only to query recently written data. The support of querying the [long-term storage]({{< relref "../about-grafana-phlare-architecture/index.md#long-term-storage" >}}) is planned for the next releases.


[//TODO]:<> (Do this!)

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

### Connecting to ingesters

You must configure the querier with the same `-ingester.ring.*` flags (or their respective YAML configuration parameters) that you use to configure the ingesters so that the querier can access the ingester hash ring and discover the addresses of the ingesters.

## Querier configuration

For details about querier configuration, refer to [querier]({{< relref "../../configure/reference-configuration-parameters/index.md#querier" >}}).
