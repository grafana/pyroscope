---
description: Learn about Grafana Fire anonymous usage statistics reporting
menuTitle: About anonymous usage statistics reporting
title: About Grafana Fire anonymous usage statistics reporting
weight: 30
---

# About Grafana Fire anonymous usage statistics reporting

Grafana Fire includes a system that optionally and anonymously reports non-sensitive, non-personally identifiable information about the running Fire cluster to a remote statistics server.
Fire maintainers use this anonymous information to learn more about how the open source community runs Fire and what the Fire team should focus on when working on the next features and documentation improvements.

The anonymous usage statistics reporting is **enabled by default**.
You can opt-out setting the CLI flag `-usage-stats.enabled=false` or its respective YAML configuration option.

## The statistics server

When usage statistics reporting is enabled, information is collected by a server that Grafana Labs runs. Statistics are collected at `https://stats.grafana.org`.

## Which information is collected

When the usage statistics reporting is enabled, Grafana Fire collects the following information:

- Information about the **Fire cluster and version**:
  - A unique, randomly-generated Fire cluster identifier, such as `3749b5e2-b727-4107-95ae-172abac27496`.
  - The timestamp when the anonymous usage statistics reporting was enabled for the first time, and the cluster identifier was created.
  - The Fire version, such as `2.3.0`.
  - The Fire branch, revision, and Golang version that was used to build the binary.
- Information about the **environment** where Fire is running:
  - The operating system, such as `linux`.
  - The architecture, such as `amd64`.
  - The Fire memory utilization and number of goroutines.
  - The number of logical CPU cores available to the Fire process.
- Information about the Fire **configuration**:
  - The `-target` parameter value, such as `all` when running Fire in monolithic mode.
  - The `-blocks-storage.backend` value, such as `s3`.
  - The `-ingester.ring.replication-factor` value, such as `3`.
  - The `-ingester.ring.store` value, such as `memberlist`.
  - The minimum and maximum value of `-ingester.out-of-order-time-window`, which can be overridden on a per-tenant basis (the tenant ID is not shared).
- Information about the Fire **cluster scale**:
  - Ingester:
    - The number of in-memory series.
    - The number of tenants that have in-memory series.
    - The number of tenants that have out-of-order ingestion enabled.
    - The number of samples and exemplars ingested.
  - Querier, _where no information is tracked about the actual request or query_:
    - The number of requests to queriers that are split by API endpoint type:
      - Remote read.
      - Instant query.
      - Range query.
      - Exemplars query.
      - Labels query.
      - Series query.
      - Metadata query.
      - Cardinality analysis query.

> **Note**: Fire maintainers commit to keeping the list of tracked information updated over time, and reporting any change both via the CHANGELOG and the release notes.

## Disable the anonymous usage statistics reporting

If possible, we ask you to keep the usage reporting feature enabled and help us understand more about how the open source community runs Fire.
In case you want to opt-out from anonymous usage statistics reporting, set the CLI flag `-usage-stats.enabled=false` or change the following YAML configuration:

```yaml
usage_stats:
  enabled: false
```
