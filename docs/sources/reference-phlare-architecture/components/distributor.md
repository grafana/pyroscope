---
title: "Grafana Phlare distributor"
menuTitle: "Distributor"
description: "The distributor validates time-series data and sends the data to ingesters."
weight: 20
---

# Grafana Phlare distributor

The distributor is a stateless component that receives profiling data from the agent.
The distributor then divides the data into batches and sends it to multiple [ingesters]({{< relref "ingester.md" >}}) in parallel, shards the series among ingesters, and replicates each series by the configured replication factor. By default, the configured replication factor is three.

## Validation

The distributor cleans and validates data that it receives before writing the data to the ingesters.
Because a single request can contain valid and invalid profiles, samples, metadata, and exemplars, the distributor only passes valid data to the ingesters. The distributor does not include invalid data in its requests to the ingesters.
If the request contains invalid data, the distributor returns a 400 HTTP status code and the details appear in the response body.
The details about the first invalid data are typically logged by the agent.

The distributor data cleanup includes the following transformation:

* Ensure the profile has a timestamp set, if not it will default to the time the distributor received the profile.
* The distributor will remove samples that are having values of `0` and will sum samples that share the same stacktrace.

## Replication

The distributor shards and replicates incoming series across ingesters.
You can configure the number of ingester replicas that each series is written to via the `-ingester.ring.replication-factor` flag, which is `1` by default.
Distributors use consistent hashing, in conjunction with a configurable replication factor, to determine which ingesters receive a given series.

Sharding and replication uses the ingesters' hash ring.
For each incoming series, the distributor computes a hash using the profile name, labels, and tenant ID.
The computed hash is called a _token_.
The distributor looks up the token in the hash ring to determine which ingesters to write a series to.

For more information, see [hash ring]({{< relref "../hash-ring/index.md" >}}).

#### Quorum consistency

Because distributors share access to the same hash ring, write requests can be sent to any distributor. You can also set up a stateless load balancer in front of it.

To ensure consistent query results, Phlare uses [Dynamo-style](https://www.allthingsdistributed.com/files/amazon-dynamo-sosp2007.pdf) quorum consistency on reads and writes.
The distributor waits for a successful response from `n`/2 + 1 ingesters, where `n` is the configured replication factor, before sending a successful response to the Agent push request.

## Load balancing across distributors

We recommend randomly load balancing write requests across distributor instances.
If you're running Grafana Phlare in a Kubernetes cluster, you can define a Kubernetes [Service](https://kubernetes.io/docs/concepts/services-networking/service/) as ingress for the distributors.

> **Note:** A Kubernetes Service balances TCP connections across Kubernetes endpoints and does not balance HTTP requests within a single TCP connection.
> If you enable HTTP persistent connections (HTTP keep-alive), because the Agent uses HTTP keep-alive, it re-uses the same TCP connection for each push HTTP request.
> This can cause distributors to receive an uneven distribution of push HTTP requests.
