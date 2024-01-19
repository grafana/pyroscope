---
title: "Configure Pyroscope disk storage"
menuTitle: "Configure disk storage"
description: ""
weight: 30
aliases:
  - /docs/phlare/latest/configure-server/configure-disk-storage/
---

# Configure Pyroscope disk storage

Pyroscope's [ingester] component processes the received profiling data.
First it keeps the data organized in memory, in the so-called "head block". Once
the size of the head block exceeds a threshold or the head block is older than
`-pyroscopedb.max-block-duration` (by default 3 hours), the ingester will write
the block to the local persistent disk (see [block format] for more detail about
the block's layout). Each of those blocks are identified by an [ULID] and stored
within Grafana Pyroscope's data path `-pyroscopedb.data-path=` (by default
`./data`). This directory is organized by the following:

* `./<tenant-id>`: Each tenant has its own subdirectory with the following subdirectories:
   * `head/<block-id>`: Contains the current data still being written.
   * `local/<block-id>`: Contains the finished blocks, which are kept locally

## Object storage

When [object storage is configured][object-store], finished blocks are
uploaded to the object store bucket.

## High disk utilization

To avoid losing the most recent data, Pyroscope will remove the oldest blocks
when it detects that the volume on which the data path is located is close to
running out of disk. This high utilization mode will be active every
`-pyroscopedb.retention-policy-enforcement-interval` when:

* less than `-pyroscopedb.retention-policy-min-disk-available-percentage=0.05` of the total size of the volume is available and
* the available disk space smaller then `-pyroscopedb.retention-policy-min-free-disk-gb=10`.

The deletion will be logged like this:

```
level=warn caller=pyroscopedb.go:231 ts=2022-10-05T13:19:09.770693308Z msg="disk utilization is high, deleted oldest block" path=data/anonymous/local/01GDZYHKKKY2ANY6PCJJZGT1N8
```

[block format]: {{< relref "../reference-pyroscope-architecture/block-format/" >}}
[object-store]: {{< relref "./configure-object-storage-backend.md" >}}
[ULID]: https://github.com/ulid/spec
[ingester]: {{< relref "../reference-pyroscope-architecture/components/ingester.md" >}}
