---
title: "Grafana Mimir distributor"
menuTitle: "Distributor"
description: "The distributor validates time-series data and sends the data to ingesters."
weight: 20
---

# Grafana Mimir distributor

The distributor is a stateless component that receives time-series data from Prometheus or the Grafana agent.
The distributor validates the data for correctness and to ensure that it is within the configured limits for a given tenant.
The distributor then divides the data into batches and sends it to multiple [ingesters]({{< relref "ingester.md" >}}) in parallel, shards the series among ingesters, and replicates each series by the configured replication factor. By default, the configured replication factor is three.

## Validation

The distributor cleans and validates data that it receives before writing the data to the ingesters.
Because a single request can contain valid and invalid metrics, samples, metadata, and exemplars, the distributor only passes valid data to the ingesters. The distributor does not include invalid data in its requests to the ingesters.
If the request contains invalid data, the distributor returns a 400 HTTP status code and the details appear in the response body.
The details about the first invalid data are typically logged by the sender, be it Prometheus or Grafana Agent.

The distributor data cleanup includes the following transformation:

- The metric metadata `help` is truncated to fit in the length defined via the `-validation.max-metadata-length` flag.

The distributor validation includes the following checks:

- The metric metadata and labels conform to the [Prometheus exposition format](https://prometheus.io/docs/concepts/data_model/).
- The metric metadata (`name` and `unit`) are not longer than what is defined via the `-validation.max-metadata-length` flag.
- The number of labels of each metric is not higher than `-validation.max-label-names-per-series`.
- Each metric label name is not longer than `-validation.max-length-label-name`.
- Each metric label value is not longer than `-validation.max-length-label-value`.
- Each sample timestamp is not newer than `-validation.create-grace-period`.
- Each exemplar has a timestamp and at least one non-empty label name and value pair.
- Each exemplar has no more than 128 labels.

> **Note:** For each tenant, you can override the validation checks by modifying the overrides section of the [runtime configuration]({{< relref "../../configure/about-runtime-configuration.md" >}}).

## Rate limiting

The distributor includes two different types of rate limiters that apply to each tenant.

- **Request rate**<br />
  The maximum number of requests per second that can be served across Grafana Mimir cluster for each tenant.

- **Ingestion rate**<br />
  The maximum samples per second that can be ingested across Grafana Mimir cluster for each tenant.

If any of these rates is exceeded, the distributor drops the request and returns an HTTP 429 response code.

Internally, these limits are implemented using a per-distributor local rate limiter.
The local rate limiter for each distributor is configured with a limit of `limit / N`, where `N` is the number of healthy distributor replicas.
The distributor automatically adjusts the request and ingestion rate limits if the number of distributor replicas change.
Because these rate limits are implemented using a per-distributor local rate limiter, they require that write requests are [evenly distributed across the pool of distributors]({{< relref "#load-balancing-across-distributors" >}}).

Use the following flags to configure the rate limits:

- `-distributor.request-rate-limit`: Request rate limit, which is per tenant, and which is in requests per second
- `-distributor.request-burst-size`: Request burst size (in number of requests) allowed, which is per tenant
- `-distributor.ingestion-rate-limit`: Ingestion rate limit, which is per tenant, and which is in samples per second
- `-distributor.ingestion-burst-size`: Ingestion burst size (in number of samples) allowed, which is per tenant

> **Note:** You can override rate limiting on a per-tenant basis by setting `request_rate`, `ingestion_rate`, `request_burst_size` and `ingestion_burst_size` in the overrides section of the runtime configuration.

> **Note:** By default, Prometheus remote write doesn't retry requests on 429 HTTP response status code. To modify this behavior, use `retry_on_http_429: true` in the Prometheus [`remote_write` configuration](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#remote_write).

### Configuration

The distributors form a [hash ring]({{< relref "../hash-ring/index.md" >}}) (called the distributors’ ring) to discover each other and enforce limits correctly.
To configure the distributors' hash ring, refer to [configuring hash rings]({{< relref "../../configure/configuring-hash-rings.md" >}}).

## High-availability tracker

Remote write senders, such as Prometheus, can be configured in pairs, which means that metrics continue to be scraped and written to Grafana Mimir even when one of the remote write senders is down for maintenance or is unavailable due to a failure.
We refer to this configuration as high-availability (HA) pairs.

The distributor includes an HA tracker.
When the HA tracker is enabled, the distributor deduplicates incoming series from Prometheus HA pairs.
This enables you to have multiple HA replicas of the same Prometheus servers that write the same series to Mimir and then deduplicates the series in the Mimir distributor.

For more information about HA deduplication and how to configure it, refer to [configure HA deduplication]({{< relref "../../configure/configuring-high-availability-deduplication.md" >}}).

## Sharding and replication

The distributor shards and replicates incoming series across ingesters.
You can configure the number of ingester replicas that each series is written to via the `-ingester.ring.replication-factor` flag, which is `3` by default.
Distributors use consistent hashing, in conjunction with a configurable replication factor, to determine which ingesters receive a given series.

Sharding and replication uses the ingesters' hash ring.
For each incoming series, the distributor computes a hash using the metric name, labels, and tenant ID.
The computed hash is called a _token_.
The distributor looks up the token in the hash ring to determine which ingesters to write a series to.

For more information, see [hash ring]({{< relref "../hash-ring/index.md" >}}).

#### Quorum consistency

Because distributors share access to the same hash ring, write requests can be sent to any distributor. You can also set up a stateless load balancer in front of it.

To ensure consistent query results, Mimir uses [Dynamo-style](https://www.allthingsdistributed.com/files/amazon-dynamo-sosp2007.pdf) quorum consistency on reads and writes.
The distributor waits for a successful response from `n`/2 + 1 ingesters, where `n` is the configured replication factor, before sending a successful response to the Prometheus write request.

## Load balancing across distributors

We recommend randomly load balancing write requests across distributor instances.
If you're running Grafana Mimir in a Kubernetes cluster, you can define a Kubernetes [Service](https://kubernetes.io/docs/concepts/services-networking/service/) as ingress for the distributors.

> **Note:** A Kubernetes Service balances TCP connections across Kubernetes endpoints and does not balance HTTP requests within a single TCP connection.
> If you enable HTTP persistent connections (HTTP keep-alive), because Prometheus uses HTTP keep-alive, it re-uses the same TCP connection for each remote-write HTTP request of a remote-write shard.
> This can cause distributors to receive an uneven distribution of remote-write HTTP requests.
> To improve the balancing of requests between distributors, consider increasing `min_shards` in the Prometheus [remote write config](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#remote_write).

## Configuration

The distributors must form a hash ring (also called the _distributors ring_) so that they can discover each other and correctly enforce limits.

The default configuration uses `memberlist` as backend for the distributors ring.
If you want to configure a different backend, for example, `consul` or `etcd`, you can use the following CLI flags (and their respective YAML configuration options) to configure the distributors ring KV store:

- `-distributor.ring.store`: The backend storage to use.
- `-distributor.ring.consul.*`: The Consul client configuration. Set this flag only if `consul` is the configured backend storage.
- `-distributor.ring.etcd.*`: The etcd client configuration. Set this flag only if `etcd` is the configured backend storage.
