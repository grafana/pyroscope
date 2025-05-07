---
description: Learn about Grafana Pyroscope anonymous usage statistics reporting
menuTitle: Anonymous usage statistics reporting
title: About Grafana Pyroscope anonymous usage statistics reporting
weight: 30
---

# About Grafana Pyroscope anonymous usage statistics reporting

By default, Pyroscope reports anonymous, non-sensitive, non-personally identifiable information about the running cluster to a remote statistics server.
Pyroscope maintainers use this anonymous information to learn more about how the open source community runs Pyroscope and what the Pyroscope team should focus on when working on the next features and documentation improvements.

The anonymous usage statistics reporting is **enabled by default**.
You can opt-out setting the CLI flag `-usage-stats.enabled=false` or its respective YAML configuration option.

## The statistics server

When usage statistics reporting is enabled, information is collected by a server that Grafana Labs runs. Statistics are collected at `https://stats.grafana.org`.

## Which information is collected

When the usage statistics reporting is enabled, Grafana Pyroscope collects the following information:

- Information about the **Pyroscope cluster and version**:
  - A unique, randomly-generated Pyroscope cluster identifier, such as `3749b5e2-b727-4107-95ae-172abac27496`.
  - The timestamp when the anonymous usage statistics reporting was enabled for the first time, and the cluster identifier was created.
  - The Pyroscope version, such as `1.13.1`.
  - The Pyroscope branch, revision, and Golang version that was used to build the binary.
- Information about the **environment** where Pyroscope is running:
  - The operating system, such as `linux`.
  - The architecture, such as `amd64`.
  - The Pyroscope memory utilization and number of goroutines.
  - The number of logical CPU cores available to the Pyroscope process.
- Information about the Pyroscope **configuration**:
  - The `-target` parameter value, such as `all` when running Pyroscope in monolithic mode.
  - The `-storage.backend` value, such as `s3`.
  - The `-distributor.replication-factor` value, such as `3`.
- Information about the Pyroscope **cluster scale**:
  - Distributor:
    - Bytes received.
    - Profiles received with breakdown by profile type and programming language.
    - Profile sizes with breakdown by programming language.
  - Ingester:
    - Number of active tenants.


{{< admonition type="note" >}}
Pyroscope maintainers commit to keeping the list of tracked information updated over time, and reporting any change both via the CHANGELOG and the release notes.
{{< /admonition >}}

## Disable the anonymous usage statistics reporting

If possible, we ask you to keep the usage reporting feature enabled and help us understand more about how the open source community runs Pyroscope.
In case you want to opt-out from anonymous usage statistics reporting, set the CLI flag `-usage-stats.enabled=false` or change the following YAML configuration:

```yaml
analytics:
  reporting_enabled: false
```
