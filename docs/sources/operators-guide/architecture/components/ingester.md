---
title: "Grafana Mimir ingester"
menuTitle: "Ingester"
description: "The ingester writes incoming series to long-term storage."
weight: 30
---

# Grafana Mimir ingester

The ingester is a stateful component that writes incoming series to [long-term storage]({{< relref "../about-grafana-mimir-architecture/index.md#long-term-storage" >}}) on the write path and returns series samples for queries on the read path.

Incoming series from [distributors]({{< relref "distributor.md" >}}) are not immediately written to the long-term storage but are either kept in ingesters memory or offloaded to ingesters disk.
Eventually, all series are written to disk and periodically uploaded (by default every two hours) to the long-term storage.
For this reason, the [queriers]({{< relref "querier.md" >}}) might need to fetch samples from both ingesters and long-term storage while executing a query on the read path.

Any Grafana Mimir component that calls the ingesters starts by first looking up ingesters registered in the [hash ring]({{< relref "../hash-ring/index.md" >}}) to determine which ingesters are available.
Each ingester could be in one of the following states:

- `PENDING`<br />
  The ingester has just started. While in this state, the ingester does not receive write or read requests.
- `JOINING`<br />
  The ingester starts up and joins the ring. While in this state, the ingester does not receive write or read requests.
  The ingester loads tokens from disk (if `-ingester.ring.tokens-file-path` is configured) or generates a set of new random tokens.
  Finally, the ingester optionally observes the ring for token conflicts, and once resolved, moves to the `ACTIVE` state.
- `ACTIVE`<br />
  The ingester is up and running. While in this state, the ingester can receive both write and read requests.
- `LEAVING`<br />
  The ingester is shutting down and leaving the ring. While in this state, the ingester doesn't receive write requests, but can still receive read requests.
- `UNHEALTHY`<br />
  The ingester has failed to heartbeat to the hash ring. While in this state, distributors bypass the ingester, which means that the ingester does not receive write or read requests.

To configure the ingesters' hash ring, refer to [configuring hash rings]({{< relref "../../configure/configuring-hash-rings.md" >}}).

## Ingesters write de-amplification

Ingesters store recently received samples in-memory in order to perform write de-amplification.
If the ingesters immediately write received samples to the long-term storage, the system would have difficulty scaling due to the high pressure on the long-term storage.
For this reason, the ingesters batch and compress samples in-memory and periodically upload them to the long-term storage.

Write de-amplification is the main source of Mimir's low total cost of ownership (TCO).

## Ingesters failure and data loss

If an ingester process crashes or exits abruptly, all the in-memory series that have not yet been uploaded to the long-term storage could be lost.
There are the following ways to mitigate this failure mode:

- Replication
- Write-ahead log (WAL)
- Write-behind log (WBL), only used if out-of-order ingestion is enabled.

### Replication

By default, each series is replicated to three ingesters.
Writes to the Mimir cluster are successful if a quorum of ingesters received the data, which is a minimum of 2 with a replication factor of 3.
If the Mimir cluster loses an ingester, the in-memory series samples held by the lost ingester are available at least in one other ingester.
In the event of a single ingester failure, no time series samples are lost.
If multiple ingesters fail, time series might be lost if the failure affects all the ingesters holding the replicas of a specific time series.

### Write-ahead log

The write-ahead log (WAL) writes all incoming series to a persistent disk until the series are uploaded to the long-term storage.
If an ingester fails, a subsequent process restart replays the WAL and recovers the in-memory series samples.

Contrary to the sole replication, and given that the persistent disk data is not lost, in the event of the failure of multiple ingesters, each ingester recovers the in-memory series samples from WAL after a subsequent restart.
Replication is still recommended in order to gracefully handle a single ingester failure.

### Write-behind log

The write-behind log (WBL) is similar to the WAL, but it only writes incoming out-of-order samples to a persistent disk until the series are uploaded to long-term storage.

There is a different log for this because it is not possible to know if a sample is out-of-order until Mimir tries to append it.
First Mimir needs to attempt to append it, the TSDB will detect that it is out-of-order, append it anyway if out-of-order is enabled and then write it to the log.

If the ingesters fail, the same characteristics as in the WAL apply.

## Zone aware replication

Zone aware replication ensures that the ingester replicas for a given time series are divided across different zones.
Zones can represent logical or physical failure domains, for example, different data centers.
Dividing replicas across multiple zones prevents data loss and service interruptions when there is a zone-wide outage.

To set up multi-zone replication, refer to [Configuring zone-aware replication]({{< relref "../../configure/configuring-zone-aware-replication.md" >}}).

## Shuffle sharding

Shuffle sharding can be used to reduce the effect that multiple tenants can have on each other.

For more information on shuffle sharding, refer to [Configuring shuffle sharding]({{< relref "../../configure/configuring-shuffle-sharding/index.md" >}}).

## Out-of-order samples ingestion

Out-of-order samples are discarded by default. If the system writing samples to Mimir produces out-of-order samples, you can enable ingestion of such samples.

For more information about out-of-order samples ingestion, refer to [Configuring out of order samples ingestion]({{< relref "../../configure/configure-out-of-order-samples-ingestion.md" >}}).
