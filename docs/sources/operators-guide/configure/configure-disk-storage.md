---
title: "Configure Grafana Fire disk storage"
menuTitle: "Configure disk storage"
description: ""
weight: 20
---

# Configure Grafana Fire disk storage

Grafana Fire ingesters require disk space for storing received profiles as [blocks]({{<relref "">}}).
Blocks can then be uploaded to [object storage]({{<relref "../architecture/block-format.md">}}) for durability.

Because the disk is used for reading and writing profiles you should provision one with enough input/output throughput capabilities.
In Kubernetes this means configuring the adequate [storage class](https://kubernetes.io/docs/concepts/storage/storage-classes/) for you claimed volumes.

The configuration below show an example on how to select to which folder blocks should be written too using `data_path`:

```yaml
firedb:
  data_path: /data/firedb
  max_block_duration: 30m
```

Typically blocks are flushed to disk once they reach 1GB of in-memory size, but you can also provide a upper time limit using `max_block_duration`.

Ingesters will ensure there's enough disk space and start deleting old blocks once the volume provided is under **10GB and 5%** of free space.
