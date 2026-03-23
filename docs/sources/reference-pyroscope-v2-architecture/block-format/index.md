---
title: "Block format"
menuTitle: "Block format"
description: "Learn how Pyroscope v2 stores profiling data in object storage."
weight: 40
keywords:
  - Pyroscope v2
  - block format
  - object storage
  - datasets
---

# Block format

In Pyroscope v2, a block is a single object in object storage (`block.bin`) containing data from one or more _datasets_. Each dataset holds profiling data for a specific service and includes its own TSDB index, symbol data, and profile tables. Block metadata — stored in the [metastore](../components/metastore/) and embedded in the object itself — describes the datasets, their labels, and byte offsets within the object.

## Object storage layout

Segments (level 0, not yet compacted) and compacted blocks are stored in separate top-level directories. Segments are not yet split by tenant and use an anonymous tenant directory. After compaction, blocks are organized by tenant:

```
segments/
  {shard}/
    anonymous/
      {block_id}/
        block.bin

blocks/
  {shard}/
    {tenant}/
      {block_id}/
        block.bin

dlq/
  {shard}/
    {tenant}/
      {block_id}/
        block.bin
```

## Block structure

Each `block.bin` object contains a sequence of datasets followed by a metadata footer:

```
Offset    | Content
----------|-------------------------------------------
0         | Dataset 0 data
          | Dataset 1 data
          | ...
          | Dataset N data
          | Protobuf-encoded block metadata
end-8     | uint32 (big-endian): raw metadata size
end-4     | uint32 (big-endian): CRC32 of metadata + size
```

## Datasets

A dataset is a self-contained region within the block that stores profiling data for a specific service. Each dataset contains:

* A [TSDB index](https://ganeshvernekar.com/blog/prometheus-tsdb-persistent-block-and-its-index/) mapping series labels to profiles
* Symbol data (`symbols.symdb`) for stack traces and function names
* A [Parquet](https://parquet.apache.org/docs/) table of profile samples

Datasets are annotated with labels (such as `service_name` and `profile_type`) that allow the query path to select only the relevant datasets without reading the entire block.

A separate tenant-wide dataset index allows queries that don't target a specific service to locate the relevant datasets.

## Block metadata

Block metadata is a protobuf-encoded structure that describes the block's contents:

* Block ID ([ULID](https://github.com/ulid/spec)), tenant, shard, compaction level, and time range
* A list of datasets with their byte offsets (table of contents), labels, and sizes
* A string table for deduplicating strings across the metadata entry

The metadata is stored both in the [metastore](../components/metastore/) index and embedded in the block object itself.
