---
title: "(Optional) Grafana Mimir overrides-exporter"
menuTitle: "(Optional) Overrides-exporter"
description: "The overrides-exporter exports Prometheus metrics containing the configured per-tenant limits."
weight: 110
---

# (Optional) Grafana Mimir overrides-exporter

Grafana Mimir supports applying overrides on a per-tenant basis.
A number of overrides configure limits that prevent a single tenant from using too many resources.
The overrides-exporter component exposes limits as Prometheus metrics so that operators can understand how close tenants are to their limits.

For more information about configuring overrides, refer to [Runtime configuration file]({{< relref "../../configure/about-runtime-configuration.md" >}}).

## Running the overrides-exporter

The overrides-exporter must be explicitly enabled.

> **Warning:**
> The metrics emitted by the overrides-exporter have high cardinality.
> It's recommended to run only a single replica of the overrides-exporter to limit that cardinality.

With a `runtime.yaml` file as follows:

<!-- prettier-ignore-start -->
[embedmd]:# (../../../../configurations/overrides-exporter-runtime.yaml)
```yaml
# file: runtime.yaml
# In this example, we're overriding ingestion limits for a single tenant.
overrides:
  "user1":
    ingestion_burst_size: 350000
    ingestion_rate: 350000
    max_global_series_per_metric: 300000
    max_global_series_per_user: 300000
```
<!-- prettier-ignore-end -->

Run the overrides-exporter by providing the `-target`, and `-runtime-config.file` flags:

```
mimir -target=overrides-exporter -runtime-config.file=runtime.yaml
```

After the overrides-exporter starts, you can to use `curl` to inspect the tenant overrides:

```bash
curl -s http://localhost:8080/metrics | grep cortex_limits_overrides
```

The output metrics look similar to the following:

```console
# HELP cortex_limits_overrides Resource limit overrides applied to tenants
# TYPE cortex_limits_overrides gauge
cortex_limits_overrides{limit_name="ingestion_burst_size",user="user1"} 350000
cortex_limits_overrides{limit_name="ingestion_rate",user="user1"} 350000
cortex_limits_overrides{limit_name="max_global_series_per_metric",user="user1"} 300000
cortex_limits_overrides{limit_name="max_global_series_per_user",user="user1"} 300000
```

With these metrics, you can set up alerts to know when tenants are close to hitting their limits
before they exceed them.
