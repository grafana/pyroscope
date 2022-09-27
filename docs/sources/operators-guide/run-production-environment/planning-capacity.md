---
aliases:
  - /docs/mimir/latest/operators-guide/running-production-environment/planning-capacity/
description: Learn how to plan the resources required to deploy Grafana Mimir.
menuTitle: Planning capacity
title: Planning Grafana Mimir capacity
weight: 10
---

# Planning Grafana Mimir capacity

The information that follows is an overview about the CPU, memory, and disk space that Grafana Mimir requires at scale.
You can get a rough idea about the required resources, rather than a prescriptive recommendation about the exact amount of CPU, memory, and disk space.

The resources utilization is estimated based on a general production workload, and the assumption
is that Grafana Mimir is running with one tenant and the default configuration.
Your real resources’ utilization likely differs, because it is based on actual data, configuration settings, and traffic patterns.
For example, the real resources’ utilization might differ based on the actual number
or length of series' labels, or the percentage of queries that reach the store-gateway.

The resources’ utilization are the minimum requirements.
To gracefully handle traffic peaks, run Grafana Mimir with 50% extra capacity for memory and disk.

## Monolithic mode

When Grafana Mimir is running in monolithic mode, you can estimate the required resources by summing up all of the requirements for each Grafana Mimir component.
For more information about per component requirements, refer to [Microservices mode](#microservices-mode).

## Microservices mode

When Grafana Mimir is running in microservices mode, you can estimate the required resources of each component individually.

### Distributor

The [distributor]({{< relref "../architecture/components/distributor.md" >}}) component resources utilization is determined by the number of received samples per second.

Estimated required CPU and memory:

- CPU: 1 core every 25,000 samples per second.
- Memory: 1GB every 25,000 samples per second.

**How to estimate the rate of samples per second:**

1. Query the number of active series across all of your Prometheus servers:
   ```
   sum(prometheus_tsdb_head_series)
   ```
1. Check the [scrape_interval](https://prometheus.io/docs/prometheus/latest/configuration/configuration/) that you configured in Prometheus.
1. Estimate the rate of samples per second by using the following formula:
   ```
   estimated rate = (<active series> * (60 / <scrape interval in seconds>)) / 60
   ```

### Ingester

The [ingester]({{< relref "../architecture/components/ingester.md" >}}) component resources’ utilization is determined by the number of series that are in memory.

Estimated required CPU, memory, and disk space:

- CPU: 1 core for every 300,000 series in memory
- Memory: 2.5GB for every 300,000 series in memory
- Disk space: 5GB for every 300,000 series in memory

[//]: # "We estimated a scrape interval of 15s."

**How to estimate the number of series in memory:**

1. Query the number of active series across all your Prometheus servers:
   ```
   sum(prometheus_tsdb_head_series)
   ```
1. Check the configured `-ingester.ring.replication-factor` (defaults to `3`)
1. Estimate the total number of series in memory across all ingesters using the following formula:
   ```
   total number of in-memory series = <active series> * <replication factor>
   ```

### Query-frontend

The [query-frontend]({{< relref "../architecture/components/query-frontend/index.md" >}}) component resources utilization is determined by the number of queries per second.

Estimated required CPU and memory:

- CPU: 1 core for every 250 queries per second
- Memory: 1GB for every 250 queries per second

### (Optional) Query-scheduler

The [query-scheduler]({{< relref "../architecture/components/query-scheduler/index.md" >}}) component resources’ utilization is determined by the number of queries per second.

Estimated required CPU and memory:

- CPU: 1 core for every 500 queries per second
- Memory: 100MB for every 500 queries per second

### Querier

The [querier]({{< relref "../architecture/components/querier.md" >}}) component resources utilization is determined by the number of queries per second.

Estimated required CPU and memory:

- CPU: 1 core for every 10 queries per second
- Memory: 1GB for every 10 queries per second

> **Note:** The estimate is 1 CPU core and 1GB per query, with an average query latency of 100ms.

### Store-gateway

The [store-gateway]({{< relref "../architecture/components/store-gateway.md" >}}) component resources’ utilization is determined by the number of queries per second and active series before ingesters replication.

Estimated required CPU, memory, and disk space:

- CPU: 1 core every 10 queries per second
- Memory: 1GB every 10 queries per second
- Disk: 13GB every 1 million active series

> **Note:** The CPU and memory requirements are computed by estimating 1 CPU core and 1GB per query, an average query latency of 1s when reaching the store-gateway, and only 10% of queries reaching the store-gateway.

> **Note**: The disk requirement has been estimated assuming 2 bytes per sample for compacted blocks (both index and chunks), the index-header being 0.10% of a block size, a scrape interval of 15 seconds, a retention of 1 year and store-gateways replication factor configured to 3. The resulting estimated store-gateway disk space for one series is 13KB.

**How to estimate the number of active series before ingesters replication:**

1. Query the number of active series across all your Prometheus servers:
   ```
   sum(prometheus_tsdb_head_series)
   ```

### (Optional) Ruler

The [ruler]({{< relref "../architecture/components/ruler/index.md" >}}) component resources utilization is determined by the number of rules evaluated per second.

When [internal]({{< relref "../architecture/components/ruler/index.md#internal" >}}) mode is used (default), rules evaluation is computationally equal to queries execution, so the querier resources recommendations apply to ruler too.

When [remote]({{< relref "../architecture/components/ruler/index.md#internal" >}}) operational mode is used, most of the computational load is shifted to query-frontend and querier components. So those should be scaled accordingly to deal both with queries and rules evaluation workload.

### Compactor

The [compactor]({{< relref "../architecture/components/compactor/index.md" >}}) component resources utilization is determined by the number of active series.

The compactor can scale horizontally both in Grafana Mimir clusters with one tenant and multiple tenants.
We recommend to run at least one compactor instance every 20 million active series ingested in total in the Grafana Mimir cluster, calculated before ingesters replication.

Assuming you run one compactor instance every 20 million active series, the estimated required CPU, memory and disk for each compactor instance are:

- CPU: 1 core
- Memory: 4GB
- Disk: 300GB

For more information about disk requirements, refer to [Compactor disk utilization]({{< relref "../architecture/components/compactor/index.md#compactor-disk-utilization" >}}).

**To estimate the number of active series before ingesters replication, query the number of active series across all Prometheus servers:**

```
sum(prometheus_tsdb_head_series)
```

### (Optional) Alertmanager

The [Alertmanager]({{< relref "../architecture/components/alertmanager.md" >}}) component resources’ utilization is determined by the number of alerts firing at the same time.

Estimated required CPU and memory:

- CPU: 1 CPU core for every 100 firing alerts
- Memory: 1GB for every 100 firing alerts
