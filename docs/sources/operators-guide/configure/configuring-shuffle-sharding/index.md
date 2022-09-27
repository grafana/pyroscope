---
aliases:
  - /docs/mimir/latest/operators-guide/configuring/configuring-shuffle-sharding/
description: Learn how to configure shuffle sharding.
menuTitle: Configuring shuffle sharding
title: Configuring Grafana Mimir shuffle sharding
weight: 80
---

# Configuring Grafana Mimir shuffle sharding

Grafana Mimir leverages sharding to horizontally scale both single- and multi-tenant clusters beyond the capacity of a single node.

## Background

Grafana Mimir uses a sharding strategy that distributes the workload across a subset of the instances that run a given component.
For example, on the write path, each tenant's series are sharded across a subset of the ingesters.
The size of this subset, which is the number of instances, is configured using the `shard size` parameter, which by default is `0`.
This default value means that each tenant uses all available instances, in order to fairly balance resources such as CPU and memory usage, and to maximize the usage of these resources across the cluster.

In a multi-tenant cluster this default (`0`) value introduces the following downsides:

- An outage affects all tenants.
- A misbehaving tenant, for example, a tenant that causes an out-of-memory error, can negatively affect all other tenants.

Configuring a shard size value higher than `0` enables shuffle sharding. The goal of shuffle sharding is to reduce the blast radius of an outage and better isolate tenants.

## About shuffle sharding

Shuffle sharding is a technique that isolates different tenant's workloads and gives each tenant a single-tenant experience, even if they're running in a shared cluster.
For more information about how AWS describes shuffle sharding, refer to [What is shuffle sharding?](https://aws.amazon.com/builders-library/workload-isolation-using-shuffle-sharding/).

Shuffle sharding assigns each tenant a shard that is composed of a subset of the Grafana Mimir instances.
This technique minimizes the number of overlapping instances between two tenants.
Shuffle sharding provides the following benefits:

- An outage on some Grafana Mimir cluster instances or nodes only affect a subset of tenants.
- A misbehaving tenant only affects its shard instances.
  Assuming that each tenant shard is relatively small compared to the total number of instances in the cluster, it’s likely that any other tenant runs on different instances or that only a subset of instances match the affected instances.

Using shuffle sharding doesn’t require more resources, but can result in unbalanced instances.

### Low overlapping instances probability

For example, in a Grafana Mimir cluster that runs 50 ingesters and assigns each tenant four out of 50 ingesters, by shuffling instances between each tenant, there are 230,000 possible combinations.

Randomly picking two tenants yields the following probabilities:

- 71% chance that they do not share any instance
- 26% chance that they share only 1 instance
- 2.7% chance that they share 2 instances
- 0.08% chance that they share 3 instances
- 0.0004% chance that their instances fully overlap

![Shuffle sharding probability](shuffle-sharding-probability.png)

[//]: # "Diagram source of shuffle-sharding probability at https://docs.google.com/spreadsheets/d/1FXbiWTXi6bdERtamH-IfmpgFq1fNL4GP_KX_yJvbRi4/edit"

## Grafana Mimir shuffle sharding

Grafana Mimir supports shuffle sharding in the following components:

- [Ingesters](#ingesters-shuffle-sharding)
- [Query-frontend / Query-scheduler](#query-frontend-and-query-scheduler-shuffle-sharding)
- [Store-gateway](#store-gateway-shuffle-sharding)
- [Ruler](#ruler-shuffle-sharding)
- [Compactor](#compactor-shuffle-sharding)
- [Alertmanager](#alertmanager-shuffle-sharding)

When you run Grafana Mimir with the default configuration, shuffle sharding is disabled and you need to explicitly enable it by increasing the shard size either globally or for a given tenant.

> **Note:** If the shard size value is equal to or higher than the number of available instances, for example where `-distributor.ingestion-tenant-shard-size` is higher than the number of ingesters, then shuffle sharding is disabled and all instances are used again.

### Guaranteed properties

The Grafana Mimir shuffle sharding implementation provides the following benefits:

- **Stability**<br />
  Given a consistent state of the hash ring, the shuffle sharding algorithm always selects the same instances for a given tenant, even across different machines.
- **Consistency**<br />
  Adding or removing an instance from the hash ring leads to, at most, only one instance changed in each tenant's shard.
- **Shuffling**<br />
  Probabilistically and for a large enough cluster, shuffle sharding ensures that every tenant receives a different set of instances with a reduced number of overlapping instances between two tenants, which improves failure isolation.
- **Zone-awareness**<br />
  When you enable [zone-aware replication]({{< relref "../configuring-zone-aware-replication.md" >}}), the subset of instances selected for each tenant contains a balanced number of instances for each availability zone.

### Ingesters shuffle sharding

By default, the Grafana Mimir distributor divides the received series among all running ingesters.

When you enable ingester shuffle sharding, the distributor and ruler on the write path divide each tenant series among `-distributor.ingestion-tenant-shard-size` number of ingesters, while on the read path, the querier and ruler queries only the subset of ingesters that hold the series for a given tenant.

The shard size can be overridden on a per-tenant basis by setting `ingestion_tenant_shard_size` in the overrides section of the runtime configuration.

#### Ingesters write path

To enable shuffle sharding for ingesters on the write path, configure the following flags (or their respective YAML configuration options) on the distributor, ingester, and ruler:

- `-distributor.ingestion-tenant-shard-size=<size>`<br />
  `<size>`: Set the size to the number of ingesters each tenant series should be sharded to. If `<size>` is `0` or is greater than the number of available ingesters in the Grafana Mimir cluster, the tenant series are sharded across all ingesters.

#### Ingesters read path

Assuming that you have enabled shuffle sharding for the write path, to enable shuffle sharding for ingesters on the read path, configure the following flags (or their respective YAML configuration options) on the querier and ruler:

- `-distributor.ingestion-tenant-shard-size=<size>`

The following flags are set appropriately by default to enable shuffle sharding for ingesters on the read path. If you need to modify their defaults:

- `-querier.shuffle-sharding-ingesters-enabled=true`<br />
  Shuffle sharding for ingesters on the read path can be explicitly enabled or disabled.
- `-querier.query-ingesters-within=<period>`<br />
  Queriers and rulers fetch in-memory series from the minimum set of required ingesters, selecting only ingesters which might have received series since 'now - query ingesters within'. If this period is `0`, shuffle sharding for ingesters on the read path is disabled, which means all ingesters in the Mimir cluster are queried for any tenant.
  The configured `<period>` should be:
  - greater than `-querier.query-store-after` and,
  - greater than the estimated minimum amount of time for the oldest samples stored in a block uploaded by ingester to be discovered and available for querying.
    When running Grafana Mimir with the default configuration, the estimated minimum amount of time for the oldest sample in a uploaded block to be available for querying is `3h`.

If you enable ingesters shuffle sharding only for the write path, queriers and rulers on the read path always query all ingesters instead of querying the subset of ingesters that belong to the tenant's shard.
Keeping ingesters shuffle sharding enabled only on the write path does not lead to incorrect query results, but might increase query latency.

#### Rollout strategy

If you’re running a Grafana Mimir cluster with shuffle sharding disabled, and you want to enable it for the ingesters, use the following rollout strategy to avoid missing querying for any series currently in the ingesters:

1. Explicitly disable ingesters shuffle-sharding on the read path via `-querier.shuffle-sharding-ingesters-enabled=false` since this is enabled by default.
1. Enable ingesters shuffle sharding on the write path.
1. Wait for at least the amount of time specified via `-querier.query-ingesters-within`.
1. Enable ingesters shuffle-sharding on the read path via `-querier.shuffle-sharding-ingesters-enabled=true`.

#### Limitation: Decreasing the tenant shard size

The current shuffle sharding implementation in Grafana Mimir has a limitation that prevents you from safely decreasing the tenant shard size when you enable ingesters’ shuffle sharding on the read path.

If a tenant’s shard decreases in size, there is currently no way for the queriers and rulers to know how large the tenant shard was previously, and as a result, they potentially miss an ingester with data for that tenant.
The query-ingesters-within period, which is used to select the ingesters that might have received series since 'now - query ingesters within', doesn't work correctly for finding tenant shards if the tenant shard size is decreased.

Although decreasing the tenant shard size is not supported, consider the following workaround:

1. Disable shuffle sharding on the read path via `-querier.shuffle-sharding-ingesters-enabled=false`.
1. Decrease the configured tenant shard size.
1. Wait for at least the amount of time specified via `-querier.query-ingesters-within`.
1. Re-enable shuffle sharding on the read path via `-querier.shuffle-sharding-ingesters-enabled=true`.

### Query-frontend and query-scheduler shuffle sharding

By default, all Grafana Mimir queriers can execute queries for any tenant.

When you enable shuffle sharding by setting `-query-frontend.max-queriers-per-tenant` (or its respective YAML configuration option) to a value higher than `0` and lower than the number of available queriers, only the specified number of queriers are eligible to execute queries for a given tenant.

Note that this distribution happens in query-frontend, or query-scheduler, if used.
When using query-scheduler, the `-query-frontend.max-queriers-per-tenant` option must be set for the query-scheduler component.
When you don't use query-frontend (with or without query-scheduler), this option is not available.

You can override the maximum number of queriers on a per-tenant basis by setting `max_queriers_per_tenant` in the overrides section of the runtime configuration.

#### The impact of a "query of death"

In the event a tenant sends a "query of death" which causes a querier to crash, the crashed querier becomes disconnected from the query-frontend or query-scheduler, and another running querier is immediately assigned to the tenant's shard.

If the tenant repeatedly sends this query, the new querier assigned to the tenant's shard crashes as well, and yet another querier is assigned to the shard.
This cascading failure can potentially result in all running queriers to crash, one by one, which invalidates the assumption that shuffle sharding contains the blast radius of queries of death.

To mitigate this negative impact, there are experimental configuration options that enable you to configure a time delay between when a querier disconnects due to a crash and when the crashed querier is replaced by a healthy querier.
When you configure a time delay, a tenant that repeatedly sends a "query of death" runs with reduced querier capacity after a querier has crashed.
The tenant could end up having no available queriers, but this configuration reduces the likelihood that the crash impacts other tenants.

A delay of 1 minute might be a reasonable trade-off:

- Query-frontend: `-query-frontend.querier-forget-delay=1m`
- Query-scheduler: `-query-scheduler.querier-forget-delay=1m`

### Store-gateway shuffle sharding

By default, a tenant's blocks are divided among all Grafana Mimir store-gateways.

When you enable store-gateway shuffle sharding by setting `-store-gateway.tenant-shard-size` (or its respective YAML configuration option) to a value higher than `0` and lower than the number of available store-gateways, only the specified number of store-gateways are eligible to load and query blocks for a given tenant.
You must set this flag on the store-gateway, querier, and ruler.

You can override the store-gateway shard size on a per-tenant basis by setting `store_gateway_tenant_shard_size` in the overrides section of the runtime configuration.

For more information about the store-gateway, refer to [store-gateway]({{< relref "../../architecture/components/store-gateway.md" >}}).

### Ruler shuffle sharding

By default, tenant rule groups are sharded by all Grafana Mimir rulers.

When you enable ruler shuffle sharding by setting `-ruler.tenant-shard-size` (or its respective YAML configuration option) to a value higher than `0` and lower than the number of available rulers, only the specified number of rulers are eligible to evaluate rule groups for a given tenant.

You can override the ruler shard size on a per-tenant basis by setting `ruler_tenant_shard_size` in the overrides section of the runtime configuration.

### Compactor shuffle sharding

By default, tenant blocks can be compacted by any Grafana Mimir compactor.

When you enable compactor shuffle sharding by setting `-compactor.compactor-tenant-shard-size` (or its respective YAML configuration option) to a value higher than `0` and lower than the number of available compactors, only the specified number of compactors are eligible to compact blocks for a given tenant.

You can override the compactor shard size on a per-tenant basis setting by `compactor_tenant_shard_size` in the overrides section of the runtime configuration.

### Alertmanager shuffle sharding

Alertmanager only performs distribution across replicas per tenant. The state and workload is not divided any further. The replication factor setting `-alertmanager.sharding-ring.replication-factor` determines how many replicas are used for a tenant.

As a result, shuffle sharding is effectively always enabled for Alertmanager.

### Shuffle sharding impact to the KV store

Shuffle sharding does not add additional overhead to the KV store.
Shards are computed client-side and are not stored in the ring.
KV store sizing depends primarily on the number of replicas of any component that uses the ring, for example, ingesters, and the number of tokens per replica.

However, in some components, each tenant's shard is cached in-memory on the client-side, which might slightly increase their memory footprint. Increased memory footprint can happen mostly in the distributor.
