---
description: Learn how to configure write-path sampling to reduce the volume of profiles Pyroscope stores.
menuTitle: Configure sampling
title: Configure write-path sampling
weight: 300
keywords:
  - Pyroscope
  - sampling
  - usage groups
  - keep_stripped_profiles
---

# Configure write-path sampling

Write-path sampling lets the distributor drop a fraction of ingested profiles per tenant to reduce the volume of data that Pyroscope stores. Sampling is server-side and independent of the sample rate that your profiler uses when it collects stack traces.

For how sampling works and what happens to sampled-out profiles, refer to [Write-path sampling](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/reference-pyroscope-v2-architecture/sampling/) in the v2 architecture reference.

{{< admonition type="tip" >}}
If you send profiles to Grafana Cloud, you can use [Adaptive Profiles](https://grafana.com/docs/grafana-cloud/observe-and-act/adaptive-telemetry/adaptive-profiles/) instead of configuring write-path sampling yourself. Adaptive Profiles is a managed feature that samples every service at a reduced rate by default and boosts services to full resolution on demand.
{{< /admonition >}}

## Before you begin

- Run the Pyroscope v2 architecture. Write-path sampling applies to the v2 distributor.
- Enable runtime configuration so that you can set per-tenant overrides. Refer to [Runtime configuration and per-tenant overrides](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-server/about-configurations/#runtime-configuration-and-per-tenant-overrides).

## Configure sampling

You configure sampling per tenant through runtime overrides. First enable usage groups to classify incoming profiles, then set a sampling probability for one or more usage groups.

The probability is the fraction of matching profiles that the distributor keeps. A value of `1.0` keeps all profiles, and a lower value keeps that fraction and samples out the rest.

The following example enables a usage group for every service in `tenant-a`, then keeps 50% of `service-a` profiles and 10% of `service-b` profiles:

```yaml
overrides:
  "tenant-a":
    distributor_usage_groups:
      service/${labels.service_name}: "{}"
    distributor_sampling:
      usage_groups:
        service/service-a:
          probability: 0.5
        service/service-b:
          probability: 0.1
```

When a profile matches more than one usage group, the lowest probability applies.

By default, the distributor drops sampled-out profiles, which leaves gaps in the affected time series. To keep totals for sampled-out profiles instead, set the `keep_stripped_profiles` limit for the tenant. For details, refer to [Retain sampled-out profiles](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/reference-pyroscope-v2-architecture/sampling/#retain-sampled-out-profiles).

## Troubleshoot sampling

Here are some common issues and how to troubleshoot them.

### Sampling doesn't take effect

Sampling only applies to profiles that match a configured usage group. Profiles that don't match any usage group in `distributor_sampling` are always kept.

- Confirm that `distributor_usage_groups` classifies the profiles that you want to sample.
- Confirm that the group names under `distributor_sampling.usage_groups` match the usage group names.
- Allow up to `-runtime-config.reload-period` (default `10s`) for changes to the runtime configuration file to take effect.

### Sampled-out profiles don't return an error

Sampling isn't the same as ingest-limit throttling. When a tenant hits an ingestion limit, the distributor returns HTTP 429 for rejected requests. Sampled-out profiles don't produce that error. When `keep_stripped_profiles` is `false`, the distributor drops them without a client-visible error.
