---
title: "Configure Grafana Fire profiles storage retention"
menuTitle: "Configure profiles storage retention"
description: "Learn how to configure Grafana Fire profiles storage retention."
weight: 70
---

# Configure Grafana Fire profiles storage retention

Grafana Fire stores the profiles in a object storage.

By default, profiles that are stored in the object storage are never deleted, and the storage utilization will increase over time.
You can configure the object storage retention to automatically delete all of the profiles data older than the configured period.

## Configure the storage retention

The [compactor]({{< relref "../architecture/components/compactor/index.md" >}}) is the Fire component that is responsible for enforcing the storage retention.
To configure the storage retention, set the CLI flag `-compactor.blocks-retention-period` or change the following YAML configuration:

```yaml
limits:
  # Delete from storage profiles data older than 1 year.
  compactor_blocks_retention_period: 1y
```

To configure the storage retention on a per-tenant basis, set overrides in the [runtime configuration]({{< relref "about-runtime-configuration.md" >}}):

```yaml
overrides:
  tenant1:
    # Delete from storage tenant1's profiles data older than 1 year.
    compactor_blocks_retention_period: 1y
  tenant2:
    # Delete from storage tenant2's profiles data older than 2 years.
    compactor_blocks_retention_period: 2y
  tenant3:
    # Disable retention for tenant3's profiles (never delete its data).
    compactor_blocks_retention_period: 0
```

## Per-series retention

Grafana Fire doesn’t support per-series deletion and retention, nor does it support Prometheus' [Delete series API](https://prometheus.io/docs/prometheus/latest/querying/api/#delete-series).
