---
title: "Grafana Mimir store-gateway"
menuTitle: "Store-gateway"
description: "The store-gateway queries blocks from long-term storage."
weight: 70
---

# Grafana Mimir store-gateway

The store-gateway component, which is stateful, queries blocks from [long-term storage]({{< relref "../about-grafana-mimir-architecture/index.md#long-term-storage" >}}).
On the read path, the [querier]({{< relref "querier.md" >}}) and the [ruler]({{< relref "ruler/index.md" >}}) use the store-gateway when handling the query, whether the query comes from a user or from when a rule is being evaluated.

To find the right blocks to look up at query time, the store-gateway requires an almost up-to-date view of the bucket in long-term storage.
The store-gateway keeps the bucket view updated using one of the following options:

- Periodically downloading the [bucket index]({{< relref "../bucket-index/index.md" >}}) (default)
- Periodically scanning the bucket

### Bucket index enabled (default)

To discover each tenant's blocks and block deletion marks, at startup, store-gateways fetch the [bucket index]({{< relref "../bucket-index/index.md" >}}) from long-term storage for each tenant that belongs to their [shard](#blocks-sharding-and-replication).

For each discovered block, the store-gateway downloads the [index header](#blocks-index-header) to the local disk.
During this initial bucket-synchronization phase, the store-gateway’s `/ready` readiness probe endpoint reports a not-ready status.

For more information about the bucket index, refer to [bucket index]({{< relref "../bucket-index/index.md" >}}).

Store-gateways periodically re-download the bucket index to obtain an updated view of the long-term storage and discover new blocks uploaded by ingesters and compactors, or deleted by compactors.

It is possible that the compactor might have deleted blocks or marked others for deletion since the store-gateway last checked the block.
The store-gateway downloads the index header for new blocks, and offloads (deletes) the local copy of index header for deleted blocks.
You can configure the `-blocks-storage.bucket-store.sync-interval` flag to control the frequency with which the store-gateway checks for changes in the long-term storage.

When a query executes, store-gateway downloads chunks, but it does not fully download the whole block; the store-gateway downloads only the portions of index and chunks that are required to run a given query.
To avoid the store-gateway having to re-download the index header during subsequent restarts, we recommend running the store-gateway with a persistent disk.
For example, if you're running the Grafana Mimir cluster in Kubernetes, you can use a StatefulSet with a PersistentVolumeClaim for the store-gateways.

For more information about the index-header, refer to [Binary index-header documentation]({{< relref "../binary-index-header.md" >}}).

### Bucket index disabled

When the bucket index is disabled, the overall workflow is still nearly the same.
The difference occurs during the discovery phase of the blocks at startup, and during the periodic checks.
Iterations over the entire long-term storage download `meta.json` metadata files while filtering out blocks that don't belong to tenants in their shard.

## Blocks sharding and replication

The store-gateway uses blocks sharding to horizontally scale blocks in a large cluster.

Blocks are replicated across multiple store-gateway instances based on a replication factor configured via `-store-gateway.sharding-ring.replication-factor`.
The blocks replication is used to protect from query failures caused by some blocks not loaded by any store-gateway instance at a given time, such as in the event of a store-gateway failure or while restarting a store-gateway instance (for example, during a rolling update).

Store-gateway instances build a [hash ring]({{< relref "../hash-ring/index.md" >}}) and shard and replicate blocks across the pool of store-gateway instances registered in the ring.

Store-gateways continuously monitor the ring state.
When the ring topology changes, for example, when a new instance is added or removed, or the instance becomes healthy or unhealthy, each store-gateway instance resynchronizes the blocks assigned to its shard.
The store-gateway resynchronization process uses the block ID hash that matches the token ranges assigned to the instance within the ring.

The store-gateway loads the index-header of each block that belongs to its store-gateway shard.
After the store-gateway loads a block’s index header, the block is ready to be queried by queriers.
When the querier queries blocks via a store-gateway, the response contains the list of queried block IDs.
If a querier attempts to query a block that the store-gateway has not loaded, the querier retries the query on a different store-gateway up to the `-store-gateway.sharding-ring.replication-factor` value, which by default is `3`.
The query fails if the block can't be successfully queried from any replica.

> **Note:** You must configure the [hash ring]({{< relref "../hash-ring/index.md" >}}) via the `-store-gateway.sharding-ring.*` flags or their respective YAML configuration parameters.

### Sharding strategy

The store-gateway uses shuffle-sharding to divide the blocks of each tenant across a subset of store-gateway instances.

> **Note:** When shuffle-sharding is in use, the number of store-gateway instances that load the blocks of a tenant is limited, which means that the blast radius of issues introduced by the tenant's workload is confined to its shard instances.

The `-store-gateway.tenant-shard-size` flag (or their respective YAML configuration parameters) determines the default number of store-gateway instances per tenant.
The `store_gateway_tenant_shard_size` in the limits overrides can override the shard size on a per-tenant basis.

The default `-store-gateway.tenant-shard-size` value is 0, which means that tenant's blocks are sharded across all store-gateway instances.

For more information about shuffle sharding, refer to [configure shuffle sharding]({{< relref "../../configure/configuring-shuffle-sharding/index.md" >}}).

### Auto-forget

Store-gateways include an auto-forget feature that they can use to unregister an instance from another store-gateway's ring when a store-gateway does not properly shut down.
Under normal conditions, when a store-gateway instance shuts down, it automatically unregisters from the ring. However, in the event of a crash or node failure, the instance might not properly unregister, which can leave a spurious entry in the ring.

The auto-forget feature works as follows: when an healthy store-gateway instance identifies an instance in the ring that is unhealthy for longer than 10 times the configured `-store-gateway.sharding-ring.heartbeat-timeout` value, the healthy instance removes the unhealthy instance from the ring.

### Zone-awareness

Store-gateway replication optionally supports [zone-awareness]({{< relref "../../configure/configuring-zone-aware-replication.md" >}}). When you enable zone-aware replication and the blocks replication factor is greater than 1, each block is replicated across store-gateway instances located in different availability zones.

**To enable zone-aware replication for the store-gateways**:

1. Configure the availability zone for each store-gateway via the `-store-gateway.sharding-ring.instance-availability-zone` CLI flag or its respective YAML configuration parameter.
1. Enable blocks zone-aware replication via the `-store-gateway.sharding-ring.zone-awareness-enabled` CLI flag or its respective YAML configuration parameter.
   Set this zone-aware replication flag on store-gateways, queriers, and rulers.
1. To apply the new configuration, roll out store-gateways, queriers, and rulers.

### Waiting for stable ring at startup

If a cluster cold starts or scales up to two or more store-gateway instances simultaneously, the store-gateways could start at different times. As a result, the store-gateway runs the initial blocks synchronization based on a different state of the hash ring.

For example, in the event of a cold start, the first store-gateway that joins the ring might load all blocks because the sharding logic runs based on the current state of the ring, which contains one single store-gateway.

To reduce the likelihood of store-gateways starting at different times, you can configure the store-gateway to wait for a stable ring at startup. A ring is considered stable when no instance is added or removed from the ring for the minimum duration specified in the `-store-gateway.sharding-ring.wait-stability-min-duration` flag. If the ring continues to change after reaching the maximum duration specified in the `-store-gateway.sharding-ring.wait-stability-max-duration` flag, the store-gateway stops waiting for a stable ring and proceeds starting up.

To enable waiting for the ring to be stable at startup, start the store-gateway with `-store-gateway.sharding-ring.wait-stability-min-duration=1m`, which is the recommended value for production systems.

## Blocks index-header

The [index-header]({{< relref "../binary-index-header.md" >}}) is a subset of the block index that the store-gateway downloads from long-term storage and keeps on the local disk.
Keeping the index-header on the local disk makes query execution faster.

### Index-header lazy loading

By default, a store-gateway downloads the index-headers to disk and doesn't load them to memory until required.
When required by a query, index-headers are memory-mapped and automatically released by the store-gateway after the amount of inactivity time you specify in `-blocks-storage.bucket-store.index-header-lazy-loading-idle-timeout` has passed.

Grafana Mimir provides a configuration flag `-blocks-storage.bucket-store.index-header-lazy-loading-enabled=false` to disable index-header lazy loading.
When disabled, the store-gateway memory-maps all index-headers, which provides faster access to the data in the index-header.
However, in a cluster with a large number of blocks, each store-gateway might have a large amount of memory-mapped index-headers, regardless of how frequently they are used at query time.

## Caching

The store-gateway supports the following type of caches:

- [Index cache](#index-cache)
- [Chunks cache](#chunks-cache)
- [Metadata cache](#metadata-cache)

We recommend that you use caching in a production environment.
For more information about configuring the cache, refer to [production tips]({{< relref "../../run-production-environment/production-tips/index.md#caching" >}}).

### Index cache

The store-gateway can use a cache to accelerate series and label lookups from block indexes. The store-gateway supports the following backends:

- `inmemory`
- `memcached`

#### In-memory index cache

By default, the `inmemory` index cache is enabled.

Consider the following trade-offs of using the in-memory index cache:

- Pros: There is no latency.
- Cons: When the replication factor is > 1, then the data that resides in the memory of the store-gateway will be duplicated among different instances. This leads to an increase in overall memory usage and a reduced cache hit ratio.

You can configure the index cache max size using the `-blocks-storage.bucket-store.index-cache.inmemory.max-size-bytes` flag or its respective YAML configuration parameter.

#### Memcached index cache

The `memcached` index cache uses [Memcached](https://memcached.org/) as the cache backend.

Consider the following trade-offs of using the Memcached index cache:

- Pros: You can scale beyond a single node's memory by creating a Memcached cluster, that is shared by multiple store-gateway instances.
- Cons: The system experiences higher latency in the cache round trip compared to the latency experienced when using in-memory cache.

The Memcached client uses a jump hash algorithm to shard cached entries across a cluster of Memcached servers.
Because the memcached client uses a jump hash algorithm, ensure that memcached servers are not located behind a load balancer, and configure the address of the memcached servers so that servers are added to or removed from the end of the list whenever a scale up or scale down occurs.

For example, if you're running Memcached in Kubernetes, you might:

1. Deploy your Memcached cluster using a [StatefulSet](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/).
1. Create a [headless service](https://kubernetes.io/docs/concepts/services-networking/service/#headless-services) for Memcached StatefulSet.
1. Configure the Mimir's Memcached client address using the `dnssrvnoa+` [service discovery]({{< relref "../../configure/about-dns-service-discovery.md" >}}).

**To configure the Memcached backend**:

1. Use `-blocks-storage.bucket-store.index-cache.backend=memcached`.
1. Use the `-blocks-storage.bucket-store.index-cache.memcached.addresses` flag to set the address of the Memcached service.

[DNS service discovery]({{< relref "../../configure/about-dns-service-discovery.md" >}}) resolves the addresses of the Memcached servers.

### Chunks cache

The store-gateway can also use a cache to store [chunks]({{< relref "../../reference-glossary.md#chunk" >}}) that are fetched from long-term storage.
Chunks contain actual samples, and can be reused if a query hits the same series for the same time range.
Chunks can only be cached in Memcached.

To enable chunks cache, set `-blocks-storage.bucket-store.chunks-cache.backend=memcached`.
You can configure the Memcached client via flags that include the prefix `-blocks-storage.bucket-store.chunks-cache.memcached.*`.

> **Note:** There are additional low-level flags that begin with the prefix `-blocks-storage.bucket-store.chunks-cache.*` that you can use to configure chunks cache.

### Metadata cache

Store-gateways and [queriers]({{< relref "querier.md" >}}) can use memcached to cache the following bucket metadata:

- List of tenants
- List of blocks per tenant
- Block `meta.json` existence and content
- Block `deletion-mark.json` existence and content
- Tenant `bucket-index.json.gz` content

Using the metadata cache reduces the number of API calls to long-term storage and eliminates API calls that scale linearly as the number of querier and store-gateway replicas increases.

To enable metadata cache, set `-blocks-storage.bucket-store.metadata-cache.backend`.

> **Note**: Currently, only `memcached` backend is supported. The Memcached client includes additional configuration available via flags that begin with the prefix `-blocks-storage.bucket-store.metadata-cache.memcached.*`.

Additional flags for configuring metadata cache begin with the prefix `-blocks-storage.bucket-store.metadata-cache.*`. By configuring TTL to zero or a negative value, caching of given item type is disabled.

> **Note:** The same memcached backend cluster should be shared between store-gateways and queriers.\_

## Store-gateway HTTP endpoints

- `GET /store-gateway/ring`<br />
  Displays the status of the store-gateways ring, including the tokens owned by each store-gateway and an option to remove (or forget) instances from the ring.

## Store-gateway configuration

For more information about store-gateway configuration, refer to [store_gateway]({{< relref "../../configure/reference-configuration-parameters/index.md#store_gateway" >}}).
