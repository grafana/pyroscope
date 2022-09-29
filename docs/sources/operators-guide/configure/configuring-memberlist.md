---
aliases:
  - /docs/fire/latest/operators-guide/configuring/configuring-memberlist/
description: Learn how to configure Grafana Fire memberlist.
menuTitle: Configuring memberlist
title: Configuring Grafana Fire memberlist
weight: 50
---

# Configuring Grafana Fire memberlist

[Hash rings]({{< relref "../architecture/hash-ring/index.md" >}}) are a distributed consistent hashing scheme and are widely used by Grafana Fire for sharding and replication.

Each of the following Grafana Fire components builds an independent hash ring.
The CLI flags used to configure the hash ring of each component have the following prefixes:

- Ingesters: `-ingester.ring.*`
- Distributors: `-distributor.ring.*`

The rest of the documentation refers to these prefixes as `<prefix>`.
You can configure each parameter either via the CLI flag or its respective YAML [config option]({{< relref "reference-configuration-parameters/index.md" >}}).

### Memberlist

By default, Grafana Fire uses `memberlist` as the KV store backend.

At startup, a Grafana Fire instance connects to other Fire replicas to join the cluster.
A Grafana Fire instance discovers the other replicas to join by resolving the addresses configured in `-memberlist.join`.
The `-memberlist.join` CLI flag must resolve to other replicas in the cluster and can be specified multiple times.

The `-memberlist.join` can be set to:

- An address in the `<ip>:<port>` format.
- An address in the `<hostname>:<port>` format.
- An address in the [DNS service discovery]({{< relref "about-dns-service-discovery.md" >}}) format.

The default port is `7946`.

> **Note**: At a minimum, configure one or more addresses that resolve to a consistent subset of replicas (for example, all the ingesters).

> **Note**: If you're running Grafana Fire in Kubernetes, define a [headless Kubernetes Service](https://kubernetes.io/docs/concepts/services-networking/service/#headless-services) which resolves to the IP addresses of all Grafana Fire pods. Then you set `-memberlist.join` to `dnssrv+<service name>.<namespace>.svc.cluster.local:<port>`.

> **Note**: The `memberlist` backend is configured globally and can't be customized on a per-component basis. Because `memberlist` is configured globally, the `memberlist` backend differs from other supported backends, such as Consul or etcd.

Grafana Fire supports TLS for memberlist connections between its components.
For more information about TLS configuration, refer to [secure communications with TLS]({{< relref "../secure/securing-communications-with-tls.md" >}}).

To see all supported configuration parameters, refer to [memberlist]({{< relref "reference-configuration-parameters/index.md#memberlist" >}}).

#### Configuring the memberlist address and port

By default, Grafana Fire memberlist protocol listens on address `0.0.0.0` and port `7946`.
If you run multiple Fire processes on the same node or the port `7946` is not available, you can change the bind and advertise port by setting the following parameters:

- `-memberlist.bind-addr`: IP address to listen on the local machine.
- `-memberlist.bind-port`: Port to listen on the local machine.
- `-memberlist.advertise-addr`: IP address to advertise to other Fire replicas. The other replicas will connect to this IP to talk to the instance.
- `-memberlist.advertise-port`: Port to advertise to other Fire replicas. The other replicas will connect to this port to talk to the instance.

### Fine tuning memberlist changes propagation latency

The `cortex_ring_oldest_member_timestamp` metric can be used to measure the propagation of hash ring changes.
This metric tracks the oldest heartbeat timestamp across all instances in the ring.
You can execute the following query to measure the age of the oldest heartbeat timestamp in the ring:

```promql
max(time() - cortex_ring_oldest_member_timestamp{state="ACTIVE"})
```

The measured age shouldn't be higher than the configured `<prefix>.heartbeat-period` plus a reasonable delta (for example, 15 seconds).
If you experience a higher changes propagation latency, you can adjust the following settings:

- Decrease `-memberlist.gossip-interval`
- Increase `-memberlist.gossip-nodes`
- Decrease `-memberlist.pullpush-interval`
- Increase `-memberlist.retransmit-factor`

# About Grafana Fire DNS service discovery

Some clients in Grafana Fire support service discovery via DNS to locate the addresses of backend servers to connect to. The following clients support service discovery via DNS:

- [Memcached server addresses]({{< relref "reference-configuration-parameters/index.md#memcached" >}})
  - `-blocks-storage.bucket-store.chunks-cache.memcached.addresses`
  - `-blocks-storage.bucket-store.index-cache.memcached.addresses`
  - `-blocks-storage.bucket-store.metadata-cache.memcached.addresses`
  - `-query-frontend.results-cache.memcached.addresses`
- [Memberlist KV store]({{< relref "reference-configuration-parameters/index.md#memberlist" >}})
  - `-memberlist.join`
- [Alertmanager URL configured in the ruler]({{< relref "reference-configuration-parameters/index.md#ruler" >}})
  - `-ruler.alertmanager-url`

## Supported discovery modes

DNS service discovery supports different discovery modes.
You select a discovery mode by adding one of the following supported prefixes to the address:

- **`dns+`**<br />
  The domain name after the prefix is looked up as an A/AAAA query. For example: `dns+memcached.local:11211`.
- **`dnssrv+`**<br />
  The domain name after the prefix is looked up as a SRV query, and then each SRV record is resolved as an A/AAAA record. For example: `dnssrv+_memcached._tcp.memcached.namespace.svc.cluster.local`.
- **`dnssrvnoa+`**<br />
  The domain name after the prefix is looked up as a SRV query, with no A/AAAA lookup made after that. For example: `dnssrvnoa+_memcached._tcp.memcached.namespace.svc.cluster.local`.

If no prefix is provided, the provided IP or hostname is used without pre-resolving it.
