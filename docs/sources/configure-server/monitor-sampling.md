---
description: Use Pyroscope metrics to confirm that write-path sampling is active and to measure how much data it drops, by tenant and usage group.
menuTitle: Monitor sampling
title: Monitor write-path sampling
weight: 350
keywords:
  - Pyroscope
  - sampling
  - metrics
  - monitoring
  - usage groups
---

# Monitor write-path sampling

After you configure write-path sampling, you can use metrics from the Pyroscope distributor to confirm that sampling is active and to measure how much data it drops. This page describes the metrics to watch and shows example queries, broken down by tenant and usage group.

To configure sampling, refer to [Configure write-path sampling](/docs/pyroscope/<PYROSCOPE_VERSION>/configure-server/configure-sampling/).

For how sampling works and what happens to sampled-out profiles, refer to [Write-path sampling](/docs/pyroscope/<PYROSCOPE_VERSION>/reference-pyroscope-v2-architecture/sampling/) in the v2 architecture reference.

## Before you begin

- Run the Pyroscope v2 architecture with sampling configured for at least one tenant. Refer to [Configure write-path sampling](/docs/pyroscope/<PYROSCOPE_VERSION>/configure-server/configure-sampling/).
- Scrape metrics from the Pyroscope distributor with Prometheus or a compatible agent. Pyroscope exposes metrics at the `/metrics` endpoint on the HTTP listen port (`4040` by default).
- The distributor makes the sampling decision, so these metrics come from the distributor. In microservices mode, scrape all distributor instances. The `sum()` aggregation in the example queries already combines metrics from all instances, so the queries work in both monolithic and microservices mode.

The example queries use PromQL and assume that Pyroscope metrics carry the `pyroscope_` prefix from the source code. Your deployment might add labels such as `job`, `namespace`, or `pod`. Add those labels to the queries to scope results to one Pyroscope cluster.

## Sampling metrics

The following diagram shows where the distributor emits each metric relative to the sampling decision. Understanding this flow helps you interpret the PromQL queries in later sections.

{{< mermaid >}}
flowchart LR
    A["Profile arrives"] --> B["Ingest limit check"]
    B --> C{"Sampling decision"}
    C -->|"Sampled out"| D["discarded_bytes_total\ndiscarded_samples_total\nusage_group_discarded_bytes_total"]
    C -->|"Kept"| E["Validation and normalization"]
    E --> F["usage_group_received_decompressed_total"]
    F --> G["Forward to segment-writers"]
{{< /mermaid >}}

Because `pyroscope_usage_group_received_decompressed_total` only counts bytes that pass the sampling decision, you must add the discarded bytes back to calculate the total volume that reached the distributor.

Sampling reuses the distributor's discard metrics with the `reason` label value `dropped_by_sampling_rules`.

| Metric | Labels | Description |
| --- | --- | --- |
| `pyroscope_discarded_samples_total` | `reason`, `tenant` | Counts discarded profiles. The counter increments for every sampled-out profile, even when you enable `keep_stripped_profiles`. |
| `pyroscope_discarded_bytes_total` | `reason`, `tenant` | Counts discarded uncompressed bytes. When you disable `keep_stripped_profiles`, this reflects the full profile size. When you enable it, this counts only the stripped stack-trace bytes because the distributor retains the totals. |
| `pyroscope_usage_group_discarded_bytes_total` | `reason`, `tenant`, `usage_group` | Same as `pyroscope_discarded_bytes_total`, broken down by usage group. Profiles that don't match any usage group use `usage_group="other"`. |
| `pyroscope_usage_group_received_decompressed_total` | `type`, `tenant`, `usage_group` | Counts received uncompressed bytes that survive sampling, by profile type, tenant, and usage group. Use this metric as the denominator when you calculate the fraction of data that sampling drops. |

## Confirm that sampling is active

The clearest signal that sampling drops data is a nonzero rate of `pyroscope_discarded_samples_total` for the `dropped_by_sampling_rules` reason. This counter increments for every sampled-out profile, whether or not you retain totals with `keep_stripped_profiles`.

```promql
sum(rate(pyroscope_discarded_samples_total{reason="dropped_by_sampling_rules"}[5m]))
```

The result is the number of profiles per second that sampling drops across all tenants. A value above zero confirms that sampling is active.

If the query returns no data or stays at zero when you expect drops, then sampling isn't matching any profiles. Confirm that your usage groups and probabilities are correct. Refer to [Troubleshoot sampling](/docs/pyroscope/<PYROSCOPE_VERSION>/configure-server/configure-sampling/#troubleshoot-sampling).

{{< admonition type="note" >}}
When you enable `keep_stripped_profiles`, `pyroscope_discarded_bytes_total` can stay low even though sampling is active because the distributor retains the profile totals and only the stripped stack-trace bytes count as discarded. Use `pyroscope_discarded_samples_total` to confirm sampling in that case. The distributor marks the retained series with the `__sampled__="true"` label. For details, refer to [Retain sampled-out profiles](/docs/pyroscope/<PYROSCOPE_VERSION>/reference-pyroscope-v2-architecture/sampling/#retain-sampled-out-profiles).
{{< /admonition >}}

## Measure the volume that sampling drops

To measure how much data sampling removes, use `pyroscope_discarded_bytes_total` for the `dropped_by_sampling_rules` reason.

```promql
sum(rate(pyroscope_discarded_bytes_total{reason="dropped_by_sampling_rules"}[5m]))
```

The result is the number of uncompressed bytes per second that sampling drops across all tenants.

### Calculate the drop fraction

To express the drop as a fraction of the data that Pyroscope receives, compare the discarded bytes to the received bytes. Because `pyroscope_usage_group_received_decompressed_total` only counts the bytes that survive sampling, add the discarded bytes back to get the total that reached the sampling stage.

```promql
sum(rate(pyroscope_usage_group_discarded_bytes_total{reason="dropped_by_sampling_rules"}[5m]))
/
(
  sum(rate(pyroscope_usage_group_received_decompressed_total[5m]))
  +
  sum(rate(pyroscope_usage_group_discarded_bytes_total{reason="dropped_by_sampling_rules"}[5m]))
)
```

The result is the fraction of ingested bytes that sampling drops. A value of `0` means nothing dropped and a value of `1` means everything dropped.

### Break down by tenant

Add `sum by (tenant)` to attribute dropped volume to each tenant.

```promql
sum by (tenant) (rate(pyroscope_discarded_bytes_total{reason="dropped_by_sampling_rules"}[5m]))
```

The result is one series per tenant, showing the uncompressed bytes per second that sampling drops for that tenant.

### Break down by usage group

Use `pyroscope_usage_group_discarded_bytes_total` to attribute dropped volume to each usage group within a tenant.

```promql
sum by (tenant, usage_group) (rate(pyroscope_usage_group_discarded_bytes_total{reason="dropped_by_sampling_rules"}[5m]))
```

The result is one series per tenant and usage group, showing the uncompressed bytes per second that sampling drops for that group.

To calculate the fraction of each usage group's data that sampling drops, divide the discarded bytes by the total that reached the sampling stage for the same group.

```promql
sum by (tenant, usage_group) (rate(pyroscope_usage_group_discarded_bytes_total{reason="dropped_by_sampling_rules"}[5m]))
/
(
  sum by (tenant, usage_group) (rate(pyroscope_usage_group_received_decompressed_total[5m]))
  +
  sum by (tenant, usage_group) (rate(pyroscope_usage_group_discarded_bytes_total{reason="dropped_by_sampling_rules"}[5m]))
)
```

The result is one series per tenant and usage group, showing the fraction of that group's ingested bytes that sampling drops. Compare this value against the probability you set for the group in [Configure write-path sampling](/docs/pyroscope/<PYROSCOPE_VERSION>/configure-server/configure-sampling/). A group with probability `0.1` keeps about 10% of its profiles, so it drops close to 90% of the matching volume.

If the observed drop fraction diverges significantly from the configured probability, check whether usage group matchers classify profiles as you expect. Profiles that don't match any group appear under `usage_group="other"` and aren't sampled. For more troubleshooting steps, refer to [Troubleshoot sampling](/docs/pyroscope/<PYROSCOPE_VERSION>/configure-server/configure-sampling/#troubleshoot-sampling).

## Alert on dropped volume

Sampling behavior changes as your traffic and probabilities change. You can turn the drop-fraction query into a Prometheus alerting rule and alert when a group drops more or less data than you expect.

The following example alerts when a tenant and usage group drops more than 95% of its ingested bytes over 10 minutes, which can indicate a probability that's set too low:

```yaml
groups:
  - name: pyroscope-sampling
    rules:
      - alert: PyroscopeSamplingDropHigh
        expr: |
          sum by (tenant, usage_group) (rate(pyroscope_usage_group_discarded_bytes_total{reason="dropped_by_sampling_rules"}[10m]))
          /
          (
            sum by (tenant, usage_group) (rate(pyroscope_usage_group_received_decompressed_total[10m]))
            +
            sum by (tenant, usage_group) (rate(pyroscope_usage_group_discarded_bytes_total{reason="dropped_by_sampling_rules"}[10m]))
          )
          > 0.95
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Write-path sampling drops over 95% of usage group {{ $labels.usage_group }} for tenant {{ $labels.tenant }}"
```

Set the threshold to match the probabilities you configure. A group with a low probability is expected to drop most of its volume, so alert only when the fraction moves outside the range that you plan for.

You can also detect when sampling unexpectedly stops, for example, after a failed config reload that resets the probability to `1.0`. The following alert fires when no profiles are sampled out for a group that you expect to be sampled:

```yaml
      - alert: PyroscopeSamplingNotActive
        expr: |
          sum by (tenant, usage_group) (rate(pyroscope_usage_group_discarded_bytes_total{reason="dropped_by_sampling_rules"}[10m])) == 0
          and
          sum by (tenant, usage_group) (rate(pyroscope_usage_group_received_decompressed_total[10m])) > 0
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "No sampling drops detected for usage group {{ $labels.usage_group }} in tenant {{ $labels.tenant }}"
```

## Next steps

- Adjust per-group probabilities. Refer to [Configure write-path sampling](/docs/pyroscope/<PYROSCOPE_VERSION>/configure-server/configure-sampling/).
- Retain totals for sampled-out profiles. Refer to [Retain sampled-out profiles](/docs/pyroscope/<PYROSCOPE_VERSION>/reference-pyroscope-v2-architecture/sampling/#retain-sampled-out-profiles).
