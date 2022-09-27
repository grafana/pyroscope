---
aliases:
  - /docs/mimir/latest/operators-guide/configuring/configuring-hash-rings/
description: Learn how to configure Grafana Mimir hash rings.
menuTitle: Configuring hash rings
title: Configuring Grafana Mimir hash rings
weight: 60
---

# Configuring Grafana Mimir hash rings

[Hash rings]({{< relref "../architecture/hash-ring/index.md" >}}) are a distributed consistent hashing scheme and are widely used by Grafana Mimir for sharding and replication.

Each of the following Grafana Mimir components builds an independent hash ring.
The CLI flags used to configure the hash ring of each component have the following prefixes:

- Ingesters: `-ingester.ring.*`
- Distributors: `-distributor.ring.*`
- Compactors: `-compactor.ring.*`
- Store-gateways: `-store-gateway.sharding-ring.*`
- (Optional) Query-schedulers: `-query-scheduler.ring.*`
- (Optional) Rulers: `-ruler.ring.*`
- (Optional) Alertmanagers: `-alertmanager.sharding-ring.*`

The rest of the documentation refers to these prefixes as `<prefix>`.
You can configure each parameter either via the CLI flag or its respective YAML [config option]({{< relref "reference-configuration-parameters/index.md" >}}).

## Configuring the key-value store

Hash ring data structures need to be shared between Grafana Mimir instances.
To propagate changes to the hash ring, Grafana Mimir uses a [key-value store]({{< relref "../architecture/key-value-store.md" >}}).
The key-value store is required and can be configured independently for the hash rings of different components.

Grafana Mimir supports the following key-value (KV) store backends for hash rings:

- `memberlist` (default)
- `consul`
- `etcd`
- `multi`

You can configure the KV store backend setting the `<prefix>.store` CLI flag (for example, `-ingester.ring.store`).

### Memberlist (default)

By default, Grafana Mimir uses `memberlist` as the KV store backend.

At startup, a Grafana Mimir instance connects to other Mimir replicas to join the cluster.
A Grafana Mimir instance discovers the other replicas to join by resolving the addresses configured in `-memberlist.join`.
The `-memberlist.join` CLI flag must resolve to other replicas in the cluster and can be specified multiple times.

The `-memberlist.join` can be set to:

- An address in the `<ip>:<port>` format.
- An address in the `<hostname>:<port>` format.
- An address in the [DNS service discovery]({{< relref "about-dns-service-discovery.md" >}}) format.

The default port is `7946`.

> **Note**: At a minimum, configure one or more addresses that resolve to a consistent subset of replicas (for example, all the ingesters).

> **Note**: If you're running Grafana Mimir in Kubernetes, define a [headless Kubernetes Service](https://kubernetes.io/docs/concepts/services-networking/service/#headless-services) which resolves to the IP addresses of all Grafana Mimir pods. Then you set `-memberlist.join` to `dnssrv+<service name>.<namespace>.svc.cluster.local:<port>`.

> **Note**: The `memberlist` backend is configured globally and can't be customized on a per-component basis. Because `memberlist` is configured globally, the `memberlist` backend differs from other supported backends, such as Consul or etcd.

Grafana Mimir supports TLS for memberlist connections between its components.
For more information about TLS configuration, refer to [secure communications with TLS]({{< relref "../secure/securing-communications-with-tls.md" >}}).

To see all supported configuration parameters, refer to [memberlist]({{< relref "reference-configuration-parameters/index.md#memberlist" >}}).

#### Configuring the memberlist address and port

By default, Grafana Mimir memberlist protocol listens on address `0.0.0.0` and port `7946`.
If you run multiple Mimir processes on the same node or the port `7946` is not available, you can change the bind and advertise port by setting the following parameters:

- `-memberlist.bind-addr`: IP address to listen on the local machine.
- `-memberlist.bind-port`: Port to listen on the local machine.
- `-memberlist.advertise-addr`: IP address to advertise to other Mimir replicas. The other replicas will connect to this IP to talk to the instance.
- `-memberlist.advertise-port`: Port to advertise to other Mimir replicas. The other replicas will connect to this port to talk to the instance.

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

### Consul

To use [Consul](https://www.consul.io) as a backend KV store, set the following parameters:

- `<prefix>.consul.hostname`: Consul hostname and port separated by colon. For example, `consul:8500`.
- `<prefix>.consul.acl-token`: [ACL token](https://www.consul.io/docs/security/acl/acl-system) used to authenticate to Consul. If Consul authentication is disabled, you can leave the token empty.

To see all supported configuration parameters, refer [consul]({{< relref "reference-configuration-parameters/index.md#consul" >}}).

### Etcd

To use [etcd](https://etcd.io) as a backend KV store, set the following parameters:

- `<prefix>.etcd.endpoints`: etcd hostname and port separated by a colon. For example, `etcd:2379`.
- `<prefix>.etcd.username`: Username used to authenticate to etcd. If etcd authentication is disabled, you can leave the username empty.
- `<prefix>.etcd.password`: Password used to authenticate to etcd. If etcd authentication is disabled, you can leave the password empty.

Grafana Mimir supports TLS between its components and etcd.
For more information about TLS configuration, refer to [secure communications with TLS]({{< relref "../secure/securing-communications-with-tls.md" >}}).

To see all supported configuration parameters, refer to [etcd]({{< relref "reference-configuration-parameters/index.md#etcd" >}}).

### Multi

The `multi` backend is an implementation that you should use only for migrating between two other backends.

The `multi` backend uses two different KV stores: the primary and the secondary.
The primary backend receives all reads and writes.
The secondary backend only receives writes.
The primary and secondary backends can be swapped in real-time, which enables you to switch from one backend to another with no downtime.

You can use the following parameters to configure the multi KV store settings:

- `<prefix>.multi.primary`: The type of primary backend store.
- `<prefix>.multi.secondary`: The type of the secondary backend store.
- `<prefix>.multi.mirror-enabled`: Whether mirroring of writes to the secondary backend store is enabled.
- `<prefix>.multi.mirror-timeout`: The maximum time allowed to mirror a change to the secondary backend store.

> **Note**: Grafana Mimir does not log an error if it is unable to mirror writes to the secondary backend store. The total number of errors is only tracked through the metric `cortex_multikv_mirror_write_errors_total`.

The multi KV primary backend and mirroring can also be configured in the [runtime configuration file]({{< relref "about-runtime-configuration.md" >}}).
Changes to a multi KV Store in the runtime configuration apply to _all_ components using a multi KV store.

The following example shows a runtime configuration file for the multi KV store:

```yaml
multi_kv_config:
  # The runtime configuration only allows to override the primary backend and whether mirroring is enabled.
  primary: consul
  mirror_enabled: true
```

> **Note**: The runtime configuration settings take precedence over CLI flags.

#### Ingester migration example

The following steps show how to migrate ingesters from Consul to etcd:

1. Configure `-ingester.ring.store=multi`, `-ingester.ring.multi.primary=consul`, `-ingester.ring.multi.secondary=etcd`, and `-ingester.ring.multi.mirror-enabled=true`. Configure both Consul settings `-ingester.ring.consul.*` and etcd settings `-ingester.ring.etcd.*`.
1. Apply changes to your Grafana Mimir cluster. After changes have rolled out, Grafana Mimir uses Consul as primary KV store, and all writes are mirrored to etcd too.
1. Configure `primary: etcd` in the `multi_kv_config` block of the [runtime configuration file]({{< relref "about-runtime-configuration.md" >}}). Changes in the runtime configuration file are reloaded live, without the need to restart the process.
1. Wait until all Mimir instances have reloaded the updated configuration.
1. Configure `mirror_enabled: false` in the `multi_kv_config` block of the [runtime configuration file]({{< relref "about-runtime-configuration.md" >}}).
1. Wait until all Mimir instances have reloaded the updated configuration.
1. Configure `-ingester.ring.store=etcd` and remove both the multi and Consul configuration because they are no longer required.
1. Apply changes to your Grafana Mimir cluster. After changes have rolled out, Grafana Mimir only uses etcd.
