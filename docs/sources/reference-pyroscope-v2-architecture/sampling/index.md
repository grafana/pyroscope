---
title: "Write-path sampling"
menuTitle: "Sampling"
description: "Learn how Pyroscope v2 samples ingested profiles and how to retain or query sampled-out totals."
weight: 55
keywords:
  - Pyroscope v2
  - sampling
  - sampled-out profiles
  - keep_stripped_profiles
  - include_stripped_profiles
---

# Write-path sampling

The [distributor](/docs/pyroscope/<PYROSCOPE_VERSION>/reference-pyroscope-v2-architecture/components/distributor/) can drop a fraction of ingested profiles to reduce the volume of data Pyroscope stores. This write-path sampling is server-side and independent of the sample rate your profiler uses when it collects stack traces.

When the distributor samples a profile out, it drops the profile by default. That leaves gaps in the affected time series. You can keep the totals for those profiles and choose whether time-series queries include them.

## Why sampling matters

Sampling reduces storage and query cost when a tenant or service produces more profiling data than you need at full resolution. Dropping sampled-out profiles is the cheapest option, but the time series then has missing points.

If you need accurate totals for cost or capacity views, keep the sampled-out profiles as totals-only data. Stack traces are dropped so the size reduction still applies. Flame graphs built from those stripped profiles are empty.

## How sampling works

The distributor applies per-tenant sampling rules after it validates an incoming profile. When a profile matches a rule, the distributor makes a probabilistic decision to accept or drop it.

{{< mermaid >}}
flowchart TD
    A[Profile ingested] --> B[Distributor validates profile]
    B --> C{Matches a sampling rule?}
    C -->|No| K[Forward to segment-writers]
    C -->|Yes| D{Probabilistic decision}
    D -->|Kept| K
    D -->|Sampled out| E{keep_stripped_profiles}
    E -->|false, default| F[Drop profile]
    E -->|true| G["Strip to totals, add __sampled__=&quot;true&quot;"]
    G --> H[Store totals-only series]
{{< /mermaid >}}

- If the profile is accepted, the distributor forwards it to [segment-writers](/docs/pyroscope/<PYROSCOPE_VERSION>/reference-pyroscope-v2-architecture/components/segment-writer/) as usual.
- If the profile is sampled out, the distributor drops it by default. To keep totals for sampled-out profiles instead, refer to [Retain sampled-out profiles](#retain-sampled-out-profiles).

This is not the same as ingest-limit throttling. When a tenant hits an ingestion limit, the distributor returns HTTP 429 for rejected requests. Sampled-out profiles don't produce that error.

You configure sampling per tenant through runtime overrides, using usage groups and a per-group probability. To configure sampling, refer to [Configure write-path sampling](/docs/pyroscope/<PYROSCOPE_VERSION>/configure-server/configure-sampling/). To confirm that sampling is active and measure the volume it drops, refer to [Monitor write-path sampling](/docs/pyroscope/<PYROSCOPE_VERSION>/configure-server/monitor-sampling/).

## Retain sampled-out profiles

Set the `keep_stripped_profiles` limit for the tenant to keep totals instead of dropping sampled-out profiles. The default is `false`.

When the limit is enabled, the distributor reduces each sampled-out profile to a single totals sample:

- Sample values are summed so the series totals stay accurate.
- Stack traces and symbols are removed to keep the size reduction. Flame graphs for these profiles are empty.
- Sample labels are removed, so span- and trace-attributed breakdowns aren't available for these totals.
- The series is marked with the `__sampled__="true"` label.

To configure this limit, refer to [`keep_stripped_profiles`](/docs/pyroscope/<PYROSCOPE_VERSION>/configure-server/reference-configuration-parameters/#limits) in the configuration reference.

Whether these retained profiles appear in query results is controlled separately on the read path.

## Query sampled-out profiles

When the distributor retains sampled-out profiles, the [query-backend](/docs/pyroscope/<PYROSCOPE_VERSION>/reference-pyroscope-v2-architecture/components/query-backend/) stores them as totals-only series marked with `__sampled__="true"`.

The query-backend handles these series differently depending on the query:

- Stack-based queries, such as flame graph, tree, pprof, and heatmap, always exclude `__sampled__` series. Those series have no stack traces to contribute.
- Time-series queries include `__sampled__` series only when the `include_stripped_profiles` limit is enabled for the querying tenant. The default is `false`. For a multi-tenant query, these series are included only when the setting is enabled for every tenant in the query.

To configure this limit, refer to [`include_stripped_profiles`](/docs/pyroscope/<PYROSCOPE_VERSION>/configure-server/reference-configuration-parameters/#limits) in the configuration reference.

## Related sampling concepts

Write-path sampling is one of several sampling ideas in the Pyroscope ecosystem:

- **Profiler sample rate**: How often a language SDK or profiler collects stack traces. Refer to the [language SDK](/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/language-sdks/) documentation for the language you instrument.
- **Scrape-target sampling**: How Grafana Alloy profiles a subset of scrape targets. Refer to [Sampling scrape targets](/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/grafana-alloy/sampling/).
