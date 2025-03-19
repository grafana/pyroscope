---
title: "Configure disk storage"
menuTitle: "Configure disk storage"
description: "Learn about the ingester component and how to configure disk storage for Pyroscope."
aliases:
  - /docs/phlare/latest/configure-server/configure-disk-storage/
  - ../configure-disk-storage/ # https://grafana.com/docs/pyroscope/latest/configure-server/configure-disk-storage/
---

# Configure disk storage

The [ingester] component in Grafana Pyroscope processes the received profiling data.
First it keeps the data organized in memory, in the so-called "head block".
Once the size of the head block exceeds a threshold or the head block is older than
`-pyroscopedb.max-block-duration` (by default 1 hour), the ingester writes
the block to the local persistent disk.
Refer to [block format] for more detail about the block's layout.

Each of those blocks are identified by an [ULID] and stored
within Pyroscope's data path `-pyroscopedb.data-path=` (by default
`./data`).
This directory is organized by the following:

* `./<tenant-id>`: Each tenant has its own subdirectory with the following subdirectories:
   * `head/<block-id>`: Contains the current data still being written.
   * `local/<block-id>`: Contains the finished blocks, which are kept locally

## Object storage

When [object storage is configured][object-store], finished blocks are
uploaded to the object store bucket.

## High disk utilization

To avoid losing the most recent data, Pyroscope removes the oldest blocks
when it detects that the volume on which the data path is located is close to
running out of disk space.
The check for high disk utilization runs every
`-pyroscopedb.retention-policy-enforcement-interval` when:

* less than `-pyroscopedb.retention-policy-min-disk-available-percentage=0.05` of the total size of the volume is available and
* the available disk space smaller then `-pyroscopedb.retention-policy-min-free-disk-gb=10`.

The deletion is logged like this:

```
level=warn caller=pyroscopedb.go:231 ts=2022-10-05T13:19:09.770693308Z msg="disk utilization is high, deleted oldest block" path=data/anonymous/local/01GDZYHKKKY2ANY6PCJJZGT1N8
```

[block format]: ../../../reference-pyroscope-architecture/block-format/
[object-store]: ../configure-object-storage-backend/
[ULID]: https://github.com/ulid/spec
[ingester]: ../../../reference-pyroscope-architecture/components/ingester/
