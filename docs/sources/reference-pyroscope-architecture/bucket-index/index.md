---
aliases:
  - ../../operators-guide/architecture/bucket-index/
description: The bucket index enhances query performance.
menuTitle: Bucket index
title: Grafana Pyroscope bucket index
weight: 50
---

# Grafana Pyroscope bucket index

The bucket index is a per-tenant file that contains the list of blocks and block deletion marks in the storage. The bucket index is stored in the backend object storage, is periodically updated by the compactor, and used by store-gateways to discover blocks in the storage.

## Benefits

The [store-gateway]({{< relref "../components/store-gateway" >}}) must have an almost[^1] up-to-date view of the storage bucket, in order to find the right blocks to look up at query time and to load a block.

Because of this, they need to periodically scan the bucket to look for new blocks uploaded by ingesters or compactors, and blocks deleted (or marked for deletion) by compactors.

When the bucket index is enabled, store-gateways periodically look up the per-tenant bucket index instead of scanning the bucket via `list objects` operations.

This provides the following benefits:

1. Reduced number of API calls to the object storage by store-gateway
1. No "list objects" storage API calls performed by store-gateway

## Structure of the index

The `bucket-index.json.gz` contains:

- **`blocks`**<br />
  List of complete blocks of a tenant, including blocks marked for deletion. Partial blocks are excluded from the index.
- **`block_deletion_marks`**<br />
  List of block deletion marks.
- **`updated_at`**<br />
  A Unix timestamp, with precision measured in seconds, displays the last time index was updated and written to the storage.

## How it gets updated

The [compactor]({{< relref "../components/compactor" >}}) periodically scans the bucket and uploads an updated bucket index to the storage.
You can configure the frequency with which the bucket index is updated via `-compactor.cleanup-interval`.

The use of the bucket index is optional, but the index is built and updated by the compactor even if `-blocks-storage.bucket-store.bucket-index.enabled=false`.
This behavior ensures that the bucket index for any tenant exists and that query result consistency is guaranteed if a Grafana Pyroscope cluster operator enables the bucket index in a live cluster.
The overhead introduced by keeping the bucket index updated is not significant.

## How it's used by the store-gateway

The [store-gateway]({{< relref "../components/store-gateway" >}}), at startup and periodically, fetches the bucket index for each tenant that belongs to its shard, and uses it as the source of truth for the blocks and deletion marks in the storage. This removes the need to periodically scan the bucket to discover blocks belonging to its shard.

[^1]:
    Ingesters regularly add new blocks to the bucket as they offload data to long-term storage,
    and compactors subsequently compact these blocks and mark the original blocks for deletion.
    Actual deletion happens after the delay value that is associated with the parameter `-compactor.deletion-delay`.
    An attempt to fetch a deleted block will lead to failure of the query.
    Therefore, in this context, an _almost up-to-date_ view is a view that’s outdated by less than the value of `-compactor.deletion-delay`.
