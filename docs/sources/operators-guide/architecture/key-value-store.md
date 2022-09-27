---
title: "Grafana Mimir key-value store"
menuTitle: "Key-value store"
description: "The key-value store is a database that stores data indexed by key."
weight: 70
---

# Grafana Mimir key-value store

A key-value (KV) store is a database that stores data indexed by key.
Grafana Mimir requires a key-value store for the following features:

- [Hash ring]({{< relref "hash-ring/index.md" >}})
- [(Optional) Distributor high-availability tracker]({{< relref "../configure/configuring-high-availability-deduplication.md" >}})

## Supported key-value store backends

Grafana Mimir supports the following key-value (KV) store backends:

- Gossip-based [memberlist](https://github.com/hashicorp/memberlist) protocol (default)
- [Consul](https://www.consul.io)
- [Etcd](https://etcd.io)

### Gossip-based memberlist protocol (default)

By default, Grafana Mimir instances use a Gossip-based protocol to join a memberlist cluster.
The data is shared between the instances using peer-to-peer communication and no external dependency is required.

We recommend that you use memberlist to run Grafana Mimir.

To configure memberlist, refer to [configuring hash rings]({{< relref "../configure/configuring-hash-rings.md" >}}).

### Consul

Grafana Mimir supports [Consul](https://www.consul.io) as a backend KV store.
If you want to use Consul, you must install it. The Grafana Mimir installation does not include Consul.

To configure Consul, refer to [configuring hash rings]({{< relref "../configure/configuring-hash-rings.md" >}}).

### Etcd

Grafana Mimir supports [etcd](https://etcd.io) as a backend KV store.
If you want to use etcd, you must install it. The Grafana Mimir installation does not include etcd.

To configure etcd, refer to [configuring hash rings]({{< relref "../configure/configuring-hash-rings.md" >}}).
