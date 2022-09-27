---
aliases:
  - /docs/mimir/latest/operators-guide/configuring/configuring-custom-trackers/
description: Use the custom tracker to count the number of active series on an ingester.
menuTitle: Configuring custom active series trackers
title: Configuring custom active series trackers
weight: 100
---

# Configuring custom active series trackers

You can use the custom tracker feature to count the number of active series on an ingester that match a particular label pattern.

The label pattern to match against is specified using the `-ingester.active-series-custom-trackers` CLI flag (or its respective YAML configuration option). Each custom tracker is defined as a key-value pair, where the key is the name of the tracker and the value is the label matcher. Both the key and the value are type `<string>`.

The following example configures a custom tracker to count the active series coming from `dev` and `prod` namespaces for each tenant.

```yaml
active_series_custom_trackers:
  dev: '{namespace=~"dev-.*"}'
  prod: '{namespace=~"prod-.*"}'
```

If you configure a custom tracker for an ingester, the ingester exposes a `cortex_ingester_active_series_custom_tracker` gauge metric on its [/metrics endpoint]({{< relref "../reference-http-api/index.md#metrics" >}}).

Each custom tracker counts the active series matching its label pattern on a per-tenant basis, which means that each custom tracker generates as many as `# of tenants` series with metric name `cortex_ingester_active_series_custom_tracker`. To reduce the cardinality of this metric, only custom trackers that have matched at least one series are exposed on the metric, and they are removed if they become `0`.

Series with metric name `cortex_ingester_active_series_custom_tracker` have two labels applied: `name` and `user`. The value of the `name` label is the name of the custom tracker specified in the configuration. The value of the `user` label is the tenant-id for which the series count applies.

To illustrate this, assume that two custom trackers are configured as in the preceding YAML snippet, and that your Grafana Mimir cluster has two tenants: `tenant_1` and `tenant_with_only_prod_metrics`. Assume that `tenant_with_only_prod_metrics` has three series with labels that match the pattern `{namespace=~"prod-.*"}` and none that match the patten `{namespace=~"dev-.*"}`. Also assume that `tenant_1` has five series that match the pattern `{namespace=~"dev-.*"}` and 10 series that match the pattern `{namespace=~"prod-.*"}`.

In this example, the following output appears when the `/metrics` endpoint for the ingester component is scraped:

```
cortex_ingester_active_series_custom_tracker{name="dev", user="tenant_1"}                         5
cortex_ingester_active_series_custom_tracker{name="prod", user="tenant_1"}                       10
cortex_ingester_active_series_custom_tracker{name="prod", user="tenant_with_only_prod_metrics"}   3
```

For specific tenants, you can override the default configuration as previously described. To do so, edit the [runtime configuration]({{< relref "./about-runtime-configuration.md" >}}).

You can override the active series custom trackers’ configuration for the tenant `tenant_with_only_prod_metrics` to track two services instead of the default matchers. See the following example:

```
overrides:
  tenant_with_only_prod_metrics:
    active_series_custom_trackers:
      service1: '{service="service1"}'
      service2: '{service="service2"}'
```

After adding this override, and assuming that there is one matching series for `service1` and two matching series for `service2`, the output at `/metrics` changes:

```
cortex_ingester_active_series_custom_tracker{name="dev", user="tenant_1"}                                           5
cortex_ingester_active_series_custom_tracker{name="prod", user="tenant_1"}                                         10
cortex_ingester_active_series_custom_tracker{name="service1", user="tenant_with_only_prod_metrics"}                 1
cortex_ingester_active_series_custom_tracker{name="service2", user="tenant_with_only_prod_metrics"}                 2
```

To set up runtime overrides, refer to [runtime configuration]({{< relref "./about-runtime-configuration.md" >}}).

> **Note:** The custom active series trackers are exposed on each ingester. To understand the count of active series matching a particular label pattern in your Grafana Mimir cluster at a global level, you must collect and sum this metric across all ingesters. If you're running Grafana Mimir with a `replication_factor` > 1, you must also adjust for the fact that the same series will be replicated `RF` times across your ingesters.
