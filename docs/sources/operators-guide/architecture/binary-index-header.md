---
title: "Grafana Mimir binary index-header"
menuTitle: "Binary index-header"
description: "The binary index-header contains information that the store-gateway uses at query time."
weight: 40
---

# Grafana Mimir binary index-header

To query series inside blocks from object storage, the [store-gateway]({{< relref "components/store-gateway.md" >}}) must obtain information about each block index.
To obtain the required information, the store-gateway builds an index-header for each block and stores it on local disk.

The store-gateway uses `GET byte range request` to build the index-header, which contains specific sections of the block's index. The store-gateway uses the index-header at query time.

Because downloading specific sections of the original block's index is a computationally easy operation, the index-header is not uploaded to the object storage.
If the index-header is not available on local disk, store-gateway instances (or the same instance after a rolling update completes without a persistent disk) re-build the index-header from the original block's index.

## Format (version 1)

The index-header is a subset of the block index and contains:

- [Symbol Table](https://github.com/prometheus/prometheus/blob/master/tsdb/docs/format/index.md#symbol-table): Used to unintern string values
- [Posting Offset Table](https://github.com/prometheus/prometheus/blob/master/tsdb/docs/format/index.md#postings-offset-table): Used to look up postings

The following example shows the format of the index-header file that is located in each block store-gateway local directory. It is terminated by a table of contents that serves as an entry point into the index.

```
┌─────────────────────────────┬───────────────────────────────┐
│    magic(0xBAAAD792) <4b>   │      version(1) <1 byte>      │
├─────────────────────────────┬───────────────────────────────┤
│  index version(2) <1 byte>  │ index PostingOffsetTable <8b> │
├─────────────────────────────┴───────────────────────────────┤
│ ┌─────────────────────────────────────────────────────────┐ │
│ │      Symbol Table (exact copy from original index)      │ │
│ ├─────────────────────────────────────────────────────────┤ │
│ │      Posting Offset Table (exact copy from index)       │ │
│ ├─────────────────────────────────────────────────────────┤ │
│ │                          TOC                            │ │
│ └─────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```
