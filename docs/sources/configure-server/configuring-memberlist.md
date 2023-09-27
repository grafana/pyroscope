---
description: Learn how to configure Pyroscope memberlist.
menuTitle: Configuring memberlist
title: Configuring Pyroscope memberlist
weight: 50
aliases:
  - /docs/phlare/latest/operators-guide/configuring/configuring-memberlist/
  - /docs/phlare/latest/configure-server/configuring-memberlist/
---

# Configuring Pyroscope memberlist

[Hash rings]({{< relref "../reference-pyroscope-architecture/hash-ring/index.md" >}}) are a distributed consistent hashing scheme and are widely used by Pyroscope for sharding and replication.
Pyroscope only supports hash ring via the memberlist protocol.
You can configure memberlist by either the CLI flag or its respective YAML [config option]({{< relref "./reference-configuration-parameters/index.md#memberlist" >}}).

## Memberlist

Pyroscope uses `memberlist` as the KV store backend.
At startup, a Pyroscope instance connects to other Pyroscope replicas to join the cluster.
A Pyroscope instance discovers the other replicas to join by resolving the addresses configured in `-memberlist.join`.
The `-memberlist.join` CLI flag must resolve to other replicas in the cluster and can be specified multiple times.

The `-memberlist.join` can be set to an address in the following formats:

- `<ip>:<port>`
- `<hostname>:<port>`
- [DNS service discovery](#supported-discovery-modes)

> **Note**: At a minimum, configure one or more addresses that resolve to a consistent subset of replicas (for example, all the ingesters).

> **Note**: If you're running Pyroscope in Kubernetes, define a [headless Kubernetes Service](https://kubernetes.io/docs/concepts/services-networking/service/#headless-services) which resolves to the IP addresses of all Pyroscope pods, then set `-memberlist.join` to `dnssrv+<service name>.<namespace>.svc.cluster.local:<port>`.

Pyroscope supports TLS for memberlist connections between its components.
To see all supported configuration parameters, refer to [memberlist]({{< relref "./reference-configuration-parameters/index.md#memberlist" >}}).

#### Configuring the memberlist address and port

By default, Pyroscope memberlist protocol listens on address `0.0.0.0` and port `7946`.
If you run multiple Pyroscope processes on the same node or the port `7946` is not available, you can change the bind and advertise port by setting the following parameters:

- `-memberlist.bind-addr`: IP address to listen on the local machine.
- `-memberlist.bind-port`: Port to listen on the local machine.
- `-memberlist.advertise-addr`: IP address to advertise to other Pyroscope replicas. The other replicas will connect to this IP to talk to the instance.
- `-memberlist.advertise-port`: Port to advertise to other Pyroscope replicas. The other replicas will connect to this port to talk to the instance.

### Fine tuning memberlist changes propagation latency

The `pyroscope_ring_oldest_member_timestamp` metric can be used to measure the propagation of hash ring changes.
This metric tracks the oldest heartbeat timestamp across all instances in the ring.
You can execute the following query to measure the age of the oldest heartbeat timestamp in the ring:

```promql
max(time() - pyroscope_ring_oldest_member_timestamp{state="ACTIVE"})
```

The measured age shouldn't be higher than the configured `<prefix>.heartbeat-period` plus a reasonable delta (for example, 15 seconds).
If you experience a higher changes propagation latency, you can adjust the following settings:

- Decrease `-memberlist.gossip-interval`
- Increase `-memberlist.gossip-nodes`
- Decrease `-memberlist.pullpush-interval`
- Increase `-memberlist.retransmit-factor`

## About Pyroscope DNS service discovery

Some clients in Pyroscope support service discovery via DNS to locate the addresses of backend servers to connect to. The following clients support service discovery via DNS:

- [Memberlist KV store]({{< relref "./reference-configuration-parameters/index.md#memberlist" >}})
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
