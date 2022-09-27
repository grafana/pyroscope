---
aliases:
  - /docs/mimir/latest/operators-guide/configuring/about-versioning/
description: Learn about guarantees for this Grafana Mimir major release.
menuTitle: About versioning
title: About Grafana Mimir versioning
weight: 50
---

# About Grafana Mimir versioning

This topic describes our guarantees for this Grafana Mimir major release.

## Flags, configuration, and minor version upgrades

Upgrading Grafana Mimir from one minor version to the next minor version should work, but we don't want to bump the major version every time we remove a configuration parameter.
We will keep deprecated flags and YAML configuration parameters in place for two minor releases.
You can use the `deprecated_flags_inuse_total` metric to generate an alert that helps you determine if you're using a deprecated flag.

These guarantees don't apply to [experimental features](#experimental-features).

## Reading old data

The Grafana Mimir maintainers commit to ensuring that future versions can read data written by versions within the last two years.
In practice, we expect to be able to read data written more than two years ago, but a minimum of two years is our guarantee.

## API Compatibility

Grafana Mimir strives to be 100% compatible with the Prometheus HTTP API which is by default served by endpoints with the /prometheus HTTP path prefix `/prometheus/*`.

We consider any deviation from this 100% API compatibility to be a bug, except for the following scenarios:

- Additional API endpoints for creating, removing, modifying alerts, and recording rules.
- Additional APIs that push metrics (under `/prometheus/api/push`).
- Additional API endpoints for management of Grafana Mimir, such as the ring. These APIs are not included in any compatibility guarantees.
- [Delete series API](https://prometheus.io/docs/prometheus/latest/querying/api/#delete-series).

## Experimental features

Grafana Mimir is an actively developed project and we encourage the introduction of new features and capabilities.
Not everything in each release of Grafana Mimir is considered production-ready.
We mark as "Experimental" all features and flags that we don't consider production-ready.

We do not guarantee backwards compatibility for experimental features and flags.
Experimental configuration and flags are subject to change.

The following features are currently experimental:

- Ruler
  - Tenant federation
  - Use query-frontend for rule evaluation
- Distributor
  - Metrics relabeling
  - Request rate limit
    - `-distributor.request-rate-limit`
    - `-distributor.request-burst-limit`
  - OTLP ingestion path
- Exemplar storage
  - `-ingester.max-global-exemplars-per-user`
  - `-ingester.exemplars-update-period`
  - API endpoint `/api/v1/query_exemplars`
- Hash ring
  - Disabling ring heartbeat timeouts
    - `-distributor.ring.heartbeat-timeout=0`
    - `-ingester.ring.heartbeat-timeout=0`
    - `-ruler.ring.heartbeat-timeout=0`
    - `-alertmanager.sharding-ring.heartbeat-timeout=0`
    - `-compactor.ring.heartbeat-timeout=0`
    - `-store-gateway.sharding-ring.heartbeat-timeout=0`
  - Disabling ring heartbeats
    - `-distributor.ring.heartbeat-period=0`
    - `-ingester.ring.heartbeat-period=0`
    - `-ruler.ring.heartbeat-period=0`
    - `-alertmanager.sharding-ring.heartbeat-period=0`
    - `-compactor.ring.heartbeat-period=0`
    - `-store-gateway.sharding-ring.heartbeat-period=0`
  - Exclude ingesters running in specific zones (`-ingester.ring.excluded-zones`)
- Memberlist
  - Cluster label support
    - `-memberlist.cluster-label`
    - `-memberlist.cluster-label-verification-disabled`
- Ingester
  - Add variance to chunks end time to spread writing across time (`-blocks-storage.tsdb.head-chunks-end-time-variance`)
  - Snapshotting of in-memory TSDB data on disk when shutting down (`-blocks-storage.tsdb.memory-snapshot-on-shutdown`)
  - Out-of-order samples ingestion (`-ingester.out-of-order-allowance`)
- Query-frontend
  - `-query-frontend.querier-forget-delay`
  - Instant query splitting (`-query-frontend.split-instant-queries-by-interval`)
- Query-scheduler
  - `-query-scheduler.querier-forget-delay`
  - Ring-based service discovery (`-query-scheduler.service-discovery-mode` and `-query-scheduler.ring.*`)
  - Max number of used instances (`-query-scheduler.max-used-instances`)
- Store-gateway
  - `-blocks-storage.bucket-store.index-header.map-populate-enabled`
- Blocks Storage, Alertmanager, and Ruler support for partitioning access to the same storage bucket
  - `-alertmanager-storage.storage-prefix`
  - `-blocks-storage.storage-prefix`
  - `-ruler-storage.storage-prefix`
- Compactor
  - HTTP API for uploading TSDB blocks
- Anonymous usage statistics tracking
- Read-write deployment mode
- `/api/v1/user_limits` API endpoint

## Deprecated features

The following features are currently deprecated:

- Ingester:
  - `active_series_custom_trackers` YAML config parameter in the ingester block. The configuration has been moved to limit config, the ingester config will be removed in version 2.4.0.
