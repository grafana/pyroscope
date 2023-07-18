---
aliases:
  - /docs/phlare/latest/operators-guide/configuring/configuring-memberlist/
description: Learn how to configure Grafana Phlare memberlist.
menuTitle: Configuring memberlist
title: Configuring Grafana Phlare memberlist
weight: 50
---

# Configuring Grafana Phlare memberlist

[Hash rings]({{< relref "../reference-phlare-architecture/hash-ring/index.md" >}}) are a distributed consistent hashing scheme and are widely used by Grafana Phlare for sharding and replication.

Grafana Phlare only support hash ring via memberlist protocol.

You can configure memberlist either via the CLI flag or its respective YAML [config option]({{< relref "reference-configuration-parameters/index.md#memberlist" >}}).

## Memberlist

Grafana Phlare uses `memberlist` as the KV store backend.

At startup, a Grafana Phlare instance connects to other Phlare replicas to join the cluster.
A Grafana Phlare instance discovers the other replicas to join by resolving the addresses configured in `-memberlist.join`.
The `-memberlist.join` CLI flag must resolve to other replicas in the cluster and can be specified multiple times.

The `-memberlist.join` can be set to:

- An address in the `<ip>:<port>` format.
- An address in the `<hostname>:<port>` format.
- An address in the [DNS service discovery](#supported-discovery-modes) format.

The default port is `7946`.

> **Note**: At a minimum, configure one or more addresses that resolve to a consistent subset of replicas (for example, all the ingesters).

> **Note**: If you're running Grafana Phlare in Kubernetes, define a [headless Kubernetes Service](https://kubernetes.io/docs/concepts/services-networking/service/#headless-services) which resolves to the IP addresses of all Grafana Phlare pods. Then you set `-memberlist.join` to `dnssrv+<service name>.<namespace>.svc.cluster.local:<port>`.

Grafana Phlare supports TLS for memberlist connections between its components.

To see all supported configuration parameters, refer to [memberlist]({{< relref "reference-configuration-parameters/index.md#memberlist" >}}).

#### Configuring the memberlist address and port

By default, Grafana Phlare memberlist protocol listens on address `0.0.0.0` and port `7946`.
If you run multiple Phlare processes on the same node or the port `7946` is not available, you can change the bind and advertise port by setting the following parameters:

- `-memberlist.bind-addr`: IP address to listen on the local machine.
- `-memberlist.bind-port`: Port to listen on the local machine.
- `-memberlist.advertise-addr`: IP address to advertise to other Phlare replicas. The other replicas will connect to this IP to talk to the instance.
- `-memberlist.advertise-port`: Port to advertise to other Phlare replicas. The other replicas will connect to this port to talk to the instance.

### Fine tuning memberlist changes propagation latency

The `phlare_ring_oldest_member_timestamp` metric can be used to measure the propagation of hash ring changes.
This metric tracks the oldest heartbeat timestamp across all instances in the ring.
You can execute the following query to measure the age of the oldest heartbeat timestamp in the ring:

```promql
max(time() - phlare_ring_oldest_member_timestamp{state="ACTIVE"})
```

The measured age shouldn't be higher than the configured `<prefix>.heartbeat-period` plus a reasonable delta (for example, 15 seconds).
If you experience a higher changes propagation latency, you can adjust the following settings:

- Decrease `-memberlist.gossip-interval`
- Increase `-memberlist.gossip-nodes`
- Decrease `-memberlist.pullpush-interval`
- Increase `-memberlist.retransmit-factor`

## About Grafana Phlare DNS service discovery

Some clients in Grafana Phlare support service discovery via DNS to locate the addresses of backend servers to connect to. The following clients support service discovery via DNS:

- [Memberlist KV store]({{< relref "reference-configuration-parameters/index.md#memberlist" >}})
  - `-memberlist.join`

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
