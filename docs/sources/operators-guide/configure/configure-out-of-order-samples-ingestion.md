---
description: Learn how to configure Grafana Mimir to handle out-of-order samples ingestion.
menuTitle: Configure out-of-order samples ingestion
title: Configure out-of-order samples ingestion
weight: 120
---

# Configure out-of-order samples ingestion

If you have out-of-order samples, due to the nature of your architecture or the system that you are observing, then you can configure Grafana Mimir to set an out-of-order time-window threshold for how old samples can be ingested.

As an **experimental** feature, Mimir allows you to ingest out-of-order samples. As a result, no sample is dropped if it is within the configured time window.

## Configure out-of-order samples ingestion instance-wide

To configure Mimir to accept out-of-order samples, see the following configuration snippet:

```yaml
limits:
  # Allow ingestion of out-of-order samples up to 5 minutes since the latest received sample for the series.
  out_of_order_time_window: 5m
```

## Configure out-of-order samples per tenant

If your Mimir has multitenancy enabled, you can still use the preceding method to set a default out-of-order time window threshold for all tenants.
If a particular tenant needs a custom threshold, you can use the runtime configuration to set a per-tenant override.

1. Enable [runtime configuration]({{< relref "about-runtime-configuration.md" >}}).
1. Add an override for the tenant that needs a custom out-of-order time window:

```yaml
overrides:
  tenant1:
    out_of_order_time_window: 2h
  tenant2:
    out_of_order_time_window: 30m
```

Setting `out_of_order_time_window` to `0s` disables the out-of-order ingestion while you can still continue to query the out-of-order samples ingested till now.

## Query caching with out-of-order ingestion enabled

Once a query has been cached, out-of-order samples that get ingested later can potentially change those query results.

To avoid caching queries that can get outdated, you can set `-query-frontend.max-cache-freshness` to match the `out_of_order_time_window` so that you don't cache queries
for the time window where you still expect samples to arrive. Doing so can increase the load on your Mimir cluster depending on query characteristics.

## Recording rules when out-of-order ingestion is enabled

Similar to the problem above with query caching, the samples recorded via the recording rules can get outdated with new out-of-order samples being ingested.
So you should expect some difference in results if you happen to run the raw query of the recording rule. The difference highly depends on your out-of-order ingestion pattern.

If you happen to have a shorter `out_of_order_time_window`, say less than 10 minutes, then you can use `-ruler.evaluation-delay-duration` to delay your rule evaluation up to that time.

## Understand out-of-order

Previously, Mimir and Prometheus TSDB had a couple of rules over what timestamps are accepted.

The moment that a new series sample arrives, Mimir needs to determine if the series already exists, and whether or not the sample is too old:

- If the series exists within the Head block of the TSDB, the incoming sample must have a newer timestamp than the latest sample that is stored for the series. Otherwise, the ingesters consider it to be out-of-order.
- If the series does not exist, then the sample has to be within bounds, which go back 1 hour from TSDB's head-block max time (when using 2 hour block range). If it fails to be within bounds, then the ingesters consider it to be out-of-bounds.

The experimental out-of-order ingestion helps fix both the issues.

> **Note:** If you're writing metrics using Prometheus remote write or the Grafana Agent, then out-of-order samples are unexpected.
> Prometheus and Grafana Agent guarantee that samples are written in-order for the same series.
