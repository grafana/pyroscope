---
title: "Configure Grafana Phlare disk storage"
menuTitle: "Configure disk storage"
description: ""
weight: 20
---

# Configure Grafana Phlare disk storage

Grafana Phlare's [ingester] component processes the received profiling data.
First it keeps the data organized in memory, in the so called head block. Once
the size of it exceeds a threshold or the head block is older than
`-phlaredb.max-block-duration` (by default 3 hours), it will write the block to
the local persistent disk (see [block format] for more detail about the block's
layout). Each of those blocks are identified by an [ULID] and stored within
Grafana Phlare's data path `-phlaredb.data-path=` (by default
`./data`) is organized the following:

* `./<tenant-id>`: Each tenant has its own subdirectory with the following subdirectories:
   * `head/<block-id>`: Contains the current data still being written.
   * `local/<block-id>`: Contains the finished blocks, which are kept locally

## Object storage

When an [object storage is configured][object-store], finished blocks are
uploaded to the object store bucket.

## High disk utilization

To avoid loosing the most recent data, Grafana Phlare will remove the oldest
blocks  when it detects that the volume on which the data path is located is
close to running out of disk. This high utilization mode will be active when:

Less than 5% of the total size of the volume is available and that is also
smaller then 10GiB.

The deletion will be logged like this:

```
level=warn caller=phlaredb.go:231 ts=2022-10-05T13:19:09.770693308Z msg="disk utilization is high, deleted oldest block" path=data/anonymous/local/01GDZYHKKKY2ANY6PCJJZGT1N8
```

[block format]: {{< relref "../reference-phlare-architecture/block-format/" >}}
[object-store]: {{< relref "./configure-object-storage-backend.md" >}}
[ULID]: https://github.com/ulid/spec
[ingester]: {{< relref "../reference-phlare-architecture/components/ingester.md" >}}
