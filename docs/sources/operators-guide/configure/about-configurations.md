---
aliases:
  - /docs/mimir/latest/operators-guide/configuring/about-configurations/
description: Learn about Grafana Mimir configuration and key guidelines to consider.
menuTitle: About configurations
title: About Grafana Mimir configurations
weight: 10
---

# About Grafana Mimir configurations

You can configure Grafana Mimir via a ([YAML](https://en.wikipedia.org/wiki/YAML)-based) configuration file or CLI (command-line-interface) flags. It is best to specify your configuration via the configuration file rather than CLI flags. Every parameter that is set in the configuration file can also be set via a corresponding CLI flag. If you specify both CLI flags and configuration parameters, CLI flags take precedence over corresponding values in a configuration file. You can specify the configuration file by using the `-config.file` CLI flag.

To see the most common CLI flags that you need to get started with Grafana Mimir, run the `mimir -help` command. To see all of the available CLI flags, run the `mimir -help-all` command.

A given configuration loads at startup and cannot be modified at runtime. However, Grafana Mimir does have a second configuration file, known as the _runtime configuration_, that is dynamically reloaded. For more information, see [About runtime configuration]({{< relref "about-runtime-configuration.md" >}}).

To see the current configuration state of any component, use the [`/config`]({{< relref "../reference-http-api/index.md#configuration" >}}) or [`/runtime_config`]({{< relref "../reference-http-api/index.md#runtime-configuration" >}}) HTTP API endpoint.

## Common configurations

Some configurations, such as object storage backend, are repeated for multiple components.
To avoid repetition in the configuration file, use the [`common`]({{< relref "../configure/reference-configuration-parameters/index.md#common" >}}) configuration section or `-common.*` CLI flags.
Common configurations are first applied to all of the specific configurations, which allows the common configurations to be overridden later by specific values.

For example, the following configuration uses the same Amazon S3 object storage bucket called `mimir`. The common storage is located in the `us-east` region for both the ruler and alertmanager stores, and the blocks storage uses the `mimir-blocks` bucket from the same region:

```yaml
common:
  storage:
    backend: s3
    s3:
      region: us-east
      bucket_name: mimir

blocks_storage:
  s3:
    bucket_name: mimir-blocks
```

For a reference of this configuration, see [Configure Grafana Mimir object storage backend]({{< relref "configure-object-storage-backend.md" >}}).

The precedence of the common configuration is as follows, where each configuration overrides the previous one:

- YAML common values
- YAML specific values
- CLI common flags
- CLI specific flags

## Operational considerations

Use a single configuration file, and either pass it to all replicas of Grafana Mimir (if you are running multiple single-process Mimir replicas) or to all components of Grafana Mimir (if you are running Grafana Mimir as microservices). If you are running Grafana Mimir on Kubernetes, you can achieve this by storing the configuration file in a [ConfigMap](https://kubernetes.io/docs/concepts/configuration/configmap/) and mounting it in each Grafana Mimir container.

This recommendation helps to avoid a common misconfiguration pitfall: while certain configuration parameters might look like they’re only needed by one type of component, they might in fact be used by multiple components. For example, the `-ingester.ring.replication-factor` CLI flag is not only required by ingesters, but also by distributors, queriers, and rulers (in [internal]({{< relref "../architecture/components/ruler/index.md#internal" >}}) operational mode).

By using a single configuration file, you ensure that each component gets all of the configuration that it needs without needing to track which parameter belongs to which component.
There is no harm in passing a configuration that is specific to one component (such as an ingester) to another component (such as a querier). In such case, the configuration is simply ignored.

If you need to, you can use advanced CLI flags to override specific values on a particular Grafana Mimir component or replica. This can be helpful if you want to change a parameter that is specific to a certain component, without having to do a full restart of all other components.

The most common use case for CLI flags is to use the `-target` flag to run Grafana Mimir as microservices. By setting the `-target` CLI flag, all Grafana Mimir components share the same configuration file, but you can make them behave as a given component by specifying a `-target` command-line value, such as `-target=ingester` or `-target=querier`.
