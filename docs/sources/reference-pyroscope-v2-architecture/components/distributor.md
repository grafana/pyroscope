---
title: "Pyroscope v2 distributor"
menuTitle: "Distributor"
description: "The distributor receives profiling data and routes it to segment-writers."
weight: 10
keywords:
  - Pyroscope v2
  - distributor
  - ingestion
---

# Pyroscope v2 distributor

The distributor is a stateless component that serves as the entry point for the ingestion path. It receives profiling data from agents and routes it to [segment-writers](../segment-writer/) for storage.

## Profile routing

Unlike v1 where profiles are routed to ingesters based on hash ring token distribution, the v2 distributor routes profiles to segment-writers based on the profile's `service_name` label. This co-location strategy ensures that profiles from the same application are stored together, which is crucial for:

- **Query performance**: Profiles likely to be queried together are stored in the same blocks
- **Compaction efficiency**: Related data can be compacted more effectively
- **Storage optimization**: Reduces the number of objects needed to satisfy typical queries

## Distribution algorithm

The distributor uses a three-step process to determine where to place a profile:

1. **Tenant shards**: Find suitable locations from the total shards using the `tenant_id`.
1. **Dataset shards**: Narrow down to locations suitable for the `service_name` label.
1. **Final placement**: Select the exact shard using consistent hashing or adaptive load balancing.

This algorithm balances data locality with even distribution across the cluster.

For detailed information about the distribution algorithm, refer to [Data distribution](../../data-distribution/).

## Validation

The distributor cleans and validates data before sending it to segment-writers:

- Ensures profiles have timestamps set (defaults to receive time if missing)
- Removes samples with zero values
- Sums samples that share the same stacktrace

If a request contains invalid data, the distributor returns a 400 HTTP status code with details in the response body.

## Retain sampled-out profiles

When a profile is sampled out, the distributor drops it by default, which leaves gaps in the affected time series.
To keep the totals for these profiles, set the `keep_stripped_profiles` limit for the tenant (default `false`).

When enabled, the distributor reduces a sampled-out profile to totals instead of dropping it:

- Samples are collapsed to a single totals sample, so the profile totals stay accurate.
- Stacktraces are removed, because sampled-out profiles don't contribute to stack-based views such as flame graphs.
- Sample labels are removed, so span- and trace-attributed breakdowns aren't available for these totals.
- Each retained series is marked with the `__sampled__="true"` label.

Whether these retained profiles appear in query results is controlled separately on the read path. Refer to [Query sampled-out profiles](/docs/pyroscope/<PYROSCOPE_VERSION>/reference-pyroscope-v2-architecture/components/query-backend/#query-sampled-out-profiles).

To configure this limit, refer to [`keep_stripped_profiles`](/docs/pyroscope/<PYROSCOPE_VERSION>/configure-server/reference-configuration-parameters/#limits) in the configuration reference.

## Load balancing

Randomly load balance write requests across distributor instances. If you're running Pyroscope in a Kubernetes cluster, you can define a Kubernetes [Service](https://kubernetes.io/docs/concepts/services-networking/service/) as ingress for the distributors.

The distributor discovers segment-writers through memberlist-based ring discovery, which maintains the list of available segment-writer instances.

## Stateless design

The distributor is completely stateless and disk-less:

- Requires no local storage
- Scales horizontally by adding more instances
- Allows instances to be added or removed without data migration
- Supports deployment in ephemeral containers
