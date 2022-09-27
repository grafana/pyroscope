---
aliases:
  - /docs/mimir/latest/operators-guide/configuring/configuring-high-availability-deduplication/
description: Learn how to configure Grafana Mimir to handle HA Prometheus server deduplication.
menuTitle: Configuring high-availability deduplication
title: Configuring Grafana Mimir high-availability deduplication
weight: 70
---

# Configuring Grafana Mimir high-availability deduplication

You can have more than one Prometheus instance that scrapes the same metrics for redundancy. Grafana Mimir already performs replication for redundancy,
so you do not need to ingest the same data twice. In Grafana Mimir, you can deduplicate the data that you receive from HA pairs of Prometheus instances.

Assume that there are two teams, each running their own Prometheus instance, which monitors different services: Prometheus `team-1` and Prometheus `team-2`.
If the teams are running Prometheus HA pairs, the individual Prometheus instances would be `team-1.a` and `team-1.b`, and `team-2.a` and `team-2.b`.

Grafana Mimir only ingests from either `team-1.a` or `team-1.b`, and only from `team-2.a` or `team-2.b`. It does this by electing a leader replica for each
Prometheus server. For example, in the case of `team-1`, the leader replica would be `team-1.a`. As long as `team-1.a` is the leader, the samples
that `team-1.b` sends are dropped. And if Grafana Mimir does not see any new samples from `team-1.a` for a short period of time (30 seconds by default), it switches the leader to `team-1.b`.

If `team-1.a` goes down for more than 30 seconds, Grafana Mimir’s HA sample handling will have switched and elected `team-1.b` as the leader. The failure
timeout ensures that too much data is not dropped before failover to the other replica.

> **Note:** In a scenario where the default scrape period is 15 seconds, and the timeouts in Grafana Mimir are set to the default values,
> when a leader-election failover occurs, you'll likely only lose a single scrape of data. For any query using the `rate()` function, make the rate time interval
> at least four times that of the scrape period to account for any of these failover scenarios.
> For example, with the default scrape period of 15 seconds, use a rate time-interval at least 1-minute.

## Distributor high-availability (HA) tracker

The [distributor]({{< relref "../architecture/components/distributor.md" >}}) includes a high-availability (HA) tracker.

The HA tracker deduplicates incoming samples based on a cluster and replica label expected on each incoming series.
The cluster label uniquely identifies the cluster of redundant Prometheus servers for a given tenant.
The replica label uniquely identifies the replica within the Prometheus cluster.
Incoming samples are considered duplicated (and thus dropped) if they are received from any replica that is not the currently elected leader within a cluster.

If the HA tracker is enabled but incoming samples contain only one or none of the cluster and replica labels, these samples are accepted by default and never deduplicated.

> Note: for performance reasons, the HA tracker only checks the cluster and replica label of the first series in the request to determine whether all series in the request should be deduplicated. This assumes that all series inside the request have the same cluster and replica labels, which is typically true when Prometheus is configured with external labels. Ensure this requirement is honored if you have a non-standard Prometheus setup (for example, you're using Prometheus federation or have a metrics proxy in between).

## Configuration

This section includes information about how to configure Prometheus and Grafana Mimir.

### How to configure Prometheus

To configure Prometheus, set two identifiers for each Prometheus server, one for the cluster. For example, set `team-1` or `team-2`, and one to identify the replica in the cluster, for example `a` or `b`.
The easiest approach is to set [external labels](https://prometheus.io/docs/prometheus/latest/configuration/configuration/). The default labels are `cluster` and `__replica__`.

The following example shows how to set identifiers in Prometheus:

```
global:
  external_labels:
    cluster: prom-team1
    __replica__: replica1
```

and

```
global:
  external_labels:
    cluster: prom-team1
    __replica__: replica2
```

> **Note:** The preceding labels are external labels and have nothing to do with `remote_write` configuration.

These two label names are configurable on a per-tenant basis within Grafana Mimir. For example, if the label name of one cluster is used by
some workloads, set the label name of another cluster to something else that uniquely identifies the second cluster.

Set the replica label so that the value for each Prometheus cluster is unique in that cluster.

> **Note:** Grafana Mimir drops this label when ingesting data, but preserves the cluster label. This way, your timeseries won't change when replicas change.

### How to configure Grafana Mimir

The minimal configuration required is as follows:

1. Enable the HA tracker.
1. Configure the HA tracker KV store.
1. Configure expected label names for each cluster and its replica.

#### Enable the HA tracker

To enable the HA tracker feature, set the `-distributor.ha-tracker.enable=true` CLI flag (or its YAML configuration option) in the distributor.

Next, decide whether you want to enable it for all tenants or just a subset of tenants.
To enable it for all tenants, set `-distributor.ha-tracker.enable-for-all-users=true`.
Alternatively, you can enable the HA tracker only on a per-tenant basis, keeping the default `-distributor.ha-tracker.enable-for-all-users=false` and overriding it on a per-tenant basis setting `accept_ha_samples` in the overrides section of the runtime configuration.

#### Configure the HA tracker KV store

The HA tracker requires a key-value (KV) store to coordinate which replica is currently elected.
The supported KV stores for the HA tracker are `consul` and `etcd`.

> **Note:** `memberlist` is not supported. Memberlist-based KV stores propagate updates using the Gossip protocol, which is too slow for the
> HA tracker. The result would be that different distributors might see a different Prometheus server elected as leaders at the same time.

The following CLI flags (and their respective YAML configuration options) are available for configuring the HA tracker KV store:

- `-distributor.ha-tracker.store`: The backend storage to use, which is either `consul` or `etcd`.
- `-distributor.ha-tracker.consul.*`: The Consul client configuration. Only use this if you have defined `consul` as your backend storage.
- `-distributor.ha-tracker.etcd.*`: The etcd client configuration. Only use this if you have defined `etcd` as your backend storage.

#### Configure expected label names for each Prometheus cluster and replica

The HA tracker deduplicates incoming series that have cluster and replica labels.
You can configure the name of these labels either globally or on a per-tenant basis.

Configure the default cluster and replica label names using the following CLI flags (or their respective YAML configuration options):

- `-distributor.ha-tracker.cluster`: Name of the label whose value uniquely identifies a Prometheus HA cluster (defaults to `cluster`).
- `-distributor.ha-tracker.replica`: Name of the label whose value uniquely identifies a Prometheus replica within the HA cluster (defaults to `__replica__`).

> **Note:** The HA label names can be overridden on a per-tenant basis by setting `ha_cluster_label` and `ha_replica_label` in the overrides section of the runtime configuration.

#### Example configuration

The following configuration example snippet enables the HA tracker for all tenants via a YAML configuration file:

```yaml
limits:
  accept_ha_samples: true
distributor:
  ha_tracker:
    enable_ha_tracker: true
    kvstore:
      [store: <string> | default = "consul"]
      [consul | etcd: <config>]
```

For more information, see [distributor]({{< relref "reference-configuration-parameters/index.md#distributor" >}}). The HA tracker flags are prefixed with `-distributor.ha-tracker.*`.
