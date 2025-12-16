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

This algorithm balances data locality with even distribution across the cluster. Distributors are aware of availability zones and route profiles to segment-writers in the same zone to avoid cross-AZ traffic penalties.

For detailed information about the distribution algorithm, refer to [Data distribution](../../data-distribution/).

## Validation

The distributor cleans and validates data before sending it to segment-writers:

- Ensures profiles have timestamps set (defaults to receive time if missing)
- Removes samples with zero values
- Sums samples that share the same stacktrace

If a request contains invalid data, the distributor returns a 400 HTTP status code with details in the response body.

## Load balancing

Randomly load balance write requests across distributor instances. If you're running Pyroscope in a Kubernetes cluster, you can define a Kubernetes [Service](https://kubernetes.io/docs/concepts/services-networking/service/) as ingress for the distributors.

The distributor discovers segment-writers through memberlist-based ring discovery, which maintains the list of available segment-writer instances.

## Stateless design

The distributor is completely stateless and disk-less:

- Requires no local storage
- Scales horizontally by adding more instances
- Allows instances to be added or removed without data migration
- Supports deployment in ephemeral containers
