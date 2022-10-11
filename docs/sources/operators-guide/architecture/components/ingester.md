---
title: "Grafana Phlare ingester"
menuTitle: "Ingester"
description: "The ingester writes incoming profiles to long-term storage."
weight: 30
---

# Grafana Phlare ingester

The ingester is a stateful component that writes incoming profiles first to [on disk storage]({{< relref "../about-grafana-phlare-architecture/index.md#long-term-storage" >}}) on the write path and returns series samples for queries on the read path.

Incoming profiles from [distributors]({{< relref "distributor.md" >}}) are not immediately written to the long-term storage but are either kept in ingesters memory or offloaded to ingesters disk.
Eventually, all profiles are written to disk and periodically uploaded to the long-term storage.
For this reason, the [queriers]({{< relref "querier.md" >}}) might need to fetch samples from both ingesters and long-term storage while executing a query on the read path.

Any Grafana Phlare component that calls the ingesters starts by first looking up ingesters registered in the [hash ring]({{< relref "../hash-ring/index.md" >}}) to determine which ingesters are available.
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

To configure the ingesters' hash ring, refer to [configuring memberlist]({{< relref "../../configure/configuring-memberlist.md" >}}).

## Ingesters write de-amplification

Ingesters store recently received samples in-memory in order to perform write de-amplification.
If the ingesters immediately write received samples to the long-term storage, the system would have difficulty scaling due to the high pressure on the long-term storage.
For this reason, the ingesters batch and compress samples in-memory and periodically upload them to the long-term storage.

Write de-amplification is the main source of Phlare's low total cost of ownership (TCO).

## Ingesters failure and data loss

If an ingester process crashes or exits abruptly, all the in-memory profiles
that have not yet been uploaded to the long-term storage could be lost. There
are the following ways to mitigate this failure mode:

- Replication

### Replication

By default, each profile series is replicated to three ingesters. Writes to the
Phlare cluster are successful if a quorum of ingesters received the data, which
is a minimum of 2 with a replication factor of 3. If the Phlare cluster loses an
ingester, the in-memory profiles held by the head block of the lost ingester
are available at least in one other ingester. In the event of a single ingester
failure, no profiles are lost. If multiple ingesters fail, profiles might be
lost if the failure affects all the ingesters holding the replicas of a
specific profile series.
