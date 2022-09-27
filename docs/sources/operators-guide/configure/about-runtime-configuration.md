---
aliases:
  - /docs/mimir/latest/operators-guide/configuring/about-runtime-configuration/
description:
  Runtime configuration enables you to change a subset of configurations
  without restarting Grafana Mimir.
menuTitle: About runtime configuration
title: About Grafana Mimir runtime configuration
weight: 40
---

# About Grafana Mimir runtime configuration

A runtime configuration file is a file that contains configuration parameters, which is periodically reloaded while Mimir is running.
It allows you to change a subset of Grafana Mimir’s configuration without having to restart the Grafana Mimir component or instance.

Runtime configuration is available for a subset of the configuration that was set at startup.
A Grafana Mimir operator can observe the configuration and use runtime configuration to make immediate adjustments to Grafana Mimir.

Runtime configuration values take precedence over command-line options.

If multiple runtime configuration files are specified the runtime config files will be merged in a left to right order.

## Enable runtime configuration

To enable runtime configuration, specify a comma-separated list of file paths upon startup by using the `-runtime-config.file=<filepath>,<filepath>` CLI flag or from within your YAML configuration file in the `runtime_config` block.

By default, Grafana Mimir reloads the contents of these files every 10 seconds and merges these files from left to right. You can configure this interval by using the `-runtime-config.reload-period=<duration>` CLI flag or by specifying the `period` value in your YAML configuration file.

When running Grafana Mimir on Kubernetes, store the runtime configuration files in a [ConfigMap](https://kubernetes.io/docs/concepts/configuration/configmap/) and mount the ConfigMaps in each container.

## Viewing the runtime configuration

Use Grafana Mimir’s `/runtime_config` endpoint to see the current value of the runtime configuration, including the overrides. To see only the non-default values of the configuration, specify the endpoint with `/runtime_config?mode=diff`.

## Runtime configuration of per-tenant limits

The runtime configuration file is primarily used to set and adjust limits that are appropriate for each tenant based on their ingest and query needs.

The values that are defined in the limits section of your YAML configuration define the default set of limits that are applied to tenants. For example, if you set the `ingestion_rate` to `25,000` in your YAML configuration file, any tenant in your cluster that sends more than 25,000 samples per second (SPS) is rate limited.

You can use the runtime configuration file to override this behavior. For example, if you have a tenant (`tenant1`) that needs to send twice as many data points as the current limit, and you have another tenant (`tenant2`) that needs to send three times as many data points, you can modify the contents of your runtime configuration file as follows:

```yaml
overrides:
  tenant1:
    ingestion_rate: 50000
  tenant2:
    ingestion_rate: 75000
```

As a result, Grafana Mimir allows `tenant1` to send 50,000 SPS, and `tenant2` to send 75,000 SPS, while maintaining a 25,000 SPS rate limit on all other tenants.

- On a per-tenant basis, you can override all of the limits listed in the [`limits`]({{< relref "reference-configuration-parameters/index.md#limits" >}}) block within the runtime configuration file.
- For each tenant, you can override different limits.
- For any tenant or limit that is not overridden in the runtime configuration file, you can inherit the limit values that are specified in the `limits` block.

## Ingester instance limits

The runtime configuration file can be used to dynamically adjust Grafana Mimir ingester instance limits. While per-tenant limits are limits applied to each tenant, per-ingester-instance limits are limits applied to each ingester process.
Ingester limits ensure individual ingesters are not overwhelmed, regardless of any per-tenant limits. These limits can be set under the `ingester.instance_limits` block in the global configuration file, with CLI flags, or under the `ingester_limits` field in the runtime configuration file.

The runtime configuration allows you to override initial values, which is useful for advanced operators who need to dynamically change them in response to changes in ingest or query load.

Everything under the `instance_limits` section within the [`ingester`]({{< relref "reference-configuration-parameters/index.md#ingester" >}}) block can be overridden via runtime configuration.
The following example shows a portion of the runtime configuration that changes the ingester limits:

```yaml
ingester_limits:
  max_ingestion_rate: 20000
  max_series: 1500000
  max_tenants: 1000
  max_inflight_push_requests: 30000
```

## Runtime configuration of ingester streaming

An advanced runtime configuration option controls if ingesters transfer encoded chunks (the default) or transfer decoded series to queriers at query time.

The parameter `ingester_stream_chunks_when_using_blocks` might only be used in runtime configuration.
A value of `true` transfers encoded chunks, and a value of `false` transfers decoded series.

> **Note:** We strongly recommend that you use the default setting, which is `true`, except in rare cases where users observe Grafana Mimir rules evaluation slowing down.
